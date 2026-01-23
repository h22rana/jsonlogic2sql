# API Reference

Complete API documentation for jsonlogic2sql.

## Functions

### Transpile

```go
func Transpile(dialect Dialect, jsonLogic string) (string, error)
```

Converts a JSON Logic string to a SQL WHERE clause using the specified dialect.

### TranspileFromMap

```go
func TranspileFromMap(dialect Dialect, logic map[string]interface{}) (string, error)
```

Converts a pre-parsed JSON Logic map to a SQL WHERE clause.

### TranspileFromInterface

```go
func TranspileFromInterface(dialect Dialect, logic interface{}) (string, error)
```

Converts any JSON Logic interface{} to a SQL WHERE clause.

### TranspileCondition

```go
func TranspileCondition(dialect Dialect, jsonLogic string) (string, error)
```

Converts a JSON Logic string to a SQL condition **without** the WHERE keyword. Useful for embedding conditions in larger queries.

### TranspileConditionFromMap

```go
func TranspileConditionFromMap(dialect Dialect, logic map[string]interface{}) (string, error)
```

Converts a pre-parsed JSON Logic map to a SQL condition without WHERE.

### TranspileConditionFromInterface

```go
func TranspileConditionFromInterface(dialect Dialect, logic interface{}) (string, error)
```

Converts any JSON Logic interface{} to a SQL condition without WHERE.

### NewTranspiler

```go
func NewTranspiler(dialect Dialect) (*Transpiler, error)
```

Creates a new transpiler instance with the specified dialect.

### NewTranspilerWithConfig

```go
func NewTranspilerWithConfig(config *TranspilerConfig) (*Transpiler, error)
```

Creates a new transpiler instance with custom configuration.

### NewOperatorRegistry

```go
func NewOperatorRegistry() *OperatorRegistry
```

Creates a new empty operator registry for managing custom operators.

## Types

### Transpiler

Main transpiler instance.

**Methods:**

| Method | Description |
|--------|-------------|
| `Transpile(jsonLogic string) (string, error)` | Convert JSON string to SQL with WHERE |
| `TranspileFromMap(logic map[string]interface{}) (string, error)` | Convert map to SQL with WHERE |
| `TranspileFromInterface(logic interface{}) (string, error)` | Convert interface to SQL with WHERE |
| `TranspileCondition(jsonLogic string) (string, error)` | Convert JSON string to SQL without WHERE |
| `TranspileConditionFromMap(logic map[string]interface{}) (string, error)` | Convert map to SQL without WHERE |
| `TranspileConditionFromInterface(logic interface{}) (string, error)` | Convert interface to SQL without WHERE |
| `GetDialect() Dialect` | Get the configured dialect |
| `SetSchema(schema *Schema)` | Set schema for field validation |
| `RegisterOperator(name string, handler OperatorHandler) error` | Register custom operator with handler |
| `RegisterOperatorFunc(name string, fn OperatorFunc) error` | Register custom operator with function |
| `RegisterDialectAwareOperator(name string, handler DialectAwareOperatorHandler) error` | Register dialect-aware operator |
| `RegisterDialectAwareOperatorFunc(name string, fn DialectAwareOperatorFunc) error` | Register dialect-aware function |
| `UnregisterOperator(name string) bool` | Remove a custom operator |
| `HasCustomOperator(name string) bool` | Check if operator is registered |
| `ListCustomOperators() []string` | List all custom operator names |
| `ClearCustomOperators()` | Remove all custom operators |

### TranspilerConfig

Configuration options for the transpiler.

```go
type TranspilerConfig struct {
    Dialect Dialect   // Required: target SQL dialect
    Schema  *Schema   // Optional: schema for field validation
}
```

### Dialect

SQL dialect type.

```go
type Dialect int

const (
    DialectBigQuery    Dialect // Google BigQuery SQL
    DialectSpanner     Dialect // Google Cloud Spanner SQL
    DialectPostgreSQL  Dialect // PostgreSQL SQL
    DialectDuckDB      Dialect // DuckDB SQL
    DialectClickHouse  Dialect // ClickHouse SQL
)
```

### OperatorFunc

Function type for simple custom operator implementations.

```go
type OperatorFunc func(operator string, args []interface{}) (string, error)
```

### OperatorHandler

Interface for custom operator implementations that need state.

```go
type OperatorHandler interface {
    ToSQL(operator string, args []interface{}) (string, error)
}
```

### DialectAwareOperatorFunc

Function type for dialect-aware custom operator implementations.

```go
type DialectAwareOperatorFunc func(operator string, args []interface{}, dialect Dialect) (string, error)
```

### DialectAwareOperatorHandler

Interface for dialect-aware custom operator implementations.

```go
type DialectAwareOperatorHandler interface {
    ToSQLWithDialect(operator string, args []interface{}, dialect Dialect) (string, error)
}
```

### OperatorRegistry

Thread-safe registry for managing custom operators.

**Methods:**

| Method | Description |
|--------|-------------|
| `Register(operatorName string, handler OperatorHandler)` | Add operator handler |
| `RegisterFunc(operatorName string, fn OperatorFunc)` | Add operator function |
| `Unregister(operatorName string) bool` | Remove an operator |
| `Get(operatorName string) (OperatorHandler, bool)` | Get operator handler |
| `Has(operatorName string) bool` | Check if operator exists |
| `List() []string` | List all operator names |
| `Clear()` | Remove all operators |
| `Clone() *OperatorRegistry` | Create a copy of the registry |
| `Merge(other *OperatorRegistry)` | Merge operators from another registry |

### Schema

Schema for field validation.

**Methods:**

| Method | Description |
|--------|-------------|
| `HasField(fieldName string) bool` | Check if field exists in schema |
| `ValidateField(fieldName string) error` | Validate field existence |
| `GetFieldType(fieldName string) string` | Get field type as string |
| `IsArrayType(fieldName string) bool` | Check if field is array type |
| `IsStringType(fieldName string) bool` | Check if field is string type |
| `IsNumericType(fieldName string) bool` | Check if field is numeric type |
| `IsBooleanType(fieldName string) bool` | Check if field is boolean type |
| `IsEnumType(fieldName string) bool` | Check if field is enum type |
| `GetAllowedValues(fieldName string) []string` | Get allowed values for enum |
| `ValidateEnumValue(fieldName, value string) error` | Validate enum value |
| `GetFields() []string` | Get all field names |

### FieldSchema

Field definition for schema.

```go
type FieldSchema struct {
    Name          string    // Field name (e.g., "order.amount")
    Type          FieldType // Field type
    AllowedValues []string  // For enum types: list of valid values
}
```

### FieldType

Field type constants.

```go
type FieldType int

const (
    FieldTypeString   FieldType // String field type
    FieldTypeInteger  FieldType // Integer field type
    FieldTypeNumber   FieldType // Numeric field type (float/decimal)
    FieldTypeBoolean  FieldType // Boolean field type
    FieldTypeArray    FieldType // Array field type
    FieldTypeObject   FieldType // Object/struct field type
    FieldTypeEnum     FieldType // Enum field type (requires AllowedValues)
)
```

### TranspileError

Structured error type returned by transpilation operations.

```go
type TranspileError struct {
    Code     ErrorCode // Error code (e.g., ErrUnsupportedOperator)
    Operator string    // The operator that caused the error
    Path     string    // JSONPath to the error location
    Message  string    // Human-readable error message
    Cause    error     // Underlying error (if any)
}
```

**Methods:**

| Method | Description |
|--------|-------------|
| `Error() string` | Returns formatted error message with code and path |
| `Unwrap() error` | Returns the underlying cause for errors.Unwrap support |

### ErrorCode

Error code type.

**Categories:**

| Range | Category |
|-------|----------|
| E001-E099 | Structural/validation errors |
| E100-E199 | Operator-specific errors |
| E200-E299 | Type/schema errors |
| E300-E399 | Argument errors |

See [Error Handling](error-handling.md) for complete error code reference.

## Helper Functions

### AsTranspileError

```go
func AsTranspileError(err error) (*TranspileError, bool)
```

Extract TranspileError from error chain.

### IsErrorCode

```go
func IsErrorCode(err error, code ErrorCode) bool
```

Check if error has specific code.

## Schema Functions

### NewSchema

```go
func NewSchema(fields []FieldSchema) *Schema
```

Create a new schema from field definitions.

### NewSchemaFromJSON

```go
func NewSchemaFromJSON(data []byte) (*Schema, error)
```

Create a schema from JSON bytes.

### NewSchemaFromFile

```go
func NewSchemaFromFile(filepath string) (*Schema, error)
```

Create a schema from a JSON file.

## See Also

- [Getting Started](getting-started.md) - Basic usage examples
- [Error Handling](error-handling.md) - Error codes and handling
- [Custom Operators](custom-operators.md) - Operator registration
