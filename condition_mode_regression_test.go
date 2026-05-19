package jsonlogic2sql

import (
	"fmt"
	"reflect"
	"testing"
)

func TestTranspileCondition_PredicateIfAcceptsBooleanConstants(t *testing.T) {
	tests := []struct {
		name      string
		logic     string
		want      string
		wantParam func(Dialect) string
		params    []QueryParam
	}{
		{
			name:  "boolean branch arms",
			logic: `{"if":[{">":[{"var":"age"},18]},true,false]}`,
			want:  "CASE WHEN age > 18 THEN TRUE ELSE FALSE END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN age > %s THEN TRUE ELSE FALSE END", testPlaceholder(d, 1))
			},
			params: []QueryParam{{Name: "p1", Value: float64(18)}},
		},
		{
			name:  "boolean condition",
			logic: `{"if":[true,{">":[{"var":"age"},18]},false]}`,
			want:  "age > 18",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("age > %s", testPlaceholder(d, 1))
			},
			params: []QueryParam{{Name: "p1", Value: float64(18)}},
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
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if !reflect.DeepEqual(gotParams, tt.params) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.params)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_PredicateIfAcceptsSchemaTypedValueConditions(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "name", Type: FieldTypeString},
		{Name: "amount", Type: FieldTypeNumber},
	})

	tests := []struct {
		name      string
		logic     string
		want      string
		wantParam func(Dialect) string
		params    []QueryParam
	}{
		{
			name:  "boolean field condition",
			logic: `{"if":[{"var":"flag"},{">":[{"var":"amount"},0]},false]}`,
			want:  "CASE WHEN flag IS TRUE THEN amount > 0 ELSE FALSE END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN flag IS TRUE THEN amount > %s ELSE FALSE END", testPlaceholder(d, 1))
			},
			params: []QueryParam{{Name: "p1", Value: float64(0)}},
		},
		{
			name:  "string field condition",
			logic: `{"if":[{"var":"name"},{">":[{"var":"amount"},0]},false]}`,
			want:  "CASE WHEN (name IS NOT NULL AND name != '') THEN amount > 0 ELSE FALSE END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN (name IS NOT NULL AND name != '') THEN amount > %s ELSE FALSE END", testPlaceholder(d, 1))
			},
			params: []QueryParam{{Name: "p1", Value: float64(0)}},
		},
		{
			name:  "numeric field condition",
			logic: `{"if":[{"var":"amount"},{"==":[{"var":"flag"},true]},false]}`,
			want:  "CASE WHEN (amount IS NOT NULL AND amount != 0) THEN flag = TRUE ELSE FALSE END",
			wantParam: func(Dialect) string {
				return "CASE WHEN (amount IS NOT NULL AND amount != 0) THEN flag = TRUE ELSE FALSE END"
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
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if !reflect.DeepEqual(gotParams, tt.params) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.params)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_PredicateIfAllowsSchemaTypedValueCondition(t *testing.T) {
	t.Parallel()

	logic := `{"if":[{"var":"flag"},{">":[{"var":"amount"},0]},false]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			got, conditionErr := tr.TranspileCondition(logic)
			if conditionErr != nil {
				t.Fatalf("TranspileCondition() error = %v", conditionErr)
			}
			if want := "CASE WHEN flag IS TRUE THEN amount > 0 ELSE FALSE END"; got != want {
				t.Fatalf("TranspileCondition() = %q, want %q", got, want)
			}
			sql, params, paramErr := tr.TranspileParameterizedCondition(logic)
			if paramErr != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", paramErr)
			}
			if want := fmt.Sprintf("CASE WHEN flag IS TRUE THEN amount > %s ELSE FALSE END", testPlaceholder(d, 1)); sql != want {
				t.Fatalf("TranspileParameterizedCondition() = %q, want %q", sql, want)
			}
			if want := []QueryParam{{Name: "p1", Value: float64(0)}}; !reflect.DeepEqual(params, want) {
				t.Fatalf("params = %#v, want %#v", params, want)
			}
		})
	}
}

func TestTranspileCondition_TruthinessOnlyExpressionsAllowMixedValueBranchesAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "amount", Type: FieldTypeNumber},
	})

	tests := []struct {
		name      string
		logic     string
		want      string
		wantParam func(Dialect) string
		params    []QueryParam
	}{
		{
			name:  "predicate if condition folds mixed-type or truthiness",
			logic: `{"if":[{"or":[{"var":"flag"},"x"]},{">":[{"var":"amount"},0]},false]}`,
			want:  "amount > 0",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("amount > %s", testPlaceholder(d, 1))
			},
			params: []QueryParam{{Name: "p1", Value: float64(0)}},
		},
		{
			name:  "double not accepts mixed-type if truthiness",
			logic: `{"!!":{"if":[{"var":"flag"},"x",0]}}`,
			want:  "CASE WHEN flag IS TRUE THEN TRUE ELSE FALSE END",
			wantParam: func(Dialect) string {
				return "CASE WHEN flag IS TRUE THEN TRUE ELSE FALSE END"
			},
			params: []QueryParam{},
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
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if !reflect.DeepEqual(gotParams, tt.params) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.params)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_PredicateIfSkipsUnreachableBranches(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "x", Type: FieldTypeNumber},
	})

	tests := []struct {
		name       string
		logic      string
		want       string
		wantParam  func(Dialect) string
		params     []QueryParam
		wantNoBind bool
	}{
		{
			name:  "true condition skips invalid else",
			logic: `{"if":[true,{">":[{"var":"x"},1]},{">":[{"var":"missing"},1]}]}`,
			want:  "x > 1",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("x > %s", testPlaceholder(d, 1))
			},
			params: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
		{
			name:  "false condition skips invalid then",
			logic: `{"if":[false,{">":[{"var":"missing"},1]},{">":[{"var":"x"},1]}]}`,
			want:  "x > 1",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("x > %s", testPlaceholder(d, 1))
			},
			params: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
		{
			name:  "false condition without else skips invalid then",
			logic: `{"if":[false,{">":[{"var":"missing"},1]}]}`,
			want:  "FALSE",
			wantParam: func(Dialect) string {
				return "FALSE"
			},
			wantNoBind: true,
		},
		{
			name:  "dynamic condition keeps case and true condition becomes else",
			logic: `{"if":[{">":[{"var":"x"},1]},{">":[{"var":"x"},2]},true,{">":[{"var":"x"},3]},{">":[{"var":"missing"},1]}]}`,
			want:  "CASE WHEN x > 1 THEN x > 2 ELSE x > 3 END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf(
					"CASE WHEN x > %s THEN x > %s ELSE x > %s END",
					testPlaceholder(d, 1),
					testPlaceholder(d, 2),
					testPlaceholder(d, 3),
				)
			},
			params: []QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: float64(3)},
			},
		},
		{
			name:  "false logical condition rolls back skipped params",
			logic: `{"if":[{"and":[false,{">":[{"var":"x"},1]}]},{">":[{"var":"missing"},1]},{">":[{"var":"x"},2]}]}`,
			want:  "x > 2",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("x > %s", testPlaceholder(d, 1))
			},
			params: []QueryParam{{Name: "p1", Value: float64(2)}},
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
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if tt.wantNoBind {
						if len(gotParams) != 0 {
							t.Fatalf("params = %#v, want none", gotParams)
						}
						return
					}
					if !reflect.DeepEqual(gotParams, tt.params) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.params)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_CustomPredicateBooleanConstantsShortCircuitAllDialectsSchemaRequired(t *testing.T) {
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
			name:  "or skips unreachable value operand after custom true",
			logic: `{"or":[{"alwaysTrue":[]},{"var":"missing"}]}`,
			want:  "TRUE",
			wantParam: func(Dialect) string {
				return "TRUE"
			},
		},
		{
			name:  "and skips unreachable value operand after custom false",
			logic: `{"and":[{"alwaysFalse":[]},{"var":"missing"}]}`,
			want:  "FALSE",
			wantParam: func(Dialect) string {
				return "FALSE"
			},
		},
		{
			name:  "if skips unreachable then branch after custom false",
			logic: `{"if":[{"alwaysFalse":[]},{"var":"missing"},{">":[{"var":"x"},0]}]}`,
			want:  "x > 0",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("x > %s", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(0)}},
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
							if want := tt.wantParam(d); gotParam != want {
								t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
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

func registerBooleanConstantOperators(t *testing.T, tr *Transpiler) {
	t.Helper()

	if err := tr.RegisterOperatorFunc("alwaysTrue", func(_ string, _ []OperatorArg) (OperatorResult, error) {
		return PredicateSQL("TRUE"), nil
	}); err != nil {
		t.Fatalf("RegisterOperatorFunc(alwaysTrue) error = %v", err)
	}
	if err := tr.RegisterOperatorFunc("alwaysFalse", func(_ string, _ []OperatorArg) (OperatorResult, error) {
		return PredicateSQL("FALSE"), nil
	}); err != nil {
		t.Fatalf("RegisterOperatorFunc(alwaysFalse) error = %v", err)
	}
}

func TestTranspileCondition_BooleanConstantsArePredicates(t *testing.T) {
	tests := []struct {
		name      string
		logic     string
		want      func(Dialect) string
		wantParam func(Dialect) string
		params    []QueryParam
	}{
		{
			name:  "root true",
			logic: `true`,
			want: func(Dialect) string {
				return "TRUE"
			},
			wantParam: func(Dialect) string {
				return "TRUE"
			},
		},
		{
			name:  "and skips true constant",
			logic: `{"and":[true,{">":[{"var":"x"},0]}]}`,
			want: func(Dialect) string {
				return "x > 0"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("x > %s", testPlaceholder(d, 1))
			},
			params: []QueryParam{{Name: "p1", Value: float64(0)}},
		},
		{
			name:  "or skips false constant",
			logic: `{"or":[false,{">":[{"var":"x"},0]}]}`,
			want: func(Dialect) string {
				return "x > 0"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("x > %s", testPlaceholder(d, 1))
			},
			params: []QueryParam{{Name: "p1", Value: float64(0)}},
		},
		{
			name:  "array predicate true constant",
			logic: `{"some":[{"var":"items"},true]}`,
			want: func(d Dialect) string {
				if d == DialectClickHouse {
					return "arrayExists(elem -> TRUE, items)"
				}
				return testDuckDBUnnestSourceAliases(d, "EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE TRUE)")
			},
			wantParam: func(d Dialect) string {
				if d == DialectClickHouse {
					return "arrayExists(elem -> TRUE, items)"
				}
				return testDuckDBUnnestSourceAliases(d, "EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE TRUE)")
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
					got, err := tr.TranspileCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if want := tt.want(d); got != want {
						t.Fatalf("TranspileCondition() = %q, want %q", got, want)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if !reflect.DeepEqual(gotParams, tt.params) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.params)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_DecisiveBooleanConstantsShortCircuit(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "x", Type: FieldTypeNumber},
	})

	tests := []struct {
		name  string
		logic string
		want  string
	}{
		{
			name:  "and false skips later schema error",
			logic: `{"and":[false,{">":[{"var":"missing"},1]}]}`,
			want:  "FALSE",
		},
		{
			name:  "or true skips later schema error",
			logic: `{"or":[true,{">":[{"var":"missing"},1]}]}`,
			want:  "TRUE",
		},
		{
			name:  "and false after dynamic operand rolls back params",
			logic: `{"and":[{">":[{"var":"x"},1]},false,{">":[{"var":"missing"},1]}]}`,
			want:  "FALSE",
		},
		{
			name:  "or true after dynamic operand rolls back params",
			logic: `{"or":[{">":[{"var":"x"},1]},true,{">":[{"var":"missing"},1]}]}`,
			want:  "TRUE",
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
		})
	}
}

func TestTranspileCondition_RejectsValueOperandsInPredicateContexts(t *testing.T) {
	tests := []struct {
		name  string
		logic string
	}{
		{
			name:  "logical operand cannot be value fallback",
			logic: `{"and":[{"or":[0,5]},{">":[{"var":"amount"},1]}]}`,
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
					if _, err := tr.TranspileCondition(tt.logic); !IsErrorCode(err, ErrInvalidExpressionContext) {
						t.Fatalf("TranspileCondition() error = %v, want %s", err, ErrInvalidExpressionContext)
					}

					_, params, err := tr.TranspileParameterizedCondition(tt.logic)
					if !IsErrorCode(err, ErrInvalidExpressionContext) {
						t.Fatalf("TranspileParameterizedCondition() error = %v, want %s", err, ErrInvalidExpressionContext)
					}
					if len(params) != 0 {
						t.Fatalf("params = %#v, want none", params)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_ArrayPredicateLambdasAcceptTruthinessFallbacks(t *testing.T) {
	t.Parallel()

	logic := `{"some":[{"var":"items"},{"or":[0,{"==":[{"var":""},1]}]}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			want := "EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE elem = 1)"
			wantParam := "EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE elem = " + testPlaceholder(d, 1) + ")"
			if d == DialectClickHouse {
				want = "arrayExists(elem -> elem = 1, items)"
				wantParam = "arrayExists(elem -> elem = " + testPlaceholder(d, 1) + ", items)"
			} else {
				want = testDuckDBUnnestSourceAliases(d, want)
				wantParam = testDuckDBUnnestSourceAliases(d, wantParam)
			}

			got, err := tr.TranspileCondition(logic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			if got != want {
				t.Fatalf("TranspileCondition() = %q, want %q", got, want)
			}

			gotParam, gotParams, err := tr.TranspileParameterizedCondition(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotParam != wantParam {
				t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, wantParam)
			}
			wantParams := []QueryParam{{Name: "p1", Value: float64(1)}}
			if !reflect.DeepEqual(gotParams, wantParams) {
				t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
			}
		})
	}
}

func TestTranspileCondition_RejectsValueCustomOperator(t *testing.T) {
	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			err = tr.RegisterOperatorFunc("valueOnly", func(_ string, _ []OperatorArg) (OperatorResult, error) {
				return ValueSQL("VALUE_ONLY()", ExpressionTypeString), nil
			})
			if err != nil {
				t.Fatalf("RegisterOperatorFunc() error = %v", err)
			}

			logic := `{"valueOnly":[{"var":"name"}]}`
			_, err = tr.TranspileCondition(logic)
			if !IsErrorCode(err, ErrInvalidExpressionContext) {
				t.Fatalf("TranspileCondition() error = %v, want %s", err, ErrInvalidExpressionContext)
			}

			_, params, err := tr.TranspileParameterizedCondition(logic)
			if !IsErrorCode(err, ErrInvalidExpressionContext) {
				t.Fatalf("TranspileParameterizedCondition() error = %v, want %s", err, ErrInvalidExpressionContext)
			}
			if len(params) != 0 {
				t.Fatalf("params = %#v, want none", params)
			}
		})
	}
}

func TestTranspileCondition_TypedCustomOperatorUsesPredicateContext(t *testing.T) {
	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			err = tr.RegisterOperatorFunc("isGreater", func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 2 {
					return OperatorResult{}, fmt.Errorf("isGreater requires exactly 2 arguments")
				}
				return PredicateSQL(fmt.Sprintf("%s>%s", args[0].SQL, args[1].SQL)), nil
			})
			if err != nil {
				t.Fatalf("RegisterOperatorFunc() error = %v", err)
			}

			logic := `{"isGreater":[{"var":"amount"},10]}`
			got, err := tr.TranspileCondition(logic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			if got != "amount>10" {
				t.Fatalf("TranspileCondition() = %q, want %q", got, "amount>10")
			}

			gotParam, gotParams, err := tr.TranspileParameterizedCondition(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			wantParam := fmt.Sprintf("amount>%s", testPlaceholder(d, 1))
			if gotParam != wantParam {
				t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, wantParam)
			}
			wantParams := []QueryParam{{Name: "p1", Value: float64(10)}}
			if !reflect.DeepEqual(gotParams, wantParams) {
				t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
			}
		})
	}
}

func TestTranspileCondition_TypedCustomPredicateTruthiness(t *testing.T) {
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

			tests := []struct {
				name  string
				logic string
				want  string
			}{
				{
					name:  "not custom predicate",
					logic: `{"!":{"isPositive":[{"var":"amount"}]}}`,
					want:  "NOT (amount > 0)",
				},
				{
					name:  "double-not custom predicate",
					logic: `{"!!":{"isPositive":[{"var":"amount"}]}}`,
					want:  "amount > 0",
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := tr.TranspileCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if got != tt.want {
						t.Fatalf("TranspileCondition() = %q, want %q", got, tt.want)
					}

					gotParam, params, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if gotParam != tt.want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, tt.want)
					}
					if len(params) != 0 {
						t.Fatalf("params = %#v, want none", params)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_DefaultedVarMetadataSurvivesValueFolding(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "age", Type: FieldTypeInteger},
		{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"active"}},
	})

	tests := []struct {
		name          string
		logic         string
		want          string
		wantParam     func(Dialect) string
		params        []QueryParam
		wantErr       bool
		wantErrParams bool
	}{
		{
			name:  "constant if keeps numeric defaulted field",
			logic: `{"==":[{"if":[true,{"var":["age",1.5]},0]},1.5]}`,
			want:  "COALESCE(age, 1.5) = 1.5",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("COALESCE(age, %s) = %s", testPlaceholder(d, 1), testPlaceholder(d, 2))
			},
			params: []QueryParam{
				{Name: "p1", Value: float64(1.5)},
				{Name: "p2", Value: float64(1.5)},
			},
		},
		{
			name:  "constant logical keeps numeric defaulted field",
			logic: `{"==":[{"or":[false,{"var":["age",1.5]}]},1.5]}`,
			want:  "COALESCE(age, 1.5) = 1.5",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("COALESCE(age, %s) = %s", testPlaceholder(d, 1), testPlaceholder(d, 2))
			},
			params: []QueryParam{
				{Name: "p1", Value: float64(1.5)},
				{Name: "p2", Value: float64(1.5)},
			},
		},
		{
			name:          "constant if validates enum default",
			logic:         `{"==":[{"if":[true,{"var":["status","bogus"]},"active"]},"active"]}`,
			wantErr:       true,
			wantErrParams: true,
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
					got, err := tr.TranspileCondition(tt.logic)
					if tt.wantErr {
						if err == nil {
							t.Fatal("TranspileCondition() expected error")
						}
					} else {
						if err != nil {
							t.Fatalf("TranspileCondition() error = %v", err)
						}
						if got != tt.want {
							t.Fatalf("TranspileCondition() = %q, want %q", got, tt.want)
						}
					}

					gotParam, gotParams, err := tr.TranspileParameterizedCondition(tt.logic)
					if tt.wantErrParams {
						if err == nil {
							t.Fatal("TranspileParameterizedCondition() expected error")
						}
						return
					}
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if !reflect.DeepEqual(gotParams, tt.params) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.params)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_MixedTypedCustomOperatorsAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "name", Type: FieldTypeString},
		{Name: "score", Type: FieldTypeNumber},
	})

	logic := `{"and":[{">":[{"strlen":[{"var":"name"}]},3]},{"isPositive":[{"var":"score"}]}]}`

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
					registerErr := tr.RegisterOperatorFunc("strlen", func(_ string, args []OperatorArg) (OperatorResult, error) {
						if len(args) != 1 {
							return OperatorResult{}, fmt.Errorf("strlen requires exactly 1 argument")
						}
						return ValueSQL(fmt.Sprintf("LENGTH(%s)", args[0].SQL), ExpressionTypeNumber), nil
					})
					if registerErr != nil {
						t.Fatalf("RegisterOperatorFunc(strlen) error = %v", registerErr)
					}
					registerErr = tr.RegisterOperatorFunc("isPositive", func(_ string, args []OperatorArg) (OperatorResult, error) {
						if len(args) != 1 {
							return OperatorResult{}, fmt.Errorf("isPositive requires exactly 1 argument")
						}
						return PredicateSQL(fmt.Sprintf("%s > 0", args[0].SQL)), nil
					})
					if registerErr != nil {
						t.Fatalf("RegisterOperatorFunc(isPositive) error = %v", registerErr)
					}

					got, err := tr.TranspileCondition(logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if want := "(LENGTH(name) > 3 AND score > 0)"; got != want {
						t.Fatalf("TranspileCondition() = %q, want %q", got, want)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedCondition(logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					wantParam := fmt.Sprintf("(LENGTH(name) > %s AND score > 0)", testPlaceholder(d, 1))
					if gotParam != wantParam {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, wantParam)
					}
					wantParams := []QueryParam{{Name: "p1", Value: float64(3)}}
					if !reflect.DeepEqual(gotParams, wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_StrictEqualityMismatchedTypedExpressions(t *testing.T) {
	tests := []struct {
		name  string
		logic string
		want  string
	}{
		{
			name:  "strict equality folds string expression versus number expression",
			logic: `{"===":[{"cat":["5"]},{"+":[2,3]}]}`,
			want:  "FALSE",
		},
		{
			name:  "strict inequality folds string expression versus number expression",
			logic: `{"!==":[{"cat":["5"]},{"+":[2,3]}]}`,
			want:  "TRUE",
		},
		{
			name:  "strict equality folds array expression versus number literal",
			logic: `{"===":[{"map":[[1,2],{"var":""}]},5]}`,
			want:  "FALSE",
		},
		{
			name:  "strict inequality folds array expression versus number literal",
			logic: `{"!==":[{"map":[[1,2],{"var":""}]},5]}`,
			want:  "TRUE",
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
					got, err := tr.TranspileCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if got != tt.want {
						t.Fatalf("TranspileCondition() = %q, want %q", got, tt.want)
					}

					gotParam, params, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if gotParam != tt.want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, tt.want)
					}
					if len(params) != 0 {
						t.Fatalf("params = %#v, want none", params)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_NullTypedValueEqualityAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{{Name: "flag", Type: FieldTypeBoolean}})
	nullExpr := `{"if":[{"var":"flag"},null,null]}`

	tests := []struct {
		name  string
		logic string
		want  string
	}{
		{
			name:  "null expression equals non-null literal",
			logic: fmt.Sprintf(`{"==":[%s,5]}`, nullExpr),
			want:  "FALSE",
		},
		{
			name:  "null expression does not equal non-null literal",
			logic: fmt.Sprintf(`{"!=":[%s,5]}`, nullExpr),
			want:  "TRUE",
		},
		{
			name:  "null expression strictly equals null literal",
			logic: fmt.Sprintf(`{"===":[%s,null]}`, nullExpr),
			want:  "TRUE",
		},
		{
			name:  "null expression strictly does not equal null literal",
			logic: fmt.Sprintf(`{"!==":[%s,null]}`, nullExpr),
			want:  "FALSE",
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
					got, err := tr.TranspileCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if got != tt.want {
						t.Fatalf("TranspileCondition() = %q, want %q", got, tt.want)
					}

					gotParam, params, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if gotParam != tt.want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, tt.want)
					}
					if len(params) != 0 {
						t.Fatalf("params = %#v, want none", params)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_StrictEqualitySchemaFieldExpressionMismatches(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "num", Type: FieldTypeNumber},
		{Name: "code", Type: FieldTypeString},
	})

	tests := []struct {
		name  string
		logic string
		want  string
	}{
		{
			name:  "numeric field strict equals string expression",
			logic: `{"===":[{"var":"num"},{"cat":["5"]}]}`,
			want:  "FALSE",
		},
		{
			name:  "numeric field strict not equals string expression",
			logic: `{"!==":[{"var":"num"},{"cat":["5"]}]}`,
			want:  "TRUE",
		},
		{
			name:  "string expression strict equals numeric field",
			logic: `{"===":[{"cat":["5"]},{"var":"num"}]}`,
			want:  "FALSE",
		},
		{
			name:  "string field strict equals numeric expression",
			logic: `{"===":[{"var":"code"},{"+":[2,3]}]}`,
			want:  "FALSE",
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
		})
	}
}

func TestTranspileCondition_ComparisonOperandsUseValueSemantics(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{{Name: "x", Type: FieldTypeNumber}})

	tests := []struct {
		name       string
		logic      string
		wantSQL    string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:    "literal truthy if condition folds before comparison",
			logic:   `{"==":[{"if":["nonempty","x","y"]},"x"]}`,
			wantSQL: "TRUE",
			wantParam: func(Dialect) string {
				return "TRUE"
			},
		},
		{
			name:    "unreachable comparison operand branch is not parsed",
			logic:   `{"<":[{"if":[false,{"var":"bad-name"},3]},4]}`,
			wantSQL: "TRUE",
			wantParam: func(Dialect) string {
				return "TRUE"
			},
		},
		{
			name:    "value logical fallback folds before comparison",
			logic:   `{"==":[{"or":[0,5]},5]}`,
			wantSQL: "TRUE",
			wantParam: func(Dialect) string {
				return "TRUE"
			},
		},
		{
			name:    "nested predicate materializes before equality",
			logic:   `{"==":[{">":[{"var":"x"},1]},false]}`,
			wantSQL: "CASE WHEN x > 1 THEN TRUE ELSE FALSE END = FALSE",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN x > %s THEN TRUE ELSE FALSE END = FALSE", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
		{
			name:    "nested predicate materializes before array membership",
			logic:   `{"in":[{">":[{"var":"x"},1]},[false]]}`,
			wantSQL: "CASE WHEN x > 1 THEN TRUE ELSE FALSE END IN (FALSE)",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN x > %s THEN TRUE ELSE FALSE END IN (FALSE)", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
		{
			name:    "nested predicate coerces to number before ordering",
			logic:   `{">":[{"==":[{"var":"x"},1]},0]}`,
			wantSQL: "(CASE WHEN x = 1 THEN 1 ELSE 0 END) > 0",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("(CASE WHEN x = %s THEN 1 ELSE 0 END) > %s", testPlaceholder(d, 1), testPlaceholder(d, 2))
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(0)},
			},
		},
		{
			name:    "chained comparison coerces nested predicate to number",
			logic:   `{"<":[0,{"==":[{"var":"x"},1]},2]}`,
			wantSQL: "(0 < (CASE WHEN x = 1 THEN 1 ELSE 0 END) AND (CASE WHEN x = 1 THEN 1 ELSE 0 END) < 2)",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("(%s < (CASE WHEN x = %s THEN 1 ELSE 0 END) AND (CASE WHEN x = %s THEN 1 ELSE 0 END) < %s)",
					testPlaceholder(d, 2), testPlaceholder(d, 1), testPlaceholder(d, 1), testPlaceholder(d, 3))
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(0)},
				{Name: "p3", Value: float64(2)},
			},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
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
							got, err := tr.TranspileCondition(tt.logic)
							if err != nil {
								t.Fatalf("TranspileCondition() error = %v", err)
							}
							if got != tt.wantSQL {
								t.Fatalf("TranspileCondition() = %q, want %q", got, tt.wantSQL)
							}

							gotParam, gotParams, err := tr.TranspileParameterizedCondition(tt.logic)
							if err != nil {
								t.Fatalf("TranspileParameterizedCondition() error = %v", err)
							}
							if want := tt.wantParam(d); gotParam != want {
								t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
							}
							if len(tt.wantParams) == 0 && len(gotParams) == 0 {
								return
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

func TestTranspileCondition_InRightHandValueExpressionsAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{{Name: "flag", Type: FieldTypeBoolean}})

	stringContainmentSQL := func(d Dialect, haystack, needle string) string {
		switch d {
		case DialectPostgreSQL:
			return fmt.Sprintf("POSITION(%s IN %s) > 0", needle, haystack)
		case DialectClickHouse:
			return fmt.Sprintf("position(%s, %s) > 0", haystack, needle)
		default:
			return fmt.Sprintf("STRPOS(%s, %s) > 0", haystack, needle)
		}
	}
	arrayLiteralSQL := func(d Dialect, first, second string) string {
		if d == DialectPostgreSQL {
			return fmt.Sprintf("ARRAY[%s, %s]", first, second)
		}
		return fmt.Sprintf("[%s, %s]", first, second)
	}
	arrayMembershipSQL := func(d Dialect, value, array string) string {
		return testNullSafeArrayMembershipSQL(d, value, array)
	}

	tests := []struct {
		name       string
		logic      string
		wantSQL    func(Dialect) string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "string-producing rhs uses containment",
			logic: `{"in":["b",{"cat":["abc"]}]}`,
			wantSQL: func(d Dialect) string {
				return stringContainmentSQL(d, "CONCAT('abc')", "'b'")
			},
			wantParam: func(d Dialect) string {
				return stringContainmentSQL(d, fmt.Sprintf("CONCAT(%s)", testPlaceholder(d, 1)), testPlaceholder(d, 2))
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: "abc"},
				{Name: "p2", Value: "b"},
			},
		},
		{
			name:  "array-producing rhs uses dialect membership",
			logic: `{"in":[1,{"if":[{"var":"flag"},[1,2],[3,4]]}]}`,
			wantSQL: func(d Dialect) string {
				array := fmt.Sprintf("CASE WHEN flag IS TRUE THEN %s ELSE %s END",
					arrayLiteralSQL(d, "1", "2"),
					arrayLiteralSQL(d, "3", "4"))
				return arrayMembershipSQL(d, "1", array)
			},
			wantParam: func(d Dialect) string {
				array := fmt.Sprintf("CASE WHEN flag IS TRUE THEN %s ELSE %s END",
					arrayLiteralSQL(d, testPlaceholder(d, 1), testPlaceholder(d, 2)),
					arrayLiteralSQL(d, testPlaceholder(d, 3), testPlaceholder(d, 4)))
				return arrayMembershipSQL(d, testPlaceholder(d, 5), array)
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: float64(3)},
				{Name: "p4", Value: float64(4)},
				{Name: "p5", Value: float64(1)},
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
					got, err := tr.TranspileCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if want := tt.wantSQL(d); got != want {
						t.Fatalf("TranspileCondition() = %q, want %q", got, want)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if len(tt.wantParams) == 0 && len(gotParams) == 0 {
						return
					}
					if !reflect.DeepEqual(gotParams, tt.wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_InStringHaystackStringifiesNeedlesAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeNumber},
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "haystack", Type: FieldTypeString},
		{Name: "name", Type: FieldTypeString},
		{Name: "needle", Type: FieldTypeString},
	})

	stringContainmentSQL := func(d Dialect, haystack, needle string) string {
		switch d {
		case DialectPostgreSQL:
			return fmt.Sprintf("POSITION(%s IN %s) > 0", needle, haystack)
		case DialectClickHouse:
			return fmt.Sprintf("position(%s, %s) > 0", haystack, needle)
		default:
			return fmt.Sprintf("STRPOS(%s, %s) > 0", haystack, needle)
		}
	}
	runtimeNeedleStringContainmentSQL := func(d Dialect, haystack, needle string) string {
		return fmt.Sprintf(
			"((%s = '' AND (%s IS NOT NULL AND %s != '')) OR (%s != '' AND %s))",
			needle,
			haystack,
			haystack,
			needle,
			stringContainmentSQL(d, haystack, needle),
		)
	}
	stringCastSQL := func(d Dialect, expr string) string {
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
	jsonStringCastSQL := func(d Dialect, expr string) string {
		return fmt.Sprintf("COALESCE(%s, 'null')", stringCastSQL(d, expr))
	}
	boolStringSQL := func(expr string) string {
		return fmt.Sprintf("CASE WHEN %s IS TRUE THEN 'true' WHEN %s IS FALSE THEN 'false' ELSE 'null' END", expr, expr)
	}

	tests := []struct {
		name       string
		logic      string
		wantSQL    func(Dialect) string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "schema numeric field needle casts for string containment",
			logic: `{"in":[{"var":"amount"},"12345"]}`,
			wantSQL: func(d Dialect) string {
				return stringContainmentSQL(d, "'12345'", jsonStringCastSQL(d, "amount"))
			},
			wantParam: func(d Dialect) string {
				return runtimeNeedleStringContainmentSQL(d, testPlaceholder(d, 1), jsonStringCastSQL(d, "amount"))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "12345"}},
		},
		{
			name:  "schema boolean field needle uses jsonlogic string form",
			logic: `{"in":[{"var":"flag"},"true"]}`,
			wantSQL: func(d Dialect) string {
				return stringContainmentSQL(d, "'true'", boolStringSQL("flag"))
			},
			wantParam: func(d Dialect) string {
				return runtimeNeedleStringContainmentSQL(d, testPlaceholder(d, 1), boolStringSQL("flag"))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "true"}},
		},
		{
			name:  "schema string field needle preserves null string coercion",
			logic: `{"in":[{"var":"name"},"null"]}`,
			wantSQL: func(d Dialect) string {
				return stringContainmentSQL(d, "'null'", "COALESCE(name, 'null')")
			},
			wantParam: func(d Dialect) string {
				return runtimeNeedleStringContainmentSQL(d, testPlaceholder(d, 1), "COALESCE(name, 'null')")
			},
			wantParams: []QueryParam{{Name: "p1", Value: "null"}},
		},
		{
			name:  "runtime string field needle requires non-empty haystack when needle is empty",
			logic: `{"in":[{"var":"needle"},{"var":"haystack"}]}`,
			wantSQL: func(d Dialect) string {
				return runtimeNeedleStringContainmentSQL(d, "haystack", "COALESCE(needle, 'null')")
			},
			wantParam: func(d Dialect) string {
				return runtimeNeedleStringContainmentSQL(d, "haystack", "COALESCE(needle, 'null')")
			},
			wantParams: []QueryParam{},
		},
		{
			name:  "literal number needle stringifies for typed string expression",
			logic: `{"in":[3,{"cat":["12345"]}]}`,
			wantSQL: func(d Dialect) string {
				return stringContainmentSQL(d, "CONCAT('12345')", "'3'")
			},
			wantParam: func(d Dialect) string {
				return stringContainmentSQL(d, fmt.Sprintf("CONCAT(%s)", testPlaceholder(d, 1)), testPlaceholder(d, 2))
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: "12345"},
				{Name: "p2", Value: "3"},
			},
		},
		{
			name:  "empty string needle requires non-empty string field",
			logic: `{"in":["",{"var":"name"}]}`,
			wantSQL: func(Dialect) string {
				return "(name IS NOT NULL AND name != '')"
			},
			wantParam: func(Dialect) string {
				return "(name IS NOT NULL AND name != '')"
			},
			wantParams: []QueryParam{},
		},
		{
			name:  "empty array needle stringifies to empty string",
			logic: `{"in":[[],{"var":"name"}]}`,
			wantSQL: func(Dialect) string {
				return "(name IS NOT NULL AND name != '')"
			},
			wantParam: func(Dialect) string {
				return "(name IS NOT NULL AND name != '')"
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
					got, err := tr.TranspileCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if want := tt.wantSQL(d); got != want {
						t.Fatalf("TranspileCondition() = %q, want %q", got, want)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if len(tt.wantParams) == 0 && len(gotParams) == 0 {
						return
					}
					if !reflect.DeepEqual(gotParams, tt.wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
					}
				})
			}
		})
	}
}

func TestTranspileParameterizedCondition_InUnknownRHSStringContainmentDoesNotLeakParamsAllDialects(t *testing.T) {
	t.Parallel()

	stringContainmentSQL := func(d Dialect, haystack, needle string) string {
		switch d {
		case DialectPostgreSQL:
			return fmt.Sprintf("POSITION(%s IN %s) > 0", needle, haystack)
		case DialectClickHouse:
			return fmt.Sprintf("position(%s, %s) > 0", haystack, needle)
		default:
			return fmt.Sprintf("STRPOS(%s, %s) > 0", haystack, needle)
		}
	}
	nonEmptyStringSQL := func(sql string) string {
		return fmt.Sprintf("(%s IS NOT NULL AND %s != '')", sql, sql)
	}

	tests := []struct {
		name       string
		logic      string
		register   func(*Transpiler) error
		wantSQL    func(Dialect) string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "literal needle with unknown typed if rhs",
			logic: `{"in":["x",{"if":[{">":[{"var":"a"},0]},{"var":"s"},{"var":"t"}]}]}`,
			wantSQL: func(d Dialect) string {
				rhs := "CASE WHEN a > 0 THEN s ELSE t END"
				return stringContainmentSQL(d, rhs, "'x'")
			},
			wantParam: func(d Dialect) string {
				rhs := fmt.Sprintf("CASE WHEN a > %s THEN s ELSE t END", testPlaceholder(d, 1))
				return stringContainmentSQL(d, rhs, testPlaceholder(d, 2))
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: "x"},
			},
		},
		{
			name:  "empty literal needle with unknown typed if rhs",
			logic: `{"in":["",{"if":[{">":[{"var":"a"},0]},{"var":"s"},{"var":"t"}]}]}`,
			wantSQL: func(Dialect) string {
				return nonEmptyStringSQL("CASE WHEN a > 0 THEN s ELSE t END")
			},
			wantParam: func(d Dialect) string {
				rhs := fmt.Sprintf("CASE WHEN a > %s THEN s ELSE t END", testPlaceholder(d, 1))
				return nonEmptyStringSQL(rhs)
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(0)}},
		},
		{
			name:  "literal needle with custom unknown typed rhs",
			logic: `{"in":["x",{"unknownHaystack":[]}]}`,
			register: func(tr *Transpiler) error {
				return tr.RegisterOperatorFunc("unknownHaystack", func(string, []OperatorArg) (OperatorResult, error) {
					return ValueSQL("custom_haystack", ExpressionTypeUnknown), nil
				})
			},
			wantSQL: func(d Dialect) string {
				return stringContainmentSQL(d, "custom_haystack", "'x'")
			},
			wantParam: func(d Dialect) string {
				return stringContainmentSQL(d, "custom_haystack", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "x"}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					tr, err := NewTranspiler(d, defaultTestSchema())
					if err != nil {
						t.Fatalf("NewTranspiler() error = %v", err)
					}
					if tt.register != nil {
						if registerErr := tt.register(tr); registerErr != nil {
							t.Fatalf("RegisterOperatorFunc() error = %v", registerErr)
						}
					}

					got, err := tr.TranspileCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if want := tt.wantSQL(d); got != want {
						t.Fatalf("TranspileCondition() = %q, want %q", got, want)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if !reflect.DeepEqual(gotParams, tt.wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_LiteralComparisonsEmitFoldedBooleansAllDialects(t *testing.T) {
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
			name:  "literal in empty string haystack is false",
			logic: `{"in":["",""]}`,
			want:  "FALSE",
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
			name:  "empty array needle is false against empty string haystack",
			logic: `{"in":[[],""]}`,
			want:  "FALSE",
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
		})
	}
}

func TestTranspileCondition_SchemaRequiredComparisonUsesFoldedValueLiterals(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "code", Type: FieldTypeString},
		{Name: "amount", Type: FieldTypeInteger},
	})

	tests := []struct {
		name       string
		logic      string
		wantSQL    string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:    "strict string field equality folds impossible folded number literal",
			logic:   `{"===":[{"var":"code"},{"or":[0,5]}]}`,
			wantSQL: "FALSE",
			wantParam: func(Dialect) string {
				return "FALSE"
			},
			wantParams: []QueryParam{},
		},
		{
			name:    "strict string field inequality folds impossible folded number literal",
			logic:   `{"!==":[{"var":"code"},{"or":[0,5]}]}`,
			wantSQL: "TRUE",
			wantParam: func(Dialect) string {
				return "TRUE"
			},
			wantParams: []QueryParam{},
		},
		{
			name:    "loose string field equality coerces folded number literal",
			logic:   `{"==":[{"var":"code"},{"or":[0,5]}]}`,
			wantSQL: "code = '5'",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("code = %s", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "5"}},
		},
		{
			name:    "loose integer field equality coerces folded string literal",
			logic:   `{"==":[{"var":"amount"},{"or":["","5"]}]}`,
			wantSQL: "amount = 5",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("amount = %s", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: int64(5)}},
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
					got, err := tr.TranspileCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if got != tt.wantSQL {
						t.Fatalf("TranspileCondition() = %q, want %q", got, tt.wantSQL)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if len(tt.wantParams) == 0 && len(gotParams) == 0 {
						return
					}
					if !reflect.DeepEqual(gotParams, tt.wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_NestedValueOperandPreservesFieldMetadata(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "code", Type: FieldTypeString},
		{Name: "flag", Type: FieldTypeBoolean},
	})

	tests := []struct {
		name       string
		logic      string
		wantSQL    string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:    "loose equality coerces literal for folded string field",
			logic:   `{"==":[{"if":[true,{"var":"code"},"x"]},5]}`,
			wantSQL: "code = '5'",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("code = %s", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "5"}},
		},
		{
			name:    "loose equality coerces literal for dynamic string expression",
			logic:   `{"==":[{"if":[{"var":"flag"},{"var":"code"},"5"]},5]}`,
			wantSQL: "CASE WHEN flag IS TRUE THEN code ELSE '5' END = '5'",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN flag IS TRUE THEN code ELSE %s END = %s",
					testPlaceholder(d, 1), testPlaceholder(d, 2))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "5"}, {Name: "p2", Value: "5"}},
		},
		{
			name:    "strict equality folds impossible folded string field comparison",
			logic:   `{"===":[{"if":[true,{"var":"code"},"x"]},5]}`,
			wantSQL: "FALSE",
			wantParam: func(Dialect) string {
				return "FALSE"
			},
			wantParams: nil,
		},
		{
			name:    "strict equality folds impossible dynamic string expression",
			logic:   `{"===":[{"if":[{"var":"flag"},{"var":"code"},"x"]},5]}`,
			wantSQL: "FALSE",
			wantParam: func(Dialect) string {
				return "FALSE"
			},
			wantParams: []QueryParam{},
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
					got, err := tr.TranspileCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if got != tt.wantSQL {
						t.Fatalf("TranspileCondition() = %q, want %q", got, tt.wantSQL)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if !reflect.DeepEqual(gotParams, tt.wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_LiteralPredicateResultsShortCircuit(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "code", Type: FieldTypeString},
	})

	tests := []struct {
		name  string
		logic string
		want  string
	}{
		{
			name:  "false literal comparison stops and before missing field",
			logic: `{"and":[{"!=":[null,null]},{">":[{"var":"missing"},1]}]}`,
			want:  "FALSE",
		},
		{
			name:  "true literal comparison stops or before missing field",
			logic: `{"or":[{"==":[1,1]},{">":[{"var":"missing"},1]}]}`,
			want:  "TRUE",
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
		})
	}
}

func TestTranspileParameterizedCondition_FoldedPredicateShortCircuitRollsBackParams(t *testing.T) {
	tests := []struct {
		name       string
		logic      string
		wantSQL    string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:    "and skips truthy literal comparison",
			logic:   `{"and":[{"==":[1,1]},{">":[{"var":"x"},0]}]}`,
			wantSQL: "x > 0",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("x > %s", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(0)}},
		},
		{
			name:    "or skips falsy literal comparison",
			logic:   `{"or":[{"==":[1,2]},{">":[{"var":"x"},0]}]}`,
			wantSQL: "x > 0",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("x > %s", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(0)}},
		},
		{
			name:    "if false condition skips condition params",
			logic:   `{"if":[{"==":[1,2]},{">":[{"var":"x"},0]},{">":[{"var":"y"},0]}]}`,
			wantSQL: "y > 0",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("y > %s", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(0)}},
		},
		{
			name:    "if true condition skips condition params",
			logic:   `{"if":[{"==":[1,1]},{">":[{"var":"x"},0]},{">":[{"var":"y"},0]}]}`,
			wantSQL: "x > 0",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("x > %s", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(0)}},
		},
		{
			name:    "if true folded condition after dynamic branch preserves earlier params",
			logic:   `{"if":[{">":[{"var":"a"},0]},{">":[{"var":"b"},0]},{"==":[1,1]},{">":[{"var":"c"},0]},{">":[{"var":"d"},0]}]}`,
			wantSQL: "CASE WHEN a > 0 THEN b > 0 ELSE c > 0 END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN a > %s THEN b > %s ELSE c > %s END",
					testPlaceholder(d, 1), testPlaceholder(d, 2), testPlaceholder(d, 3))
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(0)},
				{Name: "p3", Value: float64(0)},
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
					got, err := tr.TranspileCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if got != tt.wantSQL {
						t.Fatalf("TranspileCondition() = %q, want %q", got, tt.wantSQL)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if !reflect.DeepEqual(gotParams, tt.wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.wantParams)
					}
				})
			}
		})
	}
}

func TestTranspileCondition_DoubleBangUsesValueTruthinessExplicitly(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
	})

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			logic := `{"!!":{"or":[0,{"var":"flag"}]}}`
			got, err := tr.TranspileCondition(logic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			if got != "flag IS TRUE" {
				t.Fatalf("TranspileCondition() = %q, want %q", got, "flag IS TRUE")
			}

			gotParam, params, err := tr.TranspileParameterizedCondition(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotParam != "flag IS TRUE" {
				t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, "flag IS TRUE")
			}
			if len(params) != 0 {
				t.Fatalf("params = %#v, want none", params)
			}
		})
	}
}

func TestTranspileCondition_UnaryEmptyArrayTruthinessAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
	})

	tests := []struct {
		name  string
		logic string
		want  string
	}{
		{
			name:  "not empty array",
			logic: `{"!":[[]]}`,
			want:  "TRUE",
		},
		{
			name:  "double bang empty array",
			logic: `{"!!":[[]]}`,
			want:  "FALSE",
		},
		{
			name:  "nested in logical",
			logic: `{"and":[{"!":[[]]},{"==":[1,1]}]}`,
			want:  "TRUE",
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
							got, err := tr.TranspileCondition(tt.logic)
							if err != nil {
								t.Fatalf("TranspileCondition() error = %v", err)
							}
							if got != tt.want {
								t.Fatalf("TranspileCondition() = %q, want %q", got, tt.want)
							}

							gotParam, params, err := tr.TranspileParameterizedCondition(tt.logic)
							if err != nil {
								t.Fatalf("TranspileParameterizedCondition() error = %v", err)
							}
							if gotParam != tt.want {
								t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, tt.want)
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

func TestTranspile_RejectsMalformedNestedValueOperandsAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "x", Type: FieldTypeNumber},
		{Name: "items", Type: FieldTypeArray},
	})

	tests := []struct {
		name  string
		logic string
		value bool
	}{
		{
			name:  "comparison operand arithmetic contains multi-key var",
			logic: `{">":[{"+":[{"var":"x","extra":true},1]},2]}`,
		},
		{
			name:  "value arithmetic contains multi-key var",
			logic: `{"+":[{"var":"x","extra":true},1]}`,
			value: true,
		},
		{
			name:  "array lambda comparison operand contains multi-key var",
			logic: `{"some":[{"var":"items"},{">":[{"+":[{"var":"value","extra":true},1]},2]}]}`,
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
							if tt.value {
								if _, err := tr.TranspileValue(tt.logic); !IsErrorCode(err, ErrMultipleKeys) {
									t.Fatalf("TranspileValue() error = %v, want %s", err, ErrMultipleKeys)
								}
								sql, params, err := tr.TranspileParameterizedValue(tt.logic)
								if !IsErrorCode(err, ErrMultipleKeys) {
									t.Fatalf("TranspileParameterizedValue() error = %v, want %s (SQL %q params %#v)",
										err, ErrMultipleKeys, sql, params)
								}
								if len(params) != 0 {
									t.Fatalf("params = %#v, want none", params)
								}
								return
							}

							if _, err := tr.TranspileCondition(tt.logic); !IsErrorCode(err, ErrMultipleKeys) {
								t.Fatalf("TranspileCondition() error = %v, want %s", err, ErrMultipleKeys)
							}
							sql, params, err := tr.TranspileParameterizedCondition(tt.logic)
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
		})
	}
}
