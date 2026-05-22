package parser

import (
	"fmt"
	"strings"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func (p *Parser) truthinessSQL(res expressionResult, path string) (string, error) {
	if res.Kind == operators.ExpressionKindPredicate {
		return res.SQL, nil
	}
	if res.truthKnown {
		if res.truthy {
			return sqlTrue, nil
		}
		return sqlFalse, nil
	}
	switch res.Type {
	case operators.ExpressionTypeNull:
		return sqlFalse, nil
	case operators.ExpressionTypeBoolean:
		return fmt.Sprintf("%s IS TRUE", res.SQL), nil
	case operators.ExpressionTypeString:
		return fmt.Sprintf("(%s IS NOT NULL AND %s != '')", res.SQL, res.SQL), nil
	case operators.ExpressionTypeNumber:
		return fmt.Sprintf("(%s IS NOT NULL AND %s != 0)", res.SQL, res.SQL), nil
	case operators.ExpressionTypeArray:
		lengthCheck := p.config.ArrayLengthFunc(res.SQL)
		return fmt.Sprintf("(%s IS NOT NULL AND %s > 0)", res.SQL, lengthCheck), nil
	case operators.ExpressionTypeUnknown:
		if res.requiresKnownTruthiness {
			return "", tperrors.New(tperrors.ErrInvalidExpressionContext, "", path,
				"truthiness requires a statically known accumulator type")
		}
		if res.fieldValue {
			return "", tperrors.New(tperrors.ErrInvalidExpressionContext, "", path,
				"truthiness requires a statically known field type; provide schema information")
		}
		return fmt.Sprintf("(%s IS NOT NULL AND %s != FALSE AND %s != 0 AND %s != '')",
			res.SQL, res.SQL, res.SQL, res.SQL), nil
	}
	return fmt.Sprintf("(%s IS NOT NULL AND %s != FALSE AND %s != 0 AND %s != '')",
		res.SQL, res.SQL, res.SQL, res.SQL), nil
}

func (p *Parser) parseTruthinessResult(expr interface{}, path string) (expressionResult, string, error) {
	return p.parseTruthinessExpression(expr, path)
}

func (p *Parser) parseTruthinessExpression(expr interface{}, path string) (expressionResult, string, error) {
	if res, ok := nonFiniteNativeFloatTruthResult(expr); ok {
		condition, err := p.truthinessSQL(res, path)
		return res, condition, err
	}
	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return expressionResult{}, "", tperrors.NewMultipleKeys(path)
		}
		for operator, args := range obj {
			operatorPath := tperrors.BuildPath(path, operator, -1)
			switch operator {
			case "and", "or":
				arr, ok := args.([]interface{})
				if !ok {
					return expressionResult{}, "", tperrors.NewOperatorRequiresArray(operator, operatorPath)
				}
				return p.parseTruthinessLogical(operator, arr, operatorPath)
			case "if":
				arr, ok := args.([]interface{})
				if !ok {
					return expressionResult{}, "", tperrors.NewOperatorRequiresArray(operator, operatorPath)
				}
				return p.parseTruthinessIf(arr, operatorPath)
			}
		}
	}
	res, err := p.parseExpressionAny(expr, path)
	if err != nil {
		return expressionResult{}, "", err
	}
	condition, err := p.truthinessSQL(res, path)
	if err != nil {
		return expressionResult{}, "", err
	}
	return res, condition, nil
}

func (p *Parser) parseTruthinessLogical(operator string, args []interface{}, path string) (expressionResult, string, error) {
	if len(args) == 0 {
		return expressionResult{}, "", tperrors.NewInsufficientArgs(operator, path, 1, 0)
	}
	parts := make([]string, 0, len(args))
	for i, arg := range args {
		res, condition, err := p.parseTruthinessResult(arg, tperrors.BuildArrayPath(path, i))
		if err != nil {
			return expressionResult{}, "", err
		}
		if res.truthKnown {
			if operator == "and" && res.truthy {
				continue
			}
			if operator == "or" && !res.truthy {
				continue
			}
			if operator == "and" && !res.truthy {
				return booleanPredicateResult(false), sqlFalse, nil
			}
			if operator == "or" && res.truthy {
				return booleanPredicateResult(true), sqlTrue, nil
			}
		}
		parts = append(parts, condition)
	}
	if len(parts) == 0 {
		res := booleanPredicateResult(operator == "and")
		return res, res.SQL, nil
	}
	if len(parts) == 1 {
		res := predicateResult(parts[0])
		return res, res.SQL, nil
	}
	joiner := " AND "
	if operator == "or" {
		joiner = " OR "
	}
	sql := fmt.Sprintf("(%s)", strings.Join(parts, joiner))
	return predicateResult(sql), sql, nil
}

func (p *Parser) parseTruthinessIf(args []interface{}, path string) (expressionResult, string, error) {
	if len(args) < 2 {
		return expressionResult{}, "", tperrors.NewInsufficientArgs("if", path, 2, len(args))
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
			return expressionResult{}, "", err
		}
		if cond.truthKnown && !cond.truthy {
			continue
		}
		thenRes, thenCondition, err := p.parseTruthinessResult(args[i+1], tperrors.BuildArrayPath(path, i+1))
		if err != nil {
			return expressionResult{}, "", err
		}
		if cond.truthKnown && cond.truthy {
			if len(parts) == 0 {
				return thenRes, thenCondition, nil
			}
			sql := fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), thenCondition)
			return predicateResult(sql), sql, nil
		}
		parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", condition, thenCondition))
	}
	elseRes := booleanPredicateResult(false)
	elseCondition := elseRes.SQL
	if hasElse {
		var err error
		elseRes, elseCondition, err = p.parseTruthinessResult(args[len(args)-1], tperrors.BuildArrayPath(path, len(args)-1))
		if err != nil {
			return expressionResult{}, "", err
		}
		if len(parts) == 0 {
			return elseRes, elseCondition, nil
		}
	}
	if len(parts) == 0 {
		return booleanPredicateResult(false), sqlFalse, nil
	}
	sql := fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), elseCondition)
	return predicateResult(sql), sql, nil
}

func (p *Parser) parseTruthinessResultParam(expr interface{}, path string, pc *params.ParamCollector) (expressionResult, string, error) {
	checkpoint := pc.Checkpoint()
	res, condition, err := p.parseTruthinessExpressionParam(expr, path, pc)
	if err != nil {
		return expressionResult{}, "", err
	}
	if (res.truthKnown || (res.Kind != operators.ExpressionKindPredicate && res.Type == operators.ExpressionTypeNull)) &&
		canRollbackParamRefs(res) {
		pc.Restore(checkpoint)
	}
	return res, condition, nil
}

func (p *Parser) parseTruthinessExpressionParam(
	expr interface{},
	path string,
	pc *params.ParamCollector,
) (expressionResult, string, error) {
	if res, ok := nonFiniteNativeFloatTruthResult(expr); ok {
		condition, err := p.truthinessSQL(res, path)
		return res, condition, err
	}
	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return expressionResult{}, "", tperrors.NewMultipleKeys(path)
		}
		for operator, args := range obj {
			operatorPath := tperrors.BuildPath(path, operator, -1)
			switch operator {
			case "and", "or":
				arr, ok := args.([]interface{})
				if !ok {
					return expressionResult{}, "", tperrors.NewOperatorRequiresArray(operator, operatorPath)
				}
				return p.parseTruthinessLogicalParam(operator, arr, operatorPath, pc)
			case "if":
				arr, ok := args.([]interface{})
				if !ok {
					return expressionResult{}, "", tperrors.NewOperatorRequiresArray(operator, operatorPath)
				}
				return p.parseTruthinessIfParam(arr, operatorPath, pc)
			}
		}
	}
	res, err := p.parseExpressionAnyParam(expr, path, pc)
	if err != nil {
		return expressionResult{}, "", err
	}
	condition, err := p.truthinessSQL(res, path)
	if err != nil {
		return expressionResult{}, "", err
	}
	return res, condition, nil
}

func (p *Parser) parseTruthinessLogicalParam(
	operator string,
	args []interface{},
	path string,
	pc *params.ParamCollector,
) (expressionResult, string, error) {
	if len(args) == 0 {
		return expressionResult{}, "", tperrors.NewInsufficientArgs(operator, path, 1, 0)
	}
	checkpoint := pc.Checkpoint()
	parts := make([]string, 0, len(args))
	partResults := make([]expressionResult, 0, len(args))
	for i, arg := range args {
		operandCheckpoint := pc.Checkpoint()
		res, condition, err := p.parseTruthinessResultParam(arg, tperrors.BuildArrayPath(path, i), pc)
		if err != nil {
			return expressionResult{}, "", err
		}
		if res.truthKnown && canRollbackParamRefs(res) {
			if operator == "and" && res.truthy {
				pc.Restore(operandCheckpoint)
				continue
			}
			if operator == "or" && !res.truthy {
				pc.Restore(operandCheckpoint)
				continue
			}
			if operator == "and" && !res.truthy {
				pc.Restore(checkpoint)
				return booleanPredicateResult(false), sqlFalse, nil
			}
			if operator == "or" && res.truthy {
				pc.Restore(checkpoint)
				return booleanPredicateResult(true), sqlTrue, nil
			}
		}
		parts = append(parts, condition)
		partResults = append(partResults, res)
	}
	if len(parts) == 0 {
		res := booleanPredicateResult(operator == "and")
		return res, res.SQL, nil
	}
	if len(parts) == 1 {
		res := preserveParamRefsIfNeeded(predicateResult(parts[0]), partResults[0])
		return res, res.SQL, nil
	}
	joiner := " AND "
	if operator == "or" {
		joiner = " OR "
	}
	sql := fmt.Sprintf("(%s)", strings.Join(parts, joiner))
	res := preserveParamRefsIfNeeded(predicateResult(sql), partResults...)
	return res, sql, nil
}

func (p *Parser) parseTruthinessIfParam(
	args []interface{},
	path string,
	pc *params.ParamCollector,
) (expressionResult, string, error) {
	if len(args) < 2 {
		return expressionResult{}, "", tperrors.NewInsufficientArgs("if", path, 2, len(args))
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
			return expressionResult{}, "", err
		}
		if cond.truthKnown && !cond.truthy {
			paramRefs.mark(cond)
			continue
		}
		paramRefs.mark(cond)
		thenRes, thenCondition, err := p.parseTruthinessResultParam(args[i+1], tperrors.BuildArrayPath(path, i+1), pc)
		if err != nil {
			return expressionResult{}, "", err
		}
		paramRefs.mark(thenRes)
		if cond.truthKnown && cond.truthy {
			if len(parts) == 0 {
				return paramRefs.apply(thenRes), thenCondition, nil
			}
			sql := fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), thenCondition)
			return paramRefs.apply(predicateResult(sql)), sql, nil
		}
		parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", condition, thenCondition))
	}
	elseRes := booleanPredicateResult(false)
	elseCondition := elseRes.SQL
	if hasElse {
		var err error
		elseRes, elseCondition, err = p.parseTruthinessResultParam(
			args[len(args)-1],
			tperrors.BuildArrayPath(path, len(args)-1),
			pc,
		)
		if err != nil {
			return expressionResult{}, "", err
		}
		paramRefs.mark(elseRes)
		if len(parts) == 0 {
			return paramRefs.apply(elseRes), elseCondition, nil
		}
	}
	if len(parts) == 0 {
		return paramRefs.apply(booleanPredicateResult(false)), sqlFalse, nil
	}
	sql := fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), elseCondition)
	return paramRefs.apply(predicateResult(sql)), sql, nil
}
