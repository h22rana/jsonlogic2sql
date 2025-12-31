package parser

import (
	"fmt"

	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/validator"
)

// CustomOperatorHandler is an interface for custom operator implementations.
// This mirrors the public OperatorHandler interface.
type CustomOperatorHandler interface {
	ToSQL(operator string, args []interface{}) (string, error)
}

// CustomOperatorLookup is a function type for looking up custom operators.
type CustomOperatorLookup func(operatorName string) (CustomOperatorHandler, bool)

// Parser parses JSON Logic expressions and converts them to SQL WHERE clauses
type Parser struct {
	validator      *validator.Validator
	dataOp         *operators.DataOperator
	comparisonOp   *operators.ComparisonOperator
	logicalOp      *operators.LogicalOperator
	numericOp      *operators.NumericOperator
	stringOp       *operators.StringOperator
	arrayOp        *operators.ArrayOperator
	customOpLookup CustomOperatorLookup
	schema         operators.SchemaProvider
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

// SetCustomOperatorLookup sets the function used to look up custom operators.
// This also sets up the validator to recognize custom operators.
func (p *Parser) SetCustomOperatorLookup(lookup CustomOperatorLookup) {
	p.customOpLookup = lookup
	// Also set up the validator to recognize custom operators
	p.validator.SetCustomOperatorChecker(func(operatorName string) bool {
		if lookup == nil {
			return false
		}
		_, ok := lookup(operatorName)
		return ok
	})
}

// SetSchema sets the schema provider for field validation and type checking
func (p *Parser) SetSchema(schema operators.SchemaProvider) {
	p.schema = schema
	// Set schema on all operators
	if p.dataOp != nil {
		p.dataOp.SetSchema(schema)
	}
	if p.comparisonOp != nil {
		p.comparisonOp.SetSchema(schema)
	}
	if p.logicalOp != nil {
		p.logicalOp.SetSchema(schema)
	}
	if p.numericOp != nil {
		p.numericOp.SetSchema(schema)
	}
	if p.stringOp != nil {
		p.stringOp.SetSchema(schema)
	}
	if p.arrayOp != nil {
		p.arrayOp.SetSchema(schema)
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
	// Check for custom operators first
	if p.customOpLookup != nil {
		if handler, ok := p.customOpLookup(operator); ok {
			// Process the arguments for the custom operator
			processedArgs, err := p.processCustomOperatorArgs(args)
			if err != nil {
				return "", fmt.Errorf("failed to process custom operator arguments: %v", err)
			}
			return handler.ToSQL(operator, processedArgs)
		}
	}

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
			// Process arguments to handle custom operators in nested expressions
			processedArgs, err := p.processArgs(arr)
			if err != nil {
				return "", fmt.Errorf("failed to process %s arguments: %v", operator, err)
			}
			return p.logicalOp.ToSQL(operator, processedArgs)
		}
		return "", fmt.Errorf("%s operator requires array arguments", operator)
	case "!", "!!":
		// These unary operators can accept both array and non-array arguments
		if arr, ok := args.([]interface{}); ok {
			// Process arguments to handle custom operators
			processedArgs, err := p.processArgs(arr)
			if err != nil {
				return "", fmt.Errorf("failed to process %s arguments: %v", operator, err)
			}
			return p.logicalOp.ToSQL(operator, processedArgs)
		}
		// Process non-array argument to handle custom operators before wrapping
		processedArg, err := p.processArg(args)
		if err != nil {
			return "", fmt.Errorf("failed to process %s argument: %v", operator, err)
		}
		return p.logicalOp.ToSQL(operator, []interface{}{processedArg})

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

// isBuiltInOperator checks if an operator is a built-in operator
func (p *Parser) isBuiltInOperator(operator string) bool {
	builtInOps := map[string]bool{
		// Data access
		"var": true, "missing": true, "missing_some": true,
		// Comparison
		"==": true, "===": true, "!=": true, "!==": true,
		">": true, ">=": true, "<": true, "<=": true, "in": true,
		// Logical
		"and": true, "or": true, "!": true, "!!": true, "if": true,
		// Numeric
		"+": true, "-": true, "*": true, "/": true, "%": true,
		"max": true, "min": true,
		// String
		"cat": true, "substr": true,
		// Array
		"map": true, "filter": true, "reduce": true,
		"all": true, "some": true, "none": true, "merge": true,
	}
	return builtInOps[operator]
}

// processArgs recursively processes arguments to handle custom operators at any nesting level.
// It converts custom operators to SQL while preserving the structure of built-in operators
// but with their nested custom operators already processed.
func (p *Parser) processArgs(args []interface{}) ([]interface{}, error) {
	processed := make([]interface{}, len(args))

	for i, arg := range args {
		processedArg, err := p.processArg(arg)
		if err != nil {
			return nil, err
		}
		processed[i] = processedArg
	}

	return processed, nil
}

// processArg processes a single argument, recursively handling custom operators
func (p *Parser) processArg(arg interface{}) (interface{}, error) {
	// If it's a complex expression (map with single key)
	if exprMap, ok := arg.(map[string]interface{}); ok {
		if len(exprMap) == 1 {
			for operator, opArgs := range exprMap {
				// Check if it's a custom operator (not built-in)
				if !p.isBuiltInOperator(operator) {
					// It's a custom operator, parse it to SQL
					return p.parseOperator(operator, opArgs)
				}

				// It's a built-in operator - recursively process its arguments
				// to handle any nested custom operators
				processedOpArgs, err := p.processOpArgs(opArgs)
				if err != nil {
					return nil, err
				}
				// Return the expression with processed arguments
				return map[string]interface{}{operator: processedOpArgs}, nil
			}
		}
		// Multi-key maps - keep as is
		return arg, nil
	}

	// Arrays need recursive processing too
	if arr, ok := arg.([]interface{}); ok {
		return p.processArgs(arr)
	}

	// Primitives - keep as is
	return arg, nil
}

// processOpArgs processes operator arguments (can be array or single value)
func (p *Parser) processOpArgs(opArgs interface{}) (interface{}, error) {
	if arr, ok := opArgs.([]interface{}); ok {
		return p.processArgs(arr)
	}
	// Single argument
	return p.processArg(opArgs)
}

// processCustomOperatorArgs processes arguments for custom operators.
// It converts all expressions (including var) to their SQL representation.
func (p *Parser) processCustomOperatorArgs(args interface{}) ([]interface{}, error) {
	// Handle array arguments
	if arr, ok := args.([]interface{}); ok {
		processed := make([]interface{}, len(arr))
		for i, arg := range arr {
			sql, err := p.processArgToSQL(arg)
			if err != nil {
				return nil, err
			}
			processed[i] = sql
		}
		return processed, nil
	}

	// Handle single argument (wrap in array)
	sql, err := p.processArgToSQL(args)
	if err != nil {
		return nil, err
	}
	return []interface{}{sql}, nil
}

// processArgToSQL converts a single argument to its SQL representation.
func (p *Parser) processArgToSQL(arg interface{}) (interface{}, error) {
	// Handle complex expressions (maps)
	if exprMap, ok := arg.(map[string]interface{}); ok {
		if len(exprMap) == 1 {
			for operator, opArgs := range exprMap {
				// Parse any expression (including var)
				sql, err := p.parseOperator(operator, opArgs)
				if err != nil {
					return nil, err
				}
				return sql, nil
			}
		}
	}

	// Handle primitive values - convert to SQL representation
	return p.primitiveToSQL(arg), nil
}

// primitiveToSQL converts a primitive value to its SQL representation.
func (p *Parser) primitiveToSQL(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		return fmt.Sprintf("'%s'", v)
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	case nil:
		return "NULL"
	default:
		// Numbers and other types
		return fmt.Sprintf("%v", v)
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
