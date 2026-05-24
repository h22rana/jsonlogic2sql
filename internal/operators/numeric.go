package operators

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// NumericOperator handles numeric operations like +, -, *, /, %, max, min.
type NumericOperator struct {
	config       *OperatorConfig
	dataOp       *DataOperator
	comparisonOp *ComparisonOperator
}

// NewNumericOperator creates a new NumericOperator instance.
func NewNumericOperator(config *OperatorConfig) *NumericOperator {
	config = normalizeOperatorConfig(config)
	return &NumericOperator{
		config:       config,
		dataOp:       NewDataOperator(config),
		comparisonOp: NewComparisonOperator(config),
	}
}

func unaryMinusSQL(operand string) string {
	if strings.HasPrefix(strings.TrimSpace(operand), "-") {
		operand = fmt.Sprintf("(%s)", operand)
	}
	return fmt.Sprintf("(-%s)", operand)
}

func (n *NumericOperator) schema() SchemaProvider {
	return schemaFromConfig(n.config)
}

// validateNumericOperand checks if a field used in a numeric operation is of numeric type.
func (n *NumericOperator) validateNumericOperand(value interface{}) error {
	fieldName := n.extractFieldNameFromValue(value)
	if fieldName == "" {
		return validateNumericExpressionOperand(value)
	}

	fieldType := n.schema().GetFieldType(fieldName)
	if fieldType == "" {
		return validateNumericExpressionOperand(value)
	}

	if !n.schema().IsNumericType(fieldName) {
		return fmt.Errorf("numeric operation on non-numeric field '%s' (type: %s)", fieldName, fieldType)
	}

	return validateNumericExpressionOperand(value)
}

func validateNumericExpressionOperand(value interface{}) error {
	if pv, ok := value.(ProcessedValue); ok && pv.IsSQL && pv.HasExpressionInfo {
		return validateNumericExpressionShape(pv.Kind, pv.Type)
	}
	if expr, ok := value.(map[string]interface{}); ok && len(expr) == 1 {
		for operator := range expr {
			switch operator {
			case OpCat, OpSubstr:
				return numericExpressionTypeError(ExpressionTypeString)
			case OpMap, OpFilter, OpMerge:
				return numericExpressionTypeError(ExpressionTypeArray)
			}
		}
	}
	return nil
}

func validateNumericExpressionShape(kind ExpressionKind, typ ExpressionType) error {
	if kind == ExpressionKindPredicate {
		return nil
	}
	switch typ {
	case ExpressionTypeUnknown, ExpressionTypeNull, ExpressionTypeBoolean, ExpressionTypeNumber:
		return nil
	case ExpressionTypeString, ExpressionTypeArray, ExpressionTypeObject:
		return numericExpressionTypeError(typ)
	default:
		return fmt.Errorf("numeric operation has unsupported value expression type %s", expressionTypeName(typ))
	}
}

func numericExpressionTypeError(typ ExpressionType) error {
	return fmt.Errorf("numeric operation on incompatible value expression (type: %s)", expressionTypeName(typ))
}

// extractFieldNameFromValue extracts field name from a value that might be a var expression.
func (n *NumericOperator) extractFieldNameFromValue(value interface{}) string {
	if pv, ok := value.(ProcessedValue); ok && pv.IsSQL && pv.IsField {
		return pv.FieldName
	}
	if varExpr, ok := value.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			return n.extractFieldName(varName)
		}
	}
	return ""
}

// extractFieldName extracts the field name from a var argument.
func (n *NumericOperator) extractFieldName(varName interface{}) string {
	if pv, ok := varName.(ProcessedValue); ok && pv.IsSQL && pv.IsField {
		return pv.FieldName
	}
	if nameStr, ok := varName.(string); ok {
		return nameStr
	}
	if nameArr, ok := varName.([]interface{}); ok && len(nameArr) > 0 {
		if pv, ok := nameArr[0].(ProcessedValue); ok && pv.IsSQL && pv.IsField {
			return pv.FieldName
		}
		if nameStr, ok := nameArr[0].(string); ok {
			return nameStr
		}
	}
	return ""
}

// ToSQL converts a numeric operation to SQL.
func (n *NumericOperator) ToSQL(operator string, args []interface{}) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("numeric operator %s requires at least one argument", operator)
	}

	switch operator {
	case OpAdd:
		return n.handleAddition(args)
	case OpSubtract:
		return n.handleSubtraction(args)
	case OpMultiply:
		return n.handleMultiplication(args)
	case OpDivide:
		return n.handleDivision(args)
	case OpModulo:
		return n.handleModulo(args)
	case OpMax:
		return n.handleMax(args)
	case OpMin:
		return n.handleMin(args)
	default:
		return "", fmt.Errorf("unsupported numeric operator: %s", operator)
	}
}

// handleAddition converts + operator to SQL.
func (n *NumericOperator) handleAddition(args []interface{}) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("addition requires at least 1 argument")
	}

	// Validate operand types
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}

	// Handle unary plus (cast to number)
	if len(args) == 1 {
		operand, err := n.valueToSQL(args[0])
		if err != nil {
			return "", fmt.Errorf("invalid unary plus argument: %w", err)
		}
		return fmt.Sprintf("CAST(%s AS NUMERIC)", operand), nil
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid addition argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	return fmt.Sprintf("(%s)", strings.Join(operands, " + ")), nil
}

// handleSubtraction converts - operator to SQL.
func (n *NumericOperator) handleSubtraction(args []interface{}) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("subtraction requires at least 1 argument")
	}

	// Validate operand types
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}

	// Handle unary minus (negation) - wrap in parentheses for safety in nested expressions
	if len(args) == 1 {
		operand, err := n.valueToSQL(args[0])
		if err != nil {
			return "", fmt.Errorf("invalid unary minus argument: %w", err)
		}
		return unaryMinusSQL(operand), nil
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid subtraction argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	return fmt.Sprintf("(%s)", strings.Join(operands, " - ")), nil
}

// handleMultiplication converts * operator to SQL.
func (n *NumericOperator) handleMultiplication(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("multiplication requires at least 2 arguments")
	}

	// Validate operand types
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid multiplication argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	return fmt.Sprintf("(%s)", strings.Join(operands, " * ")), nil
}

// handleDivision converts / operator to SQL.
func (n *NumericOperator) handleDivision(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("division requires at least 2 arguments")
	}

	// Validate operand types
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid division argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	return fmt.Sprintf("(%s)", strings.Join(operands, " / ")), nil
}

// handleModulo converts % operator to SQL.
func (n *NumericOperator) handleModulo(args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("modulo requires exactly 2 arguments")
	}

	// Validate operand types
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}

	left, err := n.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid modulo left argument: %w", err)
	}

	right, err := n.valueToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid modulo right argument: %w", err)
	}

	return n.config.ModuloSQL(left, right), nil
}

// handleMax converts max operator to SQL.
func (n *NumericOperator) handleMax(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("max requires at least 2 arguments")
	}

	// Validate operand types
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid max argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	return n.config.GreatestSQL(operands), nil
}

// handleMin converts min operator to SQL.
func (n *NumericOperator) handleMin(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("min requires at least 2 arguments")
	}

	// Validate operand types
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid min argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	return n.config.LeastSQL(operands), nil
}

func numericBoolLiteral(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func numericSQLFromProcessedValue(pv ProcessedValue) (string, error) {
	if !pv.HasExpressionInfo {
		return pv.Value, nil
	}
	if err := validateNumericExpressionShape(pv.Kind, pv.Type); err != nil {
		return "", err
	}
	if pv.Kind == ExpressionKindPredicate {
		return PredicateNumberSQL(pv.Value), nil
	}
	if pv.Type == ExpressionTypeBoolean {
		return BooleanValueNumberSQL(pv.Value), nil
	}
	return pv.Value, nil
}

// valueToSQL converts a value to SQL, handling var expressions and literals.
func (n *NumericOperator) valueToSQL(value interface{}) (string, error) {
	// Handle ProcessedValue (pre-processed SQL from parser)
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return numericSQLFromProcessedValue(pv)
		}
		// It's a literal, convert it
		return n.dataOp.valueToSQL(pv.Value)
	}

	if b, ok := value.(bool); ok {
		return numericBoolLiteral(b), nil
	}

	// Handle plain strings: attempt numeric coercion per JSONLogic spec,
	// otherwise treat as a safely-quoted literal to prevent SQL injection.
	// Pre-processed SQL from the parser arrives as ProcessedValue (handled above),
	// so any plain string here is a raw JSON literal.
	// Trim whitespace before numeric checks to match JSONLogic's JS coercion
	// (e.g. " 3 " → 3), but quote the original if it's non-numeric.
	if str, ok := value.(string); ok {
		trimmed := strings.TrimSpace(str)
		if isIntegerLiteral(trimmed) {
			return trimmed, nil
		}
		if num, err := strconv.ParseFloat(trimmed, 64); err == nil && !math.IsNaN(num) && !math.IsInf(num, 0) {
			return strconv.FormatFloat(num, 'f', -1, 64), nil
		}
		return n.dataOp.valueToSQL(str)
	}
	if num, ok := value.(json.Number); ok {
		numberLiteral, err := normalizeJSONNumberLiteral(num)
		if err != nil {
			return "", err
		}
		return numberLiteral, nil
	}

	// Handle var expressions and complex expressions
	if expr, ok := value.(map[string]interface{}); ok {
		if len(expr) != 1 {
			return "", fmt.Errorf("operator object must have exactly one key")
		}
		if varExpr, hasVar := expr[OpVar]; hasVar {
			return n.dataOp.ToSQL(OpVar, []interface{}{varExpr})
		}
		// Handle complex expressions by recursively parsing them
		for operator, args := range expr {
			if arr, ok := args.([]interface{}); ok {
				// Handle different operator types
				switch operator {
				case OpEqual, OpStrictEqual, OpNotEqual, OpStrictNotEqual,
					OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual, OpIn:
					// Process arguments first to handle nested expressions
					processedArgs, err := n.processComplexArgsForComparison(arr)
					if err != nil {
						return "", err
					}
					// Delegate to comparison operator
					sql, err := n.comparisonOp.ToSQL(operator, processedArgs)
					if err != nil {
						return "", err
					}
					return PredicateNumberSQL(sql), nil
				case OpAdd, OpSubtract, OpMultiply, OpDivide, OpModulo, OpMax, OpMin:
					// Recursively process the arguments
					processedArgs, err := n.processComplexArgs(arr)
					if err != nil {
						return "", err
					}
					// Generate SQL for the complex expression
					return n.generateComplexSQL(operator, processedArgs)
				case OpIf:
					// Handle if operator - delegate to logical operator
					logicalOp := NewLogicalOperator(n.config)
					return logicalOp.ToSQL(OpIf, arr)
				case OpAnd, OpOr, OpNot, OpDoubleBang:
					// Handle logical operators - delegate to logical operator
					logicalOp := NewLogicalOperator(n.config)
					sql, err := logicalOp.ToSQL(operator, arr)
					if err != nil {
						return "", err
					}
					return PredicateNumberSQL(sql), nil
				case OpReduce, OpFilter, OpMap, OpSome, OpAll, OpNone, OpMerge:
					// Handle array operators - delegate to array operator
					arrayOp := NewArrayOperator(n.config)
					return arrayOp.ToSQL(operator, arr)
				default:
					// Try to use the expression parser callback for unknown operators
					// This enables support for custom operators in nested contexts
					if n.config != nil && n.config.HasExpressionParser() {
						return n.config.ParseExpression(expr, jsonPathRoot)
					}
					return "", fmt.Errorf("unsupported operator in numeric expression: %s", operator)
				}
			}
		}
	}

	// Handle primitive values
	return n.dataOp.valueToSQL(value)
}

// processComplexArgs recursively processes arguments for complex expressions.
func (n *NumericOperator) processComplexArgs(args []interface{}) ([]string, error) {
	processed := make([]string, len(args))

	for i, arg := range args {
		sql, err := n.valueToSQL(arg)
		if err != nil {
			return nil, err
		}
		processed[i] = sql
	}

	return processed, nil
}

// processComplexArgsForComparison processes arguments for comparison operators.
// Var expressions and primitives are passed through so the comparison operator
// can perform schema-based type coercion (e.g. number→string for string fields).
// Only nested complex expressions (arithmetic, logical, etc.) are pre-evaluated
// to SQL and wrapped in SQLResult.
func (n *NumericOperator) processComplexArgsForComparison(args []interface{}) ([]interface{}, error) {
	processed := make([]interface{}, len(args))

	for i, arg := range args {
		if exprMap, ok := arg.(map[string]interface{}); ok && len(exprMap) == 1 {
			if _, isVar := exprMap[OpVar]; isVar {
				processed[i] = arg
				continue
			}
			sql, err := n.valueToSQL(arg)
			if err != nil {
				return nil, err
			}
			processed[i] = SQLResult(sql)
			continue
		}
		// Primitives (string, float64, bool, nil) and ProcessedValue pass through
		// so comparison can apply schema coercion and proper quoting.
		processed[i] = arg
	}

	return processed, nil
}

// generateComplexSQL generates SQL for complex expressions.
func (n *NumericOperator) generateComplexSQL(operator string, args []string) (string, error) {
	switch operator {
	case OpAdd:
		if len(args) < 2 {
			return "", fmt.Errorf("addition requires at least 2 arguments")
		}
		return fmt.Sprintf("(%s)", strings.Join(args, " + ")), nil
	case OpSubtract:
		if len(args) == 1 {
			// Unary minus (negation) - wrap in parentheses for safety in nested expressions
			return unaryMinusSQL(args[0]), nil
		}
		if len(args) < 2 {
			return "", fmt.Errorf("subtraction requires at least 1 argument")
		}
		return fmt.Sprintf("(%s)", strings.Join(args, " - ")), nil
	case OpMultiply:
		if len(args) < 2 {
			return "", fmt.Errorf("multiplication requires at least 2 arguments")
		}
		return fmt.Sprintf("(%s)", strings.Join(args, " * ")), nil
	case OpDivide:
		if len(args) < 2 {
			return "", fmt.Errorf("division requires at least 2 arguments")
		}
		return fmt.Sprintf("(%s)", strings.Join(args, " / ")), nil
	case OpModulo:
		if len(args) != 2 {
			return "", fmt.Errorf("modulo requires exactly 2 arguments")
		}
		return n.config.ModuloSQL(args[0], args[1]), nil
	case OpMax:
		if len(args) < 2 {
			return "", fmt.Errorf("max requires at least 2 arguments")
		}
		return n.config.GreatestSQL(args), nil
	case OpMin:
		if len(args) < 2 {
			return "", fmt.Errorf("min requires at least 2 arguments")
		}
		return n.config.LeastSQL(args), nil
	default:
		// For other operators (array, logical, etc.), they should have been pre-processed
		// If we see them here, it means they weren't processed correctly
		return "", fmt.Errorf("unsupported operator in numeric expression: %s", operator)
	}
}

// ToSQLParam is the parameterized variant of ToSQL. Keep in sync.
func (n *NumericOperator) ToSQLParam(operator string, args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("numeric operator %s requires at least one argument", operator)
	}

	switch operator {
	case OpAdd:
		return n.handleAdditionParam(args, pc)
	case OpSubtract:
		return n.handleSubtractionParam(args, pc)
	case OpMultiply:
		return n.handleMultiplicationParam(args, pc)
	case OpDivide:
		return n.handleDivisionParam(args, pc)
	case OpModulo:
		return n.handleModuloParam(args, pc)
	case OpMax:
		return n.handleMaxParam(args, pc)
	case OpMin:
		return n.handleMinParam(args, pc)
	default:
		return "", fmt.Errorf("unsupported numeric operator: %s", operator)
	}
}

// handleAdditionParam is the parameterized variant of handleAddition. Keep in sync.
func (n *NumericOperator) handleAdditionParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("addition requires at least 1 argument")
	}
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}
	if len(args) == 1 {
		operand, err := n.valueToSQLParam(args[0], pc)
		if err != nil {
			return "", fmt.Errorf("invalid unary plus argument: %w", err)
		}
		return fmt.Sprintf("CAST(%s AS NUMERIC)", operand), nil
	}
	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQLParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid addition argument %d: %w", i, err)
		}
		operands[i] = operand
	}
	return fmt.Sprintf("(%s)", strings.Join(operands, " + ")), nil
}

// handleSubtractionParam is the parameterized variant of handleSubtraction. Keep in sync.
func (n *NumericOperator) handleSubtractionParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("subtraction requires at least 1 argument")
	}
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}
	if len(args) == 1 {
		operand, err := n.valueToSQLParam(args[0], pc)
		if err != nil {
			return "", fmt.Errorf("invalid unary minus argument: %w", err)
		}
		return unaryMinusSQL(operand), nil
	}
	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQLParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid subtraction argument %d: %w", i, err)
		}
		operands[i] = operand
	}
	return fmt.Sprintf("(%s)", strings.Join(operands, " - ")), nil
}

// handleMultiplicationParam is the parameterized variant of handleMultiplication. Keep in sync.
func (n *NumericOperator) handleMultiplicationParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("multiplication requires at least 2 arguments")
	}
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}
	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQLParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid multiplication argument %d: %w", i, err)
		}
		operands[i] = operand
	}
	return fmt.Sprintf("(%s)", strings.Join(operands, " * ")), nil
}

// handleDivisionParam is the parameterized variant of handleDivision. Keep in sync.
func (n *NumericOperator) handleDivisionParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("division requires at least 2 arguments")
	}
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}
	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQLParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid division argument %d: %w", i, err)
		}
		operands[i] = operand
	}
	return fmt.Sprintf("(%s)", strings.Join(operands, " / ")), nil
}

// handleModuloParam is the parameterized variant of handleModulo. Keep in sync.
func (n *NumericOperator) handleModuloParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("modulo requires exactly 2 arguments")
	}
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}
	left, err := n.valueToSQLParam(args[0], pc)
	if err != nil {
		return "", fmt.Errorf("invalid modulo left argument: %w", err)
	}
	right, err := n.valueToSQLParam(args[1], pc)
	if err != nil {
		return "", fmt.Errorf("invalid modulo right argument: %w", err)
	}
	return n.config.ModuloSQL(left, right), nil
}

// handleMaxParam is the parameterized variant of handleMax. Keep in sync.
func (n *NumericOperator) handleMaxParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("max requires at least 2 arguments")
	}
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}
	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQLParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid max argument %d: %w", i, err)
		}
		operands[i] = operand
	}
	return n.config.GreatestSQL(operands), nil
}

// handleMinParam is the parameterized variant of handleMin. Keep in sync.
func (n *NumericOperator) handleMinParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("min requires at least 2 arguments")
	}
	for _, arg := range args {
		if err := n.validateNumericOperand(arg); err != nil {
			return "", err
		}
	}
	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQLParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid min argument %d: %w", i, err)
		}
		operands[i] = operand
	}
	return n.config.LeastSQL(operands), nil
}

// valueToSQLParam is the parameterized variant of valueToSQL. Keep in sync.
func (n *NumericOperator) valueToSQLParam(value interface{}, pc *params.ParamCollector) (string, error) {
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return numericSQLFromProcessedValue(pv)
		}
		return n.dataOp.valueToSQLParam(pv.Value, pc)
	}

	if b, ok := value.(bool); ok {
		return numericBoolLiteral(b), nil
	}

	if str, ok := value.(string); ok {
		trimmed := strings.TrimSpace(str)
		if isIntegerLiteral(trimmed) {
			n, err := strconv.ParseInt(trimmed, 10, 64)
			if err == nil {
				return pc.Add(n), nil
			}
			// Integer overflows int64; store as string to preserve full precision.
			// *big.Int is not used because database/sql's defaultConverter rejects
			// it (unsupported type). Callers binding large integers should convert
			// the string to their driver's appropriate numeric type.
			return pc.Add(trimmed), nil
		}
		if num, err := strconv.ParseFloat(trimmed, 64); err == nil && !math.IsNaN(num) && !math.IsInf(num, 0) {
			return pc.Add(num), nil
		}
		return n.dataOp.valueToSQLParam(str, pc)
	}
	if num, ok := value.(json.Number); ok {
		if _, err := normalizeJSONNumberLiteral(num); err != nil {
			return "", err
		}
		return pc.Add(jsonNumberParamValue(num)), nil
	}

	if expr, ok := value.(map[string]interface{}); ok {
		if len(expr) != 1 {
			return "", fmt.Errorf("operator object must have exactly one key")
		}
		if varExpr, hasVar := expr[OpVar]; hasVar {
			return n.dataOp.ToSQLParam(OpVar, []interface{}{varExpr}, pc)
		}
		for operator, args := range expr {
			if arr, ok := args.([]interface{}); ok {
				switch operator {
				case OpEqual, OpStrictEqual, OpNotEqual, OpStrictNotEqual,
					OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual, OpIn:
					processedArgs, err := n.processComplexArgsForComparisonParam(arr, pc)
					if err != nil {
						return "", err
					}
					sql, err := n.comparisonOp.ToSQLParam(operator, processedArgs, pc)
					if err != nil {
						return "", err
					}
					return PredicateNumberSQL(sql), nil
				case OpAdd, OpSubtract, OpMultiply, OpDivide, OpModulo, OpMax, OpMin:
					processedArgs, err := n.processComplexArgsParam(arr, pc)
					if err != nil {
						return "", err
					}
					return n.generateComplexSQL(operator, processedArgs)
				case OpIf:
					logicalOp := NewLogicalOperator(n.config)
					return logicalOp.ToSQLParam(OpIf, arr, pc)
				case OpAnd, OpOr, OpNot, OpDoubleBang:
					logicalOp := NewLogicalOperator(n.config)
					sql, err := logicalOp.ToSQLParam(operator, arr, pc)
					if err != nil {
						return "", err
					}
					return PredicateNumberSQL(sql), nil
				case OpReduce, OpFilter, OpMap, OpSome, OpAll, OpNone, OpMerge:
					arrayOp := NewArrayOperator(n.config)
					return arrayOp.ToSQLParam(operator, arr, pc)
				default:
					if n.config != nil && n.config.HasParamExpressionParser() {
						return n.config.ParseExpressionParam(expr, jsonPathRoot, pc)
					}
					return "", fmt.Errorf("unsupported operator in numeric expression: %s", operator)
				}
			}
		}
	}

	return n.dataOp.valueToSQLParam(value, pc)
}

// processComplexArgsParam is the parameterized variant of processComplexArgs. Keep in sync.
func (n *NumericOperator) processComplexArgsParam(args []interface{}, pc *params.ParamCollector) ([]string, error) {
	processed := make([]string, len(args))
	for i, arg := range args {
		sql, err := n.valueToSQLParam(arg, pc)
		if err != nil {
			return nil, err
		}
		processed[i] = sql
	}
	return processed, nil
}

// processComplexArgsForComparisonParam is the parameterized variant of processComplexArgsForComparison. Keep in sync.
func (n *NumericOperator) processComplexArgsForComparisonParam(args []interface{}, pc *params.ParamCollector) ([]interface{}, error) {
	processed := make([]interface{}, len(args))
	for i, arg := range args {
		if exprMap, ok := arg.(map[string]interface{}); ok && len(exprMap) == 1 {
			if _, isVar := exprMap[OpVar]; isVar {
				processed[i] = arg
				continue
			}
			sql, err := n.valueToSQLParam(arg, pc)
			if err != nil {
				return nil, err
			}
			processed[i] = SQLResult(sql)
			continue
		}
		processed[i] = arg
	}
	return processed, nil
}

// isIntegerLiteral reports whether s matches ^[+-]?[0-9]+$ - a bare integer
// with an optional sign and no decimal point, exponent, or other characters.
// Validated strings are safe to emit directly as SQL numeric literals.
func isIntegerLiteral(s string) bool {
	if len(s) == 0 {
		return false
	}
	start := 0
	if s[0] == '+' || s[0] == '-' {
		start = 1
	}
	if start >= len(s) {
		return false
	}
	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
