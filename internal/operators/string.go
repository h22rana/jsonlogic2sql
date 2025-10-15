package operators

import (
	"fmt"
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

	// Third argument: length (optional)
	if len(args) == 3 {
		length, err := s.valueToSQL(args[2])
		if err != nil {
			return "", fmt.Errorf("invalid substring length argument: %v", err)
		}
		// Convert 0-based start to 1-based: start + 1
		return fmt.Sprintf("SUBSTRING(%s, %s + 1, %s)", str, start, length), nil
	}

	// If no length provided, use SUBSTRING without length parameter
	// Convert 0-based start to 1-based: start + 1
	return fmt.Sprintf("SUBSTRING(%s, %s + 1)", str, start), nil
}

// valueToSQL converts a value to SQL, handling var expressions and literals
func (s *StringOperator) valueToSQL(value interface{}) (string, error) {
	// Handle var expressions
	if expr, ok := value.(map[string]interface{}); ok {
		if varExpr, hasVar := expr["var"]; hasVar {
			return s.dataOp.ToSQL("var", []interface{}{varExpr})
		}
	}

	// Handle primitive values
	return s.dataOp.valueToSQL(value)
}
