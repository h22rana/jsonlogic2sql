package operators

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

func TestStringOperator_valueToSQL_Extended(t *testing.T) {
	op := NewStringOperator(testFieldOnlyConfig())

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
	config := NewOperatorConfig(dialect.DialectClickHouse, &fieldOnlySchemaProvider{})
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
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
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
	op := NewStringOperator(testFieldOnlyConfig())

	result, err := op.ToSQL("substr", []interface{}{
		map[string]interface{}{"var": "name"},
		map[string]interface{}{"var": "start_pos"},
	})
	if err != nil {
		t.Errorf("ToSQL() unexpected error = %v", err)
	}
	expected := "SUBSTR(name, (CASE WHEN start_pos < 0 THEN GREATEST((LENGTH(name) + start_pos + 1), 1) ELSE (start_pos + 1) END))"
	if result != expected {
		t.Errorf("ToSQL() = %v, want %v", result, expected)
	}
}

// stringSchemaProvider is a configurable schema provider for string operator tests.

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
	op := NewStringOperator(testFieldOnlyConfig())

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
	op := NewStringOperator(testFieldOnlyConfig())

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
