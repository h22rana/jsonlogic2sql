package jsonlogic2sql

import (
	"fmt"
	"strings"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

func mustNewSchema(fields []FieldSchema) *Schema {
	schema, err := NewSchema(fields)
	if err != nil {
		panic(err)
	}
	return schema
}

func mustTestTranspiler(tb testing.TB, d Dialect) *Transpiler {
	tb.Helper()
	tr, err := NewTranspiler(d, defaultTestSchema())
	if err != nil {
		tb.Fatalf("NewTranspiler(%s) error = %v", d, err)
	}
	return tr
}

func emptyTestSchema() *Schema {
	return mustNewSchema(nil)
}

func testSupportsGeneralReduce(d Dialect) bool {
	return d == DialectDuckDB || d == DialectClickHouse
}

func testStringContainmentSQL(d Dialect, haystack, needle string) string {
	switch d {
	case DialectPostgreSQL:
		return fmt.Sprintf("POSITION(%s IN %s) > 0", needle, haystack)
	case DialectClickHouse:
		return fmt.Sprintf("position(%s, %s) > 0", haystack, needle)
	case dialect.DialectUnspecified, DialectBigQuery, DialectSpanner, DialectDuckDB:
		return fmt.Sprintf("STRPOS(%s, %s) > 0", haystack, needle)
	}
	return fmt.Sprintf("STRPOS(%s, %s) > 0", haystack, needle)
}

func testRuntimeStringContainmentSQL(d Dialect, haystack, needle string) string {
	return fmt.Sprintf(
		"((%s = '' AND (%s IS NOT NULL)) OR (%s != '' AND %s))",
		needle,
		haystack,
		needle,
		testStringContainmentSQL(d, haystack, needle),
	)
}

func testNullSafeArrayMembershipSQL(d Dialect, valueSQL, arraySQL string) string {
	return testArrayMembershipSQLWithAlias(d, "__j2s_member", valueSQL, arraySQL)
}

func testArrayMembershipSQLWithAlias(d Dialect, memberAlias, valueSQL, arraySQL string) string {
	return testArrayMembershipSQLWithAliases(d, "__j2s_members", memberAlias, valueSQL, arraySQL)
}

func testArrayMembershipSQLWithAliases(d Dialect, tableAlias, memberAlias, valueSQL, arraySQL string) string {
	condition := testNullSafeArrayMemberEqualitySQL(memberAlias, valueSQL)
	switch d {
	case DialectClickHouse:
		return fmt.Sprintf("arrayExists(%s -> %s, %s)", memberAlias, condition, arraySQL)
	case DialectPostgreSQL, DialectDuckDB:
		return fmt.Sprintf(
			"EXISTS (SELECT 1 FROM UNNEST(%s) AS %s(%s) WHERE %s)",
			arraySQL,
			tableAlias,
			memberAlias,
			condition,
		)
	}
	return fmt.Sprintf(
		"EXISTS (SELECT 1 FROM UNNEST(%s) AS %s WHERE %s)",
		arraySQL,
		memberAlias,
		condition,
	)
}

func testNullSafeArrayMemberEqualitySQL(memberSQL, valueSQL string) string {
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

func testArrayLiteralMembershipSQL(valueSQL string, itemSQLs ...string) string {
	nonNullItems := make([]string, 0, len(itemSQLs))
	hasNull := false
	for _, itemSQL := range itemSQLs {
		if strings.EqualFold(strings.TrimSpace(itemSQL), "NULL") {
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

func testDuckDBUnnestSourceAliases(d Dialect, sql string) string {
	if d != DialectDuckDB {
		return sql
	}
	return testDuckDBColumnAliasUNNEST(sql)
}

func testDuckDBColumnAliasUNNEST(sql string) string {
	const prefix = "FROM UNNEST("

	var out strings.Builder
	start := 0
	for {
		idx := strings.Index(sql[start:], prefix)
		if idx < 0 {
			out.WriteString(sql[start:])
			return out.String()
		}
		idx += start
		openParen := idx + len("FROM UNNEST")
		closeParen := testMatchingSQLParen(sql, openParen)
		if closeParen < 0 {
			out.WriteString(sql[start:])
			return out.String()
		}

		aliasPrefixStart := closeParen + 1
		if !strings.HasPrefix(sql[aliasPrefixStart:], " AS ") {
			out.WriteString(sql[start : closeParen+1])
			start = closeParen + 1
			continue
		}

		aliasStart := aliasPrefixStart + len(" AS ")
		aliasEnd := aliasStart
		for aliasEnd < len(sql) && testIsIdentifierChar(sql[aliasEnd]) {
			aliasEnd++
		}
		alias := sql[aliasStart:aliasEnd]
		if !testIsArrayElementAlias(alias) || (aliasEnd < len(sql) && sql[aliasEnd] == '(') {
			out.WriteString(sql[start:aliasEnd])
			start = aliasEnd
			continue
		}

		out.WriteString(sql[start:aliasEnd])
		out.WriteString("(")
		out.WriteString(alias)
		out.WriteString(")")
		start = aliasEnd
	}
}

func testMatchingSQLParen(sql string, openParen int) int {
	if openParen >= len(sql) || sql[openParen] != '(' {
		return -1
	}
	depth := 0
	inString := false
	for i := openParen; i < len(sql); i++ {
		switch sql[i] {
		case '\'':
			if inString && i+1 < len(sql) && sql[i+1] == '\'' {
				i++
				continue
			}
			inString = !inString
		case '(':
			if !inString {
				depth++
			}
		case ')':
			if !inString {
				depth--
				if depth == 0 {
					return i
				}
			}
		}
	}
	return -1
}

func testIsIdentifierChar(ch byte) bool {
	return ch == '_' || (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func testIsArrayElementAlias(alias string) bool {
	if alias == "elem" {
		return true
	}
	if !strings.HasPrefix(alias, "elem") {
		return false
	}
	for _, ch := range alias[len("elem"):] {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return len(alias) > len("elem")
}

func defaultTestSchema() *Schema {
	return mustNewSchema([]FieldSchema{
		{Name: "a", Type: FieldTypeNumber},
		{Name: "b", Type: FieldTypeNumber},
		{Name: "c", Type: FieldTypeNumber},
		{Name: "x", Type: FieldTypeNumber},
		{Name: "y", Type: FieldTypeNumber},
		{Name: "z", Type: FieldTypeNumber},
		{Name: "e", Type: FieldTypeNumber},
		{Name: "min", Type: FieldTypeNumber},
		{Name: "max", Type: FieldTypeNumber},
		{Name: "temp", Type: FieldTypeNumber},
		{Name: "limit", Type: FieldTypeNumber},
		{Name: "score1", Type: FieldTypeNumber},
		{Name: "score2", Type: FieldTypeNumber},
		{Name: "score3", Type: FieldTypeNumber},
		{Name: "totalPrice", Type: FieldTypeNumber},
		{Name: "totalQuantity", Type: FieldTypeNumber},
		{Name: "slider", Type: FieldTypeNumber},
		{Name: "positive", Type: FieldTypeNumber},
		{Name: "negative", Type: FieldTypeNumber},
		{Name: "amount", Type: FieldTypeNumber},
		{Name: "field", Type: FieldTypeString},
		{Name: "field1", Type: FieldTypeString},
		{Name: "field2", Type: FieldTypeString},
		{Name: "deleted_at", Type: FieldTypeString},
		{Name: "first", Type: FieldTypeString},
		{Name: "last", Type: FieldTypeString},
		{Name: "shop", Type: FieldTypeString},
		{Name: "desc", Type: FieldTypeString},
		{Name: "formula", Type: FieldTypeString},
		{Name: "phone", Type: FieldTypeString},
		{Name: "address", Type: FieldTypeString},
		{Name: "role", Type: FieldTypeString},
		{Name: "region", Type: FieldTypeString},
		{Name: "char", Type: FieldTypeString},
		{Name: "nickname", Type: FieldTypeString},
		{Name: "s", Type: FieldTypeString},
		{Name: "t", Type: FieldTypeString},
		{Name: "bar", Type: FieldTypeString},
		{Name: "archived_at", Type: FieldTypeString},
		{Name: "gender", Type: FieldTypeString},
		{Name: "first_name", Type: FieldTypeString},
		{Name: "last_name", Type: FieldTypeString},
		{Name: "phrase", Type: FieldTypeString},
		{Name: "action", Type: FieldTypeString},
		{Name: "product", Type: FieldTypeString},
		{Name: "comparison", Type: FieldTypeString},
		{Name: "location", Type: FieldTypeString},
		{Name: "color2", Type: FieldTypeString},
		{Name: "qty", Type: FieldTypeNumber},
		{Name: "d", Type: FieldTypeNumber},
		{Name: "failedAttempts", Type: FieldTypeNumber},
		{Name: "country", Type: FieldTypeString},
		{Name: "price", Type: FieldTypeNumber},
		{Name: "price1", Type: FieldTypeNumber},
		{Name: "price2", Type: FieldTypeNumber},
		{Name: "tax_rate", Type: FieldTypeNumber},
		{Name: "quantity", Type: FieldTypeNumber},
		{Name: "count", Type: FieldTypeInteger},
		{Name: "age", Type: FieldTypeInteger},
		{Name: "score", Type: FieldTypeNumber},
		{Name: "value", Type: FieldTypeNumber},
		{Name: "total", Type: FieldTypeNumber},
		{Name: "tax", Type: FieldTypeNumber},
		{Name: "discount", Type: FieldTypeNumber},
		{Name: "balance", Type: FieldTypeNumber},
		{Name: "status", Type: FieldTypeString},
		{Name: "name", Type: FieldTypeString},
		{Name: "email", Type: FieldTypeString},
		{Name: "code", Type: FieldTypeString},
		{Name: "category", Type: FieldTypeString},
		{Name: "type", Type: FieldTypeString},
		{Name: "description", Type: FieldTypeString},
		{Name: "text", Type: FieldTypeString},
		{Name: "bio", Type: FieldTypeString},
		{Name: "col", Type: FieldTypeString},
		{Name: "column", Type: FieldTypeString},
		{Name: "firstName", Type: FieldTypeString},
		{Name: "lastName", Type: FieldTypeString},
		{Name: "active", Type: FieldTypeBoolean},
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "verified", Type: FieldTypeBoolean},
		{Name: "is_active", Type: FieldTypeBoolean},
		{Name: "is_verified", Type: FieldTypeBoolean},
		{Name: "arr", Type: FieldTypeArray},
		{Name: "arr1", Type: FieldTypeArray},
		{Name: "arr2", Type: FieldTypeArray},
		{Name: "array1", Type: FieldTypeArray},
		{Name: "array2", Type: FieldTypeArray},
		{Name: "items", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "numbers", Type: FieldTypeArray},
		{Name: "scores", Type: FieldTypeArray},
		{Name: "ages", Type: FieldTypeArray},
		{Name: "emails", Type: FieldTypeArray},
		{Name: "names", Type: FieldTypeArray},
		{Name: "statuses", Type: FieldTypeArray},
		{Name: "prices", Type: FieldTypeArray},
		{Name: "readings", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "transactions", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "logs", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "flags", Type: FieldTypeArray},
		{Name: "temperatures", Type: FieldTypeArray},
		{Name: "moreNumbers", Type: FieldTypeArray},
		{Name: "amounts", Type: FieldTypeArray},
		{Name: "totals", Type: FieldTypeArray},
		{Name: "cars", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "results", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "errors", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "nums", Type: FieldTypeArray},
		{Name: "tags", Type: FieldTypeArray},
		{Name: "values", Type: FieldTypeArray},
		{Name: "data", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "groups", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "records", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "users", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "products", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "payments", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "accounts", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "bag.numbers", Type: FieldTypeArray},
		{Name: "bag.words", Type: FieldTypeArray},
		{Name: "bag.records", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "fixture.windowed_metrics.24h.events.total", Type: FieldTypeNumber},
		{Name: "stats.7d.10m.count", Type: FieldTypeNumber},
		{Name: "data.history.24h.tx.count", Type: FieldTypeNumber},
		{Name: "metrics.30d.total", Type: FieldTypeNumber},
		{Name: "order.history.daily.total", Type: FieldTypeNumber},
		{Name: "transaction.amount", Type: FieldTypeNumber},
		{Name: "profile", Type: FieldTypeObject, Fields: []FieldSchema{
			{Name: "first", Type: FieldTypeString},
			{Name: "last", Type: FieldTypeString},
			{Name: "name", Type: FieldTypeString},
			{Name: "status", Type: FieldTypeString},
			{Name: "age", Type: FieldTypeInteger},
			{Name: "score", Type: FieldTypeNumber},
			{Name: "region", Type: FieldTypeString},
		}},
		{Name: "user", Type: FieldTypeObject, Fields: []FieldSchema{
			{Name: "name", Type: FieldTypeString},
			{Name: "email", Type: FieldTypeString},
			{Name: "status", Type: FieldTypeString},
			{Name: "amount", Type: FieldTypeNumber},
			{Name: "score", Type: FieldTypeNumber},
			{Name: "verified", Type: FieldTypeBoolean},
			{Name: "accountAgeDays", Type: FieldTypeNumber},
			{Name: "items", Type: FieldTypeArray, ElementFields: elementTestFields()},
		}},
		{Name: "request", Type: FieldTypeObject, Fields: []FieldSchema{
			{Name: "params", Type: FieldTypeObject, Fields: []FieldSchema{
				{Name: "input_mode", Type: FieldTypeString},
				{Name: "category_code", Type: FieldTypeString},
				{Name: "is_verified", Type: FieldTypeBoolean},
			}},
		}},
		{Name: "metadata", Type: FieldTypeObject, Fields: []FieldSchema{
			{Name: "id", Type: FieldTypeString},
		}},
	})
}

func elementTestFields() []FieldSchema {
	return []FieldSchema{
		{Name: "type", Type: FieldTypeString},
		{Name: "name", Type: FieldTypeString},
		{Name: "label", Type: FieldTypeString},
		{Name: "code", Type: FieldTypeString},
		{Name: "email", Type: FieldTypeString},
		{Name: "category", Type: FieldTypeString},
		{Name: "desc", Type: FieldTypeString},
		{Name: "left", Type: FieldTypeString},
		{Name: "right", Type: FieldTypeString},
		{Name: "expected_status", Type: FieldTypeString},
		{Name: "safe_field", Type: FieldTypeString},
		{Name: "a", Type: FieldTypeNumber},
		{Name: "b", Type: FieldTypeNumber},
		{Name: "status", Type: FieldTypeString},
		{Name: "priority", Type: FieldTypeNumber},
		{Name: "role", Type: FieldTypeString},
		{Name: "vendor", Type: FieldTypeString},
		{Name: "product", Type: FieldTypeString},
		{Name: "age", Type: FieldTypeNumber},
		{Name: "year", Type: FieldTypeNumber},
		{Name: "count", Type: FieldTypeNumber},
		{Name: "value", Type: FieldTypeNumber},
		{Name: "amount", Type: FieldTypeNumber},
		{Name: "score", Type: FieldTypeNumber},
		{Name: "price", Type: FieldTypeNumber},
		{Name: "tax", Type: FieldTypeNumber},
		{Name: "discount", Type: FieldTypeNumber},
		{Name: "base", Type: FieldTypeNumber},
		{Name: "active", Type: FieldTypeBoolean},
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "tags", Type: FieldTypeArray},
		{Name: "values", Type: FieldTypeArray, ElementFields: []FieldSchema{
			{Name: "type", Type: FieldTypeString},
			{Name: "name", Type: FieldTypeString},
			{Name: "value", Type: FieldTypeNumber},
			{Name: "base", Type: FieldTypeNumber},
			{Name: "tags", Type: FieldTypeArray},
		}},
		{Name: "items", Type: FieldTypeArray, ElementFields: []FieldSchema{
			{Name: "score", Type: FieldTypeNumber},
			{Name: "value", Type: FieldTypeNumber},
			{Name: "active", Type: FieldTypeBoolean},
		}},
		{Name: "members", Type: FieldTypeArray, ElementFields: []FieldSchema{
			{Name: "role", Type: FieldTypeString},
		}},
		{Name: "subitems", Type: FieldTypeArray},
	}
}

type testSchemaMode struct {
	name   string
	schema *Schema
}

func schemaRequiredModes(schema *Schema) []testSchemaMode {
	return []testSchemaMode{
		{name: "schema-required", schema: schema},
	}
}
