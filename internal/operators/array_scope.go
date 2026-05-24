package operators

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
)

func (a *ArrayOperator) elemAlias() string {
	if a == nil || a.scopeDepth == 0 {
		return ElemVar
	}
	return ElemVar + strconv.Itoa(a.scopeDepth)
}

func (a *ArrayOperator) clone() *ArrayOperator {
	child := *a
	return &child
}

func (a *ArrayOperator) withChildScope() *ArrayOperator {
	child := a.clone()
	child.scopeDepth++
	childAlias := child.elemAlias()
	child.visibleElems = appendStringCopy(a.visibleElems, childAlias)
	child.visibleScopes = appendStringCopy(a.visibleScopes, "")
	return child
}

func (a *ArrayOperator) withPath(path string) *ArrayOperator {
	if path == "" {
		path = jsonPathRoot
	}
	child := a.clone()
	child.exprPath = path
	return child
}

func (a *ArrayOperator) withValueScope(enabled bool) *ArrayOperator {
	child := a.clone()
	child.valueScope = enabled
	return child
}

func (a *ArrayOperator) withValueSemantics(enabled bool) *ArrayOperator {
	child := a.clone()
	child.valueSemantics = enabled
	return child
}

func (a *ArrayOperator) withArrayLambdaSource(
	scope arrayLambdaScope,
	sourceScopes []string,
	arrayValue typedValueSQL,
	valueSemantics bool,
) *ArrayOperator {
	child := a.clone()
	child.lambdaScope = scope
	child.valueSemantics = valueSemantics
	child.elementType = ExpressionTypeUnknown
	child.elementSchemaType = ""
	child.elementNestedTypes = nil
	child.elementNestedSchemaType = ""
	child.elementArraySchemaScopes = nil
	child.hasElementType = false

	scopes := normalizeSchemaScopes(sourceScopes)
	elemTypes := typedValueElementTypes(arrayValue)
	elemSchemaType := typedValueElementSchemaType(arrayValue)
	if len(scopes) > 0 && arrayLambdaSourceUsesSchemaScope(scopes, elemTypes) {
		child.schemaScope = scopes[0]
		child.schemaScopes = scopes
		if len(child.visibleScopes) > 0 {
			child.visibleScopes = replaceLastStringCopy(a.visibleScopes, scopes[0])
		}
		return child
	}

	child.schemaScope = ""
	child.schemaScopes = nil
	if len(elemTypes) > 0 {
		child.elementType = elemTypes[0]
		child.elementSchemaType = elemSchemaType
		child.elementNestedTypes = slices.Clone(elemTypes[1:])
		child.hasElementType = true
		if elemTypes[0] == ExpressionTypeArray {
			child.elementNestedSchemaType = elemSchemaType
			child.elementArraySchemaScopes = scopes
		}
	}
	return child
}

func arrayLambdaSourceUsesSchemaScope(scopes []string, elemTypes []ExpressionType) bool {
	if len(scopes) == 0 {
		return false
	}
	if len(elemTypes) == 0 {
		return true
	}
	return elemTypes[0] == ExpressionTypeObject
}

func (a *ArrayOperator) withReduceValueScope(accumulator typedValueSQL, accumulatorSQL string) *ArrayOperator {
	child := a.clone()
	child.valueSemantics = true
	child.accumulatorType = accumulator.typ
	child.hasAccumulatorType = true
	child.accumulatorSQL = accumulatorSQL
	child.accumulatorElemTypes = typedValueElementTypes(accumulator)
	child.accumulatorElemSchemaType = typedValueElementSchemaType(accumulator)
	child.accumulatorSchemaScopes = typedValueSchemaScopes(accumulator)
	return child
}

func appendStringCopy(values []string, value string) []string {
	copied := make([]string, len(values)+1)
	copy(copied, values)
	copied[len(values)] = value
	return copied
}

func replaceLastStringCopy(values []string, value string) []string {
	copied := make([]string, len(values))
	copy(copied, values)
	copied[len(copied)-1] = value
	return copied
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
	if len(scopes) == 1 {
		if scopes[0] == "" {
			return nil
		}
		return scopes
	}
	needsNormalize := false
	for i, scope := range scopes {
		if scope == "" {
			needsNormalize = true
			break
		}
		for j := 0; j < i; j++ {
			if scopes[j] == scope {
				needsNormalize = true
				break
			}
		}
		if needsNormalize {
			break
		}
	}
	if !needsNormalize {
		return scopes
	}

	normalized := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if scope == "" {
			continue
		}
		if stringSliceContains(normalized, scope) {
			continue
		}
		normalized = append(normalized, scope)
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func stringSliceContains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
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
		return jsonPathRoot
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
	if fieldName == "" {
		return ExpressionTypeUnknown
	}
	switch a.schema().GetFieldType(fieldName) {
	case SchemaTypeBoolean:
		return ExpressionTypeBoolean
	case SchemaTypeString, SchemaTypeEnum:
		return ExpressionTypeString
	case SchemaTypeInteger, SchemaTypeNumber:
		return ExpressionTypeNumber
	case SchemaTypeArray:
		return ExpressionTypeArray
	case objectFieldType:
		return ExpressionTypeObject
	default:
		return ExpressionTypeUnknown
	}
}

func (a *ArrayOperator) typedFieldSQL(sql, fieldName string, typ ExpressionType) typedValueSQL {
	value := typedValueSQL{sql: sql, typ: typ, schemaType: normalizeSchemaType(a.schema().GetFieldType(fieldName))}
	if typ == ExpressionTypeArray && a.fieldHasObjectArrayElements(fieldName) {
		value.elemType = ExpressionTypeObject
		value.elemTypes = []ExpressionType{ExpressionTypeObject}
		value.schemaScopes = singleSchemaScope(fieldName)
	} else if typ == ExpressionTypeArray {
		if elemType, elemSchemaType := a.fieldArrayElementType(fieldName); elemType != ExpressionTypeUnknown {
			value.elemType = elemType
			value.elemTypes = []ExpressionType{elemType}
			value.elemSchemaType = elemSchemaType
		}
	}
	return value
}

func (a *ArrayOperator) fieldHasObjectArrayElements(fieldName string) bool {
	if fieldName == "" {
		return false
	}
	provider, ok := a.schema().(ArrayElementSchemaProvider)
	return ok && provider.HasArrayElementFields(fieldName)
}

func (a *ArrayOperator) fieldArrayElementType(fieldName string) (ExpressionType, string) {
	if fieldName == "" {
		return ExpressionTypeUnknown, ""
	}
	provider, ok := a.schema().(ArrayElementTypeProvider)
	if !ok {
		return ExpressionTypeUnknown, ""
	}
	elemSchemaType := normalizeSchemaType(provider.GetArrayElementType(fieldName))
	return schemaFieldTypeExpressionType(elemSchemaType), elemSchemaType
}

func (a *ArrayOperator) validateScopedFieldName(fieldName string) error {
	if fieldName != "" && len(a.currentSchemaScopes()) == 0 {
		return fmt.Errorf("field '%s' cannot be validated because the array element schema is unknown", fieldName)
	}
	_, err := a.resolveFieldInScopes(a.currentSchemaScopes(), fieldName)
	return err
}

func (a *ArrayOperator) hasScopedField(fieldName string) bool {
	if fieldName == "" {
		return true
	}
	scopes := a.currentSchemaScopes()
	if len(scopes) == 0 {
		return false
	}
	for _, scope := range scopes {
		if _, err := a.resolveFieldInScope(scope, fieldName); err == nil {
			return true
		}
	}
	return false
}

func (a *ArrayOperator) resolveScopedFieldNames(fieldName string) []string {
	if fieldName != "" && len(a.currentSchemaScopes()) == 0 {
		return nil
	}
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
		var firstElementType string
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
				firstElementType = schemaArrayElementType(schema, resolved)
				firstAllowed = schema.GetAllowedValues(resolved)
				continue
			}
			if typ := schema.GetFieldType(resolved); typ != firstType {
				return nil, fmt.Errorf("field '%s' has incompatible schema types across array source scopes", fieldName)
			}
			if firstType == SchemaTypeArray && schemaArrayElementType(schema, resolved) != firstElementType {
				return nil, fmt.Errorf("field '%s' has incompatible array element types across array source scopes", fieldName)
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

func (a *ArrayOperator) scopedSQLFieldResult(sql string, fieldNames ...string) ProcessedValue {
	result := SQLFieldResult(sql)
	fieldNames = normalizeSchemaScopes(fieldNames)
	fieldName := ""
	if len(fieldNames) > 0 {
		fieldName = fieldNames[0]
	}
	if fieldName == "" {
		if scopes := a.currentObjectElementSchemaScopes(); len(scopes) > 0 {
			result.HasExpressionInfo = true
			result.Kind = ExpressionKindValue
			result.Type = ExpressionTypeObject
			result.SchemaType = objectFieldType
			result.ArrayElementSchemaScopes = scopes
			return result
		}
		if a.hasElementType && a.elementType != ExpressionTypeUnknown {
			result.HasExpressionInfo = true
			result.Kind = ExpressionKindValue
			result.Type = a.elementType
			result.SchemaType = a.elementSchemaType
			if a.elementType == ExpressionTypeArray {
				elemTypes := normalizeArrayElementTypes(a.elementNestedTypes)
				if len(elemTypes) > 0 {
					result.ArrayElementType = elemTypes[0]
					result.ArrayElementTypes = elemTypes
				}
				result.ArrayElementSchemaType = a.elementNestedSchemaType
				result.ArrayElementSchemaScopes = normalizeSchemaScopes(a.elementArraySchemaScopes)
			}
		}
		return result
	}
	result.FieldName = fieldName
	result.FieldNames = fieldNames
	if typ := a.schemaExpressionType(fieldName); typ != ExpressionTypeUnknown {
		result.HasExpressionInfo = true
		result.Kind = ExpressionKindValue
		result.Type = typ
		result.SchemaType = normalizeSchemaType(a.schema().GetFieldType(fieldName))
		if typ == ExpressionTypeArray && a.fieldHasObjectArrayElements(fieldName) {
			result.ArrayElementType = ExpressionTypeObject
			result.ArrayElementTypes = []ExpressionType{ExpressionTypeObject}
			result.ArrayElementSchemaScopes = singleSchemaScope(fieldName)
		} else if typ == ExpressionTypeArray {
			if elemType, elemSchemaType := a.fieldArrayElementType(fieldName); elemType != ExpressionTypeUnknown {
				result.ArrayElementType = elemType
				result.ArrayElementTypes = []ExpressionType{elemType}
				result.ArrayElementSchemaType = elemSchemaType
			}
		}
	}
	return result
}

func (a *ArrayOperator) currentObjectElementSchemaScopes() []string {
	scopes := normalizeSchemaScopes(a.currentSchemaScopes())
	if len(scopes) == 0 || !a.hasObjectArraySchemaScope(scopes) {
		return nil
	}
	return scopes
}

func (a *ArrayOperator) validateArrayScopeVarDefault(varName string, defaultValue interface{}) error {
	if fieldNames := a.scopedFieldNamesForVar(varName); len(fieldNames) > 0 {
		return validateVarDefaultForFields(a.schema(), fieldNames, defaultValue)
	}
	if a.isWholeCurrentElementVar(varName) && len(a.currentObjectElementSchemaScopes()) > 0 {
		return validateVarDefaultForExpressionType(ExpressionTypeObject, defaultValue, "array element")
	}
	if a.hasElementType {
		return validateVarDefaultForExpressionType(a.elementType, defaultValue, "array element")
	}
	return nil
}

func (a *ArrayOperator) isWholeCurrentElementVar(varName string) bool {
	switch a.lambdaScope {
	case arrayLambdaScopeElement, arrayLambdaScopeMap:
		return varName == ""
	case arrayLambdaScopeReduce:
		return varName == CurrentVar
	case arrayLambdaScopeNone:
		return varName == "" && a.valueScope
	default:
		return false
	}
}

func (a *ArrayOperator) shouldSimplifyNullDefault(varName string, defaultValue interface{}) bool {
	return defaultValue == nil &&
		a.isWholeCurrentElementVar(varName) &&
		len(a.currentObjectElementSchemaScopes()) > 0
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

func expressionTypeName(typ ExpressionType) string {
	switch typ {
	case ExpressionTypeNull:
		return literalKindNull
	case ExpressionTypeBoolean:
		return SchemaTypeBoolean
	case ExpressionTypeString:
		return SchemaTypeString
	case ExpressionTypeNumber:
		return SchemaTypeNumber
	case ExpressionTypeArray:
		return SchemaTypeArray
	case ExpressionTypeObject:
		return objectFieldType
	case ExpressionTypeUnknown:
		return expressionTypeNameUnknown
	default:
		return expressionTypeNameUnknown
	}
}

func schemaFieldTypeExpressionType(fieldType string) ExpressionType {
	fieldType = normalizeSchemaType(fieldType)
	switch fieldType {
	case SchemaTypeBoolean:
		return ExpressionTypeBoolean
	case SchemaTypeString, SchemaTypeEnum:
		return ExpressionTypeString
	case SchemaTypeInteger, SchemaTypeNumber:
		return ExpressionTypeNumber
	case SchemaTypeArray:
		return ExpressionTypeArray
	case objectFieldType:
		return ExpressionTypeObject
	default:
		return ExpressionTypeUnknown
	}
}

func (a *ArrayOperator) accumulatorSQLResult() ProcessedValue {
	sql := AccumulatorVar
	if a != nil && a.accumulatorSQL != "" {
		sql = a.accumulatorSQL
	}
	if a != nil && a.hasAccumulatorType && a.accumulatorType != ExpressionTypeUnknown {
		result := TypedSQLResult(sql, ExpressionKindValue, a.accumulatorType)
		if a.accumulatorType == ExpressionTypeArray {
			elemTypes := normalizeArrayElementTypes(a.accumulatorElemTypes)
			if len(elemTypes) > 0 {
				result.ArrayElementType = elemTypes[0]
				result.ArrayElementTypes = elemTypes
			}
			result.ArrayElementSchemaType = a.accumulatorElemSchemaType
			result.ArrayElementSchemaScopes = normalizeSchemaScopes(a.accumulatorSchemaScopes)
		}
		return result
	}
	result := TypedSQLResult(sql, ExpressionKindValue, ExpressionTypeUnknown)
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
	case arrayLambdaScopeElement, arrayLambdaScopeMap:
		return !a.isUnsupportedElementScopeVar(varName)
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

func (a *ArrayOperator) schema() SchemaProvider {
	return schemaFromConfig(a.config)
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
	// If it's a literal array, it's valid
	if _, ok := value.([]interface{}); ok {
		return nil
	}

	fieldNames, empty, ok := a.arraySourceSchemaScopeInfo(value)
	if !ok || empty || len(fieldNames) == 0 {
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

func (a *ArrayOperator) validateArraySourceValue(value typedValueSQL) error {
	if value.emptyArrayLiteral || value.typ == ExpressionTypeArray || value.typ == ExpressionTypeUnknown {
		if a.getDialect() == dialect.DialectPostgreSQL && value.elemType == ExpressionTypeArray {
			return fmt.Errorf("PostgreSQL array operations do not support nested array sources because UNNEST flattens multidimensional arrays")
		}
		return nil
	}
	return fmt.Errorf("array operation on non-array value (type: %s)", arrayExpressionTypeName(value.typ))
}

func (a *ArrayOperator) validateArrayResultElementTypes(elementTypes []ExpressionType) error {
	if len(elementTypes) == 0 || elementTypes[0] != ExpressionTypeArray {
		return nil
	}
	if a.getDialect() == dialect.DialectPostgreSQL {
		return fmt.Errorf("PostgreSQL does not support array values whose elements are arrays because multidimensional arrays must be rectangular")
	}
	return a.config.ValidateArrayLiteralElementTypes(elementTypes)
}

type mergeElementCompatibility struct {
	typ        ExpressionType
	schemaType string
}

func validateMergeElementCompatibility(values []typedValueSQL) (mergeElementCompatibility, error) {
	common := mergeElementCompatibility{typ: ExpressionTypeUnknown}
	var commonTypes []ExpressionType
	for _, value := range values {
		if value.emptyArrayLiteral {
			continue
		}
		elemType := value.typ
		elemTypes := typedValueElementTypes(value)
		elemSchemaType := value.schemaType
		if value.typ == ExpressionTypeArray {
			elemType = value.elemType
			elemSchemaType = value.elemSchemaType
		}
		if elemType == ExpressionTypeUnknown || elemType == ExpressionTypeNull {
			continue
		}
		elemSchemaType = normalizeSchemaType(elemSchemaType)
		if common.typ == ExpressionTypeUnknown {
			common.typ = elemType
			common.schemaType = elemSchemaType
			commonTypes = elemTypes
			continue
		}
		if common.typ != elemType {
			return mergeElementCompatibility{}, fmt.Errorf(
				"merge arguments have incompatible element types (%s and %s)",
				arrayExpressionTypeName(common.typ),
				arrayExpressionTypeName(elemType),
			)
		}
		if common.schemaType != "" && elemSchemaType != "" && common.schemaType != elemSchemaType {
			return mergeElementCompatibility{}, fmt.Errorf(
				"merge arguments have incompatible array element schema types (%s and %s)",
				common.schemaType,
				elemSchemaType,
			)
		}
		if common.schemaType == "" {
			common.schemaType = elemSchemaType
		}
		if len(commonTypes) > 0 && len(elemTypes) > 0 && !sameExpressionTypes(commonTypes, elemTypes) {
			return mergeElementCompatibility{}, fmt.Errorf(
				"merge arguments have incompatible element types (%s and %s)",
				arrayExpressionTypeName(commonTypes[len(commonTypes)-1]),
				arrayExpressionTypeName(elemTypes[len(elemTypes)-1]),
			)
		}
	}
	return common, nil
}

func (a *ArrayOperator) mergeValueToArraySQL(value typedValueSQL, common mergeElementCompatibility) (string, bool, error) {
	if value.emptyArrayLiteral {
		return "", true, nil
	}
	if value.typ == ExpressionTypeArray {
		if value.elemType == ExpressionTypeNull && common.typ == ExpressionTypeUnknown {
			return "", false, fmt.Errorf("merge null-only array argument requires a compatible typed operand")
		}
		return value.sql, false, nil
	}
	if value.typ == ExpressionTypeUnknown {
		return "", false, fmt.Errorf("merge scalar argument requires a statically known value type")
	}
	scalarSQL := value.sql
	if value.typ == ExpressionTypeNull {
		if common.typ == ExpressionTypeUnknown {
			return "", false, fmt.Errorf("merge null scalar argument requires a compatible typed operand")
		}
		scalarSQL = a.typedNullSQL(common.typ, common.schemaType)
	} else if common.typ != ExpressionTypeUnknown && value.typ != common.typ {
		return "", false, fmt.Errorf(
			"merge scalar argument has incompatible type %s; expected %s",
			arrayExpressionTypeName(value.typ),
			arrayExpressionTypeName(common.typ),
		)
	}
	arraySQL, err := a.arrayLiteral([]string{scalarSQL})
	if err != nil {
		return "", false, err
	}
	return arraySQL, false, nil
}

func (a *ArrayOperator) typedNullSQL(typ ExpressionType, schemaType ...string) string {
	if typ == ExpressionTypeNumber && firstSchemaType(schemaType...) == SchemaTypeInteger {
		return a.typedIntegerNullSQL()
	}
	switch typ {
	case ExpressionTypeBoolean:
		switch a.getDialect() {
		case dialect.DialectPostgreSQL:
			return "CAST(NULL AS BOOLEAN)"
		case dialect.DialectDuckDB:
			return "CAST(NULL AS BOOLEAN)"
		case dialect.DialectClickHouse:
			return "CAST(NULL AS Nullable(Bool))"
		case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner:
			return "CAST(NULL AS BOOL)"
		}
	case ExpressionTypeString:
		switch a.getDialect() {
		case dialect.DialectPostgreSQL:
			return "CAST(NULL AS TEXT)"
		case dialect.DialectDuckDB:
			return "CAST(NULL AS VARCHAR)"
		case dialect.DialectClickHouse:
			return "CAST(NULL AS Nullable(String))"
		case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner:
			return "CAST(NULL AS STRING)"
		}
	case ExpressionTypeNumber:
		switch a.getDialect() {
		case dialect.DialectPostgreSQL:
			return "CAST(NULL AS DOUBLE PRECISION)"
		case dialect.DialectDuckDB:
			return "CAST(NULL AS DOUBLE)"
		case dialect.DialectClickHouse:
			return "CAST(NULL AS Nullable(Float64))"
		case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner:
			return "CAST(NULL AS FLOAT64)"
		}
	case ExpressionTypeUnknown, ExpressionTypeNull, ExpressionTypeArray, ExpressionTypeObject:
		return sqlNull
	}
	return sqlNull
}

func (a *ArrayOperator) typedIntegerNullSQL() string {
	switch a.getDialect() {
	case dialect.DialectPostgreSQL:
		return "CAST(NULL AS BIGINT)"
	case dialect.DialectDuckDB:
		return "CAST(NULL AS BIGINT)"
	case dialect.DialectClickHouse:
		return "CAST(NULL AS Nullable(Int64))"
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner:
		return "CAST(NULL AS INT64)"
	}
	return "CAST(NULL AS INT64)"
}

func arrayExpressionTypeName(typ ExpressionType) string {
	switch typ {
	case ExpressionTypeNull:
		return literalKindNull
	case ExpressionTypeBoolean:
		return SchemaTypeBoolean
	case ExpressionTypeString:
		return SchemaTypeString
	case ExpressionTypeNumber:
		return SchemaTypeNumber
	case ExpressionTypeArray:
		return SchemaTypeArray
	case ExpressionTypeObject:
		return objectFieldType
	case ExpressionTypeUnknown:
		return expressionTypeNameUnknown
	default:
		return expressionTypeNameUnknown
	}
}

func (a *ArrayOperator) arraySourceSchemaScopes(value interface{}) []string {
	scopes, _, ok := a.arraySourceSchemaScopeInfo(value)
	if ok {
		return scopes
	}
	return nil
}

func (a *ArrayOperator) arraySourceSchemaScopesForValue(source interface{}, value typedValueSQL) []string {
	if scopes := typedValueSchemaScopes(value); len(scopes) > 0 {
		return scopes
	}
	return a.arraySourceSchemaScopes(source)
}

func (a *ArrayOperator) arraySourceSchemaScopesForValues(args []interface{}, values []typedValueSQL) []string {
	scopes := make([]string, 0, len(values))
	for _, value := range values {
		scopes = append(scopes, typedValueSchemaScopes(value)...)
	}
	if scopes = normalizeSchemaScopes(scopes); len(scopes) > 0 {
		return scopes
	}
	return a.arraySourceSchemaScopes(map[string]interface{}{OpMerge: args})
}

func (a *ArrayOperator) validateCompatibleArrayElementScopes(scopes []string) error {
	scopes = normalizeSchemaScopes(scopes)
	if len(scopes) <= 1 {
		return nil
	}
	if comparator, ok := a.schema().(ArrayElementSchemaComparator); ok {
		for _, scope := range scopes[1:] {
			if err := comparator.ValidateArrayElementSchemasCompatible(scopes[0], scope); err != nil {
				return err
			}
		}
		return nil
	}
	provider, ok := a.schema().(ArrayElementSchemaSignatureProvider)
	if !ok {
		return nil
	}
	first := provider.ArrayElementSchemaSignature(scopes[0])
	for _, scope := range scopes[1:] {
		if signature := provider.ArrayElementSchemaSignature(scope); signature != first {
			return fmt.Errorf("array source scopes have incompatible element schemas: %s and %s", scopes[0], scope)
		}
	}
	return nil
}

func (a *ArrayOperator) arraySourceSchemaScopeInfo(value interface{}) ([]string, bool, bool) {
	if isEmptyArrayLiteral(value) {
		return nil, true, true
	}
	// A JSONLogic if without an else renders as ELSE NULL. Null branches do
	// not provide an element schema, but they are compatible with typed array
	// branches for scoped-field validation.
	if inferLiteralValueExpressionType(value) == ExpressionTypeNull {
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
		if len(args) != binaryArrayOperatorArgCount {
			return nil, false, false
		}
		if isIdentityElementMapExpression(args[arrayExpressionArgIndex]) {
			return a.arraySourceSchemaScopeInfo(args[arraySourceArgIndex])
		}
		return a.mapProjectionArraySourceSchemaScope(args[arraySourceArgIndex], args[arrayExpressionArgIndex])
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
		return pv.SchemaFieldNames()
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

func (a *ArrayOperator) mapProjectionArraySourceSchemaScope(source, transformation interface{}) ([]string, bool, bool) {
	sourceScopes, empty, ok := a.arraySourceSchemaScopeInfo(source)
	if !ok || empty {
		return sourceScopes, empty, ok
	}
	exprMap, ok := transformation.(map[string]interface{})
	if !ok || len(exprMap) != 1 {
		return nil, false, false
	}
	varExpr, ok := exprMap[OpVar]
	if !ok {
		return nil, false, false
	}
	scoped := a.withArrayLambdaSource(arrayLambdaScopeElement, sourceScopes, typedValueSQL{}, true).
		scopedFieldNamesFromVarExpr(varExpr)
	if len(scoped) == 0 {
		return nil, false, false
	}
	for _, fieldName := range scoped {
		if !a.schema().IsArrayType(fieldName) {
			return nil, false, false
		}
	}
	return scoped, false, true
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
