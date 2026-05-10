# Custom Operators

You can extend jsonlogic2sql with custom operators to support additional SQL functions like `LENGTH`, `UPPER`, `LOWER`, etc.

## Naming Rules

Operator names are validated on registration. A valid name must:

- Not be empty or whitespace-only
- Match the pattern `!?[a-zA-Z_][a-zA-Z0-9_]*` - start with a letter or underscore (with an optional `!` prefix for negation operators), followed by letters, digits, or underscores
- Not conflict with a built-in operator name (e.g., `var`, `and`, `in`, `+`)

Valid examples: `length`, `toLower`, `my_op`, `_private`, `!contains`, `!startsWith`

Invalid examples: `""`, `" "`, `1op`, `my-op`, `my op`, `op.name`

```go
// These will return an error:
transpiler.RegisterOperatorFunc("", handler)       // empty name
transpiler.RegisterOperatorFunc("my-op", handler)  // invalid character '-'
transpiler.RegisterOperatorFunc("and", handler)    // built-in operator
```

## Using a Function

The simplest way to register a custom operator is to return a typed `OperatorResult`.
Use `ValueSQL` for scalar/value expressions and `PredicateSQL` for boolean
predicates. This lets `TranspileCondition` reject value-only operators at the
root while still allowing those operators inside comparisons.

Custom operators must use this typed contract. Legacy handlers that return only
a SQL string are no longer accepted because the transpiler cannot infer whether
the SQL is a predicate or a value expression safely.

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

func main() {
    transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

    // Register a custom "length" operator
    err := transpiler.RegisterOperatorFunc("length", func(op string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
        if len(args) != 1 {
            return jsonlogic2sql.OperatorResult{}, fmt.Errorf("length requires exactly 1 argument")
        }
        return jsonlogic2sql.ValueSQL(
            fmt.Sprintf("LENGTH(%s)", args[0].SQL),
            jsonlogic2sql.ExpressionTypeNumber,
        ), nil
    })
    if err != nil {
        panic(err)
    }

    // Use the custom operator
    sql, _ := transpiler.TranspileValue(`{"length": [{"var": "email"}]}`)
    fmt.Println(sql) // Output: LENGTH(email)

    // Use in comparisons
    sql, _ = transpiler.TranspileCondition(`{">": [{"length": [{"var": "email"}]}, 10]}`)
    fmt.Println(sql) // Output: LENGTH(email) > 10
}
```

## Using a Handler Struct

For more complex operators or those that need state, implement the `OperatorHandler` interface:

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

// UpperOperator implements the OperatorHandler interface
type UpperOperator struct{}

func (u *UpperOperator) ToSQL(operator string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
    if len(args) != 1 {
        return jsonlogic2sql.OperatorResult{}, fmt.Errorf("upper requires exactly 1 argument")
    }
    return jsonlogic2sql.ValueSQL(fmt.Sprintf("UPPER(%s)", args[0].SQL), jsonlogic2sql.ExpressionTypeString), nil
}

func main() {
    transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

    // Register the handler
    err := transpiler.RegisterOperator("upper", &UpperOperator{})
    if err != nil {
        panic(err)
    }

    sql, _ := transpiler.TranspileCondition(`{"==": [{"upper": [{"var": "name"}]}, "JOHN"]}`)
    fmt.Println(sql) // Output: UPPER(name) = 'JOHN'
}
```

## Multiple Custom Operators

Register and use multiple custom operators together:

```go
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

transpiler.RegisterOperatorFunc("length", func(op string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
    return jsonlogic2sql.ValueSQL(fmt.Sprintf("LENGTH(%s)", args[0].SQL), jsonlogic2sql.ExpressionTypeNumber), nil
})

transpiler.RegisterOperatorFunc("upper", func(op string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
    return jsonlogic2sql.ValueSQL(fmt.Sprintf("UPPER(%s)", args[0].SQL), jsonlogic2sql.ExpressionTypeString), nil
})

// Use both in a complex expression
sql, _ := transpiler.TranspileCondition(`{"and": [{">": [{"length": [{"var": "name"}]}, 5]}, {"==": [{"upper": [{"var": "status"}]}, "ACTIVE"]}]}`)
// Output: (LENGTH(name) > 5 AND UPPER(status) = 'ACTIVE')
```

## Managing Custom Operators

```go
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

// Check if an operator is registered
if transpiler.HasCustomOperator("length") {
    fmt.Println("length is registered")
}

// List all custom operators
operators := transpiler.ListCustomOperators()
fmt.Println(operators)

// Unregister an operator
transpiler.UnregisterOperator("length")

// Clear all custom operators
transpiler.ClearCustomOperators()
```

## Dialect-Aware Custom Operators

For operators that generate different SQL based on the target dialect:

`var` operands are converted before they reach custom operators, so custom
operators receive dialect-correct SQL column references. For example,
`{"var":"fixture.history.24h.events.total"}` is passed as
``fixture.history.`24h`.events.total`` for BigQuery/Spanner/ClickHouse and as
`fixture.history."24h".events.total` for PostgreSQL/DuckDB.

```go
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

// safeDivide: Division that returns NULL on division by zero
transpiler.RegisterDialectAwareOperatorFunc("safeDivide",
    func(op string, args []jsonlogic2sql.OperatorArg, dialect jsonlogic2sql.Dialect) (jsonlogic2sql.OperatorResult, error) {
        if len(args) != 2 {
            return jsonlogic2sql.OperatorResult{}, fmt.Errorf("safeDivide requires exactly 2 arguments")
        }
        numerator := args[0].SQL
        denominator := args[1].SQL

        switch dialect {
        case jsonlogic2sql.DialectBigQuery:
            // BigQuery has built-in SAFE_DIVIDE
            return jsonlogic2sql.ValueSQL(
                fmt.Sprintf("SAFE_DIVIDE(%s, %s)", numerator, denominator),
                jsonlogic2sql.ExpressionTypeNumber,
            ), nil
        case jsonlogic2sql.DialectClickHouse:
            // ClickHouse uses if() expression
            return jsonlogic2sql.ValueSQL(
                fmt.Sprintf("if(%s = 0, NULL, %s / %s)", denominator, numerator, denominator),
                jsonlogic2sql.ExpressionTypeNumber,
            ), nil
        default:
            // Other dialects use CASE expression
            return jsonlogic2sql.ValueSQL(
                fmt.Sprintf("CASE WHEN %s = 0 THEN NULL ELSE %s / %s END",
                    denominator, numerator, denominator),
                jsonlogic2sql.ExpressionTypeNumber,
            ), nil
        }
    })

sql, _ := transpiler.TranspileValue(`{"safeDivide": [{"var": "total"}, {"var": "count"}]}`)
// BigQuery: SAFE_DIVIDE(total, count)
// Spanner:  CASE WHEN count = 0 THEN NULL ELSE total / count END
```

### Using a Handler Struct for Dialect-Aware Operators

```go
type SafeDivideOperator struct{}

func (s *SafeDivideOperator) ToSQLWithDialect(op string, args []jsonlogic2sql.OperatorArg, dialect jsonlogic2sql.Dialect) (jsonlogic2sql.OperatorResult, error) {
    if len(args) != 2 {
        return jsonlogic2sql.OperatorResult{}, fmt.Errorf("safeDivide requires exactly 2 arguments")
    }
    numerator := args[0].SQL
    denominator := args[1].SQL

    switch dialect {
    case jsonlogic2sql.DialectBigQuery:
        return jsonlogic2sql.ValueSQL(fmt.Sprintf("SAFE_DIVIDE(%s, %s)", numerator, denominator), jsonlogic2sql.ExpressionTypeNumber), nil
    case jsonlogic2sql.DialectSpanner:
        return jsonlogic2sql.ValueSQL(
            fmt.Sprintf("CASE WHEN %s = 0 THEN NULL ELSE %s / %s END",
                denominator, numerator, denominator),
            jsonlogic2sql.ExpressionTypeNumber,
        ), nil
    default:
        return jsonlogic2sql.OperatorResult{}, fmt.Errorf("unsupported dialect: %v", dialect)
    }
}

transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
transpiler.RegisterDialectAwareOperator("safeDivide", &SafeDivideOperator{})
```

## Nested Custom Operators

Custom operators work seamlessly when nested inside any built-in operator:

```go
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

// Register custom operators
transpiler.RegisterOperatorFunc("toLower", func(op string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
    return jsonlogic2sql.ValueSQL(fmt.Sprintf("LOWER(%s)", args[0].SQL), jsonlogic2sql.ExpressionTypeString), nil
})

transpiler.RegisterOperatorFunc("toUpper", func(op string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
    return jsonlogic2sql.ValueSQL(fmt.Sprintf("UPPER(%s)", args[0].SQL), jsonlogic2sql.ExpressionTypeString), nil
})

// Custom operators nested inside cat (string concatenation)
sql, _ := transpiler.TranspileValue(`{"cat": [{"toLower": [{"var": "firstName"}]}, " ", {"toUpper": [{"var": "lastName"}]}]}`)
// Output: CONCAT(LOWER(firstName), ' ', UPPER(lastName))

// Custom operators nested inside if (conditional)
sql, _ = transpiler.TranspileValue(`{"if": [{"==": [{"var": "type"}, "premium"]}, {"toUpper": [{"var": "name"}]}, {"toLower": [{"var": "name"}]}]}`)
// Output: CASE WHEN type = 'premium' THEN UPPER(name) ELSE LOWER(name) END

// Custom operators inside and/or (logical operators)
sql, _ = transpiler.TranspileCondition(`{"and": [{"==": [{"toLower": [{"var": "status"}]}, "active"]}, {">": [{"var": "amount"}, 100]}]}`)
// Output: (LOWER(status) = 'active' AND amount > 100)
```

### Deeply Nested Example

```json
{
  "and": [
    {"==": [{"toLower": [{"var": "status"}]}, "active"]},
    {">": [
      {"reduce": [
        {"filter": [{"var": "items"}, {">": [{"var": ""}, 0]}]},
        {"+": [{"var": "accumulator"}, {"var": "current"}]},
        0
      ]},
      1000
    ]},
    {"!=": [{"substr": [{"toUpper": [{"var": "region"}]}, 0, 2]}, "XX"]}
  ]
}
```

This demonstrates:
- `toLower` nested inside `and` → `==`
- `filter` nested inside `reduce` nested inside `>` nested inside `and`
- `toUpper` nested inside `substr` nested inside `!=` nested inside `and`

## Complex Multi-Condition Example

**JSON Logic:**
```json
{
  "and": [
    {">": [{"safeDivide": [{"var": "revenue"}, {"var": "cost"}]}, 1.5]},
    {"in": [{"var": "status"}, ["active", "pending"]]},
    {"or": [
      {"startsWith": [{"var": "region"}, "US"]},
      {">=": [{"var": "priority"}, 5]}
    ]},
    {"contains": [{"var": "category"}, "premium"]}
  ]
}
```

**BigQuery Output:**
```sql
(SAFE_DIVIDE(revenue, cost) > 1.5 AND status IN ('active', 'pending') AND (region LIKE 'US%' OR priority >= 5) AND category LIKE '%premium%')
```

**Spanner/PostgreSQL/DuckDB Output:**
```sql
(CASE WHEN cost = 0 THEN NULL ELSE revenue / cost END > 1.5 AND status IN ('active', 'pending') AND (region LIKE 'US%' OR priority >= 5) AND category LIKE '%premium%')
```

**ClickHouse Output:**
```sql
(if(cost = 0, NULL, revenue / cost) > 1.5 AND status IN ('active', 'pending') AND (region LIKE 'US%' OR priority >= 5) AND category LIKE '%premium%')
```

## See Also

- [SQL Dialects](dialects.md) - Dialect-specific SQL generation
- [API Reference](api-reference.md) - Full API documentation
- [Examples](examples.md) - More examples
