package jsonlogic2sql

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func dialectRejectsArrayLiteralElements(d Dialect) bool {
	return d == DialectBigQuery || d == DialectSpanner
}

func dialectRejectsNestedArraySources(d Dialect) bool {
	return d == DialectBigQuery || d == DialectSpanner || d == DialectPostgreSQL
}

func nestedArrayErrorFragment(d Dialect) string {
	if d == DialectPostgreSQL {
		return "support"
	}
	return "does not support array literals whose elements are arrays"
}

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

			tr, err := NewTranspiler(tt.dialect, defaultTestSchema())
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

func TestTranspileValue_RejectsMalformedVarOperandsAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "x", Type: FieldTypeNumber},
		{Name: "items", Type: FieldTypeArray, ElementType: FieldTypeNumber},
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
			logic: `{"map":[{"var":"items"},{"var":["",1,2]}]}`,
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

			for _, mode := range schemaRequiredModes(schema) {
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

func TestTranspileValue_ArraySourcesRejectMultiKeyVarObjectsAllDialects(t *testing.T) {
	t.Parallel()

	source := `{"var":"arr","bad":[]}`
	valueCases := []string{
		`{"map":[` + source + `,{"var":""}]}`,
		`{"filter":[` + source + `,true]}`,
		`{"merge":[` + source + `,[1]]}`,
		`{"reduce":[` + source + `,{"+":[{"var":"accumulator"},{"var":"current"}]},0]}`,
		`{"all":[` + source + `,true]}`,
		`{"some":[` + source + `,true]}`,
		`{"none":[` + source + `,true]}`,
	}
	conditionCases := []string{
		`{"all":[` + source + `,true]}`,
		`{"some":[` + source + `,true]}`,
		`{"none":[` + source + `,true]}`,
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, logic := range valueCases {
				t.Run("value "+logic, func(t *testing.T) {
					if _, err := tr.TranspileValue(logic); !IsErrorCode(err, ErrMultipleKeys) {
						t.Fatalf("TranspileValue() error = %v, want %s", err, ErrMultipleKeys)
					}
					sql, params, err := tr.TranspileParameterizedValue(logic)
					if !IsErrorCode(err, ErrMultipleKeys) {
						t.Fatalf("TranspileParameterizedValue() error = %v, want %s (SQL %q params %#v)",
							err, ErrMultipleKeys, sql, params)
					}
					if len(params) != 0 {
						t.Fatalf("params = %#v, want none", params)
					}
				})
			}

			for _, logic := range conditionCases {
				t.Run("condition "+logic, func(t *testing.T) {
					if _, err := tr.TranspileCondition(logic); !IsErrorCode(err, ErrMultipleKeys) {
						t.Fatalf("TranspileCondition() error = %v, want %s", err, ErrMultipleKeys)
					}
					sql, params, err := tr.TranspileParameterizedCondition(logic)
					if !IsErrorCode(err, ErrMultipleKeys) {
						t.Fatalf("TranspileParameterizedCondition() error = %v, want %s (SQL %q params %#v)",
							err, ErrMultipleKeys, sql, params)
					}
					if len(params) != 0 {
						t.Fatalf("params = %#v, want none", params)
					}
				})
			}
		})
	}
}

func TestTranspileValue_ReduceAggregatePatternsRejectMultiKeyObjectsAllDialects(t *testing.T) {
	t.Parallel()

	tests := []string{
		`{"reduce":[{"var":"arr"},{"+":[{"var":"accumulator"},{"var":"current"}],"bad":[]},0]}`,
		`{"reduce":[{"var":"arr"},{"+":[{"var":"accumulator","bad":[]},{"var":"current"}]},0]}`,
		`{"reduce":[{"var":"arr"},{"+":[{"var":"accumulator"},{"var":"current","bad":[]}]},0]}`,
		`{"reduce":[{"var":"arr"},{"min":[{"var":"accumulator"},{"var":"current"}],"bad":[]},0]}`,
		`{"reduce":[{"var":"arr"},{"max":[{"var":"accumulator"},{"var":"current","bad":[]}]},0]}`,
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, logic := range tests {
				t.Run(logic, func(t *testing.T) {
					if _, err := tr.TranspileValue(logic); !IsErrorCode(err, ErrMultipleKeys) {
						t.Fatalf("TranspileValue() error = %v, want %s", err, ErrMultipleKeys)
					}
					sql, params, err := tr.TranspileParameterizedValue(logic)
					if !IsErrorCode(err, ErrMultipleKeys) {
						t.Fatalf("TranspileParameterizedValue() error = %v, want %s (SQL %q params %#v)",
							err, ErrMultipleKeys, sql, params)
					}
					if len(params) != 0 {
						t.Fatalf("params = %#v, want none", params)
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
		`{"or":[{"map":[{"or":[false,[]]},{"var":"missing"}]},"fallback"]}`,
		`{"or":[{"filter":[{"if":[false,[1],[]]},{"var":"missing"}]},"fallback"]}`,
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
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
					if want := testStringPlaceholder(d, 1); gotParam != want {
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

func TestArrayOperators_FoldedEmptyArraySourcesAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "items", Type: FieldTypeArray, ElementType: FieldTypeNumber},
	})

	predicateTests := []struct {
		name  string
		logic string
		want  string
	}{
		{
			name:  "all folded empty or source",
			logic: `{"all":[{"or":[false,[]]},true]}`,
			want:  "FALSE",
		},
		{
			name:  "some folded empty if source",
			logic: `{"some":[{"if":[false,[1],[]]},true]}`,
			want:  "FALSE",
		},
		{
			name:  "none folded empty or source",
			logic: `{"none":[{"or":[false,[]]},true]}`,
			want:  "TRUE",
		},
	}

	valueTests := []struct {
		name       string
		logic      string
		wantSQL    func(Dialect) string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "reduce folded empty source returns numeric initial",
			logic: `{"reduce":[{"or":[false,[]]},{"+":[{"var":"accumulator"},{"var":"current"}]},5]}`,
			wantSQL: func(Dialect) string {
				return "5"
			},
			wantParam: func(d Dialect) string {
				return testPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(5)}},
		},
		{
			name:  "reduce folded if empty source returns string initial",
			logic: `{"reduce":[{"if":[false,[1],[]]},{"cat":[{"var":"accumulator"},{"var":"current"}]},"seed"]}`,
			wantSQL: func(Dialect) string {
				return "'seed'"
			},
			wantParam: func(d Dialect) string {
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "seed"}},
		},
		{
			name:  "merge skips folded empty identity",
			logic: `{"merge":[{"or":[false,[]]},[1]]}`,
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
			wantParam: func(d Dialect) string {
				ph := testPlaceholder(d, 1)
				switch d {
				case DialectPostgreSQL:
					return fmt.Sprintf("ARRAY[%s]", ph)
				case DialectClickHouse:
					return fmt.Sprintf("arrayConcat([%s])", ph)
				default:
					return fmt.Sprintf("ARRAY_CONCAT([%s])", ph)
				}
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaRequiredModes(schema) {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}

					for _, tt := range predicateTests {
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
							if gotParam != tt.want {
								t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, tt.want)
							}
							if len(gotParams) != 0 {
								t.Fatalf("params = %#v, want none", gotParams)
							}
						})
					}

					for _, tt := range valueTests {
						t.Run(tt.name, func(t *testing.T) {
							got, err := tr.TranspileValue(tt.logic)
							if err != nil {
								t.Fatalf("TranspileValue() error = %v", err)
							}
							if want := testDuckDBUnnestSourceAliases(d, tt.wantSQL(d)); got != want {
								t.Fatalf("TranspileValue() = %q, want %q", got, want)
							}

							gotParam, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
							if err != nil {
								t.Fatalf("TranspileParameterizedValue() error = %v", err)
							}
							if want := testDuckDBUnnestSourceAliases(d, tt.wantParam(d)); gotParam != want {
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

func TestArrayOperators_RejectKnownNonArraySourcesAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{
			Name: "accounts",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "status", Type: FieldTypeString},
			},
		},
	})

	valueCases := []struct {
		name      string
		logic     string
		wantError string
	}{
		{
			name:      "map null literal source",
			logic:     `{"map":[null,{"var":""}]}`,
			wantError: "array operation on non-array value (type: null)",
		},
		{
			name:      "filter folded missing if else source",
			logic:     `{"filter":[{"if":[false,{"var":"accounts"}]},true]}`,
			wantError: "array operation on non-array value (type: null)",
		},
		{
			name:      "reduce folded missing if else source",
			logic:     `{"reduce":[{"if":[false,{"var":"accounts"}]},{"var":"accumulator"},0]}`,
			wantError: "array operation on non-array value (type: null)",
		},
		{
			name:      "map folded string source",
			logic:     `{"map":[{"if":[true,"abc",{"var":"accounts"}]},{"var":""}]}`,
			wantError: "array operation on non-array value (type: string)",
		},
		{
			name:      "map predicate-valued source",
			logic:     `{"map":[{"==":[1,1]},{"var":""}]}`,
			wantError: "array operation on non-array value (type: boolean)",
		},
	}

	predicateCases := []struct {
		name      string
		logic     string
		wantError string
	}{
		{
			name:      "all folded missing if else source",
			logic:     `{"all":[{"if":[false,{"var":"accounts"}]},true]}`,
			wantError: "array operation on non-array value (type: null)",
		},
		{
			name:      "some folded missing if else source",
			logic:     `{"some":[{"if":[false,{"var":"accounts"}]},true]}`,
			wantError: "array operation on non-array value (type: null)",
		},
		{
			name:      "none folded missing if else source",
			logic:     `{"none":[{"if":[false,{"var":"accounts"}]},true]}`,
			wantError: "array operation on non-array value (type: null)",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaRequiredModes(schema) {
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
							if sql, err := tr.TranspileValue(tt.logic); err == nil || !strings.Contains(err.Error(), tt.wantError) {
								t.Fatalf("TranspileValue() SQL = %q, error = %v, want containing %q", sql, err, tt.wantError)
							}
							paramSQL, params, err := tr.TranspileParameterizedValue(tt.logic)
							if err == nil || !strings.Contains(err.Error(), tt.wantError) {
								t.Fatalf("TranspileParameterizedValue() SQL = %q params = %#v, error = %v, want containing %q",
									paramSQL, params, err, tt.wantError)
							}
						})
					}

					for _, tt := range predicateCases {
						t.Run(tt.name, func(t *testing.T) {
							if sql, err := tr.TranspileCondition(tt.logic); err == nil || !strings.Contains(err.Error(), tt.wantError) {
								t.Fatalf("TranspileCondition() SQL = %q, error = %v, want containing %q", sql, err, tt.wantError)
							}
							paramSQL, params, err := tr.TranspileParameterizedCondition(tt.logic)
							if err == nil || !strings.Contains(err.Error(), tt.wantError) {
								t.Fatalf("TranspileParameterizedCondition() SQL = %q params = %#v, error = %v, want containing %q",
									paramSQL, params, err, tt.wantError)
							}
						})
					}
				})
			}
		})
	}
}

func TestTranspileValue_MergeCastsScalarsToArraysAllDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dialect          Dialect
		wantInline       string
		wantParamSQL     string
		wantNullInline   string
		wantNullParamSQL string
	}{
		{
			dialect:          DialectBigQuery,
			wantInline:       "ARRAY_CONCAT([1], [2])",
			wantParamSQL:     "ARRAY_CONCAT([@p1], [@p2])",
			wantNullInline:   "ARRAY_CONCAT([1, NULL], [2])",
			wantNullParamSQL: "ARRAY_CONCAT([@p1, NULL], [@p2])",
		},
		{
			dialect:          DialectSpanner,
			wantInline:       "ARRAY_CONCAT([1], [2])",
			wantParamSQL:     "ARRAY_CONCAT([@p1], [@p2])",
			wantNullInline:   "ARRAY_CONCAT([1, NULL], [2])",
			wantNullParamSQL: "ARRAY_CONCAT([@p1, NULL], [@p2])",
		},
		{
			dialect:          DialectPostgreSQL,
			wantInline:       "(ARRAY[1] || ARRAY[2])",
			wantParamSQL:     "(ARRAY[$1] || ARRAY[$2])",
			wantNullInline:   "(ARRAY[1, NULL] || ARRAY[2])",
			wantNullParamSQL: "(ARRAY[$1, NULL] || ARRAY[$2])",
		},
		{
			dialect:          DialectDuckDB,
			wantInline:       "ARRAY_CONCAT([1], [2])",
			wantParamSQL:     "ARRAY_CONCAT([$1], [$2])",
			wantNullInline:   "ARRAY_CONCAT([1, NULL], [2])",
			wantNullParamSQL: "ARRAY_CONCAT([$1, NULL], [$2])",
		},
		{
			dialect:          DialectClickHouse,
			wantInline:       "arrayConcat([1], [2])",
			wantParamSQL:     "arrayConcat([{p1:Float64}], [{p2:Float64}])",
			wantNullInline:   "arrayConcat([1, NULL], [2])",
			wantNullParamSQL: "arrayConcat([{p1:Float64}, NULL], [{p2:Float64}])",
		},
	}

	for _, tt := range tests {
		t.Run(tt.dialect.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(tt.dialect, emptyTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			got, err := tr.TranspileValue(`{"merge":[1,[2]]}`)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if got != tt.wantInline {
				t.Fatalf("TranspileValue() = %q, want %q", got, tt.wantInline)
			}

			gotParam, gotParams, err := tr.TranspileParameterizedValue(`{"merge":[1,[2]]}`)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			if gotParam != tt.wantParamSQL {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, tt.wantParamSQL)
			}
			if want := []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: float64(2)}}; !reflect.DeepEqual(gotParams, want) {
				t.Fatalf("params = %#v, want %#v", gotParams, want)
			}

			gotNull, err := tr.TranspileValue(`{"merge":[[1,null],2]}`)
			if err != nil {
				t.Fatalf("TranspileValue(null-containing merge) error = %v", err)
			}
			if gotNull != tt.wantNullInline {
				t.Fatalf("TranspileValue(null-containing merge) = %q, want %q", gotNull, tt.wantNullInline)
			}

			gotNullParam, gotNullParams, err := tr.TranspileParameterizedValue(`{"merge":[[1,null],2]}`)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue(null-containing merge) error = %v", err)
			}
			if gotNullParam != tt.wantNullParamSQL {
				t.Fatalf("TranspileParameterizedValue(null-containing merge) = %q, want %q", gotNullParam, tt.wantNullParamSQL)
			}
			if want := []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: float64(2)}}; !reflect.DeepEqual(gotNullParams, want) {
				t.Fatalf("null-containing merge params = %#v, want %#v", gotNullParams, want)
			}

			if _, err := tr.TranspileValue(`{"merge":[1,["x"]]}`); err == nil ||
				!strings.Contains(err.Error(), "incompatible element types") {
				t.Fatalf("TranspileValue(incompatible merge) error = %v, want incompatible element type error", err)
			}
			if sql, params, err := tr.TranspileParameterizedValue(`{"merge":[1,["x"]]}`); err == nil ||
				!strings.Contains(err.Error(), "incompatible element types") {
				t.Fatalf("TranspileParameterizedValue(incompatible merge) SQL = %q params %#v error = %v, want incompatible element type error",
					sql, params, err)
			}

			dynamicIncompatibleMerge := `{"merge":[{"if":[true,[1],[2]]},"x"]}`
			if sql, err := tr.TranspileValue(dynamicIncompatibleMerge); err == nil ||
				!strings.Contains(err.Error(), "incompatible element types") {
				t.Fatalf("TranspileValue(dynamic incompatible merge) SQL = %q error = %v, want incompatible element type error",
					sql, err)
			}
			if sql, params, err := tr.TranspileParameterizedValue(dynamicIncompatibleMerge); err == nil ||
				!strings.Contains(err.Error(), "incompatible element types") {
				t.Fatalf("TranspileParameterizedValue(dynamic incompatible merge) SQL = %q params %#v error = %v, want incompatible element type error",
					sql, params, err)
			}
		})
	}
}

func TestTranspileValue_EmptyArrayUnaryAndReduceShortCircuitAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeNumber},
	})
	schemaRequiredModes := schemaRequiredModes(schema)

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
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "fallback"}},
		},
		{
			name:    "reduce truthy initial skips invalid reducer and wins or",
			logic:   `{"or":[{"reduce":[[],{"var":"missing"},"seed"]},"fallback"]}`,
			wantSQL: "'seed'",
			wantParam: func(d Dialect) string {
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "seed"}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaRequiredModes {
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
		name                string
		logic               string
		rejectPostgres      bool
		rejectArrayElements bool
		wantSQL             func(Dialect) string
		wantParamSQL        func(Dialect) string
		wantParams          []QueryParam
	}{
		{
			name:           "merge zero arguments",
			logic:          `{"merge":[]}`,
			rejectPostgres: true,
			wantSQL: func(Dialect) string {
				return "[]"
			},
			wantParamSQL: func(Dialect) string {
				return "[]"
			},
		},
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
			name:                "nested empty array literal",
			logic:               `[[]]`,
			rejectPostgres:      true,
			rejectArrayElements: true,
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
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "init"}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := tr.TranspileValue(tt.logic)
					gotParam, gotParams, paramErr := tr.TranspileParameterizedValue(tt.logic)
					if (d == DialectPostgreSQL && tt.rejectPostgres) ||
						(tt.rejectArrayElements && dialectRejectsArrayLiteralElements(d)) {
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
					if want := testDuckDBUnnestSourceAliases(d, tt.wantSQL(d)); got != want {
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

func TestTranspileValue_ReduceTruthinessUsesInferredTypeAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray, ElementType: FieldTypeNumber},
	})

	reduceSumSQL := func(d Dialect, initial string) string {
		if d == DialectClickHouse {
			return fmt.Sprintf("%s + coalesce(arrayReduce('sum', arr), 0)", initial)
		}
		return testDuckDBUnnestSourceAliases(d, fmt.Sprintf("%s + COALESCE((SELECT SUM(elem) FROM UNNEST(arr) AS elem), 0)", initial))
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
					numericTruthiness(reduce), testStringPlaceholder(d, 2), testStringPlaceholder(d, 3))
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

			for _, mode := range schemaRequiredModes(schema) {
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
							if want := testDuckDBUnnestSourceAliases(d, tt.wantSQL(d)); got != want {
								t.Fatalf("TranspileValue() = %q, want %q", got, want)
							}
							if strings.Contains(got, "!= FALSE") || strings.Contains(got, "!= ''") {
								t.Fatalf("TranspileValue() used mixed-type truthiness for numeric reduce: %s", got)
							}

							gotParam, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
							if err != nil {
								t.Fatalf("TranspileParameterizedValue() error = %v", err)
							}
							if want := testDuckDBUnnestSourceAliases(d, tt.wantParam(d)); gotParam != want {
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

func TestTranspileValue_ReduceAggregateCoercesPredicateTermsAllDialects(t *testing.T) {
	t.Parallel()

	logic := `{"reduce":[{"var":"items"},{"+":[{"var":"accumulator"},{">":[{"var":"current.price"},0]}]},0]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			got, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if !strings.Contains(got, "THEN 1 ELSE 0") {
				t.Fatalf("TranspileValue() did not coerce predicate aggregate term to numeric SQL: %s", got)
			}
			if strings.Contains(got, "THEN TRUE ELSE FALSE") {
				t.Fatalf("TranspileValue() kept boolean predicate value inside aggregate term: %s", got)
			}

			gotParam, params, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			if !strings.Contains(gotParam, "THEN 1 ELSE 0") {
				t.Fatalf("TranspileParameterizedValue() did not coerce predicate aggregate term to numeric SQL: %s", gotParam)
			}
			if strings.Contains(gotParam, "THEN TRUE ELSE FALSE") {
				t.Fatalf("TranspileParameterizedValue() kept boolean predicate value inside aggregate term: %s", gotParam)
			}
			if len(params) != 2 {
				t.Fatalf("params = %#v, want initial and predicate threshold params", params)
			}
		})
	}
}

func TestTranspileValue_ReduceAggregateIgnoresLambdaNamesInQuotedLiteralsAllDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		logic string
	}{
		{
			name:  "current literal",
			logic: `{"reduce":[{"var":"items"},{"+":[{"var":"accumulator"},{"==":[{"var":"current.name"},"current"]}]},0]}`,
		},
		{
			name:  "accumulator literal",
			logic: `{"reduce":[{"var":"items"},{"+":[{"var":"accumulator"},{"==":[{"var":"current.name"},"accumulator"]}]},0]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := tr.TranspileValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if !strings.Contains(got, "THEN 1 ELSE 0") {
						t.Fatalf("TranspileValue() did not keep aggregate predicate optimization: %s", got)
					}

					gotParam, params, err := tr.TranspileParameterizedValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if !strings.Contains(gotParam, "THEN 1 ELSE 0") {
						t.Fatalf("TranspileParameterizedValue() did not keep aggregate predicate optimization: %s", gotParam)
					}
					if len(params) != 2 {
						t.Fatalf("params = %#v, want initial and comparison literal params", params)
					}
				})
			}
		})
	}
}

func TestTranspileValue_ReduceMinMaxAggregateIncludesInitialAllDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		logic      string
		initial    float64
		wantSQL    func(Dialect, string) string
		wantParams []QueryParam
	}{
		{
			name:    "min includes non-winning initial",
			logic:   `{"reduce":[{"var":"values"},{"min":[{"var":"accumulator"},{"var":"current"}]},999999]}`,
			initial: 999999,
			wantSQL: func(d Dialect, initial string) string {
				if d == DialectClickHouse {
					return fmt.Sprintf("CASE WHEN length(values) > 0 THEN least(%s, coalesce(arrayReduce('min', values), %s)) ELSE %s END", initial, initial, initial)
				}
				return fmt.Sprintf("LEAST(%s, COALESCE((SELECT MIN(elem) FROM UNNEST(values) AS elem), %s))", initial, initial)
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(999999)}},
		},
		{
			name:    "max includes winning initial",
			logic:   `{"reduce":[{"var":"values"},{"max":[{"var":"accumulator"},{"var":"current"}]},100]}`,
			initial: 100,
			wantSQL: func(d Dialect, initial string) string {
				if d == DialectClickHouse {
					return fmt.Sprintf("CASE WHEN length(values) > 0 THEN greatest(%s, coalesce(arrayReduce('max', values), %s)) ELSE %s END", initial, initial, initial)
				}
				return fmt.Sprintf("GREATEST(%s, COALESCE((SELECT MAX(elem) FROM UNNEST(values) AS elem), %s))", initial, initial)
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(100)}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := tr.TranspileValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if want := testDuckDBUnnestSourceAliases(d, tt.wantSQL(d, fmt.Sprintf("%.0f", tt.initial))); got != want {
						t.Fatalf("TranspileValue() = %q, want %q", got, want)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if want := testDuckDBUnnestSourceAliases(d, tt.wantSQL(d, testPlaceholder(d, 1))); gotParam != want {
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

func TestTranspileValue_ReduceStringTruthinessUsesInferredTypeAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray, ElementType: FieldTypeString},
	})
	logic := `{"or":[{"reduce":[{"var":"arr"},{"cat":[{"var":"accumulator"},{"var":"current"}]},""]},"fallback"]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaRequiredModes(schema) {
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
					if !testSupportsGeneralReduce(d) {
						if err == nil || !strings.Contains(err.Error(), "general reduce expressions are only supported") {
							t.Fatalf("TranspileValue() = %q, error = %v, want unsupported general reduce", got, err)
						}
						gotParam, gotParams, paramErr := tr.TranspileParameterizedValue(logic)
						if paramErr == nil || !strings.Contains(paramErr.Error(), "general reduce expressions are only supported") {
							t.Fatalf("TranspileParameterizedValue() = %q params %#v, error = %v, want unsupported general reduce",
								gotParam, gotParams, paramErr)
						}
						return
					}
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

func TestTranspileValue_ReduceAccumulatorTruthinessUsesInitialTypeAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray, ElementType: FieldTypeNumber},
		{Name: "arrString", Type: FieldTypeArray, ElementType: FieldTypeString},
		{Name: "arrBool", Type: FieldTypeArray, ElementType: FieldTypeBoolean},
	})

	renderReduce := func(d Dialect, source, reducer, initial string, initialType ExpressionType) string {
		if source == "" {
			source = "arr"
		}
		if d == DialectClickHouse {
			if initialType == ExpressionTypeNumber {
				initial = fmt.Sprintf("toFloat64(%s)", initial)
			}
			return fmt.Sprintf("arrayFold((acc, elem) -> %s, %s, %s)", reducer, source, initial)
		}
		if d == DialectDuckDB {
			return fmt.Sprintf("list_reduce(%s, lambda acc, elem : %s, %s)", source, reducer, initial)
		}
		return testDuckDBUnnestSourceAliases(d, fmt.Sprintf("(SELECT %s FROM UNNEST(%s) AS elem)", reducer, source))
	}

	tests := []struct {
		name             string
		logic            string
		source           string
		wantReducer      func(initial string) string
		wantInitial      string
		wantParamReducer func(Dialect) string
		wantParamInitial func(Dialect) string
		initialType      ExpressionType
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
				return "CASE WHEN (acc IS NOT NULL AND acc != 0) THEN acc ELSE elem END"
			},
			wantParamInitial: func(d Dialect) string { return testPlaceholder(d, 1) },
			initialType:      ExpressionTypeNumber,
			wantParams:       []QueryParam{{Name: "p1", Value: float64(0)}},
			forbidden:        []string{"!= FALSE", "!= ''"},
		},
		{
			name:  "or tests defaulted numeric accumulator with numeric truthiness",
			logic: `{"reduce":[{"var":"arr"},{"or":[{"var":["accumulator",0]},{"var":"current"}]},0]}`,
			wantReducer: func(initial string) string {
				acc := fmt.Sprintf("COALESCE(%s, 0)", initial)
				return fmt.Sprintf("CASE WHEN (%s IS NOT NULL AND %s != 0) THEN %s ELSE elem END", acc, acc, acc)
			},
			wantInitial: "0",
			wantParamReducer: func(d Dialect) string {
				acc := fmt.Sprintf("COALESCE(acc, %s)", testPlaceholder(d, 2))
				return fmt.Sprintf("CASE WHEN (%s IS NOT NULL AND %s != 0) THEN %s ELSE elem END", acc, acc, acc)
			},
			wantParamInitial: func(d Dialect) string { return testPlaceholder(d, 1) },
			initialType:      ExpressionTypeNumber,
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(0)},
			},
			forbidden: []string{"!= FALSE", "!= ''"},
		},
		{
			name:   "if tests string accumulator with string truthiness",
			logic:  `{"reduce":[{"var":"arrString"},{"if":[{"var":"accumulator"},{"var":"accumulator"},{"var":"current"}]},""]}`,
			source: "arrString",
			wantReducer: func(initial string) string {
				return fmt.Sprintf("CASE WHEN (%s IS NOT NULL AND %s != '') THEN %s ELSE elem END", initial, initial, initial)
			},
			wantInitial: "''",
			wantParamReducer: func(d Dialect) string {
				return "CASE WHEN (acc IS NOT NULL AND acc != '') THEN acc ELSE elem END"
			},
			wantParamInitial: func(d Dialect) string { return testStringPlaceholder(d, 1) },
			initialType:      ExpressionTypeString,
			wantParams:       []QueryParam{{Name: "p1", Value: ""}},
			forbidden:        []string{"!= FALSE", "!= 0"},
		},
		{
			name:   "if tests boolean accumulator with boolean truthiness",
			logic:  `{"reduce":[{"var":"arrBool"},{"if":[{"var":"accumulator"},{"var":"accumulator"},{"var":"current"}]},false]}`,
			source: "arrBool",
			wantReducer: func(initial string) string {
				return fmt.Sprintf("CASE WHEN %s IS TRUE THEN %s ELSE elem END", initial, initial)
			},
			wantInitial: "FALSE",
			wantParamReducer: func(Dialect) string {
				return "CASE WHEN acc IS TRUE THEN acc ELSE elem END"
			},
			wantParamInitial: func(Dialect) string { return "FALSE" },
			initialType:      ExpressionTypeBoolean,
			forbidden:        []string{"!= FALSE", "!= 0", "!= ''"},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaRequiredModes(schema) {
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
							if !testSupportsGeneralReduce(d) {
								if err == nil || !strings.Contains(err.Error(), "general reduce expressions are only supported") {
									t.Fatalf("TranspileValue() = %q, error = %v, want unsupported general reduce", got, err)
								}
								gotParam, gotParams, paramErr := tr.TranspileParameterizedValue(tt.logic)
								if paramErr == nil || !strings.Contains(paramErr.Error(), "general reduce expressions are only supported") {
									t.Fatalf("TranspileParameterizedValue() = %q params %#v, error = %v, want unsupported general reduce",
										gotParam, gotParams, paramErr)
								}
								return
							}
							if err != nil {
								t.Fatalf("TranspileValue() error = %v", err)
							}
							want := renderReduce(d, tt.source, tt.wantReducer("acc"), tt.wantInitial, tt.initialType)
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
							wantParam := renderReduce(d, tt.source, tt.wantParamReducer(d), tt.wantParamInitial(d), tt.initialType)
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

	tests := []struct {
		name  string
		logic string
	}{
		{
			name:  "field initial with empty schema",
			logic: `{"reduce":[{"var":"arr"},{"or":[{"var":"accumulator"},{"var":"current"}]},{"var":"seed"}]}`,
		},
		{
			name:  "defaulted field initial with empty schema",
			logic: `{"reduce":[{"var":"arr"},{"or":[{"var":"accumulator"},"x"]},{"var":["seed",""]}]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			tr.SetSchema(emptyTestSchema())

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, valueErr := tr.TranspileValue(tt.logic)
					if valueErr == nil || !strings.Contains(valueErr.Error(), "field 'seed' is not defined in schema") {
						t.Fatalf("TranspileValue() = %q, error = %v, want seed schema validation error", got, valueErr)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
					if err == nil || !strings.Contains(err.Error(), "field 'seed' is not defined in schema") {
						t.Fatalf("TranspileParameterizedValue() = %q params %#v, error = %v, want seed schema validation error",
							gotParam, gotParams, err)
					}
				})
			}
		})
	}
}

func TestOrderingComparisonRejectsArrayValuedExpressionsAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "numbers", Type: FieldTypeArray, ElementType: FieldTypeNumber},
	})
	logic := `{">":[{"map":[{"var":"numbers"},{"*":[{"var":""},2]}]},10]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			if got, err := tr.TranspileCondition(logic); err == nil || !strings.Contains(err.Error(), "array-valued expression") {
				t.Fatalf("TranspileCondition() = %q, error = %v, want array-valued expression error", got, err)
			}
			if got, params, err := tr.TranspileParameterizedCondition(logic); err == nil || !strings.Contains(err.Error(), "array-valued expression") {
				t.Fatalf("TranspileParameterizedCondition() = %q params %#v, error = %v, want array-valued expression error", got, params, err)
			}
		})
	}
}

func TestTranspileValue_ReduceAccumulatorTruthinessUsesTypedCustomInitialAllDialects(t *testing.T) {
	t.Parallel()

	logic := `{"reduce":[{"var":"arr"},{"or":[{"var":"accumulator"},1]},{"zero":[]}]}`

	renderReduce := func(d Dialect, reducer string) string {
		if d == DialectClickHouse {
			return fmt.Sprintf("arrayFold((acc, elem) -> %s, arr, toFloat64(0))", reducer)
		}
		if d == DialectDuckDB {
			return fmt.Sprintf("list_reduce(arr, lambda acc, elem : %s, 0)", reducer)
		}
		return testDuckDBUnnestSourceAliases(d, fmt.Sprintf("(SELECT %s FROM UNNEST(arr) AS elem)", reducer))
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			err = tr.RegisterOperatorFunc("zero", func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 0 {
					return OperatorResult{}, fmt.Errorf("zero requires no arguments")
				}
				return ValueSQL("0", ExpressionTypeNumber), nil
			})
			if err != nil {
				t.Fatalf("RegisterOperatorFunc() error = %v", err)
			}

			got, err := tr.TranspileValue(logic)
			if !testSupportsGeneralReduce(d) {
				if err == nil || !strings.Contains(err.Error(), "general reduce expressions are only supported") {
					t.Fatalf("TranspileValue() = %q, error = %v, want unsupported general reduce", got, err)
				}
				gotParam, gotParams, paramErr := tr.TranspileParameterizedValue(logic)
				if paramErr == nil || !strings.Contains(paramErr.Error(), "general reduce expressions are only supported") {
					t.Fatalf("TranspileParameterizedValue() = %q params %#v, error = %v, want unsupported general reduce",
						gotParam, gotParams, paramErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			wantReducer := "CASE WHEN (acc IS NOT NULL AND acc != 0) THEN acc ELSE 1 END"
			if want := renderReduce(d, wantReducer); got != want {
				t.Fatalf("TranspileValue() = %q, want %q", got, want)
			}

			gotParam, gotParams, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			placeholder := testPlaceholder(d, 1)
			wantParamReducer := fmt.Sprintf("CASE WHEN (acc IS NOT NULL AND acc != 0) THEN acc ELSE %s END", placeholder)
			if want := renderReduce(d, wantParamReducer); gotParam != want {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, want)
			}
			wantParams := []QueryParam{{Name: "p1", Value: float64(1)}}
			if !reflect.DeepEqual(gotParams, wantParams) {
				t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
			}
		})
	}
}

func TestTranspileValue_ReduceAccumulatorTruthinessUsesSchemaInitialTypeAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray, ElementType: FieldTypeNumber},
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

			reducer := "CASE WHEN (acc IS NOT NULL AND acc != 0) THEN acc ELSE elem END"
			want := fmt.Sprintf("(SELECT %s FROM UNNEST(arr) AS elem)", reducer)
			switch d {
			case DialectClickHouse:
				want = fmt.Sprintf("arrayFold((acc, elem) -> %s, arr, toFloat64(seed))", reducer)
			case DialectDuckDB:
				want = fmt.Sprintf("list_reduce(arr, lambda acc, elem : %s, seed)", reducer)
			default:
				want = testDuckDBUnnestSourceAliases(d, want)
			}

			got, err := tr.TranspileValue(logic)
			if !testSupportsGeneralReduce(d) {
				if err == nil || !strings.Contains(err.Error(), "general reduce expressions are only supported") {
					t.Fatalf("TranspileValue() = %q, error = %v, want unsupported general reduce", got, err)
				}
				gotParam, gotParams, paramErr := tr.TranspileParameterizedValue(logic)
				if paramErr == nil || !strings.Contains(paramErr.Error(), "general reduce expressions are only supported") {
					t.Fatalf("TranspileParameterizedValue() = %q params %#v, error = %v, want unsupported general reduce",
						gotParam, gotParams, paramErr)
				}
				return
			}
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
	tr, err := NewTranspiler(DialectPostgreSQL, defaultTestSchema())
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
		{Name: "arr", Type: FieldTypeArray, ElementType: FieldTypeNumber},
	})
	schemaRequiredModes := schemaRequiredModes(schema)

	for _, mode := range schemaRequiredModes {
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
	schemaRequiredModes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-required", schema: schema},
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
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "fallback"}},
		},
		{
			name:  "or treats extreme underflowed number as falsy",
			logic: `{"or":[1e-9999,"fallback"]}`,
			want:  "'fallback'",
			wantParam: func(d Dialect) string {
				return testStringPlaceholder(d, 1)
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
				return testStringPlaceholder(d, 1)
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

			for _, mode := range schemaRequiredModes {
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
					testStringPlaceholder(d, 1),
					testStringPlaceholder(d, 2),
					testStringPlaceholder(d, 3),
					testStringPlaceholder(d, 4),
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
					testStringPlaceholder(d, 1),
					testStringPlaceholder(d, 2),
					testStringPlaceholder(d, 3),
					testStringPlaceholder(d, 4),
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
					testStringPlaceholder(d, 1),
					testPlaceholder(d, 2),
					testStringPlaceholder(d, 3),
					testStringPlaceholder(d, 4),
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

			for _, mode := range schemaRequiredModes(schema) {
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
	schemaRequiredModes := schemaRequiredModes(schema)

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

			for _, mode := range schemaRequiredModes {
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
					if want := testStringPlaceholder(d, 1); gotParam != want {
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
					if want := testStringPlaceholder(d, 1); gotParam != want {
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
							if want := testStringPlaceholder(d, 1); gotParam != want {
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

func TestTranspileValue_RejectsReturnedNativeNonFiniteFloatsAllDialectsSchemaRequired(t *testing.T) {
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

			for _, mode := range schemaRequiredModes(schema) {
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

			tr, err := NewTranspiler(d, defaultTestSchema())
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
					} else {
						want = testDuckDBUnnestSourceAliases(d, want)
					}
					if got != want {
						t.Fatalf("TranspileValue() = %q, want %q", got, want)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					placeholder := testStringPlaceholder(d, 1)
					wantParam := fmt.Sprintf("ARRAY(SELECT %s FROM UNNEST(arr) AS elem)", placeholder)
					if d == DialectClickHouse {
						wantParam = fmt.Sprintf("arrayMap(elem -> %s, arr)", placeholder)
					} else {
						wantParam = testDuckDBUnnestSourceAliases(d, wantParam)
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
			wantParam: "arrayMap(elem -> elem, [amount, {p1:Float64}])",
		},
	}

	for _, tt := range tests {
		t.Run(tt.dialect.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(tt.dialect, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			got, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			want := testDuckDBUnnestSourceAliases(tt.dialect, tt.want)
			if got != want {
				t.Fatalf("TranspileValue() = %q, want %q", got, want)
			}

			paramSQL, params, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			wantParam := testDuckDBUnnestSourceAliases(tt.dialect, tt.wantParam)
			if paramSQL != wantParam {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", paramSQL, wantParam)
			}
			if !reflect.DeepEqual(params, []QueryParam{{Name: "p1", Value: float64(5)}}) {
				t.Fatalf("params = %#v, want p1=5", params)
			}
		})
	}
}

func TestTranspileValue_MapTransformationArrayLiteralAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray, ElementType: FieldTypeNumber},
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
		return testDuckDBUnnestSourceAliases(d, fmt.Sprintf("ARRAY(SELECT %s FROM UNNEST(arr) AS elem)", transformation))
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
			logic: `{"map":[{"var":"arr"},[{"var":""}]]}`,
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
			logic: `{"map":[{"var":"arr"},[{"or":[0,{"var":""}]}]]}`,
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
			logic: `{"map":[{"var":"arr"},[{"var":["","fallback"]}]]}`,
			wantSQL: func(d Dialect) string {
				return mapSQL(d, arrayLiteral(d, "COALESCE(elem, 'fallback')"))
			},
			wantParam: func(d Dialect) string {
				return mapSQL(d, arrayLiteral(d, fmt.Sprintf("COALESCE(elem, %s)", testStringPlaceholder(d, 1))))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "fallback"}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaRequiredModes(schema) {
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
							if testRejectsNestedArrayValues(d) {
								expectValueAndParamErrorContains(t, tr, tt.logic, nestedArrayErrorFragment(d))
								return
							}

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
	tr, err := NewTranspiler(DialectBigQuery, defaultTestSchema())
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

func TestTranspileValue_NumericOperandsCoercePredicatesAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray, ElementType: FieldTypeNumber},
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "x", Type: FieldTypeNumber},
	})

	arrayMapSQL := func(d Dialect, expr string) string {
		if d == DialectClickHouse {
			return fmt.Sprintf("arrayMap(elem -> %s, arr)", expr)
		}
		return testDuckDBUnnestSourceAliases(d, fmt.Sprintf("ARRAY(SELECT %s FROM UNNEST(arr) AS elem)", expr))
	}

	tests := []struct {
		name       string
		logic      string
		wantSQL    func(Dialect) string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "comparison predicate addition",
			logic: `{"+":[{">":[{"var":"x"},1]},2]}`,
			wantSQL: func(Dialect) string {
				return "((CASE WHEN x > 1 THEN 1 ELSE 0 END) + 2)"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("((CASE WHEN x > %s THEN 1 ELSE 0 END) + %s)",
					testPlaceholder(d, 1), testPlaceholder(d, 2))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: float64(2)}},
		},
		{
			name:  "boolean literal addition",
			logic: `{"+":[true,false,2]}`,
			wantSQL: func(Dialect) string {
				return "(1 + 0 + 2)"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("(1 + 0 + %s)", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(2)}},
		},
		{
			name:  "custom predicate addition",
			logic: `{"+":[{"isPositive":[{"var":"x"}]},2]}`,
			wantSQL: func(Dialect) string {
				return "((CASE WHEN x > 0 THEN 1 ELSE 0 END) + 2)"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("((CASE WHEN x > 0 THEN 1 ELSE 0 END) + %s)", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(2)}},
		},
		{
			name:  "custom boolean value addition",
			logic: `{"+":[{"boolValue":[]},2]}`,
			wantSQL: func(Dialect) string {
				return "((CASE WHEN flag IS TRUE THEN 1 ELSE 0 END) + 2)"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("((CASE WHEN flag IS TRUE THEN 1 ELSE 0 END) + %s)", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(2)}},
		},
		{
			name:  "custom boolean case value addition",
			logic: `{"+":[{"boolCase":[]},2]}`,
			wantSQL: func(Dialect) string {
				return "((CASE WHEN (CASE WHEN flag IS TRUE THEN TRUE ELSE FALSE END) IS TRUE THEN 1 ELSE 0 END) + 2)"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("((CASE WHEN (CASE WHEN flag IS TRUE THEN TRUE ELSE FALSE END) IS TRUE THEN 1 ELSE 0 END) + %s)",
					testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(2)}},
		},
		{
			name:  "array scoped comparison predicate addition",
			logic: `{"map":[{"var":"arr"},{"+":[{">":[{"var":""},1]},2]}]}`,
			wantSQL: func(d Dialect) string {
				return arrayMapSQL(d, "((CASE WHEN elem > 1 THEN 1 ELSE 0 END) + 2)")
			},
			wantParam: func(d Dialect) string {
				return arrayMapSQL(d, fmt.Sprintf("((CASE WHEN elem > %s THEN 1 ELSE 0 END) + %s)",
					testPlaceholder(d, 1), testPlaceholder(d, 2)))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: float64(2)}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaRequiredModes(schema) {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}
					if err := tr.RegisterOperatorFunc("isPositive", func(_ string, args []OperatorArg) (OperatorResult, error) {
						if len(args) != 1 {
							return OperatorResult{}, fmt.Errorf("isPositive requires exactly 1 argument")
						}
						return PredicateSQL(fmt.Sprintf("%s > 0", args[0].SQL)), nil
					}); err != nil {
						t.Fatalf("RegisterOperatorFunc() error = %v", err)
					}
					if err := tr.RegisterOperatorFunc("boolValue", func(_ string, args []OperatorArg) (OperatorResult, error) {
						if len(args) != 0 {
							return OperatorResult{}, fmt.Errorf("boolValue requires no arguments")
						}
						return ValueSQL("flag", ExpressionTypeBoolean), nil
					}); err != nil {
						t.Fatalf("RegisterOperatorFunc() error = %v", err)
					}
					if err := tr.RegisterOperatorFunc("boolCase", func(_ string, args []OperatorArg) (OperatorResult, error) {
						if len(args) != 0 {
							return OperatorResult{}, fmt.Errorf("boolCase requires no arguments")
						}
						return ValueSQL("CASE WHEN flag IS TRUE THEN TRUE ELSE FALSE END", ExpressionTypeBoolean), nil
					}); err != nil {
						t.Fatalf("RegisterOperatorFunc() error = %v", err)
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
	want = "CONCAT(COALESCE(CASE WHEN flag IS TRUE THEN 'yes' ELSE 'no' END, ''))"
	if got != want {
		t.Fatalf("TranspileValue() nested string if = %q, want %q", got, want)
	}

	got, err = tr.TranspileValue(`{"cat":[{"if":[{"var":"flag"},true,false]}]}`)
	if err != nil {
		t.Fatalf("TranspileValue() nested boolean if error = %v", err)
	}
	want = "CONCAT(CASE WHEN (CASE WHEN flag IS TRUE THEN TRUE ELSE FALSE END) IS TRUE THEN 'true' WHEN (CASE WHEN flag IS TRUE THEN TRUE ELSE FALSE END) IS FALSE THEN 'false' ELSE '' END)"
	if got != want {
		t.Fatalf("TranspileValue() nested boolean if = %q, want %q", got, want)
	}
}

func TestTranspileValue_NotNormalizesNullablePredicatesAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{{Name: "col", Type: FieldTypeNumber}})

	tests := []struct {
		name       string
		logic      string
		want       string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "not comparison predicate",
			logic: `{"!":{"==":[{"var":"col"},1]}}`,
			want:  "CASE WHEN NOT (CASE WHEN col = 1 THEN TRUE ELSE FALSE END) THEN TRUE ELSE FALSE END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN NOT (CASE WHEN col = %s THEN TRUE ELSE FALSE END) THEN TRUE ELSE FALSE END",
					testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
		{
			name:  "double not comparison predicate",
			logic: `{"!!":{"==":[{"var":"col"},1]}}`,
			want:  "CASE WHEN col = 1 THEN TRUE ELSE FALSE END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN col = %s THEN TRUE ELSE FALSE END", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, cfg := range []struct {
				name   string
				schema *Schema
			}{
				{name: "schema-required", schema: schema},
			} {
				t.Run(cfg.name, func(t *testing.T) {
					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  cfg.schema,
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

func TestTranspileValue_TruthinessOnlyExpressionsAllowMixedValueBranchesAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
	})

	tests := []struct {
		name       string
		logic      string
		want       string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "if condition folds mixed-type or truthiness",
			logic: `{"if":[{"or":[{"var":"flag"},"x"]},"yes","no"]}`,
			want:  "'yes'",
			wantParam: func(d Dialect) string {
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "yes"}},
		},
		{
			name:  "double not accepts mixed-type if truthiness",
			logic: `{"!!":{"if":[{"var":"flag"},"x",0]}}`,
			want:  "CASE WHEN (CASE WHEN flag IS TRUE THEN TRUE ELSE FALSE END) THEN TRUE ELSE FALSE END",
			wantParam: func(Dialect) string {
				return "CASE WHEN (CASE WHEN flag IS TRUE THEN TRUE ELSE FALSE END) THEN TRUE ELSE FALSE END"
			},
			wantParams: []QueryParam{},
		},
	}

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
					if want := testDuckDBUnnestSourceAliases(d, tt.wantParam(d)); gotParam != want {
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
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "ok"}},
		},
		{
			name:  "schema strict mismatch skips unreachable if branch",
			logic: `{"if":[{"===":[{"var":"code"},5]},{"var":"missing"},"ok"]}`,
			want:  "'ok'",
			wantParam: func(d Dialect) string {
				return testStringPlaceholder(d, 1)
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
				return testStringPlaceholder(d, 1)
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
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "ok"}},
		},
		{
			name:    "false test skips invalid then branch",
			logic:   `{"if":[false,{"var":"bad-name"},"ok"]}`,
			wantSQL: "'ok'",
			wantParam: func(d Dialect) string {
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "ok"}},
		},
		{
			name:    "dynamic test followed by true test folds to else",
			logic:   `{"if":[{">":[{"var":"x"},0]},"positive",true,"fallback",{"var":"bad-name"}]}`,
			wantSQL: "CASE WHEN x > 0 THEN 'positive' ELSE 'fallback' END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN x > %s THEN %s ELSE %s END",
					testPlaceholder(d, 1), testStringPlaceholder(d, 2), testStringPlaceholder(d, 3))
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
			tr, err := NewTranspiler(d, defaultTestSchema())
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
				return fmt.Sprintf("CASE WHEN (name IS NOT NULL AND name != '') THEN name ELSE %s END", testStringPlaceholder(d, 1))
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
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "x"}},
		},
		{
			name:    "and skips literal true predicate",
			logic:   `{"and":[{"==":[1,1]},"x"]}`,
			wantSQL: "'x'",
			wantParam: func(d Dialect) string {
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "x"}},
		},
		{
			name:    "or skips literal false ordering predicate",
			logic:   `{"or":[{">":[1,2]},"x"]}`,
			wantSQL: "'x'",
			wantParam: func(d Dialect) string {
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "x"}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d, defaultTestSchema())
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

func TestTranspileValue_EmptySchemaFieldTruthinessRejectedAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "nickname", Type: FieldTypeString},
		{Name: "flag", Type: FieldTypeBoolean},
	})

	schemaRequiredValueCases := []string{
		`{"or":[{"var":"nickname"},"unknown"]}`,
		`{"and":[{"var":"nickname"},"known"]}`,
		`{"if":[{"var":"nickname"},"yes","no"]}`,
	}
	schemaRequiredConditionCases := []string{
		`{"!!":{"var":"nickname"}}`,
		`{"!":{"var":"nickname"}}`,
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			schemaRequired, err := NewTranspiler(d, emptyTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			for _, logic := range schemaRequiredValueCases {
				t.Run("schema-required/value/"+logic, func(t *testing.T) {
					if _, valueErr := schemaRequired.TranspileValue(logic); valueErr == nil ||
						!strings.Contains(valueErr.Error(), "field 'nickname' is not defined in schema") {
						t.Fatalf("TranspileValue() error = %v, want nickname schema validation error", valueErr)
					}
					sql, params, paramErr := schemaRequired.TranspileParameterizedValue(logic)
					if paramErr == nil || !strings.Contains(paramErr.Error(), "field 'nickname' is not defined in schema") {
						t.Fatalf("TranspileParameterizedValue() error = %v, want nickname schema validation error (SQL %q params %#v)",
							paramErr, sql, params)
					}
				})
			}
			for _, logic := range schemaRequiredConditionCases {
				t.Run("schema-required/condition/"+logic, func(t *testing.T) {
					if _, conditionErr := schemaRequired.TranspileCondition(logic); conditionErr == nil ||
						!strings.Contains(conditionErr.Error(), "field 'nickname' is not defined in schema") {
						t.Fatalf("TranspileCondition() error = %v, want nickname schema validation error", conditionErr)
					}
					sql, params, paramErr := schemaRequired.TranspileParameterizedCondition(logic)
					if paramErr == nil || !strings.Contains(paramErr.Error(), "field 'nickname' is not defined in schema") {
						t.Fatalf("TranspileParameterizedCondition() error = %v, want nickname schema validation error (SQL %q params %#v)",
							paramErr, sql, params)
					}
				})
			}

			typedSchemaTr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			got, err := typedSchemaTr.TranspileValue(`{"or":[{"var":"nickname"},"unknown"]}`)
			if err != nil {
				t.Fatalf("schema-required TranspileValue(or) error = %v", err)
			}
			if want := "CASE WHEN (nickname IS NOT NULL AND nickname != '') THEN nickname ELSE 'unknown' END"; got != want {
				t.Fatalf("schema-required TranspileValue(or) = %q, want %q", got, want)
			}

			gotParam, gotParams, err := typedSchemaTr.TranspileParameterizedValue(`{"or":[{"var":"nickname"},"unknown"]}`)
			if err != nil {
				t.Fatalf("schema-required TranspileParameterizedValue(or) error = %v", err)
			}
			if want := fmt.Sprintf("CASE WHEN (nickname IS NOT NULL AND nickname != '') THEN nickname ELSE %s END", testStringPlaceholder(d, 1)); gotParam != want {
				t.Fatalf("schema-required TranspileParameterizedValue(or) = %q, want %q", gotParam, want)
			}
			if want := []QueryParam{{Name: "p1", Value: "unknown"}}; !reflect.DeepEqual(gotParams, want) {
				t.Fatalf("schema-required params = %#v, want %#v", gotParams, want)
			}

			got, err = typedSchemaTr.TranspileValue(`{"if":[{"var":"flag"},"yes","no"]}`)
			if err != nil {
				t.Fatalf("schema-required TranspileValue(if) error = %v", err)
			}
			if want := "CASE WHEN flag IS TRUE THEN 'yes' ELSE 'no' END"; got != want {
				t.Fatalf("schema-required TranspileValue(if) = %q, want %q", got, want)
			}

			gotCond, err := typedSchemaTr.TranspileCondition(`{"!!":{"var":"nickname"}}`)
			if err != nil {
				t.Fatalf("schema-required TranspileCondition(!!) error = %v", err)
			}
			if want := "(nickname IS NOT NULL AND nickname != '')"; gotCond != want {
				t.Fatalf("schema-required TranspileCondition(!!) = %q, want %q", gotCond, want)
			}

			gotParamCond, gotCondParams, err := typedSchemaTr.TranspileParameterizedCondition(`{"!!":{"var":"nickname"}}`)
			if err != nil {
				t.Fatalf("schema-required TranspileParameterizedCondition(!!) error = %v", err)
			}
			if want := "(nickname IS NOT NULL AND nickname != '')"; gotParamCond != want {
				t.Fatalf("schema-required TranspileParameterizedCondition(!!) = %q, want %q", gotParamCond, want)
			}
			if len(gotCondParams) != 0 {
				t.Fatalf("schema-required condition params = %#v, want none", gotCondParams)
			}
		})
	}
}

func TestTranspileValue_CatStringifiesBuiltInPredicate(t *testing.T) {
	logic := `{"cat":[{"==":[{"var":"amount"},10]}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d, defaultTestSchema())
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

func TestTranspileValue_CatNullSafeStringificationAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "name", Type: FieldTypeString},
		{Name: "amount", Type: FieldTypeNumber},
	})

	tests := []struct {
		name       string
		logic      string
		want       func(Dialect) string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "empty cat returns empty string",
			logic: `{"cat":[]}`,
			want: func(_ Dialect) string {
				return "''"
			},
			wantParam: func(_ Dialect) string {
				return "''"
			},
		},
		{
			name:  "literal null stringifies to empty",
			logic: `{"cat":[null,"y"]}`,
			want: func(_ Dialect) string {
				return "CONCAT('', 'y')"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CONCAT('', %s)", testStringPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "y"}},
		},
		{
			name:  "omitted if else stringifies to empty",
			logic: `{"cat":[{"if":[false,"x"]},"y"]}`,
			want: func(_ Dialect) string {
				return "CONCAT('', 'y')"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CONCAT('', %s)", testStringPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "y"}},
		},
		{
			name:  "nullable string field stringifies to empty",
			logic: `{"cat":[{"var":"name"},"y"]}`,
			want: func(_ Dialect) string {
				return "CONCAT(COALESCE(name, ''), 'y')"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CONCAT(COALESCE(name, ''), %s)", testStringPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "y"}},
		},
		{
			name:  "nullable number field stringifies to empty",
			logic: `{"cat":[{"var":"amount"}]}`,
			want: func(d Dialect) string {
				return fmt.Sprintf("CONCAT(COALESCE(%s, ''))", testStringCastSQL(d, "amount"))
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CONCAT(COALESCE(%s, ''))", testStringCastSQL(d, "amount"))
			},
		},
	}

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

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := tr.TranspileValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if want := tt.want(d); got != want {
						t.Fatalf("TranspileValue() = %q, want %q", got, want)
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

func TestTranspileValue_CatNullSafeBehaviorAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "name", Type: FieldTypeString},
		{Name: "amount", Type: FieldTypeNumber},
	})

	tests := []struct {
		name       string
		logic      string
		want       func(Dialect) string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "string field and literal",
			logic: `{"cat":[{"var":"name"},"-x"]}`,
			want: func(_ Dialect) string {
				return "CONCAT(COALESCE(name, ''), '-x')"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CONCAT(COALESCE(name, ''), %s)", testStringPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "-x"}},
		},
		{
			name:  "number field",
			logic: `{"cat":["#",{"var":"amount"}]}`,
			want: func(d Dialect) string {
				return fmt.Sprintf("CONCAT('#', COALESCE(%s, ''))", testStringCastSQL(d, "amount"))
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CONCAT(%s, COALESCE(%s, ''))", testStringPlaceholder(d, 1), testStringCastSQL(d, "amount"))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "#"}},
		},
		{
			name:  "boolean field",
			logic: `{"cat":[{"var":"flag"}]}`,
			want: func(_ Dialect) string {
				return "CONCAT(CASE WHEN flag IS TRUE THEN 'true' WHEN flag IS FALSE THEN 'false' ELSE '' END)"
			},
			wantParam: func(_ Dialect) string {
				return "CONCAT(CASE WHEN flag IS TRUE THEN 'true' WHEN flag IS FALSE THEN 'false' ELSE '' END)"
			},
		},
		{
			name:  "predicate operand",
			logic: `{"cat":[{"==":[{"var":"amount"},10]}]}`,
			want: func(_ Dialect) string {
				return "CONCAT(CASE WHEN amount = 10 THEN 'true' ELSE 'false' END)"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CONCAT(CASE WHEN amount = %s THEN 'true' ELSE 'false' END)", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(10)}},
		},
		{
			name:  "omitted if else",
			logic: `{"cat":[{"if":[{"==":[{"var":"amount"},10]},"yes"]},"z"]}`,
			want: func(_ Dialect) string {
				condition := "amount = 10"
				return fmt.Sprintf("CONCAT(COALESCE(CASE WHEN %s THEN 'yes' ELSE NULL END, ''), 'z')", condition)
			},
			wantParam: func(d Dialect) string {
				condition := fmt.Sprintf("amount = %s", testPlaceholder(d, 1))
				return fmt.Sprintf(
					"CONCAT(COALESCE(CASE WHEN %s THEN %s ELSE NULL END, ''), %s)",
					condition,
					testStringPlaceholder(d, 2),
					testStringPlaceholder(d, 3),
				)
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(10)},
				{Name: "p2", Value: "yes"},
				{Name: "p3", Value: "z"},
			},
		},
	}

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

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if _, err := tr.TranspileCondition(tt.logic); !IsErrorCode(err, ErrInvalidExpressionContext) {
						t.Fatalf("TranspileCondition() error = %v, want %s", err, ErrInvalidExpressionContext)
					}

					gotValue, err := tr.TranspileValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if want := tt.want(d); gotValue != want {
						t.Fatalf("TranspileValue() = %q, want %q", gotValue, want)
					}

					gotFallback, err := transpileTestExpression(tr, tt.logic)
					if err != nil {
						t.Fatalf("fallback transpile error = %v", err)
					}
					if gotFallback != gotValue {
						t.Fatalf("fallback transpile = %q, want value SQL %q", gotFallback, gotValue)
					}

					if _, _, condErr := tr.TranspileParameterizedCondition(tt.logic); !IsErrorCode(condErr, ErrInvalidExpressionContext) {
						t.Fatalf("TranspileParameterizedCondition() error = %v, want %s", condErr, ErrInvalidExpressionContext)
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

					gotParamFallback, gotFallbackParams, err := transpileParameterizedExpression(tr, tt.logic)
					if err != nil {
						t.Fatalf("fallback parameterized transpile error = %v", err)
					}
					if gotParamFallback != gotParam {
						t.Fatalf("fallback parameterized SQL = %q, want %q", gotParamFallback, gotParam)
					}
					if !reflect.DeepEqual(gotFallbackParams, gotParams) {
						t.Fatalf("fallback params = %#v, want %#v", gotFallbackParams, gotParams)
					}
				})
			}
		})
	}
}

func TestTranspileCatNullSafeCustomOperatorDeepNestingAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "first", Type: FieldTypeString},
		{Name: "last", Type: FieldTypeString},
		{Name: "amount", Type: FieldTypeNumber},
	})
	logic := `{"cat":[{"if":[{"==":[{"var":"flag"},true]},{"cat":[{"prefix":["pre-",{"var":"first"}]},{"or":[null,{"upper":[{"var":"last"}]}]}]},{"cat":[{"prefix":["pre-",{"var":"amount"}]},"-fallback"]}]},"!"]}`

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
			if regErr := registerCatDeepNestingOperators(tr); regErr != nil {
				t.Fatalf("registerCatDeepNestingOperators() error = %v", regErr)
			}

			got, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			want := catDeepNestingSQL(d, false)
			if got != want {
				t.Fatalf("TranspileValue() = %q, want %q", got, want)
			}

			gotParam, gotParams, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			wantParam := catDeepNestingSQL(d, true)
			if gotParam != wantParam {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, wantParam)
			}
			wantParams := []QueryParam{
				{Name: "p1", Value: "pre-"},
				{Name: "p2", Value: "pre-"},
				{Name: "p3", Value: "-fallback"},
				{Name: "p4", Value: "!"},
			}
			if !reflect.DeepEqual(gotParams, wantParams) {
				t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
			}
		})
	}
}

func registerCatDeepNestingOperators(tr *Transpiler) error {
	if err := tr.RegisterDialectAwareOperatorFunc("upper", func(_ string, args []OperatorArg, d Dialect) (OperatorResult, error) {
		if len(args) != 1 {
			return OperatorResult{}, fmt.Errorf("upper requires one argument")
		}
		return ValueSQL(fmt.Sprintf("UPPER(%s)", customStringArgSQL(d, args[0])), ExpressionTypeString), nil
	}); err != nil {
		return err
	}
	return tr.RegisterDialectAwareOperatorFunc("prefix", func(_ string, args []OperatorArg, d Dialect) (OperatorResult, error) {
		if len(args) != 2 {
			return OperatorResult{}, fmt.Errorf("prefix requires two arguments")
		}
		return ValueSQL(fmt.Sprintf("CONCAT(%s, %s)", args[0].SQL, customStringArgSQL(d, args[1])), ExpressionTypeString), nil
	})
}

func customStringArgSQL(d Dialect, arg OperatorArg) string {
	if arg.Kind == ExpressionKindPredicate || arg.Type == ExpressionTypeBoolean {
		return fmt.Sprintf("CASE WHEN %s THEN 'true' ELSE 'false' END", arg.SQL)
	}
	switch arg.Type {
	case ExpressionTypeNull:
		return "''"
	case ExpressionTypeBoolean:
		return fmt.Sprintf("CASE WHEN %s THEN 'true' ELSE 'false' END", arg.SQL)
	case ExpressionTypeString:
		return fmt.Sprintf("COALESCE(%s, '')", arg.SQL)
	case ExpressionTypeNumber, ExpressionTypeUnknown:
		return fmt.Sprintf("COALESCE(%s, '')", testStringCastSQL(d, arg.SQL))
	case ExpressionTypeArray:
		return arg.SQL
	case ExpressionTypeObject:
		return arg.SQL
	}
	return arg.SQL
}

func catDeepNestingSQL(d Dialect, parameterized bool) string {
	prefixOne := "'pre-'"
	prefixTwo := "'pre-'"
	fallback := "'-fallback'"
	bang := "'!'"
	if parameterized {
		prefixOne = testStringPlaceholder(d, 1)
		prefixTwo = testStringPlaceholder(d, 2)
		fallback = testStringPlaceholder(d, 3)
		bang = testStringPlaceholder(d, 4)
	}

	firstSQL := "COALESCE(first, '')"
	lastSQL := "COALESCE(last, '')"

	thenNested := fmt.Sprintf(
		"CONCAT(COALESCE(CONCAT(%s, %s), ''), COALESCE(UPPER(%s), ''))",
		prefixOne,
		firstSQL,
		lastSQL,
	)
	elseNested := fmt.Sprintf(
		"CONCAT(COALESCE(CONCAT(%s, COALESCE(%s, '')), ''), %s)",
		prefixTwo,
		testStringCastSQL(d, "amount"),
		fallback,
	)
	condition := "flag = TRUE"
	branch := fmt.Sprintf("COALESCE(CASE WHEN %s THEN %s ELSE %s END, '')", condition, thenNested, elseNested)
	return fmt.Sprintf("CONCAT(%s, %s)", branch, bang)
}

func TestTranspileValue_CatStringifiesMixedLogicalBranchesAllDialectsSchemaRequired(t *testing.T) {
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
			name:  "if stringifies mixed string boolean branches",
			logic: `{"cat":[{"if":[{"==":[{"var":"x"},1]},"yes",false]}]}`,
			want:  "CONCAT(CASE WHEN x = 1 THEN 'yes' ELSE 'false' END)",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CONCAT(CASE WHEN x = %s THEN %s ELSE 'false' END)",
					testPlaceholder(d, 1), testStringPlaceholder(d, 2))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: "yes"}},
		},
		{
			name:  "or stringifies predicate fallback",
			logic: `{"cat":[{"or":[{"==":[{"var":"x"},1]},"fallback"]}]}`,
			want:  "CONCAT(CASE WHEN x = 1 THEN CASE WHEN x = 1 THEN 'true' ELSE 'false' END ELSE 'fallback' END)",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf(
					"CONCAT(CASE WHEN x = %s THEN CASE WHEN x = %s THEN 'true' ELSE 'false' END ELSE %s END)",
					testPlaceholder(d, 1), testPlaceholder(d, 2), testStringPlaceholder(d, 3),
				)
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(1)},
				{Name: "p3", Value: "fallback"},
			},
		},
		{
			name:  "and stringifies predicate fallback",
			logic: `{"cat":[{"and":[{"==":[{"var":"x"},1]},"ok"]}]}`,
			want:  "CONCAT(CASE WHEN x = 1 THEN 'ok' ELSE CASE WHEN x = 1 THEN 'true' ELSE 'false' END END)",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf(
					"CONCAT(CASE WHEN x = %s THEN %s ELSE CASE WHEN x = %s THEN 'true' ELSE 'false' END END)",
					testPlaceholder(d, 1), testStringPlaceholder(d, 3), testPlaceholder(d, 2),
				)
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(1)},
				{Name: "p3", Value: "ok"},
			},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaRequiredModes(schema) {
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

func TestTranspileValue_CatStringifiedMixedBranchesRejectArrayFieldsAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "tags", Type: FieldTypeArray},
	})
	logic := `{"cat":[{"if":[{"var":"flag"},{"var":"tags"},false]}]}`

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

			if _, valueErr := tr.TranspileValue(logic); !IsErrorCode(valueErr, ErrInvalidArgument) {
				t.Fatalf("TranspileValue() error = %v, want %s", valueErr, ErrInvalidArgument)
			}
			gotSQL, gotParams, paramErr := tr.TranspileParameterizedValue(logic)
			if !IsErrorCode(paramErr, ErrInvalidArgument) {
				t.Fatalf("TranspileParameterizedValue() error = %v, want %s (SQL %q params %#v)",
					paramErr, ErrInvalidArgument, gotSQL, gotParams)
			}
		})
	}
}

func TestTranspileValue_CatRejectsStaticArrayValuesAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "items", Type: FieldTypeArray, ElementType: FieldTypeNumber},
	})

	tests := []struct {
		name  string
		logic string
	}{
		{
			name:  "array literal",
			logic: `{"cat":[[1,2]]}`,
		},
		{
			name:  "constant if selects array literal",
			logic: `{"cat":[{"if":[true,[1],false]}]}`,
		},
		{
			name:  "array producing map",
			logic: `{"cat":[{"map":[[1],{"var":""}]}]}`,
		},
		{
			name:  "custom array value",
			logic: `{"cat":[{"arrayValue":[]}]}`,
		},
		{
			name:  "custom array value behind parameterized stringified or",
			logic: `{"cat":[{"or":[{"var":"flag"},{"arrayParam":["x"]}]}]}`,
		},
		{
			name:  "schema array field",
			logic: `{"cat":[{"var":"items"}]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaRequiredModes(schema) {
				t.Run(mode.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}
					err = tr.RegisterOperatorFunc("arrayValue", func(_ string, args []OperatorArg) (OperatorResult, error) {
						if len(args) != 0 {
							return OperatorResult{}, fmt.Errorf("arrayValue requires no arguments")
						}
						return ValueSQL("array_expr", ExpressionTypeArray), nil
					})
					if err != nil {
						t.Fatalf("RegisterOperatorFunc() error = %v", err)
					}
					err = tr.RegisterOperatorFunc("arrayParam", func(_ string, args []OperatorArg) (OperatorResult, error) {
						if len(args) != 1 {
							return OperatorResult{}, fmt.Errorf("arrayParam requires one argument")
						}
						return ValueSQL(fmt.Sprintf("[%s]", args[0].SQL), ExpressionTypeArray), nil
					})
					if err != nil {
						t.Fatalf("RegisterOperatorFunc() error = %v", err)
					}

					for _, tt := range tests {
						t.Run(tt.name, func(t *testing.T) {
							if _, valueErr := tr.TranspileValue(tt.logic); !IsErrorCode(valueErr, ErrInvalidArgument) {
								t.Fatalf("TranspileValue() error = %v, want %s", valueErr, ErrInvalidArgument)
							}
							gotSQL, gotParams, paramErr := tr.TranspileParameterizedValue(tt.logic)
							if !IsErrorCode(paramErr, ErrInvalidArgument) {
								t.Fatalf("TranspileParameterizedValue() error = %v, want %s (SQL %q params %#v)",
									paramErr, ErrInvalidArgument, gotSQL, gotParams)
							}
							if len(gotParams) != 0 {
								t.Fatalf("params = %#v, want none on error", gotParams)
							}
						})
					}
				})
			}
		})
	}
}

func TestTranspileValue_PredicateResultsAreTwoValuedBooleansAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "x", Type: FieldTypeNumber},
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "arr", Type: FieldTypeArray, ElementType: FieldTypeNumber},
	})

	predicateValue := "CASE WHEN x = 1 THEN TRUE ELSE FALSE END"
	tests := []struct {
		name       string
		logic      string
		want       func(Dialect) string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "root comparison value",
			logic: `{"==":[{"var":"x"},1]}`,
			want: func(Dialect) string {
				return predicateValue
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN x = %s THEN TRUE ELSE FALSE END", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
		{
			name:  "constant if branch returns comparison value",
			logic: `{"if":[true,{"==":[{"var":"x"},1]},false]}`,
			want: func(Dialect) string {
				return predicateValue
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN x = %s THEN TRUE ELSE FALSE END", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
		{
			name:  "and false branch returns comparison value",
			logic: `{"and":[{"==":[{"var":"x"},1]},true]}`,
			want: func(Dialect) string {
				return fmt.Sprintf("CASE WHEN x = 1 THEN TRUE ELSE %s END", predicateValue)
			},
			wantParam: func(d Dialect) string {
				placeholder := testPlaceholder(d, 1)
				return fmt.Sprintf(
					"CASE WHEN x = %s THEN TRUE ELSE CASE WHEN x = %s THEN TRUE ELSE FALSE END END",
					placeholder,
					placeholder,
				)
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
		{
			name:  "array literal contains comparison value",
			logic: `[{"==":[{"var":"x"},1]}]`,
			want: func(d Dialect) string {
				if d == DialectPostgreSQL {
					return fmt.Sprintf("ARRAY[%s]", predicateValue)
				}
				return fmt.Sprintf("[%s]", predicateValue)
			},
			wantParam: func(d Dialect) string {
				comparison := fmt.Sprintf("CASE WHEN x = %s THEN TRUE ELSE FALSE END", testPlaceholder(d, 1))
				if d == DialectPostgreSQL {
					return fmt.Sprintf("ARRAY[%s]", comparison)
				}
				return fmt.Sprintf("[%s]", comparison)
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
		{
			name:  "map transformation returns comparison value",
			logic: `{"map":[{"var":"arr"},{"==":[{"var":""},1]}]}`,
			want: func(d Dialect) string {
				if d == DialectClickHouse {
					return "arrayMap(elem -> CASE WHEN elem = 1 THEN TRUE ELSE FALSE END, arr)"
				}
				return testDuckDBUnnestSourceAliases(d, "ARRAY(SELECT CASE WHEN elem = 1 THEN TRUE ELSE FALSE END FROM UNNEST(arr) AS elem)")
			},
			wantParam: func(d Dialect) string {
				placeholder := testIntPlaceholder(d, 1)
				if d == DialectClickHouse {
					return fmt.Sprintf("arrayMap(elem -> CASE WHEN elem = %s THEN TRUE ELSE FALSE END, arr)", placeholder)
				}
				return testDuckDBUnnestSourceAliases(d, fmt.Sprintf("ARRAY(SELECT CASE WHEN elem = %s THEN TRUE ELSE FALSE END FROM UNNEST(arr) AS elem)", placeholder))
			},
			wantParams: []QueryParam{{Name: "p1", Value: int64(1)}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaRequiredModes(schema) {
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
							if want := tt.want(d); got != want {
								t.Fatalf("TranspileValue() = %q, want %q", got, want)
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

func TestTranspileValue_LiteralComparisonsEmitFoldedBooleanValuesAllDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		logic string
		want  string
	}{
		{
			name:  "null ordering coerces to known true",
			logic: `{">":[null,-1]}`,
			want:  "TRUE",
		},
		{
			name:  "loose equality coerces number string",
			logic: `{"==":[5,"5"]}`,
			want:  "TRUE",
		},
		{
			name:  "strict equality keeps type mismatch false",
			logic: `{"===":[5,"5"]}`,
			want:  "FALSE",
		},
		{
			name:  "literal in folds false",
			logic: `{"in":["x",["a","b"]]}`,
			want:  "FALSE",
		},
		{
			name:  "literal in empty array folds false",
			logic: `{"in":["x",[]]}`,
			want:  "FALSE",
		},
		{
			name:  "literal in numeric haystack folds false",
			logic: `{"in":["3",12345]}`,
			want:  "FALSE",
		},
		{
			name:  "literal in boolean haystack folds false",
			logic: `{"in":["true",true]}`,
			want:  "FALSE",
		},
		{
			name:  "literal in string haystack stringifies number",
			logic: `{"in":[3,"12345"]}`,
			want:  "TRUE",
		},
		{
			name:  "literal in string haystack stringifies boolean",
			logic: `{"in":[true,"true"]}`,
			want:  "TRUE",
		},
		{
			name:  "literal in string haystack stringifies null",
			logic: `{"in":[null,"null"]}`,
			want:  "TRUE",
		},
		{
			name:  "empty string needle matches empty string haystack",
			logic: `{"in":["",""]}`,
			want:  "TRUE",
		},
		{
			name:  "empty string needle matches non-empty string haystack",
			logic: `{"in":["","x"]}`,
			want:  "TRUE",
		},
		{
			name:  "array literal needle uses javascript string form",
			logic: `{"in":[[1,2],"x1,2y"]}`,
			want:  "TRUE",
		},
		{
			name:  "empty array needle matches empty string haystack",
			logic: `{"in":[[],""]}`,
			want:  "TRUE",
		},
		{
			name:  "empty array needle matches non-empty string haystack",
			logic: `{"in":[[],"abc"]}`,
			want:  "TRUE",
		},
		{
			name:  "literal in string haystack keeps mismatched text false",
			logic: `{"in":[0,"false"]}`,
			want:  "FALSE",
		},
		{
			name:  "literal in array uses strict equality for string number",
			logic: `{"in":["1",[1]]}`,
			want:  "FALSE",
		},
		{
			name:  "literal in array uses strict equality for boolean number",
			logic: `{"in":[false,[0]]}`,
			want:  "FALSE",
		},
		{
			name:  "literal in array matches same numeric type",
			logic: `{"in":[1,[1]]}`,
			want:  "TRUE",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
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
					if gotParam != tt.want {
						t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, tt.want)
					}
					if len(gotParams) != 0 {
						t.Fatalf("params = %#v, want none", gotParams)
					}
				})
			}
		})
	}
}

func TestTranspileValue_PredicateIfBranchesNormalizeBooleanValuesAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "x", Type: FieldTypeNumber},
		{Name: "flag", Type: FieldTypeBoolean},
	})
	logic := `{"if":[{"var":"flag"},{"==":[{"var":"x"},1]},false]}`

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

			got, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			want := "CASE WHEN flag IS TRUE THEN CASE WHEN x = 1 THEN TRUE ELSE FALSE END ELSE FALSE END"
			if got != want {
				t.Fatalf("TranspileValue() = %q, want %q", got, want)
			}

			gotParam, gotParams, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			wantParam := fmt.Sprintf(
				"CASE WHEN flag IS TRUE THEN CASE WHEN x = %s THEN TRUE ELSE FALSE END ELSE FALSE END",
				testPlaceholder(d, 1),
			)
			if gotParam != wantParam {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, wantParam)
			}
			wantParams := []QueryParam{{Name: "p1", Value: float64(1)}}
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
			logic: `[{"var":"amount"},1,{"+":[{"var":"amount"},2]}]`,
			wantSQL: func(d Dialect) string {
				if d == DialectPostgreSQL {
					return "ARRAY[amount, 1, (amount + 2)]"
				}
				return "[amount, 1, (amount + 2)]"
			},
			wantParam: func(d Dialect) string {
				if d == DialectPostgreSQL {
					return fmt.Sprintf(
						"ARRAY[amount, %s, (amount + %s)]",
						testPlaceholder(d, 1),
						testPlaceholder(d, 2),
					)
				}
				return fmt.Sprintf(
					"[amount, %s, (amount + %s)]",
					testPlaceholder(d, 1),
					testPlaceholder(d, 2),
				)
			},
			params: []QueryParam{{Name: "p1", Value: float64(1)}, {Name: "p2", Value: float64(2)}},
		},
		{
			name:  "map source array literal",
			logic: `{"map":[[1,2],{"+":[{"var":""},1]}]}`,
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
			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := tr.TranspileValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if want := testDuckDBUnnestSourceAliases(d, tt.wantSQL(d)); got != want {
						t.Fatalf("TranspileValue() = %q, want %q", got, want)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if want := testDuckDBUnnestSourceAliases(d, tt.wantParam(d)); gotParam != want {
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

func TestTranspileValue_ArrayLiteralsRejectKnownMixedElementTypesAllDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		logic string
	}{
		{
			name:  "root literal mixes number and string",
			logic: `[1,"x"]`,
		},
		{
			name:  "root literal mixes numeric field and predicate expression",
			logic: `[{"var":"amount"},{"==":[{"var":"status"},"ok"]}]`,
		},
		{
			name:  "map source literal mixes number and string",
			logic: `{"map":[[1,"x"],{"var":""}]}`,
		},
		{
			name:  "root literal mixes empty array and scalar",
			logic: `[[],1]`,
		},
		{
			name:  "root literal mixes scalar and empty array",
			logic: `[1,[]]`,
		},
		{
			name:  "map source literal mixes empty array and scalar",
			logic: `{"map":[[[],1],{"var":""}]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					_, err := tr.TranspileValue(tt.logic)
					if err == nil || !strings.Contains(err.Error(), "array literal elements must have compatible SQL types") {
						t.Fatalf("TranspileValue() error = %v, want array element type error", err)
					}

					_, _, err = tr.TranspileParameterizedValue(tt.logic)
					if err == nil || !strings.Contains(err.Error(), "array literal elements must have compatible SQL types") {
						t.Fatalf("TranspileParameterizedValue() error = %v, want array element type error", err)
					}
				})
			}
		})
	}
}

func TestTranspileValue_NestedArrayLiteralsRejectUnsupportedDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		logic              string
		rejectNestedSource bool
	}{
		{logic: `[[1]]`},
		{logic: `{"map":[[[1]],{"var":""}]}`, rejectNestedSource: true},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tc := range tests {
				t.Run(tc.logic, func(t *testing.T) {
					_, err := tr.TranspileValue(tc.logic)
					paramSQL, params, paramErr := tr.TranspileParameterizedValue(tc.logic)
					if dialectRejectsArrayLiteralElements(d) ||
						(tc.rejectNestedSource && dialectRejectsNestedArraySources(d)) {
						if !IsErrorCode(err, ErrInvalidArgument) {
							t.Fatalf("TranspileValue() error = %v, want %s", err, ErrInvalidArgument)
						}
						if !IsErrorCode(paramErr, ErrInvalidArgument) {
							t.Fatalf("TranspileParameterizedValue() error = %v, want %s (SQL %q params %#v)",
								paramErr, ErrInvalidArgument, paramSQL, params)
						}
						return
					}
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if paramErr != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", paramErr)
					}
				})
			}
		})
	}
}

func TestTranspileValue_IfRejectsArrayBranchesWithIncompatibleElementTypesAllDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		logic string
	}{
		{
			name:  "literal number array versus string array",
			logic: `{"if":[{"var":"flag"},[1],["a"]]}`,
		},
		{
			name:  "expression number array versus string array",
			logic: `{"if":[{"var":"flag"},[{"var":"amount"}],["a"]]}`,
		},
		{
			name:  "multi-branch later array mismatch",
			logic: `{"if":[{"var":"flag"},[1],{"var":"active"},[true],["a"]]}`,
		},
		{
			name:  "nested if preserves array element metadata",
			logic: `{"if":[{"var":"active"},{"if":[{"var":"flag"},[1],[2]]},["a"]]}`,
		},
		{
			name:  "value logical preserves nested array element metadata",
			logic: `{"or":[{"if":[{"var":"flag"},[],[1]]},["a"]]}`,
		},
		{
			name:  "null branch does not erase array element metadata",
			logic: `{"if":[{"var":"active"},null,{"var":"flag"},[1],["a"]]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					if sql, err := tr.TranspileValue(tt.logic); err == nil ||
						!strings.Contains(err.Error(), "array value branches must have compatible element types") {
						t.Fatalf("TranspileValue() SQL = %q error = %v, want array branch element type error", sql, err)
					}

					if sql, params, err := tr.TranspileParameterizedValue(tt.logic); err == nil ||
						!strings.Contains(err.Error(), "array value branches must have compatible element types") {
						t.Fatalf("TranspileParameterizedValue() SQL = %q params = %#v error = %v, want array branch element type error",
							sql, params, err)
					}
				})
			}
		})
	}
}

func TestTranspileValue_RejectsObjectFieldsInValueBranchesAllDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		logic string
	}{
		{
			name:  "if then branch is root object field",
			logic: `{"if":[{"var":"flag"},{"var":"profile"},"fallback"]}`,
		},
		{
			name:  "if else branch is root object field",
			logic: `{"if":[{"var":"flag"},"fallback",{"var":"metadata"}]}`,
		},
		{
			name:  "or branch is root object field",
			logic: `{"or":[false,{"var":"profile"}]}`,
		},
		{
			name:  "and branch is root object field",
			logic: `{"and":[true,{"var":"metadata"}]}`,
		},
		{
			name:  "direct root object value field",
			logic: `{"var":"profile"}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					if sql, err := tr.TranspileValue(tt.logic); err == nil ||
						!strings.Contains(err.Error(), "object field") {
						t.Fatalf("TranspileValue() SQL = %q error = %v, want object field value error", sql, err)
					}

					if sql, params, err := tr.TranspileParameterizedValue(tt.logic); err == nil ||
						!strings.Contains(err.Error(), "object field") {
						t.Fatalf("TranspileParameterizedValue() SQL = %q params = %#v error = %v, want object field value error",
							sql, params, err)
					}
				})
			}
		})
	}
}

func TestTranspileValue_AllowsNestedScalarFieldsUnderObjectsAllDialects(t *testing.T) {
	t.Parallel()

	logic := `{"if":[{"var":"flag"},{"var":"profile.status"},"fallback"]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			got, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if !strings.Contains(got, "profile.status") {
				t.Fatalf("TranspileValue() = %q, want nested scalar object field", got)
			}

			gotParam, gotParams, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			if !strings.Contains(gotParam, "profile.status") {
				t.Fatalf("TranspileParameterizedValue() = %q, want nested scalar object field", gotParam)
			}
			if wantParams := []QueryParam{{Name: "p1", Value: "fallback"}}; !reflect.DeepEqual(gotParams, wantParams) {
				t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
			}
		})
	}
}

func TestTranspileValue_IfAllowsArrayBranchesWithCompatibleElementTypesAllDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		logic string
	}{
		{
			name:  "number array branches",
			logic: `{"if":[{"var":"flag"},[1],[2]]}`,
		},
		{
			name:  "expression number array branches",
			logic: `{"if":[{"var":"flag"},[{"var":"amount"}],[2]]}`,
		},
		{
			name:  "empty branch inherits number element type",
			logic: `{"if":[{"var":"flag"},[],[2]]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					sql, err := tr.TranspileValue(tt.logic)
					if d == DialectPostgreSQL && strings.Contains(tt.logic, "[]") {
						if !IsErrorCode(err, ErrInvalidArgument) {
							t.Fatalf("TranspileValue() error = %v, want PostgreSQL empty-array type error", err)
						}
					} else if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					} else if !strings.Contains(sql, "CASE WHEN") {
						t.Fatalf("TranspileValue() SQL = %q, want CASE expression", sql)
					}

					paramSQL, _, err := tr.TranspileParameterizedValue(tt.logic)
					if d == DialectPostgreSQL && strings.Contains(tt.logic, "[]") {
						if !IsErrorCode(err, ErrInvalidArgument) {
							t.Fatalf("TranspileParameterizedValue() error = %v, want PostgreSQL empty-array type error", err)
						}
					} else if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					} else if !strings.Contains(paramSQL, "CASE WHEN") {
						t.Fatalf("TranspileParameterizedValue() SQL = %q, want CASE expression", paramSQL)
					}
				})
			}
		})
	}
}

func TestTranspileValue_ArrayExpressionSourcesPreserveElementTypesAllDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		logic               string
		rejectArrayElements bool
		wantFragments       []string
	}{
		{
			name:  "filter source from dynamic if",
			logic: `{"filter":[{"if":[{"var":"flag"},[1,2,0],[3]]},{"var":""}]}`,
		},
		{
			name:  "some source from value logical",
			logic: `{"some":[{"or":[false,[0,1]]},{"var":""}]}`,
		},
		{
			name:  "filter source from map",
			logic: `{"filter":[{"map":[[1,2,0],{"var":""}]},{"var":""}]}`,
		},
		{
			name:  "filter source from filter",
			logic: `{"filter":[{"filter":[[1,2,0],{">":[{"var":""},0]}]},{"var":""}]}`,
		},
		{
			name:  "filter source from merge",
			logic: `{"filter":[{"merge":[[1,2],[0]]},{"var":""}]}`,
		},
		{
			name:          "filter source from predicate map",
			logic:         `{"filter":[{"map":[[1,0],{">":[{"var":""},0]}]},{"var":""}]}`,
			wantFragments: []string{"elem IS TRUE"},
		},
		{
			name:                "nested literal array current element truthiness",
			logic:               `{"some":[[[1,0]],{"some":[{"var":""},{"var":""}]}]}`,
			rejectArrayElements: true,
			wantFragments:       []string{"elem1 IS NOT NULL", "elem1 != 0"},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					wantFragments := tt.wantFragments
					if len(wantFragments) == 0 {
						wantFragments = []string{"elem IS NOT NULL", "elem != 0"}
					}

					got, err := tr.TranspileValue(tt.logic)
					if tt.rejectArrayElements && dialectRejectsNestedArraySources(d) {
						if !IsErrorCode(err, ErrInvalidArgument) {
							t.Fatalf("TranspileValue() error = %v, want %s", err, ErrInvalidArgument)
						}
						gotParam, params, paramErr := tr.TranspileParameterizedValue(tt.logic)
						if !IsErrorCode(paramErr, ErrInvalidArgument) {
							t.Fatalf("TranspileParameterizedValue() error = %v, want %s (SQL %q params %#v)",
								paramErr, ErrInvalidArgument, gotParam, params)
						}
						return
					}
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					for _, want := range wantFragments {
						if !strings.Contains(got, want) {
							t.Fatalf("TranspileValue() = %q, want fragment %q", got, want)
						}
					}

					gotParam, _, err := tr.TranspileParameterizedValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					for _, want := range wantFragments {
						if !strings.Contains(gotParam, want) {
							t.Fatalf("TranspileParameterizedValue() = %q, want fragment %q", gotParam, want)
						}
					}
				})
			}
		})
	}
}

func TestTranspileParameterizedValue_ReduceAggregatePredicateBindsScopedDefaultsAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{
			Name: "items",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "x", Type: FieldTypeString},
			},
		},
	})
	logic := `{"reduce":[{"var":"items"},{"+":[{"var":"accumulator"},{"==":[{"var":["current.x","fallback"]},"fallback"]}]},0]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			sql, params, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			if strings.Contains(sql, "'fallback'") {
				t.Fatalf("TranspileParameterizedValue() inlined scoped default: %s", sql)
			}
			assertContains(t, sql, fmt.Sprintf("COALESCE(elem.x, %s)", testStringPlaceholder(d, 2)))
			assertContains(t, sql, fmt.Sprintf("= %s", testStringPlaceholder(d, 3)))
			wantParams := []QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: "fallback"},
				{Name: "p3", Value: "fallback"},
			}
			if !reflect.DeepEqual(params, wantParams) {
				t.Fatalf("params = %#v, want %#v", params, wantParams)
			}
		})
	}
}

func TestTranspileValue_ArrayLiteralExpressionElementTypeDrivesLambdaTruthinessAllDialects(t *testing.T) {
	t.Parallel()

	logic := `{"filter":[[{">":[{"var":"x"},0]},false],{"var":""}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			sql, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			assertContains(t, sql, "elem IS TRUE")

			paramSQL, params, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			assertContains(t, paramSQL, "elem IS TRUE")
			if len(params) != 1 || params[0].Value != float64(0) {
				t.Fatalf("params = %#v, want one zero comparison parameter", params)
			}
		})
	}
}

func TestTranspileValue_CatStringifiesCustomPredicate(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery, defaultTestSchema())
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

func TestTranspileValue_CatStringifiesLowercaseCaseBooleanValue(t *testing.T) {
	logic := `{"cat":[{"lowerBool":[]}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			err = tr.RegisterOperatorFunc("lowerBool", func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 0 {
					return OperatorResult{}, fmt.Errorf("lowerBool requires no arguments")
				}
				return ValueSQL("case when flag then true else false end", ExpressionTypeBoolean), nil
			})
			if err != nil {
				t.Fatalf("RegisterOperatorFunc() error = %v", err)
			}

			want := "CONCAT(CASE WHEN (case when flag then true else false end) IS TRUE THEN 'true' WHEN (case when flag then true else false end) IS FALSE THEN 'false' ELSE '' END)"
			got, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if got != want {
				t.Fatalf("TranspileValue() = %q, want %q", got, want)
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

func TestTranspileValue_IfUsesTypedCustomPredicateCondition(t *testing.T) {
	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d, defaultTestSchema())
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
			wantParam := fmt.Sprintf("CASE WHEN amount > 0 THEN %s ELSE %s END", testStringPlaceholder(d, 1), testStringPlaceholder(d, 2))
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
	tr, err := NewTranspiler(DialectPostgreSQL, defaultTestSchema())
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
	want := "CONCAT(COALESCE(CAST(CASE WHEN count = 0 THEN NULL ELSE total / count END AS TEXT), ''))"

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
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "fallback"}},
		},
		{
			name:  "and skips truthy numeric literal before fallback string",
			logic: `{"and":[1,"x"]}`,
			wantSQL: func(d Dialect) string {
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "x"}},
		},
		{
			name:  "or keeps returned truthy string literal",
			logic: `{"or":["x","fallback"]}`,
			wantSQL: func(d Dialect) string {
				return testStringPlaceholder(d, 1)
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
				return fmt.Sprintf("CONCAT(%s)", testStringPlaceholder(d, 1))
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
			tr, err := NewTranspiler(d, defaultTestSchema())
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
			wantSQL:    "CONCAT(CASE WHEN (CASE WHEN flag IS TRUE THEN TRUE ELSE FALSE END) IS TRUE THEN 'true' WHEN (CASE WHEN flag IS TRUE THEN TRUE ELSE FALSE END) IS FALSE THEN 'false' ELSE '' END)",
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
	tr, err := NewTranspiler(DialectBigQuery, defaultTestSchema())
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

	if got, err = tr.TranspileValue(`{"cat":[{"reduce":[{"var":"arr"},{"cat":[{"var":"accumulator"},{"var":"current"}]},""]}]}`); err == nil {
		t.Fatalf("TranspileValue() cat reduce should be unsupported in BigQuery, got %q", got)
	}

	clickHouse, err := NewTranspiler(DialectClickHouse, defaultTestSchema())
	if err != nil {
		t.Fatalf("NewTranspiler(ClickHouse) error = %v", err)
	}
	got, err = clickHouse.TranspileValue(`{"cat":[{"reduce":[{"var":"arr"},{"cat":[{"var":"accumulator"},{"var":"current"}]},""]}]}`)
	if err != nil {
		t.Fatalf("TranspileValue() ClickHouse cat reduce error = %v", err)
	}
	if want := "CONCAT(COALESCE(arrayFold((acc, elem) -> CONCAT(COALESCE(acc, ''), COALESCE(toString(elem), '')), arr, ''), ''))"; got != want {
		t.Fatalf("TranspileValue() ClickHouse cat reduce = %q, want %q", got, want)
	}

	duckDB, err := NewTranspiler(DialectDuckDB, defaultTestSchema())
	if err != nil {
		t.Fatalf("NewTranspiler(DuckDB) error = %v", err)
	}
	got, err = duckDB.TranspileValue(`{"cat":[{"reduce":[{"var":"arr"},{"cat":[{"var":"accumulator"},{"var":"current"}]},""]}]}`)
	if err != nil {
		t.Fatalf("TranspileValue() DuckDB cat reduce error = %v", err)
	}
	if !strings.Contains(got, "list_reduce(arr, lambda acc, elem : CONCAT(COALESCE(acc, ''), COALESCE(CAST(elem AS VARCHAR), '')), '')") {
		t.Fatalf("TranspileValue() DuckDB cat reduce did not use list_reduce string reducer: %s", got)
	}
}

func TestTranspileParameterizedValue_ArrayTransformationsUseValueSemantics(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery, defaultTestSchema())
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

	if gotSQL, gotParams, err = tr.TranspileParameterizedValue(`{"cat":[{"reduce":[{"var":"arr"},{"cat":[{"var":"accumulator"},{"var":"current"}]},""]}]}`); err == nil {
		t.Fatalf("TranspileParameterizedValue() cat reduce should be unsupported in BigQuery, got SQL %q params %#v", gotSQL, gotParams)
	}

	clickHouse, err := NewTranspiler(DialectClickHouse, defaultTestSchema())
	if err != nil {
		t.Fatalf("NewTranspiler(ClickHouse) error = %v", err)
	}
	gotSQL, gotParams, err = clickHouse.TranspileParameterizedValue(`{"cat":[{"reduce":[{"var":"arr"},{"cat":[{"var":"accumulator"},{"var":"current"}]},""]}]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedValue() ClickHouse cat reduce error = %v", err)
	}
	if want := "CONCAT(COALESCE(arrayFold((acc, elem) -> CONCAT(COALESCE(acc, ''), COALESCE(toString(elem), '')), arr, {p1:String}), ''))"; gotSQL != want {
		t.Fatalf("TranspileParameterizedValue() ClickHouse cat reduce SQL = %q, want %q", gotSQL, want)
	}
	if wantParams := []QueryParam{{Name: "p1", Value: ""}}; !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("TranspileParameterizedValue() ClickHouse cat reduce params = %#v, want %#v", gotParams, wantParams)
	}

	duckDB, err := NewTranspiler(DialectDuckDB, defaultTestSchema())
	if err != nil {
		t.Fatalf("NewTranspiler(DuckDB) error = %v", err)
	}
	gotSQL, gotParams, err = duckDB.TranspileParameterizedValue(`{"cat":[{"reduce":[{"var":"arr"},{"cat":[{"var":"accumulator"},{"var":"current"}]},""]}]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedValue() DuckDB cat reduce error = %v", err)
	}
	if !strings.Contains(gotSQL, "list_reduce(arr, lambda acc, elem : CONCAT(COALESCE(acc, ''), COALESCE(CAST(elem AS VARCHAR), '')), $1)") {
		t.Fatalf("TranspileParameterizedValue() DuckDB cat reduce did not use list_reduce string reducer: %s", gotSQL)
	}
	if wantParams := []QueryParam{{Name: "p1", Value: ""}}; !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("TranspileParameterizedValue() DuckDB cat reduce params = %#v, want %#v", gotParams, wantParams)
	}
}

func TestTranspileValue_ArrayPredicateContextsUseTruthinessLogicals(t *testing.T) {
	t.Parallel()

	logic := `{"filter":[{"var":"items"},{"or":[0,{"==":[{"var":""},1]}]}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			want := "ARRAY(SELECT elem FROM UNNEST(items) AS elem WHERE elem = 1)"
			wantParam := "ARRAY(SELECT elem FROM UNNEST(items) AS elem WHERE elem = " + testPlaceholder(d, 1) + ")"
			if d == DialectClickHouse {
				want = "arrayFilter(elem -> elem = 1, items)"
				wantParam = "arrayFilter(elem -> elem = " + testPlaceholder(d, 1) + ", items)"
			} else {
				want = testDuckDBUnnestSourceAliases(d, want)
				wantParam = testDuckDBUnnestSourceAliases(d, wantParam)
			}

			got, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if got != want {
				t.Fatalf("TranspileValue() = %q, want %q", got, want)
			}

			gotParam, gotParams, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			if gotParam != wantParam {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, wantParam)
			}
			wantParams := []QueryParam{{Name: "p1", Value: float64(1)}}
			if !reflect.DeepEqual(gotParams, wantParams) {
				t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
			}
		})
	}
}

func TestTranspileParameterizedValue_ArrayScopedDefaultUsesBindParams(t *testing.T) {
	logic := `{"map":[{"var":"names"},{"cat":[{"var":["","fallback"]}]}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d, defaultTestSchema())
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
			if want := fmt.Sprintf("COALESCE(elem, %s)", testStringPlaceholder(d, 1)); !strings.Contains(gotSQL, want) {
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
	logic := `{"map":[{"var":"items"},{"or":["x",{"var":["","fallback"]}]}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			gotSQL, gotParams, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			wantValue := testStringPlaceholder(d, 1)
			wantSQL := fmt.Sprintf("ARRAY(SELECT %s FROM UNNEST(items) AS elem)", wantValue)
			if d == DialectClickHouse {
				wantSQL = fmt.Sprintf("arrayMap(elem -> %s, items)", wantValue)
			} else {
				wantSQL = testDuckDBUnnestSourceAliases(d, wantSQL)
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
	logic := `{"map":[{"var":"items"},{"cat":[{"gt":[{"var":""},0]}]}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d, defaultTestSchema())
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

func TestTranspileValue_CustomPredicateBooleanConstantsShortCircuitAllDialectsSchemaRequired(t *testing.T) {
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
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "ok"}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, mode := range schemaRequiredModes(schema) {
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

func TestTranspileValue_RejectsDefaultedScopedObjectFieldsAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "items", Type: FieldTypeArray, ElementFields: []FieldSchema{
			{Name: "profile", Type: FieldTypeObject, Fields: []FieldSchema{
				{Name: "tier", Type: FieldTypeString},
			}},
		}},
	})
	logic := `{"map":[{"var":"items"},{"var":["profile",null]}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			expectValueAndParamErrorContains(t, tr, logic, "object field 'items.profile' cannot be used as a value expression")
		})
	}
}

func TestTranspileValue_SubstrRejectsKnownInvalidValueOperandTypesAllDialects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		logic   string
		wantErr string
	}{
		{
			name:    "array-valued source",
			logic:   `{"substr":[{"map":[[1,2],{"var":""}]},0]}`,
			wantErr: "substring source argument must be string or number, got array",
		},
		{
			name:    "string-valued start",
			logic:   `{"substr":["abcdef",{"cat":["x"]}]}`,
			wantErr: "substring start argument must be numeric, got string",
		},
		{
			name:    "predicate length",
			logic:   `{"substr":["abcdef",0,{">":[1,0]}]}`,
			wantErr: "substring length argument must be numeric, got predicate",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, emptyTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					expectValueAndParamErrorContains(t, tr, tt.logic, tt.wantErr)
				})
			}
		})
	}
}

func TestTranspileValue_SubstrNegativeIndexesAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{{Name: "text", Type: FieldTypeString}})
	tests := []struct {
		name      string
		logic     string
		want      func(Dialect) string
		wantParam func(Dialect) string
		params    []QueryParam
	}{
		{
			name:  "negative start counts from end",
			logic: `{"substr":[{"var":"text"},-5,3]}`,
			want: func(d Dialect) string {
				return fmt.Sprintf("%s(text, %s, 3)",
					testSubstrFunc(d),
					testGreatest(fmt.Sprintf("(%s + -5 + 1)", testStringLength(d)), "1"),
				)
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("%s(text, %s, %s)",
					testSubstrFunc(d),
					testGreatest(fmt.Sprintf("(%s + %s + 1)", testStringLength(d), testPlaceholder(d, 1)), "1"),
					testPlaceholder(d, 2),
				)
			},
			params: []QueryParam{{Name: "p1", Value: float64(-5)}, {Name: "p2", Value: float64(3)}},
		},
		{
			name:  "negative length stops before end",
			logic: `{"substr":[{"var":"text"},4,-2]}`,
			want: func(d Dialect) string {
				return fmt.Sprintf("%s(text, 5, %s)",
					testSubstrFunc(d),
					testGreatest(
						fmt.Sprintf("(%s - 4)", testGreatest(fmt.Sprintf("(%s + -2)", testStringLength(d)), "0")),
						"0",
					),
				)
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("%s(text, (%s + 1), %s)",
					testSubstrFunc(d),
					testPlaceholder(d, 1),
					testGreatest(
						fmt.Sprintf("(%s - %s)",
							testGreatest(fmt.Sprintf("(%s + %s)", testStringLength(d), testPlaceholder(d, 2)), "0"),
							testPlaceholder(d, 1),
						),
						"0",
					),
				)
			},
			params: []QueryParam{{Name: "p1", Value: float64(4)}, {Name: "p2", Value: float64(-2)}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()
					got, err := tr.TranspileValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if want := tt.want(d); got != want {
						t.Fatalf("TranspileValue() = %q, want %q", got, want)
					}

					gotSQL, gotParams, err := tr.TranspileParameterizedValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if want := tt.wantParam(d); gotSQL != want {
						t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotSQL, want)
					}
					if !reflect.DeepEqual(gotParams, tt.params) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.params)
					}
				})
			}
		})
	}
}

func testPlaceholder(d Dialect, index int) string {
	return testTypedPlaceholder(d, index, "Float64")
}

func testStringPlaceholder(d Dialect, index int) string {
	return testTypedPlaceholder(d, index, "String")
}

func testIntPlaceholder(d Dialect, index int) string {
	return testTypedPlaceholder(d, index, "Int64")
}

func testSubstrFunc(d Dialect) string {
	if d == DialectClickHouse {
		return "substring"
	}
	return "SUBSTR"
}

func testStringLength(d Dialect) string {
	if d == DialectClickHouse {
		return "length(text)"
	}
	return "LENGTH(text)"
}

func testGreatest(args ...string) string {
	return fmt.Sprintf("GREATEST(%s)", strings.Join(args, ", "))
}

func testTypedPlaceholder(d Dialect, index int, typ string) string {
	switch d {
	case DialectPostgreSQL, DialectDuckDB:
		return fmt.Sprintf("$%d", index)
	case DialectClickHouse:
		return fmt.Sprintf("{p%d:%s}", index, typ)
	default:
		return fmt.Sprintf("@p%d", index)
	}
}

func testStringCastSQL(d Dialect, expr string) string {
	switch d {
	case DialectPostgreSQL:
		return fmt.Sprintf("CAST(%s AS TEXT)", expr)
	case DialectDuckDB:
		return fmt.Sprintf("CAST(%s AS VARCHAR)", expr)
	case DialectClickHouse:
		return fmt.Sprintf("toString(%s)", expr)
	default:
		return fmt.Sprintf("CAST(%s AS STRING)", expr)
	}
}
