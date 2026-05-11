package operators

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// Patterns for replacing element references in pre-processed SQL strings.
// These match "item"/"current"/"accumulator" only when they appear as standalone
// identifiers (not as suffixes like "account.current" or substrings like "current_balance").
// The pattern requires either start-of-string or a non-word non-dot character before the
// keyword, and a word boundary after it. This prevents false positives on:
//   - "current_balance" (underscore is a word char, no \b before "current")
//   - "account.current" (dot before "current" is blocked by [^\w.])
//
// But correctly matches:
//   - standalone "current" → "elem"
//   - "current.field" → "elem.field" (dot AFTER is fine, matched by \b)
//   - "(current + 1)" → "(elem + 1)" (paren is non-word non-dot)
var (
	elementRefPathPattern = regexp.MustCompile(`(^|[^\w.])((?:` + regexp.QuoteMeta(ItemVar) + `|` + regexp.QuoteMeta(CurrentVar) + `)(?:\.[A-Za-z0-9_]+)*)\b`)
	accumulatorPattern    = regexp.MustCompile(`(^|[^\w.])` + regexp.QuoteMeta(AccumulatorVar) + `\b`)
)

// ArrayOperator handles array operations like map, filter, reduce, all, some, none, merge.
type ArrayOperator struct {
	config       *OperatorConfig
	dataOp       *DataOperator
	comparisonOp *ComparisonOperator
	logicalOp    *LogicalOperator
	numericOp    *NumericOperator
	scopeDepth   int
	visibleElems []string
	exprPath     string
	valueScope   bool
	// valueSemantics means the current expression position returns a JSONLogic
	// value, so and/or/if must preserve fallback values instead of boolean SQL.
	valueSemantics     bool
	accumulatorType    ExpressionType
	hasAccumulatorType bool
}

// NewArrayOperator creates a new ArrayOperator instance with optional config.
func NewArrayOperator(config *OperatorConfig) *ArrayOperator {
	return &ArrayOperator{
		config:         config,
		dataOp:         NewDataOperator(config),
		comparisonOp:   NewComparisonOperator(config),
		logicalOp:      nil, // Will be created lazily
		numericOp:      NewNumericOperator(config),
		scopeDepth:     0,
		visibleElems:   []string{ElemVar},
		exprPath:       "$",
		valueScope:     false,
		valueSemantics: false,
	}
}

func (a *ArrayOperator) elemAlias() string {
	if a == nil || a.scopeDepth == 0 {
		return ElemVar
	}
	return fmt.Sprintf("%s%d", ElemVar, a.scopeDepth)
}

func (a *ArrayOperator) withChildScope() *ArrayOperator {
	child := &ArrayOperator{
		config:             a.config,
		dataOp:             a.dataOp,
		comparisonOp:       a.comparisonOp,
		logicalOp:          a.logicalOp,
		numericOp:          a.numericOp,
		scopeDepth:         a.scopeDepth + 1,
		visibleElems:       append([]string{}, a.visibleElems...),
		exprPath:           a.exprPath,
		valueScope:         a.valueScope,
		valueSemantics:     a.valueSemantics,
		accumulatorType:    a.accumulatorType,
		hasAccumulatorType: a.hasAccumulatorType,
	}
	childAlias := child.elemAlias()
	child.visibleElems = append(child.visibleElems, childAlias)
	return child
}

func (a *ArrayOperator) withPath(path string) *ArrayOperator {
	if path == "" {
		path = "$"
	}
	child := &ArrayOperator{
		config:             a.config,
		dataOp:             a.dataOp,
		comparisonOp:       a.comparisonOp,
		logicalOp:          a.logicalOp,
		numericOp:          a.numericOp,
		scopeDepth:         a.scopeDepth,
		visibleElems:       append([]string{}, a.visibleElems...),
		exprPath:           path,
		valueScope:         a.valueScope,
		valueSemantics:     a.valueSemantics,
		accumulatorType:    a.accumulatorType,
		hasAccumulatorType: a.hasAccumulatorType,
	}
	return child
}

func (a *ArrayOperator) withValueScope(enabled bool) *ArrayOperator {
	child := &ArrayOperator{
		config:             a.config,
		dataOp:             a.dataOp,
		comparisonOp:       a.comparisonOp,
		logicalOp:          a.logicalOp,
		numericOp:          a.numericOp,
		scopeDepth:         a.scopeDepth,
		visibleElems:       append([]string{}, a.visibleElems...),
		exprPath:           a.exprPath,
		valueScope:         enabled,
		valueSemantics:     a.valueSemantics,
		accumulatorType:    a.accumulatorType,
		hasAccumulatorType: a.hasAccumulatorType,
	}
	return child
}

func (a *ArrayOperator) withValueSemantics(enabled bool) *ArrayOperator {
	child := &ArrayOperator{
		config:             a.config,
		dataOp:             a.dataOp,
		comparisonOp:       a.comparisonOp,
		logicalOp:          a.logicalOp,
		numericOp:          a.numericOp,
		scopeDepth:         a.scopeDepth,
		visibleElems:       append([]string{}, a.visibleElems...),
		exprPath:           a.exprPath,
		valueScope:         a.valueScope,
		valueSemantics:     enabled,
		accumulatorType:    a.accumulatorType,
		hasAccumulatorType: a.hasAccumulatorType,
	}
	return child
}

func (a *ArrayOperator) withAccumulatorType(typ ExpressionType) *ArrayOperator {
	child := &ArrayOperator{
		config:             a.config,
		dataOp:             a.dataOp,
		comparisonOp:       a.comparisonOp,
		logicalOp:          a.logicalOp,
		numericOp:          a.numericOp,
		scopeDepth:         a.scopeDepth,
		visibleElems:       append([]string{}, a.visibleElems...),
		exprPath:           a.exprPath,
		valueScope:         a.valueScope,
		valueSemantics:     a.valueSemantics,
		accumulatorType:    typ,
		hasAccumulatorType: true,
	}
	return child
}

func (a *ArrayOperator) currentPath() string {
	if a == nil || a.exprPath == "" {
		return "$"
	}
	return a.exprPath
}

func (a *ArrayOperator) argPath(index int) string {
	return tperrors.BuildArrayPath(a.currentPath(), index)
}

func (a *ArrayOperator) inferValueExpressionType(expr interface{}, accumulatorType ExpressionType) ExpressionType {
	if a != nil && a.config != nil && a.config.HasValueTypeInferer() {
		return a.config.InferValueExpressionType(expr, accumulatorType)
	}
	return inferLiteralValueExpressionType(expr)
}

func inferLiteralValueExpressionType(expr interface{}) ExpressionType {
	if pv, ok := expr.(ProcessedValue); ok {
		if pv.HasExpressionInfo {
			if pv.Kind == ExpressionKindPredicate {
				return ExpressionTypeBoolean
			}
			return pv.Type
		}
		if pv.IsSQL {
			return ExpressionTypeUnknown
		}
		return inferLiteralValueExpressionType(pv.Value)
	}
	switch expr.(type) {
	case nil:
		return ExpressionTypeNull
	case bool:
		return ExpressionTypeBoolean
	case string:
		return ExpressionTypeString
	case json.Number, float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return ExpressionTypeNumber
	case []interface{}:
		return ExpressionTypeArray
	default:
		return ExpressionTypeUnknown
	}
}

func (a *ArrayOperator) accumulatorSQLResult() ProcessedValue {
	if a != nil && a.hasAccumulatorType && a.accumulatorType != ExpressionTypeUnknown {
		return TypedSQLResult(AccumulatorVar, ExpressionKindValue, a.accumulatorType)
	}
	result := TypedSQLResult(AccumulatorVar, ExpressionKindValue, ExpressionTypeUnknown)
	result.RequiresKnownTruthiness = true
	return result
}

func (a *ArrayOperator) isVisibleElemPath(name string) bool {
	for _, alias := range a.visibleElems {
		if name == alias || strings.HasPrefix(name, alias+".") {
			return true
		}
	}
	return false
}

func (a *ArrayOperator) shouldUseChildScope(op string, args []interface{}) bool {
	// Use a child element alias only for nested reduce when reduce arguments
	// reference the outer element alias. Other nested array operators keep the
	// historical alias behavior to preserve existing SQL output expectations.
	if op == OpReduce {
		if len(args) > 2 && a.referencesVisibleElemAlias(args[2]) {
			return true
		}
		if len(args) > 1 && len(args) > 0 && a.referencesVisibleElemAlias(args[0]) {
			plain, dotted := a.elementRefUsage(args[1])
			return plain && dotted
		}
		return false
	}
	if op == OpMap || op == OpFilter || op == OpAll || op == OpSome || op == OpNone {
		if len(args) > 1 && len(args) > 0 && a.referencesVisibleElemAlias(args[0]) {
			plain, dotted := a.elementRefUsage(args[1])
			return plain && dotted
		}
	}
	return false
}

func (a *ArrayOperator) referencesVisibleElemAlias(expr interface{}) bool {
	switch e := expr.(type) {
	case map[string]interface{}:
		if len(e) == 1 {
			if varName, hasVar := e[OpVar]; hasVar {
				switch v := varName.(type) {
				case string:
					return a.isVisibleElemPath(v)
				case []interface{}:
					if len(v) == 0 {
						return false
					}
					if s, ok := v[0].(string); ok {
						return a.isVisibleElemPath(s)
					}
					if pv, ok := v[0].(ProcessedValue); ok && pv.IsSQL {
						for _, alias := range a.visibleElems {
							if pv.Value == alias || strings.Contains(pv.Value, alias+".") {
								return true
							}
						}
					}
				}
			}
		}
		for _, v := range e {
			if a.referencesVisibleElemAlias(v) {
				return true
			}
		}
		return false
	case []interface{}:
		for _, v := range e {
			if a.referencesVisibleElemAlias(v) {
				return true
			}
		}
		return false
	case ProcessedValue:
		if e.IsSQL {
			for _, alias := range a.visibleElems {
				if strings.Contains(e.Value, alias+".") || e.Value == alias {
					return true
				}
			}
		}
		return false
	default:
		return false
	}
}

// elementRefUsage reports whether an expression contains plain item/current refs
// and dotted item./current. refs. Nested array lambdas are not traversed to avoid
// crossing scope boundaries.
func (a *ArrayOperator) elementRefUsage(expr interface{}) (plain, dotted bool) {
	switch e := expr.(type) {
	case map[string]interface{}:
		if len(e) == 1 {
			if varName, hasVar := e[OpVar]; hasVar {
				switch v := varName.(type) {
				case string:
					switch {
					case v == ItemVar || v == CurrentVar || v == "":
						plain = true
					case strings.HasPrefix(v, ItemVar+".") || strings.HasPrefix(v, CurrentVar+"."):
						dotted = true
					}
				case []interface{}:
					if len(v) > 0 {
						if s, ok := v[0].(string); ok {
							switch {
							case s == ItemVar || s == CurrentVar || s == "":
								plain = true
							case strings.HasPrefix(s, ItemVar+".") || strings.HasPrefix(s, CurrentVar+"."):
								dotted = true
							}
						}
					}
				}
				return plain, dotted
			}
			for opName, opArgs := range e {
				if a.isArrayOperator(opName) {
					arr, ok := opArgs.([]interface{})
					if !ok {
						return false, false
					}
					if len(arr) > 0 {
						p, d := a.elementRefUsage(arr[0])
						plain = plain || p
						dotted = dotted || d
					}
					if opName == OpReduce && len(arr) > 2 {
						p, d := a.elementRefUsage(arr[2])
						plain = plain || p
						dotted = dotted || d
					}
					return plain, dotted
				}
			}
		}
		for _, v := range e {
			p, d := a.elementRefUsage(v)
			plain = plain || p
			dotted = dotted || d
		}
		return plain, dotted
	case []interface{}:
		for _, v := range e {
			p, d := a.elementRefUsage(v)
			plain = plain || p
			dotted = dotted || d
		}
		return plain, dotted
	default:
		return false, false
	}
}

func (a *ArrayOperator) rewriteOuterDottedForNested(op string, args []interface{}, outerAlias string) []interface{} {
	if op != OpMap && op != OpFilter && op != OpAll && op != OpSome && op != OpNone && op != OpReduce {
		return args
	}
	if len(args) < 2 {
		return args
	}
	newArgs := make([]interface{}, len(args))
	copy(newArgs, args)
	newArgs[1] = a.rewriteOuterDottedElementRefs(args[1], outerAlias)
	return newArgs
}

// rewriteOuterDottedElementRefs rewrites dotted item references to an explicit
// outer alias (e.g. item.base -> elem.base), while leaving current/current.*
// untouched so inner-scope current semantics remain intact.
// Nested array lambdas are not rewritten (only their source/initial outer-scope args).
func (a *ArrayOperator) rewriteOuterDottedElementRefs(expr interface{}, outerAlias string) interface{} {
	switch e := expr.(type) {
	case map[string]interface{}:
		if len(e) == 1 {
			if varName, hasVar := e[OpVar]; hasVar {
				if varStr, ok := varName.(string); ok {
					switch {
					case strings.HasPrefix(varStr, ItemVar+"."):
						return map[string]interface{}{OpVar: outerAlias + varStr[len(ItemVar):]}
					default:
						return e
					}
				}
				if varArr, ok := varName.([]interface{}); ok && len(varArr) > 0 {
					if varStr, ok := varArr[0].(string); ok {
						switch {
						case strings.HasPrefix(varStr, ItemVar+"."):
							newArr := make([]interface{}, len(varArr))
							copy(newArr, varArr)
							newArr[0] = outerAlias + varStr[len(ItemVar):]
							return map[string]interface{}{OpVar: newArr}
						default:
							return e
						}
					}
				}
				return e
			}
			for opName, opArgs := range e {
				if a.isArrayOperator(opName) {
					if arr, ok := opArgs.([]interface{}); ok {
						newArgs := make([]interface{}, len(arr))
						copy(newArgs, arr)
						if len(newArgs) > 0 {
							newArgs[0] = a.rewriteOuterDottedElementRefs(arr[0], outerAlias)
						}
						if opName == OpReduce && len(newArgs) > 2 {
							newArgs[2] = a.rewriteOuterDottedElementRefs(arr[2], outerAlias)
						}
						return map[string]interface{}{opName: newArgs}
					}
				}
			}
			for opName, opArgs := range e {
				return map[string]interface{}{opName: a.rewriteOuterDottedElementRefs(opArgs, outerAlias)}
			}
		}
		result := make(map[string]interface{}, len(e))
		for k, v := range e {
			result[k] = a.rewriteOuterDottedElementRefs(v, outerAlias)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(e))
		for i, v := range e {
			result[i] = a.rewriteOuterDottedElementRefs(v, outerAlias)
		}
		return result
	default:
		return expr
	}
}

// schema returns the schema from config, or nil if not configured.
func (a *ArrayOperator) schema() SchemaProvider {
	if a.config == nil {
		return nil
	}
	return a.config.Schema
}

// getDialect returns the configured dialect, or DialectUnspecified if not configured.
func (a *ArrayOperator) getDialect() dialect.Dialect {
	if a.config == nil {
		return dialect.DialectUnspecified
	}
	return a.config.GetDialect()
}

// getLogicalOperator returns the logical operator, creating it lazily if needed.
func (a *ArrayOperator) getLogicalOperator() *LogicalOperator {
	if a.logicalOp == nil {
		a.logicalOp = NewLogicalOperator(a.config) // Config already has schema
	}
	return a.logicalOp
}

// validateArrayOperand checks if a field used in an array operation is of array type.
func (a *ArrayOperator) validateArrayOperand(value interface{}) error {
	if a.schema() == nil {
		return nil // No schema, no validation
	}

	// If it's a literal array, it's valid
	if _, ok := value.([]interface{}); ok {
		return nil
	}

	fieldName := a.extractFieldNameFromValue(value)
	if fieldName == "" {
		return nil // Can't determine field name, skip validation
	}

	fieldType := a.schema().GetFieldType(fieldName)
	if fieldType == "" {
		return nil // Field not in schema, skip validation (existence checked by DataOperator)
	}

	if !a.schema().IsArrayType(fieldName) {
		return fmt.Errorf("array operation on non-array field '%s' (type: %s)", fieldName, fieldType)
	}

	return nil
}

// extractFieldNameFromValue extracts field name from a value that might be a var expression.
func (a *ArrayOperator) extractFieldNameFromValue(value interface{}) string {
	if varExpr, ok := value.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			return a.extractFieldName(varName)
		}
	}
	return ""
}

// extractFieldName extracts the field name from a var argument.
func (a *ArrayOperator) extractFieldName(varName interface{}) string {
	if nameStr, ok := varName.(string); ok {
		return nameStr
	}
	if nameArr, ok := varName.([]interface{}); ok && len(nameArr) > 0 {
		if nameStr, ok := nameArr[0].(string); ok {
			return nameStr
		}
	}
	return ""
}

// ToSQL converts an array operation to SQL.
func (a *ArrayOperator) ToSQL(operator string, args []interface{}) (string, error) {
	if len(args) == 0 {
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
		nestedArgs := scoped.rewriteOuterDottedForNested(operator, args, scoped.elemAlias())
		return scoped.withChildScope().ToSQL(operator, nestedArgs)
	}
	return scoped.ToSQL(operator, args)
}

func (a *ArrayOperator) shouldUseRenderedSourceChildScope(op string, args []interface{}) bool {
	if len(args) == 0 || !a.shouldUseChildScope(op, args) {
		return false
	}
	return isRenderedArrayScopeSource(args[0])
}

func isRenderedArrayScopeSource(value interface{}) bool {
	switch v := value.(type) {
	case ProcessedValue:
		return v.IsSQL && v.IsField
	case map[string]interface{}:
		if len(v) != 1 {
			return false
		}
		varName, ok := v[OpVar]
		if !ok {
			return false
		}
		arr, ok := varName.([]interface{})
		if !ok || len(arr) == 0 {
			return false
		}
		pv, ok := arr[0].(ProcessedValue)
		return ok && pv.IsSQL && pv.IsField
	default:
		return false
	}
}

// handleMap converts map operator to SQL.
// Generates: ARRAY(SELECT transformation FROM UNNEST(array) AS elem).
// For ClickHouse: Uses arrayMap or subquery with arrayJoin.
func (a *ArrayOperator) handleMap(args []interface{}) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("map requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("map"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return a.emptyArrayLiteralSQL()
	}

	// First argument: array
	array, err := a.valueToSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid map array argument: %w", err)
	}

	valueScoped := a.withValueSemantics(true)
	transformation, err := valueScoped.valueExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], false, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid map transformation argument: %w", err)
	}
	transformation = a.replaceElementRefsInSQL(transformation)

	alias := a.elemAlias()
	return a.renderMapSQL(alias, transformation, array), nil
}

// handleFilter converts filter operator to SQL.
// Generates: ARRAY(SELECT elem FROM UNNEST(array) AS elem WHERE condition).
// For ClickHouse: Uses arrayFilter function.
func (a *ArrayOperator) handleFilter(args []interface{}) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("filter requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("filter"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return a.emptyArrayLiteralSQL()
	}

	// First argument: array
	array, err := a.valueToSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid filter array argument: %w", err)
	}

	// Second argument: condition expression - rewrite element vars before SQL generation
	condition, err := a.predicateExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid filter condition argument: %w", err)
	}
	condition = a.replaceElementRefsInSQL(condition)

	alias := a.elemAlias()
	return a.renderFilterSQL(alias, array, condition), nil
}

// handleReduce converts reduce operator to SQL.
// JSONLogic reduce: {"reduce": [array, reducer_expr, initial]}.
// The reducer expression uses "accumulator" and "current" variables.
//
// For common patterns, this generates optimized SQL:
// - Addition: initial + COALESCE((SELECT SUM(elem) FROM UNNEST(array) AS elem), 0).
// - General: (SELECT reducer FROM UNNEST(array) AS elem).
// For ClickHouse: Uses arrayReduce function for aggregates.
func (a *ArrayOperator) handleReduce(args []interface{}) (string, error) {
	if len(args) != reduceOperatorArgCount {
		return "", fmt.Errorf("reduce requires exactly 3 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("reduce"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}

	// Third argument: initial value
	initial, err := a.valueToSQLAtPath(args[arrayReduceInitialArgIndex], a.argPath(arrayReduceInitialArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce initial argument: %w", err)
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return initial, nil
	}

	// First argument: array
	array, err := a.valueToSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce array argument: %w", err)
	}

	// Second argument: reducer expression
	reducerExpr := args[arrayExpressionArgIndex]

	alias := a.elemAlias()

	// Check for common reduction patterns and optimize
	if pattern := a.detectAggregatePattern(reducerExpr); pattern != nil {
		// Build the element reference: "elem" or "elem.field" if field suffix exists
		elemRef, quoteErr := a.quoteArrayScopePath(alias, pattern.fieldSuffix)
		if quoteErr != nil {
			return "", quoteErr
		}

		// Generate optimized aggregate SQL based on dialect
		switch a.getDialect() {
		case dialect.DialectClickHouse:
			// ClickHouse: For field access, we need arrayMap first to extract the field
			if pattern.fieldSuffix != "" {
				mappedRef, quoteErr := a.quoteArrayScopePath("x", pattern.fieldSuffix)
				if quoteErr != nil {
					return "", quoteErr
				}
				// initial + coalesce(arrayReduce('sum', arrayMap(x -> x.field, array)), 0)
				return fmt.Sprintf("%s + coalesce(arrayReduce('%s', arrayMap(x -> %s, %s)), 0)",
					initial, strings.ToLower(pattern.function), mappedRef, array), nil
			}
			// initial + coalesce(arrayReduce('sum', array), 0)
			return fmt.Sprintf("%s + coalesce(arrayReduce('%s', %s), 0)",
				initial, strings.ToLower(pattern.function), array), nil
		case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
			// Standard SQL: initial + COALESCE((SELECT AGG(elem.field) FROM UNNEST(array) AS elem), 0)
			return fmt.Sprintf("%s + COALESCE((SELECT %s(%s) FROM UNNEST(%s) AS %s), 0)",
				initial, pattern.function, elemRef, array, alias), nil
		}
		// Fallback for any future dialects
		return fmt.Sprintf("%s + COALESCE((SELECT %s(%s) FROM UNNEST(%s) AS %s), 0)",
			initial, pattern.function, elemRef, array, alias), nil
	}

	// General case: rewrite element vars in the AST (item/current → elem),
	// then generate SQL, apply safety net for custom ops, and finally
	// substitute accumulator with the initial value. The order matters:
	// accumulator substitution must happen LAST so that the safety net
	// doesn't corrupt initial values containing "current"/"item" field names.
	rewritten := a.rewriteElementVars(reducerExpr)
	accumulatorType := a.inferValueExpressionType(args[arrayReduceInitialArgIndex], ExpressionTypeUnknown)
	valueScoped := a.withValueSemantics(true).withAccumulatorType(accumulatorType)
	reducerWithElem, err := valueScoped.expressionToSQLWithContextAndPath(rewritten, true, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce expression: %w", err)
	}
	reducerWithElem = a.replaceElementRefsInSQL(reducerWithElem)
	reducerWithElem = replaceWithLiteral(accumulatorPattern, reducerWithElem, initial)

	// Generate SQL based on dialect
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		// ClickHouse uses arrayFold for general reduction (ClickHouse 22.8+)
		return fmt.Sprintf("arrayFold((acc, %s) -> %s, %s, %s)", alias, reducerWithElem, array, initial), nil
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		// Standard SQL using a subquery
		return fmt.Sprintf("(SELECT %s FROM UNNEST(%s) AS %s)", reducerWithElem, array, alias), nil
	}
	return fmt.Sprintf("(SELECT %s FROM UNNEST(%s) AS %s)", reducerWithElem, array, alias), nil
}

// aggregatePattern represents a detected aggregate pattern with optional field suffix.
type aggregatePattern struct {
	function    string // SQL aggregate function name (SUM, MIN, MAX)
	fieldSuffix string // Optional field suffix (e.g., "price" for "current.price")
}

// detectAggregatePattern checks if the reducer expression matches a common aggregate pattern.
// Returns the aggregate pattern if detected, nil otherwise.
func (a *ArrayOperator) detectAggregatePattern(expr interface{}) *aggregatePattern {
	exprMap, ok := expr.(map[string]interface{})
	if !ok {
		return nil
	}

	// Check for addition pattern: {"+": [{"var": "accumulator"}, {"var": "current"}]}
	// or {"+": [{"var": "accumulator"}, {"var": "current.price"}]}
	if args, hasPlus := exprMap[OpAdd]; hasPlus {
		if fieldSuffix, ok := a.isAccumulatorCurrentPattern(args); ok {
			return &aggregatePattern{function: AggregateSUM, fieldSuffix: fieldSuffix}
		}
	}

	// Check for min pattern: {"min": [{"var": "accumulator"}, {"var": "current"}]}
	// or {"min": [{"var": "accumulator"}, {"var": "current.price"}]}
	if args, hasMin := exprMap[OpMin]; hasMin {
		if fieldSuffix, ok := a.isAccumulatorCurrentPattern(args); ok {
			return &aggregatePattern{function: AggregateMIN, fieldSuffix: fieldSuffix}
		}
	}

	// Check for max pattern: {"max": [{"var": "accumulator"}, {"var": "current"}]}
	// or {"max": [{"var": "accumulator"}, {"var": "current.price"}]}
	if args, hasMax := exprMap[OpMax]; hasMax {
		if fieldSuffix, ok := a.isAccumulatorCurrentPattern(args); ok {
			return &aggregatePattern{function: AggregateMAX, fieldSuffix: fieldSuffix}
		}
	}

	return nil
}

// isAccumulatorCurrentPattern checks if args match [{"var": "accumulator"}, {"var": "current"}]
// or [{"var": "accumulator"}, {"var": "current.field"}].
// Returns (fieldSuffix, true) if pattern matches, ("", false) otherwise.
// fieldSuffix is empty for plain "current", or contains the field path (e.g., "price" for "current.price").
func (a *ArrayOperator) isAccumulatorCurrentPattern(args interface{}) (string, bool) {
	argsArr, ok := args.([]interface{})
	if !ok || len(argsArr) != 2 {
		return "", false
	}

	// Check first arg is {"var": "accumulator"}
	arg0Map, ok := argsArr[0].(map[string]interface{})
	if !ok {
		return "", false
	}
	if varName, hasVar := arg0Map[OpVar]; !hasVar || varName != AccumulatorVar {
		return "", false
	}

	// Check second arg is {"var": "current"} or {"var": "current.field"}
	arg1Map, ok := argsArr[1].(map[string]interface{})
	if !ok {
		return "", false
	}
	varName, hasVar := arg1Map[OpVar]
	if !hasVar {
		return "", false
	}

	varNameStr, ok := varName.(string)
	if !ok {
		return "", false
	}

	// Check if it's exactly "current" or starts with "current."
	if varNameStr == CurrentVar {
		return "", true // Plain current, no field suffix
	}
	if strings.HasPrefix(varNameStr, CurrentVar+".") {
		// Extract field suffix (e.g., "price" from "current.price")
		fieldSuffix := strings.TrimPrefix(varNameStr, CurrentVar+".")
		return fieldSuffix, true
	}

	return "", false
}

// handleAll converts all operator to SQL.
// This checks if all elements in an array satisfy a condition.
// Generates: NOT EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE NOT (condition)).
// For ClickHouse: Uses arrayAll function.
func (a *ArrayOperator) handleAll(args []interface{}) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("all requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("all"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return "FALSE", nil
	}

	// First argument: array
	array, err := a.valueToSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all array argument: %w", err)
	}

	// Second argument: condition expression - rewrite element vars before SQL generation
	condition, err := a.predicateExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all condition argument: %w", err)
	}
	condition = a.replaceElementRefsInSQL(condition)

	// JSONLogic spec: {"all": [[], condition]} returns false (empty array = false).
	// Without a guard, SQL NOT EXISTS on an empty UNNEST returns true (no rows to violate).
	// We add an emptiness check: array must be non-null and non-empty.
	alias := a.elemAlias()
	return a.renderAllSQL(alias, array, condition), nil
}

// handleSome converts some operator to SQL.
// This checks if some elements in an array satisfy a condition.
// Generates: EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE condition).
// For ClickHouse: Uses arrayExists function.
func (a *ArrayOperator) handleSome(args []interface{}) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("some requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("some"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return "FALSE", nil
	}

	// First argument: array
	array, err := a.valueToSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some array argument: %w", err)
	}

	// Second argument: condition expression - rewrite element vars before SQL generation
	condition, err := a.predicateExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some condition argument: %w", err)
	}
	condition = a.replaceElementRefsInSQL(condition)

	alias := a.elemAlias()
	return a.renderSomeSQL(alias, array, condition), nil
}

// handleNone converts none operator to SQL.
// This checks if no elements in an array satisfy a condition.
// Generates: NOT EXISTS (SELECT 1 FROM UNNEST(array) AS elem WHERE condition).
// For ClickHouse: Uses NOT arrayExists function.
func (a *ArrayOperator) handleNone(args []interface{}) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("none requires exactly 2 arguments")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("none"); err != nil {
			return "", err
		}
	}

	// Validate that first argument is an array type
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return "TRUE", nil
	}

	// First argument: array
	array, err := a.valueToSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none array argument: %w", err)
	}

	// Second argument: condition expression - rewrite element vars before SQL generation
	condition, err := a.predicateExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none condition argument: %w", err)
	}
	condition = a.replaceElementRefsInSQL(condition)

	alias := a.elemAlias()
	return a.renderNoneSQL(alias, array, condition), nil
}

// handleMerge converts merge operator to SQL.
// This merges multiple arrays into one.
// BigQuery/Spanner: ARRAY_CONCAT(array1, array2, ...)
// PostgreSQL: array1 || array2 || ...
func (a *ArrayOperator) handleMerge(args []interface{}) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("merge requires at least 1 argument")
	}

	// Validate dialect support
	if a.config != nil {
		if err := a.config.ValidateDialect("merge"); err != nil {
			return "", err
		}
	}

	// Validate that all arguments are array types
	for _, arg := range args {
		if err := a.validateArrayOperand(arg); err != nil {
			return "", err
		}
	}

	// Convert non-empty array arguments to SQL. Empty literal arrays are the
	// merge identity and PostgreSQL cannot render them without an element type.
	arrays := make([]string, 0, len(args))
	for i, arg := range args {
		if isEmptyArrayLiteral(arg) {
			continue
		}
		array, err := a.valueToSQLAtPath(arg, a.argPath(i))
		if err != nil {
			return "", fmt.Errorf("invalid merge array argument %d: %w", i, err)
		}
		arrays = append(arrays, array)
	}
	return a.renderMergeSQL(arrays)
}

// valueToSQL converts a value to SQL, handling var expressions, arrays, and literals.
func (a *ArrayOperator) valueToSQL(value interface{}) (string, error) {
	return a.valueToSQLAtPath(value, a.currentPath())
}

func (a *ArrayOperator) valueExpressionToSQLWithContextAndPath(expr interface{}, allowAccumulator bool, path string) (string, error) {
	if a.config == nil || !a.config.HasValueExpressionParser() {
		return a.expressionToSQLWithContextAndPath(expr, allowAccumulator, path)
	}
	rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(expr, allowAccumulator, path)
	if err != nil {
		return "", err
	}
	res, err := a.config.ParseValueExpression(rewritten, path)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) predicateExpressionToSQLWithContextAndPath(expr interface{}, path string) (string, error) {
	if a.config == nil || !a.config.HasPredicateExpressionParser() {
		return a.expressionToSQLWithContextAndPath(expr, false, path)
	}
	rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(expr, false, path)
	if err != nil {
		return "", err
	}
	res, err := a.config.ParsePredicateExpression(rewritten, path)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) valueToSQLAtPath(value interface{}, path string) (string, error) {
	// Handle ProcessedValue (pre-processed SQL from parser)
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		// It's a literal, convert it
		return a.dataOp.valueToSQL(pv.Value)
	}

	// Handle complex expressions (operators)
	if expr, ok := value.(map[string]interface{}); ok {
		// Check if it's a var expression
		if varExpr, hasVar := expr[OpVar]; hasVar {
			if sql, handled, err := a.arrayInternalVarToSQL(varExpr); handled || err != nil {
				return sql, err
			}
			return a.dataOp.ToSQL(OpVar, []interface{}{varExpr})
		}
		// Otherwise, it's a complex value expression.
		return a.valueExpressionToSQLWithContextAndPath(value, false, path)
	}

	// Handle arrays
	if arr, ok := value.([]interface{}); ok {
		elements := make([]string, len(arr))
		for i, elem := range arr {
			elementSQL, err := a.valueExpressionToSQLWithContextAndPath(elem, false, tperrors.BuildArrayPath(path, i))
			if err != nil {
				return "", fmt.Errorf("invalid array element %d: %w", i, err)
			}
			elements[i] = elementSQL
		}
		return a.arrayLiteral(elements)
	}

	// Handle primitive values
	return a.dataOp.valueToSQL(value)
}

func (a *ArrayOperator) arrayLiteral(elements []string) (string, error) {
	if a.config != nil {
		return a.config.ArrayLiteral(elements)
	}
	return fmt.Sprintf("[%s]", strings.Join(elements, ", ")), nil
}

func (a *ArrayOperator) emptyArrayLiteralSQL() (string, error) {
	return a.arrayLiteral(nil)
}

func (a *ArrayOperator) expressionToSQLWithContextAndPath(expr interface{}, allowAccumulator bool, path string) (string, error) {
	// Handle ProcessedValue (pre-processed SQL from parser)
	if pv, ok := expr.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		// It's a literal, recursively convert it
		return a.expressionToSQLWithContextAndPath(pv.Value, allowAccumulator, path)
	}

	// Handle primitive values
	if a.isPrimitive(expr) {
		return a.dataOp.valueToSQL(expr)
	}

	// Handle var expressions
	if varExpr, ok := expr.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			if sql, handled, err := a.arrayScopeVarToSQL(varName); handled || err != nil {
				return sql, err
			}
			return a.dataOp.ToSQL(OpVar, []interface{}{varName})
		}
	}

	// Handle complex expressions by delegating to other operators
	if exprMap, ok := expr.(map[string]interface{}); ok {
		for operator, args := range exprMap {
			if a.valueSemantics && operator != OpVar && !a.isArrayOperator(operator) && a.config != nil && a.config.HasValueExpressionParser() {
				return a.valueExpressionToSQLWithContextAndPath(exprMap, allowAccumulator, path)
			}
			switch operator {
			case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
				if arr, ok := args.([]interface{}); ok {
					opPath := tperrors.BuildPath(path, operator, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorWithContextAndPath(arr, allowAccumulator, opPath)
					if err != nil {
						return "", err
					}
					converted, ok := rewrittenArgs.([]interface{})
					if !ok {
						return "", fmt.Errorf("internal error: expected []interface{} for rewritten comparison args")
					}
					arr = converted
					return a.comparisonOp.ToSQL(operator, arr)
				}
			case "and", "or":
				if arr, ok := args.([]interface{}); ok {
					if a.valueSemantics && a.config != nil && a.config.HasValueExpressionParser() {
						return a.valueExpressionToSQLWithContextAndPath(exprMap, allowAccumulator, path)
					}
					opPath := tperrors.BuildPath(path, operator, -1)
					parts := make([]string, len(arr))
					for i, arg := range arr {
						part, err := a.expressionToSQLWithContextAndPath(arg, allowAccumulator, tperrors.BuildArrayPath(opPath, i))
						if err != nil {
							return "", err
						}
						parts[i] = part
					}
					if len(parts) == 1 {
						return parts[0], nil
					}
					joiner := " AND "
					if operator == "or" {
						joiner = " OR "
					}
					return fmt.Sprintf("(%s)", strings.Join(parts, joiner)), nil
				}
			case "!", "!!", "if":
				if arr, ok := args.([]interface{}); ok {
					if a.valueSemantics && operator == "if" && a.config != nil && a.config.HasValueExpressionParser() {
						return a.valueExpressionToSQLWithContextAndPath(exprMap, allowAccumulator, path)
					}
					opPath := tperrors.BuildPath(path, operator, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorWithContextAndPath(arr, allowAccumulator, opPath)
					if err != nil {
						return "", err
					}
					converted, ok := rewrittenArgs.([]interface{})
					if !ok {
						return "", fmt.Errorf("internal error: expected []interface{} for rewritten logical args")
					}
					arr = converted
					return a.getLogicalOperator().ToSQL(operator, arr)
				}
			case "+", "-", "*", "/", "%", "max", "min":
				if arr, ok := args.([]interface{}); ok {
					opPath := tperrors.BuildPath(path, operator, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorWithContextAndPath(arr, allowAccumulator, opPath)
					if err != nil {
						return "", err
					}
					converted, ok := rewrittenArgs.([]interface{})
					if !ok {
						return "", fmt.Errorf("internal error: expected []interface{} for rewritten numeric args")
					}
					arr = converted
					return a.numericOp.ToSQL(operator, arr)
				}
			case "map", "filter", "reduce", "all", "some", "none", "merge":
				// Handle nested array operators
				if arr, ok := args.([]interface{}); ok {
					target := a
					nestedArgs := arr
					if a.shouldUseChildScope(operator, arr) {
						nestedArgs = a.rewriteOuterDottedForNested(operator, arr, a.elemAlias())
						target = a.withChildScope()
					}
					target = target.withValueScope(true)
					target = target.withValueSemantics(false)
					nestedPath := tperrors.BuildPath(path, operator, -1)
					return target.ToSQLAtPath(operator, nestedArgs, nestedPath)
				}
			default:
				// Try to use the expression parser callback for unknown operators
				// This enables support for custom operators in nested contexts
				if a.config != nil && a.config.HasExpressionParser() {
					rewrittenExpr, err := a.rewriteScopedVarsForOperatorWithContextAndPath(exprMap, allowAccumulator, path)
					if err != nil {
						return "", err
					}
					if pv, ok := rewrittenExpr.(ProcessedValue); ok && pv.IsSQL {
						return pv.Value, nil
					}
					return a.config.ParseExpression(rewrittenExpr, path)
				}
				return "", fmt.Errorf("unsupported operator in array expression: %s", operator)
			}
		}
	}

	return "", fmt.Errorf("invalid expression type: %T", expr)
}

// replaceElementRefsInSQL applies word-boundary replacement of "item" and "current"
// with "elem" in a final SQL string. This is a safety net for SQL produced by custom
// operators or nested operator chains that may emit literal "item"/"current" tokens
// not reachable by the AST-level rewrite.
func (a *ArrayOperator) replaceElementRefsInSQL(sql string) string {
	return replaceOutsideSingleQuotedStrings(sql, func(segment string) string {
		return a.replaceElementRefsInSQLSegment(segment)
	})
}

func (a *ArrayOperator) replaceElementRefsInSQLSegment(sql string) string {
	return elementRefPathPattern.ReplaceAllStringFunc(sql, func(match string) string {
		loc := elementRefPathPattern.FindStringSubmatchIndex(match)
		if loc == nil {
			return match
		}
		prefix := match[loc[2]:loc[3]]
		ref := match[loc[4]:loc[5]]
		mapped := a.mapElementVarName(ref)
		quoted, err := a.quoteArrayScopeIdentifier(mapped)
		if err != nil {
			return prefix + mapped
		}
		return prefix + quoted
	})
}

func replaceOutsideSingleQuotedStrings(sql string, replace func(string) string) string {
	var b strings.Builder
	segmentStart := 0
	for i := 0; i < len(sql); {
		if sql[i] != '\'' {
			i++
			continue
		}

		b.WriteString(replace(sql[segmentStart:i]))
		quoteStart := i
		i++
		for i < len(sql) {
			if sql[i] == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					i += 2
					continue
				}
				i++
				break
			}
			i++
		}
		b.WriteString(sql[quoteStart:i])
		segmentStart = i
	}
	b.WriteString(replace(sql[segmentStart:]))
	return b.String()
}

// replaceWithLiteral replaces regex matches while preserving the captured prefix
// and treating the replacement as a literal string (no $-expansion).
func replaceWithLiteral(re *regexp.Regexp, s, replacement string) string {
	return re.ReplaceAllStringFunc(s, func(match string) string {
		// The match includes the captured prefix character (or empty at start-of-string).
		// Find where the keyword starts by checking the prefix.
		loc := re.FindStringSubmatchIndex(match)
		if loc == nil {
			return match
		}
		prefix := match[loc[2]:loc[3]]
		return prefix + replacement
	})
}

// mapElementVarName maps JSONLogic element variable names to the SQL UNNEST alias.
// Returns the mapped name, or the original if no mapping applies.
// Only exact matches ("item", "current", "") and dot-prefix matches ("item.", "current.")
// are mapped - this prevents corrupting field names like "current_balance" or "item_count".
func (a *ArrayOperator) mapElementVarName(varStr string) string {
	// Exact matches for element references
	// Note: empty string ("") is NOT rewritten here - it's handled by expressionToSQL's
	// special case which returns ElemVar directly without schema validation.
	alias := a.elemAlias()
	if varStr == ItemVar || varStr == CurrentVar {
		return alias
	}
	// Dot-notation: "item.field" → "elem.field", "current.field" → "elem.field"
	if strings.HasPrefix(varStr, ItemVar+".") {
		return alias + varStr[len(ItemVar):]
	}
	if strings.HasPrefix(varStr, CurrentVar+".") {
		return alias + varStr[len(CurrentVar):]
	}
	return varStr
}

func (a *ArrayOperator) quoteArrayScopePath(alias, suffix string) (string, error) {
	if suffix == "" {
		return alias, nil
	}
	return a.quoteArrayScopeIdentifier(alias + "." + suffix)
}

func (a *ArrayOperator) quoteArrayScopeIdentifier(name string) (string, error) {
	segments := strings.Split(name, ".")
	for i, seg := range segments {
		if dialect.ContainsQuoteCharacters(seg) {
			return "", fmt.Errorf("array-scope variable name %q contains quote characters; "+
				"use raw identifiers — the transpiler handles quoting automatically", name)
		}
		if seg == "" || !validIdentifierSegment.MatchString(seg) {
			return "", fmt.Errorf("invalid identifier %q: each segment must match [a-zA-Z0-9_]+", name)
		}
		if i > 0 && dialect.NeedsQuoting(seg) {
			segments[i] = dialect.QuoteIdentifierSegment(seg, a.getDialect())
		}
	}
	return strings.Join(segments, "."), nil
}

// mapArrayScopeVar maps array-scope variable names to the current element alias.
// It handles direct elem references and JSONLogic aliases ("", "item", "current").
func (a *ArrayOperator) mapArrayScopeVar(varName string) (string, bool, error) {
	if varName == "" {
		return a.elemAlias(), true, nil
	}
	if a.isVisibleElemPath(varName) {
		quoted, err := a.quoteArrayScopeIdentifier(varName)
		if err != nil {
			return "", true, err
		}
		return quoted, true, nil
	}
	mapped := a.mapElementVarName(varName)
	if mapped != varName {
		quoted, err := a.quoteArrayScopeIdentifier(mapped)
		if err != nil {
			return "", true, err
		}
		return quoted, true, nil
	}
	return mapped, false, nil
}

// arrayScopeVarToSQL resolves array-scope var references without schema validation.
// This keeps item/current/elem aliases working inside array lambdas when schema mode is enabled.
func (a *ArrayOperator) arrayScopeVarToSQL(varExpr interface{}) (string, bool, error) {
	if varName, ok := varExpr.(string); ok {
		mapped, handled, err := a.mapArrayScopeVar(varName)
		if err != nil {
			return "", true, err
		}
		if handled {
			return mapped, true, nil
		}
		return "", false, nil
	}

	if arr, ok := varExpr.([]interface{}); ok {
		if len(arr) == 0 {
			return "", false, nil
		}
		if err := validateVarArrayMaxEntries(arr); err != nil {
			return "", true, err
		}
		varName, ok := arr[0].(string)
		if !ok {
			return "", false, nil
		}
		mapped, handled, err := a.mapArrayScopeVar(varName)
		if err != nil {
			return "", true, err
		}
		if !handled {
			return "", false, nil
		}
		if len(arr) == 1 {
			return mapped, true, nil
		}
		defaultSQL, err := a.dataOp.valueToSQL(arr[1])
		if err != nil {
			return "", true, fmt.Errorf("invalid default value: %w", err)
		}
		return fmt.Sprintf("COALESCE(%s, %s)", mapped, defaultSQL), true, nil
	}

	return "", false, nil
}

// arrayInternalVarToSQL resolves only internal aliases that are already in elem-scope.
// Unlike arrayScopeVarToSQL, it does NOT map item/current; this is used in valueToSQL
// for operands like reduce initial values where current/item should remain outer-scope.
func (a *ArrayOperator) arrayInternalVarToSQL(varExpr interface{}) (string, bool, error) {
	if varName, ok := varExpr.(string); ok {
		if varName == "" {
			if !a.valueScope {
				return "", false, nil
			}
			return a.elemAlias(), true, nil
		}
		if a.valueScope && a.isVisibleElemPath(varName) {
			quoted, err := a.quoteArrayScopeIdentifier(varName)
			if err != nil {
				return "", true, err
			}
			return quoted, true, nil
		}
		return "", false, nil
	}

	if arr, ok := varExpr.([]interface{}); ok {
		if len(arr) == 0 {
			return "", false, nil
		}
		if err := validateVarArrayMaxEntries(arr); err != nil {
			return "", true, err
		}
		varName, ok := arr[0].(string)
		if !ok {
			return "", false, nil
		}
		var mapped string
		switch {
		case varName == "":
			if !a.valueScope {
				return "", false, nil
			}
			mapped = a.elemAlias()
		case a.valueScope && a.isVisibleElemPath(varName):
			quoted, err := a.quoteArrayScopeIdentifier(varName)
			if err != nil {
				return "", true, err
			}
			mapped = quoted
		default:
			return "", false, nil
		}
		if len(arr) == 1 {
			return mapped, true, nil
		}
		defaultSQL, err := a.dataOp.valueToSQL(arr[1])
		if err != nil {
			return "", true, fmt.Errorf("invalid default value: %w", err)
		}
		return fmt.Sprintf("COALESCE(%s, %s)", mapped, defaultSQL), true, nil
	}

	return "", false, nil
}

func (a *ArrayOperator) rewriteScopedVarsForOperatorWithContextAndPath(expr interface{}, allowAccumulator bool, path string) (interface{}, error) {
	switch e := expr.(type) {
	case map[string]interface{}:
		if len(e) == 1 {
			if varName, hasVar := e[OpVar]; hasVar {
				if allowAccumulator && varName == AccumulatorVar {
					return a.accumulatorSQLResult(), nil
				}
				if sql, handled, err := a.arrayScopeVarToSQL(varName); handled || err != nil {
					if err != nil {
						return nil, err
					}
					return SQLFieldResult(sql), nil
				}
				return e, nil
			}
			for opName, opArgs := range e {
				if a.isArrayOperator(opName) {
					arr, ok := opArgs.([]interface{})
					if !ok {
						return e, nil
					}
					newArgs := make([]interface{}, len(arr))
					copy(newArgs, arr)
					opPath := tperrors.BuildPath(path, opName, -1)
					if len(newArgs) > 0 {
						rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(arr[0], allowAccumulator, tperrors.BuildArrayPath(opPath, 0))
						if err != nil {
							return nil, err
						}
						newArgs[0] = rewritten
					}
					if opName == OpReduce && len(newArgs) > 2 {
						rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(arr[2], allowAccumulator, tperrors.BuildArrayPath(opPath, 2))
						if err != nil {
							return nil, err
						}
						newArgs[2] = rewritten
					}
					return map[string]interface{}{opName: newArgs}, nil
				}
				if !a.isBuiltInOperatorName(opName) {
					opPath := tperrors.BuildPath(path, opName, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorWithContextAndPath(opArgs, allowAccumulator, tperrors.BuildArrayPath(opPath, 0))
					if err != nil {
						return nil, err
					}
					return map[string]interface{}{opName: rewrittenArgs}, nil
				}
			}
		}
		result := make(map[string]interface{}, len(e))
		for k, v := range e {
			rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(v, allowAccumulator, tperrors.BuildPath(path, k, -1))
			if err != nil {
				return nil, err
			}
			result[k] = rewritten
		}
		return result, nil

	case []interface{}:
		result := make([]interface{}, len(e))
		for i, v := range e {
			rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(v, allowAccumulator, tperrors.BuildArrayPath(path, i))
			if err != nil {
				return nil, err
			}
			result[i] = rewritten
		}
		return result, nil

	default:
		return expr, nil
	}
}

// isArrayOperator returns true if the operator is an array operator that
// introduces its own element scope. Used to prevent rewriting into nested scopes.
func (a *ArrayOperator) isArrayOperator(op string) bool {
	switch op {
	case OpMap, OpFilter, OpReduce, OpAll, OpSome, OpNone:
		return true
	}
	return false
}

func (a *ArrayOperator) isBuiltInOperatorName(op string) bool {
	switch op {
	case OpVar, OpMissing, OpMissingSome,
		OpEqual, OpStrictEqual, OpNotEqual, OpStrictNotEqual, OpGreaterThan, OpGreaterThanOrEqual, OpLessThan, OpLessThanOrEqual, OpIn,
		OpAnd, OpOr, OpNot, OpDoubleBang, OpIf,
		OpAdd, OpSubtract, OpMultiply, OpDivide, OpModulo, OpMax, OpMin,
		OpCat, OpSubstr,
		OpMap, OpFilter, OpReduce, OpAll, OpSome, OpNone, OpMerge:
		return true
	}
	return false
}

// rewriteElementVars walks the JSONLogic AST and rewrites element variable
// references ("item", "current", "") to the UNNEST alias ("elem") before SQL
// generation. This replaces the old post-hoc string replacement approach which
// corrupted field names containing "item" or "current" as substrings.
//
// When a nested array operator is encountered, only its array source argument
// (args[0]) is rewritten - the lambda/condition (args[1+]) is left for the
// nested operator to handle, preserving correct variable scoping.
func (a *ArrayOperator) rewriteElementVars(expr interface{}) interface{} {
	switch e := expr.(type) {
	case ProcessedValue:
		// Pre-processed SQL from custom operators - use word-boundary regex
		if e.IsSQL {
			replaced := a.replaceElementRefsInSQL(e.Value)
			if replaced != e.Value {
				return ProcessedValue{Value: replaced, IsSQL: true}
			}
		}
		return e
	case map[string]interface{}:
		if len(e) == 1 {
			// Check for var expression
			if varName, hasVar := e[OpVar]; hasVar {
				if varStr, ok := varName.(string); ok {
					mapped := a.mapElementVarName(varStr)
					if mapped != varStr {
						return map[string]interface{}{OpVar: mapped}
					}
				}
				// Handle array-form var: {"var": ["current", defaultValue]}
				if varArr, ok := varName.([]interface{}); ok && len(varArr) > 0 {
					if varStr, ok := varArr[0].(string); ok {
						mapped := a.mapElementVarName(varStr)
						if mapped != varStr {
							newArr := make([]interface{}, len(varArr))
							copy(newArr, varArr)
							newArr[0] = mapped
							return map[string]interface{}{OpVar: newArr}
						}
					}
				}
				return e
			}
			// Check for nested array operator - don't rewrite its lambda body
			for opName, opArgs := range e {
				if a.isArrayOperator(opName) {
					if arr, ok := opArgs.([]interface{}); ok {
						newArgs := make([]interface{}, len(arr))
						copy(newArgs, arr)
						// Rewrite args[0] (array source - outer scope)
						if len(newArgs) > 0 {
							newArgs[0] = a.rewriteElementVars(arr[0])
						}
						// For reduce, also rewrite args[2] (initial value - outer scope)
						if opName == OpReduce && len(newArgs) > 2 {
							newArgs[2] = a.rewriteElementVars(arr[2])
						}
						return map[string]interface{}{opName: newArgs}
					}
				}
			}
			// Regular single-key operator - recursively rewrite values
			for opName, opArgs := range e {
				return map[string]interface{}{opName: a.rewriteElementVars(opArgs)}
			}
		}
		// Multi-key map - recursively rewrite all values
		result := make(map[string]interface{}, len(e))
		for k, v := range e {
			result[k] = a.rewriteElementVars(v)
		}
		return result

	case []interface{}:
		result := make([]interface{}, len(e))
		for i, v := range e {
			result[i] = a.rewriteElementVars(v)
		}
		return result

	default:
		return expr
	}
}

func isEmptyArrayLiteral(value interface{}) bool {
	arr, ok := value.([]interface{})
	return ok && len(arr) == 0
}

// isPrimitive checks if a value is a primitive type.
func (a *ArrayOperator) isPrimitive(value interface{}) bool {
	switch value.(type) {
	case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number, bool:
		return true
	case nil:
		return true
	default:
		return false
	}
}

// ToSQLParam is the parameterized variant of ToSQL. Keep in sync.
func (a *ArrayOperator) ToSQLParam(operator string, args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("array operator %s requires at least one argument", operator)
	}
	switch operator {
	case "map":
		return a.handleMapParam(args, pc)
	case "filter":
		return a.handleFilterParam(args, pc)
	case "reduce":
		return a.handleReduceParam(args, pc)
	case "all":
		return a.handleAllParam(args, pc)
	case "some":
		return a.handleSomeParam(args, pc)
	case "none":
		return a.handleNoneParam(args, pc)
	case "merge":
		return a.handleMergeParam(args, pc)
	default:
		return "", fmt.Errorf("unsupported array operator: %s", operator)
	}
}

// ToSQLParamAtPath is the parameterized variant of ToSQLAtPath. Keep in sync.
func (a *ArrayOperator) ToSQLParamAtPath(operator string, args []interface{}, pc *params.ParamCollector, path string) (string, error) {
	scoped := a.withPath(path)
	if scoped.shouldUseRenderedSourceChildScope(operator, args) {
		nestedArgs := scoped.rewriteOuterDottedForNested(operator, args, scoped.elemAlias())
		return scoped.withChildScope().ToSQLParam(operator, nestedArgs, pc)
	}
	return scoped.ToSQLParam(operator, args, pc)
}

// handleMapParam is the parameterized variant of handleMap. Keep in sync.
func (a *ArrayOperator) handleMapParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("map requires exactly 2 arguments")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("map"); err != nil {
			return "", err
		}
	}
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return a.emptyArrayLiteralSQL()
	}
	array, err := a.valueToSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid map array argument: %w", err)
	}
	valueScoped := a.withValueSemantics(true)
	transformation, err := valueScoped.valueExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, false, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid map transformation argument: %w", err)
	}
	transformation = a.replaceElementRefsInSQL(transformation)
	alias := a.elemAlias()
	return a.renderMapSQL(alias, transformation, array), nil
}

// handleFilterParam is the parameterized variant of handleFilter. Keep in sync.
func (a *ArrayOperator) handleFilterParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("filter requires exactly 2 arguments")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("filter"); err != nil {
			return "", err
		}
	}
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return a.emptyArrayLiteralSQL()
	}
	array, err := a.valueToSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid filter array argument: %w", err)
	}
	condition, err := a.predicateExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid filter condition argument: %w", err)
	}
	condition = a.replaceElementRefsInSQL(condition)
	alias := a.elemAlias()
	return a.renderFilterSQL(alias, array, condition), nil
}

// handleReduceParam is the parameterized variant of handleReduce. Keep in sync.
func (a *ArrayOperator) handleReduceParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != reduceOperatorArgCount {
		return "", fmt.Errorf("reduce requires exactly 3 arguments")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("reduce"); err != nil {
			return "", err
		}
	}
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	initial, err := a.valueToSQLParamAtPath(args[arrayReduceInitialArgIndex], pc, a.argPath(arrayReduceInitialArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce initial argument: %w", err)
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return initial, nil
	}
	array, err := a.valueToSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce array argument: %w", err)
	}
	reducerExpr := args[arrayExpressionArgIndex]
	alias := a.elemAlias()

	if pattern := a.detectAggregatePattern(reducerExpr); pattern != nil {
		elemRef, quoteErr := a.quoteArrayScopePath(alias, pattern.fieldSuffix)
		if quoteErr != nil {
			return "", quoteErr
		}

		switch a.getDialect() {
		case dialect.DialectClickHouse:
			if pattern.fieldSuffix != "" {
				mappedRef, quoteErr := a.quoteArrayScopePath("x", pattern.fieldSuffix)
				if quoteErr != nil {
					return "", quoteErr
				}
				return fmt.Sprintf("%s + coalesce(arrayReduce('%s', arrayMap(x -> %s, %s)), 0)",
					initial, strings.ToLower(pattern.function), mappedRef, array), nil
			}
			return fmt.Sprintf("%s + coalesce(arrayReduce('%s', %s), 0)",
				initial, strings.ToLower(pattern.function), array), nil
		case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
			return fmt.Sprintf("%s + COALESCE((SELECT %s(%s) FROM UNNEST(%s) AS %s), 0)",
				initial, pattern.function, elemRef, array, alias), nil
		}
		return fmt.Sprintf("%s + COALESCE((SELECT %s(%s) FROM UNNEST(%s) AS %s), 0)",
			initial, pattern.function, elemRef, array, alias), nil
	}

	rewritten := a.rewriteElementVars(reducerExpr)
	accumulatorType := a.inferValueExpressionType(args[arrayReduceInitialArgIndex], ExpressionTypeUnknown)
	valueScoped := a.withValueSemantics(true).withAccumulatorType(accumulatorType)
	reducerWithElem, err := valueScoped.expressionToSQLParamWithContextAndPath(rewritten, pc, true, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce expression: %w", err)
	}
	reducerWithElem = a.replaceElementRefsInSQL(reducerWithElem)
	reducerWithElem = replaceWithLiteral(accumulatorPattern, reducerWithElem, initial)

	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("arrayFold((acc, %s) -> %s, %s, %s)", alias, reducerWithElem, array, initial), nil
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		return fmt.Sprintf("(SELECT %s FROM UNNEST(%s) AS %s)", reducerWithElem, array, alias), nil
	}
	return fmt.Sprintf("(SELECT %s FROM UNNEST(%s) AS %s)", reducerWithElem, array, alias), nil
}

// handleAllParam is the parameterized variant of handleAll. Keep in sync.
func (a *ArrayOperator) handleAllParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("all requires exactly 2 arguments")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("all"); err != nil {
			return "", err
		}
	}
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return "FALSE", nil
	}
	array, err := a.valueToSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all array argument: %w", err)
	}
	condition, err := a.predicateExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all condition argument: %w", err)
	}
	condition = a.replaceElementRefsInSQL(condition)

	alias := a.elemAlias()
	return a.renderAllSQL(alias, array, condition), nil
}

// handleSomeParam is the parameterized variant of handleSome. Keep in sync.
func (a *ArrayOperator) handleSomeParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("some requires exactly 2 arguments")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("some"); err != nil {
			return "", err
		}
	}
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return "FALSE", nil
	}
	array, err := a.valueToSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some array argument: %w", err)
	}
	condition, err := a.predicateExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some condition argument: %w", err)
	}
	condition = a.replaceElementRefsInSQL(condition)
	alias := a.elemAlias()
	return a.renderSomeSQL(alias, array, condition), nil
}

// handleNoneParam is the parameterized variant of handleNone. Keep in sync.
func (a *ArrayOperator) handleNoneParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) != binaryArrayOperatorArgCount {
		return "", fmt.Errorf("none requires exactly 2 arguments")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("none"); err != nil {
			return "", err
		}
	}
	if err := a.validateArrayOperand(args[arraySourceArgIndex]); err != nil {
		return "", err
	}
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return "TRUE", nil
	}
	array, err := a.valueToSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none array argument: %w", err)
	}
	condition, err := a.predicateExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none condition argument: %w", err)
	}
	condition = a.replaceElementRefsInSQL(condition)
	alias := a.elemAlias()
	return a.renderNoneSQL(alias, array, condition), nil
}

// handleMergeParam is the parameterized variant of handleMerge. Keep in sync.
func (a *ArrayOperator) handleMergeParam(args []interface{}, pc *params.ParamCollector) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("merge requires at least 1 argument")
	}
	if a.config != nil {
		if err := a.config.ValidateDialect("merge"); err != nil {
			return "", err
		}
	}
	for _, arg := range args {
		if err := a.validateArrayOperand(arg); err != nil {
			return "", err
		}
	}
	arrays := make([]string, 0, len(args))
	for i, arg := range args {
		if isEmptyArrayLiteral(arg) {
			continue
		}
		array, err := a.valueToSQLParamAtPath(arg, pc, a.argPath(i))
		if err != nil {
			return "", fmt.Errorf("invalid merge array argument %d: %w", i, err)
		}
		arrays = append(arrays, array)
	}
	return a.renderMergeSQL(arrays)
}

// valueToSQLParam is the parameterized variant of valueToSQL. Keep in sync.
func (a *ArrayOperator) valueToSQLParam(value interface{}, pc *params.ParamCollector) (string, error) {
	return a.valueToSQLParamAtPath(value, pc, a.currentPath())
}

func (a *ArrayOperator) valueExpressionToSQLParamWithContextAndPath(
	expr interface{},
	pc *params.ParamCollector,
	allowAccumulator bool,
	path string,
) (string, error) {
	if a.config == nil || !a.config.HasParamValueExpressionParser() {
		return a.expressionToSQLParamWithContextAndPath(expr, pc, allowAccumulator, path)
	}
	rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(expr, allowAccumulator, path)
	if err != nil {
		return "", err
	}
	res, err := a.config.ParseValueExpressionParam(rewritten, path, pc)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) predicateExpressionToSQLParamWithContextAndPath(
	expr interface{},
	pc *params.ParamCollector,
	path string,
) (string, error) {
	if a.config == nil || !a.config.HasParamPredicateExpressionParser() {
		return a.expressionToSQLParamWithContextAndPath(expr, pc, false, path)
	}
	rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(expr, false, path)
	if err != nil {
		return "", err
	}
	res, err := a.config.ParsePredicateExpressionParam(rewritten, path, pc)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) valueToSQLParamAtPath(value interface{}, pc *params.ParamCollector, path string) (string, error) {
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		return a.dataOp.valueToSQLParam(pv.Value, pc)
	}

	if expr, ok := value.(map[string]interface{}); ok {
		if varExpr, hasVar := expr[OpVar]; hasVar {
			if sql, handled, err := a.arrayInternalVarToSQLParam(varExpr, pc); handled || err != nil {
				return sql, err
			}
			return a.dataOp.ToSQLParam(OpVar, []interface{}{varExpr}, pc)
		}
		return a.valueExpressionToSQLParamWithContextAndPath(value, pc, false, path)
	}

	if arr, ok := value.([]interface{}); ok {
		elements := make([]string, len(arr))
		for i, elem := range arr {
			elementSQL, err := a.valueExpressionToSQLParamWithContextAndPath(elem, pc, false, tperrors.BuildArrayPath(path, i))
			if err != nil {
				return "", fmt.Errorf("invalid array element %d: %w", i, err)
			}
			elements[i] = elementSQL
		}
		return a.arrayLiteral(elements)
	}

	return a.dataOp.valueToSQLParam(value, pc)
}

func (a *ArrayOperator) expressionToSQLParamWithContextAndPath(
	expr interface{},
	pc *params.ParamCollector,
	allowAccumulator bool,
	path string,
) (string, error) {
	if pv, ok := expr.(ProcessedValue); ok {
		if pv.IsSQL {
			return pv.Value, nil
		}
		return a.expressionToSQLParamWithContextAndPath(pv.Value, pc, allowAccumulator, path)
	}

	if a.isPrimitive(expr) {
		return a.dataOp.valueToSQLParam(expr, pc)
	}

	if varExpr, ok := expr.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			if sql, handled, err := a.arrayScopeVarToSQLParam(varName, pc); handled || err != nil {
				return sql, err
			}
			return a.dataOp.ToSQLParam(OpVar, []interface{}{varName}, pc)
		}
	}

	if exprMap, ok := expr.(map[string]interface{}); ok {
		for operator, args := range exprMap {
			if a.valueSemantics && operator != OpVar && !a.isArrayOperator(operator) && a.config != nil && a.config.HasParamValueExpressionParser() {
				return a.valueExpressionToSQLParamWithContextAndPath(exprMap, pc, allowAccumulator, path)
			}
			switch operator {
			case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
				if arr, ok := args.([]interface{}); ok {
					opPath := tperrors.BuildPath(path, operator, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(arr, allowAccumulator, opPath)
					if err != nil {
						return "", err
					}
					converted, ok := rewrittenArgs.([]interface{})
					if !ok {
						return "", fmt.Errorf("internal error: expected []interface{} for rewritten comparison args")
					}
					arr = converted
					return a.comparisonOp.ToSQLParam(operator, arr, pc)
				}
			case "and", "or":
				if arr, ok := args.([]interface{}); ok {
					if a.valueSemantics && a.config != nil && a.config.HasParamValueExpressionParser() {
						return a.valueExpressionToSQLParamWithContextAndPath(exprMap, pc, allowAccumulator, path)
					}
					opPath := tperrors.BuildPath(path, operator, -1)
					parts := make([]string, len(arr))
					for i, arg := range arr {
						part, err := a.expressionToSQLParamWithContextAndPath(arg, pc, allowAccumulator, tperrors.BuildArrayPath(opPath, i))
						if err != nil {
							return "", err
						}
						parts[i] = part
					}
					if len(parts) == 1 {
						return parts[0], nil
					}
					joiner := " AND "
					if operator == "or" {
						joiner = " OR "
					}
					return fmt.Sprintf("(%s)", strings.Join(parts, joiner)), nil
				}
			case "!", "!!", "if":
				if arr, ok := args.([]interface{}); ok {
					if a.valueSemantics && operator == "if" && a.config != nil && a.config.HasParamValueExpressionParser() {
						return a.valueExpressionToSQLParamWithContextAndPath(exprMap, pc, allowAccumulator, path)
					}
					opPath := tperrors.BuildPath(path, operator, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(arr, allowAccumulator, opPath)
					if err != nil {
						return "", err
					}
					converted, ok := rewrittenArgs.([]interface{})
					if !ok {
						return "", fmt.Errorf("internal error: expected []interface{} for rewritten logical args")
					}
					arr = converted
					return a.getLogicalOperator().ToSQLParam(operator, arr, pc)
				}
			case "+", "-", "*", "/", "%", "max", "min":
				if arr, ok := args.([]interface{}); ok {
					opPath := tperrors.BuildPath(path, operator, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(arr, allowAccumulator, opPath)
					if err != nil {
						return "", err
					}
					converted, ok := rewrittenArgs.([]interface{})
					if !ok {
						return "", fmt.Errorf("internal error: expected []interface{} for rewritten numeric args")
					}
					arr = converted
					return a.numericOp.ToSQLParam(operator, arr, pc)
				}
			case "map", "filter", "reduce", "all", "some", "none", "merge":
				if arr, ok := args.([]interface{}); ok {
					target := a
					nestedArgs := arr
					if a.shouldUseChildScope(operator, arr) {
						nestedArgs = a.rewriteOuterDottedForNested(operator, arr, a.elemAlias())
						target = a.withChildScope()
					}
					target = target.withValueScope(true)
					target = target.withValueSemantics(false)
					nestedPath := tperrors.BuildPath(path, operator, -1)
					return target.ToSQLParamAtPath(operator, nestedArgs, pc, nestedPath)
				}
			default:
				if a.config != nil && a.config.HasParamExpressionParser() {
					rewrittenExpr, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(exprMap, allowAccumulator, path)
					if err != nil {
						return "", err
					}
					if pv, ok := rewrittenExpr.(ProcessedValue); ok && pv.IsSQL {
						return pv.Value, nil
					}
					return a.config.ParseExpressionParam(rewrittenExpr, path, pc)
				}
				return "", fmt.Errorf("unsupported operator in array expression: %s", operator)
			}
		}
	}

	return "", fmt.Errorf("invalid expression type: %T", expr)
}

// arrayScopeVarToSQLParam is the parameterized variant of arrayScopeVarToSQL. Keep in sync.
func (a *ArrayOperator) arrayScopeVarToSQLParam(varExpr interface{}, pc *params.ParamCollector) (string, bool, error) {
	if varName, ok := varExpr.(string); ok {
		mapped, handled, err := a.mapArrayScopeVar(varName)
		if err != nil {
			return "", true, err
		}
		if handled {
			return mapped, true, nil
		}
		return "", false, nil
	}

	if arr, ok := varExpr.([]interface{}); ok {
		if len(arr) == 0 {
			return "", false, nil
		}
		if err := validateVarArrayMaxEntries(arr); err != nil {
			return "", true, err
		}
		varName, ok := arr[0].(string)
		if !ok {
			return "", false, nil
		}
		mapped, handled, err := a.mapArrayScopeVar(varName)
		if err != nil {
			return "", true, err
		}
		if !handled {
			return "", false, nil
		}
		if len(arr) == 1 {
			return mapped, true, nil
		}
		defaultSQL, err := a.dataOp.valueToSQLParam(arr[1], pc)
		if err != nil {
			return "", true, fmt.Errorf("invalid default value: %w", err)
		}
		return fmt.Sprintf("COALESCE(%s, %s)", mapped, defaultSQL), true, nil
	}

	return "", false, nil
}

func (a *ArrayOperator) rewriteArrayScopeVarParam(varExpr interface{}) (interface{}, bool, error) {
	if varName, ok := varExpr.(string); ok {
		mapped, handled, err := a.mapArrayScopeVar(varName)
		if err != nil {
			return nil, true, err
		}
		if handled {
			return SQLFieldResult(mapped), true, nil
		}
		return nil, false, nil
	}

	if arr, ok := varExpr.([]interface{}); ok {
		if len(arr) == 0 {
			return nil, false, nil
		}
		varName, ok := arr[0].(string)
		if !ok {
			return nil, false, nil
		}
		mapped, handled, err := a.mapArrayScopeVar(varName)
		if err != nil {
			return nil, true, err
		}
		if !handled {
			return nil, false, nil
		}
		if len(arr) == 1 {
			return SQLFieldResult(mapped), true, nil
		}
		newArr := make([]interface{}, len(arr))
		copy(newArr, arr)
		newArr[0] = SQLFieldResult(mapped)
		return map[string]interface{}{OpVar: newArr}, true, nil
	}

	return nil, false, nil
}

// arrayInternalVarToSQLParam is the parameterized variant of arrayInternalVarToSQL.
func (a *ArrayOperator) arrayInternalVarToSQLParam(varExpr interface{}, pc *params.ParamCollector) (string, bool, error) {
	if varName, ok := varExpr.(string); ok {
		if varName == "" {
			if !a.valueScope {
				return "", false, nil
			}
			return a.elemAlias(), true, nil
		}
		if a.valueScope && a.isVisibleElemPath(varName) {
			quoted, err := a.quoteArrayScopeIdentifier(varName)
			if err != nil {
				return "", true, err
			}
			return quoted, true, nil
		}
		return "", false, nil
	}

	if arr, ok := varExpr.([]interface{}); ok {
		if len(arr) == 0 {
			return "", false, nil
		}
		if err := validateVarArrayMaxEntries(arr); err != nil {
			return "", true, err
		}
		varName, ok := arr[0].(string)
		if !ok {
			return "", false, nil
		}
		var mapped string
		switch {
		case varName == "":
			if !a.valueScope {
				return "", false, nil
			}
			mapped = a.elemAlias()
		case a.valueScope && a.isVisibleElemPath(varName):
			quoted, err := a.quoteArrayScopeIdentifier(varName)
			if err != nil {
				return "", true, err
			}
			mapped = quoted
		default:
			return "", false, nil
		}
		if len(arr) == 1 {
			return mapped, true, nil
		}
		defaultSQL, err := a.dataOp.valueToSQLParam(arr[1], pc)
		if err != nil {
			return "", true, fmt.Errorf("invalid default value: %w", err)
		}
		return fmt.Sprintf("COALESCE(%s, %s)", mapped, defaultSQL), true, nil
	}

	return "", false, nil
}

func (a *ArrayOperator) rewriteScopedVarsForOperatorParamWithContextAndPath(
	expr interface{},
	allowAccumulator bool,
	path string,
) (interface{}, error) {
	switch e := expr.(type) {
	case map[string]interface{}:
		if len(e) == 1 {
			if varName, hasVar := e[OpVar]; hasVar {
				if allowAccumulator && varName == AccumulatorVar {
					return a.accumulatorSQLResult(), nil
				}
				if rewritten, handled, err := a.rewriteArrayScopeVarParam(varName); handled || err != nil {
					if err != nil {
						return nil, err
					}
					return rewritten, nil
				}
				return e, nil
			}
			for opName, opArgs := range e {
				if a.isArrayOperator(opName) {
					arr, ok := opArgs.([]interface{})
					if !ok {
						return e, nil
					}
					newArgs := make([]interface{}, len(arr))
					copy(newArgs, arr)
					opPath := tperrors.BuildPath(path, opName, -1)
					if len(newArgs) > 0 {
						rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(arr[0], allowAccumulator, tperrors.BuildArrayPath(opPath, 0))
						if err != nil {
							return nil, err
						}
						newArgs[0] = rewritten
					}
					if opName == OpReduce && len(newArgs) > 2 {
						rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(arr[2], allowAccumulator, tperrors.BuildArrayPath(opPath, 2))
						if err != nil {
							return nil, err
						}
						newArgs[2] = rewritten
					}
					return map[string]interface{}{opName: newArgs}, nil
				}
				if !a.isBuiltInOperatorName(opName) {
					opPath := tperrors.BuildPath(path, opName, -1)
					rewrittenArgs, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(opArgs, allowAccumulator, tperrors.BuildArrayPath(opPath, 0))
					if err != nil {
						return nil, err
					}
					return map[string]interface{}{opName: rewrittenArgs}, nil
				}
			}
		}
		result := make(map[string]interface{}, len(e))
		for k, v := range e {
			rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(v, allowAccumulator, tperrors.BuildPath(path, k, -1))
			if err != nil {
				return nil, err
			}
			result[k] = rewritten
		}
		return result, nil

	case []interface{}:
		result := make([]interface{}, len(e))
		for i, v := range e {
			rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(v, allowAccumulator, tperrors.BuildArrayPath(path, i))
			if err != nil {
				return nil, err
			}
			result[i] = rewritten
		}
		return result, nil

	default:
		return expr, nil
	}
}
