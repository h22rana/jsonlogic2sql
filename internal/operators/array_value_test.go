package operators

import (
	"fmt"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestArrayOperator_DialectValidation(t *testing.T) {
	operators := []string{"map", "filter", "reduce", "all", "some", "none", "merge"}

	t.Run("unspecified dialect returns error", func(t *testing.T) {
		config := NewOperatorConfig(dialect.DialectUnspecified, &arrayTestSchemaProvider{})
		op := NewArrayOperator(config)

		for _, operator := range operators {
			var args []any
			switch operator {
			case "map", "filter", "all", "some", "none":
				args = []any{map[string]any{"var": "arr"}, map[string]any{"var": ""}}
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
		config := NewOperatorConfig(dialect.DialectBigQuery, &arrayTestSchemaProvider{})
		op := NewArrayOperator(config)

		// Test map as representative
		args := []any{map[string]any{"var": "arr"}, map[string]any{"+": []any{map[string]any{"var": ""}, 1}}}
		_, err := op.ToSQL("map", args)
		if err != nil {
			t.Errorf("Unexpected error for BigQuery dialect: %v", err)
		}
	})

	t.Run("Spanner dialect succeeds", func(t *testing.T) {
		config := NewOperatorConfig(dialect.DialectSpanner, &arrayTestSchemaProvider{})
		op := NewArrayOperator(config)

		// Test map as representative
		args := []any{map[string]any{"var": "arr"}, map[string]any{"+": []any{map[string]any{"var": ""}, 1}}}
		_, err := op.ToSQL("map", args)
		if err != nil {
			t.Errorf("Unexpected error for Spanner dialect: %v", err)
		}
	})
}

func TestArrayOperator_valueToSQL(t *testing.T) {
	op := NewArrayOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "literal array",
			input:    []interface{}{1, 2, 3},
			expected: "[1, 2, 3]",
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
	config := NewOperatorConfig(dialect.DialectBigQuery, &arrayTestSchemaProvider{})
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
						map[string]any{">": []any{map[string]any{"var": ""}, 10}},
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
						map[string]any{"*": []any{map[string]any{"var": ""}, 1.1}},
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
				map[string]any{"-": []any{map[string]any{"var": ""}}},
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
						map[string]any{">": []any{map[string]any{"var": ""}, 5}},
						map[string]any{"<": []any{map[string]any{"var": ""}, 100}},
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
						map[string]any{"==": []any{map[string]any{"var": ""}, "active"}},
						map[string]any{"==": []any{map[string]any{"var": ""}, "pending"}},
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
						map[string]any{"==": []any{map[string]any{"var": ""}, 0}},
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
						map[string]any{">": []any{map[string]any{"var": ""}, 0}},
						map[string]any{
							"or": []any{
								map[string]any{"<": []any{map[string]any{"var": ""}, 10}},
								map[string]any{">": []any{map[string]any{"var": ""}, 100}},
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
			expected: "GREATEST(0, COALESCE((SELECT MAX(elem) FROM UNNEST(values) AS elem), 0))",
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
			expected: "LEAST(999999, COALESCE((SELECT MIN(elem) FROM UNNEST(values) AS elem), 999999))",
			hasError: false,
		},
		{
			name:     "reduce with subtraction (general pattern unsupported in standard SQL)",
			operator: "reduce",
			args: []any{
				map[string]any{"var": "numbers"},
				map[string]any{"-": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}},
				100,
			},
			expected: "",
			hasError: true,
		},
		{
			name:     "reduce with division (general pattern unsupported in standard SQL)",
			operator: "reduce",
			args: []any{
				map[string]any{"var": "numbers"},
				map[string]any{"/": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}},
				1000,
			},
			expected: "",
			hasError: true,
		},

		// All/Some/None with complex conditions
		{
			name:     "all with AND condition",
			operator: "all",
			args: []any{
				map[string]any{"var": "scores"},
				map[string]any{
					"and": []any{
						map[string]any{">=": []any{map[string]any{"var": ""}, 0}},
						map[string]any{"<=": []any{map[string]any{"var": ""}, 100}},
					},
				},
			},
			expected: "(ARRAY_LENGTH(scores) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(scores) AS elem WHERE NOT ((elem >= 0 AND elem <= 100))))",
			hasError: false,
		},
		{
			name:     "some with OR condition",
			operator: "some",
			args: []any{
				map[string]any{"var": "flags"},
				map[string]any{
					"or": []any{
						map[string]any{"==": []any{map[string]any{"var": ""}, true}},
						map[string]any{"==": []any{map[string]any{"var": ""}, 1}},
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
						map[string]any{">": []any{map[string]any{"var": ""}, 40}},
						map[string]any{"<": []any{map[string]any{"var": ""}, 50}},
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
						map[string]any{">": []any{map[string]any{"var": ""}, 0}},
					},
				},
				map[string]any{"*": []any{map[string]any{"var": ""}, 2}},
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
			expected: "ARRAY_CONCAT([1, 2, 3], moreNumbers)",
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

// TestArrayOperator_ClickHouse tests array operators with ClickHouse-specific syntax.
func TestArrayOperator_ClickHouse(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectClickHouse, &arrayTestSchemaProvider{})
	op := NewArrayOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []any
		expected string
		hasError bool
	}{
		// Map - uses arrayMap
		{
			name:     "map with transformation",
			operator: "map",
			args:     []any{map[string]any{"var": "numbers"}, map[string]any{"*": []any{map[string]any{"var": ""}, 2}}},
			expected: "arrayMap(elem -> (elem * 2), numbers)",
			hasError: false,
		},
		// Filter - uses arrayFilter
		{
			name:     "filter with condition",
			operator: "filter",
			args:     []any{map[string]any{"var": "scores"}, map[string]any{">=": []any{map[string]any{"var": ""}, 70}}},
			expected: "arrayFilter(elem -> elem >= 70, scores)",
			hasError: false,
		},
		// Reduce - uses arrayReduce for aggregates
		{
			name:     "reduce with SUM pattern",
			operator: "reduce",
			args:     []any{map[string]any{"var": "numbers"}, map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}}, 0},
			expected: "0 + coalesce(arrayReduce('sum', numbers), 0)",
			hasError: false,
		},
		{
			name:     "reduce with SUM pattern on current.price (ClickHouse)",
			operator: "reduce",
			args:     []any{map[string]any{"var": "items"}, map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current.price"}}}, 0},
			expected: "0 + coalesce(arrayReduce('sum', arrayMap(x -> x.price, items)), 0)",
			hasError: false,
		},
		{
			name:     "reduce with MAX pattern on current.value (ClickHouse)",
			operator: "reduce",
			args:     []any{map[string]any{"var": "readings"}, map[string]any{"max": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current.value"}}}, 0},
			expected: "CASE WHEN length(readings) > 0 THEN greatest(0, coalesce(arrayReduce('max', arrayMap(x -> x.value, readings)), 0)) ELSE 0 END",
			hasError: false,
		},
		{
			name:     "reduce with MIN pattern on current.amount (ClickHouse)",
			operator: "reduce",
			args:     []any{map[string]any{"var": "transactions"}, map[string]any{"min": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current.amount"}}}, 9999999},
			expected: "CASE WHEN length(transactions) > 0 THEN least(9999999, coalesce(arrayReduce('min', arrayMap(x -> x.amount, transactions)), 9999999)) ELSE 9999999 END",
			hasError: false,
		},
		{
			name:     "reduce with general pattern keeps running accumulator in ClickHouse",
			operator: "reduce",
			args:     []any{map[string]any{"var": "numbers"}, map[string]any{"-": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}}, 100},
			expected: "arrayFold((acc, elem) -> (acc - elem), numbers, toFloat64(100))",
			hasError: false,
		},
		// All - uses arrayAll
		{
			name:     "all with condition",
			operator: "all",
			args:     []any{map[string]any{"var": "values"}, map[string]any{">": []any{map[string]any{"var": ""}, 0}}},
			expected: "(length(values) > 0 AND arrayAll(elem -> elem > 0, values))",
			hasError: false,
		},
		// Some - uses arrayExists
		{
			name:     "some with condition",
			operator: "some",
			args:     []any{map[string]any{"var": "items"}, map[string]any{"==": []any{map[string]any{"var": ""}, "active"}}},
			expected: "arrayExists(elem -> elem = 'active', items)",
			hasError: false,
		},
		// None - uses NOT arrayExists
		{
			name:     "none with condition",
			operator: "none",
			args:     []any{map[string]any{"var": "statuses"}, map[string]any{"==": []any{map[string]any{"var": ""}, "error"}}},
			expected: "NOT arrayExists(elem -> elem = 'error', statuses)",
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

func newArrayOperatorWithParamParserBQ(t *testing.T) *ArrayOperator {
	t.Helper()
	config := NewOperatorConfig(dialect.DialectBigQuery, &arrayTestSchemaProvider{})
	config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
		return "", fmt.Errorf("unexpected ParamExpressionParser in test (path=%s, expr=%v)", path, expr)
	})
	return NewArrayOperator(config)
}
