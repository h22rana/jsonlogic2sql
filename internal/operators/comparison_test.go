package operators

import (
	"testing"
)

func TestComparisonOperator_ToSQL(t *testing.T) {
	op := NewComparisonOperator()

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		expected string
		hasError bool
	}{
		// Equality tests
		{
			name:     "equality with numbers",
			operator: "==",
			args:     []interface{}{1, 2},
			expected: "1 = 2",
			hasError: false,
		},
		{
			name:     "equality with strings",
			operator: "==",
			args:     []interface{}{"hello", "world"},
			expected: "'hello' = 'world'",
			hasError: false,
		},
		{
			name:     "equality with var and literal",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "status"}, "pending"},
			expected: "status = 'pending'",
			hasError: false,
		},
		{
			name:     "equality with dotted var",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "user.status"}, "active"},
			expected: "user.status = 'active'",
			hasError: false,
		},
		// Inequality tests
		{
			name:     "inequality with numbers",
			operator: "!=",
			args:     []interface{}{1, 2},
			expected: "1 != 2",
			hasError: false,
		},

		// Greater than tests
		{
			name:     "greater than with numbers",
			operator: ">",
			args:     []interface{}{5, 3},
			expected: "5 > 3",
			hasError: false,
		},
		{
			name:     "greater than with var",
			operator: ">",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, 1000},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name:     "greater than with dotted var",
			operator: ">",
			args:     []interface{}{map[string]interface{}{"var": "transaction.amount"}, 5000},
			expected: "transaction.amount > 5000",
			hasError: false,
		},

		// Greater than or equal tests
		{
			name:     "greater than or equal",
			operator: ">=",
			args:     []interface{}{5, 5},
			expected: "5 >= 5",
			hasError: false,
		},
		{
			name:     "greater than or equal with var",
			operator: ">=",
			args:     []interface{}{map[string]interface{}{"var": "failedAttempts"}, 5},
			expected: "failedAttempts >= 5",
			hasError: false,
		},

		// Less than tests
		{
			name:     "less than with numbers",
			operator: "<",
			args:     []interface{}{3, 5},
			expected: "3 < 5",
			hasError: false,
		},
		{
			name:     "less than with var",
			operator: "<",
			args:     []interface{}{map[string]interface{}{"var": "age"}, 18},
			expected: "age < 18",
			hasError: false,
		},

		// Less than or equal tests
		{
			name:     "less than or equal",
			operator: "<=",
			args:     []interface{}{5, 5},
			expected: "5 <= 5",
			hasError: false,
		},
		{
			name:     "less than or equal with var",
			operator: "<=",
			args:     []interface{}{map[string]interface{}{"var": "user.accountAgeDays"}, 7},
			expected: "user.accountAgeDays <= 7",
			hasError: false,
		},

		// Boolean tests
		{
			name:     "equality with booleans",
			operator: "==",
			args:     []interface{}{true, false},
			expected: "TRUE = FALSE",
			hasError: false,
		},
		{
			name:     "equality with var and boolean",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "verified"}, false},
			expected: "verified = FALSE",
			hasError: false,
		},

		// Null tests
		{
			name:     "equality with null",
			operator: "==",
			args:     []interface{}{nil, nil},
			expected: "NULL = NULL",
			hasError: false,
		},
		{
			name:     "inequality with null",
			operator: "!=",
			args:     []interface{}{map[string]interface{}{"var": "field"}, nil},
			expected: "field != NULL",
			hasError: false,
		},

		// in operator tests
		{
			name:     "in with string array",
			operator: "in",
			args:     []interface{}{map[string]interface{}{"var": "country"}, []interface{}{"CN", "RU"}},
			expected: "country IN ('CN', 'RU')",
			hasError: false,
		},
		{
			name:     "in with number array",
			operator: "in",
			args:     []interface{}{map[string]interface{}{"var": "status"}, []interface{}{1, 2, 3}},
			expected: "status IN (1, 2, 3)",
			hasError: false,
		},
		{
			name:     "in with empty array",
			operator: "in",
			args:     []interface{}{map[string]interface{}{"var": "field"}, []interface{}{}},
			expected: "",
			hasError: true,
		},
		{
			name:     "in with string containment",
			operator: "in",
			args:     []interface{}{map[string]interface{}{"var": "field"}, "not-array"},
			expected: "POSITION(field IN 'not-array') > 0",
			hasError: false,
		},

		// Error cases
		{
			name:     "too few arguments",
			operator: "==",
			args:     []interface{}{1},
			expected: "",
			hasError: true,
		},
		{
			name:     "too many arguments",
			operator: "==",
			args:     []interface{}{1, 2, 3},
			expected: "",
			hasError: true,
		},
		{
			name:     "unsupported operator",
			operator: "unsupported",
			args:     []interface{}{1, 2},
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

func TestComparisonOperator_valueToSQL(t *testing.T) {
	op := NewComparisonOperator()

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "literal string",
			input:    "hello",
			expected: "'hello'",
			hasError: false,
		},
		{
			name:     "literal number",
			input:    42,
			expected: "42",
			hasError: false,
		},
		{
			name:     "literal boolean",
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
			name:     "dotted var expression",
			input:    map[string]interface{}{"var": "user.name"},
			expected: "user.name",
			hasError: false,
		},
		{
			name:     "var with default",
			input:    map[string]interface{}{"var": []interface{}{"status", "pending"}},
			expected: "COALESCE(status, 'pending')",
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
					t.Errorf("valueToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("valueToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("valueToSQL() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}
