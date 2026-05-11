package jsonlogic2sql

import (
	"fmt"
	"math"
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

func TestTranspileValue_RejectsMalformedVarOperandsAllDialectsSchemaModes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "x", Type: FieldTypeNumber},
		{Name: "items", Type: FieldTypeArray},
	})

	tests := []struct {
		name  string
		logic string
		input interface{}
	}{
		{
			name:  "root defaulted var has too many operands",
			logic: `{"var":["x",1,2]}`,
			input: map[string]interface{}{"var": []interface{}{"x", float64(1), float64(2)}},
		},
		{
			name:  "array-scoped defaulted var has too many operands",
			logic: `{"map":[{"var":"items"},{"var":["current",1,2]}]}`,
			input: map[string]interface{}{
				"map": []interface{}{
					map[string]interface{}{"var": "items"},
					map[string]interface{}{"var": []interface{}{"current", float64(1), float64(2)}},
				},
			},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range allSchemaModes(schema) {
				t.Run(mode.name, func(t *testing.T) {
					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}

					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							if _, err := tr.TranspileValue(tt.logic); !IsErrorCode(err, ErrInvalidArgument) {
								t.Fatalf("TranspileValue() error = %v, want %s", err, ErrInvalidArgument)
							}
							if _, err := tr.TranspileValueFromInterface(tt.input); !IsErrorCode(err, ErrInvalidArgument) {
								t.Fatalf("TranspileValueFromInterface() error = %v, want %s", err, ErrInvalidArgument)
							}

							sql, params, err := tr.TranspileParameterizedValue(tt.logic)
							if !IsErrorCode(err, ErrInvalidArgument) {
								t.Fatalf("TranspileParameterizedValue() error = %v, want %s (SQL %q params %#v)",
									err, ErrInvalidArgument, sql, params)
							}
							if len(params) != 0 {
								t.Fatalf("params = %#v, want none", params)
							}
						})
					}
				})
			}
		})
	}
}

func TestTranspileValue_EmptyArrayFoldableContextsAllDialects(t *testing.T) {
	t.Parallel()

	valueFallbacks := []string{
		`{"or":[[],"fallback"]}`,
		`{"or":[{"map":[[],{"var":"missing"}]},"fallback"]}`,
		`{"or":[{"filter":[[],true]},"fallback"]}`,
		`{"or":[{"merge":[[]]},"fallback"]}`,
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, logic := range valueFallbacks {
				t.Run(logic, func(t *testing.T) {
					got, valueErr := tr.TranspileValue(logic)
					if valueErr != nil {
						t.Fatalf("TranspileValue() error = %v", valueErr)
					}
					if got != "'fallback'" {
						t.Fatalf("TranspileValue() = %q, want %q", got, "'fallback'")
					}

					gotParam, gotParams, paramErr := tr.TranspileParameterizedValue(logic)
					if paramErr != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", paramErr)
					}
					if want := testPlaceholder(d, 1); gotParam != want {
						t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, want)
					}
					if want := []QueryParam{{Name: "p1", Value: "fallback"}}; !reflect.DeepEqual(gotParams, want) {
						t.Fatalf("params = %#v, want %#v", gotParams, want)
					}
				})
			}

			conditionLogic := `{"all":[[],true]}`
			got, err := tr.TranspileCondition(conditionLogic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			if got != "FALSE" {
				t.Fatalf("TranspileCondition() = %q, want %q", got, "FALSE")
			}

			gotParam, gotParams, err := tr.TranspileParameterizedCondition(conditionLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotParam != "FALSE" {
				t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, "FALSE")
			}
			if len(gotParams) != 0 {
				t.Fatalf("params = %#v, want none", gotParams)
			}

			emptyReturningLogic := `{"and":[[],"fallback"]}`
			got, err = tr.TranspileValue(emptyReturningLogic)
			gotParam, gotParams, paramErr := tr.TranspileParameterizedValue(emptyReturningLogic)
			if d == DialectPostgreSQL {
				if !IsErrorCode(err, ErrInvalidArgument) {
					t.Fatalf("TranspileValue() error = %v, want %s", err, ErrInvalidArgument)
				}
				if !IsErrorCode(paramErr, ErrInvalidArgument) {
					t.Fatalf("TranspileParameterizedValue() error = %v, want %s", paramErr, ErrInvalidArgument)
				}
				return
			}
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if got != "[]" {
				t.Fatalf("TranspileValue() = %q, want %q", got, "[]")
			}
			if paramErr != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", paramErr)
			}
			if gotParam != "[]" {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, "[]")
			}
			if len(gotParams) != 0 {
				t.Fatalf("params = %#v, want none", gotParams)
			}
		})
	}
}

func TestTranspileValue_EmptyArrayUnaryAndReduceShortCircuitAllDialectsSchemaModes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeNumber},
	})
	schemaModes := allSchemaModes(schema)

	tests := []struct {
		name       string
		logic      string
		wantSQL    string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:    "double bang empty array folds false",
			logic:   `{"!!":[[]]}`,
			wantSQL: "FALSE",
			wantParam: func(Dialect) string {
				return "FALSE"
			},
		},
		{
			name:    "not empty array folds true",
			logic:   `{"!":[[]]}`,
			wantSQL: "TRUE",
			wantParam: func(Dialect) string {
				return "TRUE"
			},
		},
		{
			name:    "reduce falsy initial skips invalid reducer and falls through or",
			logic:   `{"or":[{"reduce":[[],{"var":"missing"},""]},"fallback"]}`,
			wantSQL: "'fallback'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "fallback"}},
		},
		{
			name:    "reduce truthy initial skips invalid reducer and wins or",
			logic:   `{"or":[{"reduce":[[],{"var":"missing"},"seed"]},"fallback"]}`,
			wantSQL: "'seed'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "seed"}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaModes {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
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
								t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, want)
							}
							if !reflect.DeepEqual(gotParams, tt.wantParams) {
								t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
							}
						})
					}
				})
			}
		})
	}
}

func TestTranspileValue_EmptyArrayEmissionsAllDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		logic          string
		rejectPostgres bool
		wantSQL        func(Dialect) string
		wantParamSQL   func(Dialect) string
		wantParams     []QueryParam
	}{
		{
			name:           "merge all empty",
			logic:          `{"merge":[[]]}`,
			rejectPostgres: true,
			wantSQL: func(Dialect) string {
				return "[]"
			},
			wantParamSQL: func(Dialect) string {
				return "[]"
			},
		},
		{
			name:           "map empty source",
			logic:          `{"map":[[],{"var":""}]}`,
			rejectPostgres: true,
			wantSQL: func(Dialect) string {
				return "[]"
			},
			wantParamSQL: func(Dialect) string {
				return "[]"
			},
		},
		{
			name:           "filter empty source",
			logic:          `{"filter":[[],true]}`,
			rejectPostgres: true,
			wantSQL: func(Dialect) string {
				return "[]"
			},
			wantParamSQL: func(Dialect) string {
				return "[]"
			},
		},
		{
			name:           "nested empty array literal",
			logic:          `[[]]`,
			rejectPostgres: true,
			wantSQL: func(Dialect) string {
				return "[[]]"
			},
			wantParamSQL: func(Dialect) string {
				return "[[]]"
			},
		},
		{
			name:  "merge skips empty identity",
			logic: `{"merge":[[],[1]]}`,
			wantSQL: func(d Dialect) string {
				switch d {
				case DialectPostgreSQL:
					return "ARRAY[1]"
				case DialectClickHouse:
					return "arrayConcat([1])"
				default:
					return "ARRAY_CONCAT([1])"
				}
			},
			wantParamSQL: func(d Dialect) string {
				switch d {
				case DialectPostgreSQL:
					return fmt.Sprintf("ARRAY[%s]", testPlaceholder(d, 1))
				case DialectClickHouse:
					return fmt.Sprintf("arrayConcat([%s])", testPlaceholder(d, 1))
				default:
					return fmt.Sprintf("ARRAY_CONCAT([%s])", testPlaceholder(d, 1))
				}
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
		{
			name:  "reduce empty source returns initial",
			logic: `{"reduce":[[],{"cat":[{"var":"accumulator"},{"var":"current"}]},"init"]}`,
			wantSQL: func(Dialect) string {
				return "'init'"
			},
			wantParamSQL: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "init"}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := tr.TranspileValue(tt.logic)
					gotParam, gotParams, paramErr := tr.TranspileParameterizedValue(tt.logic)
					if d == DialectPostgreSQL && tt.rejectPostgres {
						if !IsErrorCode(err, ErrInvalidArgument) {
							t.Fatalf("TranspileValue() error = %v, want %s", err, ErrInvalidArgument)
						}
						if !IsErrorCode(paramErr, ErrInvalidArgument) {
							t.Fatalf("TranspileParameterizedValue() error = %v, want %s", paramErr, ErrInvalidArgument)
						}
						return
					}
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if want := tt.wantSQL(d); got != want {
						t.Fatalf("TranspileValue() = %q, want %q", got, want)
					}
					if paramErr != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", paramErr)
					}
					if want := tt.wantParamSQL(d); gotParam != want {
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

func TestTranspileValue_ReduceTruthinessUsesInferredTypeAllDialectsSchemaModes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray},
	})

	reduceSumSQL := func(d Dialect, initial string) string {
		if d == DialectClickHouse {
			return fmt.Sprintf("%s + coalesce(arrayReduce('sum', arr), 0)", initial)
		}
		return fmt.Sprintf("%s + COALESCE((SELECT SUM(elem) FROM UNNEST(arr) AS elem), 0)", initial)
	}
	numericTruthiness := func(expr string) string {
		return fmt.Sprintf("(%s IS NOT NULL AND %s != 0)", expr, expr)
	}

	tests := []struct {
		name       string
		logic      string
		wantSQL    func(Dialect) string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "or tests numeric reduce with numeric truthiness",
			logic: `{"or":[{"reduce":[{"var":"arr"},{"+":[{"var":"accumulator"},{"var":"current"}]},0]},5]}`,
			wantSQL: func(d Dialect) string {
				reduce := reduceSumSQL(d, "0")
				return fmt.Sprintf("CASE WHEN %s THEN %s ELSE 5 END", numericTruthiness(reduce), reduce)
			},
			wantParam: func(d Dialect) string {
				reduce := reduceSumSQL(d, testPlaceholder(d, 1))
				return fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", numericTruthiness(reduce), reduce, testPlaceholder(d, 2))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(0)}, {Name: "p2", Value: float64(5)}},
		},
		{
			name:  "and tests numeric reduce with numeric truthiness",
			logic: `{"and":[{"reduce":[{"var":"arr"},{"+":[{"var":"accumulator"},{"var":"current"}]},0]},5]}`,
			wantSQL: func(d Dialect) string {
				reduce := reduceSumSQL(d, "0")
				return fmt.Sprintf("CASE WHEN %s THEN 5 ELSE %s END", numericTruthiness(reduce), reduce)
			},
			wantParam: func(d Dialect) string {
				reduce := reduceSumSQL(d, testPlaceholder(d, 1))
				return fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", numericTruthiness(reduce), testPlaceholder(d, 2), reduce)
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(0)}, {Name: "p2", Value: float64(5)}},
		},
		{
			name:  "if tests numeric reduce with numeric truthiness",
			logic: `{"if":[{"reduce":[{"var":"arr"},{"+":[{"var":"accumulator"},{"var":"current"}]},0]},"nonzero","zero"]}`,
			wantSQL: func(d Dialect) string {
				reduce := reduceSumSQL(d, "0")
				return fmt.Sprintf("CASE WHEN %s THEN 'nonzero' ELSE 'zero' END", numericTruthiness(reduce))
			},
			wantParam: func(d Dialect) string {
				reduce := reduceSumSQL(d, testPlaceholder(d, 1))
				return fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END",
					numericTruthiness(reduce), testPlaceholder(d, 2), testPlaceholder(d, 3))
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: "nonzero"},
				{Name: "p3", Value: "zero"},
			},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range allSchemaModes(schema) {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
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
							if want := tt.wantSQL(d); got != want {
								t.Fatalf("TranspileValue() = %q, want %q", got, want)
							}
							if strings.Contains(got, "!= FALSE") || strings.Contains(got, "!= ''") {
								t.Fatalf("TranspileValue() used mixed-type truthiness for numeric reduce: %s", got)
							}

							gotParam, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
							if err != nil {
								t.Fatalf("TranspileParameterizedValue() error = %v", err)
							}
							if want := tt.wantParam(d); gotParam != want {
								t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, want)
							}
							if strings.Contains(gotParam, "!= FALSE") || strings.Contains(gotParam, "!= ''") {
								t.Fatalf("TranspileParameterizedValue() used mixed-type truthiness for numeric reduce: %s", gotParam)
							}
							if !reflect.DeepEqual(gotParams, tt.wantParams) {
								t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
							}
						})
					}
				})
			}
		})
	}
}

func TestTranspileValue_ReduceStringTruthinessUsesInferredTypeAllDialectsSchemaModes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray},
	})
	logic := `{"or":[{"reduce":[{"var":"arr"},{"cat":[{"var":"accumulator"},{"var":"current"}]},""]},"fallback"]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range allSchemaModes(schema) {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}

					got, err := tr.TranspileValue(logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if !strings.Contains(got, "!= ''") {
						t.Fatalf("TranspileValue() did not use string truthiness: %s", got)
					}
					if strings.Contains(got, "!= FALSE") || strings.Contains(got, "!= 0") {
						t.Fatalf("TranspileValue() used mixed-type truthiness for string reduce: %s", got)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedValue(logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if !strings.Contains(gotParam, "!= ''") {
						t.Fatalf("TranspileParameterizedValue() did not use string truthiness: %s", gotParam)
					}
					if strings.Contains(gotParam, "!= FALSE") || strings.Contains(gotParam, "!= 0") {
						t.Fatalf("TranspileParameterizedValue() used mixed-type truthiness for string reduce: %s", gotParam)
					}
					wantParams := []QueryParam{{Name: "p1", Value: ""}, {Name: "p2", Value: "fallback"}}
					if !reflect.DeepEqual(gotParams, wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
					}
				})
			}
		})
	}
}

func TestTranspileValue_ReduceAccumulatorTruthinessUsesInitialTypeAllDialectsSchemaModes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray},
	})

	renderReduce := func(d Dialect, reducer, initial string) string {
		if d == DialectClickHouse {
			return fmt.Sprintf("arrayFold((acc, elem) -> %s, arr, %s)", reducer, initial)
		}
		return fmt.Sprintf("(SELECT %s FROM UNNEST(arr) AS elem)", reducer)
	}

	tests := []struct {
		name             string
		logic            string
		wantReducer      func(initial string) string
		wantInitial      string
		wantParamReducer func(Dialect) string
		wantParamInitial func(Dialect) string
		wantParams       []QueryParam
		forbidden        []string
	}{
		{
			name:  "or tests numeric accumulator with numeric truthiness",
			logic: `{"reduce":[{"var":"arr"},{"or":[{"var":"accumulator"},{"var":"current"}]},0]}`,
			wantReducer: func(initial string) string {
				return fmt.Sprintf("CASE WHEN (%s IS NOT NULL AND %s != 0) THEN %s ELSE elem END", initial, initial, initial)
			},
			wantInitial: "0",
			wantParamReducer: func(d Dialect) string {
				initial := testPlaceholder(d, 1)
				return fmt.Sprintf("CASE WHEN (%s IS NOT NULL AND %s != 0) THEN %s ELSE elem END", initial, initial, initial)
			},
			wantParamInitial: func(d Dialect) string { return testPlaceholder(d, 1) },
			wantParams:       []QueryParam{{Name: "p1", Value: float64(0)}},
			forbidden:        []string{"!= FALSE", "!= ''"},
		},
		{
			name:  "if tests string accumulator with string truthiness",
			logic: `{"reduce":[{"var":"arr"},{"if":[{"var":"accumulator"},{"var":"accumulator"},{"var":"current"}]},""]}`,
			wantReducer: func(initial string) string {
				return fmt.Sprintf("CASE WHEN (%s IS NOT NULL AND %s != '') THEN %s ELSE elem END", initial, initial, initial)
			},
			wantInitial: "''",
			wantParamReducer: func(d Dialect) string {
				initial := testPlaceholder(d, 1)
				return fmt.Sprintf("CASE WHEN (%s IS NOT NULL AND %s != '') THEN %s ELSE elem END", initial, initial, initial)
			},
			wantParamInitial: func(d Dialect) string { return testPlaceholder(d, 1) },
			wantParams:       []QueryParam{{Name: "p1", Value: ""}},
			forbidden:        []string{"!= FALSE", "!= 0"},
		},
		{
			name:  "if tests boolean accumulator with boolean truthiness",
			logic: `{"reduce":[{"var":"arr"},{"if":[{"var":"accumulator"},{"var":"accumulator"},{"var":"current"}]},false]}`,
			wantReducer: func(initial string) string {
				return fmt.Sprintf("CASE WHEN %s IS TRUE THEN %s ELSE elem END", initial, initial)
			},
			wantInitial: "FALSE",
			wantParamReducer: func(Dialect) string {
				return "CASE WHEN FALSE IS TRUE THEN FALSE ELSE elem END"
			},
			wantParamInitial: func(Dialect) string { return "FALSE" },
			forbidden:        []string{"!= FALSE", "!= 0", "!= ''"},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range allSchemaModes(schema) {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
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
							want := renderReduce(d, tt.wantReducer(tt.wantInitial), tt.wantInitial)
							if got != want {
								t.Fatalf("TranspileValue() = %q, want %q", got, want)
							}
							for _, bad := range tt.forbidden {
								if strings.Contains(got, bad) {
									t.Fatalf("TranspileValue() used wrong accumulator truthiness %q in %s", bad, got)
								}
							}

							gotParam, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
							if err != nil {
								t.Fatalf("TranspileParameterizedValue() error = %v", err)
							}
							wantParam := renderReduce(d, tt.wantParamReducer(d), tt.wantParamInitial(d))
							if gotParam != wantParam {
								t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, wantParam)
							}
							for _, bad := range tt.forbidden {
								if strings.Contains(gotParam, bad) {
									t.Fatalf("TranspileParameterizedValue() used wrong accumulator truthiness %q in %s", bad, gotParam)
								}
							}
							if !reflect.DeepEqual(gotParams, tt.wantParams) {
								t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
							}
						})
					}
				})
			}
		})
	}
}

func TestTranspileValue_ReduceAccumulatorTruthinessRejectsUnknownInitialTypeAllDialects(t *testing.T) {
	t.Parallel()

	logic := `{"reduce":[{"var":"arr"},{"or":[{"var":"accumulator"},{"var":"current"}]},{"var":"seed"}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			got, valueErr := tr.TranspileValue(logic)
			if !IsErrorCode(valueErr, ErrInvalidExpressionContext) {
				t.Fatalf("TranspileValue() = %q, error = %v, want %s", got, valueErr, ErrInvalidExpressionContext)
			}

			gotParam, gotParams, err := tr.TranspileParameterizedValue(logic)
			if !IsErrorCode(err, ErrInvalidExpressionContext) {
				t.Fatalf("TranspileParameterizedValue() = %q params %#v, error = %v, want %s",
					gotParam, gotParams, err, ErrInvalidExpressionContext)
			}
		})
	}
}

func TestTranspileValue_ReduceAccumulatorTruthinessUsesSchemaInitialTypeAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray},
		{Name: "seed", Type: FieldTypeNumber},
	})
	logic := `{"reduce":[{"var":"arr"},{"or":[{"var":"accumulator"},{"var":"current"}]},{"var":"seed"}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			reducer := "CASE WHEN (seed IS NOT NULL AND seed != 0) THEN seed ELSE elem END"
			want := fmt.Sprintf("(SELECT %s FROM UNNEST(arr) AS elem)", reducer)
			if d == DialectClickHouse {
				want = fmt.Sprintf("arrayFold((acc, elem) -> %s, arr, seed)", reducer)
			}

			got, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if got != want {
				t.Fatalf("TranspileValue() = %q, want %q", got, want)
			}
			if strings.Contains(got, "!= FALSE") || strings.Contains(got, "!= ''") {
				t.Fatalf("TranspileValue() used mixed-type accumulator truthiness: %s", got)
			}

			gotParam, gotParams, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			if gotParam != want {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, want)
			}
			if len(gotParams) != 0 {
				t.Fatalf("params = %#v, want none", gotParams)
			}
		})
	}
}

func TestTranspileValue_PostgreSQLEmptyArrayScannerSkipsStringLiterals(t *testing.T) {
	tr, err := NewTranspiler(DialectPostgreSQL)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	got, err := tr.TranspileValue(`"ARRAY[]"`)
	if err != nil {
		t.Fatalf("TranspileValue() string literal error = %v", err)
	}
	if got != "'ARRAY[]'" {
		t.Fatalf("TranspileValue() string literal = %q, want %q", got, "'ARRAY[]'")
	}

	gotParam, gotParams, err := tr.TranspileParameterizedValue(`"ARRAY[]"`)
	if err != nil {
		t.Fatalf("TranspileParameterizedValue() string literal error = %v", err)
	}
	if gotParam != "$1" {
		t.Fatalf("TranspileParameterizedValue() string literal = %q, want %q", gotParam, "$1")
	}
	if want := []QueryParam{{Name: "p1", Value: "ARRAY[]"}}; !reflect.DeepEqual(gotParams, want) {
		t.Fatalf("params = %#v, want %#v", gotParams, want)
	}
}

func TestTranspileValue_PostgreSQLEmptyArrayScannerAllowsTypedSQL(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray},
	})
	schemaModes := allSchemaModes(schema)

	for _, mode := range schemaModes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: DialectPostgreSQL,
				Schema:  mode.schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			customSQL := map[string]string{
				"typedArrayCast":      "CAST(ARRAY[] AS INT[])",
				"typedArrayShorthand": "ARRAY[]::INT[]",
				"untypedArray":        "ARRAY[]",
			}
			for name, sql := range customSQL {
				err = tr.RegisterOperatorFunc(name, func(_ string, _ []OperatorArg) (OperatorResult, error) {
					return ValueSQL(sql, ExpressionTypeArray), nil
				})
				if err != nil {
					t.Fatalf("RegisterOperatorFunc(%q) error = %v", name, err)
				}
			}

			for _, tt := range []struct {
				name    string
				logic   string
				wantSQL string
				wantErr ErrorCode
			}{
				{name: "cast typed array", logic: `{"typedArrayCast":[]}`, wantSQL: "CAST(ARRAY[] AS INT[])"},
				{name: "shorthand typed array", logic: `{"typedArrayShorthand":[]}`, wantSQL: "ARRAY[]::INT[]"},
				{name: "untyped array", logic: `{"untypedArray":[]}`, wantErr: ErrInvalidArgument},
			} {
				t.Run(tt.name, func(t *testing.T) {
					got, err := tr.TranspileValue(tt.logic)
					if tt.wantErr != "" {
						if !IsErrorCode(err, tt.wantErr) {
							t.Fatalf("TranspileValue() error = %v, want %s", err, tt.wantErr)
						}
					} else if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					} else if got != tt.wantSQL {
						t.Fatalf("TranspileValue() = %q, want %q", got, tt.wantSQL)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
					if tt.wantErr != "" {
						if !IsErrorCode(err, tt.wantErr) {
							t.Fatalf("TranspileParameterizedValue() error = %v, want %s", err, tt.wantErr)
						}
						return
					}
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if gotParam != tt.wantSQL {
						t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, tt.wantSQL)
					}
					if len(gotParams) != 0 {
						t.Fatalf("params = %#v, want none", gotParams)
					}
				})
			}
		})
	}
}

func TestTranspileValue_UnderflowJSONNumberTruthinessAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeNumber},
	})
	schemaModes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-less"},
		{name: "schema-aware", schema: schema},
	}

	valueCases := []struct {
		name       string
		logic      string
		want       string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "or treats underflowed number as falsy",
			logic: `{"or":[1e-400,"fallback"]}`,
			want:  "'fallback'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "fallback"}},
		},
		{
			name:  "or treats extreme underflowed number as falsy",
			logic: `{"or":[1e-9999,"fallback"]}`,
			want:  "'fallback'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "fallback"}},
		},
		{
			name:  "or keeps minimum subnormal number truthy",
			logic: `{"or":[5e-324,"fallback"]}`,
			want:  "5e-324",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: 5e-324}},
		},
		{
			name:  "if treats underflowed number as falsy",
			logic: `{"if":[1e-400,"yes","no"]}`,
			want:  "'no'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "no"}},
		},
	}

	conditionCases := []struct {
		name      string
		logic     string
		want      string
		wantParam string
	}{
		{
			name:      "double bang underflowed number",
			logic:     `{"!!":1e-400}`,
			want:      "FALSE",
			wantParam: "FALSE",
		},
		{
			name:      "not underflowed number",
			logic:     `{"!":1e-400}`,
			want:      "TRUE",
			wantParam: "TRUE",
		},
		{
			name:      "not extreme underflowed number",
			logic:     `{"!":1e-9999}`,
			want:      "TRUE",
			wantParam: "TRUE",
		},
		{
			name:      "double bang minimum subnormal number",
			logic:     `{"!!":5e-324}`,
			want:      "TRUE",
			wantParam: "TRUE",
		},
		{
			name:      "not minimum subnormal number",
			logic:     `{"!":5e-324}`,
			want:      "FALSE",
			wantParam: "FALSE",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaModes {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}

					for _, tt := range valueCases {
						t.Run(tt.name, func(t *testing.T) {
							got, err := tr.TranspileValue(tt.logic)
							if err != nil {
								t.Fatalf("TranspileValue() error = %v", err)
							}
							if got != tt.want {
								t.Fatalf("TranspileValue() = %q, want %q", got, tt.want)
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

					for _, tt := range conditionCases {
						t.Run(tt.name, func(t *testing.T) {
							got, err := tr.TranspileCondition(tt.logic)
							if err != nil {
								t.Fatalf("TranspileCondition() error = %v", err)
							}
							if got != tt.want {
								t.Fatalf("TranspileCondition() = %q, want %q", got, tt.want)
							}

							gotParam, gotParams, err := tr.TranspileParameterizedCondition(tt.logic)
							if err != nil {
								t.Fatalf("TranspileParameterizedCondition() error = %v", err)
							}
							if gotParam != tt.wantParam {
								t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, tt.wantParam)
							}
							if len(gotParams) != 0 {
								t.Fatalf("params = %#v, want none", gotParams)
							}
						})
					}
				})
			}
		})
	}
}

func TestTranspileValue_OverflowJSONNumberComparisonsDoNotShortCircuitAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeNumber},
	})

	tests := []struct {
		name       string
		logic      string
		want       string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "equality remains dynamic",
			logic: `{"if":[{"==":[1e400,1e400]},"yes","no"]}`,
			want:  "CASE WHEN 1e400 = 1e400 THEN 'yes' ELSE 'no' END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf(
					"CASE WHEN %s = %s THEN %s ELSE %s END",
					testPlaceholder(d, 1),
					testPlaceholder(d, 2),
					testPlaceholder(d, 3),
					testPlaceholder(d, 4),
				)
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: "1e400"},
				{Name: "p2", Value: "1e400"},
				{Name: "p3", Value: "yes"},
				{Name: "p4", Value: "no"},
			},
		},
		{
			name:  "inequality remains dynamic",
			logic: `{"if":[{"!=":[1e400,1e400]},"yes","no"]}`,
			want:  "CASE WHEN 1e400 != 1e400 THEN 'yes' ELSE 'no' END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf(
					"CASE WHEN %s != %s THEN %s ELSE %s END",
					testPlaceholder(d, 1),
					testPlaceholder(d, 2),
					testPlaceholder(d, 3),
					testPlaceholder(d, 4),
				)
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: "1e400"},
				{Name: "p2", Value: "1e400"},
				{Name: "p3", Value: "yes"},
				{Name: "p4", Value: "no"},
			},
		},
		{
			name:  "ordering remains dynamic",
			logic: `{"if":[{">":[1e400,1]},"yes","no"]}`,
			want:  "CASE WHEN 1e400 > 1 THEN 'yes' ELSE 'no' END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf(
					"CASE WHEN %s > %s THEN %s ELSE %s END",
					testPlaceholder(d, 1),
					testPlaceholder(d, 2),
					testPlaceholder(d, 3),
					testPlaceholder(d, 4),
				)
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: "1e400"},
				{Name: "p2", Value: float64(1)},
				{Name: "p3", Value: "yes"},
				{Name: "p4", Value: "no"},
			},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range allSchemaModes(schema) {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
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
							if got != tt.want {
								t.Fatalf("TranspileValue() = %q, want %q", got, tt.want)
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
		})
	}
}

func TestTranspileValue_NativeNaNTruthinessAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeNumber},
	})
	schemaModes := allSchemaModes(schema)

	valueLogic := map[string]interface{}{
		"or": []interface{}{math.NaN(), "fallback"},
	}
	truthyValueLogic := map[string]interface{}{
		"and": []interface{}{math.Inf(1), "fallback"},
	}
	ifNaNLogic := map[string]interface{}{
		"if": []interface{}{math.NaN(), "yes", "no"},
	}
	ifInfLogic := map[string]interface{}{
		"if": []interface{}{math.Inf(1), "yes", "no"},
	}
	conditionCases := []struct {
		name  string
		logic map[string]interface{}
		want  string
	}{
		{
			name:  "double bang native NaN",
			logic: map[string]interface{}{"!!": math.NaN()},
			want:  "FALSE",
		},
		{
			name:  "not native NaN",
			logic: map[string]interface{}{"!": math.NaN()},
			want:  "TRUE",
		},
		{
			name:  "float32 native NaN",
			logic: map[string]interface{}{"!!": float32(math.NaN())},
			want:  "FALSE",
		},
		{
			name:  "double bang native infinity",
			logic: map[string]interface{}{"!!": math.Inf(1)},
			want:  "TRUE",
		},
		{
			name:  "not native infinity",
			logic: map[string]interface{}{"!": math.Inf(-1)},
			want:  "FALSE",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaModes {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}

					got, err := tr.TranspileValueFromInterface(valueLogic)
					if err != nil {
						t.Fatalf("TranspileValueFromInterface() error = %v", err)
					}
					if got != "'fallback'" {
						t.Fatalf("TranspileValueFromInterface() = %q, want %q", got, "'fallback'")
					}

					gotParam, gotParams, err := tr.TranspileParameterizedValueFromInterface(valueLogic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValueFromInterface() error = %v", err)
					}
					if want := testPlaceholder(d, 1); gotParam != want {
						t.Fatalf("TranspileParameterizedValueFromInterface() = %q, want %q", gotParam, want)
					}
					wantParams := []QueryParam{{Name: "p1", Value: "fallback"}}
					if !reflect.DeepEqual(gotParams, wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
					}

					got, err = tr.TranspileValueFromInterface(truthyValueLogic)
					if err != nil {
						t.Fatalf("TranspileValueFromInterface(and Inf) error = %v", err)
					}
					if got != "'fallback'" {
						t.Fatalf("TranspileValueFromInterface(and Inf) = %q, want %q", got, "'fallback'")
					}

					gotParam, gotParams, err = tr.TranspileParameterizedValueFromInterface(truthyValueLogic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValueFromInterface(and Inf) error = %v", err)
					}
					if want := testPlaceholder(d, 1); gotParam != want {
						t.Fatalf("TranspileParameterizedValueFromInterface(and Inf) = %q, want %q", gotParam, want)
					}
					if wantParams := []QueryParam{{Name: "p1", Value: "fallback"}}; !reflect.DeepEqual(gotParams, wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
					}

					for _, tt := range []struct {
						name       string
						logic      map[string]interface{}
						want       string
						wantParams []QueryParam
					}{
						{
							name:       "if NaN condition",
							logic:      ifNaNLogic,
							want:       "'no'",
							wantParams: []QueryParam{{Name: "p1", Value: "no"}},
						},
						{
							name:       "if Inf condition",
							logic:      ifInfLogic,
							want:       "'yes'",
							wantParams: []QueryParam{{Name: "p1", Value: "yes"}},
						},
					} {
						t.Run(tt.name, func(t *testing.T) {
							got, err := tr.TranspileValueFromInterface(tt.logic)
							if err != nil {
								t.Fatalf("TranspileValueFromInterface() error = %v", err)
							}
							if got != tt.want {
								t.Fatalf("TranspileValueFromInterface() = %q, want %q", got, tt.want)
							}

							gotParam, gotParams, err := tr.TranspileParameterizedValueFromInterface(tt.logic)
							if err != nil {
								t.Fatalf("TranspileParameterizedValueFromInterface() error = %v", err)
							}
							if want := testPlaceholder(d, 1); gotParam != want {
								t.Fatalf("TranspileParameterizedValueFromInterface() = %q, want %q", gotParam, want)
							}
							if !reflect.DeepEqual(gotParams, tt.wantParams) {
								t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
							}
						})
					}

					for _, tt := range conditionCases {
						t.Run(tt.name, func(t *testing.T) {
							got, err := tr.TranspileConditionFromInterface(tt.logic)
							if err != nil {
								t.Fatalf("TranspileConditionFromInterface() error = %v", err)
							}
							if got != tt.want {
								t.Fatalf("TranspileConditionFromInterface() = %q, want %q", got, tt.want)
							}

							gotParam, gotParams, err := tr.TranspileParameterizedConditionFromInterface(tt.logic)
							if err != nil {
								t.Fatalf("TranspileParameterizedConditionFromInterface() error = %v", err)
							}
							if gotParam != tt.want {
								t.Fatalf("TranspileParameterizedConditionFromInterface() = %q, want %q", gotParam, tt.want)
							}
							if len(gotParams) != 0 {
								t.Fatalf("params = %#v, want none", gotParams)
							}
						})
					}
				})
			}
		})
	}
}

func TestTranspileValue_RejectsReturnedNativeNonFiniteFloatsAllDialectsSchemaModes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeNumber},
	})

	tests := []struct {
		name  string
		logic interface{}
	}{
		{
			name:  "root positive infinity",
			logic: math.Inf(1),
		},
		{
			name:  "root negative infinity",
			logic: math.Inf(-1),
		},
		{
			name:  "root NaN",
			logic: math.NaN(),
		},
		{
			name:  "float32 NaN",
			logic: float32(math.NaN()),
		},
		{
			name: "cat NaN",
			logic: map[string]interface{}{
				"cat": []interface{}{math.NaN()},
			},
		},
		{
			name: "array NaN",
			logic: []interface{}{
				math.NaN(),
			},
		},
		{
			name: "or returns infinity",
			logic: map[string]interface{}{
				"or": []interface{}{math.Inf(1), "fallback"},
			},
		},
		{
			name: "and returns NaN",
			logic: map[string]interface{}{
				"and": []interface{}{math.NaN(), "fallback"},
			},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range allSchemaModes(schema) {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}

					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							if _, err := tr.TranspileValueFromInterface(tt.logic); !IsErrorCode(err, ErrInvalidArgument) {
								t.Fatalf("TranspileValueFromInterface() error = %v, want %s", err, ErrInvalidArgument)
							}
							sql, params, err := tr.TranspileParameterizedValueFromInterface(tt.logic)
							if !IsErrorCode(err, ErrInvalidArgument) {
								t.Fatalf("TranspileParameterizedValueFromInterface() error = %v, want %s (SQL %q params %#v)",
									err, ErrInvalidArgument, sql, params)
							}
							if len(params) != 0 {
								t.Fatalf("params = %#v, want none", params)
							}
						})
					}
				})
			}
		})
	}
}

func TestTranspileValue_ArrayValueFallbackStringLiteralsNotRewritten(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		logic   string
		literal string
		param   string
	}{
		{
			name:    "current literal",
			logic:   `{"map":[{"var":"arr"},{"or":[0,"current"]}]}`,
			literal: "'current'",
			param:   "current",
		},
		{
			name:    "current and item in escaped literal",
			logic:   `{"map":[{"var":"arr"},{"or":[0,"current's item"]}]}`,
			literal: "'current''s item'",
			param:   "current's item",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

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
					want := fmt.Sprintf("ARRAY(SELECT %s FROM UNNEST(arr) AS elem)", tt.literal)
					if d == DialectClickHouse {
						want = fmt.Sprintf("arrayMap(elem -> %s, arr)", tt.literal)
					}
					if got != want {
						t.Fatalf("TranspileValue() = %q, want %q", got, want)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					placeholder := testPlaceholder(d, 1)
					wantParam := fmt.Sprintf("ARRAY(SELECT %s FROM UNNEST(arr) AS elem)", placeholder)
					if d == DialectClickHouse {
						wantParam = fmt.Sprintf("arrayMap(elem -> %s, arr)", placeholder)
					}
					if gotParam != wantParam {
						t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, wantParam)
					}
					wantParams := []QueryParam{{Name: "p1", Value: tt.param}}
					if !reflect.DeepEqual(gotParams, wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
					}
				})
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

func TestTranspileValue_MapTransformationArrayLiteralAllDialectsSchemaModes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray},
	})

	arrayLiteral := func(d Dialect, elem string) string {
		if d == DialectPostgreSQL {
			return fmt.Sprintf("ARRAY[%s]", elem)
		}
		return fmt.Sprintf("[%s]", elem)
	}
	mapSQL := func(d Dialect, transformation string) string {
		if d == DialectClickHouse {
			return fmt.Sprintf("arrayMap(elem -> %s, arr)", transformation)
		}
		return fmt.Sprintf("ARRAY(SELECT %s FROM UNNEST(arr) AS elem)", transformation)
	}

	tests := []struct {
		name       string
		logic      string
		wantSQL    func(Dialect) string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "direct array literal",
			logic: `{"map":[{"var":"arr"},[{"var":"current"}]]}`,
			wantSQL: func(d Dialect) string {
				return mapSQL(d, arrayLiteral(d, "elem"))
			},
			wantParam: func(d Dialect) string {
				return mapSQL(d, arrayLiteral(d, "elem"))
			},
			wantParams: []QueryParam{},
		},
		{
			name:  "array literal with value logical",
			logic: `{"map":[{"var":"arr"},[{"or":[0,{"var":"current"}]}]]}`,
			wantSQL: func(d Dialect) string {
				return mapSQL(d, arrayLiteral(d, "elem"))
			},
			wantParam: func(d Dialect) string {
				return mapSQL(d, arrayLiteral(d, "elem"))
			},
			wantParams: []QueryParam{},
		},
		{
			name:  "array literal with defaulted current",
			logic: `{"map":[{"var":"arr"},[{"var":["current","fallback"]}]]}`,
			wantSQL: func(d Dialect) string {
				return mapSQL(d, arrayLiteral(d, "COALESCE(elem, 'fallback')"))
			},
			wantParam: func(d Dialect) string {
				return mapSQL(d, arrayLiteral(d, fmt.Sprintf("COALESCE(elem, %s)", testPlaceholder(d, 1))))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "fallback"}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range allSchemaModes(schema) {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
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
							if !regressionParamsEqual(gotParams, tt.wantParams) {
								t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
							}
						})
					}
				})
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

func TestTranspileValue_FoldedPredicateConditionsShortCircuit(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "code", Type: FieldTypeString},
	})

	tests := []struct {
		name       string
		logic      string
		want       string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "folded equality false skips unreachable if branch",
			logic: `{"if":[{"==":[true,false]},{"var":"missing"},"ok"]}`,
			want:  "'ok'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "ok"}},
		},
		{
			name:  "schema strict mismatch skips unreachable if branch",
			logic: `{"if":[{"===":[{"var":"code"},5]},{"var":"missing"},"ok"]}`,
			want:  "'ok'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "ok"}},
		},
		{
			name:  "constant not false skips unreachable and branch",
			logic: `{"and":[{"!":"x"},{"var":"missing"}]}`,
			want:  "FALSE",
			wantParam: func(Dialect) string {
				return "FALSE"
			},
			wantParams: []QueryParam{},
		},
		{
			name:  "constant not false skips unreachable if branch",
			logic: `{"if":[{"!":"x"},{"var":"missing"},"ok"]}`,
			want:  "'ok'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "ok"}},
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
					if got != tt.want {
						t.Fatalf("TranspileValue() = %q, want %q", got, tt.want)
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

func TestTranspileValue_LiteralPredicateFallbacksAllDialects(t *testing.T) {
	tests := []struct {
		name       string
		logic      string
		wantSQL    string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:    "or skips literal false predicate",
			logic:   `{"or":[{"!=":[null,null]},"x"]}`,
			wantSQL: "'x'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "x"}},
		},
		{
			name:    "and skips literal true predicate",
			logic:   `{"and":[{"==":[1,1]},"x"]}`,
			wantSQL: "'x'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "x"}},
		},
		{
			name:    "or skips literal false ordering predicate",
			logic:   `{"or":[{">":[1,2]},"x"]}`,
			wantSQL: "'x'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "x"}},
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

func TestTranspileValue_NestedFieldMetadataPreservesSchemaValidation(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "code", Type: FieldTypeString},
		{Name: "tags", Type: FieldTypeArray},
	})

	tests := []struct {
		name  string
		logic string
	}{
		{
			name:  "numeric op rejects folded string field",
			logic: `{"+":[{"if":[true,{"var":"code"},"x"]},1]}`,
		},
		{
			name:  "string op rejects folded array field",
			logic: `{"cat":[{"if":[true,{"var":"tags"},[]]}]}`,
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
					if _, err := tr.TranspileValue(tt.logic); !IsErrorCode(err, ErrInvalidArgument) {
						t.Fatalf("TranspileValue() error = %v, want %s", err, ErrInvalidArgument)
					}
					sql, params, err := tr.TranspileParameterizedValue(tt.logic)
					if !IsErrorCode(err, ErrInvalidArgument) {
						t.Fatalf("TranspileParameterizedValue() error = %v, want %s (SQL %q params %#v)",
							err, ErrInvalidArgument, sql, params)
					}
				})
			}
		})
	}
}

func TestTranspileValue_SchemaLessUnknownTruthinessRejectedAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "nickname", Type: FieldTypeString},
		{Name: "flag", Type: FieldTypeBoolean},
	})

	schemaLessValueCases := []string{
		`{"or":[{"var":"nickname"},"unknown"]}`,
		`{"and":[{"var":"nickname"},"known"]}`,
		`{"if":[{"var":"nickname"},"yes","no"]}`,
	}
	schemaLessConditionCases := []string{
		`{"!!":{"var":"nickname"}}`,
		`{"!":{"var":"nickname"}}`,
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			schemaLess, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			for _, logic := range schemaLessValueCases {
				t.Run("schema-less/value/"+logic, func(t *testing.T) {
					if _, valueErr := schemaLess.TranspileValue(logic); !IsErrorCode(valueErr, ErrInvalidExpressionContext) {
						t.Fatalf("TranspileValue() error = %v, want %s", valueErr, ErrInvalidExpressionContext)
					}
					sql, params, paramErr := schemaLess.TranspileParameterizedValue(logic)
					if !IsErrorCode(paramErr, ErrInvalidExpressionContext) {
						t.Fatalf("TranspileParameterizedValue() error = %v, want %s (SQL %q params %#v)",
							paramErr, ErrInvalidExpressionContext, sql, params)
					}
				})
			}
			for _, logic := range schemaLessConditionCases {
				t.Run("schema-less/condition/"+logic, func(t *testing.T) {
					if _, conditionErr := schemaLess.TranspileCondition(logic); !IsErrorCode(conditionErr, ErrInvalidExpressionContext) {
						t.Fatalf("TranspileCondition() error = %v, want %s", conditionErr, ErrInvalidExpressionContext)
					}
					sql, params, paramErr := schemaLess.TranspileParameterizedCondition(logic)
					if !IsErrorCode(paramErr, ErrInvalidExpressionContext) {
						t.Fatalf("TranspileParameterizedCondition() error = %v, want %s (SQL %q params %#v)",
							paramErr, ErrInvalidExpressionContext, sql, params)
					}
				})
			}

			schemaAware, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			got, err := schemaAware.TranspileValue(`{"or":[{"var":"nickname"},"unknown"]}`)
			if err != nil {
				t.Fatalf("schema-aware TranspileValue(or) error = %v", err)
			}
			if want := "CASE WHEN (nickname IS NOT NULL AND nickname != '') THEN nickname ELSE 'unknown' END"; got != want {
				t.Fatalf("schema-aware TranspileValue(or) = %q, want %q", got, want)
			}

			gotParam, gotParams, err := schemaAware.TranspileParameterizedValue(`{"or":[{"var":"nickname"},"unknown"]}`)
			if err != nil {
				t.Fatalf("schema-aware TranspileParameterizedValue(or) error = %v", err)
			}
			if want := fmt.Sprintf("CASE WHEN (nickname IS NOT NULL AND nickname != '') THEN nickname ELSE %s END", testPlaceholder(d, 1)); gotParam != want {
				t.Fatalf("schema-aware TranspileParameterizedValue(or) = %q, want %q", gotParam, want)
			}
			if want := []QueryParam{{Name: "p1", Value: "unknown"}}; !reflect.DeepEqual(gotParams, want) {
				t.Fatalf("schema-aware params = %#v, want %#v", gotParams, want)
			}

			got, err = schemaAware.TranspileValue(`{"if":[{"var":"flag"},"yes","no"]}`)
			if err != nil {
				t.Fatalf("schema-aware TranspileValue(if) error = %v", err)
			}
			if want := "CASE WHEN flag IS TRUE THEN 'yes' ELSE 'no' END"; got != want {
				t.Fatalf("schema-aware TranspileValue(if) = %q, want %q", got, want)
			}

			gotCond, err := schemaAware.TranspileCondition(`{"!!":{"var":"nickname"}}`)
			if err != nil {
				t.Fatalf("schema-aware TranspileCondition(!!) error = %v", err)
			}
			if want := "(nickname IS NOT NULL AND nickname != '')"; gotCond != want {
				t.Fatalf("schema-aware TranspileCondition(!!) = %q, want %q", gotCond, want)
			}

			gotParamCond, gotCondParams, err := schemaAware.TranspileParameterizedCondition(`{"!!":{"var":"nickname"}}`)
			if err != nil {
				t.Fatalf("schema-aware TranspileParameterizedCondition(!!) error = %v", err)
			}
			if want := "(nickname IS NOT NULL AND nickname != '')"; gotParamCond != want {
				t.Fatalf("schema-aware TranspileParameterizedCondition(!!) = %q, want %q", gotParamCond, want)
			}
			if len(gotCondParams) != 0 {
				t.Fatalf("schema-aware condition params = %#v, want none", gotCondParams)
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

func TestTranspileValue_IfUsesTypedCustomPredicateCondition(t *testing.T) {
	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
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

			logic := `{"if":[{"isPositive":[{"var":"amount"}]},"yes","no"]}`
			want := "CASE WHEN amount > 0 THEN 'yes' ELSE 'no' END"
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
			wantParam := fmt.Sprintf("CASE WHEN amount > 0 THEN %s ELSE %s END", testPlaceholder(d, 1), testPlaceholder(d, 2))
			if gotParam != wantParam {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, wantParam)
			}
			wantParams := []QueryParam{{Name: "p1", Value: "yes"}, {Name: "p2", Value: "no"}}
			if !reflect.DeepEqual(params, wantParams) {
				t.Fatalf("params = %#v, want %#v", params, wantParams)
			}
		})
	}
}

func TestTranspileValue_TypedCustomOperatorUsesValueContext(t *testing.T) {
	tr, err := NewTranspiler(DialectPostgreSQL)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	err = tr.RegisterDialectAwareOperatorFunc("safeDivideTyped", func(_ string, args []OperatorArg, _ Dialect) (OperatorResult, error) {
		if len(args) != 2 {
			return OperatorResult{}, fmt.Errorf("safeDivideTyped requires exactly 2 arguments")
		}
		sql := fmt.Sprintf("CASE WHEN %s = 0 THEN NULL ELSE %s / %s END", args[1].SQL, args[0].SQL, args[1].SQL)
		return ValueSQL(sql, ExpressionTypeNumber), nil
	})
	if err != nil {
		t.Fatalf("RegisterDialectAwareOperatorFunc() error = %v", err)
	}

	logic := `{"cat":[{"safeDivideTyped":[{"var":"total"},{"var":"count"}]}]}`
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
			wantSQL:    "FALSE",
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
	logic := `{"map":[{"var":"items"},{"cat":[{"var":["current","fallback"]}]}]}`

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

func TestTranspileValue_CustomPredicateBooleanConstantsShortCircuitAllDialectsSchemaModes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "x", Type: FieldTypeNumber},
	})

	tests := []struct {
		name       string
		logic      string
		want       string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "or returns custom true before invalid fallback",
			logic: `{"or":[{"alwaysTrue":[]},{"var":"bad-name"}]}`,
			want:  "TRUE",
			wantParam: func(Dialect) string {
				return "TRUE"
			},
		},
		{
			name:  "and returns custom false before invalid fallback",
			logic: `{"and":[{"alwaysFalse":[]},{"var":"bad-name"}]}`,
			want:  "FALSE",
			wantParam: func(Dialect) string {
				return "FALSE"
			},
		},
		{
			name:  "if returns then value before invalid else",
			logic: `{"if":[{"alwaysTrue":[]},"ok",{"var":"bad-name"}]}`,
			want:  "'ok'",
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "ok"}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range allSchemaModes(schema) {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}
					registerBooleanConstantOperators(t, tr)

					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							got, err := tr.TranspileValue(tt.logic)
							if err != nil {
								t.Fatalf("TranspileValue() error = %v", err)
							}
							if got != tt.want {
								t.Fatalf("TranspileValue() = %q, want %q", got, tt.want)
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
