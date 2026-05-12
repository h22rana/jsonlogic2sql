package operators

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// StringOperator handles string operations like cat, substr.
type StringOperator struct {
	config *OperatorConfig
	dataOp *DataOperator
}

// NewStringOperator creates a new StringOperator instance with optional config.
func NewStringOperator(config *OperatorConfig) *StringOperator {
	return &StringOperator{
		config: config,
		dataOp: NewDataOperator(config),
	}
}

// schema returns the schema from config, or nil if not configured.
func (s *StringOperator) schema() SchemaProvider {
	if s.config == nil {
		return nil
	}
	return s.config.Schema
}

// validateStringOperand checks if a field used in a string operation is of compatible type
// Allows string types and numeric types (implicit conversion is common)
// Rejects array and object types.
func (s *StringOperator) validateStringOperand(value interface{}) error {
	if s.schema() == nil {
		return nil // No schema, no validation
	}

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
	if s.schema().IsArrayType(fieldName) || fieldType == "object" {
		return fmt.Errorf("string operation on incompatible field '%s' (type: %s)", fieldName, fieldType)
	}

	return nil
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
			case "missing", "missing_some", "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in", "!", "!!", OpAll, OpSome, OpNone:
				return ExpressionKindPredicate, ExpressionTypeBoolean
			case "and", "or":
				return ExpressionKindPredicate, ExpressionTypeBoolean
			case "+", "-", "*", "/", "%", "max", "min":
				return ExpressionKindValue, ExpressionTypeNumber
			case "cat", "substr":
				return ExpressionKindValue, ExpressionTypeString
			case "if":
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
	if len(arr)%2 == 0 {
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
	if fieldName == "" || s.schema() == nil {
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
	if len(args) == 0 {
		return "", fmt.Errorf("string operator %s requires at least one argument", operator)
	}

	switch operator {
	case "cat":
		return s.handleConcatenation(args)
	case "substr":
		return s.handleSubstring(args)
	default:
		return "", fmt.Errorf("unsupported string operator: %s", operator)
	}
}

// handleConcatenation converts cat operator to SQL.
func (s *StringOperator) handleConcatenation(args []interface{}) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("concatenation requires at least 1 argument")
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

	// Use CONCAT function for SQL concatenation
	return fmt.Sprintf("CONCAT(%s)", strings.Join(operands, ", ")), nil
}

func (s *StringOperator) valueToSQLForConcat(value interface{}) (string, error) {
	sql, err := s.valueToSQL(value)
	if err != nil {
		return "", err
	}
	kind, typ := s.inferExpressionShape(value)
	return s.stringifyConcatSQL(sql, kind, typ), nil
}

func (s *StringOperator) stringifyConcatSQL(sql string, kind ExpressionKind, typ ExpressionType) string {
	if kind == ExpressionKindPredicate || typ == ExpressionTypeBoolean {
		return s.booleanToStringSQL(sql)
	}
	return StripRedundantOuterParens(sql)
}

func (s *StringOperator) booleanToStringSQL(sql string) string {
	switch strings.ToUpper(strings.TrimSpace(sql)) {
	case "TRUE":
		return "'true'"
	case "FALSE":
		return "'false'"
	}
	condition := StripRedundantOuterParens(sql)
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(condition)), "CASE ") {
		condition = fmt.Sprintf("(%s)", condition)
	}
	return fmt.Sprintf("CASE WHEN %s THEN 'true' ELSE 'false' END", condition)
}

// handleSubstring converts substr operator to SQL.
func (s *StringOperator) handleSubstring(args []interface{}) (string, error) {
	if len(args) < 2 || len(args) > 3 {
		return "", fmt.Errorf("substring requires 2 or 3 arguments")
	}

	// Validate first argument type (string source)
	if err := s.validateStringOperand(args[0]); err != nil {
		return "", err
	}

	// First argument: string
	str, err := s.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid substring string argument: %w", err)
	}

	// Second argument: start position (convert from 0-based to 1-based)
	start, err := s.valueToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid substring start argument: %w", err)
	}

	// Convert 0-based start to 1-based, handling numeric literals cleanly
	startSQL := s.convertStartIndex(start)

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
		length, err := s.valueToSQL(args[2])
		if err != nil {
			return "", fmt.Errorf("invalid substring length argument: %w", err)
		}
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
		if varExpr, hasVar := expr[OpVar]; hasVar {
			return s.dataOp.ToSQL(OpVar, []interface{}{varExpr})
		}

		// Handle complex expressions (arithmetic, comparisons, conditionals, etc.)
		// This is a simplified approach - in a full implementation, you'd want
		// to delegate to the appropriate operator based on the expression type
		if len(expr) == 1 {
			for op, args := range expr {
				switch op {
				case "+", "-", "*", "/", "%":
					// Handle arithmetic operations
					return s.processArithmeticExpression(op, args)
				case ">", ">=", "<", "<=", "==", "===", "!=", "!==":
					// Handle comparison operations
					return s.processComparisonExpression(op, args)
				case "if":
					// Handle conditional expressions
					return s.processIfExpression(args)
				case "substr":
					// Handle nested substr operations
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("substr requires array of arguments")
					}
					return s.handleSubstring(argsSlice)
				case "cat":
					// Handle nested cat operations
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("cat requires array of arguments")
					}
					return s.handleConcatenation(argsSlice)
				case "max", "min":
					// Handle max/min operations
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("%s requires array of arguments", op)
					}
					return s.processMaxMinExpression(op, argsSlice)
				case "and", "or":
					// Handle logical operations
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("%s requires array of arguments", op)
					}
					return s.processLogicalExpression(op, argsSlice)
				case "!":
					// Handle NOT operation
					return s.processNotExpression(args)
				case "!!":
					// Handle boolean coercion
					return s.processBooleanCoercion(args)
				default:
					// Try to use the expression parser callback for unknown operators
					// This enables support for custom operators in nested contexts
					if s.config != nil && s.config.HasExpressionParser() {
						return s.config.ParseExpression(expr, "$")
					}
					return "", fmt.Errorf("unsupported expression type in string operation: %s", op)
				}
			}
		}
	}

	// Handle primitive values
	return s.dataOp.valueToSQL(value)
}

// convertStartIndex converts a 0-based start index to 1-based for SQL SUBSTR
// Handles numeric literals cleanly (e.g., "0" becomes "1", "5" becomes "6")
// For complex expressions, adds "+ 1" (e.g., "x" becomes "x + 1").
func (s *StringOperator) convertStartIndex(start string) string {
	// Try to parse as integer for clean conversion
	if num, err := strconv.Atoi(start); err == nil {
		// It's a simple integer, convert directly
		return strconv.Itoa(num + 1)
	}

	// It's a complex expression (variable, arithmetic, etc.), add "+ 1"
	return fmt.Sprintf("(%s + 1)", start)
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

	sqlOp := " AND "
	if op == "or" {
		sqlOp = " OR "
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
func (s *StringOperator) ToSQLParam(operator string, args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("string operator %s requires at least one argument", operator)
	}
	switch operator {
	case "cat":
		return s.handleConcatenationParam(args, pc)
	case "substr":
		return s.handleSubstringParam(args, pc)
	default:
		return "", fmt.Errorf("unsupported string operator: %s", operator)
	}
}

// handleConcatenationParam is the parameterized variant of handleConcatenation. Keep in sync.
func (s *StringOperator) handleConcatenationParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("concatenation requires at least 1 argument")
	}
	for _, arg := range args {
		if err := s.validateStringOperand(arg); err != nil {
			return "", err
		}
	}
	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := s.valueToSQLForConcatParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid concatenation argument %d: %w", i, err)
		}
		operands[i] = operand
	}
	return fmt.Sprintf("CONCAT(%s)", strings.Join(operands, ", ")), nil
}

func (s *StringOperator) valueToSQLForConcatParam(value interface{}, pc *params.ParamCollector) (string, error) {
	sql, err := s.valueToSQLParam(value, pc)
	if err != nil {
		return "", err
	}
	kind, typ := s.inferExpressionShape(value)
	return s.stringifyConcatSQL(sql, kind, typ), nil
}

// handleSubstringParam is the parameterized variant of handleSubstring. Keep in sync.
func (s *StringOperator) handleSubstringParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) < 2 || len(args) > 3 {
		return "", fmt.Errorf("substring requires 2 or 3 arguments")
	}
	if err := s.validateStringOperand(args[0]); err != nil {
		return "", err
	}
	str, err := s.valueToSQLParam(args[0], pc)
	if err != nil {
		return "", fmt.Errorf("invalid substring string argument: %w", err)
	}
	start, err := s.valueToSQLParam(args[1], pc)
	if err != nil {
		return "", fmt.Errorf("invalid substring start argument: %w", err)
	}
	startSQL := s.convertStartIndex(start)

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

	if len(args) == 3 {
		length, err := s.valueToSQLParam(args[2], pc)
		if err != nil {
			return "", fmt.Errorf("invalid substring length argument: %w", err)
		}
		return fmt.Sprintf("%s(%s, %s, %s)", substrFunc, str, startSQL, length), nil
	}
	return fmt.Sprintf("%s(%s, %s)", substrFunc, str, startSQL), nil
}

// valueToSQLParam is the parameterized variant of valueToSQL. Keep in sync.
func (s *StringOperator) valueToSQLParam(value interface{}, pc *params.ParamCollector) (string, error) {
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		return s.dataOp.valueToSQLParam(pv.Value, pc)
	}

	if expr, ok := value.(map[string]interface{}); ok {
		if varExpr, hasVar := expr[OpVar]; hasVar {
			return s.dataOp.ToSQLParam(OpVar, []interface{}{varExpr}, pc)
		}

		if len(expr) == 1 {
			for op, args := range expr {
				switch op {
				case "+", "-", "*", "/", "%":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("arithmetic operation requires array of arguments")
					}
					numOp := NewNumericOperator(s.config)
					return numOp.ToSQLParam(op, argsSlice, pc)
				case ">", ">=", "<", "<=", "==", "===", "!=", "!==":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("comparison operation requires array of arguments")
					}
					compOp := NewComparisonOperator(s.config)
					sql, err := compOp.ToSQLParam(op, argsSlice, pc)
					if err != nil {
						return "", err
					}
					return parenthesizeComparisonSQL(sql), nil
				case "if":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("if operation requires array of arguments")
					}
					logOp := NewLogicalOperator(s.config)
					return logOp.ToSQLParam("if", argsSlice, pc)
				case "substr":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("substr requires array of arguments")
					}
					return s.handleSubstringParam(argsSlice, pc)
				case "cat":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("cat requires array of arguments")
					}
					return s.handleConcatenationParam(argsSlice, pc)
				case "max", "min":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("%s requires array of arguments", op)
					}
					numOp := NewNumericOperator(s.config)
					return numOp.ToSQLParam(op, argsSlice, pc)
				case "and", "or":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("%s requires array of arguments", op)
					}
					logOp := NewLogicalOperator(s.config)
					return logOp.ToSQLParam(op, argsSlice, pc)
				case "!":
					argsSlice, ok := args.([]interface{})
					if ok {
						logOp := NewLogicalOperator(s.config)
						return logOp.ToSQLParam("!", argsSlice, pc)
					}
					logOp := NewLogicalOperator(s.config)
					return logOp.ToSQLParam("!", []interface{}{args}, pc)
				case "!!":
					argsSlice, ok := args.([]interface{})
					if ok {
						logOp := NewLogicalOperator(s.config)
						return logOp.ToSQLParam("!!", argsSlice, pc)
					}
					logOp := NewLogicalOperator(s.config)
					return logOp.ToSQLParam("!!", []interface{}{args}, pc)
				default:
					if s.config != nil && s.config.HasParamExpressionParser() {
						return s.config.ParseExpressionParam(expr, "$", pc)
					}
					return "", fmt.Errorf("unsupported expression type in string operation: %s", op)
				}
			}
		}
	}

	return s.dataOp.valueToSQLParam(value, pc)
}
