package operators

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestNumericOperator_ToSQLParam(t *testing.T) {
	op := NewNumericOperator(testFieldOnlyConfig())

	tests := []struct {
		name       string
		operator   string
		args       []interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
		skipParams bool
	}{
		{
			name:     "addition with two numbers",
			operator: "+",
			args:     []interface{}{5, 3},
			wantSQL:  "(@p1 + @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 5},
				{Name: "p2", Value: 3},
			},
		},
		{
			name:     "addition coerces boolean literals",
			operator: "+",
			args:     []interface{}{true, false, 2},
			wantSQL:  "(1 + 0 + @p1)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 2},
			},
		},
		{
			name:     "addition coerces predicate SQL",
			operator: "+",
			args: []interface{}{
				TypedSQLResult("x > 1", ExpressionKindPredicate, ExpressionTypeBoolean),
				2,
			},
			wantSQL: "((CASE WHEN x > 1 THEN 1 ELSE 0 END) + @p1)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 2},
			},
		},
		{
			name:     "addition with var and number",
			operator: "+",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, 100},
			wantSQL:  "(amount + @p1)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 100},
			},
		},
		{
			name:     "unary plus",
			operator: "+",
			args:     []interface{}{5},
			wantSQL:  "CAST(@p1 AS NUMERIC)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 5},
			},
		},
		{
			name:     "subtraction with two numbers",
			operator: "-",
			args:     []interface{}{10, 3},
			wantSQL:  "(@p1 - @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 10},
				{Name: "p2", Value: 3},
			},
		},
		{
			name:     "unary minus",
			operator: "-",
			args:     []interface{}{10},
			wantSQL:  "(-@p1)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 10},
			},
		},
		{
			name:       "unary minus with var",
			operator:   "-",
			args:       []interface{}{map[string]interface{}{"var": "value"}},
			wantSQL:    "(-value)",
			wantParams: nil,
		},
		{
			name:     "multiplication",
			operator: "*",
			args:     []interface{}{4, 5},
			wantSQL:  "(@p1 * @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 4},
				{Name: "p2", Value: 5},
			},
		},
		{
			name:     "division",
			operator: "/",
			args:     []interface{}{20, 4},
			wantSQL:  "(@p1 / @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 20},
				{Name: "p2", Value: 4},
			},
		},
		{
			name:     "modulo",
			operator: "%",
			args:     []interface{}{17, 5},
			wantSQL:  "(@p1 % @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 17},
				{Name: "p2", Value: 5},
			},
		},
		{
			name:     "max",
			operator: "max",
			args:     []interface{}{10, 20},
			wantSQL:  "GREATEST(@p1, @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 10},
				{Name: "p2", Value: 20},
			},
		},
		{
			name:     "min",
			operator: "min",
			args:     []interface{}{5, 15},
			wantSQL:  "LEAST(@p1, @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 5},
				{Name: "p2", Value: 15},
			},
		},
		{
			name:       "unsupported operator",
			operator:   "avg",
			args:       []interface{}{1, 2},
			wantErr:    true,
			skipParams: true,
		},
		{
			name:       "multiplication too few arguments",
			operator:   "*",
			args:       []interface{}{5},
			wantErr:    true,
			skipParams: true,
		},
		{
			name:       "division too few arguments",
			operator:   "/",
			args:       []interface{}{10},
			wantErr:    true,
			skipParams: true,
		},
		{
			name:       "modulo too few arguments",
			operator:   "%",
			args:       []interface{}{17},
			wantErr:    true,
			skipParams: true,
		},
		{
			name:       "max too few arguments",
			operator:   "max",
			args:       []interface{}{10},
			wantErr:    true,
			skipParams: true,
		},
		{
			name:       "min too few arguments",
			operator:   "min",
			args:       []interface{}{5},
			wantErr:    true,
			skipParams: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			gotSQL, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ToSQLParam() expected error, got nil (sql=%q)", gotSQL)
				}
				return
			}
			if err != nil {
				t.Fatalf("ToSQLParam() unexpected error: %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("ToSQLParam() sql = %q, want %q", gotSQL, tt.wantSQL)
			}
			if !tt.skipParams {
				assertNumericQueryParams(t, pc.Params(), tt.wantParams)
			}
		})
	}
}

func TestNumericOperator_valueToSQLParam(t *testing.T) {
	op := NewNumericOperator(testFieldOnlyConfig())

	tests := []struct {
		name       string
		input      interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:    "literal number",
			input:   42,
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 42},
			},
		},
		{
			name:    "literal float",
			input:   3.14,
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 3.14},
			},
		},
		{
			name:       "var expression",
			input:      map[string]interface{}{"var": "amount"},
			wantSQL:    "amount",
			wantParams: nil,
		},
		{
			name:       "ProcessedValue SQL",
			input:      ProcessedValue{Value: "SUM(amount)", IsSQL: true},
			wantSQL:    "SUM(amount)",
			wantParams: nil,
		},
		{
			name:       "ProcessedValue predicate SQL",
			input:      TypedSQLResult("x > 1", ExpressionKindPredicate, ExpressionTypeBoolean),
			wantSQL:    "(CASE WHEN x > 1 THEN 1 ELSE 0 END)",
			wantParams: nil,
		},
		{
			name:       "boolean literal true",
			input:      true,
			wantSQL:    "1",
			wantParams: nil,
		},
		{
			name:       "boolean literal false",
			input:      false,
			wantSQL:    "0",
			wantParams: nil,
		},
		{
			name:    "ProcessedValue literal",
			input:   ProcessedValue{Value: "hello", IsSQL: false},
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "hello"},
			},
		},
		{
			name:    "numeric string coerced to integer",
			input:   "42",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: int64(42)},
			},
		},
		{
			name:    "numeric string coerced to float",
			input:   "3.14",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 3.14},
			},
		},
		{
			name:    "non-numeric string safely parameterized",
			input:   "hello",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "hello"},
			},
		},
		{
			name:    "large integer beyond int64 preserved as string",
			input:   "9223372036854775808",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "9223372036854775808"},
			},
		},
		{
			name:    "negative large integer preserved as string",
			input:   "-9223372036854775809",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "-9223372036854775809"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			gotSQL, err := op.valueToSQLParam(tt.input, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("valueToSQLParam() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("valueToSQLParam() unexpected error: %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("valueToSQLParam() = %q, want %q", gotSQL, tt.wantSQL)
			}
			assertNumericQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestNumericOperator_ToSQLParam_WithSchemaValidation(t *testing.T) {
	schema := &numericSchemaProvider{
		fields: map[string]string{
			"amount": "integer",
			"name":   "string",
		},
	}

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewNumericOperator(config)

	t.Run("string field in numeric operation fails", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		_, err := op.ToSQLParam("+", []interface{}{
			map[string]interface{}{"var": "name"},
			10,
		}, pc)
		if err == nil {
			t.Fatal("ToSQLParam() expected error for string field in addition, got nil")
		}
		if len(pc.Params()) != 0 {
			t.Errorf("expected no params on error, got %#v", pc.Params())
		}
	})

	t.Run("numeric field succeeds", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		sql, err := op.ToSQLParam("+", []interface{}{
			map[string]interface{}{"var": "amount"},
			10,
		}, pc)
		if err != nil {
			t.Fatalf("ToSQLParam() unexpected error: %v", err)
		}
		if sql != "(amount + @p1)" {
			t.Errorf("ToSQLParam() = %q, want (amount + @p1)", sql)
		}
		assertNumericQueryParams(t, pc.Params(), []params.QueryParam{
			{Name: "p1", Value: 10},
		})
	})
}

func TestNumericOperator_valueToSQLParam_ExpressionParserCallback(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
		if pc == nil {
			t.Error("ParamCollector should be non-nil")
		}
		return "CUSTOM_NUMERIC_PARAM()", nil
	})
	op := NewNumericOperator(config)

	pc := params.NewParamCollector(params.PlaceholderNamed)
	result, err := op.valueToSQLParam(map[string]interface{}{"customOp": []interface{}{1, 2}}, pc)
	if err != nil {
		t.Fatalf("valueToSQLParam() unexpected error: %v", err)
	}
	if result != "CUSTOM_NUMERIC_PARAM()" {
		t.Errorf("valueToSQLParam() = %q, want CUSTOM_NUMERIC_PARAM()", result)
	}
	if len(pc.Params()) != 0 {
		t.Errorf("expected no params, got %#v", pc.Params())
	}
}

func TestNumericOperator_processComplexArgsParam(t *testing.T) {
	op := NewNumericOperator(testFieldOnlyConfig())

	pc := params.NewParamCollector(params.PlaceholderNamed)
	out, err := op.processComplexArgsParam([]interface{}{
		float64(2),
		map[string]interface{}{"var": "price"},
	}, pc)
	if err != nil {
		t.Fatalf("processComplexArgsParam() unexpected error: %v", err)
	}
	if len(out) != 2 || out[0] != "@p1" || out[1] != "price" {
		t.Fatalf("processComplexArgsParam() = %#v, want [@p1 price]", out)
	}
	assertNumericQueryParams(t, pc.Params(), []params.QueryParam{
		{Name: "p1", Value: float64(2)},
	})
}

func TestNumericOperator_processComplexArgsForComparisonParam(t *testing.T) {
	op := NewNumericOperator(testFieldOnlyConfig())

	t.Run("var and primitive pass through", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		args := []interface{}{
			map[string]interface{}{"var": "amount"},
			42,
		}
		result, err := op.processComplexArgsForComparisonParam(args, pc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("returned %d args, want 2", len(result))
		}
		varMap, ok := result[0].(map[string]interface{})
		if !ok {
			t.Fatalf("[0] is %T, want map[string]interface{}", result[0])
		}
		if _, hasVar := varMap["var"]; !hasVar {
			t.Error("[0] has no 'var' key")
		}
		num, ok := result[1].(int)
		if !ok {
			t.Fatalf("[1] is %T, want int", result[1])
		}
		if num != 42 {
			t.Errorf("[1] = %v, want 42", num)
		}
		if len(pc.Params()) != 0 {
			t.Errorf("expected no params, got %#v", pc.Params())
		}
	})

	t.Run("nested expression wrapped as SQLResult", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		args := []interface{}{
			map[string]interface{}{"+": []interface{}{
				map[string]interface{}{"var": "x"}, float64(1),
			}},
			float64(10),
		}
		result, err := op.processComplexArgsForComparisonParam(args, pc)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 2 {
			t.Fatalf("returned %d args, want 2", len(result))
		}
		pv, ok := result[0].(ProcessedValue)
		if !ok {
			t.Fatalf("[0] is %T, want ProcessedValue", result[0])
		}
		if !pv.IsSQL || pv.Value != "(x + @p1)" {
			t.Errorf("[0] = %+v, want SQLResult('(x + @p1)')", pv)
		}
		if result[1] != float64(10) {
			t.Errorf("[1] = %v, want 10", result[1])
		}
		assertNumericQueryParams(t, pc.Params(), []params.QueryParam{
			{Name: "p1", Value: float64(1)},
		})
	})
}
