package operators

import (
	"fmt"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// ToSQLParam is the parameterized variant of ToSQL. Keep in sync.
func (c *ComparisonOperator) ToSQLParam(operator string, args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) >= 2 && IsOrderingComparisonOperatorName(operator) {
		return c.handleChainedComparisonParam(operator, args, pc)
	}

	if len(args) != 2 {
		return "", fmt.Errorf("%s operator requires exactly 2 arguments", operator)
	}

	if operator == OpIn {
		return c.handleInParam(materializePredicateValueOperand(args[0]), args[1], pc)
	}

	leftArg := args[0]
	rightArg := args[1]

	if isEqualityOperator(operator) {
		if sql, handled, err := c.defaultedFieldFieldEqualitySQL(operator, leftArg, rightArg, pc); handled || err != nil {
			return sql, err
		}
		if sql, handled, err := c.defaultedFieldLiteralEqualitySQL(operator, leftArg, rightArg, pc); handled || err != nil {
			return sql, err
		}
		decision := c.applyEqualitySemantics(operator, leftArg, rightArg)
		if decision.unsupported != nil {
			return "", decision.unsupported
		}
		if decision.constant != nil {
			return boolSQL(*decision.constant), nil
		}
		if decision.handled {
			leftArg = decision.left
			rightArg = decision.right
		} else {
			var err error
			leftArg, rightArg, err = c.applySchemaComparisonCoercion(leftArg, rightArg)
			if err != nil {
				return "", err
			}
		}
	} else {
		var err error
		leftArg, rightArg, err = c.applySchemaComparisonCoercion(leftArg, rightArg)
		if err != nil {
			return "", err
		}
	}
	if isEqualityOperator(operator) {
		leftArg = materializePredicateValueOperand(leftArg)
		rightArg = materializePredicateValueOperand(rightArg)
	}

	if isOrderingOperator(operator) {
		leftArg = numericOrderingOperand(leftArg)
		rightArg = numericOrderingOperand(rightArg)
		if err := c.validateOrderingOperandsCompatible(leftArg, rightArg, operator); err != nil {
			return "", err
		}
	}

	leftSQL, err := c.valueToSQLParam(leftArg, pc)
	if err != nil {
		return "", fmt.Errorf("invalid left operand: %w", err)
	}

	rightSQL, err := c.valueToSQLParam(rightArg, pc)
	if err != nil {
		return "", fmt.Errorf("invalid right operand: %w", err)
	}

	isLeftNull := args[0] == nil || leftSQL == sqlNull
	isRightNull := args[1] == nil || rightSQL == sqlNull

	if isEqualityOperator(operator) && !isLeftNull && !isRightNull {
		if leftBool, ok := sqlBooleanConstant(leftSQL); ok {
			if rightBool, ok := sqlBooleanConstant(rightSQL); ok {
				return boolSQL(equalityPredicateConstant(operator, leftBool, rightBool)), nil
			}
		}
	}

	if !isLeftNull && !isRightNull {
		if sql, ok := c.strictIncompatibleFieldEqualitySQL(operator, args[0], args[1], leftSQL, rightSQL); ok {
			return sql, nil
		}
	}

	if c.shouldUseNullSafeFieldEquality(operator, args[0], args[1]) && !isLeftNull && !isRightNull {
		return nullSafeFieldEqualitySQL(operator, leftSQL, rightSQL), nil
	}

	switch operator {
	case OpEqual:
		if isLeftNull && isRightNull {
			return sqlNull + " IS NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s = %s", leftSQL, rightSQL), nil
	case OpStrictEqual:
		if isLeftNull && isRightNull {
			return sqlNull + " IS NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s = %s", leftSQL, rightSQL), nil
	case OpNotEqual:
		if isLeftNull && isRightNull {
			return sqlNull + " IS NOT NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NOT NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NOT NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s != %s", leftSQL, rightSQL), nil
	case OpStrictNotEqual:
		if isLeftNull && isRightNull {
			return sqlNull + " IS NOT NULL", nil
		}
		if isLeftNull {
			return fmt.Sprintf("%s IS NOT NULL", rightSQL), nil
		}
		if isRightNull {
			return fmt.Sprintf("%s IS NOT NULL", leftSQL), nil
		}
		return fmt.Sprintf("%s <> %s", leftSQL, rightSQL), nil
	case OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual:
		if err := c.validateOrderingOperand(leftArg, operator); err != nil {
			return "", err
		}
		if err := c.validateOrderingOperand(rightArg, operator); err != nil {
			return "", err
		}
		return fmt.Sprintf("%s %s %s", leftSQL, operator, rightSQL), nil
	default:
		return "", fmt.Errorf("unsupported comparison operator: %s", operator)
	}
}

// valueToSQLParam is the parameterized variant of valueToSQL. Keep in sync.
func (c *ComparisonOperator) valueToSQLParam(value interface{}, pc *params.ParamCollector) (string, error) {
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		return c.dataOp.valueToSQLParam(pv.Value, pc)
	}

	if varExpr, ok := value.(map[string]interface{}); ok {
		if len(varExpr) != 1 {
			return "", fmt.Errorf("operator object must have exactly one key")
		}
		for operator, args := range varExpr {
			if operator == OpVar {
				if varName, ok := args.(string); ok && varName == "" {
					return "elem", nil
				}
				return c.dataOp.ToSQLParam(OpVar, []interface{}{args}, pc)
			}
		}
	}

	if expr, ok := value.(map[string]interface{}); ok {
		if len(expr) == 1 {
			for op, args := range expr {
				switch op {
				case OpAdd, OpSubtract, OpMultiply, OpDivide, OpModulo:
					return c.processArithmeticExpressionParam(op, args, pc)
				case OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual,
					OpEqual, OpStrictEqual, OpNotEqual, OpStrictNotEqual:
					return c.processComparisonExpressionParam(op, args, pc)
				case OpMax, OpMin:
					return c.processMinMaxExpressionParam(op, args, pc)
				case OpIf:
					if arr, ok := args.([]interface{}); ok {
						logicalOp := NewLogicalOperator(c.config)
						return logicalOp.ToSQLParam(OpIf, arr, pc)
					}
					return "", fmt.Errorf("if operator requires array arguments")
				case OpReduce, OpFilter, OpMap, OpSome, OpAll, OpNone, OpMerge:
					if arr, ok := args.([]interface{}); ok {
						arrayOp := NewArrayOperator(c.config)
						return arrayOp.ToSQLParam(op, arr, pc)
					}
					return "", fmt.Errorf("array operator %s requires array arguments", op)
				case OpCat, OpSubstr:
					if arr, ok := args.([]interface{}); ok {
						stringOp := NewStringOperator(c.config)
						return stringOp.ToSQLParam(op, arr, pc)
					}
					return "", fmt.Errorf("string operator %s requires array arguments", op)
				default:
					if c.config != nil && c.config.HasParamExpressionParser() {
						return c.config.ParseExpressionParam(expr, jsonPathRoot, pc)
					}
					return "", fmt.Errorf("unsupported expression type in comparison: %s", op)
				}
			}
		}
	}

	if _, ok := value.([]interface{}); ok {
		return "", fmt.Errorf("arrays should be handled by handleInParam method")
	}

	return c.dataOp.valueToSQLParam(value, pc)
}

// handleInParam is the parameterized variant of handleIn. Keep in sync.
//
// Unlike handleIn, leftSQL is NOT pre-generated. This method generates the
// placeholder for the left operand after determining whether coercion is needed
// (P2 fix) and uses the original value's Go type rather than SQL quoting to
// distinguish string containment from array membership (P1 fix).
func (c *ComparisonOperator) handleInParam(leftOriginal, rightValue interface{}, pc *params.ParamCollector) (string, error) {
	leftFieldName := c.extractFieldNameFromValue(leftOriginal)

	if varExpr, ok := rightValue.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			rightSQL, err := c.dataOp.ToSQLParam(OpVar, []interface{}{varName}, pc)
			if err != nil {
				return "", fmt.Errorf("invalid variable in IN operator: %w", err)
			}

			var fieldName string
			if nameStr, ok := varName.(string); ok {
				fieldName = nameStr
			} else if nameArr, ok := varName.([]interface{}); ok && len(nameArr) > 0 {
				if nameStr, ok := nameArr[0].(string); ok {
					fieldName = nameStr
				}
			}

			if fieldName != "" {
				if c.schema().IsArrayType(fieldName) {
					if sql, handled, membershipErr := c.objectArrayHaystackMembershipSQL(leftOriginal, ProcessedValue{
						Value:             rightSQL,
						IsSQL:             true,
						IsField:           true,
						FieldName:         fieldName,
						HasExpressionInfo: true,
						Kind:              ExpressionKindValue,
						Type:              ExpressionTypeArray,
					}); handled || membershipErr != nil {
						return sql, membershipErr
					}
					compatible, membershipErr := c.validateArrayMembershipNeedle(fieldName, leftOriginal)
					if membershipErr != nil {
						return "", membershipErr
					}
					if !compatible {
						return boolSQL(false), nil
					}
					leftSQL, lErr := c.valueToSQLParam(leftOriginal, pc)
					if lErr != nil {
						return "", fmt.Errorf("invalid left operand: %w", lErr)
					}
					return c.arrayMembershipSQL(leftSQL, rightSQL), nil
				} else if c.schema().IsStringType(fieldName) || c.schema().IsEnumType(fieldName) {
					sql, lErr := c.stringContainmentSQLParamAuto(rightSQL, leftOriginal, pc)
					if lErr != nil {
						return "", fmt.Errorf("invalid left operand after coercion: %w", lErr)
					}
					return sql, nil
				} else if c.schema().IsNumericType(fieldName) || c.schema().IsBooleanType(fieldName) {
					return boolSQL(false), nil
				}
			}

			// Infer from left operand shape/type in a heuristic way. This
			// improves string-containment detection for nested string
			// expressions (cat/substr) while still falling back safely.
			isLeftString := c.isStringLikeInOperandSchemaRequired(leftOriginal, pc, 0)

			if isLeftString {
				sql, lErr := c.stringContainmentSQLParamAuto(rightSQL, leftOriginal, pc)
				if lErr != nil {
					return "", fmt.Errorf("invalid left operand for string containment: %w", lErr)
				}
				return sql, nil
			}
			leftSQL, err := c.valueToSQLParam(leftOriginal, pc)
			if err != nil {
				return "", fmt.Errorf("invalid left operand: %w", err)
			}
			return c.arrayMembershipSQL(leftSQL, rightSQL), nil
		}
	}

	if rightField, ok := fieldExpressionOperandSQL(rightValue); ok {
		return c.handleInSQLRightParam(leftOriginal, rightField.Value, rightField.FieldName, rightField.Type, rightField.HasExpressionInfo, pc)
	}

	if str, ok := rightValue.(string); ok {
		rightSQL, err := c.dataOp.valueToSQLParam(str, pc)
		if err != nil {
			return "", fmt.Errorf("invalid string in IN operator: %w", err)
		}
		sql, err := c.stringContainmentSQLParamAuto(rightSQL, leftOriginal, pc)
		if err != nil {
			return "", fmt.Errorf("invalid left operand for string containment: %w", err)
		}
		return sql, nil
	}

	if rightSQL, rightType, ok := valueExpressionOperandSQL(rightValue); ok {
		switch rightType {
		case ExpressionTypeString:
			sql, err := c.stringContainmentSQLParamAuto(rightSQL, leftOriginal, pc)
			if err != nil {
				return "", fmt.Errorf("invalid left operand for string containment: %w", err)
			}
			return sql, nil
		case ExpressionTypeNull, ExpressionTypeBoolean, ExpressionTypeNumber, ExpressionTypeObject:
			return boolSQL(false), nil
		case ExpressionTypeUnknown:
			if c.isStringLikeInOperandSchemaRequired(leftOriginal, pc, 0) {
				sql, err := c.stringContainmentSQLParamAuto(rightSQL, leftOriginal, pc)
				if err != nil {
					return "", fmt.Errorf("invalid left operand for string containment: %w", err)
				}
				return sql, nil
			}
		case ExpressionTypeArray:
		}
	}

	if arr, ok := rightValue.([]interface{}); ok {
		if len(arr) == 0 {
			return boolSQL(false), nil
		}
		if c.isKnownArrayOperand(leftOriginal) {
			return boolSQL(false), nil
		}
		filtered := c.strictArrayMembershipItems(leftOriginal, arr)
		if len(filtered) == 0 {
			return boolSQL(false), nil
		}
		if leftFieldName != "" && c.schema().IsEnumType(leftFieldName) {
			if err := c.validateEnumArrayMembershipItems(leftFieldName, filtered); err != nil {
				return "", err
			}
		}
		if err := c.validateDefaultedEnumFieldOperand(leftOriginal); err != nil {
			return "", err
		}
		if sql, handled, err := c.defaultedFieldArrayLiteralMembershipSQLParam(leftOriginal, filtered, pc); handled || err != nil {
			return sql, err
		}
	}
	if nonContainer, err := nonContainerInHaystackLiteral(rightValue); err != nil {
		return "", fmt.Errorf("invalid non-container in IN operator: %w", err)
	} else if nonContainer {
		return boolSQL(false), nil
	}

	// Generate leftSQL for the remaining array and string haystack paths.
	leftSQL, err := c.valueToSQLParam(leftOriginal, pc)
	if err != nil {
		return "", fmt.Errorf("invalid left operand: %w", err)
	}

	if rightSQL, rightType, ok := valueExpressionOperandSQL(rightValue); ok {
		return c.handleInSQLRightParamWithLeftSQL(leftSQL, leftOriginal, rightSQL, "", rightType, true, pc)
	}

	if arr, ok := rightValue.([]interface{}); ok {
		arr = c.strictArrayMembershipItems(leftOriginal, arr)
		if len(arr) == 0 {
			return boolSQL(false), nil
		}

		if leftFieldName != "" && c.schema().IsEnumType(leftFieldName) {
			if err := c.validateEnumArrayMembershipItems(leftFieldName, arr); err != nil {
				return "", err
			}
		}

		values, err := c.arrayMembershipItemSQLs(arr, pc)
		if err != nil {
			return "", err
		}

		return arrayLiteralMembershipSQL(leftSQL, values), nil
	}

	return "", fmt.Errorf("in operator requires array, variable, or string as second argument")
}

func (c *ComparisonOperator) handleInSQLRightParam(
	leftOriginal interface{},
	rightSQL string,
	fieldName string,
	rightType ExpressionType,
	hasRightType bool,
	pc *params.ParamCollector,
) (string, error) {
	if fieldName != "" {
		if c.schema().IsArrayType(fieldName) {
			if sql, handled, err := c.objectArrayHaystackMembershipSQL(leftOriginal, ProcessedValue{
				Value:             rightSQL,
				IsSQL:             true,
				IsField:           true,
				FieldName:         fieldName,
				HasExpressionInfo: true,
				Kind:              ExpressionKindValue,
				Type:              ExpressionTypeArray,
			}); handled || err != nil {
				return sql, err
			}
			compatible, err := c.validateArrayMembershipNeedle(fieldName, leftOriginal)
			if err != nil {
				return "", err
			}
			if !compatible {
				return boolSQL(false), nil
			}
			leftSQL, err := c.valueToSQLParam(leftOriginal, pc)
			if err != nil {
				return "", fmt.Errorf("invalid left operand: %w", err)
			}
			return c.arrayMembershipSQL(leftSQL, rightSQL), nil
		}
		if c.schema().IsStringType(fieldName) || c.schema().IsEnumType(fieldName) {
			sql, err := c.stringContainmentSQLParamAuto(rightSQL, leftOriginal, pc)
			if err != nil {
				return "", fmt.Errorf("invalid left operand after coercion: %w", err)
			}
			return sql, nil
		}
		if c.schema().IsNumericType(fieldName) || c.schema().IsBooleanType(fieldName) {
			return boolSQL(false), nil
		}
	}

	if hasRightType {
		switch rightType {
		case ExpressionTypeString:
			sql, err := c.stringContainmentSQLParamAuto(rightSQL, leftOriginal, pc)
			if err != nil {
				return "", fmt.Errorf("invalid left operand for string containment: %w", err)
			}
			return sql, nil
		case ExpressionTypeNull, ExpressionTypeBoolean, ExpressionTypeNumber, ExpressionTypeObject:
			return boolSQL(false), nil
		case ExpressionTypeUnknown:
			if c.isStringLikeInOperandSchemaRequired(leftOriginal, pc, 0) {
				sql, err := c.stringContainmentSQLParamAuto(rightSQL, leftOriginal, pc)
				if err != nil {
					return "", fmt.Errorf("invalid left operand for string containment: %w", err)
				}
				return sql, nil
			}
		case ExpressionTypeArray:
		}
	}

	leftSQL, err := c.valueToSQLParam(leftOriginal, pc)
	if err != nil {
		return "", fmt.Errorf("invalid left operand: %w", err)
	}
	return c.handleInSQLRightParamWithLeftSQL(leftSQL, leftOriginal, rightSQL, fieldName, rightType, hasRightType, pc)
}

func (c *ComparisonOperator) handleInSQLRightParamWithLeftSQL(
	leftSQL string,
	leftOriginal interface{},
	rightSQL string,
	fieldName string,
	rightType ExpressionType,
	hasRightType bool,
	pc *params.ParamCollector,
) (string, error) {
	if fieldName != "" && c.schema().IsArrayType(fieldName) {
		if sql, handled, err := c.objectArrayHaystackMembershipSQL(leftOriginal, ProcessedValue{
			Value:             rightSQL,
			IsSQL:             true,
			IsField:           true,
			FieldName:         fieldName,
			HasExpressionInfo: true,
			Kind:              ExpressionKindValue,
			Type:              ExpressionTypeArray,
		}); handled || err != nil {
			return sql, err
		}
		compatible, err := c.validateArrayMembershipNeedle(fieldName, leftOriginal)
		if err != nil {
			return "", err
		}
		if !compatible {
			return boolSQL(false), nil
		}
		return c.arrayMembershipSQL(leftSQL, rightSQL), nil
	}

	if hasRightType {
		switch rightType {
		case ExpressionTypeString:
			needleSQL, err := c.stringContainmentNeedleSQLParam(leftOriginal, leftSQL, pc)
			if err != nil {
				return "", fmt.Errorf("invalid left operand for string containment: %w", err)
			}
			return c.stringContainmentSQL(rightSQL, leftOriginal, needleSQL)
		case ExpressionTypeArray:
			if sql, handled, err := c.objectArrayHaystackMembershipSQL(leftOriginal, ProcessedValue{
				Value:             rightSQL,
				IsSQL:             true,
				HasExpressionInfo: true,
				Kind:              ExpressionKindValue,
				Type:              ExpressionTypeArray,
			}); handled || err != nil {
				return sql, err
			}
			return c.arrayMembershipSQL(leftSQL, rightSQL), nil
		case ExpressionTypeNull, ExpressionTypeBoolean, ExpressionTypeNumber, ExpressionTypeObject:
			return boolSQL(false), nil
		case ExpressionTypeUnknown:
		}
	}

	if c.isStringLikeInOperandSchemaRequired(leftOriginal, pc, 0) {
		needleSQL, err := c.stringContainmentNeedleSQLParam(leftOriginal, leftSQL, pc)
		if err != nil {
			return "", fmt.Errorf("invalid left operand for string containment: %w", err)
		}
		return c.stringContainmentSQL(rightSQL, leftOriginal, needleSQL)
	}
	return c.arrayMembershipSQL(leftSQL, rightSQL), nil
}

// handleChainedComparisonParam is the parameterized variant of handleChainedComparison. Keep in sync.
func (c *ComparisonOperator) handleChainedComparisonParam(operator string, args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("chained comparison requires at least 2 arguments")
	}

	if shouldFoldLiteralOrderingBeforeValidation(args) {
		truthy, known, err := FoldLiteralComparison(operator, args)
		if known || err != nil {
			return boolSQL(truthy), err
		}
	}

	for _, arg := range args {
		if err := c.validateOrderingOperand(arg, operator); err != nil {
			return "", err
		}
	}

	coercedArgs := make([]interface{}, len(args))
	copy(coercedArgs, args)

	var fieldName string
	for _, arg := range args {
		if name := c.extractFieldNameFromValue(arg); name != "" {
			fieldName = name
			break
		}
	}

	if fieldName != "" {
		for i, arg := range coercedArgs {
			if c.extractFieldNameFromValue(arg) == "" {
				coercedArgs[i] = c.coerceValueForComparison(arg, fieldName)
			}
		}
	}
	for i, arg := range coercedArgs {
		coercedArgs[i] = numericOrderingOperand(arg)
	}
	for i := 0; i < len(coercedArgs)-1; i++ {
		if err := c.validateOrderingOperandsCompatible(coercedArgs[i], coercedArgs[i+1], operator); err != nil {
			return "", err
		}
	}

	var sqlArgs []string
	for i, arg := range coercedArgs {
		argSQL, err := c.valueToSQLParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid argument %d: %w", i, err)
		}
		sqlArgs = append(sqlArgs, argSQL)
	}

	if len(args) == 2 {
		return fmt.Sprintf("%s %s %s", sqlArgs[0], operator, sqlArgs[1]), nil
	}

	var conditions []string
	for i := 0; i < len(sqlArgs)-1; i++ {
		condition := fmt.Sprintf("%s %s %s", sqlArgs[i], operator, sqlArgs[i+1])
		conditions = append(conditions, condition)
	}

	return fmt.Sprintf("(%s)", strings.Join(conditions, sqlAndJoiner)), nil
}

// processArithmeticExpressionParam is the parameterized variant of processArithmeticExpression. Keep in sync.
func (c *ComparisonOperator) processArithmeticExpressionParam(op string, args interface{}, pc *params.ParamCollector) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("arithmetic operation requires array of arguments")
	}

	if op == "-" && len(argsSlice) == 1 {
		operand, err := c.valueToSQLParam(argsSlice[0], pc)
		if err != nil {
			return "", fmt.Errorf("invalid unary minus argument: %w", err)
		}
		return fmt.Sprintf("(-%s)", operand), nil
	}

	if op == "+" && len(argsSlice) == 1 {
		operand, err := c.valueToSQLParam(argsSlice[0], pc)
		if err != nil {
			return "", fmt.Errorf("invalid unary plus argument: %w", err)
		}
		return fmt.Sprintf("CAST(%s AS NUMERIC)", operand), nil
	}

	if len(argsSlice) < 2 {
		return "", fmt.Errorf("arithmetic operation requires at least 2 arguments")
	}

	operands := make([]string, len(argsSlice))
	for i, arg := range argsSlice {
		operand, err := c.valueToSQLParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid arithmetic argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	switch op {
	case OpAdd:
		return fmt.Sprintf("(%s)", strings.Join(operands, " + ")), nil
	case OpSubtract:
		return fmt.Sprintf("(%s)", strings.Join(operands, " - ")), nil
	case OpMultiply:
		return fmt.Sprintf("(%s)", strings.Join(operands, " * ")), nil
	case OpDivide:
		return fmt.Sprintf("(%s)", strings.Join(operands, " / ")), nil
	case OpModulo:
		if len(operands) != 2 {
			return "", fmt.Errorf("modulo requires exactly 2 arguments")
		}
		return c.config.ModuloSQL(operands[0], operands[1]), nil
	default:
		return "", fmt.Errorf("unsupported arithmetic operation: %s", op)
	}
}

// processComparisonExpressionParam is the parameterized variant of processComparisonExpression. Keep in sync.
func (c *ComparisonOperator) processComparisonExpressionParam(op string, args interface{}, pc *params.ParamCollector) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("comparison operation requires array of arguments")
	}

	if len(argsSlice) != 2 {
		return "", fmt.Errorf("comparison operation requires exactly 2 arguments")
	}

	sql, err := c.ToSQLParam(op, argsSlice, pc)
	if err != nil {
		return "", err
	}
	if _, ok := sqlBooleanConstant(sql); ok {
		return sql, nil
	}
	return fmt.Sprintf("(%s)", sql), nil
}

// processMinMaxExpressionParam is the parameterized variant of processMinMaxExpression. Keep in sync.
func (c *ComparisonOperator) processMinMaxExpressionParam(op string, args interface{}, pc *params.ParamCollector) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("min/max operation requires array of arguments")
	}

	if len(argsSlice) < 2 {
		return "", fmt.Errorf("min/max operation requires at least 2 arguments")
	}

	operands := make([]string, len(argsSlice))
	for i, arg := range argsSlice {
		operand, err := c.valueToSQLParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid min/max argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	switch op {
	case OpMax:
		return c.config.GreatestSQL(operands), nil
	case OpMin:
		return c.config.LeastSQL(operands), nil
	default:
		return "", fmt.Errorf("unsupported min/max operation: %s", op)
	}
}
