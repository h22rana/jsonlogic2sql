package operators

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

func TestComparisonOperator_ToSQL_EqualitySemanticsWithSchema(t *testing.T) {
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
		name      string
		operator  string
		args      []interface{}
		wantSQL   string
		wantError bool
	}{
		{
			name:     "numeric field trims string literal",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, " 5 "},
			wantSQL:  "amount = 5",
		},
		{
			name:     "numeric field parses leading zero as decimal",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "010"},
			wantSQL:  "amount = 10",
		},
		{
			name:     "numeric field parses hex string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "0x10"},
			wantSQL:  "amount = 16",
		},
		{
			name:     "numeric field parses octal string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "0o10"},
			wantSQL:  "amount = 8",
		},
		{
			name:     "numeric field parses binary string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "0b10"},
			wantSQL:  "amount = 2",
		},
		{
			name:     "integer field unsafe decimal string rounds like javascript",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "9007199254740993"},
			wantSQL:  "amount = 9007199254740992",
		},
		{
			name:     "numeric field invalid string folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "abc"},
			wantSQL:  "FALSE",
		},
		{
			name:     "string field preserves Go int64 literal exactly",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, int64(9223372036854775807)},
			wantSQL:  "code = '9223372036854775807'",
		},
		{
			name:     "enum field validates exact Go int64 string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "status"}, int64(9223372036854775807)},
			wantSQL:  "status = '9223372036854775807'",
		},
		{
			name:     "string field preserves Go float32 literal formatting",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, float32(1.2)},
			wantSQL:  "code = '1.2'",
		},
		{
			name:     "enum field validates Go float32 literal formatting",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "status"}, float32(1.2)},
			wantSQL:  "status = '1.2'",
		},
		{
			name:     "string field json overflow canonicalizes infinity",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e400")},
			wantSQL:  "code = 'Infinity'",
		},
		{
			name:     "string field json overflow canonicalizes infinity for not equal",
			operator: "!=",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e400")},
			wantSQL:  "code != 'Infinity'",
		},
		{
			name:     "string field json negative overflow canonicalizes negative infinity",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("-1e400")},
			wantSQL:  "code = '-Infinity'",
		},
		{
			name:     "string field json underflow canonicalizes zero",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e-400")},
			wantSQL:  "code = '0'",
		},
		{
			name:     "string field native infinity canonicalizes",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, math.Inf(1)},
			wantSQL:  "code = 'Infinity'",
		},
		{
			name:     "string field native NaN still folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, math.NaN()},
			wantSQL:  "FALSE",
		},
		{
			name:     "enum field validates infinity canonical string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "status"}, math.Inf(1)},
			wantSQL:  "status = 'Infinity'",
		},
		{
			name:      "enum field rejects infinity canonical string",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": "limited_status"}, math.Inf(1)},
			wantError: true,
		},
		{
			name:     "integer field preserves large uint64 literal within int64 range",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, uint64(9007199254740993)},
			wantSQL:  "amount = 9007199254740993",
		},
		{
			name:     "number field preserves large uint64 literal above int64 range",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, uint64(9223372036854775808)},
			wantSQL:  "score = 9223372036854775808",
		},
		{
			name:     "strict number field folds NaN false",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "score"}, math.NaN()},
			wantSQL:  "FALSE",
		},
		{
			name:     "strict number field folds NaN true for not equal",
			operator: "!==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, math.NaN()},
			wantSQL:  "TRUE",
		},
		{
			name:     "strict number field folds infinity false",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "score"}, math.Inf(1)},
			wantSQL:  "FALSE",
		},
		{
			name:     "strict number field folds infinity true for not equal",
			operator: "!==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, math.Inf(1)},
			wantSQL:  "TRUE",
		},
		{
			name:     "integer field uint64 above int64 range folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, uint64(9223372036854775808)},
			wantSQL:  "FALSE",
		},
		{
			name:      "malformed json number literal errors before strict fold",
			operator:  "!==",
			args:      []interface{}{map[string]interface{}{"var": "code"}, json.Number("0 OR 1=1")},
			wantError: true,
		},
		{
			name:     "defaulted numeric var splits fallback equality",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"amount", "abc"}}, "abc"},
			wantSQL:  "amount IS NULL",
		},
		{
			name:     "defaulted numeric var coerces numeric string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"amount", 0}}, "50"},
			wantSQL:  "COALESCE(amount, 0) = 50",
		},
		{
			name:     "defaulted numeric var coerces numeric string with field on right",
			operator: "==",
			args:     []interface{}{"50", map[string]interface{}{"var": []interface{}{"amount", 0}}},
			wantSQL:  "50 = COALESCE(amount, 0)",
		},
		{
			name:     "defaulted numeric var invalid string folds false when default cannot match",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"amount", 0}}, "abc"},
			wantSQL:  "FALSE",
		},
		{
			name:     "defaulted numeric var invalid string folds false with field on right when default cannot match",
			operator: "==",
			args:     []interface{}{"abc", map[string]interface{}{"var": []interface{}{"amount", 0}}},
			wantSQL:  "FALSE",
		},
		{
			name:     "defaulted strict numeric mismatch folds false when default cannot match",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"amount", 0}}, "50"},
			wantSQL:  "FALSE",
		},
		{
			name:     "defaulted strict numeric mismatch folds false with field on right when default cannot match",
			operator: "===",
			args:     []interface{}{"50", map[string]interface{}{"var": []interface{}{"amount", 0}}},
			wantSQL:  "FALSE",
		},
		{
			name:     "defaulted strict numeric mismatch matches only missing field when default can match",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"amount", "50"}}, "50"},
			wantSQL:  "amount IS NULL",
		},
		{
			name:     "defaulted strict numeric mismatch matches only missing field with field on right when default can match",
			operator: "===",
			args:     []interface{}{"50", map[string]interface{}{"var": []interface{}{"amount", "50"}}},
			wantSQL:  "amount IS NULL",
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
			name:     "defaulted enum preserves valid default with var on right",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"status", "active"}}, map[string]interface{}{"var": "other"}},
			wantSQL:  "((COALESCE(status, 'active') IS NULL AND other IS NULL) OR (COALESCE(status, 'active') IS NOT NULL AND other IS NOT NULL AND COALESCE(status, 'active') = other))",
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
			name:     "numeric field invalid string folds true for not equal",
			operator: "!=",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "abc"},
			wantSQL:  "TRUE",
		},
		{
			name:     "integer field max int64 literal remains equality",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, json.Number("9223372036854775807")},
			wantSQL:  "amount = 9223372036854775807",
		},
		{
			name:     "integer field min int64 literal remains equality",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, json.Number("-9223372036854775808")},
			wantSQL:  "amount = -9223372036854775808",
		},
		{
			name:     "integer field min int64 string remains equality",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "-9223372036854775808"},
			wantSQL:  "amount = -9223372036854775808",
		},
		{
			name:     "integer field min int64 string remains inequality",
			operator: "!=",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "-9223372036854775808"},
			wantSQL:  "amount != -9223372036854775808",
		},
		{
			name:     "integer field above int64 range folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, json.Number("9223372036854775808")},
			wantSQL:  "FALSE",
		},
		{
			name:     "integer field below int64 range folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, json.Number("-9223372036854775809")},
			wantSQL:  "FALSE",
		},
		{
			name:     "integer field above int64 range folds true for not equal",
			operator: "!=",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, json.Number("9223372036854775808")},
			wantSQL:  "TRUE",
		},
		{
			name:     "strict integer field above int64 range folds false",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, json.Number("9223372036854775808")},
			wantSQL:  "FALSE",
		},
		{
			name:     "integer field fractional literal folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, 5.5},
			wantSQL:  "FALSE",
		},
		{
			name:     "number field fractional string remains numeric",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, ".5"},
			wantSQL:  "score = 0.5",
		},
		{
			name:     "number field oversized hex string remains numeric",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, "0x8000000000000000"},
			wantSQL:  "score = 9.223372036854776e+18",
		},
		{
			name:     "number field oversized binary string remains numeric",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, "0b1000000000000000000000000000000000000000000000000000000000000000"},
			wantSQL:  "score = 9.223372036854776e+18",
		},
		{
			name:     "number field large json number literal preserves exact text",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, json.Number("9223372036854775808")},
			wantSQL:  "score = 9223372036854775808",
		},
		{
			name:     "number field underflow json number literal preserves exact text",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, json.Number("1e-400")},
			wantSQL:  "score = 1e-400",
		},
		{
			name:     "integer field max int64 hex string folds false after javascript rounding",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "0x7fffffffffffffff"},
			wantSQL:  "FALSE",
		},
		{
			name:     "integer field oversized hex string folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "0x8000000000000000"},
			wantSQL:  "FALSE",
		},
		{
			name:     "boolean field numeric one",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "active"}, 1},
			wantSQL:  "active = TRUE",
		},
		{
			name:     "boolean field string zero",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "active"}, "0"},
			wantSQL:  "active = FALSE",
		},
		{
			name:     "boolean field empty string is false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "active"}, ""},
			wantSQL:  "active = FALSE",
		},
		{
			name:     "boolean field non boolean number folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "active"}, "2"},
			wantSQL:  "FALSE",
		},
		{
			name:     "boolean field non numeric string folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "active"}, "true"},
			wantSQL:  "FALSE",
		},
		{
			name:     "string field string literal remains equality",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, "abc"},
			wantSQL:  "code = 'abc'",
		},
		{
			name:     "string field numeric literal canonicalizes",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("5.0")},
			wantSQL:  "code = '5'",
		},
		{
			name:     "string field small exponent canonicalizes like javascript",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e-7")},
			wantSQL:  "code = '1e-7'",
		},
		{
			name:     "string field decimal threshold canonicalizes like javascript",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e-6")},
			wantSQL:  "code = '0.000001'",
		},
		{
			name:     "string field large numeric literal canonicalizes like javascript",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("9223372036854775808")},
			wantSQL:  "code = '9223372036854776000'",
		},
		{
			name:      "string field boolean literal errors",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": "code"}, true},
			wantError: true,
		},
		{
			name:     "enum field numeric literal validates as canonical string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "status"}, 1},
			wantSQL:  "status = '1'",
		},
		{
			name:      "enum field boolean literal errors",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": "status"}, false},
			wantError: true,
		},
		{
			name:     "strict numeric field string literal folds false",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "5"},
			wantSQL:  "FALSE",
		},
		{
			name:     "strict numeric field string literal folds true for not equal",
			operator: "!==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "5"},
			wantSQL:  "TRUE",
		},
		{
			name:     "strict string field numeric literal folds false",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "code"}, 5},
			wantSQL:  "FALSE",
		},
		{
			name:     "strict boolean field numeric literal folds false",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "active"}, 1},
			wantSQL:  "FALSE",
		},
		{
			name:     "strict boolean field boolean literal remains equality",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "active"}, true},
			wantSQL:  "active = TRUE",
		},
		{
			name:     "null comparison remains authoritative",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, nil},
			wantSQL:  "amount IS NULL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := op.ToSQL(tt.operator, tt.args)
			if tt.wantError {
				if err == nil {
					t.Fatalf("ToSQL() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ToSQL() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Fatalf("ToSQL() = %q, want %q", got, tt.wantSQL)
			}
		})
	}
}

func TestComparisonOperator_ToSQL_NullSafeFieldEqualityPreservesSchemaLiteralSemantics(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"amount": "integer",
		"code":   "string",
	})
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		wantSQL  string
		wantErr  string
	}{
		{
			name:     "strict field literal mismatch still folds",
			operator: "===",
			args: []interface{}{
				map[string]interface{}{"var": "amount"},
				"5",
			},
			wantSQL: "FALSE",
		},
		{
			name:     "loose numeric invalid string still folds",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "amount"},
				"abc",
			},
			wantSQL: "FALSE",
		},
		{
			name:     "field null still emits IS NULL",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "amount"},
				nil,
			},
			wantSQL: "amount IS NULL",
		},
		{
			name:     "field literal coercion still applies",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "code"},
				5,
			},
			wantSQL: "code = '5'",
		},
		{
			name:     "strict field field mismatch uses null-only equality",
			operator: "===",
			args: []interface{}{
				map[string]interface{}{"var": "amount"},
				map[string]interface{}{"var": "code"},
			},
			wantSQL: "(amount IS NULL AND code IS NULL)",
		},
		{
			name:     "loose field field mismatch errors",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "amount"},
				map[string]interface{}{"var": "code"},
			},
			wantErr: "loose equality between number field",
		},
		{
			name:     "loose field field inequality mismatch errors",
			operator: "!=",
			args: []interface{}{
				map[string]interface{}{"var": "amount"},
				map[string]interface{}{"var": "code"},
			},
			wantErr: "loose equality between number field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := op.ToSQL(tt.operator, tt.args)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ToSQL() = %q, want error containing %q", got, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ToSQL() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ToSQL() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Fatalf("ToSQL() = %q, want %q", got, tt.wantSQL)
			}
		})
	}
}

func TestComparisonOperator_ToSQL_EqualityConstantFoldsValidateSchema(t *testing.T) {
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
			got, err := op.ToSQL(tt.operator, tt.args)
			if err == nil {
				t.Fatalf("ToSQL() = %q, expected schema validation error", got)
			}
			if !strings.Contains(err.Error(), "contains quote characters") {
				t.Fatalf("ToSQL() error = %v, want schema validation error", err)
			}
		})
	}
}
