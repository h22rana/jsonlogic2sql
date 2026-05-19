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

// BooleanValueStringSQL converts a boolean SQL value into JavaScript-style
// string coercion for JSONLogic string containment. SQL NULL represents
// JSONLogic null/missing and stringifies to "null" in this context.
func BooleanValueStringSQL(sql string) string {
	return booleanValueStringSQL(sql, "'null'")
}

// BooleanCatStringSQL converts a boolean SQL value into JSONLogic cat's
// join-style stringification. null/missing becomes empty, while false remains
// "false".
func BooleanCatStringSQL(sql string) string {
	return booleanValueStringSQL(sql, "''")
}

func booleanValueStringSQL(sql, nullSQL string) string {
	if value, ok := sqlBooleanConstant(sql); ok {
		if value {
			return "'true'"
		}
		return "'false'"
	}
	expr := StripRedundantOuterParens(sql)
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(expr)), "CASE ") {
		expr = fmt.Sprintf("(%s)", expr)
	}
	return fmt.Sprintf("CASE WHEN %s IS TRUE THEN 'true' WHEN %s IS FALSE THEN 'false' ELSE %s END", expr, expr, nullSQL)
}

// PredicateStringSQL converts a SQL predicate into JSONLogic-style string
// output. SQL CASE treats UNKNOWN like false, matching JSONLogic predicates.
func PredicateStringSQL(sql string) string {
	if value, ok := sqlBooleanConstant(sql); ok {
		if value {
			return "'true'"
		}
		return "'false'"
	}
	condition := StripRedundantOuterParens(sql)
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(condition)), "CASE ") {
		condition = fmt.Sprintf("(%s)", condition)
	}
	return fmt.Sprintf("CASE WHEN %s THEN 'true' ELSE 'false' END", condition)
}

// ConcatStringSQL renders an expression as one null-safe JSONLogic cat
// operand. JSONLogic cat stringifies null/undefined as an empty string, while
// SQL CONCAT can null-propagate in several supported dialects.
func ConcatStringSQL(config *OperatorConfig, sql string, kind ExpressionKind, typ ExpressionType) string {
	if kind == ExpressionKindPredicate {
		return PredicateStringSQL(sql)
	}
	if typ == ExpressionTypeBoolean {
		return BooleanCatStringSQL(sql)
	}

	expr := StripRedundantOuterParens(sql)
	if typ == ExpressionTypeNull || strings.EqualFold(strings.TrimSpace(expr), "NULL") {
		return "''"
	}
	if isSingleQuotedSQLLiteral(expr) {
		return expr
	}

	switch typ {
	case ExpressionTypeNull:
		return "''"
	case ExpressionTypeBoolean:
		return BooleanCatStringSQL(expr)
	case ExpressionTypeString:
		return fmt.Sprintf("COALESCE(%s, '')", expr)
	case ExpressionTypeNumber, ExpressionTypeUnknown:
		return fmt.Sprintf("COALESCE(%s, '')", config.StringCast(expr))
	case ExpressionTypeArray:
		return fmt.Sprintf("COALESCE(%s, '')", expr)
	}
	return fmt.Sprintf("COALESCE(%s, '')", expr)
}

func isSingleQuotedSQLLiteral(sql string) bool {
	trimmed := strings.TrimSpace(sql)
	if len(trimmed) < 2 || trimmed[0] != '\'' || trimmed[len(trimmed)-1] != '\'' {
		return false
	}
	for i := 1; i < len(trimmed)-1; i++ {
		if trimmed[i] != '\'' {
			continue
		}
		if i+1 < len(trimmed)-1 && trimmed[i+1] == '\'' {
			i++
			continue
		}
		return false
	}
	return true
}
