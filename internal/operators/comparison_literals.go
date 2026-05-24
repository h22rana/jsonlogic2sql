package operators

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// FoldLiteralComparison returns the JSONLogic result for comparisons whose
// operands are fully known literals. The boolean return after the result tells
// callers whether folding was possible.
func FoldLiteralComparison(operator string, args []interface{}) (bool, bool, error) {
	if len(args) != 2 {
		return false, false, nil
	}

	switch operator {
	case OpEqual, OpStrictEqual, OpNotEqual, OpStrictNotEqual:
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
		if operator == OpNotEqual || operator == OpStrictNotEqual {
			equal = !equal
		}
		return equal, true, nil
	case OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual:
		return foldLiteralOrderingComparison(operator, args[0], args[1])
	case OpIn:
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
			case OpGreaterThan:
				return cmp > 0, true, nil
			case OpGreaterThanOrEqual:
				return cmp >= 0, true, nil
			case OpLessThan:
				return cmp < 0, true, nil
			case OpLessThanOrEqual:
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
	case OpGreaterThan:
		return leftNumber.float > rightNumber.float, true, nil
	case OpGreaterThanOrEqual:
		return leftNumber.float >= rightNumber.float, true, nil
	case OpLessThan:
		return leftNumber.float < rightNumber.float, true, nil
	case OpLessThanOrEqual:
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
	if right, ok := rightArg.(string); ok {
		leftString, ok, err := jsonLogicInStringNeedleLiteral(leftArg)
		if !ok || err != nil {
			return false, ok, err
		}
		return strings.Contains(right, leftString), true, nil
	}

	left, leftOK := equalityLiteralValue(leftArg)
	if !leftOK {
		return false, false, nil
	}
	if equalityLiteralKind(left) == "" {
		return false, false, nil
	}

	switch right := rightArg.(type) {
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
		nonContainer, err := nonContainerInHaystackLiteral(rightArg)
		if err != nil {
			return false, false, err
		}
		if nonContainer {
			return false, true, nil
		}
		return false, false, nil
	}
}

func nonContainerInHaystackLiteral(value interface{}) (bool, error) {
	switch v := value.(type) {
	case nil, bool:
		return true, nil
	case json.Number:
		_, err := normalizeJSONNumberLiteral(v)
		return true, err
	case float32, float64:
		return true, ValidateFiniteNativeFloat(v)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true, nil
	default:
		return false, nil
	}
}

func combineOrPredicates(predicates []string) string {
	switch len(predicates) {
	case 0:
		return boolSQL(false)
	case 1:
		return predicates[0]
	default:
		return fmt.Sprintf("(%s)", strings.Join(predicates, sqlOrJoiner))
	}
}

func combineAndPredicates(predicates []string) string {
	filtered := make([]string, 0, len(predicates))
	for _, predicate := range predicates {
		if strings.TrimSpace(predicate) != "" {
			filtered = append(filtered, predicate)
		}
	}
	switch len(filtered) {
	case 0:
		return boolSQL(true)
	case 1:
		return filtered[0]
	default:
		return fmt.Sprintf("(%s)", strings.Join(filtered, sqlAndJoiner))
	}
}

func boolSQL(value bool) string {
	if value {
		return sqlTrue
	}
	return sqlFalse
}

func sqlBooleanConstant(sql string) (bool, bool) {
	s := strings.TrimSpace(sql)
	if len(s) >= 2 && s[0] == '(' && s[len(s)-1] == ')' {
		inner := strings.TrimSpace(s[1 : len(s)-1])
		if inner == sqlTrue || inner == sqlFalse {
			s = inner
		}
	}

	switch s {
	case sqlTrue:
		return true, true
	case sqlFalse:
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
		return literalKindNull
	case string:
		return literalKindString
	case bool:
		return literalKindBoolean
	case json.Number, float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return literalKindNumber
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
	case literalKindNull:
		return true
	case literalKindString:
		leftValue, leftOK := left.(string)
		rightValue, rightOK := right.(string)
		return leftOK && rightOK && leftValue == rightValue
	case literalKindBoolean:
		leftValue, leftOK := left.(bool)
		rightValue, rightOK := right.(bool)
		return leftOK && rightOK && leftValue == rightValue
	case literalKindNumber:
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
	if leftKind == literalKindNull || rightKind == literalKindNull {
		return leftKind == literalKindNull && rightKind == literalKindNull
	}
	if leftKind == rightKind {
		return equalityLiteralsStrictEqual(left, right)
	}
	if leftKind == literalKindBoolean || rightKind == literalKindBoolean ||
		(leftKind == literalKindNumber && rightKind == literalKindString) ||
		(leftKind == literalKindString && rightKind == literalKindNumber) {
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
