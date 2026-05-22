# Getting Started

This guide will help you get up and running with jsonlogic2sql.

## Installation

```bash
go get github.com/h22rana/jsonlogic2sql@latest
```

## Prerequisites

- Go 1.25 or later

## Basic Usage

### Simple Example

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

func main() {
    schema, err := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
        {Name: "amount", Type: jsonlogic2sql.FieldTypeNumber},
    })
    if err != nil {
        panic(err)
    }

    sql, err := jsonlogic2sql.TranspileCondition(jsonlogic2sql.DialectBigQuery, schema, `{">": [{"var": "amount"}, 1000]}`)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: amount > 1000
}
```

### Using the Transpiler Instance

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

func main() {
    schema, err := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
        {Name: "status", Type: jsonlogic2sql.FieldTypeString},
        {Name: "amount", Type: jsonlogic2sql.FieldTypeNumber},
        {Name: "failedAttempts", Type: jsonlogic2sql.FieldTypeNumber},
        {Name: "country", Type: jsonlogic2sql.FieldTypeString},
    })
    if err != nil {
        panic(err)
    }

    // Create a transpiler instance
    transpiler, err := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery, schema)
    if err != nil {
        panic(err)
    }

    // From JSON string
    sql, err := transpiler.TranspileCondition(`{"and": [{"==": [{"var": "status"}, "pending"]}, {">": [{"var": "amount"}, 5000]}]}`)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: (status = 'pending' AND amount > 5000)

    // From pre-parsed map
    logic := map[string]interface{}{
        "or": []interface{}{
            map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": "failedAttempts"}, 5}},
            map[string]interface{}{"in": []interface{}{map[string]interface{}{"var": "country"}, []interface{}{"CN", "RU"}}},
        },
    }
    sql, err = transpiler.TranspileConditionFromMap(logic)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: (failedAttempts >= 5 OR country IN ('CN', 'RU'))
}
```

### Condition and Value Output

`TranspileCondition` returns a boolean SQL predicate without the `WHERE`
keyword. Use it for filters and join predicates:

```go
// Returns just the condition without "WHERE"
condition, err := jsonlogic2sql.TranspileCondition(
    jsonlogic2sql.DialectBigQuery,
    schema,
    `{">": [{"var": "amount"}, 1000]}`,
)
// condition = "amount > 1000"

// Use in a custom query
query := fmt.Sprintf("SELECT * FROM orders WHERE %s AND created_at > '2024-01-01'", condition)
```

Use `TranspileValue` when the JSONLogic root produces a scalar or array value:

```go
value, err := jsonlogic2sql.TranspileValue(
    jsonlogic2sql.DialectBigQuery,
    schema,
    `{"or": [false, "fallback"]}`,
)
// value = "'fallback'"
```

## Choosing a Dialect

The library supports multiple SQL dialects. You must specify a dialect when creating a transpiler:

| Dialect | Constant | Description |
|---------|----------|-------------|
| Google BigQuery | `DialectBigQuery` | Google BigQuery SQL |
| Google Cloud Spanner | `DialectSpanner` | Cloud Spanner SQL |
| PostgreSQL | `DialectPostgreSQL` | PostgreSQL SQL |
| DuckDB | `DialectDuckDB` | DuckDB SQL |
| ClickHouse | `DialectClickHouse` | ClickHouse SQL |

```go
schema, _ := jsonlogic2sql.NewSchema(nil) // literal-only expressions

// BigQuery
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery, schema)

// PostgreSQL
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectPostgreSQL, schema)

// ClickHouse
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectClickHouse, schema)
```

### Parameterized Queries

For safer SQL execution, generate SQL with bind parameter placeholders:

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

func main() {
    schema, _ := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
        {Name: "status", Type: jsonlogic2sql.FieldTypeString},
        {Name: "amount", Type: jsonlogic2sql.FieldTypeNumber},
    })
    transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery, schema)

    sql, params, err := transpiler.TranspileParameterizedCondition(
        `{"==": [{"var": "status"}, "active"]}`,
    )
    if err != nil {
        panic(err)
    }
    fmt.Println(sql)    // Output: status = @p1
    fmt.Println(params) // Output: [{p1 active}]

    // Or use the convenience function
    sql, params, err = jsonlogic2sql.TranspileParameterizedCondition(
        jsonlogic2sql.DialectPostgreSQL,
        schema,
        `{">": [{"var": "amount"}, 1000]}`,
    )
    fmt.Println(sql)    // Output: amount > $1
    fmt.Println(params) // Output: [{p1 1000}]
}
```

See [Parameterized Queries](parameterized-queries.md) for detailed documentation.

## Variable Naming

The transpiler preserves JSON Logic variable names in the SQL output, with automatic quoting for segments that are not valid unquoted SQL identifiers:

- Dot notation is preserved: `transaction.amount` → `transaction.amount`
- Nested variables: `user.account.age` → `user.account.age`
- Simple variables remain unchanged: `amount` → `amount`
- Segments outside the portable unquoted ASCII shape are quoted automatically:
  - BigQuery/Spanner/ClickHouse: `fixture.history.24h.events.total` → `` fixture.history.`24h`.events.total ``
  - PostgreSQL/DuckDB: `fixture.history.24h.events.total` → `fixture.history."24h".events.total`
  - Unicode: `profile.名前` → `` profile.`名前` `` on BigQuery/Spanner/ClickHouse and `profile."名前"` on PostgreSQL/DuckDB

## Next Steps

- [Parameterized Queries](parameterized-queries.md) - Bind-parameter output for safe SQL execution
- [Supported Operators](operators.md) - Learn about all available operators
- [SQL Dialects](dialects.md) - Dialect-specific SQL generation details
- [Custom Operators](custom-operators.md) - Extend with your own operators
- [Schema Validation](schema-validation.md) - Add field validation
- [Examples](examples.md) - See more examples
- [REPL](repl.md) - Interactive testing tool
