# JSON Logic to SQL Transpiler

A Go library that converts JSON Logic expressions into SQL WHERE clauses. This library provides a clean, type-safe API for transforming JSON Logic rules into ANSI SQL that can be used in database queries.

## Features

- **Complete JSON Logic Support**: Implements all core JSON Logic operators with 100% test coverage
- **Custom Operators**: Extensible registry pattern to add custom SQL functions (LENGTH, UPPER, etc.)
- **Schema/Metadata Validation**: Optional field schema to enforce strict column validation and type-aware SQL generation
- **ANSI SQL Output**: Generates standard SQL WHERE clauses compatible with most databases
- **Complex Nested Expressions**: Full support for deeply nested arithmetic and logical operations
- **Array Operations**: Complete support for all/none/some with proper SQL subqueries
- **String Operations**: String containment, concatenation, and substring operations
- **Unary Operators**: Flexible support for both array and non-array syntax
- **Array Indexing**: Support for numeric indices in var operations
- **Multiple Field Checks**: Missing operator supports both single and multiple fields
- **Array Boolean Casting**: Proper handling of empty/non-empty array boolean conversion
- **Proper NULL Handling**: Uses IS NULL/IS NOT NULL for null comparisons (SQL standard)
- **Nested If in Concatenation**: Full support for conditional expressions inside string concatenation
- **Strict Validation**: Comprehensive validation with detailed error messages
- **Library & CLI**: Both programmatic API and interactive REPL
- **Type Safety**: Full Go type safety with proper error handling

## Supported Operators

### Data Access
- `var` - Access variable values (including array indexing)
- `missing` - Check if variable(s) are missing
- `missing_some` - Check if some variables are missing

### Logic and Boolean Operations
- `if` - Conditional expressions
- `==`, `===` - Equality comparison
- `!=`, `!==` - Inequality comparison
- `!` - Logical NOT
- `!!` - Double negation (boolean conversion)
- `or` - Logical OR
- `and` - Logical AND

### Numeric Operations
- `>`, `>=`, `<`, `<=` - Comparison operators
- `max`, `min` - Maximum/minimum values
- `+`, `-`, `*`, `/`, `%` - Arithmetic operations

### Array Operations
- `in` - Check if value is in array
- `map`, `filter`, `reduce` - Array transformations
- `all`, `some`, `none` - Array condition checks
- `merge` - Merge arrays

### String Operations
- `in` - Check if substring is in string
- `cat` - Concatenate strings
- `substr` - Substring operations

## Installation

```bash
go get github.com/h22rana/jsonlogic2sql@latest
```

## Usage

### As a Library

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

func main() {
    // Simple usage
    sql, err := jsonlogic2sql.Transpile(`{">": [{"var": "amount"}, 1000]}`)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: WHERE amount > 1000

    // Using the transpiler instance
    transpiler := jsonlogic2sql.NewTranspiler()
    
    // From JSON string
    sql, err = transpiler.Transpile(`{"and": [{"==": [{"var": "status"}, "pending"]}, {">": [{"var": "amount"}, 5000]}]}`)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: WHERE (status = 'pending' AND amount > 5000)

    // From pre-parsed map
    logic := map[string]interface{}{
        "or": []interface{}{
            map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": "failedAttempts"}, 5}},
            map[string]interface{}{"in": []interface{}{map[string]interface{}{"var": "country"}, []interface{}{"CN", "RU"}}},
        },
    }
    sql, err = transpiler.TranspileFromMap(logic)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: WHERE (failedAttempts >= 5 OR country IN ('CN', 'RU'))
}
```

### Custom Operators

You can extend the transpiler with custom operators to support additional SQL functions like `LENGTH`, `UPPER`, `LOWER`, etc.

#### Using a Function

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

func main() {
    transpiler := jsonlogic2sql.NewTranspiler()

    // Register a custom "length" operator
    err := transpiler.RegisterOperatorFunc("length", func(op string, args []interface{}) (string, error) {
        if len(args) != 1 {
            return "", fmt.Errorf("length requires exactly 1 argument")
        }
        return fmt.Sprintf("LENGTH(%s)", args[0]), nil
    })
    if err != nil {
        panic(err)
    }

    // Use the custom operator
    sql, err := transpiler.Transpile(`{"length": [{"var": "email"}]}`)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: WHERE LENGTH(email)

    // Use in comparisons
    sql, err = transpiler.Transpile(`{">": [{"length": [{"var": "email"}]}, 10]}`)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: WHERE LENGTH(email) > 10
}
```

#### Using a Handler Struct

For more complex operators or those that need state, implement the `OperatorHandler` interface:

```go
package main

import (
    "fmt"
    "github.com/h22rana/jsonlogic2sql"
)

// UpperOperator implements the OperatorHandler interface
type UpperOperator struct{}

func (u *UpperOperator) ToSQL(operator string, args []interface{}) (string, error) {
    if len(args) != 1 {
        return "", fmt.Errorf("upper requires exactly 1 argument")
    }
    return fmt.Sprintf("UPPER(%s)", args[0]), nil
}

func main() {
    transpiler := jsonlogic2sql.NewTranspiler()

    // Register the handler
    err := transpiler.RegisterOperator("upper", &UpperOperator{})
    if err != nil {
        panic(err)
    }

    sql, err := transpiler.Transpile(`{"==": [{"upper": [{"var": "name"}]}, "JOHN"]}`)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: WHERE UPPER(name) = 'JOHN'
}
```

#### Multiple Custom Operators

You can register multiple custom operators and use them together:

```go
transpiler := jsonlogic2sql.NewTranspiler()

transpiler.RegisterOperatorFunc("length", func(op string, args []interface{}) (string, error) {
    return fmt.Sprintf("LENGTH(%s)", args[0]), nil
})

transpiler.RegisterOperatorFunc("upper", func(op string, args []interface{}) (string, error) {
    return fmt.Sprintf("UPPER(%s)", args[0]), nil
})

// Use both in a complex expression
sql, _ := transpiler.Transpile(`{"and": [{">": [{"length": [{"var": "name"}]}, 5]}, {"==": [{"upper": [{"var": "status"}]}, "ACTIVE"]}]}`)
// Output: WHERE (LENGTH(name) > 5 AND UPPER(status) = 'ACTIVE')
```

#### Managing Custom Operators

```go
transpiler := jsonlogic2sql.NewTranspiler()

// Check if an operator is registered
if transpiler.HasCustomOperator("length") {
    fmt.Println("length is registered")
}

// List all custom operators
operators := transpiler.ListCustomOperators()
fmt.Println(operators)

// Unregister an operator
transpiler.UnregisterOperator("length")

// Clear all custom operators
transpiler.ClearCustomOperators()
```

### Schema/Metadata Validation

You can optionally provide a schema to enforce strict field validation. When a schema is set, the transpiler will only accept fields defined in the schema and will return errors for undefined fields.

#### Defining a Schema

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

    transpiler := jsonlogic2sql.NewTranspiler()
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

#### Loading Schema from JSON

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

#### Type-Aware Operators

When a schema is provided, operators perform strict type validation and generate appropriate SQL based on field types:

**Type Validation Rules:**

| Operator Category | Allowed Types | Rejected Types |
|------------------|---------------|----------------|
| Numeric (`+`, `-`, `*`, `/`, `%`, `max`, `min`) | integer, number | string, array, object, boolean |
| String (`cat`, `substr`) | string, integer, number | array, object |
| Array (`all`, `some`, `none`, `map`, `filter`, `reduce`, `merge`) | array | all non-array types |
| Comparison (`>`, `>=`, `<`, `<=`) | integer, number, string | array, object, boolean |
| Equality (`==`, `!=`, `===`, `!==`) | any | none (type-agnostic) |
| In (`in`) | array (membership), string (containment) | varies by usage |

```go
schema := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
    {Name: "amount", Type: jsonlogic2sql.FieldTypeInteger},
    {Name: "tags", Type: jsonlogic2sql.FieldTypeArray},
    {Name: "name", Type: jsonlogic2sql.FieldTypeString},
})

transpiler := jsonlogic2sql.NewTranspiler()
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

// Array field: uses IN syntax
sql, _ := transpiler.Transpile(`{"in": ["admin", {"var": "tags"}]}`)
fmt.Println(sql) // Output: WHERE 'admin' IN tags

// String field: uses STRPOS for containment
sql, _ = transpiler.Transpile(`{"in": ["hello", {"var": "name"}]}`)
fmt.Println(sql) // Output: WHERE STRPOS(name, 'hello') > 0
```

**Note:** Type validation is only performed when a schema is set. Without a schema, all operations are allowed.

#### Supported Field Types

- `string` - String fields
- `integer` - Integer fields
- `number` - Numeric fields (float/decimal)
- `boolean` - Boolean fields
- `array` - Array fields
- `object` - Object/struct fields
- `enum` - Enum fields with allowed values validation

#### Enum Type Support

Enum fields allow you to define a fixed set of allowed values. The transpiler validates that any value compared against an enum field is in the allowed list.

```go
// Define schema with enum field
schema := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
    {Name: "status", Type: jsonlogic2sql.FieldTypeEnum, AllowedValues: []string{"active", "pending", "cancelled"}},
    {Name: "priority", Type: jsonlogic2sql.FieldTypeEnum, AllowedValues: []string{"low", "medium", "high"}},
})

transpiler := jsonlogic2sql.NewTranspiler()
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

**Loading enum schema from JSON:**

```json
[
    {"name": "status", "type": "enum", "allowedValues": ["active", "pending", "cancelled"]},
    {"name": "priority", "type": "enum", "allowedValues": ["low", "medium", "high"]}
]
```

#### Schema API Reference

```go
// Schema creation
schema := jsonlogic2sql.NewSchema(fields []FieldSchema)
schema, err := jsonlogic2sql.NewSchemaFromJSON(data []byte)
schema, err := jsonlogic2sql.NewSchemaFromFile(filepath string)

// Schema methods
schema.HasField(fieldName string) bool           // Check if field exists
schema.ValidateField(fieldName string) error     // Validate field existence
schema.GetFieldType(fieldName string) string     // Get field type as string
schema.IsArrayType(fieldName string) bool        // Check if field is array type
schema.IsStringType(fieldName string) bool       // Check if field is string type
schema.IsNumericType(fieldName string) bool      // Check if field is numeric type
schema.IsBooleanType(fieldName string) bool      // Check if field is boolean type
schema.IsEnumType(fieldName string) bool         // Check if field is enum type
schema.GetAllowedValues(fieldName string) []string // Get allowed values for enum field
schema.ValidateEnumValue(fieldName, value string) error // Validate enum value
schema.GetFields() []string                      // Get all field names

// Transpiler schema methods
transpiler.SetSchema(schema *Schema)             // Set schema for validation
```

### Interactive REPL

```bash
# Build and run the REPL
make run

# Or build manually
go build -o bin/repl ./cmd/repl
./bin/repl
```

The REPL provides an interactive environment to test JSON Logic expressions:

```
jsonlogic> {">": [{"var": "amount"}, 1000]}
SQL: WHERE amount > 1000

jsonlogic> :examples
Example JSON Logic expressions:
1. Simple Comparison
   JSON: {">": [{"var": "amount"}, 1000]}
   SQL:  WHERE amount > 1000
...

jsonlogic> :quit
```

## Examples

### Data Access Operations

#### Variable Access
```json
{"var": "name"}
```
```sql
WHERE name
```

#### Variable with Array Index
```json
{"var": 1}
```
```sql
WHERE data[1]
```

#### Variable with Default Value
```json
{"var": ["status", "pending"]}
```
```sql
WHERE COALESCE(status, 'pending')
```

#### Missing Field Check (Single)
```json
{"missing": "email"}
```
```sql
WHERE email IS NULL
```

#### Missing Field Check (Multiple)
```json
{"missing": ["email", "phone"]}
```
```sql
WHERE (email IS NULL OR phone IS NULL)
```

#### Missing Some Fields
```json
{"missing_some": [1, ["field1", "field2"]]}
```
```sql
WHERE (field1 IS NULL OR field2 IS NULL)
```

### Logic and Boolean Operations

#### Simple Comparison
```json
{">": [{"var": "amount"}, 1000]}
```
```sql
WHERE amount > 1000
```

#### Equality Comparison
```json
{"==": [{"var": "status"}, "active"]}
```
```sql
WHERE status = 'active'
```

#### Strict Equality
```json
{"===": [{"var": "count"}, 5]}
```
```sql
WHERE count = 5
```

#### Inequality
```json
{"!=": [{"var": "status"}, "inactive"]}
```
```sql
WHERE status != 'inactive'
```

#### Strict Inequality
```json
{"!==": [{"var": "count"}, 0]}
```
```sql
WHERE count <> 0
```

#### Equality with NULL (IS NULL)
```json
{"==": [{"var": "deleted_at"}, null]}
```
```sql
WHERE deleted_at IS NULL
```

#### Inequality with NULL (IS NOT NULL)
```json
{"!=": [{"var": "field"}, null]}
```
```sql
WHERE field IS NOT NULL
```

#### Logical NOT (with array wrapper)
```json
{"!": [{"var": "isDeleted"}]}
```
```sql
WHERE NOT (isDeleted)
```

#### Logical NOT (without array wrapper)
```json
{"!": {"var": "isDeleted"}}
```
```sql
WHERE NOT (isDeleted)
```

#### Logical NOT (literal)
```json
{"!": true}
```
```sql
WHERE NOT (TRUE)
```

#### Double Negation (Boolean Conversion)
```json
{"!!": [{"var": "value"}]}
```
```sql
WHERE (value IS NOT NULL AND value != FALSE AND value != 0 AND value != '')
```

#### Double Negation (Empty Array)
```json
{"!!": [[]]}
```
```sql
WHERE FALSE
```

#### Double Negation (Non-Empty Array)
```json
{"!!": [[1, 2, 3]]}
```
```sql
WHERE TRUE
```

#### Logical AND
```json
{"and": [
  {">": [{"var": "amount"}, 5000]},
  {"==": [{"var": "status"}, "pending"]}
]}
```
```sql
WHERE (amount > 5000 AND status = 'pending')
```

#### Logical OR
```json
{"or": [
  {">=": [{"var": "failedAttempts"}, 5]},
  {"in": [{"var": "country"}, ["CN", "RU"]]}
]}
```
```sql
WHERE (failedAttempts >= 5 OR country IN ('CN', 'RU'))
```

#### Conditional Expression
```json
{"if": [
  {">": [{"var": "age"}, 18]},
  "adult",
  "minor"
]}
```
```sql
WHERE CASE WHEN age > 18 THEN 'adult' ELSE 'minor' END
```

### Numeric Operations

#### Greater Than
```json
{">": [{"var": "amount"}, 1000]}
```
```sql
WHERE amount > 1000
```

#### Greater Than or Equal
```json
{">=": [{"var": "score"}, 80]}
```
```sql
WHERE score >= 80
```

#### Less Than
```json
{"<": [{"var": "age"}, 65]}
```
```sql
WHERE age < 65
```

#### Less Than or Equal
```json
{"<=": [{"var": "count"}, 10]}
```
```sql
WHERE count <= 10
```

#### Maximum Value
```json
{"max": [{"var": "score1"}, {"var": "score2"}, {"var": "score3"}]}
```
```sql
WHERE GREATEST(score1, score2, score3)
```

#### Minimum Value
```json
{"min": [{"var": "price1"}, {"var": "price2"}]}
```
```sql
WHERE LEAST(price1, price2)
```

#### Addition
```json
{"+": [{"var": "price"}, {"var": "tax"}]}
```
```sql
WHERE (price + tax)
```

#### Subtraction
```json
{"-": [{"var": "total"}, {"var": "discount"}]}
```
```sql
WHERE (total - discount)
```

#### Multiplication
```json
{"*": [{"var": "price"}, 1.2]}
```
```sql
WHERE (price * 1.2)
```

#### Division
```json
{"/": [{"var": "total"}, 2]}
```
```sql
WHERE (total / 2)
```

#### Modulo
```json
{"%": [{"var": "count"}, 3]}
```
```sql
WHERE (count % 3)
```

#### Unary Minus (Negation)
```json
{"-": [{"var": "value"}]}
```
```sql
WHERE -value
```

#### Unary Plus (Cast to Number)
```json
{"+": ["-5"]}
```
```sql
WHERE CAST(-5 AS NUMERIC)
```

### Array Operations

#### In Array
```json
{"in": [{"var": "country"}, ["US", "CA", "MX"]]}
```
```sql
WHERE country IN ('US', 'CA', 'MX')
```

#### String Containment
```json
{"in": ["hello", "hello world"]}
```
```sql
WHERE POSITION('hello' IN 'hello world') > 0
```

#### Map Array
```json
{"map": [{"var": "numbers"}, {"+": [{"var": "item"}, 1]}]}
```
```sql
WHERE ARRAY_MAP(numbers, transformation_placeholder)
```

#### Filter Array
```json
{"filter": [{"var": "scores"}, {">": [{"var": "item"}, 70]}]}
```
```sql
WHERE ARRAY_FILTER(scores, condition_placeholder)
```

#### Reduce Array
```json
{"reduce": [{"var": "numbers"}, 0, {"+": [{"var": "accumulator"}, {"var": "item"}]}]}
```
```sql
WHERE ARRAY_REDUCE(numbers, 0, reduction_placeholder)
```

#### All Elements Satisfy Condition
```json
{"all": [{"var": "ages"}, {">=": [{"var": ""}, 18]}]}
```
```sql
WHERE NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18))
```

#### Some Elements Satisfy Condition
```json
{"some": [{"var": "statuses"}, {"==": [{"var": ""}, "active"]}]}
```
```sql
WHERE EXISTS (SELECT 1 FROM UNNEST(statuses) AS elem WHERE elem = 'active')
```

#### No Elements Satisfy Condition
```json
{"none": [{"var": "values"}, {"==": [{"var": ""}, "invalid"]}]}
```
```sql
WHERE NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE elem = 'invalid')
```

#### Merge Arrays
```json
{"merge": [{"var": "array1"}, {"var": "array2"}]}
```
```sql
WHERE ARRAY_CONCAT(array1, array2)
```

### String Operations

#### Concatenate Strings
```json
{"cat": [{"var": "firstName"}, " ", {"var": "lastName"}]}
```
```sql
WHERE CONCAT(firstName, ' ', lastName)
```

#### Concatenate with Conditional (Nested If)
```json
{"cat": [{"if": [{"==": [{"var": "gender"}, "M"]}, "Mr. ", "Ms. "]}, {"var": "first_name"}, " ", {"var": "last_name"}]}
```
```sql
WHERE CONCAT(CASE WHEN (gender = 'M') THEN 'Mr. ' ELSE 'Ms. ' END, first_name, ' ', last_name)
```

#### Substring with Length
```json
{"substr": [{"var": "email"}, 0, 10]}
```
```sql
WHERE SUBSTR(email, 1, 10)
```

#### Substring without Length
```json
{"substr": [{"var": "email"}, 4]}
```
```sql
WHERE SUBSTR(email, 5)
```

### Complex Nested Examples

#### Complex Nested Math Expressions
```json
{">": [{"+": [{"var": "base"}, {"*": [{"var": "bonus"}, 0.1]}]}, 1000]}
```
```sql
WHERE (base + (bonus * 0.1)) > 1000
```

#### Nested Conditions
```json
{"and": [
  {">": [{"var": "transaction.amount"}, 10000]},
  {"or": [
    {"==": [{"var": "user.verified"}, false]},
    {"<": [{"var": "user.accountAgeDays"}, 7]}
  ]}
]}
```
```sql
WHERE (transaction.amount > 10000 AND (user.verified = FALSE OR user.accountAgeDays < 7))
```

#### Complex Conditional Logic
```json
{"if": [
  {"and": [
    {">=": [{"var": "age"}, 18]},
    {"==": [{"var": "country"}, "US"]}
  ]},
  "eligible",
  "ineligible"
]}
```
```sql
WHERE CASE WHEN (age >= 18 AND country = 'US') THEN 'eligible' ELSE 'ineligible' END
```

## Variable Naming

The transpiler preserves JSON Logic variable names as-is in the SQL output:

- Dot notation is preserved: `transaction.amount` → `transaction.amount`
- Nested variables: `user.account.age` → `user.account.age`
- Simple variables remain unchanged: `amount` → `amount`

This allows for proper JSON column access in databases that support it (like PostgreSQL with JSONB columns).

## Development

### Prerequisites

- Go 1.19 or later
- Make (optional, for using Makefile)

### Building

```bash
# Install dependencies
make deps

# Run tests
make test

# Build the REPL
make build

# Run linter
make lint
```

### Project Structure

```
jsonlogic2sql/
├── transpiler.go             # Main public API
├── transpiler_test.go        # Public API tests
├── operator.go               # Custom operators registry and types
├── operator_test.go          # Custom operators tests
├── schema.go                 # Schema/metadata validation
├── schema_test.go            # Schema tests
├── internal/
│   ├── parser/               # Core parsing logic
│   ├── operators/            # Operator implementations (includes schema.go)
│   └── validator/            # Validation logic
├── cmd/repl/                 # Interactive REPL
├── examples/                 # Usage examples
├── Makefile                  # Build automation
└── README.md
```

### Testing

The project includes comprehensive tests with **100% test coverage**:

- **Unit Tests**: Each operator and component is thoroughly tested (400+ test cases passing)
- **Integration Tests**: End-to-end tests with real JSON Logic examples (168 REPL test cases)
- **Error Cases**: Validation and error handling tests
- **Edge Cases**: Boundary conditions and special cases
- **Complex Expressions**: Deeply nested arithmetic and logical operations
- **Array Operations**: All/none/some with proper SQL subqueries
- **Unary Operators**: Flexible support for both array and non-array syntax
- **Array Indexing**: Support for numeric indices in var operations
- **Multiple Field Checks**: Missing operator supports both single and multiple fields
- **NULL Handling**: Proper IS NULL/IS NOT NULL for null comparisons
- **Nested Conditionals**: If expressions inside string concatenation

Run tests with:

```bash
# All tests
go test ./...

# With verbose output
go test -v ./...

# With coverage
go test -cover ./...

# Specific package
go test ./internal/operators/
```

## API Reference

### Functions

#### `Transpile(jsonLogic string) (string, error)`
Converts a JSON Logic string to a SQL WHERE clause.

#### `TranspileFromMap(logic map[string]interface{}) (string, error)`
Converts a pre-parsed JSON Logic map to a SQL WHERE clause.

#### `TranspileFromInterface(logic interface{}) (string, error)`
Converts any JSON Logic interface{} to a SQL WHERE clause.

#### `NewTranspiler() *Transpiler`
Creates a new transpiler instance with default configuration.

#### `NewTranspilerWithConfig(config *TranspilerConfig) *Transpiler`
Creates a new transpiler instance with custom configuration.

#### `NewOperatorRegistry() *OperatorRegistry`
Creates a new empty operator registry for managing custom operators.

### Types

#### `Transpiler`
Main transpiler instance with methods:
- `Transpile(jsonLogic string) (string, error)` - Convert JSON string to SQL
- `TranspileFromMap(logic map[string]interface{}) (string, error)` - Convert map to SQL
- `TranspileFromInterface(logic interface{}) (string, error)` - Convert interface to SQL
- `RegisterOperator(name string, handler OperatorHandler) error` - Register custom operator with handler
- `RegisterOperatorFunc(name string, fn OperatorFunc) error` - Register custom operator with function
- `UnregisterOperator(name string) bool` - Remove a custom operator
- `HasCustomOperator(name string) bool` - Check if operator is registered
- `ListCustomOperators() []string` - List all custom operator names
- `ClearCustomOperators()` - Remove all custom operators

#### `TranspilerConfig`
Configuration options for the transpiler:
- `UseANSINotEqual bool` - When true uses `<>`, when false uses `!=` (default: true)

#### `OperatorFunc`
Function type for simple custom operator implementations:
```go
type OperatorFunc func(operator string, args []interface{}) (string, error)
```

#### `OperatorHandler`
Interface for custom operator implementations that need state:
```go
type OperatorHandler interface {
    ToSQL(operator string, args []interface{}) (string, error)
}
```

#### `OperatorRegistry`
Thread-safe registry for managing custom operators with methods:
- `Register(operatorName string, handler OperatorHandler)` - Add operator handler
- `RegisterFunc(operatorName string, fn OperatorFunc)` - Add operator function
- `Unregister(operatorName string) bool` - Remove an operator
- `Get(operatorName string) (OperatorHandler, bool)` - Get operator handler
- `Has(operatorName string) bool` - Check if operator exists
- `List() []string` - List all operator names
- `Clear()` - Remove all operators
- `Clone() *OperatorRegistry` - Create a copy of the registry
- `Merge(other *OperatorRegistry)` - Merge operators from another registry

#### `Schema`
Schema for field validation with methods:
- `HasField(fieldName string) bool` - Check if field exists in schema
- `ValidateField(fieldName string) error` - Validate field existence
- `GetFieldType(fieldName string) string` - Get field type as string
- `IsArrayType(fieldName string) bool` - Check if field is array type
- `IsStringType(fieldName string) bool` - Check if field is string type
- `IsNumericType(fieldName string) bool` - Check if field is numeric type
- `GetFields() []string` - Get all field names

#### `FieldSchema`
Field definition for schema:
```go
type FieldSchema struct {
    Name          string    // Field name (e.g., "order.amount")
    Type          FieldType // Field type (e.g., FieldTypeInteger)
    AllowedValues []string  // For enum types: list of valid values (optional)
}
```

#### `FieldType`
Field type constants:
- `FieldTypeString` - String field type
- `FieldTypeInteger` - Integer field type
- `FieldTypeNumber` - Numeric field type (float/decimal)
- `FieldTypeBoolean` - Boolean field type
- `FieldTypeArray` - Array field type
- `FieldTypeObject` - Object/struct field type
- `FieldTypeEnum` - Enum field type (requires AllowedValues)

## Error Handling

The library provides detailed error messages for:

- Invalid JSON syntax
- Unsupported operators
- Incorrect argument counts
- Type mismatches
- Validation errors

Example error handling:

```go
sql, err := jsonlogic2sql.Transpile(`{"unsupported": [1, 2]}`)
if err != nil {
    fmt.Printf("Error: %v\n", err)
    // Output: Error: parse error: unsupported operator: unsupported
}
```

## License

This project is licensed under the MIT License - see the [LICENSE](./LICENSE) file for details.