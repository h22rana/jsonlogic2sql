package operators

import (
	"fmt"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

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

// ValidateDialect checks if the configured dialect is supported.
// Returns an error for unsupported or unspecified dialects.
// This should be called by operators to ensure dialect compatibility.
func (c *OperatorConfig) ValidateDialect(operator string) error {
	d := c.GetDialect()
	switch d {
	case dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL:
		return nil // Supported dialects
	case dialect.DialectUnspecified:
		return fmt.Errorf("operator '%s': dialect not specified", operator)
	default:
		return fmt.Errorf("operator '%s' not supported for dialect: %s", operator, d)
	}
}

// IsBigQuery returns true if the dialect is BigQuery.
func (c *OperatorConfig) IsBigQuery() bool {
	return c.GetDialect() == dialect.DialectBigQuery
}

// IsSpanner returns true if the dialect is Spanner.
func (c *OperatorConfig) IsSpanner() bool {
	return c.GetDialect() == dialect.DialectSpanner
}

// IsPostgreSQL returns true if the dialect is PostgreSQL.
func (c *OperatorConfig) IsPostgreSQL() bool {
	return c.GetDialect() == dialect.DialectPostgreSQL
}
