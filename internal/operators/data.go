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

// NewDataOperator creates a new data operator.
func NewDataOperator(config *OperatorConfig) *DataOperator {
	config = normalizeOperatorConfig(config)
	return &DataOperator{config: config}
}

func (d *DataOperator) schema() SchemaProvider {
	return schemaFromConfig(d.config)
}

func (d *DataOperator) columnNameForVar(varName string) (string, error) {
	columnName, err := d.convertVarName(varName)
	if err != nil {
		return "", err
	}
	if err := d.schema().ValidateField(varName); err != nil {
		return "", err
	}
	return columnName, nil
}

func (d *DataOperator) columnNameForFieldOperand(field interface{}, typeErr string) (string, error) {
	if pv, ok := field.(ProcessedValue); ok && pv.IsSQL {
		return pv.Value, nil
	}
	if varName, ok := field.(string); ok {
		return d.columnNameForVar(varName)
	}
	return "", fmt.Errorf("%s", typeErr)
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

func validateVarDefaultForFields(schema SchemaProvider, fieldNames []string, defaultValue interface{}) error {
	for _, fieldName := range normalizeSchemaScopes(fieldNames) {
		if err := validateVarDefaultForField(schema, fieldName, defaultValue); err != nil {
			return err
		}
	}
	return nil
}

func validateProcessedValueDefault(schema SchemaProvider, pv ProcessedValue, defaultValue interface{}) error {
	if fieldNames := pv.SchemaFieldNames(); len(fieldNames) > 0 {
		return validateVarDefaultForFields(schema, fieldNames, defaultValue)
	}
	if pv.HasExpressionInfo && pv.Kind == ExpressionKindValue {
		return validateVarDefaultForExpressionType(pv.Type, defaultValue, "array element")
	}
	return nil
}

func validateVarDefaultForField(schema SchemaProvider, fieldName string, defaultValue interface{}) error {
	if schema == nil || fieldName == "" || defaultValue == nil {
		return nil
	}
	if err := validateEqualityJSONNumberLiteral(defaultValue); err != nil {
		return err
	}
	if schema.IsEnumType(fieldName) {
		strVal, ok := defaultValue.(string)
		if !ok {
			return fmt.Errorf("default value for enum field '%s' must be string or null, got %s",
				fieldName, expressionTypeName(inferLiteralValueExpressionType(defaultValue)))
		}
		return schema.ValidateEnumValue(fieldName, strVal)
	}

	defaultType := inferLiteralValueExpressionType(defaultValue)
	if defaultType == ExpressionTypeNull {
		return nil
	}
	expected := schemaExpressionType(schema, fieldName)
	if expected == ExpressionTypeUnknown {
		return nil
	}
	if expected == ExpressionTypeArray {
		return validateArrayVarDefaultForField(schema, fieldName, defaultValue)
	}
	if expected == defaultType {
		return nil
	}
	return fmt.Errorf("default value for field '%s' has incompatible type %s; expected %s or null",
		fieldName, expressionTypeName(defaultType), expressionTypeName(expected))
}

func validateArrayVarDefaultForField(schema SchemaProvider, fieldName string, defaultValue interface{}) error {
	arr, ok := defaultValue.([]interface{})
	if !ok {
		return fmt.Errorf("default value for field '%s' has incompatible type %s; expected array or null",
			fieldName, expressionTypeName(inferLiteralValueExpressionType(defaultValue)))
	}
	provider, ok := schema.(ArrayElementSchemaProvider)
	if err := validateArrayVarDefaultElementsForField(schema, fieldName, arr); err != nil {
		return err
	}
	if !ok || !provider.HasArrayElementFields(fieldName) || len(arr) == 0 {
		return nil
	}
	return fmt.Errorf("default value for object-array field '%s' must be an empty array or null; non-empty object-array defaults are not renderable as portable SQL literals",
		fieldName)
}

func validateArrayVarDefaultElementsForField(schema SchemaProvider, fieldName string, arr []interface{}) error {
	expected, known := schemaArrayElementExpressionType(schema, fieldName)
	if !known || expected == ExpressionTypeObject {
		return nil
	}
	for i, elem := range arr {
		if err := validateEqualityJSONNumberLiteral(elem); err != nil {
			return err
		}
		actual := inferLiteralValueExpressionType(elem)
		if actual == ExpressionTypeNull {
			continue
		}
		if expected == ExpressionTypeString && schemaArrayElementType(schema, fieldName) == "enum" {
			strVal, ok := elem.(string)
			if !ok {
				return fmt.Errorf("default value for array field '%s' element %d has incompatible type %s; expected string or null",
					fieldName, i, expressionTypeName(actual))
			}
			if err := validateSchemaEnumArrayElementValue(schema, fieldName, strVal); err != nil {
				return err
			}
			continue
		}
		if actual != expected {
			return fmt.Errorf("default value for array field '%s' element %d has incompatible type %s; expected %s or null",
				fieldName, i, expressionTypeName(actual), expressionTypeName(expected))
		}
	}
	return nil
}

func validateVarDefaultForExpressionType(typ ExpressionType, defaultValue interface{}, label string) error {
	if defaultValue == nil {
		return nil
	}
	if err := validateEqualityJSONNumberLiteral(defaultValue); err != nil {
		return err
	}
	defaultType := inferLiteralValueExpressionType(defaultValue)
	if defaultType == ExpressionTypeNull || typ == ExpressionTypeUnknown || defaultType == typ {
		return nil
	}
	return fmt.Errorf("default value for %s has incompatible type %s; expected %s or null",
		label, expressionTypeName(defaultType), expressionTypeName(typ))
}

func (d *DataOperator) defaultValueToSQL(value interface{}) (string, error) {
	if arr, ok := value.([]interface{}); ok {
		return d.arrayLiteralToSQL(arr)
	}
	return d.valueToSQL(value)
}

func (d *DataOperator) defaultValueToSQLParam(value interface{}, pc *params.ParamCollector) (string, error) {
	if arr, ok := value.([]interface{}); ok {
		return d.arrayLiteralToSQLParam(arr, pc)
	}
	return d.valueToSQLParam(value, pc)
}

func (d *DataOperator) arrayLiteralToSQL(arr []interface{}) (string, error) {
	if err := d.config.ValidateArrayLiteralValue(arr); err != nil {
		return "", err
	}
	elements := make([]string, len(arr))
	var commonTypes []ExpressionType
	for i, elem := range arr {
		elementSQL, elementType, err := d.arrayLiteralElementSQL(elem)
		if err != nil {
			return "", fmt.Errorf("invalid array default element %d: %w", i, err)
		}
		commonTypes, err = updateArrayLiteralElementTypes(commonTypes, typedValueSQL{typ: elementType}, i)
		if err != nil {
			return "", err
		}
		elements[i] = elementSQL
	}
	elementTypes := normalizeArrayElementTypes(commonTypes)
	if err := d.config.ValidateArrayLiteralElementTypes(elementTypes); err != nil {
		return "", err
	}
	return d.config.ArrayLiteral(elements)
}

func (d *DataOperator) arrayLiteralToSQLParam(arr []interface{}, pc *params.ParamCollector) (string, error) {
	if err := d.config.ValidateArrayLiteralValue(arr); err != nil {
		return "", err
	}
	elements := make([]string, len(arr))
	var commonTypes []ExpressionType
	for i, elem := range arr {
		elementSQL, elementType, err := d.arrayLiteralElementSQLParam(elem, pc)
		if err != nil {
			return "", fmt.Errorf("invalid array default element %d: %w", i, err)
		}
		commonTypes, err = updateArrayLiteralElementTypes(commonTypes, typedValueSQL{typ: elementType}, i)
		if err != nil {
			return "", err
		}
		elements[i] = elementSQL
	}
	elementTypes := normalizeArrayElementTypes(commonTypes)
	if err := d.config.ValidateArrayLiteralElementTypes(elementTypes); err != nil {
		return "", err
	}
	return d.config.ArrayLiteral(elements)
}

func (d *DataOperator) arrayLiteralElementSQL(value interface{}) (string, ExpressionType, error) {
	if arr, ok := value.([]interface{}); ok {
		sql, err := d.arrayLiteralToSQL(arr)
		return sql, ExpressionTypeArray, err
	}
	sql, err := d.valueToSQL(value)
	return sql, inferLiteralValueExpressionType(value), err
}

func (d *DataOperator) arrayLiteralElementSQLParam(value interface{}, pc *params.ParamCollector) (string, ExpressionType, error) {
	if arr, ok := value.([]interface{}); ok {
		sql, err := d.arrayLiteralToSQLParam(arr, pc)
		return sql, ExpressionTypeArray, err
	}
	sql, err := d.valueToSQLParam(value, pc)
	return sql, inferLiteralValueExpressionType(value), err
}

func schemaExpressionType(schema SchemaProvider, fieldName string) ExpressionType {
	if schema == nil || fieldName == "" {
		return ExpressionTypeUnknown
	}
	switch {
	case schema.IsBooleanType(fieldName):
		return ExpressionTypeBoolean
	case schema.IsStringType(fieldName), schema.IsEnumType(fieldName):
		return ExpressionTypeString
	case schema.IsNumericType(fieldName):
		return ExpressionTypeNumber
	case schema.IsArrayType(fieldName):
		return ExpressionTypeArray
	case schema.GetFieldType(fieldName) == objectFieldType:
		return ExpressionTypeObject
	default:
		return ExpressionTypeUnknown
	}
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
				if !pv.FieldHasDefault {
					if err := validateProcessedValueDefault(d.schema(), pv, defaultValue); err != nil {
						return "", err
					}
				}
				defaultSQL, err := d.defaultValueToSQL(defaultValue)
				if err != nil {
					return "", fmt.Errorf("invalid default value: %w", err)
				}
				return d.config.CoalesceSQL(columnName, defaultSQL), nil
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
				if err := validateVarDefaultForField(d.schema(), varName, defaultValue); err != nil {
					return "", err
				}
				defaultSQL, err := d.defaultValueToSQL(defaultValue)
				if err != nil {
					return "", fmt.Errorf("invalid default value: %w", err)
				}
				return d.config.CoalesceSQL(columnName, defaultSQL), nil
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
	if _, ok := args[0].(string); ok {
		columnName, err := d.columnNameForFieldOperand(args[0], "missing operator argument must be a string or array of strings")
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s IS NULL", columnName), nil
	}
	if pv, ok := args[0].(ProcessedValue); ok && pv.IsSQL {
		columnName, err := d.columnNameForFieldOperand(pv, "missing operator argument must be a string or array of strings")
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
		for _, field := range varNames {
			columnName, err := d.columnNameForFieldOperand(field, "all variable names in missing must be strings")
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

	threshold := missingSomeMissingThreshold(minCount, len(varNames))
	if threshold > len(varNames) {
		return "FALSE", nil
	}

	// missing_some returns the missing field list only when fewer than minCount
	// fields are present. As a predicate, that is true when the missing-field
	// count reaches this derived threshold.
	var caseStatements []string
	for _, field := range varNames {
		columnName, err := d.columnNameForFieldOperand(field, "all variable names in missing_some must be strings")
		if err != nil {
			return "", err
		}
		caseStatements = append(caseStatements, fmt.Sprintf("CASE WHEN %s IS NULL THEN 1 ELSE 0 END", columnName))
	}
	if threshold == len(varNames) {
		var nullConditions []string
		for _, field := range varNames {
			columnName, err := d.columnNameForFieldOperand(field, "all variable names in missing_some must be strings")
			if err != nil {
				return "", err
			}
			nullConditions = append(nullConditions, fmt.Sprintf("%s IS NULL", columnName))
		}
		return fmt.Sprintf("(%s)", strings.Join(nullConditions, " AND ")), nil
	}

	nullCount := strings.Join(caseStatements, " + ")
	return fmt.Sprintf("(%s) >= %d", nullCount, threshold), nil
}

func missingSomeMissingThreshold(minCount float64, fieldCount int) int {
	threshold := int(math.Floor(float64(fieldCount)-minCount)) + 1
	if threshold < 1 {
		return 1
	}
	return threshold
}

// convertVarName converts a JSON Logic variable name to a SQL column reference.
// It splits the name on dots, quotes any segment that is not a valid unquoted SQL
// identifier (e.g. starts with a digit like "24h"), and rejoins with dots.
// Returns an error if any segment contains quote characters (backtick, double
// quote, or single quote), since the transpiler handles quoting automatically.
func (d *DataOperator) convertVarName(varName string) (string, error) {
	dl := dialect.DialectUnspecified
	if d.config != nil {
		dl = d.config.GetDialect()
	}

	needsQuoting := false
	forEachDottedSegment(varName, func(seg string) {
		if dialect.ContainsQuoteCharacters(seg) {
			needsQuoting = true
			return
		}
		if dialect.NeedsQuoting(seg) {
			needsQuoting = true
		}
	})
	if !needsQuoting {
		return varName, nil
	}

	var out strings.Builder
	out.Grow(len(varName) + 4)
	var quoteErr error
	firstSegment := true
	forEachDottedSegment(varName, func(seg string) {
		if quoteErr != nil {
			return
		}
		if !firstSegment {
			out.WriteByte('.')
		}
		firstSegment = false
		if dialect.ContainsQuoteCharacters(seg) {
			quoteErr = fmt.Errorf("variable name %q contains quote characters; "+
				"use raw identifiers — the transpiler handles quoting automatically", varName)
			return
		}
		if dialect.NeedsQuoting(seg) {
			out.WriteString(dialect.QuoteIdentifierSegment(seg, dl))
			return
		}
		out.WriteString(seg)
	})
	if quoteErr != nil {
		return "", quoteErr
	}
	return out.String(), nil
}

func forEachDottedSegment(name string, visit func(string)) {
	start := 0
	for {
		dot := strings.IndexByte(name[start:], '.')
		if dot < 0 {
			visit(name[start:])
			return
		}
		end := start + dot
		visit(name[start:end])
		start = end + 1
	}
}

func isValidIdentifierSegment(segment string) bool {
	return dialect.IsSafeIdentifierSegment(segment)
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
	case float32:
		if err := ValidateFiniteNativeFloat(v); err != nil {
			return "", err
		}
		return fmt.Sprintf("%v", v), nil
	case float64:
		if err := ValidateFiniteNativeFloat(v); err != nil {
			return "", err
		}
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
				if !pv.FieldHasDefault {
					if err := validateProcessedValueDefault(d.schema(), pv, defaultValue); err != nil {
						return "", err
					}
				}
				defaultSQL, err := d.defaultValueToSQLParam(defaultValue, pc)
				if err != nil {
					return "", fmt.Errorf("invalid default value: %w", err)
				}
				return d.config.CoalesceSQL(columnName, defaultSQL), nil
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
				if err := validateVarDefaultForField(d.schema(), varName, defaultValue); err != nil {
					return "", err
				}
				defaultSQL, err := d.defaultValueToSQLParam(defaultValue, pc)
				if err != nil {
					return "", fmt.Errorf("invalid default value: %w", err)
				}
				return d.config.CoalesceSQL(columnName, defaultSQL), nil
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

	threshold := missingSomeMissingThreshold(minCount, len(varNames))
	if threshold > len(varNames) {
		return "FALSE", nil
	}

	var caseStatements []string
	for _, field := range varNames {
		columnName, err := d.columnNameForFieldOperand(field, "all variable names in missing_some must be strings")
		if err != nil {
			return "", err
		}
		caseStatements = append(caseStatements, fmt.Sprintf("CASE WHEN %s IS NULL THEN 1 ELSE 0 END", columnName))
	}
	if threshold == len(varNames) {
		var nullConditions []string
		for _, field := range varNames {
			columnName, err := d.columnNameForFieldOperand(field, "all variable names in missing_some must be strings")
			if err != nil {
				return "", err
			}
			nullConditions = append(nullConditions, fmt.Sprintf("%s IS NULL", columnName))
		}
		return fmt.Sprintf("(%s)", strings.Join(nullConditions, " AND ")), nil
	}

	nullCount := strings.Join(caseStatements, " + ")
	return fmt.Sprintf("(%s) >= %s", nullCount, pc.Add(float64(threshold))), nil
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
		if err := ValidateFiniteNativeFloat(v); err != nil {
			return "", err
		}
		return pc.Add(float64(v)), nil
	case float64:
		if err := ValidateFiniteNativeFloat(v); err != nil {
			return "", err
		}
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

// ValidateFiniteNativeFloat rejects Go-native NaN/Infinity values before they
// can be emitted as SQL literals or driver bind values.
func ValidateFiniteNativeFloat(value interface{}) error {
	switch v := value.(type) {
	case float32:
		return validateFiniteFloat64(float64(v))
	case float64:
		return validateFiniteFloat64(v)
	default:
		return nil
	}
}

func validateFiniteFloat64(value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Errorf("non-finite float literal %v is not supported", value)
	}
	return nil
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
