package parser

import (
	"fmt"
	"strings"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// ParseParameterized converts a JSON Logic expression to SQL using context
// inference with bind parameter placeholders instead of inlined literals.
func (p *Parser) ParseParameterized(logic interface{}) (string, []params.QueryParam, error) {
	if err := p.validator.Validate(logic); err != nil {
		return "", nil, tperrors.NewValidationError(err)
	}
	if p.isPrimitive(logic) {
		return "", nil, tperrors.NewPrimitiveNotAllowed("$")
	}
	if _, ok := logic.([]interface{}); ok {
		return "", nil, tperrors.NewArrayNotAllowed("$")
	}

	style := params.StyleForDialect(p.config.GetDialect())
	pc := params.NewParamCollector(style)

	res, err := p.parseExpressionAnyParam(logic, "$", pc)
	if err != nil {
		return "", nil, err
	}
	if err := p.rejectUnsupportedPostgreSQLEmptyArrayResult(res); err != nil {
		return "", nil, err
	}

	if vErr := params.ValidatePlaceholderRefs(res.SQL, pc.Params(), style); vErr != nil {
		return "", nil, vErr
	}

	return res.SQL, pc.Params(), nil
}

// ParseConditionParameterized converts a JSON Logic expression to a SQL condition
// (without WHERE keyword) with bind parameter placeholders.
func (p *Parser) ParseConditionParameterized(logic interface{}) (string, []params.QueryParam, error) {
	if err := p.validator.Validate(logic); err != nil {
		return "", nil, tperrors.NewValidationError(err)
	}

	style := params.StyleForDialect(p.config.GetDialect())
	pc := params.NewParamCollector(style)

	res, err := p.parseExpressionPredicateParam(logic, "$", pc)
	if err != nil {
		return "", nil, err
	}
	if err := p.rejectUnsupportedPostgreSQLEmptyArrayResult(res); err != nil {
		return "", nil, err
	}

	if vErr := params.ValidatePlaceholderRefs(res.SQL, pc.Params(), style); vErr != nil {
		return "", nil, vErr
	}

	return res.SQL, pc.Params(), nil
}

// ParseValueParameterized converts a JSON Logic expression to a parameterized
// SQL value expression.
func (p *Parser) ParseValueParameterized(logic interface{}) (string, []params.QueryParam, error) {
	style := params.StyleForDialect(p.config.GetDialect())
	pc := params.NewParamCollector(style)

	res, err := p.parseExpressionValueParam(logic, "$", pc)
	if err != nil {
		return "", nil, err
	}
	if err := p.rejectUnsupportedPostgreSQLEmptyArrayResult(res); err != nil {
		return "", nil, err
	}
	if err := p.validateCompatibleObjectArrayScopesForResult(res, "$"); err != nil {
		return "", nil, err
	}

	sql := valueSQL(res)
	if vErr := params.ValidatePlaceholderRefs(sql, pc.Params(), style); vErr != nil {
		return "", nil, vErr
	}

	return sql, pc.Params(), nil
}

func (p *Parser) parseExpressionPredicateParam(expr interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if b, ok := expr.(bool); ok {
		return booleanPredicateResult(b), nil
	}
	if p.isPrimitive(expr) {
		return expressionResult{}, tperrors.NewInvalidExpressionContext("", path, "predicate", "value")
	}
	if _, ok := expr.([]interface{}); ok {
		return expressionResult{}, tperrors.NewInvalidExpressionContext("", path, "predicate", "value")
	}
	if pv, ok := expr.(operators.ProcessedValue); ok {
		if pv.IsSQL {
			return expressionResult{}, tperrors.NewInvalidExpressionContext("", path, "predicate", "value")
		}
		return p.parseExpressionPredicateParam(pv.Value, path, pc)
	}
	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return expressionResult{}, tperrors.NewMultipleKeys(path)
		}
		for operator, args := range obj {
			operatorPath := tperrors.BuildPath(path, operator, -1)
			return p.parseOperatorPredicateParam(operator, args, operatorPath, pc)
		}
	}
	return expressionResult{}, tperrors.New(tperrors.ErrInvalidExpression, "", path,
		fmt.Sprintf("invalid expression type: %T", expr))
}

func (p *Parser) parseExpressionValueParam(expr interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if pv, ok := expr.(operators.ProcessedValue); ok {
		if pv.IsSQL {
			if pv.HasExpressionInfo {
				res := resultFromOperator(operatorResultFromProcessedValue(pv))
				copyProcessedFieldMetadata(&res, pv)
				res.requiresKnownTruthiness = pv.RequiresKnownTruthiness
				res.preserveParamRefs = pv.PreserveParamRefs
				return p.supportedValueResult(res, path)
			}
			res := fieldOrValueResult(pv.Value, operators.ExpressionTypeUnknown, pv.IsField, pv.FieldName)
			copyProcessedFieldMetadata(&res, pv)
			res.requiresKnownTruthiness = pv.RequiresKnownTruthiness
			res.preserveParamRefs = pv.PreserveParamRefs
			return p.supportedValueResult(res, path)
		}
		return p.parseExpressionValueParam(pv.Value, path, pc)
	}
	if p.isPrimitive(expr) {
		return p.parsePrimitiveValueParam(expr, path, pc)
	}
	if arr, ok := expr.([]interface{}); ok {
		sql, elemTypes, schemaScopes, preserveParamRefs, err := p.arrayLiteralToSQLParam(arr, path, pc)
		if err != nil {
			return expressionResult{}, tperrors.Wrap(tperrors.ErrInvalidArgument, "", path, "invalid array literal", err)
		}
		res := withArrayElementTypes(
			literalValueResultWithRaw(sql, operators.ExpressionTypeArray, len(arr) > 0, expr),
			elemTypes...,
		)
		res = withArrayElementSchemaScopes(res, schemaScopes...)
		res.preserveParamRefs = preserveParamRefs
		return res, p.validateCompatibleObjectArrayScopesForResult(res, path)
	}
	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return expressionResult{}, tperrors.NewMultipleKeys(path)
		}
		for operator, args := range obj {
			operatorPath := tperrors.BuildPath(path, operator, -1)
			return p.parseOperatorValueParam(operator, args, operatorPath, pc)
		}
	}
	return expressionResult{}, tperrors.New(tperrors.ErrInvalidExpression, "", path,
		fmt.Sprintf("invalid expression type: %T", expr))
}

func (p *Parser) parseExpressionAnyParam(expr interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if p.isPrimitive(expr) {
		return p.parsePrimitiveValueParam(expr, path, pc)
	}
	if _, ok := expr.([]interface{}); ok {
		return p.parseExpressionValueParam(expr, path, pc)
	}
	if obj, ok := expr.(map[string]interface{}); ok && len(obj) == 1 {
		for operator := range obj {
			switch operator {
			case operators.OpMissing,
				operators.OpMissingSome,
				operators.OpEqual,
				operators.OpStrictEqual,
				operators.OpNotEqual,
				operators.OpStrictNotEqual,
				operators.OpGreaterThan,
				operators.OpGreaterThanOrEqual,
				operators.OpLessThan,
				operators.OpLessThanOrEqual,
				operators.OpIn,
				operators.OpNot,
				operators.OpDoubleBang,
				operators.OpAll,
				operators.OpSome,
				operators.OpNone:
				return p.parseExpressionPredicateParam(expr, path, pc)
			case operators.OpAnd, operators.OpOr, operators.OpIf:
				checkpoint := pc.Checkpoint()
				if res, err := p.parseExpressionPredicateParam(expr, path, pc); err == nil {
					return res, nil
				}
				pc.Restore(checkpoint)
				return p.parseExpressionValueParam(expr, path, pc)
			default:
				return p.parseExpressionValueParam(expr, path, pc)
			}
		}
	}
	return p.parseExpressionValueParam(expr, path, pc)
}

func (p *Parser) parseOperatorPredicateParam(operator string, args interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if p.customOpLookup != nil {
		if handler, ok := p.customOpLookup(operator); ok {
			paramCount := len(pc.Params())
			processedArgs, err := p.processCustomOperatorArgsParam(args, path, pc)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"failed to process custom operator arguments", err)
			}
			res, err := handler.ToSQL(operator, processedArgs)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"custom operator failed", err)
			}
			if ph, bad := params.FindQuotedPlaceholderRefAfter(res.SQL, pc.Params(), pc.Style(), paramCount); bad {
				return expressionResult{}, tperrors.New(tperrors.ErrCustomOperatorFailed, operator, path,
					fmt.Sprintf("custom operator produced invalid parameterized SQL: placeholder %s appears inside a quoted SQL region", ph))
			}
			if res.Kind != operators.ExpressionKindPredicate {
				return expressionResult{}, tperrors.NewInvalidExpressionContext(operator, path, "predicate", kindName(res.Kind))
			}
			return customOperatorResult(res, customResultPreservesDroppedParamRefs(res.SQL, pc, paramCount)), nil
		}
	}

	switch operator {
	case operators.OpMissing:
		sql, err := p.dataOp.ToSQLParam(operator, []interface{}{args}, pc)
		return predicateResult(sql), p.wrapOperatorError(operator, path, err)
	case operators.OpMissingSome:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		sql, err := p.dataOp.ToSQLParam(operator, arr, pc)
		return predicateResult(sql), p.wrapOperatorError(operator, path, err)
	case operators.OpEqual,
		operators.OpStrictEqual,
		operators.OpNotEqual,
		operators.OpStrictNotEqual,
		operators.OpGreaterThan,
		operators.OpGreaterThanOrEqual,
		operators.OpLessThan,
		operators.OpLessThanOrEqual,
		operators.OpIn:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		checkpoint := pc.Checkpoint()
		processedArgs, err := p.processValueArgsParam(arr, path, pc)
		if err != nil {
			return expressionResult{}, err
		}
		sql, err := p.comparisonOp.ToSQLParam(operator, processedArgs, pc)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		res, err := literalComparisonPredicateResult(operator, processedArgs, sql)
		if processedArgsPreserveParamRefs(processedArgs) {
			res.preserveParamRefs = true
		} else if res.truthKnown {
			pc.Restore(checkpoint)
		}
		return res, p.wrapOperatorError(operator, path, err)
	case operators.OpAnd, operators.OpOr:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parsePredicateLogicalParam(operator, arr, path, pc)
	case operators.OpNot:
		return p.parseNotPredicateParam(operator, args, path, false, pc)
	case operators.OpDoubleBang:
		return p.parseNotPredicateParam(operator, args, path, true, pc)
	case operators.OpIf:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parsePredicateIfParam(arr, path, pc)
	case operators.OpAll, operators.OpSome, operators.OpNone:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		sql, err := p.arrayOp.ToSQLParamAtPath(operator, arr, pc, path)
		return predicateResult(sql), p.wrapOperatorError(operator, path, err)
	case operators.OpVar,
		operators.OpMap,
		operators.OpFilter,
		operators.OpReduce,
		operators.OpMerge,
		operators.OpAdd,
		operators.OpSubtract,
		operators.OpMultiply,
		operators.OpDivide,
		operators.OpModulo,
		operators.OpMax,
		operators.OpMin,
		operators.OpCat,
		operators.OpSubstr:
		return expressionResult{}, tperrors.NewInvalidExpressionContext(operator, path, "predicate", "value")
	default:
		return expressionResult{}, tperrors.NewUnsupportedOperator(operator, path)
	}
}

func (p *Parser) parseOperatorValueParam(operator string, args interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if p.customOpLookup != nil {
		if handler, ok := p.customOpLookup(operator); ok {
			paramCount := len(pc.Params())
			processedArgs, err := p.processCustomOperatorArgsParam(args, path, pc)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"failed to process custom operator arguments", err)
			}
			res, err := handler.ToSQL(operator, processedArgs)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"custom operator failed", err)
			}
			if ph, bad := params.FindQuotedPlaceholderRefAfter(res.SQL, pc.Params(), pc.Style(), paramCount); bad {
				return expressionResult{}, tperrors.New(tperrors.ErrCustomOperatorFailed, operator, path,
					fmt.Sprintf("custom operator produced invalid parameterized SQL: placeholder %s appears inside a quoted SQL region", ph))
			}
			return customOperatorResult(res, customResultPreservesDroppedParamRefs(res.SQL, pc, paramCount)), nil
		}
	}

	switch operator {
	case operators.OpVar:
		sql, err := p.dataOp.ToSQLParam(operator, []interface{}{args}, pc)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		if pv, ok := varProcessedExpression(args); ok {
			pv.Value = sql
			res := resultFromOperator(operatorResultFromProcessedValue(pv))
			copyProcessedFieldMetadata(&res, pv)
			res.requiresKnownTruthiness = pv.RequiresKnownTruthiness
			return p.supportedValueResult(withVarDefaultMetadata(res, args), path)
		}
		fieldName := varFieldName(args)
		return p.supportedValueResult(
			withVarDefaultMetadata(p.fieldValueExpressionResult(sql, fieldName), args),
			path,
		)
	case operators.OpMissing,
		operators.OpMissingSome,
		operators.OpEqual,
		operators.OpStrictEqual,
		operators.OpNotEqual,
		operators.OpStrictNotEqual,
		operators.OpGreaterThan,
		operators.OpGreaterThanOrEqual,
		operators.OpLessThan,
		operators.OpLessThanOrEqual,
		operators.OpIn,
		operators.OpAll,
		operators.OpSome,
		operators.OpNone:
		return p.parseOperatorPredicateParam(operator, args, path, pc)
	case operators.OpNot:
		return p.parseNotValueParam(operator, args, path, false, pc)
	case operators.OpDoubleBang:
		return p.parseNotValueParam(operator, args, path, true, pc)
	case operators.OpAnd, operators.OpOr:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parseValueLogicalParam(operator, arr, path, pc)
	case operators.OpIf:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parseValueIfParam(arr, path, pc)
	case operators.OpAdd,
		operators.OpSubtract,
		operators.OpMultiply,
		operators.OpDivide,
		operators.OpModulo,
		operators.OpMax,
		operators.OpMin:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		processedArgs, err := p.processValueArgsParam(arr, path, pc)
		if err != nil {
			return expressionResult{}, err
		}
		sql, err := p.numericOp.ToSQLParam(operator, processedArgs, pc)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		res := valueResult(sql, operators.ExpressionTypeNumber)
		res.preserveParamRefs = processedArgsPreserveParamRefs(processedArgs)
		return res, nil
	case operators.OpCat:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parseCatValueParam(arr, path, pc)
	case operators.OpSubstr:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		processedArgs, err := p.processValueArgsParam(arr, path, pc)
		if err != nil {
			return expressionResult{}, err
		}
		sql, err := p.stringOp.ToSQLParam(operator, processedArgs, pc)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		res := valueResult(sql, operators.ExpressionTypeString)
		res.preserveParamRefs = processedArgsPreserveParamRefs(processedArgs)
		return res, nil
	case operators.OpMap, operators.OpFilter, operators.OpMerge:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		res, err := p.arrayOp.ToValueResultParamAtPath(operator, arr, pc, path)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		sql := res.SQL
		if arrayValueOperatorReturnsEmptyLiteral(operator, arr) || p.sqlIsEmptyArrayLiteral(sql) {
			result := literalValueResultWithRaw(sql, operators.ExpressionTypeArray, false, []interface{}{})
			result.preserveParamRefs = res.PreserveParamRefs
			return result, nil
		}
		return resultFromOperator(res), nil
	case operators.OpReduce:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		if len(arr) == 3 && isEmptyArrayLiteralValue(arr[0]) {
			return p.parseExpressionValueParam(arr[2], tperrors.BuildArrayPath(path, 2), pc)
		}
		res, err := p.arrayOp.ToValueResultParamAtPath(operator, arr, pc, path)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		return resultFromOperator(res), nil
	default:
		return expressionResult{}, tperrors.NewUnsupportedOperator(operator, path)
	}
}

func (p *Parser) parsePredicateLogicalParam(operator string, args []interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if len(args) == 0 {
		return expressionResult{}, tperrors.NewInsufficientArgs(operator, path, 1, 0)
	}
	checkpoint := pc.Checkpoint()
	parts := make([]string, 0, len(args))
	partResults := make([]expressionResult, 0, len(args))
	for i, arg := range args {
		operandCheckpoint := pc.Checkpoint()
		res, err := p.parseExpressionPredicateParam(arg, tperrors.BuildArrayPath(path, i), pc)
		if err != nil {
			return expressionResult{}, err
		}
		if res.truthKnown && canRollbackParamRefs(res) {
			if operator == logicalOpAnd && res.truthy {
				pc.Restore(operandCheckpoint)
				continue
			}
			if operator == logicalOpOr && !res.truthy {
				pc.Restore(operandCheckpoint)
				continue
			}
			if operator == logicalOpAnd && !res.truthy {
				pc.Restore(checkpoint)
				return booleanPredicateResult(false), nil
			}
			if operator == logicalOpOr && res.truthy {
				pc.Restore(checkpoint)
				return booleanPredicateResult(true), nil
			}
		}
		parts = append(parts, res.SQL)
		partResults = append(partResults, res)
	}
	if len(parts) == 0 {
		return booleanPredicateResult(operator == logicalOpAnd), nil
	}
	if len(parts) == 1 {
		return preserveParamRefsIfNeeded(predicateResult(parts[0]), partResults[0]), nil
	}
	joiner := sqlAndJoiner
	if operator == logicalOpOr {
		joiner = sqlOrJoiner
	}
	return preserveParamRefsIfNeeded(predicateResult(fmt.Sprintf("(%s)", strings.Join(parts, joiner))), partResults...), nil
}

func (p *Parser) parseNotPredicateParam(operator string, args interface{}, path string, double bool, pc *params.ParamCollector) (expressionResult, error) {
	arg, ok := unaryArg(args)
	if !ok {
		return expressionResult{}, tperrors.NewTypeMismatch(operator, path, "exactly 1 argument", "multiple arguments")
	}
	res, condition, err := p.parseTruthinessResultParam(arg, tperrors.BuildArrayPath(path, 0), pc)
	if err != nil {
		return expressionResult{}, err
	}
	if double {
		if res.truthKnown {
			return preserveParamRefsIfNeeded(booleanPredicateResult(res.truthy), res), nil
		}
		return preserveParamRefsIfNeeded(predicateResult(condition), res), nil
	}
	if res.truthKnown {
		return preserveParamRefsIfNeeded(booleanPredicateResult(!res.truthy), res), nil
	}
	condition = operators.StripRedundantOuterParens(condition)
	return preserveParamRefsIfNeeded(predicateResult(fmt.Sprintf("NOT (%s)", condition)), res), nil
}

func (p *Parser) parseNotValueParam(
	operator string,
	args interface{},
	path string,
	double bool,
	pc *params.ParamCollector,
) (expressionResult, error) {
	arg, ok := unaryArg(args)
	if !ok {
		return expressionResult{}, tperrors.NewTypeMismatch(operator, path, "exactly 1 argument", "multiple arguments")
	}
	res, condition, err := p.parseTruthinessResultParam(arg, tperrors.BuildArrayPath(path, 0), pc)
	if err != nil {
		return expressionResult{}, err
	}
	if res.truthKnown {
		return preserveParamRefsIfNeeded(booleanValueResult(res.truthy == double), res), nil
	}
	condition = operators.PredicateValueSQL(condition)
	if double {
		return preserveParamRefsIfNeeded(valueResult(condition, operators.ExpressionTypeBoolean), res), nil
	}
	return preserveParamRefsIfNeeded(
		valueResult(operators.PredicateValueSQL(fmt.Sprintf("NOT (%s)", condition)), operators.ExpressionTypeBoolean),
		res,
	), nil
}

func (p *Parser) parsePredicateIfParam(args []interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if len(args) < 2 {
		return expressionResult{}, tperrors.NewInsufficientArgs("if", path, 2, len(args))
	}
	var parts []string
	var paramRefs paramRefPreserver
	pairLimit := len(args)
	hasElse := len(args)%2 == 1
	if hasElse {
		pairLimit = len(args) - 1
	}
	for i := 0; i < pairLimit; i += 2 {
		conditionCheckpoint := pc.Checkpoint()
		cond, condition, err := p.parsePredicateIfConditionParam(args[i], tperrors.BuildArrayPath(path, i), pc)
		if err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && !cond.truthy {
			if !canRollbackParamRefs(cond) {
				paramRefs.mark(cond)
				continue
			}
			pc.Restore(conditionCheckpoint)
			continue
		}
		if cond.truthKnown && cond.truthy && canRollbackParamRefs(cond) {
			pc.Restore(conditionCheckpoint)
		}
		paramRefs.mark(cond)
		thenRes, err := p.parsePredicateIfOperandParam(args[i+1], tperrors.BuildArrayPath(path, i+1), pc)
		if err != nil {
			return expressionResult{}, err
		}
		paramRefs.mark(thenRes)
		if cond.truthKnown && cond.truthy {
			if len(parts) == 0 {
				return paramRefs.apply(thenRes), nil
			}
			return paramRefs.apply(predicateResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), thenRes.SQL))), nil
		}
		parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", condition, thenRes.SQL))
	}
	elseSQL := "FALSE"
	if hasElse {
		elseRes, err := p.parsePredicateIfOperandParam(args[len(args)-1], tperrors.BuildArrayPath(path, len(args)-1), pc)
		if err != nil {
			return expressionResult{}, err
		}
		paramRefs.mark(elseRes)
		if len(parts) == 0 {
			return paramRefs.apply(elseRes), nil
		}
		elseSQL = elseRes.SQL
	}
	if len(parts) == 0 {
		return paramRefs.apply(booleanPredicateResult(false)), nil
	}
	return paramRefs.apply(predicateResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), elseSQL))), nil
}

func (p *Parser) parsePredicateIfConditionParam(
	expr interface{},
	path string,
	pc *params.ParamCollector,
) (expressionResult, string, error) {
	return p.parseTruthinessResultParam(expr, path, pc)
}

func (p *Parser) parsePredicateIfOperandParam(expr interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if b, ok := expr.(bool); ok {
		return booleanPredicateResult(b), nil
	}
	return p.parseExpressionPredicateParam(expr, path, pc)
}

func (p *Parser) parseValueIfParam(args []interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if len(args) < 2 {
		return expressionResult{}, tperrors.NewInsufficientArgs("if", path, 2, len(args))
	}
	var parts []string
	var paramRefs paramRefPreserver
	resultRes := literalValueResult("NULL", operators.ExpressionTypeNull, false)
	typeSet := false
	mergeResultType := func(res expressionResult) error {
		if !typeSet {
			resultRes = res
			typeSet = true
			return nil
		}
		merged, err := p.compatibleValueResult(resultRes, res, path)
		if err != nil {
			return err
		}
		resultRes = merged
		return nil
	}
	pairLimit := len(args)
	hasElse := len(args)%2 == 1
	if hasElse {
		pairLimit = len(args) - 1
	}
	for i := 0; i < pairLimit; i += 2 {
		cond, condition, err := p.parseTruthinessResultParam(args[i], tperrors.BuildArrayPath(path, i), pc)
		if err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && !cond.truthy {
			paramRefs.mark(cond)
			continue
		}
		paramRefs.mark(cond)
		thenRes, err := p.parseExpressionValueParam(args[i+1], tperrors.BuildArrayPath(path, i+1), pc)
		if err != nil {
			return expressionResult{}, err
		}
		paramRefs.mark(thenRes)
		if err := mergeResultType(thenRes); err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && cond.truthy {
			if len(parts) == 0 {
				return paramRefs.apply(thenRes), nil
			}
			result := valueResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), valueSQL(thenRes)), valueTypeOf(resultRes))
			if elemTypes, ok := arrayElementTypesOf(resultRes); ok {
				result = withArrayElementTypes(result, elemTypes...)
			}
			result = withArrayElementSchemaScopes(result, resultRes.arrayElementSchemaScopes...)
			return paramRefs.apply(result), nil
		}
		parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", condition, valueSQL(thenRes)))
	}
	elseSQL := "NULL"
	if hasElse {
		elseRes, err := p.parseExpressionValueParam(args[len(args)-1], tperrors.BuildArrayPath(path, len(args)-1), pc)
		if err != nil {
			return expressionResult{}, err
		}
		paramRefs.mark(elseRes)
		if len(parts) == 0 {
			return paramRefs.apply(elseRes), nil
		}
		if err := mergeResultType(elseRes); err != nil {
			return expressionResult{}, err
		}
		elseSQL = valueSQL(elseRes)
	}
	if len(parts) == 0 {
		return paramRefs.apply(literalValueResult("NULL", operators.ExpressionTypeNull, false)), nil
	}
	result := valueResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), elseSQL), valueTypeOf(resultRes))
	if elemTypes, ok := arrayElementTypesOf(resultRes); ok {
		result = withArrayElementTypes(result, elemTypes...)
	}
	result = withArrayElementSchemaScopes(result, resultRes.arrayElementSchemaScopes...)
	return paramRefs.apply(result), nil
}

func (p *Parser) parseValueLogicalParam(operator string, args []interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if len(args) == 0 {
		return expressionResult{}, tperrors.NewInsufficientArgs(operator, path, 1, 0)
	}
	return p.parseValueLogicalFromParam(operator, args, 0, path, pc)
}

func (p *Parser) parseValueLogicalFromParam(operator string, args []interface{}, index int, path string, pc *params.ParamCollector) (expressionResult, error) {
	argPath := tperrors.BuildArrayPath(path, index)
	if current, ok := nonFiniteNativeFloatTruthResult(args[index]); ok {
		if index == len(args)-1 ||
			(operator == logicalOpOr && current.truthy) ||
			(operator == logicalOpAnd && !current.truthy) {
			return expressionResult{}, nonFiniteNativeFloatValueError(args[index], argPath)
		}
		return p.parseValueLogicalFromParam(operator, args, index+1, path, pc)
	}

	checkpoint := pc.Checkpoint()
	current, err := p.parseExpressionValueParam(args[index], argPath, pc)
	if err != nil {
		return expressionResult{}, err
	}
	if index == len(args)-1 {
		return current, nil
	}
	if current.truthKnown {
		if operator == logicalOpOr && current.truthy {
			return current, nil
		}
		if operator == logicalOpAnd && !current.truthy {
			return current, nil
		}
		if canRollbackParamRefs(current) {
			pc.Restore(checkpoint)
			return p.parseValueLogicalFromParam(operator, args, index+1, path, pc)
		}
		var rest expressionResult
		rest, err = p.parseValueLogicalFromParam(operator, args, index+1, path, pc)
		if err != nil {
			return expressionResult{}, err
		}
		return preserveParamRefsIfNeeded(rest, current), nil
	}
	condition, err := p.truthinessSQL(current, argPath)
	if err != nil {
		return expressionResult{}, err
	}
	rest, err := p.parseValueLogicalFromParam(operator, args, index+1, path, pc)
	if err != nil {
		return expressionResult{}, err
	}
	resultRes, err := p.compatibleValueResult(current, rest, path)
	if err != nil {
		return expressionResult{}, err
	}
	if operator == logicalOpOr {
		result := valueResult(fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, valueSQL(current), valueSQL(rest)), valueTypeOf(resultRes))
		if elemTypes, ok := arrayElementTypesOf(resultRes); ok {
			result = withArrayElementTypes(result, elemTypes...)
		}
		result = withArrayElementSchemaScopes(result, resultRes.arrayElementSchemaScopes...)
		return preserveParamRefsIfNeeded(result, current, rest), nil
	}
	result := valueResult(fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, valueSQL(rest), valueSQL(current)), valueTypeOf(resultRes))
	if elemTypes, ok := arrayElementTypesOf(resultRes); ok {
		result = withArrayElementTypes(result, elemTypes...)
	}
	result = withArrayElementSchemaScopes(result, resultRes.arrayElementSchemaScopes...)
	return preserveParamRefsIfNeeded(result, current, rest), nil
}

func (p *Parser) parseCatValueParam(args []interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if len(args) == 0 {
		return valueResult("''", operators.ExpressionTypeString), nil
	}
	operands := make([]string, len(args))
	var paramRefs paramRefPreserver
	for i, arg := range args {
		res, err := p.parseCatStringExpressionParam(arg, tperrors.BuildArrayPath(path, i), pc)
		if err != nil {
			return expressionResult{}, err
		}
		paramRefs.mark(res)
		operands[i] = res.SQL
	}
	return paramRefs.apply(valueResult(p.config.ConcatSQL(operands), operators.ExpressionTypeString)), nil
}

func (p *Parser) parseCatStringExpressionParam(
	expr interface{},
	path string,
	pc *params.ParamCollector,
) (expressionResult, error) {
	checkpoint := pc.Checkpoint()
	res, err := p.parseExpressionValueParam(expr, path, pc)
	if err == nil {
		return p.stringifiedCatResult(res, path)
	}
	if !isTranspileErrorCode(err, tperrors.ErrTypeMismatch) {
		return expressionResult{}, err
	}
	operator, args, ok := singleOperatorExpression(expr)
	if !ok {
		return expressionResult{}, err
	}
	arr, ok := args.([]interface{})
	if !ok {
		return expressionResult{}, err
	}
	pc.Restore(checkpoint)
	switch operator {
	case operators.OpIf:
		return p.parseStringifiedIfParam(arr, path, pc)
	case operators.OpAnd, operators.OpOr:
		return p.parseStringifiedLogicalParam(operator, arr, path, pc)
	default:
		return expressionResult{}, err
	}
}

func (p *Parser) parseStringifiedIfParam(
	args []interface{},
	path string,
	pc *params.ParamCollector,
) (expressionResult, error) {
	if len(args) < 2 {
		return expressionResult{}, tperrors.NewInsufficientArgs(operators.OpIf, path, 2, len(args))
	}
	var parts []string
	var paramRefs paramRefPreserver
	pairLimit := len(args)
	hasElse := len(args)%2 == 1
	if hasElse {
		pairLimit = len(args) - 1
	}
	for i := 0; i < pairLimit; i += 2 {
		cond, condition, err := p.parseTruthinessResultParam(args[i], tperrors.BuildArrayPath(path, i), pc)
		if err != nil {
			return expressionResult{}, err
		}
		paramRefs.mark(cond)
		if cond.truthKnown && !cond.truthy {
			continue
		}
		thenRes, err := p.parseCatStringExpressionParam(args[i+1], tperrors.BuildArrayPath(path, i+1), pc)
		if err != nil {
			return expressionResult{}, err
		}
		paramRefs.mark(thenRes)
		if cond.truthKnown && cond.truthy {
			if len(parts) == 0 {
				return paramRefs.apply(thenRes), nil
			}
			return paramRefs.apply(valueResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), thenRes.SQL),
				operators.ExpressionTypeString)), nil
		}
		parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", condition, thenRes.SQL))
	}
	elseSQL := "''"
	if hasElse {
		elseRes, err := p.parseCatStringExpressionParam(args[len(args)-1], tperrors.BuildArrayPath(path, len(args)-1), pc)
		if err != nil {
			return expressionResult{}, err
		}
		paramRefs.mark(elseRes)
		if len(parts) == 0 {
			return paramRefs.apply(elseRes), nil
		}
		elseSQL = elseRes.SQL
	}
	if len(parts) == 0 {
		return paramRefs.apply(literalValueResult("''", operators.ExpressionTypeString, false)), nil
	}
	return paramRefs.apply(valueResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), elseSQL),
		operators.ExpressionTypeString)), nil
}

func (p *Parser) parseStringifiedLogicalParam(
	operator string,
	args []interface{},
	path string,
	pc *params.ParamCollector,
) (expressionResult, error) {
	if len(args) == 0 {
		return expressionResult{}, tperrors.NewInsufficientArgs(operator, path, 1, 0)
	}
	return p.parseStringifiedLogicalFromParam(operator, args, 0, path, pc)
}

func (p *Parser) parseStringifiedLogicalFromParam(
	operator string,
	args []interface{},
	index int,
	path string,
	pc *params.ParamCollector,
) (expressionResult, error) {
	argPath := tperrors.BuildArrayPath(path, index)
	if current, ok := nonFiniteNativeFloatTruthResult(args[index]); ok {
		if index == len(args)-1 ||
			(operator == operators.OpOr && current.truthy) ||
			(operator == operators.OpAnd && !current.truthy) {
			return expressionResult{}, nonFiniteNativeFloatValueError(args[index], argPath)
		}
		return p.parseStringifiedLogicalFromParam(operator, args, index+1, path, pc)
	}

	truthCheckpoint := pc.Checkpoint()
	current, condition, err := p.parseTruthinessResultParam(args[index], argPath, pc)
	if err != nil {
		return expressionResult{}, err
	}
	if index == len(args)-1 {
		if !canRollbackParamRefs(current) {
			return p.stringifiedCatResult(current, argPath)
		}
		pc.Restore(truthCheckpoint)
		return p.parseCatStringExpressionParam(args[index], argPath, pc)
	}
	if current.truthKnown {
		if operator == operators.OpOr && current.truthy {
			if !canRollbackParamRefs(current) {
				return p.stringifiedCatResult(current, argPath)
			}
			pc.Restore(truthCheckpoint)
			return p.parseCatStringExpressionParam(args[index], argPath, pc)
		}
		if operator == operators.OpAnd && !current.truthy {
			if !canRollbackParamRefs(current) {
				return p.stringifiedCatResult(current, argPath)
			}
			pc.Restore(truthCheckpoint)
			return p.parseCatStringExpressionParam(args[index], argPath, pc)
		}
		if canRollbackParamRefs(current) {
			pc.Restore(truthCheckpoint)
			return p.parseStringifiedLogicalFromParam(operator, args, index+1, path, pc)
		}
		var rest expressionResult
		rest, err = p.parseStringifiedLogicalFromParam(operator, args, index+1, path, pc)
		if err != nil {
			return expressionResult{}, err
		}
		return preserveParamRefsIfNeeded(rest, current), nil
	}
	currentString, err := p.parseCatStringExpressionParam(args[index], argPath, pc)
	if err != nil {
		return expressionResult{}, err
	}
	rest, err := p.parseStringifiedLogicalFromParam(operator, args, index+1, path, pc)
	if err != nil {
		return expressionResult{}, err
	}
	if operator == operators.OpOr {
		return preserveParamRefsIfNeeded(
			valueResult(fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, currentString.SQL, rest.SQL),
				operators.ExpressionTypeString),
			current, currentString, rest,
		), nil
	}
	return preserveParamRefsIfNeeded(
		valueResult(fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, rest.SQL, currentString.SQL),
			operators.ExpressionTypeString),
		current, currentString, rest,
	), nil
}

// parseOperatorParam is the parameterized variant of parseOperator. Keep in sync.
func (p *Parser) parseOperatorParam(operator string, args interface{}, path string, pc *params.ParamCollector) (string, error) {
	if p.customOpLookup != nil {
		if handler, ok := p.customOpLookup(operator); ok {
			paramCount := len(pc.Params())
			processedArgs, err := p.processCustomOperatorArgsParam(args, path, pc)
			if err != nil {
				return "", tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"failed to process custom operator arguments", err)
			}
			res, err := handler.ToSQL(operator, processedArgs)
			if err != nil {
				return "", tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"custom operator failed", err)
			}
			if ph, bad := params.FindQuotedPlaceholderRefAfter(res.SQL, pc.Params(), pc.Style(), paramCount); bad {
				return "", tperrors.New(tperrors.ErrCustomOperatorFailed, operator, path,
					fmt.Sprintf("custom operator produced invalid parameterized SQL: placeholder %s appears inside a quoted SQL region", ph))
			}
			return res.SQL, nil
		}
	}

	switch operator {
	case operators.OpVar:
		sql, err := p.dataOp.ToSQLParam(operator, []interface{}{args}, pc)
		return sql, p.wrapOperatorError(operator, path, err)
	case operators.OpMissing:
		sql, err := p.dataOp.ToSQLParam(operator, []interface{}{args}, pc)
		return sql, p.wrapOperatorError(operator, path, err)
	case operators.OpMissingSome:
		if arr, ok := args.([]interface{}); ok {
			sql, err := p.dataOp.ToSQLParam(operator, arr, pc)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	case operators.OpEqual,
		operators.OpStrictEqual,
		operators.OpNotEqual,
		operators.OpStrictNotEqual,
		operators.OpGreaterThan,
		operators.OpGreaterThanOrEqual,
		operators.OpLessThan,
		operators.OpLessThanOrEqual,
		operators.OpIn:
		if arr, ok := args.([]interface{}); ok {
			processedArgs, err := p.processValueArgsParam(arr, path, pc)
			if err != nil {
				return "", err
			}
			sql, err := p.comparisonOp.ToSQLParam(operator, processedArgs, pc)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	case operators.OpAnd, operators.OpOr, operators.OpIf:
		if arr, ok := args.([]interface{}); ok {
			processedArgs, err := p.processArgsParam(arr, path, pc)
			if err != nil {
				return "", err
			}
			sql, err := p.logicalOp.ToSQLParam(operator, processedArgs, pc)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)
	case operators.OpNot, operators.OpDoubleBang:
		if arr, ok := args.([]interface{}); ok {
			processedArgs, err := p.processArgsParam(arr, path, pc)
			if err != nil {
				return "", err
			}
			sql, err := p.logicalOp.ToSQLParam(operator, processedArgs, pc)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		processedArg, err := p.processArgParam(args, path, 0, pc)
		if err != nil {
			return "", err
		}
		sql, err := p.logicalOp.ToSQLParam(operator, []interface{}{processedArg}, pc)
		return sql, p.wrapOperatorError(operator, path, err)

	case operators.OpAdd,
		operators.OpSubtract,
		operators.OpMultiply,
		operators.OpDivide,
		operators.OpModulo,
		operators.OpMax,
		operators.OpMin:
		if arr, ok := args.([]interface{}); ok {
			processedArgs, err := p.processArgsParam(arr, path, pc)
			if err != nil {
				return "", err
			}
			sql, err := p.numericOp.ToSQLParam(operator, processedArgs, pc)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	case operators.OpMap, operators.OpFilter, operators.OpReduce, operators.OpAll, operators.OpSome, operators.OpNone, operators.OpMerge:
		if arr, ok := args.([]interface{}); ok {
			sql, err := p.arrayOp.ToSQLParamAtPath(operator, arr, pc, path)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	case operators.OpCat, operators.OpSubstr:
		if arr, ok := args.([]interface{}); ok {
			sql, err := p.stringOp.ToSQLParam(operator, arr, pc)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	default:
		return "", tperrors.NewUnsupportedOperator(operator, path)
	}
}

// processArgsParam is the parameterized variant of processArgs. Keep in sync.
func (p *Parser) processArgsParam(args []interface{}, path string, pc *params.ParamCollector) ([]interface{}, error) {
	processed := make([]interface{}, len(args))
	for i, arg := range args {
		processedArg, err := p.processArgParam(arg, path, i, pc)
		if err != nil {
			return nil, err
		}
		processed[i] = processedArg
	}
	return processed, nil
}

func (p *Parser) processValueArgsParam(args []interface{}, path string, pc *params.ParamCollector) ([]interface{}, error) {
	processed := make([]interface{}, len(args))
	for i, arg := range args {
		processedArg, err := p.processValueArgParam(arg, path, i, pc)
		if err != nil {
			return nil, err
		}
		processed[i] = processedArg
	}
	return processed, nil
}

func (p *Parser) processValueArgParam(arg interface{}, path string, index int, pc *params.ParamCollector) (interface{}, error) {
	if p.isPrimitive(arg) {
		return arg, nil
	}
	argPath := tperrors.BuildArrayPath(path, index)
	if arr, ok := arg.([]interface{}); ok {
		return p.processValueArrayLiteralParam(arr, argPath, pc)
	}
	if exprMap, ok := arg.(map[string]interface{}); ok {
		if len(exprMap) != 1 {
			return nil, tperrors.NewMultipleKeys(argPath)
		}
		for operator := range exprMap {
			if operator != operators.OpVar {
				checkpoint := pc.Checkpoint()
				res, err := p.parseExpressionValueParam(arg, argPath, pc)
				if err != nil {
					return nil, err
				}
				if res.rawLiteralKnown {
					if canRollbackParamRefs(res) {
						pc.Restore(checkpoint)
						return res.rawLiteral, nil
					}
					return typedValueOperand(res), nil
				}
				if res.Kind == operators.ExpressionKindValue && valueTypeOf(res) == operators.ExpressionTypeNull {
					if canRollbackParamRefs(res) {
						pc.Restore(checkpoint)
						var nullLiteral interface{}
						return nullLiteral, nil
					}
					return typedValueOperand(res), nil
				}
				return typedValueOperand(res), nil
			}
		}
	}
	return p.processArgParam(arg, path, index, pc)
}

func (p *Parser) processValueArrayLiteralParam(arr []interface{}, path string, pc *params.ParamCollector) ([]interface{}, error) {
	processed := make([]interface{}, len(arr))
	for i, item := range arr {
		itemPath := tperrors.BuildArrayPath(path, i)
		processedItem, err := p.processValueArrayLiteralItemParam(item, itemPath, pc)
		if err != nil {
			return nil, err
		}
		processed[i] = processedItem
	}
	return processed, nil
}

func (p *Parser) processValueArrayLiteralItemParam(item interface{}, path string, pc *params.ParamCollector) (interface{}, error) {
	if p.isPrimitive(item) {
		return item, nil
	}
	if arr, ok := item.([]interface{}); ok {
		return p.processValueArrayLiteralParam(arr, path, pc)
	}
	if exprMap, ok := item.(map[string]interface{}); ok {
		if len(exprMap) != 1 {
			return nil, tperrors.NewMultipleKeys(path)
		}
		checkpoint := pc.Checkpoint()
		res, err := p.parseExpressionValueParam(item, path, pc)
		if err != nil {
			return nil, err
		}
		if res.rawLiteralKnown {
			if canRollbackParamRefs(res) {
				pc.Restore(checkpoint)
				return res.rawLiteral, nil
			}
			return typedValueOperand(res), nil
		}
		if res.Kind == operators.ExpressionKindValue && valueTypeOf(res) == operators.ExpressionTypeNull {
			if canRollbackParamRefs(res) {
				pc.Restore(checkpoint)
				var nullLiteral interface{}
				return nullLiteral, nil
			}
			return typedValueOperand(res), nil
		}
		return typedValueOperand(res), nil
	}
	return item, nil
}

// processArgParam is the parameterized variant of processArg. Keep in sync.
func (p *Parser) processArgParam(arg interface{}, path string, index int, pc *params.ParamCollector) (interface{}, error) {
	argPath := tperrors.BuildArrayPath(path, index)
	if exprMap, ok := arg.(map[string]interface{}); ok {
		if len(exprMap) != 1 {
			return nil, tperrors.NewMultipleKeys(argPath)
		}
		for operator, opArgs := range exprMap {
			operatorPath := tperrors.BuildPath(path, operator, index)

			if !p.isBuiltInOperator(operator) {
				sql, err := p.parseOperatorParam(operator, opArgs, operatorPath, pc)
				if err != nil {
					return nil, err
				}
				return operators.SQLResult(sql), nil
			}

			if p.isArrayOperator(operator) {
				sql, err := p.parseOperatorParam(operator, opArgs, operatorPath, pc)
				if err != nil {
					return nil, err
				}
				return operators.SQLResult(sql), nil
			}
			processedOpArgs, err := p.processOpArgsParam(opArgs, operatorPath, pc)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{operator: processedOpArgs}, nil
		}
	}

	if arr, ok := arg.([]interface{}); ok {
		return p.processArgsParam(arr, argPath, pc)
	}

	return arg, nil
}

// processOpArgsParam is the parameterized variant of processOpArgs. Keep in sync.
func (p *Parser) processOpArgsParam(opArgs interface{}, path string, pc *params.ParamCollector) (interface{}, error) {
	if arr, ok := opArgs.([]interface{}); ok {
		return p.processArgsParam(arr, path, pc)
	}
	return p.processArgParam(opArgs, path, 0, pc)
}

// processCustomOperatorArgsParam is the parameterized variant of processCustomOperatorArgs. Keep in sync.
func (p *Parser) processCustomOperatorArgsParam(args interface{}, path string, pc *params.ParamCollector) ([]operators.OperatorArg, error) {
	if arr, ok := args.([]interface{}); ok {
		processed := make([]operators.OperatorArg, len(arr))
		for i, arg := range arr {
			argPath := tperrors.BuildArrayPath(path, i)
			argResult, err := p.processArgToOperatorArgParam(arg, argPath, pc)
			if err != nil {
				return nil, err
			}
			processed[i] = argResult
		}
		return processed, nil
	}

	argResult, err := p.processArgToOperatorArgParam(args, path, pc)
	if err != nil {
		return nil, err
	}
	return []operators.OperatorArg{argResult}, nil
}

func (p *Parser) processArgToOperatorArgParam(arg interface{}, path string, pc *params.ParamCollector) (operators.OperatorArg, error) {
	res, err := p.parseExpressionAnyParam(arg, path, pc)
	if err != nil {
		return operators.OperatorArg{}, err
	}
	return operatorArgFromExpressionResult(res), nil
}
