// Package params provides parameter collection and placeholder generation
// for parameterized SQL output from the jsonlogic2sql transpiler.
package params

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
)

// PlaceholderStyle controls the placeholder token format in generated SQL.
type PlaceholderStyle int

const (
	// PlaceholderNamed uses @p1, @p2, ... (BigQuery, Spanner).
	PlaceholderNamed PlaceholderStyle = iota
	// PlaceholderPositional uses $1, $2, ... (PostgreSQL, DuckDB).
	PlaceholderPositional
	// PlaceholderQuestion uses sequential ? placeholders.
	// Reserved for future opt-in; no dialect maps here by default.
	PlaceholderQuestion
	// PlaceholderClickHouse uses ClickHouse query parameters: {p1:String},
	// {p2:Float64}, and so on.
	PlaceholderClickHouse
)

// QueryParam represents a single bind parameter collected during parameterized transpilation.
type QueryParam struct {
	Name           string
	Value          interface{}
	clickHouseType string
}

const (
	paramNamePrefix             = "p"
	namedPlaceholderPrefix      = "@"
	positionalPlaceholderPrefix = "$"
	questionPlaceholder         = "?"
	clickHousePlaceholderOpen   = "{"
	clickHousePlaceholderSep    = ":"
	clickHousePlaceholderClose  = "}"
	clickHouseTypeString        = "String"
	clickHouseTypeInt64         = "Int64"
	clickHouseTypeUInt64        = "UInt64"
	clickHouseTypeInt128        = "Int128"
	clickHouseTypeUInt128       = "UInt128"
	clickHouseTypeInt256        = "Int256"
	clickHouseTypeUInt256       = "UInt256"
	clickHouseTypeFloat32       = "Float32"
	clickHouseTypeFloat64       = "Float64"
	clickHouseTypeBool          = "Bool"

	clickHouseInt128Bits  = 128
	clickHouseInt256Bits  = 256
	clickHouseUInt128Bits = 128
	clickHouseUInt256Bits = 256
)

// ParamCollector accumulates bind parameters and generates placeholder tokens
// during parameterized SQL generation. It is created per-call and passed
// through method parameters to ensure thread-safety.
type ParamCollector struct {
	params []QueryParam
	style  PlaceholderStyle
	count  int
}

// Checkpoint captures a ParamCollector position for rollback when speculative
// parsing emits placeholders that are not used in the final SQL.
type Checkpoint struct {
	count  int
	length int
}

// NewParamCollector creates a ParamCollector for the given placeholder style.
func NewParamCollector(style PlaceholderStyle) *ParamCollector {
	return &ParamCollector{
		style: style,
	}
}

// Checkpoint returns the current collector position.
func (pc *ParamCollector) Checkpoint() Checkpoint {
	return Checkpoint{
		count:  pc.count,
		length: len(pc.params),
	}
}

// Restore rolls the collector back to a previously captured checkpoint.
func (pc *ParamCollector) Restore(checkpoint Checkpoint) {
	pc.count = checkpoint.count
	pc.params = pc.params[:checkpoint.length]
}

// StyleForDialect returns the default PlaceholderStyle for a SQL dialect.
func StyleForDialect(d dialect.Dialect) PlaceholderStyle {
	switch d {
	case dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		return PlaceholderPositional
	case dialect.DialectClickHouse:
		return PlaceholderClickHouse
	case dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectUnspecified:
		return PlaceholderNamed
	}
	return PlaceholderNamed
}

// Add registers a new bind parameter and returns the dialect-appropriate
// placeholder token to embed in the SQL string.
func (pc *ParamCollector) Add(value interface{}) string {
	return pc.add(value, "")
}

// AddExactNumberString registers a numeric literal that is stored as a string
// to preserve precision. The retained numeric type hint keeps exact numeric
// strings distinct from ordinary string literals for ClickHouse placeholders and
// internal type inference.
func (pc *ParamCollector) AddExactNumberString(value string) string {
	return pc.add(value, clickHouseExactNumberStringType(value))
}

// AddNumericString registers a string literal that has already been proven to
// be used in a numeric context. It binds ordinary integers/floats as numeric Go
// values and preserves oversized numeric literals as exact strings with
// dialect-specific numeric metadata.
func (pc *ParamCollector) AddNumericString(value string) (string, bool) {
	numericValue, clickHouseType, ok := numericStringParamValue(value)
	if !ok {
		return "", false
	}
	return pc.add(numericValue, clickHouseType), true
}

// RewriteStringParamAsNumeric converts an already-collected string parameter to
// the same numeric parameter shape used by numeric operators. It is intended for
// paths where an operand must be collected before the surrounding expression can
// prove that a JSON string is being used in a numeric context.
func (pc *ParamCollector) RewriteStringParamAsNumeric(index int, value string) (string, bool) {
	if index < 0 || index >= len(pc.params) {
		return "", false
	}
	current, ok := pc.params[index].Value.(string)
	if !ok || current != value || pc.params[index].clickHouseType != "" {
		return "", false
	}

	numericValue, clickHouseType, ok := numericStringParamValue(value)
	if !ok {
		return "", false
	}
	pc.params[index].Value = numericValue
	pc.params[index].clickHouseType = clickHouseType
	return FormatPlaceholderForParam(index+1, pc.params[index], pc.style), true
}

func (pc *ParamCollector) add(value interface{}, sqlType string) string {
	pc.count++
	name := paramNamePrefix + strconv.Itoa(pc.count)
	param := QueryParam{Name: name, Value: value, clickHouseType: sqlType}
	pc.params = append(pc.params, param)

	switch pc.style {
	case PlaceholderNamed:
		return namedPlaceholderPrefix + name
	case PlaceholderPositional:
		return positionalPlaceholderPrefix + strconv.Itoa(pc.count)
	case PlaceholderQuestion:
		return questionPlaceholder
	case PlaceholderClickHouse:
		return clickHousePlaceholder(param)
	default:
		return namedPlaceholderPrefix + name
	}
}

// Params returns the collected parameters in insertion order.
func (pc *ParamCollector) Params() []QueryParam {
	if pc.params == nil {
		return nil
	}
	publicParams := make([]QueryParam, len(pc.params))
	for i, param := range pc.params {
		publicParams[i] = QueryParam{Name: param.Name, Value: param.Value}
	}
	return publicParams
}

// RawParams returns collected parameters with internal placeholder metadata.
// Use this for placeholder validation/formatting; public API returns should use
// Params so QueryParam remains a simple name/value pair.
func (pc *ParamCollector) RawParams() []QueryParam {
	return pc.params
}

// Count returns the number of collected parameters.
func (pc *ParamCollector) Count() int {
	return pc.count
}

// Style returns the placeholder style used by this collector.
func (pc *ParamCollector) Style() PlaceholderStyle {
	return pc.style
}

// ValueForPlaceholder returns the stored value for a given placeholder string.
// Returns the value and true if found, or nil and false otherwise.
// This allows callers to inspect the Go type of a parameterized value when the
// placeholder token alone is insufficient to determine semantics (e.g.,
// distinguishing string containment from array membership in the "in" operator).
func (pc *ParamCollector) ValueForPlaceholder(placeholder string) (interface{}, bool) {
	for i, p := range pc.params {
		if FormatPlaceholderForParam(i+1, p, pc.style) == placeholder {
			return p.Value, true
		}
	}
	return nil, false
}

// PlaceholderValueIsString reports whether a placeholder was produced from an
// actual string literal. Exact numeric strings carry internal type metadata and
// intentionally return false here.
func (pc *ParamCollector) PlaceholderValueIsString(placeholder string) bool {
	for i, p := range pc.params {
		if FormatPlaceholderForParam(i+1, p, pc.style) != placeholder {
			continue
		}
		_, ok := p.Value.(string)
		return ok && p.clickHouseType == ""
	}
	return false
}

// ValidatePlaceholderRefs is a safety guard that scans the final SQL for each
// collected placeholder using style-specific boundary patterns outside quoted
// SQL regions and SQL comments.
// It returns E350 ErrUnreferencedPlaceholder if any placeholder is not found,
// indicating a custom operator may have dropped an argument.
func ValidatePlaceholderRefs(sql string, params []QueryParam, style PlaceholderStyle) error {
	if style == PlaceholderQuestion {
		found := countQuestionPlaceholderRefs(sql)
		for i, p := range params {
			if i >= found {
				return tperrors.New(tperrors.ErrUnreferencedPlaceholder, "", "",
					fmt.Sprintf("placeholder ? (param %q) is not referenced in generated SQL; "+
						"a custom operator may have dropped an argument", p.Name))
			}
		}
		return nil
	}

	for i, p := range params {
		placeholder := FormatPlaceholderForParam(i+1, p, style)
		if !containsPlaceholderRef(sql, placeholder, style) {
			return tperrors.New(tperrors.ErrUnreferencedPlaceholder, "", "",
				fmt.Sprintf("placeholder %s (param %q) is not referenced in generated SQL; "+
					"a custom operator may have dropped an argument", placeholder, p.Name))
		}
	}
	return nil
}

// ContainsParamRef reports whether SQL references the indexed bind parameter
// outside quoted strings and comments. Index is one-based, matching generated
// positional placeholders such as $1.
func ContainsParamRef(sql string, index int, param QueryParam, style PlaceholderStyle) bool {
	return containsPlaceholderRef(sql, FormatPlaceholderForParam(index, param, style), style)
}

func containsPlaceholderRef(sql, placeholder string, style PlaceholderStyle) bool {
	for i := 0; i < len(sql); i++ {
		switch {
		case sql[i] == '\'' || sql[i] == '"' || sql[i] == '`':
			i = skipSQLQuotedRegion(sql, i, sql[i])
			continue
		case i+1 < len(sql) && sql[i] == '-' && sql[i+1] == '-':
			i = skipSQLLineComment(sql, i+2)
			continue
		case i+1 < len(sql) && sql[i] == '/' && sql[i+1] == '*':
			i = skipSQLBlockComment(sql, i+2)
			continue
		case sql[i] == '$':
			if _, _, end, ok := sqlDollarQuotedLiteral(sql, i); ok {
				i = end
				continue
			}
		}

		if isPlaceholderAt(sql, placeholder, i, style) {
			return true
		}
	}
	return false
}

func countQuestionPlaceholderRefs(sql string) int {
	count := 0
	for i := 0; i < len(sql); i++ {
		switch {
		case sql[i] == '\'' || sql[i] == '"' || sql[i] == '`':
			i = skipSQLQuotedRegion(sql, i, sql[i])
			continue
		case i+1 < len(sql) && sql[i] == '-' && sql[i+1] == '-':
			i = skipSQLLineComment(sql, i+2)
			continue
		case i+1 < len(sql) && sql[i] == '/' && sql[i+1] == '*':
			i = skipSQLBlockComment(sql, i+2)
			continue
		case sql[i] == '$':
			if _, _, end, ok := sqlDollarQuotedLiteral(sql, i); ok {
				i = end
				continue
			}
		case sql[i] == '?':
			count++
		}
	}
	return count
}

func skipSQLQuotedRegion(sql string, start int, quote byte) int {
	for i := start + 1; i < len(sql); i++ {
		if sql[i] != quote {
			continue
		}
		if i+1 < len(sql) && sql[i+1] == quote {
			i++
			continue
		}
		return i
	}
	return len(sql) - 1
}

func skipSQLLineComment(sql string, start int) int {
	for i := start; i < len(sql); i++ {
		if sql[i] == '\n' || sql[i] == '\r' {
			return i
		}
	}
	return len(sql) - 1
}

func skipSQLBlockComment(sql string, start int) int {
	for i := start; i+1 < len(sql); i++ {
		if sql[i] == '*' && sql[i+1] == '/' {
			return i + 1
		}
	}
	return len(sql) - 1
}

func sqlDollarQuotedLiteral(sql string, start int) (int, int, int, bool) {
	delimiter, ok := dollarQuoteDelimiterAt(sql, start)
	if !ok {
		return 0, 0, 0, false
	}
	contentStart := start + len(delimiter)
	closingOffset := strings.Index(sql[contentStart:], delimiter)
	if closingOffset < 0 {
		return contentStart, len(sql), len(sql) - 1, true
	}
	contentEnd := contentStart + closingOffset
	return contentStart, contentEnd, contentEnd + len(delimiter) - 1, true
}

func dollarQuoteDelimiterAt(sql string, start int) (string, bool) {
	if start >= len(sql) || sql[start] != '$' || start+1 >= len(sql) {
		return "", false
	}
	if sql[start+1] == '$' {
		return "$$", true
	}
	if !isDollarQuoteTagFirstChar(sql[start+1]) {
		return "", false
	}
	for i := start + 2; i < len(sql); i++ {
		if sql[i] == '$' {
			return sql[start : i+1], true
		}
		if !isDollarQuoteTagChar(sql[i]) {
			return "", false
		}
	}
	return "", false
}

func isDollarQuoteTagFirstChar(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isDollarQuoteTagChar(ch byte) bool {
	return isDollarQuoteTagFirstChar(ch) || (ch >= '0' && ch <= '9')
}

func isPlaceholderAt(sql, placeholder string, start int, style PlaceholderStyle) bool {
	if !strings.HasPrefix(sql[start:], placeholder) {
		return false
	}
	if style == PlaceholderQuestion {
		return true
	}
	return hasPlaceholderBoundaries(sql, start, start+len(placeholder), style)
}

// FindQuotedPlaceholderRef scans non-bindable quoted SQL regions and returns
// the first placeholder token found inside one, if any.
//
// This helps catch custom operators that accidentally quote placeholders
// (e.g. "'@p1'" or "`@p1`"), which breaks bind semantics.
func FindQuotedPlaceholderRef(sql string, params []QueryParam, style PlaceholderStyle) (string, bool) {
	return FindQuotedPlaceholderRefAfter(sql, params, style, 0)
}

// FindQuotedPlaceholderRefAfter is like FindQuotedPlaceholderRef, but ignores
// placeholders allocated before previousCount. Use it when validating a custom
// operator result so unrelated params from earlier operands do not make quoted
// literal text look like a dropped bind parameter.
func FindQuotedPlaceholderRefAfter(sql string, params []QueryParam, style PlaceholderStyle, previousCount int) (string, bool) {
	if previousCount < 0 {
		previousCount = 0
	}
	if previousCount > len(params) {
		previousCount = len(params)
	}
	if previousCount == len(params) || style == PlaceholderQuestion {
		return "", false
	}

	placeholders := make([]string, 0, len(params)-previousCount)
	for i := previousCount; i < len(params); i++ {
		p := params[i]
		placeholders = append(placeholders, FormatPlaceholderForParam(i+1, p, style))
	}

	for i := 0; i < len(sql); i++ {
		if sql[i] == '$' {
			contentStart, contentEnd, end, ok := sqlDollarQuotedLiteral(sql, i)
			if !ok {
				continue
			}
			if ph, ok := findPlaceholderInLiteral(sql[contentStart:contentEnd], placeholders, style); ok {
				return ph, true
			}
			i = end
			continue
		}
		if sql[i] != '\'' && sql[i] != '"' && sql[i] != '`' {
			continue
		}

		contentStart, contentEnd, end := sqlQuotedRegion(sql, i, sql[i])
		literal := sql[contentStart:contentEnd]
		if ph, ok := findPlaceholderInLiteral(literal, placeholders, style); ok {
			return ph, true
		}
		i = end
	}

	return "", false
}

func sqlQuotedRegion(sql string, start int, quote byte) (int, int, int) {
	contentStart := start + 1
	for i := contentStart; i < len(sql); i++ {
		if sql[i] != quote {
			continue
		}
		if i+1 < len(sql) && sql[i+1] == quote {
			i++
			continue
		}
		return contentStart, i, i
	}
	return contentStart, len(sql), len(sql) - 1
}

func findPlaceholderInLiteral(literal string, placeholders []string, style PlaceholderStyle) (string, bool) {
	for _, ph := range placeholders {
		searchFrom := 0
		for {
			idx := strings.Index(literal[searchFrom:], ph)
			if idx < 0 {
				break
			}
			start := searchFrom + idx
			end := start + len(ph)
			if hasPlaceholderBoundaries(literal, start, end, style) {
				return ph, true
			}
			searchFrom = start + 1
		}
	}
	return "", false
}

func hasPlaceholderBoundaries(s string, start, end int, style PlaceholderStyle) bool {
	beforeOK := start == 0 || !isPlaceholderBoundaryChar(s[start-1], style)
	afterOK := end == len(s) || !isPlaceholderBoundaryChar(s[end], style)
	return beforeOK && afterOK
}

func isPlaceholderBoundaryChar(ch byte, style PlaceholderStyle) bool {
	if ch == '_' || (ch >= '0' && ch <= '9') || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
		return true
	}
	return style == PlaceholderPositional && ch == '$'
}

// FormatPlaceholderForParam returns the placeholder token for an already
// collected parameter. It is used by validation code that needs exact
// dialect-specific placeholder text without allocating a new parameter.
func FormatPlaceholderForParam(index int, param QueryParam, style PlaceholderStyle) string {
	switch style {
	case PlaceholderNamed:
		return namedPlaceholderPrefix + param.Name
	case PlaceholderPositional:
		return positionalPlaceholderPrefix + strconv.Itoa(index)
	case PlaceholderQuestion:
		return questionPlaceholder
	case PlaceholderClickHouse:
		return clickHousePlaceholder(param)
	default:
		return namedPlaceholderPrefix + param.Name
	}
}

func clickHousePlaceholder(param QueryParam) string {
	return clickHousePlaceholderOpen + param.Name + clickHousePlaceholderSep + clickHouseParamType(param) + clickHousePlaceholderClose
}

func clickHouseParamType(param QueryParam) string {
	if param.clickHouseType != "" {
		return param.clickHouseType
	}
	switch param.Value.(type) {
	case string:
		return clickHouseTypeString
	case int, int8, int16, int32, int64:
		return clickHouseTypeInt64
	case uint, uint8, uint16, uint32, uint64:
		return clickHouseTypeUInt64
	case float32:
		return clickHouseTypeFloat32
	case float64:
		return clickHouseTypeFloat64
	case bool:
		return clickHouseTypeBool
	default:
		return clickHouseTypeString
	}
}

func clickHouseExactNumberStringType(value string) string {
	trimmed := strings.TrimSpace(value)
	if isSignedDecimalIntegerLiteral(trimmed) {
		return clickHouseIntegerStringType(trimmed)
	}
	return clickHouseTypeFloat64
}

func numericStringParamValue(value string) (interface{}, string, bool) {
	trimmed := strings.TrimSpace(value)
	if isSignedDecimalIntegerLiteral(trimmed) {
		n, err := strconv.ParseInt(trimmed, 10, 64)
		if err == nil {
			return n, "", true
		}
		return trimmed, clickHouseExactNumberStringType(trimmed), true
	}

	num, err := strconv.ParseFloat(trimmed, 64)
	if err != nil || math.IsNaN(num) || math.IsInf(num, 0) {
		return nil, "", false
	}
	return num, "", true
}

func clickHouseIntegerStringType(value string) string {
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return clickHouseTypeInt64
	}
	if !strings.HasPrefix(value, "-") {
		unsigned := strings.TrimPrefix(value, "+")
		if _, err := strconv.ParseUint(unsigned, 10, 64); err == nil {
			return clickHouseTypeUInt64
		}
		if fitsUnsignedDecimalIntegerBits(unsigned, clickHouseUInt128Bits) {
			return clickHouseTypeUInt128
		}
		if fitsUnsignedDecimalIntegerBits(unsigned, clickHouseUInt256Bits) {
			return clickHouseTypeUInt256
		}
		return clickHouseTypeFloat64
	}
	if fitsSignedDecimalIntegerBits(value, clickHouseInt128Bits) {
		return clickHouseTypeInt128
	}
	if fitsSignedDecimalIntegerBits(value, clickHouseInt256Bits) {
		return clickHouseTypeInt256
	}
	return clickHouseTypeFloat64
}

func isSignedDecimalIntegerLiteral(value string) bool {
	if value == "" {
		return false
	}
	start := 0
	if value[0] == '+' || value[0] == '-' {
		start = 1
	}
	if start == len(value) {
		return false
	}
	for i := start; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func fitsSignedDecimalIntegerBits(value string, bits uint) bool {
	n, ok := parseDecimalInteger(value)
	if !ok {
		return false
	}
	limit := new(big.Int).Lsh(big.NewInt(1), bits-1)
	lowerBound := new(big.Int).Neg(new(big.Int).Set(limit))
	upperBound := new(big.Int).Sub(limit, big.NewInt(1))
	return n.Cmp(lowerBound) >= 0 && n.Cmp(upperBound) <= 0
}

func fitsUnsignedDecimalIntegerBits(value string, bits uint) bool {
	if strings.HasPrefix(value, "-") {
		return false
	}
	n, ok := parseDecimalInteger(value)
	if !ok {
		return false
	}
	upperBound := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), bits), big.NewInt(1))
	return n.Sign() >= 0 && n.Cmp(upperBound) <= 0
}

func parseDecimalInteger(value string) (*big.Int, bool) {
	normalized := strings.TrimPrefix(value, "+")
	n, ok := new(big.Int).SetString(normalized, 10)
	return n, ok
}
