package operators

import "testing"

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
