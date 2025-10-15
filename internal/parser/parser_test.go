package parser

import (
	"testing"
)

func TestNewParser(t *testing.T) {
	p := NewParser()
	if p == nil {
		t.Fatal("NewParser() returned nil")
	}
	if p.validator == nil {
		t.Fatal("validator is nil")
	}
	if p.dataOp == nil {
		t.Fatal("dataOp is nil")
	}
	if p.comparisonOp == nil {
		t.Fatal("comparisonOp is nil")
	}
	if p.logicalOp == nil {
		t.Fatal("logicalOp is nil")
	}
}

func TestParser_Parse(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		// Simple comparisons
		{
			name:     "simple greater than",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "WHERE amount > 1000",
			hasError: false,
		},
		{
			name:     "simple equality",
			input:    map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "pending"}},
			expected: "WHERE status = 'pending'",
			hasError: false,
		},
		{
			name:     "simple inequality",
			input:    map[string]interface{}{"!=": []interface{}{map[string]interface{}{"var": "verified"}, false}},
			expected: "WHERE verified != FALSE",
			hasError: false,
		},

		// AND operations
		{
			name: "and with two conditions",
			input: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 5000}},
					map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "pending"}},
				},
			},
			expected: "WHERE (amount > 5000 AND status = 'pending')",
			hasError: false,
		},
		{
			name: "and with three conditions",
			input: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
					map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "active"}},
					map[string]interface{}{"!=": []interface{}{map[string]interface{}{"var": "verified"}, false}},
				},
			},
			expected: "WHERE (amount > 1000 AND status = 'active' AND verified != FALSE)",
			hasError: false,
		},

		// OR operations
		{
			name: "or with two conditions",
			input: map[string]interface{}{
				"or": []interface{}{
					map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": "failedAttempts"}, 5}},
					map[string]interface{}{"in": []interface{}{map[string]interface{}{"var": "country"}, []interface{}{"CN", "RU"}}},
				},
			},
			expected: "WHERE (failedAttempts >= 5 OR country IN ('CN', 'RU'))",
			hasError: false,
		},

		// NOT operations
		{
			name:     "not operation",
			input:    map[string]interface{}{"!": []interface{}{map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "verified"}, true}}}},
			expected: "WHERE NOT (verified = TRUE)",
			hasError: false,
		},

		// IF operations
		{
			name: "if with condition and then",
			input: map[string]interface{}{
				"if": []interface{}{
					map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "age"}, 18}},
					"adult",
				},
			},
			expected: "WHERE CASE WHEN age > 18 THEN 'adult' ELSE NULL END",
			hasError: false,
		},
		{
			name: "if with condition, then, and else",
			input: map[string]interface{}{
				"if": []interface{}{
					map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "age"}, 18}},
					"adult",
					"minor",
				},
			},
			expected: "WHERE CASE WHEN age > 18 THEN 'adult' ELSE 'minor' END",
			hasError: false,
		},

		// Nested operations
		{
			name: "nested and/or",
			input: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "transaction.amount"}, 10000}},
					map[string]interface{}{"or": []interface{}{
						map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "user.verified"}, false}},
						map[string]interface{}{"<": []interface{}{map[string]interface{}{"var": "user.accountAgeDays"}, 7}},
					}},
				},
			},
			expected: "WHERE (transaction.amount > 10000 AND (user.verified = FALSE OR user.accountAgeDays < 7))",
			hasError: false,
		},

		// Missing operations
		{
			name:     "missing operation",
			input:    map[string]interface{}{"missing": "field"},
			expected: "WHERE field IS NULL",
			hasError: false,
		},
		{
			name:     "missing_some operation",
			input:    map[string]interface{}{"missing_some": []interface{}{1, []interface{}{"field1", "field2"}}},
			expected: "WHERE (field1 IS NULL OR field2 IS NULL)",
			hasError: false,
		},

		// IN operations
		{
			name:     "in operation with strings",
			input:    map[string]interface{}{"in": []interface{}{map[string]interface{}{"var": "country"}, []interface{}{"CN", "RU"}}},
			expected: "WHERE country IN ('CN', 'RU')",
			hasError: false,
		},
		{
			name:     "in operation with numbers",
			input:    map[string]interface{}{"in": []interface{}{map[string]interface{}{"var": "status"}, []interface{}{1, 2, 3}}},
			expected: "WHERE status IN (1, 2, 3)",
			hasError: false,
		},

		// Error cases
		{
			name:     "primitive value",
			input:    "hello",
			expected: "",
			hasError: true,
		},
		{
			name:     "array value",
			input:    []interface{}{1, 2, 3},
			expected: "",
			hasError: true,
		},
		{
			name:     "multiple keys in object",
			input:    map[string]interface{}{"a": 1, "b": 2},
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
			name:     "invalid comparison args",
			input:    map[string]interface{}{">": "not-array"},
			expected: "",
			hasError: true,
		},
		{
			name:     "invalid logical args",
			input:    map[string]interface{}{"and": "not-array"},
			expected: "",
			hasError: true,
		},

		// Numeric operations
		{
			name: "addition operation",
			input: map[string]interface{}{
				"+": []interface{}{5, 3},
			},
			expected: "WHERE (5 + 3)",
			hasError: false,
		},
		{
			name: "multiplication with var",
			input: map[string]interface{}{
				"*": []interface{}{map[string]interface{}{"var": "price"}, 1.2},
			},
			expected: "WHERE (price * 1.2)",
			hasError: false,
		},
		{
			name: "between operation",
			input: map[string]interface{}{
				"between": []interface{}{map[string]interface{}{"var": "age"}, 18, 65},
			},
			expected: "WHERE (age BETWEEN 18 AND 65)",
			hasError: false,
		},
		{
			name: "max operation",
			input: map[string]interface{}{
				"max": []interface{}{10, 20, 15},
			},
			expected: "WHERE GREATEST(10, 20, 15)",
			hasError: false,
		},

		// String operations
		{
			name: "concatenation operation",
			input: map[string]interface{}{
				"cat": []interface{}{"Hello", " ", "World"},
			},
			expected: "WHERE CONCAT('Hello', ' ', 'World')",
			hasError: false,
		},
		{
			name: "substring operation",
			input: map[string]interface{}{
				"substr": []interface{}{map[string]interface{}{"var": "name"}, 1, 5},
			},
			expected: "WHERE SUBSTRING(name, 1 + 1, 5)",
			hasError: false,
		},

		// Array operations
		{
			name: "merge operation",
			input: map[string]interface{}{
				"merge": []interface{}{[]interface{}{1, 2}, []interface{}{3, 4}},
			},
			expected: "WHERE ARRAY_CONCAT([1 2], [3 4])",
			hasError: false,
		},
		{
			name: "map operation",
			input: map[string]interface{}{
				"map": []interface{}{map[string]interface{}{"var": "numbers"}, map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": "item"}, 1}}},
			},
			expected: "WHERE ARRAY_MAP(numbers, transformation_placeholder)",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Parse(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Parse() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Parse() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("Parse() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestParser_parseExpression(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name:     "primitive value",
			input:    "hello",
			expected: "",
			hasError: true,
		},
		{
			name:     "array value",
			input:    []interface{}{1, 2, 3},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.parseExpression(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("parseExpression() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("parseExpression() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("parseExpression() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestParser_parseOperator(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name     string
		operator string
		args     interface{}
		expected string
		hasError bool
	}{
		{
			name:     "var operator",
			operator: "var",
			args:     "amount",
			expected: "amount",
			hasError: false,
		},
		{
			name:     "comparison operator",
			operator: ">",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, 1000},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name:     "logical operator",
			operator: "and",
			args:     []interface{}{map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}}},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name:     "unsupported operator",
			operator: "unsupported",
			args:     []interface{}{1, 2},
			expected: "",
			hasError: true,
		},
		{
			name:     "comparison with non-array args",
			operator: ">",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "logical with non-array args",
			operator: "and",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.parseOperator(tt.operator, tt.args)

			if tt.hasError {
				if err == nil {
					t.Errorf("parseOperator() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("parseOperator() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("parseOperator() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestParser_isPrimitive(t *testing.T) {
	p := NewParser()

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
			result := p.isPrimitive(tt.input)
			if result != tt.expected {
				t.Errorf("isPrimitive(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}
