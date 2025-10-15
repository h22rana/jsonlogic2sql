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

	// !! converts to boolean - in SQL this is typically done with CASE WHEN
	return fmt.Sprintf("CASE WHEN %s THEN TRUE ELSE FALSE END", condition), nil
}

// handleIf converts if operator to SQL
func (l *LogicalOperator) handleIf(args []interface{}) (string, error) {
	if len(args) < 2 || len(args) > 3 {
		return "", fmt.Errorf("if operator requires 2 or 3 arguments")
	}

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
			default:
				return "", fmt.Errorf("unsupported operator in logical expression: %s", operator)
			}
		}
	}

	return "", fmt.Errorf("invalid expression type: %T", expr)
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
