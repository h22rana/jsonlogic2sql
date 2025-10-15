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
	// Handle chained comparisons (2+ arguments)
	if len(args) >= 2 && (operator == "<" || operator == "<=" || operator == ">" || operator == ">=") {
		return c.handleChainedComparison(operator, args)
	}

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
		return fmt.Sprintf("%s != %s", leftSQL, rightSQL), nil
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

	// Handle pre-processed SQL strings from the parser
	// Only treat as pre-processed if it contains SQL keywords or operators
	if sqlStr, ok := value.(string); ok {
		// Check if this looks like a pre-processed SQL string (contains SQL keywords/operators)
		if strings.Contains(sqlStr, " ") || strings.Contains(sqlStr, "(") || strings.Contains(sqlStr, ")") {
			return sqlStr, nil
		}
		// Otherwise treat as a regular string literal that needs quoting
		return c.dataOp.valueToSQL(value)
	}

	// Handle complex expressions by delegating to the parser
	// This avoids circular dependencies by using the parser's expressionToSQL method
	if _, ok := value.(map[string]interface{}); ok {
		// For now, we'll return a placeholder for complex expressions
		// The parser should handle these cases when they're encountered
		return "", fmt.Errorf("complex expressions in comparisons should be handled by the parser")
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
	// Check if right side is a variable expression
	if varExpr, ok := rightValue.(map[string]interface{}); ok {
		if varName, hasVar := varExpr["var"]; hasVar {
			// Handle variable on right side: 'admin' IN roles
			rightSQL, err := c.dataOp.ToSQL("var", []interface{}{varName})
			if err != nil {
				return "", fmt.Errorf("invalid variable in IN operator: %v", err)
			}
			return fmt.Sprintf("%s IN %s", leftSQL, rightSQL), nil
		}
	}

	// The right side should be an array
	arr, ok := rightValue.([]interface{})
	if !ok {
		return "", fmt.Errorf("in operator requires array or variable as second argument")
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

// handleChainedComparison handles chained comparisons like {"<": [10, {"var": "x"}, 20, 30]}
// For 2 args: generates "a < b"
// For 3+ args: generates "(a < b AND b < c AND c < d)"
func (c *ComparisonOperator) handleChainedComparison(operator string, args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("chained comparison requires at least 2 arguments")
	}

	// Convert all arguments to SQL
	var sqlArgs []string
	for i, arg := range args {
		argSQL, err := c.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid argument %d: %v", i, err)
		}
		sqlArgs = append(sqlArgs, argSQL)
	}

	// For 2 arguments, return simple comparison without parentheses
	if len(args) == 2 {
		return fmt.Sprintf("%s %s %s", sqlArgs[0], operator, sqlArgs[1]), nil
	}

	// For 3+ arguments, generate chained comparisons with parentheses
	var conditions []string
	for i := 0; i < len(sqlArgs)-1; i++ {
		condition := fmt.Sprintf("%s %s %s", sqlArgs[i], operator, sqlArgs[i+1])
		conditions = append(conditions, condition)
	}

	return fmt.Sprintf("(%s)", strings.Join(conditions, " AND ")), nil
}
