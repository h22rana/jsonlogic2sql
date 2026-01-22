package jsonlogic2sql

import (
	"testing"
)

// TestDialectComplianceAllOperators verifies that all operators produce valid SQL
// across all supported dialects. This ensures ANSI SQL compliance where applicable.
func TestDialectComplianceAllOperators(t *testing.T) {
	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	// Common test cases that should work across all dialects
	commonTests := []struct {
		name        string
		input       string
		description string
	}{
		// Data access operators
		{"var simple", `{"var": "field"}`, "Simple variable reference"},
		{"var dotted", `{"var": "user.name"}`, "Dotted variable reference"},
		{"var with default string", `{"var": ["status", "pending"]}`, "Variable with string default"},
		{"var with default number", `{"var": ["count", 0]}`, "Variable with numeric default"},
		{"var with default null", `{"var": ["field", null]}`, "Variable with null default"},
		{"missing single", `{"missing": "field"}`, "Missing single field check"},
		{"missing array", `{"missing": ["a", "b", "c"]}`, "Missing multiple fields check"},
		{"missing_some", `{"missing_some": [2, ["a", "b", "c"]]}`, "Missing some fields check"},

		// Comparison operators
		{"equality numbers", `{"==": [1, 2]}`, "Number equality"},
		{"equality strings", `{"==": [{"var": "name"}, "John"]}`, "String equality"},
		{"equality null", `{"==": [{"var": "deleted_at"}, null]}`, "Null equality uses IS NULL"},
		{"strict equality", `{"===": [{"var": "status"}, "active"]}`, "Strict equality"},
		{"inequality numbers", `{"!=": [{"var": "count"}, 0]}`, "Number inequality"},
		{"inequality null", `{"!=": [{"var": "field"}, null]}`, "Null inequality uses IS NOT NULL"},
		{"strict inequality", `{"!==": [{"var": "type"}, "unknown"]}`, "Strict inequality"},
		{"greater than", `{">": [{"var": "amount"}, 1000]}`, "Greater than comparison"},
		{"greater than or equal", `{">=": [{"var": "score"}, 80]}`, "Greater than or equal comparison"},
		{"less than", `{"<": [{"var": "age"}, 18]}`, "Less than comparison"},
		{"less than or equal", `{"<=": [{"var": "count"}, 100]}`, "Less than or equal comparison"},
		{"in array", `{"in": [{"var": "status"}, ["active", "pending"]]}`, "Array membership check"},
		{"chained less than", `{"<": [0, {"var": "value"}, 100]}`, "Chained comparison (between)"},
		{"chained greater than or equal", `{">=": [100, {"var": "value"}, 0]}`, "Chained greater than or equal"},
		{"four value chain", `{"<": [1, {"var": "a"}, {"var": "b"}, 100]}`, "Four-value chained comparison"},

		// Logical operators
		{"and single", `{"and": [{"var": "flag"}]}`, "AND with single condition"},
		{"and multiple", `{"and": [{">": [{"var": "a"}, 1]}, {"<": [{"var": "b"}, 10]}]}`, "AND with multiple conditions"},
		{"or single", `{"or": [{"var": "flag"}]}`, "OR with single condition"},
		{"or multiple", `{"or": [{"==": [{"var": "x"}, 1]}, {"==": [{"var": "y"}, 2]}]}`, "OR with multiple conditions"},
		{"not simple", `{"!": [{"var": "flag"}]}`, "NOT operator"},
		{"not with condition", `{"!": [{"==": [{"var": "status"}, "deleted"]}]}`, "NOT with nested condition"},
		{"double bang", `{"!!": [{"var": "value"}]}`, "Boolean conversion"},
		{"if simple", `{"if": [{">": [{"var": "age"}, 18]}, "adult", "minor"]}`, "Simple IF condition"},
		{"if without else", `{"if": [{">": [{"var": "x"}, 0]}, "positive"]}`, "IF without else"},
		{"if multi-branch", `{"if": [{">": [{"var": "score"}, 90]}, "A", {">": [{"var": "score"}, 80]}, "B", "C"]}`, "Multi-branch IF"},

		// Numeric operators
		{"addition binary", `{"+": [{"var": "a"}, {"var": "b"}]}`, "Binary addition"},
		{"addition multi", `{"+": [1, 2, 3, 4]}`, "Multi-value addition"},
		{"unary plus", `{"+": [{"var": "value"}]}`, "Unary plus (numeric cast)"},
		{"subtraction binary", `{"-": [{"var": "total"}, {"var": "discount"}]}`, "Binary subtraction"},
		{"unary minus", `{"-": [{"var": "amount"}]}`, "Unary minus (negation)"},
		{"multiplication", `{"*": [{"var": "price"}, {"var": "quantity"}]}`, "Multiplication"},
		{"division", `{"/": [{"var": "total"}, 2]}`, "Division"},
		{"modulo", `{"%": [{"var": "count"}, 3]}`, "Modulo operation"},
		{"max", `{"max": [{"var": "a"}, {"var": "b"}, {"var": "c"}]}`, "Maximum value"},
		{"min", `{"min": [{"var": "x"}, {"var": "y"}]}`, "Minimum value"},
		{"nested arithmetic", `{"+": [{"*": [{"var": "a"}, 2]}, {"/": [{"var": "b"}, 3]}]}`, "Nested arithmetic"},

		// String operators
		{"cat simple", `{"cat": ["Hello", " ", "World"]}`, "Simple concatenation"},
		{"cat with vars", `{"cat": [{"var": "first"}, " ", {"var": "last"}]}`, "Concatenation with variables"},
		{"substr two args", `{"substr": [{"var": "text"}, 0]}`, "Substring from start"},
		{"substr three args", `{"substr": [{"var": "text"}, 0, 5]}`, "Substring with length"},

		// Complex nested expressions
		{"nested and or", `{"and": [{">": [{"var": "a"}, 1]}, {"or": [{"<": [{"var": "b"}, 5]}, {"==": [{"var": "c"}, true]}]}]}`, "Nested AND/OR"},
		{"comparison with arithmetic", `{">": [{"+": [{"var": "x"}, {"var": "y"}]}, 100]}`, "Comparison with arithmetic operand"},
		{"if with comparison result", `{"if": [{">": [{"var": "qty"}, 10]}, {"*": [{"var": "price"}, 0.9]}, {"var": "price"}]}`, "IF with arithmetic result"},
		{"max min nested", `{"max": [{"min": [{"var": "a"}, {"var": "b"}]}, {"min": [{"var": "c"}, {"var": "d"}]}]}`, "Nested max/min"},
	}

	for _, d := range dialects {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("Failed to create transpiler for %s: %v", d.String(), err)
			}

			for _, tt := range commonTests {
				t.Run(tt.name, func(t *testing.T) {
					result, err := tr.Transpile(tt.input)
					if err != nil {
						t.Errorf("[%s] %s: Transpile() error = %v, input = %s", d.String(), tt.description, err, tt.input)
						return
					}
					if result == "" {
						t.Errorf("[%s] %s: Transpile() returned empty result", d.String(), tt.description)
					}
					// Verify result starts with WHERE
					if len(result) < 6 || result[:5] != "WHERE" {
						t.Errorf("[%s] %s: Transpile() result should start with WHERE, got: %s", d.String(), tt.description, result)
					}
				})
			}
		})
	}
}

// TestDialectSpecificArrayOperators tests array operators with dialect-specific output.
func TestDialectSpecificArrayOperators(t *testing.T) {
	type testCase struct {
		name     string
		input    string
		expected map[Dialect]string // expected output per dialect
	}

	tests := []testCase{
		{
			name:  "map transformation",
			input: `{"map": [{"var": "numbers"}, {"*": [{"var": "item"}, 2]}]}`,
			expected: map[Dialect]string{
				DialectBigQuery:   "WHERE ARRAY(SELECT (elem * 2) FROM UNNEST(numbers) AS elem)",
				DialectSpanner:    "WHERE ARRAY(SELECT (elem * 2) FROM UNNEST(numbers) AS elem)",
				DialectPostgreSQL: "WHERE ARRAY(SELECT (elem * 2) FROM UNNEST(numbers) AS elem)",
				DialectDuckDB:     "WHERE ARRAY(SELECT (elem * 2) FROM UNNEST(numbers) AS elem)",
				DialectClickHouse: "WHERE arrayMap(elem -> (elem * 2), numbers)",
			},
		},
		{
			name:  "filter condition",
			input: `{"filter": [{"var": "scores"}, {">": [{"var": "item"}, 70]}]}`,
			expected: map[Dialect]string{
				DialectBigQuery:   "WHERE ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem > 70)",
				DialectSpanner:    "WHERE ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem > 70)",
				DialectPostgreSQL: "WHERE ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem > 70)",
				DialectDuckDB:     "WHERE ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem > 70)",
				DialectClickHouse: "WHERE arrayFilter(elem -> elem > 70, scores)",
			},
		},
		{
			name:  "all elements check",
			input: `{"all": [{"var": "ages"}, {">=": [{"var": "item"}, 18]}]}`,
			expected: map[Dialect]string{
				DialectBigQuery:   "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18))",
				DialectSpanner:    "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18))",
				DialectPostgreSQL: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18))",
				DialectDuckDB:     "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18))",
				DialectClickHouse: "WHERE arrayAll(elem -> elem >= 18, ages)",
			},
		},
		{
			name:  "some elements check",
			input: `{"some": [{"var": "items"}, {"==": [{"var": "item"}, "active"]}]}`,
			expected: map[Dialect]string{
				DialectBigQuery:   "WHERE EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE elem = 'active')",
				DialectSpanner:    "WHERE EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE elem = 'active')",
				DialectPostgreSQL: "WHERE EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE elem = 'active')",
				DialectDuckDB:     "WHERE EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE elem = 'active')",
				DialectClickHouse: "WHERE arrayExists(elem -> elem = 'active', items)",
			},
		},
		{
			name:  "none elements check",
			input: `{"none": [{"var": "values"}, {"==": [{"var": "item"}, "error"]}]}`,
			expected: map[Dialect]string{
				DialectBigQuery:   "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE elem = 'error')",
				DialectSpanner:    "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE elem = 'error')",
				DialectPostgreSQL: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE elem = 'error')",
				DialectDuckDB:     "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE elem = 'error')",
				DialectClickHouse: "WHERE NOT arrayExists(elem -> elem = 'error', values)",
			},
		},
		{
			name:  "reduce SUM pattern",
			input: `{"reduce": [{"var": "numbers"}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}`,
			expected: map[Dialect]string{
				DialectBigQuery:   "WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST(numbers) AS elem), 0)",
				DialectSpanner:    "WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST(numbers) AS elem), 0)",
				DialectPostgreSQL: "WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST(numbers) AS elem), 0)",
				DialectDuckDB:     "WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST(numbers) AS elem), 0)",
				DialectClickHouse: "WHERE 0 + coalesce(arrayReduce('sum', numbers), 0)",
			},
		},
		{
			name:  "merge two arrays",
			input: `{"merge": [{"var": "arr1"}, {"var": "arr2"}]}`,
			expected: map[Dialect]string{
				DialectBigQuery:   "WHERE ARRAY_CONCAT(arr1, arr2)",
				DialectSpanner:    "WHERE ARRAY_CONCAT(arr1, arr2)",
				DialectPostgreSQL: "WHERE (arr1 || arr2)",
				DialectDuckDB:     "WHERE ARRAY_CONCAT(arr1, arr2)",
				DialectClickHouse: "WHERE arrayConcat(arr1, arr2)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for dialect, expected := range tt.expected {
				t.Run(dialect.String(), func(t *testing.T) {
					tr, err := NewTranspiler(dialect)
					if err != nil {
						t.Fatalf("Failed to create transpiler for %s: %v", dialect.String(), err)
					}

					result, err := tr.Transpile(tt.input)
					if err != nil {
						t.Errorf("[%s] Transpile() error = %v", dialect.String(), err)
						return
					}
					if result != expected {
						t.Errorf("[%s] Transpile() = %q, want %q", dialect.String(), result, expected)
					}
				})
			}
		})
	}
}

// TestDialectSpecificStringFunctions tests string position functions across dialects.
func TestDialectSpecificStringFunctions(t *testing.T) {
	type testCase struct {
		name     string
		input    string
		expected map[Dialect]string
	}

	tests := []testCase{
		{
			name:  "in string containment with var on right",
			input: `{"in": ["test", {"var": "description"}]}`,
			expected: map[Dialect]string{
				DialectBigQuery:   "WHERE STRPOS(description, 'test') > 0",
				DialectSpanner:    "WHERE STRPOS(description, 'test') > 0",
				DialectPostgreSQL: "WHERE POSITION('test' IN description) > 0",
				DialectDuckDB:     "WHERE STRPOS(description, 'test') > 0",
				DialectClickHouse: "WHERE position(description, 'test') > 0",
			},
		},
		{
			name:  "substr with start and length",
			input: `{"substr": [{"var": "text"}, 5, 10]}`,
			expected: map[Dialect]string{
				DialectBigQuery:   "WHERE SUBSTR(text, 6, 10)",
				DialectSpanner:    "WHERE SUBSTR(text, 6, 10)",
				DialectPostgreSQL: "WHERE SUBSTR(text, 6, 10)",
				DialectDuckDB:     "WHERE SUBSTR(text, 6, 10)",
				DialectClickHouse: "WHERE substring(text, 6, 10)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for dialect, expected := range tt.expected {
				t.Run(dialect.String(), func(t *testing.T) {
					tr, err := NewTranspiler(dialect)
					if err != nil {
						t.Fatalf("Failed to create transpiler for %s: %v", dialect.String(), err)
					}

					result, err := tr.Transpile(tt.input)
					if err != nil {
						t.Errorf("[%s] Transpile() error = %v", dialect.String(), err)
						return
					}
					if result != expected {
						t.Errorf("[%s] Transpile() = %q, want %q", dialect.String(), result, expected)
					}
				})
			}
		})
	}
}

// TestEdgeCasesNullHandling tests null handling edge cases.
func TestEdgeCasesNullHandling(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"null == null", `{"==": [null, null]}`, "WHERE NULL IS NULL"},
		{"null != null", `{"!=": [null, null]}`, "WHERE NULL IS NOT NULL"},
		{"null === null", `{"===": [null, null]}`, "WHERE NULL IS NULL"},
		{"null !== null", `{"!==": [null, null]}`, "WHERE NULL IS NOT NULL"},
		{"var == null", `{"==": [{"var": "field"}, null]}`, "WHERE field IS NULL"},
		{"null == var", `{"==": [null, {"var": "field"}]}`, "WHERE field IS NULL"},
		{"var != null", `{"!=": [{"var": "field"}, null]}`, "WHERE field IS NOT NULL"},
		{"null != var", `{"!=": [null, {"var": "field"}]}`, "WHERE field IS NOT NULL"},
		{"null in arithmetic", `{"+": [{"var": "value"}, null]}`, "WHERE (value + NULL)"},
		{"null in comparison chain", `{"<": [0, {"var": "x"}, null]}`, "WHERE (0 < x AND x < NULL)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.Transpile(tt.input)
			if err != nil {
				t.Errorf("Transpile() error = %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("Transpile() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestEdgeCasesBooleanValues tests boolean value handling.
func TestEdgeCasesBooleanValues(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"true literal", `{"==": [{"var": "flag"}, true]}`, "WHERE flag = TRUE"},
		{"false literal", `{"==": [{"var": "flag"}, false]}`, "WHERE flag = FALSE"},
		{"boolean in and", `{"and": [true, false]}`, "WHERE (TRUE AND FALSE)"},
		{"boolean in or", `{"or": [false, true]}`, "WHERE (FALSE OR TRUE)"},
		{"not true", `{"!": [true]}`, "WHERE NOT (TRUE)"},
		{"not false", `{"!": [false]}`, "WHERE NOT (FALSE)"},
		{"double bang true", `{"!!": [true]}`, "WHERE (TRUE IS NOT NULL AND TRUE != FALSE AND TRUE != 0 AND TRUE != '')"},
		{"double bang false", `{"!!": [false]}`, "WHERE (FALSE IS NOT NULL AND FALSE != FALSE AND FALSE != 0 AND FALSE != '')"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.Transpile(tt.input)
			if err != nil {
				t.Errorf("Transpile() error = %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("Transpile() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestEdgeCasesEmptyInputs tests handling of empty or minimal inputs.
func TestEdgeCasesEmptyInputs(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		hasError bool
	}{
		{"empty string input", ``, true},
		{"empty object", `{}`, true},
		{"only whitespace", `   `, true},
		{"invalid JSON", `{invalid}`, true},
		{"array at root", `[1, 2, 3]`, true},
		{"primitive at root string", `"hello"`, true},
		{"primitive at root number", `42`, true},
		{"and with empty array", `{"and": []}`, true},
		{"or with empty array", `{"or": []}`, true},
		{"cat with empty array", `{"cat": []}`, true},
		{"in with empty array", `{"in": [{"var": "x"}, []]}`, true},
		{"missing with empty array", `{"missing": []}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tr.Transpile(tt.input)
			if tt.hasError && err == nil {
				t.Errorf("Transpile() expected error for input: %s", tt.input)
			}
			if !tt.hasError && err != nil {
				t.Errorf("Transpile() unexpected error = %v for input: %s", err, tt.input)
			}
		})
	}
}

// TestEdgeCasesSpecialCharacters tests handling of special characters in strings.
func TestEdgeCasesSpecialCharacters(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"single quote in string", `{"==": [{"var": "name"}, "O'Brien"]}`, "WHERE name = 'O''Brien'"},
		{"unicode characters", `{"==": [{"var": "text"}, "日本語"]}`, "WHERE text = '日本語'"},
		{"unicode with parentheses", `{"==": [{"var": "shop"}, "SPA(スパ)"]}`, "WHERE shop = 'SPA(スパ)'"},
		{"empty string", `{"==": [{"var": "value"}, ""]}`, "WHERE value = ''"},
		{"string with spaces", `{"==": [{"var": "name"}, "John Doe"]}`, "WHERE name = 'John Doe'"},
		{"string with SQL keywords", `{"==": [{"var": "desc"}, "SELECT * FROM users"]}`, "WHERE desc = 'SELECT * FROM users'"},
		{"string with comparison operators", `{"==": [{"var": "formula"}, "a > b AND c < d"]}`, "WHERE formula = 'a > b AND c < d'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.Transpile(tt.input)
			if err != nil {
				t.Errorf("Transpile() error = %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("Transpile() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestEdgeCasesNumericBoundaries tests handling of numeric boundary values.
func TestEdgeCasesNumericBoundaries(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"zero", `{"==": [{"var": "x"}, 0]}`, "WHERE x = 0"},
		{"negative number", `{"==": [{"var": "x"}, -100]}`, "WHERE x = -100"},
		{"large number", `{"==": [{"var": "x"}, 9999999999999]}`, "WHERE x = 9.999999999999e+12"}, // JSON numbers are float64
		{"decimal", `{"==": [{"var": "x"}, 3.14159]}`, "WHERE x = 3.14159"},
		{"scientific notation", `{"==": [{"var": "x"}, 1e10]}`, "WHERE x = 1e+10"}, // Preserved as scientific notation
		{"negative decimal", `{"==": [{"var": "x"}, -0.001]}`, "WHERE x = -0.001"},
		{"negative in subtraction", `{"-": [5, -3]}`, "WHERE (5 - -3)"},
		{"unary minus on negative", `{"-": [-5]}`, "WHERE (--5)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.Transpile(tt.input)
			if err != nil {
				t.Errorf("Transpile() error = %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("Transpile() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestEdgeCasesDeeplyNested tests handling of deeply nested expressions.
func TestEdgeCasesDeeplyNested(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	tests := []struct {
		name  string
		input string
	}{
		{
			"nested and 5 levels",
			`{"and": [{"and": [{"and": [{"and": [{"and": [{">": [{"var": "x"}, 0]}]}]}]}]}]}`,
		},
		{
			"nested or 5 levels",
			`{"or": [{"or": [{"or": [{"or": [{"or": [{"<": [{"var": "y"}, 100]}]}]}]}]}]}`,
		},
		{
			"nested if 3 levels",
			`{"if": [{"var": "a"}, {"if": [{"var": "b"}, {"if": [{"var": "c"}, "deep", "c_false"]}, "b_false"]}, "a_false"]}`,
		},
		{
			"nested arithmetic 4 levels",
			`{"+": [{"+": [{"+": [{"+": [{"var": "a"}, {"var": "b"}]}, {"var": "c"}]}, {"var": "d"}]}, {"var": "e"}]}`,
		},
		{
			"mixed nested logical and comparison",
			`{"and": [{"or": [{">": [{"var": "a"}, 1]}, {"<": [{"var": "b"}, 2]}]}, {"and": [{"!=": [{"var": "c"}, null]}, {"or": [{">=": [{"var": "d"}, 10]}, {"<=": [{"var": "e"}, 20]}]}]}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.Transpile(tt.input)
			if err != nil {
				t.Errorf("Transpile() error = %v", err)
				return
			}
			if result == "" {
				t.Errorf("Transpile() returned empty result for deeply nested expression")
			}
		})
	}
}

// TestANSIComplianceCommonOperators verifies that standard ANSI SQL operators
// are used consistently across all dialects where applicable.
func TestANSIComplianceCommonOperators(t *testing.T) {
	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	// These SQL constructs should be identical across all dialects
	ansiTests := []struct {
		name        string
		input       string
		mustContain []string
		mustNotFail bool
		description string
	}{
		{
			name:        "CASE WHEN for if",
			input:       `{"if": [{">": [{"var": "x"}, 0]}, "pos", "neg"]}`,
			mustContain: []string{"CASE", "WHEN", "THEN", "ELSE", "END"},
			description: "IF should translate to CASE WHEN THEN ELSE END",
		},
		{
			name:        "IS NULL for null equality",
			input:       `{"==": [{"var": "x"}, null]}`,
			mustContain: []string{"IS NULL"},
			description: "Null equality should use IS NULL",
		},
		{
			name:        "IS NOT NULL for null inequality",
			input:       `{"!=": [{"var": "x"}, null]}`,
			mustContain: []string{"IS NOT NULL"},
			description: "Null inequality should use IS NOT NULL",
		},
		{
			name:        "AND for logical and",
			input:       `{"and": [{">": [{"var": "a"}, 1]}, {"<": [{"var": "b"}, 10]}]}`,
			mustContain: []string{"AND"},
			description: "Logical AND should use AND keyword",
		},
		{
			name:        "OR for logical or",
			input:       `{"or": [{"==": [{"var": "x"}, 1]}, {"==": [{"var": "y"}, 2]}]}`,
			mustContain: []string{"OR"},
			description: "Logical OR should use OR keyword",
		},
		{
			name:        "NOT for negation",
			input:       `{"!": [{"var": "flag"}]}`,
			mustContain: []string{"NOT"},
			description: "Logical NOT should use NOT keyword",
		},
		{
			name:        "IN for array membership",
			input:       `{"in": [{"var": "x"}, [1, 2, 3]]}`,
			mustContain: []string{"IN"},
			description: "Array membership should use IN keyword",
		},
		{
			name:        "COALESCE for default values",
			input:       `{"var": ["field", "default"]}`,
			mustContain: []string{"COALESCE"},
			description: "Default values should use COALESCE",
		},
		{
			name:        "GREATEST for max",
			input:       `{"max": [{"var": "a"}, {"var": "b"}]}`,
			mustContain: []string{"GREATEST"},
			description: "Max should use GREATEST function",
		},
		{
			name:        "LEAST for min",
			input:       `{"min": [{"var": "a"}, {"var": "b"}]}`,
			mustContain: []string{"LEAST"},
			description: "Min should use LEAST function",
		},
		{
			name:        "CONCAT for string concatenation",
			input:       `{"cat": ["a", "b", "c"]}`,
			mustContain: []string{"CONCAT"},
			description: "String concatenation should use CONCAT",
		},
	}

	for _, tt := range ansiTests {
		t.Run(tt.name, func(t *testing.T) {
			for _, d := range dialects {
				t.Run(d.String(), func(t *testing.T) {
					tr, err := NewTranspiler(d)
					if err != nil {
						t.Fatalf("Failed to create transpiler: %v", err)
					}

					result, err := tr.Transpile(tt.input)
					if err != nil {
						t.Errorf("[%s] %s: error = %v", d.String(), tt.description, err)
						return
					}

					for _, expected := range tt.mustContain {
						if !containsString(result, expected) {
							t.Errorf("[%s] %s: result %q should contain %q", d.String(), tt.description, result, expected)
						}
					}
				})
			}
		})
	}
}

// containsString is a helper function to check if a string contains a substring.
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestTranspileConditionAllDialects tests TranspileCondition across all dialects.
func TestTranspileConditionAllDialects(t *testing.T) {
	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	input := `{">": [{"var": "amount"}, 1000]}`
	expected := "amount > 1000"

	for _, d := range dialects {
		t.Run(d.String(), func(t *testing.T) {
			result, err := TranspileCondition(d, input)
			if err != nil {
				t.Errorf("[%s] TranspileCondition() error = %v", d.String(), err)
				return
			}
			if result != expected {
				t.Errorf("[%s] TranspileCondition() = %q, want %q", d.String(), result, expected)
			}
		})
	}
}
