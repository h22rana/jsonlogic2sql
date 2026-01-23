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

## Basic Usage

```
jsonlogic> {">": [{"var": "amount"}, 1000]}
SQL: WHERE amount > 1000

jsonlogic> {"and": [{"==": [{"var": "status"}, "active"]}, {">": [{"var": "count"}, 5]}]}
SQL: WHERE (status = 'active' AND count > 5)
```

## Commands

| Command | Description |
|---------|-------------|
| `:help` | Show available commands |
| `:examples` | Show example JSON Logic expressions |
| `:dialect` | Change the SQL dialect |
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
SQL: WHERE (a || b)
```

The prompt shows the current dialect in brackets.

## Large JSON Input

For JSON Logic expressions larger than terminal line limits (~4KB), use the `:file` command:

```bash
# Save your large JSON to a file
echo '{"and": [...very large JSON...]}' > input.json

# In the REPL, load it with :file
[BigQuery] jsonlogic> :file input.json
SQL: WHERE (...)
```

## Examples Command

The `:examples` command shows common JSON Logic patterns:

```
jsonlogic> :examples
Example JSON Logic expressions:

1. Simple Comparison
   JSON: {">": [{"var": "amount"}, 1000]}
   SQL:  WHERE amount > 1000

2. Equality Check
   JSON: {"==": [{"var": "status"}, "active"]}
   SQL:  WHERE status = 'active'

3. Logical AND
   JSON: {"and": [{">": [{"var": "a"}, 10]}, {"<": [{"var": "b"}, 20]}]}
   SQL:  WHERE (a > 10 AND b < 20)

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
