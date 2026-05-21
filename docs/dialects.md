# SQL Dialect Support

jsonlogic2sql supports multiple SQL dialects, generating appropriate syntax for each target database.

## Supported Dialects

| Dialect | Constant | Status |
|---------|----------|--------|
| Google BigQuery | `DialectBigQuery` | Fully Supported |
| Google Cloud Spanner | `DialectSpanner` | Fully Supported |
| PostgreSQL | `DialectPostgreSQL` | Fully Supported |
| DuckDB | `DialectDuckDB` | Fully Supported |
| ClickHouse | `DialectClickHouse` | Fully Supported |

## Usage

```go
schema, _ := jsonlogic2sql.NewSchema(nil) // literal-only expressions

// Create transpiler with specific dialect
transpiler, err := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery, schema)

// Or use convenience functions
sql, err := jsonlogic2sql.TranspileCondition(jsonlogic2sql.DialectPostgreSQL, schema, jsonLogic)
```

## Operator Compatibility by Dialect

All JSON Logic operators are supported across all dialects. The library generates appropriate SQL syntax for each.

| Operator Category | Operators | BigQuery | Spanner | PostgreSQL | DuckDB | ClickHouse |
|-------------------|-----------|:--------:|:-------:|:----------:|:------:|:----------:|
| **Data Access** | `var`, `missing`, `missing_some` | ✓ | ✓ | ✓ | ✓ | ✓ |
| **Comparison** | `==`, `===`, `!=`, `!==`, `>`, `>=`, `<`, `<=` | ✓ | ✓ | ✓ | ✓ | ✓ |
| **Logical** | `and`, `or`, `!`, `!!`, `if` | ✓ | ✓ | ✓ | ✓ | ✓ |
| **Numeric** | `+`, `-`, `*`, `/`, `%`, `max`, `min` | ✓ | ✓ | ✓ | ✓ | ✓ |
| **Array** | `in`, `map`, `filter`, `reduce`, `all`, `some`, `none`, `merge` | ✓ | ✓ | ✓ | ✓ | ✓ |
| **String** | `in`, `cat`, `substr` | ✓ | ✓ | ✓ | ✓ | ✓ |

## Identifier Quoting

Path segments that are not valid unquoted SQL identifiers (e.g. start with a digit) are automatically quoted using the dialect-appropriate character:

| Dialect | Quote Character | Example |
|---------|----------------|---------|
| BigQuery | Backtick (`` ` ``) | `` fixture.history.`24h`.events.total `` |
| Spanner | Backtick (`` ` ``) | `` fixture.history.`24h`.events.total `` |
| PostgreSQL | Double quote (`"`) | `fixture.history."24h".events.total` |
| DuckDB | Double quote (`"`) | `fixture.history."24h".events.total` |
| ClickHouse | Backtick (`` ` ``) | `` fixture.history.`24h`.events.total `` |

Segments that only contain letters, digits, and underscores (and don't start with a digit) remain unquoted. The same per-segment quoting is applied inside array lambdas such as `{"var":"24h"}`, inside reduce scopes such as `{"var":"current.24h"}`, and before variable references are passed to custom operators.

## Dialect-Specific SQL Generation

Some operators generate different SQL based on the target dialect:

| Operator | BigQuery | Spanner | PostgreSQL | DuckDB | ClickHouse |
|----------|----------|---------|------------|--------|------------|
| `merge` (arrays/scalars) | `ARRAY_CONCAT(a, [x])` | `ARRAY_CONCAT(a, [x])` | `(a \|\| ARRAY[x])` | `ARRAY_CONCAT(a, [x])` | `arrayConcat(a, [x])` |
| `map` (arrays) | `ARRAY(SELECT ... UNNEST)` | `ARRAY(SELECT ... UNNEST)` | `ARRAY(SELECT ... UNNEST)` | `ARRAY(SELECT ... UNNEST)` | `arrayMap(x -> ..., arr)` |
| `filter` (arrays) | `ARRAY(SELECT ... WHERE)` | `ARRAY(SELECT ... WHERE)` | `ARRAY(SELECT ... WHERE)` | `ARRAY(SELECT ... WHERE)` | `arrayFilter(x -> ..., arr)` |
| `substr` | `SUBSTR(s, i, n)` | `SUBSTR(s, i, n)` | `SUBSTR(s, i, n)` | `SUBSTR(s, i, n)` | `substring(s, i, n)` |
| `in` (array) | `EXISTS ... UNNEST(array)` | `EXISTS ... UNNEST(array)` | `EXISTS ... UNNEST(array)` | `EXISTS ... UNNEST(array)` | `arrayExists(...)` |
| `in` (string) | `STRPOS(h, n) > 0` | `STRPOS(h, n) > 0` | `POSITION(n IN h) > 0` | `STRPOS(h, n) > 0` | `position(h, n) > 0` |

DuckDB `UNNEST` scopes use an explicit column alias, for example
`UNNEST(items) AS elem(elem)`, because DuckDB otherwise exposes the element as
a struct-like `unnest` column. BigQuery, Spanner, and PostgreSQL use the shorter
`UNNEST(items) AS elem` form. ClickHouse uses lambda array functions instead of
`UNNEST`.

PostgreSQL array literals use `ARRAY[...]`. Empty-array value results are
rejected whenever the emitted SQL would contain an untyped `ARRAY[]`, because
PostgreSQL requires an explicit element type and the transpiler does not always
have enough type context. Foldable contexts that do not need to emit the empty
array, such as value fallbacks and empty-array `all`/`some`/`none` predicates,
can still fold normally.

## SQL Function Reference by Dialect

| Function | BigQuery | Spanner | PostgreSQL | DuckDB | ClickHouse |
|----------|----------|---------|------------|--------|------------|
| String position | `STRPOS()` | `STRPOS()` | `POSITION()` | `STRPOS()` | `position()` |
| String concat | `CONCAT()` | `CONCAT()` | `CONCAT()` | `CONCAT()` | `concat()` |
| Substring | `SUBSTR()` | `SUBSTR()` | `SUBSTR()` | `SUBSTR()` | `substring()` |
| Array map | `UNNEST` subquery | `UNNEST` subquery | `UNNEST` subquery | `UNNEST` subquery | `arrayMap()` |
| Array filter | `UNNEST` subquery | `UNNEST` subquery | `UNNEST` subquery | `UNNEST` subquery | `arrayFilter()` |
| Array reduce | `SUM/MIN/MAX` aggregate patterns | `SUM/MIN/MAX` aggregate patterns | `SUM/MIN/MAX` aggregate patterns | `SUM/MIN/MAX` aggregate patterns; `list_reduce()` for arbitrary scalar reducers without nested subqueries | `arrayReduce()` for aggregate patterns; `arrayFold()` for arbitrary reducers |
| Array concat | `ARRAY_CONCAT()` | `ARRAY_CONCAT()` | `\|\|` | `ARRAY_CONCAT()` | `arrayConcat()` |
| Max of values | `GREATEST()` | `GREATEST()` | `GREATEST()` | `GREATEST()` | `greatest()` |
| Min of values | `LEAST()` | `LEAST()` | `LEAST()` | `LEAST()` | `least()` |
| Safe divide | `SAFE_DIVIDE()` | N/A (use CASE) | N/A (use CASE) | N/A (use CASE) | `if()` expression |
| Regex match | `REGEXP_CONTAINS()` | `REGEXP_CONTAINS()` | `~` | `regexp_matches()` | `match()` |

String containment coerces nullable needles with JavaScript-style
stringification (`NULL` becomes `'null'`) and treats empty needles as a match
for any non-null haystack, matching JavaScript `indexOf`. `cat` uses
JSONLogic's join-style stringification instead, so nullable operands are
wrapped with
`COALESCE(value, '')`, and untyped or numeric operands are cast with the
dialect's string cast before `COALESCE`.

Array membership uses null-safe element equality rather than compact dialect
helpers such as `= ANY`, `list_contains`, or `has`, because JSONLogic treats
`null in [null]` as true while SQL nullable equality normally returns
`UNKNOWN`.

## Custom Dialect-Aware Operators

You can create custom operators that generate different SQL per dialect:

```go
transpiler.RegisterDialectAwareOperatorFunc("safeDivide",
    func(op string, args []jsonlogic2sql.OperatorArg, dialect jsonlogic2sql.Dialect) (jsonlogic2sql.OperatorResult, error) {
        numerator := args[0].SQL
        denominator := args[1].SQL

        switch dialect {
        case jsonlogic2sql.DialectBigQuery:
            return jsonlogic2sql.ValueSQL(fmt.Sprintf("SAFE_DIVIDE(%s, %s)", numerator, denominator), jsonlogic2sql.ExpressionTypeNumber), nil
        case jsonlogic2sql.DialectClickHouse:
            return jsonlogic2sql.ValueSQL(fmt.Sprintf("if(%s = 0, NULL, %s / %s)", denominator, numerator, denominator), jsonlogic2sql.ExpressionTypeNumber), nil
        default:
            return jsonlogic2sql.ValueSQL(
                fmt.Sprintf("CASE WHEN %s = 0 THEN NULL ELSE %s / %s END",
                    denominator, numerator, denominator),
                jsonlogic2sql.ExpressionTypeNumber,
            ), nil
        }
    })
```

See [Custom Operators](custom-operators.md#dialect-aware-custom-operators) for more details.

## Parameterized Query Placeholder Styles

When using parameterized queries, the placeholder style varies by dialect:

| Dialect | Style | Example Placeholders |
|---------|-------|---------------------|
| BigQuery | Named | `@p1`, `@p2`, `@p3` |
| Spanner | Named | `@p1`, `@p2`, `@p3` |
| ClickHouse | Named | `@p1`, `@p2`, `@p3` |
| PostgreSQL | Positional | `$1`, `$2`, `$3` |
| DuckDB | Positional | `$1`, `$2`, `$3` |

See [Parameterized Queries](parameterized-queries.md) for detailed documentation and database driver integration examples.

## Unsupported Dialects

### MySQL

MySQL is not supported because it lacks native `UNNEST()` for arrays. This would require complex `JSON_TABLE` workarounds that produce semantically different behavior from other dialects.

## See Also

- [Custom Operators](custom-operators.md) - Create dialect-aware operators
- [Parameterized Queries](parameterized-queries.md) - Bind-parameter output for safe SQL execution
- [API Reference](api-reference.md) - Full API documentation
