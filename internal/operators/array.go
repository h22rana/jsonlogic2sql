package operators

import (
	"fmt"
	"strings"
)

// ArrayOperator handles array operations like map, filter, reduce, all, some, none, merge
type ArrayOperator struct {
	dataOp       *DataOperator
	comparisonOp *ComparisonOperator
	logicalOp    *LogicalOperator
	numericOp    *NumericOperator
}

// NewArrayOperator creates a new ArrayOperator instance
func NewArrayOperator() *ArrayOperator {
	return &ArrayOperator{
		dataOp:       NewDataOperator(),
		comparisonOp: NewComparisonOperator(),
		logicalOp:    nil, // Will be created lazily
		numericOp:    NewNumericOperator(),
	}
}

// getLogicalOperator returns the logical operator, creating it lazily if needed
func (a *ArrayOperator) getLogicalOperator() *LogicalOperator {
	if a.logicalOp == nil {
		a.logicalOp = NewLogicalOperator()
	}
	return a.logicalOp
}

// ToSQL converts an array operation to SQL
func (a *ArrayOperator) ToSQL(operator string, args []interface{}) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("array operator %s requires at least one argument", operator)
	}

	switch operator {
	case "map":
		return a.handleMap(args)
	case "filter":
		return a.handleFilter(args)
	case "reduce":
		return a.handleReduce(args)
	case "all":
		return a.handleAll(args)
	case "some":
		return a.handleSome(args)
	case "none":
		return a.handleNone(args)
	case "merge":
		return a.handleMerge(args)
	default:
		return "", fmt.Errorf("unsupported array operator: %s", operator)
	}
}

// handleMap converts map operator to SQL
// Note: This is a simplified implementation. In practice, map operations on arrays
// would require more complex SQL with window functions or subqueries.
func (a *ArrayOperator) handleMap(args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("map requires exactly 2 arguments")
	}

	// First argument: array
	array, err := a.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid map array argument: %v", err)
	}

	// Second argument: transformation expression
	// For now, we'll return a placeholder since map operations are complex in SQL
	// In a real implementation, this would need to handle the transformation logic
	return fmt.Sprintf("ARRAY_MAP(%s, %s)", array, "transformation_placeholder"), nil
}

// handleFilter converts filter operator to SQL
// Note: This is a simplified implementation. In practice, filter operations on arrays
// would require more complex SQL with array functions.
func (a *ArrayOperator) handleFilter(args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("filter requires exactly 2 arguments")
	}

	// First argument: array
	array, err := a.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid filter array argument: %v", err)
	}

	// Second argument: condition expression
	// For now, we'll return a placeholder since filter operations are complex in SQL
	return fmt.Sprintf("ARRAY_FILTER(%s, %s)", array, "condition_placeholder"), nil
}

// handleReduce converts reduce operator to SQL
// Note: This is a simplified implementation. In practice, reduce operations on arrays
// would require more complex SQL with aggregate functions.
func (a *ArrayOperator) handleReduce(args []interface{}) (string, error) {
	if len(args) != 3 {
		return "", fmt.Errorf("reduce requires exactly 3 arguments")
	}

	// First argument: array
	array, err := a.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid reduce array argument: %v", err)
	}

	// Second argument: initial value
	initial, err := a.valueToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid reduce initial argument: %v", err)
	}

	// Third argument: reduction expression
	// For now, we'll return a placeholder since reduce operations are complex in SQL
	return fmt.Sprintf("ARRAY_REDUCE(%s, %s, %s)", array, initial, "reduction_placeholder"), nil
}

// handleAll converts all operator to SQL
// This checks if all elements in an array satisfy a condition
func (a *ArrayOperator) handleAll(args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("all requires exactly 2 arguments")
	}

	// First argument: array
	array, err := a.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid all array argument: %v", err)
	}

	// Second argument: condition expression
	condition, err := a.expressionToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid all condition argument: %v", err)
	}

	// Use standard SQL: NOT EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE NOT (condition))
	// Replace 'elem' in condition with the actual element reference
	conditionWithElem := a.replaceElementReference(condition, "elem")
	return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM UNNEST(%s) AS elem WHERE NOT (%s))", array, conditionWithElem), nil
}

// handleSome converts some operator to SQL
// This checks if some elements in an array satisfy a condition
func (a *ArrayOperator) handleSome(args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("some requires exactly 2 arguments")
	}

	// First argument: array
	array, err := a.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid some array argument: %v", err)
	}

	// Second argument: condition expression
	condition, err := a.expressionToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid some condition argument: %v", err)
	}

	// Use standard SQL: EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE condition)
	conditionWithElem := a.replaceElementReference(condition, "elem")
	return fmt.Sprintf("EXISTS (SELECT 1 FROM UNNEST(%s) AS elem WHERE %s)", array, conditionWithElem), nil
}

// handleNone converts none operator to SQL
// This checks if no elements in an array satisfy a condition
func (a *ArrayOperator) handleNone(args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("none requires exactly 2 arguments")
	}

	// First argument: array
	array, err := a.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid none array argument: %v", err)
	}

	// Second argument: condition expression
	condition, err := a.expressionToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid none condition argument: %v", err)
	}

	// Use standard SQL: NOT EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE condition)
	conditionWithElem := a.replaceElementReference(condition, "elem")
	return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM UNNEST(%s) AS elem WHERE %s)", array, conditionWithElem), nil
}

// handleMerge converts merge operator to SQL
// This merges multiple arrays into one
func (a *ArrayOperator) handleMerge(args []interface{}) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("merge requires at least 1 argument")
	}

	// Convert all array arguments to SQL
	arrays := make([]string, len(args))
	for i, arg := range args {
		array, err := a.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid merge array argument %d: %v", i, err)
		}
		arrays[i] = array
	}

	// Use ARRAY_CONCAT or similar function to merge arrays
	return fmt.Sprintf("ARRAY_CONCAT(%s)", strings.Join(arrays, ", ")), nil
}

// valueToSQL converts a value to SQL, handling var expressions, arrays, and literals
func (a *ArrayOperator) valueToSQL(value interface{}) (string, error) {
	// Handle var expressions
	if expr, ok := value.(map[string]interface{}); ok {
		if varExpr, hasVar := expr["var"]; hasVar {
			return a.dataOp.ToSQL("var", []interface{}{varExpr})
		}
	}

	// Handle arrays
	if arr, ok := value.([]interface{}); ok {
		// Convert array elements to SQL literals
		elements := make([]string, len(arr))
		for i, elem := range arr {
			elementSQL, err := a.dataOp.valueToSQL(elem)
			if err != nil {
				return "", fmt.Errorf("invalid array element %d: %v", i, err)
			}
			elements[i] = elementSQL
		}
		return fmt.Sprintf("[%s]", strings.Join(elements, " ")), nil
	}

	// Handle primitive values
	return a.dataOp.valueToSQL(value)
}

// expressionToSQL converts a JSON Logic expression to SQL
func (a *ArrayOperator) expressionToSQL(expr interface{}) (string, error) {
	// Handle primitive values
	if a.isPrimitive(expr) {
		return a.dataOp.valueToSQL(expr)
	}

	// Handle var expressions
	if varExpr, ok := expr.(map[string]interface{}); ok {
		if varName, hasVar := varExpr["var"]; hasVar {
			// Special case: empty var name represents the current element in array operations
			if varName == "" {
				return "elem", nil
			}
			return a.dataOp.ToSQL("var", []interface{}{varName})
		}
	}

	// Handle complex expressions by delegating to other operators
	if exprMap, ok := expr.(map[string]interface{}); ok {
		for operator, args := range exprMap {
			switch operator {
			case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
				if arr, ok := args.([]interface{}); ok {
					return a.comparisonOp.ToSQL(operator, arr)
				}
			case "and", "or", "!", "!!", "if", "?:":
				if arr, ok := args.([]interface{}); ok {
					return a.getLogicalOperator().ToSQL(operator, arr)
				}
			case "+", "-", "*", "/", "%", "max", "min", "between":
				if arr, ok := args.([]interface{}); ok {
					return a.numericOp.ToSQL(operator, arr)
				}
			default:
				return "", fmt.Errorf("unsupported operator in array expression: %s", operator)
			}
		}
	}

	return "", fmt.Errorf("invalid expression type: %T", expr)
}

// replaceElementReference replaces element references in conditions
// For now, this is a simple implementation that assumes the condition uses a variable
func (a *ArrayOperator) replaceElementReference(condition, elementName string) string {
	// Replace variable references in the condition with the element name
	// This handles cases where {"var": "item"} should become "elem"
	// Simple string replacement for now - in a more complex implementation,
	// you'd want to parse the SQL and replace variable references properly
	return strings.ReplaceAll(condition, "item", elementName)
}

// isPrimitive checks if a value is a primitive type
func (a *ArrayOperator) isPrimitive(value interface{}) bool {
	switch value.(type) {
	case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		return true
	case nil:
		return true
	default:
		return false
	}
}
