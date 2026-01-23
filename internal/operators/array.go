package operators

import (
	"fmt"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

// ArrayOperator handles array operations like map, filter, reduce, all, some, none, merge.
type ArrayOperator struct {
	config       *OperatorConfig
	dataOp       *DataOperator
	comparisonOp *ComparisonOperator
	logicalOp    *LogicalOperator
	numericOp    *NumericOperator
}

// NewArrayOperator creates a new ArrayOperator instance with optional config.
func NewArrayOperator(config *OperatorConfig) *ArrayOperator {
	return &ArrayOperator{
		config:       config,
		dataOp:       NewDataOperator(config),
		comparisonOp: NewComparisonOperator(config),
		logicalOp:    nil, // Will be created lazily
		numericOp:    NewNumericOperator(config),
	}
}

// schema returns the schema from config, or nil if not configured.
func (a *ArrayOperator) schema() SchemaProvider {
	if a.config == nil {
		return nil
	}
	return a.config.Schema
}

// getDialect returns the configured dialect, or DialectUnspecified if not configured.
func (a *ArrayOperator) getDialect() dialect.Dialect {
	if a.config == nil {
		return dialect.DialectUnspecified
	}
	return a.config.GetDialect()
}

// getLogicalOperator returns the logical operator, creating it lazily if needed.
func (a *ArrayOperator) getLogicalOperator() *LogicalOperator {
	if a.logicalOp == nil {
		a.logicalOp = NewLogicalOperator(a.config) // Config already has schema
	}
	return a.logicalOp
}

// validateArrayOperand checks if a field used in an array operation is of array type.
func (a *ArrayOperator) validateArrayOperand(value interface{}) error {
	if a.schema() == nil {
		return nil // No schema, no validation
	}

	// If it's a literal array, it's valid
	if _, ok := value.([]interface{}); ok {
		return nil
	}

	fieldName := a.extractFieldNameFromValue(value)
	if fieldName == "" {
		return nil // Can't determine field name, skip validation
	}

	fieldType := a.schema().GetFieldType(fieldName)
	if fieldType == "" {
		return nil // Field not in schema, skip validation (existence checked by DataOperator)
	}

	if !a.schema().IsArrayType(fieldName) {
		return fmt.Errorf("array operation on non-array field '%s' (type: %s)", fieldName, fieldType)
	}

	return nil
}

// extractFieldNameFromValue extracts field name from a value that might be a var expression.
func (a *ArrayOperator) extractFieldNameFromValue(value interface{}) string {
	if varExpr, ok := value.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			return a.extractFieldName(varName)
		}
	}
	return ""
}

// extractFieldName extracts the field name from a var argument.
func (a *ArrayOperator) extractFieldName(varName interface{}) string {
	if nameStr, ok := varName.(string); ok {
		return nameStr
	}
	if nameArr, ok := varName.([]interface{}); ok && len(nameArr) > 0 {
		if nameStr, ok := nameArr[0].(string); ok {
			return nameStr
		}
	}
	return ""
}

// ToSQL converts an array operation to SQL.
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

// handleMap converts map operator to SQL.
// Generates: ARRAY(SELECT transformation FROM UNNEST(array) AS elem).
// For ClickHouse: Uses arrayMap or subquery with arrayJoin.
func (a *ArrayOperator) handleMap(args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("map requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("map"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[0]); err != nil {
		return "", err
	}

	// First argument: array
	array, err := a.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid map array argument: %w", err)
	}

	// Second argument: transformation expression
	transformation, err := a.expressionToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid map transformation argument: %w", err)
	}

	// Replace element reference in transformation
	transformationWithElem := a.replaceElementReference(transformation)

	// Generate SQL based on dialect
	//nolint:exhaustive // default handles BigQuery/Spanner/PostgreSQL/DuckDB
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("arrayMap(elem -> %s, %s)", transformationWithElem, array), nil
	default:
		return fmt.Sprintf("ARRAY(SELECT %s FROM UNNEST(%s) AS elem)", transformationWithElem, array), nil
	}
}

// handleFilter converts filter operator to SQL.
// Generates: ARRAY(SELECT elem FROM UNNEST(array) AS elem WHERE condition).
// For ClickHouse: Uses arrayFilter function.
func (a *ArrayOperator) handleFilter(args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("filter requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("filter"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[0]); err != nil {
		return "", err
	}

	// First argument: array
	array, err := a.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid filter array argument: %w", err)
	}

	// Second argument: condition expression
	condition, err := a.expressionToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid filter condition argument: %w", err)
	}

	// Replace element reference in condition
	conditionWithElem := a.replaceElementReference(condition)

	// Generate SQL based on dialect
	//nolint:exhaustive // default handles BigQuery/Spanner/PostgreSQL/DuckDB
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("arrayFilter(elem -> %s, %s)", conditionWithElem, array), nil
	default:
		return fmt.Sprintf("ARRAY(SELECT elem FROM UNNEST(%s) AS elem WHERE %s)", array, conditionWithElem), nil
	}
}

// handleReduce converts reduce operator to SQL.
// JSONLogic reduce: {"reduce": [array, reducer_expr, initial]}.
// The reducer expression uses "accumulator" and "current" variables.
//
// For common patterns, this generates optimized SQL:
// - Addition: initial + COALESCE((SELECT SUM(elem) FROM UNNEST(array) AS elem), 0).
// - General: (SELECT reducer FROM UNNEST(array) AS elem).
// For ClickHouse: Uses arrayReduce function for aggregates.
func (a *ArrayOperator) handleReduce(args []interface{}) (string, error) {
	if len(args) != 3 {
		return "", fmt.Errorf("reduce requires exactly 3 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("reduce"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[0]); err != nil {
		return "", err
	}

	// First argument: array
	array, err := a.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid reduce array argument: %w", err)
	}

	// Second argument: reducer expression
	reducerExpr := args[1]

	// Third argument: initial value
	initial, err := a.valueToSQL(args[2])
	if err != nil {
		return "", fmt.Errorf("invalid reduce initial argument: %w", err)
	}

	// Check for common reduction patterns and optimize
	if aggregateFunc := a.detectAggregatePattern(reducerExpr); aggregateFunc != "" {
		// Generate optimized aggregate SQL based on dialect
		//nolint:exhaustive // default handles BigQuery/Spanner/PostgreSQL/DuckDB
		switch a.getDialect() {
		case dialect.DialectClickHouse:
			// ClickHouse: initial + coalesce(arrayReduce('sum', array), 0)
			return fmt.Sprintf("%s + coalesce(arrayReduce('%s', %s), 0)",
				initial, strings.ToLower(aggregateFunc), array), nil
		default:
			// Standard SQL: initial + COALESCE((SELECT AGG(elem) FROM UNNEST(array) AS elem), 0)
			return fmt.Sprintf("%s + COALESCE((SELECT %s(elem) FROM UNNEST(%s) AS elem), 0)",
				initial, aggregateFunc, array), nil
		}
	}

	// General case: evaluate reducer expression with element reference
	reducer, err := a.expressionToSQL(reducerExpr)
	if err != nil {
		return "", fmt.Errorf("invalid reduce expression: %w", err)
	}

	// Replace variable references
	reducerWithElem := a.replaceElementReference(reducer)
	// Replace accumulator with initial for the subquery context
	reducerWithElem = strings.ReplaceAll(reducerWithElem, AccumulatorVar, initial)

	// Generate SQL based on dialect
	//nolint:exhaustive // default handles BigQuery/Spanner/PostgreSQL/DuckDB
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		// ClickHouse uses arrayFold for general reduction (ClickHouse 22.8+)
		return fmt.Sprintf("arrayFold((acc, elem) -> %s, %s, %s)", reducerWithElem, array, initial), nil
	default:
		// Standard SQL using a subquery
		return fmt.Sprintf("(SELECT %s FROM UNNEST(%s) AS elem)", reducerWithElem, array), nil
	}
}

// detectAggregatePattern checks if the reducer expression matches a common aggregate pattern.
// Returns the SQL aggregate function name if detected, empty string otherwise.
func (a *ArrayOperator) detectAggregatePattern(expr interface{}) string {
	exprMap, ok := expr.(map[string]interface{})
	if !ok {
		return ""
	}

	// Check for addition pattern: {"+": [{"var": "accumulator"}, {"var": "current"}]}
	if args, hasPlus := exprMap[OpAdd]; hasPlus {
		if a.isAccumulatorCurrentPattern(args) {
			return AggregateSUM
		}
	}

	// Check for min pattern: {"min": [{"var": "accumulator"}, {"var": "current"}]}
	if args, hasMin := exprMap[OpMin]; hasMin {
		if a.isAccumulatorCurrentPattern(args) {
			return AggregateMIN
		}
	}

	// Check for max pattern: {"max": [{"var": "accumulator"}, {"var": "current"}]}
	if args, hasMax := exprMap[OpMax]; hasMax {
		if a.isAccumulatorCurrentPattern(args) {
			return AggregateMAX
		}
	}

	return ""
}

// isAccumulatorCurrentPattern checks if args match [{"var": "accumulator"}, {"var": "current"}].
func (a *ArrayOperator) isAccumulatorCurrentPattern(args interface{}) bool {
	argsArr, ok := args.([]interface{})
	if !ok || len(argsArr) != 2 {
		return false
	}

	// Check first arg is {"var": "accumulator"}
	arg0Map, ok := argsArr[0].(map[string]interface{})
	if !ok {
		return false
	}
	if varName, hasVar := arg0Map[OpVar]; !hasVar || varName != AccumulatorVar {
		return false
	}

	// Check second arg is {"var": "current"}
	arg1Map, ok := argsArr[1].(map[string]interface{})
	if !ok {
		return false
	}
	if varName, hasVar := arg1Map[OpVar]; !hasVar || varName != CurrentVar {
		return false
	}

	return true
}

// handleAll converts all operator to SQL.
// This checks if all elements in an array satisfy a condition.
// Generates: NOT EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE NOT (condition)).
// For ClickHouse: Uses arrayAll function.
func (a *ArrayOperator) handleAll(args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("all requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("all"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[0]); err != nil {
		return "", err
	}

	// First argument: array
	array, err := a.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid all array argument: %w", err)
	}

	// Second argument: condition expression
	condition, err := a.expressionToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid all condition argument: %w", err)
	}

	// Replace 'elem' in condition with the actual element reference
	conditionWithElem := a.replaceElementReference(condition)

	// Generate SQL based on dialect
	//nolint:exhaustive // default handles BigQuery/Spanner/PostgreSQL/DuckDB
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("arrayAll(elem -> %s, %s)", conditionWithElem, array), nil
	default:
		// Standard SQL: NOT EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE NOT (condition))
		return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM UNNEST(%s) AS elem WHERE NOT (%s))", array, conditionWithElem), nil
	}
}

// handleSome converts some operator to SQL.
// This checks if some elements in an array satisfy a condition.
// Generates: EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE condition).
// For ClickHouse: Uses arrayExists function.
func (a *ArrayOperator) handleSome(args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("some requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("some"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[0]); err != nil {
		return "", err
	}

	// First argument: array
	array, err := a.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid some array argument: %w", err)
	}

	// Second argument: condition expression
	condition, err := a.expressionToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid some condition argument: %w", err)
	}

	// Replace element reference in condition
	conditionWithElem := a.replaceElementReference(condition)

	// Generate SQL based on dialect
	//nolint:exhaustive // default handles BigQuery/Spanner/PostgreSQL/DuckDB
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("arrayExists(elem -> %s, %s)", conditionWithElem, array), nil
	default:
		// Standard SQL: EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE condition)
		return fmt.Sprintf("EXISTS (SELECT 1 FROM UNNEST(%s) AS elem WHERE %s)", array, conditionWithElem), nil
	}
}

// handleNone converts none operator to SQL.
// This checks if no elements in an array satisfy a condition.
// Generates: NOT EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE condition).
// For ClickHouse: Uses NOT arrayExists function.
func (a *ArrayOperator) handleNone(args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("none requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("none"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[0]); err != nil {
		return "", err
	}

	// First argument: array
	array, err := a.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid none array argument: %w", err)
	}

	// Second argument: condition expression
	condition, err := a.expressionToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid none condition argument: %w", err)
	}

	// Replace element reference in condition
	conditionWithElem := a.replaceElementReference(condition)

	// Generate SQL based on dialect
	//nolint:exhaustive // default handles BigQuery/Spanner/PostgreSQL/DuckDB
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("NOT arrayExists(elem -> %s, %s)", conditionWithElem, array), nil
	default:
		// Standard SQL: NOT EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE condition)
		return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM UNNEST(%s) AS elem WHERE %s)", array, conditionWithElem), nil
	}
}

// handleMerge converts merge operator to SQL.
// This merges multiple arrays into one.
// BigQuery/Spanner: ARRAY_CONCAT(array1, array2, ...)
// PostgreSQL: array1 || array2 || ...
func (a *ArrayOperator) handleMerge(args []interface{}) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("merge requires at least 1 argument")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("merge"); err != nil {
			return "", err
		}
	}

	// Validate that all arguments are array types
	for _, arg := range args {
		if err := a.validateArrayOperand(arg); err != nil {
			return "", err
		}
	}

	// Convert all array arguments to SQL
	arrays := make([]string, len(args))
	for i, arg := range args {
		array, err := a.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid merge array argument %d: %w", i, err)
		}
		arrays[i] = array
	}

	// Generate SQL based on dialect
	d := dialect.DialectUnspecified
	if a.config != nil {
		d = a.config.GetDialect()
	}

	switch d {
	case dialect.DialectPostgreSQL:
		// PostgreSQL: Use || operator for array concatenation
		if len(arrays) == 1 {
			return arrays[0], nil
		}
		return fmt.Sprintf("(%s)", strings.Join(arrays, " || ")), nil
	case dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectDuckDB:
		// BigQuery/Spanner/DuckDB: Use ARRAY_CONCAT function
		return fmt.Sprintf("ARRAY_CONCAT(%s)", strings.Join(arrays, ", ")), nil
	case dialect.DialectClickHouse:
		// ClickHouse: Use arrayConcat function
		return fmt.Sprintf("arrayConcat(%s)", strings.Join(arrays, ", ")), nil
	case dialect.DialectUnspecified:
		return "", fmt.Errorf("merge: dialect not specified")
	default:
		return "", fmt.Errorf("merge: unsupported dialect %s", d)
	}
}

// valueToSQL converts a value to SQL, handling var expressions, arrays, and literals.
func (a *ArrayOperator) valueToSQL(value interface{}) (string, error) {
	// Handle ProcessedValue (pre-processed SQL from parser)
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		// It's a literal, convert it
		return a.dataOp.valueToSQL(pv.Value)
	}

	// Handle complex expressions (operators)
	if expr, ok := value.(map[string]interface{}); ok {
		// Check if it's a var expression
		if varExpr, hasVar := expr[OpVar]; hasVar {
			return a.dataOp.ToSQL(OpVar, []interface{}{varExpr})
		}
		// Otherwise, it's a complex expression - convert it using expressionToSQL
		return a.expressionToSQL(value)
	}

	// Handle arrays
	if arr, ok := value.([]interface{}); ok {
		// Convert array elements to SQL literals
		elements := make([]string, len(arr))
		for i, elem := range arr {
			elementSQL, err := a.dataOp.valueToSQL(elem)
			if err != nil {
				return "", fmt.Errorf("invalid array element %d: %w", i, err)
			}
			elements[i] = elementSQL
		}
		return fmt.Sprintf("[%s]", strings.Join(elements, " ")), nil
	}

	// Handle primitive values
	return a.dataOp.valueToSQL(value)
}

// expressionToSQL converts a JSON Logic expression to SQL.
func (a *ArrayOperator) expressionToSQL(expr interface{}) (string, error) {
	// Handle ProcessedValue (pre-processed SQL from parser)
	if pv, ok := expr.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		// It's a literal, recursively convert it
		return a.expressionToSQL(pv.Value)
	}

	// Handle primitive values
	if a.isPrimitive(expr) {
		return a.dataOp.valueToSQL(expr)
	}

	// Handle var expressions
	if varExpr, ok := expr.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			// Special case: empty var name represents the current element in array operations
			if varName == "" {
				return ElemVar, nil
			}
			return a.dataOp.ToSQL(OpVar, []interface{}{varName})
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
			case "and", "or", "!", "!!", "if":
				if arr, ok := args.([]interface{}); ok {
					return a.getLogicalOperator().ToSQL(operator, arr)
				}
			case "+", "-", "*", "/", "%", "max", "min":
				if arr, ok := args.([]interface{}); ok {
					return a.numericOp.ToSQL(operator, arr)
				}
			case "map", "filter", "reduce", "all", "some", "none", "merge":
				// Handle nested array operators
				if arr, ok := args.([]interface{}); ok {
					return a.ToSQL(operator, arr)
				}
			default:
				// Try to use the expression parser callback for unknown operators
				// This enables support for custom operators in nested contexts
				if a.config != nil && a.config.HasExpressionParser() {
					return a.config.ParseExpression(exprMap, "$")
				}
				return "", fmt.Errorf("unsupported operator in array expression: %s", operator)
			}
		}
	}

	return "", fmt.Errorf("invalid expression type: %T", expr)
}

// replaceElementReference replaces element references in conditions.
// For now, this is a simple implementation that assumes the condition uses a variable.
// It replaces "item" and "current" references with "elem" for UNNEST subqueries.
func (a *ArrayOperator) replaceElementReference(condition string) string {
	// Replace variable references in the condition with the element name
	// This handles cases where {"var": "item"} or {"var": "current"} should become "elem"
	// Simple string replacement for now - in a more complex implementation,
	// you'd want to parse the SQL and replace variable references properly
	result := strings.ReplaceAll(condition, ItemVar, ElemVar)
	result = strings.ReplaceAll(result, CurrentVar, ElemVar)
	return result
}

// isPrimitive checks if a value is a primitive type.
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
