package jsonlogic2sql

import (
	"fmt"
	"maps"
	"regexp"
	"strings"
	"sync"

	"github.com/h22rana/jsonlogic2sql/internal/operators"
)

// ExpressionKind identifies whether a custom operator argument/result is a
// general value expression or a boolean predicate expression.
type ExpressionKind = operators.ExpressionKind

const (
	// ExpressionKindValue identifies scalar or array SQL value expressions.
	ExpressionKindValue = operators.ExpressionKindValue
	// ExpressionKindPredicate identifies boolean SQL predicate expressions.
	ExpressionKindPredicate = operators.ExpressionKindPredicate
)

// ExpressionType carries coarse result type metadata for value expressions.
type ExpressionType = operators.ExpressionType

const (
	// ExpressionTypeUnknown means the value type is not known statically.
	ExpressionTypeUnknown = operators.ExpressionTypeUnknown
	// ExpressionTypeNull identifies SQL NULL values.
	ExpressionTypeNull = operators.ExpressionTypeNull
	// ExpressionTypeBoolean identifies boolean values.
	ExpressionTypeBoolean = operators.ExpressionTypeBoolean
	// ExpressionTypeString identifies string values.
	ExpressionTypeString = operators.ExpressionTypeString
	// ExpressionTypeNumber identifies numeric values.
	ExpressionTypeNumber = operators.ExpressionTypeNumber
	// ExpressionTypeArray identifies array values.
	ExpressionTypeArray = operators.ExpressionTypeArray
)

// OperatorArg is the typed SQL representation passed to custom operators.
type OperatorArg = operators.OperatorArg

// OperatorResult is the typed SQL representation returned by custom operators.
type OperatorResult = operators.OperatorResult

// PredicateSQL returns a custom-operator result usable in predicate contexts.
func PredicateSQL(sql string) OperatorResult {
	return operators.PredicateSQL(sql)
}

// ValueSQL returns a custom-operator result usable in value contexts.
func ValueSQL(sql string, typ ExpressionType) OperatorResult {
	return operators.ValueSQL(sql, typ)
}

// OperatorFunc is a function type for custom operator implementations.
// It receives the operator name and its arguments, and returns the SQL representation.
//
// Example:
//
//	lengthOp := func(operator string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
//	    if len(args) != 1 {
//	        return jsonlogic2sql.OperatorResult{}, fmt.Errorf("length requires exactly 1 argument")
//	    }
//	    // args[0] will be the SQL representation of the argument
//	    return jsonlogic2sql.ValueSQL(fmt.Sprintf("LENGTH(%s)", args[0].SQL), jsonlogic2sql.ExpressionTypeNumber), nil
//	}
type OperatorFunc func(operator string, args []OperatorArg) (OperatorResult, error)

// LegacyOperatorFunc is accepted temporarily by registration helpers so older
// tests and callers can be migrated incrementally. New code should use
// OperatorFunc and return PredicateSQL or ValueSQL explicitly.
type LegacyOperatorFunc func(operator string, args []interface{}) (string, error)

// DialectAwareOperatorFunc is a function type for dialect-aware custom operator implementations.
// It receives the operator name, arguments, and the target SQL dialect.
//
// Example:
//
//	nowOp := func(operator string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
//	    switch dialect {
//	    case DialectBigQuery:
//	        return ValueSQL("CURRENT_TIMESTAMP()", ExpressionTypeUnknown), nil
//	    case DialectSpanner:
//	        return ValueSQL("CURRENT_TIMESTAMP()", ExpressionTypeUnknown), nil
//	    default:
//	        return OperatorResult{}, fmt.Errorf("unsupported dialect: %s", dialect)
//	    }
//	}
type DialectAwareOperatorFunc func(operator string, args []OperatorArg, dialect Dialect) (OperatorResult, error)

// LegacyDialectAwareOperatorFunc is the pre-typed dialect-aware function shape.
type LegacyDialectAwareOperatorFunc func(operator string, args []interface{}, dialect Dialect) (string, error)

// OperatorHandler is an interface for custom operator implementations.
// Implement this interface for more complex operators that need state.
//
// Example:
//
//	type MyOperator struct {
//	    prefix string
//	}
//
//	func (m *MyOperator) ToSQL(operator string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
//	    return jsonlogic2sql.ValueSQL(fmt.Sprintf("%s_%s", m.prefix, args[0].SQL), jsonlogic2sql.ExpressionTypeUnknown), nil
//	}
type OperatorHandler interface {
	// ToSQL converts the operator and its arguments to SQL.
	// The args slice contains typed SQL representations of each argument.
	ToSQL(operator string, args []OperatorArg) (OperatorResult, error)
}

// LegacyOperatorHandler is the pre-typed custom handler shape.
type LegacyOperatorHandler interface {
	ToSQL(operator string, args []interface{}) (string, error)
}

// DialectAwareOperatorHandler is an interface for dialect-aware custom operator implementations.
// Implement this interface when your operator needs to generate different SQL for different dialects.
//
// Example:
//
//	type CurrentTimeOperator struct{}
//
//	func (c *CurrentTimeOperator) ToSQLWithDialect(operator string, args []jsonlogic2sql.OperatorArg, dialect Dialect) (jsonlogic2sql.OperatorResult, error) {
//	    switch dialect {
//	    case DialectBigQuery:
//	        return jsonlogic2sql.ValueSQL("CURRENT_TIMESTAMP()", jsonlogic2sql.ExpressionTypeUnknown), nil
//	    case DialectSpanner:
//	        return jsonlogic2sql.ValueSQL("CURRENT_TIMESTAMP()", jsonlogic2sql.ExpressionTypeUnknown), nil
//	    default:
//	        return jsonlogic2sql.OperatorResult{}, fmt.Errorf("unsupported dialect: %s", dialect)
//	    }
//	}
type DialectAwareOperatorHandler interface {
	// ToSQLWithDialect converts the operator and its arguments to SQL for the specified dialect.
	// The args slice contains typed SQL representations of each argument.
	ToSQLWithDialect(operator string, args []OperatorArg, dialect Dialect) (OperatorResult, error)
}

// LegacyDialectAwareOperatorHandler is the pre-typed dialect-aware handler shape.
type LegacyDialectAwareOperatorHandler interface {
	ToSQLWithDialect(operator string, args []interface{}, dialect Dialect) (string, error)
}

type legacyHandlerWrapper struct {
	handler LegacyOperatorHandler
}

func (w *legacyHandlerWrapper) ToSQL(operator string, args []OperatorArg) (OperatorResult, error) {
	return w.ToSQLInContext(operator, args, operators.ExpressionKindValue)
}

func (w *legacyHandlerWrapper) ToSQLInContext(operator string, args []OperatorArg, kind operators.ExpressionKind) (OperatorResult, error) {
	sql, err := w.handler.ToSQL(operator, legacyArgs(args))
	if err != nil {
		return OperatorResult{}, err
	}
	return legacyOperatorResultForKind(sql, kind), nil
}

// funcHandler wraps an OperatorFunc to implement OperatorHandler.
type funcHandler struct {
	fn any
}

func (f *funcHandler) ToSQL(operator string, args []OperatorArg) (OperatorResult, error) {
	return f.ToSQLInContext(operator, args, operators.ExpressionKindValue)
}

func (f *funcHandler) ToSQLInContext(operator string, args []OperatorArg, kind operators.ExpressionKind) (OperatorResult, error) {
	switch fn := f.fn.(type) {
	case OperatorFunc:
		return fn(operator, args)
	case func(string, []OperatorArg) (OperatorResult, error):
		return fn(operator, args)
	case LegacyOperatorFunc:
		legacyArgs := legacyArgs(args)
		sql, err := fn(operator, legacyArgs)
		if err != nil {
			return OperatorResult{}, err
		}
		return legacyOperatorResultForKind(sql, kind), nil
	case func(string, []interface{}) (string, error):
		legacyArgs := legacyArgs(args)
		sql, err := fn(operator, legacyArgs)
		if err != nil {
			return OperatorResult{}, err
		}
		return legacyOperatorResultForKind(sql, kind), nil
	default:
		return OperatorResult{}, fmt.Errorf("unsupported operator function type %T", f.fn)
	}
}

// dialectAwareFuncHandler wraps a DialectAwareOperatorFunc to implement both OperatorHandler and DialectAwareOperatorHandler.
type dialectAwareFuncHandler struct {
	fn any
}

// ToSQL implements OperatorHandler but returns an error indicating dialect is required.
// This allows the handler to be stored in the registry while being identifiable as dialect-aware.
func (d *dialectAwareFuncHandler) ToSQL(operator string, _ []OperatorArg) (OperatorResult, error) {
	return OperatorResult{}, fmt.Errorf("operator %s requires dialect - use ToSQLWithDialect instead", operator)
}

func (d *dialectAwareFuncHandler) ToSQLWithDialect(operator string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
	switch fn := d.fn.(type) {
	case DialectAwareOperatorFunc:
		return fn(operator, args, dialect)
	case func(string, []OperatorArg, Dialect) (OperatorResult, error):
		return fn(operator, args, dialect)
	case LegacyDialectAwareOperatorFunc:
		sql, err := fn(operator, legacyArgs(args), dialect)
		if err != nil {
			return OperatorResult{}, err
		}
		return legacyOperatorResultForKind(sql, operators.ExpressionKindValue), nil
	case func(string, []interface{}, Dialect) (string, error):
		sql, err := fn(operator, legacyArgs(args), dialect)
		if err != nil {
			return OperatorResult{}, err
		}
		return legacyOperatorResultForKind(sql, operators.ExpressionKindValue), nil
	default:
		return OperatorResult{}, fmt.Errorf("unsupported dialect-aware operator function type %T", d.fn)
	}
}

func legacyArgs(args []OperatorArg) []interface{} {
	out := make([]interface{}, len(args))
	for i, arg := range args {
		out[i] = arg.SQL
	}
	return out
}

func legacyOperatorResultForKind(sql string, kind operators.ExpressionKind) OperatorResult {
	if kind == operators.ExpressionKindPredicate {
		return PredicateSQL(sql)
	}
	return ValueSQL(sql, ExpressionTypeUnknown)
}

// dialectAwareHandlerWrapper wraps a DialectAwareOperatorHandler to implement OperatorHandler.
// It stores the dialect from the transpiler config and uses it when ToSQL is called.
type dialectAwareHandlerWrapper struct {
	handler any
	dialect Dialect
}

func (w *dialectAwareHandlerWrapper) ToSQL(operator string, args []OperatorArg) (OperatorResult, error) {
	return w.ToSQLInContext(operator, args, operators.ExpressionKindValue)
}

func (w *dialectAwareHandlerWrapper) ToSQLInContext(operator string, args []OperatorArg, kind operators.ExpressionKind) (OperatorResult, error) {
	switch handler := w.handler.(type) {
	case DialectAwareOperatorHandler:
		return handler.ToSQLWithDialect(operator, args, w.dialect)
	case LegacyDialectAwareOperatorHandler:
		sql, err := handler.ToSQLWithDialect(operator, legacyArgs(args), w.dialect)
		if err != nil {
			return OperatorResult{}, err
		}
		return legacyOperatorResultForKind(sql, kind), nil
	default:
		return OperatorResult{}, fmt.Errorf("unsupported dialect-aware operator handler type %T", w.handler)
	}
}

type boundDialectAwareFuncHandler struct {
	fn      any
	dialect Dialect
}

func (b *boundDialectAwareFuncHandler) ToSQL(operator string, args []OperatorArg) (OperatorResult, error) {
	return b.ToSQLInContext(operator, args, operators.ExpressionKindValue)
}

func (b *boundDialectAwareFuncHandler) ToSQLInContext(operator string, args []OperatorArg, kind operators.ExpressionKind) (OperatorResult, error) {
	switch typed := b.fn.(type) {
	case DialectAwareOperatorFunc:
		return typed(operator, args, b.dialect)
	case func(string, []OperatorArg, Dialect) (OperatorResult, error):
		return typed(operator, args, b.dialect)
	case LegacyDialectAwareOperatorFunc:
		sql, err := typed(operator, legacyArgs(args), b.dialect)
		if err != nil {
			return OperatorResult{}, err
		}
		return legacyOperatorResultForKind(sql, kind), nil
	case func(string, []interface{}, Dialect) (string, error):
		sql, err := typed(operator, legacyArgs(args), b.dialect)
		if err != nil {
			return OperatorResult{}, err
		}
		return legacyOperatorResultForKind(sql, kind), nil
	default:
		return OperatorResult{}, fmt.Errorf("unsupported dialect-aware operator function type %T", b.fn)
	}
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
func (r *OperatorRegistry) Register(operatorName string, handler any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch h := handler.(type) {
	case OperatorHandler:
		r.handlers[operatorName] = h
	case LegacyOperatorHandler:
		r.handlers[operatorName] = &legacyHandlerWrapper{handler: h}
	default:
		r.handlers[operatorName] = &funcHandler{fn: func(string, []OperatorArg) (OperatorResult, error) {
			return OperatorResult{}, fmt.Errorf("unsupported operator handler type %T", handler)
		}}
	}
}

// RegisterFunc adds a custom operator function to the registry.
// This is a convenience method for simple operators that don't need state.
//
// Example:
//
//	registry := NewOperatorRegistry()
//	registry.RegisterFunc("length", func(op string, args []OperatorArg) (OperatorResult, error) {
//	    return ValueSQL(fmt.Sprintf("LENGTH(%s)", args[0].SQL), ExpressionTypeNumber), nil
//	})
func (r *OperatorRegistry) RegisterFunc(operatorName string, fn any) {
	r.Register(operatorName, &funcHandler{fn: fn})
}

// RegisterDialectAwareFunc adds a dialect-aware custom operator function to the registry.
// Use this for operators that need to generate different SQL based on the target dialect.
//
// Example:
//
//	registry := NewOperatorRegistry()
//	registry.RegisterDialectAwareFunc("now", func(op string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
//	    return ValueSQL("CURRENT_TIMESTAMP()", ExpressionTypeUnknown), nil
//	})
func (r *OperatorRegistry) RegisterDialectAwareFunc(operatorName string, fn any) {
	r.Register(operatorName, &dialectAwareFuncHandler{fn: fn})
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

var validOperatorNameRe = regexp.MustCompile(`^!?[a-zA-Z_][a-zA-Z0-9_]*$`)

// validateOperatorName checks if an operator name is valid.
// Names must be non-empty, match ^!?[a-zA-Z_][a-zA-Z0-9_]*$ (optional !
// prefix for negation operators like !contains), and not conflict with
// built-in operators.
func validateOperatorName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("operator name must not be empty")
	}
	if !validOperatorNameRe.MatchString(name) {
		return fmt.Errorf("operator name %q must match pattern !?[a-zA-Z_][a-zA-Z0-9_]*", name)
	}

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
