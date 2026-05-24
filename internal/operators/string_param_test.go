package operators

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestStringOperator_ToSQLParam(t *testing.T) {
	t.Run("cat with strings", func(t *testing.T) {
		op := NewStringOperator(testFieldOnlyConfig())
		pc := params.NewParamCollector(params.PlaceholderNamed)
		sql, err := op.ToSQLParam("cat", []interface{}{"hello", " ", "world"}, pc)
		if err != nil {
			t.Fatalf("ToSQLParam: %v", err)
		}
		wantSQL := "CONCAT(@p1, @p2, @p3)"
		if sql != wantSQL {
			t.Errorf("SQL = %q, want %q", sql, wantSQL)
		}
		wantParams := []params.QueryParam{
			{Name: "p1", Value: "hello"},
			{Name: "p2", Value: " "},
			{Name: "p3", Value: "world"},
		}
		if !reflect.DeepEqual(pc.Params(), wantParams) {
			t.Errorf("Params = %#v, want %#v", pc.Params(), wantParams)
		}
	})

	t.Run("cat with var and string", func(t *testing.T) {
		config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
		config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
			if m, ok := expr.(map[string]interface{}); ok {
				if v, ok := m["var"]; ok {
					return fmt.Sprintf("%v", v), nil
				}
			}
			return "", fmt.Errorf("unsupported")
		})
		op := NewStringOperator(config)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		sql, err := op.ToSQLParam("cat", []interface{}{
			map[string]interface{}{"var": "name"},
			"!",
		}, pc)
		if err != nil {
			t.Fatalf("ToSQLParam: %v", err)
		}
		wantSQL := "CONCAT(COALESCE(CAST(name AS STRING), ''), @p1)"
		if sql != wantSQL {
			t.Errorf("SQL = %q, want %q", sql, wantSQL)
		}
		wantParams := []params.QueryParam{{Name: "p1", Value: "!"}}
		if !reflect.DeepEqual(pc.Params(), wantParams) {
			t.Errorf("Params = %#v, want %#v", pc.Params(), wantParams)
		}
	})

	t.Run("cat with null", func(t *testing.T) {
		op := NewStringOperator(testFieldOnlyConfig())
		pc := params.NewParamCollector(params.PlaceholderNamed)
		sql, err := op.ToSQLParam("cat", []interface{}{nil, "world"}, pc)
		if err != nil {
			t.Fatalf("ToSQLParam: %v", err)
		}
		wantSQL := "CONCAT('', @p1)"
		if sql != wantSQL {
			t.Errorf("SQL = %q, want %q", sql, wantSQL)
		}
		wantParams := []params.QueryParam{{Name: "p1", Value: "world"}}
		if !reflect.DeepEqual(pc.Params(), wantParams) {
			t.Errorf("Params = %#v, want %#v", pc.Params(), wantParams)
		}
	})

	t.Run("cat preserves comparison if branches in parameterized mode", func(t *testing.T) {
		op := NewStringOperator(testFieldOnlyConfig())
		pc := params.NewParamCollector(params.PlaceholderNamed)
		sql, err := op.ToSQLParam("cat", []interface{}{
			map[string]interface{}{
				"==": []interface{}{
					map[string]interface{}{
						"if": []interface{}{
							map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "x"}, 0}},
							"a",
							map[string]interface{}{"<": []interface{}{map[string]interface{}{"var": "y"}, 0}},
							"b",
						},
					},
					"b",
				},
			},
		}, pc)
		if err != nil {
			t.Fatalf("ToSQLParam: %v", err)
		}
		wantSQL := "CONCAT(CASE WHEN (CASE WHEN x > @p1 THEN @p2 WHEN y < @p3 THEN @p4 END = @p5) THEN 'true' ELSE 'false' END)"
		if sql != wantSQL {
			t.Errorf("SQL = %q, want %q", sql, wantSQL)
		}
		wantParams := []params.QueryParam{
			{Name: "p1", Value: 0},
			{Name: "p2", Value: "a"},
			{Name: "p3", Value: 0},
			{Name: "p4", Value: "b"},
			{Name: "p5", Value: "b"},
		}
		if !reflect.DeepEqual(pc.Params(), wantParams) {
			t.Errorf("Params = %#v, want %#v", pc.Params(), wantParams)
		}
	})

	t.Run("cat stringifies explicit if else branch in parameterized mode", func(t *testing.T) {
		op := NewStringOperator(testFieldOnlyConfig())
		pc := params.NewParamCollector(params.PlaceholderNamed)
		sql, err := op.ToSQLParam("cat", []interface{}{
			map[string]interface{}{
				"if": []interface{}{
					map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "x"}, 1}},
					true,
					"fallback",
				},
			},
		}, pc)
		if err != nil {
			t.Fatalf("ToSQLParam: %v", err)
		}
		wantSQL := "CONCAT(CASE WHEN (x = @p1) THEN 'true' ELSE @p2 END)"
		if sql != wantSQL {
			t.Errorf("SQL = %q, want %q", sql, wantSQL)
		}
		wantParams := []params.QueryParam{
			{Name: "p1", Value: 1},
			{Name: "p2", Value: "fallback"},
		}
		if !reflect.DeepEqual(pc.Params(), wantParams) {
			t.Errorf("Params = %#v, want %#v", pc.Params(), wantParams)
		}
	})

	t.Run("substr with string and indices", func(t *testing.T) {
		config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
		op := NewStringOperator(config)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		sql, err := op.ToSQLParam("substr", []interface{}{"hello", float64(0), float64(3)}, pc)
		if err != nil {
			t.Fatalf("ToSQLParam: %v", err)
		}
		wantSQL := "SUBSTR(@p1, (@p2 + 1), @p3)"
		if sql != wantSQL {
			t.Errorf("SQL = %q, want %q", sql, wantSQL)
		}
		wantParams := []params.QueryParam{
			{Name: "p1", Value: "hello"},
			{Name: "p2", Value: float64(0)},
			{Name: "p3", Value: float64(3)},
		}
		if !reflect.DeepEqual(pc.Params(), wantParams) {
			t.Errorf("Params = %#v, want %#v", pc.Params(), wantParams)
		}
	})

	t.Run("unsupported string operator", func(t *testing.T) {
		op := NewStringOperator(testFieldOnlyConfig())
		pc := params.NewParamCollector(params.PlaceholderNamed)
		_, err := op.ToSQLParam("unsupported", []interface{}{"x"}, pc)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestStringOperator_valueToSQLParam(t *testing.T) {
	op := NewStringOperator(testFieldOnlyConfig())

	tests := []struct {
		name       string
		value      interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:    "string literal",
			value:   "test",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "test"},
			},
		},
		{
			name:    "number literal",
			value:   42,
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 42},
			},
		},
		{
			name:       "boolean true",
			value:      true,
			wantSQL:    "TRUE",
			wantParams: nil,
		},
		{
			name:       "nil",
			value:      nil,
			wantSQL:    "NULL",
			wantParams: nil,
		},
		{
			name:       "ProcessedValue SQL",
			value:      SQLResult("some_col"),
			wantSQL:    "some_col",
			wantParams: nil,
		},
		{
			name:    "ProcessedValue literal",
			value:   LiteralResult("lit"),
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "lit"},
			},
		},
		{
			name: "comparison coerces to numeric inside parameterized arithmetic string context",
			value: map[string]interface{}{
				"+": []interface{}{
					map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "x"}, 1}},
					1,
				},
			},
			wantSQL: "((CASE WHEN x = @p1 THEN 1 ELSE 0 END) + @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 1},
				{Name: "p2", Value: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.valueToSQLParam(tt.value, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("valueToSQLParam: %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", got, tt.wantSQL)
			}
			if !reflect.DeepEqual(pc.Params(), tt.wantParams) {
				t.Errorf("Params = %#v, want %#v", pc.Params(), tt.wantParams)
			}
		})
	}
}

func TestStringOperator_ToSQLParam_Dialects(t *testing.T) {
	args := []interface{}{"hello", float64(0), float64(3)}
	wantParams := []params.QueryParam{
		{Name: "p1", Value: "hello"},
		{Name: "p2", Value: float64(0)},
		{Name: "p3", Value: float64(3)},
	}

	tests := []struct {
		dialect dialect.Dialect
		wantSQL string
	}{
		{dialect.DialectBigQuery, "SUBSTR(@p1, (@p2 + 1), @p3)"},
		// handleSubstringParam uses SUBSTR for PostgreSQL (same as BigQuery); ClickHouse uses lowercase substring.
		{dialect.DialectPostgreSQL, "SUBSTR($1, ($2 + 1), $3)"},
		{dialect.DialectClickHouse, "substring({p1:String}, ({p2:Float64} + 1), {p3:Float64})"},
	}

	for _, tt := range tests {
		t.Run(tt.dialect.String(), func(t *testing.T) {
			config := NewOperatorConfig(tt.dialect, &fieldOnlySchemaProvider{})
			op := NewStringOperator(config)
			pc := params.NewParamCollector(params.StyleForDialect(tt.dialect))
			sql, err := op.ToSQLParam("substr", args, pc)
			if err != nil {
				t.Fatalf("ToSQLParam: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", sql, tt.wantSQL)
			}
			if !reflect.DeepEqual(pc.Params(), wantParams) {
				t.Errorf("Params = %#v, want %#v", pc.Params(), wantParams)
			}
		})
	}
}
