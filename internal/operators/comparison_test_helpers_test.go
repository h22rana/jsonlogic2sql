package operators

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func testRuntimeStringContainmentSQL(d dialect.Dialect, haystack, needle string) string {
	var containment string
	switch d {
	case dialect.DialectPostgreSQL:
		containment = fmt.Sprintf("POSITION(%s IN %s) > 0", needle, haystack)
	case dialect.DialectClickHouse:
		containment = fmt.Sprintf("position(%s, %s) > 0", haystack, needle)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectDuckDB:
		containment = fmt.Sprintf("STRPOS(%s, %s) > 0", haystack, needle)
	}
	return fmt.Sprintf(
		"((%s = '' AND (%s IS NOT NULL)) OR (%s != '' AND %s))",
		needle,
		haystack,
		needle,
		containment,
	)
}

func testNullSafeArrayMembershipSQL(d dialect.Dialect, valueSQL, arraySQL string) string {
	return testNullSafeArrayMembershipSQLWithAliases(d, "__j2s_members", "__j2s_member", valueSQL, arraySQL)
}

func testNullSafeArrayMembershipSQLWithAliases(
	d dialect.Dialect,
	tableAlias string,
	memberAlias string,
	valueSQL string,
	arraySQL string,
) string {
	condition := fmt.Sprintf(
		"((%s IS NULL AND %s IS NULL) OR (%s IS NOT NULL AND %s IS NOT NULL AND %s = %s))",
		memberAlias,
		valueSQL,
		memberAlias,
		valueSQL,
		memberAlias,
		valueSQL,
	)
	if d == dialect.DialectClickHouse {
		return fmt.Sprintf("arrayExists(%s -> %s, %s)", memberAlias, condition, arraySQL)
	}
	if d == dialect.DialectPostgreSQL || d == dialect.DialectDuckDB {
		return fmt.Sprintf("EXISTS (SELECT 1 FROM UNNEST(%s) AS %s(%s) WHERE %s)", arraySQL, tableAlias, memberAlias, condition)
	}
	return fmt.Sprintf("EXISTS (SELECT 1 FROM UNNEST(%s) AS %s WHERE %s)", arraySQL, memberAlias, condition)
}

type comparisonSchemaProvider struct {
	fields      map[string]string   // field name -> type
	enumValues  map[string][]string // field name -> allowed values
	knownFields map[string]bool     // fields that exist
	validateErr error
}

func newComparisonSchemaProvider(fields map[string]string) *comparisonSchemaProvider {
	known := make(map[string]bool)
	for k := range fields {
		known[k] = true
	}
	return &comparisonSchemaProvider{
		fields:      fields,
		enumValues:  make(map[string][]string),
		knownFields: known,
	}
}

func (m *comparisonSchemaProvider) HasField(fieldName string) bool {
	return m.knownFields[fieldName]
}

func (m *comparisonSchemaProvider) GetFieldType(fieldName string) string {
	return m.fields[fieldName]
}

func (m *comparisonSchemaProvider) ValidateField(fieldName string) error {
	if m.validateErr != nil {
		return m.validateErr
	}
	if !m.HasField(fieldName) {
		return fmt.Errorf("field '%s' is not defined in schema", fieldName)
	}
	return nil
}

func (m *comparisonSchemaProvider) IsArrayType(fieldName string) bool {
	return m.fields[fieldName] == "array"
}

func (m *comparisonSchemaProvider) IsStringType(fieldName string) bool {
	return m.fields[fieldName] == "string"
}

func (m *comparisonSchemaProvider) IsNumericType(fieldName string) bool {
	t := m.fields[fieldName]
	return t == "integer" || t == "number"
}

func (m *comparisonSchemaProvider) IsBooleanType(fieldName string) bool {
	return m.fields[fieldName] == "boolean"
}

func (m *comparisonSchemaProvider) IsEnumType(fieldName string) bool {
	_, ok := m.enumValues[fieldName]
	return ok
}

func (m *comparisonSchemaProvider) GetAllowedValues(fieldName string) []string {
	return m.enumValues[fieldName]
}

func (m *comparisonSchemaProvider) ValidateEnumValue(fieldName, value string) error {
	allowed := m.enumValues[fieldName]
	for _, v := range allowed {
		if v == value {
			return nil
		}
	}
	return fmt.Errorf("invalid enum value '%s' for field '%s'", value, fieldName)
}

func assertQueryParams(t *testing.T, got, want []params.QueryParam) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Params() len %d, want %d\ngot: %#v", len(got), len(want), got)
	}
	for i := range got {
		if got[i].Name != want[i].Name || !reflect.DeepEqual(got[i].Value, want[i].Value) {
			t.Fatalf("Params()[%d] = {Name:%q Value:%#v (%T)}, want {Name:%q Value:%#v (%T)}",
				i, got[i].Name, got[i].Value, got[i].Value,
				want[i].Name, want[i].Value, want[i].Value)
		}
	}
}
