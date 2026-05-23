package parser

import (
	"fmt"
	"strings"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

const (
	sqlTrue        = "TRUE"
	sqlFalse       = "FALSE"
	logicalOpAnd   = "and"
	logicalOpOr    = "or"
	sqlAndJoiner   = " AND "
	sqlOrJoiner    = " OR "
	unknownTypeSQL = "unknown"
	objectTypeSQL  = "object"
)

type expressionResult struct {
	operators.OperatorResult
	truthKnown               bool
	truthy                   bool
	fieldValue               bool
	fieldName                string
	fieldNames               []string
	fieldHasDefault          bool
	fieldDefault             interface{}
	fieldDefaultKnown        bool
	rawLiteralKnown          bool
	rawLiteral               interface{}
	arrayElementTypeKnown    bool
	arrayElementType         operators.ExpressionType
	arrayElementTypes        []operators.ExpressionType
	arrayElementSchemaScopes []string
	requiresKnownTruthiness  bool
	preserveParamRefs        bool
}

func resultFromOperator(res operators.OperatorResult) expressionResult {
	if res.Kind == operators.ExpressionKindPredicate {
		return predicateResult(res.SQL)
	}
	result := expressionResult{OperatorResult: res}
	result.preserveParamRefs = res.PreserveParamRefs
	applyStaticOperatorTruth(&result)
	if elemTypes := operatorResultArrayElementTypes(res); len(elemTypes) > 0 {
		result.arrayElementTypeKnown = true
		result.arrayElementType = elemTypes[0]
		result.arrayElementTypes = elemTypes
	}
	result.arrayElementSchemaScopes = normalizeParserSchemaScopes(res.ArrayElementSchemaScopes)
	return result
}

func applyStaticOperatorTruth(result *expressionResult) {
	if result.Kind != operators.ExpressionKindValue {
		return
	}
	if result.Type == operators.ExpressionTypeNull || result.EmptyArrayLiteral {
		result.truthKnown = true
		result.truthy = false
	}
}

func customOperatorResult(res operators.OperatorResult, preserveParamRefs bool) expressionResult {
	result := resultFromOperator(res)
	result.preserveParamRefs = preserveParamRefs
	return result
}

func canRollbackParamRefs(res expressionResult) bool {
	return !res.preserveParamRefs
}

func preserveParamRefsIfNeeded(res expressionResult, sources ...expressionResult) expressionResult {
	for _, source := range sources {
		if !canRollbackParamRefs(source) {
			res.preserveParamRefs = true
			return res
		}
	}
	return res
}

type paramRefPreserver struct {
	preserve bool
}

func (p *paramRefPreserver) mark(sources ...expressionResult) {
	for _, source := range sources {
		if !canRollbackParamRefs(source) {
			p.preserve = true
			return
		}
	}
}

func (p paramRefPreserver) apply(res expressionResult) expressionResult {
	if p.preserve {
		res.preserveParamRefs = true
	}
	return res
}

func predicateResult(sql string) expressionResult {
	switch normalizedSQLBooleanConstant(sql) {
	case sqlTrue:
		return booleanPredicateResult(true)
	case sqlFalse:
		return booleanPredicateResult(false)
	}
	return expressionResult{OperatorResult: operators.PredicateSQL(sql)}
}

func normalizedSQLBooleanConstant(sql string) string {
	switch strings.TrimSpace(sql) {
	case sqlTrue:
		return sqlTrue
	case sqlFalse:
		return sqlFalse
	default:
		return ""
	}
}

func booleanPredicateResult(value bool) expressionResult {
	if value {
		return expressionResult{
			OperatorResult: operators.PredicateSQL(sqlTrue),
			truthKnown:     true,
			truthy:         true,
		}
	}
	return expressionResult{
		OperatorResult: operators.PredicateSQL(sqlFalse),
		truthKnown:     true,
		truthy:         false,
	}
}

func valueResult(sql string, typ operators.ExpressionType) expressionResult {
	return expressionResult{OperatorResult: operators.ValueSQL(sql, typ)}
}

func withArrayElementTypes(res expressionResult, elemTypes ...operators.ExpressionType) expressionResult {
	elemTypes = normalizeExpressionTypes(elemTypes)
	if valueTypeOf(res) != operators.ExpressionTypeArray ||
		len(elemTypes) == 0 {
		return res
	}
	res.arrayElementTypeKnown = true
	res.arrayElementType = elemTypes[0]
	res.arrayElementTypes = elemTypes
	res.OperatorResult.ArrayElementType = elemTypes[0]
	res.OperatorResult.ArrayElementTypes = elemTypes
	return res
}

func withArrayElementSchemaScopes(res expressionResult, scopes ...string) expressionResult {
	if valueTypeOf(res) != operators.ExpressionTypeArray {
		return res
	}
	scopes = normalizeParserSchemaScopes(scopes)
	if len(scopes) == 0 {
		return res
	}
	res.arrayElementSchemaScopes = scopes
	res.OperatorResult.ArrayElementSchemaScopes = scopes
	return res
}

func fieldValueResult(sql string, typ operators.ExpressionType, fieldName ...string) expressionResult {
	res := valueResult(sql, typ)
	res.fieldValue = true
	res.fieldNames = normalizeParserSchemaScopes(fieldName)
	if len(res.fieldNames) > 0 {
		res.fieldName = res.fieldNames[0]
	}
	return res
}

func withVarDefaultMetadata(res expressionResult, args interface{}) expressionResult {
	defaultValue, hasDefault, defaultKnown := varDefaultLiteral(args)
	if !hasDefault {
		return res
	}
	res.fieldHasDefault = true
	if defaultKnown {
		res.fieldDefault = defaultValue
		res.fieldDefaultKnown = true
	}
	return res
}

func copyProcessedFieldMetadata(res *expressionResult, pv operators.ProcessedValue) {
	if !pv.IsField {
		return
	}
	res.fieldValue = true
	res.fieldNames = normalizeParserSchemaScopes(pv.SchemaFieldNames())
	res.fieldName = pv.FieldName
	if res.fieldName == "" && len(res.fieldNames) > 0 {
		res.fieldName = res.fieldNames[0]
	}
	res.fieldHasDefault = pv.FieldHasDefault
	res.fieldDefaultKnown = pv.FieldDefaultLiteralKnown
	res.fieldDefault = pv.FieldDefaultLiteral
}

func fieldOrValueResult(sql string, typ operators.ExpressionType, isField bool, fieldName ...string) expressionResult {
	if isField {
		return fieldValueResult(sql, typ, fieldName...)
	}
	return valueResult(sql, typ)
}

func (p *Parser) supportedValueResult(res expressionResult, path string) (expressionResult, error) {
	if err := p.rejectObjectFieldValue(res, path); err != nil {
		return expressionResult{}, err
	}
	return res, nil
}

func (p *Parser) rejectObjectFieldValue(res expressionResult, path string) error {
	if !res.fieldValue {
		return nil
	}
	for _, fieldName := range res.schemaFieldNames() {
		if p.config.Schema.GetFieldType(fieldName) == objectTypeSQL {
			return tperrors.New(tperrors.ErrInvalidArgument, "", path,
				fmt.Sprintf("object field '%s' cannot be used as a value expression; reference a nested field instead", fieldName))
		}
	}
	return nil
}

func (res expressionResult) schemaFieldNames() []string {
	if len(res.fieldNames) > 0 {
		return res.fieldNames
	}
	if res.fieldName != "" {
		return []string{res.fieldName}
	}
	return nil
}

func literalValueResult(sql string, typ operators.ExpressionType, truthy bool) expressionResult {
	res := valueResult(sql, typ)
	res.truthKnown = true
	res.truthy = truthy
	return res
}

func booleanValueResult(value bool) expressionResult {
	if value {
		return literalValueResult(sqlTrue, operators.ExpressionTypeBoolean, true)
	}
	return literalValueResult(sqlFalse, operators.ExpressionTypeBoolean, false)
}

func literalValueResultWithRaw(sql string, typ operators.ExpressionType, truthy bool, raw interface{}) expressionResult {
	res := literalValueResult(sql, typ, truthy)
	res.rawLiteralKnown = true
	res.rawLiteral = raw
	return res
}

func kindName(kind operators.ExpressionKind) string {
	switch kind {
	case operators.ExpressionKindPredicate:
		return "predicate"
	case operators.ExpressionKindValue:
		return "value"
	default:
		return unknownTypeSQL
	}
}

func typeName(typ operators.ExpressionType) string {
	switch typ {
	case operators.ExpressionTypeNull:
		return "null"
	case operators.ExpressionTypeBoolean:
		return "boolean"
	case operators.ExpressionTypeString:
		return "string"
	case operators.ExpressionTypeNumber:
		return "number"
	case operators.ExpressionTypeArray:
		return "array"
	case operators.ExpressionTypeObject:
		return objectTypeSQL
	case operators.ExpressionTypeUnknown:
		return unknownTypeSQL
	default:
		return unknownTypeSQL
	}
}

func valueTypeOf(res expressionResult) operators.ExpressionType {
	if res.Kind == operators.ExpressionKindPredicate {
		return operators.ExpressionTypeBoolean
	}
	return res.Type
}

func (p *Parser) fieldExpressionType(fieldName string) operators.ExpressionType {
	if fieldName == "" {
		return operators.ExpressionTypeUnknown
	}
	switch {
	case p.config.Schema.IsBooleanType(fieldName):
		return operators.ExpressionTypeBoolean
	case p.config.Schema.IsStringType(fieldName), p.config.Schema.IsEnumType(fieldName):
		return operators.ExpressionTypeString
	case p.config.Schema.IsNumericType(fieldName):
		return operators.ExpressionTypeNumber
	case p.config.Schema.IsArrayType(fieldName):
		return operators.ExpressionTypeArray
	case p.config.Schema.GetFieldType(fieldName) == objectTypeSQL:
		return operators.ExpressionTypeObject
	default:
		return operators.ExpressionTypeUnknown
	}
}

func (p *Parser) fieldValueExpressionResult(sql, fieldName string) expressionResult {
	res := fieldValueResult(sql, p.fieldExpressionType(fieldName), fieldName)
	if p.fieldHasObjectArrayElements(fieldName) {
		res = withArrayElementTypes(res, operators.ExpressionTypeObject)
		res = withArrayElementSchemaScopes(res, fieldName)
	} else if elemType := p.fieldArrayElementExpressionType(fieldName); elemType != operators.ExpressionTypeUnknown {
		res = withArrayElementTypes(res, elemType)
	}
	return res
}

func (p *Parser) fieldHasObjectArrayElements(fieldName string) bool {
	if fieldName == "" {
		return false
	}
	provider, ok := p.config.Schema.(operators.ArrayElementSchemaProvider)
	return ok && provider.HasArrayElementFields(fieldName)
}

func (p *Parser) fieldArrayElementExpressionType(fieldName string) operators.ExpressionType {
	if fieldName == "" {
		return operators.ExpressionTypeUnknown
	}
	provider, ok := p.config.Schema.(operators.ArrayElementTypeProvider)
	if !ok {
		return operators.ExpressionTypeUnknown
	}
	return schemaFieldTypeExpressionType(provider.GetArrayElementType(fieldName))
}

func schemaFieldTypeExpressionType(fieldType string) operators.ExpressionType {
	switch fieldType {
	case "boolean":
		return operators.ExpressionTypeBoolean
	case "string", "enum":
		return operators.ExpressionTypeString
	case "integer", "number":
		return operators.ExpressionTypeNumber
	case "array":
		return operators.ExpressionTypeArray
	case objectTypeSQL:
		return operators.ExpressionTypeObject
	default:
		return operators.ExpressionTypeUnknown
	}
}

func varFieldName(args interface{}) string {
	switch v := args.(type) {
	case string:
		return v
	case operators.ProcessedValue:
		if v.IsSQL && v.IsField {
			return v.FieldName
		}
	case []interface{}:
		if len(v) == 0 {
			return ""
		}
		if pv, ok := v[0].(operators.ProcessedValue); ok && pv.IsSQL && pv.IsField {
			return pv.FieldName
		}
		if field, ok := v[0].(string); ok {
			return field
		}
	}
	return ""
}

func varProcessedExpression(args interface{}) (operators.ProcessedValue, bool) {
	arr, ok := args.([]interface{})
	if !ok || len(arr) == 0 {
		return operators.ProcessedValue{}, false
	}
	pv, ok := arr[0].(operators.ProcessedValue)
	return pv, ok && pv.IsSQL && pv.HasExpressionInfo
}

func varDefaultLiteral(args interface{}) (interface{}, bool, bool) {
	v, ok := args.([]interface{})
	if !ok || len(v) < 2 {
		return nil, false, false
	}
	defaultValue := v[1]
	if pv, ok := defaultValue.(operators.ProcessedValue); ok {
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

func valueOperandSQL(res expressionResult) string {
	if res.Kind != operators.ExpressionKindPredicate || res.SQL == sqlTrue || res.SQL == sqlFalse {
		return res.SQL
	}
	return fmt.Sprintf("(%s)", operators.StripRedundantOuterParens(res.SQL))
}

func valueSQL(res expressionResult) string {
	if res.Kind == operators.ExpressionKindPredicate {
		return operators.PredicateValueSQL(res.SQL)
	}
	return res.SQL
}

func valueOperatorResult(res expressionResult) operators.OperatorResult {
	var opResult operators.OperatorResult
	if res.Kind == operators.ExpressionKindPredicate {
		opResult = operators.ValueSQL(operators.PredicateValueSQL(res.SQL), operators.ExpressionTypeBoolean)
	} else {
		opResult = res.OperatorResult
	}
	if expressionResultIsEmptyArrayLiteral(res) {
		opResult.EmptyArrayLiteral = true
	}
	opResult.PreserveParamRefs = res.preserveParamRefs
	if res.arrayElementTypeKnown {
		opResult.ArrayElementType = res.arrayElementType
		opResult.ArrayElementTypes = normalizeExpressionTypes(res.arrayElementTypes)
	}
	opResult.ArrayElementSchemaScopes = normalizeParserSchemaScopes(res.arrayElementSchemaScopes)
	return opResult
}

func expressionResultIsEmptyArrayLiteral(res expressionResult) bool {
	return res.Kind == operators.ExpressionKindValue &&
		valueTypeOf(res) == operators.ExpressionTypeArray &&
		(res.EmptyArrayLiteral || (res.rawLiteralKnown && isEmptyArrayLiteralValue(res.rawLiteral)))
}

func (p *Parser) catStringSQL(res expressionResult) string {
	if res.rawLiteralKnown && res.rawLiteral != nil {
		switch valueTypeOf(res) {
		case operators.ExpressionTypeNull:
			return "''"
		case operators.ExpressionTypeBoolean:
			return operators.PredicateStringSQL(valueSQL(res))
		case operators.ExpressionTypeString:
			return operators.StripRedundantOuterParens(valueSQL(res))
		case operators.ExpressionTypeNumber:
			return p.config.StringCast(operators.StripRedundantOuterParens(valueSQL(res)))
		case operators.ExpressionTypeArray, operators.ExpressionTypeObject, operators.ExpressionTypeUnknown:
		}
	}
	sql := valueSQL(res)
	if res.Kind == operators.ExpressionKindPredicate {
		sql = res.SQL
	}
	return operators.ConcatStringSQL(p.config, sql, res.Kind, valueTypeOf(res))
}

func typedValueOperand(res expressionResult) operators.ProcessedValue {
	pv := operators.TypedSQLResult(valueOperandSQL(res), res.Kind, valueTypeOf(res))
	pv.RequiresKnownTruthiness = res.requiresKnownTruthiness
	pv.PreserveParamRefs = res.preserveParamRefs
	if res.arrayElementTypeKnown {
		pv.ArrayElementType = res.arrayElementType
		pv.ArrayElementTypes = normalizeExpressionTypes(res.arrayElementTypes)
	}
	pv.ArrayElementSchemaScopes = normalizeParserSchemaScopes(res.arrayElementSchemaScopes)
	if res.fieldValue {
		pv.IsField = true
		pv.FieldNames = normalizeParserSchemaScopes(res.fieldNames)
		pv.FieldName = res.fieldName
		if pv.FieldName == "" && len(pv.FieldNames) > 0 {
			pv.FieldName = pv.FieldNames[0]
		}
		pv.FieldHasDefault = res.fieldHasDefault
		pv.FieldDefaultLiteralKnown = res.fieldDefaultKnown
		pv.FieldDefaultLiteral = res.fieldDefault
	}
	return pv
}

func operatorResultFromProcessedValue(pv operators.ProcessedValue) operators.OperatorResult {
	res := operators.OperatorResult{
		SQL:  pv.Value,
		Kind: pv.Kind,
		Type: pv.Type,
	}
	res.PreserveParamRefs = pv.PreserveParamRefs
	if pv.ArrayElementType != operators.ExpressionTypeUnknown {
		res.ArrayElementType = pv.ArrayElementType
	}
	if len(pv.ArrayElementTypes) > 0 {
		res.ArrayElementTypes = normalizeExpressionTypes(pv.ArrayElementTypes)
		if res.ArrayElementType == operators.ExpressionTypeUnknown && len(res.ArrayElementTypes) > 0 {
			res.ArrayElementType = res.ArrayElementTypes[0]
		}
	}
	res.ArrayElementSchemaScopes = normalizeParserSchemaScopes(pv.ArrayElementSchemaScopes)
	return res
}

func operatorResultArrayElementTypes(res operators.OperatorResult) []operators.ExpressionType {
	if types := normalizeExpressionTypes(res.ArrayElementTypes); len(types) > 0 {
		return types
	}
	if res.ArrayElementType != operators.ExpressionTypeUnknown && res.ArrayElementType != operators.ExpressionTypeNull {
		return []operators.ExpressionType{res.ArrayElementType}
	}
	return nil
}

func operatorArgFromExpressionResult(res expressionResult) operators.OperatorArg {
	arg := operators.OperatorArg{
		SQL:  res.SQL,
		Kind: res.Kind,
		Type: valueTypeOf(res),
	}
	if elemTypes, ok := arrayElementTypesOf(res); ok {
		arg.ArrayElementType = elemTypes[0]
		arg.ArrayElementTypes = elemTypes
	}
	arg.ArrayElementSchemaScopes = normalizeParserSchemaScopes(res.arrayElementSchemaScopes)
	return arg
}

func normalizeExpressionTypes(types []operators.ExpressionType) []operators.ExpressionType {
	if len(types) == 0 || types[0] == operators.ExpressionTypeUnknown || types[0] == operators.ExpressionTypeNull {
		return nil
	}
	normalized := make([]operators.ExpressionType, 0, len(types))
	for _, typ := range types {
		if typ == operators.ExpressionTypeUnknown || typ == operators.ExpressionTypeNull {
			break
		}
		normalized = append(normalized, typ)
	}
	return normalized
}

func normalizeParserSchemaScopes(scopes []string) []string {
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

func processedArgsPreserveParamRefs(args []interface{}) bool {
	for _, arg := range args {
		if pv, ok := arg.(operators.ProcessedValue); ok && pv.PreserveParamRefs {
			return true
		}
	}
	return false
}

func customResultPreservesDroppedParamRefs(sql string, pc *params.ParamCollector, paramCount int) bool {
	collected := pc.Params()
	for i := paramCount; i < len(collected); i++ {
		if !params.ContainsParamRef(sql, i+1, collected[i], pc.Style()) {
			return true
		}
	}
	return false
}

func literalComparisonPredicateResult(operator string, args []interface{}, sql string) (expressionResult, error) {
	truthy, known, err := operators.FoldLiteralComparison(operator, args)
	if err != nil {
		return expressionResult{}, err
	}
	if known {
		return booleanPredicateResult(truthy), nil
	}
	return predicateResult(sql), nil
}
