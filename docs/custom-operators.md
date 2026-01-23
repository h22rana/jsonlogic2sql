# Custom Operators

You can extend jsonlogic2sql with custom operators to support additional SQL functions like `LENGTH`, `UPPER`, `LOWER`, etc.

## Using a Function

The simplest way to register a custom operator:

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

func main() {
    transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

    // Register a custom "length" operator
    err := transpiler.RegisterOperatorFunc("length", func(op string, args []interface{}) (string, error) {
        if len(args) != 1 {
            return "", fmt.Errorf("length requires exactly 1 argument")
        }
        return fmt.Sprintf("LENGTH(%s)", args[0]), nil
    })
    if err != nil {
        panic(err)
    }

    // Use the custom operator
    sql, _ := transpiler.Transpile(`{"length": [{"var": "email"}]}`)
    fmt.Println(sql) // Output: WHERE LENGTH(email)

    // Use in comparisons
    sql, _ = transpiler.Transpile(`{">": [{"length": [{"var": "email"}]}, 10]}`)
    fmt.Println(sql) // Output: WHERE LENGTH(email) > 10
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

func (u *UpperOperator) ToSQL(operator string, args []interface{}) (string, error) {
    if len(args) != 1 {
        return "", fmt.Errorf("upper requires exactly 1 argument")
    }
    return fmt.Sprintf("UPPER(%s)", args[0]), nil
}

func main() {
    transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

    // Register the handler
    err := transpiler.RegisterOperator("upper", &UpperOperator{})
    if err != nil {
        panic(err)
    }

    sql, _ := transpiler.Transpile(`{"==": [{"upper": [{"var": "name"}]}, "JOHN"]}`)
    fmt.Println(sql) // Output: WHERE UPPER(name) = 'JOHN'
}
```

## Multiple Custom Operators

Register and use multiple custom operators together:

```go
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

transpiler.RegisterOperatorFunc("length", func(op string, args []interface{}) (string, error) {
    return fmt.Sprintf("LENGTH(%s)", args[0]), nil
})

transpiler.RegisterOperatorFunc("upper", func(op string, args []interface{}) (string, error) {
    return fmt.Sprintf("UPPER(%s)", args[0]), nil
})

// Use both in a complex expression
sql, _ := transpiler.Transpile(`{"and": [{">": [{"length": [{"var": "name"}]}, 5]}, {"==": [{"upper": [{"var": "status"}]}, "ACTIVE"]}]}`)
// Output: WHERE (LENGTH(name) > 5 AND UPPER(status) = 'ACTIVE')
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

```go
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

// safeDivide: Division that returns NULL on division by zero
transpiler.RegisterDialectAwareOperatorFunc("safeDivide",
    func(op string, args []interface{}, dialect jsonlogic2sql.Dialect) (string, error) {
        if len(args) != 2 {
            return "", fmt.Errorf("safeDivide requires exactly 2 arguments")
        }
        numerator := args[0].(string)
        denominator := args[1].(string)

        switch dialect {
        case jsonlogic2sql.DialectBigQuery:
            // BigQuery has built-in SAFE_DIVIDE
            return fmt.Sprintf("SAFE_DIVIDE(%s, %s)", numerator, denominator), nil
        case jsonlogic2sql.DialectClickHouse:
            // ClickHouse uses if() expression
            return fmt.Sprintf("if(%s = 0, NULL, %s / %s)", denominator, numerator, denominator), nil
        default:
            // Other dialects use CASE expression
            return fmt.Sprintf("CASE WHEN %s = 0 THEN NULL ELSE %s / %s END",
                denominator, numerator, denominator), nil
        }
    })

sql, _ := transpiler.Transpile(`{"safeDivide": [{"var": "total"}, {"var": "count"}]}`)
// BigQuery: WHERE SAFE_DIVIDE(total, count)
// Spanner:  WHERE CASE WHEN count = 0 THEN NULL ELSE total / count END
```

### Using a Handler Struct for Dialect-Aware Operators

```go
type SafeDivideOperator struct{}

func (s *SafeDivideOperator) ToSQLWithDialect(op string, args []interface{}, dialect jsonlogic2sql.Dialect) (string, error) {
    if len(args) != 2 {
        return "", fmt.Errorf("safeDivide requires exactly 2 arguments")
    }
    numerator := args[0].(string)
    denominator := args[1].(string)

    switch dialect {
    case jsonlogic2sql.DialectBigQuery:
        return fmt.Sprintf("SAFE_DIVIDE(%s, %s)", numerator, denominator), nil
    case jsonlogic2sql.DialectSpanner:
        return fmt.Sprintf("CASE WHEN %s = 0 THEN NULL ELSE %s / %s END",
            denominator, numerator, denominator), nil
    default:
        return "", fmt.Errorf("unsupported dialect: %v", dialect)
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
transpiler.RegisterOperatorFunc("toLower", func(op string, args []interface{}) (string, error) {
    return fmt.Sprintf("LOWER(%s)", args[0]), nil
})

transpiler.RegisterOperatorFunc("toUpper", func(op string, args []interface{}) (string, error) {
    return fmt.Sprintf("UPPER(%s)", args[0]), nil
})

// Custom operators nested inside cat (string concatenation)
sql, _ := transpiler.Transpile(`{"cat": [{"toLower": [{"var": "firstName"}]}, " ", {"toUpper": [{"var": "lastName"}]}]}`)
// Output: WHERE CONCAT(LOWER(firstName), ' ', UPPER(lastName))

// Custom operators nested inside if (conditional)
sql, _ = transpiler.Transpile(`{"if": [{"==": [{"var": "type"}, "premium"]}, {"toUpper": [{"var": "name"}]}, {"toLower": [{"var": "name"}]}]}`)
// Output: WHERE CASE WHEN type = 'premium' THEN UPPER(name) ELSE LOWER(name) END

// Custom operators inside and/or (logical operators)
sql, _ = transpiler.Transpile(`{"and": [{"==": [{"toLower": [{"var": "status"}]}, "active"]}, {">": [{"var": "amount"}, 100]}]}`)
// Output: WHERE (LOWER(status) = 'active' AND amount > 100)
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
WHERE (SAFE_DIVIDE(revenue, cost) > 1.5 AND status IN ('active', 'pending') AND (region LIKE 'US%' OR priority >= 5) AND category LIKE '%premium%')
```

**Spanner/PostgreSQL/DuckDB Output:**
```sql
WHERE (CASE WHEN cost = 0 THEN NULL ELSE revenue / cost END > 1.5 AND status IN ('active', 'pending') AND (region LIKE 'US%' OR priority >= 5) AND category LIKE '%premium%')
```

**ClickHouse Output:**
```sql
WHERE (if(cost = 0, NULL, revenue / cost) > 1.5 AND status IN ('active', 'pending') AND (region LIKE 'US%' OR priority >= 5) AND category LIKE '%premium%')
```

## See Also

- [SQL Dialects](dialects.md) - Dialect-specific SQL generation
- [API Reference](api-reference.md) - Full API documentation
- [Examples](examples.md) - More examples
