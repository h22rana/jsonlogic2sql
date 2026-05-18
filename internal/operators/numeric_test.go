package operators

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
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
			name:     "addition coerces boolean literals",
			operator: "+",
			args:     []interface{}{true, false, 2},
			expected: "(1 + 0 + 2)",
			hasError: false,
		},
		{
			name:     "addition coerces predicate SQL",
			operator: "+",
			args: []interface{}{
				TypedSQLResult("x > 1", ExpressionKindPredicate, ExpressionTypeBoolean),
				2,
			},
			expected: "((CASE WHEN x > 1 THEN 1 ELSE 0 END) + 2)",
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
			name:     "unary minus with negative literal",
			operator: "-",
			args:     []interface{}{-5},
			expected: "(-(-5))",
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

// numericSchemaProvider is a configurable schema provider for numeric operator tests.
type numericSchemaProvider struct {
	fields map[string]string
}

func (m *numericSchemaProvider) HasField(fieldName string) bool {
	_, ok := m.fields[fieldName]
	return ok
}

func (m *numericSchemaProvider) GetFieldType(fieldName string) string {
	return m.fields[fieldName]
}

func (m *numericSchemaProvider) ValidateField(_ string) error {
	return nil
}

func (m *numericSchemaProvider) IsArrayType(fieldName string) bool {
	return m.fields[fieldName] == "array"
}

func (m *numericSchemaProvider) IsStringType(fieldName string) bool {
	return m.fields[fieldName] == "string"
}

func (m *numericSchemaProvider) IsNumericType(fieldName string) bool {
	t := m.fields[fieldName]
	return t == "integer" || t == "number"
}

func (m *numericSchemaProvider) IsBooleanType(fieldName string) bool {
	return m.fields[fieldName] == "boolean"
}

func (m *numericSchemaProvider) IsEnumType(_ string) bool {
	return false
}

func (m *numericSchemaProvider) GetAllowedValues(_ string) []string {
	return nil
}

func (m *numericSchemaProvider) ValidateEnumValue(_, _ string) error {
	return nil
}

func TestNumericOperator_validateNumericOperand(t *testing.T) {
	schema := &numericSchemaProvider{
		fields: map[string]string{
			"amount":   "integer",
			"price":    "number",
			"name":     "string",
			"tags":     "array",
			"verified": "boolean",
		},
	}

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewNumericOperator(config)

	tests := []struct {
		name     string
		value    interface{}
		hasError bool
	}{
		{
			name:     "numeric integer field passes",
			value:    map[string]interface{}{"var": "amount"},
			hasError: false,
		},
		{
			name:     "numeric number field passes",
			value:    map[string]interface{}{"var": "price"},
			hasError: false,
		},
		{
			name:     "string field fails",
			value:    map[string]interface{}{"var": "name"},
			hasError: true,
		},
		{
			name:     "array field fails",
			value:    map[string]interface{}{"var": "tags"},
			hasError: true,
		},
		{
			name:     "boolean field fails",
			value:    map[string]interface{}{"var": "verified"},
			hasError: true,
		},
		{
			name:     "literal number - no validation",
			value:    42,
			hasError: false,
		},
		{
			name:     "non-var map - no validation",
			value:    map[string]interface{}{"other": "value"},
			hasError: false,
		},
		{
			name:     "unknown field - no validation (no type in schema)",
			value:    map[string]interface{}{"var": "unknown_field"},
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := op.validateNumericOperand(tt.value)
			if tt.hasError {
				if err == nil {
					t.Errorf("validateNumericOperand() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("validateNumericOperand() unexpected error = %v", err)
				}
			}
		})
	}

	// Test with nil schema - should always pass
	opNoSchema := NewNumericOperator(nil)
	if err := opNoSchema.validateNumericOperand(map[string]interface{}{"var": "name"}); err != nil {
		t.Errorf("validateNumericOperand() with nil schema should pass, got %v", err)
	}
}

func TestNumericOperator_ToSQL_WithSchemaValidation(t *testing.T) {
	schema := &numericSchemaProvider{
		fields: map[string]string{
			"amount": "integer",
			"name":   "string",
		},
	}

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewNumericOperator(config)

	// Should fail: string field in numeric operation
	_, err := op.ToSQL("+", []interface{}{
		map[string]interface{}{"var": "name"},
		10,
	})
	if err == nil {
		t.Errorf("ToSQL() expected error for string field in addition, got nil")
	}

	// Should succeed: numeric field in numeric operation
	result, err := op.ToSQL("+", []interface{}{
		map[string]interface{}{"var": "amount"},
		10,
	})
	if err != nil {
		t.Errorf("ToSQL() unexpected error = %v", err)
	}
	if result != "(amount + 10)" {
		t.Errorf("ToSQL() = %v, want (amount + 10)", result)
	}
}

func TestNumericOperator_valueToSQL_ProcessedValue(t *testing.T) {
	op := NewNumericOperator(nil)

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
	op := NewNumericOperator(nil)

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
			expected: "(a % b)",
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
	op := NewNumericOperator(nil)

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
	op := NewNumericOperator(nil)

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
	op := NewNumericOperator(nil)

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
	config := NewOperatorConfig(dialect.DialectBigQuery, nil)
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
	op := NewNumericOperator(nil)

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
	op := NewNumericOperator(nil)

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
	op := NewNumericOperator(nil)

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

func assertNumericQueryParams(t *testing.T, got, want []params.QueryParam) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("params: got %d entries, want %d: %#v vs %#v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i].Name != want[i].Name {
			t.Errorf("param[%d].Name = %q, want %q", i, got[i].Name, want[i].Name)
		}
		gv, wv := got[i].Value, want[i].Value
		if !numericParamValuesEqual(gv, wv) {
			t.Errorf("param[%d].Value = %#v (%T), want %#v (%T)", i, gv, gv, wv, wv)
		}
	}
}

// numericParamValuesEqual treats float64 and int whole numbers as equal for JSON/Go literal drift.
func numericParamValuesEqual(a, b interface{}) bool {
	if a == b {
		return true
	}
	af, aok := toFloat64ForCompare(a)
	bf, bok := toFloat64ForCompare(b)
	return aok && bok && af == bf
}

func toFloat64ForCompare(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	default:
		return 0, false
	}
}

func TestNumericOperator_ToSQLParam(t *testing.T) {
	op := NewNumericOperator(nil)

	tests := []struct {
		name       string
		operator   string
		args       []interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
		skipParams bool
	}{
		{
			name:     "addition with two numbers",
			operator: "+",
			args:     []interface{}{5, 3},
			wantSQL:  "(@p1 + @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 5},
				{Name: "p2", Value: 3},
			},
		},
		{
			name:     "addition coerces boolean literals",
			operator: "+",
			args:     []interface{}{true, false, 2},
			wantSQL:  "(1 + 0 + @p1)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 2},
			},
		},
		{
			name:     "addition coerces predicate SQL",
			operator: "+",
			args: []interface{}{
				TypedSQLResult("x > 1", ExpressionKindPredicate, ExpressionTypeBoolean),
				2,
			},
			wantSQL: "((CASE WHEN x > 1 THEN 1 ELSE 0 END) + @p1)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 2},
			},
		},
		{
			name:     "addition with var and number",
			operator: "+",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, 100},
			wantSQL:  "(amount + @p1)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 100},
			},
		},
		{
			name:     "unary plus",
			operator: "+",
			args:     []interface{}{5},
			wantSQL:  "CAST(@p1 AS NUMERIC)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 5},
			},
		},
		{
			name:     "subtraction with two numbers",
			operator: "-",
			args:     []interface{}{10, 3},
			wantSQL:  "(@p1 - @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 10},
				{Name: "p2", Value: 3},
			},
		},
		{
			name:     "unary minus",
			operator: "-",
			args:     []interface{}{10},
			wantSQL:  "(-@p1)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 10},
			},
		},
		{
			name:       "unary minus with var",
			operator:   "-",
			args:       []interface{}{map[string]interface{}{"var": "value"}},
			wantSQL:    "(-value)",
			wantParams: nil,
		},
		{
			name:     "multiplication",
			operator: "*",
			args:     []interface{}{4, 5},
			wantSQL:  "(@p1 * @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 4},
				{Name: "p2", Value: 5},
			},
		},
		{
			name:     "division",
			operator: "/",
			args:     []interface{}{20, 4},
			wantSQL:  "(@p1 / @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 20},
				{Name: "p2", Value: 4},
			},
		},
		{
			name:     "modulo",
			operator: "%",
			args:     []interface{}{17, 5},
			wantSQL:  "(@p1 % @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 17},
				{Name: "p2", Value: 5},
			},
		},
		{
			name:     "max",
			operator: "max",
			args:     []interface{}{10, 20},
			wantSQL:  "GREATEST(@p1, @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 10},
				{Name: "p2", Value: 20},
			},
		},
		{
			name:     "min",
			operator: "min",
			args:     []interface{}{5, 15},
			wantSQL:  "LEAST(@p1, @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 5},
				{Name: "p2", Value: 15},
			},
		},
		{
			name:       "unsupported operator",
			operator:   "avg",
			args:       []interface{}{1, 2},
			wantErr:    true,
			skipParams: true,
		},
		{
			name:       "multiplication too few arguments",
			operator:   "*",
			args:       []interface{}{5},
			wantErr:    true,
			skipParams: true,
		},
		{
			name:       "division too few arguments",
			operator:   "/",
			args:       []interface{}{10},
			wantErr:    true,
			skipParams: true,
		},
		{
			name:       "modulo too few arguments",
			operator:   "%",
			args:       []interface{}{17},
			wantErr:    true,
			skipParams: true,
		},
		{
			name:       "max too few arguments",
			operator:   "max",
			args:       []interface{}{10},
			wantErr:    true,
			skipParams: true,
		},
		{
			name:       "min too few arguments",
			operator:   "min",
			args:       []interface{}{5},
			wantErr:    true,
			skipParams: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			gotSQL, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ToSQLParam() expected error, got nil (sql=%q)", gotSQL)
				}
				return
			}
			if err != nil {
				t.Fatalf("ToSQLParam() unexpected error: %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("ToSQLParam() sql = %q, want %q", gotSQL, tt.wantSQL)
			}
			if !tt.skipParams {
				assertNumericQueryParams(t, pc.Params(), tt.wantParams)
			}
		})
	}
}

func TestNumericOperator_valueToSQLParam(t *testing.T) {
	op := NewNumericOperator(nil)

	tests := []struct {
		name       string
		input      interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:    "literal number",
			input:   42,
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 42},
			},
		},
		{
			name:    "literal float",
			input:   3.14,
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 3.14},
			},
		},
		{
			name:       "var expression",
			input:      map[string]interface{}{"var": "amount"},
			wantSQL:    "amount",
			wantParams: nil,
		},
		{
			name:       "ProcessedValue SQL",
			input:      ProcessedValue{Value: "SUM(amount)", IsSQL: true},
			wantSQL:    "SUM(amount)",
			wantParams: nil,
		},
		{
			name:       "ProcessedValue predicate SQL",
			input:      TypedSQLResult("x > 1", ExpressionKindPredicate, ExpressionTypeBoolean),
			wantSQL:    "(CASE WHEN x > 1 THEN 1 ELSE 0 END)",
			wantParams: nil,
		},
		{
			name:       "boolean literal true",
			input:      true,
			wantSQL:    "1",
			wantParams: nil,
		},
		{
			name:       "boolean literal false",
			input:      false,
			wantSQL:    "0",
			wantParams: nil,
		},
		{
			name:    "ProcessedValue literal",
			input:   ProcessedValue{Value: "hello", IsSQL: false},
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "hello"},
			},
		},
		{
			name:    "numeric string coerced to integer",
			input:   "42",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: int64(42)},
			},
		},
		{
			name:    "numeric string coerced to float",
			input:   "3.14",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 3.14},
			},
		},
		{
			name:    "non-numeric string safely parameterized",
			input:   "hello",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "hello"},
			},
		},
		{
			name:    "large integer beyond int64 preserved as string",
			input:   "9223372036854775808",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "9223372036854775808"},
			},
		},
		{
			name:    "negative large integer preserved as string",
			input:   "-9223372036854775809",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "-9223372036854775809"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			gotSQL, err := op.valueToSQLParam(tt.input, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("valueToSQLParam() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("valueToSQLParam() unexpected error: %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("valueToSQLParam() = %q, want %q", gotSQL, tt.wantSQL)
			}
			assertNumericQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestNumericOperator_ToSQLParam_WithSchemaValidation(t *testing.T) {
	schema := &numericSchemaProvider{
		fields: map[string]string{
			"amount": "integer",
			"name":   "string",
		},
	}

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewNumericOperator(config)

	t.Run("string field in numeric operation fails", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		_, err := op.ToSQLParam("+", []interface{}{
			map[string]interface{}{"var": "name"},
			10,
		}, pc)
		if err == nil {
			t.Fatal("ToSQLParam() expected error for string field in addition, got nil")
		}
		if len(pc.Params()) != 0 {
			t.Errorf("expected no params on error, got %#v", pc.Params())
		}
	})

	t.Run("numeric field succeeds", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		sql, err := op.ToSQLParam("+", []interface{}{
			map[string]interface{}{"var": "amount"},
			10,
		}, pc)
		if err != nil {
			t.Fatalf("ToSQLParam() unexpected error: %v", err)
		}
		if sql != "(amount + @p1)" {
			t.Errorf("ToSQLParam() = %q, want (amount + @p1)", sql)
		}
		assertNumericQueryParams(t, pc.Params(), []params.QueryParam{
			{Name: "p1", Value: 10},
		})
	})
}

func TestNumericOperator_valueToSQLParam_ExpressionParserCallback(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, nil)
	config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
		if pc == nil {
			t.Error("ParamCollector should be non-nil")
		}
		return "CUSTOM_NUMERIC_PARAM()", nil
	})
	op := NewNumericOperator(config)

	pc := params.NewParamCollector(params.PlaceholderNamed)
	result, err := op.valueToSQLParam(map[string]interface{}{"customOp": []interface{}{1, 2}}, pc)
	if err != nil {
		t.Fatalf("valueToSQLParam() unexpected error: %v", err)
	}
	if result != "CUSTOM_NUMERIC_PARAM()" {
		t.Errorf("valueToSQLParam() = %q, want CUSTOM_NUMERIC_PARAM()", result)
	}
	if len(pc.Params()) != 0 {
		t.Errorf("expected no params, got %#v", pc.Params())
	}
}

func TestNumericOperator_processComplexArgsParam(t *testing.T) {
	op := NewNumericOperator(nil)

	pc := params.NewParamCollector(params.PlaceholderNamed)
	out, err := op.processComplexArgsParam([]interface{}{
		float64(2),
		map[string]interface{}{"var": "price"},
	}, pc)
	if err != nil {
		t.Fatalf("processComplexArgsParam() unexpected error: %v", err)
	}
	if len(out) != 2 || out[0] != "@p1" || out[1] != "price" {
		t.Fatalf("processComplexArgsParam() = %#v, want [@p1 price]", out)
	}
	assertNumericQueryParams(t, pc.Params(), []params.QueryParam{
		{Name: "p1", Value: float64(2)},
	})
}

func TestNumericOperator_processComplexArgsForComparisonParam(t *testing.T) {
	op := NewNumericOperator(nil)

	t.Run("var and primitive pass through", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		args := []interface{}{
			map[string]interface{}{"var": "amount"},
			42,
		}
		result, err := op.processComplexArgsForComparisonParam(args, pc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("returned %d args, want 2", len(result))
		}
		varMap, ok := result[0].(map[string]interface{})
		if !ok {
			t.Fatalf("[0] is %T, want map[string]interface{}", result[0])
		}
		if _, hasVar := varMap["var"]; !hasVar {
			t.Error("[0] has no 'var' key")
		}
		num, ok := result[1].(int)
		if !ok {
			t.Fatalf("[1] is %T, want int", result[1])
		}
		if num != 42 {
			t.Errorf("[1] = %v, want 42", num)
		}
		if len(pc.Params()) != 0 {
			t.Errorf("expected no params, got %#v", pc.Params())
		}
	})

	t.Run("nested expression wrapped as SQLResult", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		args := []interface{}{
			map[string]interface{}{"+": []interface{}{
				map[string]interface{}{"var": "x"}, float64(1),
			}},
			float64(10),
		}
		result, err := op.processComplexArgsForComparisonParam(args, pc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("returned %d args, want 2", len(result))
		}
		pv, ok := result[0].(ProcessedValue)
		if !ok {
			t.Fatalf("[0] is %T, want ProcessedValue", result[0])
		}
		if !pv.IsSQL || pv.Value != "(x + @p1)" {
			t.Errorf("[0] = %+v, want SQLResult('(x + @p1)')", pv)
		}
		if result[1] != float64(10) {
			t.Errorf("[1] = %v, want 10", result[1])
		}
		assertNumericQueryParams(t, pc.Params(), []params.QueryParam{
			{Name: "p1", Value: float64(1)},
		})
	})
}
