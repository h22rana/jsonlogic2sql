// Package dialect provides SQL dialect definitions for the transpiler.
package dialect

import (
	"fmt"
	"strings"
	"unicode"
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
// outside the portable unquoted ASCII set [A-Za-z0-9_]. Schema validation may
// allow Unicode letters and digits, but quoting them keeps generated SQL
// portable across the supported dialects.
func NeedsQuoting(segment string) bool {
	if segment == "" {
		return false
	}
	if segment[0] >= '0' && segment[0] <= '9' {
		return true
	}
	for i := 0; i < len(segment); i++ {
		c := segment[i]
		if (c >= 'A' && c <= 'Z') ||
			(c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '_' {
			continue
		}
		return true
	}
	return false
}

// IsSafeIdentifierSegment reports whether a raw schema/var path segment is
// made only of letters, digits, or underscores. Dialect-specific quoting is
// handled later by NeedsQuoting and QuoteIdentifierSegment.
func IsSafeIdentifierSegment(segment string) bool {
	if segment == "" {
		return false
	}
	for _, r := range segment {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
	}
	return true
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
