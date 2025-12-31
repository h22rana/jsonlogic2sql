package jsonlogic2sql

import (
	"fmt"
	"maps"
	"sync"
)

// OperatorFunc is a function type for custom operator implementations.
// It receives the operator name and its arguments, and returns the SQL representation.
//
// Example:
//
//	lengthOp := func(operator string, args []interface{}) (string, error) {
//	    if len(args) != 1 {
//	        return "", fmt.Errorf("length requires exactly 1 argument")
//	    }
//	    // args[0] will be the SQL representation of the argument
//	    return fmt.Sprintf("LENGTH(%s)", args[0]), nil
//	}
type OperatorFunc func(operator string, args []interface{}) (string, error)

// OperatorHandler is an interface for custom operator implementations.
// Implement this interface for more complex operators that need state.
//
// Example:
//
//	type MyOperator struct {
//	    prefix string
//	}
//
//	func (m *MyOperator) ToSQL(operator string, args []interface{}) (string, error) {
//	    return fmt.Sprintf("%s_%s", m.prefix, args[0]), nil
//	}
type OperatorHandler interface {
	// ToSQL converts the operator and its arguments to SQL.
	// The args slice contains the SQL representations of each argument.
	ToSQL(operator string, args []interface{}) (string, error)
}

// funcHandler wraps an OperatorFunc to implement OperatorHandler
type funcHandler struct {
	fn OperatorFunc
}

func (f *funcHandler) ToSQL(operator string, args []interface{}) (string, error) {
	return f.fn(operator, args)
}

// OperatorRegistry manages custom operator registrations.
// It is thread-safe and can be used concurrently.
type OperatorRegistry struct {
	mu       sync.RWMutex
	handlers map[string]OperatorHandler
}

// NewOperatorRegistry creates a new empty operator registry.
func NewOperatorRegistry() *OperatorRegistry {
	return &OperatorRegistry{
		handlers: make(map[string]OperatorHandler),
	}
}

// Register adds a custom operator handler to the registry.
// If an operator with the same name already exists, it will be replaced.
//
// Example:
//
//	registry := NewOperatorRegistry()
//	registry.Register("length", &LengthOperator{})
func (r *OperatorRegistry) Register(operatorName string, handler OperatorHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[operatorName] = handler
}

// RegisterFunc adds a custom operator function to the registry.
// This is a convenience method for simple operators that don't need state.
//
// Example:
//
//	registry := NewOperatorRegistry()
//	registry.RegisterFunc("length", func(op string, args []interface{}) (string, error) {
//	    return fmt.Sprintf("LENGTH(%s)", args[0]), nil
//	})
func (r *OperatorRegistry) RegisterFunc(operatorName string, fn OperatorFunc) {
	r.Register(operatorName, &funcHandler{fn: fn})
}

// Unregister removes a custom operator from the registry.
// Returns true if the operator was found and removed, false otherwise.
func (r *OperatorRegistry) Unregister(operatorName string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.handlers[operatorName]; exists {
		delete(r.handlers, operatorName)
		return true
	}
	return false
}

// Get retrieves a custom operator handler from the registry.
// Returns the handler and true if found, nil and false otherwise.
func (r *OperatorRegistry) Get(operatorName string) (OperatorHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	handler, ok := r.handlers[operatorName]
	return handler, ok
}

// Has checks if an operator is registered.
func (r *OperatorRegistry) Has(operatorName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.handlers[operatorName]
	return ok
}

// List returns a slice of all registered operator names.
func (r *OperatorRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		names = append(names, name)
	}
	return names
}

// Clear removes all registered operators.
func (r *OperatorRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers = make(map[string]OperatorHandler)
}

// Clone creates a copy of the registry with all registered operators.
func (r *OperatorRegistry) Clone() *OperatorRegistry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	clone := NewOperatorRegistry()
	for name, handler := range r.handlers {
		clone.handlers[name] = handler
	}
	return clone
}

// Merge adds all operators from another registry to this one.
// Existing operators with the same name will be replaced.
func (r *OperatorRegistry) Merge(other *OperatorRegistry) {
	other.mu.RLock()
	defer other.mu.RUnlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	maps.Copy(r.handlers, other.handlers)
}

// validateOperatorName checks if an operator name is valid.
// Returns an error if the name conflicts with built-in operators.
func validateOperatorName(name string) error {
	builtInOperators := map[string]bool{
		// Data access
		"var": true, "missing": true, "missing_some": true,
		// Logical and Boolean
		"if": true, "==": true, "===": true, "!=": true, "!==": true,
		"and": true, "or": true, "!": true, "!!": true,
		// Numeric
		">": true, ">=": true, "<": true, "<=": true,
		"+": true, "-": true, "*": true, "/": true, "%": true,
		"max": true, "min": true,
		// String and Array
		"cat": true, "substr": true,
		"in":  true,
		"map": true, "filter": true, "reduce": true,
		"all": true, "some": true, "none": true, "merge": true,
	}

	if builtInOperators[name] {
		return fmt.Errorf("cannot override built-in operator: %s", name)
	}
	return nil
}
