# Supported Operators

This document lists all JSON Logic operators supported by jsonlogic2sql.

## Data Access

| Operator | Description | Example |
|----------|-------------|---------|
| `var` | Access variable values (including array indexing) | `{"var": "name"}` |
| `missing` | Check if variable(s) are missing | `{"missing": "email"}` |
| `missing_some` | Check if some variables are missing | `{"missing_some": [1, ["a", "b"]]}` |

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

| Operator | Description |
|----------|-------------|
| `if` | Conditional expressions |
| `==`, `===` | Equality comparison |
| `!=`, `!==` | Inequality comparison |
| `!` | Logical NOT |
| `!!` | Double negation (boolean conversion) |
| `or` | Logical OR |
| `and` | Logical AND |

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

### Equality with NULL

```json
{"==": [{"var": "deleted_at"}, null]}
```
```sql
WHERE deleted_at IS NULL
```

### Inequality with NULL

```json
{"!=": [{"var": "field"}, null]}
```
```sql
WHERE field IS NOT NULL
```

### Logical NOT

```json
{"!": [{"var": "isDeleted"}]}
```
```sql
WHERE NOT (isDeleted)
```

### Double Negation (Boolean Conversion)

```json
{"!!": [{"var": "value"}]}
```

Without schema (generic truthiness check):
```sql
WHERE (value IS NOT NULL AND value != FALSE AND value != 0 AND value != '')
```

With schema, the `!!` operator generates type-appropriate SQL. See [Schema-Aware Truthiness](schema-validation.md#schema-aware-truthiness) for details.

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

### Conditional Expression (if)

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

| Operator | Description |
|----------|-------------|
| `>`, `>=`, `<`, `<=` | Comparison operators |
| `max`, `min` | Maximum/minimum values |
| `+`, `-`, `*`, `/`, `%` | Arithmetic operations |

### Comparison Operators

```json
{">": [{"var": "amount"}, 1000]}
{">=": [{"var": "score"}, 80]}
{"<": [{"var": "age"}, 65]}
{"<=": [{"var": "count"}, 10]}
```
```sql
WHERE amount > 1000
WHERE score >= 80
WHERE age < 65
WHERE count <= 10
```

### Maximum/Minimum

```json
{"max": [{"var": "score1"}, {"var": "score2"}, {"var": "score3"}]}
{"min": [{"var": "price1"}, {"var": "price2"}]}
```
```sql
WHERE GREATEST(score1, score2, score3)
WHERE LEAST(price1, price2)
```

### Arithmetic Operations

```json
{"+": [{"var": "price"}, {"var": "tax"}]}
{"-": [{"var": "total"}, {"var": "discount"}]}
{"*": [{"var": "price"}, 1.2]}
{"/": [{"var": "total"}, 2]}
{"%": [{"var": "count"}, 3]}
```
```sql
WHERE (price + tax)
WHERE (total - discount)
WHERE (price * 1.2)
WHERE (total / 2)
WHERE (count % 3)
```

### Unary Operations

```json
{"-": [{"var": "value"}]}
{"+": ["-5"]}
```
```sql
WHERE -value
WHERE CAST(-5 AS NUMERIC)
```

## Array Operations

| Operator | Description |
|----------|-------------|
| `in` | Check if value is in array |
| `map`, `filter`, `reduce` | Array transformations |
| `all`, `some`, `none` | Array condition checks |
| `merge` | Merge arrays |

### In Array

```json
{"in": [{"var": "country"}, ["US", "CA", "MX"]]}
```
```sql
WHERE country IN ('US', 'CA', 'MX')
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

### Reduce Array

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

| Operator | Description |
|----------|-------------|
| `in` | Check if substring is in string |
| `cat` | Concatenate strings |
| `substr` | Substring operations |

### String Containment

```json
{"in": ["hello", "hello world"]}
```
```sql
WHERE POSITION('hello' IN 'hello world') > 0
```

### Concatenate Strings

```json
{"cat": [{"var": "firstName"}, " ", {"var": "lastName"}]}
```
```sql
WHERE CONCAT(firstName, ' ', lastName)
```

### Concatenate with Conditional

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

## See Also

- [SQL Dialects](dialects.md) - Dialect-specific operator behavior
- [Custom Operators](custom-operators.md) - Add your own operators
- [Examples](examples.md) - More complex examples
