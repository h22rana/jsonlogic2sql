package jsonlogic2sql

import (
	"errors"
	"testing"
)

func TestAsTranspileError(t *testing.T) {
	transpiler, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	tests := []struct {
		name         string
		jsonLogic    string
		expectError  bool
		expectedCode ErrorCode
		expectedPath string
	}{
		{
			name:         "unsupported operator in nested expression",
			jsonLogic:    `{"and": [{"unknown_op": [1]}]}`,
			expectError:  true,
			expectedCode: ErrUnsupportedOperator,
			expectedPath: "$.and.unknown_op[0]",
		},
		{
			name:         "invalid JSON",
			jsonLogic:    `{invalid json}`,
			expectError:  true,
			expectedCode: ErrInvalidJSON,
		},
		{
			name:         "validation error - unsupported operator",
			jsonLogic:    `{"unknown_op": [1, 2]}`,
			expectError:  true,
			expectedCode: ErrValidation,
		},
		{
			name:         "validation error - wrong argument type",
			jsonLogic:    `{"and": "not-array"}`,
			expectError:  true,
			expectedCode: ErrValidation,
		},
		{
			name:        "valid expression",
			jsonLogic:   `{"==": [{"var": "status"}, "active"]}`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := transpiler.Transpile(tt.jsonLogic)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got nil")
					return
				}

				tpErr, ok := AsTranspileError(err)
				if !ok {
					t.Errorf("Expected TranspileError but got: %T", err)
					return
				}

				if tpErr.Code != tt.expectedCode {
					t.Errorf("Expected error code %s, got %s", tt.expectedCode, tpErr.Code)
				}

				if tt.expectedPath != "" && tpErr.Path != tt.expectedPath {
					t.Errorf("Expected path %s, got %s", tt.expectedPath, tpErr.Path)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestIsErrorCode(t *testing.T) {
	transpiler, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	// Test ErrUnsupportedOperator
	_, err = transpiler.Transpile(`{"and": [{"unknown": [1]}]}`)
	if !IsErrorCode(err, ErrUnsupportedOperator) {
		t.Errorf("Expected ErrUnsupportedOperator")
	}
	if IsErrorCode(err, ErrInvalidJSON) {
		t.Errorf("Should not match ErrInvalidJSON")
	}

	// Test ErrValidation
	_, err = transpiler.Transpile(`{"unknown_op": [1]}`)
	if !IsErrorCode(err, ErrValidation) {
		t.Errorf("Expected ErrValidation")
	}

	// Test ErrInvalidJSON
	_, err = transpiler.Transpile(`{invalid}`)
	if !IsErrorCode(err, ErrInvalidJSON) {
		t.Errorf("Expected ErrInvalidJSON")
	}

	// Test nil error
	if IsErrorCode(nil, ErrInvalidJSON) {
		t.Errorf("nil error should not match any code")
	}
}

func TestTranspileErrorUnwrap(t *testing.T) {
	transpiler, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	// Test that we can unwrap to get the underlying error
	_, err = transpiler.Transpile(`{invalid json}`)
	if err == nil {
		t.Fatal("Expected error")
	}

	tpErr, ok := AsTranspileError(err)
	if !ok {
		t.Fatal("Expected TranspileError")
	}

	// The cause should be a JSON syntax error
	if tpErr.Cause == nil {
		t.Error("Expected non-nil cause for JSON error")
	}

	// Verify errors.Unwrap works
	cause := errors.Unwrap(tpErr)
	if cause == nil {
		t.Error("Expected non-nil cause from Unwrap")
	}
}

func TestErrorCodeValues(t *testing.T) {
	// Verify error codes have expected string values
	tests := []struct {
		code     ErrorCode
		expected string
	}{
		{ErrInvalidExpression, "E001"},
		{ErrEmptyArray, "E002"},
		{ErrMultipleKeys, "E003"},
		{ErrPrimitiveNotAllowed, "E004"},
		{ErrArrayNotAllowed, "E005"},
		{ErrValidation, "E006"},
		{ErrInvalidJSON, "E007"},
		{ErrUnsupportedOperator, "E100"},
		{ErrOperatorRequiresArray, "E101"},
		{ErrCustomOperatorFailed, "E102"},
		{ErrTypeMismatch, "E200"},
		{ErrFieldNotInSchema, "E201"},
		{ErrInvalidFieldType, "E202"},
		{ErrInvalidEnumValue, "E203"},
		{ErrInsufficientArgs, "E300"},
		{ErrTooManyArgs, "E301"},
		{ErrInvalidArgument, "E302"},
		{ErrInvalidArgType, "E303"},
		{ErrInvalidDefaultValue, "E304"},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			if string(tt.code) != tt.expected {
				t.Errorf("ErrorCode %s has wrong value, expected %s", tt.code, tt.expected)
			}
		})
	}
}
