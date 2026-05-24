package operators

import (
	"fmt"
	"strings"
)

type typedValueSQL struct {
	sql               string
	typ               ExpressionType
	schemaType        string
	elemType          ExpressionType
	elemTypes         []ExpressionType
	elemSchemaType    string
	schemaScopes      []string
	emptyArrayLiteral bool
	preserveParamRefs bool
}

func normalizeArrayElementTypes(types []ExpressionType) []ExpressionType {
	if len(types) == 0 || types[0] == ExpressionTypeUnknown {
		return nil
	}
	normalized := make([]ExpressionType, 0, len(types))
	for _, typ := range types {
		if typ == ExpressionTypeUnknown {
			break
		}
		normalized = append(normalized, typ)
	}
	return normalized
}

func typedValueElementTypes(value typedValueSQL) []ExpressionType {
	if types := normalizeArrayElementTypes(value.elemTypes); len(types) > 0 {
		return types
	}
	if value.elemType != ExpressionTypeUnknown {
		return []ExpressionType{value.elemType}
	}
	return nil
}

func typedValueElementSchemaType(value typedValueSQL) string {
	return normalizeSchemaType(value.elemSchemaType)
}

func typedValueSchemaScopes(value typedValueSQL) []string {
	return normalizeSchemaScopes(value.schemaScopes)
}

func typedValuesPreserveParamRefs(values []typedValueSQL) bool {
	for _, value := range values {
		if value.preserveParamRefs {
			return true
		}
	}
	return false
}

func preserveParamRefsFromTypedValues(result OperatorResult, values ...typedValueSQL) OperatorResult {
	for _, value := range values {
		if value.preserveParamRefs {
			result.PreserveParamRefs = true
			return result
		}
	}
	return result
}

func typedValueTypeChain(value typedValueSQL) []ExpressionType {
	if value.typ == ExpressionTypeUnknown {
		return nil
	}
	if value.emptyArrayLiteral {
		return []ExpressionType{ExpressionTypeArray}
	}
	if value.typ != ExpressionTypeArray {
		return []ExpressionType{value.typ}
	}
	chain := []ExpressionType{ExpressionTypeArray}
	if elemTypes := typedValueElementTypes(value); len(elemTypes) > 0 {
		chain = append(chain, elemTypes...)
	}
	return chain
}

func updateArrayLiteralElementTypes(common []ExpressionType, value typedValueSQL, index int) ([]ExpressionType, error) {
	elemTypes := typedValueTypeChain(value)
	if len(elemTypes) == 0 {
		return common, nil
	}
	if elemTypes[0] == ExpressionTypeNull {
		if len(common) == 0 {
			return elemTypes, nil
		}
		return common, nil
	}
	if len(common) > 0 && common[0] == ExpressionTypeNull {
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

func mergeArrayLiteralElementTypes(common, elemTypes []ExpressionType) ([]ExpressionType, bool) {
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

func isArrayOnlyTypePrefix(short, long []ExpressionType) bool {
	if len(short) == 0 || len(short) >= len(long) {
		return false
	}
	for i, typ := range short {
		if typ != ExpressionTypeArray || typ != long[i] {
			return false
		}
	}
	return true
}

func firstArrayElementType(types []ExpressionType) ExpressionType {
	normalized := normalizeArrayElementTypes(types)
	if len(normalized) == 0 {
		return ExpressionTypeUnknown
	}
	return normalized[0]
}

func operatorResultElementTypes(res OperatorResult) []ExpressionType {
	if types := normalizeArrayElementTypes(res.ArrayElementTypes); len(types) > 0 {
		return types
	}
	if res.ArrayElementType != ExpressionTypeUnknown {
		return []ExpressionType{res.ArrayElementType}
	}
	return nil
}

func operatorResultElementSchemaType(res OperatorResult) string {
	return normalizeSchemaType(res.ArrayElementSchemaType)
}

func arrayValueSQLWithElementTypes(sql string, elemTypes ...ExpressionType) OperatorResult {
	normalized := normalizeArrayElementTypes(elemTypes)
	if len(normalized) == 0 {
		return ArrayValueSQL(sql, ExpressionTypeUnknown)
	}
	res := ArrayValueSQL(sql, normalized[0])
	res.ArrayElementTypes = normalized
	return res
}

func arrayValueSQLWithMetadata(sql string, elemTypes []ExpressionType, schemaScopes []string, elemSchemaTypes ...string) OperatorResult {
	res := arrayValueSQLWithElementTypes(sql, elemTypes...)
	res.ArrayElementSchemaScopes = normalizeSchemaScopes(schemaScopes)
	res.ArrayElementSchemaType = firstSchemaType(elemSchemaTypes...)
	return res
}

func mappedArrayElementTypes(result OperatorResult) []ExpressionType {
	typ := expressionTypeFromResultKind(result.Kind, result.Type)
	if typ == ExpressionTypeArray {
		return append([]ExpressionType{ExpressionTypeArray}, operatorResultElementTypes(result)...)
	}
	if typ == ExpressionTypeUnknown {
		return nil
	}
	return []ExpressionType{typ}
}

func mappedArrayElementSchemaType(result OperatorResult) string {
	typ := expressionTypeFromResultKind(result.Kind, result.Type)
	if typ == ExpressionTypeArray {
		return operatorResultElementSchemaType(result)
	}
	if typ == ExpressionTypeUnknown || typ == ExpressionTypeNull {
		return ""
	}
	return normalizeSchemaType(result.SchemaType)
}

func mergeElementTypes(values []typedValueSQL, common mergeElementCompatibility) []ExpressionType {
	if common.typ == ExpressionTypeUnknown || common.typ == ExpressionTypeNull {
		return nil
	}
	if common.typ != ExpressionTypeArray {
		return []ExpressionType{common.typ}
	}
	for _, value := range values {
		elemTypes := typedValueElementTypes(value)
		if len(elemTypes) > 0 && elemTypes[0] == ExpressionTypeArray {
			return elemTypes
		}
	}
	return []ExpressionType{common.typ}
}

func sameExpressionTypes(left, right []ExpressionType) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func arrayElementTypesName(types []ExpressionType) string {
	if len(types) == 0 {
		return expressionTypeName(ExpressionTypeUnknown)
	}
	parts := make([]string, len(types))
	for i, typ := range types {
		parts[i] = expressionTypeName(typ)
	}
	return strings.Join(parts, " of ")
}

func normalizeSchemaType(typ string) string {
	return strings.ToLower(strings.TrimSpace(typ))
}

func firstSchemaType(types ...string) string {
	for _, typ := range types {
		if normalized := normalizeSchemaType(typ); normalized != "" {
			return normalized
		}
	}
	return ""
}
