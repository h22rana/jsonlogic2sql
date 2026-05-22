package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func isEmptyArrayLiteralValue(value interface{}) bool {
	arr, ok := value.([]interface{})
	return ok && len(arr) == 0
}

func arrayValueOperatorReturnsEmptyLiteral(operator string, args []interface{}) bool {
	switch operator {
	case operators.OpMap, operators.OpFilter:
		return len(args) > 0 && isEmptyArrayLiteralValue(args[0])
	case operators.OpMerge:
		if len(args) == 0 {
			return false
		}
		for _, arg := range args {
			if !isEmptyArrayLiteralValue(arg) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (p *Parser) sqlIsEmptyArrayLiteral(sql string) bool {
	if p.config == nil {
		return sql == "[]"
	}
	emptySQL, err := p.config.ArrayLiteral(nil)
	return err == nil && sql == emptySQL
}

func (p *Parser) literalToSQL(value interface{}) (string, error) {
	return p.dataOp.ValueToSQL(value)
}

func (p *Parser) literalToSQLParam(value interface{}, pc *params.ParamCollector) (string, error) {
	return p.dataOp.ValueToSQLParam(value, pc)
}

func (p *Parser) arrayLiteralToSQL(arr []interface{}, path string) (string, []operators.ExpressionType, error) {
	parts := make([]string, len(arr))
	var commonTypes []operators.ExpressionType
	for i, elem := range arr {
		res, err := p.parseExpressionValue(elem, tperrors.BuildArrayPath(path, i))
		if err != nil {
			return "", nil, fmt.Errorf("invalid array element %d: %w", i, err)
		}
		commonTypes, err = updateArrayLiteralElementTypes(commonTypes, res, i)
		if err != nil {
			return "", nil, err
		}
		parts[i] = valueSQL(res)
	}
	elementTypes := normalizeExpressionTypes(commonTypes)
	if err := p.config.ValidateArrayLiteralElementTypes(elementTypes); err != nil {
		return "", nil, err
	}
	sql, err := p.config.ArrayLiteral(parts)
	return sql, elementTypes, err
}

func (p *Parser) arrayLiteralToSQLParam(
	arr []interface{},
	path string,
	pc *params.ParamCollector,
) (string, []operators.ExpressionType, error) {
	parts := make([]string, len(arr))
	var commonTypes []operators.ExpressionType
	for i, elem := range arr {
		res, err := p.parseExpressionValueParam(elem, tperrors.BuildArrayPath(path, i), pc)
		if err != nil {
			return "", nil, fmt.Errorf("invalid array element %d: %w", i, err)
		}
		commonTypes, err = updateArrayLiteralElementTypes(commonTypes, res, i)
		if err != nil {
			return "", nil, err
		}
		parts[i] = valueSQL(res)
	}
	elementTypes := normalizeExpressionTypes(commonTypes)
	if err := p.config.ValidateArrayLiteralElementTypes(elementTypes); err != nil {
		return "", nil, err
	}
	sql, err := p.config.ArrayLiteral(parts)
	return sql, elementTypes, err
}

func expressionResultTypeChain(res expressionResult) []operators.ExpressionType {
	typ := valueTypeOf(res)
	if typ == operators.ExpressionTypeUnknown {
		return nil
	}
	if expressionResultIsEmptyArrayLiteral(res) {
		return []operators.ExpressionType{operators.ExpressionTypeArray}
	}
	if typ != operators.ExpressionTypeArray {
		return []operators.ExpressionType{typ}
	}
	chain := []operators.ExpressionType{operators.ExpressionTypeArray}
	if elemTypes, ok := arrayElementTypesOf(res); ok {
		chain = append(chain, elemTypes...)
	}
	return chain
}

func updateArrayLiteralElementTypes(common []operators.ExpressionType, res expressionResult, index int) ([]operators.ExpressionType, error) {
	elemTypes := expressionResultTypeChain(res)
	if len(elemTypes) == 0 {
		return common, nil
	}
	if elemTypes[0] == operators.ExpressionTypeNull {
		if len(common) == 0 {
			return elemTypes, nil
		}
		return common, nil
	}
	if len(common) > 0 && common[0] == operators.ExpressionTypeNull {
		return elemTypes, nil
	}
	if len(common) == 0 {
		return elemTypes, nil
	}
	merged, ok := mergeArrayLiteralElementTypes(common, elemTypes)
	if !ok {
		return common, fmt.Errorf("array literal elements must have compatible SQL types: element %d has type %s, previous non-null elements have type %s",
			index,
			arrayElementTypesName(elemTypes),
			arrayElementTypesName(common))
	}
	return merged, nil
}

func mergeArrayLiteralElementTypes(
	common []operators.ExpressionType,
	elemTypes []operators.ExpressionType,
) ([]operators.ExpressionType, bool) {
	if sameExpressionTypes(common, elemTypes) {
		return common, true
	}
	if isArrayOnlyTypePrefix(common, elemTypes) {
		return elemTypes, true
	}
	if isArrayOnlyTypePrefix(elemTypes, common) {
		return common, true
	}
	return nil, false
}

func isArrayOnlyTypePrefix(short, long []operators.ExpressionType) bool {
	if len(short) == 0 || len(short) >= len(long) {
		return false
	}
	for i, typ := range short {
		if typ != operators.ExpressionTypeArray || typ != long[i] {
			return false
		}
	}
	return true
}

func literalTypeAndTruth(value interface{}) (operators.ExpressionType, bool, bool) {
	switch v := value.(type) {
	case nil:
		return operators.ExpressionTypeNull, true, false
	case bool:
		return operators.ExpressionTypeBoolean, true, v
	case string:
		return operators.ExpressionTypeString, true, v != ""
	case json.Number, float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return operators.ExpressionTypeNumber, true, !isZeroLiteral(value)
	case []interface{}:
		return operators.ExpressionTypeArray, true, len(v) > 0
	default:
		return operators.ExpressionTypeUnknown, false, false
	}
}

func isZeroLiteral(value interface{}) bool {
	switch v := value.(type) {
	case json.Number:
		return isZeroJSONNumberLiteral(v.String())
	case float32:
		return v == 0 || math.IsNaN(float64(v))
	case float64:
		return v == 0 || math.IsNaN(v)
	case int:
		return v == 0
	case int8:
		return v == 0
	case int16:
		return v == 0
	case int32:
		return v == 0
	case int64:
		return v == 0
	case uint:
		return v == 0
	case uint8:
		return v == 0
	case uint16:
		return v == 0
	case uint32:
		return v == 0
	case uint64:
		return v == 0
	default:
		return false
	}
}

func isZeroJSONNumberLiteral(s string) bool {
	f, err := strconv.ParseFloat(s, 64)
	if err == nil || errors.Is(err, strconv.ErrRange) {
		return f == 0
	}

	// Malformed json.Number values should already be rejected when rendered, but
	// keep a conservative fallback for manually constructed values.
	if exponent := strings.IndexAny(s, "eE"); exponent >= 0 {
		s = s[:exponent]
	}
	for _, ch := range s {
		if ch >= '1' && ch <= '9' {
			return false
		}
	}
	return true
}

func nonFiniteNativeFloatTruthResult(value interface{}) (expressionResult, bool) {
	switch v := value.(type) {
	case float32:
		f := float64(v)
		if math.IsNaN(f) {
			return literalValueResultWithRaw("", operators.ExpressionTypeNumber, false, value), true
		}
		if math.IsInf(f, 0) {
			return literalValueResultWithRaw("", operators.ExpressionTypeNumber, true, value), true
		}
	case float64:
		if math.IsNaN(v) {
			return literalValueResultWithRaw("", operators.ExpressionTypeNumber, false, value), true
		}
		if math.IsInf(v, 0) {
			return literalValueResultWithRaw("", operators.ExpressionTypeNumber, true, value), true
		}
	}
	return expressionResult{}, false
}

func nonFiniteNativeFloatValueError(value interface{}, path string) error {
	if err := operators.ValidateFiniteNativeFloat(value); err != nil {
		return tperrors.Wrap(tperrors.ErrInvalidArgument, "", path, "invalid literal", err)
	}
	return tperrors.New(tperrors.ErrInvalidArgument, "", path, fmt.Sprintf("invalid literal: %T is not non-finite", value))
}

// isPrimitive checks if a value is a primitive type.
func (p *Parser) isPrimitive(value interface{}) bool {
	switch value.(type) {
	case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number, bool:
		return true
	case nil:
		return true
	default:
		return false
	}
}
