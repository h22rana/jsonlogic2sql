package operators

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// handleReduce converts reduce operator to SQL.
// JSONLogic reduce: {"reduce": [array, reducer_expr, initial]}.
// The reducer expression uses "accumulator" and "current" variables.
//
// For common patterns, this generates optimized SQL:
// - Addition: initial + COALESCE((SELECT SUM(elem) FROM UNNEST(array) AS elem), 0).
// - Min/max: LEAST/GREATEST initial combined with MIN/MAX over UNNEST(array).
// ClickHouse uses arrayReduce for aggregate patterns and arrayFold for
// arbitrary reducer expressions. DuckDB uses list_reduce for arbitrary
// reducers. Other standard SQL dialects reject arbitrary reducer expressions
// that cannot be lowered to SUM, MIN, or MAX.
func (a *ArrayOperator) handleReduce(args []interface{}) (string, error) {
	res, err := a.handleReduceResult(args)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) handleReduceResult(args []interface{}) (OperatorResult, error) {
	if len(args) != reduceOperatorArgCount {
		return OperatorResult{}, fmt.Errorf("reduce requires exactly 3 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("reduce"); err != nil {
			return OperatorResult{}, err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return OperatorResult{}, err
	}

	// Third argument: initial value
	initialValue, err := a.valueToTypedSQLAtPath(args[arrayReduceInitialArgIndex], a.argPath(arrayReduceInitialArgIndex))
	if err != nil {
		return OperatorResult{}, fmt.Errorf("invalid reduce initial argument: %w", err)
	}
	initial := initialValue.sql
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return operatorResultFromTypedValue(initialValue), nil
	}

	// First argument: array
	arrayValue, err := a.valueToTypedSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return OperatorResult{}, fmt.Errorf("invalid reduce array argument: %w", err)
	}
	if arraySourceErr := a.validateArraySourceValue(arrayValue); arraySourceErr != nil {
		return OperatorResult{}, fmt.Errorf("invalid reduce array argument: %w", arraySourceErr)
	}
	if arrayValue.emptyArrayLiteral {
		return operatorResultFromTypedValue(initialValue), nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopesForValue(args[arraySourceArgIndex], arrayValue)
	if scopeErr := a.validateCompatibleArrayElementScopes(sourceScopes); scopeErr != nil {
		return OperatorResult{}, fmt.Errorf("invalid reduce array argument: %w", scopeErr)
	}

	// Second argument: reducer expression
	reducerExpr := args[arrayExpressionArgIndex]

	alias := a.elemAlias()

	// Check for common reduction patterns and optimize
	reduceScoped := a.withArrayLambdaSource(arrayLambdaScopeReduce, sourceScopes, arrayValue, false)
	if pattern := reduceScoped.detectAggregatePattern(reducerExpr); pattern != nil {
		numericInitial, initialErr := a.numericAggregateInitialSQL(args[arrayReduceInitialArgIndex], initialValue)
		if initialErr != nil {
			return OperatorResult{}, fmt.Errorf("invalid reduce initial argument: %w", initialErr)
		}
		if aggregateErr := reduceScoped.validateAggregatePattern(pattern); aggregateErr != nil {
			return OperatorResult{}, aggregateErr
		}
		// Generate optimized aggregate SQL based on dialect
		switch a.getDialect() {
		case dialect.DialectClickHouse:
			// ClickHouse: For field access, we need arrayMap first to extract the field
			aggregateInput := array
			if pattern.requiresArrayMap() {
				mapAlias := alias
				if !pattern.hasTermExpr {
					mapAlias = "x"
				}
				mappedRef, quoteErr := reduceScoped.aggregateElementSQL(mapAlias, pattern)
				if quoteErr != nil {
					if errors.Is(quoteErr, errUnsupportedGeneralReduce) {
						goto generalReduce
					}
					return OperatorResult{}, quoteErr
				}
				aggregateInput = fmt.Sprintf("arrayMap(%s -> %s, %s)", mapAlias, mappedRef, array)
			}
			aggregateSQL := fmt.Sprintf("arrayReduce('%s', %s)", strings.ToLower(pattern.function), aggregateInput)
			return ValueSQL(renderReduceAggregateResult(pattern.function, numericInitial, aggregateSQL, true, array), ExpressionTypeNumber), nil
		case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
			// Standard SQL: aggregate the array once and combine it with the
			// initial accumulator according to the reducer operator.
			elemRef, quoteErr := reduceScoped.aggregateElementSQL(alias, pattern)
			if quoteErr != nil {
				return OperatorResult{}, quoteErr
			}
			aggregateSQL := fmt.Sprintf("(SELECT %s(%s) FROM %s)", pattern.function, elemRef, a.unnestSourceSQL(array, alias))
			return ValueSQL(renderReduceAggregateResult(pattern.function, numericInitial, aggregateSQL, false, ""), ExpressionTypeNumber), nil
		}
		// Fallback for any future dialects
		elemRef, quoteErr := reduceScoped.aggregateElementSQL(alias, pattern)
		if quoteErr != nil {
			return OperatorResult{}, quoteErr
		}
		aggregateSQL := fmt.Sprintf("(SELECT %s(%s) FROM %s)", pattern.function, elemRef, a.unnestSourceSQL(array, alias))
		return ValueSQL(renderReduceAggregateResult(pattern.function, numericInitial, aggregateSQL, false, ""), ExpressionTypeNumber), nil
	}

generalReduce:
	accumulatorSQL := AccumulatorVar
	if a.getDialect() == dialect.DialectClickHouse || a.getDialect() == dialect.DialectDuckDB {
		accumulatorSQL = "acc"
	}
	valueScoped := reduceScoped.withReduceValueScope(initialValue, accumulatorSQL)
	reducerResult, err := valueScoped.valueExpressionResultWithContextAndPath(reducerExpr, true, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return OperatorResult{}, fmt.Errorf("invalid reduce expression: %w", err)
	}
	sql, err := a.renderGeneralReduceSQL(alias, array, reducerResult.SQL, initial, initialValue.typ)
	if err != nil {
		return OperatorResult{}, err
	}
	return operatorResultWithSQL(reducerResult, sql), nil
}

func operatorResultFromTypedValue(value typedValueSQL) OperatorResult {
	if value.typ == ExpressionTypeArray {
		res := arrayValueSQLWithMetadata(value.sql, typedValueElementTypes(value), typedValueSchemaScopes(value), typedValueElementSchemaType(value))
		res.EmptyArrayLiteral = value.emptyArrayLiteral
		res.PreserveParamRefs = value.preserveParamRefs
		return res
	}
	res := ValueSQL(value.sql, value.typ)
	res.SchemaType = value.schemaType
	res.PreserveParamRefs = value.preserveParamRefs
	return res
}

func operatorResultWithSQL(result OperatorResult, sql string) OperatorResult {
	result.SQL = sql
	return result
}

// aggregatePattern represents a detected aggregate pattern with optional field suffix.
type aggregatePattern struct {
	function     string // SQL aggregate function name (SUM, MIN, MAX)
	fieldSuffix  string // Optional field suffix (e.g., "price" for "current.price")
	defaultValue interface{}
	hasDefault   bool
	termExpr     interface{}
	hasTermExpr  bool
}

func (p *aggregatePattern) requiresArrayMap() bool {
	return p.fieldSuffix != "" || p.hasDefault || p.hasTermExpr
}

func (a *ArrayOperator) aggregateElementSQL(alias string, pattern *aggregatePattern) (string, error) {
	if pattern.hasTermExpr {
		result, err := a.aggregateTermResult(pattern.termExpr, a.argPath(arrayExpressionArgIndex))
		if err != nil {
			return "", err
		}
		sql, err := a.aggregateNumericTermSQL(result)
		if err != nil {
			return "", err
		}
		if containsWholeIdentifier(sql, CurrentVar) || containsWholeIdentifier(sql, AccumulatorVar) {
			return "", errUnsupportedGeneralReduce
		}
		return sql, nil
	}
	return a.aggregateElementRef(alias, pattern)
}

func (a *ArrayOperator) aggregateElementRef(alias string, pattern *aggregatePattern) (string, error) {
	elemType, err := a.aggregateElementType(pattern)
	if err != nil {
		return "", err
	}
	elemRef, err := a.quoteArrayScopePath(alias, pattern.fieldSuffix)
	if err != nil {
		return "", err
	}
	elemRef, err = numericAggregateElementSQL(elemRef, elemType)
	if err != nil {
		return "", err
	}
	if !pattern.hasDefault {
		return elemRef, nil
	}
	defaultSQL, err := a.numericAggregateDefaultSQL(pattern.defaultValue)
	if err != nil {
		return "", fmt.Errorf("invalid current default value: %w", err)
	}
	return a.config.CoalesceSQL(elemRef, defaultSQL), nil
}

func (a *ArrayOperator) aggregateElementRefParam(
	alias string,
	pattern *aggregatePattern,
	pc *params.ParamCollector,
) (string, error) {
	elemType, err := a.aggregateElementType(pattern)
	if err != nil {
		return "", err
	}
	elemRef, err := a.quoteArrayScopePath(alias, pattern.fieldSuffix)
	if err != nil {
		return "", err
	}
	elemRef, err = numericAggregateElementSQL(elemRef, elemType)
	if err != nil {
		return "", err
	}
	if !pattern.hasDefault {
		return elemRef, nil
	}
	defaultSQL, err := a.numericAggregateDefaultSQLParam(pattern.defaultValue, pc)
	if err != nil {
		return "", fmt.Errorf("invalid current default value: %w", err)
	}
	return a.config.CoalesceSQL(elemRef, defaultSQL), nil
}

func (a *ArrayOperator) validateAggregatePattern(pattern *aggregatePattern) error {
	if pattern.hasTermExpr {
		return nil
	}
	elemType, err := a.aggregateElementType(pattern)
	if err != nil {
		return err
	}
	if _, err := numericAggregateElementSQL(a.elemAlias(), elemType); err != nil {
		return err
	}
	if pattern.hasDefault {
		return validateNumericAggregateDefault(pattern.defaultValue)
	}
	return nil
}

func (a *ArrayOperator) aggregateElementType(pattern *aggregatePattern) (ExpressionType, error) {
	if pattern.fieldSuffix != "" {
		scopes := a.currentSchemaScopes()
		if len(scopes) == 0 {
			return ExpressionTypeUnknown, fmt.Errorf(
				"numeric reduce aggregate current field %q cannot be validated because the array element schema is unknown",
				pattern.fieldSuffix,
			)
		}
		fields, err := a.resolveFieldNamesInScopes(scopes, pattern.fieldSuffix)
		if err != nil {
			return ExpressionTypeUnknown, err
		}
		for _, field := range fields {
			fieldType := a.schema().GetFieldType(field)
			if fieldType == "" {
				continue
			}
			if !a.schema().IsNumericType(field) {
				return ExpressionTypeUnknown, fmt.Errorf(
					"numeric reduce aggregate requires numeric current field %q, got field %q of type %s",
					pattern.fieldSuffix,
					field,
					fieldType,
				)
			}
		}
		return ExpressionTypeNumber, nil
	}

	if a.hasObjectArraySchemaScope(a.currentSchemaScopes()) {
		return ExpressionTypeUnknown, fmt.Errorf(
			"numeric reduce aggregate over current requires scalar array elements or current.<numeric-field>, got object array scope %q",
			strings.Join(a.currentSchemaScopes(), ","),
		)
	}
	if a.hasElementType {
		return a.elementType, nil
	}
	return ExpressionTypeUnknown, fmt.Errorf(
		"numeric reduce aggregate over current requires a known numeric array element type; define elementType or use current.<numeric-field>",
	)
}

func (a *ArrayOperator) hasObjectArraySchemaScope(scopes []string) bool {
	provider, ok := a.schema().(ArrayElementSchemaProvider)
	if !ok {
		return false
	}
	for _, scope := range scopes {
		if provider.HasArrayElementFields(scope) {
			return true
		}
	}
	return false
}

func numericAggregateElementSQL(sql string, typ ExpressionType) (string, error) {
	switch typ {
	case ExpressionTypeUnknown, ExpressionTypeNumber:
		return sql, nil
	case ExpressionTypeBoolean:
		return BooleanValueNumberSQL(sql), nil
	case ExpressionTypeNull:
		return predicateNumberFalse, nil
	case ExpressionTypeString, ExpressionTypeArray, ExpressionTypeObject:
		return "", fmt.Errorf("numeric reduce aggregate requires numeric current value, got %s", expressionTypeName(typ))
	default:
		return "", fmt.Errorf("numeric reduce aggregate has unsupported current value type %s", expressionTypeName(typ))
	}
}

func (a *ArrayOperator) numericAggregateDefaultSQL(value interface{}) (string, error) {
	if err := validateNumericAggregateDefault(value); err != nil {
		return "", err
	}
	return a.numericOp.valueToSQL(value)
}

func (a *ArrayOperator) numericAggregateDefaultSQLParam(value interface{}, pc *params.ParamCollector) (string, error) {
	if err := validateNumericAggregateDefault(value); err != nil {
		return "", err
	}
	return a.numericOp.valueToSQLParam(value, pc)
}

func (a *ArrayOperator) numericAggregateInitialSQL(value interface{}, initial typedValueSQL) (string, error) {
	if str, ok := value.(string); ok && isNumericString(str) {
		return a.numericOp.valueToSQL(str)
	}
	switch initial.typ {
	case ExpressionTypeNumber:
		return initial.sql, nil
	case ExpressionTypeBoolean:
		return BooleanValueNumberSQL(initial.sql), nil
	case ExpressionTypeNull:
		return predicateNumberFalse, nil
	case ExpressionTypeUnknown:
		return initial.sql, nil
	case ExpressionTypeString:
		return "", fmt.Errorf("numeric reduce aggregate initial must be numeric, boolean, or null, got string")
	case ExpressionTypeArray:
		return "", fmt.Errorf("numeric reduce aggregate initial cannot be array-valued")
	case ExpressionTypeObject:
		return "", fmt.Errorf("numeric reduce aggregate initial cannot be object-valued")
	default:
		return "", fmt.Errorf("numeric reduce aggregate initial has unsupported type %s", expressionTypeName(initial.typ))
	}
}

func (a *ArrayOperator) numericAggregateInitialSQLParam(
	value interface{},
	initial typedValueSQL,
) (string, error) {
	if str, ok := value.(string); ok && isNumericString(str) {
		return fmt.Sprintf("CAST(%s AS NUMERIC)", initial.sql), nil
	}
	return a.numericAggregateInitialSQL(value, initial)
}

func validateNumericAggregateDefault(value interface{}) error {
	if str, ok := value.(string); ok {
		if isNumericString(str) {
			return nil
		}
		return fmt.Errorf("numeric reduce aggregate default must be numeric, boolean, or null, got string")
	}
	typ := inferLiteralValueExpressionType(value)
	switch typ {
	case ExpressionTypeNumber, ExpressionTypeBoolean, ExpressionTypeNull:
		return nil
	case ExpressionTypeUnknown, ExpressionTypeString, ExpressionTypeArray, ExpressionTypeObject:
		return fmt.Errorf(
			"numeric reduce aggregate default must be numeric, boolean, or null, got %s",
			expressionTypeName(typ),
		)
	default:
		return fmt.Errorf("numeric reduce aggregate default has unsupported type %s", expressionTypeName(typ))
	}
}

func isNumericString(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	if isIntegerLiteral(trimmed) {
		return true
	}
	num, err := strconv.ParseFloat(trimmed, 64)
	return err == nil && !math.IsNaN(num) && !math.IsInf(num, 0)
}

func (a *ArrayOperator) aggregateElementSQLParam(
	alias string,
	pattern *aggregatePattern,
	pc *params.ParamCollector,
) (string, error) {
	if pattern.hasTermExpr {
		result, err := a.aggregateTermResultParam(pattern.termExpr, pc, a.argPath(arrayExpressionArgIndex))
		if err != nil {
			return "", err
		}
		sql, err := a.aggregateNumericTermSQL(result)
		if err != nil {
			return "", err
		}
		if containsWholeIdentifier(sql, CurrentVar) || containsWholeIdentifier(sql, AccumulatorVar) {
			return "", errUnsupportedGeneralReduce
		}
		return sql, nil
	}
	return a.aggregateElementRefParam(alias, pattern, pc)
}

func (a *ArrayOperator) aggregateTermResult(expr interface{}, path string) (OperatorResult, error) {
	if isAggregatePredicateTerm(expr) {
		sql, err := a.predicateExpressionToSQLWithContextAndPath(expr, path)
		if err != nil {
			return OperatorResult{}, err
		}
		return PredicateSQL(sql), nil
	}
	return a.withValueSemantics(true).valueExpressionResultWithContextAndPath(expr, true, path)
}

func (a *ArrayOperator) aggregateTermResultParam(expr interface{}, pc *params.ParamCollector, path string) (OperatorResult, error) {
	if isAggregatePredicateTerm(expr) {
		rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(expr, false, path)
		if err != nil {
			return OperatorResult{}, err
		}
		res, err := a.config.ParsePredicateExpressionParam(rewritten, path, pc)
		if err != nil {
			return OperatorResult{}, err
		}
		out := PredicateSQL(res.SQL)
		out.PreserveParamRefs = res.PreserveParamRefs
		return out, nil
	}
	return a.withValueSemantics(true).valueExpressionResultParamWithContextAndPath(expr, pc, true, path)
}

func isAggregatePredicateTerm(expr interface{}) bool {
	operator, _, ok := arrayOperatorArgs(expr)
	if !ok {
		return false
	}
	switch operator {
	case "missing", "missing_some", "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in", "!", "!!", OpAll, OpSome, OpNone:
		return true
	default:
		return false
	}
}

func (a *ArrayOperator) aggregateNumericTermSQL(result OperatorResult) (string, error) {
	if result.Kind == ExpressionKindPredicate {
		return PredicateNumberSQL(result.SQL), nil
	}
	switch result.Type {
	case ExpressionTypeNull:
		return predicateNumberFalse, nil
	case ExpressionTypeBoolean:
		return BooleanValueNumberSQL(result.SQL), nil
	case ExpressionTypeNumber:
		return result.SQL, nil
	case ExpressionTypeString, ExpressionTypeUnknown:
		return a.numericOp.ToSQL(OpAdd, []interface{}{
			TypedSQLResult(result.SQL, result.Kind, result.Type),
		})
	case ExpressionTypeArray:
		return "", fmt.Errorf("numeric aggregate term cannot be array-valued")
	case ExpressionTypeObject:
		return "", fmt.Errorf("numeric aggregate term cannot be object-valued")
	default:
		return "", fmt.Errorf("numeric aggregate term has unsupported type %s", expressionTypeName(result.Type))
	}
}

func renderReduceAggregateResult(function, initial, aggregateSQL string, clickhouse bool, sourceArray string) string {
	coalesce := "COALESCE"
	least := "LEAST"
	greatest := "GREATEST"
	if clickhouse {
		coalesce = "coalesce"
		least = "least"
		greatest = "greatest"
	}

	switch function {
	case AggregateMIN:
		if clickhouse {
			return fmt.Sprintf("CASE WHEN length(%s) > 0 THEN %s(%s, %s(%s, %s)) ELSE %s END",
				sourceArray, least, initial, coalesce, aggregateSQL, initial, initial)
		}
		return fmt.Sprintf("%s(%s, %s(%s, %s))", least, initial, coalesce, aggregateSQL, initial)
	case AggregateMAX:
		if clickhouse {
			return fmt.Sprintf("CASE WHEN length(%s) > 0 THEN %s(%s, %s(%s, %s)) ELSE %s END",
				sourceArray, greatest, initial, coalesce, aggregateSQL, initial, initial)
		}
		return fmt.Sprintf("%s(%s, %s(%s, %s))", greatest, initial, coalesce, aggregateSQL, initial)
	default:
		return fmt.Sprintf("%s + %s(%s, 0)", initial, coalesce, aggregateSQL)
	}
}

// detectAggregatePattern checks if the reducer expression matches a common aggregate pattern.
// Returns the aggregate pattern if detected, nil otherwise.
func (a *ArrayOperator) detectAggregatePattern(expr interface{}) *aggregatePattern {
	exprMap, ok := expr.(map[string]interface{})
	if !ok || len(exprMap) != 1 {
		return nil
	}

	// Check for addition pattern: {"+": [{"var": "accumulator"}, {"var": "current"}]}
	// or {"+": [{"var": "accumulator"}, {"var": "current.price"}]}
	if args, hasPlus := exprMap[OpAdd]; hasPlus {
		if pattern, ok := a.isAccumulatorCurrentPattern(args); ok {
			pattern.function = AggregateSUM
			return pattern
		}
		if termExpr, ok := accumulatorIndependentTerm(args); ok {
			return &aggregatePattern{function: AggregateSUM, termExpr: termExpr, hasTermExpr: true}
		}
	}

	// Check for min pattern: {"min": [{"var": "accumulator"}, {"var": "current"}]}
	// or {"min": [{"var": "accumulator"}, {"var": "current.price"}]}
	if args, hasMin := exprMap[OpMin]; hasMin {
		if pattern, ok := a.isAccumulatorCurrentPattern(args); ok {
			pattern.function = AggregateMIN
			return pattern
		}
		if termExpr, ok := accumulatorIndependentTerm(args); ok {
			return &aggregatePattern{function: AggregateMIN, termExpr: termExpr, hasTermExpr: true}
		}
	}

	// Check for max pattern: {"max": [{"var": "accumulator"}, {"var": "current"}]}
	// or {"max": [{"var": "accumulator"}, {"var": "current.price"}]}
	if args, hasMax := exprMap[OpMax]; hasMax {
		if pattern, ok := a.isAccumulatorCurrentPattern(args); ok {
			pattern.function = AggregateMAX
			return pattern
		}
		if termExpr, ok := accumulatorIndependentTerm(args); ok {
			return &aggregatePattern{function: AggregateMAX, termExpr: termExpr, hasTermExpr: true}
		}
	}

	return nil
}

func accumulatorIndependentTerm(args interface{}) (interface{}, bool) {
	argsArr, ok := args.([]interface{})
	if !ok || len(argsArr) != 2 {
		return nil, false
	}
	if isBareAccumulatorVar(argsArr[0]) && !referencesReduceAccumulator(argsArr[1]) {
		return argsArr[1], true
	}
	if isBareAccumulatorVar(argsArr[1]) && !referencesReduceAccumulator(argsArr[0]) {
		return argsArr[0], true
	}
	return nil, false
}

func isBareAccumulatorVar(expr interface{}) bool {
	exprMap, ok := expr.(map[string]interface{})
	if !ok || len(exprMap) != 1 {
		return false
	}
	varExpr, ok := exprMap[OpVar]
	if !ok {
		return false
	}
	varName, ok := varExpr.(string)
	return ok && varName == AccumulatorVar
}

func referencesReduceAccumulator(expr interface{}) bool {
	switch v := expr.(type) {
	case map[string]interface{}:
		for op, raw := range v {
			if op == OpVar {
				return varExprReferencesAccumulator(raw)
			}
			if op == OpReduce {
				args, ok := raw.([]interface{})
				if !ok {
					return referencesReduceAccumulator(raw)
				}
				if len(args) > arraySourceArgIndex && referencesReduceAccumulator(args[arraySourceArgIndex]) {
					return true
				}
				if len(args) > arrayReduceInitialArgIndex && referencesReduceAccumulator(args[arrayReduceInitialArgIndex]) {
					return true
				}
				continue
			}
			if referencesReduceAccumulator(raw) {
				return true
			}
		}
	case []interface{}:
		for _, item := range v {
			if referencesReduceAccumulator(item) {
				return true
			}
		}
	}
	return false
}

func varExprReferencesAccumulator(varExpr interface{}) bool {
	switch v := varExpr.(type) {
	case string:
		return v == AccumulatorVar
	case []interface{}:
		if len(v) == 0 {
			return false
		}
		first, ok := v[0].(string)
		return ok && first == AccumulatorVar
	default:
		return false
	}
}

func containsWholeIdentifier(sql, ident string) bool {
	if ident == "" {
		return false
	}
	for i := 0; i < len(sql); i++ {
		switch sql[i] {
		case '\'':
			i = skipSQLQuotedRegion(sql, i, '\'')
			continue
		case '"':
			i = skipSQLQuotedRegion(sql, i, '"')
			continue
		case '`':
			i = skipSQLQuotedRegion(sql, i, '`')
			continue
		case '-':
			if i+1 < len(sql) && sql[i+1] == '-' {
				i = skipSQLLineComment(sql, i)
				continue
			}
		case '/':
			if i+1 < len(sql) && sql[i+1] == '*' {
				i = skipSQLBlockComment(sql, i)
				continue
			}
		case '$':
			if end, ok := skipSQLDollarQuotedRegion(sql, i); ok {
				i = end
				continue
			}
		}
		if wholeIdentifierAt(sql, ident, i) {
			return true
		}
	}
	return false
}

func wholeIdentifierAt(sql, ident string, start int) bool {
	end := start + len(ident)
	if end > len(sql) || sql[start:end] != ident {
		return false
	}
	if start > 0 && sql[start-1] == '.' {
		return false
	}
	return (start == 0 || !isSQLIdentifierByte(sql[start-1])) &&
		(end == len(sql) || !isSQLIdentifierByte(sql[end]))
}

func isSQLIdentifierByte(ch byte) bool {
	return ch == '_' || (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func skipSQLDollarQuotedRegion(sql string, start int) (int, bool) {
	delimiter, ok := sqlDollarQuoteDelimiterAt(sql, start)
	if !ok {
		return 0, false
	}
	contentStart := start + len(delimiter)
	closingOffset := strings.Index(sql[contentStart:], delimiter)
	if closingOffset < 0 {
		return len(sql) - 1, true
	}
	return contentStart + closingOffset + len(delimiter) - 1, true
}

func sqlDollarQuoteDelimiterAt(sql string, start int) (string, bool) {
	if start >= len(sql) || sql[start] != '$' || start+1 >= len(sql) {
		return "", false
	}
	if sql[start+1] == '$' {
		return "$$", true
	}
	if !isSQLDollarQuoteTagFirstChar(sql[start+1]) {
		return "", false
	}
	for i := start + 2; i < len(sql); i++ {
		if sql[i] == '$' {
			return sql[start : i+1], true
		}
		if !isSQLDollarQuoteTagChar(sql[i]) {
			return "", false
		}
	}
	return "", false
}

func isSQLDollarQuoteTagFirstChar(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isSQLDollarQuoteTagChar(ch byte) bool {
	return isSQLDollarQuoteTagFirstChar(ch) || (ch >= '0' && ch <= '9')
}

// isAccumulatorCurrentPattern checks if args match accumulator with current/current.field.
// It also accepts JSONLogic's defaulted var form, for example
// {"var":["current.price", 0]}, by aggregating COALESCE(elem.price, 0).
func (a *ArrayOperator) isAccumulatorCurrentPattern(args interface{}) (*aggregatePattern, bool) {
	argsArr, ok := args.([]interface{})
	if !ok || len(argsArr) != 2 {
		return nil, false
	}

	// Check first arg is {"var": "accumulator"}
	arg0Map, ok := argsArr[0].(map[string]interface{})
	if !ok || len(arg0Map) != 1 {
		return nil, false
	}
	if varName, hasVar := arg0Map[OpVar]; !hasVar || varName != AccumulatorVar {
		return nil, false
	}

	// Check second arg is {"var": "current"} or {"var": "current.field"}
	arg1Map, ok := argsArr[1].(map[string]interface{})
	if !ok || len(arg1Map) != 1 {
		return nil, false
	}
	varName, hasVar := arg1Map[OpVar]
	if !hasVar {
		return nil, false
	}

	var defaultValue interface{}
	var varNameStr string
	hasDefault := false
	switch v := varName.(type) {
	case string:
		varNameStr = v
	case []interface{}:
		if err := validateVarArrayMaxEntries(v); err != nil || len(v) == 0 {
			return nil, false
		}
		first, ok := v[0].(string)
		if !ok {
			return nil, false
		}
		varNameStr = first
		if len(v) > 1 {
			defaultValue = v[1]
			hasDefault = true
		}
	default:
		return nil, false
	}

	pattern := &aggregatePattern{defaultValue: defaultValue, hasDefault: hasDefault}

	// Check if it's exactly "current" or starts with "current."
	if varNameStr == CurrentVar {
		return pattern, true // Plain current, no field suffix
	}
	if strings.HasPrefix(varNameStr, CurrentVar+".") {
		// Extract field suffix (e.g., "price" from "current.price")
		fieldSuffix := strings.TrimPrefix(varNameStr, CurrentVar+".")
		if fieldSuffix == "" {
			return nil, false
		}
		if err := a.validateScopedFieldName(fieldSuffix); err != nil {
			return nil, false
		}
		pattern.fieldSuffix = fieldSuffix
		return pattern, true
	}

	return nil, false
}
