package operators

import (
	"fmt"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

func TestComparisonOperator_arrayMembershipSQL(t *testing.T) {
	tests := []struct {
		name     string
		dialect  dialect.Dialect
		valueSQL string
		arraySQL string
		expected string
	}{
		{
			name:     "BigQuery - null-safe UNNEST",
			dialect:  dialect.DialectBigQuery,
			valueSQL: "'test'",
			arraySQL: "tags",
			expected: testNullSafeArrayMembershipSQL(dialect.DialectBigQuery, "'test'", "tags"),
		},
		{
			name:     "Spanner - null-safe UNNEST",
			dialect:  dialect.DialectSpanner,
			valueSQL: "'test'",
			arraySQL: "tags",
			expected: testNullSafeArrayMembershipSQL(dialect.DialectSpanner, "'test'", "tags"),
		},
		{
			name:     "PostgreSQL - null-safe UNNEST",
			dialect:  dialect.DialectPostgreSQL,
			valueSQL: "'test'",
			arraySQL: "tags",
			expected: testNullSafeArrayMembershipSQL(dialect.DialectPostgreSQL, "'test'", "tags"),
		},
		{
			name:     "DuckDB - null-safe UNNEST",
			dialect:  dialect.DialectDuckDB,
			valueSQL: "'test'",
			arraySQL: "tags",
			expected: testNullSafeArrayMembershipSQL(dialect.DialectDuckDB, "'test'", "tags"),
		},
		{
			name:     "ClickHouse - arrayExists",
			dialect:  dialect.DialectClickHouse,
			valueSQL: "'test'",
			arraySQL: "tags",
			expected: testNullSafeArrayMembershipSQL(dialect.DialectClickHouse, "'test'", "tags"),
		},
		{
			name:     "Unspecified dialect - fallback to null-safe UNNEST",
			dialect:  dialect.DialectUnspecified,
			valueSQL: "42",
			arraySQL: "numbers",
			expected: testNullSafeArrayMembershipSQL(dialect.DialectUnspecified, "42", "numbers"),
		},
		{
			name:     "nil config - fallback to null-safe UNNEST",
			dialect:  dialect.Dialect(0), // placeholder, will use nil config
			valueSQL: "'val'",
			arraySQL: "arr",
			expected: testNullSafeArrayMembershipSQL(dialect.DialectUnspecified, "'val'", "arr"),
		},
		{
			name:     "BigQuery - avoids value alias collision",
			dialect:  dialect.DialectBigQuery,
			valueSQL: "__j2s_member",
			arraySQL: "tags",
			expected: testNullSafeArrayMembershipSQLWithAliases(
				dialect.DialectBigQuery,
				"__j2s_members",
				"__j2s_member_1",
				"__j2s_member",
				"tags",
			),
		},
		{
			name:     "PostgreSQL - avoids value and table alias collisions",
			dialect:  dialect.DialectPostgreSQL,
			valueSQL: "__j2s_members",
			arraySQL: "tags",
			expected: testNullSafeArrayMembershipSQLWithAliases(
				dialect.DialectPostgreSQL,
				"__j2s_members_1",
				"__j2s_member_1",
				"__j2s_members",
				"tags",
			),
		},
		{
			name:     "DuckDB - avoids array alias collision",
			dialect:  dialect.DialectDuckDB,
			valueSQL: "needle",
			arraySQL: "__j2s_member",
			expected: testNullSafeArrayMembershipSQLWithAliases(
				dialect.DialectDuckDB,
				"__j2s_members",
				"__j2s_member_1",
				"needle",
				"__j2s_member",
			),
		},
		{
			name:     "ClickHouse - skips multiple collided aliases",
			dialect:  dialect.DialectClickHouse,
			valueSQL: "__j2s_member_1",
			arraySQL: "tags",
			expected: testNullSafeArrayMembershipSQLWithAliases(
				dialect.DialectClickHouse,
				"__j2s_members",
				"__j2s_member_2",
				"__j2s_member_1",
				"tags",
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var op *ComparisonOperator
			if tt.name == "nil config - fallback to null-safe UNNEST" {
				op = NewComparisonOperator(testFieldOnlyConfig())
			} else {
				config := NewOperatorConfig(tt.dialect, &fieldOnlySchemaProvider{})
				op = NewComparisonOperator(config)
			}
			result := op.arrayMembershipSQL(tt.valueSQL, tt.arraySQL)
			if result != tt.expected {
				t.Errorf("arrayMembershipSQL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestComparisonOperator_coerceValueForComparison(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"age":         "integer",
		"price":       "number",
		"name":        "string",
		"description": "string",
		"is_active":   "boolean",
		"tags":        "array",
	})

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name      string
		value     interface{}
		fieldName string
		expected  interface{}
	}{
		// String-to-number coercion for numeric fields
		{
			name:      "string integer to numeric field",
			value:     "50000",
			fieldName: "age",
			expected:  int64(50000),
		},
		{
			name:      "string float to numeric field",
			value:     "3.14",
			fieldName: "price",
			expected:  float64(3.14),
		},
		{
			name:      "non-numeric string to numeric field (no coercion)",
			value:     "hello",
			fieldName: "age",
			expected:  "hello",
		},
		// Number-to-string coercion for string fields
		{
			name:      "float64 integer to string field",
			value:     float64(5960),
			fieldName: "name",
			expected:  "5960",
		},
		{
			name:      "float64 fractional to string field",
			value:     float64(3.14),
			fieldName: "name",
			expected:  "3.14",
		},
		{
			name:      "float32 to string field",
			value:     float32(1.5),
			fieldName: "name",
			expected:  "1.5",
		},
		{
			name:      "int to string field",
			value:     42,
			fieldName: "name",
			expected:  "42",
		},
		{
			name:      "int8 to string field",
			value:     int8(10),
			fieldName: "name",
			expected:  "10",
		},
		{
			name:      "int16 to string field",
			value:     int16(100),
			fieldName: "name",
			expected:  "100",
		},
		{
			name:      "int32 to string field",
			value:     int32(1000),
			fieldName: "name",
			expected:  "1000",
		},
		{
			name:      "int64 to string field",
			value:     int64(9999),
			fieldName: "name",
			expected:  "9999",
		},
		{
			name:      "uint to string field",
			value:     uint(7),
			fieldName: "name",
			expected:  "7",
		},
		{
			name:      "uint8 to string field",
			value:     uint8(8),
			fieldName: "name",
			expected:  "8",
		},
		{
			name:      "uint16 to string field",
			value:     uint16(16),
			fieldName: "name",
			expected:  "16",
		},
		{
			name:      "uint32 to string field",
			value:     uint32(32),
			fieldName: "name",
			expected:  "32",
		},
		{
			name:      "uint64 to string field",
			value:     uint64(64),
			fieldName: "name",
			expected:  "64",
		},
		// No coercion cases
		{
			name:      "empty field name returns value as-is",
			value:     "test",
			fieldName: "",
			expected:  "test",
		},
		{
			name:      "boolean field - no coercion",
			value:     "true",
			fieldName: "is_active",
			expected:  "true",
		},
		{
			name:      "array field - no coercion",
			value:     "something",
			fieldName: "tags",
			expected:  "something",
		},
		{
			name:      "number already a number for numeric field",
			value:     42,
			fieldName: "age",
			expected:  42,
		},
		{
			name:      "string already a string for string field",
			value:     "hello",
			fieldName: "name",
			expected:  "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := op.coerceValueForComparison(tt.value, tt.fieldName)
			if fmt.Sprintf("%v", result) != fmt.Sprintf("%v", tt.expected) {
				t.Errorf("coerceValueForComparison() = %v (%T), want %v (%T)", result, result, tt.expected, tt.expected)
			}
		})
	}

	// A field-only schema has no type metadata, so no coercion is applied.
	opFieldOnly := NewComparisonOperator(testFieldOnlyConfig())
	result := opFieldOnly.coerceValueForComparison("50000", "age")
	if result != "50000" {
		t.Errorf("coerceValueForComparison() with field-only schema = %v, want '50000'", result)
	}
}

func TestComparisonOperator_validateEnumValue(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"status":  "string",
		"country": "string",
		"age":     "integer",
	})
	schema.enumValues["status"] = []string{"active", "inactive", "pending"}
	schema.enumValues["country"] = []string{"US", "UK", "JP"}

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name      string
		value     interface{}
		fieldName string
		hasError  bool
	}{
		{
			name:      "valid enum value",
			value:     "active",
			fieldName: "status",
			hasError:  false,
		},
		{
			name:      "another valid enum value",
			value:     "pending",
			fieldName: "status",
			hasError:  false,
		},
		{
			name:      "invalid enum value",
			value:     "deleted",
			fieldName: "status",
			hasError:  true,
		},
		{
			name:      "valid country enum value",
			value:     "US",
			fieldName: "country",
			hasError:  false,
		},
		{
			name:      "invalid country enum value",
			value:     "XX",
			fieldName: "country",
			hasError:  true,
		},
		{
			name:      "null value skips validation",
			value:     nil,
			fieldName: "status",
			hasError:  false,
		},
		{
			name:      "non-enum field skips validation",
			value:     "anything",
			fieldName: "age",
			hasError:  false,
		},
		{
			name:      "empty field name skips validation",
			value:     "something",
			fieldName: "",
			hasError:  false,
		},
		{
			name:      "non-string value converted to string for validation",
			value:     123,
			fieldName: "status",
			hasError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := op.validateEnumValue(tt.value, tt.fieldName)
			if tt.hasError {
				if err == nil {
					t.Errorf("validateEnumValue() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("validateEnumValue() unexpected error = %v", err)
				}
			}
		})
	}

	// A field-only schema has no enum metadata, so enum validation is a no-op.
	opFieldOnly := NewComparisonOperator(testFieldOnlyConfig())
	if err := opFieldOnly.validateEnumValue("anything", "status"); err != nil {
		t.Errorf("validateEnumValue() with field-only schema should return nil, got %v", err)
	}
}

func TestComparisonOperator_extractFieldName(t *testing.T) {
	op := NewComparisonOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		varName  interface{}
		expected string
	}{
		{
			name:     "string var name",
			varName:  "fieldName",
			expected: "fieldName",
		},
		{
			name:     "array with string first element",
			varName:  []interface{}{"fieldName", "default"},
			expected: "fieldName",
		},
		{
			name:     "array with non-string first element",
			varName:  []interface{}{123, "default"},
			expected: "",
		},
		{
			name:     "empty array",
			varName:  []interface{}{},
			expected: "",
		},
		{
			name:     "numeric var name",
			varName:  42,
			expected: "",
		},
		{
			name:     "nil var name",
			varName:  nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := op.extractFieldName(tt.varName)
			if result != tt.expected {
				t.Errorf("extractFieldName() = %v, want %v", result, tt.expected)
			}
		})
	}
}
