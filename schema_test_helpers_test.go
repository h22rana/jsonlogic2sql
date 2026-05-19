package jsonlogic2sql

import (
	"fmt"
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
		"((%s = '' AND (%s IS NOT NULL AND %s != '')) OR (%s != '' AND %s))",
		needle,
		haystack,
		haystack,
		needle,
		testStringContainmentSQL(d, haystack, needle),
	)
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
		{Name: "arr", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "arr1", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "arr2", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "array1", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "array2", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "items", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "numbers", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "scores", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "ages", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "emails", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "names", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "statuses", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "prices", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "readings", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "transactions", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "logs", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "flags", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "temperatures", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "moreNumbers", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "amounts", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "totals", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "cars", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "results", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "errors", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "nums", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "tags", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "values", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "data", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "groups", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "records", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "users", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "products", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "payments", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "accounts", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "bag.numbers", Type: FieldTypeArray, ElementFields: elementTestFields()},
		{Name: "bag.words", Type: FieldTypeArray, ElementFields: elementTestFields()},
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
