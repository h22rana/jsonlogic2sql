package operators

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestStringOperator_ToSQL(t *testing.T) {
	op := NewStringOperator(nil)

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
			expected: "CONCAT(firstName, ' ', 'Doe')",
			hasError: false,
		},
		{
			name:     "concatenation with dotted var",
			operator: "cat",
			args:     []interface{}{map[string]interface{}{"var": "user.firstName"}, " ", map[string]interface{}{"var": "user.lastName"}},
			expected: "CONCAT(user.firstName, ' ', user.lastName)",
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
	op := NewStringOperator(nil)

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
	op := NewStringOperator(nil)

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
			expected: "CONCAT(SUBSTR(name, 1, 2), '-', SUBSTR(id, 1, 4))",
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
			expected: "CONCAT(CONCAT('prefix-', name), '-suffix')",
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
			expected: "SUBSTR(CONCAT(first, last), 1, 10)",
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
			expected: "CONCAT(CONCAT(SUBSTR(code, 1, 2), '-'), SUBSTR(id, 1, 4))",
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
			expected: "CONCAT(SUBSTR(card, 1, 4), '****', SUBSTR(card, -3))",
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
			expected: "CONCAT('Max: ', GREATEST(amount, 1000))",
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
			expected: "CONCAT('Min: ', LEAST(value, 0))",
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
	op := NewStringOperator(nil)

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
	op := NewStringOperator(nil)

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
	op := NewStringOperator(nil)

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

func TestStringOperator_valueToSQL_Extended(t *testing.T) {
	op := NewStringOperator(nil)

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "ProcessedValue SQL",
			input:    ProcessedValue{Value: "UPPER(name)", IsSQL: true},
			expected: "UPPER(name)",
			hasError: false,
		},
		{
			name:     "ProcessedValue literal",
			input:    ProcessedValue{Value: "hello", IsSQL: false},
			expected: "'hello'",
			hasError: false,
		},
		{
			name:     "addition inside string context",
			input:    map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": "x"}, 1}},
			expected: "(x + 1)",
			hasError: false,
		},
		{
			name:     "comparison inside string context",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "x"}, 0}},
			expected: "(x > 0)",
			hasError: false,
		},
		{
			name: "comparison remains grouped inside arithmetic string context",
			input: map[string]interface{}{
				"+": []interface{}{
					map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "x"}, 1}},
					1,
				},
			},
			expected: "((x = 1) + 1)",
			hasError: false,
		},
		{
			name:     "not expression",
			input:    map[string]interface{}{"!": []interface{}{map[string]interface{}{"var": "verified"}}},
			expected: "NOT (verified)",
			hasError: false,
		},
		{
			name:     "boolean coercion expression",
			input:    map[string]interface{}{"!!": []interface{}{map[string]interface{}{"var": "value"}}},
			expected: "(value IS NOT NULL AND value != FALSE AND value != 0 AND value != '')",
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

func TestStringOperator_ClickHouseDialect(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectClickHouse, nil)
	op := NewStringOperator(config)

	result, err := op.ToSQL("substr", []interface{}{"Hello World", 6, 5})
	if err != nil {
		t.Errorf("ToSQL() unexpected error = %v", err)
	}
	expected := "substring('Hello World', 7, 5)"
	if result != expected {
		t.Errorf("ToSQL() = %v, want %v", result, expected)
	}
}

func TestStringOperator_valueToSQL_ExpressionParserCallback(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, nil)
	config.SetExpressionParser(func(expr any, path string) (string, error) {
		return "CUSTOM_STRING()", nil
	})
	op := NewStringOperator(config)

	result, err := op.valueToSQL(map[string]interface{}{"toLower": []interface{}{map[string]interface{}{"var": "name"}}})
	if err != nil {
		t.Errorf("valueToSQL() unexpected error = %v", err)
	}
	if result != "CUSTOM_STRING()" {
		t.Errorf("valueToSQL() = %v, want CUSTOM_STRING()", result)
	}
}

func TestStringOperator_convertStartIndex_ComplexExpression(t *testing.T) {
	op := NewStringOperator(nil)

	result, err := op.ToSQL("substr", []interface{}{
		map[string]interface{}{"var": "name"},
		map[string]interface{}{"var": "start_pos"},
	})
	if err != nil {
		t.Errorf("ToSQL() unexpected error = %v", err)
	}
	expected := "SUBSTR(name, (start_pos + 1))"
	if result != expected {
		t.Errorf("ToSQL() = %v, want %v", result, expected)
	}
}

// stringSchemaProvider is a configurable schema provider for string operator tests.
type stringSchemaProvider struct {
	fields map[string]string
}

func (m *stringSchemaProvider) HasField(fieldName string) bool {
	_, ok := m.fields[fieldName]
	return ok
}

func (m *stringSchemaProvider) GetFieldType(fieldName string) string {
	return m.fields[fieldName]
}

func (m *stringSchemaProvider) ValidateField(_ string) error {
	return nil
}

func (m *stringSchemaProvider) IsArrayType(fieldName string) bool {
	return m.fields[fieldName] == "array"
}

func (m *stringSchemaProvider) IsStringType(fieldName string) bool {
	return m.fields[fieldName] == "string"
}

func (m *stringSchemaProvider) IsNumericType(fieldName string) bool {
	t := m.fields[fieldName]
	return t == "integer" || t == "number"
}

func (m *stringSchemaProvider) IsBooleanType(fieldName string) bool {
	return m.fields[fieldName] == "boolean"
}

func (m *stringSchemaProvider) IsEnumType(_ string) bool {
	return false
}

func (m *stringSchemaProvider) GetAllowedValues(_ string) []string {
	return nil
}

func (m *stringSchemaProvider) ValidateEnumValue(_, _ string) error {
	return nil
}

func TestStringOperator_validateStringOperand(t *testing.T) {
	schema := &stringSchemaProvider{
		fields: map[string]string{
			"name":     "string",
			"amount":   "integer",
			"tags":     "array",
			"metadata": "object",
			"verified": "boolean",
		},
	}

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewStringOperator(config)

	tests := []struct {
		name     string
		value    interface{}
		hasError bool
	}{
		{
			name:     "string field passes",
			value:    map[string]interface{}{"var": "name"},
			hasError: false,
		},
		{
			name:     "numeric field passes (implicit conversion)",
			value:    map[string]interface{}{"var": "amount"},
			hasError: false,
		},
		{
			name:     "array field fails",
			value:    map[string]interface{}{"var": "tags"},
			hasError: true,
		},
		{
			name:     "object field fails",
			value:    map[string]interface{}{"var": "metadata"},
			hasError: true,
		},
		{
			name:     "boolean field passes (not explicitly rejected)",
			value:    map[string]interface{}{"var": "verified"},
			hasError: false,
		},
		{
			name:     "literal value - no validation",
			value:    "hello",
			hasError: false,
		},
		{
			name:     "non-var map - no validation",
			value:    map[string]interface{}{"other": "value"},
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := op.validateStringOperand(tt.value)
			if tt.hasError {
				if err == nil {
					t.Errorf("validateStringOperand() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("validateStringOperand() unexpected error = %v", err)
				}
			}
		})
	}
}

func TestStringOperator_extractFieldName(t *testing.T) {
	op := NewStringOperator(nil)

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
			name:     "number",
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

func TestStringOperator_processComparisonExpression(t *testing.T) {
	op := NewStringOperator(nil)

	tests := []struct {
		name     string
		operator string
		args     interface{}
		expected string
		hasError bool
	}{
		{
			name:     "greater than",
			operator: ">",
			args:     []interface{}{map[string]interface{}{"var": "x"}, 5},
			expected: "(x > 5)",
			hasError: false,
		},
		{
			name:     "greater than or equal",
			operator: ">=",
			args:     []interface{}{map[string]interface{}{"var": "x"}, 5},
			expected: "(x >= 5)",
			hasError: false,
		},
		{
			name:     "less than",
			operator: "<",
			args:     []interface{}{map[string]interface{}{"var": "x"}, 5},
			expected: "(x < 5)",
			hasError: false,
		},
		{
			name:     "less than or equal",
			operator: "<=",
			args:     []interface{}{map[string]interface{}{"var": "x"}, 5},
			expected: "(x <= 5)",
			hasError: false,
		},
		{
			name:     "equality",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "x"}, 5},
			expected: "(x = 5)",
			hasError: false,
		},
		{
			name:     "strict equality",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "x"}, 5},
			expected: "(x = 5)",
			hasError: false,
		},
		{
			name:     "inequality",
			operator: "!=",
			args:     []interface{}{map[string]interface{}{"var": "x"}, 5},
			expected: "(x != 5)",
			hasError: false,
		},
		{
			name:     "strict inequality",
			operator: "!==",
			args:     []interface{}{map[string]interface{}{"var": "x"}, 5},
			expected: "(x <> 5)",
			hasError: false,
		},
		{
			name:     "unsupported comparison",
			operator: "<>",
			args:     []interface{}{1, 2},
			expected: "",
			hasError: true,
		},
		{
			name:     "non-array args error",
			operator: ">",
			args:     "invalid",
			expected: "",
			hasError: true,
		},
		{
			name:     "chained comparison",
			operator: ">",
			args:     []interface{}{1, 2, 3},
			expected: "((1 > 2 AND 2 > 3))",
			hasError: false,
		},
		{
			name:     "comparison preserves if branches on left operand",
			operator: "==",
			args: []interface{}{
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
			expected: "(CASE WHEN x > 0 THEN 'a' WHEN y < 0 THEN 'b' END = 'b')",
			hasError: false,
		},
		{
			name:     "comparison preserves if branches on right operand",
			operator: "==",
			args: []interface{}{
				"b",
				map[string]interface{}{
					"if": []interface{}{
						map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "x"}, 0}},
						"a",
						map[string]interface{}{"<": []interface{}{map[string]interface{}{"var": "y"}, 0}},
						"b",
					},
				},
			},
			expected: "('b' = CASE WHEN x > 0 THEN 'a' WHEN y < 0 THEN 'b' END)",
			hasError: false,
		},
		{
			name:     "wrong number of equality args",
			operator: "==",
			args:     []interface{}{1, 2, 3},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.processComparisonExpression(tt.operator, tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("processComparisonExpression() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("processComparisonExpression() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("processComparisonExpression() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestStringOperator_ToSQLParam(t *testing.T) {
	t.Run("cat with strings", func(t *testing.T) {
		op := NewStringOperator(nil)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		sql, err := op.ToSQLParam("cat", []interface{}{"hello", " ", "world"}, pc)
		if err != nil {
			t.Fatalf("ToSQLParam: %v", err)
		}
		wantSQL := "CONCAT(@p1, @p2, @p3)"
		if sql != wantSQL {
			t.Errorf("SQL = %q, want %q", sql, wantSQL)
		}
		wantParams := []params.QueryParam{
			{Name: "p1", Value: "hello"},
			{Name: "p2", Value: " "},
			{Name: "p3", Value: "world"},
		}
		if !reflect.DeepEqual(pc.Params(), wantParams) {
			t.Errorf("Params = %#v, want %#v", pc.Params(), wantParams)
		}
	})

	t.Run("cat with var and string", func(t *testing.T) {
		config := NewOperatorConfig(dialect.DialectBigQuery, nil)
		config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
			if m, ok := expr.(map[string]interface{}); ok {
				if v, ok := m["var"]; ok {
					return fmt.Sprintf("%v", v), nil
				}
			}
			return "", fmt.Errorf("unsupported")
		})
		op := NewStringOperator(config)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		sql, err := op.ToSQLParam("cat", []interface{}{
			map[string]interface{}{"var": "name"},
			"!",
		}, pc)
		if err != nil {
			t.Fatalf("ToSQLParam: %v", err)
		}
		wantSQL := "CONCAT(name, @p1)"
		if sql != wantSQL {
			t.Errorf("SQL = %q, want %q", sql, wantSQL)
		}
		wantParams := []params.QueryParam{{Name: "p1", Value: "!"}}
		if !reflect.DeepEqual(pc.Params(), wantParams) {
			t.Errorf("Params = %#v, want %#v", pc.Params(), wantParams)
		}
	})

	t.Run("cat preserves comparison if branches in parameterized mode", func(t *testing.T) {
		op := NewStringOperator(nil)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		sql, err := op.ToSQLParam("cat", []interface{}{
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
		}, pc)
		if err != nil {
			t.Fatalf("ToSQLParam: %v", err)
		}
		wantSQL := "CONCAT(CASE WHEN (CASE WHEN x > @p1 THEN @p2 WHEN y < @p3 THEN @p4 END = @p5) THEN 'true' ELSE 'false' END)"
		if sql != wantSQL {
			t.Errorf("SQL = %q, want %q", sql, wantSQL)
		}
		wantParams := []params.QueryParam{
			{Name: "p1", Value: 0},
			{Name: "p2", Value: "a"},
			{Name: "p3", Value: 0},
			{Name: "p4", Value: "b"},
			{Name: "p5", Value: "b"},
		}
		if !reflect.DeepEqual(pc.Params(), wantParams) {
			t.Errorf("Params = %#v, want %#v", pc.Params(), wantParams)
		}
	})

	t.Run("substr with string and indices", func(t *testing.T) {
		config := NewOperatorConfig(dialect.DialectBigQuery, nil)
		op := NewStringOperator(config)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		sql, err := op.ToSQLParam("substr", []interface{}{"hello", float64(0), float64(3)}, pc)
		if err != nil {
			t.Fatalf("ToSQLParam: %v", err)
		}
		wantSQL := "SUBSTR(@p1, (@p2 + 1), @p3)"
		if sql != wantSQL {
			t.Errorf("SQL = %q, want %q", sql, wantSQL)
		}
		wantParams := []params.QueryParam{
			{Name: "p1", Value: "hello"},
			{Name: "p2", Value: float64(0)},
			{Name: "p3", Value: float64(3)},
		}
		if !reflect.DeepEqual(pc.Params(), wantParams) {
			t.Errorf("Params = %#v, want %#v", pc.Params(), wantParams)
		}
	})

	t.Run("unsupported string operator", func(t *testing.T) {
		op := NewStringOperator(nil)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		_, err := op.ToSQLParam("unsupported", []interface{}{"x"}, pc)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestStringOperator_valueToSQLParam(t *testing.T) {
	op := NewStringOperator(nil)

	tests := []struct {
		name       string
		value      interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:    "string literal",
			value:   "test",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "test"},
			},
		},
		{
			name:    "number literal",
			value:   42,
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 42},
			},
		},
		{
			name:       "boolean true",
			value:      true,
			wantSQL:    "TRUE",
			wantParams: nil,
		},
		{
			name:       "nil",
			value:      nil,
			wantSQL:    "NULL",
			wantParams: nil,
		},
		{
			name:       "ProcessedValue SQL",
			value:      SQLResult("some_col"),
			wantSQL:    "some_col",
			wantParams: nil,
		},
		{
			name:    "ProcessedValue literal",
			value:   LiteralResult("lit"),
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "lit"},
			},
		},
		{
			name: "comparison coerces to numeric inside parameterized arithmetic string context",
			value: map[string]interface{}{
				"+": []interface{}{
					map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "x"}, 1}},
					1,
				},
			},
			wantSQL: "((CASE WHEN x = @p1 THEN 1 ELSE 0 END) + @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 1},
				{Name: "p2", Value: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.valueToSQLParam(tt.value, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("valueToSQLParam: %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", got, tt.wantSQL)
			}
			if !reflect.DeepEqual(pc.Params(), tt.wantParams) {
				t.Errorf("Params = %#v, want %#v", pc.Params(), tt.wantParams)
			}
		})
	}
}

func TestStringOperator_ToSQLParam_Dialects(t *testing.T) {
	args := []interface{}{"hello", float64(0), float64(3)}
	wantParams := []params.QueryParam{
		{Name: "p1", Value: "hello"},
		{Name: "p2", Value: float64(0)},
		{Name: "p3", Value: float64(3)},
	}

	tests := []struct {
		dialect dialect.Dialect
		wantSQL string
	}{
		{dialect.DialectBigQuery, "SUBSTR(@p1, (@p2 + 1), @p3)"},
		// handleSubstringParam uses SUBSTR for PostgreSQL (same as BigQuery); ClickHouse uses lowercase substring.
		{dialect.DialectPostgreSQL, "SUBSTR(@p1, (@p2 + 1), @p3)"},
		{dialect.DialectClickHouse, "substring(@p1, (@p2 + 1), @p3)"},
	}

	for _, tt := range tests {
		t.Run(tt.dialect.String(), func(t *testing.T) {
			config := NewOperatorConfig(tt.dialect, nil)
			op := NewStringOperator(config)
			pc := params.NewParamCollector(params.PlaceholderNamed)
			sql, err := op.ToSQLParam("substr", args, pc)
			if err != nil {
				t.Fatalf("ToSQLParam: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", sql, tt.wantSQL)
			}
			if !reflect.DeepEqual(pc.Params(), wantParams) {
				t.Errorf("Params = %#v, want %#v", pc.Params(), wantParams)
			}
		})
	}
}
