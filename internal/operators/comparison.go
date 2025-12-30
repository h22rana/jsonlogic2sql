package operators

import (
	"fmt"
	"strings"
)

// ComparisonOperator handles comparison operators (==, ===, !=, !==, >, >=, <, <=)
type ComparisonOperator struct {
	dataOp *DataOperator
	schema SchemaProvider
}

// NewComparisonOperator creates a new comparison operator
func NewComparisonOperator() *ComparisonOperator {
	return &ComparisonOperator{
		dataOp: NewDataOperator(),
	}
}

// SetSchema sets the schema provider for field validation and type checking
func (c *ComparisonOperator) SetSchema(schema SchemaProvider) {
	c.schema = schema
	if c.dataOp != nil {
		c.dataOp.SetSchema(schema)
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

	// Handle NULL comparisons - use IS NULL/IS NOT NULL instead of = NULL/!= NULL
	isLeftNull := args[0] == nil || leftSQL == "NULL"
	isRightNull := args[1] == nil || rightSQL == "NULL"

	switch operator {
	case "==":
		// Handle NULL comparisons
		if isLeftNull && isRightNull {
			return "NULL IS NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s = %s", leftSQL, rightSQL), nil
	case "===":
		// Strict equality - same as == but handle NULL
		if isLeftNull && isRightNull {
			return "NULL IS NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s = %s", leftSQL, rightSQL), nil
	case "!=":
		// Handle NULL comparisons
		if isLeftNull && isRightNull {
			return "NULL IS NOT NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NOT NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NOT NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s != %s", leftSQL, rightSQL), nil
	case "!==":
		// Strict inequality - same as != but handle NULL
		if isLeftNull && isRightNull {
			return "NULL IS NOT NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NOT NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NOT NULL", leftSQL), nil
		}
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
					// Special case: empty var name represents the current element in array operations
					if varName, ok := args.(string); ok && varName == "" {
						return "elem", nil
					}
					return c.dataOp.ToSQL("var", []interface{}{args})
				}
			}
		}
	}

	// Handle pre-processed SQL strings from the parser
	// Only treat as pre-processed if it contains SQL keywords or operators
	if sqlStr, ok := value.(string); ok {
		// Check if this looks like a pre-processed SQL string (contains SQL keywords/operators)
		// This includes strings with spaces, parentheses, or SQL function names
		if strings.Contains(sqlStr, " ") || strings.Contains(sqlStr, "(") || strings.Contains(sqlStr, ")") ||
			strings.Contains(sqlStr, "ARRAY_") || strings.Contains(sqlStr, "EXISTS") || strings.Contains(sqlStr, "SELECT") {
			return sqlStr, nil
		}
		// Otherwise treat as a regular string literal that needs quoting
		return c.dataOp.valueToSQL(value)
	}

	// Handle complex expressions (arithmetic, comparisons, etc.)
	// Note: Array operators (reduce, filter, some, etc.) should be pre-processed by the parser
	// into SQL strings, but if they're not, we'll return an error here
	if expr, ok := value.(map[string]interface{}); ok {
		if len(expr) == 1 {
			for op, args := range expr {
				switch op {
				case "+", "-", "*", "/", "%":
					// Handle arithmetic operations
					return c.processArithmeticExpression(op, args)
				case ">", ">=", "<", "<=", "==", "===", "!=", "!==":
					// Handle comparison operations
					return c.processComparisonExpression(op, args)
				case "max", "min":
					// Handle min/max operations
					return c.processMinMaxExpression(op, args)
				case "if":
					// Handle if operator - delegate to logical operator
					if arr, ok := args.([]interface{}); ok {
						logicalOp := NewLogicalOperator()
						return logicalOp.ToSQL("if", arr)
					}
					return "", fmt.Errorf("if operator requires array arguments")
				case "reduce", "filter", "map", "some", "all", "none", "merge":
					// Array operators should have been pre-processed by the parser/logical operator
					// If we see them here, it means they weren't processed correctly
					// Try to process them directly as a fallback
					if arr, ok := args.([]interface{}); ok {
						arrayOp := NewArrayOperator()
						return arrayOp.ToSQL(op, arr)
					}
					return "", fmt.Errorf("array operator %s requires array arguments", op)
				case "cat", "substr":
					// Handle string operators
					if arr, ok := args.([]interface{}); ok {
						stringOp := NewStringOperator()
						return stringOp.ToSQL(op, arr)
					}
					return "", fmt.Errorf("string operator %s requires array arguments", op)
				default:
					return "", fmt.Errorf("unsupported expression type in comparison: %s", op)
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
	// Check if right side is a variable expression
	if varExpr, ok := rightValue.(map[string]interface{}); ok {
		if varName, hasVar := varExpr["var"]; hasVar {
			// Handle variable on right side
			// According to JSON Logic spec, "in" supports both:
			// 1. Array membership: {"in": [value, array]} → value IN array
			// 2. String containment: {"in": [substring, string]} → substring contained in string
			//
			// Use schema to determine the correct SQL:
			// - If variable is an ARRAY column: 'value' IN column (array membership)
			// - If variable is a STRING column: STRPOS(column, 'value') > 0 (string containment)
			rightSQL, err := c.dataOp.ToSQL("var", []interface{}{varName})
			if err != nil {
				return "", fmt.Errorf("invalid variable in IN operator: %v", err)
			}

			// Extract field name from varName (handle both string and array cases)
			var fieldName string
			if nameStr, ok := varName.(string); ok {
				fieldName = nameStr
			} else if nameArr, ok := varName.([]interface{}); ok && len(nameArr) > 0 {
				if nameStr, ok := nameArr[0].(string); ok {
					fieldName = nameStr
				}
			}

			// Use schema to determine type if available
			if c.schema != nil && fieldName != "" {
				if c.schema.IsArrayType(fieldName) {
					// Array type: use array membership syntax
					return fmt.Sprintf("%s IN %s", leftSQL, rightSQL), nil
				} else if c.schema.IsStringType(fieldName) {
					// String type: use string containment syntax
					return fmt.Sprintf("STRPOS(%s, %s) > 0", rightSQL, leftSQL), nil
				}
			}

			// No schema or unknown type: use heuristic based on left side
			// If left side is a literal (quoted), assume string containment
			isLeftLiteral := strings.HasPrefix(leftSQL, "'") && strings.HasSuffix(leftSQL, "'")
			if isLeftLiteral {
				// Use STRPOS for string containment (default for Spanner/BigQuery)
				return fmt.Sprintf("STRPOS(%s, %s) > 0", rightSQL, leftSQL), nil
			}
			// Otherwise, assume array membership
			return fmt.Sprintf("%s IN %s", leftSQL, rightSQL), nil
		}
	}

	// Check if right side is an array
	if arr, ok := rightValue.([]interface{}); ok {
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

	// Check if right side is a string (string containment)
	if str, ok := rightValue.(string); ok {
		// Use POSITION function for string containment: POSITION(left IN right) > 0
		rightSQL, err := c.dataOp.valueToSQL(str)
		if err != nil {
			return "", fmt.Errorf("invalid string in IN operator: %v", err)
		}
		return fmt.Sprintf("POSITION(%s IN %s) > 0", leftSQL, rightSQL), nil
	}

	// Check if right side is a number (convert to string for containment)
	if num, ok := rightValue.(float64); ok {
		// Convert number to string for containment check
		rightSQL, err := c.dataOp.valueToSQL(num)
		if err != nil {
			return "", fmt.Errorf("invalid number in IN operator: %v", err)
		}
		return fmt.Sprintf("POSITION(%s IN %s) > 0", leftSQL, rightSQL), nil
	}

	return "", fmt.Errorf("in operator requires array, variable, string, or number as second argument")
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

// processArithmeticExpression handles arithmetic operations within comparison operations
func (c *ComparisonOperator) processArithmeticExpression(op string, args interface{}) (string, error) {
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
		operand, err := c.valueToSQL(arg)
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

// processComparisonExpression handles comparison operations within comparison operations
func (c *ComparisonOperator) processComparisonExpression(op string, args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("comparison operation requires array of arguments")
	}

	if len(argsSlice) != 2 {
		return "", fmt.Errorf("comparison operation requires exactly 2 arguments")
	}

	// Convert arguments to SQL
	left, err := c.valueToSQL(argsSlice[0])
	if err != nil {
		return "", fmt.Errorf("invalid comparison left argument: %v", err)
	}

	right, err := c.valueToSQL(argsSlice[1])
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

// processMinMaxExpression handles min/max operations within comparison operations
func (c *ComparisonOperator) processMinMaxExpression(op string, args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("min/max operation requires array of arguments")
	}

	if len(argsSlice) < 2 {
		return "", fmt.Errorf("min/max operation requires at least 2 arguments")
	}

	// Convert arguments to SQL
	operands := make([]string, len(argsSlice))
	for i, arg := range argsSlice {
		operand, err := c.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid min/max argument %d: %v", i, err)
		}
		operands[i] = operand
	}

	// Generate SQL based on operation
	switch op {
	case "max":
		return fmt.Sprintf("GREATEST(%s)", strings.Join(operands, ", ")), nil
	case "min":
		return fmt.Sprintf("LEAST(%s)", strings.Join(operands, ", ")), nil
	default:
		return "", fmt.Errorf("unsupported min/max operation: %s", op)
	}
}
