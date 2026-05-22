package dialect

import (
	"testing"
)

func TestDialect_String(t *testing.T) {
	tests := []struct {
		dialect  Dialect
		expected string
	}{
		{DialectBigQuery, "BigQuery"},
		{DialectSpanner, "Spanner"},
		{DialectPostgreSQL, "PostgreSQL"},
		{DialectDuckDB, "DuckDB"},
		{DialectClickHouse, "ClickHouse"},
		{DialectUnspecified, "Unspecified"},
		{Dialect(999), "Unknown(999)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.dialect.String(); got != tt.expected {
				t.Errorf("Dialect.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDialect_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		dialect  Dialect
		expected bool
	}{
		{"BigQuery is valid", DialectBigQuery, true},
		{"Spanner is valid", DialectSpanner, true},
		{"PostgreSQL is valid", DialectPostgreSQL, true},
		{"DuckDB is valid", DialectDuckDB, true},
		{"ClickHouse is valid", DialectClickHouse, true},
		{"Unspecified is not valid", DialectUnspecified, false},
		{"Unknown dialect is not valid", Dialect(999), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dialect.IsValid(); got != tt.expected {
				t.Errorf("Dialect.IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDialect_Validate(t *testing.T) {
	tests := []struct {
		name        string
		dialect     Dialect
		expectError bool
	}{
		{"BigQuery validates", DialectBigQuery, false},
		{"Spanner validates", DialectSpanner, false},
		{"PostgreSQL validates", DialectPostgreSQL, false},
		{"DuckDB validates", DialectDuckDB, false},
		{"ClickHouse validates", DialectClickHouse, false},
		{"Unspecified returns error", DialectUnspecified, true},
		{"Unknown dialect returns error", Dialect(999), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.dialect.Validate()
			if tt.expectError && err == nil {
				t.Errorf("Dialect.Validate() expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Dialect.Validate() unexpected error: %v", err)
			}
		})
	}
}

func TestNeedsQuoting(t *testing.T) {
	tests := []struct {
		segment  string
		expected bool
	}{
		{"name", false},
		{"user_name", false},
		{"_private", false},
		{"tx", false},
		{"24h", true},
		{"7d", true},
		{"10m", true},
		{"120d", true},
		{"\uff124h", true},
		{"名前", true},
		{"col-name", true},
		{"has space", true},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.segment, func(t *testing.T) {
			if got := NeedsQuoting(tt.segment); got != tt.expected {
				t.Errorf("NeedsQuoting(%q) = %v, want %v", tt.segment, got, tt.expected)
			}
		})
	}
}

func TestIsSafeIdentifierSegment(t *testing.T) {
	tests := []struct {
		segment  string
		expected bool
	}{
		{"name", true},
		{"user_name", true},
		{"名前", true},
		{"\uff124h", true},
		{"", false},
		{"col-name", false},
		{"has space", false},
		{"has`tick", false},
		{"has'quote", false},
	}

	for _, tt := range tests {
		t.Run(tt.segment, func(t *testing.T) {
			if got := IsSafeIdentifierSegment(tt.segment); got != tt.expected {
				t.Errorf("IsSafeIdentifierSegment(%q) = %v, want %v", tt.segment, got, tt.expected)
			}
		})
	}
}

func TestQuoteIdentifierSegment(t *testing.T) {
	tests := []struct {
		name     string
		segment  string
		dialect  Dialect
		expected string
	}{
		{"BigQuery backtick", "24h", DialectBigQuery, "`24h`"},
		{"Spanner backtick", "24h", DialectSpanner, "`24h`"},
		{"ClickHouse backtick", "7d", DialectClickHouse, "`7d`"},
		{"PostgreSQL double quote", "24h", DialectPostgreSQL, `"24h"`},
		{"DuckDB double quote", "10m", DialectDuckDB, `"10m"`},
		{"BigQuery escapes embedded backtick", "ab`cd", DialectBigQuery, "`ab``cd`"},
		{"PostgreSQL escapes embedded double quote", `ab"cd`, DialectPostgreSQL, `"ab""cd"`},
		{"Unspecified uses backtick", "24h", DialectUnspecified, "`24h`"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := QuoteIdentifierSegment(tt.segment, tt.dialect); got != tt.expected {
				t.Errorf("QuoteIdentifierSegment(%q, %v) = %q, want %q", tt.segment, tt.dialect, got, tt.expected)
			}
		})
	}
}

func TestContainsQuoteCharacters(t *testing.T) {
	tests := []struct {
		segment  string
		expected bool
	}{
		{"normal", false},
		{"24h", false},
		{"_field", false},
		{"", false},
		{"`quoted`", true},
		{`"quoted"`, true},
		{"has`tick", true},
		{`has"quote`, true},
		{"'quoted'", true},
		{"has'single", true},
	}

	for _, tt := range tests {
		t.Run(tt.segment, func(t *testing.T) {
			if got := ContainsQuoteCharacters(tt.segment); got != tt.expected {
				t.Errorf("ContainsQuoteCharacters(%q) = %v, want %v", tt.segment, got, tt.expected)
			}
		})
	}
}
