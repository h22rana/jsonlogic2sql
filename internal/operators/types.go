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

func (a OperatorArg) String() string {
	return a.SQL
}

// OperatorResult is the typed SQL representation returned by custom operators
// and parser expression callbacks.
type OperatorResult struct {
	SQL               string
	Kind              ExpressionKind
	Type              ExpressionType
	EmptyArrayLiteral bool
	// ArrayElementType optionally carries the scalar element type when Type is
	// ExpressionTypeArray. ExpressionTypeUnknown means the element type is not
	// statically known.
	ArrayElementType ExpressionType
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

// ArrayValueSQL creates a value-expression result for array SQL and preserves
// the scalar element type when it is statically known. Use ExpressionTypeUnknown
// when the element type cannot be determined.
func ArrayValueSQL(sql string, elemType ExpressionType) OperatorResult {
	res := ValueSQL(sql, ExpressionTypeArray)
	res.ArrayElementType = elemType
	return res
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
	// FieldName is the original schema field name when IsField is true and the
	// source field is known. Array-scoped aliases may leave this empty.
	FieldName string
	// FieldNames carries every possible schema field represented by this SQL
	// value. Dynamic array sources can resolve the same scoped var against more
	// than one compatible array element schema.
	FieldNames []string
	// FieldHasDefault indicates the field came from a JSONLogic var with a
	// default value, such as {"var":["age", 18]}.
	FieldHasDefault bool
	// FieldDefaultLiteralKnown is true when FieldDefaultLiteral can be used for
	// schema-required equality folding and validation.
	FieldDefaultLiteralKnown bool
	// FieldDefaultLiteral is the raw JSONLogic default value when known.
	FieldDefaultLiteral interface{}
	// HasExpressionInfo indicates that Kind and Type carry parser-derived
	// expression metadata for this SQL value.
	HasExpressionInfo bool
	// Kind identifies whether Value is a value expression or predicate.
	Kind ExpressionKind
	// Type identifies the coarse SQL value type when known.
	Type ExpressionType
	// RequiresKnownTruthiness forces callers to reject truthiness checks when
	// Type is unknown instead of emitting mixed-type fallback SQL.
	RequiresKnownTruthiness bool
	// PreserveParamRefs marks SQL from a parameterized custom operator that did
	// not reference every placeholder allocated for its arguments. Parser folds
	// must keep those params so placeholder validation still reports dropped
	// custom-operator arguments.
	PreserveParamRefs bool
	// ArrayElementType optionally carries the scalar element type for
	// array-valued SQL expressions. ExpressionTypeUnknown means unknown.
	ArrayElementType ExpressionType
}

// SQLResult creates a ProcessedValue marked as SQL.
// Use this when returning generated SQL expressions from operators.
func SQLResult(sql string) ProcessedValue {
	return ProcessedValue{Value: sql, IsSQL: true}
}

// TypedSQLResult creates a ProcessedValue marked as SQL with expression
// metadata preserved from the parser.
func TypedSQLResult(sql string, kind ExpressionKind, typ ExpressionType) ProcessedValue {
	return ProcessedValue{
		Value:             sql,
		IsSQL:             true,
		HasExpressionInfo: true,
		Kind:              kind,
		Type:              typ,
	}
}

// SQLFieldResult creates a ProcessedValue marked as a SQL field operand.
func SQLFieldResult(sql string) ProcessedValue {
	return ProcessedValue{Value: sql, IsSQL: true, IsField: true}
}

// SchemaFieldNames returns every schema field represented by this processed
// value, falling back to FieldName for older single-scope values.
func (p ProcessedValue) SchemaFieldNames() []string {
	if len(p.FieldNames) > 0 {
		return p.FieldNames
	}
	if p.FieldName != "" {
		return []string{p.FieldName}
	}
	return nil
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
