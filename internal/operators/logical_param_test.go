package operators

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestLogicalOperator_ToSQLParam(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	tests := []struct {
		name      string
		operator  string
		args      []interface{}
		wantSQL   string
		wantError bool
	}{
		{
			name:     "and with processed SQL",
			operator: "and",
			args: []interface{}{
				ProcessedValue{Value: "status = @p1", IsSQL: true},
				ProcessedValue{Value: "age > @p2", IsSQL: true},
			},
			wantSQL:   "(status = @p1 AND age > @p2)",
			wantError: false,
		},
		{
			name:     "or with processed SQL",
			operator: "or",
			args: []interface{}{
				ProcessedValue{Value: "failedAttempts >= @p1", IsSQL: true},
				ProcessedValue{Value: "country IN (@p2)", IsSQL: true},
			},
			wantSQL:   "(failedAttempts >= @p1 OR country IN (@p2))",
			wantError: false,
		},
		{
			name:      "not with processed SQL",
			operator:  "!",
			args:      []interface{}{ProcessedValue{Value: "verified = TRUE", IsSQL: true}},
			wantSQL:   "NOT (verified = TRUE)",
			wantError: false,
		},
		{
			name:      "!! with processed SQL",
			operator:  "!!",
			args:      []interface{}{ProcessedValue{Value: "name", IsSQL: true}},
			wantSQL:   "(name IS NOT NULL AND name != FALSE AND name != 0 AND name != '')",
			wantError: false,
		},
		{
			name:     "if with processed SQL",
			operator: "if",
			args: []interface{}{
				ProcessedValue{Value: "age > @p1", IsSQL: true},
				ProcessedValue{Value: "adult", IsSQL: true},
				ProcessedValue{Value: "minor", IsSQL: true},
			},
			wantSQL:   "CASE WHEN age > @p1 THEN adult ELSE minor END",
			wantError: false,
		},
		{
			name:      "unsupported operator",
			operator:  "xor",
			args:      []interface{}{ProcessedValue{Value: "1", IsSQL: true}},
			wantError: true,
		},
		{
			name:      "and with no args",
			operator:  "and",
			args:      []interface{}{},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if tt.wantError {
				if err == nil {
					t.Fatalf("ToSQLParam() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ToSQLParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("ToSQLParam() = %q, want %q", got, tt.wantSQL)
			}
		})
	}
}

func TestLogicalOperator_expressionToSQLParam(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	t.Run("primitive string", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.expressionToSQLParam("hello", pc)
		if err != nil {
			t.Fatalf("expressionToSQLParam() error = %v", err)
		}
		if got != "@p1" {
			t.Errorf("got %q, want @p1", got)
		}
		ps := pc.Params()
		if len(ps) != 1 || ps[0].Value != "hello" {
			t.Errorf("Params() = %#v, want one param with value hello", ps)
		}
	})

	t.Run("primitive nil", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.expressionToSQLParam(nil, pc)
		if err != nil {
			t.Fatalf("expressionToSQLParam() error = %v", err)
		}
		if got != "NULL" {
			t.Errorf("got %q, want NULL", got)
		}
		if len(pc.Params()) != 0 {
			t.Errorf("expected no params, got %d", len(pc.Params()))
		}
	})

	t.Run("primitive bool true", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.expressionToSQLParam(true, pc)
		if err != nil {
			t.Fatalf("expressionToSQLParam() error = %v", err)
		}
		if got != "TRUE" {
			t.Errorf("got %q, want TRUE", got)
		}
	})

	t.Run("primitive bool false", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.expressionToSQLParam(false, pc)
		if err != nil {
			t.Fatalf("expressionToSQLParam() error = %v", err)
		}
		if got != "FALSE" {
			t.Errorf("got %q, want FALSE", got)
		}
	})

	t.Run("numeric value", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.expressionToSQLParam(int64(42), pc)
		if err != nil {
			t.Fatalf("expressionToSQLParam() error = %v", err)
		}
		if got != "@p1" {
			t.Errorf("got %q, want @p1", got)
		}
		ps := pc.Params()
		if len(ps) != 1 || ps[0].Value != int64(42) {
			t.Errorf("Params() = %#v, want one param with int64(42)", ps)
		}
	})
}

func TestLogicalOperator_handleIfParam(t *testing.T) {
	op := NewLogicalOperator(testFieldOnlyConfig())

	t.Run("simple if with else", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		args := []interface{}{
			ProcessedValue{Value: "score > @p0", IsSQL: true},
			"A",
			"B",
		}
		got, err := op.handleIfParam(args, pc)
		if err != nil {
			t.Fatalf("handleIfParam() error = %v", err)
		}
		want := "CASE WHEN score > @p0 THEN @p1 ELSE @p2 END"
		if got != want {
			t.Errorf("handleIfParam() = %q, want %q", got, want)
		}
		ps := pc.Params()
		if len(ps) != 2 || ps[0].Value != "A" || ps[1].Value != "B" {
			t.Errorf("Params() = %#v, want A and B string binds", ps)
		}
	})

	t.Run("multiple condition-value pairs without final else", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		args := []interface{}{
			map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 90}},
			"A",
			map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "score"}, 80}},
			"B",
		}
		got, err := op.handleIfParam(args, pc)
		if err != nil {
			t.Fatalf("handleIfParam() error = %v", err)
		}
		want := "CASE WHEN score > @p1 THEN @p2 WHEN score > @p3 THEN @p4 END"
		if got != want {
			t.Errorf("handleIfParam() = %q, want %q", got, want)
		}
		wantParams := []params.QueryParam{
			{Name: "p1", Value: 90},
			{Name: "p2", Value: "A"},
			{Name: "p3", Value: 80},
			{Name: "p4", Value: "B"},
		}
		if !reflect.DeepEqual(pc.Params(), wantParams) {
			t.Errorf("Params() = %#v, want %#v", pc.Params(), wantParams)
		}
	})
}

func TestLogicalOperator_processArgsParam(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
		if m, ok := expr.(map[string]interface{}); ok {
			if varName, ok := m["var"]; ok {
				return fmt.Sprintf("%v", varName), nil
			}
		}
		return "MOCK_SQL", nil
	})
	op := NewLogicalOperator(config)

	t.Run("var preserved and comparison becomes SQL", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		varArg := map[string]interface{}{"var": "col"}
		args := []interface{}{
			varArg,
			map[string]interface{}{"==": []interface{}{
				map[string]interface{}{"var": "a"},
				int64(10),
			}},
		}
		out, err := op.processArgsParam(args, pc)
		if err != nil {
			t.Fatalf("processArgsParam() error = %v", err)
		}
		if len(out) != 2 {
			t.Fatalf("len(out) = %d, want 2", len(out))
		}
		if !reflect.DeepEqual(out[0], varArg) {
			t.Errorf("var arg should be unchanged, got %#v want %#v", out[0], varArg)
		}
		pv, ok := out[1].(ProcessedValue)
		if !ok || !pv.IsSQL {
			t.Fatalf("second arg should be ProcessedValue SQL, got %#v", out[1])
		}
		if pv.Value != "a = @p1" {
			t.Errorf("comparison SQL = %q, want a = @p1", pv.Value)
		}
		if len(pc.Params()) != 1 || pc.Params()[0].Value != int64(10) {
			t.Errorf("params = %#v, want one bind int64(10)", pc.Params())
		}
	})

	t.Run("primitives pass through", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		args := []interface{}{int64(1), "x"}
		out, err := op.processArgsParam(args, pc)
		if err != nil {
			t.Fatalf("processArgsParam() error = %v", err)
		}
		if len(out) != 2 || out[0] != int64(1) || out[1] != "x" {
			t.Errorf("out = %#v, want unchanged primitives", out)
		}
	})

	t.Run("nested expression via mock parser", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		args := []interface{}{
			map[string]interface{}{"custom": []interface{}{"ignored"}},
		}
		out, err := op.processArgsParam(args, pc)
		if err != nil {
			t.Fatalf("processArgsParam() error = %v", err)
		}
		if len(out) != 1 {
			t.Fatalf("len(out) = %d, want 1", len(out))
		}
		pv, ok := out[0].(ProcessedValue)
		if !ok || !pv.IsSQL || pv.Value != "MOCK_SQL" {
			t.Errorf("got %#v, want ProcessedValue{MOCK_SQL, IsSQL:true}", out[0])
		}
	})
}
