package jsonlogic2sql

import (
	"encoding/json"
	"fmt"

	"github.com/h22rana/jsonlogic2sql/internal/parser"
)

// TranspilerConfig holds configuration options for the transpiler
type TranspilerConfig struct {
	UseANSINotEqual bool // true: <>, false: !=
}

// Transpiler provides the main API for converting JSON Logic to SQL WHERE clauses
type Transpiler struct {
	parser           *parser.Parser
	config           *TranspilerConfig
	customOperators  *OperatorRegistry
}

// NewTranspiler creates a new transpiler instance
func NewTranspiler() *Transpiler {
	t := &Transpiler{
		parser: parser.NewParser(),
		config: &TranspilerConfig{
			UseANSINotEqual: true, // Default to ANSI SQL <>
		},
		customOperators: NewOperatorRegistry(),
	}
	t.setupCustomOperatorLookup()
	return t
}

// NewTranspilerWithConfig creates a new transpiler instance with custom configuration
func NewTranspilerWithConfig(config *TranspilerConfig) *Transpiler {
	t := &Transpiler{
		parser:          parser.NewParser(),
		config:          config,
		customOperators: NewOperatorRegistry(),
	}
	t.setupCustomOperatorLookup()
	return t
}

// setupCustomOperatorLookup configures the parser to use our custom operator registry
func (t *Transpiler) setupCustomOperatorLookup() {
	t.parser.SetCustomOperatorLookup(func(operatorName string) (parser.CustomOperatorHandler, bool) {
		handler, ok := t.customOperators.Get(operatorName)
		if !ok {
			return nil, false
		}
		// Wrap the public OperatorHandler to implement parser.CustomOperatorHandler
		return handler, true
	})
}

// RegisterOperator registers a custom operator handler.
// The handler will be called when the operator is encountered during transpilation.
// Returns an error if the operator name conflicts with a built-in operator.
//
// Example:
//
//	transpiler := jsonlogic2sql.NewTranspiler()
//	transpiler.RegisterOperator("length", &LengthOperator{})
//	sql, _ := transpiler.Transpile(`{"length": [{"var": "email"}]}`)
//	// Output: WHERE LENGTH(email)
func (t *Transpiler) RegisterOperator(name string, handler OperatorHandler) error {
	if err := validateOperatorName(name); err != nil {
		return err
	}
	t.customOperators.Register(name, handler)
	return nil
}

// RegisterOperatorFunc registers a custom operator function.
// This is a convenience method for simple operators that don't need state.
// Returns an error if the operator name conflicts with a built-in operator.
//
// Example:
//
//	transpiler := jsonlogic2sql.NewTranspiler()
//	transpiler.RegisterOperatorFunc("length", func(op string, args []interface{}) (string, error) {
//	    if len(args) != 1 {
//	        return "", fmt.Errorf("length requires exactly 1 argument")
//	    }
//	    return fmt.Sprintf("LENGTH(%s)", args[0]), nil
//	})
//	sql, _ := transpiler.Transpile(`{"length": [{"var": "email"}]}`)
//	// Output: WHERE LENGTH(email)
func (t *Transpiler) RegisterOperatorFunc(name string, fn OperatorFunc) error {
	if err := validateOperatorName(name); err != nil {
		return err
	}
	t.customOperators.RegisterFunc(name, fn)
	return nil
}

// UnregisterOperator removes a custom operator from the transpiler.
// Returns true if the operator was found and removed, false otherwise.
func (t *Transpiler) UnregisterOperator(name string) bool {
	return t.customOperators.Unregister(name)
}

// HasCustomOperator checks if a custom operator is registered.
func (t *Transpiler) HasCustomOperator(name string) bool {
	return t.customOperators.Has(name)
}

// ListCustomOperators returns a slice of all registered custom operator names.
func (t *Transpiler) ListCustomOperators() []string {
	return t.customOperators.List()
}

// ClearCustomOperators removes all registered custom operators.
func (t *Transpiler) ClearCustomOperators() {
	t.customOperators.Clear()
}

// Transpile converts a JSON Logic string to a SQL WHERE clause
func (t *Transpiler) Transpile(jsonLogic string) (string, error) {
	var logic interface{}
	if err := json.Unmarshal([]byte(jsonLogic), &logic); err != nil {
		return "", fmt.Errorf("invalid JSON: %v", err)
	}

	return t.parser.Parse(logic)
}

// TranspileFromMap converts a pre-parsed JSON Logic map to a SQL WHERE clause
func (t *Transpiler) TranspileFromMap(logic map[string]interface{}) (string, error) {
	return t.parser.Parse(logic)
}

// TranspileFromInterface converts any JSON Logic interface{} to a SQL WHERE clause
func (t *Transpiler) TranspileFromInterface(logic interface{}) (string, error) {
	return t.parser.Parse(logic)
}

// Convenience functions for direct usage without creating a Transpiler instance

// Transpile converts a JSON Logic string to a SQL WHERE clause
func Transpile(jsonLogic string) (string, error) {
	t := NewTranspiler()
	return t.Transpile(jsonLogic)
}

// TranspileFromMap converts a pre-parsed JSON Logic map to a SQL WHERE clause
func TranspileFromMap(logic map[string]interface{}) (string, error) {
	t := NewTranspiler()
	return t.TranspileFromMap(logic)
}

// TranspileFromInterface converts any JSON Logic interface{} to a SQL WHERE clause
func TranspileFromInterface(logic interface{}) (string, error) {
	t := NewTranspiler()
	return t.TranspileFromInterface(logic)
}
