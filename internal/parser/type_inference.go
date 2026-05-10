package parser

import "github.com/h22rana/jsonlogic2sql/internal/operators"

func (p *Parser) inferReduceResultType(args []interface{}) operators.ExpressionType {
	if len(args) != 3 {
		return operators.ExpressionTypeUnknown
	}
	initialType := p.inferValueExpressionType(args[2], operators.ExpressionTypeUnknown)
	reducerType := p.inferValueExpressionType(args[1], initialType)
	if reducerType != operators.ExpressionTypeUnknown {
		return reducerType
	}
	return initialType
}

func (p *Parser) inferValueExpressionType(expr interface{}, accumulatorType operators.ExpressionType) operators.ExpressionType {
	if pv, ok := expr.(operators.ProcessedValue); ok {
		if pv.HasExpressionInfo {
			if pv.Kind == operators.ExpressionKindPredicate {
				return operators.ExpressionTypeBoolean
			}
			return pv.Type
		}
		if pv.IsSQL {
			return operators.ExpressionTypeUnknown
		}
		return p.inferValueExpressionType(pv.Value, accumulatorType)
	}
	if typ, known, _ := literalTypeAndTruth(expr); known {
		return typ
	}
	if _, ok := expr.([]interface{}); ok {
		return operators.ExpressionTypeArray
	}
	obj, ok := expr.(map[string]interface{})
	if !ok || len(obj) != 1 {
		return operators.ExpressionTypeUnknown
	}
	for operator, args := range obj {
		switch operator {
		case operators.OpVar:
			return p.inferVarExpressionType(args, accumulatorType)
		case operators.OpMissing, operators.OpMissingSome,
			operators.OpEqual, operators.OpStrictEqual, operators.OpNotEqual, operators.OpStrictNotEqual,
			operators.OpGreaterThan, operators.OpGreaterThanOrEqual, operators.OpLessThan, operators.OpLessThanOrEqual,
			operators.OpIn, operators.OpNot, operators.OpDoubleBang,
			operators.OpAll, operators.OpSome, operators.OpNone:
			return operators.ExpressionTypeBoolean
		case operators.OpAnd, operators.OpOr:
			if arr, ok := args.([]interface{}); ok {
				return p.inferLogicalResultType(arr, accumulatorType)
			}
		case operators.OpIf:
			if arr, ok := args.([]interface{}); ok {
				return p.inferIfResultType(arr, accumulatorType)
			}
		case operators.OpAdd, operators.OpSubtract, operators.OpMultiply, operators.OpDivide, operators.OpModulo, operators.OpMax, operators.OpMin:
			return operators.ExpressionTypeNumber
		case operators.OpCat, operators.OpSubstr:
			return operators.ExpressionTypeString
		case operators.OpMap, operators.OpFilter, operators.OpMerge:
			return operators.ExpressionTypeArray
		case operators.OpReduce:
			if arr, ok := args.([]interface{}); ok {
				return p.inferReduceResultType(arr)
			}
		}
	}
	return operators.ExpressionTypeUnknown
}

func (p *Parser) inferVarExpressionType(args interface{}, accumulatorType operators.ExpressionType) operators.ExpressionType {
	fieldName := varFieldName(args)
	switch fieldName {
	case operators.AccumulatorVar:
		return accumulatorType
	case "", operators.CurrentVar, operators.ItemVar:
		return operators.ExpressionTypeUnknown
	}
	if typ := p.fieldExpressionType(fieldName); typ != operators.ExpressionTypeUnknown {
		return typ
	}
	if arr, ok := args.([]interface{}); ok && len(arr) > 1 {
		return p.inferValueExpressionType(arr[1], accumulatorType)
	}
	return operators.ExpressionTypeUnknown
}

func (p *Parser) inferLogicalResultType(args []interface{}, accumulatorType operators.ExpressionType) operators.ExpressionType {
	resultType := operators.ExpressionTypeUnknown
	typeSet := false
	for _, arg := range args {
		nextType := p.inferValueExpressionType(arg, accumulatorType)
		if !typeSet {
			resultType = nextType
			typeSet = true
		} else {
			resultType = mergeInferredTypes(resultType, nextType)
		}
		if typeSet && resultType == operators.ExpressionTypeUnknown {
			return resultType
		}
	}
	return resultType
}

func (p *Parser) inferIfResultType(args []interface{}, accumulatorType operators.ExpressionType) operators.ExpressionType {
	resultType := operators.ExpressionTypeUnknown
	typeSet := false
	for i := 1; i < len(args); i += 2 {
		nextType := p.inferValueExpressionType(args[i], accumulatorType)
		if !typeSet {
			resultType = nextType
			typeSet = true
		} else {
			resultType = mergeInferredTypes(resultType, nextType)
		}
		if typeSet && resultType == operators.ExpressionTypeUnknown {
			return resultType
		}
	}
	if len(args)%2 == 1 {
		nextType := p.inferValueExpressionType(args[len(args)-1], accumulatorType)
		if !typeSet {
			resultType = nextType
		} else {
			resultType = mergeInferredTypes(resultType, nextType)
		}
	}
	return resultType
}

func mergeInferredTypes(current, next operators.ExpressionType) operators.ExpressionType {
	if current == operators.ExpressionTypeUnknown || next == operators.ExpressionTypeUnknown {
		return operators.ExpressionTypeUnknown
	}
	if current == operators.ExpressionTypeNull {
		return next
	}
	if next == operators.ExpressionTypeNull || current == next {
		return current
	}
	return operators.ExpressionTypeUnknown
}
