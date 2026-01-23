# JSON Logic to SQL Transpiler

A Go library that converts JSON Logic expressions into SQL. This library provides a clean, type-safe API for transforming JSON Logic rules into SQL WHERE clauses or standalone conditions, with support for multiple SQL dialects.

## Features

- **Complete JSON Logic Support**: Implements all core JSON Logic operators with comprehensive test coverage
- **SQL Dialect Support**: Target BigQuery, Spanner, PostgreSQL, DuckDB, or ClickHouse with dialect-specific SQL generation
- **Custom Operators**: Extensible registry pattern to add custom SQL functions (LENGTH, UPPER, etc.)
- **Dialect-Aware Custom Operators**: Register operators that generate different SQL per dialect
- **Nested Custom Operators**: Custom operators work seamlessly when nested inside built-in operators (`cat`, `if`, `and`, etc.)
- **Schema/Metadata Validation**: Optional field schema to enforce strict column validation and type-aware SQL generation
- **Schema-Aware Truthiness**: The `!!` operator generates type-appropriate SQL based on field schema (boolean → `IS TRUE`, string → `!= ''`, numeric → `!= 0`, array → `CARDINALITY > 0`)
- **Dialect-Specific SQL**: Generates optimized SQL with proper syntax for each supported dialect
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
- **Structured Errors**: Error codes, JSONPath locations, and programmatic error handling via `AsTranspileError()` and `IsErrorCode()`
- **Library & CLI**: Both programmatic API and interactive REPL
- **Type Safety**: Full Go type safety with proper error handling

## Important Notes

> **Semantic Correctness Assumption:** This library assumes that the input JSONLogic is semantically correct. The transpiler generates SQL that directly corresponds to the JSONLogic structure without validating the logical correctness of the expressions. For example, if a JSONLogic expression uses a non-boolean value in a boolean context (e.g., `{"and": [{"var": "name"}]}`), the generated SQL will reflect this structure. It is the caller's responsibility to ensure that JSONLogic expressions are semantically valid for their intended use case.

> **SQL Injection:** This library does NOT handle SQL injection prevention. The caller is responsible for validating input and using parameterized queries where appropriate.

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

## Supported SQL Dialects

All default JSON Logic operators are supported for BigQuery, Spanner, PostgreSQL, DuckDB, and ClickHouse dialects. The library generates appropriate SQL syntax for each dialect.

| Operator Category | Operators | BigQuery | Spanner | PostgreSQL | DuckDB | ClickHouse |
|-------------------|-----------|:--------:|:-------:|:----------:|:------:|:----------:|
| **Data Access** | `var`, `missing`, `missing_some` | ✓ | ✓ | ✓ | ✓ | ✓ |
| **Comparison** | `==`, `===`, `!=`, `!==`, `>`, `>=`, `<`, `<=` | ✓ | ✓ | ✓ | ✓ | ✓ |
| **Logical** | `and`, `or`, `!`, `!!`, `if` | ✓ | ✓ | ✓ | ✓ | ✓ |
| **Numeric** | `+`, `-`, `*`, `/`, `%`, `max`, `min` | ✓ | ✓ | ✓ | ✓ | ✓ |
| **Array** | `in`, `map`, `filter`, `reduce`, `all`, `some`, `none`, `merge` | ✓ | ✓ | ✓ | ✓ | ✓ |
| **String** | `in`, `cat`, `substr` | ✓ | ✓ | ✓ | ✓ | ✓ |

### Dialect-Specific SQL Generation

While all operators are supported for all dialects, some operators generate different SQL based on the target dialect. For example:

| Operator | BigQuery | Spanner | PostgreSQL | DuckDB | ClickHouse |
|----------|----------|---------|------------|--------|------------|
| `merge` (arrays) | `ARRAY_CONCAT(a, b)` | `ARRAY_CONCAT(a, b)` | `(a \|\| b)` | `ARRAY_CONCAT(a, b)` | `arrayConcat(a, b)` |
| `map` (arrays) | `ARRAY(SELECT ... UNNEST)` | `ARRAY(SELECT ... UNNEST)` | `ARRAY(SELECT ... UNNEST)` | `ARRAY(SELECT ... UNNEST)` | `arrayMap(x -> ..., arr)` |
| `filter` (arrays) | `ARRAY(SELECT ... WHERE)` | `ARRAY(SELECT ... WHERE)` | `ARRAY(SELECT ... WHERE)` | `ARRAY(SELECT ... WHERE)` | `arrayFilter(x -> ..., arr)` |
| `substr` | `SUBSTR(s, i, n)` | `SUBSTR(s, i, n)` | `SUBSTR(s, i, n)` | `SUBSTR(s, i, n)` | `substring(s, i, n)` |
| `in` (string) | `STRPOS(h, n) > 0` | `STRPOS(h, n) > 0` | `POSITION(n IN h) > 0` | `STRPOS(h, n) > 0` | `position(h, n) > 0` |
| `safeDivide` (custom) | `SAFE_DIVIDE(a, b)` | `CASE WHEN b = 0 ...` | `CASE WHEN b = 0 ...` | `CASE WHEN b = 0 ...` | `if(b = 0, NULL, a/b)` |
| `arrayLength` (custom) | `ARRAY_LENGTH(arr)` | `ARRAY_LENGTH(arr)` | `CARDINALITY(arr)` | `ARRAY_LENGTH(arr)` | `length(arr)` |
| `regexpContains` (custom) | `REGEXP_CONTAINS(s, r'p')` | `REGEXP_CONTAINS(s, 'p')` | `s ~ 'p'` | `regexp_matches(s, 'p')` | `match(s, 'p')` |

See [Dialect-Aware Custom Operators](#dialect-aware-custom-operators) for details on creating operators with dialect-specific behavior.

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
    // Simple usage with dialect (required)
    sql, err := jsonlogic2sql.Transpile(jsonlogic2sql.DialectBigQuery, `{">": [{"var": "amount"}, 1000]}`)
    if err != nil {
        panic(err)
    }
    fmt.Println(sql) // Output: WHERE amount > 1000

    // Using the transpiler instance
    transpiler, err := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
    if err != nil {
        panic(err)
    }

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
    transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

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
    transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

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
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

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
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

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

#### Dialect-Aware Custom Operators

For operators that generate different SQL based on the target dialect, use the dialect-aware registration methods. This is useful when SQL syntax differs between dialects (BigQuery, Spanner, PostgreSQL, DuckDB, ClickHouse):

```go
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

// safeDivide: Division that returns NULL on division by zero
// This demonstrates a real dialect difference:
// - BigQuery has built-in SAFE_DIVIDE function
// - Spanner requires a CASE expression
transpiler.RegisterDialectAwareOperatorFunc("safeDivide",
    func(op string, args []interface{}, dialect jsonlogic2sql.Dialect) (string, error) {
        if len(args) != 2 {
            return "", fmt.Errorf("safeDivide requires exactly 2 arguments")
        }
        numerator := args[0].(string)
        denominator := args[1].(string)
        switch dialect {
        case jsonlogic2sql.DialectBigQuery:
            // BigQuery has built-in SAFE_DIVIDE that returns NULL on division by zero
            return fmt.Sprintf("SAFE_DIVIDE(%s, %s)", numerator, denominator), nil
        case jsonlogic2sql.DialectSpanner:
            // Spanner doesn't have SAFE_DIVIDE, use CASE expression
            return fmt.Sprintf("CASE WHEN %s = 0 THEN NULL ELSE %s / %s END", denominator, numerator, denominator), nil
        default:
            return "", fmt.Errorf("unsupported dialect: %v", dialect)
        }
    })

sql, _ := transpiler.Transpile(`{"safeDivide": [{"var": "total"}, {"var": "count"}]}`)
// BigQuery: WHERE SAFE_DIVIDE(total, count)
// Spanner:  WHERE CASE WHEN count = 0 THEN NULL ELSE total / count END
```

You can also use a handler struct for dialect-aware operators:

```go
type SafeDivideOperator struct{}

func (s *SafeDivideOperator) ToSQLWithDialect(op string, args []interface{}, dialect jsonlogic2sql.Dialect) (string, error) {
    if len(args) != 2 {
        return "", fmt.Errorf("safeDivide requires exactly 2 arguments")
    }
    numerator := args[0].(string)
    denominator := args[1].(string)
    switch dialect {
    case jsonlogic2sql.DialectBigQuery:
        return fmt.Sprintf("SAFE_DIVIDE(%s, %s)", numerator, denominator), nil
    case jsonlogic2sql.DialectSpanner:
        return fmt.Sprintf("CASE WHEN %s = 0 THEN NULL ELSE %s / %s END", denominator, numerator, denominator), nil
    default:
        return "", fmt.Errorf("unsupported dialect: %v", dialect)
    }
}

transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
transpiler.RegisterDialectAwareOperator("safeDivide", &SafeDivideOperator{})
```

#### Complex Multi-Condition Example

Here's a realistic example combining `safeDivide` with other operators:

**JSON Logic:**
```json
{
  "and": [
    {">": [{"safeDivide": [{"var": "revenue"}, {"var": "cost"}]}, 1.5]},
    {"in": [{"var": "status"}, ["active", "pending"]]},
    {"or": [
      {"startsWith": [{"var": "region"}, "US"]},
      {">=": [{"var": "priority"}, 5]}
    ]},
    {"contains": [{"var": "category"}, "premium"]}
  ]
}
```

**BigQuery Output:**
```sql
WHERE (SAFE_DIVIDE(revenue, cost) > 1.5 AND status IN ('active', 'pending') AND (region LIKE 'US%' OR priority >= 5) AND category LIKE '%premium%')
```

**Spanner Output:**
```sql
WHERE (CASE WHEN cost = 0 THEN NULL ELSE revenue / cost END > 1.5 AND status IN ('active', 'pending') AND (region LIKE 'US%' OR priority >= 5) AND category LIKE '%premium%')
```

**PostgreSQL Output:**
```sql
WHERE (CASE WHEN cost = 0 THEN NULL ELSE revenue / cost END > 1.5 AND status IN ('active', 'pending') AND (region LIKE 'US%' OR priority >= 5) AND category LIKE '%premium%')
```

**DuckDB Output:**
```sql
WHERE (CASE WHEN cost = 0 THEN NULL ELSE revenue / cost END > 1.5 AND status IN ('active', 'pending') AND (region LIKE 'US%' OR priority >= 5) AND category LIKE '%premium%')
```

**ClickHouse Output:**
```sql
WHERE (if(cost = 0, NULL, revenue / cost) > 1.5 AND status IN ('active', 'pending') AND (region LIKE 'US%' OR priority >= 5) AND category LIKE '%premium%')
```

This example filters records where:
- Profit margin (revenue/cost) is greater than 1.5x (using safe division)
- Status is either "active" or "pending"
- Either the region starts with "US" OR priority is 5 or higher
- Category contains "premium"

#### Nested Custom Operators

Custom operators work seamlessly when nested inside any built-in operator. This enables complex transformations combining custom SQL functions with JSONLogic's built-in operators.

```go
transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)

// Register custom operators
transpiler.RegisterOperatorFunc("toLower", func(op string, args []interface{}) (string, error) {
    return fmt.Sprintf("LOWER(%s)", args[0]), nil
})

transpiler.RegisterOperatorFunc("toUpper", func(op string, args []interface{}) (string, error) {
    return fmt.Sprintf("UPPER(%s)", args[0]), nil
})

// Custom operators nested inside cat (string concatenation)
sql, _ := transpiler.Transpile(`{"cat": [{"toLower": [{"var": "firstName"}]}, " ", {"toUpper": [{"var": "lastName"}]}]}`)
// Output: WHERE CONCAT(LOWER(firstName), ' ', UPPER(lastName))

// Custom operators nested inside if (conditional)
sql, _ = transpiler.Transpile(`{"if": [{"==": [{"var": "type"}, "premium"]}, {"toUpper": [{"var": "name"}]}, {"toLower": [{"var": "name"}]}]}`)
// Output: WHERE CASE WHEN type = 'premium' THEN UPPER(name) ELSE LOWER(name) END

// Custom operators nested inside comparison with reduce
sql, _ = transpiler.Transpile(`{"==": [{"toLower": [{"var": "status"}]}, "active"]}`)
// Output: WHERE LOWER(status) = 'active'

// Custom operators inside and/or (logical operators)
sql, _ = transpiler.Transpile(`{"and": [{"==": [{"toLower": [{"var": "status"}]}, "active"]}, {">": [{"var": "amount"}, 100]}]}`)
// Output: WHERE (LOWER(status) = 'active' AND amount > 100)
```

**Deeply Nested Example:**

```json
{
  "and": [
    {"==": [{"toLower": [{"var": "status"}]}, "active"]},
    {">": [
      {"reduce": [
        {"filter": [{"var": "items"}, {">": [{"var": ""}, 0]}]},
        {"+": [{"var": "accumulator"}, {"var": "current"}]},
        0
      ]},
      1000
    ]},
    {"!=": [{"substr": [{"toUpper": [{"var": "region"}]}, 0, 2]}, "XX"]}
  ]
}
```

This demonstrates:
- `toLower` nested inside `and` → `==`
- `filter` nested inside `reduce` nested inside `>` nested inside `and`
- `toUpper` nested inside `substr` nested inside `!=` nested inside `and`

All custom operators are dialect-aware, so they generate the correct SQL for each target database when using `RegisterDialectAwareOperatorFunc`.

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

#### REPL Commands

| Command | Description |
|---------|-------------|
| `:help` | Show available commands |
| `:examples` | Show example JSON Logic expressions |
| `:dialect` | Change the SQL dialect |
| `:file <path>` | Read JSON Logic from a file (for large inputs) |
| `:clear` | Clear the screen |
| `:quit` | Exit the REPL |

#### Large JSON Input Support

For JSON Logic expressions larger than ~4KB (terminal line input limit), use the `:file` command to load from a file:

```bash
# Save your large JSON to a file
echo '{"and": [...very large JSON...]}' > input.json

# In the REPL, load it with :file
[BigQuery] jsonlogic> :file input.json
SQL: WHERE (...)
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
-- Without schema (generic truthiness check):
WHERE (value IS NOT NULL AND value != FALSE AND value != 0 AND value != '')

-- With schema (type-appropriate SQL - see Schema-Aware Truthiness below)
```

#### Schema-Aware Truthiness

When a schema is provided, the `!!` operator generates type-appropriate SQL to avoid type mismatch errors in strongly-typed databases:

| Field Type | JSONLogic | Generated SQL |
|------------|-----------|---------------|
| Boolean | `{"!!": {"var": "is_verified"}}` | `is_verified IS TRUE` |
| String | `{"!!": {"var": "name"}}` | `(name IS NOT NULL AND name != '')` |
| Integer/Number | `{"!!": {"var": "amount"}}` | `(amount IS NOT NULL AND amount != 0)` |
| Array (BigQuery/Spanner/PostgreSQL/DuckDB) | `{"!!": {"var": "tags"}}` | `(tags IS NOT NULL AND CARDINALITY(tags) > 0)` |
| Array (ClickHouse) | `{"!!": {"var": "tags"}}` | `(tags IS NOT NULL AND length(tags) > 0)` |

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
WHERE ARRAY(SELECT (elem + 1) FROM UNNEST(numbers) AS elem)
```

#### Filter Array
```json
{"filter": [{"var": "scores"}, {">": [{"var": "item"}, 70]}]}
```
```sql
WHERE ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem > 70)
```

#### Reduce Array (SUM pattern)
```json
{"reduce": [{"var": "numbers"}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}
```
```sql
WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST(numbers) AS elem), 0)
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
├── Makefile                  # Build automation
└── README.md
```

### Testing

The project includes comprehensive tests:

- **Unit Tests**: Each operator and component is thoroughly tested (3,000+ test cases passing)
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
- **Nested Custom Operators**: Custom operators work inside all built-in operators

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

#### `Transpile(dialect Dialect, jsonLogic string) (string, error)`
Converts a JSON Logic string to a SQL WHERE clause using the specified dialect.

#### `TranspileFromMap(dialect Dialect, logic map[string]interface{}) (string, error)`
Converts a pre-parsed JSON Logic map to a SQL WHERE clause using the specified dialect.

#### `TranspileFromInterface(dialect Dialect, logic interface{}) (string, error)`
Converts any JSON Logic interface{} to a SQL WHERE clause using the specified dialect.

#### `TranspileCondition(dialect Dialect, jsonLogic string) (string, error)`
Converts a JSON Logic string to a SQL condition **without** the WHERE keyword. Useful when embedding conditions in larger queries.

#### `TranspileConditionFromMap(dialect Dialect, logic map[string]interface{}) (string, error)`
Converts a pre-parsed JSON Logic map to a SQL condition without the WHERE keyword.

#### `TranspileConditionFromInterface(dialect Dialect, logic interface{}) (string, error)`
Converts any JSON Logic interface{} to a SQL condition without the WHERE keyword.

#### `NewTranspiler(dialect Dialect) (*Transpiler, error)`
Creates a new transpiler instance with the specified dialect. Dialect is required - use `DialectBigQuery`, `DialectSpanner`, `DialectPostgreSQL`, `DialectDuckDB`, or `DialectClickHouse`.

#### `NewTranspilerWithConfig(config *TranspilerConfig) (*Transpiler, error)`
Creates a new transpiler instance with custom configuration. `Config.Dialect` is required.

#### `NewOperatorRegistry() *OperatorRegistry`
Creates a new empty operator registry for managing custom operators.

### Types

#### `Transpiler`
Main transpiler instance with methods:
- `Transpile(jsonLogic string) (string, error)` - Convert JSON string to SQL with WHERE
- `TranspileFromMap(logic map[string]interface{}) (string, error)` - Convert map to SQL with WHERE
- `TranspileFromInterface(logic interface{}) (string, error)` - Convert interface to SQL with WHERE
- `TranspileCondition(jsonLogic string) (string, error)` - Convert JSON string to SQL without WHERE
- `TranspileConditionFromMap(logic map[string]interface{}) (string, error)` - Convert map to SQL without WHERE
- `TranspileConditionFromInterface(logic interface{}) (string, error)` - Convert interface to SQL without WHERE
- `GetDialect() Dialect` - Get the configured dialect
- `SetSchema(schema *Schema)` - Set schema for field validation
- `RegisterOperator(name string, handler OperatorHandler) error` - Register custom operator with handler
- `RegisterOperatorFunc(name string, fn OperatorFunc) error` - Register custom operator with function
- `RegisterDialectAwareOperator(name string, handler DialectAwareOperatorHandler) error` - Register dialect-aware operator
- `RegisterDialectAwareOperatorFunc(name string, fn DialectAwareOperatorFunc) error` - Register dialect-aware function
- `UnregisterOperator(name string) bool` - Remove a custom operator
- `HasCustomOperator(name string) bool` - Check if operator is registered
- `ListCustomOperators() []string` - List all custom operator names
- `ClearCustomOperators()` - Remove all custom operators

#### `TranspilerConfig`
Configuration options for the transpiler:
- `Dialect Dialect` - Required: target SQL dialect (`DialectBigQuery`, `DialectSpanner`, `DialectPostgreSQL`, `DialectDuckDB`, or `DialectClickHouse`)
- `Schema *Schema` - Optional schema for field validation (can also be set via `SetSchema()`)

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

#### `DialectAwareOperatorFunc`
Function type for dialect-aware custom operator implementations:
```go
type DialectAwareOperatorFunc func(operator string, args []interface{}, dialect Dialect) (string, error)
```

#### `DialectAwareOperatorHandler`
Interface for dialect-aware custom operator implementations:
```go
type DialectAwareOperatorHandler interface {
    ToSQLWithDialect(operator string, args []interface{}, dialect Dialect) (string, error)
}
```

#### `Dialect`
SQL dialect type with constants:
- `DialectBigQuery` - Google BigQuery SQL dialect
- `DialectSpanner` - Google Cloud Spanner SQL dialect
- `DialectPostgreSQL` - PostgreSQL SQL dialect
- `DialectDuckDB` - DuckDB SQL dialect
- `DialectClickHouse` - ClickHouse SQL dialect

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

#### `TranspileError`
Structured error type returned by transpilation operations:
```go
type TranspileError struct {
    Code     ErrorCode   // Error code (e.g., ErrUnsupportedOperator)
    Operator string      // The operator that caused the error
    Path     string      // JSONPath to the error location
    Message  string      // Human-readable error message
    Cause    error       // Underlying error (if any)
}
```

Methods:
- `Error() string` - Returns formatted error message with code and path
- `Unwrap() error` - Returns the underlying cause for errors.Unwrap support

#### `ErrorCode`
Error code type with categories:
- **E001-E099**: Structural/validation errors
- **E100-E199**: Operator-specific errors
- **E200-E299**: Type/schema errors
- **E300-E399**: Argument errors

#### Helper Functions
- `AsTranspileError(err error) (*TranspileError, bool)` - Extract TranspileError from error chain
- `IsErrorCode(err error, code ErrorCode) bool` - Check if error has specific code

## Error Handling

The library provides structured errors with error codes for programmatic error handling. All errors are wrapped in a `TranspileError` type that includes:

- **Code**: A unique error code (e.g., `E100`, `E302`)
- **Operator**: The operator that caused the error
- **Path**: JSONPath to the error location (e.g., `$.and[0].>`)
- **Message**: Human-readable description
- **Cause**: The underlying error (if any)

### Error Codes

| Category | Code Range | Examples |
|----------|------------|----------|
| Structural/Validation | E001-E099 | `ErrInvalidExpression`, `ErrValidation`, `ErrInvalidJSON` |
| Operator-specific | E100-E199 | `ErrUnsupportedOperator`, `ErrOperatorRequiresArray`, `ErrCustomOperatorFailed` |
| Type/Schema | E200-E299 | `ErrTypeMismatch`, `ErrFieldNotInSchema`, `ErrInvalidEnumValue` |
| Argument | E300-E399 | `ErrInsufficientArgs`, `ErrTooManyArgs`, `ErrInvalidArgument` |

### Programmatic Error Handling

```go
sql, err := transpiler.Transpile(jsonLogic)
if err != nil {
    // Method 1: Use helper function
    if tpErr, ok := jsonlogic2sql.AsTranspileError(err); ok {
        fmt.Printf("Error code: %s\n", tpErr.Code)    // e.g., "E100"
        fmt.Printf("Path: %s\n", tpErr.Path)          // e.g., "$.and[0].unknown"
        fmt.Printf("Operator: %s\n", tpErr.Operator)  // e.g., "unknown"
        fmt.Printf("Message: %s\n", tpErr.Message)    // Human-readable message
    }

    // Method 2: Check specific error code
    if jsonlogic2sql.IsErrorCode(err, jsonlogic2sql.ErrUnsupportedOperator) {
        // Handle unsupported operator specifically
    }

    // Method 3: Use standard errors.As
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

### Example Error Output

```
Error: [E100] at $.and.unknown[0] (operator: unknown): unsupported operator: unknown
Error: [E007]: invalid JSON: invalid character 'i' looking for beginning of object key string
Error: [E302] at $.var (operator: var): operator error: field 'bad.field' is not defined in schema
```

### Available Error Codes

```go
// Structural/validation errors
jsonlogic2sql.ErrInvalidExpression   // E001
jsonlogic2sql.ErrEmptyArray          // E002
jsonlogic2sql.ErrMultipleKeys        // E003
jsonlogic2sql.ErrPrimitiveNotAllowed // E004
jsonlogic2sql.ErrArrayNotAllowed     // E005
jsonlogic2sql.ErrValidation          // E006
jsonlogic2sql.ErrInvalidJSON         // E007

// Operator-specific errors
jsonlogic2sql.ErrUnsupportedOperator   // E100
jsonlogic2sql.ErrOperatorRequiresArray // E101
jsonlogic2sql.ErrCustomOperatorFailed  // E102

// Type/schema errors
jsonlogic2sql.ErrTypeMismatch     // E200
jsonlogic2sql.ErrFieldNotInSchema // E201
jsonlogic2sql.ErrInvalidFieldType // E202
jsonlogic2sql.ErrInvalidEnumValue // E203

// Argument errors
jsonlogic2sql.ErrInsufficientArgs    // E300
jsonlogic2sql.ErrTooManyArgs         // E301
jsonlogic2sql.ErrInvalidArgument     // E302
jsonlogic2sql.ErrInvalidArgType      // E303
jsonlogic2sql.ErrInvalidDefaultValue // E304
```

## License

This project is licensed under the MIT License - see the [LICENSE](./LICENSE) file for details.