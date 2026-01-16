package operators

import "github.com/h22rana/jsonlogic2sql/internal/dialect"

// OperatorConfig holds shared configuration for all operators.
// By using a shared config object, all operators automatically see
// configuration changes without requiring individual SetSchema calls.
type OperatorConfig struct {
	Schema  SchemaProvider
	Dialect dialect.Dialect
}

// NewOperatorConfig creates a new operator config with dialect and optional schema.
func NewOperatorConfig(d dialect.Dialect, schema SchemaProvider) *OperatorConfig {
	return &OperatorConfig{
		Dialect: d,
		Schema:  schema,
	}
}

// HasSchema returns true if a schema is configured.
func (c *OperatorConfig) HasSchema() bool {
	return c != nil && c.Schema != nil
}

// GetDialect returns the configured dialect.
func (c *OperatorConfig) GetDialect() dialect.Dialect {
	if c == nil {
		return dialect.DialectUnspecified
	}
	return c.Dialect
}
