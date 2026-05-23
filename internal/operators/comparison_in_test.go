package operators

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

func TestComparisonOperator_valueToSQL_Extended(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		// ProcessedValue - SQL
		{
			name:     "ProcessedValue with SQL",
			input:    ProcessedValue{Value: "some_column > 5", IsSQL: true},
			expected: "some_column > 5",
			hasError: false,
		},
		// ProcessedValue - Literal
		{
			name:     "ProcessedValue with literal string",
			input:    ProcessedValue{Value: "hello", IsSQL: false},
			expected: "'hello'",
			hasError: false,
		},
		// nil value
		{
			name:     "nil value",
			input:    nil,
			expected: "NULL",
			hasError: false,
		},
		// boolean false
		{
			name:     "boolean false",
			input:    false,
			expected: "FALSE",
			hasError: false,
		},
		// empty var name (current element reference)
		{
			name:     "empty var name returns elem",
			input:    map[string]interface{}{"var": ""},
			expected: "elem",
			hasError: false,
		},
		// Arithmetic expression inside comparison
		{
			name:     "addition expression",
			input:    map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": "x"}, 10}},
			expected: "(x + 10)",
			hasError: false,
		},
		{
			name:     "subtraction expression",
			input:    map[string]interface{}{"-": []interface{}{map[string]interface{}{"var": "x"}, 5}},
			expected: "(x - 5)",
			hasError: false,
		},
		{
			name:     "multiplication expression",
			input:    map[string]interface{}{"*": []interface{}{map[string]interface{}{"var": "x"}, 2}},
			expected: "(x * 2)",
			hasError: false,
		},
		{
			name:     "division expression",
			input:    map[string]interface{}{"/": []interface{}{map[string]interface{}{"var": "x"}, 2}},
			expected: "(x / 2)",
			hasError: false,
		},
		{
			name:     "modulo expression",
			input:    map[string]interface{}{"%": []interface{}{map[string]interface{}{"var": "x"}, 3}},
			expected: "MOD(CAST(x AS NUMERIC), CAST(3 AS NUMERIC))",
			hasError: false,
		},
		// Comparison expression inside comparison
		{
			name:     "nested greater than expression",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "a"}, 5}},
			expected: "(a > 5)",
			hasError: false,
		},
		// Min/Max expression
		{
			name:     "max expression",
			input:    map[string]interface{}{"max": []interface{}{map[string]interface{}{"var": "a"}, 100}},
			expected: "GREATEST(a, 100)",
			hasError: false,
		},
		{
			name:     "min expression",
			input:    map[string]interface{}{"min": []interface{}{map[string]interface{}{"var": "a"}, 0}},
			expected: "LEAST(a, 0)",
			hasError: false,
		},
		// If expression
		{
			name: "if expression",
			input: map[string]interface{}{"if": []interface{}{
				map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "x"}, 0}},
				"positive",
				"negative",
			}},
			expected: "CASE WHEN x > 0 THEN 'positive' ELSE 'negative' END",
			hasError: false,
		},
		{
			name:     "if expression non-array args error",
			input:    map[string]interface{}{"if": "invalid"},
			expected: "",
			hasError: true,
		},
		// Cat/Substr string operations
		{
			name:     "cat expression",
			input:    map[string]interface{}{"cat": []interface{}{"hello", " ", "world"}},
			expected: "CONCAT('hello', ' ', 'world')",
			hasError: false,
		},
		{
			name:     "cat expression non-array error",
			input:    map[string]interface{}{"cat": "invalid"},
			expected: "",
			hasError: true,
		},
		{
			name:     "substr expression",
			input:    map[string]interface{}{"substr": []interface{}{"hello", 1, 3}},
			expected: "SUBSTR('hello', 2, 3)",
			hasError: false,
		},
		{
			name:     "substr expression non-array error",
			input:    map[string]interface{}{"substr": "invalid"},
			expected: "",
			hasError: true,
		},
		// Array should error
		{
			name:     "array value should error",
			input:    []interface{}{1, 2, 3},
			expected: "",
			hasError: true,
		},
		// Array operators
		{
			name: "reduce expression non-array error",
			input: map[string]interface{}{
				"reduce": "invalid",
			},
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
					t.Errorf("valueToSQL() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestComparisonOperator_handleIn_WithVarRightSide(t *testing.T) {
	// Test in with var on right side and schema indicating array type
	schema := newComparisonSchemaProvider(map[string]string{
		"tags":        "array",
		"description": "string",
		"name":        "string",
	})

	tests := []struct {
		name     string
		dialect  dialect.Dialect
		leftArg  interface{}
		rightArg interface{}
		expected string
		hasError bool
	}{
		// Array field on right side: use arrayMembershipSQL
		{
			name:     "BigQuery in with array var on right",
			dialect:  dialect.DialectBigQuery,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "tags"},
			expected: testNullSafeArrayMembershipSQL(dialect.DialectBigQuery, "'test'", "tags"),
			hasError: false,
		},
		{
			name:     "PostgreSQL in with array var on right",
			dialect:  dialect.DialectPostgreSQL,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "tags"},
			expected: testNullSafeArrayMembershipSQL(dialect.DialectPostgreSQL, "'test'", "tags"),
			hasError: false,
		},
		{
			name:     "DuckDB in with array var on right",
			dialect:  dialect.DialectDuckDB,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "tags"},
			expected: testNullSafeArrayMembershipSQL(dialect.DialectDuckDB, "'test'", "tags"),
			hasError: false,
		},
		{
			name:     "ClickHouse in with array var on right",
			dialect:  dialect.DialectClickHouse,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "tags"},
			expected: testNullSafeArrayMembershipSQL(dialect.DialectClickHouse, "'test'", "tags"),
			hasError: false,
		},
		// String field on right side: use STRPOS
		{
			name:     "BigQuery in with string var on right",
			dialect:  dialect.DialectBigQuery,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "description"},
			expected: "STRPOS(description, 'test') > 0",
			hasError: false,
		},
		{
			name:     "PostgreSQL in with string var on right",
			dialect:  dialect.DialectPostgreSQL,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "description"},
			expected: "POSITION('test' IN description) > 0",
			hasError: false,
		},
		{
			name:     "ClickHouse in with string var on right",
			dialect:  dialect.DialectClickHouse,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "description"},
			expected: "position(description, 'test') > 0",
			hasError: false,
		},
		// Non-container right-hand literals are known false in JSONLogic.
		{
			name:     "in with number on right side folds false",
			dialect:  dialect.DialectBigQuery,
			leftArg:  "3",
			rightArg: float64(12345),
			expected: "FALSE",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewOperatorConfig(tt.dialect, schema)
			op := NewComparisonOperator(config)
			result, err := op.ToSQL("in", []interface{}{tt.leftArg, tt.rightArg})
			if tt.hasError {
				if err == nil {
					t.Errorf("ToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("ToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("ToSQL() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestComparisonOperator_handleIn_SchemaRequired_VarRightSide(t *testing.T) {
	// With field-only schema metadata, test heuristic based on left side being a literal.
	op := NewComparisonOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		leftArg  interface{}
		rightArg interface{}
		expected string
		hasError bool
	}{
		{
			name:     "literal string left and var right - string containment",
			leftArg:  "search",
			rightArg: map[string]interface{}{"var": "field"},
			expected: "STRPOS(field, 'search') > 0",
			hasError: false,
		},
		{
			name:     "var left and var right - array membership fallback",
			leftArg:  map[string]interface{}{"var": "item"},
			rightArg: map[string]interface{}{"var": "collection"},
			expected: testNullSafeArrayMembershipSQL(dialect.DialectUnspecified, "item", "collection"),
			hasError: false,
		},
		{
			name:     "boolean right side folds false",
			leftArg:  "test",
			rightArg: true,
			expected: "FALSE",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.ToSQL("in", []interface{}{tt.leftArg, tt.rightArg})
			if tt.hasError {
				if err == nil {
					t.Errorf("ToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("ToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("ToSQL() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestComparisonOperator_handleIn_SchemaRequired_StringExpressionHeuristic(t *testing.T) {
	leftStringExpr := map[string]interface{}{
		OpCat: []interface{}{
			map[string]interface{}{
				OpSubstr: []interface{}{
					map[string]interface{}{OpVar: "profile.first"},
					float64(0),
					float64(2),
				},
			},
			"-x",
		},
	}
	leftNumericExpr := map[string]interface{}{
		"+": []interface{}{float64(1), float64(2)},
	}
	rightVar := map[string]interface{}{OpVar: "profile.name"}

	tests := []struct {
		name          string
		d             dialect.Dialect
		leftArg       interface{}
		expectedSQL   string
		expectedError bool
	}{
		{
			name:    "bigquery nested string expression uses containment",
			d:       dialect.DialectBigQuery,
			leftArg: leftStringExpr,
			expectedSQL: testRuntimeStringContainmentSQL(
				dialect.DialectBigQuery,
				"profile.name",
				"CONCAT(COALESCE(SUBSTR(profile.first, 1, 2), ''), '-x')",
			),
		},
		{
			name:    "postgres nested string expression uses containment",
			d:       dialect.DialectPostgreSQL,
			leftArg: leftStringExpr,
			expectedSQL: testRuntimeStringContainmentSQL(
				dialect.DialectPostgreSQL,
				"profile.name",
				"CONCAT(COALESCE(SUBSTR(profile.first, 1, 2), ''), '-x')",
			),
		},
		{
			name:    "clickhouse nested string expression uses containment",
			d:       dialect.DialectClickHouse,
			leftArg: leftStringExpr,
			expectedSQL: testRuntimeStringContainmentSQL(
				dialect.DialectClickHouse,
				"profile.name",
				"CONCAT(COALESCE(substring(profile.first, 1, 2), ''), '-x')",
			),
		},
		{
			name:        "bigquery numeric expression uses null-safe membership fallback",
			d:           dialect.DialectBigQuery,
			leftArg:     leftNumericExpr,
			expectedSQL: testNullSafeArrayMembershipSQL(dialect.DialectBigQuery, "(1 + 2)", "profile.name"),
		},
		{
			name:        "postgres numeric expression uses null-safe membership fallback",
			d:           dialect.DialectPostgreSQL,
			leftArg:     leftNumericExpr,
			expectedSQL: testNullSafeArrayMembershipSQL(dialect.DialectPostgreSQL, "(1 + 2)", "profile.name"),
		},
		{
			name:        "clickhouse numeric expression uses null-safe membership fallback",
			d:           dialect.DialectClickHouse,
			leftArg:     leftNumericExpr,
			expectedSQL: testNullSafeArrayMembershipSQL(dialect.DialectClickHouse, "(1 + 2)", "profile.name"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := NewComparisonOperator(NewOperatorConfig(tt.d, &fieldOnlySchemaProvider{}))
			got, err := op.ToSQL("in", []interface{}{tt.leftArg, rightVar})
			if tt.expectedError {
				if err == nil {
					t.Fatal("ToSQL() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ToSQL() error = %v", err)
			}
			if got != tt.expectedSQL {
				t.Fatalf("ToSQL() = %q, want %q", got, tt.expectedSQL)
			}
		})
	}
}

func TestComparisonOperator_handleIn_WithEnumValidation(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"status": "string",
	})
	schema.enumValues["status"] = []string{"active", "inactive", "pending"}

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	// Valid enum values in array
	result, err := op.ToSQL("in", []interface{}{
		map[string]interface{}{"var": "status"},
		[]interface{}{"active", "pending"},
	})
	if err != nil {
		t.Errorf("ToSQL() unexpected error = %v", err)
	}
	expected := "status IN ('active', 'pending')"
	if result != expected {
		t.Errorf("ToSQL() = %v, want %v", result, expected)
	}

	// Invalid enum values in array
	_, err = op.ToSQL("in", []interface{}{
		map[string]interface{}{"var": "status"},
		[]interface{}{"active", "deleted"},
	})
	if err == nil {
		t.Errorf("ToSQL() expected error for invalid enum value, got nil")
	}
}

func TestComparisonOperator_ToSQL_WithSchemaCoercion(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"age":  "integer",
		"name": "string",
	})

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		expected string
		hasError bool
	}{
		{
			name:     "string to number coercion - left field numeric, right literal string",
			operator: ">=",
			args:     []interface{}{map[string]interface{}{"var": "age"}, "50000"},
			expected: "age >= 50000",
			hasError: false,
		},
		{
			name:     "number to string coercion - left field string, right literal number",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "name"}, float64(5960)},
			expected: "name = '5960'",
			hasError: false,
		},
		{
			name:     "coercion for right field and left literal",
			operator: "==",
			args:     []interface{}{"50000", map[string]interface{}{"var": "age"}},
			expected: "50000 = age",
			hasError: false,
		},
		{
			name:     "coercion for right field string and left literal number",
			operator: "==",
			args:     []interface{}{float64(42), map[string]interface{}{"var": "name"}},
			expected: "'42' = name",
			hasError: false,
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
					t.Errorf("ToSQL() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestComparisonOperator_handleIn_WithSchemaArrayVar(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"tags": "array",
	})

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	// Test in with array-type field on right side (array membership)
	result, err := op.ToSQL("in", []interface{}{
		"test",
		map[string]interface{}{"var": "tags"},
	})
	if err != nil {
		t.Errorf("ToSQL() unexpected error = %v", err)
	}
	expected := testNullSafeArrayMembershipSQL(dialect.DialectBigQuery, "'test'", "tags")
	if result != expected {
		t.Errorf("ToSQL() = %v, want %v", result, expected)
	}

	result, err = op.ToSQL("in", []interface{}{
		map[string]interface{}{"var": "tags"},
		[]interface{}{"test", "other"},
	})
	if err != nil {
		t.Errorf("ToSQL() unexpected error for array-valued left operand = %v", err)
	}
	if expected := "FALSE"; result != expected {
		t.Errorf("ToSQL() = %v, want %v", result, expected)
	}
}

func TestComparisonOperator_valueToSQL_ExpressionParserCallback(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	config.SetExpressionParser(func(expr any, path string) (string, error) {
		return "CUSTOM_FUNC()", nil
	})
	op := NewComparisonOperator(config)

	// Unknown operator with expression parser set should delegate
	result, err := op.valueToSQL(map[string]interface{}{"customOp": []interface{}{1, 2}})
	if err != nil {
		t.Errorf("valueToSQL() unexpected error = %v", err)
	}
	if result != "CUSTOM_FUNC()" {
		t.Errorf("valueToSQL() = %v, want CUSTOM_FUNC()", result)
	}
}

func TestComparisonOperator_handleIn_StringCoercion(t *testing.T) {
	// Test that number left side gets coerced to string when right side is a string field
	schema := newComparisonSchemaProvider(map[string]string{
		"name": "string",
	})

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	// Number left side with string var on right - should coerce number to string
	result, err := op.ToSQL("in", []interface{}{
		float64(123),
		map[string]interface{}{"var": "name"},
	})
	if err != nil {
		t.Errorf("ToSQL() unexpected error = %v", err)
	}
	expected := "STRPOS(name, '123') > 0"
	if result != expected {
		t.Errorf("ToSQL() = %v, want %v", result, expected)
	}
}
