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
// Create transpiler with specific dialect
transpiler, err := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

// Or use convenience functions
sql, err := jsonlogic2sql.Transpile(jsonlogic2sql.DialectPostgreSQL, jsonLogic)
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

## Dialect-Specific SQL Generation

Some operators generate different SQL based on the target dialect:

| Operator | BigQuery | Spanner | PostgreSQL | DuckDB | ClickHouse |
|----------|----------|---------|------------|--------|------------|
| `merge` (arrays) | `ARRAY_CONCAT(a, b)` | `ARRAY_CONCAT(a, b)` | `(a \|\| b)` | `ARRAY_CONCAT(a, b)` | `arrayConcat(a, b)` |
| `map` (arrays) | `ARRAY(SELECT ... UNNEST)` | `ARRAY(SELECT ... UNNEST)` | `ARRAY(SELECT ... UNNEST)` | `ARRAY(SELECT ... UNNEST)` | `arrayMap(x -> ..., arr)` |
| `filter` (arrays) | `ARRAY(SELECT ... WHERE)` | `ARRAY(SELECT ... WHERE)` | `ARRAY(SELECT ... WHERE)` | `ARRAY(SELECT ... WHERE)` | `arrayFilter(x -> ..., arr)` |
| `substr` | `SUBSTR(s, i, n)` | `SUBSTR(s, i, n)` | `SUBSTR(s, i, n)` | `SUBSTR(s, i, n)` | `substring(s, i, n)` |
| `in` (string) | `STRPOS(h, n) > 0` | `STRPOS(h, n) > 0` | `POSITION(n IN h) > 0` | `STRPOS(h, n) > 0` | `position(h, n) > 0` |

## SQL Function Reference by Dialect

| Function | BigQuery | Spanner | PostgreSQL | DuckDB | ClickHouse |
|----------|----------|---------|------------|--------|------------|
| String position | `STRPOS()` | `STRPOS()` | `POSITION()` | `STRPOS()` | `position()` |
| String concat | `CONCAT()` | `CONCAT()` | `CONCAT()` | `CONCAT()` | `concat()` |
| Substring | `SUBSTR()` | `SUBSTR()` | `SUBSTR()` | `SUBSTR()` | `substring()` |
| Array map | `UNNEST` subquery | `UNNEST` subquery | `UNNEST` subquery | `UNNEST` subquery | `arrayMap()` |
| Array filter | `UNNEST` subquery | `UNNEST` subquery | `UNNEST` subquery | `UNNEST` subquery | `arrayFilter()` |
| Array reduce | `SUM/MIN/MAX` | `SUM/MIN/MAX` | `SUM/MIN/MAX` | `SUM/MIN/MAX` | `arrayReduce()` |
| Array concat | `ARRAY_CONCAT()` | `ARRAY_CONCAT()` | `\|\|` | `ARRAY_CONCAT()` | `arrayConcat()` |
| Max of values | `GREATEST()` | `GREATEST()` | `GREATEST()` | `GREATEST()` | `greatest()` |
| Min of values | `LEAST()` | `LEAST()` | `LEAST()` | `LEAST()` | `least()` |
| Null coalesce | `COALESCE()` | `COALESCE()` | `COALESCE()` | `COALESCE()` | `coalesce()` |
| Safe divide | `SAFE_DIVIDE()` | N/A (use CASE) | N/A (use CASE) | N/A (use CASE) | `if()` expression |
| Regex match | `REGEXP_CONTAINS()` | `REGEXP_CONTAINS()` | `~` | `regexp_matches()` | `match()` |

## Custom Dialect-Aware Operators

You can create custom operators that generate different SQL per dialect:

```go
transpiler.RegisterDialectAwareOperatorFunc("safeDivide",
    func(op string, args []interface{}, dialect jsonlogic2sql.Dialect) (string, error) {
        numerator := args[0].(string)
        denominator := args[1].(string)

        switch dialect {
        case jsonlogic2sql.DialectBigQuery:
            return fmt.Sprintf("SAFE_DIVIDE(%s, %s)", numerator, denominator), nil
        case jsonlogic2sql.DialectClickHouse:
            return fmt.Sprintf("if(%s = 0, NULL, %s / %s)", denominator, numerator, denominator), nil
        default:
            return fmt.Sprintf("CASE WHEN %s = 0 THEN NULL ELSE %s / %s END",
                denominator, numerator, denominator), nil
        }
    })
```

See [Custom Operators](custom-operators.md#dialect-aware-custom-operators) for more details.

## Unsupported Dialects

### MySQL

MySQL is not supported because it lacks native `UNNEST()` for arrays. This would require complex `JSON_TABLE` workarounds that produce semantically different behavior from other dialects.

## See Also

- [Custom Operators](custom-operators.md) - Create dialect-aware operators
- [API Reference](api-reference.md) - Full API documentation
