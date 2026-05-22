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
	if len(args) == 0 {
		return "", fmt.Errorf("array operator %s requires at least one argument", operator)
	}
	switch operator {
	case "map":
		return a.handleMapParam(args, pc)
	case "filter":
		return a.handleFilterParam(args, pc)
	case "reduce":
		return a.handleReduceParam(args, pc)
	case "all":
		return a.handleAllParam(args, pc)
	case "some":
		return a.handleSomeParam(args, pc)
	case "none":
		return a.handleNoneParam(args, pc)
	case "merge":
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
	if len(args) == 0 {
		return OperatorResult{}, fmt.Errorf("array operator %s requires at least one argument", operator)
	}

	switch operator {
	case OpMap:
		return a.handleMapResultParam(args, pc)
	case OpFilter:
		return a.handleFilterResultParam(args, pc)
	case OpMerge:
		return a.handleMergeResultParam(args, pc)
	default:
		return OperatorResult{}, fmt.Errorf("unsupported value array operator: %s", operator)
	}
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
	if arraySourceErr := validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return OperatorResult{}, fmt.Errorf("invalid map array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return emptyArrayResult(a.emptyArrayLiteralSQL())
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])
	valueScoped := a.withLambdaScope(arrayLambdaScopeElement).
		withSchemaScopes(sourceScopes).
		withSourceElementType(arrayValue, sourceScopes).
		withValueSemantics(true)
	transformation, err := valueScoped.valueExpressionResultParamWithContextAndPath(
		args[arrayExpressionArgIndex],
		pc,
		false,
		a.argPath(arrayExpressionArgIndex),
	)
	if err != nil {
		return OperatorResult{}, fmt.Errorf("invalid map transformation argument: %w", err)
	}
	alias := a.elemAlias()
	sql := a.renderMapSQL(alias, transformation.SQL, array)
	return arrayValueSQLWithElementTypes(sql, mappedArrayElementTypes(transformation)...), nil
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
	if arraySourceErr := validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return OperatorResult{}, fmt.Errorf("invalid filter array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return emptyArrayResult(a.emptyArrayLiteralSQL())
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])
	condition, err := a.withLambdaScope(arrayLambdaScopeElement).
		withSchemaScopes(sourceScopes).
		withSourceElementType(arrayValue, sourceScopes).
		truthinessExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return OperatorResult{}, fmt.Errorf("invalid filter condition argument: %w", err)
	}
	alias := a.elemAlias()
	return arrayValueSQLWithElementTypes(a.renderFilterSQL(alias, array, condition), typedValueElementTypes(arrayValue)...), nil
}

// handleReduceParam is the parameterized variant of handleReduce. Keep in sync.
func (a *ArrayOperator) handleReduceParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != reduceOperatorArgCount {
		return "", fmt.Errorf("reduce requires exactly 3 arguments")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("reduce"); err != nil {
			return "", err
		}
	}
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	initialValue, err := a.valueToTypedSQLParamAtPath(args[arrayReduceInitialArgIndex], pc, a.argPath(arrayReduceInitialArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce initial argument: %w", err)
	}
	initial := initialValue.sql
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return initial, nil
	}
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce array argument: %w", err)
	}
	if arraySourceErr := validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return "", fmt.Errorf("invalid reduce array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return initial, nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])
	reducerExpr := args[arrayExpressionArgIndex]
	alias := a.elemAlias()

	reduceScoped := a.withLambdaScope(arrayLambdaScopeReduce).
		withSchemaScopes(sourceScopes).
		withSourceElementType(arrayValue, sourceScopes)
	if pattern := reduceScoped.detectAggregatePattern(reducerExpr); pattern != nil {
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
					return "", quoteErr
				}
				aggregateInput = fmt.Sprintf("arrayMap(%s -> %s, %s)", mapAlias, mappedRef, array)
			}
			aggregateSQL := fmt.Sprintf("arrayReduce('%s', %s)", strings.ToLower(pattern.function), aggregateInput)
			return renderReduceAggregateResult(pattern.function, initial, aggregateSQL, true, array), nil
		case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
			elemRef, quoteErr := reduceScoped.aggregateElementSQLParam(alias, pattern, pc)
			if quoteErr != nil {
				return "", quoteErr
			}
			aggregateSQL := fmt.Sprintf("(SELECT %s(%s) FROM %s)", pattern.function, elemRef, a.unnestSourceSQL(array, alias))
			return renderReduceAggregateResult(pattern.function, initial, aggregateSQL, false, ""), nil
		}
		elemRef, quoteErr := reduceScoped.aggregateElementSQLParam(alias, pattern, pc)
		if quoteErr != nil {
			return "", quoteErr
		}
		aggregateSQL := fmt.Sprintf("(SELECT %s(%s) FROM %s)", pattern.function, elemRef, a.unnestSourceSQL(array, alias))
		return renderReduceAggregateResult(pattern.function, initial, aggregateSQL, false, ""), nil
	}

generalReduceParam:
	accumulatorSQL := AccumulatorVar
	if a.getDialect() == dialect.DialectClickHouse || a.getDialect() == dialect.DialectDuckDB {
		accumulatorSQL = "acc"
	}
	valueScoped := reduceScoped.withValueSemantics(true).withAccumulatorType(initialValue.typ).withAccumulatorSQL(accumulatorSQL)
	reducerWithElem, err := valueScoped.expressionToSQLParamWithContextAndPath(reducerExpr, pc, true, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce expression: %w", err)
	}
	return a.renderGeneralReduceSQL(alias, array, reducerWithElem, initial, initialValue.typ)
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
		return "FALSE", nil
	}
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all array argument: %w", err)
	}
	if arraySourceErr := validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return "", fmt.Errorf("invalid all array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return "FALSE", nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])
	condition, err := a.withLambdaScope(arrayLambdaScopeElement).
		withSchemaScopes(sourceScopes).
		withSourceElementType(arrayValue, sourceScopes).
		truthinessExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all condition argument: %w", err)
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
		return "FALSE", nil
	}
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some array argument: %w", err)
	}
	if arraySourceErr := validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return "", fmt.Errorf("invalid some array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return "FALSE", nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])
	condition, err := a.withLambdaScope(arrayLambdaScopeElement).
		withSchemaScopes(sourceScopes).
		withSourceElementType(arrayValue, sourceScopes).
		truthinessExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some condition argument: %w", err)
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
		return "TRUE", nil
	}
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none array argument: %w", err)
	}
	if arraySourceErr := validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return "", fmt.Errorf("invalid none array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return "TRUE", nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])
	condition, err := a.withLambdaScope(arrayLambdaScopeElement).
		withSchemaScopes(sourceScopes).
		withSourceElementType(arrayValue, sourceScopes).
		truthinessExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none condition argument: %w", err)
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
	if len(args) < 1 {
		return OperatorResult{}, fmt.Errorf("merge requires at least 1 argument")
	}
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
	return arrayValueSQLWithElementTypes(sql, mergeElementTypes(values, common)...), nil
}

// valueToSQLParam is the parameterized variant of valueToSQL. Keep in sync.
