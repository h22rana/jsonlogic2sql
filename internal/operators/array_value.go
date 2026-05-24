package operators

import (
	"fmt"
	"strings"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
)

func (a *ArrayOperator) valueToSQL(value interface{}) (string, error) {
	return a.valueToSQLAtPath(value, a.currentPath())
}

func (a *ArrayOperator) valueExpressionToSQLWithContextAndPath(expr interface{}, allowAccumulator bool, path string) (string, error) {
	res, err := a.valueExpressionResultWithContextAndPath(expr, allowAccumulator, path)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) valueExpressionResultWithContextAndPath(expr interface{}, allowAccumulator bool, path string) (OperatorResult, error) {
	if a.config == nil || !a.config.HasValueExpressionParser() {
		sql, err := a.expressionToSQLWithContextAndPath(expr, allowAccumulator, path)
		if err != nil {
			return OperatorResult{}, err
		}
		return a.localValueExpressionResult(expr, sql), nil
	}
	if a.shouldParseScopedArrayExpressionLocally(expr) {
		sql, err := a.expressionToSQLWithContextAndPath(expr, allowAccumulator, path)
		if err != nil {
			return OperatorResult{}, err
		}
		return a.localValueExpressionResult(expr, sql), nil
	}
	rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(expr, allowAccumulator, path)
	if err != nil {
		return OperatorResult{}, err
	}
	res, err := a.config.ParseValueExpression(rewritten, path)
	if err != nil {
		return OperatorResult{}, err
	}
	return res, nil
}

func (a *ArrayOperator) localValueExpressionResult(expr interface{}, sql string) OperatorResult {
	if isPredicateArrayExpression(expr) {
		return ValueSQL(PredicateValueSQL(sql), ExpressionTypeBoolean)
	}
	if result, ok := a.localArrayExpressionMetadata(expr, sql); ok {
		return result
	}
	return ValueSQL(sql, a.inferValueExpressionType(expr))
}

func (a *ArrayOperator) localArrayExpressionMetadata(expr interface{}, sql string) (OperatorResult, bool) {
	operator, args, ok := arrayOperatorArgs(expr)
	if !ok {
		return OperatorResult{}, false
	}
	switch operator {
	case OpMap:
		return a.localMapExpressionMetadata(args, sql)
	case OpFilter:
		return a.localFilterExpressionMetadata(args, sql)
	case OpMerge:
		return a.localMergeExpressionMetadata(args, sql)
	default:
		return OperatorResult{}, false
	}
}

func (a *ArrayOperator) localMapExpressionMetadata(args []interface{}, sql string) (OperatorResult, bool) {
	if len(args) != 2 {
		return OperatorResult{}, false
	}
	source, err := a.valueToTypedSQLAtPath(args[0], a.argPath(0))
	if err != nil {
		return OperatorResult{}, false
	}
	elementOp := a.withArrayLambdaSource(arrayLambdaScopeMap, typedValueSchemaScopes(source), source, true)
	res, err := elementOp.valueExpressionResultWithContextAndPath(args[1], false, a.argPath(1))
	if err != nil {
		return OperatorResult{}, false
	}
	result := arrayValueSQLWithMetadata(sql, mappedArrayElementTypes(res), res.ArrayElementSchemaScopes, mappedArrayElementSchemaType(res))
	return result, true
}

func (a *ArrayOperator) localFilterExpressionMetadata(args []interface{}, sql string) (OperatorResult, bool) {
	if len(args) != 2 {
		return OperatorResult{}, false
	}
	source, err := a.valueToTypedSQLAtPath(args[0], a.argPath(0))
	if err != nil {
		return OperatorResult{}, false
	}
	return arrayValueSQLWithMetadata(sql, typedValueElementTypes(source), typedValueSchemaScopes(source), typedValueElementSchemaType(source)), true
}

func (a *ArrayOperator) localMergeExpressionMetadata(args []interface{}, sql string) (OperatorResult, bool) {
	values := make([]typedValueSQL, 0, len(args))
	for i, arg := range args {
		if isEmptyArrayLiteral(arg) {
			values = append(values, typedValueSQL{typ: ExpressionTypeArray, emptyArrayLiteral: true})
			continue
		}
		value, err := a.valueToTypedSQLAtPath(arg, a.argPath(i))
		if err != nil {
			return OperatorResult{}, false
		}
		values = append(values, value)
	}
	common, err := validateMergeElementCompatibility(values)
	if err != nil {
		return OperatorResult{}, false
	}
	return arrayValueSQLWithMetadata(sql, mergeElementTypes(values, common), a.arraySourceSchemaScopesForValues(args, values), common.schemaType), true
}

func isPredicateArrayExpression(expr interface{}) bool {
	operator, _, ok := arrayOperatorArgs(expr)
	if !ok {
		return false
	}
	switch operator {
	case OpAll, OpSome, OpNone:
		return true
	default:
		return false
	}
}

func (a *ArrayOperator) predicateExpressionToSQLWithContextAndPath(expr interface{}, path string) (string, error) {
	if a.config == nil || !a.config.HasPredicateExpressionParser() {
		return a.expressionToSQLWithContextAndPath(expr, false, path)
	}
	if a.shouldParseScopedArrayExpressionLocally(expr) {
		return a.expressionToSQLWithContextAndPath(expr, false, path)
	}
	rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(expr, false, path)
	if err != nil {
		return "", err
	}
	res, err := a.config.ParsePredicateExpression(rewritten, path)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) truthinessExpressionToSQLWithContextAndPath(expr interface{}, path string) (string, error) {
	if a.config == nil || !a.config.HasTruthinessExpressionParser() {
		return a.predicateExpressionToSQLWithContextAndPath(expr, path)
	}
	if a.shouldParseScopedArrayExpressionLocally(expr) {
		sql, err := a.expressionToSQLWithContextAndPath(expr, false, path)
		if err != nil {
			return "", err
		}
		return a.localTruthinessExpressionSQL(expr, sql, path)
	}
	rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(expr, false, path)
	if err != nil {
		return "", err
	}
	return a.config.ParseTruthinessExpression(rewritten, path)
}

func (a *ArrayOperator) localTruthinessExpressionSQL(expr interface{}, sql, path string) (string, error) {
	if isPredicateArrayExpression(expr) {
		return sql, nil
	}
	result := a.localValueExpressionResult(expr, sql)
	if result.Kind == ExpressionKindPredicate {
		return result.SQL, nil
	}
	switch result.Type {
	case ExpressionTypeNull:
		return "FALSE", nil
	case ExpressionTypeBoolean:
		return fmt.Sprintf("%s IS TRUE", result.SQL), nil
	case ExpressionTypeString:
		return fmt.Sprintf("(%s IS NOT NULL AND %s != '')", result.SQL, result.SQL), nil
	case ExpressionTypeNumber:
		return fmt.Sprintf("(%s IS NOT NULL AND %s != 0)", result.SQL, result.SQL), nil
	case ExpressionTypeArray:
		lengthCheck := a.arrayLengthSQL(result.SQL)
		return fmt.Sprintf("(%s IS NOT NULL AND %s > 0)", result.SQL, lengthCheck), nil
	case ExpressionTypeObject:
		return "", tperrors.New(
			tperrors.ErrInvalidExpressionContext,
			"",
			path,
			"object-valued expression cannot be used as a locally scoped array condition",
		)
	case ExpressionTypeUnknown:
		return "", tperrors.New(
			tperrors.ErrInvalidExpressionContext,
			"",
			path,
			"truthiness requires a statically known value type for locally scoped array expression",
		)
	default:
		return "", tperrors.New(
			tperrors.ErrInvalidExpressionContext,
			"",
			path,
			"unsupported locally scoped array expression type",
		)
	}
}

func (a *ArrayOperator) valueToSQLAtPath(value interface{}, path string) (string, error) {
	result, err := a.valueToTypedSQLAtPath(value, path)
	if err != nil {
		return "", err
	}
	return result.sql, nil
}

func (a *ArrayOperator) valueToTypedSQLAtPath(value interface{}, path string) (typedValueSQL, error) {
	// Handle ProcessedValue (pre-processed SQL from parser)
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			if pv.HasExpressionInfo {
				return typedSQLFromOperatorResult(OperatorResult{
					SQL:                      pv.Value,
					Kind:                     pv.Kind,
					Type:                     pv.Type,
					SchemaType:               pv.SchemaType,
					PreserveParamRefs:        pv.PreserveParamRefs,
					ArrayElementType:         pv.ArrayElementType,
					ArrayElementTypes:        pv.ArrayElementTypes,
					ArrayElementSchemaType:   pv.ArrayElementSchemaType,
					ArrayElementSchemaScopes: pv.ArrayElementSchemaScopes,
				}), nil
			}
			return typedValueSQL{sql: pv.Value, typ: ExpressionTypeUnknown}, nil
		}
		// It's a literal, convert it
		sql, err := a.dataOp.valueToSQL(pv.Value)
		if err != nil {
			return typedValueSQL{}, err
		}
		return typedValueSQL{sql: sql, typ: inferLiteralValueExpressionType(pv.Value)}, nil
	}

	// Handle complex expressions (operators)
	if expr, ok := value.(map[string]interface{}); ok {
		if len(expr) != 1 {
			return typedValueSQL{}, tperrors.NewMultipleKeys(path)
		}
		if varExpr, hasVar := expr[OpVar]; hasVar {
			if sql, handled, err := a.arrayInternalVarToSQL(varExpr); handled || err != nil {
				if err != nil {
					return typedValueSQL{}, err
				}
				pv := a.scopedSQLFieldResult(sql, a.scopedFieldNamesFromVarExpr(varExpr)...)
				if pv.HasExpressionInfo {
					return typedSQLFromProcessedValue(pv), nil
				}
				return typedValueSQL{sql: sql, typ: a.inferValueExpressionType(value)}, nil
			}
			sql, err := a.dataOp.ToSQL(OpVar, []interface{}{varExpr})
			if err != nil {
				return typedValueSQL{}, err
			}
			fieldType := a.schemaExpressionType(a.extractFieldName(varExpr))
			if fieldType == ExpressionTypeUnknown {
				fieldType = a.inferValueExpressionType(value)
			}
			return a.typedFieldSQL(sql, a.extractFieldName(varExpr), fieldType), nil
		}
		// Otherwise, it's a complex value expression.
		res, err := a.valueExpressionResultWithContextAndPath(value, false, path)
		if err != nil {
			return typedValueSQL{}, err
		}
		return typedSQLFromOperatorResult(res), nil
	}

	// Handle arrays
	if arr, ok := value.([]interface{}); ok {
		if err := a.config.ValidateArrayLiteralValue(arr); err != nil {
			return typedValueSQL{}, err
		}
		elements := make([]string, len(arr))
		var commonTypes []ExpressionType
		var schemaScopes []string
		for i, elem := range arr {
			element, err := a.valueToTypedSQLAtPath(elem, tperrors.BuildArrayPath(path, i))
			if err != nil {
				return typedValueSQL{}, fmt.Errorf("invalid array element %d: %w", i, err)
			}
			schemaScopes = append(schemaScopes, typedValueSchemaScopes(element)...)
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
		schemaScopes = normalizeSchemaScopes(schemaScopes)
		if err := a.validateCompatibleArrayElementScopes(schemaScopes); err != nil {
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
			schemaScopes:      schemaScopes,
			emptyArrayLiteral: len(arr) == 0,
		}, nil
	}

	// Handle primitive values
	sql, err := a.dataOp.valueToSQL(value)
	if err != nil {
		return typedValueSQL{}, err
	}
	return typedValueSQL{sql: sql, typ: inferLiteralValueExpressionType(value)}, nil
}

func (a *ArrayOperator) arrayLiteral(elements []string) (string, error) {
	if a.config != nil {
		return a.config.ArrayLiteral(elements)
	}
	return fmt.Sprintf("[%s]", strings.Join(elements, ", ")), nil
}

func (a *ArrayOperator) emptyArrayLiteralSQL() (string, error) {
	return a.arrayLiteral(nil)
}
