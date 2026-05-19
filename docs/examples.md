# Examples

This document provides comprehensive examples of JSON Logic expressions and their SQL output. Predicate examples are suitable for `TranspileCondition`; scalar and array-producing examples use `TranspileValue`.

## Data Access Operations

### Variable Access

```json
{"var": "name"}
```
```sql
name
```

> **Note:** JSONLogic's numeric `var` form, such as `{"var": 1}`, is not
> supported in SQL output. SQL row values are column-name based, so `var`
> operands must be string field names or `[fieldName, defaultValue]` arrays.

### Variable with Default Value

```json
{"var": ["status", "pending"]}
```
```sql
COALESCE(status, 'pending')
```

With a schema, equality and inequality comparisons against `[field, default]`
vars keep the `COALESCE` expression and still coerce the comparison literal
where supported.

### Missing Field Check (Single)

```json
{"missing": "email"}
```
```sql
email IS NULL
```

### Missing Field Check (Multiple)

```json
{"missing": ["email", "phone"]}
```
```sql
(email IS NULL OR phone IS NULL)
```

### Missing Some Fields

```json
{"missing_some": [1, ["field1", "field2"]]}
```
```sql
(field1 IS NULL OR field2 IS NULL)
```

## Logic and Boolean Operations

### Simple Comparison

```json
{">": [{"var": "amount"}, 1000]}
```
```sql
amount > 1000
```

### Equality Comparison

```json
{"==": [{"var": "status"}, "active"]}
```
```sql
status = 'active'
```

### Strict Equality

```json
{"===": [{"var": "count"}, 5]}
```
```sql
count = 5
```

### Inequality

```json
{"!=": [{"var": "status"}, "inactive"]}
```
```sql
status != 'inactive'
```

### Strict Inequality

```json
{"!==": [{"var": "count"}, 0]}
```
```sql
count <> 0
```

### Equality with NULL (IS NULL)

```json
{"==": [{"var": "deleted_at"}, null]}
```
```sql
deleted_at IS NULL
```

### Inequality with NULL (IS NOT NULL)

```json
{"!=": [{"var": "field"}, null]}
```
```sql
field IS NOT NULL
```

### Logical NOT (with array wrapper)

```json
{"!": [{"var": "isDeleted"}]}
```
```sql
-- With schema: isDeleted is boolean
NOT (isDeleted IS TRUE)
```

### Logical NOT (without array wrapper)

```json
{"!": {"var": "isDeleted"}}
```
```sql
-- With schema: isDeleted is boolean
NOT (isDeleted IS TRUE)
```

### Logical NOT (literal)

```json
{"!": true}
```
```sql
NOT (TRUE)
```

### Double Negation (Boolean Conversion)

```json
{"!!": [{"var": "value"}]}
```
```sql
-- Without schema:
-- error: field truthiness requires schema/type information

-- With schema (type-appropriate SQL)
```

### Double Negation (Empty Array)

```json
{"!!": [[]]}
```
```sql
FALSE
```

### Double Negation (Non-Empty Array)

```json
{"!!": [[1, 2, 3]]}
```
```sql
TRUE
```

### Logical AND

```json
{"and": [
  {">": [{"var": "amount"}, 5000]},
  {"==": [{"var": "status"}, "pending"]}
]}
```
```sql
(amount > 5000 AND status = 'pending')
```

### Logical OR

```json
{"or": [
  {">=": [{"var": "failedAttempts"}, 5]},
  {"in": [{"var": "country"}, ["CN", "RU"]]}
]}
```
```sql
(failedAttempts >= 5 OR country IN ('CN', 'RU'))
```

### Conditional Expression

```json
{"if": [
  {">": [{"var": "age"}, 18]},
  "adult",
  "minor"
]}
```
```sql
CASE WHEN age > 18 THEN 'adult' ELSE 'minor' END
```

## Numeric Operations

### Greater Than

```json
{">": [{"var": "amount"}, 1000]}
```
```sql
amount > 1000
```

### Greater Than or Equal

```json
{">=": [{"var": "score"}, 80]}
```
```sql
score >= 80
```

### Less Than

```json
{"<": [{"var": "age"}, 65]}
```
```sql
age < 65
```

### Less Than or Equal

```json
{"<=": [{"var": "count"}, 10]}
```
```sql
count <= 10
```

### Maximum Value

```json
{"max": [{"var": "score1"}, {"var": "score2"}, {"var": "score3"}]}
```
```sql
GREATEST(score1, score2, score3)
```

### Minimum Value

```json
{"min": [{"var": "price1"}, {"var": "price2"}]}
```
```sql
LEAST(price1, price2)
```

### Addition

```json
{"+": [{"var": "price"}, {"var": "tax"}]}
```
```sql
(price + tax)
```

### Subtraction

```json
{"-": [{"var": "total"}, {"var": "discount"}]}
```
```sql
(total - discount)
```

### Multiplication

```json
{"*": [{"var": "price"}, 1.2]}
```
```sql
(price * 1.2)
```

### Division

```json
{"/": [{"var": "total"}, 2]}
```
```sql
(total / 2)
```

### Modulo

```json
{"%": [{"var": "count"}, 3]}
```
```sql
(count % 3)
```

### Unary Minus (Negation)

```json
{"-": [{"var": "value"}]}
```
```sql
-value
```

### Unary Plus (Cast to Number)

```json
{"+": ["-5"]}
```
```sql
CAST(-5 AS NUMERIC)
```

### String Operands (Numeric Coercion)

String literals in arithmetic are coerced to numbers when valid, or safely quoted otherwise:

```json
{"+": ["42", 1]}
{"*": [" 3 ", 2]}
{"+": ["3.14", 1]}
{"+": ["hello", 1]}
```
```sql
(42 + 1)
(3 * 2)
(3.14 + 1)
('hello' + 1)
```

Large integers are preserved without precision loss:

```json
{"+": ["9223372036854775808", 1]}
```
```sql
(9223372036854775808 + 1)
```

## Array Operations

### In Array

```json
{"in": [{"var": "country"}, ["US", "CA", "MX"]]}
```
```sql
country IN ('US', 'CA', 'MX')
```

### In Array with Type Coercion (Schema)

When a schema is provided, array elements are coerced to match the field type:

```json
// Schema: sector_code is string type, amount is integer type
{"in": [{"var": "sector_code"}, [5960, 9000]]}
{"in": [{"var": "amount"}, ["100", "200", "300"]]}
```
```sql
sector_code IN ('5960', '9000')
amount IN (100, 200, 300)
```

### String Containment

```json
{"in": ["hello", "hello world"]}
```
```sql
POSITION('hello' IN 'hello world') > 0
```

### Map Array

```json
{"map": [{"var": "numbers"}, {"+": [{"var": ""}, 1]}]}
```
```sql
ARRAY(SELECT (elem + 1) FROM UNNEST(numbers) AS elem)
```

### Filter Array

```json
{"filter": [{"var": "scores"}, {">": [{"var": ""}, 70]}]}
```
```sql
ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem > 70)
```

### Reduce Array (SUM pattern)

```json
{"reduce": [{"var": "numbers"}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}
```
```sql
0 + COALESCE((SELECT SUM(elem) FROM UNNEST(numbers) AS elem), 0)
```

### All Elements Satisfy Condition

```json
{"all": [{"var": "ages"}, {">=": [{"var": ""}, 18]}]}
```
```sql
NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18))
```

### Some Elements Satisfy Condition

```json
{"some": [{"var": "statuses"}, {"==": [{"var": ""}, "active"]}]}
```
```sql
EXISTS (SELECT 1 FROM UNNEST(statuses) AS elem WHERE elem = 'active')
```

### No Elements Satisfy Condition

```json
{"none": [{"var": "values"}, {"==": [{"var": ""}, "invalid"]}]}
```
```sql
NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE elem = 'invalid')
```

### Merge Arrays

```json
{"merge": [{"var": "array1"}, {"var": "array2"}]}
```
```sql
ARRAY_CONCAT(array1, array2)
```

## String Operations

### Concatenate Strings

```json
{"cat": [{"var": "firstName"}, " ", {"var": "lastName"}]}
```
```sql
CONCAT(COALESCE(CAST(firstName AS STRING), ''), ' ', COALESCE(CAST(lastName AS STRING), ''))
```

### Concatenate with Conditional (Nested If)

```json
{"cat": [{"if": [{"==": [{"var": "gender"}, "M"]}, "Mr. ", "Ms. "]}, {"var": "first_name"}, " ", {"var": "last_name"}]}
```
```sql
CONCAT(COALESCE(CASE WHEN gender = 'M' THEN 'Mr. ' ELSE 'Ms. ' END, ''), COALESCE(CAST(first_name AS STRING), ''), ' ', COALESCE(CAST(last_name AS STRING), ''))
```

### Substring with Length

```json
{"substr": [{"var": "email"}, 0, 10]}
```
```sql
SUBSTR(email, 1, 10)
```

### Substring without Length

```json
{"substr": [{"var": "email"}, 4]}
```
```sql
SUBSTR(email, 5)
```

## Complex Nested Examples

### Complex Nested Math Expressions

```json
{">": [{"+": [{"var": "base"}, {"*": [{"var": "bonus"}, 0.1]}]}, 1000]}
```
```sql
(base + (bonus * 0.1)) > 1000
```

### Nested Conditions

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
(transaction.amount > 10000 AND (user.verified = FALSE OR user.accountAgeDays < 7))
```

### Complex Conditional Logic

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
CASE WHEN (age >= 18 AND country = 'US') THEN 'eligible' ELSE 'ineligible' END
```

### Fraud Detection Example

```json
{"and": [
  {">": [{"var": "transaction.amount"}, 10000]},
  {"or": [
    {"==": [{"var": "user.verified"}, false]},
    {"<": [{"var": "user.accountAgeDays"}, 7]},
    {">=": [{"var": "user.failedAttempts"}, 3]}
  ]},
  {"in": [{"var": "user.country"}, ["high_risk_1", "high_risk_2"]]}
]}
```
```sql
(transaction.amount > 10000 AND (user.verified = FALSE OR user.accountAgeDays < 7 OR user.failedAttempts >= 3) AND user.country IN ('high_risk_1', 'high_risk_2'))
```

### Eligibility Check Example

```json
{"if": [
  {"and": [
    {">=": [{"var": "age"}, 18]},
    {"==": [{"var": "hasLicense"}, true]},
    {"<": [{"var": "violations"}, 3]}
  ]},
  "approved",
  {"if": [
    {"<": [{"var": "age"}, 18]},
    "too_young",
    "rejected"
  ]}
]}
```
```sql
CASE WHEN (age >= 18 AND hasLicense = TRUE AND violations < 3) THEN 'approved' ELSE CASE WHEN age < 18 THEN 'too_young' ELSE 'rejected' END END
```

## Parameterized Query Examples

All predicate examples above can be generated with bind parameter placeholders instead of inlined literals by using the `TranspileParameterizedCondition` family of methods.

### Simple Parameterized Comparison

```go
sql, params, _ := jsonlogic2sql.TranspileParameterizedCondition(
    jsonlogic2sql.DialectBigQuery,
    `{"==": [{"var": "status"}, "active"]}`,
)
// sql    = "status = @p1"
// params = [{Name: "p1", Value: "active"}]
```

### Parameterized IN List

```go
sql, params, _ := jsonlogic2sql.TranspileParameterizedCondition(
    jsonlogic2sql.DialectBigQuery,
    `{"in": [{"var": "country"}, ["US", "CA", "MX"]]}`,
)
// sql    = "country IN (@p1, @p2, @p3)"
// params = [{Name: "p1", Value: "US"}, {Name: "p2", Value: "CA"}, {Name: "p3", Value: "MX"}]
```

### Parameterized Nested Conditions (PostgreSQL)

```go
sql, params, _ := jsonlogic2sql.TranspileParameterizedCondition(
    jsonlogic2sql.DialectPostgreSQL,
    `{"and": [{">": [{"var": "amount"}, 10000]}, {"==": [{"var": "status"}, "pending"]}]}`,
)
// sql    = "(amount > $1 AND status = $2)"
// params = [{Name: "p1", Value: 10000}, {Name: "p2", Value: "pending"}]
```

### Parameterized Condition (Without WHERE)

```go
condition, params, _ := jsonlogic2sql.TranspileParameterizedCondition(
    jsonlogic2sql.DialectBigQuery,
    `{">": [{"var": "amount"}, 1000]}`,
)
// condition = "amount > @p1"
// params    = [{Name: "p1", Value: 1000}]

query := fmt.Sprintf("SELECT * FROM orders WHERE %s AND created_at > @date", condition)
```

See [Parameterized Queries](parameterized-queries.md) for full documentation.

## See Also

- [Operators](operators.md) - Full operator reference
- [Custom Operators](custom-operators.md) - Creating custom operators
- [Schema Validation](schema-validation.md) - Field validation
- [Parameterized Queries](parameterized-queries.md) - Bind-parameter output for safe SQL execution
