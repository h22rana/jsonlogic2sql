package operators

import (
	"fmt"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestNewOperatorConfig(t *testing.T) {
	tests := []struct {
		name        string
		dialect     dialect.Dialect
		schema      SchemaProvider
		wantDialect dialect.Dialect
	}{
		{
			name:        "BigQuery with nil provider uses empty schema",
			dialect:     dialect.DialectBigQuery,
			schema:      nil,
			wantDialect: dialect.DialectBigQuery,
		},
		{
			name:        "Spanner with nil provider uses empty schema",
			dialect:     dialect.DialectSpanner,
			schema:      nil,
			wantDialect: dialect.DialectSpanner,
		},
		{
			name:        "PostgreSQL with nil provider uses empty schema",
			dialect:     dialect.DialectPostgreSQL,
			schema:      nil,
			wantDialect: dialect.DialectPostgreSQL,
		},
		{
			name:        "DuckDB with nil provider uses empty schema",
			dialect:     dialect.DialectDuckDB,
			schema:      nil,
			wantDialect: dialect.DialectDuckDB,
		},
		{
			name:        "ClickHouse with nil provider uses empty schema",
			dialect:     dialect.DialectClickHouse,
			schema:      nil,
			wantDialect: dialect.DialectClickHouse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewOperatorConfig(tt.dialect, tt.schema)
			if config.Dialect != tt.wantDialect {
				t.Errorf("Dialect = %v, want %v", config.Dialect, tt.wantDialect)
			}
			if config.Schema == nil {
				t.Fatal("Schema should be normalized to an empty provider")
			}
			if err := config.Schema.ValidateField("missing"); err == nil {
				t.Fatal("empty schema provider should reject field access")
			}
		})
	}
}

type fieldOnlySchemaProvider struct{}

func (m *fieldOnlySchemaProvider) GetFieldType(_ string) string { return "" }

func (m *fieldOnlySchemaProvider) HasField(_ string) bool { return true }

func (m *fieldOnlySchemaProvider) ValidateField(_ string) error { return nil }

func (m *fieldOnlySchemaProvider) GetAllowedValues(_ string) []string { return nil }

func (m *fieldOnlySchemaProvider) IsNumericType(_ string) bool { return false }

func (m *fieldOnlySchemaProvider) IsStringType(_ string) bool { return false }

func (m *fieldOnlySchemaProvider) IsArrayType(_ string) bool { return false }

func (m *fieldOnlySchemaProvider) IsBooleanType(_ string) bool { return false }

func (m *fieldOnlySchemaProvider) IsEnumType(_ string) bool { return false }

func (m *fieldOnlySchemaProvider) ValidateEnumValue(_, _ string) error { return nil }

func testFieldOnlyConfig() *OperatorConfig {
	return NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
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

func TestOperatorConfig_SetExpressionParser(t *testing.T) {
	// Test setting a parser on a valid config
	config := NewOperatorConfig(dialect.DialectBigQuery, nil)
	if config.HasExpressionParser() {
		t.Errorf("HasExpressionParser() should be false before setting")
	}

	parser := func(expr any, path string) (string, error) {
		return "PARSED", nil
	}
	config.SetExpressionParser(parser)

	if !config.HasExpressionParser() {
		t.Errorf("HasExpressionParser() should be true after setting")
	}

	// Test the parser works
	result, err := config.ParseExpression(nil, "$")
	if err != nil {
		t.Errorf("ParseExpression() unexpected error = %v", err)
	}
	if result != "PARSED" {
		t.Errorf("ParseExpression() = %v, want PARSED", result)
	}

	// Test setting on nil config (should not panic)
	var nilConfig *OperatorConfig
	nilConfig.SetExpressionParser(parser) // Should be a no-op
}

func TestOperatorConfig_HasExpressionParser(t *testing.T) {
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
			name:   "config without parser",
			config: &OperatorConfig{Dialect: dialect.DialectBigQuery},
			want:   false,
		},
		{
			name: "config with parser",
			config: &OperatorConfig{
				Dialect: dialect.DialectBigQuery,
				ExpressionParser: func(expr any, path string) (string, error) {
					return "", nil
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.HasExpressionParser(); got != tt.want {
				t.Errorf("HasExpressionParser() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOperatorConfig_ParseExpression(t *testing.T) {
	// Test with no parser configured
	config := NewOperatorConfig(dialect.DialectBigQuery, nil)
	_, err := config.ParseExpression(nil, "$")
	if err == nil {
		t.Errorf("ParseExpression() expected error when no parser configured, got nil")
	}

	// Test with nil config
	var nilConfig *OperatorConfig
	_, err = nilConfig.ParseExpression(nil, "$")
	if err == nil {
		t.Errorf("ParseExpression() expected error for nil config, got nil")
	}

	// Test with parser that returns error
	config.SetExpressionParser(func(expr any, path string) (string, error) {
		return "", fmt.Errorf("parse error")
	})
	_, err = config.ParseExpression(nil, "$")
	if err == nil {
		t.Errorf("ParseExpression() expected error from parser, got nil")
	}

	// Test with parser that succeeds
	config.SetExpressionParser(func(expr any, path string) (string, error) {
		return "RESULT_SQL", nil
	})
	result, err := config.ParseExpression(map[string]interface{}{"custom": "op"}, "$.custom")
	if err != nil {
		t.Errorf("ParseExpression() unexpected error = %v", err)
	}
	if result != "RESULT_SQL" {
		t.Errorf("ParseExpression() = %v, want RESULT_SQL", result)
	}
}

func TestOperatorConfig_SetParamExpressionParser(t *testing.T) {
	// Test setting a parser on a valid config
	config := NewOperatorConfig(dialect.DialectBigQuery, nil)
	if config.HasParamExpressionParser() {
		t.Errorf("HasParamExpressionParser() should be false before setting")
	}

	parser := func(expr any, path string, pc *params.ParamCollector) (string, error) {
		return "PARSED", nil
	}
	config.SetParamExpressionParser(parser)

	if !config.HasParamExpressionParser() {
		t.Errorf("HasParamExpressionParser() should be true after setting")
	}

	pc := params.NewParamCollector(params.PlaceholderNamed)
	// Test the parser works
	result, err := config.ParseExpressionParam(nil, "$", pc)
	if err != nil {
		t.Errorf("ParseExpressionParam() unexpected error = %v", err)
	}
	if result != "PARSED" {
		t.Errorf("ParseExpressionParam() = %v, want PARSED", result)
	}

	// Test setting on nil config (should not panic)
	var nilConfig *OperatorConfig
	nilConfig.SetParamExpressionParser(parser) // Should be a no-op
}

func TestOperatorConfig_HasParamExpressionParser(t *testing.T) {
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
			name:   "config without parser",
			config: &OperatorConfig{Dialect: dialect.DialectBigQuery},
			want:   false,
		},
		{
			name: "config with parser",
			config: &OperatorConfig{
				Dialect: dialect.DialectBigQuery,
				ParamExpressionParser: func(expr any, path string, pc *params.ParamCollector) (string, error) {
					return "", nil
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.HasParamExpressionParser(); got != tt.want {
				t.Errorf("HasParamExpressionParser() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOperatorConfig_ParseExpressionParam(t *testing.T) {
	pc := params.NewParamCollector(params.PlaceholderNamed)

	// Test with no parser configured
	config := NewOperatorConfig(dialect.DialectBigQuery, nil)
	_, err := config.ParseExpressionParam(nil, "$", pc)
	if err == nil {
		t.Errorf("ParseExpressionParam() expected error when no parser configured, got nil")
	}

	// Test with nil config
	var nilConfig *OperatorConfig
	_, err = nilConfig.ParseExpressionParam(nil, "$", pc)
	if err == nil {
		t.Errorf("ParseExpressionParam() expected error for nil config, got nil")
	}

	// Test with parser that returns error
	config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
		return "", fmt.Errorf("parse error")
	})
	_, err = config.ParseExpressionParam(nil, "$", pc)
	if err == nil {
		t.Errorf("ParseExpressionParam() expected error from parser, got nil")
	}

	// Test with parser that succeeds
	config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
		return "RESULT_SQL", nil
	})
	result, err := config.ParseExpressionParam(map[string]interface{}{"custom": "op"}, "$.custom", pc)
	if err != nil {
		t.Errorf("ParseExpressionParam() unexpected error = %v", err)
	}
	if result != "RESULT_SQL" {
		t.Errorf("ParseExpressionParam() = %v, want RESULT_SQL", result)
	}
}
