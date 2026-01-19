package operators

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

func TestNewOperatorConfig(t *testing.T) {
	tests := []struct {
		name        string
		dialect     dialect.Dialect
		schema      SchemaProvider
		wantDialect dialect.Dialect
		wantSchema  bool
	}{
		{
			name:        "BigQuery without schema",
			dialect:     dialect.DialectBigQuery,
			schema:      nil,
			wantDialect: dialect.DialectBigQuery,
			wantSchema:  false,
		},
		{
			name:        "Spanner without schema",
			dialect:     dialect.DialectSpanner,
			schema:      nil,
			wantDialect: dialect.DialectSpanner,
			wantSchema:  false,
		},
		{
			name:        "PostgreSQL without schema",
			dialect:     dialect.DialectPostgreSQL,
			schema:      nil,
			wantDialect: dialect.DialectPostgreSQL,
			wantSchema:  false,
		},
		{
			name:        "DuckDB without schema",
			dialect:     dialect.DialectDuckDB,
			schema:      nil,
			wantDialect: dialect.DialectDuckDB,
			wantSchema:  false,
		},
		{
			name:        "ClickHouse without schema",
			dialect:     dialect.DialectClickHouse,
			schema:      nil,
			wantDialect: dialect.DialectClickHouse,
			wantSchema:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewOperatorConfig(tt.dialect, tt.schema)
			if config.Dialect != tt.wantDialect {
				t.Errorf("Dialect = %v, want %v", config.Dialect, tt.wantDialect)
			}
			if config.HasSchema() != tt.wantSchema {
				t.Errorf("HasSchema() = %v, want %v", config.HasSchema(), tt.wantSchema)
			}
		})
	}
}

func TestOperatorConfig_HasSchema(t *testing.T) {
	tests := []struct {
		name   string
		config *OperatorConfig
		want   bool
	}{
		{
			name:   "nil config",
			config: nil,
			want:   false,
		},
		{
			name:   "config with nil schema",
			config: &OperatorConfig{Dialect: dialect.DialectBigQuery, Schema: nil},
			want:   false,
		},
		{
			name:   "config with schema",
			config: &OperatorConfig{Dialect: dialect.DialectBigQuery, Schema: &mockSchemaProvider{}},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.HasSchema(); got != tt.want {
				t.Errorf("HasSchema() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOperatorConfig_GetDialect(t *testing.T) {
	tests := []struct {
		name   string
		config *OperatorConfig
		want   dialect.Dialect
	}{
		{
			name:   "nil config",
			config: nil,
			want:   dialect.DialectUnspecified,
		},
		{
			name:   "BigQuery dialect",
			config: &OperatorConfig{Dialect: dialect.DialectBigQuery},
			want:   dialect.DialectBigQuery,
		},
		{
			name:   "Spanner dialect",
			config: &OperatorConfig{Dialect: dialect.DialectSpanner},
			want:   dialect.DialectSpanner,
		},
		{
			name:   "PostgreSQL dialect",
			config: &OperatorConfig{Dialect: dialect.DialectPostgreSQL},
			want:   dialect.DialectPostgreSQL,
		},
		{
			name:   "DuckDB dialect",
			config: &OperatorConfig{Dialect: dialect.DialectDuckDB},
			want:   dialect.DialectDuckDB,
		},
		{
			name:   "ClickHouse dialect",
			config: &OperatorConfig{Dialect: dialect.DialectClickHouse},
			want:   dialect.DialectClickHouse,
		},
		{
			name:   "Unspecified dialect",
			config: &OperatorConfig{Dialect: dialect.DialectUnspecified},
			want:   dialect.DialectUnspecified,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.GetDialect(); got != tt.want {
				t.Errorf("GetDialect() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOperatorConfig_ValidateDialect(t *testing.T) {
	tests := []struct {
		name      string
		config    *OperatorConfig
		operator  string
		wantError bool
	}{
		{
			name:      "BigQuery is valid",
			config:    &OperatorConfig{Dialect: dialect.DialectBigQuery},
			operator:  "test",
			wantError: false,
		},
		{
			name:      "Spanner is valid",
			config:    &OperatorConfig{Dialect: dialect.DialectSpanner},
			operator:  "test",
			wantError: false,
		},
		{
			name:      "PostgreSQL is valid",
			config:    &OperatorConfig{Dialect: dialect.DialectPostgreSQL},
			operator:  "test",
			wantError: false,
		},
		{
			name:      "DuckDB is valid",
			config:    &OperatorConfig{Dialect: dialect.DialectDuckDB},
			operator:  "test",
			wantError: false,
		},
		{
			name:      "ClickHouse is valid",
			config:    &OperatorConfig{Dialect: dialect.DialectClickHouse},
			operator:  "test",
			wantError: false,
		},
		{
			name:      "Unspecified dialect returns error",
			config:    &OperatorConfig{Dialect: dialect.DialectUnspecified},
			operator:  "merge",
			wantError: true,
		},
		{
			name:      "nil config returns unspecified error",
			config:    nil,
			operator:  "test",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.ValidateDialect(tt.operator)
			if (err != nil) != tt.wantError {
				t.Errorf("ValidateDialect() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestOperatorConfig_IsBigQuery(t *testing.T) {
	tests := []struct {
		name   string
		config *OperatorConfig
		want   bool
	}{
		{
			name:   "is BigQuery",
			config: &OperatorConfig{Dialect: dialect.DialectBigQuery},
			want:   true,
		},
		{
			name:   "is not BigQuery - Spanner",
			config: &OperatorConfig{Dialect: dialect.DialectSpanner},
			want:   false,
		},
		{
			name:   "is not BigQuery - PostgreSQL",
			config: &OperatorConfig{Dialect: dialect.DialectPostgreSQL},
			want:   false,
		},
		{
			name:   "nil config",
			config: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsBigQuery(); got != tt.want {
				t.Errorf("IsBigQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOperatorConfig_IsSpanner(t *testing.T) {
	tests := []struct {
		name   string
		config *OperatorConfig
		want   bool
	}{
		{
			name:   "is Spanner",
			config: &OperatorConfig{Dialect: dialect.DialectSpanner},
			want:   true,
		},
		{
			name:   "is not Spanner - BigQuery",
			config: &OperatorConfig{Dialect: dialect.DialectBigQuery},
			want:   false,
		},
		{
			name:   "nil config",
			config: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsSpanner(); got != tt.want {
				t.Errorf("IsSpanner() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOperatorConfig_IsPostgreSQL(t *testing.T) {
	tests := []struct {
		name   string
		config *OperatorConfig
		want   bool
	}{
		{
			name:   "is PostgreSQL",
			config: &OperatorConfig{Dialect: dialect.DialectPostgreSQL},
			want:   true,
		},
		{
			name:   "is not PostgreSQL - BigQuery",
			config: &OperatorConfig{Dialect: dialect.DialectBigQuery},
			want:   false,
		},
		{
			name:   "nil config",
			config: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsPostgreSQL(); got != tt.want {
				t.Errorf("IsPostgreSQL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOperatorConfig_IsDuckDB(t *testing.T) {
	tests := []struct {
		name   string
		config *OperatorConfig
		want   bool
	}{
		{
			name:   "is DuckDB",
			config: &OperatorConfig{Dialect: dialect.DialectDuckDB},
			want:   true,
		},
		{
			name:   "is not DuckDB - BigQuery",
			config: &OperatorConfig{Dialect: dialect.DialectBigQuery},
			want:   false,
		},
		{
			name:   "nil config",
			config: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsDuckDB(); got != tt.want {
				t.Errorf("IsDuckDB() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOperatorConfig_IsClickHouse(t *testing.T) {
	tests := []struct {
		name   string
		config *OperatorConfig
		want   bool
	}{
		{
			name:   "is ClickHouse",
			config: &OperatorConfig{Dialect: dialect.DialectClickHouse},
			want:   true,
		},
		{
			name:   "is not ClickHouse - BigQuery",
			config: &OperatorConfig{Dialect: dialect.DialectBigQuery},
			want:   false,
		},
		{
			name:   "nil config",
			config: nil,
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsClickHouse(); got != tt.want {
				t.Errorf("IsClickHouse() = %v, want %v", got, tt.want)
			}
		})
	}
}

// mockSchemaProvider implements SchemaProvider for testing.
type mockSchemaProvider struct{}

func (m *mockSchemaProvider) GetFieldType(fieldName string) string {
	return "string"
}

func (m *mockSchemaProvider) HasField(fieldName string) bool {
	return true
}

func (m *mockSchemaProvider) ValidateField(fieldName string) error {
	return nil
}

func (m *mockSchemaProvider) GetAllowedValues(fieldName string) []string {
	return nil
}

func (m *mockSchemaProvider) IsNumericType(fieldName string) bool {
	return false
}

func (m *mockSchemaProvider) IsStringType(fieldName string) bool {
	return true
}

func (m *mockSchemaProvider) IsArrayType(fieldName string) bool {
	return false
}

func (m *mockSchemaProvider) IsBooleanType(fieldName string) bool {
	return false
}

func (m *mockSchemaProvider) IsEnumType(fieldName string) bool {
	return false
}

func (m *mockSchemaProvider) ValidateEnumValue(fieldName, value string) error {
	return nil
}
