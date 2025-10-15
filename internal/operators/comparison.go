package operators

import (
	"fmt"
	"strings"
)

// ComparisonOperator handles comparison operators (==, ===, !=, !==, >, >=, <, <=)
type ComparisonOperator struct {
	dataOp *DataOperator
}

// NewComparisonOperator creates a new comparison operator
func NewComparisonOperator() *ComparisonOperator {
	return &ComparisonOperator{
		dataOp: NewDataOperator(),
	}
}

// ToSQL converts a comparison operator to SQL
func (c *ComparisonOperator) ToSQL(operator string, args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("%s operator requires exactly 2 arguments", operator)
	}

	// Special handling for 'in' operator - right side should be an array
	if operator == "in" {
		leftSQL, err := c.valueToSQL(args[0])
		if err != nil {
			return "", fmt.Errorf("invalid left operand: %v", err)
		}
		return c.handleIn(leftSQL, args[1])
	}

	leftSQL, err := c.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid left operand: %v", err)
	}

	rightSQL, err := c.valueToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid right operand: %v", err)
	}

	switch operator {
	case "==":
		return fmt.Sprintf("%s = %s", leftSQL, rightSQL), nil
	case "===":
		// Strict equality - in SQL this is the same as regular equality
		// but we could add type checking if needed
		return fmt.Sprintf("%s = %s", leftSQL, rightSQL), nil
	case "!=":
		return fmt.Sprintf("%s <> %s", leftSQL, rightSQL), nil
	case "!==":
		// Strict inequality - in SQL this is the same as regular inequality
		return fmt.Sprintf("%s <> %s", leftSQL, rightSQL), nil
	case ">":
		return fmt.Sprintf("%s > %s", leftSQL, rightSQL), nil
	case ">=":
		return fmt.Sprintf("%s >= %s", leftSQL, rightSQL), nil
	case "<":
		return fmt.Sprintf("%s < %s", leftSQL, rightSQL), nil
	case "<=":
		return fmt.Sprintf("%s <= %s", leftSQL, rightSQL), nil
	default:
		return "", fmt.Errorf("unsupported comparison operator: %s", operator)
	}
}

// valueToSQL converts a value to SQL, handling both literals and var expressions
func (c *ComparisonOperator) valueToSQL(value interface{}) (string, error) {
	// Check if it's a var expression
	if varExpr, ok := value.(map[string]interface{}); ok {
		if len(varExpr) == 1 {
			for operator, args := range varExpr {
				if operator == "var" {
					return c.dataOp.ToSQL("var", []interface{}{args})
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

// handleIn converts in operator to SQL
func (c *ComparisonOperator) handleIn(leftSQL string, rightValue interface{}) (string, error) {
	// The right side should be an array
	arr, ok := rightValue.([]interface{})
	if !ok {
		return "", fmt.Errorf("in operator requires array as second argument")
	}

	if len(arr) == 0 {
		return "", fmt.Errorf("in operator array cannot be empty")
	}

	// Convert array elements to SQL values
	var values []string
	for _, item := range arr {
		valueSQL, err := c.dataOp.valueToSQL(item)
		if err != nil {
			return "", fmt.Errorf("invalid array element: %v", err)
		}
		values = append(values, valueSQL)
	}

	return fmt.Sprintf("%s IN (%s)", leftSQL, strings.Join(values, ", ")), nil
}
