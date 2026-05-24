package operators

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestComparisonOperator_ToSQLParam(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		operator   string
		args       []interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:     "equality with var and literal",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "status"}, "pending"},
			wantSQL:  "status = @p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "pending"},
			},
		},
		{
			name:     "equality with numbers",
			operator: "==",
			args:     []interface{}{1, 2},
			wantSQL:  "@p1 = @p2",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 1},
				{Name: "p2", Value: 2},
			},
		},
		{
			name:       "equality with null",
			operator:   "==",
			args:       []interface{}{nil, nil},
			wantSQL:    "NULL IS NULL",
			wantParams: nil,
		},
		{
			name:       "var and null",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "field"}, nil},
			wantSQL:    "field IS NULL",
			wantParams: nil,
		},
		{
			name:       "inequality with var and null",
			operator:   "!=",
			args:       []interface{}{map[string]interface{}{"var": "field"}, nil},
			wantSQL:    "field IS NOT NULL",
			wantParams: nil,
		},
		{
			name:       "strict equality !==  with null",
			operator:   "!==",
			args:       []interface{}{map[string]interface{}{"var": "field"}, nil},
			wantSQL:    "field IS NOT NULL",
			wantParams: nil,
		},
		{
			name:     "greater than with var and number",
			operator: ">",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, 1000},
			wantSQL:  "amount > @p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 1000},
			},
		},
		{
			name:     "in with array",
			operator: "in",
			args:     []interface{}{map[string]interface{}{"var": "country"}, []interface{}{"CN", "RU"}},
			wantSQL:  "country IN (@p1, @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "CN"},
				{Name: "p2", Value: "RU"},
			},
		},
		{
			name:     "chained comparison",
			operator: "<",
			args: []interface{}{
				10,
				map[string]interface{}{"var": "age"},
				30,
			},
			wantSQL: "(@p1 < age AND age < @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 10},
				{Name: "p2", Value: 30},
			},
		},
		{
			name:     "too few arguments",
			operator: "==",
			args:     []interface{}{1},
			wantErr:  true,
		},
		{
			name:     "in with empty array",
			operator: "in",
			args:     []interface{}{map[string]interface{}{"var": "field"}, []interface{}{}},
			wantSQL:  "FALSE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ToSQLParam() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ToSQLParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("ToSQLParam() SQL = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_ToSQLParam_NullSafeFieldEquality(t *testing.T) {
	defaultConfig := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	defaultOp := NewComparisonOperator(defaultConfig)
	pc := params.NewParamCollector(params.PlaceholderNamed)
	got, err := defaultOp.ToSQLParam("==", []interface{}{
		map[string]interface{}{"var": "a"},
		map[string]interface{}{"var": "b"},
	}, pc)
	if err != nil {
		t.Fatalf("ToSQLParam() default unexpected error = %v", err)
	}
	if want := "((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b))"; got != want {
		t.Fatalf("ToSQLParam() default = %q, want %q", got, want)
	}
	assertQueryParams(t, pc.Params(), nil)

	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		operator   string
		args       []interface{}
		wantSQL    string
		wantParams []params.QueryParam
	}{
		{
			name:     "loose equality var to var has no params",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				map[string]interface{}{"var": "b"},
			},
			wantSQL:    "((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b))",
			wantParams: nil,
		},
		{
			name:     "strict equality var to var has no params",
			operator: "===",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				map[string]interface{}{"var": "b"},
			},
			wantSQL:    "((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b))",
			wantParams: nil,
		},
		{
			name:     "loose inequality var to var has no params",
			operator: "!=",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				map[string]interface{}{"var": "b"},
			},
			wantSQL:    "((a IS NULL AND b IS NOT NULL) OR (a IS NOT NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a != b))",
			wantParams: nil,
		},
		{
			name:     "strict inequality var to var has no params",
			operator: "!==",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				map[string]interface{}{"var": "b"},
			},
			wantSQL:    "((a IS NULL AND b IS NOT NULL) OR (a IS NOT NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a <> b))",
			wantParams: nil,
		},
		{
			name:     "defaulted vars preserve default param ordering",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": []interface{}{"a", "left-default"}},
				map[string]interface{}{"var": []interface{}{"b", "right-default"}},
			},
			wantSQL: "((COALESCE(a, @p1) IS NULL AND COALESCE(b, @p2) IS NULL) OR (COALESCE(a, @p1) IS NOT NULL AND COALESCE(b, @p2) IS NOT NULL AND COALESCE(a, @p1) = COALESCE(b, @p2)))",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "left-default"},
				{Name: "p2", Value: "right-default"},
			},
		},
		{
			name:     "field literal remains parameterized comparison",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				"x",
			},
			wantSQL: "a = @p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "x"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if err != nil {
				t.Fatalf("ToSQLParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Fatalf("ToSQLParam() = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_valueToSQLParam(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		input      interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:    "literal string",
			input:   "hello",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "hello"},
			},
		},
		{
			name:    "literal number",
			input:   42,
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 42},
			},
		},
		{
			name:       "literal boolean",
			input:      true,
			wantSQL:    "TRUE",
			wantParams: nil,
		},
		{
			name:       "var expression",
			input:      map[string]interface{}{"var": "amount"},
			wantSQL:    "amount",
			wantParams: nil,
		},
		{
			name:       "ProcessedValue SQL",
			input:      ProcessedValue{Value: "some_col + 1", IsSQL: true},
			wantSQL:    "some_col + 1",
			wantParams: nil,
		},
		{
			name:    "ProcessedValue literal",
			input:   ProcessedValue{Value: "inner", IsSQL: false},
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "inner"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.valueToSQLParam(tt.input, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("valueToSQLParam() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("valueToSQLParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("valueToSQLParam() = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}
