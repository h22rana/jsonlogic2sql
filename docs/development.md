# Development Guide

This guide covers development setup, testing, and contribution guidelines for jsonlogic2sql.

## Prerequisites

- Go 1.19 or later
- Make (optional, for using Makefile)

## Quick Start

```bash
# Clone the repository
git clone https://github.com/h22rana/jsonlogic2sql.git
cd jsonlogic2sql

# Install dependencies
make deps

# Run tests
make test

# Build the REPL
make build

# Run linter
make lint
```

## Project Structure

```
jsonlogic2sql/
├── transpiler.go             # Main public API
├── transpiler_test.go        # Public API tests
├── operator.go               # Custom operators registry and types
├── operator_test.go          # Custom operators tests
├── schema.go                 # Schema/metadata validation
├── schema_test.go            # Schema tests
├── errors.go                 # Public error types
├── internal/
│   ├── parser/               # Core parsing logic
│   │   ├── parser.go         # Recursive descent parser
│   │   └── parser_test.go
│   ├── operators/            # Operator implementations
│   │   ├── config.go         # Shared operator config
│   │   ├── constants.go      # Operator name constants
│   │   ├── data.go           # var, missing, missing_some
│   │   ├── comparison.go     # ==, !=, >, <, in, etc.
│   │   ├── logical.go        # and, or, !, !!, if
│   │   ├── numeric.go        # +, -, *, /, %, max, min
│   │   ├── string.go         # cat, substr
│   │   ├── array.go          # map, filter, reduce, all, some, none, merge
│   │   └── schema.go         # SchemaProvider interface
│   ├── dialect/              # SQL dialect definitions
│   │   └── dialect.go        # Dialect constants and helpers
│   ├── errors/               # Internal error types
│   │   └── errors.go         # Error constructors
│   └── validator/            # Pre-validation logic
├── cmd/repl/                 # Interactive REPL
│   └── main.go
├── docs/                     # Documentation
├── Makefile                  # Build automation
├── CLAUDE.md                 # AI assistant context
└── README.md                 # Project overview
```

## Make Commands

| Command | Description |
|---------|-------------|
| `make test` | Run all tests |
| `make build` | Build REPL binary |
| `make lint` | Run linter |
| `make lint/fix` | Run linter with auto-fix |
| `make run` | Run REPL |
| `make deps` | Install dependencies |
| `make help` | Show available commands |

## Testing

The project includes comprehensive tests:

- **3,000+ test cases** across all packages
- **168 REPL test cases** for integration testing
- Full coverage for all operators and dialects

### Running Tests

```bash
# All tests
go test ./...

# With verbose output
go test -v ./...

# With coverage
go test -cover ./...

# Specific package
go test ./internal/operators/

# Run specific test
go test -run TestTranspile ./...
```

### Test Categories

| Category | Description |
|----------|-------------|
| Unit Tests | Each operator and component thoroughly tested |
| Integration Tests | End-to-end tests with real JSON Logic examples |
| Error Cases | Validation and error handling tests |
| Edge Cases | Boundary conditions and special cases |
| Dialect Tests | All 5 dialects tested for compatibility |

## Adding a New Dialect

1. **Add dialect constant** in `internal/dialect/dialect.go`:
   ```go
   const (
       DialectUnspecified Dialect = iota
       DialectBigQuery
       DialectSpanner
       DialectPostgreSQL
       DialectDuckDB
       DialectClickHouse
       DialectNewDialect  // New dialect
   )
   ```

2. **Update `String()` and `IsValid()`** methods in dialect.go

3. **Check each operator** - Review the [Dialect Compatibility Matrix](dialects.md) and implement dialect-specific SQL where needed:
   ```go
   switch config.GetDialect() {
   case dialect.DialectBigQuery:
       return "STRPOS(str, val) > 0", nil
   case dialect.DialectNewDialect:
       return "LOCATE(val, str) > 0", nil  // Dialect-specific syntax
   default:
       return "", fmt.Errorf("operator not supported for dialect: %s", config.GetDialect())
   }
   ```

4. **Update documentation** - Add the new dialect to [dialects.md](dialects.md)

5. **Add tests** for the new dialect in relevant `*_test.go` files

## Adding a New Operator

1. **Choose the appropriate file** based on operator category:
   - `data.go` - Data access operators
   - `comparison.go` - Comparison operators
   - `logical.go` - Logical operators
   - `numeric.go` - Numeric operators
   - `string.go` - String operators
   - `array.go` - Array operators

2. **Add operator constant** in `constants.go`:
   ```go
   const (
       OperatorNewOp = "newOp"
   )
   ```

3. **Implement the operator** following existing patterns

4. **Register in parser** - Add to operator dispatch in `parser.go`

5. **Add tests** - Create comprehensive test cases

6. **Update documentation** - Add to [operators.md](operators.md)

## Code Style

- Follow standard Go conventions
- Run `make lint/fix` before committing
- Keep functions focused and testable
- Document exported functions and types

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests and linter
5. Submit a pull request

### Commit Guidelines

- Use descriptive commit messages
- Group related changes in commits
- Reference issues when applicable

## See Also

- [REPL](repl.md) - Interactive testing tool
- [API Reference](api-reference.md) - Full API documentation
