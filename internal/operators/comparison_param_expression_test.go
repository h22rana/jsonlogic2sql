package operators

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestComparisonOperator_processArithmeticExpressionParam(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		operator   string
		args       interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:     "addition",
			operator: "+",
			args:     []interface{}{1, 2},
			wantSQL:  "(@p1 + @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 1},
				{Name: "p2", Value: 2},
			},
		},
		{
			name:     "unary minus",
			operator: "-",
			args:     []interface{}{42},
			wantSQL:  "(-@p1)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 42},
			},
		},
		{
			name:     "unsupported operator",
			operator: "^",
			args:     []interface{}{2, 3},
			wantErr:  true,
		},
		{
			name:     "non-array args",
			operator: "+",
			args:     "invalid",
			wantErr:  true,
		},
		{
			name:     "insufficient args for binary op",
			operator: "*",
			args:     []interface{}{5},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.processArithmeticExpressionParam(tt.operator, tt.args, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("processArithmeticExpressionParam() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("processArithmeticExpressionParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("processArithmeticExpressionParam() = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_processComparisonExpressionParam(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		operator   string
		args       interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:     "greater than",
			operator: ">",
			args:     []interface{}{5, 3},
			wantSQL:  "(@p1 > @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 5},
				{Name: "p2", Value: 3},
			},
		},
		{
			name:     "unsupported comparison",
			operator: "<>",
			args:     []interface{}{1, 2},
			wantErr:  true,
		},
		{
			name:     "non-array args",
			operator: ">",
			args:     "invalid",
			wantErr:  true,
		},
		{
			name:     "wrong number of args",
			operator: ">",
			args:     []interface{}{1, 2, 3},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.processComparisonExpressionParam(tt.operator, tt.args, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("processComparisonExpressionParam() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("processComparisonExpressionParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("processComparisonExpressionParam() = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_processMinMaxExpressionParam(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		operator   string
		args       interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:     "max",
			operator: "max",
			args:     []interface{}{5, 10},
			wantSQL:  "GREATEST(@p1, @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 5},
				{Name: "p2", Value: 10},
			},
		},
		{
			name:     "unsupported min/max operator",
			operator: "avg",
			args:     []interface{}{1, 2},
			wantErr:  true,
		},
		{
			name:     "non-array args",
			operator: "max",
			args:     "invalid",
			wantErr:  true,
		},
		{
			name:     "insufficient args",
			operator: "max",
			args:     []interface{}{5},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.processMinMaxExpressionParam(tt.operator, tt.args, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("processMinMaxExpressionParam() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("processMinMaxExpressionParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("processMinMaxExpressionParam() = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_ToSQLParam_WithSchemaCoercion(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"age": "integer",
	})
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	pc := params.NewParamCollector(params.PlaceholderNamed)
	got, err := op.ToSQLParam(">=", []interface{}{
		map[string]interface{}{"var": "age"},
		"50000",
	}, pc)
	if err != nil {
		t.Fatalf("ToSQLParam() unexpected error = %v", err)
	}
	if got != "age >= @p1" {
		t.Errorf("ToSQLParam() = %q, want %q", got, "age >= @p1")
	}
	assertQueryParams(t, pc.Params(), []params.QueryParam{
		{Name: "p1", Value: int64(50000)},
	})
}
