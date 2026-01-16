package operators

import (
	"testing"
)

func TestNumericOperator_ToSQL(t *testing.T) {
	op := NewNumericOperator(nil)

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		expected string
		hasError bool
	}{
		// Addition tests
		{
			name:     "addition with two numbers",
			operator: "+",
			args:     []interface{}{5, 3},
			expected: "(5 + 3)",
			hasError: false,
		},
		{
			name:     "addition with three numbers",
			operator: "+",
			args:     []interface{}{1, 2, 3},
			expected: "(1 + 2 + 3)",
			hasError: false,
		},
		{
			name:     "addition with var and number",
			operator: "+",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, 100},
			expected: "(amount + 100)",
			hasError: false,
		},
		{
			name:     "addition with dotted var",
			operator: "+",
			args:     []interface{}{map[string]interface{}{"var": "user.score"}, 50},
			expected: "(user.score + 50)",
			hasError: false,
		},
		{
			name:     "unary plus (cast to number)",
			operator: "+",
			args:     []interface{}{5},
			expected: "CAST(5 AS NUMERIC)",
			hasError: false,
		},

		// Subtraction tests
		{
			name:     "subtraction with two numbers",
			operator: "-",
			args:     []interface{}{10, 3},
			expected: "(10 - 3)",
			hasError: false,
		},
		{
			name:     "subtraction with three numbers",
			operator: "-",
			args:     []interface{}{20, 5, 2},
			expected: "(20 - 5 - 2)",
			hasError: false,
		},
		{
			name:     "subtraction with var and number",
			operator: "-",
			args:     []interface{}{map[string]interface{}{"var": "balance"}, 50},
			expected: "(balance - 50)",
			hasError: false,
		},
		{
			name:     "unary minus (negation)",
			operator: "-",
			args:     []interface{}{10},
			expected: "(-10)",
			hasError: false,
		},
		{
			name:     "unary minus with var",
			operator: "-",
			args:     []interface{}{map[string]interface{}{"var": "value"}},
			expected: "(-value)",
			hasError: false,
		},

		// Multiplication tests
		{
			name:     "multiplication with two numbers",
			operator: "*",
			args:     []interface{}{4, 5},
			expected: "(4 * 5)",
			hasError: false,
		},
		{
			name:     "multiplication with three numbers",
			operator: "*",
			args:     []interface{}{2, 3, 4},
			expected: "(2 * 3 * 4)",
			hasError: false,
		},
		{
			name:     "multiplication with var and number",
			operator: "*",
			args:     []interface{}{map[string]interface{}{"var": "price"}, 1.2},
			expected: "(price * 1.2)",
			hasError: false,
		},
		{
			name:     "multiplication with too few arguments",
			operator: "*",
			args:     []interface{}{5},
			expected: "",
			hasError: true,
		},

		// Division tests
		{
			name:     "division with two numbers",
			operator: "/",
			args:     []interface{}{20, 4},
			expected: "(20 / 4)",
			hasError: false,
		},
		{
			name:     "division with three numbers",
			operator: "/",
			args:     []interface{}{100, 2, 5},
			expected: "(100 / 2 / 5)",
			hasError: false,
		},
		{
			name:     "division with var and number",
			operator: "/",
			args:     []interface{}{map[string]interface{}{"var": "total"}, 2},
			expected: "(total / 2)",
			hasError: false,
		},
		{
			name:     "division with too few arguments",
			operator: "/",
			args:     []interface{}{10},
			expected: "",
			hasError: true,
		},

		// Modulo tests
		{
			name:     "modulo with two numbers",
			operator: "%",
			args:     []interface{}{17, 5},
			expected: "(17 % 5)",
			hasError: false,
		},
		{
			name:     "modulo with var and number",
			operator: "%",
			args:     []interface{}{map[string]interface{}{"var": "count"}, 3},
			expected: "(count % 3)",
			hasError: false,
		},
		{
			name:     "modulo with wrong argument count",
			operator: "%",
			args:     []interface{}{17, 5, 2},
			expected: "",
			hasError: true,
		},
		{
			name:     "modulo with too few arguments",
			operator: "%",
			args:     []interface{}{17},
			expected: "",
			hasError: true,
		},

		// Max tests
		{
			name:     "max with two numbers",
			operator: "max",
			args:     []interface{}{10, 20},
			expected: "GREATEST(10, 20)",
			hasError: false,
		},
		{
			name:     "max with three numbers",
			operator: "max",
			args:     []interface{}{5, 15, 10},
			expected: "GREATEST(5, 15, 10)",
			hasError: false,
		},
		{
			name:     "max with var and numbers",
			operator: "max",
			args:     []interface{}{map[string]interface{}{"var": "score"}, 100, 50},
			expected: "GREATEST(score, 100, 50)",
			hasError: false,
		},
		{
			name:     "max with too few arguments",
			operator: "max",
			args:     []interface{}{10},
			expected: "",
			hasError: true,
		},

		// Min tests
		{
			name:     "min with two numbers",
			operator: "min",
			args:     []interface{}{10, 20},
			expected: "LEAST(10, 20)",
			hasError: false,
		},
		{
			name:     "min with three numbers",
			operator: "min",
			args:     []interface{}{5, 15, 10},
			expected: "LEAST(5, 15, 10)",
			hasError: false,
		},
		{
			name:     "min with var and numbers",
			operator: "min",
			args:     []interface{}{map[string]interface{}{"var": "score"}, 100, 50},
			expected: "LEAST(score, 100, 50)",
			hasError: false,
		},
		{
			name:     "min with too few arguments",
			operator: "min",
			args:     []interface{}{10},
			expected: "",
			hasError: true,
		},

		// Unsupported operator
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

func TestNumericOperator_valueToSQL(t *testing.T) {
	op := NewNumericOperator(nil)

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "literal number",
			input:    42,
			expected: "42",
			hasError: false,
		},
		{
			name:     "literal float",
			input:    3.14,
			expected: "3.14",
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
			input:    map[string]interface{}{"var": "user.score"},
			expected: "user.score",
			hasError: false,
		},
		{
			name:     "non-var object",
			input:    map[string]interface{}{"other": "value"},
			expected: "",
			hasError: true,
		},
		// Nested expression tests
		{
			name:     "nested unary minus",
			input:    map[string]interface{}{"-": []interface{}{map[string]interface{}{"var": "x"}}},
			expected: "(-x)",
			hasError: false,
		},
		{
			name:     "nested addition",
			input:    map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": "a"}, 5}},
			expected: "(a + 5)",
			hasError: false,
		},
		{
			name:     "multiplication with nested unary minus",
			input:    map[string]interface{}{"*": []interface{}{2, map[string]interface{}{"-": []interface{}{map[string]interface{}{"var": "x"}}}}},
			expected: "(2 * (-x))",
			hasError: false,
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
