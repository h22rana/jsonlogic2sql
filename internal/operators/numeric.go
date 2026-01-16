package operators

import (
	"fmt"
	"strings"
)

// NumericOperator handles numeric operations like +, -, *, /, %, max, min.
type NumericOperator struct {
	config       *OperatorConfig
	dataOp       *DataOperator
	comparisonOp *ComparisonOperator
}

// NewNumericOperator creates a new NumericOperator instance with optional config.
func NewNumericOperator(config *OperatorConfig) *NumericOperator {
	return &NumericOperator{
		config:       config,
		dataOp:       NewDataOperator(config),
		comparisonOp: NewComparisonOperator(config),
	}
}

// schema returns the schema from config, or nil if not configured.
func (n *NumericOperator) schema() SchemaProvider {
	if n.config == nil {
		return nil
	}
	return n.config.Schema
}

// validateNumericOperand checks if a field used in a numeric operation is of numeric type.
func (n *NumericOperator) validateNumericOperand(value interface{}) error {
	if n.schema() == nil {
		return nil // No schema, no validation
	}

	fieldName := n.extractFieldNameFromValue(value)
	if fieldName == "" {
		return nil // Can't determine field name, skip validation
	}

	fieldType := n.schema().GetFieldType(fieldName)
	if fieldType == "" {
		return nil // Field not in schema, skip validation (existence checked by DataOperator)
	}

	if !n.schema().IsNumericType(fieldName) {
		return fmt.Errorf("numeric operation on non-numeric field '%s' (type: %s)", fieldName, fieldType)
	}

	return nil
}

// extractFieldNameFromValue extracts field name from a value that might be a var expression.
func (n *NumericOperator) extractFieldNameFromValue(value interface{}) string {
	if varExpr, ok := value.(map[string]interface{}); ok {
		if varName, hasVar := varExpr["var"]; hasVar {
			return n.extractFieldName(varName)
		}
	}
	return ""
}

// extractFieldName extracts the field name from a var argument.
func (n *NumericOperator) extractFieldName(varName interface{}) string {
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

// ToSQL converts a numeric operation to SQL.
func (n *NumericOperator) ToSQL(operator string, args []interface{}) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("numeric operator %s requires at least one argument", operator)
	}

	switch operator {
	case "+":
		return n.handleAddition(args)
	case "-":
		return n.handleSubtraction(args)
	case "*":
		return n.handleMultiplication(args)
	case "/":
		return n.handleDivision(args)
	case "%":
		return n.handleModulo(args)
	case "max":
		return n.handleMax(args)
	case "min":
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
		return fmt.Sprintf("(-%s)", operand), nil
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

	return fmt.Sprintf("(%s %% %s)", left, right), nil
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

	return fmt.Sprintf("GREATEST(%s)", strings.Join(operands, ", ")), nil
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

	return fmt.Sprintf("LEAST(%s)", strings.Join(operands, ", ")), nil
}

// valueToSQL converts a value to SQL, handling var expressions and literals.
func (n *NumericOperator) valueToSQL(value interface{}) (string, error) {
	// Handle pre-processed SQL strings from the parser
	if sqlStr, ok := value.(string); ok {
		// This is a pre-processed SQL string from the parser
		return sqlStr, nil
	}

	// Handle var expressions and complex expressions
	if expr, ok := value.(map[string]interface{}); ok {
		if varExpr, hasVar := expr["var"]; hasVar {
			return n.dataOp.ToSQL("var", []interface{}{varExpr})
		}
		// Handle complex expressions by recursively parsing them
		for operator, args := range expr {
			if arr, ok := args.([]interface{}); ok {
				// Handle different operator types
				switch operator {
				case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
					// Process arguments first to handle nested expressions
					processedArgs, err := n.processComplexArgsForComparison(arr)
					if err != nil {
						return "", err
					}
					// Delegate to comparison operator
					return n.comparisonOp.ToSQL(operator, processedArgs)
				case "+", "-", "*", "/", "%", "max", "min":
					// Recursively process the arguments
					processedArgs, err := n.processComplexArgs(arr)
					if err != nil {
						return "", err
					}
					// Generate SQL for the complex expression
					return n.generateComplexSQL(operator, processedArgs)
				case "if":
					// Handle if operator - delegate to logical operator
					logicalOp := NewLogicalOperator(n.config)
					return logicalOp.ToSQL("if", arr)
				case "and", "or", "!":
					// Handle logical operators - delegate to logical operator
					logicalOp := NewLogicalOperator(n.config)
					return logicalOp.ToSQL(operator, arr)
				case "reduce", "filter", "map", "some", "all", "none", "merge":
					// Handle array operators - delegate to array operator
					arrayOp := NewArrayOperator(n.config)
					return arrayOp.ToSQL(operator, arr)
				default:
					// For other operators, they should have been pre-processed
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

// processComplexArgsForComparison processes arguments for comparison operators
// Returns []interface{} instead of []string to match comparison operator's expectations.
func (n *NumericOperator) processComplexArgsForComparison(args []interface{}) ([]interface{}, error) {
	processed := make([]interface{}, len(args))

	for i, arg := range args {
		sql, err := n.valueToSQL(arg)
		if err != nil {
			return nil, err
		}
		processed[i] = sql
	}

	return processed, nil
}

// generateComplexSQL generates SQL for complex expressions.
func (n *NumericOperator) generateComplexSQL(operator string, args []string) (string, error) {
	switch operator {
	case "+":
		if len(args) < 2 {
			return "", fmt.Errorf("addition requires at least 2 arguments")
		}
		return fmt.Sprintf("(%s)", strings.Join(args, " + ")), nil
	case "-":
		if len(args) == 1 {
			// Unary minus (negation) - wrap in parentheses for safety in nested expressions
			return fmt.Sprintf("(-%s)", args[0]), nil
		}
		if len(args) < 2 {
			return "", fmt.Errorf("subtraction requires at least 1 argument")
		}
		return fmt.Sprintf("(%s)", strings.Join(args, " - ")), nil
	case "*":
		if len(args) < 2 {
			return "", fmt.Errorf("multiplication requires at least 2 arguments")
		}
		return fmt.Sprintf("(%s)", strings.Join(args, " * ")), nil
	case "/":
		if len(args) < 2 {
			return "", fmt.Errorf("division requires at least 2 arguments")
		}
		return fmt.Sprintf("(%s)", strings.Join(args, " / ")), nil
	case "%":
		if len(args) < 2 {
			return "", fmt.Errorf("modulo requires at least 2 arguments")
		}
		return fmt.Sprintf("(%s)", strings.Join(args, " % ")), nil
	case "max":
		if len(args) < 2 {
			return "", fmt.Errorf("max requires at least 2 arguments")
		}
		return fmt.Sprintf("GREATEST(%s)", strings.Join(args, ", ")), nil
	case "min":
		if len(args) < 2 {
			return "", fmt.Errorf("min requires at least 2 arguments")
		}
		return fmt.Sprintf("LEAST(%s)", strings.Join(args, ", ")), nil
	default:
		// For other operators (array, logical, etc.), they should have been pre-processed
		// If we see them here, it means they weren't processed correctly
		return "", fmt.Errorf("unsupported operator in numeric expression: %s", operator)
	}
}
