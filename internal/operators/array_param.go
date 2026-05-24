package operators

import (
	"errors"
	"fmt"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// ToSQLParam is the parameterized variant of ToSQL.
func (a *ArrayOperator) ToSQLParam(operator string, args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) == 0 && operator != OpMerge {
		return "", fmt.Errorf("array operator %s requires at least one argument", operator)
	}
	switch operator {
	case OpMap:
		return a.handleMapParam(args, pc)
	case OpFilter:
		return a.handleFilterParam(args, pc)
	case OpReduce:
		return a.handleReduceParam(args, pc)
	case OpAll:
		return a.handleAllParam(args, pc)
	case OpSome:
		return a.handleSomeParam(args, pc)
	case OpNone:
		return a.handleNoneParam(args, pc)
	case OpMerge:
		return a.handleMergeParam(args, pc)
	default:
		return "", fmt.Errorf("unsupported array operator: %s", operator)
	}
}

// ToSQLParamAtPath is the parameterized variant of ToSQLAtPath. Keep in sync.
func (a *ArrayOperator) ToSQLParamAtPath(operator string, args []interface{}, pc *params.ParamCollector, path string) (string, error) {
	scoped := a.withPath(path)
	if scoped.shouldUseRenderedSourceChildScope(operator, args) {
		return scoped.withChildScope().ToSQLParam(operator, args, pc)
	}
	return scoped.ToSQLParam(operator, args, pc)
}

// ToValueResultParamAtPath is the parameterized variant of ToValueResultAtPath.
func (a *ArrayOperator) ToValueResultParamAtPath(
	operator string,
	args []interface{},
	pc *params.ParamCollector,
	path string,
) (OperatorResult, error) {
	scoped := a.withPath(path)
	if scoped.shouldUseRenderedSourceChildScope(operator, args) {
		return scoped.withChildScope().ToValueResultParam(operator, args, pc)
	}
	return scoped.ToValueResultParam(operator, args, pc)
}

// ToValueResultParam is the parameterized variant of ToValueResult.
func (a *ArrayOperator) ToValueResultParam(
	operator string,
	args []interface{},
	pc *params.ParamCollector,
) (OperatorResult, error) {
	if len(args) == 0 && operator != OpMerge {
		return OperatorResult{}, fmt.Errorf("array operator %s requires at least one argument", operator)
	}

	switch operator {
	case OpMap:
		return a.handleMapResultParam(args, pc)
	case OpFilter:
		return a.handleFilterResultParam(args, pc)
	case OpReduce:
		return a.handleReduceResultParam(args, pc)
	case OpMerge:
		return a.handleMergeResultParam(args, pc)
	default:
		return OperatorResult{}, fmt.Errorf("unsupported value array operator: %s", operator)
	}
}

func (a *ArrayOperator) emptyArrayResultParam(sources ...typedValueSQL) (OperatorResult, error) {
	result, err := emptyArrayResult(a.emptyArrayLiteralSQL())
	if err != nil {
		return OperatorResult{}, err
	}
	return preserveParamRefsFromTypedValues(result, sources...), nil
}

// handleMapParam is the parameterized variant of handleMap. Keep in sync.
func (a *ArrayOperator) handleMapParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	res, err := a.handleMapResultParam(args, pc)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) handleMapResultParam(args []interface{}, pc *params.ParamCollector) (OperatorResult, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return OperatorResult{}, fmt.Errorf("map requires exactly 2 arguments")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("map"); err != nil {
			return OperatorResult{}, err
		}
	}
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return OperatorResult{}, err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return emptyArrayResult(a.emptyArrayLiteralSQL())
	}
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return OperatorResult{}, fmt.Errorf("invalid map array argument: %w", err)
	}
	if arraySourceErr := a.validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return OperatorResult{}, fmt.Errorf("invalid map array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return a.emptyArrayResultParam(arrayValue)
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopesForValue(args[arraySourceArgIndex], arrayValue)
	valueScoped := a.withArrayLambdaSource(arrayLambdaScopeElement, sourceScopes, arrayValue, true)
	transformation, err := valueScoped.valueExpressionResultParamWithContextAndPath(
		args[arrayExpressionArgIndex],
		pc,
		false,
		a.argPath(arrayExpressionArgIndex),
	)
	if err != nil {
		return OperatorResult{}, fmt.Errorf("invalid map transformation argument: %w", err)
	}
	if err := a.validateCompatibleArrayElementScopes(sourceScopes); err != nil {
		return OperatorResult{}, fmt.Errorf("invalid map array argument: %w", err)
	}
	alias := a.elemAlias()
	sql := a.renderMapSQL(alias, transformation.SQL, array)
	elementTypes := mappedArrayElementTypes(transformation)
	if err := a.validateArrayResultElementTypes(elementTypes); err != nil {
		return OperatorResult{}, err
	}
	var elementSchemaScopes []string
	switch expressionTypeFromResultKind(transformation.Kind, transformation.Type) {
	case ExpressionTypeObject:
		if scopes := normalizeSchemaScopes(transformation.ArrayElementSchemaScopes); len(scopes) > 0 {
			elementSchemaScopes = scopes
		} else if isIdentityElementMapExpression(args[arrayExpressionArgIndex]) {
			elementSchemaScopes = sourceScopes
		}
	case ExpressionTypeArray:
		elementSchemaScopes = normalizeSchemaScopes(transformation.ArrayElementSchemaScopes)
	case ExpressionTypeUnknown, ExpressionTypeNull, ExpressionTypeBoolean, ExpressionTypeString, ExpressionTypeNumber:
	}
	result := arrayValueSQLWithMetadata(sql, elementTypes, elementSchemaScopes, mappedArrayElementSchemaType(transformation))
	result.PreserveParamRefs = arrayValue.preserveParamRefs || transformation.PreserveParamRefs
	return result, nil
}

// handleFilterParam is the parameterized variant of handleFilter. Keep in sync.
func (a *ArrayOperator) handleFilterParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	res, err := a.handleFilterResultParam(args, pc)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) handleFilterResultParam(args []interface{}, pc *params.ParamCollector) (OperatorResult, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return OperatorResult{}, fmt.Errorf("filter requires exactly 2 arguments")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("filter"); err != nil {
			return OperatorResult{}, err
		}
	}
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return OperatorResult{}, err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return emptyArrayResult(a.emptyArrayLiteralSQL())
	}
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return OperatorResult{}, fmt.Errorf("invalid filter array argument: %w", err)
	}
	if arraySourceErr := a.validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return OperatorResult{}, fmt.Errorf("invalid filter array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return a.emptyArrayResultParam(arrayValue)
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopesForValue(args[arraySourceArgIndex], arrayValue)
	condition, err := a.withArrayLambdaSource(arrayLambdaScopeElement, sourceScopes, arrayValue, false).
		truthinessExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return OperatorResult{}, fmt.Errorf("invalid filter condition argument: %w", err)
	}
	if err := a.validateCompatibleArrayElementScopes(sourceScopes); err != nil {
		return OperatorResult{}, fmt.Errorf("invalid filter array argument: %w", err)
	}
	alias := a.elemAlias()
	result := arrayValueSQLWithMetadata(
		a.renderFilterSQL(alias, array, condition),
		typedValueElementTypes(arrayValue),
		typedValueSchemaScopes(arrayValue),
		typedValueElementSchemaType(arrayValue),
	)
	result.PreserveParamRefs = arrayValue.preserveParamRefs
	return result, nil
}

// handleReduceParam is the parameterized variant of handleReduce. Keep in sync.
func (a *ArrayOperator) handleReduceParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	res, err := a.handleReduceResultParam(args, pc)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) handleReduceResultParam(args []interface{}, pc *params.ParamCollector) (OperatorResult, error) {
	if len(args) != reduceOperatorArgCount {
		return OperatorResult{}, fmt.Errorf("reduce requires exactly 3 arguments")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("reduce"); err != nil {
			return OperatorResult{}, err
		}
	}
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return OperatorResult{}, err
	}
	initialValue, err := a.valueToTypedSQLParamAtPath(args[arrayReduceInitialArgIndex], pc, a.argPath(arrayReduceInitialArgIndex))
	if err != nil {
		return OperatorResult{}, fmt.Errorf("invalid reduce initial argument: %w", err)
	}
	initial := initialValue.sql
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return operatorResultFromTypedValue(initialValue), nil
	}
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return OperatorResult{}, fmt.Errorf("invalid reduce array argument: %w", err)
	}
	if arraySourceErr := a.validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return OperatorResult{}, fmt.Errorf("invalid reduce array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return preserveParamRefsFromTypedValues(operatorResultFromTypedValue(initialValue), arrayValue), nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopesForValue(args[arraySourceArgIndex], arrayValue)
	if scopeErr := a.validateCompatibleArrayElementScopes(sourceScopes); scopeErr != nil {
		return OperatorResult{}, fmt.Errorf("invalid reduce array argument: %w", scopeErr)
	}
	reducerExpr := args[arrayExpressionArgIndex]
	alias := a.elemAlias()

	reduceScoped := a.withArrayLambdaSource(arrayLambdaScopeReduce, sourceScopes, arrayValue, false)
	if pattern := reduceScoped.detectAggregatePattern(reducerExpr); pattern != nil {
		numericInitial, initialErr := a.numericAggregateInitialSQLParam(args[arrayReduceInitialArgIndex], initialValue)
		if initialErr != nil {
			return OperatorResult{}, fmt.Errorf("invalid reduce initial argument: %w", initialErr)
		}
		if aggregateErr := reduceScoped.validateAggregatePattern(pattern); aggregateErr != nil {
			return OperatorResult{}, aggregateErr
		}
		switch a.getDialect() {
		case dialect.DialectClickHouse:
			aggregateInput := array
			if pattern.requiresArrayMap() {
				mapAlias := alias
				if !pattern.hasTermExpr {
					mapAlias = "x"
				}
				mappedRef, quoteErr := reduceScoped.aggregateElementSQLParam(mapAlias, pattern, pc)
				if quoteErr != nil {
					if errors.Is(quoteErr, errUnsupportedGeneralReduce) {
						goto generalReduceParam
					}
					return OperatorResult{}, quoteErr
				}
				aggregateInput = fmt.Sprintf("arrayMap(%s -> %s, %s)", mapAlias, mappedRef, array)
			}
			aggregateSQL := fmt.Sprintf("arrayReduce('%s', %s)", strings.ToLower(pattern.function), aggregateInput)
			return preserveParamRefsFromTypedValues(
				ValueSQL(renderReduceAggregateResult(pattern.function, numericInitial, aggregateSQL, true, array), ExpressionTypeNumber),
				initialValue, arrayValue,
			), nil
		case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
			elemRef, quoteErr := reduceScoped.aggregateElementSQLParam(alias, pattern, pc)
			if quoteErr != nil {
				return OperatorResult{}, quoteErr
			}
			aggregateSQL := fmt.Sprintf("(SELECT %s(%s) FROM %s)", pattern.function, elemRef, a.unnestSourceSQL(array, alias))
			return preserveParamRefsFromTypedValues(
				ValueSQL(renderReduceAggregateResult(pattern.function, numericInitial, aggregateSQL, false, ""), ExpressionTypeNumber),
				initialValue, arrayValue,
			), nil
		}
		elemRef, quoteErr := reduceScoped.aggregateElementSQLParam(alias, pattern, pc)
		if quoteErr != nil {
			return OperatorResult{}, quoteErr
		}
		aggregateSQL := fmt.Sprintf("(SELECT %s(%s) FROM %s)", pattern.function, elemRef, a.unnestSourceSQL(array, alias))
		return preserveParamRefsFromTypedValues(
			ValueSQL(renderReduceAggregateResult(pattern.function, numericInitial, aggregateSQL, false, ""), ExpressionTypeNumber),
			initialValue, arrayValue,
		), nil
	}

generalReduceParam:
	accumulatorSQL := AccumulatorVar
	if a.getDialect() == dialect.DialectClickHouse || a.getDialect() == dialect.DialectDuckDB {
		accumulatorSQL = "acc"
	}
	valueScoped := reduceScoped.withReduceValueScope(initialValue, accumulatorSQL)
	reducerResult, err := valueScoped.valueExpressionResultParamWithContextAndPath(reducerExpr, pc, true, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return OperatorResult{}, fmt.Errorf("invalid reduce expression: %w", err)
	}
	sql, err := a.renderGeneralReduceSQL(alias, array, reducerResult.SQL, initial, initialValue.typ)
	if err != nil {
		return OperatorResult{}, err
	}
	return preserveParamRefsFromTypedValues(operatorResultWithSQL(reducerResult, sql), initialValue, arrayValue), nil
}

// handleAllParam is the parameterized variant of handleAll. Keep in sync.
func (a *ArrayOperator) handleAllParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("all requires exactly 2 arguments")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("all"); err != nil {
			return "", err
		}
	}
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return sqlFalse, nil
	}
	paramStart := len(pc.RawParams())
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all array argument: %w", err)
	}
	if arraySourceErr := a.validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return "", fmt.Errorf("invalid all array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return sqlFalse, validateNewParamRefs(sqlFalse, pc, paramStart)
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopesForValue(args[arraySourceArgIndex], arrayValue)
	condition, err := a.withArrayLambdaSource(arrayLambdaScopeElement, sourceScopes, arrayValue, false).
		truthinessExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all condition argument: %w", err)
	}
	if err := a.validateCompatibleArrayElementScopes(sourceScopes); err != nil {
		return "", fmt.Errorf("invalid all array argument: %w", err)
	}

	alias := a.elemAlias()
	return a.renderAllSQL(alias, array, condition), nil
}

// handleSomeParam is the parameterized variant of handleSome. Keep in sync.
func (a *ArrayOperator) handleSomeParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("some requires exactly 2 arguments")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("some"); err != nil {
			return "", err
		}
	}
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return sqlFalse, nil
	}
	paramStart := len(pc.RawParams())
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some array argument: %w", err)
	}
	if arraySourceErr := a.validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return "", fmt.Errorf("invalid some array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return sqlFalse, validateNewParamRefs(sqlFalse, pc, paramStart)
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopesForValue(args[arraySourceArgIndex], arrayValue)
	condition, err := a.withArrayLambdaSource(arrayLambdaScopeElement, sourceScopes, arrayValue, false).
		truthinessExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some condition argument: %w", err)
	}
	if err := a.validateCompatibleArrayElementScopes(sourceScopes); err != nil {
		return "", fmt.Errorf("invalid some array argument: %w", err)
	}
	alias := a.elemAlias()
	return a.renderSomeSQL(alias, array, condition), nil
}

// handleNoneParam is the parameterized variant of handleNone. Keep in sync.
func (a *ArrayOperator) handleNoneParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("none requires exactly 2 arguments")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("none"); err != nil {
			return "", err
		}
	}
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return sqlTrue, nil
	}
	paramStart := len(pc.RawParams())
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none array argument: %w", err)
	}
	if arraySourceErr := a.validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return "", fmt.Errorf("invalid none array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return sqlTrue, validateNewParamRefs(sqlTrue, pc, paramStart)
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopesForValue(args[arraySourceArgIndex], arrayValue)
	condition, err := a.withArrayLambdaSource(arrayLambdaScopeElement, sourceScopes, arrayValue, false).
		truthinessExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none condition argument: %w", err)
	}
	if err := a.validateCompatibleArrayElementScopes(sourceScopes); err != nil {
		return "", fmt.Errorf("invalid none array argument: %w", err)
	}
	alias := a.elemAlias()
	return a.renderNoneSQL(alias, array, condition), nil
}

// handleMergeParam is the parameterized variant of handleMerge. Keep in sync.
func (a *ArrayOperator) handleMergeParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	res, err := a.handleMergeResultParam(args, pc)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) handleMergeResultParam(args []interface{}, pc *params.ParamCollector) (OperatorResult, error) {
	if a.config != nil {
		if err := a.config.ValidateDialect("merge"); err != nil {
			return OperatorResult{}, err
		}
	}
	values := make([]typedValueSQL, 0, len(args))
	for i, arg := range args {
		if isEmptyArrayLiteral(arg) {
			values = append(values, typedValueSQL{typ: ExpressionTypeArray, emptyArrayLiteral: true})
			continue
		}
		arrayValue, err := a.valueToTypedSQLParamAtPath(arg, pc, a.argPath(i))
		if err != nil {
			return OperatorResult{}, fmt.Errorf("invalid merge argument %d: %w", i, err)
		}
		values = append(values, arrayValue)
	}
	common, err := validateMergeElementCompatibility(values)
	if err != nil {
		return OperatorResult{}, err
	}
	sourceScopes := a.arraySourceSchemaScopesForValues(args, values)
	if scopeErr := a.validateCompatibleArrayElementScopes(sourceScopes); scopeErr != nil {
		return OperatorResult{}, fmt.Errorf("invalid merge argument schemas: %w", scopeErr)
	}

	arrays := make([]string, 0, len(values))
	for i, value := range values {
		var arraySQL string
		var skip bool
		arraySQL, skip, err = a.mergeValueToArraySQL(value, common)
		if err != nil {
			return OperatorResult{}, fmt.Errorf("invalid merge argument %d: %w", i, err)
		}
		if skip {
			continue
		}
		arrays = append(arrays, arraySQL)
	}
	sql, err := a.renderMergeSQL(arrays)
	if err != nil {
		return OperatorResult{}, err
	}
	elementTypes := mergeElementTypes(values, common)
	if err := a.validateArrayResultElementTypes(elementTypes); err != nil {
		return OperatorResult{}, err
	}
	result := arrayValueSQLWithMetadata(sql, elementTypes, sourceScopes, common.schemaType)
	result.PreserveParamRefs = typedValuesPreserveParamRefs(values)
	return result, nil
}

// valueToSQLParam is the parameterized variant of valueToSQL. Keep in sync.
