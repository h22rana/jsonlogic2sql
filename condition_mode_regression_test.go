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
			tr, err := NewTranspiler(d)
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

func TestTranspileCondition_PredicateIfRejectsSchemaLessValueCondition(t *testing.T) {
	t.Parallel()

	logic := `{"if":[{"var":"flag"},{">":[{"var":"amount"},0]},false]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			if _, conditionErr := tr.TranspileCondition(logic); !IsErrorCode(conditionErr, ErrInvalidExpressionContext) {
				t.Fatalf("TranspileCondition() error = %v, want %s", conditionErr, ErrInvalidExpressionContext)
			}
			sql, params, paramErr := tr.TranspileParameterizedCondition(logic)
			if !IsErrorCode(paramErr, ErrInvalidExpressionContext) {
				t.Fatalf("TranspileParameterizedCondition() error = %v, want %s (SQL %q params %#v)",
					paramErr, ErrInvalidExpressionContext, sql, params)
			}
			if len(params) != 0 {
				t.Fatalf("params = %#v, want none", params)
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

func TestTranspileCondition_CustomPredicateBooleanConstantsShortCircuitAllDialectsSchemaModes(t *testing.T) {
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
				return "EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE TRUE)"
			},
			wantParam: func(d Dialect) string {
				if d == DialectClickHouse {
					return "arrayExists(elem -> TRUE, items)"
				}
				return "EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE TRUE)"
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
		{
			name:  "array predicate cannot be value fallback",
			logic: `{"some":[{"var":"items"},{"or":[0,{"==":[{"var":"current"},1]}]}]}`,
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

func TestTranspileCondition_RejectsValueCustomOperator(t *testing.T) {
	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
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
			tr, err := NewTranspiler(d)
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

func TestTranspileCondition_MixedTypedCustomOperatorsAllDialectsSchemaModes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "name", Type: FieldTypeString},
		{Name: "score", Type: FieldTypeNumber},
	})

	logic := `{"and":[{">":[{"strlen":[{"var":"name"}]},3]},{"isPositive":[{"var":"score"}]}]}`

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
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
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
				{name: "schema-less"},
				{name: "schema-aware", schema: schema},
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

			tr, err := NewTranspiler(d)
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

func TestTranspileCondition_SchemaAwareComparisonUsesFoldedValueLiterals(t *testing.T) {
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
			tr, err := NewTranspiler(d)
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

func TestTranspileCondition_UnaryEmptyArrayTruthinessAllDialectsSchemaModes(t *testing.T) {
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
