//nolint:goconst // JSONLogic operator and SQL token strings stay inline in parser switches for readability.
package parser

import (
	"fmt"
	"strings"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func (p *Parser) parsePrimitiveValue(expr interface{}, path string) (expressionResult, error) {
	sql, err := p.literalToSQL(expr)
	if err != nil {
		return expressionResult{}, tperrors.Wrap(tperrors.ErrInvalidArgument, "", path, "invalid literal", err)
	}
	typ, known, truthy := literalTypeAndTruth(expr)
	if known {
		return literalValueResultWithRaw(sql, typ, truthy, expr), nil
	}
	return valueResult(sql, typ), nil
}

func (p *Parser) parsePrimitiveValueParam(expr interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	sql, err := p.literalToSQLParam(expr, pc)
	if err != nil {
		return expressionResult{}, tperrors.Wrap(tperrors.ErrInvalidArgument, "", path, "invalid literal", err)
	}
	typ, known, truthy := literalTypeAndTruth(expr)
	if known {
		return literalValueResultWithRaw(sql, typ, truthy, expr), nil
	}
	return valueResult(sql, typ), nil
}

func (p *Parser) parseExpressionPredicate(expr interface{}, path string) (expressionResult, error) {
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
		return p.parseExpressionPredicate(pv.Value, path)
	}
	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return expressionResult{}, tperrors.NewMultipleKeys(path)
		}
		for operator, args := range obj {
			operatorPath := tperrors.BuildPath(path, operator, -1)
			return p.parseOperatorPredicate(operator, args, operatorPath)
		}
	}
	return expressionResult{}, tperrors.New(tperrors.ErrInvalidExpression, "", path,
		fmt.Sprintf("invalid expression type: %T", expr))
}

func (p *Parser) parseExpressionValue(expr interface{}, path string) (expressionResult, error) {
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
		return p.parseExpressionValue(pv.Value, path)
	}
	if p.isPrimitive(expr) {
		return p.parsePrimitiveValue(expr, path)
	}
	if arr, ok := expr.([]interface{}); ok {
		sql, elemTypes, err := p.arrayLiteralToSQL(arr, path)
		if err != nil {
			return expressionResult{}, tperrors.Wrap(tperrors.ErrInvalidArgument, "", path, "invalid array literal", err)
		}
		return withArrayElementTypes(
			literalValueResultWithRaw(sql, operators.ExpressionTypeArray, len(arr) > 0, expr),
			elemTypes...,
		), nil
	}
	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return expressionResult{}, tperrors.NewMultipleKeys(path)
		}
		for operator, args := range obj {
			operatorPath := tperrors.BuildPath(path, operator, -1)
			return p.parseOperatorValue(operator, args, operatorPath)
		}
	}
	return expressionResult{}, tperrors.New(tperrors.ErrInvalidExpression, "", path,
		fmt.Sprintf("invalid expression type: %T", expr))
}

func (p *Parser) parseExpressionAny(expr interface{}, path string) (expressionResult, error) {
	if p.isPrimitive(expr) {
		return p.parsePrimitiveValue(expr, path)
	}
	if _, ok := expr.([]interface{}); ok {
		return p.parseExpressionValue(expr, path)
	}
	if obj, ok := expr.(map[string]interface{}); ok && len(obj) == 1 {
		for operator := range obj {
			switch operator {
			case "missing", "missing_some", "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in", "!", "!!", operators.OpAll, operators.OpSome, operators.OpNone:
				return p.parseExpressionPredicate(expr, path)
			case "and", "or", "if":
				if res, err := p.parseExpressionPredicate(expr, path); err == nil {
					return res, nil
				}
				return p.parseExpressionValue(expr, path)
			default:
				return p.parseExpressionValue(expr, path)
			}
		}
	}
	return p.parseExpressionValue(expr, path)
}

func (p *Parser) parseOperatorPredicate(operator string, args interface{}, path string) (expressionResult, error) {
	if p.customOpLookup != nil {
		if handler, ok := p.customOpLookup(operator); ok {
			processedArgs, err := p.processCustomOperatorArgs(args, path)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"failed to process custom operator arguments", err)
			}
			res, err := handler.ToSQL(operator, processedArgs)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"custom operator failed", err)
			}
			if res.Kind != operators.ExpressionKindPredicate {
				return expressionResult{}, tperrors.NewInvalidExpressionContext(operator, path, "predicate", kindName(res.Kind))
			}
			return resultFromOperator(res), nil
		}
	}

	switch operator {
	case "missing":
		sql, err := p.dataOp.ToSQL(operator, []interface{}{args})
		return predicateResult(sql), p.wrapOperatorError(operator, path, err)
	case "missing_some":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		sql, err := p.dataOp.ToSQL(operator, arr)
		return predicateResult(sql), p.wrapOperatorError(operator, path, err)
	case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		processedArgs, err := p.processValueArgs(arr, path)
		if err != nil {
			return expressionResult{}, err
		}
		sql, err := p.comparisonOp.ToSQL(operator, processedArgs)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		res, err := literalComparisonPredicateResult(operator, processedArgs, sql)
		return res, p.wrapOperatorError(operator, path, err)
	case "and", "or":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parsePredicateLogical(operator, arr, path)
	case "!":
		return p.parseNotPredicate(operator, args, path, false)
	case "!!":
		return p.parseNotPredicate(operator, args, path, true)
	case "if":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parsePredicateIf(arr, path)
	case operators.OpAll, operators.OpSome, operators.OpNone:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		sql, err := p.arrayOp.ToSQLAtPath(operator, arr, path)
		return predicateResult(sql), p.wrapOperatorError(operator, path, err)
	case "var", operators.OpMap, operators.OpFilter, operators.OpReduce, operators.OpMerge, "+", "-", "*", "/", "%", "max", "min", "cat", "substr":
		return expressionResult{}, tperrors.NewInvalidExpressionContext(operator, path, "predicate", "value")
	default:
		return expressionResult{}, tperrors.NewUnsupportedOperator(operator, path)
	}
}

func (p *Parser) parseOperatorValue(operator string, args interface{}, path string) (expressionResult, error) {
	if p.customOpLookup != nil {
		if handler, ok := p.customOpLookup(operator); ok {
			processedArgs, err := p.processCustomOperatorArgs(args, path)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"failed to process custom operator arguments", err)
			}
			res, err := handler.ToSQL(operator, processedArgs)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"custom operator failed", err)
			}
			return resultFromOperator(res), nil
		}
	}

	switch operator {
	case "var":
		sql, err := p.dataOp.ToSQL(operator, []interface{}{args})
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		if pv, ok := varProcessedExpression(args); ok {
			pv.Value = sql
			res := resultFromOperator(operatorResultFromProcessedValue(pv))
			copyProcessedFieldMetadata(&res, pv)
			res.requiresKnownTruthiness = pv.RequiresKnownTruthiness
			return withVarDefaultMetadata(res, args), nil
		}
		fieldName := varFieldName(args)
		return p.supportedValueResult(
			withVarDefaultMetadata(fieldValueResult(sql, p.fieldExpressionType(fieldName), fieldName), args),
			path,
		)
	case "missing", "missing_some", "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in", operators.OpAll, operators.OpSome, operators.OpNone:
		return p.parseOperatorPredicate(operator, args, path)
	case "!":
		return p.parseNotValue(operator, args, path, false)
	case "!!":
		return p.parseNotValue(operator, args, path, true)
	case "and", "or":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parseValueLogical(operator, arr, path)
	case "if":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parseValueIf(arr, path)
	case "+", "-", "*", "/", "%", "max", "min":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		processedArgs, err := p.processValueArgs(arr, path)
		if err != nil {
			return expressionResult{}, err
		}
		sql, err := p.numericOp.ToSQL(operator, processedArgs)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		return valueResult(sql, operators.ExpressionTypeNumber), nil
	case "cat":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parseCatValue(arr, path)
	case "substr":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		processedArgs, err := p.processValueArgs(arr, path)
		if err != nil {
			return expressionResult{}, err
		}
		sql, err := p.stringOp.ToSQL(operator, processedArgs)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		return valueResult(sql, operators.ExpressionTypeString), nil
	case operators.OpMap, operators.OpFilter, operators.OpMerge:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		res, err := p.arrayOp.ToValueResultAtPath(operator, arr, path)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		sql := res.SQL
		if arrayValueOperatorReturnsEmptyLiteral(operator, arr) || p.sqlIsEmptyArrayLiteral(sql) {
			return literalValueResultWithRaw(sql, operators.ExpressionTypeArray, false, []interface{}{}), nil
		}
		return resultFromOperator(res), nil
	case operators.OpReduce:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		if len(arr) == 3 && isEmptyArrayLiteralValue(arr[0]) {
			return p.parseExpressionValue(arr[2], tperrors.BuildArrayPath(path, 2))
		}
		sql, err := p.arrayOp.ToSQLAtPath(operator, arr, path)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		return valueResult(sql, p.inferReduceResultType(arr)), nil
	default:
		return expressionResult{}, tperrors.NewUnsupportedOperator(operator, path)
	}
}

func (p *Parser) parsePredicateLogical(operator string, args []interface{}, path string) (expressionResult, error) {
	if len(args) == 0 {
		return expressionResult{}, tperrors.NewInsufficientArgs(operator, path, 1, 0)
	}
	parts := make([]string, 0, len(args))
	for i, arg := range args {
		argPath := tperrors.BuildArrayPath(path, i)
		res, err := p.parseExpressionPredicate(arg, argPath)
		if err != nil {
			return expressionResult{}, err
		}
		if res.truthKnown {
			if operator == "and" && res.truthy {
				continue
			}
			if operator == "or" && !res.truthy {
				continue
			}
			if operator == "and" && !res.truthy {
				return booleanPredicateResult(false), nil
			}
			if operator == "or" && res.truthy {
				return booleanPredicateResult(true), nil
			}
		}
		parts = append(parts, res.SQL)
	}
	if len(parts) == 0 {
		return booleanPredicateResult(operator == "and"), nil
	}
	if len(parts) == 1 {
		return predicateResult(parts[0]), nil
	}
	joiner := " AND "
	if operator == "or" {
		joiner = " OR "
	}
	return predicateResult(fmt.Sprintf("(%s)", strings.Join(parts, joiner))), nil
}

func unaryArg(args interface{}) (interface{}, bool) {
	if arr, ok := args.([]interface{}); ok {
		if len(arr) != 1 {
			return nil, false
		}
		return arr[0], true
	}
	return args, true
}

func (p *Parser) parseNotPredicate(operator string, args interface{}, path string, double bool) (expressionResult, error) {
	arg, ok := unaryArg(args)
	if !ok {
		return expressionResult{}, tperrors.NewTypeMismatch(operator, path, "exactly 1 argument", "multiple arguments")
	}
	res, condition, err := p.parseTruthinessResult(arg, tperrors.BuildArrayPath(path, 0))
	if err != nil {
		return expressionResult{}, err
	}
	if double {
		if res.truthKnown {
			return booleanPredicateResult(res.truthy), nil
		}
		return predicateResult(condition), nil
	}
	if res.truthKnown {
		return booleanPredicateResult(!res.truthy), nil
	}
	condition = operators.StripRedundantOuterParens(condition)
	return predicateResult(fmt.Sprintf("NOT (%s)", condition)), nil
}

func (p *Parser) parseNotValue(operator string, args interface{}, path string, double bool) (expressionResult, error) {
	arg, ok := unaryArg(args)
	if !ok {
		return expressionResult{}, tperrors.NewTypeMismatch(operator, path, "exactly 1 argument", "multiple arguments")
	}
	res, condition, err := p.parseTruthinessResult(arg, tperrors.BuildArrayPath(path, 0))
	if err != nil {
		return expressionResult{}, err
	}
	if res.truthKnown {
		return booleanValueResult(res.truthy == double), nil
	}
	condition = operators.PredicateValueSQL(condition)
	if double {
		return valueResult(condition, operators.ExpressionTypeBoolean), nil
	}
	return valueResult(operators.PredicateValueSQL(fmt.Sprintf("NOT (%s)", condition)), operators.ExpressionTypeBoolean), nil
}

func (p *Parser) parsePredicateIf(args []interface{}, path string) (expressionResult, error) {
	if len(args) < 2 {
		return expressionResult{}, tperrors.NewInsufficientArgs("if", path, 2, len(args))
	}
	var parts []string
	pairLimit := len(args)
	hasElse := len(args)%2 == 1
	if hasElse {
		pairLimit = len(args) - 1
	}
	for i := 0; i < pairLimit; i += 2 {
		cond, condition, err := p.parsePredicateIfCondition(args[i], tperrors.BuildArrayPath(path, i))
		if err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && !cond.truthy {
			continue
		}
		thenRes, err := p.parsePredicateIfOperand(args[i+1], tperrors.BuildArrayPath(path, i+1))
		if err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && cond.truthy {
			if len(parts) == 0 {
				return thenRes, nil
			}
			return predicateResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), thenRes.SQL)), nil
		}
		parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", condition, thenRes.SQL))
	}
	elseSQL := "FALSE"
	if hasElse {
		elseRes, err := p.parsePredicateIfOperand(args[len(args)-1], tperrors.BuildArrayPath(path, len(args)-1))
		if err != nil {
			return expressionResult{}, err
		}
		if len(parts) == 0 {
			return elseRes, nil
		}
		elseSQL = elseRes.SQL
	}
	if len(parts) == 0 {
		return booleanPredicateResult(false), nil
	}
	return predicateResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), elseSQL)), nil
}

func (p *Parser) parsePredicateIfCondition(expr interface{}, path string) (expressionResult, string, error) {
	return p.parseTruthinessResult(expr, path)
}

func (p *Parser) parsePredicateIfOperand(expr interface{}, path string) (expressionResult, error) {
	if b, ok := expr.(bool); ok {
		return booleanPredicateResult(b), nil
	}
	return p.parseExpressionPredicate(expr, path)
}

func (p *Parser) parseValueIf(args []interface{}, path string) (expressionResult, error) {
	if len(args) < 2 {
		return expressionResult{}, tperrors.NewInsufficientArgs("if", path, 2, len(args))
	}
	var parts []string
	resultRes := literalValueResult("NULL", operators.ExpressionTypeNull, false)
	typeSet := false
	mergeResultType := func(res expressionResult) error {
		if !typeSet {
			resultRes = res
			typeSet = true
			return nil
		}
		merged, err := compatibleValueResult(resultRes, res, path)
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
		cond, condition, err := p.parseTruthinessResult(args[i], tperrors.BuildArrayPath(path, i))
		if err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && !cond.truthy {
			continue
		}
		thenRes, err := p.parseExpressionValue(args[i+1], tperrors.BuildArrayPath(path, i+1))
		if err != nil {
			return expressionResult{}, err
		}
		if err := mergeResultType(thenRes); err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && cond.truthy {
			if len(parts) == 0 {
				return thenRes, nil
			}
			result := valueResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), valueSQL(thenRes)), valueTypeOf(resultRes))
			if elemTypes, ok := arrayElementTypesOf(resultRes); ok {
				result = withArrayElementTypes(result, elemTypes...)
			}
			return result, nil
		}
		parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", condition, valueSQL(thenRes)))
	}
	elseSQL := "NULL"
	if hasElse {
		elseRes, err := p.parseExpressionValue(args[len(args)-1], tperrors.BuildArrayPath(path, len(args)-1))
		if err != nil {
			return expressionResult{}, err
		}
		if len(parts) == 0 {
			return elseRes, nil
		}
		if err := mergeResultType(elseRes); err != nil {
			return expressionResult{}, err
		}
		elseSQL = valueSQL(elseRes)
	}
	if len(parts) == 0 {
		return literalValueResult("NULL", operators.ExpressionTypeNull, false), nil
	}
	result := valueResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), elseSQL), valueTypeOf(resultRes))
	if elemTypes, ok := arrayElementTypesOf(resultRes); ok {
		result = withArrayElementTypes(result, elemTypes...)
	}
	return result, nil
}

func (p *Parser) parseValueLogical(operator string, args []interface{}, path string) (expressionResult, error) {
	if len(args) == 0 {
		return expressionResult{}, tperrors.NewInsufficientArgs(operator, path, 1, 0)
	}
	return p.parseValueLogicalFrom(operator, args, 0, path)
}

func (p *Parser) parseValueLogicalFrom(operator string, args []interface{}, index int, path string) (expressionResult, error) {
	argPath := tperrors.BuildArrayPath(path, index)
	if current, ok := nonFiniteNativeFloatTruthResult(args[index]); ok {
		if index == len(args)-1 ||
			(operator == "or" && current.truthy) ||
			(operator == "and" && !current.truthy) {
			return expressionResult{}, nonFiniteNativeFloatValueError(args[index], argPath)
		}
		return p.parseValueLogicalFrom(operator, args, index+1, path)
	}

	current, err := p.parseExpressionValue(args[index], argPath)
	if err != nil {
		return expressionResult{}, err
	}
	if index == len(args)-1 {
		return current, nil
	}
	if current.truthKnown {
		if operator == "or" && current.truthy {
			return current, nil
		}
		if operator == "and" && !current.truthy {
			return current, nil
		}
		return p.parseValueLogicalFrom(operator, args, index+1, path)
	}
	condition, err := p.truthinessSQL(current, argPath)
	if err != nil {
		return expressionResult{}, err
	}
	rest, err := p.parseValueLogicalFrom(operator, args, index+1, path)
	if err != nil {
		return expressionResult{}, err
	}
	resultRes, err := compatibleValueResult(current, rest, path)
	if err != nil {
		return expressionResult{}, err
	}
	if operator == "or" {
		result := valueResult(fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, valueSQL(current), valueSQL(rest)), valueTypeOf(resultRes))
		if elemTypes, ok := arrayElementTypesOf(resultRes); ok {
			result = withArrayElementTypes(result, elemTypes...)
		}
		return result, nil
	}
	result := valueResult(fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, valueSQL(rest), valueSQL(current)), valueTypeOf(resultRes))
	if elemTypes, ok := arrayElementTypesOf(resultRes); ok {
		result = withArrayElementTypes(result, elemTypes...)
	}
	return result, nil
}

func (p *Parser) parseCatValue(args []interface{}, path string) (expressionResult, error) {
	if len(args) == 0 {
		return expressionResult{}, tperrors.NewInsufficientArgs(operators.OpCat, path, 1, 0)
	}
	operands := make([]string, len(args))
	for i, arg := range args {
		res, err := p.parseCatStringExpression(arg, tperrors.BuildArrayPath(path, i))
		if err != nil {
			return expressionResult{}, err
		}
		operands[i] = res.SQL
	}
	return valueResult(fmt.Sprintf("CONCAT(%s)", strings.Join(operands, ", ")), operators.ExpressionTypeString), nil
}

func (p *Parser) parseCatStringExpression(expr interface{}, path string) (expressionResult, error) {
	res, err := p.parseExpressionValue(expr, path)
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
	switch operator {
	case operators.OpIf:
		return p.parseStringifiedIf(arr, path)
	case operators.OpAnd, operators.OpOr:
		return p.parseStringifiedLogical(operator, arr, path)
	default:
		return expressionResult{}, err
	}
}

func (p *Parser) stringifiedCatResult(res expressionResult, path string) (expressionResult, error) {
	if err := p.validateCatStringifiableResult(res, path); err != nil {
		return expressionResult{}, err
	}
	return valueResult(p.catStringSQL(res), operators.ExpressionTypeString), nil
}

func (p *Parser) validateCatStringifiableResult(res expressionResult, path string) error {
	if res.fieldValue && res.fieldName != "" {
		fieldType := p.config.Schema.GetFieldType(res.fieldName)
		if fieldType != "" {
			if p.config.Schema.IsStringType(res.fieldName) || p.config.Schema.IsNumericType(res.fieldName) {
				return nil
			}
			if p.config.Schema.IsArrayType(res.fieldName) || fieldType == "object" {
				return tperrors.New(tperrors.ErrInvalidArgument, operators.OpCat, path,
					fmt.Sprintf("string operation on incompatible field '%s' (type: %s)", res.fieldName, fieldType))
			}
		}
	}
	if valueTypeOf(res) == operators.ExpressionTypeArray {
		return tperrors.New(tperrors.ErrInvalidArgument, operators.OpCat, path,
			"string operation on incompatible array value")
	}
	return nil
}

func (p *Parser) parseStringifiedIf(args []interface{}, path string) (expressionResult, error) {
	if len(args) < 2 {
		return expressionResult{}, tperrors.NewInsufficientArgs(operators.OpIf, path, 2, len(args))
	}
	var parts []string
	pairLimit := len(args)
	hasElse := len(args)%2 == 1
	if hasElse {
		pairLimit = len(args) - 1
	}
	for i := 0; i < pairLimit; i += 2 {
		cond, condition, err := p.parseTruthinessResult(args[i], tperrors.BuildArrayPath(path, i))
		if err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && !cond.truthy {
			continue
		}
		thenRes, err := p.parseCatStringExpression(args[i+1], tperrors.BuildArrayPath(path, i+1))
		if err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && cond.truthy {
			if len(parts) == 0 {
				return thenRes, nil
			}
			return valueResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), thenRes.SQL),
				operators.ExpressionTypeString), nil
		}
		parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", condition, thenRes.SQL))
	}
	elseSQL := "''"
	if hasElse {
		elseRes, err := p.parseCatStringExpression(args[len(args)-1], tperrors.BuildArrayPath(path, len(args)-1))
		if err != nil {
			return expressionResult{}, err
		}
		if len(parts) == 0 {
			return elseRes, nil
		}
		elseSQL = elseRes.SQL
	}
	if len(parts) == 0 {
		return literalValueResult("''", operators.ExpressionTypeString, false), nil
	}
	return valueResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), elseSQL),
		operators.ExpressionTypeString), nil
}

func (p *Parser) parseStringifiedLogical(operator string, args []interface{}, path string) (expressionResult, error) {
	if len(args) == 0 {
		return expressionResult{}, tperrors.NewInsufficientArgs(operator, path, 1, 0)
	}
	return p.parseStringifiedLogicalFrom(operator, args, 0, path)
}

func (p *Parser) parseStringifiedLogicalFrom(operator string, args []interface{}, index int, path string) (expressionResult, error) {
	argPath := tperrors.BuildArrayPath(path, index)
	if current, ok := nonFiniteNativeFloatTruthResult(args[index]); ok {
		if index == len(args)-1 ||
			(operator == operators.OpOr && current.truthy) ||
			(operator == operators.OpAnd && !current.truthy) {
			return expressionResult{}, nonFiniteNativeFloatValueError(args[index], argPath)
		}
		return p.parseStringifiedLogicalFrom(operator, args, index+1, path)
	}

	current, condition, err := p.parseTruthinessResult(args[index], argPath)
	if err != nil {
		return expressionResult{}, err
	}
	if index == len(args)-1 {
		return p.parseCatStringExpression(args[index], argPath)
	}
	if current.truthKnown {
		if operator == operators.OpOr && current.truthy {
			return p.parseCatStringExpression(args[index], argPath)
		}
		if operator == operators.OpAnd && !current.truthy {
			return p.parseCatStringExpression(args[index], argPath)
		}
		return p.parseStringifiedLogicalFrom(operator, args, index+1, path)
	}
	currentString, err := p.parseCatStringExpression(args[index], argPath)
	if err != nil {
		return expressionResult{}, err
	}
	rest, err := p.parseStringifiedLogicalFrom(operator, args, index+1, path)
	if err != nil {
		return expressionResult{}, err
	}
	if operator == operators.OpOr {
		return valueResult(fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, currentString.SQL, rest.SQL),
			operators.ExpressionTypeString), nil
	}
	return valueResult(fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, rest.SQL, currentString.SQL),
		operators.ExpressionTypeString), nil
}
