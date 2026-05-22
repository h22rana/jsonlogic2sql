package operators

import (
	"fmt"
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
		pv.Kind == ExpressionKindValue && pv.Type == ExpressionTypeArray {
		return fmt.Errorf("ordering comparison '%s' on array-valued expression is not supported", operator)
	}

	return nil
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
	return operator == "==" || operator == "===" || operator == "!=" || operator == "!=="
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
	if operator != "==" && operator != "!=" {
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

	if operator == "===" {
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
	case "==", "===":
		return fmt.Sprintf("((%s IS NULL AND %s IS NULL) OR (%s IS NOT NULL AND %s IS NOT NULL AND %s = %s))",
			leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL)
	case "!=":
		return fmt.Sprintf("((%s IS NULL AND %s IS NOT NULL) OR (%s IS NOT NULL AND %s IS NULL) OR (%s IS NOT NULL AND %s IS NOT NULL AND %s != %s))",
			leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL)
	case "!==":
		return fmt.Sprintf("((%s IS NULL AND %s IS NOT NULL) OR (%s IS NOT NULL AND %s IS NULL) OR (%s IS NOT NULL AND %s IS NOT NULL AND %s <> %s))",
			leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL, leftSQL, rightSQL)
	default:
		return ""
	}
}

func isStrictEqualityOperator(operator string) bool {
	return operator == "===" || operator == "!=="
}

func impossibleEqualityPredicateConstant(operator string) *bool {
	result := operator == "!=" || operator == "!=="

	return &result
}

func equalityPredicateConstant(operator string, left, right bool) bool {
	switch operator {
	case "==", "===":
		return left == right
	case "!=", "!==":
		return left != right
	default:
		return false
	}
}
