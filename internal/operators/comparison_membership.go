package operators

import (
	"fmt"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

const (
	arrayMembershipElementAlias = "__j2s_member"
	arrayMembershipTableAlias   = "__j2s_members"
)

// arrayMembershipSQL generates JSONLogic-strict array membership SQL.
// JSONLogic array membership follows JavaScript indexOf semantics, so NULL
// members must compare as equal to a NULL needle instead of relying on SQL's
// nullable =/IN behavior.
func (c *ComparisonOperator) arrayMembershipSQL(valueSQL, arraySQL string) string {
	d := dialect.DialectUnspecified
	if c.config != nil {
		d = c.config.GetDialect()
	}

	memberAlias, tableAlias := arrayMembershipAliases(valueSQL, arraySQL)
	condition := nullSafeArrayMemberEqualitySQL(memberAlias, valueSQL)
	switch d {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("arrayExists(%s -> %s, %s)", memberAlias, condition, arraySQL)
	case dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		return fmt.Sprintf("EXISTS (SELECT 1 FROM UNNEST(%s) AS %s(%s) WHERE %s)",
			arraySQL, tableAlias, memberAlias, condition)
	case dialect.DialectUnspecified,
		dialect.DialectBigQuery,
		dialect.DialectSpanner:
		return fmt.Sprintf("EXISTS (SELECT 1 FROM UNNEST(%s) AS %s WHERE %s)",
			arraySQL, memberAlias, condition)
	}
	// Fallback for any future dialects
	return fmt.Sprintf("EXISTS (SELECT 1 FROM UNNEST(%s) AS %s WHERE %s)",
		arraySQL, memberAlias, condition)
}

func arrayMembershipAliases(valueSQL, arraySQL string) (string, string) {
	fragments := []string{valueSQL, arraySQL}
	memberAlias := uniqueInternalAlias(arrayMembershipElementAlias, fragments...)
	tableAlias := uniqueInternalAlias(arrayMembershipTableAlias, append(fragments, memberAlias)...)
	return memberAlias, tableAlias
}

func uniqueInternalAlias(base string, fragments ...string) string {
	for suffix := 0; ; suffix++ {
		alias := base
		if suffix > 0 {
			alias = fmt.Sprintf("%s_%d", base, suffix)
		}
		if !aliasAppearsInSQL(alias, fragments...) {
			return alias
		}
	}
}

func aliasAppearsInSQL(alias string, fragments ...string) bool {
	for _, fragment := range fragments {
		if strings.Contains(fragment, alias) {
			return true
		}
	}
	return false
}

func nullSafeArrayMemberEqualitySQL(memberSQL, valueSQL string) string {
	return fmt.Sprintf(
		"((%s IS NULL AND %s IS NULL) OR (%s IS NOT NULL AND %s IS NOT NULL AND %s = %s))",
		memberSQL,
		valueSQL,
		memberSQL,
		valueSQL,
		memberSQL,
		valueSQL,
	)
}

func arrayLiteralMembershipSQL(valueSQL string, itemSQLs []string) string {
	nonNullItems := make([]string, 0, len(itemSQLs))
	hasNull := false
	for _, itemSQL := range itemSQLs {
		if strings.EqualFold(strings.TrimSpace(itemSQL), sqlNull) {
			hasNull = true
			continue
		}
		nonNullItems = append(nonNullItems, itemSQL)
	}

	switch {
	case hasNull && len(nonNullItems) == 0:
		return fmt.Sprintf("%s IS NULL", valueSQL)
	case hasNull:
		return fmt.Sprintf("(%s IS NULL OR %s IN (%s))", valueSQL, valueSQL, strings.Join(nonNullItems, ", "))
	default:
		return fmt.Sprintf("%s IN (%s)", valueSQL, strings.Join(nonNullItems, ", "))
	}
}

// strposFunc returns the appropriate string position function call based on dialect.
// BigQuery/Spanner/DuckDB: STRPOS(haystack, needle)
// PostgreSQL: POSITION(needle IN haystack)
// ClickHouse: position(haystack, needle).
func (c *ComparisonOperator) strposFunc(haystack, needle string) string {
	d := dialect.DialectUnspecified
	if c.config != nil {
		d = c.config.GetDialect()
	}

	switch d {
	case dialect.DialectPostgreSQL:
		return fmt.Sprintf("POSITION(%s IN %s)", needle, haystack)
	case dialect.DialectClickHouse:
		return fmt.Sprintf("position(%s, %s)", haystack, needle)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectDuckDB:
		return fmt.Sprintf("STRPOS(%s, %s)", haystack, needle)
	}
	// Fallback for any future dialects
	return fmt.Sprintf("STRPOS(%s, %s)", haystack, needle)
}
