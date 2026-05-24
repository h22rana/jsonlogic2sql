package operators

import (
	"fmt"
)

// handleAll converts all operator to SQL.
// This checks if all elements in an array satisfy a condition.
// Generates: NOT EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE NOT (condition)).
// For ClickHouse: Uses arrayAll function.
func (a *ArrayOperator) handleAll(args []interface{}) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("all requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("all"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return sqlFalse, nil
	}

	// First argument: array
	arrayValue, err := a.valueToTypedSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all array argument: %w", err)
	}
	if arraySourceErr := a.validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return "", fmt.Errorf("invalid all array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return sqlFalse, nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopesForValue(args[arraySourceArgIndex], arrayValue)

	// Second argument: truthiness expression - rewrite element vars before SQL generation
	condition, err := a.withArrayLambdaSource(arrayLambdaScopeElement, sourceScopes, arrayValue, false).
		truthinessExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all condition argument: %w", err)
	}
	if err := a.validateCompatibleArrayElementScopes(sourceScopes); err != nil {
		return "", fmt.Errorf("invalid all array argument: %w", err)
	}

	// JSONLogic spec: {"all": [[], condition]} returns false (empty array = false).
	// Without a guard, SQL NOT EXISTS on an empty UNNEST returns true (no rows to violate).
	// We add an emptiness check: array must be non-null and non-empty.
	alias := a.elemAlias()
	return a.renderAllSQL(alias, array, condition), nil
}

// handleSome converts some operator to SQL.
// This checks if some elements in an array satisfy a condition.
// Generates: EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE condition).
// For ClickHouse: Uses arrayExists function.
func (a *ArrayOperator) handleSome(args []interface{}) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("some requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("some"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return sqlFalse, nil
	}

	// First argument: array
	arrayValue, err := a.valueToTypedSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some array argument: %w", err)
	}
	if arraySourceErr := a.validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return "", fmt.Errorf("invalid some array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return sqlFalse, nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopesForValue(args[arraySourceArgIndex], arrayValue)

	// Second argument: truthiness expression - rewrite element vars before SQL generation
	condition, err := a.withArrayLambdaSource(arrayLambdaScopeElement, sourceScopes, arrayValue, false).
		truthinessExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some condition argument: %w", err)
	}
	if err := a.validateCompatibleArrayElementScopes(sourceScopes); err != nil {
		return "", fmt.Errorf("invalid some array argument: %w", err)
	}

	alias := a.elemAlias()
	return a.renderSomeSQL(alias, array, condition), nil
}

// handleNone converts none operator to SQL.
// This checks if no elements in an array satisfy a condition.
// Generates: NOT EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE condition).
// For ClickHouse: Uses NOT arrayExists function.
func (a *ArrayOperator) handleNone(args []interface{}) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("none requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("none"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return sqlTrue, nil
	}

	// First argument: array
	arrayValue, err := a.valueToTypedSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none array argument: %w", err)
	}
	if arraySourceErr := a.validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return "", fmt.Errorf("invalid none array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return sqlTrue, nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopesForValue(args[arraySourceArgIndex], arrayValue)

	// Second argument: truthiness expression - rewrite element vars before SQL generation
	condition, err := a.withArrayLambdaSource(arrayLambdaScopeElement, sourceScopes, arrayValue, false).
		truthinessExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none condition argument: %w", err)
	}
	if err := a.validateCompatibleArrayElementScopes(sourceScopes); err != nil {
		return "", fmt.Errorf("invalid none array argument: %w", err)
	}

	alias := a.elemAlias()
	return a.renderNoneSQL(alias, array, condition), nil
}
