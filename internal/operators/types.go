package operators

// ExpressionKind identifies whether a generated SQL expression is a general
// value expression or a boolean predicate expression.
type ExpressionKind int

const (
	// ExpressionKindValue identifies scalar or array SQL value expressions.
	ExpressionKindValue ExpressionKind = iota
	// ExpressionKindPredicate identifies boolean SQL predicate expressions.
	ExpressionKindPredicate
)

// ExpressionType carries coarse result type metadata for context validation.
type ExpressionType int

const (
	// ExpressionTypeUnknown means the value type is not known statically.
	ExpressionTypeUnknown ExpressionType = iota
	// ExpressionTypeNull identifies SQL NULL values.
	ExpressionTypeNull
	// ExpressionTypeBoolean identifies boolean values.
	ExpressionTypeBoolean
	// ExpressionTypeString identifies string values.
	ExpressionTypeString
	// ExpressionTypeNumber identifies numeric values.
	ExpressionTypeNumber
	// ExpressionTypeArray identifies array values.
	ExpressionTypeArray
)

// OperatorArg is the typed SQL representation passed to custom operators.
type OperatorArg struct {
	SQL  string
	Kind ExpressionKind
	Type ExpressionType
}

// OperatorResult is the typed SQL representation returned by custom operators
// and parser expression callbacks.
type OperatorResult struct {
	SQL  string
	Kind ExpressionKind
	Type ExpressionType
}

// PredicateSQL creates a custom-operator result that can be used in predicate
// contexts such as TranspileCondition and array filters.
func PredicateSQL(sql string) OperatorResult {
	return OperatorResult{SQL: sql, Kind: ExpressionKindPredicate, Type: ExpressionTypeBoolean}
}

// ValueSQL creates a custom-operator result for value-expression contexts.
func ValueSQL(sql string, typ ExpressionType) OperatorResult {
	return OperatorResult{SQL: sql, Kind: ExpressionKindValue, Type: typ}
}

// ProcessedValue represents a value that has been processed during transpilation.
// It carries metadata about whether the value is already SQL or a literal that needs quoting.
type ProcessedValue struct {
	// Value is the string representation (either SQL expression or literal value)
	Value string
	// IsSQL indicates whether Value is a pre-processed SQL expression (true)
	// or a literal value that may need quoting (false)
	IsSQL bool
	// IsField indicates that Value came from a var operand after scope rewriting.
	// This lets comparison operators preserve field-to-field semantics for
	// array-scoped vars without re-validating internal aliases as schema fields.
	IsField bool
}

// SQLResult creates a ProcessedValue marked as SQL.
// Use this when returning generated SQL expressions from operators.
func SQLResult(sql string) ProcessedValue {
	return ProcessedValue{Value: sql, IsSQL: true}
}

// SQLFieldResult creates a ProcessedValue marked as a SQL field operand.
func SQLFieldResult(sql string) ProcessedValue {
	return ProcessedValue{Value: sql, IsSQL: true, IsField: true}
}

// LiteralResult creates a ProcessedValue marked as a literal.
// Use this when returning literal values that may need quoting.
func LiteralResult(val string) ProcessedValue {
	return ProcessedValue{Value: val, IsSQL: false}
}

// String returns the value as a string.
func (p ProcessedValue) String() string {
	return p.Value
}

// IsEmpty returns true if the value is empty.
func (p ProcessedValue) IsEmpty() bool {
	return p.Value == ""
}
