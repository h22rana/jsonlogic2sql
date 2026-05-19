package operators

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestLogicalOperator_ToSQL(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		expected string
		hasError bool
	}{
		// and operator tests
		{
			name:     "and with single condition",
			operator: "and",
			args:     []interface{}{map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}}},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name:     "and with two conditions",
			operator: "and",
			args: []interface{}{
				map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 5000}},
				map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "pending"}},
			},
			expected: "(amount > 5000 AND status = 'pending')",
			hasError: false,
		},
		{
			name:     "and with three conditions",
			operator: "and",
			args: []interface{}{
				map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
				map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "active"}},
				map[string]interface{}{"!=": []interface{}{map[string]interface{}{"var": "verified"}, false}},
			},
			expected: "(amount > 1000 AND status = 'active' AND verified != FALSE)",
			hasError: false,
		},
		{
			name:     "and with no arguments",
			operator: "and",
			args:     []interface{}{},
			expected: "",
			hasError: true,
		},

		// or operator tests
		{
			name:     "or with single condition",
			operator: "or",
			args:     []interface{}{map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": "failedAttempts"}, 5}}},
			expected: "failedAttempts >= 5",
			hasError: false,
		},
		{
			name:     "or with two conditions",
			operator: "or",
			args: []interface{}{
				map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": "failedAttempts"}, 5}},
				map[string]interface{}{"in": []interface{}{map[string]interface{}{"var": "country"}, []interface{}{"CN", "RU"}}},
			},
			expected: "(failedAttempts >= 5 OR country IN ('CN', 'RU'))",
			hasError: false,
		},
		{
			name:     "or with no arguments",
			operator: "or",
			args:     []interface{}{},
			expected: "",
			hasError: true,
		},

		// not operator tests
		{
			name:     "not with simple condition",
			operator: "!",
			args:     []interface{}{map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "verified"}, true}}},
			expected: "NOT (verified = TRUE)",
			hasError: false,
		},
		{
			name:     "not with complex condition",
			operator: "!",
			args:     []interface{}{map[string]interface{}{"and": []interface{}{map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}}}}},
			expected: "NOT (amount > 1000)",
			hasError: false,
		},
		{
			name:     "not with wrong argument count",
			operator: "!",
			args:     []interface{}{true, false},
			expected: "",
			hasError: true,
		},

		// if operator tests
		{
			name:     "if with condition and then",
			operator: "if",
			args: []interface{}{
				map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "age"}, 18}},
				"adult",
			},
			expected: "CASE WHEN age > 18 THEN 'adult' ELSE NULL END",
			hasError: false,
		},
		{
			name:     "if with condition, then, and else",
			operator: "if",
			args: []interface{}{
				map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "age"}, 18}},
				"adult",
				"minor",
			},
			expected: "CASE WHEN age > 18 THEN 'adult' ELSE 'minor' END",
			hasError: false,
		},
		{
			name:     "if with boolean values",
			operator: "if",
			args: []interface{}{
				map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "verified"}, true}},
				true,
				false,
			},
			expected: "CASE WHEN verified = TRUE THEN TRUE ELSE FALSE END",
			hasError: false,
		},
		{
			name:     "if with numeric values",
			operator: "if",
			args: []interface{}{
				map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
				100,
				50,
			},
			expected: "CASE WHEN amount > 1000 THEN 100 ELSE 50 END",
			hasError: false,
		},
		{
			name:     "if with too few arguments",
			operator: "if",
			args:     []interface{}{true},
			expected: "",
			hasError: true,
		},
		{
			name:     "if with multiple condition-value pairs and no else",
			operator: "if",
			args:     []interface{}{true, "a", "b", "c"},
			expected: "CASE WHEN TRUE THEN 'a' WHEN 'b' THEN 'c' END",
			hasError: false,
		},

		// nested logical operators
		{
			name:     "nested and/or",
			operator: "and",
			args: []interface{}{
				map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "transaction.amount"}, 10000}},
				map[string]interface{}{"or": []interface{}{
					map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "user.verified"}, false}},
					map[string]interface{}{"<": []interface{}{map[string]interface{}{"var": "user.accountAgeDays"}, 7}},
				}},
			},
			expected: "(transaction.amount > 10000 AND (user.verified = FALSE OR user.accountAgeDays < 7))",
			hasError: false,
		},

		// unsupported operator
		{
			name:     "unsupported operator",
			operator: "unsupported",
			args:     []interface{}{true},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.ToSQL(tt.operator, tt.args)

			if tt.hasError {
				if err == nil {
					t.Errorf("ToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("ToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("ToSQL() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestLogicalOperator_expressionToSQL(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "primitive string",
			input:    "hello",
			expected: "'hello'",
			hasError: false,
		},
		{
			name:     "primitive number",
			input:    42,
			expected: "42",
			hasError: false,
		},
		{
			name:     "primitive boolean",
			input:    true,
			expected: "TRUE",
			hasError: false,
		},
		{
			name:     "var expression",
			input:    map[string]interface{}{"var": "amount"},
			expected: "amount",
			hasError: false,
		},
		{
			name:     "comparison expression",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name:     "logical expression",
			input:    map[string]interface{}{"and": []interface{}{map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}}}},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name:     "array expression",
			input:    []interface{}{1, 2, 3},
			expected: "",
			hasError: true,
		},
		{
			name:     "unsupported operator",
			input:    map[string]interface{}{"unsupported": []interface{}{1, 2}},
			expected: "",
			hasError: true,
		},
		{
			name:     "multiple keys in object",
			input:    map[string]interface{}{"a": 1, "b": 2},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.expressionToSQL(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("expressionToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expressionToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("expressionToSQL() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestLogicalOperator_isPrimitive(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	tests := []struct {
		input    interface{}
		expected bool
	}{
		{"hello", true},
		{42, true},
		{true, true},
		{false, true},
		{nil, true},
		{3.14, true},
		{int64(123), true},
		{[]interface{}{1, 2}, false},
		{map[string]interface{}{"a": 1}, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := op.isPrimitive(tt.input)
			if result != tt.expected {
				t.Errorf("isPrimitive(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLogicalOperator_handleDoubleNot(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		args     []interface{}
		expected string
		hasError bool
	}{
		{
			name:     "empty array",
			args:     []interface{}{[]interface{}{}},
			expected: "FALSE",
			hasError: false,
		},
		{
			name:     "non-empty array",
			args:     []interface{}{[]interface{}{1, 2, 3}},
			expected: "TRUE",
			hasError: false,
		},
		{
			name:     "var expression",
			args:     []interface{}{map[string]interface{}{"var": "value"}},
			expected: "(value IS NOT NULL AND value != FALSE AND value != 0 AND value != '')",
			hasError: false,
		},
		{
			name:     "numeric literal",
			args:     []interface{}{42},
			expected: "(42 IS NOT NULL AND 42 != FALSE AND 42 != 0 AND 42 != '')",
			hasError: false,
		},
		{
			name:     "string literal",
			args:     []interface{}{"hello"},
			expected: "('hello' IS NOT NULL AND 'hello' != FALSE AND 'hello' != 0 AND 'hello' != '')",
			hasError: false,
		},
		{
			name:     "boolean true",
			args:     []interface{}{true},
			expected: "(TRUE IS NOT NULL AND TRUE != FALSE AND TRUE != 0 AND TRUE != '')",
			hasError: false,
		},
		{
			name:     "no arguments - error",
			args:     []interface{}{},
			expected: "",
			hasError: true,
		},
		{
			name:     "too many arguments - error",
			args:     []interface{}{1, 2},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.handleDoubleNot(tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("handleDoubleNot() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("handleDoubleNot() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("handleDoubleNot() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestLogicalOperator_handleIf_EdgeCases(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		args     []interface{}
		expected string
		hasError bool
	}{
		{
			name:     "simple if-then-else with nested conditions",
			args:     []interface{}{map[string]interface{}{"var": "flag"}, "yes", "no"},
			expected: "CASE WHEN flag THEN 'yes' ELSE 'no' END",
			hasError: false,
		},
		{
			name:     "if-then (two args)",
			args:     []interface{}{true, "result"},
			expected: "CASE WHEN TRUE THEN 'result' ELSE NULL END",
			hasError: false,
		},
		{
			name: "multiple condition-value pairs",
			args: []interface{}{
				map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 90}},
				"A",
				map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 80}},
				"B",
				"C",
			},
			expected: "CASE WHEN score > 90 THEN 'A' WHEN score > 80 THEN 'B' ELSE 'C' END",
			hasError: false,
		},
		{
			name: "multiple condition-value pairs without final else",
			args: []interface{}{
				map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 90}},
				"A",
				map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 80}},
				"B",
			},
			expected: "CASE WHEN score > 90 THEN 'A' WHEN score > 80 THEN 'B' END",
			hasError: false,
		},
		{
			name:     "insufficient args - error",
			args:     []interface{}{true},
			expected: "",
			hasError: true,
		},
		{
			name:     "no args - error",
			args:     []interface{}{},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.handleIf(tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("handleIf() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("handleIf() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("handleIf() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

// truthinessSchemaProvider is a configurable schema provider for truthiness tests.
type truthinessSchemaProvider struct {
	fields map[string]string // field name -> type
}

func (m *truthinessSchemaProvider) HasField(fieldName string) bool {
	_, exists := m.fields[fieldName]
	return exists
}

func (m *truthinessSchemaProvider) GetFieldType(fieldName string) string {
	return m.fields[fieldName]
}

func (m *truthinessSchemaProvider) ValidateField(_ string) error {
	return nil // Allow all fields for testing
}

func (m *truthinessSchemaProvider) IsArrayType(fieldName string) bool {
	return m.fields[fieldName] == "array"
}

func (m *truthinessSchemaProvider) IsStringType(fieldName string) bool {
	return m.fields[fieldName] == "string"
}

func (m *truthinessSchemaProvider) IsNumericType(fieldName string) bool {
	t := m.fields[fieldName]
	return t == "integer" || t == "number"
}

func (m *truthinessSchemaProvider) IsBooleanType(fieldName string) bool {
	return m.fields[fieldName] == "boolean"
}

func (m *truthinessSchemaProvider) IsEnumType(fieldName string) bool {
	return m.fields[fieldName] == "enum"
}

func (m *truthinessSchemaProvider) GetAllowedValues(_ string) []string {
	return nil
}

func (m *truthinessSchemaProvider) ValidateEnumValue(_, _ string) error {
	return nil
}

func TestLogicalOperator_SchemaRequiredTruthiness(t *testing.T) {
	schema := &truthinessSchemaProvider{
		fields: map[string]string{
			"is_verified":   "boolean",
			"is_active":     "boolean",
			"name":          "string",
			"email":         "string",
			"amount":        "integer",
			"price":         "number",
			"tags":          "array",
			"items":         "array",
			"unknown_field": "", // empty type
		},
	}

	config := &OperatorConfig{Schema: schema}
	op := NewLogicalOperator(config)

	tests := []struct {
		name     string
		args     []interface{}
		expected string
		hasError bool
	}{
		// Boolean type tests
		{
			name:     "boolean field - is_verified",
			args:     []interface{}{map[string]interface{}{"var": "is_verified"}},
			expected: "is_verified IS TRUE",
			hasError: false,
		},
		{
			name:     "boolean field - is_active",
			args:     []interface{}{map[string]interface{}{"var": "is_active"}},
			expected: "is_active IS TRUE",
			hasError: false,
		},
		// String type tests
		{
			name:     "string field - name",
			args:     []interface{}{map[string]interface{}{"var": "name"}},
			expected: "(name IS NOT NULL AND name != '')",
			hasError: false,
		},
		{
			name:     "string field - email",
			args:     []interface{}{map[string]interface{}{"var": "email"}},
			expected: "(email IS NOT NULL AND email != '')",
			hasError: false,
		},
		// Numeric type tests
		{
			name:     "integer field - amount",
			args:     []interface{}{map[string]interface{}{"var": "amount"}},
			expected: "(amount IS NOT NULL AND amount != 0)",
			hasError: false,
		},
		{
			name:     "number field - price",
			args:     []interface{}{map[string]interface{}{"var": "price"}},
			expected: "(price IS NOT NULL AND price != 0)",
			hasError: false,
		},
		// Array type tests
		{
			name:     "array field - tags",
			args:     []interface{}{map[string]interface{}{"var": "tags"}},
			expected: "(tags IS NOT NULL AND ARRAY_LENGTH(tags) > 0)",
			hasError: false,
		},
		{
			name:     "array field - items",
			args:     []interface{}{map[string]interface{}{"var": "items"}},
			expected: "(items IS NOT NULL AND ARRAY_LENGTH(items) > 0)",
			hasError: false,
		},
		// Unknown/empty type - fallback to generic
		{
			name:     "unknown type field - fallback to generic",
			args:     []interface{}{map[string]interface{}{"var": "unknown_field"}},
			expected: "(unknown_field IS NOT NULL AND unknown_field != FALSE AND unknown_field != 0 AND unknown_field != '')",
			hasError: false,
		},
		// Field not in schema - fallback to generic
		{
			name:     "field not in schema - fallback to generic",
			args:     []interface{}{map[string]interface{}{"var": "not_in_schema"}},
			expected: "(not_in_schema IS NOT NULL AND not_in_schema != FALSE AND not_in_schema != 0 AND not_in_schema != '')",
			hasError: false,
		},
		// Non-var expressions - fallback to generic
		{
			name:     "numeric literal - fallback to generic",
			args:     []interface{}{42},
			expected: "(42 IS NOT NULL AND 42 != FALSE AND 42 != 0 AND 42 != '')",
			hasError: false,
		},
		{
			name:     "comparison expression - fallback to generic",
			args:     []interface{}{map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 100}}},
			expected: "(amount > 100 IS NOT NULL AND amount > 100 != FALSE AND amount > 100 != 0 AND amount > 100 != '')",
			hasError: false,
		},
		// Var with default value - should still work
		{
			name:     "var with default value - boolean type",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"is_verified", false}}},
			expected: "COALESCE(is_verified, FALSE) IS TRUE",
			hasError: false,
		},
		{
			name:     "var with default value - string type",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"name", "default"}}},
			expected: "(COALESCE(name, 'default') IS NOT NULL AND COALESCE(name, 'default') != '')",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.handleDoubleNot(tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("handleDoubleNot() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("handleDoubleNot() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("handleDoubleNot() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestLogicalOperator_ClickHouseArrayTruthiness(t *testing.T) {
	schema := &truthinessSchemaProvider{
		fields: map[string]string{
			"tags": "array",
		},
	}

	config := NewOperatorConfig(5, schema) // 5 = DialectClickHouse
	op := NewLogicalOperator(config)

	result, err := op.handleDoubleNot([]interface{}{map[string]interface{}{"var": "tags"}})
	if err != nil {
		t.Errorf("handleDoubleNot() unexpected error = %v", err)
	}

	expected := "(tags IS NOT NULL AND length(tags) > 0)"
	if result != expected {
		t.Errorf("handleDoubleNot() = %v, want %v", result, expected)
	}
}

func TestLogicalOperator_extractVarFieldName(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		arg      interface{}
		expected string
	}{
		{
			name:     "simple var string",
			arg:      map[string]interface{}{"var": "fieldName"},
			expected: "fieldName",
		},
		{
			name:     "var with array - field name and default",
			arg:      map[string]interface{}{"var": []interface{}{"fieldName", "default"}},
			expected: "fieldName",
		},
		{
			name:     "nested field name",
			arg:      map[string]interface{}{"var": "user.profile.name"},
			expected: "user.profile.name",
		},
		{
			name:     "not a var expression",
			arg:      map[string]interface{}{">": []interface{}{1, 2}},
			expected: "",
		},
		{
			name:     "multiple keys - not valid",
			arg:      map[string]interface{}{"var": "field", "other": "value"},
			expected: "",
		},
		{
			name:     "primitive value",
			arg:      42,
			expected: "",
		},
		{
			name:     "empty array for var",
			arg:      map[string]interface{}{"var": []interface{}{}},
			expected: "",
		},
		{
			name:     "array with non-string first element",
			arg:      map[string]interface{}{"var": []interface{}{123, "default"}},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := op.extractVarFieldName(tt.arg)
			if result != tt.expected {
				t.Errorf("extractVarFieldName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLogicalOperator_expressionToSQL_Extended(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		// ProcessedValue SQL
		{
			name:     "ProcessedValue SQL",
			input:    ProcessedValue{Value: "some_expr > 0", IsSQL: true},
			expected: "some_expr > 0",
			hasError: false,
		},
		// ProcessedValue literal
		{
			name:     "ProcessedValue literal string",
			input:    ProcessedValue{Value: "hello", IsSQL: false},
			expected: "'hello'",
			hasError: false,
		},
		// Primitive nil
		{
			name:     "primitive nil",
			input:    nil,
			expected: "NULL",
			hasError: false,
		},
		// Primitive float
		{
			name:     "primitive float",
			input:    3.14,
			expected: "3.14",
			hasError: false,
		},
		// missing operator
		{
			name:     "missing operator",
			input:    map[string]interface{}{"missing": "field"},
			expected: "field IS NULL",
			hasError: false,
		},
		// missing_some operator
		{
			name:     "missing_some operator",
			input:    map[string]interface{}{"missing_some": []interface{}{1, []interface{}{"a", "b"}}},
			expected: "(a IS NULL OR b IS NULL)",
			hasError: false,
		},
		// missing_some non-array error
		{
			name:     "missing_some non-array error",
			input:    map[string]interface{}{"missing_some": "invalid"},
			expected: "",
			hasError: true,
		},
		// Numeric operator
		{
			name:     "numeric addition",
			input:    map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": "a"}, 5}},
			expected: "(a + 5)",
			hasError: false,
		},
		// Numeric operator non-array error
		{
			name:     "numeric operator non-array error",
			input:    map[string]interface{}{"+": "invalid"},
			expected: "",
			hasError: true,
		},
		// String cat operator
		{
			name:     "cat operator",
			input:    map[string]interface{}{"cat": []interface{}{"hello", " ", "world"}},
			expected: "CONCAT('hello', ' ', 'world')",
			hasError: false,
		},
		// String operator non-array error
		{
			name:     "cat operator non-array error",
			input:    map[string]interface{}{"cat": "invalid"},
			expected: "",
			hasError: true,
		},
		// Comparison operator non-array error
		{
			name:     "comparison non-array error",
			input:    map[string]interface{}{"==": "invalid"},
			expected: "",
			hasError: true,
		},
		// Logical operator non-array error for and/or/if
		{
			name:     "and operator non-array error",
			input:    map[string]interface{}{"and": "invalid"},
			expected: "",
			hasError: true,
		},
		// Unary ! with non-array arg (wraps in array)
		{
			name:     "not with non-array arg",
			input:    map[string]interface{}{"!": map[string]interface{}{"var": "verified"}},
			expected: "NOT (verified)",
			hasError: false,
		},
		// !! with non-array arg (wraps in array)
		{
			name:     "double not with non-array arg",
			input:    map[string]interface{}{"!!": map[string]interface{}{"var": "value"}},
			expected: "(value IS NOT NULL AND value != FALSE AND value != 0 AND value != '')",
			hasError: false,
		},
		// Invalid expression type
		{
			name:     "invalid expression type - struct",
			input:    struct{ Name string }{Name: "test"},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.expressionToSQL(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("expressionToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expressionToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("expressionToSQL() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestLogicalOperator_expressionToSQL_ArrayOperators(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewLogicalOperator(config)

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name: "map operator in logical context",
			input: map[string]interface{}{
				"map": []interface{}{
					map[string]interface{}{"var": "nums"},
					map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": ""}, 1}},
				},
			},
			expected: "ARRAY(SELECT (elem + 1) FROM UNNEST(nums) AS elem)",
			hasError: false,
		},
		{
			name:     "array operator non-array error",
			input:    map[string]interface{}{"map": "invalid"},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.expressionToSQL(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("expressionToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expressionToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("expressionToSQL() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestLogicalOperator_expressionToSQL_ExpressionParserCallback(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	config.SetExpressionParser(func(expr any, path string) (string, error) {
		return "CUSTOM_LOGICAL()", nil
	})
	op := NewLogicalOperator(config)

	// Unknown operator with expression parser should delegate
	result, err := op.expressionToSQL(map[string]interface{}{"toLower": []interface{}{map[string]interface{}{"var": "name"}}})
	if err != nil {
		t.Errorf("expressionToSQL() unexpected error = %v", err)
	}
	if result != "CUSTOM_LOGICAL()" {
		t.Errorf("expressionToSQL() = %v, want CUSTOM_LOGICAL()", result)
	}
}

func TestLogicalOperator_processArgs(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	tests := []struct {
		name        string
		args        []interface{}
		expectedLen int
		hasError    bool
	}{
		{
			name: "var expression kept as-is",
			args: []interface{}{
				map[string]interface{}{"var": "field"},
				42,
			},
			expectedLen: 2,
			hasError:    false,
		},
		{
			name: "complex expression converted to SQL",
			args: []interface{}{
				map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": "a"}, 5}},
				"literal",
			},
			expectedLen: 2,
			hasError:    false,
		},
		{
			name:        "primitive values kept as-is",
			args:        []interface{}{42, "hello", true},
			expectedLen: 3,
			hasError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.processArgs(tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("processArgs() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("processArgs() unexpected error = %v", err)
				}
				if len(result) != tt.expectedLen {
					t.Errorf("processArgs() returned %d args, want %d", len(result), tt.expectedLen)
				}
			}
		})
	}
}

func TestLogicalOperator_isPrimitive_Extended(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	// Test additional types for completeness
	tests := []struct {
		name     string
		input    interface{}
		expected bool
	}{
		{"int8", int8(1), true},
		{"int16", int16(1), true},
		{"int32", int32(1), true},
		{"uint", uint(1), true},
		{"uint8", uint8(1), true},
		{"uint16", uint16(1), true},
		{"uint32", uint32(1), true},
		{"uint64", uint64(1), true},
		{"float32", float32(1.0), true},
		{"struct", struct{}{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := op.isPrimitive(tt.input)
			if result != tt.expected {
				t.Errorf("isPrimitive(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLogicalOperator_handleIf_MultiCondition(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	// Test with 7 args (3 condition/value pairs + else)
	result, err := op.handleIf([]interface{}{
		map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 90}},
		"A",
		map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 80}},
		"B",
		map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 70}},
		"C",
		"F",
	})
	if err != nil {
		t.Errorf("handleIf() unexpected error = %v", err)
	}
	expected := "CASE WHEN score > 90 THEN 'A' WHEN score > 80 THEN 'B' WHEN score > 70 THEN 'C' ELSE 'F' END"
	if result != expected {
		t.Errorf("handleIf() = %v, want %v", result, expected)
	}
}

func TestLogicalOperator_ToSQLParam(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	tests := []struct {
		name      string
		operator  string
		args      []interface{}
		wantSQL   string
		wantError bool
	}{
		{
			name:     "and with processed SQL",
			operator: "and",
			args: []interface{}{
				ProcessedValue{Value: "status = @p1", IsSQL: true},
				ProcessedValue{Value: "age > @p2", IsSQL: true},
			},
			wantSQL:   "(status = @p1 AND age > @p2)",
			wantError: false,
		},
		{
			name:     "or with processed SQL",
			operator: "or",
			args: []interface{}{
				ProcessedValue{Value: "failedAttempts >= @p1", IsSQL: true},
				ProcessedValue{Value: "country IN (@p2)", IsSQL: true},
			},
			wantSQL:   "(failedAttempts >= @p1 OR country IN (@p2))",
			wantError: false,
		},
		{
			name:      "not with processed SQL",
			operator:  "!",
			args:      []interface{}{ProcessedValue{Value: "verified = TRUE", IsSQL: true}},
			wantSQL:   "NOT (verified = TRUE)",
			wantError: false,
		},
		{
			name:      "!! with processed SQL",
			operator:  "!!",
			args:      []interface{}{ProcessedValue{Value: "name", IsSQL: true}},
			wantSQL:   "(name IS NOT NULL AND name != FALSE AND name != 0 AND name != '')",
			wantError: false,
		},
		{
			name:     "if with processed SQL",
			operator: "if",
			args: []interface{}{
				ProcessedValue{Value: "age > @p1", IsSQL: true},
				ProcessedValue{Value: "adult", IsSQL: true},
				ProcessedValue{Value: "minor", IsSQL: true},
			},
			wantSQL:   "CASE WHEN age > @p1 THEN adult ELSE minor END",
			wantError: false,
		},
		{
			name:      "unsupported operator",
			operator:  "xor",
			args:      []interface{}{ProcessedValue{Value: "1", IsSQL: true}},
			wantError: true,
		},
		{
			name:      "and with no args",
			operator:  "and",
			args:      []interface{}{},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if tt.wantError {
				if err == nil {
					t.Fatalf("ToSQLParam() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ToSQLParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("ToSQLParam() = %q, want %q", got, tt.wantSQL)
			}
		})
	}
}

func TestLogicalOperator_expressionToSQLParam(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	t.Run("primitive string", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.expressionToSQLParam("hello", pc)
		if err != nil {
			t.Fatalf("expressionToSQLParam() error = %v", err)
		}
		if got != "@p1" {
			t.Errorf("got %q, want @p1", got)
		}
		ps := pc.Params()
		if len(ps) != 1 || ps[0].Value != "hello" {
			t.Errorf("Params() = %#v, want one param with value hello", ps)
		}
	})

	t.Run("primitive nil", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.expressionToSQLParam(nil, pc)
		if err != nil {
			t.Fatalf("expressionToSQLParam() error = %v", err)
		}
		if got != "NULL" {
			t.Errorf("got %q, want NULL", got)
		}
		if len(pc.Params()) != 0 {
			t.Errorf("expected no params, got %d", len(pc.Params()))
		}
	})

	t.Run("primitive bool true", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.expressionToSQLParam(true, pc)
		if err != nil {
			t.Fatalf("expressionToSQLParam() error = %v", err)
		}
		if got != "TRUE" {
			t.Errorf("got %q, want TRUE", got)
		}
	})

	t.Run("primitive bool false", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.expressionToSQLParam(false, pc)
		if err != nil {
			t.Fatalf("expressionToSQLParam() error = %v", err)
		}
		if got != "FALSE" {
			t.Errorf("got %q, want FALSE", got)
		}
	})

	t.Run("numeric value", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.expressionToSQLParam(int64(42), pc)
		if err != nil {
			t.Fatalf("expressionToSQLParam() error = %v", err)
		}
		if got != "@p1" {
			t.Errorf("got %q, want @p1", got)
		}
		ps := pc.Params()
		if len(ps) != 1 || ps[0].Value != int64(42) {
			t.Errorf("Params() = %#v, want one param with int64(42)", ps)
		}
	})
}

func TestLogicalOperator_handleIfParam(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	t.Run("simple if with else", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		args := []interface{}{
			ProcessedValue{Value: "score > @p0", IsSQL: true},
			"A",
			"B",
		}
		got, err := op.handleIfParam(args, pc)
		if err != nil {
			t.Fatalf("handleIfParam() error = %v", err)
		}
		want := "CASE WHEN score > @p0 THEN @p1 ELSE @p2 END"
		if got != want {
			t.Errorf("handleIfParam() = %q, want %q", got, want)
		}
		ps := pc.Params()
		if len(ps) != 2 || ps[0].Value != "A" || ps[1].Value != "B" {
			t.Errorf("Params() = %#v, want A and B string binds", ps)
		}
	})

	t.Run("multiple condition-value pairs without final else", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		args := []interface{}{
			map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 90}},
			"A",
			map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 80}},
			"B",
		}
		got, err := op.handleIfParam(args, pc)
		if err != nil {
			t.Fatalf("handleIfParam() error = %v", err)
		}
		want := "CASE WHEN score > @p1 THEN @p2 WHEN score > @p3 THEN @p4 END"
		if got != want {
			t.Errorf("handleIfParam() = %q, want %q", got, want)
		}
		wantParams := []params.QueryParam{
			{Name: "p1", Value: 90},
			{Name: "p2", Value: "A"},
			{Name: "p3", Value: 80},
			{Name: "p4", Value: "B"},
		}
		if !reflect.DeepEqual(pc.Params(), wantParams) {
			t.Errorf("Params() = %#v, want %#v", pc.Params(), wantParams)
		}
	})
}

func TestLogicalOperator_processArgsParam(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
		if m, ok := expr.(map[string]interface{}); ok {
			if varName, ok := m["var"]; ok {
				return fmt.Sprintf("%v", varName), nil
			}
		}
		return "MOCK_SQL", nil
	})
	op := NewLogicalOperator(config)

	t.Run("var preserved and comparison becomes SQL", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		varArg := map[string]interface{}{"var": "col"}
		args := []interface{}{
			varArg,
			map[string]interface{}{"==": []interface{}{
				map[string]interface{}{"var": "a"},
				int64(10),
			}},
		}
		out, err := op.processArgsParam(args, pc)
		if err != nil {
			t.Fatalf("processArgsParam() error = %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("len(out) = %d, want 2", len(out))
		}
		if !reflect.DeepEqual(out[0], varArg) {
			t.Errorf("var arg should be unchanged, got %#v want %#v", out[0], varArg)
		}
		pv, ok := out[1].(ProcessedValue)
		if !ok || !pv.IsSQL {
			t.Fatalf("second arg should be ProcessedValue SQL, got %#v", out[1])
		}
		if pv.Value != "a = @p1" {
			t.Errorf("comparison SQL = %q, want a = @p1", pv.Value)
		}
		if len(pc.Params()) != 1 || pc.Params()[0].Value != int64(10) {
			t.Errorf("params = %#v, want one bind int64(10)", pc.Params())
		}
	})

	t.Run("primitives pass through", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		args := []interface{}{int64(1), "x"}
		out, err := op.processArgsParam(args, pc)
		if err != nil {
			t.Fatalf("processArgsParam() error = %v", err)
		}
		if len(out) != 2 || out[0] != int64(1) || out[1] != "x" {
			t.Errorf("out = %#v, want unchanged primitives", out)
		}
	})

	t.Run("nested expression via mock parser", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		args := []interface{}{
			map[string]interface{}{"custom": []interface{}{"ignored"}},
		}
		out, err := op.processArgsParam(args, pc)
		if err != nil {
			t.Fatalf("processArgsParam() error = %v", err)
		}
		if len(out) != 1 {
			t.Fatalf("len(out) = %d, want 1", len(out))
		}
		pv, ok := out[0].(ProcessedValue)
		if !ok || !pv.IsSQL || pv.Value != "MOCK_SQL" {
			t.Errorf("got %#v, want ProcessedValue{MOCK_SQL, IsSQL:true}", out[0])
		}
	})
}
