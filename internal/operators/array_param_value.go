package operators

import (
	"fmt"
	"strings"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func (a *ArrayOperator) valueToSQLParam(value interface{}, pc *params.ParamCollector) (string, error) {
	return a.valueToSQLParamAtPath(value, pc, a.currentPath())
}

func (a *ArrayOperator) valueExpressionToSQLParamWithContextAndPath(
	expr interface{},
	pc *params.ParamCollector,
	allowAccumulator bool,
	path string,
) (string, error) {
	res, err := a.valueExpressionResultParamWithContextAndPath(expr, pc, allowAccumulator, path)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) valueExpressionResultParamWithContextAndPath(
	expr interface{},
	pc *params.ParamCollector,
	allowAccumulator bool,
	path string,
) (OperatorResult, error) {
	if a.config == nil || !a.config.HasParamValueExpressionParser() {
		sql, err := a.expressionToSQLParamWithContextAndPath(expr, pc, allowAccumulator, path)
		if err != nil {
			return OperatorResult{}, err
		}
		return a.localValueExpressionResult(expr, sql), nil
	}
	if a.shouldParseScopedArrayExpressionLocally(expr) {
		sql, err := a.expressionToSQLParamWithContextAndPath(expr, pc, allowAccumulator, path)
		if err != nil {
			return OperatorResult{}, err
		}
		return a.localValueExpressionResult(expr, sql), nil
	}
	rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(expr, allowAccumulator, path)
	if err != nil {
		return OperatorResult{}, err
	}
	res, err := a.config.ParseValueExpressionParam(rewritten, path, pc)
	if err != nil {
		return OperatorResult{}, err
	}
	return res, nil
}

func (a *ArrayOperator) predicateExpressionToSQLParamWithContextAndPath(
	expr interface{},
	pc *params.ParamCollector,
	path string,
) (string, error) {
	if a.config == nil || !a.config.HasParamPredicateExpressionParser() {
		return a.expressionToSQLParamWithContextAndPath(expr, pc, false, path)
	}
	if a.shouldParseScopedArrayExpressionLocally(expr) {
		return a.expressionToSQLParamWithContextAndPath(expr, pc, false, path)
	}
	rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(expr, false, path)
	if err != nil {
		return "", err
	}
	res, err := a.config.ParsePredicateExpressionParam(rewritten, path, pc)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) truthinessExpressionToSQLParamWithContextAndPath(
	expr interface{},
	pc *params.ParamCollector,
	path string,
) (string, error) {
	if a.config == nil || !a.config.HasParamTruthinessExpressionParser() {
		return a.predicateExpressionToSQLParamWithContextAndPath(expr, pc, path)
	}
	if a.shouldParseScopedArrayExpressionLocally(expr) {
		sql, err := a.expressionToSQLParamWithContextAndPath(expr, pc, false, path)
		if err != nil {
			return "", err
		}
		return a.localTruthinessExpressionSQL(expr, sql, path)
	}
	rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(expr, false, path)
	if err != nil {
		return "", err
	}
	return a.config.ParseTruthinessExpressionParam(rewritten, path, pc)
}

func (a *ArrayOperator) valueToSQLParamAtPath(value interface{}, pc *params.ParamCollector, path string) (string, error) {
	result, err := a.valueToTypedSQLParamAtPath(value, pc, path)
	if err != nil {
		return "", err
	}
	return result.sql, nil
}

func (a *ArrayOperator) valueToTypedSQLParamAtPath(value interface{}, pc *params.ParamCollector, path string) (typedValueSQL, error) {
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			if pv.HasExpressionInfo {
				return typedSQLFromOperatorResult(OperatorResult{
					SQL:               pv.Value,
					Kind:              pv.Kind,
					Type:              pv.Type,
					ArrayElementType:  pv.ArrayElementType,
					ArrayElementTypes: pv.ArrayElementTypes,
				}), nil
			}
			return typedValueSQL{sql: pv.Value, typ: ExpressionTypeUnknown}, nil
		}
		sql, err := a.dataOp.valueToSQLParam(pv.Value, pc)
		if err != nil {
			return typedValueSQL{}, err
		}
		return typedValueSQL{sql: sql, typ: inferLiteralValueExpressionType(pv.Value)}, nil
	}

	if expr, ok := value.(map[string]interface{}); ok {
		if len(expr) != 1 {
			return typedValueSQL{}, tperrors.NewMultipleKeys(path)
		}
		if varExpr, hasVar := expr[OpVar]; hasVar {
			if sql, handled, err := a.arrayInternalVarToSQLParam(varExpr, pc); handled || err != nil {
				if err != nil {
					return typedValueSQL{}, err
				}
				return typedValueSQL{sql: sql, typ: a.inferValueExpressionType(value)}, nil
			}
			sql, err := a.dataOp.ToSQLParam(OpVar, []interface{}{varExpr}, pc)
			if err != nil {
				return typedValueSQL{}, err
			}
			fieldType := a.schemaExpressionType(a.extractFieldName(varExpr))
			if fieldType == ExpressionTypeUnknown {
				fieldType = a.inferValueExpressionType(value)
			}
			return typedValueSQL{sql: sql, typ: fieldType}, nil
		}
		res, err := a.valueExpressionResultParamWithContextAndPath(value, pc, false, path)
		if err != nil {
			return typedValueSQL{}, err
		}
		return typedSQLFromOperatorResult(res), nil
	}

	if arr, ok := value.([]interface{}); ok {
		elements := make([]string, len(arr))
		var commonTypes []ExpressionType
		for i, elem := range arr {
			element, err := a.valueToTypedSQLParamAtPath(elem, pc, tperrors.BuildArrayPath(path, i))
			if err != nil {
				return typedValueSQL{}, fmt.Errorf("invalid array element %d: %w", i, err)
			}
			commonTypes, err = updateArrayLiteralElementTypes(commonTypes, element, i)
			if err != nil {
				return typedValueSQL{}, err
			}
			elements[i] = element.sql
		}
		elementTypes := normalizeArrayElementTypes(commonTypes)
		if err := a.config.ValidateArrayLiteralElementTypes(elementTypes); err != nil {
			return typedValueSQL{}, err
		}
		sql, err := a.arrayLiteral(elements)
		if err != nil {
			return typedValueSQL{}, err
		}
		return typedValueSQL{
			sql:               sql,
			typ:               ExpressionTypeArray,
			elemType:          firstArrayElementType(elementTypes),
			elemTypes:         elementTypes,
			emptyArrayLiteral: len(arr) == 0,
		}, nil
	}

	sql, err := a.dataOp.valueToSQLParam(value, pc)
	if err != nil {
		return typedValueSQL{}, err
	}
	return typedValueSQL{sql: sql, typ: inferLiteralValueExpressionType(value)}, nil
}

func expressionTypeFromResultKind(kind ExpressionKind, typ ExpressionType) ExpressionType {
	if kind == ExpressionKindPredicate {
		return ExpressionTypeBoolean
	}
	return typ
}

func typedSQLFromOperatorResult(res OperatorResult) typedValueSQL {
	typ := expressionTypeFromResultKind(res.Kind, res.Type)
	out := typedValueSQL{
		sql:               res.SQL,
		typ:               typ,
		emptyArrayLiteral: res.EmptyArrayLiteral,
	}
	if typ == ExpressionTypeArray {
		elemTypes := operatorResultElementTypes(res)
		if len(elemTypes) > 0 {
			out.elemType = elemTypes[0]
			out.elemTypes = elemTypes
		}
	}
	return out
}

func (a *ArrayOperator) expressionToSQLParamWithContextAndPath(
	expr interface{},
	pc *params.ParamCollector,
	allowAccumulator bool,
	path string,
) (string, error) {
	if pv, ok := expr.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		return a.expressionToSQLParamWithContextAndPath(pv.Value, pc, allowAccumulator, path)
	}

	if a.isPrimitive(expr) {
		return a.dataOp.valueToSQLParam(expr, pc)
	}

	if varExpr, ok := expr.(map[string]interface{}); ok {
		if len(varExpr) != 1 {
			return "", tperrors.NewMultipleKeys(path)
		}
		if varName, hasVar := varExpr[OpVar]; hasVar {
			if allowAccumulator {
				if rewritten, handled, err := a.rewriteAccumulatorVarParam(varName); handled || err != nil {
					if err != nil {
						return "", err
					}
					if pv, ok := rewritten.(ProcessedValue); ok && pv.IsSQL {
						return pv.Value, nil
					}
					if rewrittenVar, ok := rewritten.(map[string]interface{}); ok {
						return a.dataOp.ToSQLParam(OpVar, []interface{}{rewrittenVar[OpVar]}, pc)
					}
				}
			}
			if sql, handled, err := a.arrayScopeVarToSQLParam(varName, pc); handled || err != nil {
				return sql, err
			}
			return a.dataOp.ToSQLParam(OpVar, []interface{}{varName}, pc)
		}
	}

	if exprMap, ok := expr.(map[string]interface{}); ok {
		for operator, args := range exprMap {
			if a.valueSemantics && operator != OpVar && !a.isArrayOperator(operator) && a.config != nil && a.config.HasParamValueExpressionParser() {
				return a.valueExpressionToSQLParamWithContextAndPath(exprMap, pc, allowAccumulator, path)
			}
			switch operator {
			case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
				if arr, ok := args.([]interface{}); ok {
					opPath := tperrors.BuildPath(path, operator, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(arr, allowAccumulator, opPath)
					if err != nil {
						return "", err
					}
					converted, ok := rewrittenArgs.([]interface{})
					if !ok {
						return "", fmt.Errorf("internal error: expected []interface{} for rewritten comparison args")
					}
					arr = converted
					return a.comparisonOp.ToSQLParam(operator, arr, pc)
				}
			case "and", "or":
				if arr, ok := args.([]interface{}); ok {
					if a.valueSemantics && a.config != nil && a.config.HasParamValueExpressionParser() {
						return a.valueExpressionToSQLParamWithContextAndPath(exprMap, pc, allowAccumulator, path)
					}
					opPath := tperrors.BuildPath(path, operator, -1)
					parts := make([]string, len(arr))
					for i, arg := range arr {
						part, err := a.expressionToSQLParamWithContextAndPath(arg, pc, allowAccumulator, tperrors.BuildArrayPath(opPath, i))
						if err != nil {
							return "", err
						}
						parts[i] = part
					}
					if len(parts) == 1 {
						return parts[0], nil
					}
					joiner := " AND "
					if operator == "or" {
						joiner = " OR "
					}
					return fmt.Sprintf("(%s)", strings.Join(parts, joiner)), nil
				}
			case "!", "!!", "if":
				if arr, ok := args.([]interface{}); ok {
					if a.valueSemantics && operator == "if" && a.config != nil && a.config.HasParamValueExpressionParser() {
						return a.valueExpressionToSQLParamWithContextAndPath(exprMap, pc, allowAccumulator, path)
					}
					opPath := tperrors.BuildPath(path, operator, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(arr, allowAccumulator, opPath)
					if err != nil {
						return "", err
					}
					converted, ok := rewrittenArgs.([]interface{})
					if !ok {
						return "", fmt.Errorf("internal error: expected []interface{} for rewritten logical args")
					}
					arr = converted
					return a.getLogicalOperator().ToSQLParam(operator, arr, pc)
				}
			case "+", "-", "*", "/", "%", "max", "min":
				if arr, ok := args.([]interface{}); ok {
					opPath := tperrors.BuildPath(path, operator, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(arr, allowAccumulator, opPath)
					if err != nil {
						return "", err
					}
					converted, ok := rewrittenArgs.([]interface{})
					if !ok {
						return "", fmt.Errorf("internal error: expected []interface{} for rewritten numeric args")
					}
					arr = converted
					return a.numericOp.ToSQLParam(operator, arr, pc)
				}
			case "map", "filter", "reduce", "all", "some", "none", "merge":
				if arr, ok := args.([]interface{}); ok {
					target := a
					nestedArgs := arr
					if a.shouldUseChildScope(operator, arr) {
						var rewriteErr error
						nestedArgs, rewriteErr = a.rewriteNestedArrayOuterScopeArgsParam(operator, arr, allowAccumulator, path)
						if rewriteErr != nil {
							return "", rewriteErr
						}
						target = a.withChildScope()
					}
					target = target.withValueScope(true)
					target = target.withValueSemantics(false)
					nestedPath := tperrors.BuildPath(path, operator, -1)
					return target.withPath(nestedPath).ToSQLParam(operator, nestedArgs, pc)
				}
			default:
				if a.config != nil && a.config.HasParamExpressionParser() {
					rewrittenExpr, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(exprMap, allowAccumulator, path)
					if err != nil {
						return "", err
					}
					if pv, ok := rewrittenExpr.(ProcessedValue); ok && pv.IsSQL {
						return pv.Value, nil
					}
					return a.config.ParseExpressionParam(rewrittenExpr, path, pc)
				}
				return "", fmt.Errorf("unsupported operator in array expression: %s", operator)
			}
		}
	}

	return "", fmt.Errorf("invalid expression type: %T", expr)
}

// arrayScopeVarToSQLParam is the parameterized variant of arrayScopeVarToSQL. Keep in sync.
