package operators

import (
	"fmt"
	"strings"
)

// DataOperator handles data access operators (var, missing, missing_some).
type DataOperator struct {
	config *OperatorConfig
}

// NewDataOperator creates a new data operator with optional config.
func NewDataOperator(config *OperatorConfig) *DataOperator {
	return &DataOperator{config: config}
}

// schema returns the schema from config, or nil if not configured.
func (d *DataOperator) schema() SchemaProvider {
	if d.config == nil {
		return nil
	}
	return d.config.Schema
}

// ToSQL converts a data operator to SQL.
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

// handleVar converts var operator to SQL.
func (d *DataOperator) handleVar(args []interface{}) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("var operator requires at least 1 argument")
	}

	// Handle string argument (direct variable name)
	if varName, ok := args[0].(string); ok {
		// Validate field against schema if schema is provided
		if d.schema() != nil {
			if err := d.schema().ValidateField(varName); err != nil {
				return "", err
			}
		}
		columnName := d.convertVarName(varName)
		return columnName, nil
	}

	// For SQL context, var operator only accepts string arguments (column names)
	// Numeric array indexing is not supported

	// Handle array argument [varName, defaultValue]
	if arr, ok := args[0].([]interface{}); ok {
		if len(arr) == 0 {
			return "", fmt.Errorf("var operator array cannot be empty")
		}

		// Check if first element is a string (variable name)
		if varName, ok := arr[0].(string); ok {
			// Validate field against schema if schema is provided
			if d.schema() != nil {
				if err := d.schema().ValidateField(varName); err != nil {
					return "", err
				}
			}
			columnName := d.convertVarName(varName)

			// If there's a default value, use COALESCE
			if len(arr) > 1 {
				defaultValue := arr[1]
				defaultSQL, err := d.valueToSQL(defaultValue)
				if err != nil {
					return "", fmt.Errorf("invalid default value: %w", err)
				}
				return fmt.Sprintf("COALESCE(%s, %s)", columnName, defaultSQL), nil
			}

			return columnName, nil
		}

		// For SQL context, var operator only accepts string arguments (column names)
		// Numeric array indexing is not supported
		return "", fmt.Errorf("var operator first argument must be a string")
	}

	return "", fmt.Errorf("var operator requires string, number, or array argument")
}

// handleMissing converts missing operator to SQL.
func (d *DataOperator) handleMissing(args []interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("missing operator requires exactly 1 argument")
	}

	// Handle single string argument
	if varName, ok := args[0].(string); ok {
		// Validate field against schema if schema is provided
		if d.schema() != nil {
			if err := d.schema().ValidateField(varName); err != nil {
				return "", err
			}
		}
		columnName := d.convertVarName(varName)
		return fmt.Sprintf("%s IS NULL", columnName), nil
	}

	// Handle array of fields to check if any are missing
	if varNames, ok := args[0].([]interface{}); ok {
		if len(varNames) == 0 {
			return "", fmt.Errorf("missing operator array cannot be empty")
		}

		var nullConditions []string
		for _, varName := range varNames {
			name, ok := varName.(string)
			if !ok {
				return "", fmt.Errorf("all variable names in missing must be strings")
			}
			// Validate field against schema if schema is provided
			if d.schema() != nil {
				if err := d.schema().ValidateField(name); err != nil {
					return "", err
				}
			}
			columnName := d.convertVarName(name)
			nullConditions = append(nullConditions, fmt.Sprintf("%s IS NULL", columnName))
		}

		// Check if ANY of the fields are missing (OR condition)
		return fmt.Sprintf("(%s)", strings.Join(nullConditions, " OR ")), nil
	}

	return "", fmt.Errorf("missing operator argument must be a string or array of strings")
}

// handleMissingSome converts missing_some operator to SQL.
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

	// For minCount = 1, use simpler OR syntax
	if minCount == 1 {
		var nullConditions []string
		for _, varName := range varNames {
			name, ok := varName.(string)
			if !ok {
				return "", fmt.Errorf("all variable names in missing_some must be strings")
			}
			// Validate field against schema if schema is provided
			if d.schema() != nil {
				if err := d.schema().ValidateField(name); err != nil {
					return "", err
				}
			}
			columnName := d.convertVarName(name)
			nullConditions = append(nullConditions, fmt.Sprintf("%s IS NULL", columnName))
		}
		return fmt.Sprintf("(%s)", strings.Join(nullConditions, " OR ")), nil
	}

	// For other minCount values, use the counting approach
	// Convert variable names to column names and build CASE WHEN conditions to count NULLs
	var caseStatements []string
	for _, varName := range varNames {
		name, ok := varName.(string)
		if !ok {
			return "", fmt.Errorf("all variable names in missing_some must be strings")
		}
		// Validate field against schema if schema is provided
		if d.schema() != nil {
			if err := d.schema().ValidateField(name); err != nil {
				return "", err
			}
		}
		columnName := d.convertVarName(name)
		caseStatements = append(caseStatements, fmt.Sprintf("CASE WHEN %s IS NULL THEN 1 ELSE 0 END", columnName))
	}

	// Count how many are NULL and compare with minimum
	nullCount := strings.Join(caseStatements, " + ")
	return fmt.Sprintf("(%s) >= %d", nullCount, int(minCount)), nil
}

// convertVarName converts a JSON Logic variable name to SQL column name
// Preserves dot notation for nested properties: "user.verified" -> "user.verified".
func (d *DataOperator) convertVarName(varName string) string {
	// Keep the original dot notation as-is for nested properties
	// This allows for proper JSON column access in databases that support it
	return varName
}

// getNumber extracts a number from an interface{} and returns it as float64.
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

// valueToSQL converts a Go value to SQL literal.
func (d *DataOperator) valueToSQL(value interface{}) (string, error) {
	// Handle ProcessedValue (pre-processed SQL from parser)
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		// It's a literal, recursively convert it
		return d.valueToSQL(pv.Value)
	}

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
