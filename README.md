# JSON Logic to SQL Transpiler

A Go library that converts JSON Logic expressions into SQL. This library provides a clean, type-safe API for transforming JSON Logic rules into SQL WHERE clauses or standalone conditions, with support for multiple SQL dialects.

## Features

- **Complete JSON Logic Support**: Implements all core JSON Logic operators
- **SQL Dialect Support**: Target BigQuery, Spanner, PostgreSQL, DuckDB, or ClickHouse
- **Custom Operators**: Extensible registry pattern for custom SQL functions
- **Schema Validation**: Optional field schema for strict column validation
- **Structured Errors**: Error codes and JSONPath locations for debugging
- **Library & CLI**: Both programmatic API and interactive REPL

## Quick Start

```bash
go get github.com/h22rana/jsonlogic2sql@latest
```

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

func main() {
    sql, err := jsonlogic2sql.Transpile(
        jsonlogic2sql.DialectBigQuery,
        `{">": [{"var": "amount"}, 1000]}`,
    )
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: WHERE amount > 1000
}
```

## Supported Operators

| Category | Operators |
|----------|-----------|
| **Data Access** | `var`, `missing`, `missing_some` |
| **Comparison** | `==`, `===`, `!=`, `!==`, `>`, `>=`, `<`, `<=` |
| **Logical** | `and`, `or`, `!`, `!!`, `if` |
| **Numeric** | `+`, `-`, `*`, `/`, `%`, `max`, `min` |
| **Array** | `in`, `map`, `filter`, `reduce`, `all`, `some`, `none`, `merge` |
| **String** | `in`, `cat`, `substr` |

## Supported Dialects

| Dialect | Constant |
|---------|----------|
| Google BigQuery | `DialectBigQuery` |
| Google Cloud Spanner | `DialectSpanner` |
| PostgreSQL | `DialectPostgreSQL` |
| DuckDB | `DialectDuckDB` |
| ClickHouse | `DialectClickHouse` |

## Documentation

- [Getting Started](docs/getting-started.md) - Installation and basic usage
- [Operators](docs/operators.md) - All supported operators with examples
- [SQL Dialects](docs/dialects.md) - Dialect-specific SQL generation
- [Custom Operators](docs/custom-operators.md) - Extend with your own operators
- [Schema Validation](docs/schema-validation.md) - Field validation and type checking
- [Examples](docs/examples.md) - Comprehensive examples
- [API Reference](docs/api-reference.md) - Full API documentation
- [Error Handling](docs/error-handling.md) - Error codes and programmatic handling
- [Development](docs/development.md) - Contributing and development guide
- [REPL](docs/repl.md) - Interactive testing tool

## Important Notes

> **Semantic Correctness Assumption:** This library assumes that the input JSONLogic is semantically correct. The transpiler generates SQL that directly corresponds to the JSONLogic structure without validating the logical correctness of the expressions.

> **SQL Injection:** This library does NOT handle SQL injection prevention. The caller is responsible for validating input and using parameterized queries where appropriate.

## Interactive REPL

```bash
make run
```

```
[BigQuery] jsonlogic> {">": [{"var": "amount"}, 1000]}
SQL: WHERE amount > 1000

[BigQuery] jsonlogic> :dialect
Select dialect: PostgreSQL

[PostgreSQL] jsonlogic> {"merge": [{"var": "a"}, {"var": "b"}]}
SQL: WHERE (a || b)
```

## Development

```bash
make test      # Run all tests (3,000+ test cases)
make build     # Build REPL binary
make lint      # Run linter
make run       # Run REPL
```

## License

This project is licensed under the MIT License - see the [LICENSE](./LICENSE) file for details.
