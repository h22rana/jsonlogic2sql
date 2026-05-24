package operators

import (
	"fmt"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

// validateOrderingOperand checks if a field used in an ordering comparison is of a valid type
// Only numeric and string types support ordering comparisons (>, >=, <, <=)
// Rejects array, object, and boolean types.
func (c *ComparisonOperator) validateOrderingOperand(value interface{}, operator string) error {
	fieldName := c.extractFieldNameFromValue(value)
	if fieldName != "" {
		fieldType := c.schema().GetFieldType(fieldName)
		if fieldType == "" {
			return nil // Field not in schema, skip validation (existence checked by DataOperator)
		}

		// Allow numeric and string types for ordering comparisons
		if c.schema().IsNumericType(fieldName) || c.schema().IsStringType(fieldName) {
			return nil
		}

		// Disallow array, object, boolean for ordering comparisons
		return fmt.Errorf("ordering comparison '%s' on incompatible field '%s' (type: %s)", operator, fieldName, fieldType)
	}

	if pv, ok := value.(ProcessedValue); ok && pv.IsSQL && pv.HasExpressionInfo &&
		pv.Kind == ExpressionKindValue &&
		(pv.Type == ExpressionTypeArray || pv.Type == ExpressionTypeObject) {
		return fmt.Errorf("ordering comparison '%s' on %s-valued expression is not supported", operator, expressionTypeName(pv.Type))
	}

	return nil
}

func (c *ComparisonOperator) validateOrderingOperandsCompatible(left, right interface{}, operator string) error {
	leftKind, leftKnown, err := c.orderingOperandKind(left, operator)
	if err != nil {
		return err
	}
	rightKind, rightKnown, err := c.orderingOperandKind(right, operator)
	if err != nil {
		return err
	}
	if !leftKnown || !rightKnown || leftKind == rightKind {
		return nil
	}
	return fmt.Errorf(
		"ordering comparison '%s' between incompatible operand types %s and %s is not supported",
		operator,
		expressionTypeName(leftKind),
		expressionTypeName(rightKind),
	)
}

func (c *ComparisonOperator) orderingOperandKind(value interface{}, operator string) (ExpressionType, bool, error) {
	if err := c.validateOrderingOperand(value, operator); err != nil {
		return ExpressionTypeUnknown, false, err
	}
	if fieldName := c.extractFieldNameFromValue(value); fieldName != "" {
		switch {
		case c.schema().IsNumericType(fieldName):
			return ExpressionTypeNumber, true, nil
		case c.schema().IsStringType(fieldName), c.schema().IsEnumType(fieldName):
			return ExpressionTypeString, true, nil
		case c.schema().IsBooleanType(fieldName):
			return ExpressionTypeBoolean, true, nil
		case c.schema().IsArrayType(fieldName):
			return ExpressionTypeArray, true, nil
		case c.schema().GetFieldType(fieldName) == objectFieldType:
			return ExpressionTypeObject, true, nil
		default:
			return ExpressionTypeUnknown, false, nil
		}
	}
	if pv, ok := value.(ProcessedValue); ok {
		if pv.HasExpressionInfo {
			if pv.Kind == ExpressionKindPredicate {
				return ExpressionTypeNumber, true, nil
			}
			switch pv.Type {
			case ExpressionTypeBoolean:
				return ExpressionTypeNumber, true, nil
			case ExpressionTypeNumber, ExpressionTypeString, ExpressionTypeArray, ExpressionTypeObject:
				return pv.Type, true, nil
			case ExpressionTypeNull, ExpressionTypeUnknown:
				return ExpressionTypeUnknown, false, nil
			default:
				return ExpressionTypeUnknown, false, nil
			}
		}
		if !pv.IsSQL {
			return literalOrderingKind(pv.Value)
		}
		return ExpressionTypeUnknown, false, nil
	}
	return literalOrderingKind(value)
}

func literalOrderingKind(value interface{}) (ExpressionType, bool, error) {
	typ := inferLiteralValueExpressionType(value)
	switch typ {
	case ExpressionTypeBoolean:
		return ExpressionTypeNumber, true, nil
	case ExpressionTypeNumber, ExpressionTypeString:
		return typ, true, nil
	case ExpressionTypeNull, ExpressionTypeUnknown:
		return ExpressionTypeUnknown, false, nil
	case ExpressionTypeArray, ExpressionTypeObject:
		return typ, true, nil
	default:
		return ExpressionTypeUnknown, false, nil
	}
}

// extractFieldNameFromValue extracts field name from a value that might be a var expression.
func (c *ComparisonOperator) extractFieldNameFromValue(value interface{}) string {
	if pv, ok := value.(ProcessedValue); ok && pv.IsSQL && pv.IsField {
		return pv.FieldName
	}
	if varExpr, ok := value.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			return c.extractFieldName(varName)
		}
	}
	return ""
}

func (c *ComparisonOperator) isKnownArrayOperand(value interface{}) bool {
	if pv, ok := value.(ProcessedValue); ok && pv.IsSQL && pv.HasExpressionInfo &&
		pv.Kind == ExpressionKindValue && pv.Type == ExpressionTypeArray {
		return true
	}
	fieldName := c.extractFieldNameFromValue(value)
	return fieldName != "" && c.schema().IsArrayType(fieldName)
}

func (c *ComparisonOperator) fieldHasObjectArrayElements(fieldName string) bool {
	provider, ok := c.schema().(ArrayElementSchemaProvider)
	return ok && provider.HasArrayElementFields(fieldName)
}

func (c *ComparisonOperator) arrayElementEqualityKind(fieldName string) (string, bool) {
	if fieldName == "" {
		return "", false
	}
	if elemType := normalizeSchemaType(schemaArrayElementType(c.schema(), fieldName)); elemType != "" {
		return elemType, true
	}
	elemType, known := schemaArrayElementExpressionType(c.schema(), fieldName)
	if !known {
		return "", false
	}
	switch elemType {
	case ExpressionTypeString:
		return literalKindString, true
	case ExpressionTypeNumber:
		return literalKindNumber, true
	case ExpressionTypeBoolean:
		return literalKindBoolean, true
	case ExpressionTypeArray:
		return literalKindArray, true
	case ExpressionTypeObject:
		return objectFieldType, true
	case ExpressionTypeUnknown, ExpressionTypeNull:
		return "", false
	}
	return "", false
}

func (c *ComparisonOperator) arrayElementMembershipKind(fieldName string) (string, bool) {
	elemKind, elemKnown := c.arrayElementEqualityKind(fieldName)
	if !elemKnown {
		return "", false
	}
	switch elemKind {
	case SchemaTypeInteger:
		return literalKindNumber, true
	case SchemaTypeEnum:
		return literalKindString, true
	default:
		return elemKind, true
	}
}

func (c *ComparisonOperator) validateArrayMembershipNeedle(fieldName string, needle interface{}) (bool, error) {
	elemKind, elemKnown := c.arrayElementMembershipKind(fieldName)
	if !elemKnown {
		return true, nil
	}
	needleKinds, needleKnown := c.strictArrayMembershipLeftKinds(needle)
	if !needleKnown {
		return true, nil
	}
	if _, ok := needleKinds[literalKindNull]; ok {
		if len(needleKinds) == 1 {
			return true, nil
		}
	}
	if _, ok := needleKinds[elemKind]; !ok {
		return false, nil
	}
	if schemaArrayElementType(c.schema(), fieldName) == SchemaTypeInteger {
		return integerArrayMembershipNeedleCompatible(needle)
	}
	if literal, ok := equalityLiteralValue(needle); ok && equalityLiteralKind(literal) == SchemaTypeString {
		str, ok := literal.(string)
		if !ok {
			return true, nil
		}
		if err := validateSchemaEnumArrayElementValue(c.schema(), fieldName, str); err != nil {
			return false, err
		}
		return true, nil
	}
	if str, ok := staticStringExpressionValue(needle); ok {
		if err := validateSchemaEnumArrayElementValue(c.schema(), fieldName, str); err != nil {
			return false, err
		}
	}
	return true, nil
}

func integerArrayMembershipNeedleCompatible(needle interface{}) (bool, error) {
	literal, ok := equalityLiteralValue(needle)
	if !ok || equalityLiteralKind(literal) != literalKindNumber {
		return true, nil
	}
	if err := validateEqualityJSONNumberLiteral(literal); err != nil {
		return false, err
	}
	n, handled, valid := jsNumberFromLiteral(literal)
	if !handled {
		return true, nil
	}
	if !valid || !n.integral {
		return false, nil
	}
	_, ok = int64FromJSNumber(n)
	return ok, nil
}

func (c *ComparisonOperator) validateFieldEqualityArrayCompatibility(
	leftField equalityFieldOperand,
	rightField equalityFieldOperand,
) error {
	if leftField.fieldName == "" || rightField.fieldName == "" ||
		!c.schema().IsArrayType(leftField.fieldName) ||
		!c.schema().IsArrayType(rightField.fieldName) {
		return nil
	}
	leftElemKind, leftElemKnown := c.arrayElementEqualityKind(leftField.fieldName)
	rightElemKind, rightElemKnown := c.arrayElementEqualityKind(rightField.fieldName)
	if leftElemKnown && rightElemKnown && leftElemKind != rightElemKind {
		return fmt.Errorf(
			"equality between array fields %q and %q has incompatible array element types %s and %s",
			leftField.fieldName,
			rightField.fieldName,
			leftElemKind,
			rightElemKind,
		)
	}
	if c.config.GetDialect() == dialect.DialectBigQuery {
		return fmt.Errorf(
			"equality between array fields %q and %q is not supported for BigQuery",
			leftField.fieldName,
			rightField.fieldName,
		)
	}
	leftObject := c.fieldHasObjectArrayElements(leftField.fieldName)
	rightObject := c.fieldHasObjectArrayElements(rightField.fieldName)
	if !leftObject && !rightObject {
		return nil
	}
	if leftField.fieldName == rightField.fieldName {
		return nil
	}
	if !leftObject || !rightObject {
		return fmt.Errorf(
			"equality between array field %q and array field %q has incompatible element schemas",
			leftField.fieldName,
			rightField.fieldName,
		)
	}
	comparator, ok := c.schema().(ArrayElementSchemaComparator)
	if !ok {
		return fmt.Errorf(
			"equality between object-array fields %q and %q requires comparable element schemas",
			leftField.fieldName,
			rightField.fieldName,
		)
	}
	if err := comparator.ValidateArrayElementSchemasCompatible(leftField.fieldName, rightField.fieldName); err != nil {
		return fmt.Errorf("equality between object-array fields %q and %q is not supported: %w",
			leftField.fieldName,
			rightField.fieldName,
			err)
	}
	return nil
}

func (c *ComparisonOperator) objectArrayHaystackMembershipSQL(
	leftOriginal interface{},
	right ProcessedValue,
) (string, bool, error) {
	if right.FieldName != "" {
		if !c.fieldHasObjectArrayElements(right.FieldName) {
			return "", false, nil
		}
	} else if !processedArrayHasObjectElements(right) {
		return "", false, nil
	}
	kind, known := c.membershipNeedleEqualityKind(leftOriginal)
	if known && kind != objectFieldType {
		return boolSQL(false), true, nil
	}
	return "", true, fmt.Errorf("in operator against object-array values is not supported for object or unknown needles")
}

func processedArrayHasObjectElements(value ProcessedValue) bool {
	if !value.IsSQL || !value.HasExpressionInfo ||
		value.Kind != ExpressionKindValue || value.Type != ExpressionTypeArray {
		return false
	}
	if len(value.ArrayElementTypes) > 0 {
		return value.ArrayElementTypes[0] == ExpressionTypeObject
	}
	return value.ArrayElementType == ExpressionTypeObject
}

func (c *ComparisonOperator) membershipNeedleEqualityKind(value interface{}) (string, bool) {
	if field, ok := c.extractEqualityFieldOperand(value); ok {
		return c.schemaEqualityKind(field.fieldName)
	}
	if kind, ok := expressionEqualityKind(value); ok {
		return kind, true
	}
	literal, ok := equalityLiteralValue(value)
	if !ok {
		return "", false
	}
	kind := equalityLiteralKind(literal)
	return kind, kind != ""
}

func staticStringExpressionValue(value interface{}) (string, bool) {
	pv, ok := value.(ProcessedValue)
	if !ok || !pv.IsSQL || !pv.HasExpressionInfo ||
		pv.Kind != ExpressionKindValue || pv.Type != ExpressionTypeString {
		return "", false
	}
	return staticSQLStringValue(pv.Value)
}

func staticSQLStringValue(sql string) (string, bool) {
	sql = StripRedundantOuterParens(sql)
	if !isSQLStringLiteral(sql) {
		return "", false
	}
	unquoted := sql[1 : len(sql)-1]
	return strings.ReplaceAll(unquoted, "''", "'"), true
}

// extractFieldName extracts the field name from a var argument.
func (c *ComparisonOperator) extractFieldName(varName interface{}) string {
	if pv, ok := varName.(ProcessedValue); ok && pv.IsSQL && pv.IsField {
		return pv.FieldName
	}
	if nameStr, ok := varName.(string); ok {
		return nameStr
	}
	if nameArr, ok := varName.([]interface{}); ok && len(nameArr) > 0 {
		if pv, ok := nameArr[0].(ProcessedValue); ok && pv.IsSQL && pv.IsField {
			return pv.FieldName
		}
		if nameStr, ok := nameArr[0].(string); ok {
			return nameStr
		}
	}
	return ""
}

func (c *ComparisonOperator) extractEqualityFieldOperand(value interface{}) (equalityFieldOperand, bool) {
	if pv, ok := value.(ProcessedValue); ok && pv.IsSQL && pv.IsField {
		return equalityFieldOperandFromProcessedValue(pv), true
	}
	varExpr, ok := value.(map[string]interface{})
	if !ok {
		return equalityFieldOperand{}, false
	}
	if len(varExpr) != 1 {
		return equalityFieldOperand{}, false
	}
	varName, hasVar := varExpr[OpVar]
	if !hasVar {
		return equalityFieldOperand{}, false
	}
	switch v := varName.(type) {
	case string:
		return equalityFieldOperand{fieldName: v}, true
	case []interface{}:
		if len(v) == 0 {
			return equalityFieldOperand{}, false
		}
		if name, ok := v[0].(string); ok {
			operand := equalityFieldOperand{fieldName: name}
			if len(v) > 1 {
				setEqualityFieldDefault(&operand, v[1])
			}
			return operand, true
		}
		if pv, ok := v[0].(ProcessedValue); ok && pv.IsSQL && pv.IsField {
			operand := equalityFieldOperandFromProcessedValue(pv)
			if len(v) > 1 {
				setEqualityFieldDefault(&operand, v[1])
			}
			return operand, true
		}
	}
	return equalityFieldOperand{}, false
}

func equalityFieldOperandFromProcessedValue(pv ProcessedValue) equalityFieldOperand {
	operand := equalityFieldOperand{fieldName: pv.FieldName, processed: true}
	if pv.FieldHasDefault {
		operand.hasDefault = true
		if pv.FieldDefaultLiteralKnown {
			operand.defaultLiteral = pv.FieldDefaultLiteral
			operand.defaultLiteralKnown = true
		}
	}
	return operand
}

func setEqualityFieldDefault(operand *equalityFieldOperand, defaultValue interface{}) {
	operand.hasDefault = true
	if literal, ok := equalityLiteralValue(defaultValue); ok {
		operand.defaultLiteral = literal
		operand.defaultLiteralKnown = true
	}
}

func isEqualityOperator(operator string) bool {
	switch operator {
	case OpEqual, OpStrictEqual, OpNotEqual, OpStrictNotEqual:
		return true
	default:
		return false
	}
}

func (c *ComparisonOperator) shouldUseNullSafeFieldEquality(operator string, leftArg, rightArg interface{}) bool {
	if !isEqualityOperator(operator) {
		return false
	}
	return c.isNullSafeFieldOperand(leftArg) && c.isNullSafeFieldOperand(rightArg)
}

func (c *ComparisonOperator) looseIncompatibleFieldEqualityError(
	operator string,
	leftField, rightField equalityFieldOperand,
) error {
	if operator != OpEqual && operator != OpNotEqual {
		return nil
	}
	leftKind, leftKnown := c.schemaEqualityKind(leftField.fieldName)
	rightKind, rightKnown := c.schemaEqualityKind(rightField.fieldName)
	if !leftKnown || !rightKnown || leftKind == rightKind {
		return nil
	}
	return fmt.Errorf(
		"loose equality between %s field %q and %s field %q is not supported",
		leftKind,
		leftField.fieldName,
		rightKind,
		rightField.fieldName,
	)
}

func (c *ComparisonOperator) strictIncompatibleFieldEqualitySQL(
	operator string,
	leftArg, rightArg interface{},
	leftSQL, rightSQL string,
) (string, bool) {
	if !c.hasStrictIncompatibleFieldEqualityOperands(operator, leftArg, rightArg) {
		return "", false
	}

	if operator == OpStrictEqual {
		return fmt.Sprintf("(%s IS NULL AND %s IS NULL)", leftSQL, rightSQL), true
	}
	return fmt.Sprintf("(%s IS NOT NULL OR %s IS NOT NULL)", leftSQL, rightSQL), true
}

func (c *ComparisonOperator) hasStrictIncompatibleFieldEqualityOperands(operator string, leftArg, rightArg interface{}) bool {
	if !isStrictEqualityOperator(operator) {
		return false
	}

	leftField, leftOK := c.extractEqualityFieldOperand(leftArg)
	rightField, rightOK := c.extractEqualityFieldOperand(rightArg)
	if !leftOK || !rightOK || leftField.hasDefault || rightField.hasDefault {
		return false
	}

	leftKind, leftKnown := c.schemaEqualityKind(leftField.fieldName)
	rightKind, rightKnown := c.schemaEqualityKind(rightField.fieldName)
	return leftKnown && rightKnown && leftKind != rightKind
}

func (c *ComparisonOperator) isNullSafeFieldOperand(value interface{}) bool {
	if _, ok := c.extractEqualityFieldOperand(value); ok {
		return true
	}
	return isProcessedSQLFieldOperand(value)
}

func isProcessedSQLFieldOperand(value interface{}) bool {
	if pv, ok := value.(ProcessedValue); ok {
		return pv.IsSQL && pv.IsField
	}
	return false
}

func nullSafeFieldEqualitySQL(operator, leftSQL, rightSQL string) string {
	switch operator {
	case OpEqual, OpStrictEqual:
		return fmt.Sprintf("((%s IS NULL AND %s IS NULL) OR (%s IS NOT NULL AND %s IS NOT NULL AND %s = %s))",
			leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL)
	case OpNotEqual:
		return fmt.Sprintf("((%s IS NULL AND %s IS NOT NULL) OR (%s IS NOT NULL AND %s IS NULL) OR (%s IS NOT NULL AND %s IS NOT NULL AND %s != %s))",
			leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL)
	case OpStrictNotEqual:
		return fmt.Sprintf("((%s IS NULL AND %s IS NOT NULL) OR (%s IS NOT NULL AND %s IS NULL) OR (%s IS NOT NULL AND %s IS NOT NULL AND %s <> %s))",
			leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL)
	default:
		return ""
	}
}

func isStrictEqualityOperator(operator string) bool {
	return operator == OpStrictEqual || operator == OpStrictNotEqual
}

func impossibleEqualityPredicateConstant(operator string) *bool {
	result := operator == OpNotEqual || operator == OpStrictNotEqual

	return &result
}

func equalityPredicateConstant(operator string, left, right bool) bool {
	switch operator {
	case OpEqual, OpStrictEqual:
		return left == right
	case OpNotEqual, OpStrictNotEqual:
		return left != right
	default:
		return false
	}
}
