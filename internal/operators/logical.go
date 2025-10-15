package operators

import (
	"fmt"
	"strings"
)

// LogicalOperator handles logical operators (and, or, !, !!, if)
type LogicalOperator struct {
	comparisonOp *ComparisonOperator
	dataOp       *DataOperator
}

// NewLogicalOperator creates a new logical operator
func NewLogicalOperator() *LogicalOperator {
	return &LogicalOperator{
		comparisonOp: NewComparisonOperator(),
		dataOp:       NewDataOperator(),
	}
}

// ToSQL converts a logical operator to SQL
func (l *LogicalOperator) ToSQL(operator string, args []interface{}) (string, error) {
	switch operator {
	case "and":
		return l.handleAnd(args)
	case "or":
		return l.handleOr(args)
	case "!":
		return l.handleNot(args)
	case "!!":
		return l.handleDoubleNot(args)
	case "if":
		return l.handleIf(args)
	case "?:":
		return l.handleTernary(args)
	default:
		return "", fmt.Errorf("unsupported logical operator: %s", operator)
	}
}

// handleAnd converts and operator to SQL
func (l *LogicalOperator) handleAnd(args []interface{}) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("and operator requires at least 1 argument")
	}

	var conditions []string
	for i, arg := range args {
		condition, err := l.expressionToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid and argument %d: %v", i, err)
		}
		conditions = append(conditions, condition)
	}

	if len(conditions) == 1 {
		return conditions[0], nil
	}

	return fmt.Sprintf("(%s)", strings.Join(conditions, " AND ")), nil
}

// handleOr converts or operator to SQL
func (l *LogicalOperator) handleOr(args []interface{}) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("or operator requires at least 1 argument")
	}

	var conditions []string
	for i, arg := range args {
		condition, err := l.expressionToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid or argument %d: %v", i, err)
		}
		conditions = append(conditions, condition)
	}

	if len(conditions) == 1 {
		return conditions[0], nil
	}

	return fmt.Sprintf("(%s)", strings.Join(conditions, " OR ")), nil
}

// handleNot converts ! operator to SQL
func (l *LogicalOperator) handleNot(args []interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("! operator requires exactly 1 argument")
	}

	condition, err := l.expressionToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid ! argument: %v", err)
	}

	return fmt.Sprintf("NOT (%s)", condition), nil
}

// handleDoubleNot converts !! operator to SQL (boolean conversion)
func (l *LogicalOperator) handleDoubleNot(args []interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("!! operator requires exactly 1 argument")
	}

	condition, err := l.expressionToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid !! argument: %v", err)
	}

	// !! converts to boolean - check for non-null/truthy values
	// This checks for non-null, non-false, non-zero, non-empty string
	return fmt.Sprintf("(%s IS NOT NULL AND %s != FALSE AND %s != 0 AND %s != '')",
		condition, condition, condition, condition), nil
}

// handleIf converts if operator to SQL
func (l *LogicalOperator) handleIf(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("if requires at least 2 arguments")
	}

	// Handle nested IF statements (multiple condition/value pairs)
	if len(args) > 3 && len(args)%2 == 1 {
		// Odd number of arguments means we have multiple condition/value pairs + final else
		var caseParts []string

		// Process condition/value pairs
		for i := 0; i < len(args)-1; i += 2 {
			condition, err := l.expressionToSQL(args[i])
			if err != nil {
				return "", fmt.Errorf("invalid if condition %d: %v", i/2, err)
			}

			value, err := l.expressionToSQL(args[i+1])
			if err != nil {
				return "", fmt.Errorf("invalid if value %d: %v", i/2, err)
			}

			caseParts = append(caseParts, fmt.Sprintf("WHEN %s THEN %s", condition, value))
		}

		// Handle final else value
		elseValue, err := l.expressionToSQL(args[len(args)-1])
		if err != nil {
			return "", fmt.Errorf("invalid if else value: %v", err)
		}

		return fmt.Sprintf("CASE %s ELSE %s END", strings.Join(caseParts, " "), elseValue), nil
	}

	// Handle simple IF (2-3 arguments)
	// Convert condition
	condition, err := l.expressionToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid if condition: %v", err)
	}

	// Convert then value
	thenValue, err := l.expressionToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid if then value: %v", err)
	}

	// Handle else value (optional)
	if len(args) == 3 {
		elseValue, err := l.expressionToSQL(args[2])
		if err != nil {
			return "", fmt.Errorf("invalid if else value: %v", err)
		}
		return fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, thenValue, elseValue), nil
	}

	// No else value - use NULL
	return fmt.Sprintf("CASE WHEN %s THEN %s ELSE NULL END", condition, thenValue), nil
}

// expressionToSQL converts any expression to SQL
func (l *LogicalOperator) expressionToSQL(expr interface{}) (string, error) {
	// Handle primitive values
	if l.isPrimitive(expr) {
		return l.dataOp.valueToSQL(expr)
	}

	// Handle arrays (should not happen in logical context, but handle gracefully)
	if _, ok := expr.([]interface{}); ok {
		return "", fmt.Errorf("arrays not supported in logical expressions")
	}

	// Handle objects (operators)
	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return "", fmt.Errorf("operator object must have exactly one key")
		}

		for operator, args := range obj {
			// Handle different operator types
			switch operator {
			case "var", "missing", "missing_some":
				return l.dataOp.ToSQL(operator, []interface{}{args})
			case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
				if arr, ok := args.([]interface{}); ok {
					return l.comparisonOp.ToSQL(operator, arr)
				}
				return "", fmt.Errorf("comparison operator requires array arguments")
			case "and", "or", "!", "!!", "if":
				if arr, ok := args.([]interface{}); ok {
					return l.ToSQL(operator, arr)
				}
				return "", fmt.Errorf("logical operator requires array arguments")
			case "+", "-", "*", "/", "%", "max", "min":
				if arr, ok := args.([]interface{}); ok {
					numericOp := NewNumericOperator()
					return numericOp.ToSQL(operator, arr)
				}
				return "", fmt.Errorf("numeric operator requires array arguments")
			case "cat", "substr":
				if arr, ok := args.([]interface{}); ok {
					stringOp := NewStringOperator()
					return stringOp.ToSQL(operator, arr)
				}
				return "", fmt.Errorf("string operator requires array arguments")
			case "map", "filter", "reduce", "all", "some", "none", "merge":
				if arr, ok := args.([]interface{}); ok {
					arrayOp := NewArrayOperator()
					return arrayOp.ToSQL(operator, arr)
				}
				return "", fmt.Errorf("array operator requires array arguments")
			default:
				return "", fmt.Errorf("unsupported operator in logical expression: %s", operator)
			}
		}
	}

	return "", fmt.Errorf("invalid expression type: %T", expr)
}

// handleTernary converts ternary operator to SQL CASE WHEN statement
func (l *LogicalOperator) handleTernary(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("ternary operator requires at least 2 arguments")
	}

	// For 2 arguments: condition ? true_value : NULL
	if len(args) == 2 {
		condition, err := l.expressionToSQL(args[0])
		if err != nil {
			return "", fmt.Errorf("invalid condition: %v", err)
		}

		trueValue, err := l.expressionToSQL(args[1])
		if err != nil {
			return "", fmt.Errorf("invalid true value: %v", err)
		}

		return fmt.Sprintf("CASE WHEN %s THEN %s ELSE NULL END", condition, trueValue), nil
	}

	// For 3+ arguments: condition ? true_value : false_value
	condition, err := l.expressionToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid condition: %v", err)
	}

	trueValue, err := l.expressionToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid true value: %v", err)
	}

	falseValue, err := l.expressionToSQL(args[2])
	if err != nil {
		return "", fmt.Errorf("invalid false value: %v", err)
	}

	return fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, trueValue, falseValue), nil
}

// isPrimitive checks if a value is a primitive type
func (l *LogicalOperator) isPrimitive(value interface{}) bool {
	switch value.(type) {
	case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		return true
	case nil:
		return true
	default:
		return false
	}
}
