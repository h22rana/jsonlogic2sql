package operators

import (
	"testing"
)

func TestStringOperator_ToSQL(t *testing.T) {
	op := NewStringOperator()

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		expected string
		hasError bool
	}{
		// Concatenation tests
		{
			name:     "concatenation with two strings",
			operator: "cat",
			args:     []interface{}{"Hello", "World"},
			expected: "CONCAT('Hello', 'World')",
			hasError: false,
		},
		{
			name:     "concatenation with three strings",
			operator: "cat",
			args:     []interface{}{"Hello", " ", "World"},
			expected: "CONCAT('Hello', ' ', 'World')",
			hasError: false,
		},
		{
			name:     "concatenation with var and string",
			operator: "cat",
			args:     []interface{}{map[string]interface{}{"var": "firstName"}, " ", "Doe"},
			expected: "CONCAT(firstName, ' ', 'Doe')",
			hasError: false,
		},
		{
			name:     "concatenation with dotted var",
			operator: "cat",
			args:     []interface{}{map[string]interface{}{"var": "user.firstName"}, " ", map[string]interface{}{"var": "user.lastName"}},
			expected: "CONCAT(user.firstName, ' ', user.lastName)",
			hasError: false,
		},
		{
			name:     "concatenation with single string",
			operator: "cat",
			args:     []interface{}{"Hello"},
			expected: "CONCAT('Hello')",
			hasError: false,
		},
		{
			name:     "concatenation with no arguments",
			operator: "cat",
			args:     []interface{}{},
			expected: "",
			hasError: true,
		},

		// Substring tests
		{
			name:     "substring with start position",
			operator: "substr",
			args:     []interface{}{"Hello World", 6},
			expected: "SUBSTRING('Hello World', 6 + 1)",
			hasError: false,
		},
		{
			name:     "substring with start and length",
			operator: "substr",
			args:     []interface{}{"Hello World", 6, 5},
			expected: "SUBSTRING('Hello World', 6 + 1, 5)",
			hasError: false,
		},
		{
			name:     "substring with var and numbers",
			operator: "substr",
			args:     []interface{}{map[string]interface{}{"var": "fullName"}, 1, 5},
			expected: "SUBSTRING(fullName, 1 + 1, 5)",
			hasError: false,
		},
		{
			name:     "substring with dotted var",
			operator: "substr",
			args:     []interface{}{map[string]interface{}{"var": "user.email"}, 1, 10},
			expected: "SUBSTRING(user.email, 1 + 1, 10)",
			hasError: false,
		},
		{
			name:     "substring with too few arguments",
			operator: "substr",
			args:     []interface{}{"Hello"},
			expected: "",
			hasError: true,
		},
		{
			name:     "substring with too many arguments",
			operator: "substr",
			args:     []interface{}{"Hello", 1, 2, 3},
			expected: "",
			hasError: true,
		},

		// Unsupported operator
		{
			name:     "unsupported operator",
			operator: "unsupported",
			args:     []interface{}{"Hello"},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.ToSQL(tt.operator, tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestStringOperator_valueToSQL(t *testing.T) {
	op := NewStringOperator()

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "literal string",
			input:    "Hello",
			expected: "'Hello'",
			hasError: false,
		},
		{
			name:     "literal number",
			input:    42,
			expected: "42",
			hasError: false,
		},
		{
			name:     "var expression",
			input:    map[string]interface{}{"var": "name"},
			expected: "name",
			hasError: false,
		},
		{
			name:     "dotted var expression",
			input:    map[string]interface{}{"var": "user.name"},
			expected: "user.name",
			hasError: false,
		},
		{
			name:     "non-var object",
			input:    map[string]interface{}{"other": "value"},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.valueToSQL(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}
