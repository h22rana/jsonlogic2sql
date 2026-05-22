package parser

import (
	"fmt"
	"strings"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
)

func compatibleValueType(left, right expressionResult, path string) (operators.ExpressionType, error) {
	leftType := valueTypeOf(left)
	rightType := valueTypeOf(right)
	if leftType == operators.ExpressionTypeNull {
		return rightType, nil
	}
	if rightType == operators.ExpressionTypeNull {
		return leftType, nil
	}
	if leftType == rightType {
		if leftType == operators.ExpressionTypeArray {
			if _, _, err := compatibleArrayElementType(left, right, path); err != nil {
				return operators.ExpressionTypeUnknown, err
			}
		}
		return leftType, nil
	}
	if leftType == operators.ExpressionTypeUnknown || rightType == operators.ExpressionTypeUnknown {
		return operators.ExpressionTypeUnknown, nil
	}
	return operators.ExpressionTypeUnknown, tperrors.NewTypeMismatch("", path,
		"compatible value result types", fmt.Sprintf("%s and %s", typeName(leftType), typeName(rightType)))
}

func compatibleValueResult(left, right expressionResult, path string) (expressionResult, error) {
	typ, err := compatibleValueType(left, right, path)
	if err != nil {
		return expressionResult{}, err
	}
	res := valueResult("", typ)
	if typ != operators.ExpressionTypeArray {
		return res, nil
	}
	elemTypes, known, err := mergedArrayElementTypes(left, right, path)
	if err != nil {
		return expressionResult{}, err
	}
	if known {
		res = withArrayElementTypes(res, elemTypes...)
	}
	return res, nil
}

func mergedArrayElementTypes(left, right expressionResult, path string) ([]operators.ExpressionType, bool, error) {
	if valueTypeOf(left) == operators.ExpressionTypeArray && valueTypeOf(right) == operators.ExpressionTypeArray {
		return compatibleArrayElementTypes(left, right, path)
	}
	if valueTypeOf(left) == operators.ExpressionTypeArray && valueTypeOf(right) == operators.ExpressionTypeNull {
		elemTypes, known := arrayElementTypesOf(left)
		return elemTypes, known, nil
	}
	if valueTypeOf(right) == operators.ExpressionTypeArray && valueTypeOf(left) == operators.ExpressionTypeNull {
		elemTypes, known := arrayElementTypesOf(right)
		return elemTypes, known, nil
	}
	return nil, false, nil
}

func compatibleArrayElementType(left, right expressionResult, path string) (operators.ExpressionType, bool, error) {
	elemTypes, known, err := compatibleArrayElementTypes(left, right, path)
	if !known || err != nil {
		return operators.ExpressionTypeUnknown, known, err
	}
	return elemTypes[0], true, nil
}

func compatibleArrayElementTypes(left, right expressionResult, path string) ([]operators.ExpressionType, bool, error) {
	if valueTypeOf(left) != operators.ExpressionTypeArray || valueTypeOf(right) != operators.ExpressionTypeArray {
		return nil, false, nil
	}
	leftTypes, leftKnown := arrayElementTypesOf(left)
	rightTypes, rightKnown := arrayElementTypesOf(right)
	if leftKnown && rightKnown {
		if sameExpressionTypes(leftTypes, rightTypes) {
			return leftTypes, true, nil
		}
		return nil, false, tperrors.NewTypeMismatch("", path,
			"array value branches must have compatible element types",
			fmt.Sprintf("%s and %s", arrayElementTypesName(leftTypes), arrayElementTypesName(rightTypes)))
	}
	if leftKnown && arrayElementTypeNeutral(right) {
		return leftTypes, true, nil
	}
	if rightKnown && arrayElementTypeNeutral(left) {
		return rightTypes, true, nil
	}
	return nil, false, nil
}

func arrayElementTypesOf(res expressionResult) ([]operators.ExpressionType, bool) {
	if valueTypeOf(res) != operators.ExpressionTypeArray || !res.arrayElementTypeKnown {
		return nil, false
	}
	elemTypes := normalizeExpressionTypes(res.arrayElementTypes)
	if len(elemTypes) == 0 && res.arrayElementType != operators.ExpressionTypeUnknown {
		elemTypes = []operators.ExpressionType{res.arrayElementType}
	}
	if len(elemTypes) == 0 {
		return nil, false
	}
	return elemTypes, true
}

func sameExpressionTypes(left, right []operators.ExpressionType) bool {
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

func arrayElementTypesName(types []operators.ExpressionType) string {
	if len(types) == 0 {
		return typeName(operators.ExpressionTypeUnknown)
	}
	parts := make([]string, len(types))
	for i, typ := range types {
		parts[i] = typeName(typ)
	}
	return strings.Join(parts, " of ")
}

func arrayElementTypeNeutral(res expressionResult) bool {
	if valueTypeOf(res) != operators.ExpressionTypeArray || !res.rawLiteralKnown {
		return false
	}
	arr, ok := res.rawLiteral.([]interface{})
	if !ok {
		return false
	}
	for _, elem := range arr {
		typ, known, _ := literalTypeAndTruth(elem)
		if !known || typ != operators.ExpressionTypeNull {
			return false
		}
	}
	return true
}
