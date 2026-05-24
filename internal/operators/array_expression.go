package operators

import (
	"fmt"
	"strings"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
)

func (a *ArrayOperator) expressionToSQLWithContextAndPath(expr interface{}, allowAccumulator bool, path string) (string, error) {
	// Handle ProcessedValue (pre-processed SQL from parser)
	if pv, ok := expr.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		// It's a literal, recursively convert it
		return a.expressionToSQLWithContextAndPath(pv.Value, allowAccumulator, path)
	}

	// Handle primitive values
	if a.isPrimitive(expr) {
		return a.dataOp.valueToSQL(expr)
	}

	// Handle var expressions
	if varExpr, ok := expr.(map[string]interface{}); ok {
		if len(varExpr) != 1 {
			return "", tperrors.NewMultipleKeys(path)
		}
		if varName, hasVar := varExpr[OpVar]; hasVar {
			if allowAccumulator {
				if rewritten, handled, err := a.rewriteAccumulatorVar(varName); handled || err != nil {
					return rewritten.Value, err
				}
			}
			if sql, handled, err := a.arrayScopeVarToSQL(varName); handled || err != nil {
				return sql, err
			}
			return a.dataOp.ToSQL(OpVar, []interface{}{varName})
		}
	}

	// Handle complex expressions by delegating to other operators
	if exprMap, ok := expr.(map[string]interface{}); ok {
		for operator, args := range exprMap {
			if a.valueSemantics && operator != OpVar && !a.isArrayOperator(operator) && a.config != nil && a.config.HasValueExpressionParser() {
				return a.valueExpressionToSQLWithContextAndPath(exprMap, allowAccumulator, path)
			}
			switch operator {
			case OpEqual, OpStrictEqual, OpNotEqual, OpStrictNotEqual,
				OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual, OpIn:
				if arr, ok := args.([]interface{}); ok {
					opPath := tperrors.BuildPath(path, operator, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorWithContextAndPath(arr, allowAccumulator, opPath)
					if err != nil {
						return "", err
					}
					converted, ok := rewrittenArgs.([]interface{})
					if !ok {
						return "", fmt.Errorf("internal error: expected []interface{} for rewritten comparison args")
					}
					arr = converted
					return a.comparisonOp.ToSQL(operator, arr)
				}
			case OpAnd, OpOr:
				if arr, ok := args.([]interface{}); ok {
					if a.valueSemantics && a.config != nil && a.config.HasValueExpressionParser() {
						return a.valueExpressionToSQLWithContextAndPath(exprMap, allowAccumulator, path)
					}
					opPath := tperrors.BuildPath(path, operator, -1)
					parts := make([]string, len(arr))
					for i, arg := range arr {
						part, err := a.expressionToSQLWithContextAndPath(arg, allowAccumulator, tperrors.BuildArrayPath(opPath, i))
						if err != nil {
							return "", err
						}
						parts[i] = part
					}
					if len(parts) == 1 {
						return parts[0], nil
					}
					joiner := sqlAndJoiner
					if operator == OpOr {
						joiner = sqlOrJoiner
					}
					return fmt.Sprintf("(%s)", strings.Join(parts, joiner)), nil
				}
			case OpNot, OpDoubleBang, OpIf:
				if arr, ok := args.([]interface{}); ok {
					if a.valueSemantics && operator == OpIf && a.config != nil && a.config.HasValueExpressionParser() {
						return a.valueExpressionToSQLWithContextAndPath(exprMap, allowAccumulator, path)
					}
					opPath := tperrors.BuildPath(path, operator, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorWithContextAndPath(arr, allowAccumulator, opPath)
					if err != nil {
						return "", err
					}
					converted, ok := rewrittenArgs.([]interface{})
					if !ok {
						return "", fmt.Errorf("internal error: expected []interface{} for rewritten logical args")
					}
					arr = converted
					return a.getLogicalOperator().ToSQL(operator, arr)
				}
			case OpAdd, OpSubtract, OpMultiply, OpDivide, OpModulo, OpMax, OpMin:
				if arr, ok := args.([]interface{}); ok {
					opPath := tperrors.BuildPath(path, operator, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorWithContextAndPath(arr, allowAccumulator, opPath)
					if err != nil {
						return "", err
					}
					converted, ok := rewrittenArgs.([]interface{})
					if !ok {
						return "", fmt.Errorf("internal error: expected []interface{} for rewritten numeric args")
					}
					arr = converted
					return a.numericOp.ToSQL(operator, arr)
				}
			case OpMap, OpFilter, OpReduce, OpAll, OpSome, OpNone, OpMerge:
				// Handle nested array operators
				if arr, ok := args.([]interface{}); ok {
					target := a
					nestedArgs := arr
					if a.shouldUseChildScope(operator, arr) {
						var rewriteErr error
						nestedArgs, rewriteErr = a.rewriteNestedArrayOuterScopeArgs(operator, arr, allowAccumulator, path)
						if rewriteErr != nil {
							return "", rewriteErr
						}
						target = a.withChildScope()
					}
					target = target.withValueScope(true)
					target = target.withValueSemantics(false)
					nestedPath := tperrors.BuildPath(path, operator, -1)
					return target.withPath(nestedPath).ToSQL(operator, nestedArgs)
				}
			default:
				// Try to use the expression parser callback for unknown operators
				// This enables support for custom operators in nested contexts
				if a.config != nil && a.config.HasExpressionParser() {
					rewrittenExpr, err := a.rewriteScopedVarsForOperatorWithContextAndPath(exprMap, allowAccumulator, path)
					if err != nil {
						return "", err
					}
					if pv, ok := rewrittenExpr.(ProcessedValue); ok && pv.IsSQL {
						return pv.Value, nil
					}
					return a.config.ParseExpression(rewrittenExpr, path)
				}
				return "", fmt.Errorf("unsupported operator in array expression: %s", operator)
			}
		}
	}

	return "", fmt.Errorf("invalid expression type: %T", expr)
}
