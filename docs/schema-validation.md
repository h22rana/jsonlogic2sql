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
    schema, err := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
        {Name: "order.amount", Type: jsonlogic2sql.FieldTypeInteger},
        {Name: "order.status", Type: jsonlogic2sql.FieldTypeString},
        {Name: "user.verified", Type: jsonlogic2sql.FieldTypeBoolean},
        {Name: "user.roles", Type: jsonlogic2sql.FieldTypeArray},
    })
    if err != nil {
        panic(err)
    }

    transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
    transpiler.SetSchema(schema)

    // Valid field - works
    sql, err := transpiler.TranspileCondition(`{"==": [{"var": "order.status"}, "active"]}`)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: order.status = 'active'

    // Invalid field - returns error
    _, err = transpiler.TranspileCondition(`{"==": [{"var": "invalid.field"}, "value"]}`)
    if err != nil {
        fmt.Println(err) // Output: field 'invalid.field' is not defined in schema
    }
}
```

**Note:** Field names must be raw, unquoted identifiers. The transpiler handles identifier quoting automatically based on the target dialect. `NewSchema`, `NewSchemaFromJSON`, and `NewSchemaFromFile` return construction-time errors for schema field names that contain quote characters (backtick, double quote, or single quote).

## Validated Schema Construction

```go
schema, err := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
    {Name: "order.amount", Type: jsonlogic2sql.FieldTypeInteger},
    {Name: "order.status", Type: jsonlogic2sql.FieldTypeString},
})
if err != nil {
    panic(err)
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

## Nested Object and Array Element Fields

Schemas can describe object fields with `fields` and array element fields with
`elementFields`. The transpiler flattens those definitions internally for type
validation, while array lambdas still emit SQL relative to the element alias.
`fields` is valid only on `object` fields, and `elementFields` is valid only on
`array` fields.

```json
[
  {
    "name": "profile",
    "type": "object",
    "fields": [
      { "name": "country", "type": "string" },
      {
        "name": "status",
        "type": "enum",
        "allowedValues": ["active", "blocked"]
      }
    ]
  },
  {
    "name": "payment_methods",
    "type": "array",
    "elementFields": [
      {
        "name": "type",
        "type": "enum",
        "allowedValues": ["BALANCE", "CARD"]
      },
      { "name": "amount", "type": "number" },
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

Those entries define these schema paths: `profile.country`,
`profile.status`, `payment_methods.type`, `payment_methods.amount`, and
`payment_methods.details.issuer`.

```json
{"some":[{"var":"payment_methods"},{"==":[{"var":"type"},"BALANCE"]}]}
```

In a lambda, `{"var":"type"}` resolves against the current array element and
is validated as `payment_methods.type`, then emitted as `elem.type`.
Unknown scoped fields are rejected in schema-aware mode.

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
| Equality (`==`, `!=`, `===`, `!==`) | any | unsupported loose string/enum-string vs boolean literal |
| In (`in`) | array (membership), string (containment) | varies by usage |

### Example

```go
schema, err := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
    {Name: "amount", Type: jsonlogic2sql.FieldTypeInteger},
    {Name: "tags", Type: jsonlogic2sql.FieldTypeArray},
    {Name: "name", Type: jsonlogic2sql.FieldTypeString},
})
if err != nil {
    panic(err)
}

transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
transpiler.SetSchema(schema)

// Valid: numeric operation on integer field
sql, _ := transpiler.TranspileValue(`{"+": [{"var": "amount"}, 10]}`)
fmt.Println(sql) // Output: (amount + 10)

// Valid: array operation on array field
sql, _ = transpiler.TranspileCondition(`{"some": [{"var": "tags"}, {"==": [{"var": ""}, "important"]}]}`)
fmt.Println(sql) // Output: EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE elem = 'important')

// Error: numeric operation on string field
_, err := transpiler.TranspileValue(`{"+": [{"var": "name"}, 10]}`)
// Error: numeric operation on non-numeric field 'name' (type: string)

// Error: array operation on non-array field
_, err = transpiler.TranspileCondition(`{"some": [{"var": "amount"}, {"==": [{"var": ""}, 0]}]}`)
// Error: array operation on non-array field 'amount' (type: integer)
```

### In Operator Behavior

The `in` operator behavior depends on the field type:

```go
// Array field: uses dialect-specific array membership syntax
sql, _ := transpiler.TranspileCondition(`{"in": ["admin", {"var": "tags"}]}`)
fmt.Println(sql)
// BigQuery/Spanner: 'admin' IN UNNEST(tags)
// PostgreSQL:       'admin' = ANY(tags)
// DuckDB:           list_contains(tags, 'admin')
// ClickHouse:       has(tags, 'admin')

// String field: uses STRPOS for containment
sql, _ = transpiler.TranspileCondition(`{"in": ["hello", {"var": "name"}]}`)
fmt.Println(sql) // Output: STRPOS(name, 'hello') > 0
```

### In Operator Behavior Without Schema

When no schema is provided, `in` uses heuristics to infer whether to generate:

- string containment (`STRPOS` / `POSITION` / `position`)
- array membership (`IN UNNEST` / `= ANY` / `list_contains` / `has`)

Current heuristics treat obvious string-producing left operands as containment, including:

- string literals
- string SQL literals
- string expressions rooted at `cat` or `substr`

For deterministic behavior across all expression shapes (especially custom operators), use schema-aware mode.

### Type Coercion

When a schema is provided, the transpiler automatically coerces literal values to match the field's type. This prevents type errors in strict-typing databases like BigQuery and Spanner.

**Number to String** - When a string field is compared with numeric literals, the numbers are coerced to quoted strings:

```go
// Schema: category_code is string type
sql, _ := transpiler.TranspileCondition(`{"in": [{"var": "category_code"}, [5960, 9000]]}`)
fmt.Println(sql)
// Output: category_code IN ('5960', '9000')
// Without schema: category_code IN (5960, 9000) - would fail in BigQuery
```

For equality, this is a canonical string match. For example, `code == 5` emits
`code = '5'`; it does not also match strings that JavaScript would coerce to the
same number at runtime, such as `"05"`, `"5.0"`, or `" 5 "`.

**String to Number** - When a numeric field is compared with string literals that are valid numbers, the strings are coerced to unquoted numbers:

```go
// Schema: amount is integer type
sql, _ := transpiler.TranspileCondition(`{">=": [{"var": "amount"}, "50000"]}`)
fmt.Println(sql)
// Output: amount >= 50000
// Without schema: amount >= '50000'
```

For equality and inequality, numeric string literals follow JavaScript-like
`ToNumber` semantics where the value is known at transpile time:

```go
// Schema: amount is integer type
sql, _ = transpiler.TranspileCondition(`{"==": [{"var": "amount"}, "010"]}`)
fmt.Println(sql)
// Output: amount = 10

sql, _ = transpiler.TranspileCondition(`{"==": [{"var": "amount"}, "abc"]}`)
fmt.Println(sql)
// Output: FALSE
```

**Boolean Equality** - When a boolean field is compared with numeric or string
literals, equality and inequality coerce through JavaScript-like `ToNumber`
semantics:

```go
// Schema: active is boolean type
sql, _ := transpiler.TranspileCondition(`{"==": [{"var": "active"}, "1"]}`)
fmt.Println(sql)
// Output: active = TRUE

sql, _ = transpiler.TranspileCondition(`{"==": [{"var": "active"}, "2"]}`)
fmt.Println(sql)
// Output: FALSE
```

**Strict Equality** - With a schema, `===` and `!==` fold known field/literal type
mismatches before loose coercion:

```go
// Schema: amount is integer type
sql, _ = transpiler.TranspileCondition(`{"===": [{"var": "amount"}, "5"]}`)
fmt.Println(sql)
// Output: FALSE
```

Loose equality between string or enum-string fields and boolean literals is not
portable SQL and returns an error:

```go
// Schema: code is string type
_, err := transpiler.TranspileCondition(`{"==": [{"var": "code"}, true]}`)
// Error: loose equality between string field "code" and boolean literal is not supported
```

For string and enum fields, numeric literals are converted to their canonical
JavaScript string form before comparison. This includes non-finite JSON numeric
values produced by overflow:

```go
// Schema: code is string type
sql, _ := transpiler.TranspileCondition(`{"==": [{"var": "code"}, 1e400]}`)
fmt.Println(sql)
// Output: code = 'Infinity'
```

**Defaulted Variables** - Equality and inequality apply the same schema-aware
literal coercion to `[field, default]` vars while preserving the `COALESCE`
expression emitted by the `var` operator:

```go
// Schema: price is integer type
sql, _ = transpiler.TranspileCondition(`{"==": [{"var": ["price", 0]}, "50"]}`)
fmt.Println(sql)
// Output: COALESCE(price, 0) = 50

sql, _ = transpiler.TranspileCondition(`{"===": [{"var": ["price", 0]}, "50"]}`)
fmt.Println(sql)
// Output: FALSE

sql, _ = transpiler.TranspileCondition(`{"===": [{"var": ["price", "50"]}, "50"]}`)
fmt.Println(sql)
// Output: COALESCE(price, '50') = '50'
```

Strict or value-space folds only happen when both the field value and the
visible default cannot match. Expression defaults are not folded because their
runtime value is unknown. The default value itself is emitted as provided by the
`var` operator; it is not coerced to the schema type before `COALESCE`.

Basic schema coercion applies to comparison operators (`==`, `!=`, `>`, `>=`, `<`, `<=`), the `in` operator with array literals, and string containment checks. Equality and inequality add the JS-aware literal handling described above. Schema coercion also applies to comparisons nested within numeric expressions (e.g., `{"+": [{"==": [{"var": "status"}, 123]}, 0]}` correctly coerces `123` to `'123'` for a string field).

**Numeric String Coercion** - In numeric operations (`+`, `-`, `*`, `/`, `%`), string operands are coerced per JSONLogic's JavaScript-like semantics. Valid numeric strings are converted to numbers, whitespace is trimmed, and non-numeric strings are safely quoted:

```go
sql, _ := transpiler.TranspileValue(`{"+": ["42", 1]}`)
fmt.Println(sql)
// Output: (42 + 1)
// "42" coerced to number; "hello" would become 'hello'
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

Without a schema, field truthiness is rejected with `ErrInvalidExpressionContext`
instead of emitting non-portable mixed-type comparisons. Add field types to the
schema before using a `var` operand directly in `!!`, `!`, value-mode `and` /
`or`, or value-mode `if` conditions.

## Enum Type Support

Enum fields allow you to define a fixed set of allowed values:

```go
// Define schema with enum field
schema, err := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
    {Name: "status", Type: jsonlogic2sql.FieldTypeEnum, AllowedValues: []string{"active", "pending", "cancelled"}},
    {Name: "priority", Type: jsonlogic2sql.FieldTypeEnum, AllowedValues: []string{"low", "medium", "high"}},
})
if err != nil {
    panic(err)
}

transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
transpiler.SetSchema(schema)

// Valid enum value - works
sql, err := transpiler.TranspileCondition(`{"==": [{"var": "status"}, "active"]}`)
// Output: status = 'active'

// Valid enum IN array - works
sql, err = transpiler.TranspileCondition(`{"in": [{"var": "status"}, ["active", "pending"]]}`)
// Output: status IN ('active', 'pending')

// Invalid enum value - returns error
_, err = transpiler.TranspileCondition(`{"==": [{"var": "status"}, "invalid"]}`)
// Error: invalid enum value 'invalid' for field 'status': allowed values are [active pending cancelled]
```

Visible enum defaults are also validated because they become literal SQL inside
`COALESCE`:

```go
_, err = transpiler.TranspileCondition(`{"==": [{"var": ["status", "unknown"]}, "active"]}`)
// Error: invalid enum value 'unknown' for field 'status': allowed values are [active pending cancelled]
```

Expression defaults cannot be validated statically and are left to the generated
SQL expression.

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
schema, err := jsonlogic2sql.NewSchema(fields)
err = jsonlogic2sql.ValidateSchemaFields(fields)
schema, err = jsonlogic2sql.NewSchemaFromJSON(data)
schema, err = jsonlogic2sql.NewSchemaFromFile(filepath)

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
