package parser

import (
	"fmt"

	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/validator"
)

// Parser parses JSON Logic expressions and converts them to SQL WHERE clauses
type Parser struct {
	validator        *validator.Validator
	dataOp           *operators.DataOperator
	comparisonOp     *operators.ComparisonOperator
	logicalOp        *operators.LogicalOperator
}

// NewParser creates a new parser instance
func NewParser() *Parser {
	return &Parser{
		validator:    validator.NewValidator(),
		dataOp:       operators.NewDataOperator(),
		comparisonOp: operators.NewComparisonOperator(),
		logicalOp:    operators.NewLogicalOperator(),
	}
}

// Parse converts a JSON Logic expression to SQL WHERE clause
func (p *Parser) Parse(logic interface{}) (string, error) {
	// First validate the expression
	if err := p.validator.Validate(logic); err != nil {
		return "", fmt.Errorf("validation error: %v", err)
	}

	// Parse the expression
	sql, err := p.parseExpression(logic)
	if err != nil {
		return "", fmt.Errorf("parse error: %v", err)
	}

	// Wrap in WHERE clause
	return fmt.Sprintf("WHERE %s", sql), nil
}

// parseExpression recursively parses JSON Logic expressions
func (p *Parser) parseExpression(expr interface{}) (string, error) {
	// Handle primitive values (should not happen in normal JSON Logic, but handle gracefully)
	if p.isPrimitive(expr) {
		return "", fmt.Errorf("primitive values not supported in WHERE clauses")
	}

	// Handle arrays (should not happen in normal JSON Logic, but handle gracefully)
	if _, ok := expr.([]interface{}); ok {
		return "", fmt.Errorf("arrays not supported in WHERE clauses")
	}

	// Handle objects (operators)
	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return "", fmt.Errorf("operator object must have exactly one key")
		}

		for operator, args := range obj {
			return p.parseOperator(operator, args)
		}
	}

	return "", fmt.Errorf("invalid expression type: %T", expr)
}

// parseOperator parses a specific operator
func (p *Parser) parseOperator(operator string, args interface{}) (string, error) {
	// Handle different operator types
	switch operator {
	// Data access operators
	case "var":
		return p.dataOp.ToSQL(operator, []interface{}{args})
	case "missing", "missing_some":
		if arr, ok := args.([]interface{}); ok {
			return p.dataOp.ToSQL(operator, arr)
		}
		return "", fmt.Errorf("%s operator requires array arguments", operator)

	// Comparison operators
	case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
		if arr, ok := args.([]interface{}); ok {
			return p.comparisonOp.ToSQL(operator, arr)
		}
		return "", fmt.Errorf("comparison operator requires array arguments")

	// Logical operators
	case "and", "or", "!", "!!", "if":
		if arr, ok := args.([]interface{}); ok {
			return p.logicalOp.ToSQL(operator, arr)
		}
		return "", fmt.Errorf("logical operator requires array arguments")

	// For now, return error for unsupported operators
	// TODO: Add support for numeric, array, and string operators
	default:
		return "", fmt.Errorf("unsupported operator: %s", operator)
	}
}

// isPrimitive checks if a value is a primitive type
func (p *Parser) isPrimitive(value interface{}) bool {
	switch value.(type) {
	case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		return true
	case nil:
		return true
	default:
		return false
	}
}
