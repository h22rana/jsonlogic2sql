package operators

import (
	"fmt"
)

// handleMerge converts merge operator to SQL.
// JSONLogic merge casts scalar arguments into single-element arrays.
// BigQuery/Spanner: ARRAY_CONCAT(array1, array2, ...)
// PostgreSQL: array1 || array2 || ...
func (a *ArrayOperator) handleMerge(args []interface{}) (string, error) {
	res, err := a.handleMergeResult(args)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) handleMergeResult(args []interface{}) (OperatorResult, error) {
	// Validate dialect support
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
		arrayValue, err := a.valueToTypedSQLAtPath(arg, a.argPath(i))
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
	return arrayValueSQLWithMetadata(sql, elementTypes, sourceScopes), nil
}
