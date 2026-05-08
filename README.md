# JSON Logic to SQL Transpiler

A Go library that converts JSON Logic expressions into SQL predicate and value expressions, with support for multiple SQL dialects.

## Features

- **Complete JSON Logic Support**: Implements all core JSON Logic operators
- **SQL Dialect Support**: Target BigQuery, Spanner, PostgreSQL, DuckDB, or ClickHouse
- **Parameterized Queries**: Generate SQL with bind placeholders (`@p1`, `$1`) and separate parameter values for safe execution
- **Custom Operators**: Extensible registry pattern for custom SQL functions
- **Schema Validation**: Optional field schema for strict column validation
- **Identifier Quoting**: Path segments such as `24h` and `7d` are quoted per dialect
- **Structured Errors**: Error codes and JSONPath locations for debugging
- **Regression Matrices**: Cross-dialect schema-aware/schema-less matrix tests for nested built-in and custom operator flows
- **Array Scope Safety**: Nested array operators keep inner/outer element aliases distinct where required
- **Library & CLI**: Both programmatic API and interactive REPL

## Quick Start

```bash
go get github.com/h22rana/jsonlogic2sql@latest
```

### Inline SQL

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

func main() {
    sql, err := jsonlogic2sql.TranspileCondition(
        jsonlogic2sql.DialectBigQuery,
        `{">": [{"var": "amount"}, 1000]}`,
    )
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: amount > 1000
}
```

Use `TranspileValue` for value-producing expressions such as arithmetic, string concatenation, `map`, `reduce`, or JSONLogic value fallback:

```go
sql, err := jsonlogic2sql.TranspileValue(
    jsonlogic2sql.DialectBigQuery,
    `{"or": [false, "fallback"]}`,
)
fmt.Println(sql) // Output: 'fallback'
```

### Parameterized Queries

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

func main() {
    sql, params, err := jsonlogic2sql.TranspileParameterizedCondition(
        jsonlogic2sql.DialectBigQuery,
        `{"and": [{"==": [{"var": "status"}, "active"]}, {">": [{"var": "amount"}, 1000]}]}`,
    )
    if err != nil {
        panic(err)
    }
    fmt.Println(sql)    // Output: (status = @p1 AND amount > @p2)
    fmt.Println(params) // Output: [{p1 active} {p2 1000}]
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
- [Parameterized Queries](docs/parameterized-queries.md) - Bind-parameter output for safe SQL execution
- [Operators](docs/operators.md) - All supported operators with examples
- [SQL Dialects](docs/dialects.md) - Dialect-specific SQL generation
- [Custom Operators](docs/custom-operators.md) - Extend with your own operators
- [Schema Validation](docs/schema-validation.md) - Field validation and type checking
- [Examples](docs/examples.md) - Comprehensive examples
- [API Reference](docs/api-reference.md) - Full API documentation
- [Error Handling](docs/error-handling.md) - Error codes and programmatic handling
- [Development](docs/development.md) - Contributing and development guide
- [REPL](docs/repl.md) - Interactive testing tool
- [WASM Playground](docs/wasm-playground.md) - Browser-based demo via WebAssembly

## Important Notes

> **Semantic Correctness Assumption:** This library assumes that the input JSONLogic is semantically correct. The transpiler generates SQL that directly corresponds to the JSONLogic structure without validating the logical correctness of the expressions.

> **SQL Injection:** This library includes hardening measures against SQL injection - identifier names are validated against a whitelist pattern, string literals are escaped, and numeric string operands are safely coerced. For maximum safety, use the [parameterized query API](docs/parameterized-queries.md) which generates SQL with bind placeholders instead of inlined literals.

> **Equality Semantics:** With a schema, equality operators use type-aware literal coercion for numeric and boolean fields, and strict equality (`===`/`!==`) folds schema-known type mismatches. The same comparison rules apply to `[field, default]` vars while preserving `COALESCE(...)`, and visible enum defaults are validated. Field-to-field equality uses ordinary SQL by default (`a = b`); enable `NullSafeFieldEquality` to also match rows where both fields are `NULL`. For portability, runtime string-column coercion is not modeled: `code == 5` emits the canonical match `code = '5'` (`1e400` emits `'Infinity'`), while loose string/boolean comparisons such as `code == true` return an unsupported-comparison error.

> **Identifier Quoting:** JSON Logic `var` names and schema field names should use raw, unquoted identifiers. The transpiler quotes invalid unquoted path segments automatically, for example `fixture.history.24h.events.total` becomes ``fixture.history.`24h`.events.total`` for BigQuery/Spanner/ClickHouse and `fixture.history."24h".events.total` for PostgreSQL/DuckDB. `NewSchema` returns an error when schema field names contain quote characters; use raw identifiers and let the transpiler apply dialect-specific quoting.

> **Condition vs Value APIs:** `TranspileCondition` returns SQL predicates that callers can put after `WHERE`. `TranspileValue` returns SQL value expressions. Value-producing JSONLogic such as `{"or":[false,"fallback"]}` is valid in value mode, but is rejected in condition mode instead of generating non-portable SQL like `FALSE OR 'fallback'`.

> **`in` Operator Inference:** Without a schema, `in` uses heuristics to infer string containment vs array membership. For deterministic behavior (especially with complex expressions), prefer schema-aware mode.

## Interactive REPL

```bash
make run
```

```
[BigQuery] jsonlogic> {">": [{"var": "amount"}, 1000]}
SQL: amount > 1000

[BigQuery] jsonlogic> :params
Parameterized mode: ON (output uses bind placeholders)

[BigQuery] jsonlogic> {"==": [{"var": "status"}, "active"]}
SQL:    status = @p1
Params: [{p1: "active"}]

[BigQuery] jsonlogic> :value
Expression mode: value

[BigQuery] jsonlogic> :dialect
Select dialect: PostgreSQL

[PostgreSQL] jsonlogic> {"merge": [{"var": "a"}, {"var": "b"}]}
SQL: (a || b)
```

## Development

```bash
make test       # Run all tests (including cross-dialect matrix suites)
make bench      # Run benchmarks
make build      # Build REPL binary
make build/wasm # Build WASM binary for browser playground
make lint       # Run linter
make run        # Run REPL
```

## License

This project is licensed under the MIT License - see the [LICENSE](./LICENSE) file for details.
