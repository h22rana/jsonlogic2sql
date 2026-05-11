package operators

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// validIdentifierSegment matches a conservative identifier segment allowlist
// used when no schema is configured.
var validIdentifierSegment = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// validJSONNumberLiteral matches strict JSON numeric literals.
// This prevents crafted json.Number values (from map/interface APIs) from being
// inlined as arbitrary SQL fragments.
var validJSONNumberLiteral = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?$`)

// DataOperator handles data access operators (var, missing, missing_some).
type DataOperator struct {
	config *OperatorConfig
}

const (
	maxSafeJSInt       = int64(1<<53 - 1)
	minSafeJSInt       = -maxSafeJSInt
	maxVarArrayEntries = 2
)

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

func (d *DataOperator) columnNameForVar(varName string) (string, error) {
	columnName, err := d.convertVarName(varName)
	if err != nil {
		return "", err
	}
	if d.schema() != nil {
		if err := d.schema().ValidateField(varName); err != nil {
			return "", err
		}
	}
	return columnName, nil
}

func validateVarArrayOperand(arr []interface{}) error {
	if len(arr) == 0 {
		return fmt.Errorf("var operator array cannot be empty")
	}
	return validateVarArrayMaxEntries(arr)
}

func validateVarArrayMaxEntries(arr []interface{}) error {
	if len(arr) > maxVarArrayEntries {
		return fmt.Errorf("var operator array accepts at most %d entries", maxVarArrayEntries)
	}
	return nil
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
		// Special case: empty var name represents the current element in array operations
		// In JSON Logic, {"var": ""} means "the current data context"
		// In array operations (map, filter, reduce), this refers to the current element
		if varName == "" {
			return ElemVar, nil
		}

		columnName, err := d.columnNameForVar(varName)
		if err != nil {
			return "", err
		}
		return columnName, nil
	}

	// For SQL context, var operator only accepts string arguments (column names)
	// Numeric array indexing is not supported

	// Handle array argument [varName, defaultValue]
	if arr, ok := args[0].([]interface{}); ok {
		if err := validateVarArrayOperand(arr); err != nil {
			return "", err
		}

		if pv, ok := arr[0].(ProcessedValue); ok && pv.IsSQL {
			columnName := pv.Value
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

		// Check if first element is a string (variable name)
		if varName, ok := arr[0].(string); ok {
			columnName, err := d.columnNameForVar(varName)
			if err != nil {
				return "", err
			}

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
		columnName, err := d.columnNameForVar(varName)
		if err != nil {
			return "", err
		}
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
			columnName, err := d.columnNameForVar(name)
			if err != nil {
				return "", err
			}
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
			columnName, err := d.columnNameForVar(name)
			if err != nil {
				return "", err
			}
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
		columnName, err := d.columnNameForVar(name)
		if err != nil {
			return "", err
		}
		caseStatements = append(caseStatements, fmt.Sprintf("CASE WHEN %s IS NULL THEN 1 ELSE 0 END", columnName))
	}

	// Count how many are NULL and compare with minimum
	nullCount := strings.Join(caseStatements, " + ")
	return fmt.Sprintf("(%s) >= %d", nullCount, int(minCount)), nil
}

// convertVarName converts a JSON Logic variable name to a SQL column reference.
// It splits the name on dots, quotes any segment that is not a valid unquoted SQL
// identifier (e.g. starts with a digit like "24h"), and rejoins with dots.
// When no schema is configured, it still validates each raw segment against a
// conservative allowlist before quoting to avoid accepting arbitrary SQL text.
// Returns an error if any segment contains quote characters (backtick, double
// quote, or single quote), since the transpiler handles quoting automatically.
func (d *DataOperator) convertVarName(varName string) (string, error) {
	dl := dialect.DialectUnspecified
	if d.config != nil {
		dl = d.config.GetDialect()
	}

	segments := strings.Split(varName, ".")
	for i, seg := range segments {
		if dialect.ContainsQuoteCharacters(seg) {
			return "", fmt.Errorf("variable name %q contains quote characters; "+
				"use raw identifiers — the transpiler handles quoting automatically", varName)
		}
		if d.schema() == nil && (seg == "" || !validIdentifierSegment.MatchString(seg)) {
			return "", fmt.Errorf("invalid identifier %q: each segment must match [a-zA-Z0-9_]+", varName)
		}
		if dialect.NeedsQuoting(seg) {
			segments[i] = dialect.QuoteIdentifierSegment(seg, dl)
		}
	}
	return strings.Join(segments, "."), nil
}

// getNumber extracts a number from an interface{} and returns it as float64.
func (d *DataOperator) getNumber(value interface{}) (float64, error) {
	switch v := value.(type) {
	case json.Number:
		n, err := v.Float64()
		if err != nil {
			return 0, fmt.Errorf("not a number")
		}
		return n, nil
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
	case json.Number:
		numberLiteral, err := normalizeJSONNumberLiteral(v)
		if err != nil {
			return "", err
		}
		return numberLiteral, nil
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

// ValueToSQL converts a Go value to a SQL literal or expression. It exposes the
// same conversion used internally by operators for parser-level value mode.
func (d *DataOperator) ValueToSQL(value interface{}) (string, error) {
	return d.valueToSQL(value)
}

// ToSQLParam is the parameterized variant of ToSQL. Keep in sync.
func (d *DataOperator) ToSQLParam(operator string, args []interface{}, pc *params.ParamCollector) (string, error) {
	switch operator {
	case "var":
		return d.handleVarParam(args, pc)
	case "missing":
		return d.handleMissing(args)
	case "missing_some":
		return d.handleMissingSomeParam(args, pc)
	default:
		return "", fmt.Errorf("unsupported data operator: %s", operator)
	}
}

// handleVarParam is the parameterized variant of handleVar. Keep in sync.
func (d *DataOperator) handleVarParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("var operator requires at least 1 argument")
	}

	if varName, ok := args[0].(string); ok {
		if varName == "" {
			return ElemVar, nil
		}

		columnName, err := d.columnNameForVar(varName)
		if err != nil {
			return "", err
		}
		return columnName, nil
	}

	if arr, ok := args[0].([]interface{}); ok {
		if err := validateVarArrayOperand(arr); err != nil {
			return "", err
		}

		if pv, ok := arr[0].(ProcessedValue); ok && pv.IsSQL {
			columnName := pv.Value
			if len(arr) > 1 {
				defaultValue := arr[1]
				defaultSQL, err := d.valueToSQLParam(defaultValue, pc)
				if err != nil {
					return "", fmt.Errorf("invalid default value: %w", err)
				}
				return fmt.Sprintf("COALESCE(%s, %s)", columnName, defaultSQL), nil
			}
			return columnName, nil
		}

		if varName, ok := arr[0].(string); ok {
			columnName, err := d.columnNameForVar(varName)
			if err != nil {
				return "", err
			}

			if len(arr) > 1 {
				defaultValue := arr[1]
				defaultSQL, err := d.valueToSQLParam(defaultValue, pc)
				if err != nil {
					return "", fmt.Errorf("invalid default value: %w", err)
				}
				return fmt.Sprintf("COALESCE(%s, %s)", columnName, defaultSQL), nil
			}

			return columnName, nil
		}

		return "", fmt.Errorf("var operator first argument must be a string")
	}

	return "", fmt.Errorf("var operator requires string, number, or array argument")
}

// handleMissingSomeParam is the parameterized variant of handleMissingSome. Keep in sync.
func (d *DataOperator) handleMissingSomeParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("missing_some operator requires exactly 2 arguments")
	}

	minCount, err := d.getNumber(args[0])
	if err != nil {
		return "", fmt.Errorf("missing_some operator first argument must be a number")
	}

	varNames, ok := args[1].([]interface{})
	if !ok {
		return "", fmt.Errorf("missing_some operator second argument must be an array")
	}

	if len(varNames) == 0 {
		return "", fmt.Errorf("missing_some operator variable list cannot be empty")
	}

	if minCount == 1 {
		var nullConditions []string
		for _, varName := range varNames {
			name, ok := varName.(string)
			if !ok {
				return "", fmt.Errorf("all variable names in missing_some must be strings")
			}
			columnName, err := d.columnNameForVar(name)
			if err != nil {
				return "", err
			}
			nullConditions = append(nullConditions, fmt.Sprintf("%s IS NULL", columnName))
		}
		return fmt.Sprintf("(%s)", strings.Join(nullConditions, " OR ")), nil
	}

	var caseStatements []string
	for _, varName := range varNames {
		name, ok := varName.(string)
		if !ok {
			return "", fmt.Errorf("all variable names in missing_some must be strings")
		}
		columnName, err := d.columnNameForVar(name)
		if err != nil {
			return "", err
		}
		caseStatements = append(caseStatements, fmt.Sprintf("CASE WHEN %s IS NULL THEN 1 ELSE 0 END", columnName))
	}

	nullCount := strings.Join(caseStatements, " + ")
	return fmt.Sprintf("(%s) >= %s", nullCount, pc.Add(minCount)), nil
}

// valueToSQLParam is the parameterized variant of valueToSQL. Keep in sync.
// Structural tokens (NULL, TRUE, FALSE) remain inline; user-data values
// are registered with the ParamCollector and replaced by a placeholder.
func (d *DataOperator) valueToSQLParam(value interface{}, pc *params.ParamCollector) (string, error) {
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		return d.valueToSQLParam(pv.Value, pc)
	}

	switch v := value.(type) {
	case string:
		return pc.Add(v), nil
	case json.Number:
		if _, err := normalizeJSONNumberLiteral(v); err != nil {
			return "", err
		}
		return pc.Add(jsonNumberParamValue(v)), nil
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return pc.Add(v), nil
	case float32:
		return pc.Add(float64(v)), nil
	case float64:
		return pc.Add(v), nil
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

// ValueToSQLParam is the parameterized variant of ValueToSQL.
func (d *DataOperator) ValueToSQLParam(value interface{}, pc *params.ParamCollector) (string, error) {
	return d.valueToSQLParam(value, pc)
}

// jsonNumberParamValue converts a json.Number into a driver-friendly bind value.
// Integers in JS-safe range stay float64 for backward compatibility with existing
// parameterized behavior. Large integers and out-of-range floats are preserved as
// strings to avoid precision loss, matching the inline path's verbatim behavior.
func jsonNumberParamValue(num json.Number) interface{} {
	numStr := num.String()
	if isIntegerLiteral(numStr) {
		if i, err := strconv.ParseInt(numStr, 10, 64); err == nil {
			if i >= minSafeJSInt && i <= maxSafeJSInt {
				return float64(i)
			}
			return numStr
		}
		return numStr
	}
	if f, err := strconv.ParseFloat(numStr, 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
		if f == 0 && hasNonZeroSignificand(numStr) {
			return numStr
		}
		return f
	}
	return numStr
}

// normalizeJSONNumberLiteral validates json.Number text and returns the literal
// for safe SQL emission.
func normalizeJSONNumberLiteral(num json.Number) (string, error) {
	numStr := num.String()
	if !validJSONNumberLiteral.MatchString(numStr) {
		return "", fmt.Errorf("invalid json number literal: %q", numStr)
	}
	return numStr, nil
}

// hasNonZeroSignificand reports whether a numeric string has a non-zero digit
// in its significand (the part before 'e'/'E'). Used to detect float64
// underflow where ParseFloat returns 0 but the original value is non-zero.
func hasNonZeroSignificand(s string) bool {
	for _, c := range s {
		if c == 'e' || c == 'E' {
			break
		}
		if c >= '1' && c <= '9' {
			return true
		}
	}
	return false
}
