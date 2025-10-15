package operators

import (
	"testing"
)

func TestDataOperator_ToSQL(t *testing.T) {
	op := NewDataOperator()

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		expected string
		hasError bool
	}{
		// var operator tests
		{
			name:     "var with simple string",
			operator: "var",
			args:     []interface{}{"amount"},
			expected: "amount",
			hasError: false,
		},
		{
			name:     "var with dotted string",
			operator: "var",
			args:     []interface{}{"transaction.amount"},
			expected: "transaction.amount",
			hasError: false,
		},
		{
			name:     "var with nested dotted string",
			operator: "var",
			args:     []interface{}{"user.account.age"},
			expected: "user.account.age",
			hasError: false,
		},
		{
			name:     "var with array and default",
			operator: "var",
			args:     []interface{}{[]interface{}{"amount", 0}},
			expected: "COALESCE(amount, 0)",
			hasError: false,
		},
		{
			name:     "var with array and string default",
			operator: "var",
			args:     []interface{}{[]interface{}{"status", "pending"}},
			expected: "COALESCE(status, 'pending')",
			hasError: false,
		},
		{
			name:     "var with array and boolean default",
			operator: "var",
			args:     []interface{}{[]interface{}{"verified", true}},
			expected: "COALESCE(verified, TRUE)",
			hasError: false,
		},
		{
			name:     "var with array and null default",
			operator: "var",
			args:     []interface{}{[]interface{}{"field", nil}},
			expected: "COALESCE(field, NULL)",
			hasError: false,
		},
		{
			name:     "var with empty array",
			operator: "var",
			args:     []interface{}{[]interface{}{}},
			expected: "",
			hasError: true,
		},
		{
			name:     "var with non-string first arg",
			operator: "var",
			args:     []interface{}{[]interface{}{123, 0}},
			expected: "",
			hasError: true,
		},
		{
			name:     "var with no args",
			operator: "var",
			args:     []interface{}{},
			expected: "",
			hasError: true,
		},

		// missing operator tests
		{
			name:     "missing with simple string",
			operator: "missing",
			args:     []interface{}{"field"},
			expected: "field IS NULL",
			hasError: false,
		},
		{
			name:     "missing with dotted string",
			operator: "missing",
			args:     []interface{}{"user.name"},
			expected: "user.name IS NULL",
			hasError: false,
		},
		{
			name:     "missing with wrong arg count",
			operator: "missing",
			args:     []interface{}{"field", "extra"},
			expected: "",
			hasError: true,
		},
		{
			name:     "missing with non-string arg",
			operator: "missing",
			args:     []interface{}{123},
			expected: "",
			hasError: true,
		},

		// missing_some operator tests
		{
			name:     "missing_some with single field",
			operator: "missing_some",
			args:     []interface{}{1, []interface{}{"field"}},
			expected: "(field IS NULL)",
			hasError: false,
		},
		{
			name:     "missing_some with multiple fields",
			operator: "missing_some",
			args:     []interface{}{2, []interface{}{"field1", "field2", "field3"}},
			expected: "(CASE WHEN field1 IS NULL THEN 1 ELSE 0 END + CASE WHEN field2 IS NULL THEN 1 ELSE 0 END + CASE WHEN field3 IS NULL THEN 1 ELSE 0 END) >= 2",
			hasError: false,
		},
		{
			name:     "missing_some with dotted fields",
			operator: "missing_some",
			args:     []interface{}{1, []interface{}{"user.name", "user.email"}},
			expected: "(user.name IS NULL OR user.email IS NULL)",
			hasError: false,
		},
		{
			name:     "missing_some with wrong arg count",
			operator: "missing_some",
			args:     []interface{}{1},
			expected: "",
			hasError: true,
		},
		{
			name:     "missing_some with non-number first arg",
			operator: "missing_some",
			args:     []interface{}{"1", []interface{}{"field"}},
			expected: "",
			hasError: true,
		},
		{
			name:     "missing_some with non-array second arg",
			operator: "missing_some",
			args:     []interface{}{1, "field"},
			expected: "",
			hasError: true,
		},
		{
			name:     "missing_some with empty array",
			operator: "missing_some",
			args:     []interface{}{1, []interface{}{}},
			expected: "",
			hasError: true,
		},
		{
			name:     "missing_some with non-string in array",
			operator: "missing_some",
			args:     []interface{}{1, []interface{}{"field", 123}},
			expected: "",
			hasError: true,
		},

		// unsupported operator
		{
			name:     "unsupported operator",
			operator: "unsupported",
			args:     []interface{}{"test"},
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

func TestDataOperator_convertVarName(t *testing.T) {
	op := NewDataOperator()

	tests := []struct {
		input    string
		expected string
	}{
		{"amount", "amount"},
		{"transaction.amount", "transaction.amount"},
		{"user.account.age", "user.account.age"},
		{"a.b.c.d", "a.b.c.d"},
		{"simple", "simple"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := op.convertVarName(tt.input)
			if result != tt.expected {
				t.Errorf("convertVarName(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDataOperator_valueToSQL(t *testing.T) {
	op := NewDataOperator()

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{"string", "hello", "'hello'", false},
		{"string with quotes", "he'llo", "'he''llo'", false},
		{"integer", 42, "42", false},
		{"float", 3.14, "3.14", false},
		{"boolean true", true, "TRUE", false},
		{"boolean false", false, "FALSE", false},
		{"null", nil, "NULL", false},
		{"int64", int64(123), "123", false},
		{"float32", float32(1.5), "1.5", false},
		{"unsupported type", []string{"a"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.valueToSQL(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("valueToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("valueToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("valueToSQL() = %s, expected %s", result, tt.expected)
				}
			}
		})
	}
}
