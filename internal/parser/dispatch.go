//nolint:goconst // JSONLogic operator and SQL token strings stay inline in parser switches for readability.
package parser

import (
	"errors"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
)

func singleOperatorExpression(expr interface{}) (string, interface{}, bool) {
	obj, ok := expr.(map[string]interface{})
	if !ok || len(obj) != 1 {
		return "", nil, false
	}
	for operator, args := range obj {
		return operator, args, true
	}
	return "", nil, false
}

func isTranspileErrorCode(err error, code tperrors.ErrorCode) bool {
	var tpErr *tperrors.TranspileError
	return errors.As(err, &tpErr) && tpErr.Code == code
}

// wrapOperatorError wraps an operator error with TranspileError if it isn't already.
func (p *Parser) wrapOperatorError(operator, path string, err error) error {
	if err == nil {
		return nil
	}
	// Check if it's already a TranspileError
	var tpErr *tperrors.TranspileError
	if errors.As(err, &tpErr) {
		return err
	}
	// Wrap with appropriate error code based on error message
	return tperrors.Wrap(tperrors.ErrInvalidArgument, operator, path, "operator error", err)
}

// parseOperator parses a specific operator.
// path is the JSONPath to this operator for error reporting.
func (p *Parser) parseOperator(operator string, args interface{}, path string) (string, error) {
	// Check for custom operators first
	if p.customOpLookup != nil {
		if handler, ok := p.customOpLookup(operator); ok {
			// Process the arguments for the custom operator
			processedArgs, err := p.processCustomOperatorArgs(args, path)
			if err != nil {
				return "", tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"failed to process custom operator arguments", err)
			}
			res, err := handler.ToSQL(operator, processedArgs)
			if err != nil {
				return "", tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"custom operator failed", err)
			}
			return res.SQL, nil
		}
	}

	// Handle different operator types
	switch operator {
	// Data access operators
	case "var":
		sql, err := p.dataOp.ToSQL(operator, []interface{}{args})
		return sql, p.wrapOperatorError(operator, path, err)
	case "missing":
		// missing takes a single string argument, wrap it in an array
		sql, err := p.dataOp.ToSQL(operator, []interface{}{args})
		return sql, p.wrapOperatorError(operator, path, err)
	case "missing_some":
		if arr, ok := args.([]interface{}); ok {
			sql, err := p.dataOp.ToSQL(operator, arr)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	// Comparison operators
	case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
		if arr, ok := args.([]interface{}); ok {
			// Comparison operands are value expressions.
			processedArgs, err := p.processValueArgs(arr, path)
			if err != nil {
				return "", err
			}
			sql, err := p.comparisonOp.ToSQL(operator, processedArgs)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	// Logical operators
	case "and", "or", "if":
		if arr, ok := args.([]interface{}); ok {
			// Process arguments to handle custom operators in nested expressions
			processedArgs, err := p.processArgs(arr, path)
			if err != nil {
				return "", err
			}
			sql, err := p.logicalOp.ToSQL(operator, processedArgs)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)
	case "!", "!!":
		// These unary operators can accept both array and non-array arguments
		if arr, ok := args.([]interface{}); ok {
			// Process arguments to handle custom operators
			processedArgs, err := p.processArgs(arr, path)
			if err != nil {
				return "", err
			}
			sql, err := p.logicalOp.ToSQL(operator, processedArgs)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		// Process non-array argument to handle custom operators before wrapping
		processedArg, err := p.processArg(args, path, 0)
		if err != nil {
			return "", err
		}
		sql, err := p.logicalOp.ToSQL(operator, []interface{}{processedArg})
		return sql, p.wrapOperatorError(operator, path, err)

	// Numeric operators
	case "+", "-", "*", "/", "%", "max", "min":
		if arr, ok := args.([]interface{}); ok {
			// Process arguments to handle complex expressions
			processedArgs, err := p.processArgs(arr, path)
			if err != nil {
				return "", err
			}
			sql, err := p.numericOp.ToSQL(operator, processedArgs)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	// Array operators
	case operators.OpMap, operators.OpFilter, operators.OpReduce, operators.OpAll, operators.OpSome, operators.OpNone, operators.OpMerge:
		if arr, ok := args.([]interface{}); ok {
			sql, err := p.arrayOp.ToSQLAtPath(operator, arr, path)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	// String operators
	case "cat", "substr":
		if arr, ok := args.([]interface{}); ok {
			sql, err := p.stringOp.ToSQL(operator, arr)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	// All operators are now supported
	default:
		return "", tperrors.NewUnsupportedOperator(operator, path)
	}
}

// isBuiltInOperator checks if an operator is a built-in operator.
func (p *Parser) isBuiltInOperator(operator string) bool {
	switch operator {
	case "var", "missing", "missing_some",
		"==", "===", "!=", "!==", ">", ">=", "<", "<=", "in",
		"and", "or", "!", "!!", "if",
		"+", "-", "*", "/", "%", "max", "min",
		"cat", "substr",
		"map", "filter", "reduce", "all", "some", "none", "merge":
		return true
	}
	return false
}

// isArrayOperator checks if an operator introduces/depends on array expression
// semantics that should be delegated directly to ArrayOperator without parser-level
// eager argument preprocessing.
func (p *Parser) isArrayOperator(operator string) bool {
	switch operator {
	case operators.OpMap, operators.OpFilter, operators.OpReduce, operators.OpAll, operators.OpSome, operators.OpNone, operators.OpMerge:
		return true
	}
	return false
}

// processArgs recursively processes arguments to handle custom operators at any nesting level.
// It converts custom operators to SQL while preserving the structure of built-in operators
// but with their nested custom operators already processed.
// path is the JSONPath to the parent operator.
func (p *Parser) processArgs(args []interface{}, path string) ([]interface{}, error) {
	processed := make([]interface{}, len(args))

	for i, arg := range args {
		processedArg, err := p.processArg(arg, path, i)
		if err != nil {
			return nil, err
		}
		processed[i] = processedArg
	}

	return processed, nil
}

func (p *Parser) processValueArgs(args []interface{}, path string) ([]interface{}, error) {
	processed := make([]interface{}, len(args))

	for i, arg := range args {
		processedArg, err := p.processValueArg(arg, path, i)
		if err != nil {
			return nil, err
		}
		processed[i] = processedArg
	}

	return processed, nil
}

func (p *Parser) processValueArg(arg interface{}, path string, index int) (interface{}, error) {
	if p.isPrimitive(arg) {
		return arg, nil
	}
	if exprMap, ok := arg.(map[string]interface{}); ok {
		argPath := tperrors.BuildArrayPath(path, index)
		if len(exprMap) != 1 {
			return nil, tperrors.NewMultipleKeys(argPath)
		}
		for operator := range exprMap {
			if operator != "var" {
				res, err := p.parseExpressionValue(arg, argPath)
				if err != nil {
					return nil, err
				}
				if res.rawLiteralKnown {
					return res.rawLiteral, nil
				}
				if res.Kind == operators.ExpressionKindValue && valueTypeOf(res) == operators.ExpressionTypeNull {
					var nullLiteral interface{}
					return nullLiteral, nil
				}
				return typedValueOperand(res), nil
			}
		}
	}
	return p.processArg(arg, path, index)
}

// processArg processes a single argument, recursively handling custom operators.
// Returns ProcessedValue when SQL is generated, otherwise returns the original type.
// path is the JSONPath to the parent, index is the argument index.
func (p *Parser) processArg(arg interface{}, path string, index int) (interface{}, error) {
	argPath := tperrors.BuildArrayPath(path, index)

	// If it's a complex expression (map with single key)
	if exprMap, ok := arg.(map[string]interface{}); ok {
		if len(exprMap) != 1 {
			return nil, tperrors.NewMultipleKeys(argPath)
		}
		for operator, opArgs := range exprMap {
			operatorPath := tperrors.BuildPath(path, operator, index)

			// Check if it's a custom operator (not built-in)
			if !p.isBuiltInOperator(operator) {
				// It's a custom operator, parse it to SQL
				sql, err := p.parseOperator(operator, opArgs, operatorPath)
				if err != nil {
					return nil, err
				}
				// Wrap in ProcessedValue to mark as SQL
				return operators.SQLResult(sql), nil
			}

			// It's a built-in operator - recursively process its arguments
			// to handle any nested custom operators.
			// Array operators are handled specially: parse them immediately with
			// their full operatorPath so nested custom-operator failures preserve
			// complete JSONPath context under non-array parents (e.g. == / and).
			// ArrayOperator still performs scope-aware rewrites before nested
			// custom operators are parsed.
			if p.isArrayOperator(operator) {
				sql, err := p.parseOperator(operator, opArgs, operatorPath)
				if err != nil {
					return nil, err
				}
				return operators.SQLResult(sql), nil
			}
			processedOpArgs, err := p.processOpArgs(opArgs, operatorPath)
			if err != nil {
				return nil, err
			}
			// Return the expression with processed arguments
			return map[string]interface{}{operator: processedOpArgs}, nil
		}
	}

	// Arrays need recursive processing too
	if arr, ok := arg.([]interface{}); ok {
		return p.processArgs(arr, argPath)
	}

	// Primitives - keep as is
	return arg, nil
}

// processOpArgs processes operator arguments (can be array or single value).
// path is the JSONPath to the operator.
func (p *Parser) processOpArgs(opArgs interface{}, path string) (interface{}, error) {
	if arr, ok := opArgs.([]interface{}); ok {
		return p.processArgs(arr, path)
	}
	// Single argument
	return p.processArg(opArgs, path, 0)
}

// processCustomOperatorArgs processes arguments for custom operators.
// It converts all expressions (including var) to their SQL representation.
// path is the JSONPath to the custom operator.
func (p *Parser) processCustomOperatorArgs(args interface{}, path string) ([]operators.OperatorArg, error) {
	// Handle array arguments
	if arr, ok := args.([]interface{}); ok {
		processed := make([]operators.OperatorArg, len(arr))
		for i, arg := range arr {
			argPath := tperrors.BuildArrayPath(path, i)
			argResult, err := p.processArgToOperatorArg(arg, argPath)
			if err != nil {
				return nil, err
			}
			processed[i] = argResult
		}
		return processed, nil
	}

	// Handle single argument (wrap in array)
	argResult, err := p.processArgToOperatorArg(args, path)
	if err != nil {
		return nil, err
	}
	return []operators.OperatorArg{argResult}, nil
}

func (p *Parser) processArgToOperatorArg(arg interface{}, path string) (operators.OperatorArg, error) {
	res, err := p.parseExpressionAny(arg, path)
	if err != nil {
		return operators.OperatorArg{}, err
	}
	return operators.OperatorArg{SQL: res.SQL, Kind: res.Kind, Type: valueTypeOf(res)}, nil
}
