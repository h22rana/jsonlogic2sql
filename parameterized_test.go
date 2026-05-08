package jsonlogic2sql

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func transpileInlineExpression(tp *Transpiler, logic string) (string, error) {
	sql, err := tp.TranspileCondition(logic)
	if IsErrorCode(err, ErrInvalidExpressionContext) {
		return tp.TranspileValue(logic)
	}
	return sql, err
}

func transpileParameterizedExpression(tp *Transpiler, logic string) (string, []QueryParam, error) {
	sql, params, err := tp.TranspileParameterizedCondition(logic)
	if IsErrorCode(err, ErrInvalidExpressionContext) {
		return tp.TranspileParameterizedValue(logic)
	}
	return sql, params, err
}

func TestTranspileParameterized_Comparison(t *testing.T) {
	tests := []struct {
		name       string
		dialect    Dialect
		jsonLogic  string
		wantSQL    string
		wantParams []QueryParam
	}{
		{
			name:       "equal bigquery",
			dialect:    DialectBigQuery,
			jsonLogic:  `{"==": [{"var": "email"}, "alice"]}`,
			wantSQL:    "email = @p1",
			wantParams: []QueryParam{{Name: "p1", Value: "alice"}},
		},
		{
			name:       "equal postgresql",
			dialect:    DialectPostgreSQL,
			jsonLogic:  `{"==": [{"var": "email"}, "alice"]}`,
			wantSQL:    "email = $1",
			wantParams: []QueryParam{{Name: "p1", Value: "alice"}},
		},
		{
			name:       "equal clickhouse",
			dialect:    DialectClickHouse,
			jsonLogic:  `{"==": [{"var": "email"}, "alice"]}`,
			wantSQL:    "email = @p1",
			wantParams: []QueryParam{{Name: "p1", Value: "alice"}},
		},
		{
			name:       "not equal",
			dialect:    DialectBigQuery,
			jsonLogic:  `{"!=": [{"var": "status"}, "inactive"]}`,
			wantSQL:    "status != @p1",
			wantParams: []QueryParam{{Name: "p1", Value: "inactive"}},
		},
		{
			name:       "greater than number",
			dialect:    DialectBigQuery,
			jsonLogic:  `{">": [{"var": "age"}, 18]}`,
			wantSQL:    "age > @p1",
			wantParams: []QueryParam{{Name: "p1", Value: float64(18)}},
		},
		{
			name:       "null comparison not parameterized",
			dialect:    DialectBigQuery,
			jsonLogic:  `{"==": [{"var": "name"}, null]}`,
			wantSQL:    "name IS NULL",
			wantParams: []QueryParam{},
		},
		{
			name:       "in operator with array",
			dialect:    DialectBigQuery,
			jsonLogic:  `{"in": [{"var": "x"}, [1, 2, 3]]}`,
			wantSQL:    "x IN (@p1, @p2, @p3)",
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: float64(2)}, {Name: "p3", Value: float64(3)}},
		},
		{
			name:       "in operator postgresql",
			dialect:    DialectPostgreSQL,
			jsonLogic:  `{"in": [{"var": "x"}, [1, 2]]}`,
			wantSQL:    "x IN ($1, $2)",
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: float64(2)}},
		},
		{
			name:       "chained comparison",
			dialect:    DialectBigQuery,
			jsonLogic:  `{"<": [1, {"var": "x"}, 10]}`,
			wantSQL:    "(@p1 < x AND x < @p2)",
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: float64(10)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := NewTranspiler(tt.dialect)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			gotSQL, gotParams, err := transpileParameterizedExpression(tp, tt.jsonLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", gotSQL, tt.wantSQL)
			}
			assertParams(t, gotParams, tt.wantParams)
		})
	}
}

func TestTranspileParameterized_EqualityConstantFoldsDoNotConsumeParams(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	tp.SetSchema(mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeInteger},
		{Name: "active", Type: FieldTypeBoolean},
		{Name: "code", Type: FieldTypeString},
		{Name: "name", Type: FieldTypeString},
	}))

	logic := `{"and":[{"!=":[{"var":"amount"},"abc"]},{"==":[{"var":"code"},5]},{"==":[{"var":"active"},"2"]},{"==":[{"var":"name"},"bob"]}]}`
	gotSQL, gotParams, err := tp.TranspileParameterizedCondition(logic)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}

	wantSQL := "(TRUE AND code = @p1 AND FALSE AND name = @p2)"
	if gotSQL != wantSQL {
		t.Fatalf("SQL = %q, want %q", gotSQL, wantSQL)
	}
	assertParams(t, gotParams, []QueryParam{
		{Name: "p1", Value: "5"},
		{Name: "p2", Value: "bob"},
	})
}

func TestTranspile_EqualitySemanticsAcrossDialects(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeInteger},
		{Name: "active", Type: FieldTypeBoolean},
		{Name: "code", Type: FieldTypeString},
	})
	logic := `{"and":[{"==":[{"var":"amount"},"010"]},{"==":[{"var":"active"},"0"]},{"==":[{"var":"code"},5]},{"==":[{"var":"code"},1e-7]},{"==":[{"var":"code"},9223372036854775808]},{"===":[{"var":"amount"},"5"]}]}`

	tests := []struct {
		dialect      Dialect
		wantParamSQL string
	}{
		{
			dialect:      DialectBigQuery,
			wantParamSQL: "(amount = @p1 AND active = FALSE AND code = @p2 AND code = @p3 AND code = @p4 AND FALSE)",
		},
		{
			dialect:      DialectSpanner,
			wantParamSQL: "(amount = @p1 AND active = FALSE AND code = @p2 AND code = @p3 AND code = @p4 AND FALSE)",
		},
		{
			dialect:      DialectPostgreSQL,
			wantParamSQL: "(amount = $1 AND active = FALSE AND code = $2 AND code = $3 AND code = $4 AND FALSE)",
		},
		{
			dialect:      DialectDuckDB,
			wantParamSQL: "(amount = $1 AND active = FALSE AND code = $2 AND code = $3 AND code = $4 AND FALSE)",
		},
		{
			dialect:      DialectClickHouse,
			wantParamSQL: "(amount = @p1 AND active = FALSE AND code = @p2 AND code = @p3 AND code = @p4 AND FALSE)",
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.dialect), func(t *testing.T) {
			tp, err := NewTranspiler(tt.dialect)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			tp.SetSchema(schema)

			gotSQL, err := tp.TranspileCondition(logic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			wantSQL := "(amount = 10 AND active = FALSE AND code = '5' AND code = '1e-7' AND code = '9223372036854776000' AND FALSE)"
			if gotSQL != wantSQL {
				t.Fatalf("TranspileCondition() SQL = %q, want %q", gotSQL, wantSQL)
			}

			gotParamSQL, gotParams, err := tp.TranspileParameterizedCondition(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotParamSQL != tt.wantParamSQL {
				t.Fatalf("TranspileParameterizedCondition() SQL = %q, want %q", gotParamSQL, tt.wantParamSQL)
			}
			assertParams(t, gotParams, []QueryParam{
				{Name: "p1", Value: int64(10)},
				{Name: "p2", Value: "5"},
				{Name: "p3", Value: "1e-7"},
				{Name: "p4", Value: "9223372036854776000"},
			})
		})
	}
}

func TestTranspile_DefaultedVarEqualityWithSchema(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	tp.SetSchema(mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeInteger},
	}))

	logic := `{"==":[{"var":["amount","abc"]},"abc"]}`
	gotSQL, err := tp.TranspileCondition(logic)
	if err != nil {
		t.Fatalf("TranspileCondition() error = %v", err)
	}
	if wantSQL := "COALESCE(amount, 'abc') = 'abc'"; gotSQL != wantSQL {
		t.Fatalf("TranspileCondition() SQL = %q, want %q", gotSQL, wantSQL)
	}

	gotParamSQL, gotParams, err := tp.TranspileParameterizedCondition(logic)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if wantSQL := "COALESCE(amount, @p1) = @p2"; gotParamSQL != wantSQL {
		t.Fatalf("TranspileParameterizedCondition() SQL = %q, want %q", gotParamSQL, wantSQL)
	}
	assertParams(t, gotParams, []QueryParam{
		{Name: "p1", Value: "abc"},
		{Name: "p2", Value: "abc"},
	})
}

func TestTranspile_EqualityBoundaryAndNestedSemanticsAcrossDialects(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeInteger},
	})

	tests := []struct {
		name      string
		jsonLogic string
		wantSQL   string
	}{
		{
			name:      "out of range integer equality folds false",
			jsonLogic: `{"==":[{"var":"amount"},9223372036854775808]}`,
			wantSQL:   "FALSE",
		},
		{
			name:      "out of range integer inequality folds true",
			jsonLogic: `{"!=":[{"var":"amount"},9223372036854775808]}`,
			wantSQL:   "TRUE",
		},
		{
			name:      "nested invalid numeric equality folds before outer equality",
			jsonLogic: `{"==":[{"==":[{"var":"amount"},"abc"]},false]}`,
			wantSQL:   "TRUE",
		},
		{
			name:      "nested strict mismatch folds before outer equality",
			jsonLogic: `{"==":[{"===":[{"var":"amount"},"5"]},false]}`,
			wantSQL:   "TRUE",
		},
	}

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	for _, tt := range tests {
		for _, d := range dialects {
			t.Run(fmt.Sprintf("%s_%v", tt.name, d), func(t *testing.T) {
				tp, err := NewTranspiler(d)
				if err != nil {
					t.Fatalf("NewTranspiler() error = %v", err)
				}
				tp.SetSchema(schema)

				gotSQL, err := tp.TranspileCondition(tt.jsonLogic)
				if err != nil {
					t.Fatalf("TranspileCondition() error = %v", err)
				}
				if gotSQL != tt.wantSQL {
					t.Fatalf("TranspileCondition() SQL = %q, want %q", gotSQL, tt.wantSQL)
				}

				gotParamSQL, gotParams, err := tp.TranspileParameterizedCondition(tt.jsonLogic)
				if err != nil {
					t.Fatalf("TranspileParameterizedCondition() error = %v", err)
				}
				if gotParamSQL != tt.wantSQL {
					t.Fatalf("TranspileParameterizedCondition() SQL = %q, want %q", gotParamSQL, tt.wantSQL)
				}
				assertParams(t, gotParams, nil)
			})
		}
	}
}

func TestTranspileParameterized_Logical(t *testing.T) {
	tests := []struct {
		name       string
		jsonLogic  string
		wantSQL    string
		wantParams []QueryParam
	}{
		{
			name:       "and",
			jsonLogic:  `{"and": [{"==": [{"var": "x"}, 1]}, {"==": [{"var": "y"}, 2]}]}`,
			wantSQL:    "(x = @p1 AND y = @p2)",
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: float64(2)}},
		},
		{
			name:       "or",
			jsonLogic:  `{"or": [{"==": [{"var": "x"}, "a"]}, {"==": [{"var": "y"}, "b"]}]}`,
			wantSQL:    "(x = @p1 OR y = @p2)",
			wantParams: []QueryParam{{Name: "p1", Value: "a"}, {Name: "p2", Value: "b"}},
		},
		{
			name:       "not",
			jsonLogic:  `{"!": [{"==": [{"var": "x"}, "off"]}]}`,
			wantSQL:    "NOT (x = @p1)",
			wantParams: []QueryParam{{Name: "p1", Value: "off"}},
		},
		{
			name:       "if",
			jsonLogic:  `{"if": [{"==": [{"var": "x"}, 1]}, {"var": "a"}, {"var": "b"}]}`,
			wantSQL:    "CASE WHEN x = @p1 THEN a ELSE b END",
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := NewTranspiler(DialectBigQuery)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			gotSQL, gotParams, err := transpileParameterizedExpression(tp, tt.jsonLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", gotSQL, tt.wantSQL)
			}
			assertParams(t, gotParams, tt.wantParams)
		})
	}
}

func TestTranspileParameterized_Numeric(t *testing.T) {
	tests := []struct {
		name       string
		jsonLogic  string
		wantSQL    string
		wantParams []QueryParam
	}{
		{
			name:       "addition",
			jsonLogic:  `{"+": [{"var": "x"}, 5]}`,
			wantSQL:    "(x + @p1)",
			wantParams: []QueryParam{{Name: "p1", Value: float64(5)}},
		},
		{
			name:       "subtraction",
			jsonLogic:  `{"-": [{"var": "x"}, 3]}`,
			wantSQL:    "(x - @p1)",
			wantParams: []QueryParam{{Name: "p1", Value: float64(3)}},
		},
		{
			name:       "max",
			jsonLogic:  `{"max": [{"var": "x"}, 100]}`,
			wantSQL:    "GREATEST(x, @p1)",
			wantParams: []QueryParam{{Name: "p1", Value: float64(100)}},
		},
		{
			name:      "large integer string preserved as string",
			jsonLogic: `{"*": ["9223372036854775808", 2]}`,
			wantSQL:   "(@p1 * @p2)",
			wantParams: []QueryParam{
				{Name: "p1", Value: "9223372036854775808"},
				{Name: "p2", Value: float64(2)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := NewTranspiler(DialectBigQuery)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			gotSQL, gotParams, err := transpileParameterizedExpression(tp, tt.jsonLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", gotSQL, tt.wantSQL)
			}
			assertParams(t, gotParams, tt.wantParams)
		})
	}
}

func TestTranspileParameterized_String(t *testing.T) {
	tests := []struct {
		name       string
		jsonLogic  string
		wantSQL    string
		wantParams []QueryParam
	}{
		{
			name:       "cat",
			jsonLogic:  `{"cat": [{"var": "first"}, " ", {"var": "last"}]}`,
			wantSQL:    "CONCAT(first, @p1, last)",
			wantParams: []QueryParam{{Name: "p1", Value: " "}},
		},
		{
			name:       "substr",
			jsonLogic:  `{"substr": [{"var": "name"}, 0, 3]}`,
			wantSQL:    "SUBSTR(name, (@p1 + 1), @p2)",
			wantParams: []QueryParam{{Name: "p1", Value: float64(0)}, {Name: "p2", Value: float64(3)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := NewTranspiler(DialectBigQuery)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			gotSQL, gotParams, err := transpileParameterizedExpression(tp, tt.jsonLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", gotSQL, tt.wantSQL)
			}
			assertParams(t, gotParams, tt.wantParams)
		})
	}
}

func TestTranspile_StringNestedEqualitySemanticsWithSchema(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeInteger},
	})
	logic := `{"cat":[{"==":[{"var":"amount"},"abc"]}]}`
	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	for _, d := range dialects {
		t.Run(fmt.Sprintf("%v", d), func(t *testing.T) {
			tp, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			tp.SetSchema(schema)

			gotSQL, err := transpileInlineExpression(tp, logic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			if wantSQL := "CONCAT(FALSE)"; gotSQL != wantSQL {
				t.Fatalf("TranspileCondition() SQL = %q, want %q", gotSQL, wantSQL)
			}

			gotParamSQL, gotParams, err := transpileParameterizedExpression(tp, logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if wantSQL := "CONCAT(FALSE)"; gotParamSQL != wantSQL {
				t.Fatalf("TranspileParameterizedCondition() SQL = %q, want %q", gotParamSQL, wantSQL)
			}
			assertParams(t, gotParams, nil)
		})
	}
}

func TestTranspile_StringNestedEqualityNoSchema(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	logic := `{"cat":[{"==":[{"var":"amount"},"abc"]}]}`

	gotSQL, err := transpileInlineExpression(tp, logic)
	if err != nil {
		t.Fatalf("TranspileCondition() error = %v", err)
	}
	if wantSQL := "CONCAT((amount = 'abc'))"; gotSQL != wantSQL {
		t.Fatalf("TranspileCondition() SQL = %q, want %q", gotSQL, wantSQL)
	}

	gotParamSQL, gotParams, err := transpileParameterizedExpression(tp, logic)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if wantSQL := "CONCAT((amount = @p1))"; gotParamSQL != wantSQL {
		t.Fatalf("TranspileParameterizedCondition() SQL = %q, want %q", gotParamSQL, wantSQL)
	}
	assertParams(t, gotParams, []QueryParam{{Name: "p1", Value: "abc"}})
}

func TestTranspile_StringNestedComparisonArithmeticPrecedence(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	logic := `{"cat":[{"+":[{"==":[{"var":"x"},1]},1]}]}`

	gotSQL, err := transpileInlineExpression(tp, logic)
	if err != nil {
		t.Fatalf("TranspileCondition() error = %v", err)
	}
	if wantSQL := "CONCAT(((x = 1) + 1))"; gotSQL != wantSQL {
		t.Fatalf("TranspileCondition() SQL = %q, want %q", gotSQL, wantSQL)
	}

	gotParamSQL, gotParams, err := transpileParameterizedExpression(tp, logic)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if wantSQL := "CONCAT(((x = @p1) + @p2))"; gotParamSQL != wantSQL {
		t.Fatalf("TranspileParameterizedCondition() SQL = %q, want %q", gotParamSQL, wantSQL)
	}
	assertParams(t, gotParams, []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: float64(1)}})

	nestedOperandLogic := `{"cat":[{"+":[{"==":[{"+":[{"var":"a"},1]},{"*":[{"var":"b"},2]}]},1]}]}`

	gotSQL, err = transpileInlineExpression(tp, nestedOperandLogic)
	if err != nil {
		t.Fatalf("TranspileCondition() nested operands error = %v", err)
	}
	if wantSQL := "CONCAT((((a + 1) = (b * 2)) + 1))"; gotSQL != wantSQL {
		t.Fatalf("TranspileCondition() nested operands SQL = %q, want %q", gotSQL, wantSQL)
	}

	gotParamSQL, gotParams, err = transpileParameterizedExpression(tp, nestedOperandLogic)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() nested operands error = %v", err)
	}
	if wantSQL := "CONCAT((((a + @p1) = (b * @p2)) + @p3))"; gotParamSQL != wantSQL {
		t.Fatalf("TranspileParameterizedCondition() nested operands SQL = %q, want %q", gotParamSQL, wantSQL)
	}
	assertParams(t, gotParams, []QueryParam{
		{Name: "p1", Value: float64(1)},
		{Name: "p2", Value: float64(2)},
		{Name: "p3", Value: float64(1)},
	})
}

func TestTranspileParameterized_Data(t *testing.T) {
	tests := []struct {
		name       string
		jsonLogic  string
		wantSQL    string
		wantParams []QueryParam
	}{
		{
			name:       "var with default",
			jsonLogic:  `{"==": [{"var": ["name", "unknown"]}, "test"]}`,
			wantSQL:    "COALESCE(name, @p1) = @p2",
			wantParams: []QueryParam{{Name: "p1", Value: "unknown"}, {Name: "p2", Value: "test"}},
		},
		{
			name:       "missing - no params needed",
			jsonLogic:  `{"missing": "name"}`,
			wantSQL:    "name IS NULL",
			wantParams: []QueryParam{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := NewTranspiler(DialectBigQuery)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			gotSQL, gotParams, err := tp.TranspileParameterizedCondition(tt.jsonLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", gotSQL, tt.wantSQL)
			}
			assertParams(t, gotParams, tt.wantParams)
		})
	}
}

func TestTranspileParameterizedCondition(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	sql, params, err := tp.TranspileParameterizedCondition(`{"==": [{"var": "x"}, 42]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if sql != "x = @p1" {
		t.Errorf("SQL = %q, want %q", sql, "x = @p1")
	}
	if len(params) != 1 || params[0].Value != float64(42) {
		t.Errorf("params = %v, want [{p1, 42}]", params)
	}
}

func TestTranspileParameterized_AllDialects(t *testing.T) {
	dialects := []struct {
		name       Dialect
		wantPrefix string
	}{
		{DialectBigQuery, "@p"},
		{DialectSpanner, "@p"},
		{DialectClickHouse, "@p"},
		{DialectPostgreSQL, "$"},
		{DialectDuckDB, "$"},
	}

	jsonLogic := `{"==": [{"var": "x"}, "val"]}`

	for _, d := range dialects {
		t.Run(d.name.String(), func(t *testing.T) {
			tp, err := NewTranspiler(d.name)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			sql, params, err := tp.TranspileParameterizedCondition(jsonLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if len(params) != 1 {
				t.Fatalf("expected 1 param, got %d", len(params))
			}
			if params[0].Value != "val" {
				t.Errorf("param value = %v, want %q", params[0].Value, "val")
			}
			_ = sql // SQL format differs by dialect, checked specifically in other tests
		})
	}
}

func TestTranspileParameterized_SchemaCoercion(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "price", Type: FieldTypeInteger},
		{Name: "name", Type: FieldTypeString},
	})

	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	tp.SetSchema(schema)

	// String "50" should be coerced to int64 50 for numeric field
	sql, params, err := tp.TranspileParameterizedCondition(`{">=": [{"var": "price"}, "50"]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if sql != "price >= @p1" {
		t.Errorf("SQL = %q, want %q", sql, "price >= @p1")
	}
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	// After coercion, "50" should become int64(50)
	if v, ok := params[0].Value.(int64); !ok || v != 50 {
		t.Errorf("param value = %v (%T), want int64(50)", params[0].Value, params[0].Value)
	}
}

func TestTranspileParameterized_NoParamsForPureVarExpressions(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	sql, params, err := tp.TranspileParameterizedCondition(`{"==": [{"var": "x"}, {"var": "y"}]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if sql != "x = y" {
		t.Errorf("SQL = %q, want %q", sql, "x = y")
	}
	if len(params) != 0 {
		t.Errorf("expected 0 params for var-to-var comparison, got %d", len(params))
	}
}

func TestTranspileParameterized_NullSafeFieldEquality(t *testing.T) {
	tp, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect:               DialectBigQuery,
		NullSafeFieldEquality: true,
	})
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() error = %v", err)
	}

	sql, params, err := tp.TranspileParameterizedCondition(`{"==": [{"var": "x"}, {"var": "y"}]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if want := "((x IS NULL AND y IS NULL) OR (x IS NOT NULL AND y IS NOT NULL AND x = y))"; sql != want {
		t.Fatalf("TranspileParameterizedCondition() SQL = %q, want %q", sql, want)
	}
	assertParams(t, params, nil)

	sql, params, err = tp.TranspileParameterizedCondition(`{"==": [{"var": ["x", "left"]}, {"var": ["y", "right"]}]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() defaulted vars error = %v", err)
	}
	if want := "((COALESCE(x, @p1) IS NULL AND COALESCE(y, @p2) IS NULL) OR (COALESCE(x, @p1) IS NOT NULL AND COALESCE(y, @p2) IS NOT NULL AND COALESCE(x, @p1) = COALESCE(y, @p2)))"; sql != want {
		t.Fatalf("TranspileParameterizedCondition() defaulted SQL = %q, want %q", sql, want)
	}
	assertParams(t, params, []QueryParam{
		{Name: "p1", Value: "left"},
		{Name: "p2", Value: "right"},
	})
}

func TestTranspileParameterized_CustomOperator(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	// Register a custom "length" operator that wraps its argument
	err = tp.RegisterOperatorFunc("length", func(op string, args []interface{}) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("length requires exactly 1 argument")
		}
		return fmt.Sprintf("LENGTH(%s)", args[0]), nil
	})
	if err != nil {
		t.Fatalf("RegisterOperatorFunc() error = %v", err)
	}

	sql, params, err := tp.TranspileParameterizedCondition(`{">": [{"length": [{"var": "name"}]}, 5]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if sql != "LENGTH(name) > @p1" {
		t.Errorf("SQL = %q, want %q", sql, "LENGTH(name) > @p1")
	}
	if len(params) != 1 || params[0].Value != float64(5) {
		t.Errorf("params = %v, want [{p1, 5}]", params)
	}
}

func TestTranspileParameterized_CustomOperator_OutOfRangeFloatsPreserved(t *testing.T) {
	tests := []struct {
		name      string
		jsonLogic string
		wantValue string
	}{
		{
			name:      "overflow float 1e309 preserved as string",
			jsonLogic: `{"id": [1e309]}`,
			wantValue: "1e309",
		},
		{
			name:      "underflow float 1e-400 preserved as string",
			jsonLogic: `{"id": [1e-400]}`,
			wantValue: "1e-400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := NewTranspiler(DialectBigQuery)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			err = tp.RegisterOperatorFunc("id", func(op string, args []interface{}) (string, error) {
				if len(args) != 1 {
					return "", fmt.Errorf("id requires exactly 1 argument")
				}
				return fmt.Sprintf("%s", args[0]), nil
			})
			if err != nil {
				t.Fatalf("RegisterOperatorFunc() error = %v", err)
			}

			sql, params, err := tp.TranspileParameterizedValue(tt.jsonLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if sql != "@p1" {
				t.Errorf("SQL = %q, want %q", sql, "@p1")
			}
			if len(params) != 1 {
				t.Fatalf("expected 1 param, got %d", len(params))
			}
			str, ok := params[0].Value.(string)
			if !ok {
				t.Fatalf("param[0].Value type = %T, want string", params[0].Value)
			}
			if str != tt.wantValue {
				t.Errorf("param[0].Value = %q, want %q", str, tt.wantValue)
			}
		})
	}
}

func TestTranspileParameterized_DeepNesting(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	jsonLogic := `{"and": [{"==": [{"var": "a"}, "x"]}, {"or": [{"==": [{"var": "b"}, "y"]}, {">": [{"var": "c"}, 10]}]}]}`
	sql, params, err := tp.TranspileParameterizedCondition(jsonLogic)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if sql != "(a = @p1 AND (b = @p2 OR c > @p3))" {
		t.Errorf("SQL = %q, want %q", sql, "(a = @p1 AND (b = @p2 OR c > @p3))")
	}
	if len(params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(params))
	}
	if params[0].Value != "x" {
		t.Errorf("param 0 = %v, want x", params[0].Value)
	}
	if params[1].Value != "y" {
		t.Errorf("param 1 = %v, want y", params[1].Value)
	}
	if params[2].Value != float64(10) {
		t.Errorf("param 2 = %v, want 10", params[2].Value)
	}
}

func TestTranspileParameterized_FromMap(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	logic := map[string]interface{}{
		"==": []interface{}{
			map[string]interface{}{"var": "email"},
			"alice",
		},
	}

	sql, params, err := tp.TranspileParameterizedConditionFromMap(logic)
	if err != nil {
		t.Fatalf("TranspileParameterizedConditionFromMap() error = %v", err)
	}
	if sql != "email = @p1" {
		t.Errorf("SQL = %q, want %q", sql, "email = @p1")
	}
	if len(params) != 1 || params[0].Value != "alice" {
		t.Errorf("params = %v, want [{p1, alice}]", params)
	}
}

func TestTranspileParameterized_FromInterface(t *testing.T) {
	tp, err := NewTranspiler(DialectPostgreSQL)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	var logic interface{} = map[string]interface{}{
		"==": []interface{}{
			map[string]interface{}{"var": "email"},
			"bob",
		},
	}

	sql, params, err := tp.TranspileParameterizedConditionFromInterface(logic)
	if err != nil {
		t.Fatalf("TranspileParameterizedConditionFromInterface() error = %v", err)
	}
	if sql != "email = $1" {
		t.Errorf("SQL = %q, want %q", sql, "email = $1")
	}
	if len(params) != 1 || params[0].Value != "bob" {
		t.Errorf("params = %v, want [{p1, bob}]", params)
	}
}

func TestTranspileParameterized_PackageFunctions(t *testing.T) {
	sql, params, err := TranspileParameterizedCondition(DialectBigQuery, `{"==": [{"var": "x"}, "test"]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if sql != "x = @p1" {
		t.Errorf("SQL = %q, want %q", sql, "x = @p1")
	}
	if len(params) != 1 {
		t.Errorf("expected 1 param, got %d", len(params))
	}

	sql2, params2, err := TranspileParameterizedCondition(DialectPostgreSQL, `{">": [{"var": "age"}, 21]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if sql2 != "age > $1" {
		t.Errorf("SQL = %q, want %q", sql2, "age > $1")
	}
	if len(params2) != 1 {
		t.Errorf("expected 1 param, got %d", len(params2))
	}
}

func TestTranspileParameterized_InvalidJSON(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	_, _, err = tp.TranspileParameterizedCondition(`not json`)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
	if !IsErrorCode(err, ErrInvalidJSON) {
		t.Errorf("expected ErrInvalidJSON, got: %v", err)
	}
}

func TestParameterizedErrorParity(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	tests := []struct {
		name      string
		jsonLogic string
	}{
		{"unsupported operator", `{"foobar": [1, 2]}`},
		{"empty object", `{}`},
		{"primitive at top level", `42`},
		{"array at top level", `[1, 2]`},
		{"invalid JSON", `{invalid`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errInline := tp.TranspileCondition(tt.jsonLogic)
			_, _, errParam := tp.TranspileParameterizedCondition(tt.jsonLogic)

			if errInline == nil && errParam == nil {
				return // Both succeeded, that's fine
			}
			if (errInline == nil) != (errParam == nil) {
				t.Errorf("error mismatch: inline=%v, param=%v", errInline, errParam)
				return
			}

			// Both errored; verify same error code and type
			var tpErrInline, tpErrParam *TranspileError
			inlineIsTP := errors.As(errInline, &tpErrInline)
			paramIsTP := errors.As(errParam, &tpErrParam)

			if inlineIsTP != paramIsTP {
				t.Errorf("TranspileError type mismatch: inline=%v, param=%v", inlineIsTP, paramIsTP)
				return
			}
			if inlineIsTP && paramIsTP {
				if tpErrInline.Code != tpErrParam.Code {
					t.Errorf("ErrorCode mismatch: inline=%s, param=%s", tpErrInline.Code, tpErrParam.Code)
				}
			}
		})
	}
}

func TestTranspileParameterized_BoolAndNullNotParameterized(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	// Boolean values (TRUE/FALSE) and NULL should NOT produce parameters
	sql, params, err := tp.TranspileParameterizedCondition(`{"==": [{"var": "active"}, true]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if len(params) != 0 {
		t.Errorf("expected 0 params for boolean literal, got %d: %v", len(params), params)
	}
	_ = sql
}

func TestTranspileParameterized_LargeIntegerPrecision(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	sql, params, err := transpileParameterizedExpression(tp, `{"*": ["9223372036854775808", 2]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if sql != "(@p1 * @p2)" {
		t.Errorf("SQL = %q, want %q", sql, "(@p1 * @p2)")
	}
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(params))
	}
	str, ok := params[0].Value.(string)
	if !ok {
		t.Fatalf("param[0].Value type = %T, want string", params[0].Value)
	}
	if str != "9223372036854775808" {
		t.Errorf("param[0].Value = %s, want 9223372036854775808", str)
	}
	if params[1].Value != float64(2) {
		t.Errorf("param[1].Value = %v (%T), want float64(2)", params[1].Value, params[1].Value)
	}
}

func TestTranspileParameterized_UnquotedLargeIntegerPrecision(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	sql, params, err := tp.TranspileParameterizedCondition(`{">=": [{"var": "amount"}, 9223372036854775809]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if sql != "amount >= @p1" {
		t.Errorf("SQL = %q, want %q", sql, "amount >= @p1")
	}
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	str, ok := params[0].Value.(string)
	if !ok {
		t.Fatalf("param[0].Value type = %T, want string", params[0].Value)
	}
	if str != "9223372036854775809" {
		t.Errorf("param[0].Value = %q, want %q", str, "9223372036854775809")
	}
}

func TestTranspileParameterized_OutOfRangeFloats(t *testing.T) {
	tests := []struct {
		name      string
		jsonLogic string
		wantSQL   string
		wantValue string
	}{
		{
			name:      "overflow float 1e309 preserved as string",
			jsonLogic: `{">=": [{"var": "x"}, 1e309]}`,
			wantSQL:   "x >= @p1",
			wantValue: "1e309",
		},
		{
			name:      "underflow float 1e-400 preserved as string",
			jsonLogic: `{"<=": [{"var": "x"}, 1e-400]}`,
			wantSQL:   "x <= @p1",
			wantValue: "1e-400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := NewTranspiler(DialectBigQuery)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			sql, params, err := tp.TranspileParameterizedCondition(tt.jsonLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", sql, tt.wantSQL)
			}
			if len(params) != 1 {
				t.Fatalf("expected 1 param, got %d", len(params))
			}
			str, ok := params[0].Value.(string)
			if !ok {
				t.Fatalf("param[0].Value type = %T, want string", params[0].Value)
			}
			if str != tt.wantValue {
				t.Errorf("param[0].Value = %q, want %q", str, tt.wantValue)
			}
		})
	}
}

func TestTranspileParameterized_InStringContainment(t *testing.T) {
	tests := []struct {
		name       string
		dialect    Dialect
		jsonLogic  string
		wantSQL    string
		wantParams []QueryParam
	}{
		{
			name:       "string in var without schema (BigQuery)",
			dialect:    DialectBigQuery,
			jsonLogic:  `{"in": ["foo", {"var": "bar"}]}`,
			wantSQL:    "STRPOS(bar, @p1) > 0",
			wantParams: []QueryParam{{Name: "p1", Value: "foo"}},
		},
		{
			name:       "string in var without schema (PostgreSQL)",
			dialect:    DialectPostgreSQL,
			jsonLogic:  `{"in": ["foo", {"var": "bar"}]}`,
			wantSQL:    "POSITION($1 IN bar) > 0",
			wantParams: []QueryParam{{Name: "p1", Value: "foo"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := NewTranspiler(tt.dialect)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			gotSQL, gotParams, err := tp.TranspileParameterizedCondition(tt.jsonLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", gotSQL, tt.wantSQL)
			}
			assertParams(t, gotParams, tt.wantParams)
		})
	}
}

func TestTranspileParameterized_InStringExpressionContainment_NoSchema(t *testing.T) {
	tests := []struct {
		name       string
		dialect    Dialect
		jsonLogic  string
		wantSQL    string
		wantParams []QueryParam
	}{
		{
			name:      "bigquery",
			dialect:   DialectBigQuery,
			jsonLogic: `{"in": [{"cat": [{"substr": [{"var": "profile.first"}, 0, 2]}, "-x"]}, {"var": "profile.name"}]}`,
			wantSQL:   "STRPOS(profile.name, CONCAT(SUBSTR(profile.first, (@p1 + 1), @p2), @p3)) > 0",
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: "-x"},
			},
		},
		{
			name:      "spanner",
			dialect:   DialectSpanner,
			jsonLogic: `{"in": [{"cat": [{"substr": [{"var": "profile.first"}, 0, 2]}, "-x"]}, {"var": "profile.name"}]}`,
			wantSQL:   "STRPOS(profile.name, CONCAT(SUBSTR(profile.first, (@p1 + 1), @p2), @p3)) > 0",
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: "-x"},
			},
		},
		{
			name:      "postgresql",
			dialect:   DialectPostgreSQL,
			jsonLogic: `{"in": [{"cat": [{"substr": [{"var": "profile.first"}, 0, 2]}, "-x"]}, {"var": "profile.name"}]}`,
			wantSQL:   "POSITION(CONCAT(SUBSTR(profile.first, ($1 + 1), $2), $3) IN profile.name) > 0",
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: "-x"},
			},
		},
		{
			name:      "duckdb",
			dialect:   DialectDuckDB,
			jsonLogic: `{"in": [{"cat": [{"substr": [{"var": "profile.first"}, 0, 2]}, "-x"]}, {"var": "profile.name"}]}`,
			wantSQL:   "STRPOS(profile.name, CONCAT(SUBSTR(profile.first, ($1 + 1), $2), $3)) > 0",
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: "-x"},
			},
		},
		{
			name:      "clickhouse",
			dialect:   DialectClickHouse,
			jsonLogic: `{"in": [{"cat": [{"substr": [{"var": "profile.first"}, 0, 2]}, "-x"]}, {"var": "profile.name"}]}`,
			wantSQL:   "position(profile.name, CONCAT(substring(profile.first, (@p1 + 1), @p2), @p3)) > 0",
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: "-x"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := NewTranspiler(tt.dialect)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			gotSQL, gotParams, err := tp.TranspileParameterizedCondition(tt.jsonLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", gotSQL, tt.wantSQL)
			}
			assertParams(t, gotParams, tt.wantParams)
		})
	}
}

func TestTranspileParameterized_InSchemaCoercion(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "name", Type: FieldTypeString},
		{Name: "tags", Type: FieldTypeArray},
	})

	tests := []struct {
		name       string
		jsonLogic  string
		wantSQL    string
		wantParams []QueryParam
	}{
		{
			name:       "numeric coerced to string for string field",
			jsonLogic:  `{"in": [123, {"var": "name"}]}`,
			wantSQL:    "STRPOS(name, @p1) > 0",
			wantParams: []QueryParam{{Name: "p1", Value: "123"}},
		},
		{
			name:       "string in array field uses UNNEST",
			jsonLogic:  `{"in": ["x", {"var": "tags"}]}`,
			wantSQL:    "@p1 IN UNNEST(tags)",
			wantParams: []QueryParam{{Name: "p1", Value: "x"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: DialectBigQuery,
				Schema:  schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}
			gotSQL, gotParams, err := tp.TranspileParameterizedCondition(tt.jsonLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("SQL = %q, want %q", gotSQL, tt.wantSQL)
			}
			assertParams(t, gotParams, tt.wantParams)
		})
	}
}

func TestTranspileParameterized_InCustomOperatorPlaceholder(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	_ = tp.RegisterOperatorFunc("identity", func(_ string, args []any) (string, error) {
		return fmt.Sprintf("%s", args[0]), nil
	})

	gotSQL, gotParams, err := tp.TranspileParameterizedCondition(`{"in": [{"identity": ["hello"]}, {"var": "col"}]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	wantSQL := "STRPOS(col, @p1) > 0"
	if gotSQL != wantSQL {
		t.Errorf("SQL = %q, want %q", gotSQL, wantSQL)
	}
	assertParams(t, gotParams, []QueryParam{{Name: "p1", Value: "hello"}})
}

func TestTranspileParameterized_CustomOperatorQuotedPlaceholderRejected(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	_ = tp.RegisterOperatorFunc("quote", func(_ string, args []any) (string, error) {
		return fmt.Sprintf("'%s'", args[0]), nil
	})

	_, _, err = tp.TranspileParameterizedValue(`{"quote": ["hello"]}`)
	if err == nil {
		t.Fatal("expected error for quoted placeholder in custom operator SQL")
	}
	tpErr, ok := AsTranspileError(err)
	if !ok {
		t.Fatalf("expected TranspileError, got %T (%v)", err, err)
	}
	if tpErr.Code != ErrCustomOperatorFailed {
		t.Fatalf("error code = %q, want %q", tpErr.Code, ErrCustomOperatorFailed)
	}
	if tpErr.Operator != "quote" {
		t.Fatalf("operator = %q, want %q", tpErr.Operator, "quote")
	}
}

func TestTranspileParameterized_CustomOperatorPlaceholderAsExpressionAllowed(t *testing.T) {
	tp, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	_ = tp.RegisterOperatorFunc("prefix", func(_ string, args []any) (string, error) {
		// Valid: placeholder is used as a SQL expression, not inside a quoted literal.
		return fmt.Sprintf("CONCAT('x-', %s)", args[0]), nil
	})

	sql, params, err := tp.TranspileParameterizedValue(`{"prefix": ["hello"]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if sql != "CONCAT('x-', @p1)" {
		t.Fatalf("SQL = %q, want %q", sql, "CONCAT('x-', @p1)")
	}
	assertParams(t, params, []QueryParam{{Name: "p1", Value: "hello"}})
}

func TestTranspileParameterized_CustomOperatorPlaceholderSemantics_AllDialects(t *testing.T) {
	tests := []struct {
		name        string
		dialect     Dialect
		placeholder string
	}{
		{name: "bigquery", dialect: DialectBigQuery, placeholder: "@p1"},
		{name: "spanner", dialect: DialectSpanner, placeholder: "@p1"},
		{name: "clickhouse", dialect: DialectClickHouse, placeholder: "@p1"},
		{name: "postgresql", dialect: DialectPostgreSQL, placeholder: "$1"},
		{name: "duckdb", dialect: DialectDuckDB, placeholder: "$1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tp, err := NewTranspiler(tt.dialect)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			_ = tp.RegisterOperatorFunc("identity", func(_ string, args []any) (string, error) {
				return fmt.Sprintf("%s", args[0]), nil
			})
			_ = tp.RegisterOperatorFunc("quote", func(_ string, args []any) (string, error) {
				return fmt.Sprintf("'%s'", args[0]), nil
			})

			// Valid: placeholder stays as SQL expression and is bound.
			sql, params, err := tp.TranspileParameterizedValue(`{"identity": ["hello"]}`)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition(identity) error = %v", err)
			}
			wantSQL := "" + tt.placeholder
			if sql != wantSQL {
				t.Fatalf("identity SQL = %q, want %q", sql, wantSQL)
			}
			assertParams(t, params, []QueryParam{{Name: "p1", Value: "hello"}})

			// Invalid: quoted placeholder must be rejected.
			_, _, err = tp.TranspileParameterizedValue(`{"quote": ["hello"]}`)
			if err == nil {
				t.Fatal("expected error for quoted placeholder in custom operator SQL")
			}
			tpErr, ok := AsTranspileError(err)
			if !ok {
				t.Fatalf("expected TranspileError, got %T (%v)", err, err)
			}
			if tpErr.Code != ErrCustomOperatorFailed {
				t.Fatalf("error code = %q, want %q", tpErr.Code, ErrCustomOperatorFailed)
			}
			if !strings.Contains(tpErr.Message, tt.placeholder) {
				t.Fatalf("error message = %q, want to contain placeholder %q", tpErr.Message, tt.placeholder)
			}
		})
	}
}

func assertParams(t *testing.T, got, want []QueryParam) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("params count = %d, want %d\ngot:  %v\nwant: %v", len(got), len(want), got, want)
		return
	}
	for i := range got {
		if got[i].Name != want[i].Name {
			t.Errorf("param[%d].Name = %q, want %q", i, got[i].Name, want[i].Name)
		}
		if !reflect.DeepEqual(got[i].Value, want[i].Value) {
			t.Errorf("param[%d].Value = %v (%T), want %v (%T)", i, got[i].Value, got[i].Value, want[i].Value, want[i].Value)
		}
	}
}
