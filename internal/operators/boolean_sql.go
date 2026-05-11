package operators

import (
	"fmt"
	"strings"
)

const (
	predicateValuePrefix = "CASE WHEN "
	predicateValueSuffix = " THEN TRUE ELSE FALSE END"
	predicateNumberTrue  = "1"
	predicateNumberFalse = "0"
)

// PredicateValueSQL converts nullable SQL predicate results into two-valued
// boolean value SQL. SQL CASE treats UNKNOWN like false, matching JSONLogic
// predicate values where comparisons return only true or false.
func PredicateValueSQL(sql string) string {
	switch strings.ToUpper(strings.TrimSpace(sql)) {
	case "TRUE":
		return "TRUE"
	case "FALSE":
		return "FALSE"
	}
	condition := StripRedundantOuterParens(sql)
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(condition)), "CASE ") {
		condition = fmt.Sprintf("(%s)", condition)
	}
	return fmt.Sprintf("%s%s%s", predicateValuePrefix, condition, predicateValueSuffix)
}

// PredicateNumberSQL converts a nullable SQL predicate into a numeric value.
// SQL CASE treats UNKNOWN like false, matching JSONLogic predicates before
// JavaScript-style boolean-to-number coercion.
func PredicateNumberSQL(sql string) string {
	if value, ok := sqlBooleanConstant(sql); ok {
		if value {
			return predicateNumberTrue
		}
		return predicateNumberFalse
	}
	condition := StripRedundantOuterParens(sql)
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(condition)), "CASE ") {
		condition = fmt.Sprintf("(%s)", condition)
	}
	return fmt.Sprintf("(CASE WHEN %s THEN %s ELSE %s END)", condition, predicateNumberTrue, predicateNumberFalse)
}

// BooleanValueNumberSQL converts a boolean SQL value expression into a numeric
// value. IS TRUE keeps NULL aligned with JavaScript's falsy boolean coercion.
func BooleanValueNumberSQL(sql string) string {
	if value, ok := sqlBooleanConstant(sql); ok {
		if value {
			return predicateNumberTrue
		}
		return predicateNumberFalse
	}
	expr := StripRedundantOuterParens(sql)
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(expr)), "CASE ") {
		expr = fmt.Sprintf("(%s)", expr)
	}
	return fmt.Sprintf("(CASE WHEN %s IS TRUE THEN %s ELSE %s END)", expr, predicateNumberTrue, predicateNumberFalse)
}
