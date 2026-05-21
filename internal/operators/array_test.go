package operators

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

var arrayTestUnnestAliasPattern = regexp.MustCompile(`FROM UNNEST\(([^)]*)\) AS (elem[0-9]*)`)

func arrayTestDuckDBUnnestSourceAliases(d dialect.Dialect, sql string) string {
	if d != dialect.DialectDuckDB {
		return sql
	}
	return arrayTestUnnestAliasPattern.ReplaceAllString(sql, `FROM UNNEST($1) AS $2($2)`)
}

type arrayTestSchemaProvider struct {
	mockSchemaProvider
}

func (a *arrayTestSchemaProvider) GetFieldType(fieldName string) string {
	if a.IsArrayType(fieldName) {
		return "array"
	}
	return "number"
}

func (a *arrayTestSchemaProvider) IsArrayType(fieldName string) bool {
	switch fieldName {
	case "numbers", "scores", "values", "array1", "array2", "arr", "a", "b", "c", "d",
		"moreNumbers", "ages", "statuses", "prices", "items", "readings", "transactions",
		"logs", "flags", "temperatures":
		return true
	default:
		return false
	}
}

func (a *arrayTestSchemaProvider) IsNumericType(fieldName string) bool {
	return !a.IsArrayType(fieldName)
}

func TestArrayOperator_ToSQL(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &arrayTestSchemaProvider{})
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
			args:     []interface{}{[]interface{}{1, 2, 3}, map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": ""}, 1}}},
			expected: "ARRAY(SELECT (elem + 1) FROM UNNEST([1, 2, 3]) AS elem)",
			hasError: false,
		},
		{
			name:     "map with var array",
			operator: "map",
			args:     []interface{}{map[string]interface{}{"var": "numbers"}, map[string]interface{}{"*": []interface{}{map[string]interface{}{"var": ""}, 2}}},
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
			args:     []interface{}{[]interface{}{1, 2, 3, 4, 5}, map[string]interface{}{">": []interface{}{map[string]interface{}{"var": ""}, 2}}},
			expected: "ARRAY(SELECT elem FROM UNNEST([1, 2, 3, 4, 5]) AS elem WHERE elem > 2)",
			hasError: false,
		},
		{
			name:     "filter with var array",
			operator: "filter",
			args:     []interface{}{map[string]interface{}{"var": "scores"}, map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": ""}, 70}}},
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
			expected: "0 + COALESCE((SELECT SUM(elem) FROM UNNEST([1, 2, 3, 4]) AS elem), 0)",
			hasError: false,
		},
		{
			name:     "reduce with var array (general pattern unsupported in standard SQL)",
			operator: "reduce",
			args:     []interface{}{map[string]interface{}{"var": "numbers"}, map[string]interface{}{"*": []interface{}{map[string]interface{}{"var": "accumulator"}, map[string]interface{}{"var": "current"}}}, 1},
			expected: "",
			hasError: true,
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
			args:     []interface{}{[]interface{}{10, 20, 30}, map[string]interface{}{">": []interface{}{map[string]interface{}{"var": ""}, 5}}},
			expected: "(ARRAY_LENGTH([10, 20, 30]) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST([10, 20, 30]) AS elem WHERE NOT (elem > 5)))",
			hasError: false,
		},
		{
			name:     "all with var array",
			operator: "all",
			args:     []interface{}{map[string]interface{}{"var": "ages"}, map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": ""}, 18}}},
			expected: "(ARRAY_LENGTH(ages) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18)))",
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
			args:     []interface{}{[]interface{}{1, 2, 3, 4, 5}, map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": ""}, 3}}},
			expected: "EXISTS (SELECT 1 FROM UNNEST([1, 2, 3, 4, 5]) AS elem WHERE elem = 3)",
			hasError: false,
		},
		{
			name:     "some with var array",
			operator: "some",
			args:     []interface{}{map[string]interface{}{"var": "statuses"}, map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": ""}, "active"}}},
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
			args:     []interface{}{[]interface{}{1, 2, 3, 4, 5}, map[string]interface{}{"<": []interface{}{map[string]interface{}{"var": ""}, 0}}},
			expected: "NOT EXISTS (SELECT 1 FROM UNNEST([1, 2, 3, 4, 5]) AS elem WHERE elem < 0)",
			hasError: false,
		},
		{
			name:     "none with var array",
			operator: "none",
			args:     []interface{}{map[string]interface{}{"var": "values"}, map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": ""}, "invalid"}}},
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
			expected: "ARRAY_CONCAT([1, 2], [3, 4])",
			hasError: false,
		},
		{
			name:     "merge with three arrays",
			operator: "merge",
			args:     []interface{}{[]interface{}{1, 2}, []interface{}{3, 4}, []interface{}{5, 6}},
			expected: "ARRAY_CONCAT([1, 2], [3, 4], [5, 6])",
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
			expected: "ARRAY_CONCAT([1, 2, 3])",
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
		// Note: ClickHouse has different syntax, tested separately in TestArrayOperator_ClickHouse
	}

	for _, d := range dialects {
		t.Run(d.name, func(t *testing.T) {
			config := NewOperatorConfig(d.dialect, &arrayTestSchemaProvider{})
			op := NewArrayOperator(config)
			literal123 := "[1, 2, 3]"
			literal1234 := "[1, 2, 3, 4]"
			literal12345 := "[1, 2, 3, 4, 5]"
			literal10s := "[10, 20, 30]"
			if d.dialect == dialect.DialectPostgreSQL {
				literal123 = "ARRAY[1, 2, 3]"
				literal1234 = "ARRAY[1, 2, 3, 4]"
				literal12345 = "ARRAY[1, 2, 3, 4, 5]"
				literal10s = "ARRAY[10, 20, 30]"
			}

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
					args:     []any{[]any{1, 2, 3}, map[string]any{"+": []any{map[string]any{"var": ""}, 1}}},
					expected: fmt.Sprintf("ARRAY(SELECT (elem + 1) FROM UNNEST(%s) AS elem)", literal123),
					hasError: false,
				},
				{
					name:     "map with var array",
					operator: "map",
					args:     []any{map[string]any{"var": "numbers"}, map[string]any{"*": []any{map[string]any{"var": ""}, 2}}},
					expected: "ARRAY(SELECT (elem * 2) FROM UNNEST(numbers) AS elem)",
					hasError: false,
				},
				{
					name:     "map with complex transformation",
					operator: "map",
					args:     []any{map[string]any{"var": "prices"}, map[string]any{"-": []any{map[string]any{"var": ""}, 10}}},
					expected: "ARRAY(SELECT (elem - 10) FROM UNNEST(prices) AS elem)",
					hasError: false,
				},

				// Filter tests
				{
					name:     "filter with literal array",
					operator: "filter",
					args:     []any{[]any{1, 2, 3, 4, 5}, map[string]any{">": []any{map[string]any{"var": ""}, 2}}},
					expected: fmt.Sprintf("ARRAY(SELECT elem FROM UNNEST(%s) AS elem WHERE elem > 2)", literal12345),
					hasError: false,
				},
				{
					name:     "filter with var array",
					operator: "filter",
					args:     []any{map[string]any{"var": "scores"}, map[string]any{">=": []any{map[string]any{"var": ""}, 70}}},
					expected: "ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem >= 70)",
					hasError: false,
				},
				{
					name:     "filter with equality condition",
					operator: "filter",
					args:     []any{map[string]any{"var": "statuses"}, map[string]any{"==": []any{map[string]any{"var": ""}, "active"}}},
					expected: "ARRAY(SELECT elem FROM UNNEST(statuses) AS elem WHERE elem = 'active')",
					hasError: false,
				},

				// Reduce tests - SUM pattern optimization
				{
					name:     "reduce with SUM pattern",
					operator: "reduce",
					args:     []any{[]any{1, 2, 3, 4}, map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}}, 0},
					expected: fmt.Sprintf("0 + COALESCE((SELECT SUM(elem) FROM UNNEST(%s) AS elem), 0)", literal1234),
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
					expected: "LEAST(999999, COALESCE((SELECT MIN(elem) FROM UNNEST(values) AS elem), 999999))",
					hasError: false,
				},
				{
					name:     "reduce with MAX pattern",
					operator: "reduce",
					args:     []any{map[string]any{"var": "values"}, map[string]any{"max": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}}, 0},
					expected: "GREATEST(0, COALESCE((SELECT MAX(elem) FROM UNNEST(values) AS elem), 0))",
					hasError: false,
				},
				{
					name:     "reduce with general pattern (multiplication unsupported in standard SQL)",
					operator: "reduce",
					args:     []any{map[string]any{"var": "numbers"}, map[string]any{"*": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}}, 1},
					expected: "",
					hasError: true,
				},
				{
					name:     "reduce with non-zero initial value",
					operator: "reduce",
					args:     []any{[]any{10, 20, 30}, map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}}, 100},
					expected: fmt.Sprintf("100 + COALESCE((SELECT SUM(elem) FROM UNNEST(%s) AS elem), 0)", literal10s),
					hasError: false,
				},
				// Reduce with current.field patterns (accessing object field)
				{
					name:     "reduce with SUM pattern on current.price",
					operator: "reduce",
					args:     []any{map[string]any{"var": "items"}, map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current.price"}}}, 0},
					expected: "0 + COALESCE((SELECT SUM(elem.price) FROM UNNEST(items) AS elem), 0)",
					hasError: false,
				},
				{
					name:     "reduce with MAX pattern on current.value",
					operator: "reduce",
					args:     []any{map[string]any{"var": "readings"}, map[string]any{"max": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current.value"}}}, 0},
					expected: "GREATEST(0, COALESCE((SELECT MAX(elem.value) FROM UNNEST(readings) AS elem), 0))",
					hasError: false,
				},
				{
					name:     "reduce with MIN pattern on current.amount",
					operator: "reduce",
					args:     []any{map[string]any{"var": "transactions"}, map[string]any{"min": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current.amount"}}}, 9999999},
					expected: "LEAST(9999999, COALESCE((SELECT MIN(elem.amount) FROM UNNEST(transactions) AS elem), 9999999))",
					hasError: false,
				},
				{
					name:     "reduce with SUM pattern on deeply nested current.data.metrics.count",
					operator: "reduce",
					args:     []any{map[string]any{"var": "logs"}, map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current.data.metrics.count"}}}, 0},
					expected: "0 + COALESCE((SELECT SUM(elem.data.metrics.count) FROM UNNEST(logs) AS elem), 0)",
					hasError: false,
				},

				// All tests - dialect-specific array length function
				{
					name:     "all with condition",
					operator: "all",
					args:     []any{map[string]any{"var": "ages"}, map[string]any{">=": []any{map[string]any{"var": ""}, 18}}},
					expected: func() string {
						switch d.dialect {
						case dialect.DialectPostgreSQL:
							return "(CARDINALITY(ages) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18)))"
						case dialect.DialectDuckDB:
							return "(length(ages) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18)))"
						case dialect.DialectClickHouse:
							return "(length(ages) > 0 AND arrayAll(elem -> elem >= 18, ages))"
						case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner:
							return "(ARRAY_LENGTH(ages) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18)))"
						}
						return ""
					}(),
					hasError: false,
				},

				// Some tests
				{
					name:     "some with condition",
					operator: "some",
					args:     []any{map[string]any{"var": "statuses"}, map[string]any{"==": []any{map[string]any{"var": ""}, "active"}}},
					expected: "EXISTS (SELECT 1 FROM UNNEST(statuses) AS elem WHERE elem = 'active')",
					hasError: false,
				},

				// None tests
				{
					name:     "none with condition",
					operator: "none",
					args:     []any{map[string]any{"var": "values"}, map[string]any{"==": []any{map[string]any{"var": ""}, "invalid"}}},
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
					expected := arrayTestDuckDBUnnestSourceAliases(d.dialect, tt.expected)
					if result != expected {
						t.Errorf("[%s] Expected %s, got %s", d.name, expected, result)
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
		// ClickHouse tests - uses arrayConcat
		{
			name:     "ClickHouse merge two arrays",
			dialect:  dialect.DialectClickHouse,
			args:     []any{map[string]any{"var": "array1"}, map[string]any{"var": "array2"}},
			expected: "arrayConcat(array1, array2)",
		},
		{
			name:     "ClickHouse merge three arrays",
			dialect:  dialect.DialectClickHouse,
			args:     []any{map[string]any{"var": "a"}, map[string]any{"var": "b"}, map[string]any{"var": "c"}},
			expected: "arrayConcat(a, b, c)",
		},
		{
			name:     "ClickHouse merge single array",
			dialect:  dialect.DialectClickHouse,
			args:     []any{map[string]any{"var": "arr"}},
			expected: "arrayConcat(arr)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewOperatorConfig(tt.dialect, &arrayTestSchemaProvider{})
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
			expected: "arrayFold((acc, elem) -> (acc - elem), numbers, 100)",
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

func TestArrayOperator_ToSQLParam_Merge(t *testing.T) {
	op := newArrayOperatorWithParamParserBQ(t)
	args := []interface{}{
		[]interface{}{1, 2},
		[]interface{}{3, 4},
	}
	pc := params.NewParamCollector(params.PlaceholderNamed)

	got, err := op.ToSQLParam("merge", args, pc)
	if err != nil {
		t.Fatalf("ToSQLParam(merge): %v", err)
	}
	want := "ARRAY_CONCAT([@p1, @p2], [@p3, @p4])"
	if got != want {
		t.Errorf("SQL = %q, want %q", got, want)
	}
	assertCollectedParams(t, pc, []params.QueryParam{
		{Name: "p1", Value: 1},
		{Name: "p2", Value: 2},
		{Name: "p3", Value: 3},
		{Name: "p4", Value: 4},
	})
}

func TestArrayOperator_valueToSQLParam(t *testing.T) {
	op := newArrayOperatorWithParamParserBQ(t)

	tests := []struct {
		name        string
		input       interface{}
		expectedSQL string
		wantParams  []params.QueryParam
		hasError    bool
	}{
		{
			name:        "string literal",
			input:       "test",
			expectedSQL: "@p1",
			wantParams:  []params.QueryParam{{Name: "p1", Value: "test"}},
			hasError:    false,
		},
		{
			name:        "number literal",
			input:       42,
			expectedSQL: "@p1",
			wantParams:  []params.QueryParam{{Name: "p1", Value: 42}},
			hasError:    false,
		},
		{
			name:        "boolean true",
			input:       true,
			expectedSQL: "TRUE",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "nil",
			input:       nil,
			expectedSQL: "NULL",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "ProcessedValue SQL",
			input:       ProcessedValue{Value: "elem.some_col", IsSQL: true},
			expectedSQL: "elem.some_col",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "array literal",
			input:       []interface{}{1, 2, 3},
			expectedSQL: "[@p1, @p2, @p3]",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 1},
				{Name: "p2", Value: 2},
				{Name: "p3", Value: 3},
			},
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			result, err := op.valueToSQLParam(tt.input, pc)

			if tt.hasError {
				if err == nil {
					t.Errorf("valueToSQLParam() expected error, got nil")
				}
				if pc.Count() != 0 {
					t.Errorf("valueToSQLParam() on error expected Count() 0, got %d", pc.Count())
				}
				return
			}
			if err != nil {
				t.Fatalf("valueToSQLParam() unexpected error = %v", err)
			}
			if result != tt.expectedSQL {
				t.Errorf("valueToSQLParam() = %q, want %q", result, tt.expectedSQL)
			}
			assertCollectedParams(t, pc, tt.wantParams)
		})
	}
}

func TestArrayOperator_ToSQLParam_UnsupportedOperator(t *testing.T) {
	op := newArrayOperatorWithParamParserBQ(t)
	pc := params.NewParamCollector(params.PlaceholderNamed)

	_, err := op.ToSQLParam("unsupported", []interface{}{[]interface{}{1}}, pc)
	if err == nil {
		t.Fatal("expected error for unsupported array operator")
	}
	if pc.Count() != 0 {
		t.Errorf("expected no params on error, got Count()=%d", pc.Count())
	}
}

func TestArrayOperator_ToSQLParam_DialectValidation(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectUnspecified, &arrayTestSchemaProvider{})
	config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
		return "", fmt.Errorf("unexpected ParamExpressionParser in dialect validation test")
	})
	op := NewArrayOperator(config)

	minMapArgs := []interface{}{
		[]interface{}{1, 2},
		map[string]interface{}{"var": ""},
	}
	minFilterArgs := []interface{}{
		[]interface{}{1, 2, 3},
		map[string]interface{}{">": []interface{}{map[string]interface{}{"var": ""}, 0}},
	}
	minReduceArgs := []interface{}{
		[]interface{}{1, 2},
		map[string]interface{}{"+": []interface{}{
			map[string]interface{}{"var": "accumulator"},
			map[string]interface{}{"var": "current"},
		}},
		0,
	}
	minQuantifierArgs := []interface{}{
		[]interface{}{1},
		map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": ""}, 1}},
	}

	tests := []struct {
		name     string
		operator string
		args     []interface{}
	}{
		{name: "map", operator: "map", args: minMapArgs},
		{name: "filter", operator: "filter", args: minFilterArgs},
		{name: "reduce", operator: "reduce", args: minReduceArgs},
		{name: "all", operator: "all", args: minQuantifierArgs},
		{name: "some", operator: "some", args: minQuantifierArgs},
		{name: "none", operator: "none", args: minQuantifierArgs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			_, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if err == nil {
				t.Fatalf("expected dialect validation error for operator %q", tt.operator)
			}
			if pc.Count() != 0 {
				t.Errorf("expected no params when dialect validation fails, got Count()=%d", pc.Count())
			}
		})
	}
}
