package operators

import (
	"fmt"
	"strconv"
	"strings"
)

// StringOperator handles string operations like cat, substr
type StringOperator struct {
	dataOp *DataOperator
}

// NewStringOperator creates a new StringOperator instance
func NewStringOperator() *StringOperator {
	return &StringOperator{
		dataOp: NewDataOperator(),
	}
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

		// Handle complex expressions (arithmetic, comparisons, etc.)
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
