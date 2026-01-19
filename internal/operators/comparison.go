package operators

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

// ComparisonOperator handles comparison operators (==, ===, !=, !==, >, >=, <, <=).
type ComparisonOperator struct {
	config *OperatorConfig
	dataOp *DataOperator
}

// NewComparisonOperator creates a new comparison operator with optional config.
func NewComparisonOperator(config *OperatorConfig) *ComparisonOperator {
	return &ComparisonOperator{
		config: config,
		dataOp: NewDataOperator(config), // Same config, no propagation needed
	}
}

// schema returns the schema from config, or nil if not configured.
func (c *ComparisonOperator) schema() SchemaProvider {
	if c.config == nil {
		return nil
	}
	return c.config.Schema
}

// strposFunc returns the appropriate string position function call based on dialect.
// BigQuery/Spanner/DuckDB: STRPOS(haystack, needle)
// PostgreSQL: POSITION(needle IN haystack)
// ClickHouse: position(haystack, needle).
func (c *ComparisonOperator) strposFunc(haystack, needle string) string {
	d := dialect.DialectUnspecified
	if c.config != nil {
		d = c.config.GetDialect()
	}

	//nolint:exhaustive // default handles BigQuery/Spanner/DuckDB
	switch d {
	case dialect.DialectPostgreSQL:
		return fmt.Sprintf("POSITION(%s IN %s)", needle, haystack)
	case dialect.DialectClickHouse:
		return fmt.Sprintf("position(%s, %s)", haystack, needle)
	default:
		return fmt.Sprintf("STRPOS(%s, %s)", haystack, needle)
	}
}

// validateOrderingOperand checks if a field used in an ordering comparison is of a valid type
// Only numeric and string types support ordering comparisons (>, >=, <, <=)
// Rejects array, object, and boolean types.
func (c *ComparisonOperator) validateOrderingOperand(value interface{}, operator string) error {
	if c.schema() == nil {
		return nil // No schema, no validation
	}

	fieldName := c.extractFieldNameFromValue(value)
	if fieldName == "" {
		return nil // Can't determine field name, skip validation
	}

	fieldType := c.schema().GetFieldType(fieldName)
	if fieldType == "" {
		return nil // Field not in schema, skip validation (existence checked by DataOperator)
	}

	// Allow numeric and string types for ordering comparisons
	if c.schema().IsNumericType(fieldName) || c.schema().IsStringType(fieldName) {
		return nil
	}

	// Disallow array, object, boolean for ordering comparisons
	return fmt.Errorf("ordering comparison '%s' on incompatible field '%s' (type: %s)", operator, fieldName, fieldType)
}

// extractFieldNameFromValue extracts field name from a value that might be a var expression.
func (c *ComparisonOperator) extractFieldNameFromValue(value interface{}) string {
	if varExpr, ok := value.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			return c.extractFieldName(varName)
		}
	}
	return ""
}

// extractFieldName extracts the field name from a var argument.
func (c *ComparisonOperator) extractFieldName(varName interface{}) string {
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

// coerceValueForComparison coerces a literal value based on the type of the field being compared.
// If the field is numeric and the value is a string that represents a number, it returns the unquoted number.
// This ensures proper SQL comparisons like "field >= 50000" instead of "field >= '50000'".
func (c *ComparisonOperator) coerceValueForComparison(value interface{}, fieldName string) interface{} {
	if c.schema() == nil || fieldName == "" {
		return value
	}

	// Only coerce if the field is a numeric type
	if !c.schema().IsNumericType(fieldName) {
		return value
	}

	// If value is a string, try to parse it as a number
	if strVal, ok := value.(string); ok {
		// Try to parse as integer first
		if intVal, err := strconv.ParseInt(strVal, 10, 64); err == nil {
			return intVal
		}
		// Try to parse as float
		if floatVal, err := strconv.ParseFloat(strVal, 64); err == nil {
			return floatVal
		}
	}

	return value
}

// validateEnumValue validates that a value is valid for an enum field.
// Returns nil if valid or if not an enum field.
func (c *ComparisonOperator) validateEnumValue(value interface{}, fieldName string) error {
	if c.schema() == nil || fieldName == "" {
		return nil
	}

	// Skip validation for null values (null is valid for any field)
	if value == nil {
		return nil
	}

	// Only validate if the field is an enum type
	if !c.schema().IsEnumType(fieldName) {
		return nil
	}

	// Extract string value for validation
	var strVal string
	switch v := value.(type) {
	case string:
		strVal = v
	default:
		// Non-string values for enum comparison - convert to string for validation
		strVal = fmt.Sprintf("%v", v)
	}

	return c.schema().ValidateEnumValue(fieldName, strVal)
}

// ToSQL converts a comparison operator to SQL.
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
			return "", fmt.Errorf("invalid left operand: %w", err)
		}
		// Pass the original left arg for enum validation
		return c.handleIn(leftSQL, args[1], args[0])
	}

	// Apply type coercion based on schema
	// If one side is a field and the other is a literal, coerce the literal to match the field type
	leftArg := args[0]
	rightArg := args[1]

	leftFieldName := c.extractFieldNameFromValue(leftArg)
	rightFieldName := c.extractFieldNameFromValue(rightArg)

	// If left is a field and right is a literal, coerce right based on left's type
	if leftFieldName != "" && rightFieldName == "" {
		rightArg = c.coerceValueForComparison(rightArg, leftFieldName)
		// Validate enum value if left is an enum field
		if err := c.validateEnumValue(rightArg, leftFieldName); err != nil {
			return "", err
		}
	}
	// If right is a field and left is a literal, coerce left based on right's type
	if rightFieldName != "" && leftFieldName == "" {
		leftArg = c.coerceValueForComparison(leftArg, rightFieldName)
		// Validate enum value if right is an enum field
		if err := c.validateEnumValue(leftArg, rightFieldName); err != nil {
			return "", err
		}
	}

	leftSQL, err := c.valueToSQL(leftArg)
	if err != nil {
		return "", fmt.Errorf("invalid left operand: %w", err)
	}

	rightSQL, err := c.valueToSQL(rightArg)
	if err != nil {
		return "", fmt.Errorf("invalid right operand: %w", err)
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
	case ">", ">=", "<", "<=":
		// Validate operands for ordering comparisons
		if err := c.validateOrderingOperand(args[0], operator); err != nil {
			return "", err
		}
		if err := c.validateOrderingOperand(args[1], operator); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s %s %s", leftSQL, operator, rightSQL), nil
	default:
		return "", fmt.Errorf("unsupported comparison operator: %s", operator)
	}
}

// valueToSQL converts a value to SQL, handling both literals and var expressions.
func (c *ComparisonOperator) valueToSQL(value interface{}) (string, error) {
	// Check if it's a ProcessedValue (pre-processed SQL from parser)
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		// It's a literal, quote it
		return c.dataOp.valueToSQL(pv.Value)
	}

	// Check if it's a var expression
	if varExpr, ok := value.(map[string]interface{}); ok {
		if len(varExpr) == 1 {
			for operator, args := range varExpr {
				if operator == "var" {
					// Special case: empty var name represents the current element in array operations
					if varName, ok := args.(string); ok && varName == "" {
						return "elem", nil
					}
					return c.dataOp.ToSQL(OpVar, []interface{}{args})
				}
			}
		}
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
						logicalOp := NewLogicalOperator(c.config)
						return logicalOp.ToSQL("if", arr)
					}
					return "", fmt.Errorf("if operator requires array arguments")
				case "reduce", "filter", "map", "some", "all", "none", "merge":
					// Array operators should have been pre-processed by the parser/logical operator
					// If we see them here, it means they weren't processed correctly
					// Try to process them directly as a fallback
					if arr, ok := args.([]interface{}); ok {
						arrayOp := NewArrayOperator(c.config)
						return arrayOp.ToSQL(op, arr)
					}
					return "", fmt.Errorf("array operator %s requires array arguments", op)
				case "cat", "substr":
					// Handle string operators
					if arr, ok := args.([]interface{}); ok {
						stringOp := NewStringOperator(c.config)
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
// leftOriginal is the original left argument (before SQL conversion) for enum validation.
func (c *ComparisonOperator) handleIn(leftSQL string, rightValue, leftOriginal interface{}) (string, error) {
	// Extract field name from left side for enum validation
	leftFieldName := c.extractFieldNameFromValue(leftOriginal)

	// Check if right side is a variable expression
	if varExpr, ok := rightValue.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			// Handle variable on right side
			// According to JSON Logic spec, "in" supports both:
			// 1. Array membership: {"in": [value, array]} → value IN array
			// 2. String containment: {"in": [substring, string]} → substring contained in string
			//
			// Use schema to determine the correct SQL:
			// - If variable is an ARRAY column: 'value' IN column (array membership)
			// - If variable is a STRING column: STRPOS(column, 'value') > 0 (string containment)
			rightSQL, err := c.dataOp.ToSQL(OpVar, []interface{}{varName})
			if err != nil {
				return "", fmt.Errorf("invalid variable in IN operator: %w", err)
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
			if c.schema() != nil && fieldName != "" {
				if c.schema().IsArrayType(fieldName) {
					// Array type: use array membership syntax
					return fmt.Sprintf("%s IN %s", leftSQL, rightSQL), nil
				} else if c.schema().IsStringType(fieldName) {
					// String type: use string containment syntax
					return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, leftSQL)), nil
				}
			}

			// No schema or unknown type: use heuristic based on left side
			// If left side is a literal (quoted), assume string containment
			isLeftLiteral := strings.HasPrefix(leftSQL, "'") && strings.HasSuffix(leftSQL, "'")
			if isLeftLiteral {
				// Use STRPOS/position for string containment
				return fmt.Sprintf("%s > 0", c.strposFunc(rightSQL, leftSQL)), nil
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

		// Validate enum values if left side is an enum field
		if leftFieldName != "" && c.schema() != nil && c.schema().IsEnumType(leftFieldName) {
			for _, item := range arr {
				if err := c.validateEnumValue(item, leftFieldName); err != nil {
					return "", err
				}
			}
		}

		// Convert array elements to SQL values
		var values []string
		for _, item := range arr {
			valueSQL, err := c.dataOp.valueToSQL(item)
			if err != nil {
				return "", fmt.Errorf("invalid array element: %w", err)
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
			return "", fmt.Errorf("invalid string in IN operator: %w", err)
		}
		return fmt.Sprintf("POSITION(%s IN %s) > 0", leftSQL, rightSQL), nil
	}

	// Check if right side is a number (convert to string for containment)
	if num, ok := rightValue.(float64); ok {
		// Convert number to string for containment check
		rightSQL, err := c.dataOp.valueToSQL(num)
		if err != nil {
			return "", fmt.Errorf("invalid number in IN operator: %w", err)
		}
		return fmt.Sprintf("POSITION(%s IN %s) > 0", leftSQL, rightSQL), nil
	}

	return "", fmt.Errorf("in operator requires array, variable, string, or number as second argument")
}

// handleChainedComparison handles chained comparisons like {"<": [10, {"var": "x"}, 20, 30]}
// For 2 args: generates "a < b"
// For 3+ args: generates "(a < b AND b < c AND c < d)".
func (c *ComparisonOperator) handleChainedComparison(operator string, args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("chained comparison requires at least 2 arguments")
	}

	// Validate operands for ordering comparisons
	for _, arg := range args {
		if err := c.validateOrderingOperand(arg, operator); err != nil {
			return "", err
		}
	}

	// Apply type coercion: find field names and coerce adjacent literals
	coercedArgs := make([]interface{}, len(args))
	copy(coercedArgs, args)

	// Find the field name from any var expression to use for coercion
	var fieldName string
	for _, arg := range args {
		if name := c.extractFieldNameFromValue(arg); name != "" {
			fieldName = name
			break
		}
	}

	// Coerce all literal arguments based on the field type
	if fieldName != "" {
		for i, arg := range coercedArgs {
			if c.extractFieldNameFromValue(arg) == "" {
				coercedArgs[i] = c.coerceValueForComparison(arg, fieldName)
			}
		}
	}

	// Convert all arguments to SQL
	var sqlArgs []string
	for i, arg := range coercedArgs {
		argSQL, err := c.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid argument %d: %w", i, err)
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

// processArithmeticExpression handles arithmetic operations within comparison operations.
func (c *ComparisonOperator) processArithmeticExpression(op string, args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("arithmetic operation requires array of arguments")
	}

	// Handle unary minus (negation) - single argument case
	if op == "-" && len(argsSlice) == 1 {
		operand, err := c.valueToSQL(argsSlice[0])
		if err != nil {
			return "", fmt.Errorf("invalid unary minus argument: %w", err)
		}
		return fmt.Sprintf("(-%s)", operand), nil
	}

	// Handle unary plus (cast to number) - single argument case
	if op == "+" && len(argsSlice) == 1 {
		operand, err := c.valueToSQL(argsSlice[0])
		if err != nil {
			return "", fmt.Errorf("invalid unary plus argument: %w", err)
		}
		return fmt.Sprintf("CAST(%s AS NUMERIC)", operand), nil
	}

	if len(argsSlice) < 2 {
		return "", fmt.Errorf("arithmetic operation requires at least 2 arguments")
	}

	// Convert arguments to SQL
	operands := make([]string, len(argsSlice))
	for i, arg := range argsSlice {
		operand, err := c.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid arithmetic argument %d: %w", i, err)
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

// processComparisonExpression handles comparison operations within comparison operations.
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
		return "", fmt.Errorf("invalid comparison left argument: %w", err)
	}

	right, err := c.valueToSQL(argsSlice[1])
	if err != nil {
		return "", fmt.Errorf("invalid comparison right argument: %w", err)
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

// processMinMaxExpression handles min/max operations within comparison operations.
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
			return "", fmt.Errorf("invalid min/max argument %d: %w", i, err)
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
