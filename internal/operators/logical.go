package operators

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/params"
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

	condition = StripRedundantOuterParens(condition)
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

	// Try to extract field name for schema-aware type checking
	fieldName := l.extractVarFieldName(args[0])

	condition, err := l.expressionToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid !! argument: %w", err)
	}

	// If we have a schema and a field name, generate type-appropriate SQL
	if fieldName != "" && l.config != nil && l.config.Schema != nil {
		return l.generateTypeSafeTruthiness(condition, fieldName)
	}

	// Fallback: generic truthiness check for non-null/truthy values
	// This checks for non-null, non-false, non-zero, non-empty string
	return fmt.Sprintf("(%s IS NOT NULL AND %s != FALSE AND %s != 0 AND %s != '')",
		condition, condition, condition, condition), nil
}

// extractVarFieldName extracts the field name from a var expression.
// Returns empty string if the argument is not a simple var expression.
func (l *LogicalOperator) extractVarFieldName(arg interface{}) string {
	obj, ok := arg.(map[string]interface{})
	if !ok {
		return ""
	}

	if len(obj) != 1 {
		return ""
	}

	varArgs, ok := obj["var"]
	if !ok {
		return ""
	}

	// Handle {"var": "fieldName"} format
	if fieldName, ok := varArgs.(string); ok {
		return fieldName
	}

	// Handle {"var": ["fieldName", defaultValue]} format
	if arr, ok := varArgs.([]interface{}); ok && len(arr) > 0 {
		if fieldName, ok := arr[0].(string); ok {
			return fieldName
		}
	}

	return ""
}

// generateTypeSafeTruthiness generates type-appropriate SQL for truthiness check.
func (l *LogicalOperator) generateTypeSafeTruthiness(condition, fieldName string) (string, error) {
	schema := l.config.Schema

	// Check field type and generate appropriate SQL
	switch {
	case schema.IsBooleanType(fieldName):
		// For boolean fields: field IS TRUE
		// This is the cleanest check for boolean truthiness
		return fmt.Sprintf("%s IS TRUE", condition), nil

	case schema.IsStringType(fieldName):
		// For string fields: field IS NOT NULL AND field != ''
		return fmt.Sprintf("(%s IS NOT NULL AND %s != '')", condition, condition), nil

	case schema.IsNumericType(fieldName):
		// For numeric fields (integer/number): field IS NOT NULL AND field != 0
		return fmt.Sprintf("(%s IS NOT NULL AND %s != 0)", condition, condition), nil

	case schema.IsArrayType(fieldName):
		// For array fields: check non-null and non-empty
		lengthCheck := l.config.ArrayLengthFunc(condition)
		return fmt.Sprintf("(%s IS NOT NULL AND %s > 0)", condition, lengthCheck), nil

	default:
		// Unknown type or field not in schema: use generic check
		return fmt.Sprintf("(%s IS NOT NULL AND %s != FALSE AND %s != 0 AND %s != '')",
			condition, condition, condition, condition), nil
	}
}

// handleIf converts if operator to SQL.
func (l *LogicalOperator) handleIf(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("if requires at least 2 arguments")
	}

	// Handle nested IF statements (multiple condition/value pairs, with optional final else).
	if len(args) > 3 {
		var caseParts []string
		pairLimit := len(args)
		hasElse := len(args)%2 == 1
		if hasElse {
			pairLimit = len(args) - 1
		}

		// Process condition/value pairs
		for i := 0; i < pairLimit; i += 2 {
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

		if hasElse {
			// Handle final else value
			elseValue, err := l.expressionToSQL(args[len(args)-1])
			if err != nil {
				return "", fmt.Errorf("invalid if else value: %w", err)
			}

			return fmt.Sprintf("CASE %s ELSE %s END", strings.Join(caseParts, " "), elseValue), nil
		}

		return fmt.Sprintf("CASE %s END", strings.Join(caseParts, " ")), nil
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
				// Try to use the expression parser callback for unknown operators
				// This enables support for custom operators in nested contexts
				if l.config != nil && l.config.HasExpressionParser() {
					return l.config.ParseExpression(obj, "$")
				}
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

outer:
	for i, arg := range args {
		// If it's a complex expression, convert it to SQL
		if exprMap, ok := arg.(map[string]interface{}); ok {
			if len(exprMap) != 1 {
				return nil, fmt.Errorf("operator object must have exactly one key")
			}
			// Check if it's a complex expression (not just a var)
			for operator := range exprMap {
				if operator != "var" {
					// It's a complex expression, convert it to SQL
					sql, err := l.expressionToSQL(arg)
					if err != nil {
						return nil, fmt.Errorf("invalid argument %d: %w", i, err)
					}
					// Store as SQL fragment so comparison does not quote it as a literal.
					processed[i] = ProcessedValue{Value: sql, IsSQL: true}
					continue outer
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
	case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number, bool:
		return true
	case nil:
		return true
	default:
		return false
	}
}

// ToSQLParam is the parameterized variant of ToSQL. Keep in sync.
func (l *LogicalOperator) ToSQLParam(operator string, args []interface{}, pc *params.ParamCollector) (string, error) {
	switch operator {
	case "and":
		return l.handleAndParam(args, pc)
	case "or":
		return l.handleOrParam(args, pc)
	case "!":
		return l.handleNotParam(args, pc)
	case "!!":
		return l.handleDoubleNotParam(args, pc)
	case "if":
		return l.handleIfParam(args, pc)
	default:
		return "", fmt.Errorf("unsupported logical operator: %s", operator)
	}
}

// handleAndParam is the parameterized variant of handleAnd. Keep in sync.
func (l *LogicalOperator) handleAndParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("and operator requires at least 1 argument")
	}
	var conditions []string
	for i, arg := range args {
		condition, err := l.expressionToSQLParam(arg, pc)
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

// handleOrParam is the parameterized variant of handleOr. Keep in sync.
func (l *LogicalOperator) handleOrParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("or operator requires at least 1 argument")
	}
	var conditions []string
	for i, arg := range args {
		condition, err := l.expressionToSQLParam(arg, pc)
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

// handleNotParam is the parameterized variant of handleNot. Keep in sync.
func (l *LogicalOperator) handleNotParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("! operator requires exactly 1 argument")
	}
	condition, err := l.expressionToSQLParam(args[0], pc)
	if err != nil {
		return "", fmt.Errorf("invalid ! argument: %w", err)
	}
	condition = StripRedundantOuterParens(condition)
	return fmt.Sprintf("NOT (%s)", condition), nil
}

// handleDoubleNotParam is the parameterized variant of handleDoubleNot. Keep in sync.
func (l *LogicalOperator) handleDoubleNotParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("!! operator requires exactly 1 argument")
	}

	if arr, ok := args[0].([]interface{}); ok {
		if len(arr) == 0 {
			return "FALSE", nil
		}
		return "TRUE", nil
	}

	fieldName := l.extractVarFieldName(args[0])

	condition, err := l.expressionToSQLParam(args[0], pc)
	if err != nil {
		return "", fmt.Errorf("invalid !! argument: %w", err)
	}

	if fieldName != "" && l.config != nil && l.config.Schema != nil {
		return l.generateTypeSafeTruthiness(condition, fieldName)
	}

	return fmt.Sprintf("(%s IS NOT NULL AND %s != FALSE AND %s != 0 AND %s != '')",
		condition, condition, condition, condition), nil
}

// handleIfParam is the parameterized variant of handleIf. Keep in sync.
func (l *LogicalOperator) handleIfParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("if requires at least 2 arguments")
	}

	if len(args) > 3 {
		var caseParts []string
		pairLimit := len(args)
		hasElse := len(args)%2 == 1
		if hasElse {
			pairLimit = len(args) - 1
		}

		for i := 0; i < pairLimit; i += 2 {
			condition, err := l.expressionToSQLParam(args[i], pc)
			if err != nil {
				return "", fmt.Errorf("invalid if condition %d: %w", i/2, err)
			}
			value, err := l.expressionToSQLParam(args[i+1], pc)
			if err != nil {
				return "", fmt.Errorf("invalid if value %d: %w", i/2, err)
			}
			caseParts = append(caseParts, fmt.Sprintf("WHEN %s THEN %s", condition, value))
		}
		if hasElse {
			elseValue, err := l.expressionToSQLParam(args[len(args)-1], pc)
			if err != nil {
				return "", fmt.Errorf("invalid if else value: %w", err)
			}
			return fmt.Sprintf("CASE %s ELSE %s END", strings.Join(caseParts, " "), elseValue), nil
		}
		return fmt.Sprintf("CASE %s END", strings.Join(caseParts, " ")), nil
	}

	condition, err := l.expressionToSQLParam(args[0], pc)
	if err != nil {
		return "", fmt.Errorf("invalid if condition: %w", err)
	}
	thenValue, err := l.expressionToSQLParam(args[1], pc)
	if err != nil {
		return "", fmt.Errorf("invalid if then value: %w", err)
	}
	if len(args) == 3 {
		elseValue, err := l.expressionToSQLParam(args[2], pc)
		if err != nil {
			return "", fmt.Errorf("invalid if else value: %w", err)
		}
		return fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, thenValue, elseValue), nil
	}
	return fmt.Sprintf("CASE WHEN %s THEN %s ELSE NULL END", condition, thenValue), nil
}

// expressionToSQLParam is the parameterized variant of expressionToSQL. Keep in sync.
func (l *LogicalOperator) expressionToSQLParam(expr interface{}, pc *params.ParamCollector) (string, error) {
	if pv, ok := expr.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		return l.dataOp.valueToSQLParam(pv.Value, pc)
	}

	if l.isPrimitive(expr) {
		return l.dataOp.valueToSQLParam(expr, pc)
	}

	if _, ok := expr.([]interface{}); ok {
		return "", fmt.Errorf("arrays not supported in logical expressions")
	}

	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return "", fmt.Errorf("operator object must have exactly one key")
		}

		for operator, args := range obj {
			switch operator {
			case "var", "missing":
				return l.dataOp.ToSQLParam(operator, []interface{}{args}, pc)
			case "missing_some":
				if arr, ok := args.([]interface{}); ok {
					return l.dataOp.ToSQLParam(operator, arr, pc)
				}
				return "", fmt.Errorf("missing_some operator requires array arguments")
			case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
				if arr, ok := args.([]interface{}); ok {
					processedArgs, err := l.processArgsParam(arr, pc)
					if err != nil {
						return "", fmt.Errorf("failed to process comparison arguments: %w", err)
					}
					return l.comparisonOp.ToSQLParam(operator, processedArgs, pc)
				}
				return "", fmt.Errorf("comparison operator requires array arguments")
			case "and", "or", "if":
				if arr, ok := args.([]interface{}); ok {
					return l.ToSQLParam(operator, arr, pc)
				}
				return "", fmt.Errorf("logical operator requires array arguments")
			case "!", "!!":
				if arr, ok := args.([]interface{}); ok {
					return l.ToSQLParam(operator, arr, pc)
				}
				return l.ToSQLParam(operator, []interface{}{args}, pc)
			case "+", "-", "*", "/", "%", "max", "min":
				if arr, ok := args.([]interface{}); ok {
					numericOp := NewNumericOperator(l.config)
					return numericOp.ToSQLParam(operator, arr, pc)
				}
				return "", fmt.Errorf("numeric operator requires array arguments")
			case "cat", "substr":
				if arr, ok := args.([]interface{}); ok {
					stringOp := NewStringOperator(l.config)
					return stringOp.ToSQLParam(operator, arr, pc)
				}
				return "", fmt.Errorf("string operator requires array arguments")
			case "map", "filter", "reduce", "all", "some", "none", "merge":
				if arr, ok := args.([]interface{}); ok {
					arrayOp := NewArrayOperator(l.config)
					return arrayOp.ToSQLParam(operator, arr, pc)
				}
				return "", fmt.Errorf("array operator requires array arguments")
			default:
				if l.config != nil && l.config.HasParamExpressionParser() {
					return l.config.ParseExpressionParam(obj, "$", pc)
				}
				return "", fmt.Errorf("unsupported operator in logical expression: %s", operator)
			}
		}
	}

	return "", fmt.Errorf("invalid expression type: %T", expr)
}

// processArgsParam is the parameterized variant of processArgs. Keep in sync.
func (l *LogicalOperator) processArgsParam(args []interface{}, pc *params.ParamCollector) ([]interface{}, error) {
	processed := make([]interface{}, len(args))
outer:
	for i, arg := range args {
		if exprMap, ok := arg.(map[string]interface{}); ok {
			if len(exprMap) != 1 {
				return nil, fmt.Errorf("operator object must have exactly one key")
			}
			for operator := range exprMap {
				if operator != "var" {
					sql, err := l.expressionToSQLParam(arg, pc)
					if err != nil {
						return nil, fmt.Errorf("invalid argument %d: %w", i, err)
					}
					processed[i] = ProcessedValue{Value: sql, IsSQL: true}
					continue outer
				}
			}
			processed[i] = arg
			continue
		}
		processed[i] = arg
	}
	return processed, nil
}
