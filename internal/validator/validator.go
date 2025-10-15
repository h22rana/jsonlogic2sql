package validator

import (
	"fmt"
)

// ValidationError represents a validation error with context
type ValidationError struct {
	Operator string
	Message  string
	Path     string
}

func (e ValidationError) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("validation error at %s: %s", e.Path, e.Message)
	}
	return fmt.Sprintf("validation error: %s", e.Message)
}

// Validator validates JSON Logic expressions
type Validator struct {
	supportedOperators map[string]OperatorSpec
}

// OperatorSpec defines the specification for an operator
type OperatorSpec struct {
	Name        string
	MinArgs     int
	MaxArgs     int
	ArgTypes    []ArgType
	Description string
}

// ArgType represents the expected type of an argument
type ArgType int

const (
	AnyType ArgType = iota
	NumberType
	StringType
	BooleanType
	ArrayType
	ObjectType
	VariableType
)

// NewValidator creates a new validator with all supported operators
func NewValidator() *Validator {
	return &Validator{
		supportedOperators: getSupportedOperators(),
	}
}

// Validate validates a JSON Logic expression
func (v *Validator) Validate(logic interface{}) error {
	return v.validateRecursive(logic, "")
}

// validateRecursive recursively validates JSON Logic expressions
func (v *Validator) validateRecursive(logic interface{}, path string) error {
	// Handle primitive values (literals) including null
	if v.isPrimitive(logic) {
		return nil
	}

	// Handle arrays
	if arr, ok := logic.([]interface{}); ok {
		return v.validateArray(arr, path)
	}

	// Handle objects (operators)
	if obj, ok := logic.(map[string]interface{}); ok {
		return v.validateObject(obj, path)
	}

	return ValidationError{
		Message: fmt.Sprintf("invalid type: %T", logic),
		Path:    path,
	}
}

// validateArray validates an array expression
func (v *Validator) validateArray(arr []interface{}, path string) error {
	// Empty arrays are valid in JSONLogic (they evaluate to falsy)
	// So we don't reject them

	for i, item := range arr {
		itemPath := fmt.Sprintf("%s[%d]", path, i)
		if err := v.validateRecursive(item, itemPath); err != nil {
			return err
		}
	}

	return nil
}

// validateObject validates an object expression (operator)
func (v *Validator) validateObject(obj map[string]interface{}, path string) error {
	if len(obj) != 1 {
		return ValidationError{
			Message: "operator object must have exactly one key",
			Path:    path,
		}
	}

	for operator, args := range obj {
		operatorPath := fmt.Sprintf("%s.%s", path, operator)

		// Check if operator is supported
		spec, exists := v.supportedOperators[operator]
		if !exists {
			return ValidationError{
				Operator: operator,
				Message:  fmt.Sprintf("unsupported operator: %s", operator),
				Path:     path,
			}
		}

		// Validate arguments
		if err := v.validateOperatorArgs(operator, args, spec, operatorPath); err != nil {
			return err
		}
	}

	return nil
}

// validateOperatorArgs validates the arguments for a specific operator
func (v *Validator) validateOperatorArgs(operator string, args interface{}, spec OperatorSpec, path string) error {
	// Handle different argument structures
	switch operator {
	case "var":
		// var can have 1 or 2 arguments: [path] or [path, default]
		// It can also be a number for array indexing
		if _, ok := args.(string); ok {
			// Single string argument
			return nil
		}
		if v.isNumber(args) {
			// Single numeric argument for array indexing
			return nil
		}
		if arr, ok := args.([]interface{}); ok {
			if len(arr) < 1 || len(arr) > 2 {
				return ValidationError{
					Operator: operator,
					Message:  "var operator requires 1 or 2 arguments",
					Path:     path,
				}
			}
			// First argument can be string or number
			if !v.isString(arr[0]) && !v.isNumber(arr[0]) {
				return ValidationError{
					Operator: operator,
					Message:  "var operator first argument must be a string or number",
					Path:     path,
				}
			}
			return nil
		}
		return ValidationError{
			Operator: operator,
			Message:  "var operator requires string, number, or array arguments",
			Path:     path,
		}

	case "missing", "missing_some":
		// These operators have specific argument requirements
		return v.validateMissingOperator(operator, args, path)

	default:
		// Standard operator validation
		return v.validateStandardOperator(operator, args, spec, path)
	}
}

// validateMissingOperator validates missing and missing_some operators
func (v *Validator) validateMissingOperator(operator string, args interface{}, path string) error {
	if operator == "missing" {
		// missing takes a string argument directly or an array of strings
		if _, ok := args.(string); ok {
			return nil
		}
		if arr, ok := args.([]interface{}); ok {
			// Check that all array elements are strings
			for _, elem := range arr {
				if !v.isString(elem) {
					return ValidationError{
						Operator: operator,
						Message:  "all elements in missing operator array must be strings",
						Path:     path,
					}
				}
			}
			return nil
		}
		return ValidationError{
			Operator: operator,
			Message:  "missing operator argument must be a string or array of strings",
			Path:     path,
		}
	} else if operator == "missing_some" {
		// missing_some takes an array argument
		arr, ok := args.([]interface{})
		if !ok {
			return ValidationError{
				Operator: operator,
				Message:  "missing_some operator requires array argument",
				Path:     path,
			}
		}
		if len(arr) != 2 {
			return ValidationError{
				Operator: operator,
				Message:  "missing_some operator requires exactly 2 arguments",
				Path:     path,
			}
		}
		// First argument should be a number
		if !v.isNumber(arr[0]) {
			return ValidationError{
				Operator: operator,
				Message:  "missing_some operator first argument must be a number",
				Path:     path,
			}
		}
		// Second argument should be an array
		if _, ok := arr[1].([]interface{}); !ok {
			return ValidationError{
				Operator: operator,
				Message:  "missing_some operator second argument must be an array",
				Path:     path,
			}
		}
	}

	return nil
}

// validateStandardOperator validates standard operators with array arguments
func (v *Validator) validateStandardOperator(operator string, args interface{}, spec OperatorSpec, path string) error {
	// Special handling for unary operators (! and !!) - they can accept non-array arguments
	if operator == "!" || operator == "!!" {
		// Accept both array and non-array arguments
		if arr, ok := args.([]interface{}); ok {
			if len(arr) != 1 {
				return ValidationError{
					Operator: operator,
					Message:  fmt.Sprintf("%s operator requires exactly 1 argument", operator),
					Path:     path,
				}
			}
			// Validate the single argument recursively
			return v.validateRecursive(arr[0], fmt.Sprintf("%s[0]", path))
		}
		// Non-array argument is also valid for unary operators
		return v.validateRecursive(args, path)
	}

	arr, ok := args.([]interface{})
	if !ok {
		return ValidationError{
			Operator: operator,
			Message:  fmt.Sprintf("%s operator requires array argument", operator),
			Path:     path,
		}
	}

	// Check argument count
	if len(arr) < spec.MinArgs {
		return ValidationError{
			Operator: operator,
			Message:  fmt.Sprintf("%s operator requires at least %d arguments, got %d", operator, spec.MinArgs, len(arr)),
			Path:     path,
		}
	}

	if spec.MaxArgs != -1 && len(arr) > spec.MaxArgs {
		return ValidationError{
			Operator: operator,
			Message:  fmt.Sprintf("%s operator requires at most %d arguments, got %d", operator, spec.MaxArgs, len(arr)),
			Path:     path,
		}
	}

	// Validate argument types
	for i, arg := range arr {
		if i < len(spec.ArgTypes) {
			if err := v.validateArgType(arg, spec.ArgTypes[i], fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateArgType validates that an argument matches the expected type
func (v *Validator) validateArgType(arg interface{}, expectedType ArgType, path string) error {
	switch expectedType {
	case AnyType:
		return nil
	case NumberType:
		if !v.isNumber(arg) {
			return ValidationError{
				Message: fmt.Sprintf("expected number, got %T", arg),
				Path:    path,
			}
		}
	case StringType:
		if !v.isString(arg) {
			return ValidationError{
				Message: fmt.Sprintf("expected string, got %T", arg),
				Path:    path,
			}
		}
	case BooleanType:
		if !v.isBoolean(arg) {
			return ValidationError{
				Message: fmt.Sprintf("expected boolean, got %T", arg),
				Path:    path,
			}
		}
	case ArrayType:
		if !v.isArray(arg) {
			return ValidationError{
				Message: fmt.Sprintf("expected array, got %T", arg),
				Path:    path,
			}
		}
	case ObjectType:
		if !v.isObject(arg) {
			return ValidationError{
				Message: fmt.Sprintf("expected object, got %T", arg),
				Path:    path,
			}
		}
	}

	return nil
}

// Helper methods for type checking
func (v *Validator) isPrimitive(value interface{}) bool {
	return v.isNumber(value) || v.isString(value) || v.isBoolean(value) || value == nil
}

func (v *Validator) isNumber(value interface{}) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	}
	return false
}

func (v *Validator) isString(value interface{}) bool {
	_, ok := value.(string)
	return ok
}

func (v *Validator) isBoolean(value interface{}) bool {
	_, ok := value.(bool)
	return ok
}

func (v *Validator) isArray(value interface{}) bool {
	_, ok := value.([]interface{})
	return ok
}

func (v *Validator) isObject(value interface{}) bool {
	_, ok := value.(map[string]interface{})
	return ok
}

// getSupportedOperators returns the map of supported operators with their specifications
func getSupportedOperators() map[string]OperatorSpec {
	return map[string]OperatorSpec{
		// Data access operators
		"var": {
			Name:        "var",
			MinArgs:     1,
			MaxArgs:     2,
			Description: "Access variable value",
		},
		"missing": {
			Name:        "missing",
			MinArgs:     1,
			MaxArgs:     1,
			ArgTypes:    []ArgType{StringType},
			Description: "Check if variable is missing",
		},
		"missing_some": {
			Name:        "missing_some",
			MinArgs:     2,
			MaxArgs:     2,
			Description: "Check if some variables are missing",
		},

		// Logic and Boolean operations
		"if": {
			Name:        "if",
			MinArgs:     2,
			MaxArgs:     -1, // Variable number of arguments for nested IF
			Description: "Conditional expression",
		},
		"==": {
			Name:        "==",
			MinArgs:     2,
			MaxArgs:     2,
			ArgTypes:    []ArgType{AnyType, AnyType},
			Description: "Equality comparison",
		},
		"===": {
			Name:        "===",
			MinArgs:     2,
			MaxArgs:     2,
			ArgTypes:    []ArgType{AnyType, AnyType},
			Description: "Strict equality comparison",
		},
		"!=": {
			Name:        "!=",
			MinArgs:     2,
			MaxArgs:     2,
			ArgTypes:    []ArgType{AnyType, AnyType},
			Description: "Inequality comparison",
		},
		"!==": {
			Name:        "!==",
			MinArgs:     2,
			MaxArgs:     2,
			ArgTypes:    []ArgType{AnyType, AnyType},
			Description: "Strict inequality comparison",
		},
		"!": {
			Name:        "!",
			MinArgs:     1,
			MaxArgs:     1,
			ArgTypes:    []ArgType{AnyType},
			Description: "Logical NOT",
		},
		"!!": {
			Name:        "!!",
			MinArgs:     1,
			MaxArgs:     1,
			ArgTypes:    []ArgType{AnyType},
			Description: "Double negation (boolean conversion)",
		},
		"?:": {
			Name:        "?:",
			MinArgs:     2,
			MaxArgs:     -1, // Variable number of arguments for chained ternary
			ArgTypes:    []ArgType{AnyType, AnyType},
			Description: "Ternary operator (condition ? true_value : false_value)",
		},
		"or": {
			Name:        "or",
			MinArgs:     1,
			MaxArgs:     -1, // Variable number of arguments
			Description: "Logical OR",
		},
		"and": {
			Name:        "and",
			MinArgs:     1,
			MaxArgs:     -1, // Variable number of arguments
			Description: "Logical AND",
		},

		// Numeric operations
		">": {
			Name:        ">",
			MinArgs:     2,
			MaxArgs:     -1, // Variable number of arguments for chained comparisons
			ArgTypes:    []ArgType{AnyType, AnyType},
			Description: "Greater than",
		},
		">=": {
			Name:        ">=",
			MinArgs:     2,
			MaxArgs:     -1, // Variable number of arguments for chained comparisons
			ArgTypes:    []ArgType{AnyType, AnyType},
			Description: "Greater than or equal",
		},
		"<": {
			Name:        "<",
			MinArgs:     2,
			MaxArgs:     -1, // Variable number of arguments for chained comparisons
			ArgTypes:    []ArgType{AnyType, AnyType},
			Description: "Less than",
		},
		"<=": {
			Name:        "<=",
			MinArgs:     2,
			MaxArgs:     -1, // Variable number of arguments for chained comparisons
			ArgTypes:    []ArgType{AnyType, AnyType},
			Description: "Less than or equal",
		},
		"between": {
			Name:        "between",
			MinArgs:     3,
			MaxArgs:     3,
			ArgTypes:    []ArgType{AnyType, AnyType, AnyType},
			Description: "Check if value is between two numbers",
		},
		"max": {
			Name:        "max",
			MinArgs:     1,
			MaxArgs:     -1,
			Description: "Maximum value",
		},
		"min": {
			Name:        "min",
			MinArgs:     1,
			MaxArgs:     -1,
			Description: "Minimum value",
		},
		"+": {
			Name:        "+",
			MinArgs:     1,
			MaxArgs:     -1,
			Description: "Addition",
		},
		"-": {
			Name:        "-",
			MinArgs:     1,
			MaxArgs:     -1,
			Description: "Subtraction",
		},
		"*": {
			Name:        "*",
			MinArgs:     1,
			MaxArgs:     -1,
			Description: "Multiplication",
		},
		"/": {
			Name:        "/",
			MinArgs:     1,
			MaxArgs:     -1,
			Description: "Division",
		},
		"%": {
			Name:        "%",
			MinArgs:     2,
			MaxArgs:     2,
			ArgTypes:    []ArgType{AnyType, AnyType},
			Description: "Modulo",
		},

		// Array operations
		"in": {
			Name:        "in",
			MinArgs:     2,
			MaxArgs:     2,
			ArgTypes:    []ArgType{AnyType, AnyType}, // Allow variables on right side
			Description: "Check if value is in array",
		},
		"map": {
			Name:        "map",
			MinArgs:     2,
			MaxArgs:     2,
			Description: "Map over array",
		},
		"filter": {
			Name:        "filter",
			MinArgs:     2,
			MaxArgs:     2,
			Description: "Filter array",
		},
		"reduce": {
			Name:        "reduce",
			MinArgs:     3,
			MaxArgs:     3,
			Description: "Reduce array",
		},
		"all": {
			Name:        "all",
			MinArgs:     2,
			MaxArgs:     2,
			Description: "Check if all elements satisfy condition",
		},
		"some": {
			Name:        "some",
			MinArgs:     2,
			MaxArgs:     2,
			Description: "Check if some elements satisfy condition",
		},
		"none": {
			Name:        "none",
			MinArgs:     2,
			MaxArgs:     2,
			Description: "Check if no elements satisfy condition",
		},
		"merge": {
			Name:        "merge",
			MinArgs:     1,
			MaxArgs:     -1,
			Description: "Merge arrays",
		},

		// String operations
		"cat": {
			Name:        "cat",
			MinArgs:     1,
			MaxArgs:     -1,
			Description: "Concatenate strings",
		},
		"substr": {
			Name:        "substr",
			MinArgs:     2,
			MaxArgs:     3,
			Description: "Substring operation",
		},
	}
}

// GetSupportedOperators returns a list of all supported operators
func (v *Validator) GetSupportedOperators() []string {
	operators := make([]string, 0, len(v.supportedOperators))
	for op := range v.supportedOperators {
		operators = append(operators, op)
	}
	return operators
}

// IsOperatorSupported checks if an operator is supported
func (v *Validator) IsOperatorSupported(operator string) bool {
	_, exists := v.supportedOperators[operator]
	return exists
}
