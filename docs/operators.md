# Supported Operators

This document lists all JSON Logic operators supported by jsonlogic2sql.
Examples show the SQL expression returned by the transpiler. Predicate examples
use `TranspileCondition`; scalar and array-producing examples use
`TranspileValue`.

## Data Access

| Operator | Description | Example |
|----------|-------------|---------|
| `var` | Access variable values by field name | `{"var": "name"}` |
| `missing` | Check if variable(s) are missing | `{"missing": "email"}` |
| `missing_some` | Check if some variables are missing | `{"missing_some": [1, ["a", "b"]]}` |

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
vars preserve the `COALESCE` expression while applying schema-required coercion to
the comparison literal. The default value is emitted as provided; visible enum
defaults are validated when enum values are configured.

> **Note:** Path segments that start with a digit (e.g. `24h`, `7d`) are automatically quoted using the dialect-appropriate character. See [Identifier Quoting](dialects.md#identifier-quoting) for details.

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
status = 'active'
```

### Strict Equality

```json
{"===": [{"var": "count"}, 5]}
```
```sql
count = 5
```

With a schema, strict equality folds field/literal type mismatches that are
known at transpile time:

```json
{"===": [{"var": "count"}, "5"]}
```
```sql
FALSE
```

### Schema-Aware Equality Coercion

When a schema is configured, equality and inequality use type-aware literal
coercion where the JSONLogic behavior is portable SQL:

- Numeric fields coerce known string and boolean literals through JavaScript-like
  `ToNumber` semantics: `"010"` becomes `10`, `"0x10"` becomes `16`, `true`
  becomes `1`, and non-numeric strings such as `"abc"` fold to `FALSE` for
  `==`.
- Boolean fields coerce known numeric and string literals through `ToNumber`,
  then map `1` to `TRUE` and `0` to `FALSE`; any other finite number cannot
  match a boolean field and folds to a constant.
- String fields compared with numeric literals keep the canonical string match,
  for example `code == 5` emits `code = '5'`.
- Loose string/boolean field comparisons such as `code == true` return an error
  because JSONLogic's runtime string coercion cannot be expressed portably in
  SQL.

The string/number case is intentionally canonical, not a complete JavaScript
runtime model. `code == 5` matches `'5'`, but it does not also match strings
that JavaScript would coerce to the same number, such as `'05'`, `'5.0'`, or
`' 5 '`.

### Inequality

```json
{"!=": [{"var": "status"}, "inactive"]}
```
```sql
status != 'inactive'
```

### Equality with NULL

```json
{"==": [{"var": "deleted_at"}, null]}
```
```sql
deleted_at IS NULL
```

### Inequality with NULL

```json
{"!=": [{"var": "field"}, null]}
```
```sql
field IS NOT NULL
```

### Null-Safe Field Equality

Field-to-field equality uses JSONLogic-compatible null-safe SQL by default:

```json
{"==": [{"var": "a"}, {"var": "b"}]}
```
```sql
((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b))
```

This matches JSONLogic's `null == null` and `null === null` behavior when both
compared fields are `NULL`.

For inequality, the null-safe fallback checks one-null-only rows plus the ordinary
comparison:

```sql
((a IS NULL AND b IS NOT NULL) OR (a IS NOT NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a != b))
```

This mode only applies when both operands are `var` expressions, including
`[field, default]` vars. Comparisons such as `field == null` and field/literal
schema coercion keep their existing SQL.

### Logical NOT

```json
{"!": [{"var": "isDeleted"}]}
```
```sql
-- With schema: isDeleted is boolean
NOT (isDeleted IS TRUE)
```

### Double Negation (Boolean Conversion)

```json
{"!!": [{"var": "value"}]}
```

Field truthiness requires schema metadata because the transpiler must know
whether `value` should be treated as a string, number, boolean, or array. With a
declared field type, the `!!` operator generates type-appropriate SQL. See
[Schema-Aware Truthiness](schema-validation.md#schema-required-truthiness) for
details.

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

### Conditional Expression (if)

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
amount > 1000
score >= 80
age < 65
count <= 10
```

### Maximum/Minimum

```json
{"max": [{"var": "score1"}, {"var": "score2"}, {"var": "score3"}]}
{"min": [{"var": "price1"}, {"var": "price2"}]}
```
```sql
GREATEST(score1, score2, score3)
LEAST(price1, price2)
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
(price + tax)
(total - discount)
(price * 1.2)
(total / 2)
(count % 3)
```

### String Operands in Arithmetic

When string literals appear in numeric operations, the transpiler coerces them following JSONLogic's JavaScript-like semantics:

- **Valid numeric strings** are coerced to numbers: `"42"` becomes `42`, `"3.14"` becomes `3.14`
- **Whitespace-padded numeric strings** are trimmed then coerced: `" 3 "` becomes `3`
- **Non-numeric strings** are safely quoted as string literals: `"hello"` becomes `'hello'`
- **Special float values** (`"NaN"`, `"Inf"`) are safely quoted: `"NaN"` becomes `'NaN'`
- **Large integers** are preserved exactly without float64 precision loss: `"9223372036854775808"` stays `9223372036854775808`

```json
{"+": ["42", 1]}
{"*": [" 3 ", 2]}
{"+": ["hello", 1]}
```
```sql
(42 + 1)
(3 * 2)
('hello' + 1)
```

### Unary Operations

```json
{"-": [{"var": "value"}]}
{"+": ["-5"]}
```
```sql
-value
CAST(-5 AS NUMERIC)
```

## Array Operations

| Operator | Description |
|----------|-------------|
| `in` | Check if value is in array |
| `map`, `filter`, `reduce` | Array transformations |
| `all`, `some`, `none` | Array condition checks |
| `merge` | Merge arrays, casting scalar arguments to single-element arrays |

### In Array

```json
{"in": [{"var": "country"}, ["US", "CA", "MX"]]}
```
```sql
country IN ('US', 'CA', 'MX')
```

When the right-hand side is an array-typed field (with schema), `in` uses
dialect-specific array membership syntax (e.g., BigQuery/Spanner use
`value IN UNNEST(array)`; PostgreSQL uses `value = ANY(array)`).

When the right-hand side is a known non-container value such as a number,
boolean, null, or an empty array literal, `in` folds to `FALSE` because
JSONLogic membership only applies to strings and arrays.

When a schema is provided, array elements are automatically coerced to match the field type. For example, numeric values in the array are quoted as strings when the field is a string type:

```json
// Schema: merchant_code is string type
{"in": [{"var": "merchant_code"}, [5960, 9000]]}
```
```sql
merchant_code IN ('5960', '9000')
```

See [Type Coercion](schema-validation.md#type-coercion) for details.

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

### Reduce Array

```json
{"reduce": [{"var": "numbers"}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}
```
```sql
0 + COALESCE((SELECT SUM(elem) FROM UNNEST(numbers) AS elem), 0)
```

### Nested Array Scope

Inside `map`, `filter`, `all`, `some`, and `none` lambdas, bare vars resolve against the current element:

```json
{"map": [{"var": "records"}, {"var": "type"}]}
```
```sql
ARRAY(SELECT elem.type FROM UNNEST(records) AS elem)
```

Use `{"var": ""}` for the current element itself, and array-form vars for defaults, for example `{"var":["type","unknown"]}` -> `COALESCE(elem.type, 'unknown')`.

The implementation-specific dotted aliases `.type`, `item.type`, `current.type`, and generated SQL aliases such as `elem.type` are not supported in these lambdas. If the element schema really defines a nested field with that name, for example an `elem` object with a `type` field, `{"var":"elem.type"}` is treated as normal JSONLogic data access and emitted under the current SQL element alias. In `reduce`, use official JSONLogic names: `{"var":"current"}`, `{"var":"current.type"}`, and `{"var":"accumulator"}`. The whole reduce scope (`{"var":""}`) is an object in JSONLogic and is not emitted as a scalar SQL expression.

For nested array operators, the transpiler keeps inner and outer generated SQL aliases distinct (for example `elem`, `elem1`) when needed.

### All Elements Satisfy Condition

```json
{"all": [{"var": "ages"}, {">=": [{"var": ""}, 18]}]}
```
```sql
-- BigQuery/Spanner
(ARRAY_LENGTH(ages) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18)))
-- PostgreSQL
(CARDINALITY(ages) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18)))
-- DuckDB
(length(ages) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18)))
-- ClickHouse
(length(ages) > 0 AND arrayAll(elem -> elem >= 18, ages))
```

> **Note:** The array length guard ensures JSONLogic spec compliance - `{"all": [[], condition]}` returns `false` (not `true`). Each dialect uses its native array length function: `ARRAY_LENGTH` (BigQuery/Spanner), `CARDINALITY` (PostgreSQL), `length` (DuckDB/ClickHouse).

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

JSONLogic `merge` casts non-array arguments to arrays. The transpiler follows
that behavior when the scalar element type is statically known:

```json
{"merge": [1, [2]]}
```
```sql
ARRAY_CONCAT([1], [2])
```

Unknown-typed or incompatible scalar element types return an explicit error
instead of generating dialect-specific invalid array SQL.

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
POSITION('hello' IN 'hello world') > 0
```

For string containment, non-string needles are coerced with JavaScript-style
stringification before searching. For example, nullable field needles become
`COALESCE(value, 'null')`, while boolean needles become `'true'`, `'false'`,
or `'null'`.

### Concatenate Strings

```json
{"cat": [{"var": "firstName"}, " ", {"var": "lastName"}]}
```
```sql
CONCAT(COALESCE(CAST(firstName AS STRING), ''), ' ', COALESCE(CAST(lastName AS STRING), ''))
```

`cat` follows JSONLogic stringification: `null` stringifies as an empty
string, and nullable operands are wrapped so SQL `CONCAT` does not return
`NULL` for the whole expression.

### Concatenate with Conditional

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

## See Also

- [SQL Dialects](dialects.md) - Dialect-specific operator behavior
- [Custom Operators](custom-operators.md) - Add your own operators
- [Examples](examples.md) - More complex examples
