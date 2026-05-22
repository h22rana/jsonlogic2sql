package operators

import (
	"testing"
)

func TestStringOperator_ToSQL(t *testing.T) {
	op := NewStringOperator(testFieldOnlyConfig())

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
			expected: "CONCAT(COALESCE(CAST(firstName AS STRING), ''), ' ', 'Doe')",
			hasError: false,
		},
		{
			name:     "concatenation with dotted var",
			operator: "cat",
			args:     []interface{}{map[string]interface{}{"var": "user.firstName"}, " ", map[string]interface{}{"var": "user.lastName"}},
			expected: "CONCAT(COALESCE(CAST(user.firstName AS STRING), ''), ' ', COALESCE(CAST(user.lastName AS STRING), ''))",
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
			name:     "concatenation with null",
			operator: "cat",
			args:     []interface{}{nil, "World"},
			expected: "CONCAT('', 'World')",
			hasError: false,
		},
		{
			name:     "concatenation with number literal and nil config",
			operator: "cat",
			args:     []interface{}{1},
			expected: "CONCAT(CAST(1 AS STRING))",
			hasError: false,
		},
		{
			name:     "cat preserves comparison if branches",
			operator: "cat",
			args: []interface{}{
				map[string]interface{}{
					"==": []interface{}{
						map[string]interface{}{
							"if": []interface{}{
								map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "x"}, 0}},
								"a",
								map[string]interface{}{"<": []interface{}{map[string]interface{}{"var": "y"}, 0}},
								"b",
							},
						},
						"b",
					},
				},
			},
			expected: "CONCAT(CASE WHEN (CASE WHEN x > 0 THEN 'a' WHEN y < 0 THEN 'b' END = 'b') THEN 'true' ELSE 'false' END)",
			hasError: false,
		},
		{
			name:     "cat stringifies explicit if else branch",
			operator: "cat",
			args: []interface{}{
				map[string]interface{}{
					"if": []interface{}{
						map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "x"}, 1}},
						true,
						"fallback",
					},
				},
			},
			expected: "CONCAT(CASE WHEN (x = 1) THEN 'true' ELSE 'fallback' END)",
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
	op := NewStringOperator(testFieldOnlyConfig())

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

func TestStripRedundantOuterParens(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{name: "single wrapper", sql: "(amount = 'abc')", want: "amount = 'abc'"},
		{name: "nested wrapper", sql: "((amount = 'abc'))", want: "amount = 'abc'"},
		{name: "not whole expression", sql: "(a = 1) OR (b = 2)", want: "(a = 1) OR (b = 2)"},
		{name: "quoted parenthesis", sql: "(name = ')')", want: "name = ')'"},
		{name: "escaped quote", sql: "(name = 'a''b')", want: "name = 'a''b'"},
		{name: "scalar subquery", sql: "(SELECT value FROM UNNEST(arr) AS value)", want: "(SELECT value FROM UNNEST(arr) AS value)"},
		{name: "nested scalar subquery wrapper", sql: "((SELECT value FROM UNNEST(arr) AS value))", want: "(SELECT value FROM UNNEST(arr) AS value)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripRedundantOuterParens(tt.sql); got != tt.want {
				t.Fatalf("StripRedundantOuterParens() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStringOperator_NestedOperations(t *testing.T) {
	op := NewStringOperator(testFieldOnlyConfig())

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
			expected: "CONCAT(COALESCE(SUBSTR(name, 1, 2), ''), '-', COALESCE(SUBSTR(id, 1, 4), ''))",
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
			expected: "CONCAT(COALESCE(CONCAT('prefix-', COALESCE(CAST(name AS STRING), '')), ''), '-suffix')",
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
			expected: "SUBSTR(CONCAT(COALESCE(CAST(first AS STRING), ''), COALESCE(CAST(last AS STRING), '')), 1, 10)",
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
			expected: "CONCAT(COALESCE(CONCAT(COALESCE(SUBSTR(code, 1, 2), ''), '-'), ''), COALESCE(SUBSTR(id, 1, 4), ''))",
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
			expected: "CONCAT(COALESCE(SUBSTR(card, 1, 4), ''), '****', COALESCE(SUBSTR(card, -3), ''))",
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
			expected: "CONCAT('Max: ', COALESCE(CAST(GREATEST(amount, 1000) AS STRING), ''))",
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
			expected: "CONCAT('Min: ', COALESCE(CAST(LEAST(value, 0) AS STRING), ''))",
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
			expected: "CONCAT('Status: ', CASE WHEN (x > 0 AND x < 100) THEN 'OK' ELSE 'ERROR' END)",
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
			expected: "CONCAT('Result: ', CASE WHEN (type = 'A' OR type = 'B') THEN 'VALID' ELSE 'INVALID' END)",
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

func TestStringOperator_processArithmeticExpression(t *testing.T) {
	op := NewStringOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		operator string
		args     interface{}
		expected string
		hasError bool
	}{
		{
			name:     "addition",
			operator: "+",
			args:     []interface{}{map[string]interface{}{"var": "a"}, 5},
			expected: "(a + 5)",
			hasError: false,
		},
		{
			name:     "subtraction",
			operator: "-",
			args:     []interface{}{map[string]interface{}{"var": "a"}, 3},
			expected: "(a - 3)",
			hasError: false,
		},
		{
			name:     "multiplication",
			operator: "*",
			args:     []interface{}{map[string]interface{}{"var": "a"}, 2},
			expected: "(a * 2)",
			hasError: false,
		},
		{
			name:     "division",
			operator: "/",
			args:     []interface{}{map[string]interface{}{"var": "a"}, 4},
			expected: "(a / 4)",
			hasError: false,
		},
		{
			name:     "modulo",
			operator: "%",
			args:     []interface{}{map[string]interface{}{"var": "a"}, 3},
			expected: "(a % 3)",
			hasError: false,
		},
		{
			name:     "unary minus",
			operator: "-",
			args:     []interface{}{map[string]interface{}{"var": "x"}},
			expected: "(-x)",
			hasError: false,
		},
		{
			name:     "unary plus (cast)",
			operator: "+",
			args:     []interface{}{"42"},
			expected: "CAST('42' AS NUMERIC)",
			hasError: false,
		},
		{
			name:     "non-array args error",
			operator: "+",
			args:     "invalid",
			expected: "",
			hasError: true,
		},
		{
			name:     "insufficient args for binary",
			operator: "*",
			args:     []interface{}{5},
			expected: "",
			hasError: true,
		},
		{
			name:     "unsupported operator",
			operator: "^",
			args:     []interface{}{2, 3},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.processArithmeticExpression(tt.operator, tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("processArithmeticExpression() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("processArithmeticExpression() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("processArithmeticExpression() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestStringOperator_processNotExpression(t *testing.T) {
	op := NewStringOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		args     interface{}
		expected string
		hasError bool
	}{
		{
			name:     "not with array of one element",
			args:     []interface{}{map[string]interface{}{"var": "verified"}},
			expected: "NOT (verified)",
			hasError: false,
		},
		{
			name:     "not with single value (non-array)",
			args:     map[string]interface{}{"var": "flag"},
			expected: "NOT (flag)",
			hasError: false,
		},
		{
			name:     "not with literal true",
			args:     []interface{}{true},
			expected: "NOT (TRUE)",
			hasError: false,
		},
		{
			name:     "not with too many args error",
			args:     []interface{}{true, false},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.processNotExpression(tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("processNotExpression() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("processNotExpression() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("processNotExpression() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestStringOperator_processBooleanCoercion(t *testing.T) {
	op := NewStringOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		args     interface{}
		expected string
		hasError bool
	}{
		{
			name:     "boolean coercion with array of one element",
			args:     []interface{}{map[string]interface{}{"var": "value"}},
			expected: "(value IS NOT NULL AND value != FALSE AND value != 0 AND value != '')",
			hasError: false,
		},
		{
			name:     "boolean coercion with single value (non-array)",
			args:     map[string]interface{}{"var": "flag"},
			expected: "(flag IS NOT NULL AND flag != FALSE AND flag != 0 AND flag != '')",
			hasError: false,
		},
		{
			name:     "boolean coercion with literal",
			args:     []interface{}{42},
			expected: "(42 IS NOT NULL AND 42 != FALSE AND 42 != 0 AND 42 != '')",
			hasError: false,
		},
		{
			name:     "boolean coercion with too many args error",
			args:     []interface{}{1, 2},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.processBooleanCoercion(tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("processBooleanCoercion() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("processBooleanCoercion() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("processBooleanCoercion() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}
