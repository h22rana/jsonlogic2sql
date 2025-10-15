package operators

import (
	"testing"
)

func TestArrayOperator_ToSQL(t *testing.T) {
	op := NewArrayOperator()

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		expected string
		hasError bool
	}{
		// Map tests
		{
			name:     "map with array and expression",
			operator: "map",
			args:     []interface{}{[]interface{}{1, 2, 3}, map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": "item"}, 1}}},
			expected: "ARRAY_MAP([1 2 3], transformation_placeholder)",
			hasError: false,
		},
		{
			name:     "map with var array",
			operator: "map",
			args:     []interface{}{map[string]interface{}{"var": "numbers"}, map[string]interface{}{"*": []interface{}{map[string]interface{}{"var": "item"}, 2}}},
			expected: "ARRAY_MAP(numbers, transformation_placeholder)",
			hasError: false,
		},
		{
			name:     "map with wrong argument count",
			operator: "map",
			args:     []interface{}{[]interface{}{1, 2, 3}},
			expected: "",
			hasError: true,
		},

		// Filter tests
		{
			name:     "filter with array and condition",
			operator: "filter",
			args:     []interface{}{[]interface{}{1, 2, 3, 4, 5}, map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "item"}, 2}}},
			expected: "ARRAY_FILTER([1 2 3 4 5], condition_placeholder)",
			hasError: false,
		},
		{
			name:     "filter with var array",
			operator: "filter",
			args:     []interface{}{map[string]interface{}{"var": "scores"}, map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": "item"}, 70}}},
			expected: "ARRAY_FILTER(scores, condition_placeholder)",
			hasError: false,
		},
		{
			name:     "filter with wrong argument count",
			operator: "filter",
			args:     []interface{}{[]interface{}{1, 2, 3}},
			expected: "",
			hasError: true,
		},

		// Reduce tests
		{
			name:     "reduce with array, initial, and expression",
			operator: "reduce",
			args:     []interface{}{[]interface{}{1, 2, 3, 4}, 0, map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": "accumulator"}, map[string]interface{}{"var": "item"}}}},
			expected: "ARRAY_REDUCE([1 2 3 4], 0, reduction_placeholder)",
			hasError: false,
		},
		{
			name:     "reduce with var array",
			operator: "reduce",
			args:     []interface{}{map[string]interface{}{"var": "numbers"}, 1, map[string]interface{}{"*": []interface{}{map[string]interface{}{"var": "accumulator"}, map[string]interface{}{"var": "item"}}}},
			expected: "ARRAY_REDUCE(numbers, 1, reduction_placeholder)",
			hasError: false,
		},
		{
			name:     "reduce with wrong argument count",
			operator: "reduce",
			args:     []interface{}{[]interface{}{1, 2, 3}, 0},
			expected: "",
			hasError: true,
		},

		// All tests
		{
			name:     "all with array and condition",
			operator: "all",
			args:     []interface{}{[]interface{}{10, 20, 30}, map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "item"}, 5}}},
			expected: "ARRAY_ALL([10 20 30], condition_placeholder)",
			hasError: false,
		},
		{
			name:     "all with var array",
			operator: "all",
			args:     []interface{}{map[string]interface{}{"var": "ages"}, map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": "item"}, 18}}},
			expected: "ARRAY_ALL(ages, condition_placeholder)",
			hasError: false,
		},
		{
			name:     "all with wrong argument count",
			operator: "all",
			args:     []interface{}{[]interface{}{1, 2, 3}},
			expected: "",
			hasError: true,
		},

		// Some tests
		{
			name:     "some with array and condition",
			operator: "some",
			args:     []interface{}{[]interface{}{1, 2, 3, 4, 5}, map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "item"}, 3}}},
			expected: "ARRAY_SOME([1 2 3 4 5], condition_placeholder)",
			hasError: false,
		},
		{
			name:     "some with var array",
			operator: "some",
			args:     []interface{}{map[string]interface{}{"var": "statuses"}, map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "item"}, "active"}}},
			expected: "ARRAY_SOME(statuses, condition_placeholder)",
			hasError: false,
		},
		{
			name:     "some with wrong argument count",
			operator: "some",
			args:     []interface{}{[]interface{}{1, 2, 3}},
			expected: "",
			hasError: true,
		},

		// None tests
		{
			name:     "none with array and condition",
			operator: "none",
			args:     []interface{}{[]interface{}{1, 2, 3, 4, 5}, map[string]interface{}{"<": []interface{}{map[string]interface{}{"var": "item"}, 0}}},
			expected: "ARRAY_NONE([1 2 3 4 5], condition_placeholder)",
			hasError: false,
		},
		{
			name:     "none with var array",
			operator: "none",
			args:     []interface{}{map[string]interface{}{"var": "values"}, map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "item"}, "invalid"}}},
			expected: "ARRAY_NONE(values, condition_placeholder)",
			hasError: false,
		},
		{
			name:     "none with wrong argument count",
			operator: "none",
			args:     []interface{}{[]interface{}{1, 2, 3}},
			expected: "",
			hasError: true,
		},

		// Merge tests
		{
			name:     "merge with two arrays",
			operator: "merge",
			args:     []interface{}{[]interface{}{1, 2}, []interface{}{3, 4}},
			expected: "ARRAY_CONCAT([1 2], [3 4])",
			hasError: false,
		},
		{
			name:     "merge with three arrays",
			operator: "merge",
			args:     []interface{}{[]interface{}{1, 2}, []interface{}{3, 4}, []interface{}{5, 6}},
			expected: "ARRAY_CONCAT([1 2], [3 4], [5 6])",
			hasError: false,
		},
		{
			name:     "merge with var arrays",
			operator: "merge",
			args:     []interface{}{map[string]interface{}{"var": "array1"}, map[string]interface{}{"var": "array2"}},
			expected: "ARRAY_CONCAT(array1, array2)",
			hasError: false,
		},
		{
			name:     "merge with single array",
			operator: "merge",
			args:     []interface{}{[]interface{}{1, 2, 3}},
			expected: "ARRAY_CONCAT([1 2 3])",
			hasError: false,
		},
		{
			name:     "merge with no arguments",
			operator: "merge",
			args:     []interface{}{},
			expected: "",
			hasError: true,
		},

		// Unsupported operator
		{
			name:     "unsupported operator",
			operator: "unsupported",
			args:     []interface{}{[]interface{}{1, 2, 3}},
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

func TestArrayOperator_valueToSQL(t *testing.T) {
	op := NewArrayOperator()

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "literal array",
			input:    []interface{}{1, 2, 3},
			expected: "[1 2 3]",
			hasError: false,
		},
		{
			name:     "literal string",
			input:    "Hello",
			expected: "'Hello'",
			hasError: false,
		},
		{
			name:     "var expression",
			input:    map[string]interface{}{"var": "items"},
			expected: "items",
			hasError: false,
		},
		{
			name:     "dotted var expression",
			input:    map[string]interface{}{"var": "user.items"},
			expected: "user.items",
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
