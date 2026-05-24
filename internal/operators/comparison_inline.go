package operators

import (
	"fmt"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// ToSQL converts a comparison operator to SQL.
func (c *ComparisonOperator) ToSQL(operator string, args []interface{}) (string, error) {
	// Handle chained comparisons (2+ arguments)
	if len(args) >= 2 && IsOrderingComparisonOperatorName(operator) {
		return c.handleChainedComparison(operator, args)
	}

	if len(args) != 2 {
		return "", fmt.Errorf("%s operator requires exactly 2 arguments", operator)
	}

	// Special handling for 'in' operator - right side should be an array
	if operator == OpIn {
		leftArg := materializePredicateValueOperand(args[0])
		if sql, handled, err := c.handleInStringifiableLiteralNeedle(leftArg, args[1]); handled || err != nil {
			return sql, err
		}
		if str, ok := args[1].(string); ok {
			rightSQL, err := c.dataOp.valueToSQL(str)
			if err != nil {
				return "", fmt.Errorf("invalid string in IN operator: %w", err)
			}
			if literal, ok, err := jsonLogicInStringNeedleLiteral(leftArg); ok || err != nil {
				if err != nil {
					return "", fmt.Errorf("invalid left operand for string containment: %w", err)
				}
				needleSQL, err := c.dataOp.valueToSQL(literal)
				if err != nil {
					return "", fmt.Errorf("invalid left operand for string containment: %w", err)
				}
				return c.stringContainmentSQL(rightSQL, leftArg, needleSQL)
			}
		}
		if arr, ok := args[1].([]interface{}); ok {
			filtered := c.strictArrayMembershipItems(leftArg, arr)
			if len(filtered) == 0 {
				return boolSQL(false), nil
			}
			leftFieldName := c.extractFieldNameFromValue(leftArg)
			if leftFieldName != "" && c.schema().IsEnumType(leftFieldName) {
				if err := c.validateEnumArrayMembershipItems(leftFieldName, filtered); err != nil {
					return "", err
				}
			}
			if err := c.validateDefaultedEnumFieldOperand(leftArg); err != nil {
				return "", err
			}
			if sql, handled, err := c.defaultedFieldArrayLiteralMembershipSQL(leftArg, filtered); handled || err != nil {
				return sql, err
			}
		}
		leftSQL, err := c.valueToSQL(leftArg)
		if err != nil {
			return "", fmt.Errorf("invalid left operand: %w", err)
		}
		// Keep the left operand metadata available for enum validation.
		return c.handleIn(leftSQL, args[1], leftArg)
	}

	// Apply type coercion based on schema
	// If one side is a field and the other is a literal, coerce the literal to match the field type
	leftArg := args[0]
	rightArg := args[1]

	if isEqualityOperator(operator) {
		if sql, handled, err := c.defaultedFieldFieldEqualitySQL(operator, leftArg, rightArg, nil); handled || err != nil {
			return sql, err
		}
		if sql, handled, err := c.defaultedFieldLiteralEqualitySQL(operator, leftArg, rightArg, nil); handled || err != nil {
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

	leftSQL, err := c.valueToSQL(leftArg)
	if err != nil {
		return "", fmt.Errorf("invalid left operand: %w", err)
	}

	rightSQL, err := c.valueToSQL(rightArg)
	if err != nil {
		return "", fmt.Errorf("invalid right operand: %w", err)
	}

	// Handle NULL comparisons - use IS NULL/IS NOT NULL instead of = NULL/!= NULL
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
		// Handle NULL comparisons
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
		// Strict equality - same as == but handle NULL
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
		// Handle NULL comparisons
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
		// Strict inequality - same as != but handle NULL
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
		// Validate operands for ordering comparisons
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

func isOrderingOperator(operator string) bool {
	return IsOrderingComparisonOperatorName(operator)
}

// valueToSQL converts a value to SQL, handling both literals and var expressions.
func (c *ComparisonOperator) valueToSQL(value interface{}) (string, error) {
	// Check if it's a ProcessedValue (pre-processed SQL from parser)
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		// It's a literal, quote it
		return c.dataOp.valueToSQL(pv.Value)
	}

	// Check if it's a var expression
	if varExpr, ok := value.(map[string]interface{}); ok {
		if len(varExpr) != 1 {
			return "", fmt.Errorf("operator object must have exactly one key")
		}
		for operator, args := range varExpr {
			if operator == OpVar {
				// Special case: empty var name represents the current element in array operations
				if varName, ok := args.(string); ok && varName == "" {
					return ElemVar, nil
				}
				return c.dataOp.ToSQL(OpVar, []interface{}{args})
			}
		}
	}

	// Handle complex expressions (arithmetic, comparisons, etc.)
	// Note: Array operators (reduce, filter, some, etc.) should be pre-processed by the parser
	// into SQL strings, but if they're not, we'll return an error here
	if expr, ok := value.(map[string]interface{}); ok {
		if len(expr) == 1 {
			for op, args := range expr {
				switch op {
				case OpAdd, OpSubtract, OpMultiply, OpDivide, OpModulo:
					// Handle arithmetic operations
					return c.processArithmeticExpression(op, args)
				case OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual,
					OpEqual, OpStrictEqual, OpNotEqual, OpStrictNotEqual:
					// Handle comparison operations
					return c.processComparisonExpression(op, args)
				case OpMax, OpMin:
					// Handle min/max operations
					return c.processMinMaxExpression(op, args)
				case OpIf:
					// Handle if operator - delegate to logical operator
					if arr, ok := args.([]interface{}); ok {
						logicalOp := NewLogicalOperator(c.config)
						return logicalOp.ToSQL(OpIf, arr)
					}
					return "", fmt.Errorf("if operator requires array arguments")
				case OpReduce, OpFilter, OpMap, OpSome, OpAll, OpNone, OpMerge:
					// Array operators should have been pre-processed by the parser/logical operator
					// If we see them here, it means they weren't processed correctly
					// Try to process them directly as a fallback
					if arr, ok := args.([]interface{}); ok {
						arrayOp := NewArrayOperator(c.config)
						return arrayOp.ToSQL(op, arr)
					}
					return "", fmt.Errorf("array operator %s requires array arguments", op)
				case OpCat, OpSubstr:
					// Handle string operators
					if arr, ok := args.([]interface{}); ok {
						stringOp := NewStringOperator(c.config)
						return stringOp.ToSQL(op, arr)
					}
					return "", fmt.Errorf("string operator %s requires array arguments", op)
				default:
					// Try to use the expression parser callback for unknown operators
					// This enables support for custom operators in nested contexts
					if c.config != nil && c.config.HasExpressionParser() {
						return c.config.ParseExpression(expr, jsonPathRoot)
					}
					return "", fmt.Errorf("unsupported expression type in comparison: %s", op)
				}
			}
		}
	}

	// Handle arrays (for 'in' operator)
	if _, ok := value.([]interface{}); ok {
		return "", fmt.Errorf("arrays should be handled by handleIn method")
	}

	// Otherwise treat as literal value
	return c.dataOp.valueToSQL(value)
}

func isSQLStringLiteral(sql string) bool {
	trimmed := strings.TrimSpace(sql)
	return len(trimmed) >= 2 && strings.HasPrefix(trimmed, "'") && strings.HasSuffix(trimmed, "'")
}

func materializePredicateValueOperand(value interface{}) interface{} {
	pv, ok := value.(ProcessedValue)
	if !ok || !pv.IsSQL || !pv.HasExpressionInfo || pv.Kind != ExpressionKindPredicate {
		return value
	}
	pv.Value = PredicateValueSQL(pv.Value)
	pv.Kind = ExpressionKindValue
	pv.Type = ExpressionTypeBoolean
	return pv
}

func valueExpressionOperandSQL(value interface{}) (string, ExpressionType, bool) {
	pv, ok := value.(ProcessedValue)
	if !ok || !pv.IsSQL || !pv.HasExpressionInfo || pv.Kind != ExpressionKindValue {
		return "", ExpressionTypeUnknown, false
	}
	return pv.Value, pv.Type, true
}

func fieldExpressionOperandSQL(value interface{}) (ProcessedValue, bool) {
	pv, ok := value.(ProcessedValue)
	return pv, ok && pv.IsSQL && pv.IsField
}

func numericOrderingOperand(value interface{}) interface{} {
	if b, ok := value.(bool); ok {
		if b {
			return int64(1)
		}
		return int64(0)
	}
	pv, ok := value.(ProcessedValue)
	if !ok || !pv.IsSQL || !pv.HasExpressionInfo {
		return value
	}
	switch {
	case pv.Kind == ExpressionKindPredicate:
		pv.Value = PredicateNumberSQL(pv.Value)
	case pv.Type == ExpressionTypeBoolean:
		pv.Value = BooleanValueNumberSQL(pv.Value)
	default:
		return value
	}
	pv.Kind = ExpressionKindValue
	pv.Type = ExpressionTypeNumber
	return pv
}

// isStringLikeInOperandSchemaRequired classifies whether the left operand of "in"
// should be treated as a string in schema-required mode.
//
// This is intentionally heuristic (not a full type system). It covers:
// - literal strings
// - SQL string literals
// - ProcessedValue placeholders bound to string params
// - expressions rooted in known string-producing operators (cat/substr/if branches).
func (c *ComparisonOperator) isStringLikeInOperandSchemaRequired(
	value interface{},
	pc *params.ParamCollector,
	depth int,
) bool {
	// Keep recursive inference bounded on malformed/hostile input.
	if depth > 20 {
		return false
	}

	switch v := value.(type) {
	case string:
		return true
	case ProcessedValue:
		if v.HasExpressionInfo && v.Kind == ExpressionKindValue && v.Type == ExpressionTypeString {
			return true
		}
		if !v.IsSQL {
			return true
		}
		if isSQLStringLiteral(v.Value) {
			return true
		}
		if pc != nil {
			if paramValue, found := pc.ValueForPlaceholder(strings.TrimSpace(v.Value)); found {
				_, ok := paramValue.(string)
				return ok
			}
		}
		return false
	case map[string]interface{}:
		if len(v) != 1 {
			return false
		}
		for op, args := range v {
			switch op {
			case OpCat:
				arr, ok := args.([]interface{})
				return ok && len(arr) > 0
			case OpSubstr:
				arr, ok := args.([]interface{})
				return ok && len(arr) >= 2 && len(arr) <= 3
			case OpIf:
				arr, ok := args.([]interface{})
				if !ok || len(arr) < 3 {
					return false
				}
				// Branch results are at odd indexes and (for odd-length forms)
				// the trailing else value at the last index.
				for i := 1; i < len(arr); i += 2 {
					if !c.isStringLikeInOperandSchemaRequired(arr[i], pc, depth+1) {
						return false
					}
				}
				if len(arr)%2 == 1 {
					return c.isStringLikeInOperandSchemaRequired(arr[len(arr)-1], pc, depth+1)
				}
				return true
			default:
				return false
			}
		}
	}

	return false
}

// handleIn converts in operator to SQL
// leftOriginal is the original left argument (before SQL conversion) for enum validation.
func (c *ComparisonOperator) handleIn(leftSQL string, rightValue, leftOriginal interface{}) (string, error) {
	// Extract field name from left side for enum validation
	leftFieldName := c.extractFieldNameFromValue(leftOriginal)

	// Check if right side is a variable expression
	if varExpr, ok := rightValue.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			// Handle variable on right side
			// According to JSON Logic spec, "in" supports both:
			// 1. Array membership: {"in": [value, array]} → value IN array
			// 2. String containment: {"in": [substring, string]} → substring contained in string
			//
			// Use schema to determine the correct SQL:
			// - If variable is an ARRAY column: 'value' IN column (array membership)
			// - If variable is a STRING column: STRPOS(column, 'value') > 0 (string containment)
			rightSQL, err := c.dataOp.ToSQL(OpVar, []interface{}{varName})
			if err != nil {
				return "", fmt.Errorf("invalid variable in IN operator: %w", err)
			}

			// Extract field name from varName (handle both string and array cases)
			var fieldName string
			if nameStr, ok := varName.(string); ok {
				fieldName = nameStr
			} else if nameArr, ok := varName.([]interface{}); ok && len(nameArr) > 0 {
				if nameStr, ok := nameArr[0].(string); ok {
					fieldName = nameStr
				}
			}

			// Use schema to determine type if available
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
					// Array type: use null-safe JSONLogic element membership.
					return c.arrayMembershipSQL(leftSQL, rightSQL), nil
				} else if c.schema().IsStringType(fieldName) || c.schema().IsEnumType(fieldName) {
					// Coerce left side literal to string if needed (e.g., 123 → '123')
					coercedLeftSQL, err := c.stringContainmentNeedleSQL(leftOriginal, leftSQL)
					if err != nil {
						return "", fmt.Errorf("invalid left operand after coercion: %w", err)
					}
					// String type: use string containment syntax
					return c.stringContainmentSQL(rightSQL, leftOriginal, coercedLeftSQL)
				} else if c.schema().IsNumericType(fieldName) || c.schema().IsBooleanType(fieldName) {
					return boolSQL(false), nil
				}
			}

			// Unknown type: use heuristic based on left operand type.
			// Known string-producing expressions (cat/substr) and string literals
			// use containment; otherwise fall back to array membership.
			if c.isStringLikeInOperandSchemaRequired(leftOriginal, nil, 0) || isSQLStringLiteral(leftSQL) {
				// Use STRPOS/position for string containment
				needleSQL, err := c.stringContainmentNeedleSQL(leftOriginal, leftSQL)
				if err != nil {
					return "", fmt.Errorf("invalid left operand for string containment: %w", err)
				}
				return c.stringContainmentSQL(rightSQL, leftOriginal, needleSQL)
			}
			// Otherwise, assume array membership
			return c.arrayMembershipSQL(leftSQL, rightSQL), nil
		}
	}

	if rightField, ok := fieldExpressionOperandSQL(rightValue); ok {
		return c.handleInSQLRight(leftSQL, leftOriginal, rightField.Value, rightField.FieldName, rightField.Type, rightField.HasExpressionInfo)
	}

	if rightSQL, rightType, ok := valueExpressionOperandSQL(rightValue); ok {
		return c.handleInSQLRight(leftSQL, leftOriginal, rightSQL, "", rightType, true)
	}

	// Check if right side is an array
	if arr, ok := rightValue.([]interface{}); ok {
		if len(arr) == 0 {
			return boolSQL(false), nil
		}
		if c.isKnownArrayOperand(leftOriginal) {
			return boolSQL(false), nil
		}
		arr = c.strictArrayMembershipItems(leftOriginal, arr)
		if len(arr) == 0 {
			return boolSQL(false), nil
		}
		if leftFieldName != "" && c.schema().IsEnumType(leftFieldName) {
			if err := c.validateEnumArrayMembershipItems(leftFieldName, arr); err != nil {
				return "", err
			}
		}
		if err := c.validateDefaultedEnumFieldOperand(leftOriginal); err != nil {
			return "", err
		}
		if sql, handled, err := c.defaultedFieldArrayLiteralMembershipSQL(leftOriginal, arr); handled || err != nil {
			return sql, err
		}

		values, err := c.arrayMembershipItemSQLs(arr, nil)
		if err != nil {
			return "", err
		}

		return arrayLiteralMembershipSQL(leftSQL, values), nil
	}

	if str, ok := rightValue.(string); ok {
		rightSQL, err := c.dataOp.valueToSQL(str)
		if err != nil {
			return "", fmt.Errorf("invalid string in IN operator: %w", err)
		}
		needleSQL, err := c.stringContainmentNeedleSQL(leftOriginal, leftSQL)
		if err != nil {
			return "", fmt.Errorf("invalid left operand for string containment: %w", err)
		}
		return c.stringContainmentSQL(rightSQL, leftOriginal, needleSQL)
	}

	if nonContainer, err := nonContainerInHaystackLiteral(rightValue); err != nil {
		return "", fmt.Errorf("invalid non-container in IN operator: %w", err)
	} else if nonContainer {
		return boolSQL(false), nil
	}

	return "", fmt.Errorf("in operator requires array, variable, or string as second argument")
}

func (c *ComparisonOperator) handleInStringifiableLiteralNeedle(
	leftArg interface{},
	rightValue interface{},
) (string, bool, error) {
	literal, ok, err := jsonLogicInStringNeedleLiteral(leftArg)
	if !ok || err != nil {
		return "", ok, err
	}

	needleSQL, err := c.dataOp.valueToSQL(literal)
	if err != nil {
		return "", true, fmt.Errorf("invalid left operand for string containment: %w", err)
	}

	if varExpr, ok := rightValue.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			rightSQL, err := c.dataOp.ToSQL(OpVar, []interface{}{varName})
			if err != nil {
				return "", true, fmt.Errorf("invalid variable in IN operator: %w", err)
			}
			fieldName := c.extractFieldName(varName)
			if fieldName != "" {
				if c.schema().IsStringType(fieldName) || c.schema().IsEnumType(fieldName) {
					sql, err := c.stringContainmentSQL(rightSQL, leftArg, needleSQL)
					return sql, true, err
				}
				if c.schema().IsNumericType(fieldName) || c.schema().IsBooleanType(fieldName) {
					return boolSQL(false), true, nil
				}
			}
		}
	}

	if rightField, ok := fieldExpressionOperandSQL(rightValue); ok {
		return c.handleInStringifiableLiteralNeedleSQLRight(
			leftArg,
			needleSQL,
			rightField.Value,
			rightField.FieldName,
			rightField.Type,
			rightField.HasExpressionInfo,
		)
	}

	if rightSQL, rightType, ok := valueExpressionOperandSQL(rightValue); ok {
		return c.handleInStringifiableLiteralNeedleSQLRight(leftArg, needleSQL, rightSQL, "", rightType, true)
	}

	return "", false, nil
}

func (c *ComparisonOperator) handleInStringifiableLiteralNeedleSQLRight(
	leftArg interface{},
	needleSQL string,
	rightSQL string,
	fieldName string,
	rightType ExpressionType,
	hasRightType bool,
) (string, bool, error) {
	if fieldName != "" {
		if c.schema().IsStringType(fieldName) || c.schema().IsEnumType(fieldName) {
			sql, err := c.stringContainmentSQL(rightSQL, leftArg, needleSQL)
			return sql, true, err
		}
		if c.schema().IsNumericType(fieldName) || c.schema().IsBooleanType(fieldName) {
			return boolSQL(false), true, nil
		}
	}
	if hasRightType {
		switch rightType {
		case ExpressionTypeString:
			sql, err := c.stringContainmentSQL(rightSQL, leftArg, needleSQL)
			return sql, true, err
		case ExpressionTypeNull, ExpressionTypeBoolean, ExpressionTypeNumber, ExpressionTypeObject:
			return boolSQL(false), true, nil
		case ExpressionTypeArray, ExpressionTypeUnknown:
		}
	}
	return "", false, nil
}

func (c *ComparisonOperator) handleInSQLRight(
	leftSQL string,
	leftOriginal interface{},
	rightSQL string,
	fieldName string,
	rightType ExpressionType,
	hasRightType bool,
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
			return c.arrayMembershipSQL(leftSQL, rightSQL), nil
		}
		if c.schema().IsStringType(fieldName) || c.schema().IsEnumType(fieldName) {
			coercedLeftSQL, err := c.stringContainmentNeedleSQL(leftOriginal, leftSQL)
			if err != nil {
				return "", fmt.Errorf("invalid left operand after coercion: %w", err)
			}
			return c.stringContainmentSQL(rightSQL, leftOriginal, coercedLeftSQL)
		}
		if c.schema().IsNumericType(fieldName) || c.schema().IsBooleanType(fieldName) {
			return boolSQL(false), nil
		}
	}

	if hasRightType {
		switch rightType {
		case ExpressionTypeString:
			needleSQL, err := c.stringContainmentNeedleSQL(leftOriginal, leftSQL)
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

	if c.isStringLikeInOperandSchemaRequired(leftOriginal, nil, 0) || isSQLStringLiteral(leftSQL) {
		needleSQL, err := c.stringContainmentNeedleSQL(leftOriginal, leftSQL)
		if err != nil {
			return "", fmt.Errorf("invalid left operand for string containment: %w", err)
		}
		return c.stringContainmentSQL(rightSQL, leftOriginal, needleSQL)
	}
	return c.arrayMembershipSQL(leftSQL, rightSQL), nil
}

// handleChainedComparison handles chained comparisons like {"<": [10, {"var": "x"}, 20, 30]}
// For 2 args: generates "a < b"
// For 3+ args: generates "(a < b AND b < c AND c < d)".
func (c *ComparisonOperator) handleChainedComparison(operator string, args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("chained comparison requires at least 2 arguments")
	}

	// Validate operands for ordering comparisons
	for _, arg := range args {
		if err := c.validateOrderingOperand(arg, operator); err != nil {
			return "", err
		}
	}

	// Apply type coercion: find field names and coerce adjacent literals
	coercedArgs := make([]interface{}, len(args))
	copy(coercedArgs, args)

	// Find the field name from any var expression to use for coercion
	var fieldName string
	for _, arg := range args {
		if name := c.extractFieldNameFromValue(arg); name != "" {
			fieldName = name
			break
		}
	}

	// Coerce all literal arguments based on the field type
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

	// Convert all arguments to SQL
	var sqlArgs []string
	for i, arg := range coercedArgs {
		argSQL, err := c.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid argument %d: %w", i, err)
		}
		sqlArgs = append(sqlArgs, argSQL)
	}

	// For 2 arguments, return simple comparison without parentheses
	if len(args) == 2 {
		return fmt.Sprintf("%s %s %s", sqlArgs[0], operator, sqlArgs[1]), nil
	}

	// For 3+ arguments, generate chained comparisons with parentheses
	var conditions []string
	for i := 0; i < len(sqlArgs)-1; i++ {
		condition := fmt.Sprintf("%s %s %s", sqlArgs[i], operator, sqlArgs[i+1])
		conditions = append(conditions, condition)
	}

	return fmt.Sprintf("(%s)", strings.Join(conditions, sqlAndJoiner)), nil
}

// processArithmeticExpression handles arithmetic operations within comparison operations.
func (c *ComparisonOperator) processArithmeticExpression(op string, args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("arithmetic operation requires array of arguments")
	}

	// Handle unary minus (negation) - single argument case
	if op == "-" && len(argsSlice) == 1 {
		operand, err := c.valueToSQL(argsSlice[0])
		if err != nil {
			return "", fmt.Errorf("invalid unary minus argument: %w", err)
		}
		return fmt.Sprintf("(-%s)", operand), nil
	}

	// Handle unary plus (cast to number) - single argument case
	if op == "+" && len(argsSlice) == 1 {
		operand, err := c.valueToSQL(argsSlice[0])
		if err != nil {
			return "", fmt.Errorf("invalid unary plus argument: %w", err)
		}
		return fmt.Sprintf("CAST(%s AS NUMERIC)", operand), nil
	}

	if len(argsSlice) < 2 {
		return "", fmt.Errorf("arithmetic operation requires at least 2 arguments")
	}

	// Convert arguments to SQL
	operands := make([]string, len(argsSlice))
	for i, arg := range argsSlice {
		operand, err := c.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid arithmetic argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	// Generate SQL based on operation
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

// processComparisonExpression handles comparison operations within comparison operations.
func (c *ComparisonOperator) processComparisonExpression(op string, args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("comparison operation requires array of arguments")
	}

	if len(argsSlice) != 2 {
		return "", fmt.Errorf("comparison operation requires exactly 2 arguments")
	}

	sql, err := c.ToSQL(op, argsSlice)
	if err != nil {
		return "", err
	}
	if _, ok := sqlBooleanConstant(sql); ok {
		return sql, nil
	}
	return fmt.Sprintf("(%s)", sql), nil
}

// processMinMaxExpression handles min/max operations within comparison operations.
func (c *ComparisonOperator) processMinMaxExpression(op string, args interface{}) (string, error) {
	argsSlice, ok := args.([]interface{})
	if !ok {
		return "", fmt.Errorf("min/max operation requires array of arguments")
	}

	if len(argsSlice) < 2 {
		return "", fmt.Errorf("min/max operation requires at least 2 arguments")
	}

	// Convert arguments to SQL
	operands := make([]string, len(argsSlice))
	for i, arg := range argsSlice {
		operand, err := c.valueToSQL(arg)
		if err != nil {
			return "", fmt.Errorf("invalid min/max argument %d: %w", i, err)
		}
		operands[i] = operand
	}

	// Generate SQL based on operation
	switch op {
	case OpMax:
		return c.config.GreatestSQL(operands), nil
	case OpMin:
		return c.config.LeastSQL(operands), nil
	default:
		return "", fmt.Errorf("unsupported min/max operation: %s", op)
	}
}
