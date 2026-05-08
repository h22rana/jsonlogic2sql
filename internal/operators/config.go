package operators

import (
	"fmt"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// ExpressionParser is a callback function type for parsing nested expressions.
// This allows operators to delegate expression parsing back to the parser,
// enabling support for custom operators in nested contexts.
// The path parameter is the JSONPath for error reporting.
type ExpressionParser func(expr any, path string) (string, error)

// ParamExpressionParser is the parameterized variant of ExpressionParser.
// It additionally receives a ParamCollector to register bind parameters.
type ParamExpressionParser func(expr any, path string, pc *params.ParamCollector) (string, error)

// TypedExpressionParser parses a nested expression and returns SQL plus coarse
// context/type metadata.
type TypedExpressionParser func(expr any, path string) (OperatorResult, error)

// ParamTypedExpressionParser is the parameterized variant of TypedExpressionParser.
type ParamTypedExpressionParser func(expr any, path string, pc *params.ParamCollector) (OperatorResult, error)

// OperatorConfig holds shared configuration for all operators.
// By using a shared config object, all operators automatically see
// configuration changes without requiring individual SetSchema calls.
type OperatorConfig struct {
	Schema                     SchemaProvider
	Dialect                    dialect.Dialect
	NullSafeFieldEquality      bool
	ExpressionParser           ExpressionParser
	ParamExpressionParser      ParamExpressionParser
	ValueExpressionParser      TypedExpressionParser
	PredicateExpressionParser  TypedExpressionParser
	ParamValueExpressionParser ParamTypedExpressionParser
	ParamPredicateParser       ParamTypedExpressionParser
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
	case dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB, dialect.DialectClickHouse:
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

// IsDuckDB returns true if the dialect is DuckDB.
func (c *OperatorConfig) IsDuckDB() bool {
	return c.GetDialect() == dialect.DialectDuckDB
}

// IsClickHouse returns true if the dialect is ClickHouse.
func (c *OperatorConfig) IsClickHouse() bool {
	return c.GetDialect() == dialect.DialectClickHouse
}

// ArrayLengthFunc returns the dialect-specific SQL function call for array length.
func (c *OperatorConfig) ArrayLengthFunc(expr string) string {
	switch c.GetDialect() {
	case dialect.DialectPostgreSQL:
		return fmt.Sprintf("CARDINALITY(%s)", expr)
	case dialect.DialectClickHouse, dialect.DialectDuckDB:
		return fmt.Sprintf("length(%s)", expr)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner:
		return fmt.Sprintf("ARRAY_LENGTH(%s)", expr)
	}
	return fmt.Sprintf("ARRAY_LENGTH(%s)", expr)
}

// ArrayLiteral renders a SQL array/list literal for the configured dialect.
func (c *OperatorConfig) ArrayLiteral(elements []string) (string, error) {
	body := strings.Join(elements, ", ")
	switch c.GetDialect() {
	case dialect.DialectPostgreSQL:
		return fmt.Sprintf("ARRAY[%s]", body), nil
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectDuckDB, dialect.DialectClickHouse:
		return fmt.Sprintf("[%s]", body), nil
	}
	return fmt.Sprintf("[%s]", body), nil
}

// SetExpressionParser sets the callback for parsing nested expressions.
// This should be called by the parser after all operators are created.
func (c *OperatorConfig) SetExpressionParser(parser ExpressionParser) {
	if c != nil {
		c.ExpressionParser = parser
	}
}

// HasExpressionParser returns true if an expression parser is configured.
func (c *OperatorConfig) HasExpressionParser() bool {
	return c != nil && c.ExpressionParser != nil
}

// ParseExpression parses a nested expression using the configured parser.
// Returns an error if no parser is configured.
func (c *OperatorConfig) ParseExpression(expr any, path string) (string, error) {
	if !c.HasExpressionParser() {
		return "", fmt.Errorf("expression parser not configured")
	}
	return c.ExpressionParser(expr, path)
}

// SetParamExpressionParser sets the callback for parsing nested expressions
// in the parameterized pipeline. Called once in NewParser.
func (c *OperatorConfig) SetParamExpressionParser(parser ParamExpressionParser) {
	if c != nil {
		c.ParamExpressionParser = parser
	}
}

// HasParamExpressionParser returns true if a parameterized expression parser is configured.
func (c *OperatorConfig) HasParamExpressionParser() bool {
	return c != nil && c.ParamExpressionParser != nil
}

// ParseExpressionParam parses a nested expression through the parameterized pipeline.
func (c *OperatorConfig) ParseExpressionParam(expr any, path string, pc *params.ParamCollector) (string, error) {
	if !c.HasParamExpressionParser() {
		return "", fmt.Errorf("parameterized expression parser not configured")
	}
	return c.ParamExpressionParser(expr, path, pc)
}

// SetValueExpressionParser sets the callback for value-expression parsing.
func (c *OperatorConfig) SetValueExpressionParser(parser TypedExpressionParser) {
	if c != nil {
		c.ValueExpressionParser = parser
	}
}

// HasValueExpressionParser returns true if a value parser is configured.
func (c *OperatorConfig) HasValueExpressionParser() bool {
	return c != nil && c.ValueExpressionParser != nil
}

// ParseValueExpression parses a nested expression as a value expression.
func (c *OperatorConfig) ParseValueExpression(expr any, path string) (OperatorResult, error) {
	if !c.HasValueExpressionParser() {
		return OperatorResult{}, fmt.Errorf("value expression parser not configured")
	}
	return c.ValueExpressionParser(expr, path)
}

// SetPredicateExpressionParser sets the callback for predicate-expression parsing.
func (c *OperatorConfig) SetPredicateExpressionParser(parser TypedExpressionParser) {
	if c != nil {
		c.PredicateExpressionParser = parser
	}
}

// HasPredicateExpressionParser returns true if a predicate parser is configured.
func (c *OperatorConfig) HasPredicateExpressionParser() bool {
	return c != nil && c.PredicateExpressionParser != nil
}

// ParsePredicateExpression parses a nested expression as a predicate.
func (c *OperatorConfig) ParsePredicateExpression(expr any, path string) (OperatorResult, error) {
	if !c.HasPredicateExpressionParser() {
		return OperatorResult{}, fmt.Errorf("predicate expression parser not configured")
	}
	return c.PredicateExpressionParser(expr, path)
}

// SetParamValueExpressionParser sets the parameterized value parser callback.
func (c *OperatorConfig) SetParamValueExpressionParser(parser ParamTypedExpressionParser) {
	if c != nil {
		c.ParamValueExpressionParser = parser
	}
}

// HasParamValueExpressionParser returns true if a parameterized value parser is configured.
func (c *OperatorConfig) HasParamValueExpressionParser() bool {
	return c != nil && c.ParamValueExpressionParser != nil
}

// ParseValueExpressionParam parses a nested value expression through the parameterized pipeline.
func (c *OperatorConfig) ParseValueExpressionParam(expr any, path string, pc *params.ParamCollector) (OperatorResult, error) {
	if !c.HasParamValueExpressionParser() {
		return OperatorResult{}, fmt.Errorf("parameterized value expression parser not configured")
	}
	return c.ParamValueExpressionParser(expr, path, pc)
}

// SetParamPredicateExpressionParser sets the parameterized predicate parser callback.
func (c *OperatorConfig) SetParamPredicateExpressionParser(parser ParamTypedExpressionParser) {
	if c != nil {
		c.ParamPredicateParser = parser
	}
}

// HasParamPredicateExpressionParser returns true if a parameterized predicate parser is configured.
func (c *OperatorConfig) HasParamPredicateExpressionParser() bool {
	return c != nil && c.ParamPredicateParser != nil
}

// ParsePredicateExpressionParam parses a nested predicate through the parameterized pipeline.
func (c *OperatorConfig) ParsePredicateExpressionParam(expr any, path string, pc *params.ParamCollector) (OperatorResult, error) {
	if !c.HasParamPredicateExpressionParser() {
		return OperatorResult{}, fmt.Errorf("parameterized predicate expression parser not configured")
	}
	return c.ParamPredicateParser(expr, path, pc)
}
