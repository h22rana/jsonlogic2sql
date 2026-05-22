// Package dialect provides SQL dialect definitions for the transpiler.
package dialect

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Dialect represents a SQL dialect that the transpiler can target.
type Dialect int

const (
	// DialectUnspecified is the zero value, indicating no dialect was set.
	// This will cause an error if used - users must explicitly set a dialect.
	DialectUnspecified Dialect = iota

	// DialectBigQuery targets Google BigQuery SQL syntax.
	DialectBigQuery

	// DialectSpanner targets Google Cloud Spanner SQL syntax.
	DialectSpanner

	// DialectPostgreSQL targets PostgreSQL SQL syntax.
	DialectPostgreSQL

	// DialectDuckDB targets DuckDB SQL syntax.
	DialectDuckDB

	// DialectClickHouse targets ClickHouse SQL syntax.
	DialectClickHouse
)

// String returns the string representation of the dialect.
func (d Dialect) String() string {
	switch d {
	case DialectBigQuery:
		return "BigQuery"
	case DialectSpanner:
		return "Spanner"
	case DialectPostgreSQL:
		return "PostgreSQL"
	case DialectDuckDB:
		return "DuckDB"
	case DialectClickHouse:
		return "ClickHouse"
	case DialectUnspecified:
		return "Unspecified"
	default:
		return fmt.Sprintf("Unknown(%d)", int(d))
	}
}

// IsValid returns true if the dialect is a valid, specified dialect.
func (d Dialect) IsValid() bool {
	switch d {
	case DialectBigQuery, DialectSpanner, DialectPostgreSQL, DialectDuckDB, DialectClickHouse:
		return true
	case DialectUnspecified:
		return false
	default:
		return false
	}
}

// Validate returns an error if the dialect is not valid.
func (d Dialect) Validate() error {
	if d == DialectUnspecified {
		return fmt.Errorf("dialect not specified: must set Dialect in TranspilerConfig (use DialectBigQuery, DialectSpanner, DialectPostgreSQL, DialectDuckDB, or DialectClickHouse)")
	}
	if !d.IsValid() {
		return fmt.Errorf("unsupported dialect: %s", d.String())
	}
	return nil
}

// NeedsQuoting returns true if an identifier segment requires quoting.
// A segment needs quoting if it starts with a digit or contains characters
// other than letters, digits, and underscores.
func NeedsQuoting(segment string) bool {
	if segment == "" {
		return false
	}
	first, _ := utf8.DecodeRuneInString(segment)
	if unicode.IsDigit(first) {
		return true
	}
	for _, r := range segment {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return true
		}
	}
	return false
}

// ContainsQuoteCharacters returns true if the segment contains backticks, double
// quotes, or single quotes. These characters are used for identifier quoting and
// must not appear in raw variable names — the transpiler handles quoting automatically.
func ContainsQuoteCharacters(segment string) bool {
	return strings.ContainsAny(segment, "`\"'")
}

// QuoteIdentifierSegment wraps a single identifier segment with dialect-appropriate
// quote characters. It also escapes any embedded quote characters within the segment.
//   - BigQuery / Spanner / ClickHouse: backtick (`)
//   - PostgreSQL / DuckDB: double quote (")
func QuoteIdentifierSegment(segment string, d Dialect) string {
	//nolint:exhaustive // default uses backtick (safe for GoogleSQL family)
	switch d {
	case DialectPostgreSQL, DialectDuckDB:
		escaped := strings.ReplaceAll(segment, `"`, `""`)
		return `"` + escaped + `"`
	default:
		escaped := strings.ReplaceAll(segment, "`", "``")
		return "`" + escaped + "`"
	}
}
