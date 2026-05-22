// Package params provides parameter collection and placeholder generation
// for parameterized SQL output from the jsonlogic2sql transpiler.
package params

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
)

// PlaceholderStyle controls the placeholder token format in generated SQL.
type PlaceholderStyle int

const (
	// PlaceholderNamed uses @p1, @p2, ... (BigQuery, Spanner, ClickHouse).
	PlaceholderNamed PlaceholderStyle = iota
	// PlaceholderPositional uses $1, $2, ... (PostgreSQL, DuckDB).
	PlaceholderPositional
	// PlaceholderQuestion uses sequential ? placeholders.
	// Reserved for future opt-in; no dialect maps here by default.
	PlaceholderQuestion
)

// QueryParam represents a single bind parameter collected during parameterized transpilation.
type QueryParam struct {
	Name  string
	Value interface{}
}

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
	case dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectClickHouse, dialect.DialectUnspecified:
		return PlaceholderNamed
	}
	return PlaceholderNamed
}

// Add registers a new bind parameter and returns the dialect-appropriate
// placeholder token to embed in the SQL string.
func (pc *ParamCollector) Add(value interface{}) string {
	pc.count++
	name := "p" + strconv.Itoa(pc.count)
	pc.params = append(pc.params, QueryParam{Name: name, Value: value})

	switch pc.style {
	case PlaceholderNamed:
		return "@" + name
	case PlaceholderPositional:
		return "$" + strconv.Itoa(pc.count)
	case PlaceholderQuestion:
		return "?"
	default:
		return "@" + name
	}
}

// Params returns the collected parameters in insertion order.
func (pc *ParamCollector) Params() []QueryParam {
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
		if formatPlaceholder(i+1, p.Name, pc.style) == placeholder {
			return p.Value, true
		}
	}
	return nil, false
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
		placeholder := formatPlaceholder(i+1, p.Name, style)
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
	return containsPlaceholderRef(sql, formatPlaceholder(index, param.Name, style), style)
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
		placeholders = append(placeholders, formatPlaceholder(i+1, p.Name, style))
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

// formatPlaceholder returns the placeholder string for a given index/name/style.
func formatPlaceholder(index int, name string, style PlaceholderStyle) string {
	switch style {
	case PlaceholderNamed:
		return "@" + name
	case PlaceholderPositional:
		return "$" + strconv.Itoa(index)
	case PlaceholderQuestion:
		return "?"
	default:
		return "@" + name
	}
}
