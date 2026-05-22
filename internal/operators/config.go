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

// TruthinessExpressionParser parses an expression in JSONLogic truthiness
// context and returns a SQL predicate.
type TruthinessExpressionParser func(expr any, path string) (string, error)

// ParamTruthinessExpressionParser is the parameterized variant of
// TruthinessExpressionParser.
type ParamTruthinessExpressionParser func(expr any, path string, pc *params.ParamCollector) (string, error)

// ValueTypeInferer returns the static value type for an expression when it can
// be inferred without generating SQL.
type ValueTypeInferer func(expr any, accumulatorType ExpressionType) ExpressionType

// OperatorConfig holds shared configuration for all operators.
// By using a shared config object, all operators automatically see
// configuration changes without requiring individual SetSchema calls.
type OperatorConfig struct {
	Schema                     SchemaProvider
	Dialect                    dialect.Dialect
	ExpressionParser           ExpressionParser
	ParamExpressionParser      ParamExpressionParser
	ValueExpressionParser      TypedExpressionParser
	PredicateExpressionParser  TypedExpressionParser
	TruthinessParser           TruthinessExpressionParser
	ParamValueExpressionParser ParamTypedExpressionParser
	ParamPredicateParser       ParamTypedExpressionParser
	ParamTruthinessParser      ParamTruthinessExpressionParser
	ValueTypeInferer           ValueTypeInferer
}

type emptySchemaProvider struct{}

func (emptySchemaProvider) HasField(_ string) bool { return false }

func (emptySchemaProvider) GetFieldType(_ string) string { return "" }

func (emptySchemaProvider) ValidateField(fieldName string) error {
	return fmt.Errorf("field '%s' is not defined in schema", fieldName)
}

func (emptySchemaProvider) IsArrayType(_ string) bool { return false }

func (emptySchemaProvider) IsStringType(_ string) bool { return false }

func (emptySchemaProvider) IsNumericType(_ string) bool { return false }

func (emptySchemaProvider) IsBooleanType(_ string) bool { return false }

func (emptySchemaProvider) IsEnumType(_ string) bool { return false }

func (emptySchemaProvider) GetAllowedValues(_ string) []string { return nil }

func (emptySchemaProvider) ValidateEnumValue(_, _ string) error { return nil }

func (emptySchemaProvider) ResolveScopedField(scopePath, fieldName string) (string, error) {
	if scopePath == "" {
		return "", fmt.Errorf("field '%s' is not defined in schema", fieldName)
	}
	return "", fmt.Errorf("field '%s' is not defined in schema scope '%s'", fieldName, scopePath)
}

func normalizeSchemaProvider(schema SchemaProvider) SchemaProvider {
	if schema == nil {
		return emptySchemaProvider{}
	}
	return schema
}

func normalizeOperatorConfig(config *OperatorConfig) *OperatorConfig {
	if config == nil {
		return NewOperatorConfig(dialect.DialectUnspecified, nil)
	}
	config.Schema = normalizeSchemaProvider(config.Schema)
	return config
}

func schemaFromConfig(config *OperatorConfig) SchemaProvider {
	if config == nil {
		return emptySchemaProvider{}
	}
	return normalizeSchemaProvider(config.Schema)
}

// NewOperatorConfig creates a new operator config with dialect and schema provider.
func NewOperatorConfig(d dialect.Dialect, schema SchemaProvider) *OperatorConfig {
	return &OperatorConfig{
		Dialect: d,
		Schema:  normalizeSchemaProvider(schema),
	}
}

// SetSchema replaces the schema provider, normalizing nil to an empty
// schema that rejects field access.
func (c *OperatorConfig) SetSchema(schema SchemaProvider) {
	if c == nil {
		return
	}
	c.Schema = normalizeSchemaProvider(schema)
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

// ValidateArrayLiteralElementTypes rejects array literal shapes that the target
// dialect cannot represent directly.
func (c *OperatorConfig) ValidateArrayLiteralElementTypes(elementTypes []ExpressionType) error {
	if len(elementTypes) == 0 || elementTypes[0] != ExpressionTypeArray {
		return nil
	}
	switch c.GetDialect() {
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner:
		return fmt.Errorf("%s does not support array literals whose elements are arrays", c.GetDialect())
	case dialect.DialectPostgreSQL, dialect.DialectDuckDB, dialect.DialectClickHouse:
		return nil
	}
	return nil
}

// StringCast renders a dialect-specific cast to a SQL string type.
func (c *OperatorConfig) StringCast(expr string) string {
	switch c.GetDialect() {
	case dialect.DialectPostgreSQL:
		return fmt.Sprintf("CAST(%s AS TEXT)", expr)
	case dialect.DialectDuckDB:
		return fmt.Sprintf("CAST(%s AS VARCHAR)", expr)
	case dialect.DialectClickHouse:
		return fmt.Sprintf("toString(%s)", expr)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner:
		return fmt.Sprintf("CAST(%s AS STRING)", expr)
	}
	return fmt.Sprintf("CAST(%s AS STRING)", expr)
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

// SetTruthinessExpressionParser sets the callback for JSONLogic truthiness parsing.
func (c *OperatorConfig) SetTruthinessExpressionParser(parser TruthinessExpressionParser) {
	if c != nil {
		c.TruthinessParser = parser
	}
}

// HasTruthinessExpressionParser returns true if a truthiness parser is configured.
func (c *OperatorConfig) HasTruthinessExpressionParser() bool {
	return c != nil && c.TruthinessParser != nil
}

// ParseTruthinessExpression parses a nested expression in JSONLogic truthiness context.
func (c *OperatorConfig) ParseTruthinessExpression(expr any, path string) (string, error) {
	if !c.HasTruthinessExpressionParser() {
		return "", fmt.Errorf("truthiness expression parser not configured")
	}
	return c.TruthinessParser(expr, path)
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

// SetParamTruthinessExpressionParser sets the parameterized truthiness parser callback.
func (c *OperatorConfig) SetParamTruthinessExpressionParser(parser ParamTruthinessExpressionParser) {
	if c != nil {
		c.ParamTruthinessParser = parser
	}
}

// HasParamTruthinessExpressionParser returns true if a parameterized truthiness parser is configured.
func (c *OperatorConfig) HasParamTruthinessExpressionParser() bool {
	return c != nil && c.ParamTruthinessParser != nil
}

// ParseTruthinessExpressionParam parses a nested expression in parameterized truthiness context.
func (c *OperatorConfig) ParseTruthinessExpressionParam(expr any, path string, pc *params.ParamCollector) (string, error) {
	if !c.HasParamTruthinessExpressionParser() {
		return "", fmt.Errorf("parameterized truthiness expression parser not configured")
	}
	return c.ParamTruthinessParser(expr, path, pc)
}

// SetValueTypeInferer sets the callback used by operators that need parser
// type inference without recursively rendering SQL.
func (c *OperatorConfig) SetValueTypeInferer(inferer ValueTypeInferer) {
	if c != nil {
		c.ValueTypeInferer = inferer
	}
}

// HasValueTypeInferer returns true if a value-type inference callback is configured.
func (c *OperatorConfig) HasValueTypeInferer() bool {
	return c != nil && c.ValueTypeInferer != nil
}

// InferValueExpressionType infers a value expression type through the configured parser callback.
func (c *OperatorConfig) InferValueExpressionType(expr any, accumulatorType ExpressionType) ExpressionType {
	if !c.HasValueTypeInferer() {
		return ExpressionTypeUnknown
	}
	return c.ValueTypeInferer(expr, accumulatorType)
}
