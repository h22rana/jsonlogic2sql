package operators

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func (c *ComparisonOperator) fieldEqualityKind(fieldName string) string {
	if fieldName == "" {
		return ""
	}
	switch {
	case c.schema().IsNumericType(fieldName):
		return "number"
	case c.schema().IsBooleanType(fieldName):
		return "boolean"
	case c.schema().IsStringType(fieldName), c.schema().IsEnumType(fieldName):
		return "string"
	default:
		return ""
	}
}

func (c *ComparisonOperator) setLiteralForFieldSide(dec *equalityDecision, fieldOnLeft bool, value interface{}) {
	if fieldOnLeft {
		dec.right = value
		return
	}
	dec.left = value
}

func (c *ComparisonOperator) validateEqualityFieldOperand(field equalityFieldOperand) error {
	if field.fieldName == "" {
		return nil
	}
	if !field.processed {
		if err := c.schema().ValidateField(field.fieldName); err != nil {
			return err
		}
	} else if !c.schema().HasField(field.fieldName) {
		return fmt.Errorf("field '%s' is not defined in schema", field.fieldName)
	}
	if !field.hasDefault || !field.defaultLiteralKnown {
		return nil
	}
	if err := validateEqualityJSONNumberLiteral(field.defaultLiteral); err != nil {
		return err
	}
	if c.schema().IsEnumType(field.fieldName) {
		return c.validateEnumValue(field.defaultLiteral, field.fieldName)
	}
	return nil
}

func (c *ComparisonOperator) applyEqualitySemantics(operator string, leftArg, rightArg interface{}) equalityDecision {
	dec := equalityDecision{left: leftArg, right: rightArg}
	if !isEqualityOperator(operator) {
		return dec
	}

	leftField, leftIsField := c.extractEqualityFieldOperand(leftArg)
	rightField, rightIsField := c.extractEqualityFieldOperand(rightArg)
	if leftIsField {
		if err := c.validateEqualityFieldOperand(leftField); err != nil {
			dec.unsupported = err
			dec.handled = true
			return dec
		}
	}
	if rightIsField {
		if err := c.validateEqualityFieldOperand(rightField); err != nil {
			dec.unsupported = err
			dec.handled = true
			return dec
		}
	}
	if leftIsField && rightIsField {
		if err := c.looseIncompatibleFieldEqualityError(operator, leftField, rightField); err != nil {
			dec.unsupported = err
			dec.handled = true
			return dec
		}
		if c.hasStrictIncompatibleFieldEqualityOperands(operator, leftArg, rightArg) {
			return dec
		}
	}
	if leftIsField == rightIsField {
		return c.applyTypedExpressionEqualitySemantics(dec, operator, leftArg, rightArg)
	}

	field := leftField
	literalArg := rightArg
	fieldOnLeft := true
	if !leftIsField {
		field = rightField
		literalArg = leftArg
		fieldOnLeft = false
	}
	if typedNullExpression(literalArg) {
		c.setLiteralForFieldSide(&dec, fieldOnLeft, nil)
		literalArg = nil
	}
	fieldName := field.fieldName

	fieldKind := c.fieldEqualityKind(fieldName)
	if fieldKind == "" {
		return c.applyTypedExpressionEqualitySemantics(dec, operator, leftArg, rightArg)
	}

	if isStrictEqualityOperator(operator) {
		if exprKind, exprTyped := expressionEqualityKind(literalArg); exprTyped && exprKind != fieldKind {
			if field.defaultCanHaveStrictKind(exprKind) {
				return dec
			}
			dec.handled = true
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
	}

	literal, ok := equalityLiteralValue(literalArg)
	if !ok {
		return dec
	}

	dec.handled = true

	if err := validateEqualityJSONNumberLiteral(literal); err != nil {
		dec.unsupported = err
		return dec
	}

	if literal == nil {
		return dec
	}

	if isStrictEqualityOperator(operator) {
		literalKind := equalityLiteralKind(literal)
		if literalKind != "" && literalKind != fieldKind {
			if field.hasDefault && field.defaultCanStrictEqual(literal) {
				return dec
			}
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
		if fieldKind == "number" {
			if _, isJSONNumber := literal.(json.Number); !isJSONNumber {
				if _, handled, valid := jsNumberFromLiteral(literal); handled && !valid {
					dec.constant = impossibleEqualityPredicateConstant(operator)
					return dec
				}
			}
		}
		if fieldKind == "number" && c.schema().GetFieldType(fieldName) == "integer" {
			if jsonNumberIntegerOutsideInt64(literal) {
				if field.hasDefault && field.defaultCanStrictEqual(literal) {
					return dec
				}
				dec.constant = impossibleEqualityPredicateConstant(operator)
				return dec
			}
			if n, handled, valid := jsNumberFromLiteral(literal); handled {
				if !valid || !n.integral {
					if field.hasDefault && field.defaultCanStrictEqual(literal) {
						return dec
					}
					dec.constant = impossibleEqualityPredicateConstant(operator)
					return dec
				}
				if _, ok := int64FromJSNumber(n); !ok {
					if field.hasDefault && field.defaultCanStrictEqual(literal) {
						return dec
					}
					dec.constant = impossibleEqualityPredicateConstant(operator)
					return dec
				}
			}
		}
		if err := c.validateEnumValue(literal, fieldName); err != nil {
			dec.unsupported = err
		}
		return dec
	}

	switch fieldKind {
	case "number":
		if c.schema().GetFieldType(fieldName) == "number" {
			if _, ok := literal.(json.Number); ok {
				return dec
			}
		}
		n, handled, valid := jsNumberFromLiteral(literal)
		if !handled {
			return dec
		}
		if !valid {
			if field.hasDefault && field.defaultCanLooseEqual(literal) {
				return dec
			}
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
		value := n.value
		if c.schema().GetFieldType(fieldName) == "integer" {
			if jsonNumberIntegerOutsideInt64(literal) {
				if field.hasDefault && field.defaultCanLooseEqual(literal) {
					return dec
				}
				dec.constant = impossibleEqualityPredicateConstant(operator)
				return dec
			}
			if !n.integral {
				if field.hasDefault && field.defaultCanLooseEqual(literal) {
					return dec
				}
				dec.constant = impossibleEqualityPredicateConstant(operator)
				return dec
			}
			i, ok := int64FromJSNumber(n)
			if !ok {
				if field.hasDefault && field.defaultCanLooseEqual(literal) {
					return dec
				}
				dec.constant = impossibleEqualityPredicateConstant(operator)
				return dec
			}
			value = i
		}
		c.setLiteralForFieldSide(&dec, fieldOnLeft, value)
	case "boolean":
		n, handled, valid := jsNumberFromLiteral(literal)
		if !handled {
			return dec
		}
		if !valid {
			if field.hasDefault && field.defaultCanLooseEqual(literal) {
				return dec
			}
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
		switch n.float {
		case 0:
			c.setLiteralForFieldSide(&dec, fieldOnLeft, false)
		case 1:
			c.setLiteralForFieldSide(&dec, fieldOnLeft, true)
		default:
			if field.hasDefault && field.defaultCanLooseEqual(literal) {
				return dec
			}
			dec.constant = impossibleEqualityPredicateConstant(operator)
		}
	case "string":
		if _, ok := literal.(bool); ok {
			dec.unsupported = fmt.Errorf(
				"loose equality between string field %q and boolean literal is not supported", fieldName)
			return dec
		}
		if equalityLiteralKind(literal) == "number" {
			if canonical, handled, possible := stringFieldNumericLiteralString(literal); handled {
				if !possible {
					dec.constant = impossibleEqualityPredicateConstant(operator)
					return dec
				}
				c.setLiteralForFieldSide(&dec, fieldOnLeft, canonical)
			}
		}
		if err := c.validateEnumValue(equalityLiteralForFieldSide(dec, fieldOnLeft), fieldName); err != nil {
			dec.unsupported = err
		}
	}

	return dec
}

func (c *ComparisonOperator) applyTypedExpressionEqualitySemantics(dec equalityDecision, operator string, leftArg, rightArg interface{}) equalityDecision {
	leftIsNull := typedNullExpression(leftArg)
	rightIsNull := typedNullExpression(rightArg)
	if leftIsNull || rightIsNull {
		dec.handled = true
		if leftIsNull {
			dec.left = nil
		}
		if rightIsNull {
			dec.right = nil
		}
		return dec
	}

	leftKind, leftTyped := expressionEqualityKind(leftArg)
	rightKind, rightTyped := expressionEqualityKind(rightArg)
	if leftTyped && rightTyped {
		if isStrictEqualityOperator(operator) && leftKind != rightKind {
			dec.handled = true
			dec.constant = impossibleEqualityPredicateConstant(operator)
		}
		return dec
	}
	if leftTyped == rightTyped {
		return dec
	}

	exprKind := leftKind
	literalArg := rightArg
	exprOnLeft := true
	if !leftTyped {
		exprKind = rightKind
		literalArg = leftArg
		exprOnLeft = false
	}

	literal, ok := equalityLiteralValue(literalArg)
	if !ok {
		return dec
	}
	if err := validateEqualityJSONNumberLiteral(literal); err != nil {
		dec.handled = true
		dec.unsupported = err
		return dec
	}
	if literal == nil {
		return dec
	}

	if isStrictEqualityOperator(operator) {
		literalKind := equalityLiteralKind(literal)
		if literalKind != "" && literalKind != exprKind {
			dec.handled = true
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
		if exprKind == "number" {
			if _, isJSONNumber := literal.(json.Number); !isJSONNumber {
				if _, handled, valid := jsNumberFromLiteral(literal); handled && !valid {
					dec.handled = true
					dec.constant = impossibleEqualityPredicateConstant(operator)
					return dec
				}
			}
		}
		return dec
	}

	switch exprKind {
	case "number":
		n, handled, valid := jsNumberFromLiteral(literal)
		if !handled {
			return dec
		}
		dec.handled = true
		if !valid {
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
		c.setLiteralForFieldSide(&dec, exprOnLeft, n.value)
	case "boolean":
		n, handled, valid := jsNumberFromLiteral(literal)
		if !handled {
			return dec
		}
		dec.handled = true
		if !valid {
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
		switch n.float {
		case 0:
			c.setLiteralForFieldSide(&dec, exprOnLeft, false)
		case 1:
			c.setLiteralForFieldSide(&dec, exprOnLeft, true)
		default:
			dec.constant = impossibleEqualityPredicateConstant(operator)
		}
	case "string":
		dec.handled = true
		if _, ok := literal.(bool); ok {
			dec.unsupported = fmt.Errorf("loose equality between string expression and boolean literal is not supported")
			return dec
		}
		if equalityLiteralKind(literal) == "number" {
			if canonical, handled, possible := stringFieldNumericLiteralString(literal); handled {
				if !possible {
					dec.constant = impossibleEqualityPredicateConstant(operator)
					return dec
				}
				c.setLiteralForFieldSide(&dec, exprOnLeft, canonical)
			}
		}
	}

	return dec
}

func expressionEqualityKind(value interface{}) (string, bool) {
	pv, ok := value.(ProcessedValue)
	if !ok || !pv.IsSQL || !pv.HasExpressionInfo {
		return "", false
	}
	if pv.Kind == ExpressionKindPredicate {
		return "boolean", true
	}
	switch pv.Type {
	case ExpressionTypeNull:
		return "null", true
	case ExpressionTypeBoolean:
		return "boolean", true
	case ExpressionTypeString:
		return "string", true
	case ExpressionTypeNumber:
		return "number", true
	case ExpressionTypeArray:
		return "array", true
	case ExpressionTypeUnknown:
		return "", false
	}
	return "", false
}

func (c *ComparisonOperator) schemaEqualityKind(fieldName string) (string, bool) {
	switch {
	case c.schema().IsStringType(fieldName), c.schema().IsEnumType(fieldName):
		return "string", true
	case c.schema().IsNumericType(fieldName):
		return "number", true
	case c.schema().IsBooleanType(fieldName):
		return "boolean", true
	case c.schema().IsArrayType(fieldName):
		return "array", true
	case c.schema().GetFieldType(fieldName) == "object":
		return "object", true
	default:
		return "", false
	}
}

func (c *ComparisonOperator) strictArrayMembershipLeftKinds(leftOriginal interface{}) (map[string]struct{}, bool) {
	if field, ok := c.extractEqualityFieldOperand(leftOriginal); ok {
		fieldKind, ok := c.schemaEqualityKind(field.fieldName)
		if !ok {
			return nil, false
		}
		kinds := map[string]struct{}{fieldKind: {}}
		if field.hasDefault {
			if !field.defaultLiteralKnown {
				return nil, false
			}
			defaultKind := equalityLiteralKind(field.defaultLiteral)
			if defaultKind == "" {
				return nil, false
			}
			kinds[defaultKind] = struct{}{}
		}
		return kinds, true
	}
	if kind, ok := expressionEqualityKind(leftOriginal); ok {
		return map[string]struct{}{kind: {}}, true
	}
	leftLiteral, ok := equalityLiteralValue(leftOriginal)
	if !ok {
		return nil, false
	}
	if kind := equalityLiteralKind(leftLiteral); kind != "" {
		return map[string]struct{}{kind: {}}, true
	}
	return nil, false
}

func (c *ComparisonOperator) strictArrayMembershipItems(
	leftOriginal interface{},
	items []interface{},
) []interface{} {
	leftKinds, known := c.strictArrayMembershipLeftKinds(leftOriginal)
	if !known {
		return items
	}

	filtered := make([]interface{}, 0, len(items))
	for _, item := range items {
		literal, ok := equalityLiteralValue(item)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		itemKind := equalityLiteralKind(literal)
		if _, ok := leftKinds[itemKind]; itemKind == "null" || itemKind == "" || ok {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func (c *ComparisonOperator) defaultedFieldArrayLiteralMembershipSQL(
	leftOriginal interface{},
	items []interface{},
) (string, bool, error) {
	field, ok := c.defaultedFieldArrayMembershipOperand(leftOriginal)
	if !ok {
		return "", false, nil
	}
	fieldSQL, err := c.fieldSQLForDefaultedArrayMembership(leftOriginal, field)
	if err != nil {
		return "", true, err
	}
	fieldItems, defaultItems, ok := c.partitionDefaultedFieldArrayMembershipItems(field, items)
	if !ok {
		return "", false, nil
	}

	predicates := make([]string, 0, 2)
	if len(fieldItems) > 0 {
		fieldItemSQLs, err := c.arrayMembershipItemSQLs(fieldItems, nil)
		if err != nil {
			return "", true, err
		}
		predicates = append(predicates, fmt.Sprintf("(%s IS NOT NULL AND %s)", fieldSQL, arrayLiteralMembershipSQL(fieldSQL, fieldItemSQLs)))
	}
	if len(defaultItems) > 0 {
		defaultSQL, err := c.dataOp.valueToSQL(field.defaultLiteral)
		if err != nil {
			return "", true, fmt.Errorf("invalid default value: %w", err)
		}
		defaultItemSQLs, err := c.arrayMembershipItemSQLs(defaultItems, nil)
		if err != nil {
			return "", true, err
		}
		predicates = append(predicates, fmt.Sprintf("(%s IS NULL AND %s)", fieldSQL, arrayLiteralMembershipSQL(defaultSQL, defaultItemSQLs)))
	}
	return combineOrPredicates(predicates), true, nil
}

func (c *ComparisonOperator) defaultedFieldArrayLiteralMembershipSQLParam(
	leftOriginal interface{},
	items []interface{},
	pc *params.ParamCollector,
) (string, bool, error) {
	field, ok := c.defaultedFieldArrayMembershipOperand(leftOriginal)
	if !ok {
		return "", false, nil
	}
	fieldSQL, err := c.fieldSQLForDefaultedArrayMembership(leftOriginal, field)
	if err != nil {
		return "", true, err
	}
	fieldItems, defaultItems, ok := c.partitionDefaultedFieldArrayMembershipItems(field, items)
	if !ok {
		return "", false, nil
	}

	predicates := make([]string, 0, 2)
	if len(fieldItems) > 0 {
		fieldItemSQLs, err := c.arrayMembershipItemSQLs(fieldItems, pc)
		if err != nil {
			return "", true, err
		}
		predicates = append(predicates, fmt.Sprintf("(%s IS NOT NULL AND %s)", fieldSQL, arrayLiteralMembershipSQL(fieldSQL, fieldItemSQLs)))
	}
	if len(defaultItems) > 0 {
		defaultSQL, err := c.dataOp.valueToSQLParam(field.defaultLiteral, pc)
		if err != nil {
			return "", true, fmt.Errorf("invalid default value: %w", err)
		}
		defaultItemSQLs, err := c.arrayMembershipItemSQLs(defaultItems, pc)
		if err != nil {
			return "", true, err
		}
		predicates = append(predicates, fmt.Sprintf("(%s IS NULL AND %s)", fieldSQL, arrayLiteralMembershipSQL(defaultSQL, defaultItemSQLs)))
	}
	return combineOrPredicates(predicates), true, nil
}

func (c *ComparisonOperator) defaultedFieldArrayMembershipOperand(leftOriginal interface{}) (equalityFieldOperand, bool) {
	field, ok := c.extractEqualityFieldOperand(leftOriginal)
	if !ok || !field.hasDefault || !field.defaultLiteralKnown || field.fieldName == "" {
		return equalityFieldOperand{}, false
	}
	if _, ok := c.schemaEqualityKind(field.fieldName); !ok {
		return equalityFieldOperand{}, false
	}
	if equalityLiteralKind(field.defaultLiteral) == "" {
		return equalityFieldOperand{}, false
	}
	return field, true
}

func (c *ComparisonOperator) fieldSQLForDefaultedArrayMembership(leftOriginal interface{}, field equalityFieldOperand) (string, error) {
	if pv, ok := leftOriginal.(ProcessedValue); ok && pv.IsSQL && pv.IsField {
		return pv.Value, nil
	}
	if varExpr, ok := leftOriginal.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			switch v := varName.(type) {
			case []interface{}:
				if len(v) == 0 {
					return "", fmt.Errorf("var operator array cannot be empty")
				}
				if pv, ok := v[0].(ProcessedValue); ok && pv.IsSQL && pv.IsField {
					return pv.Value, nil
				}
				if name, ok := v[0].(string); ok {
					return c.dataOp.ToSQL(OpVar, []interface{}{name})
				}
			case ProcessedValue:
				if v.IsSQL && v.IsField {
					return v.Value, nil
				}
			}
		}
	}
	return c.dataOp.ToSQL(OpVar, []interface{}{field.fieldName})
}

func (c *ComparisonOperator) partitionDefaultedFieldArrayMembershipItems(
	field equalityFieldOperand,
	items []interface{},
) ([]interface{}, []interface{}, bool) {
	fieldKind, ok := c.schemaEqualityKind(field.fieldName)
	if !ok {
		return nil, nil, false
	}
	defaultKind := equalityLiteralKind(field.defaultLiteral)
	if defaultKind == "" {
		return nil, nil, false
	}

	fieldItems := make([]interface{}, 0, len(items))
	defaultItems := make([]interface{}, 0, len(items))
	for _, item := range items {
		literal, ok := equalityLiteralValue(item)
		if !ok {
			return nil, nil, false
		}
		itemKind := equalityLiteralKind(literal)
		if itemKind == fieldKind {
			fieldItems = append(fieldItems, item)
		}
		if itemKind == defaultKind {
			defaultItems = append(defaultItems, item)
		}
		if itemKind == "" {
			return nil, nil, false
		}
	}
	return fieldItems, defaultItems, true
}

func (c *ComparisonOperator) arrayMembershipItemSQLs(items []interface{}, pc *params.ParamCollector) ([]string, error) {
	values := make([]string, 0, len(items))
	for _, item := range items {
		var (
			valueSQL string
			err      error
		)
		if pc == nil {
			valueSQL, err = c.dataOp.valueToSQL(item)
		} else {
			valueSQL, err = c.dataOp.valueToSQLParam(item, pc)
		}
		if err != nil {
			return nil, fmt.Errorf("invalid array element: %w", err)
		}
		values = append(values, valueSQL)
	}
	return values, nil
}

func (c *ComparisonOperator) validateEnumArrayMembershipItems(fieldName string, items []interface{}) error {
	for _, item := range items {
		literal, ok := equalityLiteralValue(item)
		if !ok || equalityLiteralKind(literal) != "string" {
			continue
		}
		if err := c.validateEnumValue(item, fieldName); err != nil {
			return err
		}
	}
	return nil
}

func (c *ComparisonOperator) validateDefaultedEnumFieldOperand(value interface{}) error {
	field, ok := c.extractEqualityFieldOperand(value)
	if !ok || !field.hasDefault || !field.defaultLiteralKnown {
		return nil
	}
	return c.validateEnumValue(field.defaultLiteral, field.fieldName)
}

func typedNullExpression(value interface{}) bool {
	pv, ok := value.(ProcessedValue)
	return ok && pv.IsSQL && pv.HasExpressionInfo &&
		pv.Kind == ExpressionKindValue && pv.Type == ExpressionTypeNull
}

func equalityLiteralForFieldSide(dec equalityDecision, fieldOnLeft bool) interface{} {
	if fieldOnLeft {
		return dec.right
	}
	return dec.left
}

func (c *ComparisonOperator) applySchemaComparisonCoercion(leftArg, rightArg interface{}) (interface{}, interface{}, error) {
	leftFieldName := c.extractFieldNameFromValue(leftArg)
	rightFieldName := c.extractFieldNameFromValue(rightArg)
	leftIsField := leftFieldName != "" || isProcessedSQLFieldOperand(leftArg)
	rightIsField := rightFieldName != "" || isProcessedSQLFieldOperand(rightArg)

	if leftIsField && rightIsField {
		return leftArg, rightArg, nil
	}
	if leftFieldName != "" && !rightIsField {
		rightArg = c.coerceValueForComparison(rightArg, leftFieldName)
		if err := c.validateEnumValue(rightArg, leftFieldName); err != nil {
			return nil, nil, err
		}
	}
	if rightFieldName != "" && !leftIsField {
		leftArg = c.coerceValueForComparison(leftArg, rightFieldName)
		if err := c.validateEnumValue(leftArg, rightFieldName); err != nil {
			return nil, nil, err
		}
	}
	return leftArg, rightArg, nil
}

// coerceValueForComparison coerces a literal value based on the type of the field being compared.
// If the field is numeric and the value is a string that represents a number, it returns the unquoted number.
// If the field is a string and the value is a number, it returns the number as a string so it gets quoted.
// This ensures proper SQL comparisons like "field >= 50000" instead of "field >= '50000'"
// and "string_field IN ('5960', '9000')" instead of "string_field IN (5960, 9000)".
func (c *ComparisonOperator) coerceValueForComparison(value interface{}, fieldName string) interface{} {
	if fieldName == "" {
		return value
	}

	// Coerce string → number for numeric fields
	if c.schema().IsNumericType(fieldName) {
		if strVal, ok := value.(string); ok {
			// Try to parse as integer first
			if intVal, err := strconv.ParseInt(strVal, 10, 64); err == nil {
				return intVal
			}
			// Try to parse as float
			if floatVal, err := strconv.ParseFloat(strVal, 64); err == nil {
				return floatVal
			}
		}
		return value
	}

	// Coerce number → string for string fields
	// Handles float64 (from JSON unmarshal) and all Go integer types (from TranspileFromMap)
	if c.schema().IsStringType(fieldName) {
		switch v := value.(type) {
		case json.Number:
			return v.String()
		case float64:
			if v == float64(int64(v)) {
				return fmt.Sprintf("%d", int64(v))
			}
			return fmt.Sprintf("%g", v)
		case float32:
			return fmt.Sprintf("%g", v)
		case int:
			return strconv.Itoa(v)
		case int8:
			return strconv.FormatInt(int64(v), 10)
		case int16:
			return strconv.FormatInt(int64(v), 10)
		case int32:
			return strconv.FormatInt(int64(v), 10)
		case int64:
			return strconv.FormatInt(v, 10)
		case uint:
			return strconv.FormatUint(uint64(v), 10)
		case uint8:
			return strconv.FormatUint(uint64(v), 10)
		case uint16:
			return strconv.FormatUint(uint64(v), 10)
		case uint32:
			return strconv.FormatUint(uint64(v), 10)
		case uint64:
			return strconv.FormatUint(v, 10)
		}
		return value
	}

	return value
}

// validateEnumValue validates that a value is valid for an enum field.
// Returns nil if valid or if not an enum field.
func (c *ComparisonOperator) validateEnumValue(value interface{}, fieldName string) error {
	if fieldName == "" {
		return nil
	}

	// Skip validation for null values (null is valid for any field)
	if value == nil {
		return nil
	}

	// Only validate if the field is an enum type
	if !c.schema().IsEnumType(fieldName) {
		return nil
	}

	// Extract string value for validation
	var strVal string
	switch v := value.(type) {
	case string:
		strVal = v
	default:
		// Non-string values for enum comparison - convert to string for validation
		strVal = fmt.Sprintf("%v", v)
	}

	return c.schema().ValidateEnumValue(fieldName, strVal)
}
