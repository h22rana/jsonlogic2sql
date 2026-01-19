// Package dialect provides SQL dialect definitions for the transpiler.
package dialect

import "fmt"

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
	return d == DialectBigQuery || d == DialectSpanner || d == DialectPostgreSQL || d == DialectDuckDB || d == DialectClickHouse
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
