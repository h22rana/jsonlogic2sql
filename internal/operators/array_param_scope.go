package operators

import (
	"fmt"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func (a *ArrayOperator) arrayScopeVarToSQLParam(varExpr interface{}, pc *params.ParamCollector) (string, bool, error) {
	if varName, ok := varExpr.(string); ok {
		mapped, handled, err := a.mapArrayScopeVar(varName)
		if err != nil {
			return "", true, err
		}
		if handled {
			return mapped, true, nil
		}
		return "", false, nil
	}

	if arr, ok := varExpr.([]interface{}); ok {
		if len(arr) == 0 {
			return "", false, nil
		}
		if err := validateVarArrayMaxEntries(arr); err != nil {
			return "", true, err
		}
		varName, ok := arr[0].(string)
		if !ok {
			return "", false, nil
		}
		mapped, handled, err := a.mapArrayScopeVar(varName)
		if err != nil {
			return "", true, err
		}
		if !handled {
			return "", false, nil
		}
		if len(arr) == 1 {
			return mapped, true, nil
		}
		if defaultErr := a.validateArrayScopeVarDefault(varName, arr[1]); defaultErr != nil {
			return "", true, defaultErr
		}
		defaultSQL, err := a.dataOp.defaultValueToSQLParam(arr[1], pc)
		if err != nil {
			return "", true, fmt.Errorf("invalid default value: %w", err)
		}
		return a.config.CoalesceSQL(mapped, defaultSQL), true, nil
	}

	return "", false, nil
}

func (a *ArrayOperator) rewriteArrayScopeVar(varExpr interface{}) (interface{}, bool, error) {
	if varName, ok := varExpr.(string); ok {
		mapped, handled, err := a.mapArrayScopeVar(varName)
		if err != nil {
			return nil, true, err
		}
		if handled {
			return a.scopedSQLFieldResult(mapped, a.scopedFieldNamesForVar(varName)...), true, nil
		}
		return nil, false, nil
	}

	if arr, ok := varExpr.([]interface{}); ok {
		if len(arr) == 0 {
			return nil, false, nil
		}
		if err := validateVarArrayMaxEntries(arr); err != nil {
			return nil, true, err
		}
		varName, ok := arr[0].(string)
		if !ok {
			return nil, false, nil
		}
		mapped, handled, err := a.mapArrayScopeVar(varName)
		if err != nil {
			return nil, true, err
		}
		if !handled {
			return nil, false, nil
		}
		if len(arr) == 1 {
			return a.scopedSQLFieldResult(mapped, a.scopedFieldNamesForVar(varName)...), true, nil
		}
		newArr := make([]interface{}, len(arr))
		copy(newArr, arr)
		newArr[0] = a.scopedSQLFieldResult(mapped, a.scopedFieldNamesForVar(varName)...)
		return map[string]interface{}{OpVar: newArr}, true, nil
	}

	return nil, false, nil
}

func (a *ArrayOperator) rewriteArrayScopeVarParam(varExpr interface{}) (interface{}, bool, error) {
	return a.rewriteArrayScopeVar(varExpr)
}

// arrayInternalVarToSQLParam is the parameterized variant of arrayInternalVarToSQL.
func (a *ArrayOperator) arrayInternalVarToSQLParam(varExpr interface{}, pc *params.ParamCollector) (string, bool, error) {
	if a.lambdaScope != arrayLambdaScopeNone {
		return a.arrayScopeVarToSQLParam(varExpr, pc)
	}
	if varName, ok := varExpr.(string); ok {
		if varName == "" {
			if !a.valueScope {
				return "", false, nil
			}
			return a.elemAlias(), true, nil
		}
		if a.valueScope && a.isVisibleElemPath(varName) {
			quoted, err := a.quoteArrayScopeIdentifier(varName)
			if err != nil {
				return "", true, err
			}
			return quoted, true, nil
		}
		return "", false, nil
	}

	if arr, ok := varExpr.([]interface{}); ok {
		if len(arr) == 0 {
			return "", false, nil
		}
		if err := validateVarArrayMaxEntries(arr); err != nil {
			return "", true, err
		}
		varName, ok := arr[0].(string)
		if !ok {
			return "", false, nil
		}
		var mapped string
		switch {
		case varName == "":
			if !a.valueScope {
				return "", false, nil
			}
			mapped = a.elemAlias()
		case a.valueScope && a.isVisibleElemPath(varName):
			quoted, err := a.quoteArrayScopeIdentifier(varName)
			if err != nil {
				return "", true, err
			}
			mapped = quoted
		default:
			return "", false, nil
		}
		if len(arr) == 1 {
			return mapped, true, nil
		}
		if err := a.validateArrayScopeVarDefault(varName, arr[1]); err != nil {
			return "", true, err
		}
		defaultSQL, err := a.dataOp.defaultValueToSQLParam(arr[1], pc)
		if err != nil {
			return "", true, fmt.Errorf("invalid default value: %w", err)
		}
		return a.config.CoalesceSQL(mapped, defaultSQL), true, nil
	}

	return "", false, nil
}

func (a *ArrayOperator) rewriteScopedVarsForOperatorParamWithContextAndPath(
	expr interface{},
	allowAccumulator bool,
	path string,
) (interface{}, error) {
	switch e := expr.(type) {
	case map[string]interface{}:
		if len(e) != 1 {
			return nil, tperrors.NewMultipleKeys(path)
		}
		if varName, hasVar := e[OpVar]; hasVar {
			if allowAccumulator {
				if rewritten, handled, err := a.rewriteAccumulatorVarParam(varName); handled || err != nil {
					if err != nil {
						return nil, err
					}
					return rewritten, nil
				}
			}
			if rewritten, handled, err := a.rewriteArrayScopeVarParam(varName); handled || err != nil {
				if err != nil {
					return nil, err
				}
				return rewritten, nil
			}
			return e, nil
		}
		for opName, opArgs := range e {
			if rewrittenArgs, handled, err := a.rewriteScopedMissingFields(opName, opArgs, allowAccumulator); handled || err != nil {
				if err != nil {
					return nil, err
				}
				return map[string]interface{}{opName: rewrittenArgs}, nil
			}
			if a.isArrayOperator(opName) {
				arr, ok := opArgs.([]interface{})
				if !ok {
					return e, nil
				}
				newArgs := make([]interface{}, len(arr))
				copy(newArgs, arr)
				opPath := tperrors.BuildPath(path, opName, -1)
				if len(newArgs) > 0 {
					rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(arr[0], allowAccumulator, tperrors.BuildArrayPath(opPath, 0))
					if err != nil {
						return nil, err
					}
					newArgs[0] = rewritten
				}
				if opName == OpReduce && len(newArgs) > 2 {
					rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(arr[2], allowAccumulator, tperrors.BuildArrayPath(opPath, 2))
					if err != nil {
						return nil, err
					}
					newArgs[2] = rewritten
				}
				return map[string]interface{}{opName: newArgs}, nil
			}
			if !a.isBuiltInOperatorName(opName) {
				opPath := tperrors.BuildPath(path, opName, -1)
				rewrittenArgs, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(opArgs, allowAccumulator, tperrors.BuildArrayPath(opPath, 0))
				if err != nil {
					return nil, err
				}
				return map[string]interface{}{opName: rewrittenArgs}, nil
			}
		}
		result := make(map[string]interface{}, len(e))
		for k, v := range e {
			rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(v, allowAccumulator, tperrors.BuildPath(path, k, -1))
			if err != nil {
				return nil, err
			}
			result[k] = rewritten
		}
		return result, nil

	case []interface{}:
		result := make([]interface{}, len(e))
		for i, v := range e {
			rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(v, allowAccumulator, tperrors.BuildArrayPath(path, i))
			if err != nil {
				return nil, err
			}
			result[i] = rewritten
		}
		return result, nil

	default:
		return expr, nil
	}
}
