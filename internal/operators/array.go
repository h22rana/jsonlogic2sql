package operators

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

var accumulatorPattern = regexp.MustCompile(`(^|[^\w.])` + regexp.QuoteMeta(AccumulatorVar) + `\b`)

type arrayLambdaScope int

const (
	arrayLambdaScopeNone arrayLambdaScope = iota
	arrayLambdaScopeElement
	arrayLambdaScopeReduce
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
	valueSemantics     bool
	accumulatorType    ExpressionType
	hasAccumulatorType bool
}

type typedValueSQL struct {
	sql               string
	typ               ExpressionType
	emptyArrayLiteral bool
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
		visibleScopes:  []string{""},
		exprPath:       "$",
		valueScope:     false,
		lambdaScope:    arrayLambdaScopeNone,
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
		visibleScopes:      append([]string{}, a.visibleScopes...),
		exprPath:           a.exprPath,
		valueScope:         a.valueScope,
		lambdaScope:        a.lambdaScope,
		schemaScope:        a.schemaScope,
		schemaScopes:       append([]string{}, a.schemaScopes...),
		valueSemantics:     a.valueSemantics,
		accumulatorType:    a.accumulatorType,
		hasAccumulatorType: a.hasAccumulatorType,
	}
	childAlias := child.elemAlias()
	child.visibleElems = append(child.visibleElems, childAlias)
	child.visibleScopes = append(child.visibleScopes, "")
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
		visibleScopes:      append([]string{}, a.visibleScopes...),
		exprPath:           path,
		valueScope:         a.valueScope,
		lambdaScope:        a.lambdaScope,
		schemaScope:        a.schemaScope,
		schemaScopes:       append([]string{}, a.schemaScopes...),
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
		visibleScopes:      append([]string{}, a.visibleScopes...),
		exprPath:           a.exprPath,
		valueScope:         enabled,
		lambdaScope:        a.lambdaScope,
		schemaScope:        a.schemaScope,
		schemaScopes:       append([]string{}, a.schemaScopes...),
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
		visibleScopes:      append([]string{}, a.visibleScopes...),
		exprPath:           a.exprPath,
		valueScope:         a.valueScope,
		lambdaScope:        a.lambdaScope,
		schemaScope:        a.schemaScope,
		schemaScopes:       append([]string{}, a.schemaScopes...),
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
		visibleScopes:      append([]string{}, a.visibleScopes...),
		exprPath:           a.exprPath,
		valueScope:         a.valueScope,
		lambdaScope:        a.lambdaScope,
		schemaScope:        a.schemaScope,
		schemaScopes:       append([]string{}, a.schemaScopes...),
		valueSemantics:     a.valueSemantics,
		accumulatorType:    typ,
		hasAccumulatorType: true,
	}
	return child
}

func (a *ArrayOperator) withLambdaScope(scope arrayLambdaScope) *ArrayOperator {
	child := &ArrayOperator{
		config:             a.config,
		dataOp:             a.dataOp,
		comparisonOp:       a.comparisonOp,
		logicalOp:          a.logicalOp,
		numericOp:          a.numericOp,
		scopeDepth:         a.scopeDepth,
		visibleElems:       append([]string{}, a.visibleElems...),
		visibleScopes:      append([]string{}, a.visibleScopes...),
		exprPath:           a.exprPath,
		valueScope:         a.valueScope,
		lambdaScope:        scope,
		schemaScope:        a.schemaScope,
		schemaScopes:       append([]string{}, a.schemaScopes...),
		valueSemantics:     a.valueSemantics,
		accumulatorType:    a.accumulatorType,
		hasAccumulatorType: a.hasAccumulatorType,
	}
	return child
}

func (a *ArrayOperator) withSchemaScopes(scopes []string) *ArrayOperator {
	scopes = normalizeSchemaScopes(scopes)
	scope := ""
	if len(scopes) > 0 {
		scope = scopes[0]
	}
	child := &ArrayOperator{
		config:             a.config,
		dataOp:             a.dataOp,
		comparisonOp:       a.comparisonOp,
		logicalOp:          a.logicalOp,
		numericOp:          a.numericOp,
		scopeDepth:         a.scopeDepth,
		visibleElems:       append([]string{}, a.visibleElems...),
		visibleScopes:      append([]string{}, a.visibleScopes...),
		exprPath:           a.exprPath,
		valueScope:         a.valueScope,
		lambdaScope:        a.lambdaScope,
		schemaScope:        scope,
		schemaScopes:       append([]string{}, scopes...),
		valueSemantics:     a.valueSemantics,
		accumulatorType:    a.accumulatorType,
		hasAccumulatorType: a.hasAccumulatorType,
	}
	if len(child.visibleScopes) > 0 {
		child.visibleScopes[len(child.visibleScopes)-1] = scope
	}
	return child
}

func singleSchemaScope(scope string) []string {
	if scope == "" {
		return nil
	}
	return []string{scope}
}

func normalizeSchemaScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(scopes))
	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if scope == "" {
			continue
		}
		if _, exists := seen[scope]; exists {
			continue
		}
		seen[scope] = struct{}{}
		normalized = append(normalized, scope)
	}
	return normalized
}

func (a *ArrayOperator) currentSchemaScopes() []string {
	if a == nil {
		return nil
	}
	if len(a.schemaScopes) > 0 {
		return a.schemaScopes
	}
	return singleSchemaScope(a.schemaScope)
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

func (a *ArrayOperator) inferValueExpressionType(expr interface{}) ExpressionType {
	if a != nil && a.config != nil && a.config.HasValueTypeInferer() {
		return a.config.InferValueExpressionType(expr, ExpressionTypeUnknown)
	}
	return inferLiteralValueExpressionType(expr)
}

func (a *ArrayOperator) schemaExpressionType(fieldName string) ExpressionType {
	if fieldName == "" || a.schema() == nil {
		return ExpressionTypeUnknown
	}
	switch a.schema().GetFieldType(fieldName) {
	case "boolean":
		return ExpressionTypeBoolean
	case "string", "enum":
		return ExpressionTypeString
	case "integer", "number":
		return ExpressionTypeNumber
	case "array":
		return ExpressionTypeArray
	default:
		return ExpressionTypeUnknown
	}
}

func (a *ArrayOperator) resolveScopedFieldName(fieldName string) (string, error) {
	return a.resolveFieldInScopes(a.currentSchemaScopes(), fieldName)
}

func (a *ArrayOperator) resolveScopedFieldNames(fieldName string) []string {
	resolved, err := a.resolveFieldNamesInScopes(a.currentSchemaScopes(), fieldName)
	if err != nil {
		return nil
	}
	return resolved
}

func (a *ArrayOperator) resolveFieldInScope(scopePath, fieldName string) (string, error) {
	return a.resolveFieldInScopes(singleSchemaScope(scopePath), fieldName)
}

func (a *ArrayOperator) resolveFieldInScopes(scopePaths []string, fieldName string) (string, error) {
	resolved, err := a.resolveFieldNamesInScopes(scopePaths, fieldName)
	if err != nil {
		return "", err
	}
	if len(resolved) == 0 {
		return "", nil
	}
	return resolved[0], nil
}

func (a *ArrayOperator) resolveFieldNamesInScopes(scopePaths []string, fieldName string) ([]string, error) {
	if fieldName == "" {
		return nil, nil
	}
	schema := a.schema()
	if schema == nil {
		return singleSchemaScope(fieldName), nil
	}
	if scoped, ok := schema.(ScopedSchemaProvider); ok {
		scopes := normalizeSchemaScopes(scopePaths)
		if len(scopes) == 0 {
			resolved, err := scoped.ResolveScopedField("", fieldName)
			if err != nil {
				return nil, err
			}
			return singleSchemaScope(resolved), nil
		}

		var firstType string
		var firstAllowed []string
		resolvedFields := make([]string, 0, len(scopes))
		for i, scope := range scopes {
			resolved, err := scoped.ResolveScopedField(scope, fieldName)
			if err != nil {
				return nil, err
			}
			resolvedFields = append(resolvedFields, resolved)
			if i == 0 {
				firstType = schema.GetFieldType(resolved)
				firstAllowed = schema.GetAllowedValues(resolved)
				continue
			}
			if typ := schema.GetFieldType(resolved); typ != firstType {
				return nil, fmt.Errorf("field '%s' has incompatible schema types across array source scopes", fieldName)
			}
			if !sameStringSet(schema.GetAllowedValues(resolved), firstAllowed) {
				return nil, fmt.Errorf("field '%s' has incompatible enum values across array source scopes", fieldName)
			}
		}
		return normalizeSchemaScopes(resolvedFields), nil
	}
	if err := schema.ValidateField(fieldName); err != nil {
		return nil, err
	}
	return singleSchemaScope(fieldName), nil
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	if len(left) == 0 {
		return true
	}
	counts := make(map[string]int, len(left))
	for _, value := range left {
		counts[value]++
	}
	for _, value := range right {
		counts[value]--
		if counts[value] < 0 {
			return false
		}
	}
	return true
}

func (a *ArrayOperator) scopedSQLFieldResult(sql, fieldName string) ProcessedValue {
	result := SQLFieldResult(sql)
	if fieldName == "" {
		return result
	}
	result.FieldName = fieldName
	if typ := a.schemaExpressionType(fieldName); typ != ExpressionTypeUnknown {
		result.HasExpressionInfo = true
		result.Kind = ExpressionKindValue
		result.Type = typ
	}
	return result
}

func (a *ArrayOperator) scopedSQLFieldResultFromVarExpr(sql string, varExpr interface{}) ProcessedValue {
	result := a.scopedSQLFieldResult(sql, a.scopedFieldNameFromVarExpr(varExpr))
	defaultValue, hasDefault, defaultKnown := scopedVarDefaultLiteral(varExpr)
	if !hasDefault {
		return result
	}
	result.FieldHasDefault = true
	if defaultKnown {
		result.FieldDefaultLiteral = defaultValue
		result.FieldDefaultLiteralKnown = true
	}
	return result
}

func scopedVarDefaultLiteral(varExpr interface{}) (interface{}, bool, bool) {
	arr, ok := varExpr.([]interface{})
	if !ok || len(arr) < 2 {
		return nil, false, false
	}
	defaultValue := arr[1]
	if pv, ok := defaultValue.(ProcessedValue); ok {
		if pv.IsSQL {
			return nil, true, false
		}
		return pv.Value, true, true
	}
	switch defaultValue.(type) {
	case map[string]interface{}, []interface{}:
		return nil, true, false
	default:
		return defaultValue, true, true
	}
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

func (a *ArrayOperator) accumulatorSQLResultWithSQL(sql string) ProcessedValue {
	result := a.accumulatorSQLResult()
	result.Value = sql
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
	if !a.isArrayOperator(op) || len(args) == 0 {
		return false
	}
	if isRenderedArrayScopeSource(args[0]) ||
		a.referencesVisibleElemAlias(args[0]) ||
		a.referencesCurrentScopeAlias(args[0]) {
		return true
	}
	return op == OpReduce && len(args) > 2 &&
		(a.referencesVisibleElemAlias(args[2]) || a.referencesCurrentScopeAlias(args[2]))
}

func (a *ArrayOperator) shouldParseScopedArrayExpressionLocally(expr interface{}) bool {
	operator, args, ok := arrayOperatorArgs(expr)
	return ok && a.shouldUseChildScope(operator, args)
}

func arrayOperatorArgs(expr interface{}) (string, []interface{}, bool) {
	exprMap, ok := expr.(map[string]interface{})
	if !ok || len(exprMap) != 1 {
		return "", nil, false
	}
	for operator, rawArgs := range exprMap {
		args, ok := rawArgs.([]interface{})
		return operator, args, ok
	}
	return "", nil, false
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

func (a *ArrayOperator) referencesCurrentScopeAlias(expr interface{}) bool {
	switch e := expr.(type) {
	case map[string]interface{}:
		if len(e) == 1 {
			if varName, hasVar := e[OpVar]; hasVar {
				return a.varExprReferencesCurrentScopeAlias(varName)
			}
		}
		for _, v := range e {
			if a.referencesCurrentScopeAlias(v) {
				return true
			}
		}
		return false
	case []interface{}:
		for _, v := range e {
			if a.referencesCurrentScopeAlias(v) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (a *ArrayOperator) varExprReferencesCurrentScopeAlias(varExpr interface{}) bool {
	switch v := varExpr.(type) {
	case string:
		return a.varNameReferencesCurrentScopeAlias(v)
	case []interface{}:
		if len(v) == 0 {
			return false
		}
		if s, ok := v[0].(string); ok {
			return a.varNameReferencesCurrentScopeAlias(s)
		}
		if pv, ok := v[0].(ProcessedValue); ok && pv.IsSQL {
			for _, alias := range a.visibleElems {
				if pv.Value == alias || strings.Contains(pv.Value, alias+".") {
					return true
				}
			}
		}
	case ProcessedValue:
		if v.IsSQL {
			for _, alias := range a.visibleElems {
				if v.Value == alias || strings.Contains(v.Value, alias+".") {
					return true
				}
			}
		}
	}
	return false
}

func (a *ArrayOperator) varNameReferencesCurrentScopeAlias(varName string) bool {
	switch a.lambdaScope {
	case arrayLambdaScopeElement:
		return !isUnsupportedElementScopeVar(varName)
	case arrayLambdaScopeReduce:
		if varName == CurrentVar {
			return true
		}
		if strings.HasPrefix(varName, CurrentVar+".") {
			return strings.TrimPrefix(varName, CurrentVar+".") != ""
		}
		return false
	case arrayLambdaScopeNone:
		return a.isVisibleElemPath(varName)
	default:
		return false
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

	fieldNames := a.arraySourceFieldNamesFromValue(value)
	if len(fieldNames) == 0 {
		return nil // Can't determine field name, skip validation
	}

	for _, fieldName := range fieldNames {
		fieldType := a.schema().GetFieldType(fieldName)
		if fieldType == "" {
			return nil // Field not in schema, skip validation (existence checked by DataOperator)
		}

		if !a.schema().IsArrayType(fieldName) {
			return fmt.Errorf("array operation on non-array field '%s' (type: %s)", fieldName, fieldType)
		}
	}

	return nil
}

func (a *ArrayOperator) arraySourceSchemaScopes(value interface{}) []string {
	scopes, _, ok := a.arraySourceSchemaScopeInfo(value)
	if ok {
		return scopes
	}
	return nil
}

func (a *ArrayOperator) arraySourceSchemaScopeInfo(value interface{}) ([]string, bool, bool) {
	if isEmptyArrayLiteral(value) {
		return nil, true, true
	}
	if fieldNames := a.arraySourceFieldNamesFromValue(value); len(fieldNames) > 0 {
		return fieldNames, false, true
	}
	operator, args, ok := arrayOperatorArgs(value)
	if !ok {
		return nil, false, false
	}

	switch operator {
	case OpFilter:
		if len(args) != binaryArrayOperatorArgCount {
			return nil, false, false
		}
		return a.arraySourceSchemaScopeInfo(args[arraySourceArgIndex])
	case OpMap:
		if len(args) != binaryArrayOperatorArgCount || !isIdentityElementMapExpression(args[arrayExpressionArgIndex]) {
			return nil, false, false
		}
		return a.arraySourceSchemaScopeInfo(args[arraySourceArgIndex])
	case OpMerge:
		return a.mergeArraySourceSchemaScopes(args)
	case OpIf:
		return a.ifArraySourceSchemaScope(args)
	case OpOr:
		return a.logicalArraySourceSchemaScope(OpOr, args)
	case OpAnd:
		return a.logicalArraySourceSchemaScope(OpAnd, args)
	default:
		return nil, false, false
	}
}

func (a *ArrayOperator) arraySourceFieldNamesFromValue(value interface{}) []string {
	if pv, ok := value.(ProcessedValue); ok && pv.IsSQL && pv.IsField {
		return singleSchemaScope(pv.FieldName)
	}
	if varExpr, ok := value.(map[string]interface{}); ok {
		if varName, hasVar := varExpr[OpVar]; hasVar {
			if scoped := a.scopedFieldNamesFromVarExpr(varName); len(scoped) > 0 {
				return scoped
			}
			return singleSchemaScope(a.extractFieldName(varName))
		}
	}
	return nil
}

func isIdentityElementMapExpression(expr interface{}) bool {
	exprMap, ok := expr.(map[string]interface{})
	if !ok || len(exprMap) != 1 {
		return false
	}
	varName, ok := exprMap[OpVar]
	if !ok {
		return false
	}
	return varName == ""
}

func (a *ArrayOperator) mergeArraySourceSchemaScopes(args []interface{}) ([]string, bool, bool) {
	var scopes []string
	for _, arg := range args {
		argScopes, empty, ok := a.arraySourceSchemaScopeInfo(arg)
		if !ok {
			return nil, false, false
		}
		if empty {
			continue
		}
		scopes = append(scopes, argScopes...)
	}
	scopes = normalizeSchemaScopes(scopes)
	if len(scopes) == 0 {
		return nil, true, true
	}
	return scopes, false, true
}

func (a *ArrayOperator) ifArraySourceSchemaScope(args []interface{}) ([]string, bool, bool) {
	if len(args) < 2 {
		return nil, false, false
	}

	var candidates []interface{}
	pairLimit := len(args)
	hasElse := len(args)%2 == 1
	if hasElse {
		pairLimit = len(args) - 1
	}

	for i := 0; i < pairLimit; i += 2 {
		if truthy, known := staticJSONLogicTruthiness(args[i]); known {
			if truthy {
				return a.arraySourceSchemaScopeInfo(args[i+1])
			}
			continue
		}
		candidates = append(candidates, args[i+1])
	}

	if hasElse {
		candidates = append(candidates, args[len(args)-1])
	} else if len(candidates) > 0 {
		return nil, false, false
	}
	return a.mergeArraySourceSchemaScopes(candidates)
}

func (a *ArrayOperator) logicalArraySourceSchemaScope(operator string, args []interface{}) ([]string, bool, bool) {
	if len(args) == 0 {
		return nil, false, false
	}

	var candidates []interface{}
	for _, arg := range args {
		if truthy, known := staticJSONLogicTruthiness(arg); known {
			switch {
			case operator == OpOr && !truthy:
				continue
			case operator == OpOr && truthy:
				return a.arraySourceSchemaScopeInfo(arg)
			case operator == OpAnd && truthy:
				continue
			case operator == OpAnd && !truthy:
				return a.arraySourceSchemaScopeInfo(arg)
			}
		}
		candidates = append(candidates, arg)
	}
	return a.mergeArraySourceSchemaScopes(candidates)
}

func staticJSONLogicTruthiness(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case nil:
		return false, true
	case bool:
		return v, true
	case string:
		return v != "", true
	case []interface{}:
		return len(v) > 0, true
	case json.Number:
		f, err := strconv.ParseFloat(v.String(), 64)
		if err != nil {
			if errors.Is(err, strconv.ErrRange) {
				return f != 0, true
			}
			return false, false
		}
		return f != 0, true
	case int:
		return v != 0, true
	case int8:
		return v != 0, true
	case int16:
		return v != 0, true
	case int32:
		return v != 0, true
	case int64:
		return v != 0, true
	case uint:
		return v != 0, true
	case uint8:
		return v != 0, true
	case uint16:
		return v != 0, true
	case uint32:
		return v != 0, true
	case uint64:
		return v != 0, true
	case float32:
		f := float64(v)
		if math.IsNaN(f) {
			return false, true
		}
		return f != 0, true
	case float64:
		if math.IsNaN(v) {
			return false, true
		}
		return v != 0, true
	default:
		return false, false
	}
}

// extractFieldName extracts the field name from a var argument.
func (a *ArrayOperator) extractFieldName(varName interface{}) string {
	if pv, ok := varName.(ProcessedValue); ok && pv.IsSQL && pv.IsField {
		return pv.FieldName
	}
	if nameStr, ok := varName.(string); ok {
		return nameStr
	}
	if nameArr, ok := varName.([]interface{}); ok && len(nameArr) > 0 {
		if pv, ok := nameArr[0].(ProcessedValue); ok && pv.IsSQL && pv.IsField {
			return pv.FieldName
		}
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
		return scoped.withChildScope().ToSQL(operator, args)
	}
	return scoped.ToSQL(operator, args)
}

func (a *ArrayOperator) shouldUseRenderedSourceChildScope(op string, args []interface{}) bool {
	return a.shouldUseChildScope(op, args)
}

func (a *ArrayOperator) rewriteNestedArrayOuterScopeArgs(
	operator string,
	args []interface{},
	allowAccumulator bool,
	path string,
) ([]interface{}, error) {
	rewrittenArgs := make([]interface{}, len(args))
	copy(rewrittenArgs, args)

	opPath := tperrors.BuildPath(path, operator, -1)
	if len(rewrittenArgs) > 0 {
		rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(
			args[0],
			allowAccumulator,
			tperrors.BuildArrayPath(opPath, 0),
		)
		if err != nil {
			return nil, err
		}
		rewrittenArgs[0] = rewritten
	}
	if operator == OpReduce && len(rewrittenArgs) > arrayReduceInitialArgIndex {
		rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(
			args[arrayReduceInitialArgIndex],
			allowAccumulator,
			tperrors.BuildArrayPath(opPath, arrayReduceInitialArgIndex),
		)
		if err != nil {
			return nil, err
		}
		rewrittenArgs[arrayReduceInitialArgIndex] = rewritten
	}
	return rewrittenArgs, nil
}

func (a *ArrayOperator) rewriteNestedArrayOuterScopeArgsParam(
	operator string,
	args []interface{},
	allowAccumulator bool,
	path string,
) ([]interface{}, error) {
	rewrittenArgs := make([]interface{}, len(args))
	copy(rewrittenArgs, args)

	opPath := tperrors.BuildPath(path, operator, -1)
	if len(rewrittenArgs) > 0 {
		rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(
			args[0],
			allowAccumulator,
			tperrors.BuildArrayPath(opPath, 0),
		)
		if err != nil {
			return nil, err
		}
		rewrittenArgs[0] = rewritten
	}
	if operator == OpReduce && len(rewrittenArgs) > arrayReduceInitialArgIndex {
		rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(
			args[arrayReduceInitialArgIndex],
			allowAccumulator,
			tperrors.BuildArrayPath(opPath, arrayReduceInitialArgIndex),
		)
		if err != nil {
			return nil, err
		}
		rewrittenArgs[arrayReduceInitialArgIndex] = rewritten
	}
	return rewrittenArgs, nil
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
	arrayValue, err := a.valueToTypedSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid map array argument: %w", err)
	}
	if arrayValue.emptyArrayLiteral {
		return a.emptyArrayLiteralSQL()
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])

	valueScoped := a.withLambdaScope(arrayLambdaScopeElement).withSchemaScopes(sourceScopes).withValueSemantics(true)
	transformation, err := valueScoped.valueExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], false, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid map transformation argument: %w", err)
	}

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
	arrayValue, err := a.valueToTypedSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid filter array argument: %w", err)
	}
	if arrayValue.emptyArrayLiteral {
		return a.emptyArrayLiteralSQL()
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])

	// Second argument: truthiness expression - rewrite element vars before SQL generation
	condition, err := a.withLambdaScope(arrayLambdaScopeElement).
		withSchemaScopes(sourceScopes).
		truthinessExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid filter condition argument: %w", err)
	}

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
	initialValue, err := a.valueToTypedSQLAtPath(args[arrayReduceInitialArgIndex], a.argPath(arrayReduceInitialArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce initial argument: %w", err)
	}
	initial := initialValue.sql
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return initial, nil
	}

	// First argument: array
	arrayValue, err := a.valueToTypedSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce array argument: %w", err)
	}
	if arrayValue.emptyArrayLiteral {
		return initial, nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])

	// Second argument: reducer expression
	reducerExpr := args[arrayExpressionArgIndex]

	alias := a.elemAlias()

	// Check for common reduction patterns and optimize
	reduceScoped := a.withLambdaScope(arrayLambdaScopeReduce).withSchemaScopes(sourceScopes)
	if pattern := reduceScoped.detectAggregatePattern(reducerExpr); pattern != nil {
		// Build the element reference: "elem" or "elem.field" if field suffix exists
		elemRef, quoteErr := a.quoteArrayScopePath(alias, pattern.fieldSuffix)
		if quoteErr != nil {
			return "", quoteErr
		}

		// Generate optimized aggregate SQL based on dialect
		switch a.getDialect() {
		case dialect.DialectClickHouse:
			// ClickHouse: For field access, we need arrayMap first to extract the field
			aggregateInput := array
			if pattern.fieldSuffix != "" {
				mappedRef, quoteErr := a.quoteArrayScopePath("x", pattern.fieldSuffix)
				if quoteErr != nil {
					return "", quoteErr
				}
				aggregateInput = fmt.Sprintf("arrayMap(x -> %s, %s)", mappedRef, array)
			}
			aggregateSQL := fmt.Sprintf("arrayReduce('%s', %s)", strings.ToLower(pattern.function), aggregateInput)
			return renderReduceAggregateResult(pattern.function, initial, aggregateSQL, true, array), nil
		case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
			// Standard SQL: aggregate the array once and combine it with the
			// initial accumulator according to the reducer operator.
			aggregateSQL := fmt.Sprintf("(SELECT %s(%s) FROM UNNEST(%s) AS %s)", pattern.function, elemRef, array, alias)
			return renderReduceAggregateResult(pattern.function, initial, aggregateSQL, false, ""), nil
		}
		// Fallback for any future dialects
		aggregateSQL := fmt.Sprintf("(SELECT %s(%s) FROM UNNEST(%s) AS %s)", pattern.function, elemRef, array, alias)
		return renderReduceAggregateResult(pattern.function, initial, aggregateSQL, false, ""), nil
	}

	// General case: parse the reducer in official reduce scope, then
	// substitute accumulator with the initial value. The order matters:
	// accumulator substitution must happen LAST so initial values containing
	// the word "accumulator" are treated as SQL literals.
	valueScoped := reduceScoped.withValueSemantics(true).withAccumulatorType(initialValue.typ)
	reducerWithElem, err := valueScoped.expressionToSQLWithContextAndPath(reducerExpr, true, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce expression: %w", err)
	}
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
		if fieldSuffix == "" {
			return "", false
		}
		if _, err := a.resolveScopedFieldName(fieldSuffix); err != nil {
			return "", false
		}
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
	arrayValue, err := a.valueToTypedSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all array argument: %w", err)
	}
	if arrayValue.emptyArrayLiteral {
		return "FALSE", nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])

	// Second argument: truthiness expression - rewrite element vars before SQL generation
	condition, err := a.withLambdaScope(arrayLambdaScopeElement).
		withSchemaScopes(sourceScopes).
		truthinessExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all condition argument: %w", err)
	}

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
	arrayValue, err := a.valueToTypedSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some array argument: %w", err)
	}
	if arrayValue.emptyArrayLiteral {
		return "FALSE", nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])

	// Second argument: truthiness expression - rewrite element vars before SQL generation
	condition, err := a.withLambdaScope(arrayLambdaScopeElement).
		withSchemaScopes(sourceScopes).
		truthinessExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some condition argument: %w", err)
	}

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
	arrayValue, err := a.valueToTypedSQLAtPath(args[arraySourceArgIndex], a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none array argument: %w", err)
	}
	if arrayValue.emptyArrayLiteral {
		return "TRUE", nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])

	// Second argument: truthiness expression - rewrite element vars before SQL generation
	condition, err := a.withLambdaScope(arrayLambdaScopeElement).
		withSchemaScopes(sourceScopes).
		truthinessExpressionToSQLWithContextAndPath(args[arrayExpressionArgIndex], a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none condition argument: %w", err)
	}

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
		arrayValue, err := a.valueToTypedSQLAtPath(arg, a.argPath(i))
		if err != nil {
			return "", fmt.Errorf("invalid merge array argument %d: %w", i, err)
		}
		if arrayValue.emptyArrayLiteral {
			continue
		}
		arrays = append(arrays, arrayValue.sql)
	}
	return a.renderMergeSQL(arrays)
}

// valueToSQL converts a value to SQL, handling var expressions, arrays, and literals.
func (a *ArrayOperator) valueToSQL(value interface{}) (string, error) {
	return a.valueToSQLAtPath(value, a.currentPath())
}

func (a *ArrayOperator) valueExpressionToSQLWithContextAndPath(expr interface{}, allowAccumulator bool, path string) (string, error) {
	res, err := a.valueExpressionResultWithContextAndPath(expr, allowAccumulator, path)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) valueExpressionResultWithContextAndPath(expr interface{}, allowAccumulator bool, path string) (OperatorResult, error) {
	if a.config == nil || !a.config.HasValueExpressionParser() {
		sql, err := a.expressionToSQLWithContextAndPath(expr, allowAccumulator, path)
		if err != nil {
			return OperatorResult{}, err
		}
		return a.localValueExpressionResult(expr, sql), nil
	}
	if a.shouldParseScopedArrayExpressionLocally(expr) {
		sql, err := a.expressionToSQLWithContextAndPath(expr, allowAccumulator, path)
		if err != nil {
			return OperatorResult{}, err
		}
		return a.localValueExpressionResult(expr, sql), nil
	}
	rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(expr, allowAccumulator, path)
	if err != nil {
		return OperatorResult{}, err
	}
	res, err := a.config.ParseValueExpression(rewritten, path)
	if err != nil {
		return OperatorResult{}, err
	}
	return res, nil
}

func (a *ArrayOperator) localValueExpressionResult(expr interface{}, sql string) OperatorResult {
	if isPredicateArrayExpression(expr) {
		return ValueSQL(PredicateValueSQL(sql), ExpressionTypeBoolean)
	}
	return ValueSQL(sql, a.inferValueExpressionType(expr))
}

func isPredicateArrayExpression(expr interface{}) bool {
	operator, _, ok := arrayOperatorArgs(expr)
	if !ok {
		return false
	}
	switch operator {
	case OpAll, OpSome, OpNone:
		return true
	default:
		return false
	}
}

func (a *ArrayOperator) predicateExpressionToSQLWithContextAndPath(expr interface{}, path string) (string, error) {
	if a.config == nil || !a.config.HasPredicateExpressionParser() {
		return a.expressionToSQLWithContextAndPath(expr, false, path)
	}
	if a.shouldParseScopedArrayExpressionLocally(expr) {
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

func (a *ArrayOperator) truthinessExpressionToSQLWithContextAndPath(expr interface{}, path string) (string, error) {
	if a.config == nil || !a.config.HasTruthinessExpressionParser() {
		return a.predicateExpressionToSQLWithContextAndPath(expr, path)
	}
	if a.shouldParseScopedArrayExpressionLocally(expr) {
		sql, err := a.expressionToSQLWithContextAndPath(expr, false, path)
		if err != nil {
			return "", err
		}
		return a.localTruthinessExpressionSQL(expr, sql, path)
	}
	rewritten, err := a.rewriteScopedVarsForOperatorWithContextAndPath(expr, false, path)
	if err != nil {
		return "", err
	}
	return a.config.ParseTruthinessExpression(rewritten, path)
}

func (a *ArrayOperator) localTruthinessExpressionSQL(expr interface{}, sql, path string) (string, error) {
	if isPredicateArrayExpression(expr) {
		return sql, nil
	}
	result := a.localValueExpressionResult(expr, sql)
	if result.Kind == ExpressionKindPredicate {
		return result.SQL, nil
	}
	switch result.Type {
	case ExpressionTypeNull:
		return "FALSE", nil
	case ExpressionTypeBoolean:
		return fmt.Sprintf("%s IS TRUE", result.SQL), nil
	case ExpressionTypeString:
		return fmt.Sprintf("(%s IS NOT NULL AND %s != '')", result.SQL, result.SQL), nil
	case ExpressionTypeNumber:
		return fmt.Sprintf("(%s IS NOT NULL AND %s != 0)", result.SQL, result.SQL), nil
	case ExpressionTypeArray:
		lengthCheck := a.arrayLengthSQL(result.SQL)
		return fmt.Sprintf("(%s IS NOT NULL AND %s > 0)", result.SQL, lengthCheck), nil
	case ExpressionTypeUnknown:
		return "", tperrors.New(
			tperrors.ErrInvalidExpressionContext,
			"",
			path,
			"truthiness requires a statically known value type for locally scoped array expression",
		)
	default:
		return "", tperrors.New(
			tperrors.ErrInvalidExpressionContext,
			"",
			path,
			"unsupported locally scoped array expression type",
		)
	}
}

func (a *ArrayOperator) valueToSQLAtPath(value interface{}, path string) (string, error) {
	result, err := a.valueToTypedSQLAtPath(value, path)
	if err != nil {
		return "", err
	}
	return result.sql, nil
}

func (a *ArrayOperator) valueToTypedSQLAtPath(value interface{}, path string) (typedValueSQL, error) {
	// Handle ProcessedValue (pre-processed SQL from parser)
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			if pv.HasExpressionInfo {
				return typedValueSQL{sql: pv.Value, typ: expressionTypeFromResultKind(pv.Kind, pv.Type)}, nil
			}
			return typedValueSQL{sql: pv.Value, typ: ExpressionTypeUnknown}, nil
		}
		// It's a literal, convert it
		sql, err := a.dataOp.valueToSQL(pv.Value)
		if err != nil {
			return typedValueSQL{}, err
		}
		return typedValueSQL{sql: sql, typ: inferLiteralValueExpressionType(pv.Value)}, nil
	}

	// Handle complex expressions (operators)
	if expr, ok := value.(map[string]interface{}); ok {
		// Check if it's a var expression
		if varExpr, hasVar := expr[OpVar]; hasVar {
			if sql, handled, err := a.arrayInternalVarToSQL(varExpr); handled || err != nil {
				if err != nil {
					return typedValueSQL{}, err
				}
				return typedValueSQL{sql: sql, typ: a.inferValueExpressionType(value)}, nil
			}
			sql, err := a.dataOp.ToSQL(OpVar, []interface{}{varExpr})
			if err != nil {
				return typedValueSQL{}, err
			}
			return typedValueSQL{sql: sql, typ: a.inferValueExpressionType(value)}, nil
		}
		// Otherwise, it's a complex value expression.
		res, err := a.valueExpressionResultWithContextAndPath(value, false, path)
		if err != nil {
			return typedValueSQL{}, err
		}
		return typedValueSQL{
			sql:               res.SQL,
			typ:               expressionTypeFromResultKind(res.Kind, res.Type),
			emptyArrayLiteral: res.EmptyArrayLiteral,
		}, nil
	}

	// Handle arrays
	if arr, ok := value.([]interface{}); ok {
		elements := make([]string, len(arr))
		for i, elem := range arr {
			elementSQL, err := a.valueExpressionToSQLWithContextAndPath(elem, false, tperrors.BuildArrayPath(path, i))
			if err != nil {
				return typedValueSQL{}, fmt.Errorf("invalid array element %d: %w", i, err)
			}
			elements[i] = elementSQL
		}
		sql, err := a.arrayLiteral(elements)
		if err != nil {
			return typedValueSQL{}, err
		}
		return typedValueSQL{sql: sql, typ: ExpressionTypeArray, emptyArrayLiteral: len(arr) == 0}, nil
	}

	// Handle primitive values
	sql, err := a.dataOp.valueToSQL(value)
	if err != nil {
		return typedValueSQL{}, err
	}
	return typedValueSQL{sql: sql, typ: inferLiteralValueExpressionType(value)}, nil
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
		if len(varExpr) != 1 {
			return "", tperrors.NewMultipleKeys(path)
		}
		if varName, hasVar := varExpr[OpVar]; hasVar {
			if allowAccumulator {
				if rewritten, handled, err := a.rewriteAccumulatorVar(varName); handled || err != nil {
					return rewritten.Value, err
				}
			}
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
						var rewriteErr error
						nestedArgs, rewriteErr = a.rewriteNestedArrayOuterScopeArgs(operator, arr, allowAccumulator, path)
						if rewriteErr != nil {
							return "", rewriteErr
						}
						target = a.withChildScope()
					}
					target = target.withValueScope(true)
					target = target.withValueSemantics(false)
					nestedPath := tperrors.BuildPath(path, operator, -1)
					return target.withPath(nestedPath).ToSQL(operator, nestedArgs)
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

// mapArrayScopeVar maps var names according to the active JSONLogic lambda
// scope. Element lambdas resolve bare fields against the current array element;
// reduce lambdas only expose the official current/accumulator bindings.
func (a *ArrayOperator) mapArrayScopeVar(varName string) (string, bool, error) {
	switch a.lambdaScope {
	case arrayLambdaScopeElement:
		return a.mapElementScopeVar(varName)
	case arrayLambdaScopeReduce:
		return a.mapReduceScopeVar(varName)
	case arrayLambdaScopeNone:
		if a.isVisibleElemPath(varName) {
			scope, fieldName := a.visibleElemScopeAndFieldName(varName)
			if fieldName != "" {
				if _, err := a.resolveFieldInScope(scope, fieldName); err != nil {
					return "", true, err
				}
			}
			quoted, err := a.quoteArrayScopeIdentifier(varName)
			if err != nil {
				return "", true, err
			}
			return quoted, true, nil
		}
		return "", false, nil
	default:
		return "", false, nil
	}
}

func (a *ArrayOperator) mapElementScopeVar(varName string) (string, bool, error) {
	if isUnsupportedElementScopeVar(varName) {
		return "", true, unsupportedArrayScopeVarError(varName)
	}
	if varName == "" {
		return a.elemAlias(), true, nil
	}
	if _, err := a.resolveScopedFieldName(varName); err != nil {
		return "", true, err
	}
	quoted, err := a.quoteArrayScopePath(a.elemAlias(), varName)
	if err != nil {
		return "", true, err
	}
	return quoted, true, nil
}

func (a *ArrayOperator) mapReduceScopeVar(varName string) (string, bool, error) {
	if isUnsupportedReduceScopeVar(varName) {
		return "", true, unsupportedArrayScopeVarError(varName)
	}
	switch {
	case varName == CurrentVar:
		return a.elemAlias(), true, nil
	case strings.HasPrefix(varName, CurrentVar+"."):
		suffix := strings.TrimPrefix(varName, CurrentVar+".")
		if suffix == "" {
			return "", true, fmt.Errorf("unsupported reduce-scope variable %q; use %q, %q.<field>, or %q",
				varName, CurrentVar, CurrentVar, AccumulatorVar)
		}
		if _, err := a.resolveScopedFieldName(suffix); err != nil {
			return "", true, err
		}
		quoted, err := a.quoteArrayScopePath(a.elemAlias(), suffix)
		if err != nil {
			return "", true, err
		}
		return quoted, true, nil
	case varName == "":
		return "", true, fmt.Errorf("unsupported reduce-scope variable %q; use %q or %q", varName, CurrentVar, AccumulatorVar)
	default:
		return "", true, fmt.Errorf("unsupported reduce-scope variable %q; use %q, %q.<field>, or %q",
			varName, CurrentVar, CurrentVar, AccumulatorVar)
	}
}

func isUnsupportedElementScopeVar(varName string) bool {
	return strings.HasPrefix(varName, ".") ||
		strings.HasPrefix(varName, ItemVar+".") ||
		strings.HasPrefix(varName, CurrentVar+".") ||
		isInternalElemAliasPath(varName)
}

func isUnsupportedReduceScopeVar(varName string) bool {
	return strings.HasPrefix(varName, ".") ||
		strings.HasPrefix(varName, ItemVar+".") ||
		isInternalElemAliasPath(varName)
}

func isInternalElemAliasPath(varName string) bool {
	if !strings.HasPrefix(varName, ElemVar) {
		return false
	}
	rest := varName[len(ElemVar):]
	for len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
		rest = rest[1:]
	}
	return strings.HasPrefix(rest, ".")
}

func unsupportedArrayScopeVarError(varName string) error {
	return fmt.Errorf("unsupported array-scope variable %q; use bare field names relative to the current element", varName)
}

func (a *ArrayOperator) scopedFieldNameFromVarExpr(varExpr interface{}) string {
	fieldNames := a.scopedFieldNamesFromVarExpr(varExpr)
	if len(fieldNames) == 0 {
		return ""
	}
	return fieldNames[0]
}

func (a *ArrayOperator) scopedFieldNamesFromVarExpr(varExpr interface{}) []string {
	switch v := varExpr.(type) {
	case string:
		return a.scopedFieldNamesForVar(v)
	case []interface{}:
		if len(v) == 0 {
			return nil
		}
		switch first := v[0].(type) {
		case string:
			return a.scopedFieldNamesForVar(first)
		case ProcessedValue:
			if first.IsSQL && first.IsField {
				return singleSchemaScope(first.FieldName)
			}
		}
	case ProcessedValue:
		if v.IsSQL && v.IsField {
			return singleSchemaScope(v.FieldName)
		}
	}
	return nil
}

func (a *ArrayOperator) scopedFieldNameForVar(varName string) string {
	fieldNames := a.scopedFieldNamesForVar(varName)
	if len(fieldNames) == 0 {
		return ""
	}
	return fieldNames[0]
}

func (a *ArrayOperator) scopedFieldNamesForVar(varName string) []string {
	switch a.lambdaScope {
	case arrayLambdaScopeElement:
		if varName == "" || isUnsupportedElementScopeVar(varName) {
			return nil
		}
		return a.resolveScopedFieldNames(varName)
	case arrayLambdaScopeReduce:
		if varName == "" || varName == CurrentVar || varName == AccumulatorVar || isUnsupportedReduceScopeVar(varName) {
			return nil
		}
		if strings.HasPrefix(varName, CurrentVar+".") {
			suffix := strings.TrimPrefix(varName, CurrentVar+".")
			if suffix != "" {
				return a.resolveScopedFieldNames(suffix)
			}
		}
	case arrayLambdaScopeNone:
		scope, fieldName := a.visibleElemScopeAndFieldName(varName)
		if fieldName == "" {
			return nil
		}
		resolved, err := a.resolveFieldInScope(scope, fieldName)
		if err != nil {
			return nil
		}
		return singleSchemaScope(resolved)
	}
	return nil
}

func (a *ArrayOperator) visibleElemScopeAndFieldName(varName string) (string, string) {
	for i := len(a.visibleElems) - 1; i >= 0; i-- {
		alias := a.visibleElems[i]
		prefix := alias + "."
		scope := ""
		if i < len(a.visibleScopes) {
			scope = a.visibleScopes[i]
		}
		if varName == alias {
			return scope, ""
		}
		if strings.HasPrefix(varName, prefix) {
			return scope, strings.TrimPrefix(varName, prefix)
		}
	}
	return "", ""
}

func (a *ArrayOperator) rewriteScopedMissingFields(operator string, opArgs interface{}, allowAccumulator bool) (interface{}, bool, error) {
	switch operator {
	case OpMissing:
		return a.rewriteScopedMissingFieldOperand(opArgs, allowAccumulator)
	case OpMissingSome:
		args, ok := opArgs.([]interface{})
		if !ok || len(args) != 2 {
			return nil, false, nil
		}
		fields, ok := args[1].([]interface{})
		if !ok {
			return nil, false, nil
		}
		rewrittenFields, changed, err := a.rewriteScopedMissingFieldList(fields, allowAccumulator)
		if err != nil || !changed {
			return nil, changed, err
		}
		rewrittenArgs := make([]interface{}, len(args))
		copy(rewrittenArgs, args)
		rewrittenArgs[1] = rewrittenFields
		return rewrittenArgs, true, nil
	default:
		return nil, false, nil
	}
}

func (a *ArrayOperator) rewriteScopedMissingFieldOperand(field interface{}, allowAccumulator bool) (interface{}, bool, error) {
	switch v := field.(type) {
	case string:
		return a.rewriteScopedMissingFieldName(v, allowAccumulator)
	case []interface{}:
		return a.rewriteScopedMissingFieldList(v, allowAccumulator)
	default:
		return nil, false, nil
	}
}

func (a *ArrayOperator) rewriteScopedMissingFieldList(fields []interface{}, allowAccumulator bool) ([]interface{}, bool, error) {
	rewrittenFields := make([]interface{}, len(fields))
	changed := false
	for i, field := range fields {
		fieldName, ok := field.(string)
		if !ok {
			rewrittenFields[i] = field
			continue
		}
		rewritten, fieldChanged, err := a.rewriteScopedMissingFieldName(fieldName, allowAccumulator)
		if err != nil {
			return nil, false, err
		}
		rewrittenFields[i] = rewritten
		changed = changed || fieldChanged
	}
	if !changed {
		return nil, false, nil
	}
	return rewrittenFields, true, nil
}

func (a *ArrayOperator) rewriteScopedMissingFieldName(fieldName string, allowAccumulator bool) (interface{}, bool, error) {
	if allowAccumulator && fieldName == AccumulatorVar {
		return a.accumulatorSQLResult(), true, nil
	}
	mapped, handled, err := a.mapArrayScopeVar(fieldName)
	if err != nil {
		return nil, true, err
	}
	if !handled {
		return fieldName, false, nil
	}
	return a.scopedSQLFieldResult(mapped, a.scopedFieldNameForVar(fieldName)), true, nil
}

func (a *ArrayOperator) rewriteAccumulatorVar(varExpr interface{}) (ProcessedValue, bool, error) {
	if varName, ok := varExpr.(string); ok {
		if varName == AccumulatorVar {
			return a.accumulatorSQLResult(), true, nil
		}
		return ProcessedValue{}, false, nil
	}

	arr, ok := varExpr.([]interface{})
	if !ok {
		return ProcessedValue{}, false, nil
	}
	if len(arr) == 0 {
		return ProcessedValue{}, false, nil
	}
	if err := validateVarArrayMaxEntries(arr); err != nil {
		return ProcessedValue{}, true, err
	}
	varName, ok := arr[0].(string)
	if !ok || varName != AccumulatorVar {
		return ProcessedValue{}, false, nil
	}
	if len(arr) == 1 {
		return a.accumulatorSQLResult(), true, nil
	}
	defaultSQL, err := a.dataOp.valueToSQL(arr[1])
	if err != nil {
		return ProcessedValue{}, true, fmt.Errorf("invalid default value: %w", err)
	}
	return a.accumulatorSQLResultWithSQL(fmt.Sprintf("COALESCE(%s, %s)", AccumulatorVar, defaultSQL)), true, nil
}

func (a *ArrayOperator) rewriteAccumulatorVarParam(varExpr interface{}) (interface{}, bool, error) {
	if varName, ok := varExpr.(string); ok {
		if varName == AccumulatorVar {
			return a.accumulatorSQLResult(), true, nil
		}
		return nil, false, nil
	}

	arr, ok := varExpr.([]interface{})
	if !ok {
		return nil, false, nil
	}
	if len(arr) == 0 {
		return nil, false, nil
	}
	if err := validateVarArrayMaxEntries(arr); err != nil {
		return nil, true, err
	}
	varName, ok := arr[0].(string)
	if !ok || varName != AccumulatorVar {
		return nil, false, nil
	}
	if len(arr) == 1 {
		return a.accumulatorSQLResult(), true, nil
	}
	rewritten := make([]interface{}, len(arr))
	copy(rewritten, arr)
	rewritten[0] = a.accumulatorSQLResult()
	return map[string]interface{}{OpVar: rewritten}, true, nil
}

// arrayScopeVarToSQL resolves lambda-scoped var references before schema
// validation. Element lambdas are relative to the current element; reduce
// lambdas expose only JSONLogic's current/accumulator bindings.
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

// arrayInternalVarToSQL resolves vars in array-produced value expressions.
// Active lambdas delegate to arrayScopeVarToSQL; outside lambdas, only already
// generated elem aliases are treated as array-scoped.
func (a *ArrayOperator) arrayInternalVarToSQL(varExpr interface{}) (string, bool, error) {
	if a.lambdaScope != arrayLambdaScopeNone {
		return a.arrayScopeVarToSQL(varExpr)
	}
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
		if len(e) != 1 {
			return nil, tperrors.NewMultipleKeys(path)
		}
		if varName, hasVar := e[OpVar]; hasVar {
			if allowAccumulator {
				if rewritten, handled, err := a.rewriteAccumulatorVar(varName); handled || err != nil {
					if err != nil {
						return nil, err
					}
					return rewritten, nil
				}
			}
			if sql, handled, err := a.arrayScopeVarToSQL(varName); handled || err != nil {
				if err != nil {
					return nil, err
				}
				return a.scopedSQLFieldResultFromVarExpr(sql, varName), nil
			}
			return e, nil
		}
		for opName, opArgs := range e {
			if rewrittenArgs, handled, err := a.rewriteScopedMissingFields(opName, opArgs, allowAccumulator); handled || err != nil {
				if err != nil {
					return nil, err
				}
				return map[string]interface{}{opName: rewrittenArgs}, nil
			}
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
		return scoped.withChildScope().ToSQLParam(operator, args, pc)
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
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid map array argument: %w", err)
	}
	if arrayValue.emptyArrayLiteral {
		return a.emptyArrayLiteralSQL()
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])
	valueScoped := a.withLambdaScope(arrayLambdaScopeElement).withSchemaScopes(sourceScopes).withValueSemantics(true)
	transformation, err := valueScoped.valueExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, false, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid map transformation argument: %w", err)
	}
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
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid filter array argument: %w", err)
	}
	if arrayValue.emptyArrayLiteral {
		return a.emptyArrayLiteralSQL()
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])
	condition, err := a.withLambdaScope(arrayLambdaScopeElement).
		withSchemaScopes(sourceScopes).
		truthinessExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid filter condition argument: %w", err)
	}
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
	initialValue, err := a.valueToTypedSQLParamAtPath(args[arrayReduceInitialArgIndex], pc, a.argPath(arrayReduceInitialArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce initial argument: %w", err)
	}
	initial := initialValue.sql
	if isEmptyArrayLiteral(args[arraySourceArgIndex]) {
		return initial, nil
	}
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce array argument: %w", err)
	}
	if arrayValue.emptyArrayLiteral {
		return initial, nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])
	reducerExpr := args[arrayExpressionArgIndex]
	alias := a.elemAlias()

	reduceScoped := a.withLambdaScope(arrayLambdaScopeReduce).withSchemaScopes(sourceScopes)
	if pattern := reduceScoped.detectAggregatePattern(reducerExpr); pattern != nil {
		elemRef, quoteErr := a.quoteArrayScopePath(alias, pattern.fieldSuffix)
		if quoteErr != nil {
			return "", quoteErr
		}

		switch a.getDialect() {
		case dialect.DialectClickHouse:
			aggregateInput := array
			if pattern.fieldSuffix != "" {
				mappedRef, quoteErr := a.quoteArrayScopePath("x", pattern.fieldSuffix)
				if quoteErr != nil {
					return "", quoteErr
				}
				aggregateInput = fmt.Sprintf("arrayMap(x -> %s, %s)", mappedRef, array)
			}
			aggregateSQL := fmt.Sprintf("arrayReduce('%s', %s)", strings.ToLower(pattern.function), aggregateInput)
			return renderReduceAggregateResult(pattern.function, initial, aggregateSQL, true, array), nil
		case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
			aggregateSQL := fmt.Sprintf("(SELECT %s(%s) FROM UNNEST(%s) AS %s)", pattern.function, elemRef, array, alias)
			return renderReduceAggregateResult(pattern.function, initial, aggregateSQL, false, ""), nil
		}
		aggregateSQL := fmt.Sprintf("(SELECT %s(%s) FROM UNNEST(%s) AS %s)", pattern.function, elemRef, array, alias)
		return renderReduceAggregateResult(pattern.function, initial, aggregateSQL, false, ""), nil
	}

	valueScoped := reduceScoped.withValueSemantics(true).withAccumulatorType(initialValue.typ)
	reducerWithElem, err := valueScoped.expressionToSQLParamWithContextAndPath(reducerExpr, pc, true, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid reduce expression: %w", err)
	}
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
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all array argument: %w", err)
	}
	if arrayValue.emptyArrayLiteral {
		return "FALSE", nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])
	condition, err := a.withLambdaScope(arrayLambdaScopeElement).
		withSchemaScopes(sourceScopes).
		truthinessExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid all condition argument: %w", err)
	}

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
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some array argument: %w", err)
	}
	if arrayValue.emptyArrayLiteral {
		return "FALSE", nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])
	condition, err := a.withLambdaScope(arrayLambdaScopeElement).
		withSchemaScopes(sourceScopes).
		truthinessExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid some condition argument: %w", err)
	}
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
	arrayValue, err := a.valueToTypedSQLParamAtPath(args[arraySourceArgIndex], pc, a.argPath(arraySourceArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none array argument: %w", err)
	}
	if arrayValue.emptyArrayLiteral {
		return "TRUE", nil
	}
	array := arrayValue.sql
	sourceScopes := a.arraySourceSchemaScopes(args[arraySourceArgIndex])
	condition, err := a.withLambdaScope(arrayLambdaScopeElement).
		withSchemaScopes(sourceScopes).
		truthinessExpressionToSQLParamWithContextAndPath(args[arrayExpressionArgIndex], pc, a.argPath(arrayExpressionArgIndex))
	if err != nil {
		return "", fmt.Errorf("invalid none condition argument: %w", err)
	}
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
		arrayValue, err := a.valueToTypedSQLParamAtPath(arg, pc, a.argPath(i))
		if err != nil {
			return "", fmt.Errorf("invalid merge array argument %d: %w", i, err)
		}
		if arrayValue.emptyArrayLiteral {
			continue
		}
		arrays = append(arrays, arrayValue.sql)
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
	res, err := a.valueExpressionResultParamWithContextAndPath(expr, pc, allowAccumulator, path)
	if err != nil {
		return "", err
	}
	return res.SQL, nil
}

func (a *ArrayOperator) valueExpressionResultParamWithContextAndPath(
	expr interface{},
	pc *params.ParamCollector,
	allowAccumulator bool,
	path string,
) (OperatorResult, error) {
	if a.config == nil || !a.config.HasParamValueExpressionParser() {
		sql, err := a.expressionToSQLParamWithContextAndPath(expr, pc, allowAccumulator, path)
		if err != nil {
			return OperatorResult{}, err
		}
		return a.localValueExpressionResult(expr, sql), nil
	}
	if a.shouldParseScopedArrayExpressionLocally(expr) {
		sql, err := a.expressionToSQLParamWithContextAndPath(expr, pc, allowAccumulator, path)
		if err != nil {
			return OperatorResult{}, err
		}
		return a.localValueExpressionResult(expr, sql), nil
	}
	rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(expr, allowAccumulator, path)
	if err != nil {
		return OperatorResult{}, err
	}
	res, err := a.config.ParseValueExpressionParam(rewritten, path, pc)
	if err != nil {
		return OperatorResult{}, err
	}
	return res, nil
}

func (a *ArrayOperator) predicateExpressionToSQLParamWithContextAndPath(
	expr interface{},
	pc *params.ParamCollector,
	path string,
) (string, error) {
	if a.config == nil || !a.config.HasParamPredicateExpressionParser() {
		return a.expressionToSQLParamWithContextAndPath(expr, pc, false, path)
	}
	if a.shouldParseScopedArrayExpressionLocally(expr) {
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

func (a *ArrayOperator) truthinessExpressionToSQLParamWithContextAndPath(
	expr interface{},
	pc *params.ParamCollector,
	path string,
) (string, error) {
	if a.config == nil || !a.config.HasParamTruthinessExpressionParser() {
		return a.predicateExpressionToSQLParamWithContextAndPath(expr, pc, path)
	}
	if a.shouldParseScopedArrayExpressionLocally(expr) {
		sql, err := a.expressionToSQLParamWithContextAndPath(expr, pc, false, path)
		if err != nil {
			return "", err
		}
		return a.localTruthinessExpressionSQL(expr, sql, path)
	}
	rewritten, err := a.rewriteScopedVarsForOperatorParamWithContextAndPath(expr, false, path)
	if err != nil {
		return "", err
	}
	return a.config.ParseTruthinessExpressionParam(rewritten, path, pc)
}

func (a *ArrayOperator) valueToSQLParamAtPath(value interface{}, pc *params.ParamCollector, path string) (string, error) {
	result, err := a.valueToTypedSQLParamAtPath(value, pc, path)
	if err != nil {
		return "", err
	}
	return result.sql, nil
}

func (a *ArrayOperator) valueToTypedSQLParamAtPath(value interface{}, pc *params.ParamCollector, path string) (typedValueSQL, error) {
	if pv, ok := value.(ProcessedValue); ok {
		if pv.IsSQL {
			if pv.HasExpressionInfo {
				return typedValueSQL{sql: pv.Value, typ: expressionTypeFromResultKind(pv.Kind, pv.Type)}, nil
			}
			return typedValueSQL{sql: pv.Value, typ: ExpressionTypeUnknown}, nil
		}
		sql, err := a.dataOp.valueToSQLParam(pv.Value, pc)
		if err != nil {
			return typedValueSQL{}, err
		}
		return typedValueSQL{sql: sql, typ: inferLiteralValueExpressionType(pv.Value)}, nil
	}

	if expr, ok := value.(map[string]interface{}); ok {
		if varExpr, hasVar := expr[OpVar]; hasVar {
			if sql, handled, err := a.arrayInternalVarToSQLParam(varExpr, pc); handled || err != nil {
				if err != nil {
					return typedValueSQL{}, err
				}
				return typedValueSQL{sql: sql, typ: a.inferValueExpressionType(value)}, nil
			}
			sql, err := a.dataOp.ToSQLParam(OpVar, []interface{}{varExpr}, pc)
			if err != nil {
				return typedValueSQL{}, err
			}
			return typedValueSQL{sql: sql, typ: a.inferValueExpressionType(value)}, nil
		}
		res, err := a.valueExpressionResultParamWithContextAndPath(value, pc, false, path)
		if err != nil {
			return typedValueSQL{}, err
		}
		return typedValueSQL{
			sql:               res.SQL,
			typ:               expressionTypeFromResultKind(res.Kind, res.Type),
			emptyArrayLiteral: res.EmptyArrayLiteral,
		}, nil
	}

	if arr, ok := value.([]interface{}); ok {
		elements := make([]string, len(arr))
		for i, elem := range arr {
			elementSQL, err := a.valueExpressionToSQLParamWithContextAndPath(elem, pc, false, tperrors.BuildArrayPath(path, i))
			if err != nil {
				return typedValueSQL{}, fmt.Errorf("invalid array element %d: %w", i, err)
			}
			elements[i] = elementSQL
		}
		sql, err := a.arrayLiteral(elements)
		if err != nil {
			return typedValueSQL{}, err
		}
		return typedValueSQL{sql: sql, typ: ExpressionTypeArray, emptyArrayLiteral: len(arr) == 0}, nil
	}

	sql, err := a.dataOp.valueToSQLParam(value, pc)
	if err != nil {
		return typedValueSQL{}, err
	}
	return typedValueSQL{sql: sql, typ: inferLiteralValueExpressionType(value)}, nil
}

func expressionTypeFromResultKind(kind ExpressionKind, typ ExpressionType) ExpressionType {
	if kind == ExpressionKindPredicate {
		return ExpressionTypeBoolean
	}
	return typ
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
		if len(varExpr) != 1 {
			return "", tperrors.NewMultipleKeys(path)
		}
		if varName, hasVar := varExpr[OpVar]; hasVar {
			if allowAccumulator {
				if rewritten, handled, err := a.rewriteAccumulatorVarParam(varName); handled || err != nil {
					if err != nil {
						return "", err
					}
					if pv, ok := rewritten.(ProcessedValue); ok && pv.IsSQL {
						return pv.Value, nil
					}
					if rewrittenVar, ok := rewritten.(map[string]interface{}); ok {
						return a.dataOp.ToSQLParam(OpVar, []interface{}{rewrittenVar[OpVar]}, pc)
					}
				}
			}
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
						var rewriteErr error
						nestedArgs, rewriteErr = a.rewriteNestedArrayOuterScopeArgsParam(operator, arr, allowAccumulator, path)
						if rewriteErr != nil {
							return "", rewriteErr
						}
						target = a.withChildScope()
					}
					target = target.withValueScope(true)
					target = target.withValueSemantics(false)
					nestedPath := tperrors.BuildPath(path, operator, -1)
					return target.withPath(nestedPath).ToSQLParam(operator, nestedArgs, pc)
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
			return a.scopedSQLFieldResult(mapped, a.scopedFieldNameForVar(varName)), true, nil
		}
		return nil, false, nil
	}

	if arr, ok := varExpr.([]interface{}); ok {
		if len(arr) == 0 {
			return nil, false, nil
		}
		if err := validateVarArrayMaxEntries(arr); err != nil {
			return nil, true, err
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
			return a.scopedSQLFieldResult(mapped, a.scopedFieldNameForVar(varName)), true, nil
		}
		newArr := make([]interface{}, len(arr))
		copy(newArr, arr)
		newArr[0] = a.scopedSQLFieldResult(mapped, a.scopedFieldNameForVar(varName))
		return map[string]interface{}{OpVar: newArr}, true, nil
	}

	return nil, false, nil
}

// arrayInternalVarToSQLParam is the parameterized variant of arrayInternalVarToSQL.
func (a *ArrayOperator) arrayInternalVarToSQLParam(varExpr interface{}, pc *params.ParamCollector) (string, bool, error) {
	if a.lambdaScope != arrayLambdaScopeNone {
		return a.arrayScopeVarToSQLParam(varExpr, pc)
	}
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
		if len(e) != 1 {
			return nil, tperrors.NewMultipleKeys(path)
		}
		if varName, hasVar := e[OpVar]; hasVar {
			if allowAccumulator {
				if rewritten, handled, err := a.rewriteAccumulatorVarParam(varName); handled || err != nil {
					if err != nil {
						return nil, err
					}
					return rewritten, nil
				}
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
			if rewrittenArgs, handled, err := a.rewriteScopedMissingFields(opName, opArgs, allowAccumulator); handled || err != nil {
				if err != nil {
					return nil, err
				}
				return map[string]interface{}{opName: rewrittenArgs}, nil
			}
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
