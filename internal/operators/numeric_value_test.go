package operators

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

func TestNumericOperator_valueToSQL_ProcessedValue(t *testing.T) {
	op := NewNumericOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "ProcessedValue with SQL",
			input:    ProcessedValue{Value: "SUM(amount)", IsSQL: true},
			expected: "SUM(amount)",
			hasError: false,
		},
		{
			name:     "ProcessedValue with literal",
			input:    ProcessedValue{Value: "hello", IsSQL: false},
			expected: "'hello'",
			hasError: false,
		},
		{
			name:     "plain string safely quoted (not passed through as SQL)",
			input:    "pre_processed_sql",
			expected: "'pre_processed_sql'",
			hasError: false,
		},
		{
			name:     "numeric string coerced to integer",
			input:    "42",
			expected: "42",
			hasError: false,
		},
		{
			name:     "numeric string coerced to float",
			input:    "3.14",
			expected: "3.14",
			hasError: false,
		},
		{
			name:     "negative numeric string coerced",
			input:    "-7",
			expected: "-7",
			hasError: false,
		},
		{
			name:     "injection attempt safely quoted",
			input:    "1 OR 1=1",
			expected: "'1 OR 1=1'",
			hasError: false,
		},
		{
			name:     "SQL injection with semicolon safely quoted",
			input:    "1; DROP TABLE users",
			expected: "'1; DROP TABLE users'",
			hasError: false,
		},
		{
			name:     "single quote in string properly escaped",
			input:    "it's",
			expected: "'it''s'",
			hasError: false,
		},
		{
			name:     "NaN safely quoted",
			input:    "NaN",
			expected: "'NaN'",
			hasError: false,
		},
		{
			name:     "+Inf safely quoted",
			input:    "+Inf",
			expected: "'+Inf'",
			hasError: false,
		},
		{
			name:     "-Inf safely quoted",
			input:    "-Inf",
			expected: "'-Inf'",
			hasError: false,
		},
		{
			name:     "Inf safely quoted",
			input:    "Inf",
			expected: "'Inf'",
			hasError: false,
		},
		{
			name:     "large integer beyond int64 preserved exactly",
			input:    "9223372036854775808",
			expected: "9223372036854775808",
			hasError: false,
		},
		{
			name:     "integer at float64 precision boundary preserved exactly",
			input:    "9007199254740993",
			expected: "9007199254740993",
			hasError: false,
		},
		{
			name:     "scientific notation coerced via float",
			input:    "1e3",
			expected: "1000",
			hasError: false,
		},
		{
			name:     "positive sign integer preserved",
			input:    "+42",
			expected: "+42",
			hasError: false,
		},
		{
			name:     "whitespace-padded integer trimmed",
			input:    " 3 ",
			expected: "3",
			hasError: false,
		},
		{
			name:     "whitespace-padded float trimmed",
			input:    " 3.14 ",
			expected: "3.14",
			hasError: false,
		},
		{
			name:     "tab-padded integer trimmed",
			input:    "\t5\t",
			expected: "5",
			hasError: false,
		},
		{
			name:     "whitespace-only string quoted",
			input:    "   ",
			expected: "'   '",
			hasError: false,
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

func TestNumericOperator_generateComplexSQL(t *testing.T) {
	op := NewNumericOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		operator string
		args     []string
		expected string
		hasError bool
	}{
		{
			name:     "addition two args",
			operator: "+",
			args:     []string{"a", "b"},
			expected: "(a + b)",
			hasError: false,
		},
		{
			name:     "addition three args",
			operator: "+",
			args:     []string{"a", "b", "c"},
			expected: "(a + b + c)",
			hasError: false,
		},
		{
			name:     "addition insufficient args",
			operator: "+",
			args:     []string{"a"},
			expected: "",
			hasError: true,
		},
		{
			name:     "subtraction two args",
			operator: "-",
			args:     []string{"a", "b"},
			expected: "(a - b)",
			hasError: false,
		},
		{
			name:     "subtraction unary minus",
			operator: "-",
			args:     []string{"x"},
			expected: "(-x)",
			hasError: false,
		},
		{
			name:     "multiplication two args",
			operator: "*",
			args:     []string{"a", "b"},
			expected: "(a * b)",
			hasError: false,
		},
		{
			name:     "multiplication insufficient args",
			operator: "*",
			args:     []string{"a"},
			expected: "",
			hasError: true,
		},
		{
			name:     "division two args",
			operator: "/",
			args:     []string{"a", "b"},
			expected: "(a / b)",
			hasError: false,
		},
		{
			name:     "division insufficient args",
			operator: "/",
			args:     []string{"a"},
			expected: "",
			hasError: true,
		},
		{
			name:     "modulo two args",
			operator: "%",
			args:     []string{"a", "b"},
			expected: "MOD(CAST(a AS NUMERIC), CAST(b AS NUMERIC))",
			hasError: false,
		},
		{
			name:     "modulo insufficient args",
			operator: "%",
			args:     []string{"a"},
			expected: "",
			hasError: true,
		},
		{
			name:     "max two args",
			operator: "max",
			args:     []string{"a", "b"},
			expected: "GREATEST(a, b)",
			hasError: false,
		},
		{
			name:     "max insufficient args",
			operator: "max",
			args:     []string{"a"},
			expected: "",
			hasError: true,
		},
		{
			name:     "min two args",
			operator: "min",
			args:     []string{"a", "b"},
			expected: "LEAST(a, b)",
			hasError: false,
		},
		{
			name:     "min insufficient args",
			operator: "min",
			args:     []string{"a"},
			expected: "",
			hasError: true,
		},
		{
			name:     "unsupported operator",
			operator: "^",
			args:     []string{"a", "b"},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.generateComplexSQL(tt.operator, tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("generateComplexSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("generateComplexSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("generateComplexSQL() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestNumericOperator_valueToSQL_NestedComparison(t *testing.T) {
	op := NewNumericOperator(testFieldOnlyConfig())

	// processComplexArgsForComparison now wraps results in SQLResult,
	// so the comparison operator sees them as pre-processed SQL:
	// - {"var": "status"} resolves to "status" (column name, unquoted)
	// - "active" resolves to "'active'" (literal, quoted by dataOp)
	input := map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "active"}}
	result, err := op.valueToSQL(input)
	if err != nil {
		t.Errorf("valueToSQL() unexpected error = %v", err)
	}
	expected := "(CASE WHEN status = 'active' THEN 1 ELSE 0 END)"
	if result != expected {
		t.Errorf("valueToSQL() = %v, want %v", result, expected)
	}
}

func TestNumericOperator_valueToSQL_NestedIf(t *testing.T) {
	op := NewNumericOperator(testFieldOnlyConfig())

	// Test nested if expression
	input := map[string]interface{}{
		"if": []interface{}{
			map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "x"}, 0}},
			1,
			0,
		},
	}
	result, err := op.valueToSQL(input)
	if err != nil {
		t.Errorf("valueToSQL() unexpected error = %v", err)
	}
	expected := "CASE WHEN x > 0 THEN 1 ELSE 0 END"
	if result != expected {
		t.Errorf("valueToSQL() = %v, want %v", result, expected)
	}
}

func TestNumericOperator_valueToSQL_NestedLogical(t *testing.T) {
	op := NewNumericOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name: "nested and",
			input: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "x"}, 0}},
					map[string]interface{}{"<": []interface{}{map[string]interface{}{"var": "x"}, 100}},
				},
			},
			expected: "(CASE WHEN x > 0 AND x < 100 THEN 1 ELSE 0 END)",
			hasError: false,
		},
		{
			name: "nested or",
			input: map[string]interface{}{
				"or": []interface{}{
					map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "a"}},
					map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "b"}},
				},
			},
			expected: "(CASE WHEN status = 'a' OR status = 'b' THEN 1 ELSE 0 END)",
			hasError: false,
		},
		{
			name: "nested not",
			input: map[string]interface{}{
				"!": []interface{}{
					map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "x"}, 0}},
				},
			},
			expected: "(CASE WHEN NOT (x = 0) THEN 1 ELSE 0 END)",
			hasError: false,
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

func TestNumericOperator_valueToSQL_ExpressionParserCallback(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	config.SetExpressionParser(func(expr any, path string) (string, error) {
		return "CUSTOM_NUMERIC()", nil
	})
	op := NewNumericOperator(config)

	// Unknown operator with expression parser should delegate
	result, err := op.valueToSQL(map[string]interface{}{"customOp": []interface{}{1, 2}})
	if err != nil {
		t.Errorf("valueToSQL() unexpected error = %v", err)
	}
	if result != "CUSTOM_NUMERIC()" {
		t.Errorf("valueToSQL() = %v, want CUSTOM_NUMERIC()", result)
	}
}

func TestNumericOperator_processComplexArgsForComparison(t *testing.T) {
	op := NewNumericOperator(testFieldOnlyConfig())

	t.Run("var and primitive pass through", func(t *testing.T) {
		args := []interface{}{
			map[string]interface{}{"var": "amount"},
			42,
		}
		result, err := op.processComplexArgsForComparison(args)
		if err != nil {
			t.Fatalf("unexpected error = %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("returned %d args, want 2", len(result))
		}

		// Var expression passes through as-is for schema coercion
		varMap, ok := result[0].(map[string]interface{})
		if !ok {
			t.Fatalf("[0] is %T, want map[string]interface{}", result[0])
		}
		if _, hasVar := varMap["var"]; !hasVar {
			t.Errorf("[0] has no 'var' key")
		}

		// Primitive passes through as-is
		num, ok := result[1].(int)
		if !ok {
			t.Fatalf("[1] is %T, want int", result[1])
		}
		if num != 42 {
			t.Errorf("[1] = %v, want 42", num)
		}
	})

	t.Run("nested expression wrapped in SQLResult", func(t *testing.T) {
		args := []interface{}{
			map[string]interface{}{"+": []interface{}{
				map[string]interface{}{"var": "x"}, float64(1),
			}},
			float64(10),
		}
		result, err := op.processComplexArgsForComparison(args)
		if err != nil {
			t.Fatalf("unexpected error = %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("returned %d args, want 2", len(result))
		}

		// Nested arithmetic → pre-processed as SQLResult
		pv, ok := result[0].(ProcessedValue)
		if !ok {
			t.Fatalf("[0] is %T, want ProcessedValue", result[0])
		}
		if !pv.IsSQL || pv.Value != "(x + 1)" {
			t.Errorf("[0] = %+v, want SQLResult('(x + 1)')", pv)
		}

		// Primitive passes through
		if result[1] != float64(10) {
			t.Errorf("[1] = %v, want 10", result[1])
		}
	})

	t.Run("string literal passes through", func(t *testing.T) {
		args := []interface{}{
			map[string]interface{}{"var": "status"},
			"active",
		}
		result, err := op.processComplexArgsForComparison(args)
		if err != nil {
			t.Fatalf("unexpected error = %v", err)
		}
		if result[1] != "active" {
			t.Errorf("[1] = %v, want 'active'", result[1])
		}
	})
}

func TestNumericOperator_extractFieldName(t *testing.T) {
	op := NewNumericOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		varName  interface{}
		expected string
	}{
		{
			name:     "string var name",
			varName:  "field",
			expected: "field",
		},
		{
			name:     "array with string first element",
			varName:  []interface{}{"field", "default"},
			expected: "field",
		},
		{
			name:     "array with non-string first element",
			varName:  []interface{}{123},
			expected: "",
		},
		{
			name:     "empty array",
			varName:  []interface{}{},
			expected: "",
		},
		{
			name:     "non-string non-array",
			varName:  42,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := op.extractFieldName(tt.varName)
			if result != tt.expected {
				t.Errorf("extractFieldName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNumericOperator_extractFieldNameFromValue(t *testing.T) {
	op := NewNumericOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		value    interface{}
		expected string
	}{
		{
			name:     "var expression",
			value:    map[string]interface{}{"var": "amount"},
			expected: "amount",
		},
		{
			name:     "non-var map",
			value:    map[string]interface{}{"other": "value"},
			expected: "",
		},
		{
			name:     "primitive value",
			value:    42,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := op.extractFieldNameFromValue(tt.value)
			if result != tt.expected {
				t.Errorf("extractFieldNameFromValue() = %v, want %v", result, tt.expected)
			}
		})
	}
}
