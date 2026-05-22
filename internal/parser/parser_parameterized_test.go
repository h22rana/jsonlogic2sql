package parser

import (
	"fmt"
	"strings"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestParser_ParseParameterized(t *testing.T) {
	p := NewParser(operators.NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlyParserSchema{}))

	tests := []struct {
		name       string
		input      interface{}
		wantSQL    string
		wantParams []params.QueryParam
	}{
		{
			name: "simple equality",
			input: map[string]interface{}{
				"==": []interface{}{map[string]interface{}{"var": "email"}, "alice"},
			},
			wantSQL:    "email = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "alice"}},
		},
		{
			name: "greater than",
			input: map[string]interface{}{
				">": []interface{}{map[string]interface{}{"var": "age"}, float64(18)},
			},
			wantSQL:    "age > @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: float64(18)}},
		},
		{
			name: "in array",
			input: map[string]interface{}{
				"in": []interface{}{
					map[string]interface{}{"var": "x"},
					[]interface{}{float64(1), float64(2)},
				},
			},
			wantSQL: "x IN (@p1, @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
			},
		},
		{
			name: "and with two conditions",
			input: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{
						"==": []interface{}{map[string]interface{}{"var": "status"}, "active"},
					},
					map[string]interface{}{
						">": []interface{}{map[string]interface{}{"var": "age"}, float64(18)},
					},
				},
			},
			wantSQL: "(status = @p1 AND age > @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "active"},
				{Name: "p2", Value: float64(18)},
			},
		},
		{
			name: "null comparison",
			input: map[string]interface{}{
				"==": []interface{}{map[string]interface{}{"var": "field"}, nil},
			},
			wantSQL:    "field IS NULL",
			wantParams: nil,
		},
		{
			name: "boolean comparison",
			input: map[string]interface{}{
				"==": []interface{}{map[string]interface{}{"var": "active"}, true},
			},
			wantSQL:    "active = TRUE",
			wantParams: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSQL, gotParams, err := p.ParseParameterized(tt.input)
			if err != nil {
				t.Fatalf("ParseParameterized: unexpected error: %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("SQL: got %q, want %q", gotSQL, tt.wantSQL)
			}
			assertQueryParams(t, gotParams, tt.wantParams)
		})
	}
}

func TestParser_ParseParameterized_PostgreSQL(t *testing.T) {
	p := NewParser(operators.NewOperatorConfig(dialect.DialectPostgreSQL, &fieldOnlyParserSchema{}))

	tests := []struct {
		name       string
		input      interface{}
		wantSQL    string
		wantParams []params.QueryParam
	}{
		{
			name: "simple equality",
			input: map[string]interface{}{
				"==": []interface{}{map[string]interface{}{"var": "email"}, "alice"},
			},
			wantSQL:    "email = $1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "alice"}},
		},
		{
			name: "greater than",
			input: map[string]interface{}{
				">": []interface{}{map[string]interface{}{"var": "age"}, float64(18)},
			},
			wantSQL:    "age > $1",
			wantParams: []params.QueryParam{{Name: "p1", Value: float64(18)}},
		},
		{
			name: "in array",
			input: map[string]interface{}{
				"in": []interface{}{
					map[string]interface{}{"var": "x"},
					[]interface{}{float64(1), float64(2)},
				},
			},
			wantSQL: "x IN ($1, $2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
			},
		},
		{
			name: "and with two conditions",
			input: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{
						"==": []interface{}{map[string]interface{}{"var": "status"}, "active"},
					},
					map[string]interface{}{
						">": []interface{}{map[string]interface{}{"var": "age"}, float64(18)},
					},
				},
			},
			wantSQL: "(status = $1 AND age > $2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "active"},
				{Name: "p2", Value: float64(18)},
			},
		},
		{
			name: "null comparison",
			input: map[string]interface{}{
				"==": []interface{}{map[string]interface{}{"var": "field"}, nil},
			},
			wantSQL:    "field IS NULL",
			wantParams: nil,
		},
		{
			name: "boolean comparison",
			input: map[string]interface{}{
				"==": []interface{}{map[string]interface{}{"var": "active"}, true},
			},
			wantSQL:    "active = TRUE",
			wantParams: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSQL, gotParams, err := p.ParseParameterized(tt.input)
			if err != nil {
				t.Fatalf("ParseParameterized: unexpected error: %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("SQL: got %q, want %q", gotSQL, tt.wantSQL)
			}
			assertQueryParams(t, gotParams, tt.wantParams)
		})
	}
}

func TestParser_ParseConditionParameterized(t *testing.T) {
	p := NewParser(operators.NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlyParserSchema{}))

	input := map[string]interface{}{
		"==": []interface{}{map[string]interface{}{"var": "x"}, "val"},
	}
	gotSQL, gotParams, err := p.ParseConditionParameterized(input)
	if err != nil {
		t.Fatalf("ParseConditionParameterized: unexpected error: %v", err)
	}
	if strings.HasPrefix(gotSQL, "WHERE ") {
		t.Errorf("condition must not have WHERE prefix, got %q", gotSQL)
	}
	wantSQL := "x = @p1"
	if gotSQL != wantSQL {
		t.Errorf("SQL: got %q, want %q", gotSQL, wantSQL)
	}
	assertQueryParams(t, gotParams, []params.QueryParam{{Name: "p1", Value: "val"}})
}

func TestParser_ParseParameterized_Errors(t *testing.T) {
	p := NewParser(operators.NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlyParserSchema{}))

	t.Run("nil logic", func(t *testing.T) {
		_, _, err := p.ParseParameterized(nil)
		if err == nil {
			t.Fatal("expected error for nil logic")
		}
	})

	t.Run("empty map", func(t *testing.T) {
		_, _, err := p.ParseParameterized(map[string]interface{}{})
		if err == nil {
			t.Fatal("expected error for empty operator map")
		}
	})

	t.Run("top level primitive", func(t *testing.T) {
		_, _, err := p.ParseParameterized(float64(42))
		if err == nil {
			t.Fatal("expected error for primitive root expression")
		}
	})

	t.Run("invalid expression type", func(t *testing.T) {
		_, _, err := p.ParseParameterized(struct{}{})
		if err == nil {
			t.Fatal("expected error for unsupported root type")
		}
	})
}

func TestParser_ParseParameterized_CustomOperator(t *testing.T) {
	p := NewParser(operators.NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlyParserSchema{}))

	p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
		if operatorName == "twice" {
			return &mockCustomHandler{
				toSQL: func(op string, args []interface{}) (string, error) {
					return fmt.Sprintf("(%s + %s)", args[0], args[0]), nil
				},
			}, true
		}
		return nil, false
	})

	input := map[string]interface{}{
		"==": []interface{}{
			map[string]interface{}{"twice": []interface{}{map[string]interface{}{"var": "n"}}},
			float64(2),
		},
	}
	gotSQL, gotParams, err := p.ParseParameterized(input)
	if err != nil {
		t.Fatalf("ParseParameterized: unexpected error: %v", err)
	}
	wantSQL := "(n + n) = @p1"
	if gotSQL != wantSQL {
		t.Errorf("SQL: got %q, want %q", gotSQL, wantSQL)
	}
	if !strings.Contains(gotSQL, "@p1") {
		t.Errorf("expected placeholder @p1 in SQL: %q", gotSQL)
	}
	assertQueryParams(t, gotParams, []params.QueryParam{{Name: "p1", Value: float64(2)}})
}

// TestParser_primitiveToSQLParam exercises primitiveToSQLParam indirectly: strings and
// numbers become bind params; bool and nil stay inline in SQL.
func TestParser_primitiveToSQLParam(t *testing.T) {
	p := NewParser(operators.NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlyParserSchema{}))

	t.Run("string literal via equality", func(t *testing.T) {
		_, gotParams, err := p.ParseParameterized(map[string]interface{}{
			"==": []interface{}{map[string]interface{}{"var": "k"}, "v"},
		})
		if err != nil {
			t.Fatal(err)
		}
		assertQueryParams(t, gotParams, []params.QueryParam{{Name: "p1", Value: "v"}})
	})

	t.Run("integer literal via equality", func(t *testing.T) {
		_, gotParams, err := p.ParseParameterized(map[string]interface{}{
			"==": []interface{}{map[string]interface{}{"var": "k"}, int64(99)},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(gotParams) != 1 || gotParams[0].Name != "p1" {
			t.Fatalf("params: %+v", gotParams)
		}
		switch v := gotParams[0].Value.(type) {
		case int64:
			if v != 99 {
				t.Errorf("Value: got %d, want 99", v)
			}
		default:
			t.Errorf("Value type: got %T, want int64", gotParams[0].Value)
		}
	})

	t.Run("float literal via greater than", func(t *testing.T) {
		_, gotParams, err := p.ParseParameterized(map[string]interface{}{
			">": []interface{}{map[string]interface{}{"var": "k"}, float64(0.5)},
		})
		if err != nil {
			t.Fatal(err)
		}
		assertQueryParams(t, gotParams, []params.QueryParam{{Name: "p1", Value: float64(0.5)}})
	})
}
