package jsonlogic2sql

import (
	"encoding/json"
	"fmt"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/parser"
)

// Re-export dialect constants for public API.
const (
	DialectBigQuery = dialect.DialectBigQuery
	DialectSpanner  = dialect.DialectSpanner
)

// Dialect is the type for SQL dialect selection.
type Dialect = dialect.Dialect

// TranspilerConfig holds configuration options for the transpiler.
type TranspilerConfig struct {
	Dialect         Dialect // Required: target SQL dialect
	UseANSINotEqual bool    // true: <>, false: !=
	Schema          *Schema // Optional schema for field validation
}

// Transpiler provides the main API for converting JSON Logic to SQL WHERE clauses.
type Transpiler struct {
	parser          *parser.Parser
	config          *TranspilerConfig
	operatorConfig  *operators.OperatorConfig
	customOperators *OperatorRegistry
}

// SetSchema sets the schema for field validation and type checking
// This is optional - if not set, no schema validation will be performed.
func (t *Transpiler) SetSchema(schema *Schema) {
	t.operatorConfig.Schema = schema
	// All operators automatically see the new schema through the shared config
}

// NewTranspiler creates a new transpiler instance with the specified dialect.
// Dialect is required - use DialectBigQuery or DialectSpanner.
func NewTranspiler(d Dialect) (*Transpiler, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}

	opConfig := operators.NewOperatorConfig(d, nil)
	t := &Transpiler{
		parser:         parser.NewParser(opConfig),
		operatorConfig: opConfig,
		config: &TranspilerConfig{
			Dialect:         d,
			UseANSINotEqual: true, // Default to ANSI SQL <>
		},
		customOperators: NewOperatorRegistry(),
	}
	t.setupCustomOperatorLookup()
	return t, nil
}

// NewTranspilerWithConfig creates a new transpiler instance with custom configuration.
// Config.Dialect is required - use DialectBigQuery or DialectSpanner.
func NewTranspilerWithConfig(config *TranspilerConfig) (*Transpiler, error) {
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if err := config.Dialect.Validate(); err != nil {
		return nil, err
	}

	opConfig := operators.NewOperatorConfig(config.Dialect, config.Schema)
	t := &Transpiler{
		parser:          parser.NewParser(opConfig),
		operatorConfig:  opConfig,
		config:          config,
		customOperators: NewOperatorRegistry(),
	}
	t.setupCustomOperatorLookup()
	return t, nil
}

// setupCustomOperatorLookup configures the parser to use our custom operator registry.
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

// GetDialect returns the configured dialect.
func (t *Transpiler) GetDialect() Dialect {
	return t.config.Dialect
}

// RegisterOperator registers a custom operator handler.
// The handler will be called when the operator is encountered during transpilation.
// Returns an error if the operator name conflicts with a built-in operator.
//
// Example:
//
//	transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
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
//	transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
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

// RegisterDialectAwareOperator registers a dialect-aware custom operator handler.
// Use this when your operator needs to generate different SQL based on the target dialect.
// Returns an error if the operator name conflicts with a built-in operator.
//
// Example:
//
//	transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
//	transpiler.RegisterDialectAwareOperator("now", &CurrentTimeOperator{})
//	sql, _ := transpiler.Transpile(`{"now": []}`)
//	// BigQuery: WHERE CURRENT_TIMESTAMP()
//	// Spanner: WHERE CURRENT_TIMESTAMP()
func (t *Transpiler) RegisterDialectAwareOperator(name string, handler DialectAwareOperatorHandler) error {
	if err := validateOperatorName(name); err != nil {
		return err
	}
	// Wrap in a handler that implements OperatorHandler for registry storage
	wrapper := &dialectAwareHandlerWrapper{handler: handler, dialect: t.config.Dialect}
	t.customOperators.Register(name, wrapper)
	return nil
}

// RegisterDialectAwareOperatorFunc registers a dialect-aware custom operator function.
// Use this for operators that need to generate different SQL based on the target dialect.
// Returns an error if the operator name conflicts with a built-in operator.
//
// Example:
//
//	transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
//	transpiler.RegisterDialectAwareOperatorFunc("now", func(op string, args []interface{}, dialect jsonlogic2sql.Dialect) (string, error) {
//	    switch dialect {
//	    case jsonlogic2sql.DialectBigQuery:
//	        return "CURRENT_TIMESTAMP()", nil
//	    case jsonlogic2sql.DialectSpanner:
//	        return "CURRENT_TIMESTAMP()", nil
//	    default:
//	        return "", fmt.Errorf("unsupported dialect: %s", dialect)
//	    }
//	})
func (t *Transpiler) RegisterDialectAwareOperatorFunc(name string, fn DialectAwareOperatorFunc) error {
	if err := validateOperatorName(name); err != nil {
		return err
	}
	// Wrap the function with the dialect so ToSQL works correctly
	dialect := t.config.Dialect
	t.customOperators.RegisterFunc(name, func(op string, args []interface{}) (string, error) {
		return fn(op, args, dialect)
	})
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

// Transpile converts a JSON Logic string to a SQL WHERE clause.
func (t *Transpiler) Transpile(jsonLogic string) (string, error) {
	var logic interface{}
	if err := json.Unmarshal([]byte(jsonLogic), &logic); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}

	return t.parser.Parse(logic)
}

// TranspileFromMap converts a pre-parsed JSON Logic map to a SQL WHERE clause.
func (t *Transpiler) TranspileFromMap(logic map[string]interface{}) (string, error) {
	return t.parser.Parse(logic)
}

// TranspileFromInterface converts any JSON Logic interface{} to a SQL WHERE clause.
func (t *Transpiler) TranspileFromInterface(logic interface{}) (string, error) {
	return t.parser.Parse(logic)
}

// Convenience functions for direct usage without creating a Transpiler instance

// Transpile converts a JSON Logic string to a SQL WHERE clause.
// Dialect is required - use DialectBigQuery or DialectSpanner.
func Transpile(d Dialect, jsonLogic string) (string, error) {
	t, err := NewTranspiler(d)
	if err != nil {
		return "", err
	}
	return t.Transpile(jsonLogic)
}

// TranspileFromMap converts a pre-parsed JSON Logic map to a SQL WHERE clause.
// Dialect is required - use DialectBigQuery or DialectSpanner.
func TranspileFromMap(d Dialect, logic map[string]interface{}) (string, error) {
	t, err := NewTranspiler(d)
	if err != nil {
		return "", err
	}
	return t.TranspileFromMap(logic)
}

// TranspileFromInterface converts any JSON Logic interface{} to a SQL WHERE clause.
// Dialect is required - use DialectBigQuery or DialectSpanner.
func TranspileFromInterface(d Dialect, logic interface{}) (string, error) {
	t, err := NewTranspiler(d)
	if err != nil {
		return "", err
	}
	return t.TranspileFromInterface(logic)
}
