# Error Handling

jsonlogic2sql provides structured errors with error codes for programmatic error handling.

## TranspileError

All errors are wrapped in a `TranspileError` type that includes:

| Field | Description |
|-------|-------------|
| `Code` | A unique error code (e.g., `E100`, `E302`) |
| `Operator` | The operator that caused the error |
| `Path` | JSONPath to the error location (e.g., `$.and[0].>`) |
| `Message` | Human-readable description |
| `Cause` | The underlying error (if any) |

## Error Codes

### Categories

| Category | Code Range | Description |
|----------|------------|-------------|
| Structural/Validation | E001-E099 | JSON structure and validation errors |
| Operator-specific | E100-E199 | Operator-related errors |
| Type/Schema | E200-E299 | Type mismatch and schema errors |
| Argument | E300-E399 | Argument validation errors |

### Complete Error Code Reference

#### Structural/Validation Errors (E001-E099)

| Code | Constant | Description |
|------|----------|-------------|
| E001 | `ErrInvalidExpression` | Invalid expression structure |
| E002 | `ErrEmptyArray` | Empty array not allowed |
| E003 | `ErrMultipleKeys` | Multiple keys in expression |
| E004 | `ErrPrimitiveNotAllowed` | Primitive value not allowed |
| E005 | `ErrArrayNotAllowed` | Array not allowed in context |
| E006 | `ErrValidation` | General validation error |
| E007 | `ErrInvalidJSON` | Invalid JSON syntax |

#### Operator-specific Errors (E100-E199)

| Code | Constant | Description |
|------|----------|-------------|
| E100 | `ErrUnsupportedOperator` | Operator not supported |
| E101 | `ErrOperatorRequiresArray` | Operator requires array argument |
| E102 | `ErrCustomOperatorFailed` | Custom operator execution failed |

#### Type/Schema Errors (E200-E299)

| Code | Constant | Description |
|------|----------|-------------|
| E200 | `ErrTypeMismatch` | Type mismatch in operation |
| E201 | `ErrFieldNotInSchema` | Field not defined in schema |
| E202 | `ErrInvalidFieldType` | Invalid field type |
| E203 | `ErrInvalidEnumValue` | Invalid enum value |

#### Argument Errors (E300-E399)

| Code | Constant | Description |
|------|----------|-------------|
| E300 | `ErrInsufficientArgs` | Not enough arguments |
| E301 | `ErrTooManyArgs` | Too many arguments |
| E302 | `ErrInvalidArgument` | Invalid argument value |
| E303 | `ErrInvalidArgType` | Invalid argument type |
| E304 | `ErrInvalidDefaultValue` | Invalid default value |

## Programmatic Error Handling

### Method 1: Use Helper Function

```go
sql, err := transpiler.Transpile(jsonLogic)
if err != nil {
    if tpErr, ok := jsonlogic2sql.AsTranspileError(err); ok {
        fmt.Printf("Error code: %s\n", tpErr.Code)    // e.g., "E100"
        fmt.Printf("Path: %s\n", tpErr.Path)          // e.g., "$.and[0].unknown"
        fmt.Printf("Operator: %s\n", tpErr.Operator)  // e.g., "unknown"
        fmt.Printf("Message: %s\n", tpErr.Message)    // Human-readable message
    }
}
```

### Method 2: Check Specific Error Code

```go
sql, err := transpiler.Transpile(jsonLogic)
if err != nil {
    if jsonlogic2sql.IsErrorCode(err, jsonlogic2sql.ErrUnsupportedOperator) {
        // Handle unsupported operator specifically
        log.Warn("Unknown operator used in expression")
    }
}
```

### Method 3: Use Standard errors.As

```go
sql, err := transpiler.Transpile(jsonLogic)
if err != nil {
    var tpErr *jsonlogic2sql.TranspileError
    if errors.As(err, &tpErr) {
        switch tpErr.Code {
        case jsonlogic2sql.ErrInvalidJSON:
            // Handle invalid JSON
        case jsonlogic2sql.ErrValidation:
            // Handle validation error
        case jsonlogic2sql.ErrFieldNotInSchema:
            // Handle unknown field
        }
    }
}
```

## Example Error Output

```
Error: [E100] at $.and.unknown[0] (operator: unknown): unsupported operator: unknown
Error: [E007]: invalid JSON: invalid character 'i' looking for beginning of object key string
Error: [E302] at $.var (operator: var): operator error: field 'bad.field' is not defined in schema
```

## Common Error Scenarios

### Invalid JSON

```go
_, err := transpiler.Transpile(`{invalid json}`)
// Error: [E007]: invalid JSON: ...
```

### Unsupported Operator

```go
_, err := transpiler.Transpile(`{"unknownOp": [1, 2]}`)
// Error: [E100] at $.unknownOp (operator: unknownOp): unsupported operator: unknownOp
```

### Field Not in Schema

```go
schema := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
    {Name: "known_field", Type: jsonlogic2sql.FieldTypeString},
})
transpiler.SetSchema(schema)

_, err := transpiler.Transpile(`{"==": [{"var": "unknown_field"}, "test"]}`)
// Error: [E201] at $.==.var (operator: var): field 'unknown_field' is not defined in schema
```

### Invalid Enum Value

```go
schema := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
    {Name: "status", Type: jsonlogic2sql.FieldTypeEnum, AllowedValues: []string{"active", "pending"}},
})
transpiler.SetSchema(schema)

_, err := transpiler.Transpile(`{"==": [{"var": "status"}, "invalid"]}`)
// Error: [E203]: invalid enum value 'invalid' for field 'status': allowed values are [active pending]
```

### Type Mismatch

```go
schema := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
    {Name: "name", Type: jsonlogic2sql.FieldTypeString},
})
transpiler.SetSchema(schema)

_, err := transpiler.Transpile(`{"+": [{"var": "name"}, 10]}`)
// Error: [E200]: numeric operation on non-numeric field 'name' (type: string)
```

### Insufficient Arguments

```go
_, err := transpiler.Transpile(`{">": [{"var": "amount"}]}`)
// Error: [E300] at $.> (operator: >): insufficient arguments
```

## See Also

- [API Reference](api-reference.md) - Full API documentation
- [Schema Validation](schema-validation.md) - Schema-related errors
