package operators

import (
	"fmt"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// ToSQLParam is the parameterized variant of ToSQL.
func (s *StringOperator) ToSQLParam(operator string, args []interface{}, pc *params.ParamCollector) (string, error) {
	switch operator {
	case "cat":
		return s.handleConcatenationParam(args, pc)
	case "substr":
		if len(args) == 0 {
			return "", fmt.Errorf("string operator %s requires at least one argument", operator)
		}
		return s.handleSubstringParam(args, pc)
	default:
		return "", fmt.Errorf("unsupported string operator: %s", operator)
	}
}

// handleConcatenationParam is the parameterized variant of handleConcatenation. Keep in sync.
func (s *StringOperator) handleConcatenationParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) == 0 {
		return "''", nil
	}
	for _, arg := range args {
		if err := s.validateStringOperand(arg); err != nil {
			return "", err
		}
	}
	operands := make([]string, len(args))
	for i, arg := range args {
		operand, err := s.valueToSQLForConcatParam(arg, pc)
		if err != nil {
			return "", fmt.Errorf("invalid concatenation argument %d: %w", i, err)
		}
		operands[i] = operand
	}
	return s.config.ConcatSQL(operands), nil
}

func (s *StringOperator) valueToSQLForConcatParam(value interface{}, pc *params.ParamCollector) (string, error) {
	if sql, handled, err := s.ifExpressionToConcatSQLParam(value, pc); handled || err != nil {
		return sql, err
	}
	sql, err := s.valueToSQLParam(value, pc)
	if err != nil {
		return "", err
	}
	if literalSQL, ok := s.literalConcatSQL(value, sql); ok {
		return literalSQL, nil
	}
	kind, typ := s.inferExpressionShape(value)
	return s.stringifyConcatSQL(sql, kind, typ), nil
}

func (s *StringOperator) ifExpressionToConcatSQLParam(
	value interface{},
	pc *params.ParamCollector,
) (string, bool, error) {
	args, handled, err := ifExpressionArgs(value)
	if !handled || err != nil {
		return "", handled, err
	}
	sql, err := s.processStringifiedIfExpressionParam(args, pc)
	return sql, true, err
}

func (s *StringOperator) processStringifiedIfExpressionParam(
	args []interface{},
	pc *params.ParamCollector,
) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("if operation requires at least 2 arguments (condition, then)")
	}

	var result strings.Builder
	result.WriteString("CASE")

	pairLimit := len(args)
	hasElse := len(args)%2 == 1
	if hasElse {
		pairLimit = len(args) - 1
	}

	for i := 0; i < pairLimit; i += 2 {
		condition, err := s.valueToSQLParam(args[i], pc)
		if err != nil {
			return "", fmt.Errorf("invalid if condition: %w", err)
		}

		thenValue, err := s.valueToSQLForConcatParam(args[i+1], pc)
		if err != nil {
			return "", fmt.Errorf("invalid if then value: %w", err)
		}

		result.WriteString(fmt.Sprintf(" WHEN %s THEN %s", condition, thenValue))
	}

	if hasElse {
		elseValue, err := s.valueToSQLForConcatParam(args[len(args)-1], pc)
		if err != nil {
			return "", fmt.Errorf("invalid if else value: %w", err)
		}
		result.WriteString(fmt.Sprintf(" ELSE %s", elseValue))
	} else {
		result.WriteString(" ELSE ''")
	}

	result.WriteString(" END")
	return result.String(), nil
}

// handleSubstringParam is the parameterized variant of handleSubstring. Keep in sync.
func (s *StringOperator) handleSubstringParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) < 2 || len(args) > 3 {
		return "", fmt.Errorf("substring requires 2 or 3 arguments")
	}
	if err := s.validateSubstringSourceOperand(args[0]); err != nil {
		return "", err
	}
	str, err := s.valueToSQLParam(args[0], pc)
	if err != nil {
		return "", fmt.Errorf("invalid substring string argument: %w", err)
	}
	if validationErr := s.validateSubstringIndexOperand(args[1], "start"); validationErr != nil {
		return "", validationErr
	}
	start, err := s.valueToSQLParam(args[1], pc)
	if err != nil {
		return "", fmt.Errorf("invalid substring start argument: %w", err)
	}
	startSQL := s.substringStartSQL(args[1], str, start, true)

	d := dialect.DialectUnspecified
	if s.config != nil {
		d = s.config.GetDialect()
	}
	var substrFunc string
	switch d {
	case dialect.DialectClickHouse:
		substrFunc = "substring"
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		substrFunc = "SUBSTR"
	}

	if len(args) == 3 {
		if err := s.validateSubstringIndexOperand(args[2], "length"); err != nil {
			return "", err
		}
		length, err := s.valueToSQLParam(args[2], pc)
		if err != nil {
			return "", fmt.Errorf("invalid substring length argument: %w", err)
		}
		length = s.substringLengthSQL(args[1], args[2], str, start, length, true)
		return fmt.Sprintf("%s(%s, %s, %s)", substrFunc, str, startSQL, length), nil
	}
	return fmt.Sprintf("%s(%s, %s)", substrFunc, str, startSQL), nil
}

// valueToSQLParam is the parameterized variant of valueToSQL. Keep in sync.
func (s *StringOperator) valueToSQLParam(value interface{}, pc *params.ParamCollector) (string, error) {
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		return s.dataOp.valueToSQLParam(pv.Value, pc)
	}

	if expr, ok := value.(map[string]interface{}); ok {
		if len(expr) != 1 {
			return "", fmt.Errorf("operator object must have exactly one key")
		}
		if varExpr, hasVar := expr[OpVar]; hasVar {
			return s.dataOp.ToSQLParam(OpVar, []interface{}{varExpr}, pc)
		}

		if len(expr) == 1 {
			for op, args := range expr {
				switch op {
				case "+", "-", "*", "/", "%":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("arithmetic operation requires array of arguments")
					}
					numOp := NewNumericOperator(s.config)
					return numOp.ToSQLParam(op, argsSlice, pc)
				case ">", ">=", "<", "<=", "==", "===", "!=", "!==":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("comparison operation requires array of arguments")
					}
					compOp := NewComparisonOperator(s.config)
					sql, err := compOp.ToSQLParam(op, argsSlice, pc)
					if err != nil {
						return "", err
					}
					return parenthesizeComparisonSQL(sql), nil
				case "if":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("if operation requires array of arguments")
					}
					logOp := NewLogicalOperator(s.config)
					return logOp.ToSQLParam("if", argsSlice, pc)
				case "substr":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("substr requires array of arguments")
					}
					return s.handleSubstringParam(argsSlice, pc)
				case "cat":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("cat requires array of arguments")
					}
					return s.handleConcatenationParam(argsSlice, pc)
				case "max", "min":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("%s requires array of arguments", op)
					}
					numOp := NewNumericOperator(s.config)
					return numOp.ToSQLParam(op, argsSlice, pc)
				case "and", "or":
					argsSlice, ok := args.([]interface{})
					if !ok {
						return "", fmt.Errorf("%s requires array of arguments", op)
					}
					logOp := NewLogicalOperator(s.config)
					return logOp.ToSQLParam(op, argsSlice, pc)
				case "!":
					argsSlice, ok := args.([]interface{})
					if ok {
						logOp := NewLogicalOperator(s.config)
						return logOp.ToSQLParam("!", argsSlice, pc)
					}
					logOp := NewLogicalOperator(s.config)
					return logOp.ToSQLParam("!", []interface{}{args}, pc)
				case "!!":
					argsSlice, ok := args.([]interface{})
					if ok {
						logOp := NewLogicalOperator(s.config)
						return logOp.ToSQLParam("!!", argsSlice, pc)
					}
					logOp := NewLogicalOperator(s.config)
					return logOp.ToSQLParam("!!", []interface{}{args}, pc)
				default:
					if s.config != nil && s.config.HasParamExpressionParser() {
						return s.config.ParseExpressionParam(expr, "$", pc)
					}
					return "", fmt.Errorf("unsupported expression type in string operation: %s", op)
				}
			}
		}
	}

	return s.dataOp.valueToSQLParam(value, pc)
}
