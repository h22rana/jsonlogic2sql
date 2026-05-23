package parser

import (
	"fmt"
	"strings"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
)

func (p *Parser) compatibleValueType(left, right expressionResult, path string) (operators.ExpressionType, error) {
	leftType := valueTypeOf(left)
	rightType := valueTypeOf(right)
	if leftType == operators.ExpressionTypeNull {
		return rightType, nil
	}
	if rightType == operators.ExpressionTypeNull {
		return leftType, nil
	}
	if leftType == rightType {
		switch leftType {
		case operators.ExpressionTypeArray:
			if _, _, err := p.compatibleArrayElementType(left, right, path); err != nil {
				return operators.ExpressionTypeUnknown, err
			}
			if err := p.validateCompatibleObjectArrayScopes(left, right, path); err != nil {
				return operators.ExpressionTypeUnknown, err
			}
		case operators.ExpressionTypeObject:
			if err := p.validateCompatibleObjectValueScopes(left, right, path); err != nil {
				return operators.ExpressionTypeUnknown, err
			}
		case operators.ExpressionTypeUnknown,
			operators.ExpressionTypeNull,
			operators.ExpressionTypeBoolean,
			operators.ExpressionTypeString,
			operators.ExpressionTypeNumber:
		}
		return leftType, nil
	}
	if leftType == operators.ExpressionTypeUnknown || rightType == operators.ExpressionTypeUnknown {
		return operators.ExpressionTypeUnknown, nil
	}
	return operators.ExpressionTypeUnknown, tperrors.NewTypeMismatch("", path,
		"compatible value result types", fmt.Sprintf("%s and %s", typeName(leftType), typeName(rightType)))
}

func (p *Parser) compatibleValueResult(left, right expressionResult, path string) (expressionResult, error) {
	typ, err := p.compatibleValueType(left, right, path)
	if err != nil {
		return expressionResult{}, err
	}
	res := valueResult("", typ)
	if typ != operators.ExpressionTypeArray {
		if typ == operators.ExpressionTypeObject {
			res = withArrayElementSchemaScopes(res, compatibleArrayElementSchemaScopes(left, right)...)
		}
		return res, nil
	}
	elemTypes, known, err := p.mergedArrayElementTypes(left, right, path)
	if err != nil {
		return expressionResult{}, err
	}
	if known {
		res = withArrayElementTypes(res, elemTypes...)
	}
	res = withArrayElementSchemaScopes(res, compatibleArrayElementSchemaScopes(left, right)...)
	return res, nil
}

func (p *Parser) mergedArrayElementTypes(left, right expressionResult, path string) ([]operators.ExpressionType, bool, error) {
	if valueTypeOf(left) == operators.ExpressionTypeArray && valueTypeOf(right) == operators.ExpressionTypeArray {
		return p.compatibleArrayElementTypes(left, right, path)
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

func (p *Parser) compatibleArrayElementType(left, right expressionResult, path string) (operators.ExpressionType, bool, error) {
	elemTypes, known, err := p.compatibleArrayElementTypes(left, right, path)
	if !known || err != nil {
		return operators.ExpressionTypeUnknown, known, err
	}
	return elemTypes[0], true, nil
}

func (p *Parser) compatibleArrayElementTypes(left, right expressionResult, path string) ([]operators.ExpressionType, bool, error) {
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

func (p *Parser) validateCompatibleObjectArrayScopes(left, right expressionResult, path string) error {
	leftTypes, leftKnown := arrayElementTypesOf(left)
	rightTypes, rightKnown := arrayElementTypesOf(right)
	if !leftKnown || !rightKnown ||
		len(leftTypes) == 0 || len(rightTypes) == 0 ||
		!arrayElementTypesContainObject(leftTypes) ||
		!arrayElementTypesContainObject(rightTypes) {
		return nil
	}
	leftScopes := normalizeParserSchemaScopes(left.arrayElementSchemaScopes)
	rightScopes := normalizeParserSchemaScopes(right.arrayElementSchemaScopes)
	return p.validateCompatibleSchemaScopes(leftScopes, rightScopes, path,
		"array value branches must have compatible object element schemas")
}

func (p *Parser) validateCompatibleObjectValueScopes(left, right expressionResult, path string) error {
	leftScopes := normalizeParserSchemaScopes(left.arrayElementSchemaScopes)
	rightScopes := normalizeParserSchemaScopes(right.arrayElementSchemaScopes)
	return p.validateCompatibleSchemaScopes(leftScopes, rightScopes, path,
		"object value branches must have compatible schemas")
}

func (p *Parser) validateCompatibleSchemaScopes(leftScopes, rightScopes []string, path, context string) error {
	if len(leftScopes) == 0 || len(rightScopes) == 0 {
		if len(leftScopes) != len(rightScopes) {
			return tperrors.NewTypeMismatch("", path, context, unknownObjectSchemaDetail(context))
		}
		return nil
	}
	comparator, ok := p.config.Schema.(operators.ArrayElementSchemaComparator)
	if !ok {
		if sameStringSets(leftScopes, rightScopes) {
			return nil
		}
		return tperrors.NewTypeMismatch("", path, context,
			fmt.Sprintf("%s and %s", strings.Join(leftScopes, ","), strings.Join(rightScopes, ",")))
	}
	for _, leftScope := range leftScopes {
		for _, rightScope := range rightScopes {
			if err := comparator.ValidateArrayElementSchemasCompatible(leftScope, rightScope); err != nil {
				return tperrors.NewTypeMismatch("", path, context, err.Error())
			}
		}
	}
	return nil
}

func unknownObjectSchemaDetail(context string) string {
	if strings.Contains(context, "object element schemas") {
		return "known object element schema and unknown object element schema"
	}
	return "known object schema and unknown object schema"
}

func (p *Parser) validateCompatibleObjectArrayScopesForResult(res expressionResult, path string) error {
	elemTypes, known := arrayElementTypesOf(res)
	if !known || len(elemTypes) == 0 || !arrayElementTypesContainObject(elemTypes) {
		return nil
	}
	scopes := normalizeParserSchemaScopes(res.arrayElementSchemaScopes)
	if len(scopes) <= 1 {
		return nil
	}
	comparator, ok := p.config.Schema.(operators.ArrayElementSchemaComparator)
	if !ok {
		return tperrors.NewTypeMismatch("", path,
			"array value result must have compatible object element schemas",
			strings.Join(scopes, ","))
	}
	for _, scope := range scopes[1:] {
		if err := comparator.ValidateArrayElementSchemasCompatible(scopes[0], scope); err != nil {
			return tperrors.NewTypeMismatch("", path,
				"array value result must have compatible object element schemas",
				err.Error())
		}
	}
	return nil
}

func arrayElementTypesContainObject(types []operators.ExpressionType) bool {
	for _, typ := range types {
		if typ == operators.ExpressionTypeObject {
			return true
		}
	}
	return false
}

func compatibleArrayElementSchemaScopes(left, right expressionResult) []string {
	scopes := append([]string{}, left.arrayElementSchemaScopes...)
	scopes = append(scopes, right.arrayElementSchemaScopes...)
	return normalizeParserSchemaScopes(scopes)
}

func sameStringSets(left, right []string) bool {
	left = normalizeParserSchemaScopes(left)
	right = normalizeParserSchemaScopes(right)
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
	if res.Kind == operators.ExpressionKindValue &&
		valueTypeOf(res) == operators.ExpressionTypeArray &&
		res.EmptyArrayLiteral {
		return true
	}
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
