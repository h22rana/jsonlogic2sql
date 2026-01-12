package operators

// OperatorConfig holds shared configuration for all operators.
// By using a shared config object, all operators automatically see
// configuration changes without requiring individual SetSchema calls.
type OperatorConfig struct {
	Schema SchemaProvider
}

// NewOperatorConfig creates a new operator config with optional schema.
func NewOperatorConfig(schema SchemaProvider) *OperatorConfig {
	return &OperatorConfig{Schema: schema}
}

// HasSchema returns true if a schema is configured.
func (c *OperatorConfig) HasSchema() bool {
	return c != nil && c.Schema != nil
}
