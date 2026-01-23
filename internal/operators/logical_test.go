package operators

import (
	"testing"
)

func TestLogicalOperator_ToSQL(t *testing.T) {
	op := NewLogicalOperator(nil)

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
			name:     "if with too many arguments",
			operator: "if",
			args:     []interface{}{true, "a", "b", "c"},
			expected: "CASE WHEN TRUE THEN 'a' ELSE NULL END",
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
	op := NewLogicalOperator(nil)

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
	op := NewLogicalOperator(nil)

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
	op := NewLogicalOperator(nil)

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
	op := NewLogicalOperator(nil)

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

func TestLogicalOperator_SchemaAwareTruthiness(t *testing.T) {
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
			expected: "(tags IS NOT NULL AND CARDINALITY(tags) > 0)",
			hasError: false,
		},
		{
			name:     "array field - items",
			args:     []interface{}{map[string]interface{}{"var": "items"}},
			expected: "(items IS NOT NULL AND CARDINALITY(items) > 0)",
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
	op := NewLogicalOperator(nil)

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
