package operators

import (
	"testing"
)

func TestLogicalOperator_ToSQL(t *testing.T) {
	op := NewLogicalOperator()

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
	op := NewLogicalOperator()

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
	op := NewLogicalOperator()

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
