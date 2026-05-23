package operators

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestComparisonOperator_ToSQLParam_EqualitySemanticsWithSchema(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"amount":         "integer",
		"score":          "number",
		"active":         "boolean",
		"code":           "string",
		"other":          "string",
		"status":         "enum",
		"limited_status": "enum",
	})
	schema.enumValues["status"] = []string{"active", "1", "1.2", "9223372036854775807", "Infinity", "-Infinity"}
	schema.enumValues["limited_status"] = []string{"active"}
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		operator   string
		args       []interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantError  bool
	}{
		{
			name:       "numeric string literal is coerced",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "010"},
			wantSQL:    "amount = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: int64(10)}},
		},
		{
			name:       "numeric unsafe decimal string rounds like javascript",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "9007199254740993"},
			wantSQL:    "amount = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: int64(9007199254740992)}},
		},
		{
			name:       "numeric invalid string folds without params",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "abc"},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "string field preserves Go int64 literal exactly",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, int64(9223372036854775807)},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "9223372036854775807"}},
		},
		{
			name:       "enum field validates exact Go int64 string",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "status"}, int64(9223372036854775807)},
			wantSQL:    "status = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "9223372036854775807"}},
		},
		{
			name:       "string field preserves Go float32 literal formatting",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, float32(1.2)},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "1.2"}},
		},
		{
			name:       "enum field validates Go float32 literal formatting",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "status"}, float32(1.2)},
			wantSQL:    "status = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "1.2"}},
		},
		{
			name:       "string field json overflow canonicalizes infinity",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e400")},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "Infinity"}},
		},
		{
			name:       "string field json overflow canonicalizes infinity for not equal",
			operator:   "!=",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e400")},
			wantSQL:    "code != @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "Infinity"}},
		},
		{
			name:       "string field json negative overflow canonicalizes negative infinity",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("-1e400")},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "-Infinity"}},
		},
		{
			name:       "string field json underflow canonicalizes zero",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e-400")},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "0"}},
		},
		{
			name:       "string field native infinity canonicalizes",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, math.Inf(1)},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "Infinity"}},
		},
		{
			name:       "string field native NaN still folds without params",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, math.NaN()},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "enum field validates infinity canonical string",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "status"}, math.Inf(1)},
			wantSQL:    "status = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "Infinity"}},
		},
		{
			name:      "enum field rejects infinity canonical string",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": "limited_status"}, math.Inf(1)},
			wantError: true,
		},
		{
			name:       "integer field preserves large uint64 literal within int64 range",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, uint64(9007199254740993)},
			wantSQL:    "amount = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: int64(9007199254740993)}},
		},
		{
			name:       "number field preserves large uint64 literal above int64 range",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "score"}, uint64(9223372036854775808)},
			wantSQL:    "score = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: uint64(9223372036854775808)}},
		},
		{
			name:       "strict number field folds NaN without params",
			operator:   "===",
			args:       []interface{}{map[string]interface{}{"var": "score"}, math.NaN()},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "strict number field folds NaN true for not equal without params",
			operator:   "!==",
			args:       []interface{}{map[string]interface{}{"var": "score"}, math.NaN()},
			wantSQL:    "TRUE",
			wantParams: nil,
		},
		{
			name:       "strict number field folds infinity without params",
			operator:   "===",
			args:       []interface{}{map[string]interface{}{"var": "score"}, math.Inf(1)},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "strict number field folds infinity true for not equal without params",
			operator:   "!==",
			args:       []interface{}{map[string]interface{}{"var": "score"}, math.Inf(1)},
			wantSQL:    "TRUE",
			wantParams: nil,
		},
		{
			name:       "integer field uint64 above int64 range folds without params",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, uint64(9223372036854775808)},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:      "malformed json number literal errors before strict fold",
			operator:  "!==",
			args:      []interface{}{map[string]interface{}{"var": "code"}, json.Number("0 OR 1=1")},
			wantError: true,
		},
		{
			name:       "defaulted numeric var splits fallback equality",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": []interface{}{"amount", "abc"}}, "abc"},
			wantSQL:    "amount IS NULL",
			wantParams: nil,
		},
		{
			name:       "defaulted numeric var coerces numeric string",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": []interface{}{"amount", 0}}, "50"},
			wantSQL:    "COALESCE(amount, @p1) = @p2",
			wantParams: []params.QueryParam{{Name: "p1", Value: 0}, {Name: "p2", Value: int64(50)}},
		},
		{
			name:       "defaulted numeric var coerces numeric string with field on right",
			operator:   "==",
			args:       []interface{}{"50", map[string]interface{}{"var": []interface{}{"amount", 0}}},
			wantSQL:    "@p1 = COALESCE(amount, @p2)",
			wantParams: []params.QueryParam{{Name: "p1", Value: int64(50)}, {Name: "p2", Value: 0}},
		},
		{
			name:       "defaulted numeric var invalid string folds without params when default cannot match",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": []interface{}{"amount", 0}}, "abc"},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "defaulted numeric var invalid string folds without params with field on right when default cannot match",
			operator:   "==",
			args:       []interface{}{"abc", map[string]interface{}{"var": []interface{}{"amount", 0}}},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "defaulted strict numeric mismatch folds without params when default cannot match",
			operator:   "===",
			args:       []interface{}{map[string]interface{}{"var": []interface{}{"amount", 0}}, "50"},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "defaulted strict numeric mismatch folds without params with field on right when default cannot match",
			operator:   "===",
			args:       []interface{}{"50", map[string]interface{}{"var": []interface{}{"amount", 0}}},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "defaulted strict numeric mismatch matches only missing field when default can match",
			operator:   "===",
			args:       []interface{}{map[string]interface{}{"var": []interface{}{"amount", "50"}}, "50"},
			wantSQL:    "amount IS NULL",
			wantParams: nil,
		},
		{
			name:       "defaulted strict numeric mismatch matches only missing field with field on right when default can match",
			operator:   "===",
			args:       []interface{}{"50", map[string]interface{}{"var": []interface{}{"amount", "50"}}},
			wantSQL:    "amount IS NULL",
			wantParams: nil,
		},
		{
			name:      "defaulted string field boolean literal errors",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"code", ""}}, true},
			wantError: true,
		},
		{
			name:      "defaulted string field boolean literal errors with field on right",
			operator:  "==",
			args:      []interface{}{true, map[string]interface{}{"var": []interface{}{"code", ""}}},
			wantError: true,
		},
		{
			name:      "defaulted enum validates visible default",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"status", "unknown"}}, "active"},
			wantError: true,
		},
		{
			name:      "defaulted enum validates visible default before null equality",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"status", "unknown"}}, nil},
			wantError: true,
		},
		{
			name:      "defaulted enum validates visible default with var on right",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"status", "unknown"}}, map[string]interface{}{"var": "other"}},
			wantError: true,
		},
		{
			name:      "defaulted enum validates visible default with var on left",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": "other"}, map[string]interface{}{"var": []interface{}{"status", "unknown"}}},
			wantError: true,
		},
		{
			name:       "defaulted enum preserves valid default with var on right",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": []interface{}{"status", "active"}}, map[string]interface{}{"var": "other"}},
			wantSQL:    "((COALESCE(status, @p1) IS NULL AND other IS NULL) OR (COALESCE(status, @p1) IS NOT NULL AND other IS NOT NULL AND COALESCE(status, @p1) = other))",
			wantParams: []params.QueryParam{{Name: "p1", Value: "active"}},
		},
		{
			name:      "defaulted var malformed json number default errors before strict fold",
			operator:  "!==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"amount", json.Number("0 OR 1=1")}}, "abc"},
			wantError: true,
		},
		{
			name:      "defaulted var malformed json number default errors before loose fold",
			operator:  "!=",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"amount", json.Number("0 OR 1=1")}}, "abc"},
			wantError: true,
		},
		{
			name:       "numeric above int64 range folds without params",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, json.Number("9223372036854775808")},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "numeric min int64 string remains param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "-9223372036854775808"},
			wantSQL:    "amount = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: int64(-9223372036854775807 - 1)}},
		},
		{
			name:       "numeric min int64 string not equal remains param",
			operator:   "!=",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "-9223372036854775808"},
			wantSQL:    "amount != @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: int64(-9223372036854775807 - 1)}},
		},
		{
			name:       "numeric above int64 range folds true for not equal without params",
			operator:   "!=",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, json.Number("9223372036854775808")},
			wantSQL:    "TRUE",
			wantParams: nil,
		},
		{
			name:       "number oversized radix string becomes numeric param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "score"}, "0x8000000000000000"},
			wantSQL:    "score = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: float64(9223372036854775808)}},
		},
		{
			name:       "number large json number literal preserves exact param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "score"}, json.Number("9223372036854775808")},
			wantSQL:    "score = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "9223372036854775808"}},
		},
		{
			name:       "number underflow json number literal preserves exact param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "score"}, json.Number("1e-400")},
			wantSQL:    "score = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "1e-400"}},
		},
		{
			name:       "integer max int64 hex string folds after javascript rounding",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "0x7fffffffffffffff"},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "integer oversized radix string folds without params",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "0x8000000000000000"},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "boolean string literal coerces inline",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "active"}, "0"},
			wantSQL:    "active = FALSE",
			wantParams: nil,
		},
		{
			name:       "string numeric literal becomes string param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("5.0")},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "5"}},
		},
		{
			name:       "string small exponent literal becomes javascript canonical string param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e-7")},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "1e-7"}},
		},
		{
			name:       "string large numeric literal becomes javascript canonical string param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("9223372036854775808")},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "9223372036854776000"}},
		},
		{
			name:       "strict mismatch folds without params",
			operator:   "===",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "5"},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:      "string boolean literal errors before adding params",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": "code"}, true},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if tt.wantError {
				if err == nil {
					t.Fatalf("ToSQLParam() expected error, got nil")
				}
				if len(pc.Params()) != 0 {
					t.Fatalf("ToSQLParam() params on error = %#v, want none", pc.Params())
				}
				return
			}
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

func TestComparisonOperator_ToSQLParam_EqualityConstantFoldsValidateSchema(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"bad`field": "integer",
	})
	schema.validateErr = fmt.Errorf("schema field %q contains quote characters", "bad`field")
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []interface{}
	}{
		{
			name:     "loose impossible comparison validates schema",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "bad`field"}, "abc"},
		},
		{
			name:     "strict type mismatch validates schema",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "bad`field"}, "5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if err == nil {
				t.Fatalf("ToSQLParam() = %q, expected schema validation error", got)
			}
			if !strings.Contains(err.Error(), "contains quote characters") {
				t.Fatalf("ToSQLParam() error = %v, want schema validation error", err)
			}
			if len(pc.Params()) != 0 {
				t.Fatalf("ToSQLParam() params on validation error = %#v, want none", pc.Params())
			}
		})
	}
}

func TestComparisonOperator_valueToSQLParam_ExpressionParserCallback(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
		return "CUSTOM_PARAM()", nil
	})
	op := NewComparisonOperator(config)

	pc := params.NewParamCollector(params.PlaceholderNamed)
	result, err := op.valueToSQLParam(map[string]interface{}{"customOp": []interface{}{1, 2}}, pc)
	if err != nil {
		t.Fatalf("valueToSQLParam() unexpected error = %v", err)
	}
	if result != "CUSTOM_PARAM()" {
		t.Errorf("valueToSQLParam() = %q, want CUSTOM_PARAM()", result)
	}
	if len(pc.Params()) != 0 {
		t.Errorf("expected no params, got %#v", pc.Params())
	}
}
