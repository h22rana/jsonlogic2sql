package operators

import (
	"testing"
)

func TestStringOperator_ToSQL(t *testing.T) {
	op := NewStringOperator(nil)

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
			expected: "SUBSTR('Hello World', 7)",
			hasError: false,
		},
		{
			name:     "substring with start and length",
			operator: "substr",
			args:     []interface{}{"Hello World", 6, 5},
			expected: "SUBSTR('Hello World', 7, 5)",
			hasError: false,
		},
		{
			name:     "substring with var and numbers",
			operator: "substr",
			args:     []interface{}{map[string]interface{}{"var": "fullName"}, 1, 5},
			expected: "SUBSTR(fullName, 2, 5)",
			hasError: false,
		},
		{
			name:     "substring with dotted var",
			operator: "substr",
			args:     []interface{}{map[string]interface{}{"var": "user.email"}, 1, 10},
			expected: "SUBSTR(user.email, 2, 10)",
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
	op := NewStringOperator(nil)

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

func TestStringOperator_NestedOperations(t *testing.T) {
	op := NewStringOperator(nil)

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		expected string
		hasError bool
	}{
		// Nested substr inside cat
		{
			name:     "substr inside cat",
			operator: "cat",
			args: []interface{}{
				map[string]interface{}{"substr": []interface{}{map[string]interface{}{"var": "name"}, 0, 2}},
				"-",
				map[string]interface{}{"substr": []interface{}{map[string]interface{}{"var": "id"}, 0, 4}},
			},
			expected: "CONCAT(SUBSTR(name, 1, 2), '-', SUBSTR(id, 1, 4))",
			hasError: false,
		},
		// Nested cat inside cat
		{
			name:     "cat inside cat",
			operator: "cat",
			args: []interface{}{
				map[string]interface{}{"cat": []interface{}{"prefix-", map[string]interface{}{"var": "name"}}},
				"-suffix",
			},
			expected: "CONCAT(CONCAT('prefix-', name), '-suffix')",
			hasError: false,
		},
		// Nested cat inside substr
		{
			name:     "cat inside substr",
			operator: "substr",
			args: []interface{}{
				map[string]interface{}{"cat": []interface{}{map[string]interface{}{"var": "first"}, map[string]interface{}{"var": "last"}}},
				0,
				10,
			},
			expected: "SUBSTR(CONCAT(first, last), 1, 10)",
			hasError: false,
		},
		// Triple nesting: substr in cat in cat
		{
			name:     "triple nesting",
			operator: "cat",
			args: []interface{}{
				map[string]interface{}{
					"cat": []interface{}{
						map[string]interface{}{"substr": []interface{}{map[string]interface{}{"var": "code"}, 0, 2}},
						"-",
					},
				},
				map[string]interface{}{"substr": []interface{}{map[string]interface{}{"var": "id"}, 0, 4}},
			},
			expected: "CONCAT(CONCAT(SUBSTR(code, 1, 2), '-'), SUBSTR(id, 1, 4))",
			hasError: false,
		},
		// Multiple substr in cat
		{
			name:     "multiple substr in cat",
			operator: "cat",
			args: []interface{}{
				map[string]interface{}{"substr": []interface{}{map[string]interface{}{"var": "card"}, 0, 4}},
				"****",
				map[string]interface{}{"substr": []interface{}{map[string]interface{}{"var": "card"}, -4}},
			},
			expected: "CONCAT(SUBSTR(card, 1, 4), '****', SUBSTR(card, -3))",
			hasError: false,
		},
		// Max inside cat
		{
			name:     "max inside cat",
			operator: "cat",
			args: []interface{}{
				"Max: ",
				map[string]interface{}{"max": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			},
			expected: "CONCAT('Max: ', GREATEST(amount, 1000))",
			hasError: false,
		},
		// Min inside cat
		{
			name:     "min inside cat",
			operator: "cat",
			args: []interface{}{
				"Min: ",
				map[string]interface{}{"min": []interface{}{map[string]interface{}{"var": "value"}, 0}},
			},
			expected: "CONCAT('Min: ', LEAST(value, 0))",
			hasError: false,
		},
		// And inside if inside cat
		{
			name:     "and inside if inside cat",
			operator: "cat",
			args: []interface{}{
				"Status: ",
				map[string]interface{}{
					"if": []interface{}{
						map[string]interface{}{
							"and": []interface{}{
								map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "x"}, 0}},
								map[string]interface{}{"<": []interface{}{map[string]interface{}{"var": "x"}, 100}},
							},
						},
						"OK",
						"ERROR",
					},
				},
			},
			expected: "CONCAT('Status: ', CASE WHEN ((x > 0) AND (x < 100)) THEN 'OK' ELSE 'ERROR' END)",
			hasError: false,
		},
		// Or inside if inside cat
		{
			name:     "or inside if inside cat",
			operator: "cat",
			args: []interface{}{
				"Result: ",
				map[string]interface{}{
					"if": []interface{}{
						map[string]interface{}{
							"or": []interface{}{
								map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "type"}, "A"}},
								map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "type"}, "B"}},
							},
						},
						"VALID",
						"INVALID",
					},
				},
			},
			expected: "CONCAT('Result: ', CASE WHEN ((type = 'A') OR (type = 'B')) THEN 'VALID' ELSE 'INVALID' END)",
			hasError: false,
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
