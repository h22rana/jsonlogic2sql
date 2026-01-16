// Package jsonlogic2sql exports error types for programmatic error handling.
package jsonlogic2sql

import (
	"errors"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
)

// TranspileError is the structured error type returned by transpilation operations.
// Use errors.As to extract this type for programmatic error handling.
//
// Example:
//
//	sql, err := transpiler.Transpile(jsonLogic)
//	if err != nil {
//	    var tpErr *TranspileError
//	    if errors.As(err, &tpErr) {
//	        fmt.Printf("Error code: %s\n", tpErr.Code)
//	        fmt.Printf("Path: %s\n", tpErr.Path)
//	        fmt.Printf("Operator: %s\n", tpErr.Operator)
//	    }
//	}
type TranspileError = tperrors.TranspileError

// ErrorCode represents a specific error condition.
// Codes are organized by category:
//   - E001-E099: Structural/validation errors
//   - E100-E199: Operator-specific errors
//   - E200-E299: Type/schema errors
//   - E300-E399: Argument errors
type ErrorCode = tperrors.ErrorCode

// Error codes for programmatic error handling.
const (
	// Structural/validation errors (E001-E099).
	ErrInvalidExpression   = tperrors.ErrInvalidExpression
	ErrEmptyArray          = tperrors.ErrEmptyArray
	ErrMultipleKeys        = tperrors.ErrMultipleKeys
	ErrPrimitiveNotAllowed = tperrors.ErrPrimitiveNotAllowed
	ErrArrayNotAllowed     = tperrors.ErrArrayNotAllowed
	ErrValidation          = tperrors.ErrValidation
	ErrInvalidJSON         = tperrors.ErrInvalidJSON

	// Operator-specific errors (E100-E199).
	ErrUnsupportedOperator   = tperrors.ErrUnsupportedOperator
	ErrOperatorRequiresArray = tperrors.ErrOperatorRequiresArray
	ErrCustomOperatorFailed  = tperrors.ErrCustomOperatorFailed

	// Type/schema errors (E200-E299).
	ErrTypeMismatch     = tperrors.ErrTypeMismatch
	ErrFieldNotInSchema = tperrors.ErrFieldNotInSchema
	ErrInvalidFieldType = tperrors.ErrInvalidFieldType
	ErrInvalidEnumValue = tperrors.ErrInvalidEnumValue

	// Argument errors (E300-E399).
	ErrInsufficientArgs    = tperrors.ErrInsufficientArgs
	ErrTooManyArgs         = tperrors.ErrTooManyArgs
	ErrInvalidArgument     = tperrors.ErrInvalidArgument
	ErrInvalidArgType      = tperrors.ErrInvalidArgType
	ErrInvalidDefaultValue = tperrors.ErrInvalidDefaultValue
)

// AsTranspileError attempts to extract a TranspileError from an error.
// Returns the TranspileError and true if the error is or wraps a TranspileError,
// otherwise returns nil and false.
//
// Example:
//
//	sql, err := transpiler.Transpile(jsonLogic)
//	if tpErr, ok := jsonlogic2sql.AsTranspileError(err); ok {
//	    fmt.Printf("Error code: %s\n", tpErr.Code)
//	}
func AsTranspileError(err error) (*TranspileError, bool) {
	var tpErr *TranspileError
	if errors.As(err, &tpErr) {
		return tpErr, true
	}
	return nil, false
}

// IsErrorCode checks if an error has a specific error code.
// Returns true if the error is or wraps a TranspileError with the given code.
//
// Example:
//
//	if jsonlogic2sql.IsErrorCode(err, jsonlogic2sql.ErrUnsupportedOperator) {
//	    fmt.Println("Unknown operator used")
//	}
func IsErrorCode(err error, code ErrorCode) bool {
	tpErr, ok := AsTranspileError(err)
	if !ok {
		return false
	}
	return tpErr.Code == code
}
