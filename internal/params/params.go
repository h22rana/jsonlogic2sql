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

// ValidatePlaceholderRefs is a best-effort safety guard that scans the final
// SQL for each collected placeholder using style-specific boundary patterns.
// It returns E350 ErrUnreferencedPlaceholder if any placeholder is not found,
// indicating a custom operator may have dropped an argument.
//
// This is NOT a strict SQL token parser. It may produce false positives if a
// custom operator emits SQL containing placeholder-like text inside string
// literals (e.g., '@p1') or SQL comments (e.g., -- @p1, /* $1 */).
// In normal usage this does not occur because the parameterized pipeline
// replaces all user-originated literals with placeholders.
func ValidatePlaceholderRefs(sql string, params []QueryParam, style PlaceholderStyle) error {
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

func containsPlaceholderRef(sql, placeholder string, style PlaceholderStyle) bool {
	if style == PlaceholderQuestion {
		return strings.Contains(sql, placeholder)
	}

	searchFrom := 0
	for {
		idx := strings.Index(sql[searchFrom:], placeholder)
		if idx < 0 {
			return false
		}
		start := searchFrom + idx
		end := start + len(placeholder)
		if hasPlaceholderBoundaries(sql, start, end, style) {
			return true
		}
		searchFrom = start + 1
	}
}

// FindQuotedPlaceholderRef scans SQL string literals and returns the first
// placeholder token found inside a quoted string, if any.
//
// This helps catch custom operators that accidentally quote placeholders
// (e.g. "'@p1'"), which breaks bind semantics.
func FindQuotedPlaceholderRef(sql string, params []QueryParam, style PlaceholderStyle) (string, bool) {
	if len(params) == 0 || style == PlaceholderQuestion {
		return "", false
	}

	placeholders := make([]string, 0, len(params))
	for i, p := range params {
		placeholders = append(placeholders, formatPlaceholder(i+1, p.Name, style))
	}

	inString := false
	stringStart := 0

	for i := 0; i < len(sql); i++ {
		if sql[i] != '\'' {
			continue
		}

		if !inString {
			inString = true
			stringStart = i + 1
			continue
		}

		// SQL escaped quote inside string literal.
		if i+1 < len(sql) && sql[i+1] == '\'' {
			i++
			continue
		}

		// Closing quote found.
		literal := sql[stringStart:i]
		if ph, ok := findPlaceholderInLiteral(literal, placeholders, style); ok {
			return ph, true
		}
		inString = false
	}

	return "", false
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
