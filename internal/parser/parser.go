package parser

import (
	"fmt"

	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/validator"
)

// Parser parses JSON Logic expressions and converts them to SQL WHERE clauses
type Parser struct {
	validator    *validator.Validator
	dataOp       *operators.DataOperator
	comparisonOp *operators.ComparisonOperator
	logicalOp    *operators.LogicalOperator
	numericOp    *operators.NumericOperator
	stringOp     *operators.StringOperator
	arrayOp      *operators.ArrayOperator
}

// NewParser creates a new parser instance
func NewParser() *Parser {
	return &Parser{
		validator:    validator.NewValidator(),
		dataOp:       operators.NewDataOperator(),
		comparisonOp: operators.NewComparisonOperator(),
		logicalOp:    operators.NewLogicalOperator(),
		numericOp:    operators.NewNumericOperator(),
		stringOp:     operators.NewStringOperator(),
		arrayOp:      operators.NewArrayOperator(),
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
	case "missing":
		// missing takes a single string argument, wrap it in an array
		return p.dataOp.ToSQL(operator, []interface{}{args})
	case "missing_some":
		if arr, ok := args.([]interface{}); ok {
			return p.dataOp.ToSQL(operator, arr)
		}
		return "", fmt.Errorf("missing_some operator requires array arguments")

	// Comparison operators
	case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
		if arr, ok := args.([]interface{}); ok {
			// Process arguments to handle complex expressions
			processedArgs, err := p.processArgs(arr)
			if err != nil {
				return "", fmt.Errorf("failed to process comparison arguments: %v", err)
			}
			return p.comparisonOp.ToSQL(operator, processedArgs)
		}
		return "", fmt.Errorf("comparison operator requires array arguments")

	// Logical operators
	case "and", "or", "if":
		if arr, ok := args.([]interface{}); ok {
			return p.logicalOp.ToSQL(operator, arr)
		}
		return "", fmt.Errorf("%s operator requires array arguments", operator)
	case "!", "!!":
		// These unary operators can accept both array and non-array arguments
		if arr, ok := args.([]interface{}); ok {
			return p.logicalOp.ToSQL(operator, arr)
		}
		// Wrap non-array argument in array for processing
		return p.logicalOp.ToSQL(operator, []interface{}{args})

	// Numeric operators
	case "+", "-", "*", "/", "%", "max", "min":
		if arr, ok := args.([]interface{}); ok {
			// Process arguments to handle complex expressions
			processedArgs, err := p.processArgs(arr)
			if err != nil {
				return "", fmt.Errorf("failed to process numeric arguments: %v", err)
			}
			return p.numericOp.ToSQL(operator, processedArgs)
		}
		return "", fmt.Errorf("numeric operator requires array arguments")

	// Array operators
	case "map", "filter", "reduce", "all", "some", "none", "merge":
		if arr, ok := args.([]interface{}); ok {
			return p.arrayOp.ToSQL(operator, arr)
		}
		return "", fmt.Errorf("array operator requires array arguments")

	// String operators
	case "cat", "substr":
		if arr, ok := args.([]interface{}); ok {
			return p.stringOp.ToSQL(operator, arr)
		}
		return "", fmt.Errorf("string operator requires array arguments")

	// All operators are now supported
	default:
		return "", fmt.Errorf("unsupported operator: %s", operator)
	}
}

// processArgs recursively processes arguments to handle complex expressions
func (p *Parser) processArgs(args []interface{}) ([]interface{}, error) {
	processed := make([]interface{}, len(args))

	for i, arg := range args {
		// If it's a complex expression, parse it recursively
		if exprMap, ok := arg.(map[string]interface{}); ok {
			// Check if it's a complex expression (not just a var)
			if len(exprMap) == 1 {
				for operator := range exprMap {
					if operator != "var" {
						// It's a complex expression, parse it directly
						sql, err := p.parseOperator(operator, exprMap[operator])
						if err != nil {
							return nil, err
						}
						// Store the SQL as a string for the operator to use
						processed[i] = sql
						continue
					}
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
