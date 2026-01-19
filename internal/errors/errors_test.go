package errors

import (
	"errors"
	"testing"
)

func TestTranspileError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *TranspileError
		expected string
	}{
		{
			name: "full error with all fields",
			err: &TranspileError{
				Code:     ErrUnsupportedOperator,
				Operator: "unknown",
				Path:     "$.and[0]",
				Message:  "unsupported operator: unknown",
			},
			expected: "[E100] at $.and[0] (operator: unknown): unsupported operator: unknown",
		},
		{
			name: "error without path",
			err: &TranspileError{
				Code:     ErrInvalidJSON,
				Operator: "",
				Path:     "",
				Message:  "invalid JSON",
			},
			expected: "[E007]: invalid JSON",
		},
		{
			name: "error without operator",
			err: &TranspileError{
				Code:     ErrMultipleKeys,
				Operator: "",
				Path:     "$.obj",
				Message:  "operator object must have exactly one key",
			},
			expected: "[E003] at $.obj: operator object must have exactly one key",
		},
		{
			name: "error with cause",
			err: &TranspileError{
				Code:     ErrInvalidArgument,
				Operator: "+",
				Path:     "$.+[0]",
				Message:  "invalid argument at index 0",
				Cause:    errors.New("not a number"),
			},
			expected: "[E302] at $.+[0] (operator: +): invalid argument at index 0: not a number",
		},
		{
			name: "error with only code and message",
			err: &TranspileError{
				Code:    ErrValidation,
				Message: "validation failed",
			},
			expected: "[E006]: validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.err.Error()
			if result != tt.expected {
				t.Errorf("Error() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTranspileError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &TranspileError{
		Code:    ErrInvalidArgument,
		Message: "test error",
		Cause:   cause,
	}

	unwrapped := err.Unwrap()
	if !errors.Is(unwrapped, cause) {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, cause)
	}

	// Test with nil cause
	errNoCause := &TranspileError{
		Code:    ErrInvalidExpression,
		Message: "no cause",
	}
	if errNoCause.Unwrap() != nil {
		t.Errorf("Unwrap() on error without cause should return nil")
	}
}

func TestNew(t *testing.T) {
	err := New(ErrUnsupportedOperator, "test", "$.test", "test message")

	if err.Code != ErrUnsupportedOperator {
		t.Errorf("Code = %v, want %v", err.Code, ErrUnsupportedOperator)
	}
	if err.Operator != "test" {
		t.Errorf("Operator = %v, want %v", err.Operator, "test")
	}
	if err.Path != "$.test" {
		t.Errorf("Path = %v, want %v", err.Path, "$.test")
	}
	if err.Message != "test message" {
		t.Errorf("Message = %v, want %v", err.Message, "test message")
	}
	if err.Cause != nil {
		t.Errorf("Cause should be nil")
	}
}

func TestWrap(t *testing.T) {
	cause := errors.New("original error")
	err := Wrap(ErrInvalidArgument, "op", "$.path", "wrapped", cause)

	if err.Code != ErrInvalidArgument {
		t.Errorf("Code = %v, want %v", err.Code, ErrInvalidArgument)
	}
	if err.Operator != "op" {
		t.Errorf("Operator = %v, want %v", err.Operator, "op")
	}
	if err.Path != "$.path" {
		t.Errorf("Path = %v, want %v", err.Path, "$.path")
	}
	if err.Message != "wrapped" {
		t.Errorf("Message = %v, want %v", err.Message, "wrapped")
	}
	if !errors.Is(err.Cause, cause) {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestWithPath(t *testing.T) {
	original := New(ErrUnsupportedOperator, "test", "$.old", "message")
	updated := original.WithPath("$.new.path")

	// Original should be unchanged
	if original.Path != "$.old" {
		t.Errorf("Original path changed, got %v", original.Path)
	}

	// Updated should have new path
	if updated.Path != "$.new.path" {
		t.Errorf("Updated path = %v, want %v", updated.Path, "$.new.path")
	}

	// Other fields should be preserved
	if updated.Code != original.Code {
		t.Errorf("Code not preserved")
	}
	if updated.Operator != original.Operator {
		t.Errorf("Operator not preserved")
	}
	if updated.Message != original.Message {
		t.Errorf("Message not preserved")
	}
}

func TestWithOperator(t *testing.T) {
	original := New(ErrInsufficientArgs, "old_op", "$.path", "message")
	updated := original.WithOperator("new_op")

	// Original should be unchanged
	if original.Operator != "old_op" {
		t.Errorf("Original operator changed, got %v", original.Operator)
	}

	// Updated should have new operator
	if updated.Operator != "new_op" {
		t.Errorf("Updated operator = %v, want %v", updated.Operator, "new_op")
	}

	// Other fields should be preserved
	if updated.Code != original.Code {
		t.Errorf("Code not preserved")
	}
	if updated.Path != original.Path {
		t.Errorf("Path not preserved")
	}
	if updated.Message != original.Message {
		t.Errorf("Message not preserved")
	}
}

func TestNewUnsupportedOperator(t *testing.T) {
	err := NewUnsupportedOperator("foobar", "$.test")

	if err.Code != ErrUnsupportedOperator {
		t.Errorf("Code = %v, want %v", err.Code, ErrUnsupportedOperator)
	}
	if err.Operator != "foobar" {
		t.Errorf("Operator = %v, want %v", err.Operator, "foobar")
	}
	if err.Path != "$.test" {
		t.Errorf("Path = %v, want %v", err.Path, "$.test")
	}
}

func TestNewOperatorRequiresArray(t *testing.T) {
	err := NewOperatorRequiresArray("and", "$.root")

	if err.Code != ErrOperatorRequiresArray {
		t.Errorf("Code = %v, want %v", err.Code, ErrOperatorRequiresArray)
	}
	if err.Operator != "and" {
		t.Errorf("Operator = %v, want %v", err.Operator, "and")
	}
}

func TestNewInsufficientArgs(t *testing.T) {
	err := NewInsufficientArgs("+", "$.+", 2, 1)

	if err.Code != ErrInsufficientArgs {
		t.Errorf("Code = %v, want %v", err.Code, ErrInsufficientArgs)
	}
	if err.Operator != "+" {
		t.Errorf("Operator = %v, want %v", err.Operator, "+")
	}
	expectedMsg := "+ operator requires at least 2 argument(s), got 1"
	if err.Message != expectedMsg {
		t.Errorf("Message = %v, want %v", err.Message, expectedMsg)
	}
}

func TestNewTooManyArgs(t *testing.T) {
	err := NewTooManyArgs("%", "$.%", 2, 3)

	if err.Code != ErrTooManyArgs {
		t.Errorf("Code = %v, want %v", err.Code, ErrTooManyArgs)
	}
	expectedMsg := "% operator accepts at most 2 argument(s), got 3"
	if err.Message != expectedMsg {
		t.Errorf("Message = %v, want %v", err.Message, expectedMsg)
	}
}

func TestNewInvalidArgument(t *testing.T) {
	cause := errors.New("not a number")
	err := NewInvalidArgument("+", "$.+", 1, cause)

	if err.Code != ErrInvalidArgument {
		t.Errorf("Code = %v, want %v", err.Code, ErrInvalidArgument)
	}
	if !errors.Is(err.Cause, cause) {
		t.Errorf("Cause not preserved")
	}
	expectedMsg := "invalid argument at index 1"
	if err.Message != expectedMsg {
		t.Errorf("Message = %v, want %v", err.Message, expectedMsg)
	}
}

func TestNewTypeMismatch(t *testing.T) {
	err := NewTypeMismatch(">", "$.>", "number", "string")

	if err.Code != ErrTypeMismatch {
		t.Errorf("Code = %v, want %v", err.Code, ErrTypeMismatch)
	}
	expectedMsg := "expected number, got string"
	if err.Message != expectedMsg {
		t.Errorf("Message = %v, want %v", err.Message, expectedMsg)
	}
}

func TestNewFieldNotInSchema(t *testing.T) {
	err := NewFieldNotInSchema("unknown_field", "$.var")

	if err.Code != ErrFieldNotInSchema {
		t.Errorf("Code = %v, want %v", err.Code, ErrFieldNotInSchema)
	}
	if err.Operator != "var" {
		t.Errorf("Operator = %v, want %v", err.Operator, "var")
	}
}

func TestNewInvalidFieldType(t *testing.T) {
	err := NewInvalidFieldType("+", "$.+", "name", "string", "number")

	if err.Code != ErrInvalidFieldType {
		t.Errorf("Code = %v, want %v", err.Code, ErrInvalidFieldType)
	}
}

func TestNewInvalidEnumValue(t *testing.T) {
	allowed := []string{"active", "inactive", "pending"}
	err := NewInvalidEnumValue("status", "$.==", "unknown", allowed)

	if err.Code != ErrInvalidEnumValue {
		t.Errorf("Code = %v, want %v", err.Code, ErrInvalidEnumValue)
	}
	if err.Operator != "==" {
		t.Errorf("Operator = %v, want %v", err.Operator, "==")
	}
}

func TestNewMultipleKeys(t *testing.T) {
	err := NewMultipleKeys("$.obj")

	if err.Code != ErrMultipleKeys {
		t.Errorf("Code = %v, want %v", err.Code, ErrMultipleKeys)
	}
	if err.Operator != "" {
		t.Errorf("Operator should be empty for structural errors")
	}
}

func TestNewPrimitiveNotAllowed(t *testing.T) {
	err := NewPrimitiveNotAllowed("$")

	if err.Code != ErrPrimitiveNotAllowed {
		t.Errorf("Code = %v, want %v", err.Code, ErrPrimitiveNotAllowed)
	}
}

func TestNewArrayNotAllowed(t *testing.T) {
	err := NewArrayNotAllowed("$")

	if err.Code != ErrArrayNotAllowed {
		t.Errorf("Code = %v, want %v", err.Code, ErrArrayNotAllowed)
	}
}

func TestBuildPath(t *testing.T) {
	tests := []struct {
		name     string
		parent   string
		operator string
		index    int
		expected string
	}{
		{
			name:     "with parent and index",
			parent:   "$.and",
			operator: ">",
			index:    0,
			expected: "$.and.>[0]",
		},
		{
			name:     "with empty parent and index",
			parent:   "",
			operator: "and",
			index:    1,
			expected: "$.and[1]",
		},
		{
			name:     "without index (negative)",
			parent:   "$.root",
			operator: "var",
			index:    -1,
			expected: "$.root.var",
		},
		{
			name:     "empty parent without index",
			parent:   "",
			operator: "or",
			index:    -1,
			expected: "$.or",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildPath(tt.parent, tt.operator, tt.index)
			if result != tt.expected {
				t.Errorf("BuildPath(%q, %q, %d) = %q, want %q",
					tt.parent, tt.operator, tt.index, result, tt.expected)
			}
		})
	}
}

func TestBuildArrayPath(t *testing.T) {
	tests := []struct {
		name     string
		parent   string
		index    int
		expected string
	}{
		{
			name:     "with parent",
			parent:   "$.and",
			index:    2,
			expected: "$.and[2]",
		},
		{
			name:     "empty parent",
			parent:   "",
			index:    0,
			expected: "$[0]",
		},
		{
			name:     "nested path",
			parent:   "$.and[0].or",
			index:    1,
			expected: "$.and[0].or[1]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildArrayPath(tt.parent, tt.index)
			if result != tt.expected {
				t.Errorf("BuildArrayPath(%q, %d) = %q, want %q",
					tt.parent, tt.index, result, tt.expected)
			}
		})
	}
}

func TestNewValidationError(t *testing.T) {
	cause := errors.New("validation issue")
	err := NewValidationError(cause)

	if err.Code != ErrValidation {
		t.Errorf("Code = %v, want %v", err.Code, ErrValidation)
	}
	if !errors.Is(err.Cause, cause) {
		t.Errorf("Cause not preserved")
	}
	if err.Message != "validation failed" {
		t.Errorf("Message = %v, want %v", err.Message, "validation failed")
	}
}

func TestNewInvalidJSON(t *testing.T) {
	cause := errors.New("unexpected end of JSON input")
	err := NewInvalidJSON(cause)

	if err.Code != ErrInvalidJSON {
		t.Errorf("Code = %v, want %v", err.Code, ErrInvalidJSON)
	}
	if !errors.Is(err.Cause, cause) {
		t.Errorf("Cause not preserved")
	}
	if err.Message != "invalid JSON" {
		t.Errorf("Message = %v, want %v", err.Message, "invalid JSON")
	}
}

func TestErrorCodes(t *testing.T) {
	// Verify error codes are unique and in expected ranges
	codes := map[ErrorCode]bool{
		// Structural errors (E001-E099)
		ErrInvalidExpression:   true,
		ErrEmptyArray:          true,
		ErrMultipleKeys:        true,
		ErrPrimitiveNotAllowed: true,
		ErrArrayNotAllowed:     true,
		ErrValidation:          true,
		ErrInvalidJSON:         true,
		// Operator errors (E100-E199)
		ErrUnsupportedOperator:   true,
		ErrOperatorRequiresArray: true,
		ErrCustomOperatorFailed:  true,
		// Type errors (E200-E299)
		ErrTypeMismatch:     true,
		ErrFieldNotInSchema: true,
		ErrInvalidFieldType: true,
		ErrInvalidEnumValue: true,
		// Argument errors (E300-E399)
		ErrInsufficientArgs:    true,
		ErrTooManyArgs:         true,
		ErrInvalidArgument:     true,
		ErrInvalidArgType:      true,
		ErrInvalidDefaultValue: true,
	}

	// Verify we have all expected codes
	expectedCount := 19
	if len(codes) != expectedCount {
		t.Errorf("Expected %d error codes, got %d", expectedCount, len(codes))
	}
}

func TestErrorsAs(t *testing.T) {
	// Test that TranspileError works with errors.As
	err := NewUnsupportedOperator("test", "$.test")

	var transpileErr *TranspileError
	if !errors.As(err, &transpileErr) {
		t.Error("errors.As should match TranspileError")
	}

	if transpileErr.Code != ErrUnsupportedOperator {
		t.Errorf("Code = %v, want %v", transpileErr.Code, ErrUnsupportedOperator)
	}
}

func TestErrorsIs(t *testing.T) {
	// Test that wrapped errors work with errors.Is
	cause := errors.New("specific error")
	err := Wrap(ErrInvalidArgument, "op", "$.path", "wrapped", cause)

	if !errors.Is(err, cause) {
		t.Error("errors.Is should find the cause")
	}
}
