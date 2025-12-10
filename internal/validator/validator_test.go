package validator

import (
	"testing"
)

func TestNewValidator(t *testing.T) {
	v := NewValidator()
	if v == nil {
		t.Fatal("NewValidator() returned nil")
	}
	if v.supportedOperators == nil {
		t.Fatal("supportedOperators map is nil")
	}
}

func TestValidatePrimitives(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name     string
		input    interface{}
		expected error
	}{
		{"number", 42, nil},
		{"float", 3.14, nil},
		{"string", "hello", nil},
		{"boolean true", true, nil},
		{"boolean false", false, nil},
		{"null", nil, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			if (err != nil) != (tt.expected != nil) {
				t.Errorf("Validate() error = %v, expected %v", err, tt.expected)
			}
		})
	}
}

func TestValidateArrays(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name     string
		input    interface{}
		expected error
	}{
		{
			name:     "valid array",
			input:    []interface{}{1, 2, 3},
			expected: nil,
		},
		{
			name:     "empty array",
			input:    []interface{}{},
			expected: ValidationError{Message: "array cannot be empty"},
		},
		{
			name:     "nested array",
			input:    []interface{}{[]interface{}{1, 2}, []interface{}{3, 4}},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			if tt.expected == nil {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error %v, got nil", tt.expected)
				}
			}
		})
	}
}

func TestValidateOperators(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name     string
		input    interface{}
		expected error
	}{
		{
			name:     "valid comparison",
			input:    map[string]interface{}{">": []interface{}{1, 2}},
			expected: nil,
		},
		{
			name:     "valid var operator",
			input:    map[string]interface{}{"var": "amount"},
			expected: nil,
		},
		{
			name:     "valid var with default",
			input:    map[string]interface{}{"var": []interface{}{"amount", 0}},
			expected: nil,
		},
		{
			name:     "unsupported operator",
			input:    map[string]interface{}{"unsupported": []interface{}{1, 2}},
			expected: ValidationError{Operator: "unsupported", Message: "unsupported operator: unsupported"},
		},
		{
			name:     "multiple keys in operator",
			input:    map[string]interface{}{">": []interface{}{1, 2}, "<": []interface{}{3, 4}},
			expected: ValidationError{Message: "operator object must have exactly one key"},
		},
		{
			name:     "empty operator object",
			input:    map[string]interface{}{},
			expected: ValidationError{Message: "operator object must have exactly one key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			if tt.expected == nil {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error, got nil")
				}
				// Check if error message contains expected content
				if err.Error() != tt.expected.Error() {
					t.Errorf("Validate() error = %v, expected %v", err, tt.expected)
				}
			}
		})
	}
}

func TestValidateVarOperator(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name     string
		input    interface{}
		expected error
	}{
		{
			name:     "var with string",
			input:    map[string]interface{}{"var": "amount"},
			expected: nil,
		},
		{
			name:     "var with array (1 arg)",
			input:    map[string]interface{}{"var": []interface{}{"amount"}},
			expected: nil,
		},
		{
			name:     "var with array (2 args)",
			input:    map[string]interface{}{"var": []interface{}{"amount", 0}},
			expected: nil,
		},
		{
			name:     "var with too many args",
			input:    map[string]interface{}{"var": []interface{}{"amount", 0, "extra"}},
			expected: ValidationError{Operator: "var", Message: "var operator requires 1 or 2 arguments"},
		},
		{
			name:     "var with non-string first arg",
			input:    map[string]interface{}{"var": []interface{}{123, 0}},
			expected: ValidationError{Operator: "var", Message: "var operator first argument must be a string"},
		},
		{
			name:     "var with invalid args type",
			input:    map[string]interface{}{"var": 123},
			expected: ValidationError{Operator: "var", Message: "var operator requires string or array arguments"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			if tt.expected == nil {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error, got nil")
				}
			}
		})
	}
}

func TestValidateMissingOperator(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name     string
		input    interface{}
		expected error
	}{
		{
			name:     "valid missing with single field",
			input:    map[string]interface{}{"missing": "field"},
			expected: nil,
		},
		{
			name:     "valid missing with array of strings",
			input:    map[string]interface{}{"missing": []interface{}{"field"}},
			expected: nil,
		},
		{
			name:     "valid missing with multiple fields",
			input:    map[string]interface{}{"missing": []interface{}{"field", "extra"}},
			expected: nil,
		},
		{
			name:     "missing with non-string arg in array",
			input:    map[string]interface{}{"missing": []interface{}{123}},
			expected: ValidationError{Operator: "missing", Message: "missing operator array elements must be strings"},
		},
		{
			name:     "valid missing_some",
			input:    map[string]interface{}{"missing_some": []interface{}{1, []interface{}{"field1", "field2"}}},
			expected: nil,
		},
		{
			name:     "missing_some with wrong arg count",
			input:    map[string]interface{}{"missing_some": []interface{}{1}},
			expected: ValidationError{Operator: "missing_some", Message: "missing_some operator requires exactly 2 arguments"},
		},
		{
			name:     "missing_some with non-number first arg",
			input:    map[string]interface{}{"missing_some": []interface{}{"1", []interface{}{"field"}}},
			expected: ValidationError{Operator: "missing_some", Message: "missing_some operator first argument must be a number"},
		},
		{
			name:     "missing_some with non-array second arg",
			input:    map[string]interface{}{"missing_some": []interface{}{1, "field"}},
			expected: ValidationError{Operator: "missing_some", Message: "missing_some operator second argument must be an array"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			if tt.expected == nil {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error, got nil")
				}
			}
		})
	}
}

func TestValidateComparisonOperators(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name     string
		input    interface{}
		expected error
	}{
		{
			name:     "valid equality",
			input:    map[string]interface{}{"==": []interface{}{1, 2}},
			expected: nil,
		},
		{
			name:     "valid greater than",
			input:    map[string]interface{}{">": []interface{}{5, 3}},
			expected: nil,
		},
		{
			name:     "too few args",
			input:    map[string]interface{}{">": []interface{}{5}},
			expected: ValidationError{Operator: ">", Message: "> operator requires at least 2 arguments, got 1"},
		},
		{
			name:     "too many args",
			input:    map[string]interface{}{">": []interface{}{5, 3, 1}},
			expected: nil, // Now supports variable arguments for chained comparisons
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			if tt.expected == nil {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error, got nil")
				}
			}
		})
	}
}

func TestValidateLogicalOperators(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name     string
		input    interface{}
		expected error
	}{
		{
			name:     "valid and",
			input:    map[string]interface{}{"and": []interface{}{true, false}},
			expected: nil,
		},
		{
			name:     "valid or",
			input:    map[string]interface{}{"or": []interface{}{true, false}},
			expected: nil,
		},
		{
			name:     "valid not",
			input:    map[string]interface{}{"!": []interface{}{true}},
			expected: nil,
		},
		{
			name:     "and with no args",
			input:    map[string]interface{}{"and": []interface{}{}},
			expected: ValidationError{Operator: "and", Message: "and operator requires at least 1 arguments, got 0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			if tt.expected == nil {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error, got nil")
				}
			}
		})
	}
}

func TestValidateComplexExpressions(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name     string
		input    interface{}
		expected error
	}{
		{
			name: "nested and/or",
			input: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{">": []interface{}{1, 2}},
					map[string]interface{}{"or": []interface{}{
						map[string]interface{}{"==": []interface{}{3, 4}},
						map[string]interface{}{"<": []interface{}{5, 6}},
					}},
				},
			},
			expected: nil,
		},
		{
			name: "var in comparison",
			input: map[string]interface{}{
				">": []interface{}{
					map[string]interface{}{"var": "amount"},
					1000,
				},
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			if tt.expected == nil {
				if err != nil {
					t.Errorf("Validate() unexpected error = %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Validate() expected error, got nil")
				}
			}
		})
	}
}

func TestGetSupportedOperators(t *testing.T) {
	v := NewValidator()
	operators := v.GetSupportedOperators()

	expectedCount := 33 // Standard JSON Logic operators (including ===, !==, !!, cat, substr)
	if len(operators) != expectedCount {
		t.Errorf("Expected %d operators, got %d", expectedCount, len(operators))
	}

	// Check for some key operators (standard JSON Logic)
	expectedOps := []string{"var", "==", "===", ">", "and", "or", "in", "if", "cat", "substr", "!!"}
	for _, op := range expectedOps {
		found := false
		for _, supported := range operators {
			if supported == op {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected operator %s not found in supported operators", op)
		}
	}
}

func TestIsOperatorSupported(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		operator string
		expected bool
	}{
		{"var", true},
		{"==", true},
		{">", true},
		{"and", true},
		{"unsupported", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.operator, func(t *testing.T) {
			result := v.IsOperatorSupported(tt.operator)
			if result != tt.expected {
				t.Errorf("IsOperatorSupported(%s) = %v, expected %v", tt.operator, result, tt.expected)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	err := ValidationError{
		Operator: "test",
		Message:  "test message",
		Path:     "root.test",
	}

	expected := "validation error at root.test: test message"
	if err.Error() != expected {
		t.Errorf("ValidationError.Error() = %s, expected %s", err.Error(), expected)
	}

	// Test without path
	err.Path = ""
	expected = "validation error: test message"
	if err.Error() != expected {
		t.Errorf("ValidationError.Error() = %s, expected %s", err.Error(), expected)
	}
}
