# Getting Started

This guide will help you get up and running with jsonlogic2sql.

## Installation

```bash
go get github.com/h22rana/jsonlogic2sql@latest
```

## Prerequisites

- Go 1.19 or later

## Basic Usage

### Simple Example

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

func main() {
    // Simple usage with dialect (required)
    sql, err := jsonlogic2sql.Transpile(jsonlogic2sql.DialectBigQuery, `{">": [{"var": "amount"}, 1000]}`)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: WHERE amount > 1000
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
    // Create a transpiler instance
    transpiler, err := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
    if err != nil {
        panic(err)
    }

    // From JSON string
    sql, err := transpiler.Transpile(`{"and": [{"==": [{"var": "status"}, "pending"]}, {">": [{"var": "amount"}, 5000]}]}`)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: WHERE (status = 'pending' AND amount > 5000)

    // From pre-parsed map
    logic := map[string]interface{}{
        "or": []interface{}{
            map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": "failedAttempts"}, 5}},
            map[string]interface{}{"in": []interface{}{map[string]interface{}{"var": "country"}, []interface{}{"CN", "RU"}}},
        },
    }
    sql, err = transpiler.TranspileFromMap(logic)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: WHERE (failedAttempts >= 5 OR country IN ('CN', 'RU'))
}
```

### Getting SQL Without WHERE Prefix

Use `TranspileCondition` methods when you need to embed conditions in larger queries:

```go
// Returns just the condition without "WHERE"
condition, err := jsonlogic2sql.TranspileCondition(
    jsonlogic2sql.DialectBigQuery,
    `{">": [{"var": "amount"}, 1000]}`,
)
// condition = "amount > 1000"

// Use in a custom query
query := fmt.Sprintf("SELECT * FROM orders WHERE %s AND created_at > '2024-01-01'", condition)
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
// BigQuery
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

// PostgreSQL
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectPostgreSQL)

// ClickHouse
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectClickHouse)
```

## Variable Naming

The transpiler preserves JSON Logic variable names as-is in the SQL output:

- Dot notation is preserved: `transaction.amount` → `transaction.amount`
- Nested variables: `user.account.age` → `user.account.age`
- Simple variables remain unchanged: `amount` → `amount`

This allows for proper JSON column access in databases that support it (like PostgreSQL with JSONB columns).

## Next Steps

- [Supported Operators](operators.md) - Learn about all available operators
- [SQL Dialects](dialects.md) - Dialect-specific SQL generation details
- [Custom Operators](custom-operators.md) - Extend with your own operators
- [Schema Validation](schema-validation.md) - Add field validation
- [Examples](examples.md) - See more examples
- [REPL](repl.md) - Interactive testing tool
