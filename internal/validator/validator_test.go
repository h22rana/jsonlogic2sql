package validator

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNewValidator(t *testing.T) {
	v := NewValidator()
	if v == nil || v.supportedOperators == nil {
		t.Fatal("NewValidator() returned nil or has nil supportedOperators map")
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
		{"json.Number integer", json.Number("123"), nil},
		{"json.Number scientific", json.Number("1e309"), nil},
		{"string", "hello", nil},
		{"boolean true", true, nil},
		{"boolean false", false, nil},
		{"null", nil, nil},
		{"invalid json.Number literal", json.Number("1 OR 1=1"), ValidationError{}},
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
			name:     "valid not empty array truthiness",
			input:    map[string]interface{}{"!": []interface{}{[]interface{}{}}},
			expected: nil,
		},
		{
			name:     "valid double bang empty array truthiness",
			input:    map[string]interface{}{"!!": []interface{}{[]interface{}{}}},
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

func TestSetCustomOperatorChecker(t *testing.T) {
	v := NewValidator()

	// Initially no custom operator checker
	if v.customOperatorChecker != nil {
		t.Fatal("expected customOperatorChecker to be nil initially")
	}

	// Set a custom operator checker
	checker := func(operatorName string) bool {
		return operatorName == "myOp" || operatorName == "anotherOp"
	}
	v.SetCustomOperatorChecker(checker)

	if v.customOperatorChecker == nil {
		t.Fatal("expected customOperatorChecker to be set")
	}

	// Verify the checker works as expected
	if !v.customOperatorChecker("myOp") {
		t.Error("expected checker to return true for 'myOp'")
	}
	if !v.customOperatorChecker("anotherOp") {
		t.Error("expected checker to return true for 'anotherOp'")
	}
	if v.customOperatorChecker("unknownOp") {
		t.Error("expected checker to return false for 'unknownOp'")
	}
}

func TestValidateCustomOperatorArgs(t *testing.T) {
	v := NewValidator()

	// Register a custom operator checker that recognizes "toLower" and "customOp"
	v.SetCustomOperatorChecker(func(operatorName string) bool {
		return operatorName == "toLower" || operatorName == "customOp" || operatorName == "singleArgOp"
	})

	tests := []struct {
		name      string
		input     interface{}
		expectErr bool
	}{
		{
			name:      "custom operator with array args",
			input:     map[string]interface{}{"customOp": []interface{}{1, "hello", true}},
			expectErr: false,
		},
		{
			name:      "custom operator with single string arg",
			input:     map[string]interface{}{"singleArgOp": "hello"},
			expectErr: false,
		},
		{
			name:      "custom operator with single number arg",
			input:     map[string]interface{}{"singleArgOp": 42},
			expectErr: false,
		},
		{
			name:      "custom operator with valid json.Number arg",
			input:     map[string]interface{}{"singleArgOp": json.Number("42")},
			expectErr: false,
		},
		{
			name:      "custom operator with invalid json.Number arg",
			input:     map[string]interface{}{"singleArgOp": json.Number("1 OR 1=1")},
			expectErr: true,
		},
		{
			name:      "custom operator with single bool arg",
			input:     map[string]interface{}{"singleArgOp": true},
			expectErr: false,
		},
		{
			name:      "custom operator with null arg",
			input:     map[string]interface{}{"singleArgOp": nil},
			expectErr: false,
		},
		{
			name: "custom operator with nested expression arg",
			input: map[string]interface{}{
				"toLower": map[string]interface{}{"var": "name"},
			},
			expectErr: false,
		},
		{
			name: "custom operator with array containing nested expressions",
			input: map[string]interface{}{
				"customOp": []interface{}{
					map[string]interface{}{"var": "x"},
					map[string]interface{}{">": []interface{}{1, 2}},
				},
			},
			expectErr: false,
		},
		{
			name: "custom operator with array containing invalid nested expression",
			input: map[string]interface{}{
				"customOp": []interface{}{
					map[string]interface{}{"badOp": []interface{}{1}},
				},
			},
			expectErr: true, // nested expression has unsupported operator
		},
		{
			name: "custom operator with invalid nested single expression",
			input: map[string]interface{}{
				"toLower": map[string]interface{}{"badOp": []interface{}{1}},
			},
			expectErr: true, // nested expression has unsupported operator
		},
		{
			name:      "unknown operator still fails without checker match",
			input:     map[string]interface{}{"reallyUnknown": []interface{}{1}},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			if tt.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateArgType(t *testing.T) {
	v := NewValidator()

	// To test validateArgType for non-AnyType arg types, we need to register
	// operators with specific ArgTypes. Since we're in the same package, we can
	// directly modify the supportedOperators map.

	// Add a test operator that requires NumberType arg
	v.supportedOperators["_testNumOp"] = OperatorSpec{
		Name:     "_testNumOp",
		MinArgs:  1,
		MaxArgs:  1,
		ArgTypes: []ArgType{NumberType},
	}

	// Add a test operator that requires StringType arg
	v.supportedOperators["_testStrOp"] = OperatorSpec{
		Name:     "_testStrOp",
		MinArgs:  1,
		MaxArgs:  1,
		ArgTypes: []ArgType{StringType},
	}

	// Add a test operator that requires BooleanType arg
	v.supportedOperators["_testBoolOp"] = OperatorSpec{
		Name:     "_testBoolOp",
		MinArgs:  1,
		MaxArgs:  1,
		ArgTypes: []ArgType{BooleanType},
	}

	// Add a test operator that requires ArrayType arg
	v.supportedOperators["_testArrayOp"] = OperatorSpec{
		Name:     "_testArrayOp",
		MinArgs:  1,
		MaxArgs:  1,
		ArgTypes: []ArgType{ArrayType},
	}

	// Add a test operator that requires ObjectType arg
	v.supportedOperators["_testObjOp"] = OperatorSpec{
		Name:     "_testObjOp",
		MinArgs:  1,
		MaxArgs:  1,
		ArgTypes: []ArgType{ObjectType},
	}

	// Add a test operator that requires VariableType arg
	v.supportedOperators["_testVarOp"] = OperatorSpec{
		Name:     "_testVarOp",
		MinArgs:  1,
		MaxArgs:  1,
		ArgTypes: []ArgType{VariableType},
	}

	tests := []struct {
		name      string
		input     interface{}
		expectErr bool
	}{
		// NumberType tests
		{
			name:      "number type with valid int",
			input:     map[string]interface{}{"_testNumOp": []interface{}{42}},
			expectErr: false,
		},
		{
			name:      "number type with valid float",
			input:     map[string]interface{}{"_testNumOp": []interface{}{3.14}},
			expectErr: false,
		},
		{
			name:      "number type with string value",
			input:     map[string]interface{}{"_testNumOp": []interface{}{"hello"}},
			expectErr: true,
		},
		{
			name:      "number type with bool value",
			input:     map[string]interface{}{"_testNumOp": []interface{}{true}},
			expectErr: true,
		},

		// StringType tests
		{
			name:      "string type with valid string",
			input:     map[string]interface{}{"_testStrOp": []interface{}{"hello"}},
			expectErr: false,
		},
		{
			name:      "string type with number value",
			input:     map[string]interface{}{"_testStrOp": []interface{}{42}},
			expectErr: true,
		},
		{
			name:      "string type with nil value",
			input:     map[string]interface{}{"_testStrOp": []interface{}{nil}},
			expectErr: true,
		},

		// BooleanType tests
		{
			name:      "boolean type with valid bool true",
			input:     map[string]interface{}{"_testBoolOp": []interface{}{true}},
			expectErr: false,
		},
		{
			name:      "boolean type with valid bool false",
			input:     map[string]interface{}{"_testBoolOp": []interface{}{false}},
			expectErr: false,
		},
		{
			name:      "boolean type with string value",
			input:     map[string]interface{}{"_testBoolOp": []interface{}{"true"}},
			expectErr: true,
		},
		{
			name:      "boolean type with number value",
			input:     map[string]interface{}{"_testBoolOp": []interface{}{1}},
			expectErr: true,
		},

		// ArrayType tests
		{
			name:      "array type with valid array",
			input:     map[string]interface{}{"_testArrayOp": []interface{}{[]interface{}{1, 2, 3}}},
			expectErr: false,
		},
		{
			name:      "array type with string value",
			input:     map[string]interface{}{"_testArrayOp": []interface{}{"not-an-array"}},
			expectErr: true,
		},
		{
			name:      "array type with number value",
			input:     map[string]interface{}{"_testArrayOp": []interface{}{42}},
			expectErr: true,
		},

		// ObjectType tests
		{
			name:      "object type with valid object",
			input:     map[string]interface{}{"_testObjOp": []interface{}{map[string]interface{}{"key": "value"}}},
			expectErr: false,
		},
		{
			name:      "object type with string value",
			input:     map[string]interface{}{"_testObjOp": []interface{}{"not-an-object"}},
			expectErr: true,
		},
		{
			name:      "object type with number value",
			input:     map[string]interface{}{"_testObjOp": []interface{}{42}},
			expectErr: true,
		},
		{
			name:      "object type with array value",
			input:     map[string]interface{}{"_testObjOp": []interface{}{[]interface{}{1, 2}}},
			expectErr: true,
		},

		// VariableType tests (always passes)
		{
			name:      "variable type with string",
			input:     map[string]interface{}{"_testVarOp": []interface{}{"anything"}},
			expectErr: false,
		},
		{
			name:      "variable type with number",
			input:     map[string]interface{}{"_testVarOp": []interface{}{42}},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			if tt.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateStandardOperatorEdgeCases(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name      string
		input     interface{}
		expectErr bool
		errMsg    string
	}{
		// Unary operator ! with non-array argument
		{
			name:      "not with non-array arg (number)",
			input:     map[string]interface{}{"!": 42},
			expectErr: false,
		},
		{
			name:      "not with non-array arg (string)",
			input:     map[string]interface{}{"!": "hello"},
			expectErr: false,
		},
		{
			name:      "not with non-array arg (bool)",
			input:     map[string]interface{}{"!": true},
			expectErr: false,
		},
		{
			name:      "not with non-array arg (null)",
			input:     map[string]interface{}{"!": nil},
			expectErr: false,
		},
		{
			name:      "not with nested expression as non-array arg",
			input:     map[string]interface{}{"!": map[string]interface{}{"var": "active"}},
			expectErr: false,
		},
		// Unary operator !! with non-array argument
		{
			name:      "double-not with non-array arg (number)",
			input:     map[string]interface{}{"!!": 42},
			expectErr: false,
		},
		{
			name:      "double-not with non-array arg (string)",
			input:     map[string]interface{}{"!!": "hello"},
			expectErr: false,
		},
		{
			name:      "double-not with nested expression",
			input:     map[string]interface{}{"!!": map[string]interface{}{"var": "field"}},
			expectErr: false,
		},
		// Unary operators with wrong arg count
		{
			name:      "not with too many args",
			input:     map[string]interface{}{"!": []interface{}{true, false}},
			expectErr: true,
			errMsg:    "! operator requires exactly 1 argument",
		},
		{
			name:      "double-not with too many args",
			input:     map[string]interface{}{"!!": []interface{}{true, false}},
			expectErr: true,
			errMsg:    "!! operator requires exactly 1 argument",
		},
		// Unary operator with valid single array arg containing nested expression
		{
			name:      "not with array containing nested expression",
			input:     map[string]interface{}{"!": []interface{}{map[string]interface{}{"var": "active"}}},
			expectErr: false,
		},
		// Standard operator with non-array argument
		{
			name:      "equality with non-array arg",
			input:     map[string]interface{}{"==": "not-an-array"},
			expectErr: true,
			errMsg:    "== operator requires array argument",
		},
		{
			name:      "and with non-array arg",
			input:     map[string]interface{}{"and": "not-an-array"},
			expectErr: true,
			errMsg:    "and operator requires array argument",
		},
		// MaxArgs validation
		{
			name:      "modulo with too many args",
			input:     map[string]interface{}{"%": []interface{}{10, 3, 2}},
			expectErr: true,
			errMsg:    "% operator requires at most 2 arguments, got 3",
		},
		{
			name:      "substr with too many args",
			input:     map[string]interface{}{"substr": []interface{}{"hello", 1, 3, "extra"}},
			expectErr: true,
			errMsg:    "substr operator requires at most 3 arguments, got 4",
		},
		// MinArgs validation
		{
			name:      "equality with too few args",
			input:     map[string]interface{}{"==": []interface{}{1}},
			expectErr: true,
			errMsg:    "== operator requires at least 2 arguments, got 1",
		},
		{
			name:      "if with too few args",
			input:     map[string]interface{}{"if": []interface{}{true}},
			expectErr: true,
			errMsg:    "if operator requires at least 2 arguments, got 1",
		},
		{
			name:      "reduce with too few args",
			input:     map[string]interface{}{"reduce": []interface{}{[]interface{}{1, 2}}},
			expectErr: true,
			errMsg:    "reduce operator requires at least 3 arguments, got 1",
		},
		// Valid operators with proper args
		{
			name:      "valid modulo",
			input:     map[string]interface{}{"%": []interface{}{10, 3}},
			expectErr: false,
		},
		{
			name:      "valid if with 3 args",
			input:     map[string]interface{}{"if": []interface{}{true, "yes", "no"}},
			expectErr: false,
		},
		{
			name:      "valid cat with multiple args",
			input:     map[string]interface{}{"cat": []interface{}{"hello", " ", "world"}},
			expectErr: false,
		},
		{
			name:      "valid substr with 2 args",
			input:     map[string]interface{}{"substr": []interface{}{"hello", 1}},
			expectErr: false,
		},
		{
			name:      "valid substr with 3 args",
			input:     map[string]interface{}{"substr": []interface{}{"hello", 1, 3}},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errMsg != "" {
					var ve ValidationError
					ok := errors.As(err, &ve)
					if !ok {
						t.Errorf("expected ValidationError, got %T", err)
					} else if ve.Message != tt.errMsg {
						t.Errorf("expected message %q, got %q", tt.errMsg, ve.Message)
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateMissingOperatorEdgeCases(t *testing.T) {
	v := NewValidator()

	tests := []struct {
		name      string
		input     interface{}
		expectErr bool
		errMsg    string
	}{
		{
			name:      "missing with empty string",
			input:     map[string]interface{}{"missing": ""},
			expectErr: true,
			errMsg:    "missing operator argument must be a non-empty string",
		},
		{
			name:      "missing with empty array",
			input:     map[string]interface{}{"missing": []interface{}{}},
			expectErr: true,
			errMsg:    "missing operator array cannot be empty",
		},
		{
			name:      "missing with non-string non-array arg (number)",
			input:     map[string]interface{}{"missing": 123},
			expectErr: true,
			errMsg:    "missing operator argument must be a string or array of strings",
		},
		{
			name:      "missing with non-string non-array arg (bool)",
			input:     map[string]interface{}{"missing": true},
			expectErr: true,
			errMsg:    "missing operator argument must be a string or array of strings",
		},
		{
			name:      "missing with non-string non-array arg (nil)",
			input:     map[string]interface{}{"missing": nil},
			expectErr: true,
			errMsg:    "missing operator argument must be a string or array of strings",
		},
		{
			name:      "missing with array containing empty string",
			input:     map[string]interface{}{"missing": []interface{}{""}},
			expectErr: true,
			errMsg:    "missing operator array element 0 must be a non-empty string",
		},
		{
			name:      "missing with array containing mixed valid and invalid",
			input:     map[string]interface{}{"missing": []interface{}{"field1", 123}},
			expectErr: true,
			errMsg:    "missing operator array element 1 must be a non-empty string",
		},
		{
			name:      "missing_some with non-array arg (string)",
			input:     map[string]interface{}{"missing_some": "bad"},
			expectErr: true,
			errMsg:    "missing_some operator requires array argument",
		},
		{
			name:      "missing_some with non-array arg (number)",
			input:     map[string]interface{}{"missing_some": 42},
			expectErr: true,
			errMsg:    "missing_some operator requires array argument",
		},
		{
			name:      "missing_some with 3 args",
			input:     map[string]interface{}{"missing_some": []interface{}{1, []interface{}{"a"}, "extra"}},
			expectErr: true,
			errMsg:    "missing_some operator requires exactly 2 arguments",
		},
		{
			name:      "missing_some with 0 args",
			input:     map[string]interface{}{"missing_some": []interface{}{}},
			expectErr: true,
			errMsg:    "missing_some operator requires exactly 2 arguments",
		},
		{
			name:      "valid missing with single string",
			input:     map[string]interface{}{"missing": "valid_field"},
			expectErr: false,
		},
		{
			name:      "valid missing with string array",
			input:     map[string]interface{}{"missing": []interface{}{"a", "b", "c"}},
			expectErr: false,
		},
		{
			name:      "valid missing_some with float count",
			input:     map[string]interface{}{"missing_some": []interface{}{float64(2), []interface{}{"a", "b"}}},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			if tt.expectErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errMsg != "" {
					var ve ValidationError
					ok := errors.As(err, &ve)
					if !ok {
						t.Errorf("expected ValidationError, got %T", err)
					} else if ve.Message != tt.errMsg {
						t.Errorf("expected message %q, got %q", tt.errMsg, ve.Message)
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateRecursiveInvalidType(t *testing.T) {
	v := NewValidator()

	// Pass a type that is not primitive, array, or map (e.g., a channel or a struct)
	// Using a struct value to trigger the "invalid type" branch
	type customStruct struct{ X int }
	err := v.Validate(customStruct{X: 1})
	if err == nil {
		t.Fatal("expected error for invalid type, got nil")
	}
	var ve ValidationError
	ok := errors.As(err, &ve)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if ve.Message != "invalid type: validator.customStruct" {
		t.Errorf("expected message about invalid type, got %q", ve.Message)
	}
}

func TestIsArrayAndIsObject(t *testing.T) {
	v := NewValidator()

	// Test isArray via validateArgType path with a custom operator spec
	v.supportedOperators["_arrOp"] = OperatorSpec{
		Name:     "_arrOp",
		MinArgs:  1,
		MaxArgs:  1,
		ArgTypes: []ArgType{ArrayType},
	}
	v.supportedOperators["_objOp"] = OperatorSpec{
		Name:     "_objOp",
		MinArgs:  1,
		MaxArgs:  1,
		ArgTypes: []ArgType{ObjectType},
	}

	tests := []struct {
		name      string
		input     interface{}
		expectErr bool
	}{
		// isArray tests
		{
			name:      "array arg with actual array",
			input:     map[string]interface{}{"_arrOp": []interface{}{[]interface{}{1, 2}}},
			expectErr: false,
		},
		{
			name:      "array arg with string",
			input:     map[string]interface{}{"_arrOp": []interface{}{"not-array"}},
			expectErr: true,
		},
		{
			name:      "array arg with nil",
			input:     map[string]interface{}{"_arrOp": []interface{}{nil}},
			expectErr: true,
		},

		// isObject tests
		{
			name:      "object arg with actual map",
			input:     map[string]interface{}{"_objOp": []interface{}{map[string]interface{}{"a": 1}}},
			expectErr: false,
		},
		{
			name:      "object arg with string",
			input:     map[string]interface{}{"_objOp": []interface{}{"not-object"}},
			expectErr: true,
		},
		{
			name:      "object arg with array",
			input:     map[string]interface{}{"_objOp": []interface{}{[]interface{}{1}}},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.input)
			if tt.expectErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateArrayWithNestedError(t *testing.T) {
	v := NewValidator()

	// Array containing an invalid nested expression
	input := []interface{}{
		1,
		map[string]interface{}{"unsupported_op": []interface{}{1}},
	}
	err := v.Validate(input)
	if err == nil {
		t.Fatal("expected error for array with invalid nested expression")
	}
}

func TestCustomOperatorCheckerReturnsFalse(t *testing.T) {
	v := NewValidator()

	// Set a checker that always returns false
	v.SetCustomOperatorChecker(func(operatorName string) bool {
		return false
	})

	// Unknown operator should still fail even with a checker that returns false
	err := v.Validate(map[string]interface{}{"unknownOp": []interface{}{1}})
	if err == nil {
		t.Fatal("expected error for unknown operator when checker returns false")
	}
	var ve ValidationError
	ok := errors.As(err, &ve)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if ve.Operator != "unknownOp" {
		t.Errorf("expected operator 'unknownOp', got %q", ve.Operator)
	}
}

func TestValidateVarOperatorEmptyArray(t *testing.T) {
	v := NewValidator()

	// var with empty array
	err := v.Validate(map[string]interface{}{"var": []interface{}{}})
	if err == nil {
		t.Fatal("expected error for var with empty array")
	}
	var ve ValidationError
	ok := errors.As(err, &ve)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if ve.Message != "var operator requires 1 or 2 arguments" {
		t.Errorf("unexpected message: %q", ve.Message)
	}
}
