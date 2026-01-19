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
