package operators

import (
	"errors"
	"fmt"
)

type arrayLambdaScope int

const (
	arrayLambdaScopeNone arrayLambdaScope = iota
	arrayLambdaScopeElement
	arrayLambdaScopeMap
	arrayLambdaScopeReduce
)

var (
	errUnsupportedGeneralReduce        = errors.New("general reduce expressions are only supported for DuckDB list_reduce and ClickHouse arrayFold; BigQuery, Spanner, and PostgreSQL support reduce only for accumulator/current SUM, MIN, and MAX patterns")
	errUnsupportedDuckDBReduceSubquery = errors.New("DuckDB list_reduce does not support subqueries inside lambda reducers")
)

// ArrayOperator handles array operations like map, filter, reduce, all, some, none, merge.
type ArrayOperator struct {
	config        *OperatorConfig
	dataOp        *DataOperator
	comparisonOp  *ComparisonOperator
	logicalOp     *LogicalOperator
	numericOp     *NumericOperator
	scopeDepth    int
	visibleElems  []string
	visibleScopes []string
	exprPath      string
	valueScope    bool
	lambdaScope   arrayLambdaScope
	schemaScope   string
	schemaScopes  []string
	// valueSemantics means the current expression position returns a JSONLogic
	// value, so and/or/if must preserve fallback values instead of boolean SQL.
	valueSemantics            bool
	accumulatorType           ExpressionType
	hasAccumulatorType        bool
	accumulatorSQL            string
	accumulatorElemTypes      []ExpressionType
	accumulatorElemSchemaType string
	accumulatorSchemaScopes   []string
	elementType               ExpressionType
	elementSchemaType         string
	elementNestedTypes        []ExpressionType
	elementNestedSchemaType   string
	elementArraySchemaScopes  []string
	hasElementType            bool
}

// NewArrayOperator creates a new ArrayOperator instance.
func NewArrayOperator(config *OperatorConfig) *ArrayOperator {
	config = normalizeOperatorConfig(config)
	return &ArrayOperator{
		config:         config,
		dataOp:         NewDataOperator(config),
		comparisonOp:   NewComparisonOperator(config),
		logicalOp:      nil, // Will be created lazily
		numericOp:      NewNumericOperator(config),
		scopeDepth:     0,
		visibleElems:   []string{ElemVar},
		visibleScopes:  []string{""},
		exprPath:       "$",
		valueScope:     false,
		lambdaScope:    arrayLambdaScopeNone,
		valueSemantics: false,
	}
}

// ToSQL converts an array operation to SQL.
func (a *ArrayOperator) ToSQL(operator string, args []interface{}) (string, error) {
	if len(args) == 0 && operator != OpMerge {
		return "", fmt.Errorf("array operator %s requires at least one argument", operator)
	}

	switch operator {
	case "map":
		return a.handleMap(args)
	case "filter":
		return a.handleFilter(args)
	case "reduce":
		return a.handleReduce(args)
	case "all":
		return a.handleAll(args)
	case "some":
		return a.handleSome(args)
	case "none":
		return a.handleNone(args)
	case "merge":
		return a.handleMerge(args)
	default:
		return "", fmt.Errorf("unsupported array operator: %s", operator)
	}
}

// ToSQLAtPath converts an array operation to SQL using the provided JSONPath
// as the operator context for nested expression error reporting.
func (a *ArrayOperator) ToSQLAtPath(operator string, args []interface{}, path string) (string, error) {
	scoped := a.withPath(path)
	if scoped.shouldUseRenderedSourceChildScope(operator, args) {
		return scoped.withChildScope().ToSQL(operator, args)
	}
	return scoped.ToSQL(operator, args)
}

// ToValueResultAtPath converts a value-producing array operation to typed SQL
// using the provided JSONPath for nested expression error reporting.
func (a *ArrayOperator) ToValueResultAtPath(operator string, args []interface{}, path string) (OperatorResult, error) {
	scoped := a.withPath(path)
	if scoped.shouldUseRenderedSourceChildScope(operator, args) {
		return scoped.withChildScope().ToValueResult(operator, args)
	}
	return scoped.ToValueResult(operator, args)
}

// ToValueResult converts value-producing array operations to typed SQL while
// preserving array element metadata for downstream array expressions.
func (a *ArrayOperator) ToValueResult(operator string, args []interface{}) (OperatorResult, error) {
	if len(args) == 0 && operator != OpMerge {
		return OperatorResult{}, fmt.Errorf("array operator %s requires at least one argument", operator)
	}

	switch operator {
	case OpMap:
		return a.handleMapResult(args)
	case OpFilter:
		return a.handleFilterResult(args)
	case OpReduce:
		return a.handleReduceResult(args)
	case OpMerge:
		return a.handleMergeResult(args)
	default:
		return OperatorResult{}, fmt.Errorf("unsupported value array operator: %s", operator)
	}
}

func emptyArrayResult(sql string, err error) (OperatorResult, error) {
	if err != nil {
		return OperatorResult{}, err
	}
	res := ArrayValueSQL(sql, ExpressionTypeUnknown)
	res.EmptyArrayLiteral = true
	return res, nil
}
