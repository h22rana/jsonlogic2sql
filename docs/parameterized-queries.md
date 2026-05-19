# Parameterized Queries

jsonlogic2sql can generate SQL with bind parameter placeholders instead of inlined literals, returning the SQL string and a separate list of parameter values. This is the recommended approach for executing generated SQL against a database, as it prevents SQL injection and allows query plan caching.

## Quick Example

```go
schema, _ := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
    {Name: "status", Type: jsonlogic2sql.FieldTypeString},
    {Name: "email", Type: jsonlogic2sql.FieldTypeString},
    {Name: "amount", Type: jsonlogic2sql.FieldTypeNumber},
})
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery, schema)

sql, params, err := transpiler.TranspileParameterizedCondition(
    `{"and": [{"==": [{"var": "status"}, "active"]}, {">": [{"var": "amount"}, 1000]}]}`,
)
// sql    = "(status = @p1 AND amount > @p2)"
// params = [{Name: "p1", Value: "active"}, {Name: "p2", Value: 1000}]
```

For value-producing expressions, use `TranspileParameterizedValue`:

```go
sql, params, err := transpiler.TranspileParameterizedValue(
    `{"cat": ["Order ", {"var": "status"}]}`,
)
// sql    = "CONCAT(@p1, COALESCE(status, ''))"
// params = [{Name: "p1", Value: "Order "}]
```

## Placeholder Styles by Dialect

The placeholder style is determined by the dialect:

| Dialect | Style | Example |
|---------|-------|---------|
| BigQuery | Named | `@p1`, `@p2` |
| Spanner | Named | `@p1`, `@p2` |
| ClickHouse | Named | `@p1`, `@p2` |
| PostgreSQL | Positional | `$1`, `$2` |
| DuckDB | Positional | `$1`, `$2` |

### BigQuery

```go
sql, params, _ := jsonlogic2sql.TranspileParameterizedCondition(
    jsonlogic2sql.DialectBigQuery,
    schema,
    `{"==": [{"var": "email"}, "alice@example.com"]}`,
)
// sql    = "email = @p1"
// params = [{Name: "p1", Value: "alice@example.com"}]
```

### PostgreSQL

```go
sql, params, _ := jsonlogic2sql.TranspileParameterizedCondition(
    jsonlogic2sql.DialectPostgreSQL,
    schema,
    `{"==": [{"var": "email"}, "alice@example.com"]}`,
)
// sql    = "email = $1"
// params = [{Name: "p1", Value: "alice@example.com"}]
```

## What Gets Parameterized

| Value Type | Parameterized? | Output |
|------------|:--------------:|--------|
| Strings | Yes | `@p1` with bound value |
| Numbers (int, float) | Yes | `@p1` with bound value |
| `NULL` | No | `NULL` (structural SQL token) |
| `TRUE` / `FALSE` | No | `TRUE` / `FALSE` (structural SQL tokens) |
| Column names (`var`) | No | Column name as-is |

NULL and boolean values remain inline because they are structural SQL tokens used in `IS NULL`, `IS TRUE` patterns, not user-supplied data.

## LIKE Patterns with Parameters

When building `LIKE` patterns in parameterized mode, placeholders must stay as SQL expressions, not string literals.

Correct pattern construction:

```sql
WHERE name LIKE CONCAT(@p1, '%')
```

Incorrect pattern construction:

```sql
WHERE name LIKE '@p1%'
```

Why this matters:
- `CONCAT(@p1, '%')` keeps `@p1` as a bind parameter and appends `%` safely in SQL.
- `'@p1%'` turns the placeholder into a plain string literal, so no binding occurs for that pattern value.

The same rule applies to positional styles:
- PostgreSQL/DuckDB: `LIKE CONCAT($1, '%')`
- Prefix/suffix patterns: `LIKE CONCAT('%', @p1, '%')`, `LIKE CONCAT('%', $1)`

## API Reference

### Transpiler Methods

| Method | Description |
|--------|-------------|
| `TranspileParameterizedCondition(jsonLogic string)` | Predicate SQL from a JSON string |
| `TranspileParameterizedConditionFromMap(logic map[string]interface{})` | Predicate SQL from a pre-parsed map |
| `TranspileParameterizedConditionFromInterface(logic interface{})` | Predicate SQL from any interface |
| `TranspileParameterizedValue(jsonLogic string)` | Value SQL expression from a JSON string |
| `TranspileParameterizedValueFromMap(logic map[string]interface{})` | Value SQL expression from a pre-parsed map |
| `TranspileParameterizedValueFromInterface(logic interface{})` | Value SQL expression from any interface |

All methods return `(string, []QueryParam, error)`.

### Package-Level Convenience Functions

Each Transpiler method has a corresponding package-level function that takes a `Dialect` and required `*Schema` as the first arguments:

```go
condition, params, err := jsonlogic2sql.TranspileParameterizedCondition(dialect, schema, jsonLogic)
value, params, err := jsonlogic2sql.TranspileParameterizedValue(dialect, schema, jsonLogic)
// ... plus FromMap and FromInterface variants for each mode
```

### QueryParam Type

```go
type QueryParam struct {
    Name  string      // "p1", "p2", etc.
    Value interface{} // Go native type (string, float64, int64, bool)
}
```

**Value types by input:**

| JSONLogic Input | `Value` Go Type | Notes |
|-----------------|-----------------|-------|
| `"hello"` | `string` | |
| `42` | `float64` | JS-safe integers (within ±2^53−1) |
| `3.14` | `float64` | Floating-point numbers |
| `9223372036854775808` | `string` | Unquoted integers exceeding ±2^53−1 are preserved as exact strings via `json.Decoder.UseNumber` |
| Coerced integer (schema) | `int64` | Schema coerces `"50000"` → `int64(50000)` for integer fields |
| Coerced boolean equality (schema) | — | Schema coerces `1`, `0`, `"1"`, and `"0"` to inline `TRUE`/`FALSE` for boolean fields |
| `true` / `false` | — | Not parameterized (inline `TRUE`/`FALSE`) |
| `null` | — | Not parameterized (inline `NULL`) |
| Integer string `> int64` range | `string` | Quoted string inputs like `"9223372036854775808"` also preserved as string |
| `1e309` (overflow float) | `string` | Out-of-range floats preserved as original string |
| `1e-400` (underflow float) | `string` | Non-zero values that underflow to float64 zero preserved as original string |

> **Note:** JSON decoding uses `json.Decoder.UseNumber()` to preserve full precision for numeric literals. Unquoted integers like `9223372036854775808` produce exact SQL instead of lossy float64 representations. Floats outside the representable float64 range (e.g., `1e309` overflow, `1e-400` underflow to zero) are preserved as their original string to match inline transpilation behavior. Callers binding string-typed numeric params may need to convert them to their driver's numeric type.

## Schema Coercion

When a schema is configured, values are coerced **before** being bound as parameters for comparison operators where JSONLogic coercion can be modeled statically. For example, if a field is declared as `integer` and the JSONLogic contains a string `"50000"`, the bound parameter value will be `int64(50000)`, not the string `"50000"`. Equality comparisons that fold to constants, such as an integer field compared with `"abc"`, do not add placeholder values. Literal-array `in` is an exception: it uses strict `indexOf`-style membership, so mismatched literal members are filtered or fold to `FALSE` instead of being coerced.

```go
schema, err := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
    {Name: "amount", Type: jsonlogic2sql.FieldTypeInteger},
    {Name: "active", Type: jsonlogic2sql.FieldTypeBoolean},
})
if err != nil {
    // handle invalid schema
}
transpiler.SetSchema(schema)

sql, params, _ := transpiler.TranspileParameterizedCondition(
    `{">=": [{"var": "amount"}, "50000"]}`,
)
// sql    = "amount >= @p1"
// params = [{Name: "p1", Value: int64(50000)}]  // coerced from string
```

For equality and inequality, schema-required numeric and boolean coercion happens
before parameter collection:

```go
sql, params, _ = transpiler.TranspileParameterizedCondition(
    `{"and": [{"==": [{"var": "amount"}, "010"]}, {"==": [{"var": "active"}, "0"]}]}`,
)
// sql    = "(amount = @p1 AND active = FALSE)"
// params = [{Name: "p1", Value: int64(10)}]
```

The same equality coercion applies when the field is accessed with a defaulted
`var`; parameters are collected for the default and the coerced literal in SQL
order:

```go
sql, params, _ = transpiler.TranspileParameterizedCondition(
    `{"==": [{"var": ["amount", 0]}, "50"]}`,
)
// sql    = "COALESCE(amount, @p1) = @p2"
// params = [{Name: "p1", Value: int64(0)}, {Name: "p2", Value: int64(50)}]
```

## Using Parameters with Database Drivers

### BigQuery (Go)

```go
sql, params, _ := transpiler.TranspileParameterizedCondition(jsonLogic)

query := client.Query(sql)
for _, p := range params {
    query.Parameters = append(query.Parameters, bigquery.QueryParameter{
        Name:  p.Name,
        Value: p.Value,
    })
}
```

### PostgreSQL (Go - pgx)

```go
sql, params, _ := transpiler.TranspileParameterizedCondition(jsonLogic)

args := make([]interface{}, len(params))
for i, p := range params {
    args[i] = p.Value
}
rows, err := conn.Query(ctx, sql, args...)
```

### ClickHouse (Go - clickhouse-go v2)

The ClickHouse driver uses `{name:Type}` natively, not `@p1`. You can adapt the named parameters:

```go
sql, params, _ := transpiler.TranspileParameterizedCondition(jsonLogic)

// Convert @p1 → {p1:String}, @p2 → {p2:Int64}, etc.
chSQL := sql
chParams := make(clickhouse.Named, len(params))
for _, p := range params {
    placeholder := "@" + p.Name
    switch p.Value.(type) {
    case string:
        chSQL = strings.Replace(chSQL, placeholder, fmt.Sprintf("{%s:String}", p.Name), 1)
    case float64:
        chSQL = strings.Replace(chSQL, placeholder, fmt.Sprintf("{%s:Float64}", p.Name), 1)
    case int64:
        chSQL = strings.Replace(chSQL, placeholder, fmt.Sprintf("{%s:Int64}", p.Name), 1)
    }
    chParams = append(chParams, clickhouse.Named(p.Name, p.Value))
}
```

## Custom Operators

Custom operators receive SQL fragment arguments that may contain placeholders. The contract is the same as with inline mode: custom operators **must** include all provided arguments in their output SQL. Dropping an argument is a semantic bug that, in parameterized mode, additionally triggers an `E350 ErrUnreferencedPlaceholder` error.

Custom operators must also keep placeholders as SQL expressions, not quoted string literals. For example, use `CONCAT(@p1, '%')`, not `'@p1%'`. If a custom operator emits a placeholder inside a quoted SQL string literal, transpilation now fails with `E102 ErrCustomOperatorFailed`.

```go
// Good: all args used
transpiler.RegisterOperatorFunc("double", func(op string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
    return jsonlogic2sql.ValueSQL(fmt.Sprintf("(%s * 2)", args[0].SQL), jsonlogic2sql.ExpressionTypeNumber), nil // @p1 flows through
})

// Bad: dropping args causes E350
transpiler.RegisterOperatorFunc("broken", func(op string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
    return jsonlogic2sql.ValueSQL("42", jsonlogic2sql.ExpressionTypeNumber), nil // discards args containing @p1 -> E350 error
})
```

## Error Handling

The parameterized pipeline returns identical error types and codes as the inline pipeline. One additional error code is specific to parameterized mode:

| Code | Constant | Description |
|------|----------|-------------|
| E350 | `ErrUnreferencedPlaceholder` | A collected bind parameter has no matching placeholder in the generated SQL |

This typically occurs when a custom operator drops an argument. See [Error Handling](error-handling.md) for all error codes.

## Comparison: Inline vs Parameterized

| Input | Inline Condition | Parameterized Condition |
|-------|---------------------|------------------------------------------|
| `{"==": [{"var": "email"}, "alice"]}` | `email = 'alice'` | `email = @p1` + `[{p1, "alice"}]` |
| `{"in": [{"var": "x"}, [1, 2]]}` | `x IN (1, 2)` | `x IN (@p1, @p2)` + `[{p1, 1}, {p2, 2}]` |
| `{"in": [{"var": "x"}, [null, 1]]}` | `(x IS NULL OR x IN (1))` | `(x IS NULL OR x IN (@p1))` + `[{p1, 1}]` |
| `{"==": [{"var": "f"}, null]}` | `f IS NULL` | `f IS NULL` (no params) |
| `{"==": [{"var": "f"}, true]}` | `f = TRUE` | `f = TRUE` (no params) |

## See Also

- [Getting Started](getting-started.md) - Basic usage
- [API Reference](api-reference.md) - Full API documentation
- [Error Handling](error-handling.md) - Error codes and handling
- [Custom Operators](custom-operators.md) - Custom operator registration
