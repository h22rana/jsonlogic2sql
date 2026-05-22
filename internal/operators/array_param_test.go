package operators

import (
	"fmt"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestArrayOperator_ToSQLParam_Merge(t *testing.T) {
	op := newArrayOperatorWithParamParserBQ(t)
	args := []interface{}{
		[]interface{}{1, 2},
		[]interface{}{3, 4},
	}
	pc := params.NewParamCollector(params.PlaceholderNamed)

	got, err := op.ToSQLParam("merge", args, pc)
	if err != nil {
		t.Fatalf("ToSQLParam(merge): %v", err)
	}
	want := "ARRAY_CONCAT([@p1, @p2], [@p3, @p4])"
	if got != want {
		t.Errorf("SQL = %q, want %q", got, want)
	}
	assertCollectedParams(t, pc, []params.QueryParam{
		{Name: "p1", Value: 1},
		{Name: "p2", Value: 2},
		{Name: "p3", Value: 3},
		{Name: "p4", Value: 4},
	})
}

func TestArrayOperator_valueToSQLParam(t *testing.T) {
	op := newArrayOperatorWithParamParserBQ(t)

	tests := []struct {
		name        string
		input       interface{}
		expectedSQL string
		wantParams  []params.QueryParam
		hasError    bool
	}{
		{
			name:        "string literal",
			input:       "test",
			expectedSQL: "@p1",
			wantParams:  []params.QueryParam{{Name: "p1", Value: "test"}},
			hasError:    false,
		},
		{
			name:        "number literal",
			input:       42,
			expectedSQL: "@p1",
			wantParams:  []params.QueryParam{{Name: "p1", Value: 42}},
			hasError:    false,
		},
		{
			name:        "boolean true",
			input:       true,
			expectedSQL: "TRUE",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "nil",
			input:       nil,
			expectedSQL: "NULL",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "ProcessedValue SQL",
			input:       ProcessedValue{Value: "elem.some_col", IsSQL: true},
			expectedSQL: "elem.some_col",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "array literal",
			input:       []interface{}{1, 2, 3},
			expectedSQL: "[@p1, @p2, @p3]",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 1},
				{Name: "p2", Value: 2},
				{Name: "p3", Value: 3},
			},
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			result, err := op.valueToSQLParam(tt.input, pc)

			if tt.hasError {
				if err == nil {
					t.Errorf("valueToSQLParam() expected error, got nil")
				}
				if pc.Count() != 0 {
					t.Errorf("valueToSQLParam() on error expected Count() 0, got %d", pc.Count())
				}
				return
			}
			if err != nil {
				t.Fatalf("valueToSQLParam() unexpected error = %v", err)
			}
			if result != tt.expectedSQL {
				t.Errorf("valueToSQLParam() = %q, want %q", result, tt.expectedSQL)
			}
			assertCollectedParams(t, pc, tt.wantParams)
		})
	}
}

func TestArrayOperator_ToSQLParam_UnsupportedOperator(t *testing.T) {
	op := newArrayOperatorWithParamParserBQ(t)
	pc := params.NewParamCollector(params.PlaceholderNamed)

	_, err := op.ToSQLParam("unsupported", []interface{}{[]interface{}{1}}, pc)
	if err == nil {
		t.Fatal("expected error for unsupported array operator")
	}
	if pc.Count() != 0 {
		t.Errorf("expected no params on error, got Count()=%d", pc.Count())
	}
}

func TestArrayOperator_ToSQLParam_DialectValidation(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectUnspecified, &arrayTestSchemaProvider{})
	config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
		return "", fmt.Errorf("unexpected ParamExpressionParser in dialect validation test")
	})
	op := NewArrayOperator(config)

	minMapArgs := []interface{}{
		[]interface{}{1, 2},
		map[string]interface{}{"var": ""},
	}
	minFilterArgs := []interface{}{
		[]interface{}{1, 2, 3},
		map[string]interface{}{">": []interface{}{map[string]interface{}{"var": ""}, 0}},
	}
	minReduceArgs := []interface{}{
		[]interface{}{1, 2},
		map[string]interface{}{"+": []interface{}{
			map[string]interface{}{"var": "accumulator"},
			map[string]interface{}{"var": "current"},
		}},
		0,
	}
	minQuantifierArgs := []interface{}{
		[]interface{}{1},
		map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": ""}, 1}},
	}

	tests := []struct {
		name     string
		operator string
		args     []interface{}
	}{
		{name: "map", operator: "map", args: minMapArgs},
		{name: "filter", operator: "filter", args: minFilterArgs},
		{name: "reduce", operator: "reduce", args: minReduceArgs},
		{name: "all", operator: "all", args: minQuantifierArgs},
		{name: "some", operator: "some", args: minQuantifierArgs},
		{name: "none", operator: "none", args: minQuantifierArgs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			_, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if err == nil {
				t.Fatalf("expected dialect validation error for operator %q", tt.operator)
			}
			if pc.Count() != 0 {
				t.Errorf("expected no params when dialect validation fails, got Count()=%d", pc.Count())
			}
		})
	}
}

func TestContainsWholeIdentifierSkipsQuotedRegions(t *testing.T) {
	tests := []struct {
		name  string
		sql   string
		ident string
		want  bool
	}{
		{name: "bare current", sql: "current + elem.x", ident: CurrentVar, want: true},
		{name: "current field path", sql: "current.x + 1", ident: CurrentVar, want: true},
		{name: "bare accumulator", sql: "accumulator + elem.x", ident: AccumulatorVar, want: true},
		{name: "single quoted current", sql: "elem.name = 'current'", ident: CurrentVar},
		{name: "escaped single quoted current", sql: "elem.name = 'it''s current'", ident: CurrentVar},
		{name: "double quoted current", sql: `elem.name = "current"`, ident: CurrentVar},
		{name: "escaped double quoted current", sql: `elem.name = "x""current"`, ident: CurrentVar},
		{name: "backtick quoted current", sql: "elem.name = `current`", ident: CurrentVar},
		{name: "dollar quoted current", sql: "$tag$current$tag$ + elem.x", ident: CurrentVar},
		{name: "line comment current", sql: "-- current\n elem.x", ident: CurrentVar},
		{name: "block comment accumulator", sql: "/* accumulator */ elem.x", ident: AccumulatorVar},
		{name: "qualified current field", sql: "elem.current + elem.x", ident: CurrentVar},
		{name: "qualified accumulator field", sql: "elem.accumulator + elem.x", ident: AccumulatorVar},
		{name: "substring current", sql: "current_balance + elem.x", ident: CurrentVar},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsWholeIdentifier(tt.sql, tt.ident); got != tt.want {
				t.Fatalf("containsWholeIdentifier(%q, %q) = %v, want %v", tt.sql, tt.ident, got, tt.want)
			}
		})
	}
}
