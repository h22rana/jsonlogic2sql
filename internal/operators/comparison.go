package operators

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// ComparisonOperator handles comparison operators (==, ===, !=, !==, >, >=, <, <=).
type ComparisonOperator struct {
	config *OperatorConfig
	dataOp *DataOperator
}

type equalityDecision struct {
	handled     bool
	left        interface{}
	right       interface{}
	constant    *bool
	unsupported error
}

type equalityFieldOperand struct {
	fieldName           string
	hasDefault          bool
	defaultLiteral      interface{}
	defaultLiteralKnown bool
}

type jsNumberLiteral struct {
	value    interface{}
	float    float64
	integral bool
}

// NewComparisonOperator creates a new comparison operator with optional config.
func NewComparisonOperator(config *OperatorConfig) *ComparisonOperator {
	return &ComparisonOperator{
		config: config,
		dataOp: NewDataOperator(config), // Same config, no propagation needed
	}
}

// schema returns the schema from config, or nil if not configured.
func (c *ComparisonOperator) schema() SchemaProvider {
	if c.config == nil {
		return nil
	}
	return c.config.Schema
}

// arrayMembershipSQL generates dialect-specific SQL for checking if a value exists in an array column.
// BigQuery/Spanner: value IN UNNEST(array)
// PostgreSQL: value = ANY(array)
// DuckDB: list_contains(array, value)
// ClickHouse: has(array, value).
func (c *ComparisonOperator) arrayMembershipSQL(valueSQL, arraySQL string) string {
	d := dialect.DialectUnspecified
	if c.config != nil {
		d = c.config.GetDialect()
	}

	switch d {
	case dialect.DialectPostgreSQL:
		return fmt.Sprintf("%s = ANY(%s)", valueSQL, arraySQL)
	case dialect.DialectDuckDB:
		return fmt.Sprintf("list_contains(%s, %s)", arraySQL, valueSQL)
	case dialect.DialectClickHouse:
		return fmt.Sprintf("has(%s, %s)", arraySQL, valueSQL)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner:
		return fmt.Sprintf("%s IN UNNEST(%s)", valueSQL, arraySQL)
	}
	// Fallback for any future dialects
	return fmt.Sprintf("%s IN UNNEST(%s)", valueSQL, arraySQL)
}

// strposFunc returns the appropriate string position function call based on dialect.
// BigQuery/Spanner/DuckDB: STRPOS(haystack, needle)
// PostgreSQL: POSITION(needle IN haystack)
// ClickHouse: position(haystack, needle).
func (c *ComparisonOperator) strposFunc(haystack, needle string) string {
	d := dialect.DialectUnspecified
	if c.config != nil {
		d = c.config.GetDialect()
	}

	switch d {
	case dialect.DialectPostgreSQL:
		return fmt.Sprintf("POSITION(%s IN %s)", needle, haystack)
	case dialect.DialectClickHouse:
		return fmt.Sprintf("position(%s, %s)", haystack, needle)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectDuckDB:
		return fmt.Sprintf("STRPOS(%s, %s)", haystack, needle)
	}
	// Fallback for any future dialects
	return fmt.Sprintf("STRPOS(%s, %s)", haystack, needle)
}

// validateOrderingOperand checks if a field used in an ordering comparison is of a valid type
// Only numeric and string types support ordering comparisons (>, >=, <, <=)
// Rejects array, object, and boolean types.
func (c *ComparisonOperator) validateOrderingOperand(value interface{}, operator string) error {
	if c.schema() == nil {
		return nil // No schema, no validation
	}

	fieldName := c.extractFieldNameFromValue(value)
	if fieldName == "" {
		return nil // Can't determine field name, skip validation
	}

	fieldType := c.schema().GetFieldType(fieldName)
	if fieldType == "" {
		return nil // Field not in schema, skip validation (existence checked by DataOperator)
	}

	// Allow numeric and string types for ordering comparisons
	if c.schema().IsNumericType(fieldName) || c.schema().IsStringType(fieldName) {
		return nil
	}

	// Disallow array, object, boolean for ordering comparisons
	return fmt.Errorf("ordering comparison '%s' on incompatible field '%s' (type: %s)", operator, fieldName, fieldType)
}

// extractFieldNameFromValue extracts field name from a value that might be a var expression.
func (c *ComparisonOperator) extractFieldNameFromValue(value interface{}) string {
	if pv, ok := value.(ProcessedValue); ok && pv.IsSQL && pv.IsField {
		return pv.FieldName
	}
	if varExpr, ok := value.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			return c.extractFieldName(varName)
		}
	}
	return ""
}

// extractFieldName extracts the field name from a var argument.
func (c *ComparisonOperator) extractFieldName(varName interface{}) string {
	if nameStr, ok := varName.(string); ok {
		return nameStr
	}
	if nameArr, ok := varName.([]interface{}); ok && len(nameArr) > 0 {
		if nameStr, ok := nameArr[0].(string); ok {
			return nameStr
		}
	}
	return ""
}

func (c *ComparisonOperator) extractEqualityFieldOperand(value interface{}) (equalityFieldOperand, bool) {
	if pv, ok := value.(ProcessedValue); ok && pv.IsSQL && pv.IsField {
		return equalityFieldOperandFromProcessedValue(pv), true
	}
	varExpr, ok := value.(map[string]interface{})
	if !ok {
		return equalityFieldOperand{}, false
	}
	if len(varExpr) != 1 {
		return equalityFieldOperand{}, false
	}
	varName, hasVar := varExpr[OpVar]
	if !hasVar {
		return equalityFieldOperand{}, false
	}
	switch v := varName.(type) {
	case string:
		return equalityFieldOperand{fieldName: v}, true
	case []interface{}:
		if len(v) == 0 {
			return equalityFieldOperand{}, false
		}
		if name, ok := v[0].(string); ok {
			operand := equalityFieldOperand{fieldName: name}
			if len(v) > 1 {
				setEqualityFieldDefault(&operand, v[1])
			}
			return operand, true
		}
		if pv, ok := v[0].(ProcessedValue); ok && pv.IsSQL && pv.IsField {
			operand := equalityFieldOperandFromProcessedValue(pv)
			if len(v) > 1 {
				setEqualityFieldDefault(&operand, v[1])
			}
			return operand, true
		}
	}
	return equalityFieldOperand{}, false
}

func equalityFieldOperandFromProcessedValue(pv ProcessedValue) equalityFieldOperand {
	operand := equalityFieldOperand{fieldName: pv.FieldName}
	if pv.FieldHasDefault {
		operand.hasDefault = true
		if pv.FieldDefaultLiteralKnown {
			operand.defaultLiteral = pv.FieldDefaultLiteral
			operand.defaultLiteralKnown = true
		}
	}
	return operand
}

func setEqualityFieldDefault(operand *equalityFieldOperand, defaultValue interface{}) {
	operand.hasDefault = true
	if literal, ok := equalityLiteralValue(defaultValue); ok {
		operand.defaultLiteral = literal
		operand.defaultLiteralKnown = true
	}
}

func isEqualityOperator(operator string) bool {
	return operator == "==" || operator == "===" || operator == "!=" || operator == "!=="
}

func (c *ComparisonOperator) nullSafeFieldEqualityEnabled() bool {
	return c.config != nil && c.config.NullSafeFieldEquality
}

func (c *ComparisonOperator) shouldUseNullSafeFieldEquality(operator string, leftArg, rightArg interface{}) bool {
	if !c.nullSafeFieldEqualityEnabled() || !isEqualityOperator(operator) {
		return false
	}
	return c.isNullSafeFieldOperand(leftArg) && c.isNullSafeFieldOperand(rightArg)
}

func (c *ComparisonOperator) isNullSafeFieldOperand(value interface{}) bool {
	if _, ok := c.extractEqualityFieldOperand(value); ok {
		return true
	}
	return isProcessedSQLFieldOperand(value)
}

func isProcessedSQLFieldOperand(value interface{}) bool {
	if pv, ok := value.(ProcessedValue); ok {
		return pv.IsSQL && pv.IsField
	}
	return false
}

func nullSafeFieldEqualitySQL(operator, leftSQL, rightSQL string) string {
	switch operator {
	case "==", "===":
		return fmt.Sprintf("((%s IS NULL AND %s IS NULL) OR (%s IS NOT NULL AND %s IS NOT NULL AND %s = %s))",
			leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL)
	case "!=":
		return fmt.Sprintf("((%s IS NULL AND %s IS NOT NULL) OR (%s IS NOT NULL AND %s IS NULL) OR (%s IS NOT NULL AND %s IS NOT NULL AND %s != %s))",
			leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL)
	case "!==":
		return fmt.Sprintf("((%s IS NULL AND %s IS NOT NULL) OR (%s IS NOT NULL AND %s IS NULL) OR (%s IS NOT NULL AND %s IS NOT NULL AND %s <> %s))",
			leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL)
	default:
		return ""
	}
}

func isStrictEqualityOperator(operator string) bool {
	return operator == "===" || operator == "!=="
}

func impossibleEqualityPredicateConstant(operator string) *bool {
	result := operator == "!=" || operator == "!=="

	return &result
}

func equalityPredicateConstant(operator string, left, right bool) bool {
	switch operator {
	case "==", "===":
		return left == right
	case "!=", "!==":
		return left != right
	default:
		return false
	}
}

// FoldLiteralComparison returns the JSONLogic result for comparisons whose
// operands are fully known literals. The boolean return after the result tells
// callers whether folding was possible.
func FoldLiteralComparison(operator string, args []interface{}) (bool, bool, error) {
	if len(args) != 2 {
		return false, false, nil
	}

	switch operator {
	case "==", "===", "!=", "!==":
		left, leftOK := equalityLiteralValue(args[0])
		right, rightOK := equalityLiteralValue(args[1])
		if !leftOK || !rightOK {
			return false, false, nil
		}
		if equalityLiteralKind(left) == "" || equalityLiteralKind(right) == "" {
			return false, false, nil
		}
		if err := validateEqualityJSONNumberLiteral(left); err != nil {
			return false, false, err
		}
		if err := validateEqualityJSONNumberLiteral(right); err != nil {
			return false, false, err
		}
		if hasOverflowedJSONNumberLiteral(left, right) {
			return false, false, nil
		}

		var equal bool
		if isStrictEqualityOperator(operator) {
			equal = equalityLiteralsStrictEqual(left, right)
		} else {
			equal = equalityLiteralsLooseEqual(left, right)
		}
		if operator == "!=" || operator == "!==" {
			equal = !equal
		}
		return equal, true, nil
	case ">", ">=", "<", "<=":
		return foldLiteralOrderingComparison(operator, args[0], args[1])
	case "in":
		return foldLiteralInComparison(args[0], args[1])
	default:
		return false, false, nil
	}
}

func foldLiteralOrderingComparison(operator string, leftArg, rightArg interface{}) (bool, bool, error) {
	left, leftOK := equalityLiteralValue(leftArg)
	right, rightOK := equalityLiteralValue(rightArg)
	if !leftOK || !rightOK {
		return false, false, nil
	}
	if equalityLiteralKind(left) == "" || equalityLiteralKind(right) == "" {
		return false, false, nil
	}
	if hasOverflowedJSONNumberLiteral(left, right) {
		return false, false, nil
	}

	if leftString, ok := left.(string); ok {
		if rightString, ok := right.(string); ok {
			cmp := strings.Compare(leftString, rightString)
			switch operator {
			case ">":
				return cmp > 0, true, nil
			case ">=":
				return cmp >= 0, true, nil
			case "<":
				return cmp < 0, true, nil
			case "<=":
				return cmp <= 0, true, nil
			}
		}
	}

	leftNumber, leftHandled, leftValid := jsNumberFromOrderingLiteral(left)
	rightNumber, rightHandled, rightValid := jsNumberFromOrderingLiteral(right)
	if !leftHandled || !rightHandled {
		return false, false, nil
	}
	if !leftValid || !rightValid {
		return false, true, nil
	}

	switch operator {
	case ">":
		return leftNumber.float > rightNumber.float, true, nil
	case ">=":
		return leftNumber.float >= rightNumber.float, true, nil
	case "<":
		return leftNumber.float < rightNumber.float, true, nil
	case "<=":
		return leftNumber.float <= rightNumber.float, true, nil
	default:
		return false, false, nil
	}
}

func jsNumberFromOrderingLiteral(value interface{}) (jsNumberLiteral, bool, bool) {
	if value == nil {
		return newJSIntNumber(0), true, true
	}
	return jsNumberFromLiteral(value)
}

func hasOverflowedJSONNumberLiteral(values ...interface{}) bool {
	for _, value := range values {
		num, ok := value.(json.Number)
		if !ok {
			continue
		}
		if _, err := normalizeJSONNumberLiteral(num); err != nil {
			continue
		}
		f, _ := strconv.ParseFloat(num.String(), 64)
		if math.IsInf(f, 0) {
			return true
		}
	}
	return false
}

func foldLiteralInComparison(leftArg, rightArg interface{}) (bool, bool, error) {
	left, leftOK := equalityLiteralValue(leftArg)
	if !leftOK {
		return false, false, nil
	}
	if equalityLiteralKind(left) == "" {
		return false, false, nil
	}

	switch right := rightArg.(type) {
	case string:
		leftString, ok := left.(string)
		if !ok {
			return false, false, nil
		}
		return strings.Contains(right, leftString), true, nil
	case []interface{}:
		for _, item := range right {
			itemLiteral, ok := equalityLiteralValue(item)
			if !ok || equalityLiteralKind(itemLiteral) == "" {
				return false, false, nil
			}
			if equalityLiteralsStrictEqual(left, itemLiteral) {
				return true, true, nil
			}
		}
		return false, true, nil
	default:
		return false, false, nil
	}
}

func boolSQL(value bool) string {
	if value {
		return "TRUE"
	}
	return "FALSE"
}

func sqlBooleanConstant(sql string) (bool, bool) {
	s := strings.TrimSpace(sql)
	if len(s) >= 2 && s[0] == '(' && s[len(s)-1] == ')' {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if inner == "TRUE" || inner == "FALSE" {
			s = inner
		}
	}

	switch s {
	case "TRUE":
		return true, true
	case "FALSE":
		return false, true
	default:
		return false, false
	}
}

func equalityLiteralValue(value interface{}) (interface{}, bool) {
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			if typedNullExpression(pv) {
				return nil, true
			}
			return nil, false
		}
		return pv.Value, true
	}

	switch value.(type) {
	case map[string]interface{}, []interface{}:
		return nil, false
	default:
		return value, true
	}
}

func equalityLiteralKind(value interface{}) string {
	switch value.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number, float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return "number"
	default:
		return ""
	}
}

func validateEqualityJSONNumberLiteral(value interface{}) error {
	if n, ok := value.(json.Number); ok {
		_, err := normalizeJSONNumberLiteral(n)
		return err
	}
	return nil
}

func equalityLiteralsStrictEqual(left, right interface{}) bool {
	leftKind := equalityLiteralKind(left)
	rightKind := equalityLiteralKind(right)
	if leftKind == "" || leftKind != rightKind {
		return false
	}

	switch leftKind {
	case "null":
		return true
	case "string":
		leftValue, leftOK := left.(string)
		rightValue, rightOK := right.(string)
		return leftOK && rightOK && leftValue == rightValue
	case "boolean":
		leftValue, leftOK := left.(bool)
		rightValue, rightOK := right.(bool)
		return leftOK && rightOK && leftValue == rightValue
	case "number":
		leftNumber, leftHandled, leftValid := jsNumberFromLiteral(left)
		rightNumber, rightHandled, rightValid := jsNumberFromLiteral(right)
		return leftHandled && rightHandled && leftValid && rightValid && leftNumber.float == rightNumber.float
	default:
		return false
	}
}

func equalityLiteralsLooseEqual(left, right interface{}) bool {
	leftKind := equalityLiteralKind(left)
	rightKind := equalityLiteralKind(right)
	if leftKind == "" || rightKind == "" {
		return true
	}
	if leftKind == "null" || rightKind == "null" {
		return leftKind == "null" && rightKind == "null"
	}
	if leftKind == rightKind {
		return equalityLiteralsStrictEqual(left, right)
	}
	if leftKind == "boolean" || rightKind == "boolean" ||
		(leftKind == "number" && rightKind == "string") ||
		(leftKind == "string" && rightKind == "number") {
		leftNumber, leftHandled, leftValid := jsNumberFromLiteral(left)
		rightNumber, rightHandled, rightValid := jsNumberFromLiteral(right)
		return leftHandled && rightHandled && leftValid && rightValid && leftNumber.float == rightNumber.float
	}
	return false
}

func (operand equalityFieldOperand) defaultCanStrictEqual(literal interface{}) bool {
	return !operand.defaultLiteralKnown || equalityLiteralsStrictEqual(operand.defaultLiteral, literal)
}

func (operand equalityFieldOperand) defaultCanLooseEqual(literal interface{}) bool {
	return !operand.defaultLiteralKnown || equalityLiteralsLooseEqual(operand.defaultLiteral, literal)
}

func (operand equalityFieldOperand) defaultCanHaveStrictKind(kind string) bool {
	if !operand.hasDefault {
		return false
	}
	return !operand.defaultLiteralKnown || equalityLiteralKind(operand.defaultLiteral) == kind
}

func hasRadixPrefix(s string) bool {
	if len(s) < 2 || s[0] != '0' {
		return false
	}
	switch s[1] {
	case 'x', 'X', 'o', 'O', 'b', 'B':
		return true
	default:
		return false
	}
}

func radixBase(prefix byte) int {
	switch prefix {
	case 'x', 'X':
		return 16
	case 'o', 'O':
		return 8
	case 'b', 'B':
		return 2
	default:
		return 10
	}
}

func validRadixDigits(s string, base int) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			if int(r-'0') >= base {
				return false
			}
		case base == 16 && r >= 'a' && r <= 'f':
		case base == 16 && r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func jsNumberFromString(input string) (jsNumberLiteral, bool) {
	s := strings.TrimSpace(input)
	if s == "" {
		return newJSIntNumber(0), true
	}

	if s[0] == '+' || s[0] == '-' {
		remainder := s[1:]
		if hasRadixPrefix(remainder) {
			return jsNumberLiteral{}, false
		}
	} else if hasRadixPrefix(s) {
		base := radixBase(s[1])
		digits := s[2:]
		if !validRadixDigits(digits, base) {
			return jsNumberLiteral{}, false
		}
		if i, err := strconv.ParseInt(digits, base, 64); err == nil && i >= minSafeJSInt && i <= maxSafeJSInt {
			return newJSIntNumber(i), true
		}
		bigInt := new(big.Int)
		if _, ok := bigInt.SetString(digits, base); !ok {
			return jsNumberLiteral{}, false
		}
		f, _ := new(big.Float).SetInt(bigInt).Float64()
		if math.IsInf(f, 0) {
			return jsNumberLiteral{}, false
		}
		return newJSFloatNumber(f), true
	}

	if strings.Contains(s, "_") {
		return jsNumberLiteral{}, false
	}

	if i, err := strconv.ParseInt(s, 10, 64); err == nil && i >= minSafeJSInt && i <= maxSafeJSInt {
		return newJSIntNumber(i), true
	}

	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return jsNumberLiteral{}, false
	}
	return newJSFloatNumber(f), true
}

func newJSIntNumber(i int64) jsNumberLiteral {
	return jsNumberLiteral{
		value:    i,
		float:    float64(i),
		integral: true,
	}
}

func newJSFloatNumber(f float64) jsNumberLiteral {
	return jsNumberLiteral{
		value:    f,
		float:    f,
		integral: math.Trunc(f) == f,
	}
}

func newJSUintNumber(value interface{}, f float64) jsNumberLiteral {
	return jsNumberLiteral{
		value:    value,
		float:    f,
		integral: true,
	}
}

func jsNumberFromLiteral(value interface{}) (jsNumberLiteral, bool, bool) {
	switch v := value.(type) {
	case string:
		n, ok := jsNumberFromString(v)
		return n, true, ok
	case bool:
		if v {
			return newJSIntNumber(1), true, true
		}
		return newJSIntNumber(0), true, true
	case json.Number:
		if _, err := normalizeJSONNumberLiteral(v); err != nil {
			return jsNumberLiteral{}, false, false
		}
		numStr := v.String()
		if i, err := strconv.ParseInt(numStr, 10, 64); err == nil {
			return newJSIntNumber(i), true, true
		}
		f, err := strconv.ParseFloat(numStr, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return jsNumberLiteral{}, true, false
		}
		return newJSFloatNumber(f), true, true
	case float32:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return jsNumberLiteral{}, true, false
		}
		return newJSFloatNumber(f), true, true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return jsNumberLiteral{}, true, false
		}
		return newJSFloatNumber(v), true, true
	case int:
		return newJSIntNumber(int64(v)), true, true
	case int8:
		return newJSIntNumber(int64(v)), true, true
	case int16:
		return newJSIntNumber(int64(v)), true, true
	case int32:
		return newJSIntNumber(int64(v)), true, true
	case int64:
		return newJSIntNumber(v), true, true
	case uint:
		return newJSUintNumber(v, float64(v)), true, true
	case uint8:
		return newJSUintNumber(v, float64(v)), true, true
	case uint16:
		return newJSUintNumber(v, float64(v)), true, true
	case uint32:
		return newJSUintNumber(v, float64(v)), true, true
	case uint64:
		return newJSUintNumber(v, float64(v)), true, true
	default:
		return jsNumberLiteral{}, false, false
	}
}

func jsonNumberIntegerOutsideInt64(value interface{}) bool {
	num, ok := value.(json.Number)
	if !ok {
		return false
	}
	numStr := num.String()
	if !isIntegerLiteral(numStr) {
		return false
	}
	_, err := strconv.ParseInt(numStr, 10, 64)
	return err != nil
}

func int64FromJSNumber(n jsNumberLiteral) (int64, bool) {
	if i, ok := n.value.(int64); ok {
		return i, true
	}
	switch v := n.value.(type) {
	case uint:
		u := uint64(v)
		if u <= uint64(math.MaxInt64) {
			return int64(u), true
		}
		return 0, false
	case uint8:
		return int64(v), true
	case uint16:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v <= uint64(math.MaxInt64) {
			return int64(v), true
		}
		return 0, false
	}
	if !n.integral {
		return 0, false
	}

	// float64(math.MaxInt64) rounds up to 2^63, so using math.MaxInt64 as
	// a float bound can silently clamp MaxInt64+1 back to MaxInt64. Only
	// convert float-origin values that are inside the valid int64 range.
	// The lower bound is exactly representable and valid; the upper bound is
	// exclusive because it is 2^63, one greater than MaxInt64.
	const (
		minInt64Float = -9223372036854775808.0
		maxInt64Float = 9223372036854775808.0
	)
	if n.float < minInt64Float || n.float >= maxInt64Float {
		return 0, false
	}
	i := int64(n.float)
	if float64(i) != n.float {
		return 0, false
	}
	return i, true
}

func jsNumberToCanonicalString(n jsNumberLiteral) string {
	return jsNumberFloatToString(n.float)
}

func nativeIntegerLiteralString(value interface{}) (string, bool) {
	switch v := value.(type) {
	case int:
		return strconv.FormatInt(int64(v), 10), true
	case int8:
		return strconv.FormatInt(int64(v), 10), true
	case int16:
		return strconv.FormatInt(int64(v), 10), true
	case int32:
		return strconv.FormatInt(int64(v), 10), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case uint:
		return strconv.FormatUint(uint64(v), 10), true
	case uint8:
		return strconv.FormatUint(uint64(v), 10), true
	case uint16:
		return strconv.FormatUint(uint64(v), 10), true
	case uint32:
		return strconv.FormatUint(uint64(v), 10), true
	case uint64:
		return strconv.FormatUint(v, 10), true
	default:
		return "", false
	}
}

func nativeFloat32LiteralString(value interface{}) (string, bool) {
	v, ok := value.(float32)
	if !ok {
		return "", false
	}
	f := float64(v)
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", false
	}
	return strconv.FormatFloat(f, 'g', -1, 32), true
}

func stringFieldNumericLiteralString(value interface{}) (string, bool, bool) {
	if exact, ok := nativeIntegerLiteralString(value); ok {
		return exact, true, true
	}
	if exact, ok := nativeFloat32LiteralString(value); ok {
		return exact, true, true
	}
	if n, handled, valid := jsNumberFromLiteral(value); handled {
		if valid {
			return jsNumberToCanonicalString(n), true, true
		}
		if canonical, ok := invalidJSNumberStringForStringField(value); ok {
			return canonical, true, true
		}
		return "", true, false
	}
	return "", false, false
}

func invalidJSNumberStringForStringField(value interface{}) (string, bool) {
	switch v := value.(type) {
	case json.Number:
		if _, err := normalizeJSONNumberLiteral(v); err != nil {
			return "", false
		}
		f, err := strconv.ParseFloat(v.String(), 64)
		if err != nil && !math.IsInf(f, 0) && f != 0 {
			return "", false
		}
		return finiteOrInfiniteJSNumberString(f)
	case float32:
		return finiteOrInfiniteJSNumberString(float64(v))
	case float64:
		return finiteOrInfiniteJSNumberString(v)
	default:
		return "", false
	}
}

func finiteOrInfiniteJSNumberString(f float64) (string, bool) {
	switch {
	case math.IsInf(f, 1):
		return "Infinity", true
	case math.IsInf(f, -1):
		return "-Infinity", true
	case math.IsNaN(f):
		return "", false
	default:
		return jsNumberToCanonicalString(newJSFloatNumber(f)), true
	}
}

func jsNumberFloatToString(f float64) string {
	if f == 0 {
		return "0"
	}

	s := strconv.FormatFloat(f, 'g', -1, 64)
	abs := math.Abs(f)
	if abs >= 1e-6 && abs < 1e21 {
		if strings.ContainsAny(s, "eE") {
			if expanded, ok := expandScientificDecimal(s); ok {
				return expanded
			}
		}
		return s
	}
	return normalizeExponentString(s)
}

func expandScientificDecimal(s string) (string, bool) {
	expIndex := strings.IndexAny(s, "eE")
	if expIndex < 0 {
		return s, true
	}

	mantissa := s[:expIndex]
	exp, err := strconv.Atoi(s[expIndex+1:])
	if err != nil {
		return "", false
	}

	sign := ""
	if strings.HasPrefix(mantissa, "-") || strings.HasPrefix(mantissa, "+") {
		if mantissa[0] == '-' {
			sign = "-"
		}
		mantissa = mantissa[1:]
	}

	decimalPos := strings.IndexByte(mantissa, '.')
	if decimalPos < 0 {
		decimalPos = len(mantissa)
	}
	digits := strings.ReplaceAll(mantissa, ".", "")
	newPos := decimalPos + exp

	var result string
	switch {
	case newPos <= 0:
		result = "0." + strings.Repeat("0", -newPos) + digits
	case newPos >= len(digits):
		result = digits + strings.Repeat("0", newPos-len(digits))
	default:
		result = digits[:newPos] + "." + digits[newPos:]
		result = strings.TrimRight(result, "0")
		result = strings.TrimRight(result, ".")
	}

	if result == "" || result == "0" {
		return "0", true
	}
	return sign + result, true
}

func normalizeExponentString(s string) string {
	expIndex := strings.IndexAny(s, "eE")
	if expIndex < 0 {
		return s
	}

	mantissa := s[:expIndex]
	exp, err := strconv.Atoi(s[expIndex+1:])
	if err != nil {
		return s
	}
	sign := "+"
	if exp < 0 {
		sign = "-"
		exp = -exp
	}
	return fmt.Sprintf("%se%s%d", mantissa, sign, exp)
}

func (c *ComparisonOperator) fieldEqualityKind(fieldName string) string {
	if c.schema() == nil || fieldName == "" {
		return ""
	}
	switch {
	case c.schema().IsNumericType(fieldName):
		return "number"
	case c.schema().IsBooleanType(fieldName):
		return "boolean"
	case c.schema().IsStringType(fieldName), c.schema().IsEnumType(fieldName):
		return "string"
	default:
		return ""
	}
}

func (c *ComparisonOperator) setLiteralForFieldSide(dec *equalityDecision, fieldOnLeft bool, value interface{}) {
	if fieldOnLeft {
		dec.right = value
		return
	}
	dec.left = value
}

func (c *ComparisonOperator) validateEqualityFieldOperand(field equalityFieldOperand) error {
	if c.schema() == nil || field.fieldName == "" {
		return nil
	}
	if err := c.schema().ValidateField(field.fieldName); err != nil {
		return err
	}
	if !field.hasDefault || !field.defaultLiteralKnown {
		return nil
	}
	if err := validateEqualityJSONNumberLiteral(field.defaultLiteral); err != nil {
		return err
	}
	if c.schema().IsEnumType(field.fieldName) {
		return c.validateEnumValue(field.defaultLiteral, field.fieldName)
	}
	return nil
}

func (c *ComparisonOperator) applyEqualitySemantics(operator string, leftArg, rightArg interface{}) equalityDecision {
	dec := equalityDecision{left: leftArg, right: rightArg}
	if !isEqualityOperator(operator) {
		return dec
	}

	leftField, leftIsField := c.extractEqualityFieldOperand(leftArg)
	rightField, rightIsField := c.extractEqualityFieldOperand(rightArg)
	if leftIsField {
		if err := c.validateEqualityFieldOperand(leftField); err != nil {
			dec.unsupported = err
			dec.handled = true
			return dec
		}
	}
	if rightIsField {
		if err := c.validateEqualityFieldOperand(rightField); err != nil {
			dec.unsupported = err
			dec.handled = true
			return dec
		}
	}
	if leftIsField == rightIsField {
		return c.applyTypedExpressionEqualitySemantics(dec, operator, leftArg, rightArg)
	}

	field := leftField
	literalArg := rightArg
	fieldOnLeft := true
	if !leftIsField {
		field = rightField
		literalArg = leftArg
		fieldOnLeft = false
	}
	if typedNullExpression(literalArg) {
		c.setLiteralForFieldSide(&dec, fieldOnLeft, nil)
		literalArg = nil
	}
	fieldName := field.fieldName

	fieldKind := c.fieldEqualityKind(fieldName)
	if fieldKind == "" {
		return c.applyTypedExpressionEqualitySemantics(dec, operator, leftArg, rightArg)
	}

	if isStrictEqualityOperator(operator) {
		if exprKind, exprTyped := expressionEqualityKind(literalArg); exprTyped && exprKind != fieldKind {
			if field.defaultCanHaveStrictKind(exprKind) {
				return dec
			}
			dec.handled = true
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
	}

	literal, ok := equalityLiteralValue(literalArg)
	if !ok {
		return dec
	}

	dec.handled = true

	if err := validateEqualityJSONNumberLiteral(literal); err != nil {
		dec.unsupported = err
		return dec
	}

	if literal == nil {
		return dec
	}

	if isStrictEqualityOperator(operator) {
		literalKind := equalityLiteralKind(literal)
		if literalKind != "" && literalKind != fieldKind {
			if field.hasDefault && field.defaultCanStrictEqual(literal) {
				return dec
			}
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
		if fieldKind == "number" {
			if _, isJSONNumber := literal.(json.Number); !isJSONNumber {
				if _, handled, valid := jsNumberFromLiteral(literal); handled && !valid {
					dec.constant = impossibleEqualityPredicateConstant(operator)
					return dec
				}
			}
		}
		if fieldKind == "number" && c.schema().GetFieldType(fieldName) == "integer" {
			if jsonNumberIntegerOutsideInt64(literal) {
				if field.hasDefault && field.defaultCanStrictEqual(literal) {
					return dec
				}
				dec.constant = impossibleEqualityPredicateConstant(operator)
				return dec
			}
			if n, handled, valid := jsNumberFromLiteral(literal); handled {
				if !valid || !n.integral {
					if field.hasDefault && field.defaultCanStrictEqual(literal) {
						return dec
					}
					dec.constant = impossibleEqualityPredicateConstant(operator)
					return dec
				}
				if _, ok := int64FromJSNumber(n); !ok {
					if field.hasDefault && field.defaultCanStrictEqual(literal) {
						return dec
					}
					dec.constant = impossibleEqualityPredicateConstant(operator)
					return dec
				}
			}
		}
		if err := c.validateEnumValue(literal, fieldName); err != nil {
			dec.unsupported = err
		}
		return dec
	}

	switch fieldKind {
	case "number":
		if c.schema().GetFieldType(fieldName) == "number" {
			if _, ok := literal.(json.Number); ok {
				return dec
			}
		}
		n, handled, valid := jsNumberFromLiteral(literal)
		if !handled {
			return dec
		}
		if !valid {
			if field.hasDefault && field.defaultCanLooseEqual(literal) {
				return dec
			}
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
		value := n.value
		if c.schema().GetFieldType(fieldName) == "integer" {
			if jsonNumberIntegerOutsideInt64(literal) {
				if field.hasDefault && field.defaultCanLooseEqual(literal) {
					return dec
				}
				dec.constant = impossibleEqualityPredicateConstant(operator)
				return dec
			}
			if !n.integral {
				if field.hasDefault && field.defaultCanLooseEqual(literal) {
					return dec
				}
				dec.constant = impossibleEqualityPredicateConstant(operator)
				return dec
			}
			i, ok := int64FromJSNumber(n)
			if !ok {
				if field.hasDefault && field.defaultCanLooseEqual(literal) {
					return dec
				}
				dec.constant = impossibleEqualityPredicateConstant(operator)
				return dec
			}
			value = i
		}
		c.setLiteralForFieldSide(&dec, fieldOnLeft, value)
	case "boolean":
		n, handled, valid := jsNumberFromLiteral(literal)
		if !handled {
			return dec
		}
		if !valid {
			if field.hasDefault && field.defaultCanLooseEqual(literal) {
				return dec
			}
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
		switch n.float {
		case 0:
			c.setLiteralForFieldSide(&dec, fieldOnLeft, false)
		case 1:
			c.setLiteralForFieldSide(&dec, fieldOnLeft, true)
		default:
			if field.hasDefault && field.defaultCanLooseEqual(literal) {
				return dec
			}
			dec.constant = impossibleEqualityPredicateConstant(operator)
		}
	case "string":
		if _, ok := literal.(bool); ok {
			dec.unsupported = fmt.Errorf(
				"loose equality between string field %q and boolean literal is not supported", fieldName)
			return dec
		}
		if equalityLiteralKind(literal) == "number" {
			if canonical, handled, possible := stringFieldNumericLiteralString(literal); handled {
				if !possible {
					dec.constant = impossibleEqualityPredicateConstant(operator)
					return dec
				}
				c.setLiteralForFieldSide(&dec, fieldOnLeft, canonical)
			}
		}
		if err := c.validateEnumValue(equalityLiteralForFieldSide(dec, fieldOnLeft), fieldName); err != nil {
			dec.unsupported = err
		}
	}

	return dec
}

func (c *ComparisonOperator) applyTypedExpressionEqualitySemantics(dec equalityDecision, operator string, leftArg, rightArg interface{}) equalityDecision {
	leftIsNull := typedNullExpression(leftArg)
	rightIsNull := typedNullExpression(rightArg)
	if leftIsNull || rightIsNull {
		dec.handled = true
		if leftIsNull {
			dec.left = nil
		}
		if rightIsNull {
			dec.right = nil
		}
		return dec
	}

	leftKind, leftTyped := expressionEqualityKind(leftArg)
	rightKind, rightTyped := expressionEqualityKind(rightArg)
	if leftTyped && rightTyped {
		if isStrictEqualityOperator(operator) && leftKind != rightKind {
			dec.handled = true
			dec.constant = impossibleEqualityPredicateConstant(operator)
		}
		return dec
	}
	if leftTyped == rightTyped {
		return dec
	}

	exprKind := leftKind
	literalArg := rightArg
	exprOnLeft := true
	if !leftTyped {
		exprKind = rightKind
		literalArg = leftArg
		exprOnLeft = false
	}

	literal, ok := equalityLiteralValue(literalArg)
	if !ok {
		return dec
	}
	if err := validateEqualityJSONNumberLiteral(literal); err != nil {
		dec.handled = true
		dec.unsupported = err
		return dec
	}
	if literal == nil {
		return dec
	}

	if isStrictEqualityOperator(operator) {
		literalKind := equalityLiteralKind(literal)
		if literalKind != "" && literalKind != exprKind {
			dec.handled = true
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
		if exprKind == "number" {
			if _, isJSONNumber := literal.(json.Number); !isJSONNumber {
				if _, handled, valid := jsNumberFromLiteral(literal); handled && !valid {
					dec.handled = true
					dec.constant = impossibleEqualityPredicateConstant(operator)
					return dec
				}
			}
		}
		return dec
	}

	switch exprKind {
	case "number":
		n, handled, valid := jsNumberFromLiteral(literal)
		if !handled {
			return dec
		}
		dec.handled = true
		if !valid {
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
		c.setLiteralForFieldSide(&dec, exprOnLeft, n.value)
	case "boolean":
		n, handled, valid := jsNumberFromLiteral(literal)
		if !handled {
			return dec
		}
		dec.handled = true
		if !valid {
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
		switch n.float {
		case 0:
			c.setLiteralForFieldSide(&dec, exprOnLeft, false)
		case 1:
			c.setLiteralForFieldSide(&dec, exprOnLeft, true)
		default:
			dec.constant = impossibleEqualityPredicateConstant(operator)
		}
	case "string":
		dec.handled = true
		if _, ok := literal.(bool); ok {
			dec.unsupported = fmt.Errorf("loose equality between string expression and boolean literal is not supported")
			return dec
		}
		if equalityLiteralKind(literal) == "number" {
			if canonical, handled, possible := stringFieldNumericLiteralString(literal); handled {
				if !possible {
					dec.constant = impossibleEqualityPredicateConstant(operator)
					return dec
				}
				c.setLiteralForFieldSide(&dec, exprOnLeft, canonical)
			}
		}
	}

	return dec
}

func expressionEqualityKind(value interface{}) (string, bool) {
	pv, ok := value.(ProcessedValue)
	if !ok || !pv.IsSQL || !pv.HasExpressionInfo {
		return "", false
	}
	if pv.Kind == ExpressionKindPredicate {
		return "boolean", true
	}
	switch pv.Type {
	case ExpressionTypeNull:
		return "null", true
	case ExpressionTypeBoolean:
		return "boolean", true
	case ExpressionTypeString:
		return "string", true
	case ExpressionTypeNumber:
		return "number", true
	case ExpressionTypeArray:
		return "array", true
	case ExpressionTypeUnknown:
		return "", false
	}
	return "", false
}

func typedNullExpression(value interface{}) bool {
	pv, ok := value.(ProcessedValue)
	return ok && pv.IsSQL && pv.HasExpressionInfo &&
		pv.Kind == ExpressionKindValue && pv.Type == ExpressionTypeNull
}

func equalityLiteralForFieldSide(dec equalityDecision, fieldOnLeft bool) interface{} {
	if fieldOnLeft {
		return dec.right
	}
	return dec.left
}

func (c *ComparisonOperator) applySchemaComparisonCoercion(leftArg, rightArg interface{}) (interface{}, interface{}, error) {
	leftFieldName := c.extractFieldNameFromValue(leftArg)
	rightFieldName := c.extractFieldNameFromValue(rightArg)
	leftIsField := leftFieldName != "" || isProcessedSQLFieldOperand(leftArg)
	rightIsField := rightFieldName != "" || isProcessedSQLFieldOperand(rightArg)

	if leftIsField && rightIsField {
		return leftArg, rightArg, nil
	}
	if leftFieldName != "" && !rightIsField {
		rightArg = c.coerceValueForComparison(rightArg, leftFieldName)
		if err := c.validateEnumValue(rightArg, leftFieldName); err != nil {
			return nil, nil, err
		}
	}
	if rightFieldName != "" && !leftIsField {
		leftArg = c.coerceValueForComparison(leftArg, rightFieldName)
		if err := c.validateEnumValue(leftArg, rightFieldName); err != nil {
			return nil, nil, err
		}
	}
	return leftArg, rightArg, nil
}

// coerceValueForComparison coerces a literal value based on the type of the field being compared.
// If the field is numeric and the value is a string that represents a number, it returns the unquoted number.
// If the field is a string and the value is a number, it returns the number as a string so it gets quoted.
// This ensures proper SQL comparisons like "field >= 50000" instead of "field >= '50000'"
// and "string_field IN ('5960', '9000')" instead of "string_field IN (5960, 9000)".
func (c *ComparisonOperator) coerceValueForComparison(value interface{}, fieldName string) interface{} {
	if c.schema() == nil || fieldName == "" {
		return value
	}

	// Coerce string → number for numeric fields
	if c.schema().IsNumericType(fieldName) {
		if strVal, ok := value.(string); ok {
			// Try to parse as integer first
			if intVal, err := strconv.ParseInt(strVal, 10, 64); err == nil {
				return intVal
			}
			// Try to parse as float
			if floatVal, err := strconv.ParseFloat(strVal, 64); err == nil {
				return floatVal
			}
		}
		return value
	}

	// Coerce number → string for string fields
	// Handles float64 (from JSON unmarshal) and all Go integer types (from TranspileFromMap)
	if c.schema().IsStringType(fieldName) {
		switch v := value.(type) {
		case json.Number:
			return v.String()
		case float64:
			if v == float64(int64(v)) {
				return fmt.Sprintf("%d", int64(v))
			}
			return fmt.Sprintf("%g", v)
		case float32:
			return fmt.Sprintf("%g", v)
		case int:
			return strconv.Itoa(v)
		case int8:
			return strconv.FormatInt(int64(v), 10)
		case int16:
			return strconv.FormatInt(int64(v), 10)
		case int32:
			return strconv.FormatInt(int64(v), 10)
		case int64:
			return strconv.FormatInt(v, 10)
		case uint:
			return strconv.FormatUint(uint64(v), 10)
		case uint8:
			return strconv.FormatUint(uint64(v), 10)
		case uint16:
			return strconv.FormatUint(uint64(v), 10)
		case uint32:
			return strconv.FormatUint(uint64(v), 10)
		case uint64:
			return strconv.FormatUint(v, 10)
		}
		return value
	}

	return value
}

// validateEnumValue validates that a value is valid for an enum field.
// Returns nil if valid or if not an enum field.
func (c *ComparisonOperator) validateEnumValue(value interface{}, fieldName string) error {
	if c.schema() == nil || fieldName == "" {
		return nil
	}

	// Skip validation for null values (null is valid for any field)
	if value == nil {
		return nil
	}

	// Only validate if the field is an enum type
	if !c.schema().IsEnumType(fieldName) {
		return nil
	}

	// Extract string value for validation
	var strVal string
	switch v := value.(type) {
	case string:
		strVal = v
	default:
		// Non-string values for enum comparison - convert to string for validation
		strVal = fmt.Sprintf("%v", v)
	}

	return c.schema().ValidateEnumValue(fieldName, strVal)
}

// ToSQL converts a comparison operator to SQL.
func (c *ComparisonOperator) ToSQL(operator string, args []interface{}) (string, error) {
	// Handle chained comparisons (2+ arguments)
	if len(args) >= 2 && (operator == "<" || operator == "<=" || operator == ">" || operator == ">=") {
		return c.handleChainedComparison(operator, args)
	}

	if len(args) != 2 {
		return "", fmt.Errorf("%s operator requires exactly 2 arguments", operator)
	}

	// Special handling for 'in' operator - right side should be an array
	if operator == "in" {
		leftArg := materializePredicateValueOperand(args[0])
		leftSQL, err := c.valueToSQL(leftArg)
		if err != nil {
			return "", fmt.Errorf("invalid left operand: %w", err)
		}
		// Keep the left operand metadata available for enum validation.
		return c.handleIn(leftSQL, args[1], leftArg)
	}

	// Apply type coercion based on schema
	// If one side is a field and the other is a literal, coerce the literal to match the field type
	leftArg := args[0]
	rightArg := args[1]

	if isEqualityOperator(operator) {
		decision := c.applyEqualitySemantics(operator, leftArg, rightArg)
		if decision.unsupported != nil {
			return "", decision.unsupported
		}
		if decision.constant != nil {
			return boolSQL(*decision.constant), nil
		}
		if decision.handled {
			leftArg = decision.left
			rightArg = decision.right
		} else {
			var err error
			leftArg, rightArg, err = c.applySchemaComparisonCoercion(leftArg, rightArg)
			if err != nil {
				return "", err
			}
		}
	} else {
		var err error
		leftArg, rightArg, err = c.applySchemaComparisonCoercion(leftArg, rightArg)
		if err != nil {
			return "", err
		}
	}
	if isEqualityOperator(operator) {
		leftArg = materializePredicateValueOperand(leftArg)
		rightArg = materializePredicateValueOperand(rightArg)
	}

	leftSQL, err := c.valueToSQL(leftArg)
	if err != nil {
		return "", fmt.Errorf("invalid left operand: %w", err)
	}

	rightSQL, err := c.valueToSQL(rightArg)
	if err != nil {
		return "", fmt.Errorf("invalid right operand: %w", err)
	}

	// Handle NULL comparisons - use IS NULL/IS NOT NULL instead of = NULL/!= NULL
	isLeftNull := args[0] == nil || leftSQL == "NULL"
	isRightNull := args[1] == nil || rightSQL == "NULL"

	if isEqualityOperator(operator) && !isLeftNull && !isRightNull {
		if leftBool, ok := sqlBooleanConstant(leftSQL); ok {
			if rightBool, ok := sqlBooleanConstant(rightSQL); ok {
				return boolSQL(equalityPredicateConstant(operator, leftBool, rightBool)), nil
			}
		}
	}

	if c.shouldUseNullSafeFieldEquality(operator, args[0], args[1]) && !isLeftNull && !isRightNull {
		return nullSafeFieldEqualitySQL(operator, leftSQL, rightSQL), nil
	}

	switch operator {
	case "==":
		// Handle NULL comparisons
		if isLeftNull && isRightNull {
			return "NULL IS NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s = %s", leftSQL, rightSQL), nil
	case "===":
		// Strict equality - same as == but handle NULL
		if isLeftNull && isRightNull {
			return "NULL IS NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s = %s", leftSQL, rightSQL), nil
	case "!=":
		// Handle NULL comparisons
		if isLeftNull && isRightNull {
			return "NULL IS NOT NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NOT NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NOT NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s != %s", leftSQL, rightSQL), nil
	case "!==":
		// Strict inequality - same as != but handle NULL
		if isLeftNull && isRightNull {
			return "NULL IS NOT NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NOT NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NOT NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s <> %s", leftSQL, rightSQL), nil
	case ">", ">=", "<", "<=":
		// Validate operands for ordering comparisons
		if err := c.validateOrderingOperand(args[0], operator); err != nil {
			return "", err
		}
		if err := c.validateOrderingOperand(args[1], operator); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s %s %s", leftSQL, operator, rightSQL), nil
	default:
		return "", fmt.Errorf("unsupported comparison operator: %s", operator)
	}
}

// valueToSQL converts a value to SQL, handling both literals and var expressions.
func (c *ComparisonOperator) valueToSQL(value interface{}) (string, error) {
	// Check if it's a ProcessedValue (pre-processed SQL from parser)
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		// It's a literal, quote it
		return c.dataOp.valueToSQL(pv.Value)
	}

	// Check if it's a var expression
	if varExpr, ok := value.(map[string]interface{}); ok {
		if len(varExpr) == 1 {
			for operator, args := range varExpr {
				if operator == "var" {
					// Special case: empty var name represents the current element in array operations
					if varName, ok := args.(string); ok && varName == "" {
						return "elem", nil
					}
					return c.dataOp.ToSQL(OpVar, []interface{}{args})
				}
			}
		}
	}

	// Handle complex expressions (arithmetic, comparisons, etc.)
	// Note: Array operators (reduce, filter, some, etc.) should be pre-processed by the parser
	// into SQL strings, but if they're not, we'll return an error here
	if expr, ok := value.(map[string]interface{}); ok {
		if len(expr) == 1 {
			for op, args := range expr {
				switch op {
				case "+", "-", "*", "/", "%":
					// Handle arithmetic operations
					return c.processArithmeticExpression(op, args)
				case ">", ">=", "<", "<=", "==", "===", "!=", "!==":
					// Handle comparison operations
					return c.processComparisonExpression(op, args)
				case "max", "min":
					// Handle min/max operations
					return c.processMinMaxExpression(op, args)
				case "if":
					// Handle if operator - delegate to logical operator
					if arr, ok := args.([]interface{}); ok {
						logicalOp := NewLogicalOperator(c.config)
						return logicalOp.ToSQL("if", arr)
					}
					return "", fmt.Errorf("if operator requires array arguments")
				case "reduce", "filter", "map", "some", "all", "none", "merge":
					// Array operators should have been pre-processed by the parser/logical operator
					// If we see them here, it means they weren't processed correctly
					// Try to process them directly as a fallback
					if arr, ok := args.([]interface{}); ok {
						arrayOp := NewArrayOperator(c.config)
						return arrayOp.ToSQL(op, arr)
					}
					return "", fmt.Errorf("array operator %s requires array arguments", op)
				case "cat", "substr":
					// Handle string operators
					if arr, ok := args.([]interface{}); ok {
						stringOp := NewStringOperator(c.config)
						return stringOp.ToSQL(op, arr)
					}
					return "", fmt.Errorf("string operator %s requires array arguments", op)
				default:
					// Try to use the expression parser callback for unknown operators
					// This enables support for custom operators in nested contexts
					if c.config != nil && c.config.HasExpressionParser() {
						return c.config.ParseExpression(expr, "$")
					}
					return "", fmt.Errorf("unsupported expression type in comparison: %s", op)
				}
			}
		}
	}

	// Handle arrays (for 'in' operator)
	if _, ok := value.([]interface{}); ok {
		return "", fmt.Errorf("arrays should be handled by handleIn method")
	}

	// Otherwise treat as literal value
	return c.dataOp.valueToSQL(value)
}

func isSQLStringLiteral(sql string) bool {
	trimmed := strings.TrimSpace(sql)
	return len(trimmed) >= 2 && strings.HasPrefix(trimmed, "'") && strings.HasSuffix(trimmed, "'")
}

func materializePredicateValueOperand(value interface{}) interface{} {
	pv, ok := value.(ProcessedValue)
	if !ok || !pv.IsSQL || !pv.HasExpressionInfo || pv.Kind != ExpressionKindPredicate {
		return value
	}
	pv.Value = PredicateValueSQL(pv.Value)
	pv.Kind = ExpressionKindValue
	pv.Type = ExpressionTypeBoolean
	return pv
}

func valueExpressionOperandSQL(value interface{}) (string, ExpressionType, bool) {
	pv, ok := value.(ProcessedValue)
	if !ok || !pv.IsSQL || !pv.HasExpressionInfo || pv.Kind != ExpressionKindValue {
		return "", ExpressionTypeUnknown, false
	}
	return pv.Value, pv.Type, true
}

func numericOrderingOperand(value interface{}) interface{} {
	if b, ok := value.(bool); ok {
		if b {
			return int64(1)
		}
		return int64(0)
	}
	pv, ok := value.(ProcessedValue)
	if !ok || !pv.IsSQL || !pv.HasExpressionInfo {
		return value
	}
	switch {
	case pv.Kind == ExpressionKindPredicate:
		pv.Value = PredicateNumberSQL(pv.Value)
	case pv.Type == ExpressionTypeBoolean:
		pv.Value = BooleanValueNumberSQL(pv.Value)
	default:
		return value
	}
	pv.Kind = ExpressionKindValue
	pv.Type = ExpressionTypeNumber
	return pv
}

// isStringLikeInOperandNoSchema classifies whether the left operand of "in"
// should be treated as a string in schema-less mode.
//
// This is intentionally heuristic (not a full type system). It covers:
// - literal strings
// - SQL string literals
// - ProcessedValue placeholders bound to string params
// - expressions rooted in known string-producing operators (cat/substr/if branches).
func (c *ComparisonOperator) isStringLikeInOperandNoSchema(
	value interface{},
	pc *params.ParamCollector,
	depth int,
) bool {
	// Keep recursive inference bounded on malformed/hostile input.
	if depth > 20 {
		return false
	}

	switch v := value.(type) {
	case string:
		return true
	case ProcessedValue:
		if v.HasExpressionInfo && v.Kind == ExpressionKindValue && v.Type == ExpressionTypeString {
			return true
		}
		if !v.IsSQL {
			return true
		}
		if isSQLStringLiteral(v.Value) {
			return true
		}
		if pc != nil {
			if paramValue, found := pc.ValueForPlaceholder(strings.TrimSpace(v.Value)); found {
				_, ok := paramValue.(string)
				return ok
			}
		}
		return false
	case map[string]interface{}:
		if len(v) != 1 {
			return false
		}
		for op, args := range v {
			switch op {
			case OpCat:
				arr, ok := args.([]interface{})
				return ok && len(arr) > 0
			case OpSubstr:
				arr, ok := args.([]interface{})
				return ok && len(arr) >= 2 && len(arr) <= 3
			case OpIf:
				arr, ok := args.([]interface{})
				if !ok || len(arr) < 3 {
					return false
				}
				// Branch results are at odd indexes and (for odd-length forms)
				// the trailing else value at the last index.
				for i := 1; i < len(arr); i += 2 {
					if !c.isStringLikeInOperandNoSchema(arr[i], pc, depth+1) {
						return false
					}
				}
				if len(arr)%2 == 1 {
					return c.isStringLikeInOperandNoSchema(arr[len(arr)-1], pc, depth+1)
				}
				return true
			default:
				return false
			}
		}
	}

	return false
}

// handleIn converts in operator to SQL
// leftOriginal is the original left argument (before SQL conversion) for enum validation.
func (c *ComparisonOperator) handleIn(leftSQL string, rightValue, leftOriginal interface{}) (string, error) {
	// Extract field name from left side for enum validation
	leftFieldName := c.extractFieldNameFromValue(leftOriginal)

	// Check if right side is a variable expression
	if varExpr, ok := rightValue.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			// Handle variable on right side
			// According to JSON Logic spec, "in" supports both:
			// 1. Array membership: {"in": [value, array]} → value IN array
			// 2. String containment: {"in": [substring, string]} → substring contained in string
			//
			// Use schema to determine the correct SQL:
			// - If variable is an ARRAY column: 'value' IN column (array membership)
			// - If variable is a STRING column: STRPOS(column, 'value') > 0 (string containment)
			rightSQL, err := c.dataOp.ToSQL(OpVar, []interface{}{varName})
			if err != nil {
				return "", fmt.Errorf("invalid variable in IN operator: %w", err)
			}

			// Extract field name from varName (handle both string and array cases)
			var fieldName string
			if nameStr, ok := varName.(string); ok {
				fieldName = nameStr
			} else if nameArr, ok := varName.([]interface{}); ok && len(nameArr) > 0 {
				if nameStr, ok := nameArr[0].(string); ok {
					fieldName = nameStr
				}
			}

			// Use schema to determine type if available
			if c.schema() != nil && fieldName != "" {
				if c.schema().IsArrayType(fieldName) {
					// Array type: use dialect-specific array membership syntax
					return c.arrayMembershipSQL(leftSQL, rightSQL), nil
				} else if c.schema().IsStringType(fieldName) {
					// Coerce left side literal to string if needed (e.g., 123 → '123')
					coercedLeft := c.coerceValueForComparison(leftOriginal, fieldName)
					coercedLeftSQL, err := c.valueToSQL(coercedLeft)
					if err != nil {
						return "", fmt.Errorf("invalid left operand after coercion: %w", err)
					}
					// String type: use string containment syntax
					return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, coercedLeftSQL)), nil
				}
			}

			// No schema or unknown type: use heuristic based on left operand type.
			// Known string-producing expressions (cat/substr) and string literals
			// use containment; otherwise fall back to array membership.
			if c.isStringLikeInOperandNoSchema(leftOriginal, nil, 0) || isSQLStringLiteral(leftSQL) {
				// Use STRPOS/position for string containment
				return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, leftSQL)), nil
			}
			// Otherwise, assume array membership
			return c.arrayMembershipSQL(leftSQL, rightSQL), nil
		}
	}

	if rightSQL, rightType, ok := valueExpressionOperandSQL(rightValue); ok {
		if rightType == ExpressionTypeString {
			return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, leftSQL)), nil
		}
		if rightType == ExpressionTypeArray {
			return c.arrayMembershipSQL(leftSQL, rightSQL), nil
		}
	}

	// Check if right side is an array
	if arr, ok := rightValue.([]interface{}); ok {
		if len(arr) == 0 {
			return "", fmt.Errorf("in operator array cannot be empty")
		}

		// Validate enum values if left side is an enum field
		if leftFieldName != "" && c.schema() != nil && c.schema().IsEnumType(leftFieldName) {
			for _, item := range arr {
				if err := c.validateEnumValue(item, leftFieldName); err != nil {
					return "", err
				}
			}
		}

		// Apply type coercion based on schema for array elements
		if leftFieldName != "" && c.schema() != nil {
			coerced := make([]interface{}, len(arr))
			copy(coerced, arr)
			arr = coerced
			for i, item := range arr {
				arr[i] = c.coerceValueForComparison(item, leftFieldName)
			}
		}

		// Convert array elements to SQL values
		var values []string
		for _, item := range arr {
			valueSQL, err := c.dataOp.valueToSQL(item)
			if err != nil {
				return "", fmt.Errorf("invalid array element: %w", err)
			}
			values = append(values, valueSQL)
		}

		return fmt.Sprintf("%s IN (%s)", leftSQL, strings.Join(values, ", ")), nil
	}

	if str, ok := rightValue.(string); ok {
		rightSQL, err := c.dataOp.valueToSQL(str)
		if err != nil {
			return "", fmt.Errorf("invalid string in IN operator: %w", err)
		}
		return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, leftSQL)), nil
	}

	if num, ok := rightValue.(float64); ok {
		rightSQL, err := c.dataOp.valueToSQL(num)
		if err != nil {
			return "", fmt.Errorf("invalid number in IN operator: %w", err)
		}
		return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, leftSQL)), nil
	}
	if num, ok := rightValue.(json.Number); ok {
		rightSQL, err := c.dataOp.valueToSQL(num)
		if err != nil {
			return "", fmt.Errorf("invalid number in IN operator: %w", err)
		}
		return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, leftSQL)), nil
	}

	return "", fmt.Errorf("in operator requires array, variable, string, or number as second argument")
}

// handleChainedComparison handles chained comparisons like {"<": [10, {"var": "x"}, 20, 30]}
// For 2 args: generates "a < b"
// For 3+ args: generates "(a < b AND b < c AND c < d)".
func (c *ComparisonOperator) handleChainedComparison(operator string, args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("chained comparison requires at least 2 arguments")
	}

	// Validate operands for ordering comparisons
	for _, arg := range args {
		if err := c.validateOrderingOperand(arg, operator); err != nil {
			return "", err
		}
	}

	// Apply type coercion: find field names and coerce adjacent literals
	coercedArgs := make([]interface{}, len(args))
	copy(coercedArgs, args)

	// Find the field name from any var expression to use for coercion
	var fieldName string
	for _, arg := range args {
		if name := c.extractFieldNameFromValue(arg); name != "" {
			fieldName = name
			break
		}
	}

	// Coerce all literal arguments based on the field type
	if fieldName != "" {
		for i, arg := range coercedArgs {
			if c.extractFieldNameFromValue(arg) == "" {
				coercedArgs[i] = c.coerceValueForComparison(arg, fieldName)
			}
		}
	}
	for i, arg := range coercedArgs {
		coercedArgs[i] = numericOrderingOperand(arg)
	}

	// Convert all arguments to SQL
	var sqlArgs []string
	for i, arg := range coercedArgs {
		argSQL, err := c.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid argument %d: %w", i, err)
		}
		sqlArgs = append(sqlArgs, argSQL)
	}

	// For 2 arguments, return simple comparison without parentheses
	if len(args) == 2 {
		return fmt.Sprintf("%s %s %s", sqlArgs[0], operator, sqlArgs[1]), nil
	}

	// For 3+ arguments, generate chained comparisons with parentheses
	var conditions []string
	for i := 0; i < len(sqlArgs)-1; i++ {
		condition := fmt.Sprintf("%s %s %s", sqlArgs[i], operator, sqlArgs[i+1])
		conditions = append(conditions, condition)
	}

	return fmt.Sprintf("(%s)", strings.Join(conditions, " AND ")), nil
}

// processArithmeticExpression handles arithmetic operations within comparison operations.
func (c *ComparisonOperator) processArithmeticExpression(op string, args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("arithmetic operation requires array of arguments")
	}

	// Handle unary minus (negation) - single argument case
	if op == "-" && len(argsSlice) == 1 {
		operand, err := c.valueToSQL(argsSlice[0])
		if err != nil {
			return "", fmt.Errorf("invalid unary minus argument: %w", err)
		}
		return fmt.Sprintf("(-%s)", operand), nil
	}

	// Handle unary plus (cast to number) - single argument case
	if op == "+" && len(argsSlice) == 1 {
		operand, err := c.valueToSQL(argsSlice[0])
		if err != nil {
			return "", fmt.Errorf("invalid unary plus argument: %w", err)
		}
		return fmt.Sprintf("CAST(%s AS NUMERIC)", operand), nil
	}

	if len(argsSlice) < 2 {
		return "", fmt.Errorf("arithmetic operation requires at least 2 arguments")
	}

	// Convert arguments to SQL
	operands := make([]string, len(argsSlice))
	for i, arg := range argsSlice {
		operand, err := c.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid arithmetic argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	// Generate SQL based on operation
	switch op {
	case "+":
		return fmt.Sprintf("(%s)", strings.Join(operands, " + ")), nil
	case "-":
		return fmt.Sprintf("(%s)", strings.Join(operands, " - ")), nil
	case "*":
		return fmt.Sprintf("(%s)", strings.Join(operands, " * ")), nil
	case "/":
		return fmt.Sprintf("(%s)", strings.Join(operands, " / ")), nil
	case "%":
		return fmt.Sprintf("(%s)", strings.Join(operands, " % ")), nil
	default:
		return "", fmt.Errorf("unsupported arithmetic operation: %s", op)
	}
}

// processComparisonExpression handles comparison operations within comparison operations.
func (c *ComparisonOperator) processComparisonExpression(op string, args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("comparison operation requires array of arguments")
	}

	if len(argsSlice) != 2 {
		return "", fmt.Errorf("comparison operation requires exactly 2 arguments")
	}

	sql, err := c.ToSQL(op, argsSlice)
	if err != nil {
		return "", err
	}
	if _, ok := sqlBooleanConstant(sql); ok {
		return sql, nil
	}
	return fmt.Sprintf("(%s)", sql), nil
}

// processMinMaxExpression handles min/max operations within comparison operations.
func (c *ComparisonOperator) processMinMaxExpression(op string, args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("min/max operation requires array of arguments")
	}

	if len(argsSlice) < 2 {
		return "", fmt.Errorf("min/max operation requires at least 2 arguments")
	}

	// Convert arguments to SQL
	operands := make([]string, len(argsSlice))
	for i, arg := range argsSlice {
		operand, err := c.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid min/max argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	// Generate SQL based on operation
	switch op {
	case "max":
		return fmt.Sprintf("GREATEST(%s)", strings.Join(operands, ", ")), nil
	case "min":
		return fmt.Sprintf("LEAST(%s)", strings.Join(operands, ", ")), nil
	default:
		return "", fmt.Errorf("unsupported min/max operation: %s", op)
	}
}

// ToSQLParam is the parameterized variant of ToSQL. Keep in sync.
func (c *ComparisonOperator) ToSQLParam(operator string, args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) >= 2 && (operator == "<" || operator == "<=" || operator == ">" || operator == ">=") {
		return c.handleChainedComparisonParam(operator, args, pc)
	}

	if len(args) != 2 {
		return "", fmt.Errorf("%s operator requires exactly 2 arguments", operator)
	}

	if operator == "in" {
		return c.handleInParam(materializePredicateValueOperand(args[0]), args[1], pc)
	}

	leftArg := args[0]
	rightArg := args[1]

	if isEqualityOperator(operator) {
		decision := c.applyEqualitySemantics(operator, leftArg, rightArg)
		if decision.unsupported != nil {
			return "", decision.unsupported
		}
		if decision.constant != nil {
			return boolSQL(*decision.constant), nil
		}
		if decision.handled {
			leftArg = decision.left
			rightArg = decision.right
		} else {
			var err error
			leftArg, rightArg, err = c.applySchemaComparisonCoercion(leftArg, rightArg)
			if err != nil {
				return "", err
			}
		}
	} else {
		var err error
		leftArg, rightArg, err = c.applySchemaComparisonCoercion(leftArg, rightArg)
		if err != nil {
			return "", err
		}
	}
	if isEqualityOperator(operator) {
		leftArg = materializePredicateValueOperand(leftArg)
		rightArg = materializePredicateValueOperand(rightArg)
	}

	leftSQL, err := c.valueToSQLParam(leftArg, pc)
	if err != nil {
		return "", fmt.Errorf("invalid left operand: %w", err)
	}

	rightSQL, err := c.valueToSQLParam(rightArg, pc)
	if err != nil {
		return "", fmt.Errorf("invalid right operand: %w", err)
	}

	isLeftNull := args[0] == nil || leftSQL == "NULL"
	isRightNull := args[1] == nil || rightSQL == "NULL"

	if isEqualityOperator(operator) && !isLeftNull && !isRightNull {
		if leftBool, ok := sqlBooleanConstant(leftSQL); ok {
			if rightBool, ok := sqlBooleanConstant(rightSQL); ok {
				return boolSQL(equalityPredicateConstant(operator, leftBool, rightBool)), nil
			}
		}
	}

	if c.shouldUseNullSafeFieldEquality(operator, args[0], args[1]) && !isLeftNull && !isRightNull {
		return nullSafeFieldEqualitySQL(operator, leftSQL, rightSQL), nil
	}

	switch operator {
	case "==":
		if isLeftNull && isRightNull {
			return "NULL IS NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s = %s", leftSQL, rightSQL), nil
	case "===":
		if isLeftNull && isRightNull {
			return "NULL IS NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s = %s", leftSQL, rightSQL), nil
	case "!=":
		if isLeftNull && isRightNull {
			return "NULL IS NOT NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NOT NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NOT NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s != %s", leftSQL, rightSQL), nil
	case "!==":
		if isLeftNull && isRightNull {
			return "NULL IS NOT NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NOT NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NOT NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s <> %s", leftSQL, rightSQL), nil
	case ">", ">=", "<", "<=":
		if err := c.validateOrderingOperand(args[0], operator); err != nil {
			return "", err
		}
		if err := c.validateOrderingOperand(args[1], operator); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s %s %s", leftSQL, operator, rightSQL), nil
	default:
		return "", fmt.Errorf("unsupported comparison operator: %s", operator)
	}
}

// valueToSQLParam is the parameterized variant of valueToSQL. Keep in sync.
func (c *ComparisonOperator) valueToSQLParam(value interface{}, pc *params.ParamCollector) (string, error) {
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		return c.dataOp.valueToSQLParam(pv.Value, pc)
	}

	if varExpr, ok := value.(map[string]interface{}); ok {
		if len(varExpr) == 1 {
			for operator, args := range varExpr {
				if operator == "var" {
					if varName, ok := args.(string); ok && varName == "" {
						return "elem", nil
					}
					return c.dataOp.ToSQLParam(OpVar, []interface{}{args}, pc)
				}
			}
		}
	}

	if expr, ok := value.(map[string]interface{}); ok {
		if len(expr) == 1 {
			for op, args := range expr {
				switch op {
				case "+", "-", "*", "/", "%":
					return c.processArithmeticExpressionParam(op, args, pc)
				case ">", ">=", "<", "<=", "==", "===", "!=", "!==":
					return c.processComparisonExpressionParam(op, args, pc)
				case "max", "min":
					return c.processMinMaxExpressionParam(op, args, pc)
				case "if":
					if arr, ok := args.([]interface{}); ok {
						logicalOp := NewLogicalOperator(c.config)
						return logicalOp.ToSQLParam("if", arr, pc)
					}
					return "", fmt.Errorf("if operator requires array arguments")
				case "reduce", "filter", "map", "some", "all", "none", "merge":
					if arr, ok := args.([]interface{}); ok {
						arrayOp := NewArrayOperator(c.config)
						return arrayOp.ToSQLParam(op, arr, pc)
					}
					return "", fmt.Errorf("array operator %s requires array arguments", op)
				case "cat", "substr":
					if arr, ok := args.([]interface{}); ok {
						stringOp := NewStringOperator(c.config)
						return stringOp.ToSQLParam(op, arr, pc)
					}
					return "", fmt.Errorf("string operator %s requires array arguments", op)
				default:
					if c.config != nil && c.config.HasParamExpressionParser() {
						return c.config.ParseExpressionParam(expr, "$", pc)
					}
					return "", fmt.Errorf("unsupported expression type in comparison: %s", op)
				}
			}
		}
	}

	if _, ok := value.([]interface{}); ok {
		return "", fmt.Errorf("arrays should be handled by handleInParam method")
	}

	return c.dataOp.valueToSQLParam(value, pc)
}

// handleInParam is the parameterized variant of handleIn. Keep in sync.
//
// Unlike handleIn, leftSQL is NOT pre-generated. This method generates the
// placeholder for the left operand after determining whether coercion is needed
// (P2 fix) and uses the original value's Go type rather than SQL quoting to
// distinguish string containment from array membership (P1 fix).
func (c *ComparisonOperator) handleInParam(leftOriginal, rightValue interface{}, pc *params.ParamCollector) (string, error) {
	leftFieldName := c.extractFieldNameFromValue(leftOriginal)

	if varExpr, ok := rightValue.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			rightSQL, err := c.dataOp.ToSQLParam(OpVar, []interface{}{varName}, pc)
			if err != nil {
				return "", fmt.Errorf("invalid variable in IN operator: %w", err)
			}

			var fieldName string
			if nameStr, ok := varName.(string); ok {
				fieldName = nameStr
			} else if nameArr, ok := varName.([]interface{}); ok && len(nameArr) > 0 {
				if nameStr, ok := nameArr[0].(string); ok {
					fieldName = nameStr
				}
			}

			if c.schema() != nil && fieldName != "" {
				if c.schema().IsArrayType(fieldName) {
					leftSQL, lErr := c.valueToSQLParam(leftOriginal, pc)
					if lErr != nil {
						return "", fmt.Errorf("invalid left operand: %w", lErr)
					}
					return c.arrayMembershipSQL(leftSQL, rightSQL), nil
				} else if c.schema().IsStringType(fieldName) {
					coercedLeft := c.coerceValueForComparison(leftOriginal, fieldName)
					coercedLeftSQL, lErr := c.valueToSQLParam(coercedLeft, pc)
					if lErr != nil {
						return "", fmt.Errorf("invalid left operand after coercion: %w", lErr)
					}
					return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, coercedLeftSQL)), nil
				}
			}

			// No schema: infer from left operand shape/type in a heuristic way.
			// This improves string-containment detection for nested string
			// expressions (cat/substr) while still falling back safely.
			isLeftString := c.isStringLikeInOperandNoSchema(leftOriginal, pc, 0)

			leftSQL, err := c.valueToSQLParam(leftOriginal, pc)
			if err != nil {
				return "", fmt.Errorf("invalid left operand: %w", err)
			}
			if isLeftString {
				return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, leftSQL)), nil
			}
			return c.arrayMembershipSQL(leftSQL, rightSQL), nil
		}
	}

	// Generate leftSQL for all remaining paths (array, string, number on right side).
	leftSQL, err := c.valueToSQLParam(leftOriginal, pc)
	if err != nil {
		return "", fmt.Errorf("invalid left operand: %w", err)
	}

	if rightSQL, rightType, ok := valueExpressionOperandSQL(rightValue); ok {
		if rightType == ExpressionTypeString {
			return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, leftSQL)), nil
		}
		if rightType == ExpressionTypeArray {
			return c.arrayMembershipSQL(leftSQL, rightSQL), nil
		}
	}

	if arr, ok := rightValue.([]interface{}); ok {
		if len(arr) == 0 {
			return "", fmt.Errorf("in operator array cannot be empty")
		}

		if leftFieldName != "" && c.schema() != nil && c.schema().IsEnumType(leftFieldName) {
			for _, item := range arr {
				if err := c.validateEnumValue(item, leftFieldName); err != nil {
					return "", err
				}
			}
		}

		if leftFieldName != "" && c.schema() != nil {
			coerced := make([]interface{}, len(arr))
			copy(coerced, arr)
			arr = coerced
			for i, item := range arr {
				arr[i] = c.coerceValueForComparison(item, leftFieldName)
			}
		}

		var values []string
		for _, item := range arr {
			valueSQL, err := c.dataOp.valueToSQLParam(item, pc)
			if err != nil {
				return "", fmt.Errorf("invalid array element: %w", err)
			}
			values = append(values, valueSQL)
		}

		return fmt.Sprintf("%s IN (%s)", leftSQL, strings.Join(values, ", ")), nil
	}

	if str, ok := rightValue.(string); ok {
		rightSQL, err := c.dataOp.valueToSQLParam(str, pc)
		if err != nil {
			return "", fmt.Errorf("invalid string in IN operator: %w", err)
		}
		return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, leftSQL)), nil
	}

	if num, ok := rightValue.(float64); ok {
		rightSQL, err := c.dataOp.valueToSQLParam(num, pc)
		if err != nil {
			return "", fmt.Errorf("invalid number in IN operator: %w", err)
		}
		return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, leftSQL)), nil
	}
	if num, ok := rightValue.(json.Number); ok {
		rightSQL, err := c.dataOp.valueToSQLParam(num, pc)
		if err != nil {
			return "", fmt.Errorf("invalid number in IN operator: %w", err)
		}
		return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, leftSQL)), nil
	}

	return "", fmt.Errorf("in operator requires array, variable, string, or number as second argument")
}

// handleChainedComparisonParam is the parameterized variant of handleChainedComparison. Keep in sync.
func (c *ComparisonOperator) handleChainedComparisonParam(operator string, args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("chained comparison requires at least 2 arguments")
	}

	for _, arg := range args {
		if err := c.validateOrderingOperand(arg, operator); err != nil {
			return "", err
		}
	}

	coercedArgs := make([]interface{}, len(args))
	copy(coercedArgs, args)

	var fieldName string
	for _, arg := range args {
		if name := c.extractFieldNameFromValue(arg); name != "" {
			fieldName = name
			break
		}
	}

	if fieldName != "" {
		for i, arg := range coercedArgs {
			if c.extractFieldNameFromValue(arg) == "" {
				coercedArgs[i] = c.coerceValueForComparison(arg, fieldName)
			}
		}
	}
	for i, arg := range coercedArgs {
		coercedArgs[i] = numericOrderingOperand(arg)
	}

	var sqlArgs []string
	for i, arg := range coercedArgs {
		argSQL, err := c.valueToSQLParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid argument %d: %w", i, err)
		}
		sqlArgs = append(sqlArgs, argSQL)
	}

	if len(args) == 2 {
		return fmt.Sprintf("%s %s %s", sqlArgs[0], operator, sqlArgs[1]), nil
	}

	var conditions []string
	for i := 0; i < len(sqlArgs)-1; i++ {
		condition := fmt.Sprintf("%s %s %s", sqlArgs[i], operator, sqlArgs[i+1])
		conditions = append(conditions, condition)
	}

	return fmt.Sprintf("(%s)", strings.Join(conditions, " AND ")), nil
}

// processArithmeticExpressionParam is the parameterized variant of processArithmeticExpression. Keep in sync.
func (c *ComparisonOperator) processArithmeticExpressionParam(op string, args interface{}, pc *params.ParamCollector) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("arithmetic operation requires array of arguments")
	}

	if op == "-" && len(argsSlice) == 1 {
		operand, err := c.valueToSQLParam(argsSlice[0], pc)
		if err != nil {
			return "", fmt.Errorf("invalid unary minus argument: %w", err)
		}
		return fmt.Sprintf("(-%s)", operand), nil
	}

	if op == "+" && len(argsSlice) == 1 {
		operand, err := c.valueToSQLParam(argsSlice[0], pc)
		if err != nil {
			return "", fmt.Errorf("invalid unary plus argument: %w", err)
		}
		return fmt.Sprintf("CAST(%s AS NUMERIC)", operand), nil
	}

	if len(argsSlice) < 2 {
		return "", fmt.Errorf("arithmetic operation requires at least 2 arguments")
	}

	operands := make([]string, len(argsSlice))
	for i, arg := range argsSlice {
		operand, err := c.valueToSQLParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid arithmetic argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	switch op {
	case "+":
		return fmt.Sprintf("(%s)", strings.Join(operands, " + ")), nil
	case "-":
		return fmt.Sprintf("(%s)", strings.Join(operands, " - ")), nil
	case "*":
		return fmt.Sprintf("(%s)", strings.Join(operands, " * ")), nil
	case "/":
		return fmt.Sprintf("(%s)", strings.Join(operands, " / ")), nil
	case "%":
		return fmt.Sprintf("(%s)", strings.Join(operands, " % ")), nil
	default:
		return "", fmt.Errorf("unsupported arithmetic operation: %s", op)
	}
}

// processComparisonExpressionParam is the parameterized variant of processComparisonExpression. Keep in sync.
func (c *ComparisonOperator) processComparisonExpressionParam(op string, args interface{}, pc *params.ParamCollector) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("comparison operation requires array of arguments")
	}

	if len(argsSlice) != 2 {
		return "", fmt.Errorf("comparison operation requires exactly 2 arguments")
	}

	sql, err := c.ToSQLParam(op, argsSlice, pc)
	if err != nil {
		return "", err
	}
	if _, ok := sqlBooleanConstant(sql); ok {
		return sql, nil
	}
	return fmt.Sprintf("(%s)", sql), nil
}

// processMinMaxExpressionParam is the parameterized variant of processMinMaxExpression. Keep in sync.
func (c *ComparisonOperator) processMinMaxExpressionParam(op string, args interface{}, pc *params.ParamCollector) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("min/max operation requires array of arguments")
	}

	if len(argsSlice) < 2 {
		return "", fmt.Errorf("min/max operation requires at least 2 arguments")
	}

	operands := make([]string, len(argsSlice))
	for i, arg := range argsSlice {
		operand, err := c.valueToSQLParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid min/max argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	switch op {
	case "max":
		return fmt.Sprintf("GREATEST(%s)", strings.Join(operands, ", ")), nil
	case "min":
		return fmt.Sprintf("LEAST(%s)", strings.Join(operands, ", ")), nil
	default:
		return "", fmt.Errorf("unsupported min/max operation: %s", op)
	}
}
