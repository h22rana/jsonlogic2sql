package operators

import (
	"fmt"
	"strings"
)

// DataOperator handles data access operators (var, missing, missing_some)
type DataOperator struct{}

// NewDataOperator creates a new data operator
func NewDataOperator() *DataOperator {
	return &DataOperator{}
}

// ToSQL converts a data operator to SQL
func (d *DataOperator) ToSQL(operator string, args []interface{}) (string, error) {
	switch operator {
	case "var":
		return d.handleVar(args)
	case "missing":
		return d.handleMissing(args)
	case "missing_some":
		return d.handleMissingSome(args)
	default:
		return "", fmt.Errorf("unsupported data operator: %s", operator)
	}
}

// handleVar converts var operator to SQL
func (d *DataOperator) handleVar(args []interface{}) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("var operator requires at least 1 argument")
	}

	// Handle string argument (direct variable name)
	if varName, ok := args[0].(string); ok {
		columnName := d.convertVarName(varName)
		return columnName, nil
	}

	// Handle array argument [varName, defaultValue]
	if arr, ok := args[0].([]interface{}); ok {
		if len(arr) == 0 {
			return "", fmt.Errorf("var operator array cannot be empty")
		}

		varName, ok := arr[0].(string)
		if !ok {
			return "", fmt.Errorf("var operator first argument must be a string")
		}

		columnName := d.convertVarName(varName)

		// If there's a default value, use COALESCE
		if len(arr) > 1 {
			defaultValue := arr[1]
			defaultSQL, err := d.valueToSQL(defaultValue)
			if err != nil {
				return "", fmt.Errorf("invalid default value: %v", err)
			}
			return fmt.Sprintf("COALESCE(%s, %s)", columnName, defaultSQL), nil
		}

		return columnName, nil
	}

	return "", fmt.Errorf("var operator requires string or array argument")
}

// handleMissing converts missing operator to SQL
func (d *DataOperator) handleMissing(args []interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("missing operator requires exactly 1 argument")
	}

	varName, ok := args[0].(string)
	if !ok {
		return "", fmt.Errorf("missing operator argument must be a string")
	}

	columnName := d.convertVarName(varName)
	return fmt.Sprintf("%s IS NULL", columnName), nil
}

// handleMissingSome converts missing_some operator to SQL
func (d *DataOperator) handleMissingSome(args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("missing_some operator requires exactly 2 arguments")
	}

	// First argument should be the minimum count
	minCount, err := d.getNumber(args[0])
	if err != nil {
		return "", fmt.Errorf("missing_some operator first argument must be a number")
	}

	// Second argument should be an array of variable names
	varNames, ok := args[1].([]interface{})
	if !ok {
		return "", fmt.Errorf("missing_some operator second argument must be an array")
	}

	if len(varNames) == 0 {
		return "", fmt.Errorf("missing_some operator variable list cannot be empty")
	}

	// Convert variable names to column names and build IS NULL conditions
	var conditions []string
	for _, varName := range varNames {
		name, ok := varName.(string)
		if !ok {
			return "", fmt.Errorf("all variable names in missing_some must be strings")
		}
		columnName := d.convertVarName(name)
		conditions = append(conditions, fmt.Sprintf("%s IS NULL", columnName))
	}

	// Count how many are NULL and compare with minimum
	nullCount := strings.Join(conditions, " + ")
	return fmt.Sprintf("(%s) >= %d", nullCount, int(minCount)), nil
}

// convertVarName converts a JSON Logic variable name to SQL column name
// Preserves dot notation for nested properties: "user.verified" -> "user.verified"
func (d *DataOperator) convertVarName(varName string) string {
	// Keep the original dot notation as-is for nested properties
	// This allows for proper JSON column access in databases that support it
	return varName
}

// getNumber extracts a number from an interface{} and returns it as float64
func (d *DataOperator) getNumber(value interface{}) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("not a number")
	}
}

// valueToSQL converts a Go value to SQL literal
func (d *DataOperator) valueToSQL(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		// Escape single quotes in strings
		escaped := strings.ReplaceAll(v, "'", "''")
		return fmt.Sprintf("'%s'", escaped), nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%v", v), nil
	case float32, float64:
		return fmt.Sprintf("%v", v), nil
	case bool:
		if v {
			return "TRUE", nil
		}
		return "FALSE", nil
	case nil:
		return "NULL", nil
	default:
		return "", fmt.Errorf("unsupported value type: %T", value)
	}
}
