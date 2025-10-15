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
}

// NewArrayOperator creates a new ArrayOperator instance
func NewArrayOperator() *ArrayOperator {
	return &ArrayOperator{
		dataOp:       NewDataOperator(),
		comparisonOp: NewComparisonOperator(),
		logicalOp:    NewLogicalOperator(),
	}
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
	// For now, we'll return a placeholder since all operations are complex in SQL
	return fmt.Sprintf("ARRAY_ALL(%s, %s)", array, "condition_placeholder"), nil
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
	// For now, we'll return a placeholder since some operations are complex in SQL
	return fmt.Sprintf("ARRAY_SOME(%s, %s)", array, "condition_placeholder"), nil
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
	// For now, we'll return a placeholder since none operations are complex in SQL
	return fmt.Sprintf("ARRAY_NONE(%s, %s)", array, "condition_placeholder"), nil
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
