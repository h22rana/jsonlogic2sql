package operators

import (
	"fmt"
)

// handleMap converts map operator to SQL.
// Generates: ARRAY(SELECT transformation FROM UNNEST(array) AS elem).
// For ClickHouse: Uses arrayMap or subquery with arrayJoin.
func (a *ArrayOperator) handleMap(args []interface{}) (string, error) {
	res, err := a.handleMapResult(args)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) handleMapResult(args []interface{}) (OperatorResult, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return OperatorResult{}, fmt.Errorf("map requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("map"); err != nil {
			return OperatorResult{}, err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return OperatorResult{}, err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return emptyArrayResult(a.emptyArrayLiteralSQL())
	}

	// First argument: array
	arrayValue, err := a.valueToTypedSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
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

	valueScoped := a.withArrayLambdaSource(arrayLambdaScopeElement, sourceScopes, arrayValue, true)
	transformation, err := valueScoped.valueExpressionResultWithContextAndPath(
		args[arrayExpressionArgIndex],
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

// handleFilter converts filter operator to SQL.
// Generates: ARRAY(SELECT elem FROM UNNEST(array) AS elem WHERE condition).
// For ClickHouse: Uses arrayFilter function.
func (a *ArrayOperator) handleFilter(args []interface{}) (string, error) {
	res, err := a.handleFilterResult(args)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) handleFilterResult(args []interface{}) (OperatorResult, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return OperatorResult{}, fmt.Errorf("filter requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("filter"); err != nil {
			return OperatorResult{}, err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return OperatorResult{}, err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return emptyArrayResult(a.emptyArrayLiteralSQL())
	}

	// First argument: array
	arrayValue, err := a.valueToTypedSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
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

	// Second argument: truthiness expression - rewrite element vars before SQL generation
	condition, err := a.withArrayLambdaSource(arrayLambdaScopeElement, sourceScopes, arrayValue, false).
		truthinessExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return OperatorResult{}, fmt.Errorf("invalid filter condition argument: %w", err)
	}

	alias := a.elemAlias()
	return arrayValueSQLWithElementTypes(a.renderFilterSQL(alias, array, condition), typedValueElementTypes(arrayValue)...), nil
}
