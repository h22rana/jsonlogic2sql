package parser

import (
	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/params"
	"github.com/h22rana/jsonlogic2sql/internal/validator"
)

// CustomOperatorHandler is an interface for custom operator implementations.
// This mirrors the public OperatorHandler interface.
type CustomOperatorHandler interface {
	ToSQL(operator string, args []operators.OperatorArg) (operators.OperatorResult, error)
}

// CustomOperatorLookup is a function type for looking up custom operators.
type CustomOperatorLookup func(operatorName string) (CustomOperatorHandler, bool)

// Parser parses JSON Logic expressions and converts them to SQL predicate and
// value expressions.
type Parser struct {
	validator      *validator.Validator
	config         *operators.OperatorConfig
	dataOp         *operators.DataOperator
	comparisonOp   *operators.ComparisonOperator
	logicalOp      *operators.LogicalOperator
	numericOp      *operators.NumericOperator
	stringOp       *operators.StringOperator
	arrayOp        *operators.ArrayOperator
	customOpLookup CustomOperatorLookup
}

// NewParser creates a new parser instance with config.
// If config is nil, defaults to BigQuery with an empty schema for internal
// literal-only usage.
func NewParser(config *operators.OperatorConfig) *Parser {
	if config == nil {
		config = operators.NewOperatorConfig(dialect.DialectBigQuery, nil)
	}
	p := &Parser{
		validator:    validator.NewValidator(),
		config:       config,
		dataOp:       operators.NewDataOperator(config),
		comparisonOp: operators.NewComparisonOperator(config),
		logicalOp:    operators.NewLogicalOperator(config),
		numericOp:    operators.NewNumericOperator(config),
		stringOp:     operators.NewStringOperator(config),
		arrayOp:      operators.NewArrayOperator(config),
	}

	// Set the expression parser callbacks so operators can delegate
	// nested expression parsing back to the parser (enabling custom operators)
	config.SetExpressionParser(func(expr any, path string) (string, error) {
		res, err := p.parseExpressionAny(expr, path)
		if err != nil {
			return "", err
		}
		return res.SQL, nil
	})
	config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
		res, err := p.parseExpressionAnyParam(expr, path, pc)
		if err != nil {
			return "", err
		}
		return res.SQL, nil
	})
	config.SetValueExpressionParser(func(expr any, path string) (operators.OperatorResult, error) {
		res, err := p.parseExpressionValue(expr, path)
		if err != nil {
			return operators.OperatorResult{}, err
		}
		return valueOperatorResult(res), nil
	})
	config.SetPredicateExpressionParser(func(expr any, path string) (operators.OperatorResult, error) {
		res, err := p.parseExpressionPredicate(expr, path)
		return res.OperatorResult, err
	})
	config.SetTruthinessExpressionParser(func(expr any, path string) (string, error) {
		_, condition, err := p.parseTruthinessResult(expr, path)
		return condition, err
	})
	config.SetParamValueExpressionParser(func(expr any, path string, pc *params.ParamCollector) (operators.OperatorResult, error) {
		res, err := p.parseExpressionValueParam(expr, path, pc)
		if err != nil {
			return operators.OperatorResult{}, err
		}
		return valueOperatorResult(res), nil
	})
	config.SetParamPredicateExpressionParser(func(expr any, path string, pc *params.ParamCollector) (operators.OperatorResult, error) {
		res, err := p.parseExpressionPredicateParam(expr, path, pc)
		return res.OperatorResult, err
	})
	config.SetParamTruthinessExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
		_, condition, err := p.parseTruthinessResultParam(expr, path, pc)
		return condition, err
	})
	config.SetValueTypeInferer(p.inferValueExpressionType)

	return p
}

// SetCustomOperatorLookup sets the function used to look up custom operators.
// This also sets up the validator to recognize custom operators.
func (p *Parser) SetCustomOperatorLookup(lookup CustomOperatorLookup) {
	p.customOpLookup = lookup
	// Also set up the validator to recognize custom operators
	p.validator.SetCustomOperatorChecker(func(operatorName string) bool {
		if lookup == nil {
			return false
		}
		_, ok := lookup(operatorName)
		return ok
	})
}

// SetSchema sets the schema provider for field validation and type checking.
func (p *Parser) SetSchema(schema operators.SchemaProvider) {
	p.config.SetSchema(schema)
	// All operators share the same config, so they automatically see the new schema
}

// Parse converts a JSON Logic expression to SQL using context inference.
func (p *Parser) Parse(logic interface{}) (string, error) {
	// First validate the expression
	if err := p.validator.Validate(logic); err != nil {
		return "", tperrors.NewValidationError(err)
	}
	if p.isPrimitive(logic) {
		return "", tperrors.NewPrimitiveNotAllowed("$")
	}
	if _, ok := logic.([]interface{}); ok {
		return "", tperrors.NewArrayNotAllowed("$")
	}

	res, err := p.parseExpressionAny(logic, "$")
	if err != nil {
		return "", err // TranspileError already contains full context
	}
	if err := p.rejectUnsupportedPostgreSQLEmptyArrayResult(res); err != nil {
		return "", err
	}

	return res.SQL, nil
}

// ParseCondition converts a JSON Logic expression to a SQL condition without the WHERE keyword.
// This is useful when you need to embed the condition in a larger query.
func (p *Parser) ParseCondition(logic interface{}) (string, error) {
	// First validate the expression
	if err := p.validator.Validate(logic); err != nil {
		return "", tperrors.NewValidationError(err)
	}

	res, err := p.parseExpressionPredicate(logic, "$")
	if err != nil {
		return "", err // TranspileError already contains full context
	}
	if err := p.rejectUnsupportedPostgreSQLEmptyArrayResult(res); err != nil {
		return "", err
	}

	// Return condition without WHERE prefix
	return res.SQL, nil
}

// ParseValue converts a JSON Logic expression to a SQL value expression.
func (p *Parser) ParseValue(logic interface{}) (string, error) {
	// Value mode accepts arrays as values, including empty arrays nested below
	// unary and logical operators, so parsing owns structural validation here.
	res, err := p.parseExpressionValue(logic, "$")
	if err != nil {
		return "", err
	}
	if err := p.rejectUnsupportedPostgreSQLEmptyArrayResult(res); err != nil {
		return "", err
	}
	return valueSQL(res), nil
}
