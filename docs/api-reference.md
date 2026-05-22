# API Reference

Complete API documentation for jsonlogic2sql.

## Functions

### TranspileCondition

```go
func TranspileCondition(dialect Dialect, schema *Schema, jsonLogic string) (string, error)
```

Converts a JSON Logic string to a SQL predicate expression without the `WHERE` keyword. Callers add `WHERE` themselves when building full queries.

### TranspileConditionFromMap

```go
func TranspileConditionFromMap(dialect Dialect, schema *Schema, logic map[string]interface{}) (string, error)
```

Converts a pre-parsed JSON Logic map to a SQL condition without WHERE.

### TranspileConditionFromInterface

```go
func TranspileConditionFromInterface(dialect Dialect, schema *Schema, logic interface{}) (string, error)
```

Converts any JSON Logic interface{} to a SQL condition without WHERE.

### TranspileValue

```go
func TranspileValue(dialect Dialect, schema *Schema, jsonLogic string) (string, error)
```

Converts a JSON Logic string to a SQL value expression. Use this for value-producing JSONLogic such as arithmetic, string expressions, `map`, `reduce`, or value-returning `and`/`or`.

### TranspileValueFromMap

```go
func TranspileValueFromMap(dialect Dialect, schema *Schema, logic map[string]interface{}) (string, error)
```

Converts a pre-parsed JSON Logic map to a SQL value expression.

### TranspileValueFromInterface

```go
func TranspileValueFromInterface(dialect Dialect, schema *Schema, logic interface{}) (string, error)
```

Converts any JSON Logic interface{} to a SQL value expression.

### TranspileParameterizedCondition

```go
func TranspileParameterizedCondition(dialect Dialect, schema *Schema, jsonLogic string) (string, []QueryParam, error)
```

Converts a JSON Logic string to a SQL condition (without WHERE) with bind parameter placeholders.

### TranspileParameterizedConditionFromMap

```go
func TranspileParameterizedConditionFromMap(dialect Dialect, schema *Schema, logic map[string]interface{}) (string, []QueryParam, error)
```

Converts a pre-parsed JSON Logic map to a SQL condition (without WHERE) with bind parameter placeholders.

### TranspileParameterizedConditionFromInterface

```go
func TranspileParameterizedConditionFromInterface(dialect Dialect, schema *Schema, logic interface{}) (string, []QueryParam, error)
```

Converts any JSON Logic interface{} to a SQL condition (without WHERE) with bind parameter placeholders.

### TranspileParameterizedValue

```go
func TranspileParameterizedValue(dialect Dialect, schema *Schema, jsonLogic string) (string, []QueryParam, error)
```

Converts a JSON Logic string to a SQL value expression with bind parameter placeholders.

### TranspileParameterizedValueFromMap

```go
func TranspileParameterizedValueFromMap(dialect Dialect, schema *Schema, logic map[string]interface{}) (string, []QueryParam, error)
```

Converts a pre-parsed JSON Logic map to a parameterized SQL value expression.

### TranspileParameterizedValueFromInterface

```go
func TranspileParameterizedValueFromInterface(dialect Dialect, schema *Schema, logic interface{}) (string, []QueryParam, error)
```

Converts any JSON Logic interface{} to a parameterized SQL value expression.

### NewTranspiler

```go
func NewTranspiler(dialect Dialect, schema *Schema) (*Transpiler, error)
```

Creates a new transpiler instance with the specified dialect and required schema. Use `NewSchema(nil)` only for literal-only expressions.

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
| `TranspileCondition(jsonLogic string) (string, error)` | Convert JSON string to SQL predicate |
| `TranspileConditionFromMap(logic map[string]interface{}) (string, error)` | Convert map to SQL predicate |
| `TranspileConditionFromInterface(logic interface{}) (string, error)` | Convert interface to SQL predicate |
| `TranspileValue(jsonLogic string) (string, error)` | Convert JSON string to SQL value expression |
| `TranspileValueFromMap(logic map[string]interface{}) (string, error)` | Convert map to SQL value expression |
| `TranspileValueFromInterface(logic interface{}) (string, error)` | Convert interface to SQL value expression |
| `TranspileParameterizedCondition(jsonLogic string) (string, []QueryParam, error)` | Convert JSON string to parameterized SQL predicate |
| `TranspileParameterizedConditionFromMap(logic map[string]interface{}) (string, []QueryParam, error)` | Convert map to parameterized SQL predicate |
| `TranspileParameterizedConditionFromInterface(logic interface{}) (string, []QueryParam, error)` | Convert interface to parameterized SQL predicate |
| `TranspileParameterizedValue(jsonLogic string) (string, []QueryParam, error)` | Convert JSON string to parameterized SQL value expression |
| `TranspileParameterizedValueFromMap(logic map[string]interface{}) (string, []QueryParam, error)` | Convert map to parameterized SQL value expression |
| `TranspileParameterizedValueFromInterface(logic interface{}) (string, []QueryParam, error)` | Convert interface to parameterized SQL value expression |
| `GetDialect() Dialect` | Get the configured dialect |
| `SetSchema(schema *Schema) error` | Replace the required schema for field validation |
| `RegisterOperator(name string, handler OperatorHandler) error` | Register custom operator with handler |
| `RegisterOperatorFunc(name string, fn OperatorFunc) error` | Register custom operator with function |
| `RegisterDialectAwareOperator(name string, handler DialectAwareOperatorHandler) error` | Register dialect-aware operator |
| `RegisterDialectAwareOperatorFunc(name string, fn DialectAwareOperatorFunc) error` | Register dialect-aware function |
| `UnregisterOperator(name string) bool` | Remove a custom operator |
| `HasCustomOperator(name string) bool` | Check if operator is registered |
| `ListCustomOperators() []string` | List all custom operator names |
| `ClearCustomOperators()` | Remove all custom operators |

`TranspileValue*` returns standalone SQL value expressions. For PostgreSQL,
empty-array value results are rejected whenever the emitted SQL would contain
an untyped `ARRAY[]`, because PostgreSQL requires an explicit element type that
is not always available from the JSONLogic value alone. Foldable expressions
such as `{"or":[[],"fallback"]}` are still allowed because they do not emit the
empty array literal.

### TranspilerConfig

Configuration options for the transpiler.

```go
type TranspilerConfig struct {
    Dialect Dialect // Required: target SQL dialect
    Schema  *Schema // Required: schema for field validation
}
```

Use an empty schema only for literal-only expressions. Any `var` field access
must be declared in the schema.

Field-to-field equality is null-safe by default for JSONLogic-compatible field
NULL equality. `==`, `===`, `!=`, and `!==` comparisons between two `var`
operands use portable SQL that treats two `NULL` fields as equal. Field/literal
comparisons are unchanged. For strict comparisons between fields with
incompatible schema types, the generated SQL omits the cross-type equality arm:
`a === b` becomes `a IS NULL AND b IS NULL`, while `a !== b` becomes
`a IS NOT NULL OR b IS NOT NULL`. Loose `==`/`!=` comparisons between fields
with incompatible schema types return an unsupported-comparison error because
runtime mixed-field coercion is not modeled portably across SQL dialects.

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
type OperatorFunc func(operator string, args []OperatorArg) (OperatorResult, error)
```

Use `ValueSQL(sql, type)` for scalar value-producing custom operators,
`ArrayValueSQL(sql, elementType)` for array-producing custom operators with a
known immediate element type, and `PredicateSQL(sql)` for boolean predicate
custom operators. For nested arrays, set `OperatorResult.ArrayElementTypes`
with the immediate element type first.
Raw string-returning legacy custom operator functions are not accepted; the
result kind must be explicit so condition and value contexts can be validated.

### OperatorArg

Typed SQL argument passed to custom operators.

```go
type OperatorArg struct {
    SQL  string
    Kind ExpressionKind
    Type ExpressionType
}
```

### OperatorResult

Typed SQL result returned by custom operators.

```go
type OperatorResult struct {
    SQL               string
    Kind              ExpressionKind
    Type              ExpressionType
    EmptyArrayLiteral bool
    ArrayElementType  ExpressionType
    ArrayElementTypes []ExpressionType
}
```

`ArrayElementType` is only meaningful when `Type` is `ExpressionTypeArray`;
`ExpressionTypeUnknown` means the array element type is not statically known.
`ArrayElementTypes` carries nested array element types. For example,
`array<array<number>>` is represented as
`[]ExpressionType{ExpressionTypeArray, ExpressionTypeNumber}`.

### ExpressionKind

```go
const (
    ExpressionKindValue
    ExpressionKindPredicate
)
```

### ExpressionType

```go
const (
    ExpressionTypeUnknown
    ExpressionTypeNull
    ExpressionTypeBoolean
    ExpressionTypeString
    ExpressionTypeNumber
    ExpressionTypeArray
)
```

### OperatorHandler

Interface for custom operator implementations that need state.

```go
type OperatorHandler interface {
    ToSQL(operator string, args []OperatorArg) (OperatorResult, error)
}
```

### DialectAwareOperatorFunc

Function type for dialect-aware custom operator implementations.

```go
type DialectAwareOperatorFunc func(operator string, args []OperatorArg, dialect Dialect) (OperatorResult, error)
```

### DialectAwareOperatorHandler

Interface for dialect-aware custom operator implementations.

```go
type DialectAwareOperatorHandler interface {
    ToSQLWithDialect(operator string, args []OperatorArg, dialect Dialect) (OperatorResult, error)
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
    Name          string        // Field name (e.g., "order.amount")
    Type          FieldType     // Field type
    AllowedValues []string      // For enum types: list of valid values
    Fields        []FieldSchema // Nested object fields
    ElementFields []FieldSchema // Nested fields on array elements
}
```

`Name` and `Type` are required for every entry, including nested object fields
and array element fields. Object schemas use `Fields`; array schemas use
`ElementFields`. `Fields` requires `Type: FieldTypeObject`; `ElementFields`
requires `Type: FieldTypeArray`. Enum schemas require at least one
unique `AllowedValues` entry, and non-enum schemas cannot set `AllowedValues`.
Flattened field paths must be unique and cannot contain empty path segments.
Object fields are containers for nested schema paths; value expressions must
reference supported nested child fields rather than returning the object
container itself.
For example:

```json
[
  {
    "name": "profile",
    "type": "object",
    "fields": [
      { "name": "country", "type": "string" },
      { "name": "status", "type": "enum", "allowedValues": ["active", "blocked"] }
    ]
  },
  {
    "name": "payments",
    "type": "array",
    "elementFields": [
      { "name": "type", "type": "enum", "allowedValues": ["BALANCE", "CARD"] },
      {
        "name": "details",
        "type": "object",
        "fields": [
          { "name": "issuer", "type": "string" }
        ]
      }
    ]
  }
]
```

### FieldType

Field type constants.

```go
type FieldType string

const (
    FieldTypeString  FieldType = "string"  // String field type
    FieldTypeInteger FieldType = "integer" // Integer field type
    FieldTypeNumber  FieldType = "number"  // Numeric field type (float/decimal)
    FieldTypeBoolean FieldType = "boolean" // Boolean field type
    FieldTypeArray   FieldType = "array"   // Array field type
    FieldTypeObject  FieldType = "object"  // Object/struct container field type
    FieldTypeEnum    FieldType = "enum"    // Enum field type (requires AllowedValues)
)
```

### QueryParam

Represents a single bind parameter collected during parameterized transpilation.

```go
type QueryParam struct {
    Name  string      // "p1", "p2", etc.
    Value interface{} // Go native type (string, float64, int64, bool)
}
```

> **Note:** JSON decoding uses `json.Decoder.UseNumber()` to preserve precision. JS-safe integers (within ±2^53−1) bind as `float64`; larger integers (both unquoted JSON literals and quoted strings exceeding `int64`) bind as `string`. Floats outside the representable float64 range (e.g., `1e309` overflow, `1e-400` underflow to zero) are also preserved as `string`. Callers binding string-typed numeric params may need to convert them to their driver's numeric type.

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
func NewSchema(fields []FieldSchema) (*Schema, error)
```

Create a new schema from field definitions. `NewSchema` validates the schema by
default and returns an error for invalid field names, unsupported or missing
types, invalid nested `fields` / `elementFields` usage, invalid enum metadata,
duplicate flattened field paths, empty path segments, quote characters, or
SQL-control punctuation. Field path segments must be raw, unquoted identifier
tokens containing only letters, digits, and underscores. Segments outside the
portable unquoted ASCII shape, such as `24h` and `café`, are accepted and quoted
automatically by the transpiler.

### ValidateSchemaFields

```go
func ValidateSchemaFields(fields []FieldSchema) error
```

Validate schema field definitions without constructing a `Schema`. It applies
the same validation rules as `NewSchema`.

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
- [Parameterized Queries](parameterized-queries.md) - Bind-parameter output for safe SQL execution
- [Error Handling](error-handling.md) - Error codes and handling
- [Custom Operators](custom-operators.md) - Operator registration
