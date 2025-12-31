package operators

import (
	"fmt"
	"strconv"
	"strings"
)

// StringOperator handles string operations like cat, substr
type StringOperator struct {
	dataOp *DataOperator
	schema SchemaProvider
}

// NewStringOperator creates a new StringOperator instance
func NewStringOperator() *StringOperator {
	return &StringOperator{
		dataOp: NewDataOperator(),
	}
}

// SetSchema sets the schema provider for type validation
func (s *StringOperator) SetSchema(schema SchemaProvider) {
	s.schema = schema
	if s.dataOp != nil {
		s.dataOp.SetSchema(schema)
	}
}

// validateStringOperand checks if a field used in a string operation is of compatible type
// Allows string types and numeric types (implicit conversion is common)
// Rejects array and object types
func (s *StringOperator) validateStringOperand(value interface{}) error {
	if s.schema == nil {
		return nil // No schema, no validation
	}

	fieldName := s.extractFieldNameFromValue(value)
	if fieldName == "" {
		return nil // Can't determine field name, skip validation
	}

	fieldType := s.schema.GetFieldType(fieldName)
	if fieldType == "" {
		return nil // Field not in schema, skip validation (existence checked by DataOperator)
	}

	// Allow string and numeric types (implicit conversion is common)
	if s.schema.IsStringType(fieldName) || s.schema.IsNumericType(fieldName) {
		return nil
	}

	// Disallow array and object types
	if s.schema.IsArrayType(fieldName) || fieldType == "object" {
		return fmt.Errorf("string operation on incompatible field '%s' (type: %s)", fieldName, fieldType)
	}

	return nil
}

// extractFieldNameFromValue extracts field name from a value that might be a var expression
func (s *StringOperator) extractFieldNameFromValue(value interface{}) string {
	if varExpr, ok := value.(map[string]interface{}); ok {
		if varName, hasVar := varExpr["var"]; hasVar {
			return s.extractFieldName(varName)
		}
	}
	return ""
}

// extractFieldName extracts the field name from a var argument
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

// ToSQL converts a string operation to SQL
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

// handleConcatenation converts cat operator to SQL
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
		operand, err := s.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid concatenation argument %d: %v", i, err)
		}
		operands[i] = operand
	}

	// Use CONCAT function for SQL concatenation
	return fmt.Sprintf("CONCAT(%s)", strings.Join(operands, ", ")), nil
}

// handleSubstring converts substr operator to SQL
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
		return "", fmt.Errorf("invalid substring string argument: %v", err)
	}

	// Second argument: start position (convert from 0-based to 1-based)
	start, err := s.valueToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid substring start argument: %v", err)
	}

	// Convert 0-based start to 1-based, handling numeric literals cleanly
	startSQL := s.convertStartIndex(start)

	// Third argument: length (optional)
	if len(args) == 3 {
		length, err := s.valueToSQL(args[2])
		if err != nil {
			return "", fmt.Errorf("invalid substring length argument: %v", err)
		}
		return fmt.Sprintf("SUBSTR(%s, %s, %s)", str, startSQL, length), nil
	}

	// If no length provided, use SUBSTR without length parameter
	return fmt.Sprintf("SUBSTR(%s, %s)", str, startSQL), nil
}

// valueToSQL converts a value to SQL, handling var expressions, complex expressions, and literals
func (s *StringOperator) valueToSQL(value interface{}) (string, error) {
	// Handle var expressions
	if expr, ok := value.(map[string]interface{}); ok {
		if varExpr, hasVar := expr["var"]; hasVar {
			return s.dataOp.ToSQL("var", []interface{}{varExpr})
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
				default:
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
// For complex expressions, adds "+ 1" (e.g., "x" becomes "x + 1")
func (s *StringOperator) convertStartIndex(start string) string {
	// Try to parse as integer for clean conversion
	if num, err := strconv.Atoi(start); err == nil {
		// It's a simple integer, convert directly
		return strconv.Itoa(num + 1)
	}

	// It's a complex expression (variable, arithmetic, etc.), add "+ 1"
	return fmt.Sprintf("(%s + 1)", start)
}

// processArithmeticExpression handles arithmetic operations within string operations
func (s *StringOperator) processArithmeticExpression(op string, args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("arithmetic operation requires array of arguments")
	}

	if len(argsSlice) < 2 {
		return "", fmt.Errorf("arithmetic operation requires at least 2 arguments")
	}

	// Convert arguments to SQL
	operands := make([]string, len(argsSlice))
	for i, arg := range argsSlice {
		operand, err := s.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid arithmetic argument %d: %v", i, err)
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

// processIfExpression handles if/then/else expressions within string operations
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
			return "", fmt.Errorf("invalid if condition: %v", err)
		}

		// Then value
		thenValue, err := s.valueToSQL(argsSlice[i+1])
		if err != nil {
			return "", fmt.Errorf("invalid if then value: %v", err)
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
				return "", fmt.Errorf("invalid if else value: %v", err)
			}
			result.WriteString(fmt.Sprintf(" ELSE %s", elseValue))
			break
		}
	}

	result.WriteString(" END")
	return result.String(), nil
}

// processComparisonExpression handles comparison operations within string operations
func (s *StringOperator) processComparisonExpression(op string, args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("comparison operation requires array of arguments")
	}

	if len(argsSlice) != 2 {
		return "", fmt.Errorf("comparison operation requires exactly 2 arguments")
	}

	// Convert arguments to SQL
	left, err := s.valueToSQL(argsSlice[0])
	if err != nil {
		return "", fmt.Errorf("invalid comparison left argument: %v", err)
	}

	right, err := s.valueToSQL(argsSlice[1])
	if err != nil {
		return "", fmt.Errorf("invalid comparison right argument: %v", err)
	}

	// Generate SQL based on operation
	switch op {
	case ">":
		return fmt.Sprintf("(%s > %s)", left, right), nil
	case ">=":
		return fmt.Sprintf("(%s >= %s)", left, right), nil
	case "<":
		return fmt.Sprintf("(%s < %s)", left, right), nil
	case "<=":
		return fmt.Sprintf("(%s <= %s)", left, right), nil
	case "==":
		return fmt.Sprintf("(%s = %s)", left, right), nil
	case "===":
		return fmt.Sprintf("(%s = %s)", left, right), nil
	case "!=":
		return fmt.Sprintf("(%s != %s)", left, right), nil
	case "!==":
		return fmt.Sprintf("(%s <> %s)", left, right), nil
	default:
		return "", fmt.Errorf("unsupported comparison operation: %s", op)
	}
}
