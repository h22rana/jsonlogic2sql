package operators

import (
	"fmt"
	"strings"
)

// NumericOperator handles numeric operations like +, -, *, /, %, max, min, between
type NumericOperator struct {
	dataOp       *DataOperator
	comparisonOp *ComparisonOperator
}

// NewNumericOperator creates a new NumericOperator instance
func NewNumericOperator() *NumericOperator {
	return &NumericOperator{
		dataOp:       NewDataOperator(),
		comparisonOp: NewComparisonOperator(),
	}
}

// ToSQL converts a numeric operation to SQL
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
	case "between":
		return n.handleBetween(args)
	default:
		return "", fmt.Errorf("unsupported numeric operator: %s", operator)
	}
}

// handleAddition converts + operator to SQL
func (n *NumericOperator) handleAddition(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("addition requires at least 2 arguments")
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid addition argument %d: %v", i, err)
		}
		operands[i] = operand
	}

	return fmt.Sprintf("(%s)", strings.Join(operands, " + ")), nil
}

// handleSubtraction converts - operator to SQL
func (n *NumericOperator) handleSubtraction(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("subtraction requires at least 2 arguments")
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid subtraction argument %d: %v", i, err)
		}
		operands[i] = operand
	}

	return fmt.Sprintf("(%s)", strings.Join(operands, " - ")), nil
}

// handleMultiplication converts * operator to SQL
func (n *NumericOperator) handleMultiplication(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("multiplication requires at least 2 arguments")
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid multiplication argument %d: %v", i, err)
		}
		operands[i] = operand
	}

	return fmt.Sprintf("(%s)", strings.Join(operands, " * ")), nil
}

// handleDivision converts / operator to SQL
func (n *NumericOperator) handleDivision(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("division requires at least 2 arguments")
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid division argument %d: %v", i, err)
		}
		operands[i] = operand
	}

	return fmt.Sprintf("(%s)", strings.Join(operands, " / ")), nil
}

// handleModulo converts % operator to SQL
func (n *NumericOperator) handleModulo(args []interface{}) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("modulo requires exactly 2 arguments")
	}

	left, err := n.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid modulo left argument: %v", err)
	}

	right, err := n.valueToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid modulo right argument: %v", err)
	}

	return fmt.Sprintf("(%s %% %s)", left, right), nil
}

// handleMax converts max operator to SQL
func (n *NumericOperator) handleMax(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("max requires at least 2 arguments")
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid max argument %d: %v", i, err)
		}
		operands[i] = operand
	}

	return fmt.Sprintf("GREATEST(%s)", strings.Join(operands, ", ")), nil
}

// handleMin converts min operator to SQL
func (n *NumericOperator) handleMin(args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("min requires at least 2 arguments")
	}

	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := n.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid min argument %d: %v", i, err)
		}
		operands[i] = operand
	}

	return fmt.Sprintf("LEAST(%s)", strings.Join(operands, ", ")), nil
}

// handleBetween converts between operator to SQL
func (n *NumericOperator) handleBetween(args []interface{}) (string, error) {
	if len(args) != 3 {
		return "", fmt.Errorf("between requires exactly 3 arguments")
	}

	value, err := n.valueToSQL(args[0])
	if err != nil {
		return "", fmt.Errorf("invalid between value argument: %v", err)
	}

	min, err := n.valueToSQL(args[1])
	if err != nil {
		return "", fmt.Errorf("invalid between min argument: %v", err)
	}

	max, err := n.valueToSQL(args[2])
	if err != nil {
		return "", fmt.Errorf("invalid between max argument: %v", err)
	}

	return fmt.Sprintf("(%s BETWEEN %s AND %s)", value, min, max), nil
}

// valueToSQL converts a value to SQL, handling var expressions and literals
func (n *NumericOperator) valueToSQL(value interface{}) (string, error) {
	// Handle var expressions
	if expr, ok := value.(map[string]interface{}); ok {
		if varExpr, hasVar := expr["var"]; hasVar {
			return n.dataOp.ToSQL("var", []interface{}{varExpr})
		}
	}

	// Handle primitive values
	return n.dataOp.valueToSQL(value)
}
