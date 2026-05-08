package jsonlogic2sql

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestTranspileValue_EmptyArrayLiteralAllDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dialect     Dialect
		want        string
		wantErrCode ErrorCode
	}{
		{dialect: DialectBigQuery, want: "[]"},
		{dialect: DialectSpanner, want: "[]"},
		{dialect: DialectPostgreSQL, wantErrCode: ErrInvalidArgument},
		{dialect: DialectDuckDB, want: "[]"},
		{dialect: DialectClickHouse, want: "[]"},
	}

	for _, tt := range tests {
		t.Run(tt.dialect.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(tt.dialect)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			got, err := tr.TranspileValue(`[]`)
			if tt.wantErrCode != "" {
				if !IsErrorCode(err, tt.wantErrCode) {
					t.Fatalf("TranspileValue() error = %v, want %s", err, tt.wantErrCode)
				}
			} else if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			} else if got != tt.want {
				t.Fatalf("TranspileValue() = %q, want %q", got, tt.want)
			}

			got, err = tr.TranspileValueFromInterface([]interface{}{})
			if tt.wantErrCode != "" {
				if !IsErrorCode(err, tt.wantErrCode) {
					t.Fatalf("TranspileValueFromInterface() error = %v, want %s", err, tt.wantErrCode)
				}
			} else if err != nil {
				t.Fatalf("TranspileValueFromInterface() error = %v", err)
			} else if got != tt.want {
				t.Fatalf("TranspileValueFromInterface() = %q, want %q", got, tt.want)
			}

			paramSQL, params, err := tr.TranspileParameterizedValue(`[]`)
			if tt.wantErrCode != "" {
				if !IsErrorCode(err, tt.wantErrCode) {
					t.Fatalf("TranspileParameterizedValue() error = %v, want %s", err, tt.wantErrCode)
				}
			} else if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			} else if paramSQL != tt.want {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", paramSQL, tt.want)
			} else if len(params) != 0 {
				t.Fatalf("TranspileParameterizedValue() params = %#v, want none", params)
			}

			if _, err = tr.TranspileCondition(`[]`); !IsErrorCode(err, ErrValidation) {
				t.Fatalf("TranspileCondition() error = %v, want %s", err, ErrValidation)
			}
		})
	}
}

func TestTranspileValue_ArrayOperatorArrayLiteralElementsAsExpressions(t *testing.T) {
	t.Parallel()

	logic := `{"map":[[{"var":"amount"},5],{"var":""}]}`

	tests := []struct {
		dialect   Dialect
		want      string
		wantParam string
	}{
		{
			dialect:   DialectBigQuery,
			want:      "ARRAY(SELECT elem FROM UNNEST([amount, 5]) AS elem)",
			wantParam: "ARRAY(SELECT elem FROM UNNEST([amount, @p1]) AS elem)",
		},
		{
			dialect:   DialectSpanner,
			want:      "ARRAY(SELECT elem FROM UNNEST([amount, 5]) AS elem)",
			wantParam: "ARRAY(SELECT elem FROM UNNEST([amount, @p1]) AS elem)",
		},
		{
			dialect:   DialectPostgreSQL,
			want:      "ARRAY(SELECT elem FROM UNNEST(ARRAY[amount, 5]) AS elem)",
			wantParam: "ARRAY(SELECT elem FROM UNNEST(ARRAY[amount, $1]) AS elem)",
		},
		{
			dialect:   DialectDuckDB,
			want:      "ARRAY(SELECT elem FROM UNNEST([amount, 5]) AS elem)",
			wantParam: "ARRAY(SELECT elem FROM UNNEST([amount, $1]) AS elem)",
		},
		{
			dialect:   DialectClickHouse,
			want:      "arrayMap(elem -> elem, [amount, 5])",
			wantParam: "arrayMap(elem -> elem, [amount, @p1])",
		},
	}

	for _, tt := range tests {
		t.Run(tt.dialect.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(tt.dialect)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			got, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("TranspileValue() = %q, want %q", got, tt.want)
			}

			paramSQL, params, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			if paramSQL != tt.wantParam {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", paramSQL, tt.wantParam)
			}
			if !reflect.DeepEqual(params, []QueryParam{{Name: "p1", Value: float64(5)}}) {
				t.Fatalf("params = %#v, want p1=5", params)
			}
		})
	}
}

func TestTranspileValue_NestedValueLogicals(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	tests := []struct {
		name  string
		logic string
		want  string
	}{
		{
			name:  "numeric operand uses value fallback",
			logic: `{"+":[{"or":[0,5]},1]}`,
			want:  "(5 + 1)",
		},
		{
			name:  "nested numeric operand uses value fallback recursively",
			logic: `{"+":[{"*":[{"or":[0,5]},2]},1]}`,
			want:  "((5 * 2) + 1)",
		},
		{
			name:  "string operand uses value fallback",
			logic: `{"cat":[{"or":[false,"fallback"]}]}`,
			want:  "CONCAT('fallback')",
		},
		{
			name:  "primitive numeric strings keep numeric coercion",
			logic: `{"+":["42",1]}`,
			want:  "(42 + 1)",
		},
		{
			name:  "if operand branch uses value fallback",
			logic: `{"+":[{"if":[{">":[{"var":"x"},0]},{"or":[0,5]},1]},0]}`,
			want:  "(CASE WHEN x > 0 THEN 5 ELSE 1 END + 0)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tr.TranspileValue(tt.logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("TranspileValue() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTranspileValue_IfConditionsUseTruthiness(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
	})
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  schema,
	})
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() error = %v", err)
	}

	got, err := tr.TranspileValue(`{"if":[{"var":"flag"},"yes","no"]}`)
	if err != nil {
		t.Fatalf("TranspileValue() error = %v", err)
	}
	want := "CASE WHEN flag IS TRUE THEN 'yes' ELSE 'no' END"
	if got != want {
		t.Fatalf("TranspileValue() = %q, want %q", got, want)
	}

	got, err = tr.TranspileValue(`{"+":[{"if":[{"var":"flag"},1,0]},2]}`)
	if err != nil {
		t.Fatalf("TranspileValue() nested numeric if error = %v", err)
	}
	want = "(CASE WHEN flag IS TRUE THEN 1 ELSE 0 END + 2)"
	if got != want {
		t.Fatalf("TranspileValue() nested numeric if = %q, want %q", got, want)
	}

	got, err = tr.TranspileValue(`{"cat":[{"if":[{"var":"flag"},"yes","no"]}]}`)
	if err != nil {
		t.Fatalf("TranspileValue() nested string if error = %v", err)
	}
	want = "CONCAT(CASE WHEN flag IS TRUE THEN 'yes' ELSE 'no' END)"
	if got != want {
		t.Fatalf("TranspileValue() nested string if = %q, want %q", got, want)
	}

	got, err = tr.TranspileValue(`{"cat":[{"if":[{"var":"flag"},true,false]}]}`)
	if err != nil {
		t.Fatalf("TranspileValue() nested boolean if error = %v", err)
	}
	want = "CONCAT(CASE WHEN (CASE WHEN flag IS TRUE THEN TRUE ELSE FALSE END) THEN 'true' ELSE 'false' END)"
	if got != want {
		t.Fatalf("TranspileValue() nested boolean if = %q, want %q", got, want)
	}
}

func TestTranspileValue_IfConstantTestsShortCircuit(t *testing.T) {
	tests := []struct {
		name       string
		logic      string
		wantSQL    string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:    "true test returns then without parsing else",
			logic:   `{"if":[true,"ok",0]}`,
			wantSQL: "'ok'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "ok"}},
		},
		{
			name:    "false test skips invalid then branch",
			logic:   `{"if":[false,{"var":"bad-name"},"ok"]}`,
			wantSQL: "'ok'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "ok"}},
		},
		{
			name:    "dynamic test followed by true test folds to else",
			logic:   `{"if":[{">":[{"var":"x"},0]},"positive",true,"fallback",{"var":"bad-name"}]}`,
			wantSQL: "CASE WHEN x > 0 THEN 'positive' ELSE 'fallback' END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN x > %s THEN %s ELSE %s END",
					testPlaceholder(d, 1), testPlaceholder(d, 2), testPlaceholder(d, 3))
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: "positive"},
				{Name: "p3", Value: "fallback"},
			},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := tr.TranspileValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if got != tt.wantSQL {
						t.Fatalf("TranspileValue() = %q, want %q", got, tt.wantSQL)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, want)
					}
					if !reflect.DeepEqual(gotParams, tt.wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
					}
				})
			}
		})
	}
}

func TestTranspileValue_DynamicLogicalTruthinessWithSchema(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "name", Type: FieldTypeString},
		{Name: "amount", Type: FieldTypeNumber},
	})

	tests := []struct {
		name       string
		logic      string
		wantSQL    string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:    "or returns string field or fallback",
			logic:   `{"or":[{"var":"name"},"fallback"]}`,
			wantSQL: "CASE WHEN (name IS NOT NULL AND name != '') THEN name ELSE 'fallback' END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN (name IS NOT NULL AND name != '') THEN name ELSE %s END", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "fallback"}},
		},
		{
			name:    "and returns numeric fallback or original field",
			logic:   `{"and":[{"var":"amount"},10]}`,
			wantSQL: "CASE WHEN (amount IS NOT NULL AND amount != 0) THEN 10 ELSE amount END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN (amount IS NOT NULL AND amount != 0) THEN %s ELSE amount END", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(10)}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := tr.TranspileValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if got != tt.wantSQL {
						t.Fatalf("TranspileValue() = %q, want %q", got, tt.wantSQL)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedValue() SQL = %q, want %q", gotParam, want)
					}
					if !reflect.DeepEqual(gotParams, tt.wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
					}
				})
			}
		})
	}
}

func TestTranspileValue_CatStringifiesBuiltInPredicate(t *testing.T) {
	logic := `{"cat":[{"==":[{"var":"amount"},10]}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			got, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			want := "CONCAT(CASE WHEN amount = 10 THEN 'true' ELSE 'false' END)"
			if got != want {
				t.Fatalf("TranspileValue() = %q, want %q", got, want)
			}

			gotParam, gotParams, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			wantParam := fmt.Sprintf("CONCAT(CASE WHEN amount = %s THEN 'true' ELSE 'false' END)", testPlaceholder(d, 1))
			if gotParam != wantParam {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, wantParam)
			}
			wantParams := []QueryParam{{Name: "p1", Value: float64(10)}}
			if !reflect.DeepEqual(gotParams, wantParams) {
				t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
			}
		})
	}
}

func TestTranspileValue_ArrayLiteralsUseDialectSyntax(t *testing.T) {
	tests := []struct {
		name      string
		logic     string
		wantSQL   func(Dialect) string
		wantParam func(Dialect) string
		params    []QueryParam
	}{
		{
			name:  "root array literal",
			logic: `[1,2]`,
			wantSQL: func(d Dialect) string {
				if d == DialectPostgreSQL {
					return "ARRAY[1, 2]"
				}
				return "[1, 2]"
			},
			wantParam: func(d Dialect) string {
				if d == DialectPostgreSQL {
					return fmt.Sprintf("ARRAY[%s, %s]", testPlaceholder(d, 1), testPlaceholder(d, 2))
				}
				return fmt.Sprintf("[%s, %s]", testPlaceholder(d, 1), testPlaceholder(d, 2))
			},
			params: []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: float64(2)}},
		},
		{
			name:  "root array evaluates value expression elements",
			logic: `[{"var":"amount"},1,{"==":[{"var":"status"},"ok"]}]`,
			wantSQL: func(d Dialect) string {
				if d == DialectPostgreSQL {
					return "ARRAY[amount, 1, (status = 'ok')]"
				}
				return "[amount, 1, (status = 'ok')]"
			},
			wantParam: func(d Dialect) string {
				if d == DialectPostgreSQL {
					return fmt.Sprintf("ARRAY[amount, %s, (status = %s)]", testPlaceholder(d, 1), testPlaceholder(d, 2))
				}
				return fmt.Sprintf("[amount, %s, (status = %s)]", testPlaceholder(d, 1), testPlaceholder(d, 2))
			},
			params: []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: "ok"}},
		},
		{
			name:  "map source array literal",
			logic: `{"map":[[1,2],{"+":[{"var":"item"},1]}]}`,
			wantSQL: func(d Dialect) string {
				switch d {
				case DialectPostgreSQL:
					return "ARRAY(SELECT (elem + 1) FROM UNNEST(ARRAY[1, 2]) AS elem)"
				case DialectClickHouse:
					return "arrayMap(elem -> (elem + 1), [1, 2])"
				default:
					return "ARRAY(SELECT (elem + 1) FROM UNNEST([1, 2]) AS elem)"
				}
			},
			wantParam: func(d Dialect) string {
				switch d {
				case DialectPostgreSQL:
					return fmt.Sprintf("ARRAY(SELECT (elem + %s) FROM UNNEST(ARRAY[%s, %s]) AS elem)",
						testPlaceholder(d, 3), testPlaceholder(d, 1), testPlaceholder(d, 2))
				case DialectClickHouse:
					return fmt.Sprintf("arrayMap(elem -> (elem + %s), [%s, %s])",
						testPlaceholder(d, 3), testPlaceholder(d, 1), testPlaceholder(d, 2))
				default:
					return fmt.Sprintf("ARRAY(SELECT (elem + %s) FROM UNNEST([%s, %s]) AS elem)",
						testPlaceholder(d, 3), testPlaceholder(d, 1), testPlaceholder(d, 2))
				}
			},
			params: []QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: float64(1)},
			},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := tr.TranspileValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if want := tt.wantSQL(d); got != want {
						t.Fatalf("TranspileValue() = %q, want %q", got, want)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, want)
					}
					if !reflect.DeepEqual(gotParams, tt.params) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.params)
					}
				})
			}
		})
	}
}

func TestTranspileValue_CatStringifiesCustomPredicate(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	err = tr.RegisterOperatorFunc("isPositive", func(_ string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 1 {
			return OperatorResult{}, fmt.Errorf("isPositive requires exactly 1 argument")
		}
		return PredicateSQL(fmt.Sprintf("%s > 0", args[0].SQL)), nil
	})
	if err != nil {
		t.Fatalf("RegisterOperatorFunc() error = %v", err)
	}

	got, err := tr.TranspileValue(`{"cat":[{"isPositive":[{"var":"amount"}]}]}`)
	if err != nil {
		t.Fatalf("TranspileValue() error = %v", err)
	}
	want := "CONCAT(CASE WHEN amount > 0 THEN 'true' ELSE 'false' END)"
	if got != want {
		t.Fatalf("TranspileValue() = %q, want %q", got, want)
	}
}

func TestTranspileValue_LegacyCustomOperatorUsesValueContext(t *testing.T) {
	tr, err := NewTranspiler(DialectPostgreSQL)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	err = tr.RegisterDialectAwareOperatorFunc("safeDivideLegacy", func(_ string, args []any, _ Dialect) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("safeDivideLegacy requires exactly 2 arguments")
		}
		return fmt.Sprintf("CASE WHEN %s = 0 THEN NULL ELSE %s / %s END", args[1], args[0], args[1]), nil
	})
	if err != nil {
		t.Fatalf("RegisterDialectAwareOperatorFunc() error = %v", err)
	}

	logic := `{"cat":[{"safeDivideLegacy":[{"var":"total"},{"var":"count"}]}]}`
	want := "CONCAT(CASE WHEN count = 0 THEN NULL ELSE total / count END)"

	got, err := tr.TranspileValue(logic)
	if err != nil {
		t.Fatalf("TranspileValue() error = %v", err)
	}
	if got != want {
		t.Fatalf("TranspileValue() = %q, want %q", got, want)
	}

	gotParam, params, err := tr.TranspileParameterizedValue(logic)
	if err != nil {
		t.Fatalf("TranspileParameterizedValue() error = %v", err)
	}
	if gotParam != want {
		t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, want)
	}
	if len(params) != 0 {
		t.Fatalf("params = %#v, want none", params)
	}
}

func TestTranspileParameterizedValue_NestedValueLogicalsRollbackSkippedParams(t *testing.T) {
	tests := []struct {
		name       string
		logic      string
		wantSQL    func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "or skips falsy numeric literal before fallback string",
			logic: `{"or":[0,"fallback"]}`,
			wantSQL: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "fallback"}},
		},
		{
			name:  "and skips truthy numeric literal before fallback string",
			logic: `{"and":[1,"x"]}`,
			wantSQL: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "x"}},
		},
		{
			name:  "or keeps returned truthy string literal",
			logic: `{"or":["x","fallback"]}`,
			wantSQL: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "x"}},
		},
		{
			name:  "and keeps returned falsy numeric literal",
			logic: `{"and":[0,"x"]}`,
			wantSQL: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(0)}},
		},
		{
			name:  "nested numeric operand preserves emitted parameter order",
			logic: `{"+":[{"or":[0,5]},1]}`,
			wantSQL: func(d Dialect) string {
				return fmt.Sprintf("(%s + %s)", testPlaceholder(d, 1), testPlaceholder(d, 2))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(5)}, {Name: "p2", Value: float64(1)}},
		},
		{
			name:  "nested numeric operand preserves value fallback recursively",
			logic: `{"+":[{"*":[{"or":[0,5]},2]},1]}`,
			wantSQL: func(d Dialect) string {
				return fmt.Sprintf("((%s * %s) + %s)",
					testPlaceholder(d, 1), testPlaceholder(d, 2), testPlaceholder(d, 3))
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(5)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: float64(1)},
			},
		},
		{
			name:  "nested string operand preserves emitted parameter order",
			logic: `{"cat":[{"or":[false,"fallback"]}]}`,
			wantSQL: func(d Dialect) string {
				return fmt.Sprintf("CONCAT(%s)", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "fallback"}},
		},
		{
			name:  "if operand branch uses value fallback",
			logic: `{"+":[{"if":[{">":[{"var":"x"},0]},{"or":[0,5]},1]},0]}`,
			wantSQL: func(d Dialect) string {
				return fmt.Sprintf("(CASE WHEN x > %s THEN %s ELSE %s END + %s)",
					testPlaceholder(d, 1), testPlaceholder(d, 2), testPlaceholder(d, 3), testPlaceholder(d, 4))
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(5)},
				{Name: "p3", Value: float64(1)},
				{Name: "p4", Value: float64(0)},
			},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					gotSQL, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if gotSQL != tt.wantSQL(d) {
						t.Fatalf("TranspileParameterizedValue() SQL = %q, want %q", gotSQL, tt.wantSQL(d))
					}
					if !reflect.DeepEqual(gotParams, tt.wantParams) {
						t.Fatalf("TranspileParameterizedValue() params = %#v, want %#v", gotParams, tt.wantParams)
					}
				})
			}
		})
	}
}

func TestTranspileParameterizedValue_TruthinessDoesNotLeakSkippedParams(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "status", Type: FieldTypeString},
	})
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  schema,
	})
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() error = %v", err)
	}

	tests := []struct {
		name       string
		logic      string
		wantSQL    string
		wantParams []QueryParam
	}{
		{
			name:       "literal if condition",
			logic:      `{"if":["nonempty","yes","no"]}`,
			wantSQL:    "@p1",
			wantParams: []QueryParam{{Name: "p1", Value: "yes"}},
		},
		{
			name:       "boolean field if condition",
			logic:      `{"if":[{"var":"flag"},"yes","no"]}`,
			wantSQL:    "CASE WHEN flag IS TRUE THEN @p1 ELSE @p2 END",
			wantParams: []QueryParam{{Name: "p1", Value: "yes"}, {Name: "p2", Value: "no"}},
		},
		{
			name:       "nested numeric if condition",
			logic:      `{"+":[{"if":[{"var":"flag"},1,0]},2]}`,
			wantSQL:    "(CASE WHEN flag IS TRUE THEN @p1 ELSE @p2 END + @p3)",
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: float64(0)}, {Name: "p3", Value: float64(2)}},
		},
		{
			name:       "predicate condition with decisive boolean folds skipped placeholders",
			logic:      `{"if":[{"or":[{"==":[{"var":"status"},"active"]},true]},"yes","no"]}`,
			wantSQL:    "@p1",
			wantParams: []QueryParam{{Name: "p1", Value: "yes"}},
		},
		{
			name:       "parameterized not folds literal truthiness without leaked parameter",
			logic:      `{"!":"x"}`,
			wantSQL:    "NOT (TRUE)",
			wantParams: []QueryParam{},
		},
		{
			name:       "nested boolean if in cat stringifies without params",
			logic:      `{"cat":[{"if":[{"var":"flag"},true,false]}]}`,
			wantSQL:    "CONCAT(CASE WHEN (CASE WHEN flag IS TRUE THEN TRUE ELSE FALSE END) THEN 'true' ELSE 'false' END)",
			wantParams: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				gotSQL    string
				gotParams []QueryParam
				err       error
			)
			if tt.name == "parameterized not folds literal truthiness without leaked parameter" {
				gotSQL, gotParams, err = tr.TranspileParameterizedCondition(tt.logic)
			} else {
				gotSQL, gotParams, err = tr.TranspileParameterizedValue(tt.logic)
			}
			if err != nil {
				t.Fatalf("parameterized transpilation error = %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Fatalf("SQL = %q, want %q", gotSQL, tt.wantSQL)
			}
			if !reflect.DeepEqual(gotParams, tt.wantParams) {
				t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
			}
		})
	}
}

func TestTranspileValue_NestedValueLogicalsPreserveSchemaValidation(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeNumber},
		{Name: "name", Type: FieldTypeString},
	})
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  schema,
	})
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() error = %v", err)
	}

	if _, err = tr.TranspileValue(`{"+":[{"var":"name"},1]}`); err == nil {
		t.Fatal("TranspileValue() expected schema error for non-numeric field, got nil")
	}

	got, err := tr.TranspileValue(`{"+":[{"var":"amount"},1]}`)
	if err != nil {
		t.Fatalf("TranspileValue() numeric field error = %v", err)
	}
	if got != "(amount + 1)" {
		t.Fatalf("TranspileValue() = %q, want %q", got, "(amount + 1)")
	}
}

func TestTranspileValue_ArrayTransformationsUseValueSemantics(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	got, err := tr.TranspileValue(`{"map":[{"var":"arr"},{"or":[0,{"var":""}]}]}`)
	if err != nil {
		t.Fatalf("TranspileValue() map error = %v", err)
	}
	if want := "ARRAY(SELECT elem FROM UNNEST(arr) AS elem)"; got != want {
		t.Fatalf("TranspileValue() map = %q, want %q", got, want)
	}

	got, err = tr.TranspileValue(`{"map":[{"var":"arr"},{"+":[{"*":[{"or":[0,{"var":""}]},2]},1]}]}`)
	if err != nil {
		t.Fatalf("TranspileValue() nested map error = %v", err)
	}
	if want := "ARRAY(SELECT ((elem * 2) + 1) FROM UNNEST(arr) AS elem)"; got != want {
		t.Fatalf("TranspileValue() nested map = %q, want %q", got, want)
	}

	got, err = tr.TranspileValue(`{"cat":[{"reduce":[{"var":"arr"},{"cat":[{"var":"accumulator"},{"var":"current"}]},""]}]}`)
	if err != nil {
		t.Fatalf("TranspileValue() cat reduce error = %v", err)
	}
	if want := "CONCAT((SELECT CONCAT('', elem) FROM UNNEST(arr) AS elem))"; got != want {
		t.Fatalf("TranspileValue() cat reduce = %q, want %q", got, want)
	}
}

func TestTranspileParameterizedValue_ArrayTransformationsUseValueSemantics(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	gotSQL, gotParams, err := tr.TranspileParameterizedValue(`{"map":[{"var":"arr"},{"or":[0,{"var":""}]}]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedValue() map error = %v", err)
	}
	if want := "ARRAY(SELECT elem FROM UNNEST(arr) AS elem)"; gotSQL != want {
		t.Fatalf("TranspileParameterizedValue() map SQL = %q, want %q", gotSQL, want)
	}
	if len(gotParams) != 0 {
		t.Fatalf("TranspileParameterizedValue() map params = %#v, want none", gotParams)
	}

	gotSQL, gotParams, err = tr.TranspileParameterizedValue(`{"map":[{"var":"arr"},{"+":[{"*":[{"or":[0,{"var":""}]},2]},1]}]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedValue() nested map error = %v", err)
	}
	if want := "ARRAY(SELECT ((elem * @p1) + @p2) FROM UNNEST(arr) AS elem)"; gotSQL != want {
		t.Fatalf("TranspileParameterizedValue() nested map SQL = %q, want %q", gotSQL, want)
	}
	if wantParams := []QueryParam{{Name: "p1", Value: float64(2)}, {Name: "p2", Value: float64(1)}}; !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("TranspileParameterizedValue() nested map params = %#v, want %#v", gotParams, wantParams)
	}

	gotSQL, gotParams, err = tr.TranspileParameterizedValue(`{"cat":[{"reduce":[{"var":"arr"},{"cat":[{"var":"accumulator"},{"var":"current"}]},""]}]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedValue() cat reduce error = %v", err)
	}
	if want := "CONCAT((SELECT CONCAT(@p1, elem) FROM UNNEST(arr) AS elem))"; gotSQL != want {
		t.Fatalf("TranspileParameterizedValue() cat reduce SQL = %q, want %q", gotSQL, want)
	}
	if wantParams := []QueryParam{{Name: "p1", Value: ""}}; !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("TranspileParameterizedValue() cat reduce params = %#v, want %#v", gotParams, wantParams)
	}
}

func TestTranspileValue_ArrayPredicateContextsRejectValueLogicals(t *testing.T) {
	logic := `{"filter":[{"var":"items"},{"or":[0,{"==":[{"var":"current"},1]}]}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			_, err = tr.TranspileValue(logic)
			if !IsErrorCode(err, ErrInvalidExpressionContext) {
				t.Fatalf("TranspileValue() error = %v, want %s", err, ErrInvalidExpressionContext)
			}

			_, params, err := tr.TranspileParameterizedValue(logic)
			if !IsErrorCode(err, ErrInvalidExpressionContext) {
				t.Fatalf("TranspileParameterizedValue() error = %v, want %s", err, ErrInvalidExpressionContext)
			}
			if len(params) != 0 {
				t.Fatalf("params = %#v, want none", params)
			}
		})
	}
}

func TestTranspileParameterizedValue_ArrayScopedDefaultUsesBindParams(t *testing.T) {
	logic := `{"map":[{"var":"items"},{"if":[{"var":["current","fallback"]},"yes","no"]}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			gotSQL, gotParams, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			if strings.Contains(gotSQL, "'fallback'") {
				t.Fatalf("SQL inlined scoped var default: %s", gotSQL)
			}
			if want := fmt.Sprintf("COALESCE(elem, %s)", testPlaceholder(d, 1)); !strings.Contains(gotSQL, want) {
				t.Fatalf("SQL = %q, want to contain %q", gotSQL, want)
			}
			wantParams := []QueryParam{
				{Name: "p1", Value: "fallback"},
				{Name: "p2", Value: "yes"},
				{Name: "p3", Value: "no"},
			}
			if !reflect.DeepEqual(gotParams, wantParams) {
				t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
			}
		})
	}
}

func TestTranspileParameterizedValue_ArrayScopedDefaultSkippedByValueLogical(t *testing.T) {
	logic := `{"map":[{"var":"items"},{"or":["x",{"var":["current","fallback"]}]}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			gotSQL, gotParams, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			wantValue := testPlaceholder(d, 1)
			wantSQL := fmt.Sprintf("ARRAY(SELECT %s FROM UNNEST(items) AS elem)", wantValue)
			if d == DialectClickHouse {
				wantSQL = fmt.Sprintf("arrayMap(elem -> %s, items)", wantValue)
			}
			if gotSQL != wantSQL {
				t.Fatalf("SQL = %q, want %q", gotSQL, wantSQL)
			}
			wantParams := []QueryParam{{Name: "p1", Value: "x"}}
			if !reflect.DeepEqual(gotParams, wantParams) {
				t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
			}
		})
	}
}

func TestTranspileParameterizedValue_ArrayCustomPredicateKeepsTypeMetadata(t *testing.T) {
	logic := `{"map":[{"var":"items"},{"cat":[{"gt":[{"var":"current"},0]}]}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			err = tr.RegisterOperatorFunc("gt", func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 2 {
					return OperatorResult{}, fmt.Errorf("gt requires exactly 2 arguments")
				}
				return PredicateSQL(fmt.Sprintf("%s > %s", args[0].SQL, args[1].SQL)), nil
			})
			if err != nil {
				t.Fatalf("RegisterOperatorFunc() error = %v", err)
			}

			gotSQL, gotParams, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			if strings.Contains(gotSQL, "CONCAT(elem >") {
				t.Fatalf("SQL did not stringify custom predicate: %s", gotSQL)
			}
			wantPredicate := fmt.Sprintf("CASE WHEN elem > %s THEN 'true' ELSE 'false' END", testPlaceholder(d, 1))
			if !strings.Contains(gotSQL, wantPredicate) {
				t.Fatalf("SQL = %q, want to contain %q", gotSQL, wantPredicate)
			}
			wantParams := []QueryParam{{Name: "p1", Value: float64(0)}}
			if !reflect.DeepEqual(gotParams, wantParams) {
				t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
			}
		})
	}
}

func testPlaceholder(d Dialect, index int) string {
	switch d {
	case DialectPostgreSQL, DialectDuckDB:
		return fmt.Sprintf("$%d", index)
	default:
		return fmt.Sprintf("@p%d", index)
	}
}
