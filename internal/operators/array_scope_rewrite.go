package operators

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
)

func (a *ArrayOperator) quoteArrayScopePath(alias, suffix string) (string, error) {
	if suffix == "" {
		return alias, nil
	}
	return a.quoteArrayScopeIdentifier(alias + "." + suffix)
}

func (a *ArrayOperator) quoteArrayScopeIdentifier(name string) (string, error) {
	needsQuoting := false
	var validateErr error
	forEachDottedSegment(name, func(seg string) {
		if validateErr != nil {
			return
		}
		if dialect.ContainsQuoteCharacters(seg) {
			validateErr = fmt.Errorf("array-scope variable name %q contains quote characters; "+
				"use raw identifiers — the transpiler handles quoting automatically", name)
			return
		}
		if !isValidIdentifierSegment(seg) {
			validateErr = fmt.Errorf("invalid identifier %q: each segment must contain only letters, digits, or underscores", name)
			return
		}
		if dialect.NeedsQuoting(seg) {
			needsQuoting = true
		}
	})
	if validateErr != nil {
		return "", validateErr
	}
	if !needsQuoting {
		return name, nil
	}

	var out strings.Builder
	out.Grow(len(name) + 4)
	firstSegment := true
	forEachDottedSegment(name, func(seg string) {
		if !firstSegment {
			out.WriteByte('.')
		}
		firstSegment = false
		if dialect.NeedsQuoting(seg) {
			out.WriteString(dialect.QuoteIdentifierSegment(seg, a.getDialect()))
			return
		}
		out.WriteString(seg)
	})
	return out.String(), nil
}

// mapArrayScopeVar maps var names according to the active JSONLogic lambda
// scope. Element lambdas resolve bare fields against the current array element;
// reduce lambdas only expose the official current/accumulator bindings.
func (a *ArrayOperator) mapArrayScopeVar(varName string) (string, bool, error) {
	switch a.lambdaScope {
	case arrayLambdaScopeElement:
		return a.mapElementScopeVar(varName)
	case arrayLambdaScopeReduce:
		return a.mapReduceScopeVar(varName)
	case arrayLambdaScopeNone:
		if a.isVisibleElemPath(varName) {
			scope, fieldName := a.visibleElemScopeAndFieldName(varName)
			if fieldName != "" {
				if _, err := a.resolveFieldInScope(scope, fieldName); err != nil {
					return "", true, err
				}
			}
			quoted, err := a.quoteArrayScopeIdentifier(varName)
			if err != nil {
				return "", true, err
			}
			return quoted, true, nil
		}
		return "", false, nil
	default:
		return "", false, nil
	}
}

func (a *ArrayOperator) mapElementScopeVar(varName string) (string, bool, error) {
	if a.isUnsupportedElementScopeVar(varName) {
		return "", true, unsupportedArrayScopeVarError(varName)
	}
	if varName == "" {
		return a.elemAlias(), true, nil
	}
	if err := a.validateScopedFieldName(varName); err != nil {
		return "", true, err
	}
	quoted, err := a.quoteArrayScopePath(a.elemAlias(), varName)
	if err != nil {
		return "", true, err
	}
	return quoted, true, nil
}

func (a *ArrayOperator) mapReduceScopeVar(varName string) (string, bool, error) {
	if isUnsupportedReduceScopeVar(varName) {
		return "", true, unsupportedArrayScopeVarError(varName)
	}
	switch {
	case varName == CurrentVar:
		return a.elemAlias(), true, nil
	case strings.HasPrefix(varName, CurrentVar+"."):
		suffix := strings.TrimPrefix(varName, CurrentVar+".")
		if suffix == "" {
			return "", true, fmt.Errorf("unsupported reduce-scope variable %q; use %q, %q.<field>, or %q",
				varName, CurrentVar, CurrentVar, AccumulatorVar)
		}
		if err := a.validateScopedFieldName(suffix); err != nil {
			return "", true, err
		}
		quoted, err := a.quoteArrayScopePath(a.elemAlias(), suffix)
		if err != nil {
			return "", true, err
		}
		return quoted, true, nil
	case varName == "":
		return "", true, fmt.Errorf("unsupported reduce-scope variable %q; use %q or %q", varName, CurrentVar, AccumulatorVar)
	default:
		return "", true, fmt.Errorf("unsupported reduce-scope variable %q; use %q, %q.<field>, or %q",
			varName, CurrentVar, CurrentVar, AccumulatorVar)
	}
}

func (a *ArrayOperator) isUnsupportedElementScopeVar(varName string) bool {
	if isAlwaysUnsupportedElementScopeVar(varName) {
		return true
	}
	return (isBaseElemAliasPath(varName) || isNumberedElemAliasPath(varName)) && !a.hasScopedField(varName)
}

func isAlwaysUnsupportedElementScopeVar(varName string) bool {
	return strings.HasPrefix(varName, ".") ||
		strings.HasPrefix(varName, ItemVar+".") ||
		strings.HasPrefix(varName, CurrentVar+".")
}

func isUnsupportedReduceScopeVar(varName string) bool {
	return strings.HasPrefix(varName, ".") ||
		strings.HasPrefix(varName, ItemVar+".") ||
		isBaseElemAliasPath(varName) ||
		isNumberedElemAliasPath(varName)
}

func isBaseElemAliasPath(varName string) bool {
	return strings.HasPrefix(varName, ElemVar+".")
}

func isNumberedElemAliasPath(varName string) bool {
	if !strings.HasPrefix(varName, ElemVar) {
		return false
	}
	rest := varName[len(ElemVar):]
	hasDigit := false
	for len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
		hasDigit = true
		rest = rest[1:]
	}
	return hasDigit && strings.HasPrefix(rest, ".")
}

func unsupportedArrayScopeVarError(varName string) error {
	return fmt.Errorf("unsupported array-scope variable %q; use bare field names relative to the current element", varName)
}

func (a *ArrayOperator) scopedFieldNamesFromVarExpr(varExpr interface{}) []string {
	switch v := varExpr.(type) {
	case string:
		return a.scopedFieldNamesForVar(v)
	case []interface{}:
		if len(v) == 0 {
			return nil
		}
		switch first := v[0].(type) {
		case string:
			return a.scopedFieldNamesForVar(first)
		case ProcessedValue:
			if first.IsSQL && first.IsField {
				return first.SchemaFieldNames()
			}
		}
	case ProcessedValue:
		if v.IsSQL && v.IsField {
			return v.SchemaFieldNames()
		}
	}
	return nil
}

func (a *ArrayOperator) scopedFieldNamesForVar(varName string) []string {
	switch a.lambdaScope {
	case arrayLambdaScopeElement:
		if varName == "" || a.isUnsupportedElementScopeVar(varName) {
			return nil
		}
		return a.resolveScopedFieldNames(varName)
	case arrayLambdaScopeReduce:
		if varName == "" || varName == CurrentVar || varName == AccumulatorVar || isUnsupportedReduceScopeVar(varName) {
			return nil
		}
		if strings.HasPrefix(varName, CurrentVar+".") {
			suffix := strings.TrimPrefix(varName, CurrentVar+".")
			if suffix != "" {
				return a.resolveScopedFieldNames(suffix)
			}
		}
	case arrayLambdaScopeNone:
		scope, fieldName := a.visibleElemScopeAndFieldName(varName)
		if fieldName == "" {
			return nil
		}
		resolved, err := a.resolveFieldInScope(scope, fieldName)
		if err != nil {
			return nil
		}
		return singleSchemaScope(resolved)
	}
	return nil
}

func (a *ArrayOperator) visibleElemScopeAndFieldName(varName string) (string, string) {
	for i := len(a.visibleElems) - 1; i >= 0; i-- {
		alias := a.visibleElems[i]
		prefix := alias + "."
		scope := ""
		if i < len(a.visibleScopes) {
			scope = a.visibleScopes[i]
		}
		if varName == alias {
			return scope, ""
		}
		if strings.HasPrefix(varName, prefix) {
			return scope, strings.TrimPrefix(varName, prefix)
		}
	}
	return "", ""
}

func (a *ArrayOperator) rewriteScopedMissingFields(operator string, opArgs interface{}, allowAccumulator bool) (interface{}, bool, error) {
	switch operator {
	case OpMissing:
		return a.rewriteScopedMissingFieldOperand(opArgs, allowAccumulator)
	case OpMissingSome:
		args, ok := opArgs.([]interface{})
		if !ok || len(args) != 2 {
			return nil, false, nil
		}
		fields, ok := args[1].([]interface{})
		if !ok {
			return nil, false, nil
		}
		rewrittenFields, changed, err := a.rewriteScopedMissingFieldList(fields, allowAccumulator)
		if err != nil || !changed {
			return nil, changed, err
		}
		rewrittenArgs := make([]interface{}, len(args))
		copy(rewrittenArgs, args)
		rewrittenArgs[1] = rewrittenFields
		return rewrittenArgs, true, nil
	default:
		return nil, false, nil
	}
}

func (a *ArrayOperator) rewriteScopedMissingFieldOperand(field interface{}, allowAccumulator bool) (interface{}, bool, error) {
	switch v := field.(type) {
	case string:
		return a.rewriteScopedMissingFieldName(v, allowAccumulator)
	case []interface{}:
		return a.rewriteScopedMissingFieldList(v, allowAccumulator)
	default:
		return nil, false, nil
	}
}

func (a *ArrayOperator) rewriteScopedMissingFieldList(fields []interface{}, allowAccumulator bool) ([]interface{}, bool, error) {
	rewrittenFields := make([]interface{}, len(fields))
	changed := false
	for i, field := range fields {
		fieldName, ok := field.(string)
		if !ok {
			rewrittenFields[i] = field
			continue
		}
		rewritten, fieldChanged, err := a.rewriteScopedMissingFieldName(fieldName, allowAccumulator)
		if err != nil {
			return nil, false, err
		}
		rewrittenFields[i] = rewritten
		changed = changed || fieldChanged
	}
	if !changed {
		return nil, false, nil
	}
	return rewrittenFields, true, nil
}

func (a *ArrayOperator) rewriteScopedMissingFieldName(fieldName string, allowAccumulator bool) (interface{}, bool, error) {
	if allowAccumulator && fieldName == AccumulatorVar {
		return a.accumulatorSQLResult(), true, nil
	}
	mapped, handled, err := a.mapArrayScopeVar(fieldName)
	if err != nil {
		return nil, true, err
	}
	if !handled {
		return fieldName, false, nil
	}
	return a.scopedSQLFieldResult(mapped, a.scopedFieldNamesForVar(fieldName)...), true, nil
}

func (a *ArrayOperator) rewriteAccumulatorVar(varExpr interface{}) (ProcessedValue, bool, error) {
	if varName, ok := varExpr.(string); ok {
		if varName == AccumulatorVar {
			return a.accumulatorSQLResult(), true, nil
		}
		return ProcessedValue{}, false, nil
	}

	arr, ok := varExpr.([]interface{})
	if !ok {
		return ProcessedValue{}, false, nil
	}
	if len(arr) == 0 {
		return ProcessedValue{}, false, nil
	}
	if err := validateVarArrayMaxEntries(arr); err != nil {
		return ProcessedValue{}, true, err
	}
	varName, ok := arr[0].(string)
	if !ok || varName != AccumulatorVar {
		return ProcessedValue{}, false, nil
	}
	if len(arr) == 1 {
		return a.accumulatorSQLResult(), true, nil
	}
	defaultSQL, err := a.dataOp.valueToSQL(arr[1])
	if err != nil {
		return ProcessedValue{}, true, fmt.Errorf("invalid default value: %w", err)
	}
	return a.accumulatorSQLResultWithSQL(fmt.Sprintf("COALESCE(%s, %s)", a.accumulatorSQLResult().Value, defaultSQL)), true, nil
}

func (a *ArrayOperator) rewriteAccumulatorVarParam(varExpr interface{}) (interface{}, bool, error) {
	if varName, ok := varExpr.(string); ok {
		if varName == AccumulatorVar {
			return a.accumulatorSQLResult(), true, nil
		}
		return nil, false, nil
	}

	arr, ok := varExpr.([]interface{})
	if !ok {
		return nil, false, nil
	}
	if len(arr) == 0 {
		return nil, false, nil
	}
	if err := validateVarArrayMaxEntries(arr); err != nil {
		return nil, true, err
	}
	varName, ok := arr[0].(string)
	if !ok || varName != AccumulatorVar {
		return nil, false, nil
	}
	if len(arr) == 1 {
		return a.accumulatorSQLResult(), true, nil
	}
	rewritten := make([]interface{}, len(arr))
	copy(rewritten, arr)
	rewritten[0] = a.accumulatorSQLResult()
	return map[string]interface{}{OpVar: rewritten}, true, nil
}

// arrayScopeVarToSQL resolves lambda-scoped var references before schema
// validation. Element lambdas are relative to the current element; reduce
// lambdas expose only JSONLogic's current/accumulator bindings.
func (a *ArrayOperator) arrayScopeVarToSQL(varExpr interface{}) (string, bool, error) {
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
		defaultSQL, err := a.dataOp.valueToSQL(arr[1])
		if err != nil {
			return "", true, fmt.Errorf("invalid default value: %w", err)
		}
		return fmt.Sprintf("COALESCE(%s, %s)", mapped, defaultSQL), true, nil
	}

	return "", false, nil
}

// arrayInternalVarToSQL resolves vars in array-produced value expressions.
// Active lambdas delegate to arrayScopeVarToSQL; outside lambdas, only already
// generated elem aliases are treated as array-scoped.
func (a *ArrayOperator) arrayInternalVarToSQL(varExpr interface{}) (string, bool, error) {
	if a.lambdaScope != arrayLambdaScopeNone {
		return a.arrayScopeVarToSQL(varExpr)
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
		defaultSQL, err := a.dataOp.valueToSQL(arr[1])
		if err != nil {
			return "", true, fmt.Errorf("invalid default value: %w", err)
		}
		return fmt.Sprintf("COALESCE(%s, %s)", mapped, defaultSQL), true, nil
	}

	return "", false, nil
}

func (a *ArrayOperator) rewriteScopedVarsForOperatorWithContextAndPath(expr interface{}, allowAccumulator bool, path string) (interface{}, error) {
	switch e := expr.(type) {
	case map[string]interface{}:
		if len(e) != 1 {
			return nil, tperrors.NewMultipleKeys(path)
		}
		if varName, hasVar := e[OpVar]; hasVar {
			if allowAccumulator {
				if rewritten, handled, err := a.rewriteAccumulatorVar(varName); handled || err != nil {
					if err != nil {
						return nil, err
					}
					return rewritten, nil
				}
			}
			if rewritten, handled, err := a.rewriteArrayScopeVar(varName); handled || err != nil {
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
					rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(arr[0], allowAccumulator, tperrors.BuildArrayPath(opPath, 0))
					if err != nil {
						return nil, err
					}
					newArgs[0] = rewritten
				}
				if opName == OpReduce && len(newArgs) > 2 {
					rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(arr[2], allowAccumulator, tperrors.BuildArrayPath(opPath, 2))
					if err != nil {
						return nil, err
					}
					newArgs[2] = rewritten
				}
				return map[string]interface{}{opName: newArgs}, nil
			}
			if !a.isBuiltInOperatorName(opName) {
				opPath := tperrors.BuildPath(path, opName, -1)
				rewrittenArgs, err := a.rewriteScopedVarsForOperatorWithContextAndPath(opArgs, allowAccumulator, tperrors.BuildArrayPath(opPath, 0))
				if err != nil {
					return nil, err
				}
				return map[string]interface{}{opName: rewrittenArgs}, nil
			}
		}
		result := make(map[string]interface{}, len(e))
		for k, v := range e {
			rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(v, allowAccumulator, tperrors.BuildPath(path, k, -1))
			if err != nil {
				return nil, err
			}
			result[k] = rewritten
		}
		return result, nil

	case []interface{}:
		result := make([]interface{}, len(e))
		for i, v := range e {
			rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(v, allowAccumulator, tperrors.BuildArrayPath(path, i))
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

// isArrayOperator returns true if the operator is an array operator that
// introduces its own element scope. Used to prevent rewriting into nested scopes.
func (a *ArrayOperator) isArrayOperator(op string) bool {
	switch op {
	case OpMap, OpFilter, OpReduce, OpAll, OpSome, OpNone:
		return true
	}
	return false
}

func (a *ArrayOperator) isBuiltInOperatorName(op string) bool {
	switch op {
	case OpVar, OpMissing, OpMissingSome,
		OpEqual, OpStrictEqual, OpNotEqual, OpStrictNotEqual, OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual, OpIn,
		OpAnd, OpOr, OpNot, OpDoubleBang, OpIf,
		OpAdd, OpSubtract, OpMultiply, OpDivide, OpModulo, OpMax, OpMin,
		OpCat, OpSubstr,
		OpMap, OpFilter, OpReduce, OpAll, OpSome, OpNone, OpMerge:
		return true
	}
	return false
}

func isEmptyArrayLiteral(value interface{}) bool {
	arr, ok := value.([]interface{})
	return ok && len(arr) == 0
}

// isPrimitive checks if a value is a primitive type.
func (a *ArrayOperator) isPrimitive(value interface{}) bool {
	switch value.(type) {
	case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number, bool:
		return true
	case nil:
		return true
	default:
		return false
	}
}
