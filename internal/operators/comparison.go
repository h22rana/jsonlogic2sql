package operators

// ComparisonOperator handles comparison operators (==, ===, !=, !==, >, >=, <, <=).
type ComparisonOperator struct {
	config *OperatorConfig
	dataOp *DataOperator
}

type equalityDecision struct {
	handled     bool
	left        interface{}
	right       interface{}
	constant    *bool
	unsupported error
}

type equalityFieldOperand struct {
	fieldName           string
	hasDefault          bool
	defaultLiteral      interface{}
	defaultLiteralKnown bool
	processed           bool
}

type jsNumberLiteral struct {
	value    interface{}
	float    float64
	integral bool
}

// NewComparisonOperator creates a new comparison operator.
func NewComparisonOperator(config *OperatorConfig) *ComparisonOperator {
	config = normalizeOperatorConfig(config)
	return &ComparisonOperator{
		config: config,
		dataOp: NewDataOperator(config), // Same config, no propagation needed
	}
}

func (c *ComparisonOperator) schema() SchemaProvider {
	return schemaFromConfig(c.config)
}
