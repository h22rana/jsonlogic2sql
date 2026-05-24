// Package errors provides structured error types for jsonlogic2sql transpilation.
package errors

import (
	"fmt"
	"strconv"
	"strings"
)

const jsonPathRoot = "$"

// ErrorCode represents a specific error condition.
// Codes are organized by category:
//   - E001-E099: Structural/validation errors
//   - E100-E199: Operator-specific errors
//   - E200-E299: Type/schema errors
//   - E300-E399: Argument errors
type ErrorCode string

// Structural/validation error codes (E001-E099).
const (
	// ErrInvalidExpression indicates the expression type is not valid JSON Logic.
	ErrInvalidExpression ErrorCode = "E001"
	// ErrEmptyArray indicates an array cannot be empty in this context.
	ErrEmptyArray ErrorCode = "E002"
	// ErrMultipleKeys indicates an operator object must have exactly one key.
	ErrMultipleKeys ErrorCode = "E003"
	// ErrPrimitiveNotAllowed indicates primitive values are not allowed in this context.
	ErrPrimitiveNotAllowed ErrorCode = "E004"
	// ErrArrayNotAllowed indicates arrays are not allowed in this context.
	ErrArrayNotAllowed ErrorCode = "E005"
	// ErrValidation indicates a general validation error.
	ErrValidation ErrorCode = "E006"
	// ErrInvalidJSON indicates the input is not valid JSON.
	ErrInvalidJSON ErrorCode = "E007"
	// ErrInvalidExpressionContext indicates a value expression was used where
	// a predicate was required, or vice versa.
	ErrInvalidExpressionContext ErrorCode = "E008"
)

// Operator-specific error codes (E100-E199).
const (
	// ErrUnsupportedOperator indicates the operator is not recognized.
	ErrUnsupportedOperator ErrorCode = "E100"
	// ErrOperatorRequiresArray indicates the operator requires array arguments.
	ErrOperatorRequiresArray ErrorCode = "E101"
	// ErrCustomOperatorFailed indicates a custom operator returned an error.
	ErrCustomOperatorFailed ErrorCode = "E102"
)

// Type/schema error codes (E200-E299).
const (
	// ErrTypeMismatch indicates a type mismatch in the expression.
	ErrTypeMismatch ErrorCode = "E200"
	// ErrFieldNotInSchema indicates the field is not defined in the schema.
	ErrFieldNotInSchema ErrorCode = "E201"
	// ErrInvalidFieldType indicates the field type is incompatible with the operation.
	ErrInvalidFieldType ErrorCode = "E202"
	// ErrInvalidEnumValue indicates the value is not valid for the enum field.
	ErrInvalidEnumValue ErrorCode = "E203"
)

// Argument error codes (E300-E399).
const (
	// ErrInsufficientArgs indicates the operator requires more arguments.
	ErrInsufficientArgs ErrorCode = "E300"
	// ErrTooManyArgs indicates the operator received too many arguments.
	ErrTooManyArgs ErrorCode = "E301"
	// ErrInvalidArgument indicates an argument is invalid.
	ErrInvalidArgument ErrorCode = "E302"
	// ErrInvalidArgType indicates an argument has the wrong type.
	ErrInvalidArgType ErrorCode = "E303"
	// ErrInvalidDefaultValue indicates the default value is invalid.
	ErrInvalidDefaultValue ErrorCode = "E304"
	// ErrUnreferencedPlaceholder indicates a collected bind parameter has no
	// matching placeholder in the generated SQL, typically because a custom
	// operator dropped an argument.
	ErrUnreferencedPlaceholder ErrorCode = "E350"
)

// TranspileError represents an error during JSONLogic transpilation.
// It implements the error interface and provides structured context for debugging.
type TranspileError struct {
	// Code is the error code for programmatic handling.
	Code ErrorCode
	// Operator is the operator that caused the error (may be empty for structural errors).
	Operator string
	// Path is the JSONPath to the error location (e.g., "$.and[0].>").
	Path string
	// Message is the human-readable error message.
	Message string
	// Cause is the underlying error, if any.
	Cause error
}

// Error implements the error interface.
func (e *TranspileError) Error() string {
	var b strings.Builder
	b.WriteByte('[')
	b.WriteString(string(e.Code))
	b.WriteByte(']')

	if e.Path != "" {
		b.WriteString(" at ")
		b.WriteString(e.Path)
	}
	if e.Operator != "" {
		b.WriteString(" (operator: ")
		b.WriteString(e.Operator)
		b.WriteByte(')')
	}

	b.WriteString(": ")
	b.WriteString(e.Message)
	if e.Cause != nil {
		b.WriteString(": ")
		b.WriteString(e.Cause.Error())
	}

	return b.String()
}

// Unwrap returns the underlying error for errors.Is/As support.
func (e *TranspileError) Unwrap() error {
	return e.Cause
}

// New creates a new TranspileError with the given parameters.
func New(code ErrorCode, operator, path, message string) *TranspileError {
	return &TranspileError{
		Code:     code,
		Operator: operator,
		Path:     path,
		Message:  message,
	}
}

// Wrap creates a new TranspileError wrapping an underlying error.
func Wrap(code ErrorCode, operator, path, message string, cause error) *TranspileError {
	return &TranspileError{
		Code:     code,
		Operator: operator,
		Path:     path,
		Message:  message,
		Cause:    cause,
	}
}

// WithPath returns a copy of the error with the path updated.
// This is useful for adding path context as errors bubble up.
func (e *TranspileError) WithPath(path string) *TranspileError {
	next := *e
	next.Path = path
	return &next
}

// WithOperator returns a copy of the error with the operator updated.
func (e *TranspileError) WithOperator(operator string) *TranspileError {
	next := *e
	next.Operator = operator
	return &next
}

// Helper constructors for common errors.

// NewUnsupportedOperator creates an error for an unsupported operator.
func NewUnsupportedOperator(operator, path string) *TranspileError {
	return New(ErrUnsupportedOperator, operator, path,
		fmt.Sprintf("unsupported operator: %s", operator))
}

// NewOperatorRequiresArray creates an error when an operator needs array arguments.
func NewOperatorRequiresArray(operator, path string) *TranspileError {
	return New(ErrOperatorRequiresArray, operator, path,
		fmt.Sprintf("%s operator requires array arguments", operator))
}

// NewInsufficientArgs creates an error for insufficient arguments.
func NewInsufficientArgs(operator, path string, required, got int) *TranspileError {
	return New(ErrInsufficientArgs, operator, path,
		fmt.Sprintf("%s operator requires at least %d argument(s), got %d", operator, required, got))
}

// NewTooManyArgs creates an error for too many arguments.
func NewTooManyArgs(operator, path string, maxArgs, got int) *TranspileError {
	return New(ErrTooManyArgs, operator, path,
		fmt.Sprintf("%s operator accepts at most %d argument(s), got %d", operator, maxArgs, got))
}

// NewInvalidArgument creates an error for an invalid argument at a specific index.
func NewInvalidArgument(operator, path string, argIndex int, cause error) *TranspileError {
	return Wrap(ErrInvalidArgument, operator, path,
		fmt.Sprintf("invalid argument at index %d", argIndex), cause)
}

// NewTypeMismatch creates an error for a type mismatch.
func NewTypeMismatch(operator, path, expected, got string) *TranspileError {
	return New(ErrTypeMismatch, operator, path,
		fmt.Sprintf("expected %s, got %s", expected, got))
}

// NewInvalidExpressionContext creates an error for expressions used in a
// disallowed context.
func NewInvalidExpressionContext(operator, path, expected, got string) *TranspileError {
	return New(ErrInvalidExpressionContext, operator, path,
		fmt.Sprintf("expected %s expression, got %s expression", expected, got))
}

// NewFieldNotInSchema creates an error when a field is not in the schema.
func NewFieldNotInSchema(field, path string) *TranspileError {
	return New(ErrFieldNotInSchema, "var", path,
		fmt.Sprintf("field '%s' is not defined in schema", field))
}

// NewInvalidFieldType creates an error for an incompatible field type.
func NewInvalidFieldType(operator, path, field, fieldType, expectedType string) *TranspileError {
	return New(ErrInvalidFieldType, operator, path,
		fmt.Sprintf("%s operation on incompatible field '%s' (type: %s, expected: %s)", operator, field, fieldType, expectedType))
}

// NewInvalidEnumValue creates an error for an invalid enum value.
func NewInvalidEnumValue(field, path, value string, allowed []string) *TranspileError {
	return New(ErrInvalidEnumValue, "==", path,
		fmt.Sprintf("invalid enum value '%s' for field '%s': allowed values are %v", value, field, allowed))
}

// NewMultipleKeys creates an error when an operator object has multiple keys.
func NewMultipleKeys(path string) *TranspileError {
	return New(ErrMultipleKeys, "", path,
		"operator object must have exactly one key")
}

// NewPrimitiveNotAllowed creates an error when primitives are not allowed.
func NewPrimitiveNotAllowed(path string) *TranspileError {
	return New(ErrPrimitiveNotAllowed, "", path,
		"primitive values are not supported in this expression context")
}

// NewArrayNotAllowed creates an error when arrays are not allowed at top level.
func NewArrayNotAllowed(path string) *TranspileError {
	return New(ErrArrayNotAllowed, "", path,
		"arrays are not supported in this expression context")
}

// BuildPath constructs a JSONPath string for a given operator and index.
func BuildPath(parent, operator string, index int) string {
	if parent == "" {
		parent = jsonPathRoot
	}
	if index >= 0 {
		return parent + "." + operator + "[" + strconv.Itoa(index) + "]"
	}
	return parent + "." + operator
}

// BuildArrayPath constructs a JSONPath string for an array index.
func BuildArrayPath(parent string, index int) string {
	if parent == "" {
		parent = jsonPathRoot
	}
	return parent + "[" + strconv.Itoa(index) + "]"
}

// NewValidationError wraps a validation error from the validator package.
func NewValidationError(cause error) *TranspileError {
	return Wrap(ErrValidation, "", "", "validation failed", cause)
}

// NewInvalidJSON creates an error for invalid JSON input.
func NewInvalidJSON(cause error) *TranspileError {
	return Wrap(ErrInvalidJSON, "", "", "invalid JSON", cause)
}
