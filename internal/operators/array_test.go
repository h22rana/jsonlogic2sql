package operators

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

func TestArrayOperator_ToSQL(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, nil)
	op := NewArrayOperator(config)

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
			expected: "ARRAY(SELECT (elem + 1) FROM UNNEST([1 2 3]) AS elem)",
			hasError: false,
		},
		{
			name:     "map with var array",
			operator: "map",
			args:     []interface{}{map[string]interface{}{"var": "numbers"}, map[string]interface{}{"*": []interface{}{map[string]interface{}{"var": "item"}, 2}}},
			expected: "ARRAY(SELECT (elem * 2) FROM UNNEST(numbers) AS elem)",
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
			expected: "ARRAY(SELECT elem FROM UNNEST([1 2 3 4 5]) AS elem WHERE elem > 2)",
			hasError: false,
		},
		{
			name:     "filter with var array",
			operator: "filter",
			args:     []interface{}{map[string]interface{}{"var": "scores"}, map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": "item"}, 70}}},
			expected: "ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem >= 70)",
			hasError: false,
		},
		{
			name:     "filter with wrong argument count",
			operator: "filter",
			args:     []interface{}{[]interface{}{1, 2, 3}},
			expected: "",
			hasError: true,
		},

		// Reduce tests - using SUM aggregate optimization for {"+": [accumulator, current]} pattern
		{
			name:     "reduce with array, initial, and expression (sum pattern)",
			operator: "reduce",
			args:     []interface{}{[]interface{}{1, 2, 3, 4}, map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": "accumulator"}, map[string]interface{}{"var": "current"}}}, 0},
			expected: "0 + COALESCE((SELECT SUM(elem) FROM UNNEST([1 2 3 4]) AS elem), 0)",
			hasError: false,
		},
		{
			name:     "reduce with var array (general pattern)",
			operator: "reduce",
			args:     []interface{}{map[string]interface{}{"var": "numbers"}, map[string]interface{}{"*": []interface{}{map[string]interface{}{"var": "accumulator"}, map[string]interface{}{"var": "current"}}}, 1},
			expected: "(SELECT (1 * current) FROM UNNEST(numbers) AS elem)",
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
			expected: "NOT EXISTS (SELECT 1 FROM UNNEST([10 20 30]) AS elem WHERE NOT (elem > 5))",
			hasError: false,
		},
		{
			name:     "all with var array",
			operator: "all",
			args:     []interface{}{map[string]interface{}{"var": "ages"}, map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": "item"}, 18}}},
			expected: "NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18))",
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
			expected: "EXISTS (SELECT 1 FROM UNNEST([1 2 3 4 5]) AS elem WHERE elem = 3)",
			hasError: false,
		},
		{
			name:     "some with var array",
			operator: "some",
			args:     []interface{}{map[string]interface{}{"var": "statuses"}, map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "item"}, "active"}}},
			expected: "EXISTS (SELECT 1 FROM UNNEST(statuses) AS elem WHERE elem = 'active')",
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
			expected: "NOT EXISTS (SELECT 1 FROM UNNEST([1 2 3 4 5]) AS elem WHERE elem < 0)",
			hasError: false,
		},
		{
			name:     "none with var array",
			operator: "none",
			args:     []interface{}{map[string]interface{}{"var": "values"}, map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "item"}, "invalid"}}},
			expected: "NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE elem = 'invalid')",
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

// TestArrayOperator_DialectSupport tests map, filter, and reduce operators for BigQuery, Spanner, PostgreSQL, and DuckDB dialects.
func TestArrayOperator_DialectSupport(t *testing.T) {
	dialects := []struct {
		name    string
		dialect dialect.Dialect
	}{
		{"BigQuery", dialect.DialectBigQuery},
		{"Spanner", dialect.DialectSpanner},
		{"PostgreSQL", dialect.DialectPostgreSQL},
		{"DuckDB", dialect.DialectDuckDB},
	}

	for _, d := range dialects {
		t.Run(d.name, func(t *testing.T) {
			config := NewOperatorConfig(d.dialect, nil)
			op := NewArrayOperator(config)

			tests := []struct {
				name     string
				operator string
				args     []any
				expected string
				hasError bool
			}{
				// Map tests
				{
					name:     "map with literal array",
					operator: "map",
					args:     []any{[]any{1, 2, 3}, map[string]any{"+": []any{map[string]any{"var": "item"}, 1}}},
					expected: "ARRAY(SELECT (elem + 1) FROM UNNEST([1 2 3]) AS elem)",
					hasError: false,
				},
				{
					name:     "map with var array",
					operator: "map",
					args:     []any{map[string]any{"var": "numbers"}, map[string]any{"*": []any{map[string]any{"var": "item"}, 2}}},
					expected: "ARRAY(SELECT (elem * 2) FROM UNNEST(numbers) AS elem)",
					hasError: false,
				},
				{
					name:     "map with complex transformation",
					operator: "map",
					args:     []any{map[string]any{"var": "prices"}, map[string]any{"-": []any{map[string]any{"var": "item"}, 10}}},
					expected: "ARRAY(SELECT (elem - 10) FROM UNNEST(prices) AS elem)",
					hasError: false,
				},

				// Filter tests
				{
					name:     "filter with literal array",
					operator: "filter",
					args:     []any{[]any{1, 2, 3, 4, 5}, map[string]any{">": []any{map[string]any{"var": "item"}, 2}}},
					expected: "ARRAY(SELECT elem FROM UNNEST([1 2 3 4 5]) AS elem WHERE elem > 2)",
					hasError: false,
				},
				{
					name:     "filter with var array",
					operator: "filter",
					args:     []any{map[string]any{"var": "scores"}, map[string]any{">=": []any{map[string]any{"var": "item"}, 70}}},
					expected: "ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem >= 70)",
					hasError: false,
				},
				{
					name:     "filter with equality condition",
					operator: "filter",
					args:     []any{map[string]any{"var": "statuses"}, map[string]any{"==": []any{map[string]any{"var": "item"}, "active"}}},
					expected: "ARRAY(SELECT elem FROM UNNEST(statuses) AS elem WHERE elem = 'active')",
					hasError: false,
				},

				// Reduce tests - SUM pattern optimization
				{
					name:     "reduce with SUM pattern",
					operator: "reduce",
					args:     []any{[]any{1, 2, 3, 4}, map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}}, 0},
					expected: "0 + COALESCE((SELECT SUM(elem) FROM UNNEST([1 2 3 4]) AS elem), 0)",
					hasError: false,
				},
				{
					name:     "reduce with SUM pattern and var array",
					operator: "reduce",
					args:     []any{map[string]any{"var": "numbers"}, map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}}, 0},
					expected: "0 + COALESCE((SELECT SUM(elem) FROM UNNEST(numbers) AS elem), 0)",
					hasError: false,
				},
				{
					name:     "reduce with MIN pattern",
					operator: "reduce",
					args:     []any{map[string]any{"var": "values"}, map[string]any{"min": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}}, 999999},
					expected: "999999 + COALESCE((SELECT MIN(elem) FROM UNNEST(values) AS elem), 0)",
					hasError: false,
				},
				{
					name:     "reduce with MAX pattern",
					operator: "reduce",
					args:     []any{map[string]any{"var": "values"}, map[string]any{"max": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}}, 0},
					expected: "0 + COALESCE((SELECT MAX(elem) FROM UNNEST(values) AS elem), 0)",
					hasError: false,
				},
				{
					name:     "reduce with general pattern (multiplication)",
					operator: "reduce",
					args:     []any{map[string]any{"var": "numbers"}, map[string]any{"*": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}}, 1},
					expected: "(SELECT (1 * current) FROM UNNEST(numbers) AS elem)",
					hasError: false,
				},
				{
					name:     "reduce with non-zero initial value",
					operator: "reduce",
					args:     []any{[]any{10, 20, 30}, map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}}, 100},
					expected: "100 + COALESCE((SELECT SUM(elem) FROM UNNEST([10 20 30]) AS elem), 0)",
					hasError: false,
				},

				// All tests
				{
					name:     "all with condition",
					operator: "all",
					args:     []any{map[string]any{"var": "ages"}, map[string]any{">=": []any{map[string]any{"var": "item"}, 18}}},
					expected: "NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18))",
					hasError: false,
				},

				// Some tests
				{
					name:     "some with condition",
					operator: "some",
					args:     []any{map[string]any{"var": "statuses"}, map[string]any{"==": []any{map[string]any{"var": "item"}, "active"}}},
					expected: "EXISTS (SELECT 1 FROM UNNEST(statuses) AS elem WHERE elem = 'active')",
					hasError: false,
				},

				// None tests
				{
					name:     "none with condition",
					operator: "none",
					args:     []any{map[string]any{"var": "values"}, map[string]any{"==": []any{map[string]any{"var": "item"}, "invalid"}}},
					expected: "NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE elem = 'invalid')",
					hasError: false,
				},

				// Note: merge tests are in TestArrayOperator_MergeDialectSpecific due to dialect-specific output
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					result, err := op.ToSQL(tt.operator, tt.args)
					if tt.hasError {
						if err == nil {
							t.Errorf("[%s] Expected error but got none", d.name)
						}
						return
					}
					if err != nil {
						t.Errorf("[%s] Unexpected error: %v", d.name, err)
						return
					}
					if result != tt.expected {
						t.Errorf("[%s] Expected %s, got %s", d.name, tt.expected, result)
					}
				})
			}
		})
	}
}

// TestArrayOperator_MergeDialectSpecific tests the merge operator with dialect-specific output.
// BigQuery/Spanner/DuckDB use ARRAY_CONCAT, PostgreSQL uses || operator.
func TestArrayOperator_MergeDialectSpecific(t *testing.T) {
	tests := []struct {
		name     string
		dialect  dialect.Dialect
		args     []any
		expected string
	}{
		// BigQuery tests
		{
			name:     "BigQuery merge two arrays",
			dialect:  dialect.DialectBigQuery,
			args:     []any{map[string]any{"var": "array1"}, map[string]any{"var": "array2"}},
			expected: "ARRAY_CONCAT(array1, array2)",
		},
		{
			name:     "BigQuery merge three arrays",
			dialect:  dialect.DialectBigQuery,
			args:     []any{map[string]any{"var": "a"}, map[string]any{"var": "b"}, map[string]any{"var": "c"}},
			expected: "ARRAY_CONCAT(a, b, c)",
		},
		{
			name:     "BigQuery merge single array",
			dialect:  dialect.DialectBigQuery,
			args:     []any{map[string]any{"var": "arr"}},
			expected: "ARRAY_CONCAT(arr)",
		},
		// Spanner tests
		{
			name:     "Spanner merge two arrays",
			dialect:  dialect.DialectSpanner,
			args:     []any{map[string]any{"var": "array1"}, map[string]any{"var": "array2"}},
			expected: "ARRAY_CONCAT(array1, array2)",
		},
		{
			name:     "Spanner merge three arrays",
			dialect:  dialect.DialectSpanner,
			args:     []any{map[string]any{"var": "a"}, map[string]any{"var": "b"}, map[string]any{"var": "c"}},
			expected: "ARRAY_CONCAT(a, b, c)",
		},
		// PostgreSQL tests - uses || operator
		{
			name:     "PostgreSQL merge two arrays",
			dialect:  dialect.DialectPostgreSQL,
			args:     []any{map[string]any{"var": "array1"}, map[string]any{"var": "array2"}},
			expected: "(array1 || array2)",
		},
		{
			name:     "PostgreSQL merge three arrays",
			dialect:  dialect.DialectPostgreSQL,
			args:     []any{map[string]any{"var": "a"}, map[string]any{"var": "b"}, map[string]any{"var": "c"}},
			expected: "(a || b || c)",
		},
		{
			name:     "PostgreSQL merge single array",
			dialect:  dialect.DialectPostgreSQL,
			args:     []any{map[string]any{"var": "arr"}},
			expected: "arr",
		},
		// DuckDB tests - uses ARRAY_CONCAT like BigQuery/Spanner
		{
			name:     "DuckDB merge two arrays",
			dialect:  dialect.DialectDuckDB,
			args:     []any{map[string]any{"var": "array1"}, map[string]any{"var": "array2"}},
			expected: "ARRAY_CONCAT(array1, array2)",
		},
		{
			name:     "DuckDB merge three arrays",
			dialect:  dialect.DialectDuckDB,
			args:     []any{map[string]any{"var": "a"}, map[string]any{"var": "b"}, map[string]any{"var": "c"}},
			expected: "ARRAY_CONCAT(a, b, c)",
		},
		{
			name:     "DuckDB merge single array",
			dialect:  dialect.DialectDuckDB,
			args:     []any{map[string]any{"var": "arr"}},
			expected: "ARRAY_CONCAT(arr)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewOperatorConfig(tt.dialect, nil)
			op := NewArrayOperator(config)

			result, err := op.ToSQL("merge", tt.args)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// TestArrayOperator_DialectValidation tests that array operators properly validate dialect configuration.
func TestArrayOperator_DialectValidation(t *testing.T) {
	operators := []string{"map", "filter", "reduce", "all", "some", "none", "merge"}

	t.Run("unspecified dialect returns error", func(t *testing.T) {
		config := NewOperatorConfig(dialect.DialectUnspecified, nil)
		op := NewArrayOperator(config)

		for _, operator := range operators {
			var args []any
			switch operator {
			case "map", "filter", "all", "some", "none":
				args = []any{map[string]any{"var": "arr"}, map[string]any{"var": "item"}}
			case "reduce":
				args = []any{map[string]any{"var": "arr"}, map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}}, 0}
			case "merge":
				args = []any{map[string]any{"var": "arr1"}, map[string]any{"var": "arr2"}}
			}

			_, err := op.ToSQL(operator, args)
			if err == nil {
				t.Errorf("Expected error for operator %s with unspecified dialect, got none", operator)
			}
		}
	})

	t.Run("BigQuery dialect succeeds", func(t *testing.T) {
		config := NewOperatorConfig(dialect.DialectBigQuery, nil)
		op := NewArrayOperator(config)

		// Test map as representative
		args := []any{map[string]any{"var": "arr"}, map[string]any{"+": []any{map[string]any{"var": "item"}, 1}}}
		_, err := op.ToSQL("map", args)
		if err != nil {
			t.Errorf("Unexpected error for BigQuery dialect: %v", err)
		}
	})

	t.Run("Spanner dialect succeeds", func(t *testing.T) {
		config := NewOperatorConfig(dialect.DialectSpanner, nil)
		op := NewArrayOperator(config)

		// Test map as representative
		args := []any{map[string]any{"var": "arr"}, map[string]any{"+": []any{map[string]any{"var": "item"}, 1}}}
		_, err := op.ToSQL("map", args)
		if err != nil {
			t.Errorf("Unexpected error for Spanner dialect: %v", err)
		}
	})
}

func TestArrayOperator_valueToSQL(t *testing.T) {
	op := NewArrayOperator(nil)

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

// TestArrayOperator_EdgeCases tests complex and edge case scenarios for array operators.
func TestArrayOperator_EdgeCases(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, nil)
	op := NewArrayOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []any
		expected string
		hasError bool
	}{
		// Map with complex conditions
		{
			name:     "map with if-else condition",
			operator: "map",
			args: []any{
				map[string]any{"var": "items"},
				map[string]any{
					"if": []any{
						map[string]any{">": []any{map[string]any{"var": "item"}, 10}},
						"high",
						"low",
					},
				},
			},
			expected: "ARRAY(SELECT CASE WHEN elem > 10 THEN 'high' ELSE 'low' END FROM UNNEST(items) AS elem)",
			hasError: false,
		},
		{
			name:     "map with nested arithmetic",
			operator: "map",
			args: []any{
				map[string]any{"var": "prices"},
				map[string]any{
					"+": []any{
						map[string]any{"*": []any{map[string]any{"var": "item"}, 1.1}},
						5,
					},
				},
			},
			expected: "ARRAY(SELECT ((elem * 1.1) + 5) FROM UNNEST(prices) AS elem)",
			hasError: false,
		},
		{
			name:     "map with unary minus",
			operator: "map",
			args: []any{
				map[string]any{"var": "numbers"},
				map[string]any{"-": []any{map[string]any{"var": "item"}}},
			},
			expected: "ARRAY(SELECT (-elem) FROM UNNEST(numbers) AS elem)",
			hasError: false,
		},

		// Filter with complex logical conditions
		{
			name:     "filter with AND condition",
			operator: "filter",
			args: []any{
				map[string]any{"var": "items"},
				map[string]any{
					"and": []any{
						map[string]any{">": []any{map[string]any{"var": "item"}, 5}},
						map[string]any{"<": []any{map[string]any{"var": "item"}, 100}},
					},
				},
			},
			expected: "ARRAY(SELECT elem FROM UNNEST(items) AS elem WHERE (elem > 5 AND elem < 100))",
			hasError: false,
		},
		{
			name:     "filter with OR condition",
			operator: "filter",
			args: []any{
				map[string]any{"var": "statuses"},
				map[string]any{
					"or": []any{
						map[string]any{"==": []any{map[string]any{"var": "item"}, "active"}},
						map[string]any{"==": []any{map[string]any{"var": "item"}, "pending"}},
					},
				},
			},
			expected: "ARRAY(SELECT elem FROM UNNEST(statuses) AS elem WHERE (elem = 'active' OR elem = 'pending'))",
			hasError: false,
		},
		{
			name:     "filter with NOT condition",
			operator: "filter",
			args: []any{
				map[string]any{"var": "values"},
				map[string]any{
					"!": []any{
						map[string]any{"==": []any{map[string]any{"var": "item"}, 0}},
					},
				},
			},
			expected: "ARRAY(SELECT elem FROM UNNEST(values) AS elem WHERE NOT (elem = 0))",
			hasError: false,
		},
		{
			name:     "filter with nested AND/OR",
			operator: "filter",
			args: []any{
				map[string]any{"var": "items"},
				map[string]any{
					"and": []any{
						map[string]any{">": []any{map[string]any{"var": "item"}, 0}},
						map[string]any{
							"or": []any{
								map[string]any{"<": []any{map[string]any{"var": "item"}, 10}},
								map[string]any{">": []any{map[string]any{"var": "item"}, 100}},
							},
						},
					},
				},
			},
			expected: "ARRAY(SELECT elem FROM UNNEST(items) AS elem WHERE (elem > 0 AND (elem < 10 OR elem > 100)))",
			hasError: false,
		},

		// Reduce with different aggregate patterns
		{
			name:     "reduce with MAX pattern",
			operator: "reduce",
			args: []any{
				map[string]any{"var": "values"},
				map[string]any{"max": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}},
				0,
			},
			expected: "0 + COALESCE((SELECT MAX(elem) FROM UNNEST(values) AS elem), 0)",
			hasError: false,
		},
		{
			name:     "reduce with MIN pattern",
			operator: "reduce",
			args: []any{
				map[string]any{"var": "values"},
				map[string]any{"min": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}},
				999999,
			},
			expected: "999999 + COALESCE((SELECT MIN(elem) FROM UNNEST(values) AS elem), 0)",
			hasError: false,
		},
		{
			name:     "reduce with subtraction (general pattern)",
			operator: "reduce",
			args: []any{
				map[string]any{"var": "numbers"},
				map[string]any{"-": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}},
				100,
			},
			expected: "(SELECT (100 - current) FROM UNNEST(numbers) AS elem)",
			hasError: false,
		},
		{
			name:     "reduce with division (general pattern)",
			operator: "reduce",
			args: []any{
				map[string]any{"var": "numbers"},
				map[string]any{"/": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}},
				1000,
			},
			expected: "(SELECT (1000 / current) FROM UNNEST(numbers) AS elem)",
			hasError: false,
		},

		// All/Some/None with complex conditions
		{
			name:     "all with AND condition",
			operator: "all",
			args: []any{
				map[string]any{"var": "scores"},
				map[string]any{
					"and": []any{
						map[string]any{">=": []any{map[string]any{"var": "item"}, 0}},
						map[string]any{"<=": []any{map[string]any{"var": "item"}, 100}},
					},
				},
			},
			expected: "NOT EXISTS (SELECT 1 FROM UNNEST(scores) AS elem WHERE NOT ((elem >= 0 AND elem <= 100)))",
			hasError: false,
		},
		{
			name:     "some with OR condition",
			operator: "some",
			args: []any{
				map[string]any{"var": "flags"},
				map[string]any{
					"or": []any{
						map[string]any{"==": []any{map[string]any{"var": "item"}, true}},
						map[string]any{"==": []any{map[string]any{"var": "item"}, 1}},
					},
				},
			},
			expected: "EXISTS (SELECT 1 FROM UNNEST(flags) AS elem WHERE (elem = TRUE OR elem = 1))",
			hasError: false,
		},
		{
			name:     "none with comparison chain",
			operator: "none",
			args: []any{
				map[string]any{"var": "temperatures"},
				map[string]any{
					"and": []any{
						map[string]any{">": []any{map[string]any{"var": "item"}, 40}},
						map[string]any{"<": []any{map[string]any{"var": "item"}, 50}},
					},
				},
			},
			expected: "NOT EXISTS (SELECT 1 FROM UNNEST(temperatures) AS elem WHERE (elem > 40 AND elem < 50))",
			hasError: false,
		},

		// Nested array operations
		{
			name:     "nested map inside filter result",
			operator: "map",
			args: []any{
				map[string]any{
					"filter": []any{
						map[string]any{"var": "numbers"},
						map[string]any{">": []any{map[string]any{"var": "item"}, 0}},
					},
				},
				map[string]any{"*": []any{map[string]any{"var": "item"}, 2}},
			},
			expected: "ARRAY(SELECT (elem * 2) FROM UNNEST(ARRAY(SELECT elem FROM UNNEST(numbers) AS elem WHERE elem > 0)) AS elem)",
			hasError: false,
		},

		// Merge with mixed arrays
		{
			name:     "merge with literal and var arrays",
			operator: "merge",
			args: []any{
				[]any{1, 2, 3},
				map[string]any{"var": "moreNumbers"},
			},
			expected: "ARRAY_CONCAT([1 2 3], moreNumbers)",
			hasError: false,
		},
		{
			name:     "merge with four arrays",
			operator: "merge",
			args: []any{
				map[string]any{"var": "a"},
				map[string]any{"var": "b"},
				map[string]any{"var": "c"},
				map[string]any{"var": "d"},
			},
			expected: "ARRAY_CONCAT(a, b, c, d)",
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
				t.Errorf("Expected:\n%s\nGot:\n%s", tt.expected, result)
			}
		})
	}
}
