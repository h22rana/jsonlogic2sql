package operators

import (
	"fmt"
	"strings"
)

// LogicalOperator handles logical operators (and, or, !, !!, if).
type LogicalOperator struct {
	config       *OperatorConfig
	comparisonOp *ComparisonOperator
	dataOp       *DataOperator
}

// NewLogicalOperator creates a new logical operator with optional config.
func NewLogicalOperator(config *OperatorConfig) *LogicalOperator {
	return &LogicalOperator{
		config:       config,
		comparisonOp: NewComparisonOperator(config),
		dataOp:       NewDataOperator(config),
	}
}

// ToSQL converts a logical operator to SQL.
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

// handleAnd converts and operator to SQL.
func (l *LogicalOperator) handleAnd(args []interface{}) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("and operator requires at least 1 argument")
	}

	var conditions []string
	for i, arg := range args {
		condition, err := l.expressionToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid and argument %d: %w", i, err)
		}
		conditions = append(conditions, condition)
	}

	if len(conditions) == 1 {
		return conditions[0], nil
	}

	return fmt.Sprintf("(%s)", strings.Join(conditions, " AND ")), nil
}

// handleOr converts or operator to SQL.
func (l *LogicalOperator) handleOr(args []interface{}) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("or operator requires at least 1 argument")
	}

	var conditions []string
	for i, arg := range args {
		condition, err := l.expressionToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid or argument %d: %w", i, err)
		}
		conditions = append(conditions, condition)
	}

	if len(conditions) == 1 {
		return conditions[0], nil
	}

	return fmt.Sprintf("(%s)", strings.Join(conditions, " OR ")), nil
}

// handleNot converts ! operator to SQL.
func (l *LogicalOperator) handleNot(args []interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("! operator requires exactly 1 argument")
	}

	condition, err := l.expressionToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid ! argument: %w", err)
	}

	return fmt.Sprintf("NOT (%s)", condition), nil
}

// handleDoubleNot converts !! operator to SQL (boolean conversion).
func (l *LogicalOperator) handleDoubleNot(args []interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("!! operator requires exactly 1 argument")
	}

	// Check if the argument is an array (for array length checking)
	if arr, ok := args[0].([]interface{}); ok {
		// For arrays, check if they are non-empty
		// This would typically use CARDINALITY or ARRAY_LENGTH depending on the SQL dialect
		// Using a generic approach that works with most databases
		if len(arr) == 0 {
			return "FALSE", nil
		}
		return "TRUE", nil
	}

	condition, err := l.expressionToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid !! argument: %w", err)
	}

	// !! converts to boolean - check for non-null/truthy values
	// This checks for non-null, non-false, non-zero, non-empty string
	return fmt.Sprintf("(%s IS NOT NULL AND %s != FALSE AND %s != 0 AND %s != '')",
		condition, condition, condition, condition), nil
}

// handleIf converts if operator to SQL.
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
				return "", fmt.Errorf("invalid if condition %d: %w", i/2, err)
			}

			value, err := l.expressionToSQL(args[i+1])
			if err != nil {
				return "", fmt.Errorf("invalid if value %d: %w", i/2, err)
			}

			caseParts = append(caseParts, fmt.Sprintf("WHEN %s THEN %s", condition, value))
		}

		// Handle final else value
		elseValue, err := l.expressionToSQL(args[len(args)-1])
		if err != nil {
			return "", fmt.Errorf("invalid if else value: %w", err)
		}

		return fmt.Sprintf("CASE %s ELSE %s END", strings.Join(caseParts, " "), elseValue), nil
	}

	// Handle simple IF (2-3 arguments)
	// Convert condition
	condition, err := l.expressionToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid if condition: %w", err)
	}

	// Convert then value
	thenValue, err := l.expressionToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid if then value: %w", err)
	}

	// Handle else value (optional)
	if len(args) == 3 {
		elseValue, err := l.expressionToSQL(args[2])
		if err != nil {
			return "", fmt.Errorf("invalid if else value: %w", err)
		}
		return fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, thenValue, elseValue), nil
	}

	// No else value - use NULL
	return fmt.Sprintf("CASE WHEN %s THEN %s ELSE NULL END", condition, thenValue), nil
}

// expressionToSQL converts any expression to SQL.
func (l *LogicalOperator) expressionToSQL(expr interface{}) (string, error) {
	// Check if it's a ProcessedValue (pre-processed SQL from parser)
	if pv, ok := expr.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		// It's a literal, quote it
		return l.dataOp.valueToSQL(pv.Value)
	}

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
			case "var", "missing":
				return l.dataOp.ToSQL(operator, []interface{}{args})
			case "missing_some":
				// missing_some expects args to be an array [minCount, [varNames]]
				if arr, ok := args.([]interface{}); ok {
					return l.dataOp.ToSQL(operator, arr)
				}
				return "", fmt.Errorf("missing_some operator requires array arguments")
			case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
				if arr, ok := args.([]interface{}); ok {
					// Process arguments to handle complex nested expressions
					processedArgs, err := l.processArgs(arr)
					if err != nil {
						return "", fmt.Errorf("failed to process comparison arguments: %w", err)
					}
					return l.comparisonOp.ToSQL(operator, processedArgs)
				}
				return "", fmt.Errorf("comparison operator requires array arguments")
			case "and", "or", "if":
				if arr, ok := args.([]interface{}); ok {
					return l.ToSQL(operator, arr)
				}
				return "", fmt.Errorf("logical operator requires array arguments")
			case "!", "!!":
				// Allow both array and non-array arguments for unary operators
				if arr, ok := args.([]interface{}); ok {
					return l.ToSQL(operator, arr)
				}
				// Wrap non-array argument in array for consistency
				return l.ToSQL(operator, []interface{}{args})
			case "+", "-", "*", "/", "%", "max", "min":
				if arr, ok := args.([]interface{}); ok {
					numericOp := NewNumericOperator(l.config)
					return numericOp.ToSQL(operator, arr)
				}
				return "", fmt.Errorf("numeric operator requires array arguments")
			case "cat", "substr":
				if arr, ok := args.([]interface{}); ok {
					stringOp := NewStringOperator(l.config)
					return stringOp.ToSQL(operator, arr)
				}
				return "", fmt.Errorf("string operator requires array arguments")
			case "map", "filter", "reduce", "all", "some", "none", "merge":
				if arr, ok := args.([]interface{}); ok {
					arrayOp := NewArrayOperator(l.config)
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

// processArgs recursively processes arguments to handle complex expressions
// This converts nested operators (like reduce, filter, etc.) to SQL strings.
func (l *LogicalOperator) processArgs(args []interface{}) ([]interface{}, error) {
	processed := make([]interface{}, len(args))

	for i, arg := range args {
		// If it's a complex expression, convert it to SQL
		if exprMap, ok := arg.(map[string]interface{}); ok {
			// Check if it's a complex expression (not just a var)
			if len(exprMap) == 1 {
				for operator := range exprMap {
					if operator != "var" {
						// It's a complex expression, convert it to SQL
						sql, err := l.expressionToSQL(arg)
						if err != nil {
							return nil, fmt.Errorf("invalid argument %d: %w", i, err)
						}
						// Store the SQL as a string for the operator to use
						processed[i] = sql
						continue
					}
				}
			}
			// For var expressions, keep as is
			processed[i] = arg
			continue
		}
		// For simple expressions or primitives, keep as is
		processed[i] = arg
	}

	return processed, nil
}

// isPrimitive checks if a value is a primitive type.
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
