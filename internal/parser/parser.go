//nolint:goconst // JSONLogic operator and SQL token strings stay inline in parser switches for readability.
package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

type customOperatorContextHandler interface {
	ToSQLInContext(operator string, args []operators.OperatorArg, kind operators.ExpressionKind) (operators.OperatorResult, error)
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
// If config is nil, defaults to BigQuery dialect for backward compatibility.
func NewParser(config *operators.OperatorConfig) *Parser {
	if config == nil {
		// Default to BigQuery for backward compatibility in internal usage
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
		return res.OperatorResult, err
	})
	config.SetPredicateExpressionParser(func(expr any, path string) (operators.OperatorResult, error) {
		res, err := p.parseExpressionPredicate(expr, path)
		return res.OperatorResult, err
	})
	config.SetParamValueExpressionParser(func(expr any, path string, pc *params.ParamCollector) (operators.OperatorResult, error) {
		res, err := p.parseExpressionValueParam(expr, path, pc)
		return res.OperatorResult, err
	})
	config.SetParamPredicateExpressionParser(func(expr any, path string, pc *params.ParamCollector) (operators.OperatorResult, error) {
		res, err := p.parseExpressionPredicateParam(expr, path, pc)
		return res.OperatorResult, err
	})

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
	p.config.Schema = schema
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
	if p.isUnsupportedPostgreSQLEmptyArrayResult(res) {
		return "", tperrors.New(tperrors.ErrInvalidArgument, "", "$",
			"empty PostgreSQL array literals require an explicit element type")
	}
	return res.SQL, nil
}

func (p *Parser) isUnsupportedPostgreSQLEmptyArrayResult(res expressionResult) bool {
	return p.config.GetDialect() == dialect.DialectPostgreSQL && containsBarePostgreSQLEmptyArrayLiteral(res.SQL)
}

func containsBarePostgreSQLEmptyArrayLiteral(sql string) bool {
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false

	for i := 0; i < len(sql); i++ {
		ch := sql[i]

		switch {
		case inSingleQuote:
			if ch == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					i++
					continue
				}
				inSingleQuote = false
			}
			continue
		case inDoubleQuote:
			if ch == '"' {
				if i+1 < len(sql) && sql[i+1] == '"' {
					i++
					continue
				}
				inDoubleQuote = false
			}
			continue
		case inBacktick:
			if ch == '`' {
				inBacktick = false
			}
			continue
		case ch == '\'':
			inSingleQuote = true
			continue
		case ch == '"':
			inDoubleQuote = true
			continue
		case ch == '`':
			inBacktick = true
			continue
		}

		if i+len("ARRAY[]") > len(sql) || !strings.EqualFold(sql[i:i+len("ARRAY[]")], "ARRAY[]") {
			continue
		}
		if i > 0 && isSQLIdentifierChar(sql[i-1]) {
			continue
		}
		if i+len("ARRAY[]") < len(sql) && isSQLIdentifierChar(sql[i+len("ARRAY[]")]) {
			continue
		}
		if isTypedPostgreSQLEmptyArrayLiteral(sql, i) {
			continue
		}
		return true
	}

	return false
}

func isTypedPostgreSQLEmptyArrayLiteral(sql string, start int) bool {
	end := start + len("ARRAY[]")
	next := skipSQLSpaces(sql, end)
	if strings.HasPrefix(sql[next:], "::") {
		return true
	}
	return isCastPostgreSQLEmptyArrayLiteral(sql, start, end)
}

func isCastPostgreSQLEmptyArrayLiteral(sql string, start, end int) bool {
	open := skipSQLSpacesBackward(sql, start-1)
	if open < 0 || sql[open] != '(' {
		return false
	}

	wordEnd := skipSQLSpacesBackward(sql, open-1) + 1
	if wordEnd <= 0 {
		return false
	}
	wordStart := wordEnd - 1
	for wordStart >= 0 && isSQLIdentifierChar(sql[wordStart]) {
		wordStart--
	}
	if !strings.EqualFold(sql[wordStart+1:wordEnd], "CAST") {
		return false
	}

	return hasSQLWordAt(sql, skipSQLSpaces(sql, end), "AS")
}

func skipSQLSpaces(sql string, pos int) int {
	for pos < len(sql) && (sql[pos] == ' ' || sql[pos] == '\t' || sql[pos] == '\n' || sql[pos] == '\r') {
		pos++
	}
	return pos
}

func skipSQLSpacesBackward(sql string, pos int) int {
	for pos >= 0 && (sql[pos] == ' ' || sql[pos] == '\t' || sql[pos] == '\n' || sql[pos] == '\r') {
		pos--
	}
	return pos
}

func hasSQLWordAt(sql string, pos int, word string) bool {
	if pos+len(word) > len(sql) || !strings.EqualFold(sql[pos:pos+len(word)], word) {
		return false
	}
	if pos > 0 && isSQLIdentifierChar(sql[pos-1]) {
		return false
	}
	if pos+len(word) < len(sql) && isSQLIdentifierChar(sql[pos+len(word)]) {
		return false
	}
	return true
}

func isSQLIdentifierChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_'
}

type expressionResult struct {
	operators.OperatorResult
	truthKnown      bool
	truthy          bool
	fieldValue      bool
	fieldName       string
	rawLiteralKnown bool
	rawLiteral      interface{}
}

func resultFromOperator(res operators.OperatorResult) expressionResult {
	return expressionResult{OperatorResult: res}
}

func predicateResult(sql string) expressionResult {
	switch normalizedSQLBooleanConstant(sql) {
	case "TRUE":
		return booleanPredicateResult(true)
	case "FALSE":
		return booleanPredicateResult(false)
	}
	return expressionResult{OperatorResult: operators.PredicateSQL(sql)}
}

func normalizedSQLBooleanConstant(sql string) string {
	switch strings.TrimSpace(sql) {
	case "TRUE":
		return "TRUE"
	case "FALSE":
		return "FALSE"
	default:
		return ""
	}
}

func booleanPredicateResult(value bool) expressionResult {
	if value {
		return expressionResult{
			OperatorResult: operators.PredicateSQL("TRUE"),
			truthKnown:     true,
			truthy:         true,
		}
	}
	return expressionResult{
		OperatorResult: operators.PredicateSQL("FALSE"),
		truthKnown:     true,
		truthy:         false,
	}
}

func valueResult(sql string, typ operators.ExpressionType) expressionResult {
	return expressionResult{OperatorResult: operators.ValueSQL(sql, typ)}
}

func fieldValueResult(sql string, typ operators.ExpressionType, fieldName ...string) expressionResult {
	res := valueResult(sql, typ)
	res.fieldValue = true
	if len(fieldName) > 0 {
		res.fieldName = fieldName[0]
	}
	return res
}

func fieldOrValueResult(sql string, typ operators.ExpressionType, isField bool, fieldName ...string) expressionResult {
	if isField {
		return fieldValueResult(sql, typ, fieldName...)
	}
	return valueResult(sql, typ)
}

func literalValueResult(sql string, typ operators.ExpressionType, truthy bool) expressionResult {
	res := valueResult(sql, typ)
	res.truthKnown = true
	res.truthy = truthy
	return res
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
		return "unknown"
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
	case operators.ExpressionTypeUnknown:
		return "unknown"
	default:
		return "unknown"
	}
}

func valueTypeOf(res expressionResult) operators.ExpressionType {
	if res.Kind == operators.ExpressionKindPredicate {
		return operators.ExpressionTypeBoolean
	}
	return res.Type
}

func (p *Parser) fieldExpressionType(fieldName string) operators.ExpressionType {
	if p.config == nil || p.config.Schema == nil || fieldName == "" {
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
	default:
		return operators.ExpressionTypeUnknown
	}
}

func varFieldName(args interface{}) string {
	switch v := args.(type) {
	case string:
		return v
	case []interface{}:
		if len(v) == 0 {
			return ""
		}
		if field, ok := v[0].(string); ok {
			return field
		}
	}
	return ""
}

func isEmptyArrayLiteralValue(value interface{}) bool {
	arr, ok := value.([]interface{})
	return ok && len(arr) == 0
}

func arrayValueOperatorReturnsEmptyLiteral(operator string, args []interface{}) bool {
	switch operator {
	case operators.OpMap, operators.OpFilter:
		return len(args) > 0 && isEmptyArrayLiteralValue(args[0])
	case operators.OpMerge:
		if len(args) == 0 {
			return false
		}
		for _, arg := range args {
			if !isEmptyArrayLiteralValue(arg) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (p *Parser) literalToSQL(value interface{}) (string, error) {
	return p.dataOp.ValueToSQL(value)
}

func (p *Parser) literalToSQLParam(value interface{}, pc *params.ParamCollector) (string, error) {
	return p.dataOp.ValueToSQLParam(value, pc)
}

func (p *Parser) arrayLiteralToSQL(arr []interface{}, path string) (string, error) {
	parts := make([]string, len(arr))
	for i, elem := range arr {
		res, err := p.parseExpressionValue(elem, tperrors.BuildArrayPath(path, i))
		if err != nil {
			return "", fmt.Errorf("invalid array element %d: %w", i, err)
		}
		parts[i] = valueOperandSQL(res)
	}
	return p.config.ArrayLiteral(parts)
}

func (p *Parser) arrayLiteralToSQLParam(arr []interface{}, path string, pc *params.ParamCollector) (string, error) {
	parts := make([]string, len(arr))
	for i, elem := range arr {
		res, err := p.parseExpressionValueParam(elem, tperrors.BuildArrayPath(path, i), pc)
		if err != nil {
			return "", fmt.Errorf("invalid array element %d: %w", i, err)
		}
		parts[i] = valueOperandSQL(res)
	}
	return p.config.ArrayLiteral(parts)
}

func literalTypeAndTruth(value interface{}) (operators.ExpressionType, bool, bool) {
	switch v := value.(type) {
	case nil:
		return operators.ExpressionTypeNull, true, false
	case bool:
		return operators.ExpressionTypeBoolean, true, v
	case string:
		return operators.ExpressionTypeString, true, v != ""
	case json.Number, float32, float64,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return operators.ExpressionTypeNumber, true, !isZeroLiteral(value)
	case []interface{}:
		return operators.ExpressionTypeArray, true, len(v) > 0
	default:
		return operators.ExpressionTypeUnknown, false, false
	}
}

func isZeroLiteral(value interface{}) bool {
	switch v := value.(type) {
	case json.Number:
		return isZeroJSONNumberLiteral(v.String())
	case float32:
		return v == 0
	case float64:
		return v == 0
	case int:
		return v == 0
	case int8:
		return v == 0
	case int16:
		return v == 0
	case int32:
		return v == 0
	case int64:
		return v == 0
	case uint:
		return v == 0
	case uint8:
		return v == 0
	case uint16:
		return v == 0
	case uint32:
		return v == 0
	case uint64:
		return v == 0
	default:
		return false
	}
}

func isZeroJSONNumberLiteral(s string) bool {
	if exponent := strings.IndexAny(s, "eE"); exponent >= 0 {
		s = s[:exponent]
	}
	for _, ch := range s {
		if ch >= '1' && ch <= '9' {
			return false
		}
	}
	return true
}

func (p *Parser) truthinessSQL(res expressionResult, path string) (string, error) {
	if res.Kind == operators.ExpressionKindPredicate {
		return res.SQL, nil
	}
	if res.truthKnown {
		if res.truthy {
			return "TRUE", nil
		}
		return "FALSE", nil
	}
	switch res.Type {
	case operators.ExpressionTypeNull:
		return "FALSE", nil
	case operators.ExpressionTypeBoolean:
		return fmt.Sprintf("%s IS TRUE", res.SQL), nil
	case operators.ExpressionTypeString:
		return fmt.Sprintf("(%s IS NOT NULL AND %s != '')", res.SQL, res.SQL), nil
	case operators.ExpressionTypeNumber:
		return fmt.Sprintf("(%s IS NOT NULL AND %s != 0)", res.SQL, res.SQL), nil
	case operators.ExpressionTypeArray:
		lengthCheck := p.config.ArrayLengthFunc(res.SQL)
		return fmt.Sprintf("(%s IS NOT NULL AND %s > 0)", res.SQL, lengthCheck), nil
	case operators.ExpressionTypeUnknown:
		if res.fieldValue {
			return "", tperrors.New(tperrors.ErrInvalidExpressionContext, "", path,
				"truthiness requires a statically known field type; provide schema information")
		}
		return fmt.Sprintf("(%s IS NOT NULL AND %s != FALSE AND %s != 0 AND %s != '')",
			res.SQL, res.SQL, res.SQL, res.SQL), nil
	}
	return fmt.Sprintf("(%s IS NOT NULL AND %s != FALSE AND %s != 0 AND %s != '')",
		res.SQL, res.SQL, res.SQL, res.SQL), nil
}

func valueOperandSQL(res expressionResult) string {
	if res.Kind != operators.ExpressionKindPredicate || res.SQL == "TRUE" || res.SQL == "FALSE" {
		return res.SQL
	}
	return fmt.Sprintf("(%s)", operators.StripRedundantOuterParens(res.SQL))
}

func typedValueOperand(res expressionResult) operators.ProcessedValue {
	pv := operators.TypedSQLResult(valueOperandSQL(res), res.Kind, valueTypeOf(res))
	if res.fieldValue {
		pv.IsField = true
		pv.FieldName = res.fieldName
	}
	return pv
}

func literalComparisonPredicateResult(operator string, args []interface{}, sql string) (expressionResult, error) {
	res := predicateResult(sql)
	truthy, known, err := operators.FoldLiteralComparison(operator, args)
	if err != nil {
		return expressionResult{}, err
	}
	if known {
		res.truthKnown = true
		res.truthy = truthy
	}
	return res, nil
}

func (p *Parser) parseTruthinessResultParam(expr interface{}, path string, pc *params.ParamCollector) (expressionResult, string, error) {
	checkpoint := pc.Checkpoint()
	res, err := p.parseExpressionAnyParam(expr, path, pc)
	if err != nil {
		return expressionResult{}, "", err
	}
	condition, err := p.truthinessSQL(res, path)
	if err != nil {
		return expressionResult{}, "", err
	}
	if res.truthKnown || (res.Kind != operators.ExpressionKindPredicate && res.Type == operators.ExpressionTypeNull) {
		pc.Restore(checkpoint)
	}
	return res, condition, nil
}

func (p *Parser) customOperatorToSQL(
	handler CustomOperatorHandler,
	operator string,
	args []operators.OperatorArg,
	kind operators.ExpressionKind,
) (operators.OperatorResult, error) {
	if contextual, ok := handler.(customOperatorContextHandler); ok {
		return contextual.ToSQLInContext(operator, args, kind)
	}
	return handler.ToSQL(operator, args)
}

func compatibleValueType(left, right expressionResult, path string) (operators.ExpressionType, error) {
	leftType := valueTypeOf(left)
	rightType := valueTypeOf(right)
	if leftType == operators.ExpressionTypeNull {
		return rightType, nil
	}
	if rightType == operators.ExpressionTypeNull {
		return leftType, nil
	}
	if leftType == rightType {
		return leftType, nil
	}
	if leftType == operators.ExpressionTypeUnknown || rightType == operators.ExpressionTypeUnknown {
		return operators.ExpressionTypeUnknown, nil
	}
	return operators.ExpressionTypeUnknown, tperrors.NewTypeMismatch("", path,
		"compatible value result types", fmt.Sprintf("%s and %s", typeName(leftType), typeName(rightType)))
}

func (p *Parser) parsePrimitiveValue(expr interface{}, path string) (expressionResult, error) {
	sql, err := p.literalToSQL(expr)
	if err != nil {
		return expressionResult{}, tperrors.Wrap(tperrors.ErrInvalidArgument, "", path, "invalid literal", err)
	}
	typ, known, truthy := literalTypeAndTruth(expr)
	if known {
		return literalValueResultWithRaw(sql, typ, truthy, expr), nil
	}
	return valueResult(sql, typ), nil
}

func (p *Parser) parsePrimitiveValueParam(expr interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	sql, err := p.literalToSQLParam(expr, pc)
	if err != nil {
		return expressionResult{}, tperrors.Wrap(tperrors.ErrInvalidArgument, "", path, "invalid literal", err)
	}
	typ, known, truthy := literalTypeAndTruth(expr)
	if known {
		return literalValueResultWithRaw(sql, typ, truthy, expr), nil
	}
	return valueResult(sql, typ), nil
}

func (p *Parser) parseExpressionPredicate(expr interface{}, path string) (expressionResult, error) {
	if b, ok := expr.(bool); ok {
		return booleanPredicateResult(b), nil
	}
	if p.isPrimitive(expr) {
		return expressionResult{}, tperrors.NewInvalidExpressionContext("", path, "predicate", "value")
	}
	if _, ok := expr.([]interface{}); ok {
		return expressionResult{}, tperrors.NewInvalidExpressionContext("", path, "predicate", "value")
	}
	if pv, ok := expr.(operators.ProcessedValue); ok {
		if pv.IsSQL {
			return expressionResult{}, tperrors.NewInvalidExpressionContext("", path, "predicate", "value")
		}
		return p.parseExpressionPredicate(pv.Value, path)
	}
	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return expressionResult{}, tperrors.NewMultipleKeys(path)
		}
		for operator, args := range obj {
			operatorPath := tperrors.BuildPath(path, operator, -1)
			return p.parseOperatorPredicate(operator, args, operatorPath)
		}
	}
	return expressionResult{}, tperrors.New(tperrors.ErrInvalidExpression, "", path,
		fmt.Sprintf("invalid expression type: %T", expr))
}

func (p *Parser) parseExpressionValue(expr interface{}, path string) (expressionResult, error) {
	if pv, ok := expr.(operators.ProcessedValue); ok {
		if pv.IsSQL {
			if pv.HasExpressionInfo {
				res := resultFromOperator(operators.OperatorResult{
					SQL:  pv.Value,
					Kind: pv.Kind,
					Type: pv.Type,
				})
				res.fieldValue = pv.IsField
				res.fieldName = pv.FieldName
				return res, nil
			}
			return fieldOrValueResult(pv.Value, operators.ExpressionTypeUnknown, pv.IsField, pv.FieldName), nil
		}
		return p.parseExpressionValue(pv.Value, path)
	}
	if p.isPrimitive(expr) {
		return p.parsePrimitiveValue(expr, path)
	}
	if arr, ok := expr.([]interface{}); ok {
		sql, err := p.arrayLiteralToSQL(arr, path)
		if err != nil {
			return expressionResult{}, tperrors.Wrap(tperrors.ErrInvalidArgument, "", path, "invalid array literal", err)
		}
		return literalValueResultWithRaw(sql, operators.ExpressionTypeArray, len(arr) > 0, expr), nil
	}
	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return expressionResult{}, tperrors.NewMultipleKeys(path)
		}
		for operator, args := range obj {
			operatorPath := tperrors.BuildPath(path, operator, -1)
			return p.parseOperatorValue(operator, args, operatorPath)
		}
	}
	return expressionResult{}, tperrors.New(tperrors.ErrInvalidExpression, "", path,
		fmt.Sprintf("invalid expression type: %T", expr))
}

func (p *Parser) parseExpressionAny(expr interface{}, path string) (expressionResult, error) {
	if p.isPrimitive(expr) {
		return p.parsePrimitiveValue(expr, path)
	}
	if _, ok := expr.([]interface{}); ok {
		return p.parseExpressionValue(expr, path)
	}
	if obj, ok := expr.(map[string]interface{}); ok && len(obj) == 1 {
		for operator := range obj {
			switch operator {
			case "missing", "missing_some", "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in", "!", "!!", operators.OpAll, operators.OpSome, operators.OpNone:
				return p.parseExpressionPredicate(expr, path)
			case "and", "or", "if":
				if res, err := p.parseExpressionPredicate(expr, path); err == nil {
					return res, nil
				}
				return p.parseExpressionValue(expr, path)
			default:
				return p.parseExpressionValue(expr, path)
			}
		}
	}
	return p.parseExpressionValue(expr, path)
}

func (p *Parser) parseOperatorPredicate(operator string, args interface{}, path string) (expressionResult, error) {
	if p.customOpLookup != nil {
		if handler, ok := p.customOpLookup(operator); ok {
			processedArgs, err := p.processCustomOperatorArgs(args, path)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"failed to process custom operator arguments", err)
			}
			res, err := p.customOperatorToSQL(handler, operator, processedArgs, operators.ExpressionKindPredicate)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"custom operator failed", err)
			}
			if res.Kind != operators.ExpressionKindPredicate {
				return expressionResult{}, tperrors.NewInvalidExpressionContext(operator, path, "predicate", kindName(res.Kind))
			}
			return resultFromOperator(res), nil
		}
	}

	switch operator {
	case "missing":
		sql, err := p.dataOp.ToSQL(operator, []interface{}{args})
		return predicateResult(sql), p.wrapOperatorError(operator, path, err)
	case "missing_some":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		sql, err := p.dataOp.ToSQL(operator, arr)
		return predicateResult(sql), p.wrapOperatorError(operator, path, err)
	case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		processedArgs, err := p.processValueArgs(arr, path)
		if err != nil {
			return expressionResult{}, err
		}
		sql, err := p.comparisonOp.ToSQL(operator, processedArgs)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		res, err := literalComparisonPredicateResult(operator, processedArgs, sql)
		return res, p.wrapOperatorError(operator, path, err)
	case "and", "or":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parsePredicateLogical(operator, arr, path)
	case "!":
		return p.parseNotPredicate(operator, args, path, false)
	case "!!":
		return p.parseNotPredicate(operator, args, path, true)
	case "if":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parsePredicateIf(arr, path)
	case operators.OpAll, operators.OpSome, operators.OpNone:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		sql, err := p.arrayOp.ToSQLAtPath(operator, arr, path)
		return predicateResult(sql), p.wrapOperatorError(operator, path, err)
	case "var", operators.OpMap, operators.OpFilter, operators.OpReduce, operators.OpMerge, "+", "-", "*", "/", "%", "max", "min", "cat", "substr":
		return expressionResult{}, tperrors.NewInvalidExpressionContext(operator, path, "predicate", "value")
	default:
		return expressionResult{}, tperrors.NewUnsupportedOperator(operator, path)
	}
}

func (p *Parser) parseOperatorValue(operator string, args interface{}, path string) (expressionResult, error) {
	if p.customOpLookup != nil {
		if handler, ok := p.customOpLookup(operator); ok {
			processedArgs, err := p.processCustomOperatorArgs(args, path)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"failed to process custom operator arguments", err)
			}
			res, err := p.customOperatorToSQL(handler, operator, processedArgs, operators.ExpressionKindValue)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"custom operator failed", err)
			}
			return resultFromOperator(res), nil
		}
	}

	switch operator {
	case "var":
		sql, err := p.dataOp.ToSQL(operator, []interface{}{args})
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		fieldName := varFieldName(args)
		return fieldValueResult(sql, p.fieldExpressionType(fieldName), fieldName), nil
	case "missing", "missing_some", "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in", "!", "!!", operators.OpAll, operators.OpSome, operators.OpNone:
		return p.parseOperatorPredicate(operator, args, path)
	case "and", "or":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parseValueLogical(operator, arr, path)
	case "if":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parseValueIf(arr, path)
	case "+", "-", "*", "/", "%", "max", "min":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		processedArgs, err := p.processValueArgs(arr, path)
		if err != nil {
			return expressionResult{}, err
		}
		sql, err := p.numericOp.ToSQL(operator, processedArgs)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		return valueResult(sql, operators.ExpressionTypeNumber), nil
	case "cat", "substr":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		processedArgs, err := p.processValueArgs(arr, path)
		if err != nil {
			return expressionResult{}, err
		}
		sql, err := p.stringOp.ToSQL(operator, processedArgs)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		return valueResult(sql, operators.ExpressionTypeString), nil
	case operators.OpMap, operators.OpFilter, operators.OpMerge:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		sql, err := p.arrayOp.ToSQLAtPath(operator, arr, path)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		if arrayValueOperatorReturnsEmptyLiteral(operator, arr) {
			return literalValueResultWithRaw(sql, operators.ExpressionTypeArray, false, []interface{}{}), nil
		}
		return valueResult(sql, operators.ExpressionTypeArray), nil
	case operators.OpReduce:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		if len(arr) == 3 && isEmptyArrayLiteralValue(arr[0]) {
			return p.parseExpressionValue(arr[2], tperrors.BuildArrayPath(path, 2))
		}
		sql, err := p.arrayOp.ToSQLAtPath(operator, arr, path)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		return valueResult(sql, operators.ExpressionTypeUnknown), nil
	default:
		return expressionResult{}, tperrors.NewUnsupportedOperator(operator, path)
	}
}

func (p *Parser) parsePredicateLogical(operator string, args []interface{}, path string) (expressionResult, error) {
	if len(args) == 0 {
		return expressionResult{}, tperrors.NewInsufficientArgs(operator, path, 1, 0)
	}
	parts := make([]string, 0, len(args))
	for i, arg := range args {
		argPath := tperrors.BuildArrayPath(path, i)
		res, err := p.parseExpressionPredicate(arg, argPath)
		if err != nil {
			return expressionResult{}, err
		}
		if res.truthKnown {
			if operator == "and" && res.truthy {
				continue
			}
			if operator == "or" && !res.truthy {
				continue
			}
			if operator == "and" && !res.truthy {
				return booleanPredicateResult(false), nil
			}
			if operator == "or" && res.truthy {
				return booleanPredicateResult(true), nil
			}
		}
		parts = append(parts, res.SQL)
	}
	if len(parts) == 0 {
		return booleanPredicateResult(operator == "and"), nil
	}
	if len(parts) == 1 {
		return predicateResult(parts[0]), nil
	}
	joiner := " AND "
	if operator == "or" {
		joiner = " OR "
	}
	return predicateResult(fmt.Sprintf("(%s)", strings.Join(parts, joiner))), nil
}

func unaryArg(args interface{}) (interface{}, bool) {
	if arr, ok := args.([]interface{}); ok {
		if len(arr) != 1 {
			return nil, false
		}
		return arr[0], true
	}
	return args, true
}

func (p *Parser) parseNotPredicate(operator string, args interface{}, path string, double bool) (expressionResult, error) {
	arg, ok := unaryArg(args)
	if !ok {
		return expressionResult{}, tperrors.NewTypeMismatch(operator, path, "exactly 1 argument", "multiple arguments")
	}
	res, err := p.parseExpressionAny(arg, tperrors.BuildArrayPath(path, 0))
	if err != nil {
		return expressionResult{}, err
	}
	condition, err := p.truthinessSQL(res, path)
	if err != nil {
		return expressionResult{}, err
	}
	if double {
		if res.truthKnown {
			return booleanPredicateResult(res.truthy), nil
		}
		return predicateResult(condition), nil
	}
	if res.truthKnown {
		return booleanPredicateResult(!res.truthy), nil
	}
	condition = operators.StripRedundantOuterParens(condition)
	return predicateResult(fmt.Sprintf("NOT (%s)", condition)), nil
}

func (p *Parser) parsePredicateIf(args []interface{}, path string) (expressionResult, error) {
	if len(args) < 2 {
		return expressionResult{}, tperrors.NewInsufficientArgs("if", path, 2, len(args))
	}
	var parts []string
	pairLimit := len(args)
	hasElse := len(args)%2 == 1
	if hasElse {
		pairLimit = len(args) - 1
	}
	for i := 0; i < pairLimit; i += 2 {
		cond, err := p.parsePredicateIfOperand(args[i], tperrors.BuildArrayPath(path, i))
		if err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && !cond.truthy {
			continue
		}
		thenRes, err := p.parsePredicateIfOperand(args[i+1], tperrors.BuildArrayPath(path, i+1))
		if err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && cond.truthy {
			if len(parts) == 0 {
				return thenRes, nil
			}
			return predicateResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), thenRes.SQL)), nil
		}
		parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", cond.SQL, thenRes.SQL))
	}
	elseSQL := "FALSE"
	if hasElse {
		elseRes, err := p.parsePredicateIfOperand(args[len(args)-1], tperrors.BuildArrayPath(path, len(args)-1))
		if err != nil {
			return expressionResult{}, err
		}
		if len(parts) == 0 {
			return elseRes, nil
		}
		elseSQL = elseRes.SQL
	}
	if len(parts) == 0 {
		return booleanPredicateResult(false), nil
	}
	return predicateResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), elseSQL)), nil
}

func (p *Parser) parsePredicateIfOperand(expr interface{}, path string) (expressionResult, error) {
	if b, ok := expr.(bool); ok {
		return booleanPredicateResult(b), nil
	}
	return p.parseExpressionPredicate(expr, path)
}

func (p *Parser) parseValueIf(args []interface{}, path string) (expressionResult, error) {
	if len(args) < 2 {
		return expressionResult{}, tperrors.NewInsufficientArgs("if", path, 2, len(args))
	}
	var parts []string
	resultType := operators.ExpressionTypeNull
	typeSet := false
	mergeResultType := func(res expressionResult) error {
		if !typeSet {
			resultType = valueTypeOf(res)
			typeSet = true
			return nil
		}
		typ, err := compatibleValueType(valueResult("", resultType), res, path)
		if err != nil {
			return err
		}
		resultType = typ
		return nil
	}
	pairLimit := len(args)
	hasElse := len(args)%2 == 1
	if hasElse {
		pairLimit = len(args) - 1
	}
	for i := 0; i < pairLimit; i += 2 {
		cond, err := p.parseExpressionAny(args[i], tperrors.BuildArrayPath(path, i))
		if err != nil {
			return expressionResult{}, err
		}
		condition, err := p.truthinessSQL(cond, tperrors.BuildArrayPath(path, i))
		if err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && !cond.truthy {
			continue
		}
		thenRes, err := p.parseExpressionValue(args[i+1], tperrors.BuildArrayPath(path, i+1))
		if err != nil {
			return expressionResult{}, err
		}
		if err := mergeResultType(thenRes); err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && cond.truthy {
			if len(parts) == 0 {
				return thenRes, nil
			}
			return valueResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), thenRes.SQL), resultType), nil
		}
		parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", condition, thenRes.SQL))
	}
	elseSQL := "NULL"
	if hasElse {
		elseRes, err := p.parseExpressionValue(args[len(args)-1], tperrors.BuildArrayPath(path, len(args)-1))
		if err != nil {
			return expressionResult{}, err
		}
		if len(parts) == 0 {
			return elseRes, nil
		}
		if err := mergeResultType(elseRes); err != nil {
			return expressionResult{}, err
		}
		elseSQL = elseRes.SQL
	}
	if len(parts) == 0 {
		return literalValueResult("NULL", operators.ExpressionTypeNull, false), nil
	}
	return valueResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), elseSQL), resultType), nil
}

func (p *Parser) parseValueLogical(operator string, args []interface{}, path string) (expressionResult, error) {
	if len(args) == 0 {
		return expressionResult{}, tperrors.NewInsufficientArgs(operator, path, 1, 0)
	}
	return p.parseValueLogicalFrom(operator, args, 0, path)
}

func (p *Parser) parseValueLogicalFrom(operator string, args []interface{}, index int, path string) (expressionResult, error) {
	current, err := p.parseExpressionValue(args[index], tperrors.BuildArrayPath(path, index))
	if err != nil {
		return expressionResult{}, err
	}
	if index == len(args)-1 {
		return current, nil
	}
	if current.truthKnown {
		if operator == "or" && current.truthy {
			return current, nil
		}
		if operator == "and" && !current.truthy {
			return current, nil
		}
		return p.parseValueLogicalFrom(operator, args, index+1, path)
	}
	condition, err := p.truthinessSQL(current, tperrors.BuildArrayPath(path, index))
	if err != nil {
		return expressionResult{}, err
	}
	rest, err := p.parseValueLogicalFrom(operator, args, index+1, path)
	if err != nil {
		return expressionResult{}, err
	}
	resultType, err := compatibleValueType(current, rest, path)
	if err != nil {
		return expressionResult{}, err
	}
	if operator == "or" {
		return valueResult(fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, current.SQL, rest.SQL), resultType), nil
	}
	return valueResult(fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, rest.SQL, current.SQL), resultType), nil
}

// parseExpression recursively parses JSON Logic expressions.
// path is the JSONPath to the current expression for error reporting.
func (p *Parser) parseExpression(expr interface{}, path string) (string, error) {
	// Handle primitive values (should not happen in normal JSON Logic, but handle gracefully)
	if p.isPrimitive(expr) {
		return "", tperrors.NewPrimitiveNotAllowed(path)
	}

	// Handle arrays (should not happen in normal JSON Logic, but handle gracefully)
	if _, ok := expr.([]interface{}); ok {
		return "", tperrors.NewArrayNotAllowed(path)
	}

	// Handle objects (operators)
	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return "", tperrors.NewMultipleKeys(path)
		}

		for operator, args := range obj {
			operatorPath := tperrors.BuildPath(path, operator, -1)
			return p.parseOperator(operator, args, operatorPath)
		}
	}

	return "", tperrors.New(tperrors.ErrInvalidExpression, "", path,
		fmt.Sprintf("invalid expression type: %T", expr))
}

// wrapOperatorError wraps an operator error with TranspileError if it isn't already.
func (p *Parser) wrapOperatorError(operator, path string, err error) error {
	if err == nil {
		return nil
	}
	// Check if it's already a TranspileError
	var tpErr *tperrors.TranspileError
	if errors.As(err, &tpErr) {
		return err
	}
	// Wrap with appropriate error code based on error message
	return tperrors.Wrap(tperrors.ErrInvalidArgument, operator, path, "operator error", err)
}

// parseOperator parses a specific operator.
// path is the JSONPath to this operator for error reporting.
func (p *Parser) parseOperator(operator string, args interface{}, path string) (string, error) {
	// Check for custom operators first
	if p.customOpLookup != nil {
		if handler, ok := p.customOpLookup(operator); ok {
			// Process the arguments for the custom operator
			processedArgs, err := p.processCustomOperatorArgs(args, path)
			if err != nil {
				return "", tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"failed to process custom operator arguments", err)
			}
			res, err := handler.ToSQL(operator, processedArgs)
			if err != nil {
				return "", tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"custom operator failed", err)
			}
			return res.SQL, nil
		}
	}

	// Handle different operator types
	switch operator {
	// Data access operators
	case "var":
		sql, err := p.dataOp.ToSQL(operator, []interface{}{args})
		return sql, p.wrapOperatorError(operator, path, err)
	case "missing":
		// missing takes a single string argument, wrap it in an array
		sql, err := p.dataOp.ToSQL(operator, []interface{}{args})
		return sql, p.wrapOperatorError(operator, path, err)
	case "missing_some":
		if arr, ok := args.([]interface{}); ok {
			sql, err := p.dataOp.ToSQL(operator, arr)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	// Comparison operators
	case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
		if arr, ok := args.([]interface{}); ok {
			// Comparison operands are value expressions.
			processedArgs, err := p.processValueArgs(arr, path)
			if err != nil {
				return "", err
			}
			sql, err := p.comparisonOp.ToSQL(operator, processedArgs)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	// Logical operators
	case "and", "or", "if":
		if arr, ok := args.([]interface{}); ok {
			// Process arguments to handle custom operators in nested expressions
			processedArgs, err := p.processArgs(arr, path)
			if err != nil {
				return "", err
			}
			sql, err := p.logicalOp.ToSQL(operator, processedArgs)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)
	case "!", "!!":
		// These unary operators can accept both array and non-array arguments
		if arr, ok := args.([]interface{}); ok {
			// Process arguments to handle custom operators
			processedArgs, err := p.processArgs(arr, path)
			if err != nil {
				return "", err
			}
			sql, err := p.logicalOp.ToSQL(operator, processedArgs)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		// Process non-array argument to handle custom operators before wrapping
		processedArg, err := p.processArg(args, path, 0)
		if err != nil {
			return "", err
		}
		sql, err := p.logicalOp.ToSQL(operator, []interface{}{processedArg})
		return sql, p.wrapOperatorError(operator, path, err)

	// Numeric operators
	case "+", "-", "*", "/", "%", "max", "min":
		if arr, ok := args.([]interface{}); ok {
			// Process arguments to handle complex expressions
			processedArgs, err := p.processArgs(arr, path)
			if err != nil {
				return "", err
			}
			sql, err := p.numericOp.ToSQL(operator, processedArgs)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	// Array operators
	case operators.OpMap, operators.OpFilter, operators.OpReduce, operators.OpAll, operators.OpSome, operators.OpNone, operators.OpMerge:
		if arr, ok := args.([]interface{}); ok {
			sql, err := p.arrayOp.ToSQLAtPath(operator, arr, path)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	// String operators
	case "cat", "substr":
		if arr, ok := args.([]interface{}); ok {
			sql, err := p.stringOp.ToSQL(operator, arr)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	// All operators are now supported
	default:
		return "", tperrors.NewUnsupportedOperator(operator, path)
	}
}

// isBuiltInOperator checks if an operator is a built-in operator.
func (p *Parser) isBuiltInOperator(operator string) bool {
	builtInOps := map[string]bool{
		// Data access
		"var": true, "missing": true, "missing_some": true,
		// Comparison
		"==": true, "===": true, "!=": true, "!==": true,
		">": true, ">=": true, "<": true, "<=": true, "in": true,
		// Logical
		"and": true, "or": true, "!": true, "!!": true, "if": true,
		// Numeric
		"+": true, "-": true, "*": true, "/": true, "%": true,
		"max": true, "min": true,
		// String
		"cat": true, "substr": true,
		// Array
		"map": true, "filter": true, "reduce": true,
		"all": true, "some": true, "none": true, "merge": true,
	}
	return builtInOps[operator]
}

// isArrayOperator checks if an operator introduces/depends on array expression
// semantics that should be delegated directly to ArrayOperator without parser-level
// eager argument preprocessing.
func (p *Parser) isArrayOperator(operator string) bool {
	switch operator {
	case operators.OpMap, operators.OpFilter, operators.OpReduce, operators.OpAll, operators.OpSome, operators.OpNone, operators.OpMerge:
		return true
	}
	return false
}

// processArgs recursively processes arguments to handle custom operators at any nesting level.
// It converts custom operators to SQL while preserving the structure of built-in operators
// but with their nested custom operators already processed.
// path is the JSONPath to the parent operator.
func (p *Parser) processArgs(args []interface{}, path string) ([]interface{}, error) {
	processed := make([]interface{}, len(args))

	for i, arg := range args {
		processedArg, err := p.processArg(arg, path, i)
		if err != nil {
			return nil, err
		}
		processed[i] = processedArg
	}

	return processed, nil
}

func (p *Parser) processValueArgs(args []interface{}, path string) ([]interface{}, error) {
	processed := make([]interface{}, len(args))

	for i, arg := range args {
		processedArg, err := p.processValueArg(arg, path, i)
		if err != nil {
			return nil, err
		}
		processed[i] = processedArg
	}

	return processed, nil
}

func (p *Parser) processValueArg(arg interface{}, path string, index int) (interface{}, error) {
	if p.isPrimitive(arg) {
		return arg, nil
	}
	if exprMap, ok := arg.(map[string]interface{}); ok && len(exprMap) == 1 {
		for operator := range exprMap {
			if operator != "var" {
				res, err := p.parseExpressionValue(arg, tperrors.BuildArrayPath(path, index))
				if err != nil {
					return nil, err
				}
				if res.rawLiteralKnown {
					return res.rawLiteral, nil
				}
				return typedValueOperand(res), nil
			}
		}
	}
	return p.processArg(arg, path, index)
}

// processArg processes a single argument, recursively handling custom operators.
// Returns ProcessedValue when SQL is generated, otherwise returns the original type.
// path is the JSONPath to the parent, index is the argument index.
func (p *Parser) processArg(arg interface{}, path string, index int) (interface{}, error) {
	argPath := tperrors.BuildArrayPath(path, index)

	// If it's a complex expression (map with single key)
	if exprMap, ok := arg.(map[string]interface{}); ok {
		if len(exprMap) == 1 {
			for operator, opArgs := range exprMap {
				operatorPath := tperrors.BuildPath(path, operator, index)

				// Check if it's a custom operator (not built-in)
				if !p.isBuiltInOperator(operator) {
					// It's a custom operator, parse it to SQL
					sql, err := p.parseOperator(operator, opArgs, operatorPath)
					if err != nil {
						return nil, err
					}
					// Wrap in ProcessedValue to mark as SQL
					return operators.SQLResult(sql), nil
				}

				// It's a built-in operator - recursively process its arguments
				// to handle any nested custom operators.
				// Array operators are handled specially: parse them immediately with
				// their full operatorPath so nested custom-operator failures preserve
				// complete JSONPath context under non-array parents (e.g. == / and).
				// ArrayOperator still performs scope-aware rewrites before nested
				// custom operators are parsed.
				if p.isArrayOperator(operator) {
					sql, err := p.parseOperator(operator, opArgs, operatorPath)
					if err != nil {
						return nil, err
					}
					return operators.SQLResult(sql), nil
				}
				processedOpArgs, err := p.processOpArgs(opArgs, operatorPath)
				if err != nil {
					return nil, err
				}
				// Return the expression with processed arguments
				return map[string]interface{}{operator: processedOpArgs}, nil
			}
		}
		// Multi-key maps - keep as is
		return arg, nil
	}

	// Arrays need recursive processing too
	if arr, ok := arg.([]interface{}); ok {
		return p.processArgs(arr, argPath)
	}

	// Primitives - keep as is
	return arg, nil
}

// processOpArgs processes operator arguments (can be array or single value).
// path is the JSONPath to the operator.
func (p *Parser) processOpArgs(opArgs interface{}, path string) (interface{}, error) {
	if arr, ok := opArgs.([]interface{}); ok {
		return p.processArgs(arr, path)
	}
	// Single argument
	return p.processArg(opArgs, path, 0)
}

// processCustomOperatorArgs processes arguments for custom operators.
// It converts all expressions (including var) to their SQL representation.
// path is the JSONPath to the custom operator.
func (p *Parser) processCustomOperatorArgs(args interface{}, path string) ([]operators.OperatorArg, error) {
	// Handle array arguments
	if arr, ok := args.([]interface{}); ok {
		processed := make([]operators.OperatorArg, len(arr))
		for i, arg := range arr {
			argPath := tperrors.BuildArrayPath(path, i)
			argResult, err := p.processArgToOperatorArg(arg, argPath)
			if err != nil {
				return nil, err
			}
			processed[i] = argResult
		}
		return processed, nil
	}

	// Handle single argument (wrap in array)
	argResult, err := p.processArgToOperatorArg(args, path)
	if err != nil {
		return nil, err
	}
	return []operators.OperatorArg{argResult}, nil
}

func (p *Parser) processArgToOperatorArg(arg interface{}, path string) (operators.OperatorArg, error) {
	res, err := p.parseExpressionAny(arg, path)
	if err != nil {
		return operators.OperatorArg{}, err
	}
	return operators.OperatorArg{SQL: res.SQL, Kind: res.Kind, Type: valueTypeOf(res)}, nil
}

// primitiveToSQL converts a primitive value to its SQL representation.
//

func (p *Parser) primitiveToSQL(value interface{}) interface{} {
	switch v := value.(type) {
	case string:
		escaped := strings.ReplaceAll(v, "'", "''")
		return fmt.Sprintf("'%s'", escaped)
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	case nil:
		return "NULL"
	default:
		// Numbers and other types
		return fmt.Sprintf("%v", v)
	}
}

// isPrimitive checks if a value is a primitive type.
func (p *Parser) isPrimitive(value interface{}) bool {
	switch value.(type) {
	case string, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number, bool:
		return true
	case nil:
		return true
	default:
		return false
	}
}

// ParseParameterized converts a JSON Logic expression to SQL using context
// inference with bind parameter placeholders instead of inlined literals.
func (p *Parser) ParseParameterized(logic interface{}) (string, []params.QueryParam, error) {
	if err := p.validator.Validate(logic); err != nil {
		return "", nil, tperrors.NewValidationError(err)
	}
	if p.isPrimitive(logic) {
		return "", nil, tperrors.NewPrimitiveNotAllowed("$")
	}
	if _, ok := logic.([]interface{}); ok {
		return "", nil, tperrors.NewArrayNotAllowed("$")
	}

	style := params.StyleForDialect(p.config.GetDialect())
	pc := params.NewParamCollector(style)

	res, err := p.parseExpressionAnyParam(logic, "$", pc)
	if err != nil {
		return "", nil, err
	}

	if vErr := params.ValidatePlaceholderRefs(res.SQL, pc.Params(), style); vErr != nil {
		return "", nil, vErr
	}

	return res.SQL, pc.Params(), nil
}

// ParseConditionParameterized converts a JSON Logic expression to a SQL condition
// (without WHERE keyword) with bind parameter placeholders.
func (p *Parser) ParseConditionParameterized(logic interface{}) (string, []params.QueryParam, error) {
	if err := p.validator.Validate(logic); err != nil {
		return "", nil, tperrors.NewValidationError(err)
	}

	style := params.StyleForDialect(p.config.GetDialect())
	pc := params.NewParamCollector(style)

	res, err := p.parseExpressionPredicateParam(logic, "$", pc)
	if err != nil {
		return "", nil, err
	}

	if vErr := params.ValidatePlaceholderRefs(res.SQL, pc.Params(), style); vErr != nil {
		return "", nil, vErr
	}

	return res.SQL, pc.Params(), nil
}

// ParseValueParameterized converts a JSON Logic expression to a parameterized
// SQL value expression.
func (p *Parser) ParseValueParameterized(logic interface{}) (string, []params.QueryParam, error) {
	style := params.StyleForDialect(p.config.GetDialect())
	pc := params.NewParamCollector(style)

	res, err := p.parseExpressionValueParam(logic, "$", pc)
	if err != nil {
		return "", nil, err
	}
	if p.isUnsupportedPostgreSQLEmptyArrayResult(res) {
		return "", nil, tperrors.New(tperrors.ErrInvalidArgument, "", "$",
			"empty PostgreSQL array literals require an explicit element type")
	}

	if vErr := params.ValidatePlaceholderRefs(res.SQL, pc.Params(), style); vErr != nil {
		return "", nil, vErr
	}

	return res.SQL, pc.Params(), nil
}

func (p *Parser) parseExpressionPredicateParam(expr interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if b, ok := expr.(bool); ok {
		return booleanPredicateResult(b), nil
	}
	if p.isPrimitive(expr) {
		return expressionResult{}, tperrors.NewInvalidExpressionContext("", path, "predicate", "value")
	}
	if _, ok := expr.([]interface{}); ok {
		return expressionResult{}, tperrors.NewInvalidExpressionContext("", path, "predicate", "value")
	}
	if pv, ok := expr.(operators.ProcessedValue); ok {
		if pv.IsSQL {
			return expressionResult{}, tperrors.NewInvalidExpressionContext("", path, "predicate", "value")
		}
		return p.parseExpressionPredicateParam(pv.Value, path, pc)
	}
	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return expressionResult{}, tperrors.NewMultipleKeys(path)
		}
		for operator, args := range obj {
			operatorPath := tperrors.BuildPath(path, operator, -1)
			return p.parseOperatorPredicateParam(operator, args, operatorPath, pc)
		}
	}
	return expressionResult{}, tperrors.New(tperrors.ErrInvalidExpression, "", path,
		fmt.Sprintf("invalid expression type: %T", expr))
}

func (p *Parser) parseExpressionValueParam(expr interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if pv, ok := expr.(operators.ProcessedValue); ok {
		if pv.IsSQL {
			if pv.HasExpressionInfo {
				res := resultFromOperator(operators.OperatorResult{
					SQL:  pv.Value,
					Kind: pv.Kind,
					Type: pv.Type,
				})
				res.fieldValue = pv.IsField
				res.fieldName = pv.FieldName
				return res, nil
			}
			return fieldOrValueResult(pv.Value, operators.ExpressionTypeUnknown, pv.IsField, pv.FieldName), nil
		}
		return p.parseExpressionValueParam(pv.Value, path, pc)
	}
	if p.isPrimitive(expr) {
		return p.parsePrimitiveValueParam(expr, path, pc)
	}
	if arr, ok := expr.([]interface{}); ok {
		sql, err := p.arrayLiteralToSQLParam(arr, path, pc)
		if err != nil {
			return expressionResult{}, tperrors.Wrap(tperrors.ErrInvalidArgument, "", path, "invalid array literal", err)
		}
		return literalValueResultWithRaw(sql, operators.ExpressionTypeArray, len(arr) > 0, expr), nil
	}
	if obj, ok := expr.(map[string]interface{}); ok {
		if len(obj) != 1 {
			return expressionResult{}, tperrors.NewMultipleKeys(path)
		}
		for operator, args := range obj {
			operatorPath := tperrors.BuildPath(path, operator, -1)
			return p.parseOperatorValueParam(operator, args, operatorPath, pc)
		}
	}
	return expressionResult{}, tperrors.New(tperrors.ErrInvalidExpression, "", path,
		fmt.Sprintf("invalid expression type: %T", expr))
}

func (p *Parser) parseExpressionAnyParam(expr interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if p.isPrimitive(expr) {
		return p.parsePrimitiveValueParam(expr, path, pc)
	}
	if _, ok := expr.([]interface{}); ok {
		return p.parseExpressionValueParam(expr, path, pc)
	}
	if obj, ok := expr.(map[string]interface{}); ok && len(obj) == 1 {
		for operator := range obj {
			switch operator {
			case "missing", "missing_some", "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in", "!", "!!", operators.OpAll, operators.OpSome, operators.OpNone:
				return p.parseExpressionPredicateParam(expr, path, pc)
			case "and", "or", "if":
				checkpoint := pc.Checkpoint()
				if res, err := p.parseExpressionPredicateParam(expr, path, pc); err == nil {
					return res, nil
				}
				pc.Restore(checkpoint)
				return p.parseExpressionValueParam(expr, path, pc)
			default:
				return p.parseExpressionValueParam(expr, path, pc)
			}
		}
	}
	return p.parseExpressionValueParam(expr, path, pc)
}

func (p *Parser) parseOperatorPredicateParam(operator string, args interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if p.customOpLookup != nil {
		if handler, ok := p.customOpLookup(operator); ok {
			processedArgs, err := p.processCustomOperatorArgsParam(args, path, pc)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"failed to process custom operator arguments", err)
			}
			res, err := p.customOperatorToSQL(handler, operator, processedArgs, operators.ExpressionKindPredicate)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"custom operator failed", err)
			}
			if ph, bad := params.FindQuotedPlaceholderRef(res.SQL, pc.Params(), pc.Style()); bad {
				return expressionResult{}, tperrors.New(tperrors.ErrCustomOperatorFailed, operator, path,
					fmt.Sprintf("custom operator produced invalid parameterized SQL: placeholder %s appears inside a quoted string literal", ph))
			}
			if res.Kind != operators.ExpressionKindPredicate {
				return expressionResult{}, tperrors.NewInvalidExpressionContext(operator, path, "predicate", kindName(res.Kind))
			}
			return resultFromOperator(res), nil
		}
	}

	switch operator {
	case "missing":
		sql, err := p.dataOp.ToSQLParam(operator, []interface{}{args}, pc)
		return predicateResult(sql), p.wrapOperatorError(operator, path, err)
	case "missing_some":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		sql, err := p.dataOp.ToSQLParam(operator, arr, pc)
		return predicateResult(sql), p.wrapOperatorError(operator, path, err)
	case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		checkpoint := pc.Checkpoint()
		processedArgs, err := p.processValueArgsParam(arr, path, pc)
		if err != nil {
			return expressionResult{}, err
		}
		sql, err := p.comparisonOp.ToSQLParam(operator, processedArgs, pc)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		if normalizedSQLBooleanConstant(sql) != "" {
			pc.Restore(checkpoint)
		}
		res, err := literalComparisonPredicateResult(operator, processedArgs, sql)
		return res, p.wrapOperatorError(operator, path, err)
	case "and", "or":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parsePredicateLogicalParam(operator, arr, path, pc)
	case "!":
		return p.parseNotPredicateParam(operator, args, path, false, pc)
	case "!!":
		return p.parseNotPredicateParam(operator, args, path, true, pc)
	case "if":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parsePredicateIfParam(arr, path, pc)
	case operators.OpAll, operators.OpSome, operators.OpNone:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		sql, err := p.arrayOp.ToSQLParamAtPath(operator, arr, pc, path)
		return predicateResult(sql), p.wrapOperatorError(operator, path, err)
	case "var", operators.OpMap, operators.OpFilter, operators.OpReduce, operators.OpMerge, "+", "-", "*", "/", "%", "max", "min", "cat", "substr":
		return expressionResult{}, tperrors.NewInvalidExpressionContext(operator, path, "predicate", "value")
	default:
		return expressionResult{}, tperrors.NewUnsupportedOperator(operator, path)
	}
}

func (p *Parser) parseOperatorValueParam(operator string, args interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if p.customOpLookup != nil {
		if handler, ok := p.customOpLookup(operator); ok {
			processedArgs, err := p.processCustomOperatorArgsParam(args, path, pc)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"failed to process custom operator arguments", err)
			}
			res, err := p.customOperatorToSQL(handler, operator, processedArgs, operators.ExpressionKindValue)
			if err != nil {
				return expressionResult{}, tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"custom operator failed", err)
			}
			if ph, bad := params.FindQuotedPlaceholderRef(res.SQL, pc.Params(), pc.Style()); bad {
				return expressionResult{}, tperrors.New(tperrors.ErrCustomOperatorFailed, operator, path,
					fmt.Sprintf("custom operator produced invalid parameterized SQL: placeholder %s appears inside a quoted string literal", ph))
			}
			return resultFromOperator(res), nil
		}
	}

	switch operator {
	case "var":
		sql, err := p.dataOp.ToSQLParam(operator, []interface{}{args}, pc)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		fieldName := varFieldName(args)
		return fieldValueResult(sql, p.fieldExpressionType(fieldName), fieldName), nil
	case "missing", "missing_some", "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in", "!", "!!", operators.OpAll, operators.OpSome, operators.OpNone:
		return p.parseOperatorPredicateParam(operator, args, path, pc)
	case "and", "or":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parseValueLogicalParam(operator, arr, path, pc)
	case "if":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		return p.parseValueIfParam(arr, path, pc)
	case "+", "-", "*", "/", "%", "max", "min":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		processedArgs, err := p.processValueArgsParam(arr, path, pc)
		if err != nil {
			return expressionResult{}, err
		}
		sql, err := p.numericOp.ToSQLParam(operator, processedArgs, pc)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		return valueResult(sql, operators.ExpressionTypeNumber), nil
	case "cat", "substr":
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		processedArgs, err := p.processValueArgsParam(arr, path, pc)
		if err != nil {
			return expressionResult{}, err
		}
		sql, err := p.stringOp.ToSQLParam(operator, processedArgs, pc)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		return valueResult(sql, operators.ExpressionTypeString), nil
	case operators.OpMap, operators.OpFilter, operators.OpMerge:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		sql, err := p.arrayOp.ToSQLParamAtPath(operator, arr, pc, path)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		if arrayValueOperatorReturnsEmptyLiteral(operator, arr) {
			return literalValueResultWithRaw(sql, operators.ExpressionTypeArray, false, []interface{}{}), nil
		}
		return valueResult(sql, operators.ExpressionTypeArray), nil
	case operators.OpReduce:
		arr, ok := args.([]interface{})
		if !ok {
			return expressionResult{}, tperrors.NewOperatorRequiresArray(operator, path)
		}
		if len(arr) == 3 && isEmptyArrayLiteralValue(arr[0]) {
			return p.parseExpressionValueParam(arr[2], tperrors.BuildArrayPath(path, 2), pc)
		}
		sql, err := p.arrayOp.ToSQLParamAtPath(operator, arr, pc, path)
		if err != nil {
			return expressionResult{}, p.wrapOperatorError(operator, path, err)
		}
		return valueResult(sql, operators.ExpressionTypeUnknown), nil
	default:
		return expressionResult{}, tperrors.NewUnsupportedOperator(operator, path)
	}
}

func (p *Parser) parsePredicateLogicalParam(operator string, args []interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if len(args) == 0 {
		return expressionResult{}, tperrors.NewInsufficientArgs(operator, path, 1, 0)
	}
	checkpoint := pc.Checkpoint()
	parts := make([]string, 0, len(args))
	for i, arg := range args {
		res, err := p.parseExpressionPredicateParam(arg, tperrors.BuildArrayPath(path, i), pc)
		if err != nil {
			return expressionResult{}, err
		}
		if res.truthKnown {
			if operator == "and" && res.truthy {
				continue
			}
			if operator == "or" && !res.truthy {
				continue
			}
			if operator == "and" && !res.truthy {
				pc.Restore(checkpoint)
				return booleanPredicateResult(false), nil
			}
			if operator == "or" && res.truthy {
				pc.Restore(checkpoint)
				return booleanPredicateResult(true), nil
			}
		}
		parts = append(parts, res.SQL)
	}
	if len(parts) == 0 {
		return booleanPredicateResult(operator == "and"), nil
	}
	if len(parts) == 1 {
		return predicateResult(parts[0]), nil
	}
	joiner := " AND "
	if operator == "or" {
		joiner = " OR "
	}
	return predicateResult(fmt.Sprintf("(%s)", strings.Join(parts, joiner))), nil
}

func (p *Parser) parseNotPredicateParam(operator string, args interface{}, path string, double bool, pc *params.ParamCollector) (expressionResult, error) {
	arg, ok := unaryArg(args)
	if !ok {
		return expressionResult{}, tperrors.NewTypeMismatch(operator, path, "exactly 1 argument", "multiple arguments")
	}
	res, condition, err := p.parseTruthinessResultParam(arg, tperrors.BuildArrayPath(path, 0), pc)
	if err != nil {
		return expressionResult{}, err
	}
	if double {
		if res.truthKnown {
			return booleanPredicateResult(res.truthy), nil
		}
		return predicateResult(condition), nil
	}
	if res.truthKnown {
		return booleanPredicateResult(!res.truthy), nil
	}
	condition = operators.StripRedundantOuterParens(condition)
	return predicateResult(fmt.Sprintf("NOT (%s)", condition)), nil
}

func (p *Parser) parsePredicateIfParam(args []interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if len(args) < 2 {
		return expressionResult{}, tperrors.NewInsufficientArgs("if", path, 2, len(args))
	}
	var parts []string
	pairLimit := len(args)
	hasElse := len(args)%2 == 1
	if hasElse {
		pairLimit = len(args) - 1
	}
	for i := 0; i < pairLimit; i += 2 {
		cond, err := p.parsePredicateIfOperandParam(args[i], tperrors.BuildArrayPath(path, i), pc)
		if err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && !cond.truthy {
			continue
		}
		thenRes, err := p.parsePredicateIfOperandParam(args[i+1], tperrors.BuildArrayPath(path, i+1), pc)
		if err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && cond.truthy {
			if len(parts) == 0 {
				return thenRes, nil
			}
			return predicateResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), thenRes.SQL)), nil
		}
		parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", cond.SQL, thenRes.SQL))
	}
	elseSQL := "FALSE"
	if hasElse {
		elseRes, err := p.parsePredicateIfOperandParam(args[len(args)-1], tperrors.BuildArrayPath(path, len(args)-1), pc)
		if err != nil {
			return expressionResult{}, err
		}
		if len(parts) == 0 {
			return elseRes, nil
		}
		elseSQL = elseRes.SQL
	}
	if len(parts) == 0 {
		return booleanPredicateResult(false), nil
	}
	return predicateResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), elseSQL)), nil
}

func (p *Parser) parsePredicateIfOperandParam(expr interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if b, ok := expr.(bool); ok {
		return booleanPredicateResult(b), nil
	}
	return p.parseExpressionPredicateParam(expr, path, pc)
}

func (p *Parser) parseValueIfParam(args []interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if len(args) < 2 {
		return expressionResult{}, tperrors.NewInsufficientArgs("if", path, 2, len(args))
	}
	var parts []string
	resultType := operators.ExpressionTypeNull
	typeSet := false
	mergeResultType := func(res expressionResult) error {
		if !typeSet {
			resultType = valueTypeOf(res)
			typeSet = true
			return nil
		}
		typ, err := compatibleValueType(valueResult("", resultType), res, path)
		if err != nil {
			return err
		}
		resultType = typ
		return nil
	}
	pairLimit := len(args)
	hasElse := len(args)%2 == 1
	if hasElse {
		pairLimit = len(args) - 1
	}
	for i := 0; i < pairLimit; i += 2 {
		cond, condition, err := p.parseTruthinessResultParam(args[i], tperrors.BuildArrayPath(path, i), pc)
		if err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && !cond.truthy {
			continue
		}
		thenRes, err := p.parseExpressionValueParam(args[i+1], tperrors.BuildArrayPath(path, i+1), pc)
		if err != nil {
			return expressionResult{}, err
		}
		if err := mergeResultType(thenRes); err != nil {
			return expressionResult{}, err
		}
		if cond.truthKnown && cond.truthy {
			if len(parts) == 0 {
				return thenRes, nil
			}
			return valueResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), thenRes.SQL), resultType), nil
		}
		parts = append(parts, fmt.Sprintf("WHEN %s THEN %s", condition, thenRes.SQL))
	}
	elseSQL := "NULL"
	if hasElse {
		elseRes, err := p.parseExpressionValueParam(args[len(args)-1], tperrors.BuildArrayPath(path, len(args)-1), pc)
		if err != nil {
			return expressionResult{}, err
		}
		if len(parts) == 0 {
			return elseRes, nil
		}
		if err := mergeResultType(elseRes); err != nil {
			return expressionResult{}, err
		}
		elseSQL = elseRes.SQL
	}
	if len(parts) == 0 {
		return literalValueResult("NULL", operators.ExpressionTypeNull, false), nil
	}
	return valueResult(fmt.Sprintf("CASE %s ELSE %s END", strings.Join(parts, " "), elseSQL), resultType), nil
}

func (p *Parser) parseValueLogicalParam(operator string, args []interface{}, path string, pc *params.ParamCollector) (expressionResult, error) {
	if len(args) == 0 {
		return expressionResult{}, tperrors.NewInsufficientArgs(operator, path, 1, 0)
	}
	return p.parseValueLogicalFromParam(operator, args, 0, path, pc)
}

func (p *Parser) parseValueLogicalFromParam(operator string, args []interface{}, index int, path string, pc *params.ParamCollector) (expressionResult, error) {
	checkpoint := pc.Checkpoint()
	current, err := p.parseExpressionValueParam(args[index], tperrors.BuildArrayPath(path, index), pc)
	if err != nil {
		return expressionResult{}, err
	}
	if index == len(args)-1 {
		return current, nil
	}
	if current.truthKnown {
		if operator == "or" && current.truthy {
			return current, nil
		}
		if operator == "and" && !current.truthy {
			return current, nil
		}
		pc.Restore(checkpoint)
		return p.parseValueLogicalFromParam(operator, args, index+1, path, pc)
	}
	condition, err := p.truthinessSQL(current, tperrors.BuildArrayPath(path, index))
	if err != nil {
		return expressionResult{}, err
	}
	rest, err := p.parseValueLogicalFromParam(operator, args, index+1, path, pc)
	if err != nil {
		return expressionResult{}, err
	}
	resultType, err := compatibleValueType(current, rest, path)
	if err != nil {
		return expressionResult{}, err
	}
	if operator == "or" {
		return valueResult(fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, current.SQL, rest.SQL), resultType), nil
	}
	return valueResult(fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, rest.SQL, current.SQL), resultType), nil
}

// parseOperatorParam is the parameterized variant of parseOperator. Keep in sync.
func (p *Parser) parseOperatorParam(operator string, args interface{}, path string, pc *params.ParamCollector) (string, error) {
	if p.customOpLookup != nil {
		if handler, ok := p.customOpLookup(operator); ok {
			processedArgs, err := p.processCustomOperatorArgsParam(args, path, pc)
			if err != nil {
				return "", tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"failed to process custom operator arguments", err)
			}
			res, err := handler.ToSQL(operator, processedArgs)
			if err != nil {
				return "", tperrors.Wrap(tperrors.ErrCustomOperatorFailed, operator, path,
					"custom operator failed", err)
			}
			if ph, bad := params.FindQuotedPlaceholderRef(res.SQL, pc.Params(), pc.Style()); bad {
				return "", tperrors.New(tperrors.ErrCustomOperatorFailed, operator, path,
					fmt.Sprintf("custom operator produced invalid parameterized SQL: placeholder %s appears inside a quoted string literal", ph))
			}
			return res.SQL, nil
		}
	}

	switch operator {
	case "var":
		sql, err := p.dataOp.ToSQLParam(operator, []interface{}{args}, pc)
		return sql, p.wrapOperatorError(operator, path, err)
	case "missing":
		sql, err := p.dataOp.ToSQLParam(operator, []interface{}{args}, pc)
		return sql, p.wrapOperatorError(operator, path, err)
	case "missing_some":
		if arr, ok := args.([]interface{}); ok {
			sql, err := p.dataOp.ToSQLParam(operator, arr, pc)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	case "==", "===", "!=", "!==", ">", ">=", "<", "<=", "in":
		if arr, ok := args.([]interface{}); ok {
			processedArgs, err := p.processValueArgsParam(arr, path, pc)
			if err != nil {
				return "", err
			}
			sql, err := p.comparisonOp.ToSQLParam(operator, processedArgs, pc)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	case "and", "or", "if":
		if arr, ok := args.([]interface{}); ok {
			processedArgs, err := p.processArgsParam(arr, path, pc)
			if err != nil {
				return "", err
			}
			sql, err := p.logicalOp.ToSQLParam(operator, processedArgs, pc)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)
	case "!", "!!":
		if arr, ok := args.([]interface{}); ok {
			processedArgs, err := p.processArgsParam(arr, path, pc)
			if err != nil {
				return "", err
			}
			sql, err := p.logicalOp.ToSQLParam(operator, processedArgs, pc)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		processedArg, err := p.processArgParam(args, path, 0, pc)
		if err != nil {
			return "", err
		}
		sql, err := p.logicalOp.ToSQLParam(operator, []interface{}{processedArg}, pc)
		return sql, p.wrapOperatorError(operator, path, err)

	case "+", "-", "*", "/", "%", "max", "min":
		if arr, ok := args.([]interface{}); ok {
			processedArgs, err := p.processArgsParam(arr, path, pc)
			if err != nil {
				return "", err
			}
			sql, err := p.numericOp.ToSQLParam(operator, processedArgs, pc)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	case operators.OpMap, operators.OpFilter, operators.OpReduce, operators.OpAll, operators.OpSome, operators.OpNone, operators.OpMerge:
		if arr, ok := args.([]interface{}); ok {
			sql, err := p.arrayOp.ToSQLParamAtPath(operator, arr, pc, path)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	case "cat", "substr":
		if arr, ok := args.([]interface{}); ok {
			sql, err := p.stringOp.ToSQLParam(operator, arr, pc)
			return sql, p.wrapOperatorError(operator, path, err)
		}
		return "", tperrors.NewOperatorRequiresArray(operator, path)

	default:
		return "", tperrors.NewUnsupportedOperator(operator, path)
	}
}

// processArgsParam is the parameterized variant of processArgs. Keep in sync.
func (p *Parser) processArgsParam(args []interface{}, path string, pc *params.ParamCollector) ([]interface{}, error) {
	processed := make([]interface{}, len(args))
	for i, arg := range args {
		processedArg, err := p.processArgParam(arg, path, i, pc)
		if err != nil {
			return nil, err
		}
		processed[i] = processedArg
	}
	return processed, nil
}

func (p *Parser) processValueArgsParam(args []interface{}, path string, pc *params.ParamCollector) ([]interface{}, error) {
	processed := make([]interface{}, len(args))
	for i, arg := range args {
		processedArg, err := p.processValueArgParam(arg, path, i, pc)
		if err != nil {
			return nil, err
		}
		processed[i] = processedArg
	}
	return processed, nil
}

func (p *Parser) processValueArgParam(arg interface{}, path string, index int, pc *params.ParamCollector) (interface{}, error) {
	if p.isPrimitive(arg) {
		return arg, nil
	}
	if exprMap, ok := arg.(map[string]interface{}); ok && len(exprMap) == 1 {
		for operator := range exprMap {
			if operator != "var" {
				checkpoint := pc.Checkpoint()
				res, err := p.parseExpressionValueParam(arg, tperrors.BuildArrayPath(path, index), pc)
				if err != nil {
					return nil, err
				}
				if res.rawLiteralKnown {
					pc.Restore(checkpoint)
					return res.rawLiteral, nil
				}
				return typedValueOperand(res), nil
			}
		}
	}
	return p.processArgParam(arg, path, index, pc)
}

// processArgParam is the parameterized variant of processArg. Keep in sync.
func (p *Parser) processArgParam(arg interface{}, path string, index int, pc *params.ParamCollector) (interface{}, error) {
	if exprMap, ok := arg.(map[string]interface{}); ok {
		if len(exprMap) == 1 {
			for operator, opArgs := range exprMap {
				operatorPath := tperrors.BuildPath(path, operator, index)

				if !p.isBuiltInOperator(operator) {
					sql, err := p.parseOperatorParam(operator, opArgs, operatorPath, pc)
					if err != nil {
						return nil, err
					}
					return operators.SQLResult(sql), nil
				}

				if p.isArrayOperator(operator) {
					sql, err := p.parseOperatorParam(operator, opArgs, operatorPath, pc)
					if err != nil {
						return nil, err
					}
					return operators.SQLResult(sql), nil
				}
				processedOpArgs, err := p.processOpArgsParam(opArgs, operatorPath, pc)
				if err != nil {
					return nil, err
				}
				return map[string]interface{}{operator: processedOpArgs}, nil
			}
		}
		return arg, nil
	}

	if arr, ok := arg.([]interface{}); ok {
		argPath := tperrors.BuildArrayPath(path, index)
		return p.processArgsParam(arr, argPath, pc)
	}

	return arg, nil
}

// processOpArgsParam is the parameterized variant of processOpArgs. Keep in sync.
func (p *Parser) processOpArgsParam(opArgs interface{}, path string, pc *params.ParamCollector) (interface{}, error) {
	if arr, ok := opArgs.([]interface{}); ok {
		return p.processArgsParam(arr, path, pc)
	}
	return p.processArgParam(opArgs, path, 0, pc)
}

// processCustomOperatorArgsParam is the parameterized variant of processCustomOperatorArgs. Keep in sync.
func (p *Parser) processCustomOperatorArgsParam(args interface{}, path string, pc *params.ParamCollector) ([]operators.OperatorArg, error) {
	if arr, ok := args.([]interface{}); ok {
		processed := make([]operators.OperatorArg, len(arr))
		for i, arg := range arr {
			argPath := tperrors.BuildArrayPath(path, i)
			argResult, err := p.processArgToOperatorArgParam(arg, argPath, pc)
			if err != nil {
				return nil, err
			}
			processed[i] = argResult
		}
		return processed, nil
	}

	argResult, err := p.processArgToOperatorArgParam(args, path, pc)
	if err != nil {
		return nil, err
	}
	return []operators.OperatorArg{argResult}, nil
}

func (p *Parser) processArgToOperatorArgParam(arg interface{}, path string, pc *params.ParamCollector) (operators.OperatorArg, error) {
	res, err := p.parseExpressionAnyParam(arg, path, pc)
	if err != nil {
		return operators.OperatorArg{}, err
	}
	return operators.OperatorArg{SQL: res.SQL, Kind: res.Kind, Type: valueTypeOf(res)}, nil
}
