package jsonlogic2sql

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/parser"
)

// Re-export dialect constants for public API.
const (
	DialectBigQuery   = dialect.DialectBigQuery
	DialectSpanner    = dialect.DialectSpanner
	DialectPostgreSQL = dialect.DialectPostgreSQL
	DialectDuckDB     = dialect.DialectDuckDB
	DialectClickHouse = dialect.DialectClickHouse
)

// Dialect is the type for SQL dialect selection.
type Dialect = dialect.Dialect

// TranspilerConfig holds configuration options for the transpiler.
type TranspilerConfig struct {
	Dialect               Dialect // Required: target SQL dialect
	Schema                *Schema // Optional schema for field validation
	NullSafeFieldEquality bool    // Optional: match JSONLogic null equality for field-to-field equality
}

// Transpiler provides the main API for converting JSON Logic to SQL predicate
// and value expressions.
type Transpiler struct {
	parser          *parser.Parser
	config          *TranspilerConfig
	operatorConfig  *operators.OperatorConfig
	customOperators *OperatorRegistry
}

// schemaProvider returns a nil interface when schema is nil, avoiding typed-nil
// interface values that can bypass no-schema identifier validation.
func schemaProvider(schema *Schema) operators.SchemaProvider {
	if schema == nil {
		return nil
	}
	return schema
}

// SetSchema sets the schema for field validation and type checking
// This is optional - if not set, no schema validation will be performed.
func (t *Transpiler) SetSchema(schema *Schema) {
	t.operatorConfig.Schema = schemaProvider(schema)
	if t.config != nil {
		t.config.Schema = schema
	}
	// All operators automatically see the new schema through the shared config
}

// SetNullSafeFieldEquality enables or disables null-safe field-to-field equality.
// When enabled, equality operators comparing two var operands also match rows
// where both fields are NULL. The default is disabled.
func (t *Transpiler) SetNullSafeFieldEquality(enabled bool) {
	t.operatorConfig.NullSafeFieldEquality = enabled
	if t.config != nil {
		t.config.NullSafeFieldEquality = enabled
	}
}

// NewTranspiler creates a new transpiler instance with the specified dialect.
// Dialect is required - use DialectBigQuery, DialectSpanner, DialectPostgreSQL, or DialectDuckDB.
func NewTranspiler(d Dialect) (*Transpiler, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}

	opConfig := operators.NewOperatorConfig(d, nil)
	t := &Transpiler{
		parser:         parser.NewParser(opConfig),
		operatorConfig: opConfig,
		config: &TranspilerConfig{
			Dialect: d,
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

	opConfig := operators.NewOperatorConfig(config.Dialect, schemaProvider(config.Schema))
	opConfig.NullSafeFieldEquality = config.NullSafeFieldEquality
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
//	sql, _ := transpiler.TranspileValue(`{"length": [{"var": "email"}]}`)
//	// Output: LENGTH(email)
func (t *Transpiler) RegisterOperator(name string, handler any) error {
	if err := validateOperatorName(name); err != nil {
		return err
	}
	switch h := handler.(type) {
	case OperatorHandler:
		t.customOperators.Register(name, h)
	case LegacyOperatorHandler:
		t.customOperators.Register(name, &legacyHandlerWrapper{handler: h})
	default:
		return fmt.Errorf("unsupported operator handler type %T", handler)
	}
	return nil
}

// RegisterOperatorFunc registers a custom operator function.
// This is a convenience method for simple operators that don't need state.
// Returns an error if the operator name conflicts with a built-in operator.
//
// Example:
//
//	transpiler, _ := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery)
//	transpiler.RegisterOperatorFunc("length", func(op string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
//	    if len(args) != 1 {
//	        return jsonlogic2sql.OperatorResult{}, fmt.Errorf("length requires exactly 1 argument")
//	    }
//	    return jsonlogic2sql.ValueSQL(fmt.Sprintf("LENGTH(%s)", args[0].SQL), jsonlogic2sql.ExpressionTypeNumber), nil
//	})
//	sql, _ := transpiler.TranspileValue(`{"length": [{"var": "email"}]}`)
//	// Output: LENGTH(email)
func (t *Transpiler) RegisterOperatorFunc(name string, fn any) error {
	if err := validateOperatorName(name); err != nil {
		return err
	}
	if err := validateOperatorFunc(fn); err != nil {
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
//	sql, _ := transpiler.TranspileValue(`{"now": []}`)
//	// BigQuery: CURRENT_TIMESTAMP()
//	// Spanner: CURRENT_TIMESTAMP()
func (t *Transpiler) RegisterDialectAwareOperator(name string, handler any) error {
	if err := validateOperatorName(name); err != nil {
		return err
	}
	if err := validateDialectAwareOperatorHandler(handler); err != nil {
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
//	transpiler.RegisterDialectAwareOperatorFunc("now", func(op string, args []jsonlogic2sql.OperatorArg, dialect jsonlogic2sql.Dialect) (jsonlogic2sql.OperatorResult, error) {
//	    switch dialect {
//	    case jsonlogic2sql.DialectBigQuery:
//	        return jsonlogic2sql.ValueSQL("CURRENT_TIMESTAMP()", jsonlogic2sql.ExpressionTypeUnknown), nil
//	    case jsonlogic2sql.DialectSpanner:
//	        return jsonlogic2sql.ValueSQL("CURRENT_TIMESTAMP()", jsonlogic2sql.ExpressionTypeUnknown), nil
//	    default:
//	        return jsonlogic2sql.OperatorResult{}, fmt.Errorf("unsupported dialect: %s", dialect)
//	    }
//	})
func (t *Transpiler) RegisterDialectAwareOperatorFunc(name string, fn any) error {
	if err := validateOperatorName(name); err != nil {
		return err
	}
	if err := validateDialectAwareOperatorFunc(fn); err != nil {
		return err
	}
	t.customOperators.Register(name, &boundDialectAwareFuncHandler{fn: fn, dialect: t.config.Dialect})
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

// TranspileCondition converts a JSON Logic string to a SQL predicate expression.
func (t *Transpiler) TranspileCondition(jsonLogic string) (string, error) {
	logic, err := decodeJSONLogic(jsonLogic)
	if err != nil {
		return "", tperrors.NewInvalidJSON(err)
	}

	return t.parser.ParseCondition(logic)
}

// decodeJSONLogic decodes JSON using UseNumber so integer-like literals preserve
// precision (e.g. 9223372036854775808) instead of being coerced to float64.
func decodeJSONLogic(input string) (interface{}, error) {
	dec := json.NewDecoder(strings.NewReader(input))
	dec.UseNumber()

	var logic interface{}
	if err := dec.Decode(&logic); err != nil {
		return nil, err
	}

	// Reject trailing non-whitespace content.
	var extra interface{}
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("invalid JSON: trailing content")
		}
		return nil, err
	}

	return logic, nil
}

// TranspileConditionFromMap converts a pre-parsed JSON Logic map to a SQL condition without the WHERE keyword.
func (t *Transpiler) TranspileConditionFromMap(logic map[string]interface{}) (string, error) {
	return t.parser.ParseCondition(logic)
}

// TranspileConditionFromInterface converts any JSON Logic interface{} to a SQL condition without the WHERE keyword.
func (t *Transpiler) TranspileConditionFromInterface(logic interface{}) (string, error) {
	return t.parser.ParseCondition(logic)
}

// TranspileValue converts a JSON Logic string to a SQL value expression.
func (t *Transpiler) TranspileValue(jsonLogic string) (string, error) {
	logic, err := decodeJSONLogic(jsonLogic)
	if err != nil {
		return "", tperrors.NewInvalidJSON(err)
	}

	return t.parser.ParseValue(logic)
}

// TranspileValueFromMap converts a pre-parsed JSON Logic map to a SQL value expression.
func (t *Transpiler) TranspileValueFromMap(logic map[string]interface{}) (string, error) {
	return t.parser.ParseValue(logic)
}

// TranspileValueFromInterface converts any JSON Logic interface{} to a SQL value expression.
func (t *Transpiler) TranspileValueFromInterface(logic interface{}) (string, error) {
	return t.parser.ParseValue(logic)
}

// Convenience functions for direct usage without creating a Transpiler instance.

// TranspileCondition converts a JSON Logic string to a SQL condition without the WHERE keyword.
// Dialect is required - use DialectBigQuery, DialectSpanner, DialectPostgreSQL, or DialectDuckDB.
func TranspileCondition(d Dialect, jsonLogic string) (string, error) {
	t, err := NewTranspiler(d)
	if err != nil {
		return "", err
	}
	return t.TranspileCondition(jsonLogic)
}

// TranspileConditionFromMap converts a pre-parsed JSON Logic map to a SQL condition without the WHERE keyword.
// Dialect is required - use DialectBigQuery, DialectSpanner, DialectPostgreSQL, or DialectDuckDB.
func TranspileConditionFromMap(d Dialect, logic map[string]interface{}) (string, error) {
	t, err := NewTranspiler(d)
	if err != nil {
		return "", err
	}
	return t.TranspileConditionFromMap(logic)
}

// TranspileConditionFromInterface converts any JSON Logic interface{} to a SQL condition without the WHERE keyword.
// Dialect is required - use DialectBigQuery, DialectSpanner, DialectPostgreSQL, or DialectDuckDB.
func TranspileConditionFromInterface(d Dialect, logic interface{}) (string, error) {
	t, err := NewTranspiler(d)
	if err != nil {
		return "", err
	}
	return t.TranspileConditionFromInterface(logic)
}

// TranspileValue converts a JSON Logic string to a SQL value expression.
func TranspileValue(d Dialect, jsonLogic string) (string, error) {
	t, err := NewTranspiler(d)
	if err != nil {
		return "", err
	}
	return t.TranspileValue(jsonLogic)
}

// TranspileValueFromMap converts a pre-parsed JSON Logic map to a SQL value expression.
func TranspileValueFromMap(d Dialect, logic map[string]interface{}) (string, error) {
	t, err := NewTranspiler(d)
	if err != nil {
		return "", err
	}
	return t.TranspileValueFromMap(logic)
}

// TranspileValueFromInterface converts any JSON Logic interface{} to a SQL value expression.
func TranspileValueFromInterface(d Dialect, logic interface{}) (string, error) {
	t, err := NewTranspiler(d)
	if err != nil {
		return "", err
	}
	return t.TranspileValueFromInterface(logic)
}
