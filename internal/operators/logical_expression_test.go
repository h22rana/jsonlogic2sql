package operators

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

func TestLogicalOperator_extractVarFieldName(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		arg      interface{}
		expected string
	}{
		{
			name:     "simple var string",
			arg:      map[string]interface{}{"var": "fieldName"},
			expected: "fieldName",
		},
		{
			name:     "var with array - field name and default",
			arg:      map[string]interface{}{"var": []interface{}{"fieldName", "default"}},
			expected: "fieldName",
		},
		{
			name:     "nested field name",
			arg:      map[string]interface{}{"var": "user.profile.name"},
			expected: "user.profile.name",
		},
		{
			name:     "not a var expression",
			arg:      map[string]interface{}{">": []interface{}{1, 2}},
			expected: "",
		},
		{
			name:     "multiple keys - not valid",
			arg:      map[string]interface{}{"var": "field", "other": "value"},
			expected: "",
		},
		{
			name:     "primitive value",
			arg:      42,
			expected: "",
		},
		{
			name:     "empty array for var",
			arg:      map[string]interface{}{"var": []interface{}{}},
			expected: "",
		},
		{
			name:     "array with non-string first element",
			arg:      map[string]interface{}{"var": []interface{}{123, "default"}},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := op.extractVarFieldName(tt.arg)
			if result != tt.expected {
				t.Errorf("extractVarFieldName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestLogicalOperator_expressionToSQL_Extended(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		// ProcessedValue SQL
		{
			name:     "ProcessedValue SQL",
			input:    ProcessedValue{Value: "some_expr > 0", IsSQL: true},
			expected: "some_expr > 0",
			hasError: false,
		},
		// ProcessedValue literal
		{
			name:     "ProcessedValue literal string",
			input:    ProcessedValue{Value: "hello", IsSQL: false},
			expected: "'hello'",
			hasError: false,
		},
		// Primitive nil
		{
			name:     "primitive nil",
			input:    nil,
			expected: "NULL",
			hasError: false,
		},
		// Primitive float
		{
			name:     "primitive float",
			input:    3.14,
			expected: "3.14",
			hasError: false,
		},
		// missing operator
		{
			name:     "missing operator",
			input:    map[string]interface{}{"missing": "field"},
			expected: "field IS NULL",
			hasError: false,
		},
		// missing_some operator
		{
			name:     "missing_some operator",
			input:    map[string]interface{}{"missing_some": []interface{}{1, []interface{}{"a", "b"}}},
			expected: "(a IS NULL OR b IS NULL)",
			hasError: false,
		},
		// missing_some non-array error
		{
			name:     "missing_some non-array error",
			input:    map[string]interface{}{"missing_some": "invalid"},
			expected: "",
			hasError: true,
		},
		// Numeric operator
		{
			name:     "numeric addition",
			input:    map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": "a"}, 5}},
			expected: "(a + 5)",
			hasError: false,
		},
		// Numeric operator non-array error
		{
			name:     "numeric operator non-array error",
			input:    map[string]interface{}{"+": "invalid"},
			expected: "",
			hasError: true,
		},
		// String cat operator
		{
			name:     "cat operator",
			input:    map[string]interface{}{"cat": []interface{}{"hello", " ", "world"}},
			expected: "CONCAT('hello', ' ', 'world')",
			hasError: false,
		},
		// String operator non-array error
		{
			name:     "cat operator non-array error",
			input:    map[string]interface{}{"cat": "invalid"},
			expected: "",
			hasError: true,
		},
		// Comparison operator non-array error
		{
			name:     "comparison non-array error",
			input:    map[string]interface{}{"==": "invalid"},
			expected: "",
			hasError: true,
		},
		// Logical operator non-array error for and/or/if
		{
			name:     "and operator non-array error",
			input:    map[string]interface{}{"and": "invalid"},
			expected: "",
			hasError: true,
		},
		// Unary ! with non-array arg (wraps in array)
		{
			name:     "not with non-array arg",
			input:    map[string]interface{}{"!": map[string]interface{}{"var": "verified"}},
			expected: "NOT (verified)",
			hasError: false,
		},
		// !! with non-array arg (wraps in array)
		{
			name:     "double not with non-array arg",
			input:    map[string]interface{}{"!!": map[string]interface{}{"var": "value"}},
			expected: "(value IS NOT NULL AND value != FALSE AND value != 0 AND value != '')",
			hasError: false,
		},
		// Invalid expression type
		{
			name:     "invalid expression type - struct",
			input:    struct{ Name string }{Name: "test"},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.expressionToSQL(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("expressionToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expressionToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("expressionToSQL() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestLogicalOperator_expressionToSQL_ArrayOperators(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewLogicalOperator(config)

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name: "map operator in logical context",
			input: map[string]interface{}{
				"map": []interface{}{
					map[string]interface{}{"var": "nums"},
					map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": ""}, 1}},
				},
			},
			expected: "ARRAY(SELECT (elem + 1) FROM UNNEST(nums) AS elem)",
			hasError: false,
		},
		{
			name:     "array operator non-array error",
			input:    map[string]interface{}{"map": "invalid"},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.expressionToSQL(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("expressionToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expressionToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("expressionToSQL() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestLogicalOperator_expressionToSQL_ExpressionParserCallback(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	config.SetExpressionParser(func(expr any, path string) (string, error) {
		return "CUSTOM_LOGICAL()", nil
	})
	op := NewLogicalOperator(config)

	// Unknown operator with expression parser should delegate
	result, err := op.expressionToSQL(map[string]interface{}{"toLower": []interface{}{map[string]interface{}{"var": "name"}}})
	if err != nil {
		t.Errorf("expressionToSQL() unexpected error = %v", err)
	}
	if result != "CUSTOM_LOGICAL()" {
		t.Errorf("expressionToSQL() = %v, want CUSTOM_LOGICAL()", result)
	}
}

func TestLogicalOperator_processArgs(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	tests := []struct {
		name        string
		args        []interface{}
		expectedLen int
		hasError    bool
	}{
		{
			name: "var expression kept as-is",
			args: []interface{}{
				map[string]interface{}{"var": "field"},
				42,
			},
			expectedLen: 2,
			hasError:    false,
		},
		{
			name: "complex expression converted to SQL",
			args: []interface{}{
				map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": "a"}, 5}},
				"literal",
			},
			expectedLen: 2,
			hasError:    false,
		},
		{
			name:        "primitive values kept as-is",
			args:        []interface{}{42, "hello", true},
			expectedLen: 3,
			hasError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.processArgs(tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("processArgs() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("processArgs() unexpected error = %v", err)
				}
				if len(result) != tt.expectedLen {
					t.Errorf("processArgs() returned %d args, want %d", len(result), tt.expectedLen)
				}
			}
		})
	}
}

func TestLogicalOperator_isPrimitive_Extended(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	// Test additional types for completeness
	tests := []struct {
		name     string
		input    interface{}
		expected bool
	}{
		{"int8", int8(1), true},
		{"int16", int16(1), true},
		{"int32", int32(1), true},
		{"uint", uint(1), true},
		{"uint8", uint8(1), true},
		{"uint16", uint16(1), true},
		{"uint32", uint32(1), true},
		{"uint64", uint64(1), true},
		{"float32", float32(1.0), true},
		{"struct", struct{}{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := op.isPrimitive(tt.input)
			if result != tt.expected {
				t.Errorf("isPrimitive(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLogicalOperator_handleIf_MultiCondition(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	// Test with 7 args (3 condition/value pairs + else)
	result, err := op.handleIf([]interface{}{
		map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 90}},
		"A",
		map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 80}},
		"B",
		map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 70}},
		"C",
		"F",
	})
	if err != nil {
		t.Errorf("handleIf() unexpected error = %v", err)
	}
	expected := "CASE WHEN score > 90 THEN 'A' WHEN score > 80 THEN 'B' WHEN score > 70 THEN 'C' ELSE 'F' END"
	if result != expected {
		t.Errorf("handleIf() = %v, want %v", result, expected)
	}
}
