package operators

import (
	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
)

func (a *ArrayOperator) shouldUseRenderedSourceChildScope(op string, args []interface{}) bool {
	return a.shouldUseChildScope(op, args)
}

func (a *ArrayOperator) rewriteNestedArrayOuterScopeArgs(
	operator string,
	args []interface{},
	allowAccumulator bool,
	path string,
) ([]interface{}, error) {
	rewrittenArgs := make([]interface{}, len(args))
	copy(rewrittenArgs, args)

	opPath := tperrors.BuildPath(path, operator, -1)
	if len(rewrittenArgs) > 0 {
		rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(
			args[0],
			allowAccumulator,
			tperrors.BuildArrayPath(opPath, 0),
		)
		if err != nil {
			return nil, err
		}
		rewrittenArgs[0] = rewritten
	}
	if operator == OpReduce && len(rewrittenArgs) > arrayReduceInitialArgIndex {
		rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(
			args[arrayReduceInitialArgIndex],
			allowAccumulator,
			tperrors.BuildArrayPath(opPath, arrayReduceInitialArgIndex),
		)
		if err != nil {
			return nil, err
		}
		rewrittenArgs[arrayReduceInitialArgIndex] = rewritten
	}
	return rewrittenArgs, nil
}

func (a *ArrayOperator) rewriteNestedArrayOuterScopeArgsParam(
	operator string,
	args []interface{},
	allowAccumulator bool,
	path string,
) ([]interface{}, error) {
	rewrittenArgs := make([]interface{}, len(args))
	copy(rewrittenArgs, args)

	opPath := tperrors.BuildPath(path, operator, -1)
	if len(rewrittenArgs) > 0 {
		rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(
			args[0],
			allowAccumulator,
			tperrors.BuildArrayPath(opPath, 0),
		)
		if err != nil {
			return nil, err
		}
		rewrittenArgs[0] = rewritten
	}
	if operator == OpReduce && len(rewrittenArgs) > arrayReduceInitialArgIndex {
		rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(
			args[arrayReduceInitialArgIndex],
			allowAccumulator,
			tperrors.BuildArrayPath(opPath, arrayReduceInitialArgIndex),
		)
		if err != nil {
			return nil, err
		}
		rewrittenArgs[arrayReduceInitialArgIndex] = rewritten
	}
	return rewrittenArgs, nil
}

func isRenderedArrayScopeSource(value interface{}) bool {
	switch v := value.(type) {
	case ProcessedValue:
		return v.IsSQL && v.IsField
	case map[string]interface{}:
		if len(v) != 1 {
			return false
		}
		varName, ok := v[OpVar]
		if !ok {
			return false
		}
		arr, ok := varName.([]interface{})
		if !ok || len(arr) == 0 {
			return false
		}
		pv, ok := arr[0].(ProcessedValue)
		return ok && pv.IsSQL && pv.IsField
	default:
		return false
	}
}
