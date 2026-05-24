package operators

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

// StringOperator handles JSONLogic string operators.
type StringOperator struct {
	config *OperatorConfig
	dataOp *DataOperator
}

// NewStringOperator creates a new StringOperator instance.
func NewStringOperator(config *OperatorConfig) *StringOperator {
	config = normalizeOperatorConfig(config)
	return &StringOperator{
		config: config,
		dataOp: NewDataOperator(config),
	}
}

func (s *StringOperator) schema() SchemaProvider {
	return schemaFromConfig(s.config)
}

// validateStringOperand checks if a field used in a string operation is of compatible type
// Allows string types and numeric types (implicit conversion is common)
// Rejects array and object types.
func (s *StringOperator) validateStringOperand(value interface{}) error {
	fieldName := s.extractFieldNameFromValue(value)
	if fieldName == "" {
		return nil // Can't determine field name, skip validation
	}

	fieldType := s.schema().GetFieldType(fieldName)
	if fieldType == "" {
		return nil // Field not in schema, skip validation (existence checked by DataOperator)
	}

	// Allow string and numeric types (implicit conversion is common)
	if s.schema().IsStringType(fieldName) || s.schema().IsNumericType(fieldName) {
		return nil
	}

	// Disallow array and object types
	if s.schema().IsArrayType(fieldName) || fieldType == objectFieldType {
		return fmt.Errorf("string operation on incompatible field '%s' (type: %s)", fieldName, fieldType)
	}

	return nil
}

func (s *StringOperator) validateSubstringSourceOperand(value interface{}) error {
	if err := s.validateStringOperand(value); err != nil {
		return err
	}
	kind, typ := s.inferExpressionShape(value)
	if kind == ExpressionKindPredicate {
		return fmt.Errorf("substring source argument must be string or number, got predicate")
	}
	switch typ {
	case ExpressionTypeString, ExpressionTypeNumber, ExpressionTypeUnknown:
		return nil
	case ExpressionTypeBoolean, ExpressionTypeNull, ExpressionTypeArray, ExpressionTypeObject:
		return fmt.Errorf("substring source argument must be string or number, got %s", expressionTypeName(typ))
	default:
		return nil
	}
}

func (s *StringOperator) validateSubstringIndexOperand(value interface{}, name string) error {
	kind, typ := s.inferExpressionShape(value)
	if kind == ExpressionKindPredicate {
		return fmt.Errorf("substring %s argument must be numeric, got predicate", name)
	}
	switch typ {
	case ExpressionTypeNumber, ExpressionTypeUnknown:
		return nil
	case ExpressionTypeString, ExpressionTypeBoolean, ExpressionTypeNull, ExpressionTypeArray, ExpressionTypeObject:
		return fmt.Errorf("substring %s argument must be numeric, got %s", name, expressionTypeName(typ))
	default:
		return nil
	}
}

// extractFieldNameFromValue extracts field name from a value that might be a var expression.
func (s *StringOperator) extractFieldNameFromValue(value interface{}) string {
	if pv, ok := value.(ProcessedValue); ok && pv.IsSQL && pv.IsField {
		return pv.FieldName
	}
	if varExpr, ok := value.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			return s.extractFieldName(varName)
		}
	}
	return ""
}

// extractFieldName extracts the field name from a var argument.
func (s *StringOperator) extractFieldName(varName interface{}) string {
	if pv, ok := varName.(ProcessedValue); ok && pv.IsSQL && pv.IsField {
		return pv.FieldName
	}
	if nameStr, ok := varName.(string); ok {
		return nameStr
	}
	if nameArr, ok := varName.([]interface{}); ok && len(nameArr) > 0 {
		if pv, ok := nameArr[0].(ProcessedValue); ok && pv.IsSQL && pv.IsField {
			return pv.FieldName
		}
		if nameStr, ok := nameArr[0].(string); ok {
			return nameStr
		}
	}
	return ""
}

func (s *StringOperator) inferExpressionShape(value interface{}) (ExpressionKind, ExpressionType) {
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			if pv.HasExpressionInfo {
				return pv.Kind, pv.Type
			}
			return ExpressionKindValue, ExpressionTypeUnknown
		}
		return s.inferExpressionShape(pv.Value)
	}

	if typ, ok := primitiveExpressionType(value); ok {
		return ExpressionKindValue, typ
	}

	if expr, ok := value.(map[string]interface{}); ok && len(expr) == 1 {
		for op, args := range expr {
			switch op {
			case OpVar:
				return ExpressionKindValue, s.varExpressionType(args)
			case OpMissing, OpMissingSome,
				OpEqual, OpStrictEqual, OpNotEqual, OpStrictNotEqual,
				OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual,
				OpIn, OpNot, OpDoubleBang,
				OpAll, OpSome, OpNone:
				return ExpressionKindPredicate, ExpressionTypeBoolean
			case OpAnd, OpOr:
				return ExpressionKindPredicate, ExpressionTypeBoolean
			case OpAdd, OpSubtract, OpMultiply, OpDivide, OpModulo, OpMax, OpMin:
				return ExpressionKindValue, ExpressionTypeNumber
			case OpCat, OpSubstr:
				return ExpressionKindValue, ExpressionTypeString
			case OpIf:
				return ExpressionKindValue, s.inferIfExpressionType(args)
			}
		}
	}

	return ExpressionKindValue, ExpressionTypeUnknown
}

func (s *StringOperator) inferIfExpressionType(args interface{}) ExpressionType {
	arr, ok := args.([]interface{})
	if !ok || len(arr) < 2 {
		return ExpressionTypeUnknown
	}

	var result ExpressionType
	hasResult := false
	for i := 1; i < len(arr); i += 2 {
		_, typ := s.inferExpressionShape(arr[i])
		result = mergeInferredTypes(result, typ, hasResult)
		hasResult = true
	}
	if len(arr)%2 == 1 {
		_, typ := s.inferExpressionShape(arr[len(arr)-1])
		result = mergeInferredTypes(result, typ, hasResult)
		hasResult = true
	} else {
		result = mergeInferredTypes(result, ExpressionTypeNull, hasResult)
		hasResult = true
	}
	if !hasResult {
		return ExpressionTypeUnknown
	}
	return result
}

func mergeInferredTypes(current, next ExpressionType, hasCurrent bool) ExpressionType {
	if !hasCurrent {
		return next
	}
	if current == ExpressionTypeNull {
		return next
	}
	if next == ExpressionTypeNull {
		return current
	}
	if current == next {
		return current
	}
	return ExpressionTypeUnknown
}

func primitiveExpressionType(value interface{}) (ExpressionType, bool) {
	switch value.(type) {
	case string:
		return ExpressionTypeString, true
	case bool:
		return ExpressionTypeBoolean, true
	case nil:
		return ExpressionTypeNull, true
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, json.Number:
		return ExpressionTypeNumber, true
	default:
		return ExpressionTypeUnknown, false
	}
}

func (s *StringOperator) varExpressionType(args interface{}) ExpressionType {
	fieldName := s.extractFieldName(args)
	if fieldName == "" {
		return ExpressionTypeUnknown
	}
	switch {
	case s.schema().IsBooleanType(fieldName):
		return ExpressionTypeBoolean
	case s.schema().IsStringType(fieldName), s.schema().IsEnumType(fieldName):
		return ExpressionTypeString
	case s.schema().IsNumericType(fieldName):
		return ExpressionTypeNumber
	case s.schema().IsArrayType(fieldName):
		return ExpressionTypeArray
	default:
		return ExpressionTypeUnknown
	}
}

// ToSQL converts a string operation to SQL.
func (s *StringOperator) ToSQL(operator string, args []interface{}) (string, error) {
	switch operator {
	case OpCat:
		return s.handleConcatenation(args)
	case OpSubstr:
		if len(args) == 0 {
			return "", fmt.Errorf("string operator %s requires at least one argument", operator)
		}
		return s.handleSubstring(args)
	default:
		return "", fmt.Errorf("unsupported string operator: %s", operator)
	}
}

// handleConcatenation converts cat operator to SQL.
func (s *StringOperator) handleConcatenation(args []interface{}) (string, error) {
	if len(args) == 0 {
		return "''", nil
	}

	// Validate operand types
	for _, arg := range args {
		if err := s.validateStringOperand(arg); err != nil {
			return "", err
		}
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := s.valueToSQLForConcat(arg)
		if err != nil {
			return "", fmt.Errorf("invalid concatenation argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	return s.config.ConcatSQL(operands), nil
}

func (s *StringOperator) valueToSQLForConcat(value interface{}) (string, error) {
	if sql, handled, err := s.ifExpressionToConcatSQL(value); handled || err != nil {
		return sql, err
	}
	sql, err := s.valueToSQL(value)
	if err != nil {
		return "", err
	}
	if literalSQL, ok := s.literalConcatSQL(value, sql); ok {
		return literalSQL, nil
	}
	kind, typ := s.inferExpressionShape(value)
	return s.stringifyConcatSQL(sql, kind, typ), nil
}

func (s *StringOperator) literalConcatSQL(value interface{}, sql string) (string, bool) {
	typ, ok := primitiveExpressionType(value)
	if !ok {
		return "", false
	}
	expr := StripRedundantOuterParens(sql)
	switch typ {
	case ExpressionTypeNull:
		return "''", true
	case ExpressionTypeString:
		return expr, true
	case ExpressionTypeNumber:
		return s.config.StringCast(expr), true
	case ExpressionTypeBoolean:
		return PredicateStringSQL(expr), true
	case ExpressionTypeArray, ExpressionTypeObject, ExpressionTypeUnknown:
		return "", false
	default:
		return "", false
	}
}

func ifExpressionArgs(value interface{}) ([]interface{}, bool, error) {
	expr, ok := value.(map[string]interface{})
	if !ok || len(expr) != 1 {
		return nil, false, nil
	}
	args, ok := expr[OpIf]
	if !ok {
		return nil, false, nil
	}
	arr, ok := args.([]interface{})
	if !ok {
		return nil, true, fmt.Errorf("if operation requires array of arguments")
	}
	return arr, true, nil
}

func (s *StringOperator) ifExpressionToConcatSQL(value interface{}) (string, bool, error) {
	args, handled, err := ifExpressionArgs(value)
	if !handled || err != nil {
		return "", handled, err
	}
	sql, err := s.processStringifiedIfExpression(args)
	return sql, true, err
}

func (s *StringOperator) processStringifiedIfExpression(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("if operation requires at least 2 arguments (condition, then)")
	}

	var result strings.Builder
	result.WriteString("CASE")

	pairLimit := len(args)
	hasElse := len(args)%2 == 1
	if hasElse {
		pairLimit = len(args) - 1
	}

	for i := 0; i < pairLimit; i += 2 {
		condition, err := s.valueToSQL(args[i])
		if err != nil {
			return "", fmt.Errorf("invalid if condition: %w", err)
		}

		thenValue, err := s.valueToSQLForConcat(args[i+1])
		if err != nil {
			return "", fmt.Errorf("invalid if then value: %w", err)
		}

		result.WriteString(fmt.Sprintf(" WHEN %s THEN %s", condition, thenValue))
	}

	if hasElse {
		elseValue, err := s.valueToSQLForConcat(args[len(args)-1])
		if err != nil {
			return "", fmt.Errorf("invalid if else value: %w", err)
		}
		result.WriteString(fmt.Sprintf(" ELSE %s", elseValue))
	} else {
		result.WriteString(" ELSE ''")
	}

	result.WriteString(" END")
	return result.String(), nil
}

func (s *StringOperator) stringifyConcatSQL(sql string, kind ExpressionKind, typ ExpressionType) string {
	return ConcatStringSQL(s.config, sql, kind, typ)
}

// handleSubstring converts substr operator to SQL.
func (s *StringOperator) handleSubstring(args []interface{}) (string, error) {
	if len(args) < 2 || len(args) > 3 {
		return "", fmt.Errorf("substring requires 2 or 3 arguments")
	}

	// Validate first argument type (string source)
	if err := s.validateSubstringSourceOperand(args[0]); err != nil {
		return "", err
	}

	// First argument: string
	str, err := s.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid substring string argument: %w", err)
	}

	if validationErr := s.validateSubstringIndexOperand(args[1], "start"); validationErr != nil {
		return "", validationErr
	}

	// Second argument: start position (convert from 0-based to 1-based)
	start, err := s.valueToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid substring start argument: %w", err)
	}

	startSQL := s.substringStartSQL(args[1], str, start, false)

	// Get the function name based on dialect
	d := dialect.DialectUnspecified
	if s.config != nil {
		d = s.config.GetDialect()
	}

	var substrFunc string
	switch d {
	case dialect.DialectClickHouse:
		substrFunc = "substring"
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		substrFunc = "SUBSTR"
	}

	// Third argument: length (optional)
	if len(args) == 3 {
		if err := s.validateSubstringIndexOperand(args[2], "length"); err != nil {
			return "", err
		}
		length, err := s.valueToSQL(args[2])
		if err != nil {
			return "", fmt.Errorf("invalid substring length argument: %w", err)
		}
		length = s.substringLengthSQL(args[1], args[2], str, start, length, false)
		return fmt.Sprintf("%s(%s, %s, %s)", substrFunc, str, startSQL, length), nil
	}

	// If no length provided, use SUBSTR/substring without length parameter
	return fmt.Sprintf("%s(%s, %s)", substrFunc, str, startSQL), nil
}

// valueToSQL converts a value to SQL, handling var expressions, complex expressions, and literals.
func (s *StringOperator) valueToSQL(value interface{}) (string, error) {
	// Handle ProcessedValue (pre-processed SQL from parser)
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		// It's a literal, convert it
		return s.dataOp.valueToSQL(pv.Value)
	}

	// Handle var expressions
	if expr, ok := value.(map[string]interface{}); ok {
		if len(expr) != 1 {
			return "", fmt.Errorf("operator object must have exactly one key")
		}
		if varExpr, hasVar := expr[OpVar]; hasVar {
			return s.dataOp.ToSQL(OpVar, []interface{}{varExpr})
		}

		// Handle complex expressions (arithmetic, comparisons, conditionals, etc.)
		// This is a simplified approach - in a full implementation, you'd want
		// to delegate to the appropriate operator based on the expression type
		if len(expr) == 1 {
			for op, args := range expr {
				switch op {
				case OpAdd, OpSubtract, OpMultiply, OpDivide, OpModulo:
					// Handle arithmetic operations
					return s.processArithmeticExpression(op, args)
				case OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual,
					OpEqual, OpStrictEqual, OpNotEqual, OpStrictNotEqual:
					// Handle comparison operations
					return s.processComparisonExpression(op, args)
				case OpIf:
					// Handle conditional expressions
					return s.processIfExpression(args)
				case OpSubstr:
					// Handle nested substr operations
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("substr requires array of arguments")
					}
					return s.handleSubstring(argsSlice)
				case OpCat:
					// Handle nested cat operations
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("cat requires array of arguments")
					}
					return s.handleConcatenation(argsSlice)
				case OpMax, OpMin:
					// Handle max/min operations
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("%s requires array of arguments", op)
					}
					return s.processMaxMinExpression(op, argsSlice)
				case OpAnd, OpOr:
					// Handle logical operations
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("%s requires array of arguments", op)
					}
					return s.processLogicalExpression(op, argsSlice)
				case OpNot:
					// Handle NOT operation
					return s.processNotExpression(args)
				case OpDoubleBang:
					// Handle boolean coercion
					return s.processBooleanCoercion(args)
				default:
					// Try to use the expression parser callback for unknown operators
					// This enables support for custom operators in nested contexts
					if s.config != nil && s.config.HasExpressionParser() {
						return s.config.ParseExpression(expr, jsonPathRoot)
					}
					return "", fmt.Errorf("unsupported expression type in string operation: %s", op)
				}
			}
		}
	}

	// Handle primitive values
	return s.dataOp.valueToSQL(value)
}

func (s *StringOperator) substringStartSQL(rawStart interface{}, str, start string, parameterized bool) string {
	if num, ok := integerLiteralValue(rawStart); ok {
		if num >= 0 {
			if parameterized {
				return fmt.Sprintf("(%s + 1)", start)
			}
			return strconv.Itoa(num + 1)
		}
		startTerm := strconv.Itoa(num)
		if parameterized {
			startTerm = start
		}
		return s.config.GreatestSQL([]string{fmt.Sprintf("(%s + %s + 1)", s.stringLengthSQL(str), startTerm), "1"})
	}
	return fmt.Sprintf(
		"(CASE WHEN %s < 0 THEN %s ELSE (%s + 1) END)",
		start,
		s.config.GreatestSQL([]string{fmt.Sprintf("(%s + %s + 1)", s.stringLengthSQL(str), start), "1"}),
		start,
	)
}

func (s *StringOperator) substringLengthSQL(
	rawStart, rawLength interface{},
	str, start, length string,
	parameterized bool,
) string {
	if num, ok := integerLiteralValue(rawLength); ok && num >= 0 {
		if parameterized {
			return length
		}
		return strconv.Itoa(num)
	}

	startZero := s.substringStartZeroSQL(rawStart, str, start, parameterized)
	negativeLength := s.config.GreatestSQL([]string{
		fmt.Sprintf("(%s - %s)", s.config.GreatestSQL([]string{fmt.Sprintf("(%s + %s)", s.stringLengthSQL(str), length), "0"}), startZero),
		"0",
	})
	if num, ok := integerLiteralValue(rawLength); ok && num < 0 {
		return negativeLength
	}
	return fmt.Sprintf("(CASE WHEN %s < 0 THEN %s ELSE %s END)", length, negativeLength, length)
}

func (s *StringOperator) substringStartZeroSQL(rawStart interface{}, str, start string, parameterized bool) string {
	if num, ok := integerLiteralValue(rawStart); ok {
		if num >= 0 {
			if parameterized {
				return start
			}
			return strconv.Itoa(num)
		}
		startTerm := strconv.Itoa(num)
		if parameterized {
			startTerm = start
		}
		return s.config.GreatestSQL([]string{fmt.Sprintf("(%s + %s)", s.stringLengthSQL(str), startTerm), "0"})
	}
	return fmt.Sprintf(
		"(CASE WHEN %s < 0 THEN %s ELSE %s END)",
		start,
		s.config.GreatestSQL([]string{fmt.Sprintf("(%s + %s)", s.stringLengthSQL(str), start), "0"}),
		start,
	)
}

func (s *StringOperator) stringLengthSQL(str string) string {
	if s.config != nil && s.config.GetDialect() == dialect.DialectClickHouse {
		return fmt.Sprintf("length(%s)", str)
	}
	return fmt.Sprintf("LENGTH(%s)", str)
}

func integerLiteralValue(value interface{}) (int, bool) {
	const maxIntValue = int64(1<<(strconv.IntSize-1) - 1)
	const minIntValue = -maxIntValue - 1

	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return 0, false
		}
		return integerLiteralValue(pv.Value)
	}
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		if v >= minIntValue && v <= maxIntValue {
			return int(v), true
		}
	case uint:
		i, err := strconv.Atoi(strconv.FormatUint(uint64(v), 10))
		return i, err == nil
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		i, err := strconv.Atoi(strconv.FormatUint(uint64(v), 10))
		return i, err == nil
	case uint64:
		i, err := strconv.Atoi(strconv.FormatUint(v, 10))
		return i, err == nil
	case float32:
		f := float64(v)
		if !math.IsInf(f, 0) && !math.IsNaN(f) &&
			f >= float64(minIntValue) && f <= float64(maxIntValue) &&
			math.Trunc(f) == f {
			return int(f), true
		}
	case float64:
		if !math.IsInf(v, 0) && !math.IsNaN(v) &&
			v >= float64(minIntValue) && v <= float64(maxIntValue) &&
			math.Trunc(v) == v {
			return int(v), true
		}
	case json.Number:
		i, err := strconv.Atoi(v.String())
		return i, err == nil
	}
	return 0, false
}

// processArithmeticExpression handles arithmetic operations within string operations.
func (s *StringOperator) processArithmeticExpression(op string, args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("arithmetic operation requires array of arguments")
	}

	// Handle unary minus (negation) - single argument case
	if op == "-" && len(argsSlice) == 1 {
		operand, err := s.valueToSQL(argsSlice[0])
		if err != nil {
			return "", fmt.Errorf("invalid unary minus argument: %w", err)
		}
		return fmt.Sprintf("(-%s)", operand), nil
	}

	// Handle unary plus (cast to number) - single argument case
	if op == "+" && len(argsSlice) == 1 {
		operand, err := s.valueToSQL(argsSlice[0])
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
		operand, err := s.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid arithmetic argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	// Generate SQL based on operation
	switch op {
	case OpAdd:
		return fmt.Sprintf("(%s)", strings.Join(operands, " + ")), nil
	case OpSubtract:
		return fmt.Sprintf("(%s)", strings.Join(operands, " - ")), nil
	case OpMultiply:
		return fmt.Sprintf("(%s)", strings.Join(operands, " * ")), nil
	case OpDivide:
		return fmt.Sprintf("(%s)", strings.Join(operands, " / ")), nil
	case OpModulo:
		if len(operands) != 2 {
			return "", fmt.Errorf("modulo requires exactly 2 arguments")
		}
		return s.config.ModuloSQL(operands[0], operands[1]), nil
	default:
		return "", fmt.Errorf("unsupported arithmetic operation: %s", op)
	}
}

// processIfExpression handles if/then/else expressions within string operations.
func (s *StringOperator) processIfExpression(args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("if operation requires array of arguments")
	}

	if len(argsSlice) < 2 {
		return "", fmt.Errorf("if operation requires at least 2 arguments (condition, then)")
	}

	// Build CASE WHEN expression
	var result strings.Builder
	result.WriteString("CASE")

	// Process condition/then pairs
	i := 0
	for i < len(argsSlice)-1 {
		// Condition
		condition, err := s.valueToSQL(argsSlice[i])
		if err != nil {
			return "", fmt.Errorf("invalid if condition: %w", err)
		}

		// Then value
		thenValue, err := s.valueToSQL(argsSlice[i+1])
		if err != nil {
			return "", fmt.Errorf("invalid if then value: %w", err)
		}

		result.WriteString(fmt.Sprintf(" WHEN %s THEN %s", condition, thenValue))
		i += 2

		// Check if there are more condition/then pairs or just an else
		if i < len(argsSlice)-1 {
			// More pairs to process
			continue
		} else if i < len(argsSlice) {
			// Last element is the else value
			elseValue, err := s.valueToSQL(argsSlice[i])
			if err != nil {
				return "", fmt.Errorf("invalid if else value: %w", err)
			}
			result.WriteString(fmt.Sprintf(" ELSE %s", elseValue))
			break
		}
	}

	result.WriteString(" END")
	return result.String(), nil
}

func parenthesizeComparisonSQL(sql string) string {
	if _, ok := sqlBooleanConstant(sql); ok {
		return sql
	}
	return fmt.Sprintf("(%s)", sql)
}

// processComparisonExpression handles comparison operations within string operations.
func (s *StringOperator) processComparisonExpression(op string, args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("comparison operation requires array of arguments")
	}
	compOp := NewComparisonOperator(s.config)
	sql, err := compOp.ToSQL(op, argsSlice)
	if err != nil {
		return "", err
	}
	return parenthesizeComparisonSQL(sql), nil
}

// processMaxMinExpression handles max/min operations within string operations.
func (s *StringOperator) processMaxMinExpression(op string, args []interface{}) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("%s requires at least 1 argument", op)
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := s.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid %s argument %d: %w", op, i, err)
		}
		operands[i] = StripRedundantOuterParens(operand)
	}

	funcName := "GREATEST"
	if op == "min" {
		funcName = "LEAST"
	}

	return fmt.Sprintf("%s(%s)", funcName, strings.Join(operands, ", ")), nil
}

// processLogicalExpression handles and/or operations within string operations.
func (s *StringOperator) processLogicalExpression(op string, args []interface{}) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("%s requires at least 1 argument", op)
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := s.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid %s argument %d: %w", op, i, err)
		}
		if op != "and" || !isOrExpression(arg) {
			operand = StripRedundantOuterParens(operand)
		}
		operands[i] = operand
	}

	sqlOp := sqlAndJoiner
	if op == "or" {
		sqlOp = sqlOrJoiner
	}

	return fmt.Sprintf("(%s)", strings.Join(operands, sqlOp)), nil
}

func isOrExpression(arg interface{}) bool {
	expr, ok := arg.(map[string]interface{})
	if !ok || len(expr) != 1 {
		return false
	}
	_, ok = expr["or"]
	return ok
}

// processNotExpression handles NOT (!) operation within string operations.
func (s *StringOperator) processNotExpression(args interface{}) (string, error) {
	// Handle single value or array with single element
	var value interface{}
	if argsSlice, ok := args.([]interface{}); ok {
		if len(argsSlice) != 1 {
			return "", fmt.Errorf("NOT operation requires exactly 1 argument")
		}
		value = argsSlice[0]
	} else {
		value = args
	}

	operand, err := s.valueToSQL(value)
	if err != nil {
		return "", fmt.Errorf("invalid NOT argument: %w", err)
	}

	return fmt.Sprintf("NOT (%s)", operand), nil
}

// processBooleanCoercion handles boolean coercion (!!) within string operations.
func (s *StringOperator) processBooleanCoercion(args interface{}) (string, error) {
	// Handle single value or array with single element
	var value interface{}
	if argsSlice, ok := args.([]interface{}); ok {
		if len(argsSlice) != 1 {
			return "", fmt.Errorf("boolean coercion requires exactly 1 argument")
		}
		value = argsSlice[0]
	} else {
		value = args
	}

	operand, err := s.valueToSQL(value)
	if err != nil {
		return "", fmt.Errorf("invalid boolean coercion argument: %w", err)
	}

	// Boolean coercion: check if value is truthy
	return fmt.Sprintf("(%s IS NOT NULL AND %s != FALSE AND %s != 0 AND %s != '')", operand, operand, operand, operand), nil
}

// ToSQLParam is the parameterized variant of ToSQL. Keep in sync.
