# Interactive REPL

jsonlogic2sql includes an interactive REPL (Read-Eval-Print Loop) for testing JSON Logic expressions.

## Running the REPL

```bash
# Build and run
make run

# Or build manually
go build -o bin/repl ./cmd/repl
./bin/repl
```

On startup, the REPL prompts for:

1. SQL dialect
2. expression mode (`condition` by default, or `value`)
3. optional schema path

Leave the schema path empty to skip schema validation.

## Basic Usage

```
jsonlogic> {">": [{"var": "amount"}, 1000]}
SQL: amount > 1000

jsonlogic> {"and": [{"==": [{"var": "status"}, "active"]}, {">": [{"var": "count"}, 5]}]}
SQL: (status = 'active' AND count > 5)
```

## Commands

| Command | Description |
|---------|-------------|
| `:help` | Show available commands |
| `:examples` | Show example JSON Logic expressions |
| `:dialect` | Change the SQL dialect |
| `:mode condition\|value` | Switch expression mode |
| `:condition` | Use predicate/condition output mode |
| `:value` | Use value-expression output mode |
| `:params` | Toggle parameterized query output (bind placeholders) |
| `:schema <path>` | Load a schema JSON file for validation |
| `:file <path>` | Read JSON Logic from a file |
| `:clear` | Clear the screen |
| `:quit` | Exit the REPL |

## Changing Dialects

Use `:dialect` to switch between SQL dialects:

```
[BigQuery] jsonlogic> :dialect
Select dialect:
1. BigQuery
2. Spanner
3. PostgreSQL
4. DuckDB
5. ClickHouse
Enter choice (1-5): 3

[PostgreSQL] jsonlogic> {"merge": [{"var": "a"}, {"var": "b"}]}
Error: [E008] at $.merge (operator: merge): expected predicate expression, got value expression

[PostgreSQL] jsonlogic> :value
Expression mode: value

[PostgreSQL] jsonlogic> {"merge": [{"var": "a"}, {"var": "b"}]}
SQL: (a || b)
```

The prompt shows the current dialect in brackets.

## Parameterized Output

Use `:params` to toggle parameterized query mode. When enabled, the output uses bind placeholders instead of inlined literals:

```
[BigQuery] jsonlogic> :params
Parameterized mode: ON (output uses bind placeholders)

[BigQuery] jsonlogic> {"and": [{"==": [{"var": "status"}, "active"]}, {">": [{"var": "amount"}, 1000]}]}
SQL:    (status = @p1 AND amount > @p2)
Params: [{p1: "active"}, {p2: 1000}]

[BigQuery] jsonlogic> :params
Parameterized mode: OFF (output uses inlined literals)

[BigQuery] jsonlogic> {"and": [{"==": [{"var": "status"}, "active"]}, {">": [{"var": "amount"}, 1000]}]}
SQL: (status = 'active' AND amount > 1000)
```

Placeholder styles vary by dialect (`@p1` for BigQuery/Spanner/ClickHouse, `$1` for PostgreSQL/DuckDB). See [Parameterized Queries](parameterized-queries.md) for details.

### Pattern-Matching Note (`LIKE`)

In parameterized mode, pattern operators (for example `startsWith`, `endsWith`, `contains`) must keep placeholders outside string literals.

- Correct: `LIKE CONCAT(@p1, '%')`
- Incorrect: `LIKE '@p1%'`

The correct form preserves bind semantics; the incorrect form treats the placeholder text as a literal string.

## Large JSON Input

For JSON Logic expressions larger than terminal line limits (~4KB), use the `:file` command:

```bash
# Save your large JSON to a file
echo '{"and": [...very large JSON...]}' > input.json

# In the REPL, load it with :file
[BigQuery] jsonlogic> :file input.json
SQL: (...)
```

## Loading a Schema

Use `:schema` to load a schema file for field validation and type-aware SQL:

```
[BigQuery] jsonlogic> :schema schema.json
Schema loaded: schema.json
```

Schema field names should be raw, unquoted identifiers. The REPL uses the same dialect-aware identifier quoting as the library, including numeric-leading path segments in array lambdas such as `{"var":"24h"}` and reduce scopes such as `{"var":"current.24h"}`.

## Examples Command

The `:examples` command shows common JSON Logic patterns:

```
jsonlogic> :examples
Example JSON Logic expressions:

1. Simple Comparison
   JSON: {">": [{"var": "amount"}, 1000]}
   SQL:  amount > 1000

2. Equality Check
   JSON: {"==": [{"var": "status"}, "active"]}
   SQL:  status = 'active'

3. Logical AND
   JSON: {"and": [{">": [{"var": "a"}, 10]}, {"<": [{"var": "b"}, 20]}]}
   SQL:  (a > 10 AND b < 20)

...
```

## Error Messages

The REPL displays helpful error messages:

```
jsonlogic> {"unknownOp": [1, 2]}
Error: [E100] at $.unknownOp (operator: unknownOp): unsupported operator: unknownOp

jsonlogic> {invalid json}
Error: [E007]: invalid JSON: invalid character 'i' looking for beginning of object key string
```

## Tips

1. **Use `:examples` first** - Learn common patterns before writing your own
2. **Check dialect** - Some operators generate different SQL per dialect
3. **Use `:file` for complex JSON** - Avoids terminal line limits
4. **Copy SQL output** - Use it directly in your database queries

## See Also

- [Getting Started](getting-started.md) - Basic library usage
- [Examples](examples.md) - Comprehensive examples
- [Operators](operators.md) - All supported operators
