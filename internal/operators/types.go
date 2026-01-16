package operators

// ProcessedValue represents a value that has been processed during transpilation.
// It carries metadata about whether the value is already SQL or a literal that needs quoting.
type ProcessedValue struct {
	// Value is the string representation (either SQL expression or literal value)
	Value string
	// IsSQL indicates whether Value is a pre-processed SQL expression (true)
	// or a literal value that may need quoting (false)
	IsSQL bool
}

// SQLResult creates a ProcessedValue marked as SQL.
// Use this when returning generated SQL expressions from operators.
func SQLResult(sql string) ProcessedValue {
	return ProcessedValue{Value: sql, IsSQL: true}
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
