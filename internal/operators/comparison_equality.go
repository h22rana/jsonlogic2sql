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
		return literalKindNumber
	case c.schema().IsBooleanType(fieldName):
		return literalKindBoolean
	case c.schema().IsStringType(fieldName), c.schema().IsEnumType(fieldName):
		return literalKindString
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
	if c.schema().GetFieldType(field.fieldName) == objectFieldType {
		return validateVarDefaultForField(c.schema(), field.fieldName, field.defaultLiteral)
	}
	if c.schema().IsEnumType(field.fieldName) {
		return c.validateEnumValue(field.defaultLiteral, field.fieldName)
	}
	return nil
}

func (c *ComparisonOperator) defaultedFieldLiteralEqualitySQL(
	operator string,
	leftArg, rightArg interface{},
	pc *params.ParamCollector,
) (string, bool, error) {
	if !isEqualityOperator(operator) {
		return "", false, nil
	}

	fieldArg, literal, ok := c.defaultedFieldLiteralOperands(leftArg, rightArg)
	if !ok {
		return "", false, nil
	}
	if err := c.validateEqualityFieldOperand(fieldArg.field); err != nil {
		return "", true, err
	}
	if !c.defaultedFieldNeedsNullSplit(fieldArg.field) {
		return "", false, nil
	}
	fieldOnlyArg := fieldArg.withoutDefault()
	fieldSQL, err := c.valueToSQL(fieldOnlyArg)
	if err != nil {
		return "", true, fmt.Errorf("invalid field operand: %w", err)
	}

	fieldBranchSQL, fieldBranchConstant, err := c.fieldLiteralEqualityBranchSQL(operator, fieldOnlyArg, literal, pc)
	if err != nil {
		return "", true, err
	}
	defaultBranch := defaultLiteralEqualityResult(operator, fieldArg.field.defaultLiteral, literal)

	predicates := make([]string, 0, 2)
	if fieldBranchConstant != nil {
		if *fieldBranchConstant {
			predicates = append(predicates, fmt.Sprintf("%s IS NOT NULL", fieldSQL))
		}
	} else if fieldBranchSQL != "" {
		predicates = append(predicates, fmt.Sprintf("(%s IS NOT NULL AND %s)", fieldSQL, fieldBranchSQL))
	}
	if defaultBranch {
		predicates = append(predicates, fmt.Sprintf("%s IS NULL", fieldSQL))
	}
	return combineOrPredicates(predicates), true, nil
}

func (c *ComparisonOperator) defaultedFieldFieldEqualitySQL(
	operator string,
	leftArg, rightArg interface{},
	pc *params.ParamCollector,
) (string, bool, error) {
	if !isEqualityOperator(operator) {
		return "", false, nil
	}
	leftField, leftOK := c.extractEqualityFieldOperand(leftArg)
	rightField, rightOK := c.extractEqualityFieldOperand(rightArg)
	if !leftOK || !rightOK || (!leftField.hasDefault && !rightField.hasDefault) {
		return "", false, nil
	}
	if err := c.validateEqualityFieldOperand(leftField); err != nil {
		return "", true, err
	}
	if err := c.validateEqualityFieldOperand(rightField); err != nil {
		return "", true, err
	}
	if !c.defaultedFieldNeedsNullSplit(leftField) && !c.defaultedFieldNeedsNullSplit(rightField) {
		return "", false, nil
	}

	leftSQL, err := c.valueToSQL(defaultedFieldLiteralOperand{original: leftArg, field: leftField}.withoutDefault())
	if err != nil {
		return "", true, fmt.Errorf("invalid left field operand: %w", err)
	}
	rightSQL, err := c.valueToSQL(defaultedFieldLiteralOperand{original: rightArg, field: rightField}.withoutDefault())
	if err != nil {
		return "", true, fmt.Errorf("invalid right field operand: %w", err)
	}

	leftStates := defaultedEqualityStates(leftField)
	rightStates := defaultedEqualityStates(rightField)
	predicates := make([]string, 0, len(leftStates)*len(rightStates))
	for _, leftState := range leftStates {
		leftBranchArg := c.defaultedEqualityBranchArg(leftArg, leftField, leftState.useDefault)
		leftNullPredicate := defaultedEqualityNullPredicate(leftSQL, leftState.useDefault)
		for _, rightState := range rightStates {
			rightBranchArg := c.defaultedEqualityBranchArg(rightArg, rightField, rightState.useDefault)
			branchSQL, branchConstant, err := c.equalityBranchSQL(operator, leftBranchArg, rightBranchArg, pc)
			if err != nil {
				return "", true, err
			}
			conditions := make([]string, 0, 3)
			conditions = append(conditions, leftNullPredicate, defaultedEqualityNullPredicate(rightSQL, rightState.useDefault))
			if branchConstant != nil {
				if !*branchConstant {
					continue
				}
				predicates = append(predicates, combineAndPredicates(conditions))
				continue
			}
			if branchSQL == "" {
				continue
			}
			conditions = append(conditions, branchSQL)
			predicates = append(predicates, combineAndPredicates(conditions))
		}
	}
	return combineOrPredicates(predicates), true, nil
}

type defaultedEqualityState struct {
	useDefault bool
}

func defaultedEqualityStates(field equalityFieldOperand) []defaultedEqualityState {
	if field.hasDefault && field.defaultLiteralKnown {
		return []defaultedEqualityState{{useDefault: false}, {useDefault: true}}
	}
	return []defaultedEqualityState{{useDefault: false}}
}

func (c *ComparisonOperator) defaultedEqualityBranchArg(
	original interface{},
	field equalityFieldOperand,
	useDefault bool,
) interface{} {
	if useDefault {
		return field.defaultLiteral
	}
	return defaultedFieldLiteralOperand{original: original, field: field}.withoutDefault()
}

func defaultedEqualityNullPredicate(fieldSQL string, useDefault bool) string {
	if useDefault {
		return fmt.Sprintf("%s IS NULL", fieldSQL)
	}
	return fmt.Sprintf("%s IS NOT NULL", fieldSQL)
}

type defaultedFieldLiteralOperand struct {
	original interface{}
	field    equalityFieldOperand
}

func (c *ComparisonOperator) defaultedFieldLiteralOperands(
	leftArg, rightArg interface{},
) (defaultedFieldLiteralOperand, interface{}, bool) {
	leftField, leftIsField := c.extractEqualityFieldOperand(leftArg)
	rightField, rightIsField := c.extractEqualityFieldOperand(rightArg)
	if leftIsField == rightIsField {
		return defaultedFieldLiteralOperand{}, nil, false
	}

	if leftIsField {
		literal, ok := equalityLiteralValue(rightArg)
		return defaultedFieldLiteralOperand{original: leftArg, field: leftField}, literal, ok &&
			leftField.hasDefault && leftField.defaultLiteralKnown
	}

	literal, ok := equalityLiteralValue(leftArg)
	return defaultedFieldLiteralOperand{original: rightArg, field: rightField}, literal, ok &&
		rightField.hasDefault && rightField.defaultLiteralKnown
}

func (c *ComparisonOperator) defaultedFieldNeedsNullSplit(field equalityFieldOperand) bool {
	fieldKind, ok := c.schemaEqualityKind(field.fieldName)
	if !ok {
		return false
	}
	defaultKind := equalityLiteralKind(field.defaultLiteral)
	return defaultKind != "" && defaultKind != literalKindNull && defaultKind != fieldKind
}

func (o defaultedFieldLiteralOperand) withoutDefault() interface{} {
	if pv, ok := o.original.(ProcessedValue); ok && pv.IsSQL && pv.IsField {
		return processedFieldWithoutDefault(pv)
	}
	if varExpr, ok := o.original.(map[string]interface{}); ok && len(varExpr) == 1 {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			switch v := varName.(type) {
			case []interface{}:
				if len(v) > 0 {
					if pv, ok := v[0].(ProcessedValue); ok && pv.IsSQL && pv.IsField {
						return processedFieldWithoutDefault(pv)
					}
				}
			case ProcessedValue:
				if v.IsSQL && v.IsField {
					return processedFieldWithoutDefault(v)
				}
			}
		}
	}
	return map[string]interface{}{OpVar: o.field.fieldName}
}

func processedFieldWithoutDefault(pv ProcessedValue) ProcessedValue {
	pv.FieldHasDefault = false
	pv.FieldDefaultLiteralKnown = false
	pv.FieldDefaultLiteral = nil
	return pv
}

func (c *ComparisonOperator) fieldLiteralEqualityBranchSQL(
	operator string,
	fieldArg interface{},
	literal interface{},
	pc *params.ParamCollector,
) (string, *bool, error) {
	decision := c.applyEqualitySemantics(operator, fieldArg, literal)
	if decision.unsupported != nil {
		return "", nil, decision.unsupported
	}
	if decision.constant != nil {
		return "", decision.constant, nil
	}

	leftArg := fieldArg
	rightArg := literal
	if decision.handled {
		leftArg = decision.left
		rightArg = decision.right
	} else {
		var err error
		leftArg, rightArg, err = c.applySchemaComparisonCoercion(leftArg, rightArg)
		if err != nil {
			return "", nil, err
		}
	}
	leftArg = materializePredicateValueOperand(leftArg)
	rightArg = materializePredicateValueOperand(rightArg)

	var (
		leftSQL  string
		rightSQL string
		err      error
	)
	if pc == nil {
		leftSQL, err = c.valueToSQL(leftArg)
	} else {
		leftSQL, err = c.valueToSQLParam(leftArg, pc)
	}
	if err != nil {
		return "", nil, fmt.Errorf("invalid left operand: %w", err)
	}
	if pc == nil {
		rightSQL, err = c.valueToSQL(rightArg)
	} else {
		rightSQL, err = c.valueToSQLParam(rightArg, pc)
	}
	if err != nil {
		return "", nil, fmt.Errorf("invalid right operand: %w", err)
	}

	switch operator {
	case OpEqual, OpStrictEqual:
		return fmt.Sprintf("%s = %s", leftSQL, rightSQL), nil, nil
	case OpNotEqual:
		return fmt.Sprintf("%s != %s", leftSQL, rightSQL), nil, nil
	case OpStrictNotEqual:
		return fmt.Sprintf("%s <> %s", leftSQL, rightSQL), nil, nil
	default:
		return "", nil, fmt.Errorf("unsupported equality operator: %s", operator)
	}
}

func (c *ComparisonOperator) equalityBranchSQL(
	operator string,
	leftArg interface{},
	rightArg interface{},
	pc *params.ParamCollector,
) (string, *bool, error) {
	if leftLiteral, leftOK := equalityLiteralValue(leftArg); leftOK {
		if rightLiteral, rightOK := equalityLiteralValue(rightArg); rightOK {
			if err := validateEqualityJSONNumberLiteral(leftLiteral); err != nil {
				return "", nil, err
			}
			if err := validateEqualityJSONNumberLiteral(rightLiteral); err != nil {
				return "", nil, err
			}
			result := defaultLiteralEqualityResult(operator, leftLiteral, rightLiteral)
			return "", &result, nil
		}
	}

	decision := c.applyEqualitySemantics(operator, leftArg, rightArg)
	if decision.unsupported != nil {
		return "", nil, decision.unsupported
	}
	if decision.constant != nil {
		return "", decision.constant, nil
	}
	if decision.handled {
		leftArg = decision.left
		rightArg = decision.right
	} else {
		var err error
		leftArg, rightArg, err = c.applySchemaComparisonCoercion(leftArg, rightArg)
		if err != nil {
			return "", nil, err
		}
	}
	leftArg = materializePredicateValueOperand(leftArg)
	rightArg = materializePredicateValueOperand(rightArg)

	var (
		leftSQL  string
		rightSQL string
		err      error
	)
	if pc == nil {
		leftSQL, err = c.valueToSQL(leftArg)
	} else {
		leftSQL, err = c.valueToSQLParam(leftArg, pc)
	}
	if err != nil {
		return "", nil, fmt.Errorf("invalid left operand: %w", err)
	}
	if pc == nil {
		rightSQL, err = c.valueToSQL(rightArg)
	} else {
		rightSQL, err = c.valueToSQLParam(rightArg, pc)
	}
	if err != nil {
		return "", nil, fmt.Errorf("invalid right operand: %w", err)
	}

	isLeftNull := leftArg == nil || leftSQL == sqlNull
	isRightNull := rightArg == nil || rightSQL == sqlNull

	if !isLeftNull && !isRightNull {
		if leftBool, ok := sqlBooleanConstant(leftSQL); ok {
			if rightBool, ok := sqlBooleanConstant(rightSQL); ok {
				result := equalityPredicateConstant(operator, leftBool, rightBool)
				return "", &result, nil
			}
		}
		if sql, ok := c.strictIncompatibleFieldEqualitySQL(operator, leftArg, rightArg, leftSQL, rightSQL); ok {
			return sql, nil, nil
		}
		if c.shouldUseNullSafeFieldEquality(operator, leftArg, rightArg) {
			return nullSafeFieldEqualitySQL(operator, leftSQL, rightSQL), nil, nil
		}
	}

	switch operator {
	case OpEqual, OpStrictEqual:
		switch {
		case isLeftNull && isRightNull:
			result := true
			return "", &result, nil
		case isLeftNull:
			return fmt.Sprintf("%s IS NULL", rightSQL), nil, nil
		case isRightNull:
			return fmt.Sprintf("%s IS NULL", leftSQL), nil, nil
		default:
			return fmt.Sprintf("%s = %s", leftSQL, rightSQL), nil, nil
		}
	case OpNotEqual, OpStrictNotEqual:
		switch {
		case isLeftNull && isRightNull:
			result := false
			return "", &result, nil
		case isLeftNull:
			return fmt.Sprintf("%s IS NOT NULL", rightSQL), nil, nil
		case isRightNull:
			return fmt.Sprintf("%s IS NOT NULL", leftSQL), nil, nil
		case operator == OpNotEqual:
			return fmt.Sprintf("%s != %s", leftSQL, rightSQL), nil, nil
		default:
			return fmt.Sprintf("%s <> %s", leftSQL, rightSQL), nil, nil
		}
	default:
		return "", nil, fmt.Errorf("unsupported equality operator: %s", operator)
	}
}

func defaultLiteralEqualityResult(operator string, defaultLiteral, literal interface{}) bool {
	var equal bool
	if isStrictEqualityOperator(operator) {
		equal = equalityLiteralsStrictEqual(defaultLiteral, literal)
	} else {
		equal = equalityLiteralsLooseEqual(defaultLiteral, literal)
	}
	return (operator == OpEqual || operator == OpStrictEqual) == equal
}

func equalityOperandKind(value interface{}) (string, bool) {
	if kind, ok := expressionEqualityKind(value); ok {
		return kind, true
	}
	if _, ok := value.([]interface{}); ok {
		return literalKindArray, true
	}
	literal, ok := equalityLiteralValue(value)
	if !ok {
		return "", false
	}
	kind := equalityLiteralKind(literal)
	return kind, kind != ""
}

func equalityKindsHaveStructuredScalar(leftKind, rightKind string) (string, bool) {
	switch {
	case isStructuredEqualityKind(leftKind) && isScalarEqualityKind(rightKind):
		return leftKind, true
	case isStructuredEqualityKind(rightKind) && isScalarEqualityKind(leftKind):
		return rightKind, true
	default:
		return "", false
	}
}

func isStructuredEqualityKind(kind string) bool {
	return kind == literalKindArray || kind == objectFieldType
}

func isScalarEqualityKind(kind string) bool {
	switch kind {
	case literalKindString, literalKindBoolean, literalKindNumber:
		return true
	default:
		return false
	}
}

func applyStructuredScalarEqualitySemantics(dec equalityDecision, operator, structuredKind string) equalityDecision {
	dec.handled = true
	if isStrictEqualityOperator(operator) {
		dec.constant = impossibleEqualityPredicateConstant(operator)
		return dec
	}
	dec.unsupported = fmt.Errorf("%s equality with scalar operands is not supported", structuredKind)
	return dec
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
		if err := c.validateFieldEqualityArrayCompatibility(leftField, rightField); err != nil {
			dec.unsupported = err
			dec.handled = true
			return dec
		}
		if isStrictEqualityOperator(operator) && !strictIncompatibleFieldsCanUseNullBranch(leftField, rightField) {
			if structuredKind, ok := c.structuredScalarFieldEqualityKind(leftField, rightField); ok {
				return applyStructuredScalarEqualitySemantics(dec, operator, structuredKind)
			}
		}
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
	if fieldKind, ok := c.schemaEqualityKind(fieldName); ok {
		if literalKind, literalKnown := equalityOperandKind(literalArg); literalKnown {
			if structuredKind, ok := equalityKindsHaveStructuredScalar(fieldKind, literalKind); ok {
				return applyStructuredScalarEqualitySemantics(dec, operator, structuredKind)
			}
		}
	}

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
		if fieldKind == literalKindNumber {
			if _, isJSONNumber := literal.(json.Number); !isJSONNumber {
				if _, handled, valid := jsNumberFromLiteral(literal); handled && !valid {
					dec.constant = impossibleEqualityPredicateConstant(operator)
					return dec
				}
			}
		}
		if fieldKind == literalKindNumber && c.schema().GetFieldType(fieldName) == SchemaTypeInteger {
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
	case literalKindNumber:
		if c.schema().GetFieldType(fieldName) == literalKindNumber {
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
		if c.schema().GetFieldType(fieldName) == SchemaTypeInteger {
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
	case literalKindBoolean:
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
	case literalKindString:
		if _, ok := literal.(bool); ok {
			dec.unsupported = fmt.Errorf(
				"loose equality between string field %q and boolean literal is not supported", fieldName)
			return dec
		}
		if equalityLiteralKind(literal) == literalKindNumber {
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
		if structuredKind, ok := equalityKindsHaveStructuredScalar(leftKind, rightKind); ok {
			return applyStructuredScalarEqualitySemantics(dec, operator, structuredKind)
		}
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
	literalKind := equalityLiteralKind(literal)
	if structuredKind, ok := equalityKindsHaveStructuredScalar(exprKind, literalKind); ok {
		return applyStructuredScalarEqualitySemantics(dec, operator, structuredKind)
	}

	if isStrictEqualityOperator(operator) {
		if literalKind != "" && literalKind != exprKind {
			dec.handled = true
			dec.constant = impossibleEqualityPredicateConstant(operator)
			return dec
		}
		if exprKind == literalKindNumber {
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
	case literalKindNumber:
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
	case literalKindBoolean:
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
	case literalKindString:
		dec.handled = true
		if _, ok := literal.(bool); ok {
			dec.unsupported = fmt.Errorf("loose equality between string expression and boolean literal is not supported")
			return dec
		}
		if equalityLiteralKind(literal) == literalKindNumber {
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
		return literalKindBoolean, true
	}
	switch pv.Type {
	case ExpressionTypeNull:
		return literalKindNull, true
	case ExpressionTypeBoolean:
		return literalKindBoolean, true
	case ExpressionTypeString:
		return literalKindString, true
	case ExpressionTypeNumber:
		return literalKindNumber, true
	case ExpressionTypeArray:
		return literalKindArray, true
	case ExpressionTypeObject:
		return objectFieldType, true
	case ExpressionTypeUnknown:
		return "", false
	}
	return "", false
}

func (c *ComparisonOperator) schemaEqualityKind(fieldName string) (string, bool) {
	switch {
	case c.schema().IsStringType(fieldName), c.schema().IsEnumType(fieldName):
		return literalKindString, true
	case c.schema().IsNumericType(fieldName):
		return literalKindNumber, true
	case c.schema().IsBooleanType(fieldName):
		return literalKindBoolean, true
	case c.schema().IsArrayType(fieldName):
		return literalKindArray, true
	case c.schema().GetFieldType(fieldName) == objectFieldType:
		return objectFieldType, true
	default:
		return "", false
	}
}

func (c *ComparisonOperator) structuredScalarFieldEqualityKind(
	leftField, rightField equalityFieldOperand,
) (string, bool) {
	leftKind, leftKnown := c.schemaEqualityKind(leftField.fieldName)
	rightKind, rightKnown := c.schemaEqualityKind(rightField.fieldName)
	if !leftKnown || !rightKnown {
		return "", false
	}
	return equalityKindsHaveStructuredScalar(leftKind, rightKind)
}

func strictIncompatibleFieldsCanUseNullBranch(leftField, rightField equalityFieldOperand) bool {
	return equalityFieldCanEvaluateNull(leftField) && equalityFieldCanEvaluateNull(rightField)
}

func equalityFieldCanEvaluateNull(field equalityFieldOperand) bool {
	return !field.hasDefault || (field.defaultLiteralKnown && field.defaultLiteral == nil)
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
		if itemKind, ok := expressionEqualityKind(item); ok {
			if _, ok := leftKinds[itemKind]; itemKind == literalKindNull || itemKind == "" || ok {
				filtered = append(filtered, item)
			}
			continue
		}
		literal, ok := equalityLiteralValue(item)
		if !ok {
			filtered = append(filtered, item)
			continue
		}
		itemKind := equalityLiteralKind(literal)
		if _, ok := leftKinds[itemKind]; itemKind == literalKindNull || itemKind == "" || ok {
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
		defaultSQL, err := c.dataOp.defaultValueToSQL(field.defaultLiteral)
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
		defaultSQL, err := c.dataOp.defaultValueToSQLParam(field.defaultLiteral, pc)
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

func (c *ComparisonOperator) arrayMembershipItemSQLs(items []interface{}, pc *params.ParamCollector) ([]arrayLiteralMembershipItemSQL, error) {
	values := make([]arrayLiteralMembershipItemSQL, 0, len(items))
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
		values = append(values, newArrayLiteralMembershipItemSQL(item, valueSQL))
	}
	return values, nil
}

func (c *ComparisonOperator) validateEnumArrayMembershipItems(fieldName string, items []interface{}) error {
	for _, item := range items {
		literal, ok := equalityLiteralValue(item)
		if !ok || equalityLiteralKind(literal) != literalKindString {
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
