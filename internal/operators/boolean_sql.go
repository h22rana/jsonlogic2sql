package operators

import (
	"fmt"
	"strings"
)

const (
	predicateValuePrefix = "CASE WHEN "
	predicateValueSuffix = " THEN TRUE ELSE FALSE END"
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
