package operators

import (
	"fmt"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func (c *ComparisonOperator) stringContainmentSQL(
	haystackSQL string,
	needleOriginal interface{},
	needleSQL string,
) (string, error) {
	if literal, ok, err := jsonLogicInStringNeedleLiteral(needleOriginal); ok || err != nil {
		if err != nil {
			return "", err
		}
		if literal == "" {
			return nonNullStringSQL(haystackSQL), nil
		}
		return fmt.Sprintf("%s > 0", c.strposFunc(haystackSQL, needleSQL)), nil
	}
	if isSQLStringLiteral(needleSQL) {
		if strings.TrimSpace(needleSQL) == "''" {
			return nonNullStringSQL(haystackSQL), nil
		}
		return fmt.Sprintf("%s > 0", c.strposFunc(haystackSQL, needleSQL)), nil
	}
	if isSQLStringLiteral(haystackSQL) {
		if strings.TrimSpace(haystackSQL) == "''" {
			return fmt.Sprintf("(%s = '')", needleSQL), nil
		}
		return fmt.Sprintf("%s > 0", c.strposFunc(haystackSQL, needleSQL)), nil
	}
	return c.runtimeStringContainmentSQL(haystackSQL, needleSQL), nil
}

func (c *ComparisonOperator) stringContainmentSQLParamAuto(
	haystackSQL string,
	needleOriginal interface{},
	pc *params.ParamCollector,
) (string, error) {
	if literal, ok, err := jsonLogicInStringNeedleLiteral(needleOriginal); ok || err != nil {
		if err != nil {
			return "", err
		}
		if literal == "" {
			return nonNullStringSQL(haystackSQL), nil
		}
		needleSQL, needleErr := c.stringContainmentNeedleSQLParamAuto(needleOriginal, pc)
		if needleErr != nil {
			return "", needleErr
		}
		return fmt.Sprintf("%s > 0", c.strposFunc(haystackSQL, needleSQL)), nil
	}
	needleSQL, err := c.stringContainmentNeedleSQLParamAuto(needleOriginal, pc)
	if err != nil {
		return "", err
	}
	if isSQLStringLiteral(needleSQL) {
		if strings.TrimSpace(needleSQL) == "''" {
			return nonNullStringSQL(haystackSQL), nil
		}
		return fmt.Sprintf("%s > 0", c.strposFunc(haystackSQL, needleSQL)), nil
	}
	if isSQLStringLiteral(haystackSQL) {
		if strings.TrimSpace(haystackSQL) == "''" {
			return fmt.Sprintf("(%s = '')", needleSQL), nil
		}
		return fmt.Sprintf("%s > 0", c.strposFunc(haystackSQL, needleSQL)), nil
	}
	return c.runtimeStringContainmentSQL(haystackSQL, needleSQL), nil
}

func nonNullStringSQL(sql string) string {
	return fmt.Sprintf("(%s IS NOT NULL)", sql)
}

func (c *ComparisonOperator) runtimeStringContainmentSQL(haystackSQL, needleSQL string) string {
	return fmt.Sprintf(
		"((%s = '' AND %s) OR (%s != '' AND %s > 0))",
		needleSQL,
		nonNullStringSQL(haystackSQL),
		needleSQL,
		c.strposFunc(haystackSQL, needleSQL),
	)
}

func (c *ComparisonOperator) stringContainmentNeedleSQL(value interface{}, currentSQL string) (string, error) {
	if pv, ok := value.(ProcessedValue); ok {
		if !pv.IsSQL {
			return c.stringContainmentNeedleSQL(pv.Value, currentSQL)
		}
		if pv.HasExpressionInfo {
			return c.stringContainmentNeedleSQLForType(pv.Type, currentSQL)
		}
		if pv.IsField && pv.FieldName != "" {
			return c.stringContainmentNeedleSQLForField(pv.FieldName, currentSQL)
		}
		return currentSQL, nil
	}

	if fieldName := c.extractFieldNameFromValue(value); fieldName != "" {
		return c.stringContainmentNeedleSQLForField(fieldName, currentSQL)
	}

	if literal, ok, err := jsonLogicInStringNeedleLiteral(value); ok || err != nil {
		if err != nil {
			return "", err
		}
		return c.dataOp.valueToSQL(literal)
	}

	return currentSQL, nil
}

func (c *ComparisonOperator) stringContainmentNeedleSQLParam(
	value interface{},
	currentSQL string,
	pc *params.ParamCollector,
) (string, error) {
	if pv, ok := value.(ProcessedValue); ok {
		if !pv.IsSQL {
			return c.stringContainmentNeedleSQLParam(pv.Value, currentSQL, pc)
		}
		if pv.HasExpressionInfo {
			return c.stringContainmentNeedleSQLForType(pv.Type, currentSQL)
		}
		if pv.IsField && pv.FieldName != "" {
			return c.stringContainmentNeedleSQLForField(pv.FieldName, currentSQL)
		}
		return currentSQL, nil
	}

	if fieldName := c.extractFieldNameFromValue(value); fieldName != "" {
		return c.stringContainmentNeedleSQLForField(fieldName, currentSQL)
	}

	if literal, ok, err := jsonLogicInStringNeedleLiteral(value); ok || err != nil {
		if err != nil {
			return "", err
		}
		return c.dataOp.valueToSQLParam(literal, pc)
	}

	return currentSQL, nil
}

func (c *ComparisonOperator) stringContainmentNeedleSQLParamAuto(
	value interface{},
	pc *params.ParamCollector,
) (string, error) {
	if pv, ok := value.(ProcessedValue); ok && !pv.IsSQL {
		return c.stringContainmentNeedleSQLParamAuto(pv.Value, pc)
	}
	if literal, ok, err := jsonLogicInStringNeedleLiteral(value); ok || err != nil {
		if err != nil {
			return "", err
		}
		return c.dataOp.valueToSQLParam(literal, pc)
	}
	currentSQL, err := c.valueToSQLParam(value, pc)
	if err != nil {
		return "", err
	}
	return c.stringContainmentNeedleSQLParam(value, currentSQL, pc)
}

func (c *ComparisonOperator) stringContainmentNeedleSQLForField(fieldName, currentSQL string) (string, error) {
	switch {
	case c.schema().IsStringType(fieldName), c.schema().IsEnumType(fieldName):
		return c.config.CoalesceSQL(currentSQL, "'null'"), nil
	case c.schema().IsNumericType(fieldName):
		return c.config.CoalesceSQL(c.config.StringCast(currentSQL), "'null'"), nil
	case c.schema().IsBooleanType(fieldName):
		return BooleanValueStringSQL(currentSQL), nil
	case c.schema().IsArrayType(fieldName):
		return "", fmt.Errorf("string containment on incompatible array value")
	default:
		return currentSQL, nil
	}
}

func (c *ComparisonOperator) stringContainmentNeedleSQLForType(
	exprType ExpressionType,
	currentSQL string,
) (string, error) {
	switch exprType {
	case ExpressionTypeString:
		return c.config.CoalesceSQL(currentSQL, "'null'"), nil
	case ExpressionTypeNumber:
		return c.config.CoalesceSQL(c.config.StringCast(currentSQL), "'null'"), nil
	case ExpressionTypeBoolean:
		return BooleanValueStringSQL(currentSQL), nil
	case ExpressionTypeNull:
		return "'null'", nil
	case ExpressionTypeArray, ExpressionTypeObject:
		return "", fmt.Errorf("string containment on incompatible %s value", expressionTypeName(exprType))
	case ExpressionTypeUnknown:
		return currentSQL, nil
	}
	return currentSQL, nil
}

func jsonLogicStringLiteral(value interface{}) (string, bool, error) {
	if pv, ok := value.(ProcessedValue); ok && !pv.IsSQL {
		return jsonLogicStringLiteral(pv.Value)
	}
	switch v := value.(type) {
	case string:
		return v, true, nil
	case nil:
		return "null", true, nil
	case bool:
		if v {
			return "true", true, nil
		}
		return "false", true, nil
	default:
		if literal, handled, valid := stringFieldNumericLiteralString(value); handled {
			if !valid {
				return "", true, fmt.Errorf("invalid number for string containment")
			}
			return literal, true, nil
		}
		return "", false, nil
	}
}

func jsonLogicInStringNeedleLiteral(value interface{}) (string, bool, error) {
	if pv, ok := value.(ProcessedValue); ok && !pv.IsSQL {
		return jsonLogicInStringNeedleLiteral(pv.Value)
	}
	if arr, ok := value.([]interface{}); ok {
		parts := make([]string, len(arr))
		for i, item := range arr {
			part, ok, err := jsonLogicArrayElementStringLiteral(item)
			if !ok || err != nil {
				return "", ok, err
			}
			parts[i] = part
		}
		return strings.Join(parts, ","), true, nil
	}
	return jsonLogicStringLiteral(value)
}

func jsonLogicArrayElementStringLiteral(value interface{}) (string, bool, error) {
	if pv, ok := value.(ProcessedValue); ok && !pv.IsSQL {
		return jsonLogicArrayElementStringLiteral(pv.Value)
	}
	if value == nil {
		return "", true, nil
	}
	return jsonLogicInStringNeedleLiteral(value)
}
