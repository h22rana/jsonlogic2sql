# Examples

This document provides comprehensive examples of JSON Logic expressions and their SQL output.

## Data Access Operations

### Variable Access

```json
{"var": "name"}
```
```sql
WHERE name
```

### Variable with Array Index

```json
{"var": 1}
```
```sql
WHERE data[1]
```

### Variable with Default Value

```json
{"var": ["status", "pending"]}
```
```sql
WHERE COALESCE(status, 'pending')
```

### Missing Field Check (Single)

```json
{"missing": "email"}
```
```sql
WHERE email IS NULL
```

### Missing Field Check (Multiple)

```json
{"missing": ["email", "phone"]}
```
```sql
WHERE (email IS NULL OR phone IS NULL)
```

### Missing Some Fields

```json
{"missing_some": [1, ["field1", "field2"]]}
```
```sql
WHERE (field1 IS NULL OR field2 IS NULL)
```

## Logic and Boolean Operations

### Simple Comparison

```json
{">": [{"var": "amount"}, 1000]}
```
```sql
WHERE amount > 1000
```

### Equality Comparison

```json
{"==": [{"var": "status"}, "active"]}
```
```sql
WHERE status = 'active'
```

### Strict Equality

```json
{"===": [{"var": "count"}, 5]}
```
```sql
WHERE count = 5
```

### Inequality

```json
{"!=": [{"var": "status"}, "inactive"]}
```
```sql
WHERE status != 'inactive'
```

### Strict Inequality

```json
{"!==": [{"var": "count"}, 0]}
```
```sql
WHERE count <> 0
```

### Equality with NULL (IS NULL)

```json
{"==": [{"var": "deleted_at"}, null]}
```
```sql
WHERE deleted_at IS NULL
```

### Inequality with NULL (IS NOT NULL)

```json
{"!=": [{"var": "field"}, null]}
```
```sql
WHERE field IS NOT NULL
```

### Logical NOT (with array wrapper)

```json
{"!": [{"var": "isDeleted"}]}
```
```sql
WHERE NOT (isDeleted)
```

### Logical NOT (without array wrapper)

```json
{"!": {"var": "isDeleted"}}
```
```sql
WHERE NOT (isDeleted)
```

### Logical NOT (literal)

```json
{"!": true}
```
```sql
WHERE NOT (TRUE)
```

### Double Negation (Boolean Conversion)

```json
{"!!": [{"var": "value"}]}
```
```sql
-- Without schema (generic truthiness check):
WHERE (value IS NOT NULL AND value != FALSE AND value != 0 AND value != '')

-- With schema (type-appropriate SQL)
```

### Double Negation (Empty Array)

```json
{"!!": [[]]}
```
```sql
WHERE FALSE
```

### Double Negation (Non-Empty Array)

```json
{"!!": [[1, 2, 3]]}
```
```sql
WHERE TRUE
```

### Logical AND

```json
{"and": [
  {">": [{"var": "amount"}, 5000]},
  {"==": [{"var": "status"}, "pending"]}
]}
```
```sql
WHERE (amount > 5000 AND status = 'pending')
```

### Logical OR

```json
{"or": [
  {">=": [{"var": "failedAttempts"}, 5]},
  {"in": [{"var": "country"}, ["CN", "RU"]]}
]}
```
```sql
WHERE (failedAttempts >= 5 OR country IN ('CN', 'RU'))
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
WHERE CASE WHEN age > 18 THEN 'adult' ELSE 'minor' END
```

## Numeric Operations

### Greater Than

```json
{">": [{"var": "amount"}, 1000]}
```
```sql
WHERE amount > 1000
```

### Greater Than or Equal

```json
{">=": [{"var": "score"}, 80]}
```
```sql
WHERE score >= 80
```

### Less Than

```json
{"<": [{"var": "age"}, 65]}
```
```sql
WHERE age < 65
```

### Less Than or Equal

```json
{"<=": [{"var": "count"}, 10]}
```
```sql
WHERE count <= 10
```

### Maximum Value

```json
{"max": [{"var": "score1"}, {"var": "score2"}, {"var": "score3"}]}
```
```sql
WHERE GREATEST(score1, score2, score3)
```

### Minimum Value

```json
{"min": [{"var": "price1"}, {"var": "price2"}]}
```
```sql
WHERE LEAST(price1, price2)
```

### Addition

```json
{"+": [{"var": "price"}, {"var": "tax"}]}
```
```sql
WHERE (price + tax)
```

### Subtraction

```json
{"-": [{"var": "total"}, {"var": "discount"}]}
```
```sql
WHERE (total - discount)
```

### Multiplication

```json
{"*": [{"var": "price"}, 1.2]}
```
```sql
WHERE (price * 1.2)
```

### Division

```json
{"/": [{"var": "total"}, 2]}
```
```sql
WHERE (total / 2)
```

### Modulo

```json
{"%": [{"var": "count"}, 3]}
```
```sql
WHERE (count % 3)
```

### Unary Minus (Negation)

```json
{"-": [{"var": "value"}]}
```
```sql
WHERE -value
```

### Unary Plus (Cast to Number)

```json
{"+": ["-5"]}
```
```sql
WHERE CAST(-5 AS NUMERIC)
```

## Array Operations

### In Array

```json
{"in": [{"var": "country"}, ["US", "CA", "MX"]]}
```
```sql
WHERE country IN ('US', 'CA', 'MX')
```

### String Containment

```json
{"in": ["hello", "hello world"]}
```
```sql
WHERE POSITION('hello' IN 'hello world') > 0
```

### Map Array

```json
{"map": [{"var": "numbers"}, {"+": [{"var": "item"}, 1]}]}
```
```sql
WHERE ARRAY(SELECT (elem + 1) FROM UNNEST(numbers) AS elem)
```

### Filter Array

```json
{"filter": [{"var": "scores"}, {">": [{"var": "item"}, 70]}]}
```
```sql
WHERE ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem > 70)
```

### Reduce Array (SUM pattern)

```json
{"reduce": [{"var": "numbers"}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}
```
```sql
WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST(numbers) AS elem), 0)
```

### All Elements Satisfy Condition

```json
{"all": [{"var": "ages"}, {">=": [{"var": ""}, 18]}]}
```
```sql
WHERE NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18))
```

### Some Elements Satisfy Condition

```json
{"some": [{"var": "statuses"}, {"==": [{"var": ""}, "active"]}]}
```
```sql
WHERE EXISTS (SELECT 1 FROM UNNEST(statuses) AS elem WHERE elem = 'active')
```

### No Elements Satisfy Condition

```json
{"none": [{"var": "values"}, {"==": [{"var": ""}, "invalid"]}]}
```
```sql
WHERE NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE elem = 'invalid')
```

### Merge Arrays

```json
{"merge": [{"var": "array1"}, {"var": "array2"}]}
```
```sql
WHERE ARRAY_CONCAT(array1, array2)
```

## String Operations

### Concatenate Strings

```json
{"cat": [{"var": "firstName"}, " ", {"var": "lastName"}]}
```
```sql
WHERE CONCAT(firstName, ' ', lastName)
```

### Concatenate with Conditional (Nested If)

```json
{"cat": [{"if": [{"==": [{"var": "gender"}, "M"]}, "Mr. ", "Ms. "]}, {"var": "first_name"}, " ", {"var": "last_name"}]}
```
```sql
WHERE CONCAT(CASE WHEN (gender = 'M') THEN 'Mr. ' ELSE 'Ms. ' END, first_name, ' ', last_name)
```

### Substring with Length

```json
{"substr": [{"var": "email"}, 0, 10]}
```
```sql
WHERE SUBSTR(email, 1, 10)
```

### Substring without Length

```json
{"substr": [{"var": "email"}, 4]}
```
```sql
WHERE SUBSTR(email, 5)
```

## Complex Nested Examples

### Complex Nested Math Expressions

```json
{">": [{"+": [{"var": "base"}, {"*": [{"var": "bonus"}, 0.1]}]}, 1000]}
```
```sql
WHERE (base + (bonus * 0.1)) > 1000
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
WHERE (transaction.amount > 10000 AND (user.verified = FALSE OR user.accountAgeDays < 7))
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
WHERE CASE WHEN (age >= 18 AND country = 'US') THEN 'eligible' ELSE 'ineligible' END
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
WHERE (transaction.amount > 10000 AND (user.verified = FALSE OR user.accountAgeDays < 7 OR user.failedAttempts >= 3) AND user.country IN ('high_risk_1', 'high_risk_2'))
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
WHERE CASE WHEN (age >= 18 AND hasLicense = TRUE AND violations < 3) THEN 'approved' ELSE CASE WHEN age < 18 THEN 'too_young' ELSE 'rejected' END END
```

## See Also

- [Operators](operators.md) - Full operator reference
- [Custom Operators](custom-operators.md) - Creating custom operators
- [Schema Validation](schema-validation.md) - Field validation
