# Schema Validation

You can optionally provide a schema to enforce strict field validation. When a schema is set, the transpiler will only accept fields defined in the schema and will return errors for undefined fields.

## Defining a Schema

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

func main() {
    // Create a schema with field definitions
    schema := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
        {Name: "order.amount", Type: jsonlogic2sql.FieldTypeInteger},
        {Name: "order.status", Type: jsonlogic2sql.FieldTypeString},
        {Name: "user.verified", Type: jsonlogic2sql.FieldTypeBoolean},
        {Name: "user.roles", Type: jsonlogic2sql.FieldTypeArray},
    })

    transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
    transpiler.SetSchema(schema)

    // Valid field - works
    sql, err := transpiler.Transpile(`{"==": [{"var": "order.status"}, "active"]}`)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: WHERE order.status = 'active'

    // Invalid field - returns error
    _, err = transpiler.Transpile(`{"==": [{"var": "invalid.field"}, "value"]}`)
    if err != nil {
        fmt.Println(err) // Output: field 'invalid.field' is not defined in schema
    }
}
```

## Loading Schema from JSON

```go
// From JSON string
schemaJSON := `[
    {"name": "order.amount", "type": "integer"},
    {"name": "order.status", "type": "string"},
    {"name": "user.verified", "type": "boolean"},
    {"name": "user.roles", "type": "array"}
]`

schema, err := jsonlogic2sql.NewSchemaFromJSON([]byte(schemaJSON))
if err != nil {
    panic(err)
}

// From JSON file
schema, err = jsonlogic2sql.NewSchemaFromFile("schema.json")
if err != nil {
    panic(err)
}
```

## Supported Field Types

| Type | Constant | Description |
|------|----------|-------------|
| `string` | `FieldTypeString` | String fields |
| `integer` | `FieldTypeInteger` | Integer fields |
| `number` | `FieldTypeNumber` | Numeric fields (float/decimal) |
| `boolean` | `FieldTypeBoolean` | Boolean fields |
| `array` | `FieldTypeArray` | Array fields |
| `object` | `FieldTypeObject` | Object/struct fields |
| `enum` | `FieldTypeEnum` | Enum fields with allowed values |

## Type-Aware Operators

When a schema is provided, operators perform strict type validation:

| Operator Category | Allowed Types | Rejected Types |
|------------------|---------------|----------------|
| Numeric (`+`, `-`, `*`, `/`, `%`, `max`, `min`) | integer, number | string, array, object, boolean |
| String (`cat`, `substr`) | string, integer, number | array, object |
| Array (`all`, `some`, `none`, `map`, `filter`, `reduce`, `merge`) | array | all non-array types |
| Comparison (`>`, `>=`, `<`, `<=`) | integer, number, string | array, object, boolean |
| Equality (`==`, `!=`, `===`, `!==`) | any | none (type-agnostic) |
| In (`in`) | array (membership), string (containment) | varies by usage |

### Example

```go
schema := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
    {Name: "amount", Type: jsonlogic2sql.FieldTypeInteger},
    {Name: "tags", Type: jsonlogic2sql.FieldTypeArray},
    {Name: "name", Type: jsonlogic2sql.FieldTypeString},
})

transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
transpiler.SetSchema(schema)

// Valid: numeric operation on integer field
sql, _ := transpiler.Transpile(`{"+": [{"var": "amount"}, 10]}`)
fmt.Println(sql) // Output: WHERE (amount + 10)

// Valid: array operation on array field
sql, _ = transpiler.Transpile(`{"some": [{"var": "tags"}, {"==": [{"var": ""}, "important"]}]}`)
fmt.Println(sql) // Output: WHERE EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE elem = 'important')

// Error: numeric operation on string field
_, err := transpiler.Transpile(`{"+": [{"var": "name"}, 10]}`)
// Error: numeric operation on non-numeric field 'name' (type: string)

// Error: array operation on non-array field
_, err = transpiler.Transpile(`{"some": [{"var": "amount"}, {"==": [{"var": ""}, 0]}]}`)
// Error: array operation on non-array field 'amount' (type: integer)
```

### In Operator Behavior

The `in` operator behavior depends on the field type:

```go
// Array field: uses IN syntax
sql, _ := transpiler.Transpile(`{"in": ["admin", {"var": "tags"}]}`)
fmt.Println(sql) // Output: WHERE 'admin' IN tags

// String field: uses STRPOS for containment
sql, _ = transpiler.Transpile(`{"in": ["hello", {"var": "name"}]}`)
fmt.Println(sql) // Output: WHERE STRPOS(name, 'hello') > 0
```

## Schema-Aware Truthiness

When a schema is provided, the `!!` operator generates type-appropriate SQL to avoid type mismatch errors in strongly-typed databases:

| Field Type | JSONLogic | Generated SQL |
|------------|-----------|---------------|
| Boolean | `{"!!": {"var": "is_verified"}}` | `is_verified IS TRUE` |
| String | `{"!!": {"var": "name"}}` | `(name IS NOT NULL AND name != '')` |
| Integer/Number | `{"!!": {"var": "amount"}}` | `(amount IS NOT NULL AND amount != 0)` |
| Array (BigQuery/Spanner/PostgreSQL/DuckDB) | `{"!!": {"var": "tags"}}` | `(tags IS NOT NULL AND CARDINALITY(tags) > 0)` |
| Array (ClickHouse) | `{"!!": {"var": "tags"}}` | `(tags IS NOT NULL AND length(tags) > 0)` |

Without a schema, the generic truthiness check is used:
```sql
WHERE (value IS NOT NULL AND value != FALSE AND value != 0 AND value != '')
```

## Enum Type Support

Enum fields allow you to define a fixed set of allowed values:

```go
// Define schema with enum field
schema := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
    {Name: "status", Type: jsonlogic2sql.FieldTypeEnum, AllowedValues: []string{"active", "pending", "cancelled"}},
    {Name: "priority", Type: jsonlogic2sql.FieldTypeEnum, AllowedValues: []string{"low", "medium", "high"}},
})

transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
transpiler.SetSchema(schema)

// Valid enum value - works
sql, err := transpiler.Transpile(`{"==": [{"var": "status"}, "active"]}`)
// Output: WHERE status = 'active'

// Valid enum IN array - works
sql, err = transpiler.Transpile(`{"in": [{"var": "status"}, ["active", "pending"]]}`)
// Output: WHERE status IN ('active', 'pending')

// Invalid enum value - returns error
_, err = transpiler.Transpile(`{"==": [{"var": "status"}, "invalid"]}`)
// Error: invalid enum value 'invalid' for field 'status': allowed values are [active pending cancelled]
```

### Enum Schema JSON Format

```json
[
    {"name": "status", "type": "enum", "allowedValues": ["active", "pending", "cancelled"]},
    {"name": "priority", "type": "enum", "allowedValues": ["low", "medium", "high"]}
]
```

## Schema API Reference

```go
// Schema creation
schema := jsonlogic2sql.NewSchema(fields []FieldSchema)
schema, err := jsonlogic2sql.NewSchemaFromJSON(data []byte)
schema, err := jsonlogic2sql.NewSchemaFromFile(filepath string)

// Schema methods
schema.HasField(fieldName string) bool              // Check if field exists
schema.ValidateField(fieldName string) error        // Validate field existence
schema.GetFieldType(fieldName string) string        // Get field type as string
schema.IsArrayType(fieldName string) bool           // Check if field is array type
schema.IsStringType(fieldName string) bool          // Check if field is string type
schema.IsNumericType(fieldName string) bool         // Check if field is numeric type
schema.IsBooleanType(fieldName string) bool         // Check if field is boolean type
schema.IsEnumType(fieldName string) bool            // Check if field is enum type
schema.GetAllowedValues(fieldName string) []string  // Get allowed values for enum field
schema.ValidateEnumValue(fieldName, value string) error // Validate enum value
schema.GetFields() []string                         // Get all field names

// Transpiler schema methods
transpiler.SetSchema(schema *Schema)                // Set schema for validation
```

## See Also

- [Getting Started](getting-started.md) - Basic usage
- [Operators](operators.md) - All supported operators
- [Error Handling](error-handling.md) - Schema validation errors
