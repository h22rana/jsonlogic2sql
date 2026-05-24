package operators

// Element reference variable names used in array operations.
const (
	// ElemVar is the element variable name used in UNNEST subqueries.
	ElemVar = "elem"
	// ItemVar is the item variable name used in JSONLogic array operations.
	ItemVar = "item"
	// AccumulatorVar is the accumulator variable name used in reduce operations.
	AccumulatorVar = "accumulator"
	// CurrentVar is the current element variable name used in reduce operations.
	CurrentVar = "current"
)

// SQL aggregate function names.
const (
	// AggregateSUM is the SQL SUM aggregate function.
	AggregateSUM = "SUM"
	// AggregateMIN is the SQL MIN aggregate function.
	AggregateMIN = "MIN"
	// AggregateMAX is the SQL MAX aggregate function.
	AggregateMAX = "MAX"
)

const (
	jsonPathRoot = "$"

	sqlTrue      = "TRUE"
	sqlFalse     = "FALSE"
	sqlNull      = "NULL"
	sqlEmptyText = "''"
	sqlAndJoiner = " AND "
	sqlOrJoiner  = " OR "
)

const (
	literalKindNull    = "null"
	literalKindString  = SchemaTypeString
	literalKindBoolean = SchemaTypeBoolean
	literalKindNumber  = SchemaTypeNumber
	literalKindArray   = SchemaTypeArray
)

const (
	expressionTypeNameUnknown = "unknown"
)

const (
	jsonLogicStringNull  = "null"
	jsonLogicStringTrue  = "true"
	jsonLogicStringFalse = "false"
)

// JSONLogic operator names.
const (
	// OpVar is the variable access operator.
	OpVar = "var"
	// OpAnd is the logical AND operator.
	OpAnd = "and"
	// OpOr is the logical OR operator.
	OpOr = "or"
	// OpNot is the logical NOT operator.
	OpNot = "!"
	// OpDoubleBang is the boolean conversion operator.
	OpDoubleBang = "!!"
	// OpIf is the conditional operator.
	OpIf = "if"
	// OpMissing is the missing field check operator.
	OpMissing = "missing"
	// OpMissingSome is the missing some fields check operator.
	OpMissingSome = "missing_some"
)

// Arithmetic operator names.
const (
	// OpAdd is the addition operator.
	OpAdd = "+"
	// OpSubtract is the subtraction operator.
	OpSubtract = "-"
	// OpMultiply is the multiplication operator.
	OpMultiply = "*"
	// OpDivide is the division operator.
	OpDivide = "/"
	// OpModulo is the modulo operator.
	OpModulo = "%"
	// OpMax is the maximum value operator.
	OpMax = "max"
	// OpMin is the minimum value operator.
	OpMin = "min"
)

// Comparison operator names.
const (
	// OpEqual is the equality operator.
	OpEqual = "=="
	// OpStrictEqual is the strict equality operator.
	OpStrictEqual = "==="
	// OpNotEqual is the inequality operator.
	OpNotEqual = "!="
	// OpStrictNotEqual is the strict inequality operator.
	OpStrictNotEqual = "!=="
	// OpGreaterThan is the greater than operator.
	OpGreaterThan = ">"
	// OpGreaterThanOrEqual is the greater than or equal operator.
	OpGreaterThanOrEqual = ">="
	// OpLessThan is the less than operator.
	OpLessThan = "<"
	// OpLessThanOrEqual is the less than or equal operator.
	OpLessThanOrEqual = "<="
	// OpIn is the membership/containment operator.
	OpIn = "in"
)

// Array operator names.
const (
	// OpMap is the array map operator.
	OpMap = "map"
	// OpFilter is the array filter operator.
	OpFilter = "filter"
	// OpReduce is the array reduce operator.
	OpReduce = "reduce"
	// OpAll is the array all condition operator.
	OpAll = "all"
	// OpSome is the array some condition operator.
	OpSome = "some"
	// OpNone is the array none condition operator.
	OpNone = "none"
	// OpMerge is the array merge operator.
	OpMerge = "merge"
)

// String operator names.
const (
	// OpCat is the string concatenation operator.
	OpCat = "cat"
	// OpSubstr is the substring operator.
	OpSubstr = "substr"
)

// IsDataOperatorName reports whether name is a built-in data-access operator.
func IsDataOperatorName(name string) bool {
	switch name {
	case OpVar, OpMissing, OpMissingSome:
		return true
	default:
		return false
	}
}

// IsLogicalOperatorName reports whether name is a built-in logical operator.
func IsLogicalOperatorName(name string) bool {
	switch name {
	case OpAnd, OpOr, OpNot, OpDoubleBang, OpIf:
		return true
	default:
		return false
	}
}

// IsNumericOperatorName reports whether name is a built-in numeric operator.
func IsNumericOperatorName(name string) bool {
	switch name {
	case OpAdd, OpSubtract, OpMultiply, OpDivide, OpModulo, OpMax, OpMin:
		return true
	default:
		return false
	}
}

// IsComparisonOperatorName reports whether name is a built-in comparison or
// containment operator.
func IsComparisonOperatorName(name string) bool {
	switch name {
	case OpEqual, OpStrictEqual, OpNotEqual, OpStrictNotEqual,
		OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual,
		OpIn:
		return true
	default:
		return false
	}
}

// IsOrderingComparisonOperatorName reports whether name is a chained ordering
// comparison operator.
func IsOrderingComparisonOperatorName(name string) bool {
	switch name {
	case OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual:
		return true
	default:
		return false
	}
}

// IsArrayOperatorName reports whether name is a built-in array operator.
func IsArrayOperatorName(name string) bool {
	switch name {
	case OpMap, OpFilter, OpReduce, OpAll, OpSome, OpNone, OpMerge:
		return true
	default:
		return false
	}
}

// IsStringOperatorName reports whether name is a built-in string operator.
func IsStringOperatorName(name string) bool {
	switch name {
	case OpCat, OpSubstr:
		return true
	default:
		return false
	}
}

// IsBuiltInOperatorName reports whether name is reserved by the transpiler.
func IsBuiltInOperatorName(name string) bool {
	return IsDataOperatorName(name) ||
		IsLogicalOperatorName(name) ||
		IsNumericOperatorName(name) ||
		IsComparisonOperatorName(name) ||
		IsArrayOperatorName(name) ||
		IsStringOperatorName(name)
}
