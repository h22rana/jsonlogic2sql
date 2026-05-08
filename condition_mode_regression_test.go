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

func TestTranspileCondition_LegacyCustomOperatorUsesPredicateContext(t *testing.T) {
	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			err = tr.RegisterOperatorFunc("isGreaterLegacy", func(_ string, args []any) (string, error) {
				if len(args) != 2 {
					return "", fmt.Errorf("isGreaterLegacy requires exactly 2 arguments")
				}
				return fmt.Sprintf("%s>%s", args[0], args[1]), nil
			})
			if err != nil {
				t.Fatalf("RegisterOperatorFunc() error = %v", err)
			}

			logic := `{"isGreaterLegacy":[{"var":"amount"},10]}`
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

func TestTranspileCondition_ComparisonOperandsUseValueSemantics(t *testing.T) {
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
			wantSQL: "'x' = 'x'",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("%s = %s", testPlaceholder(d, 1), testPlaceholder(d, 2))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "x"}, {Name: "p2", Value: "x"}},
		},
		{
			name:    "unreachable comparison operand branch is not parsed",
			logic:   `{"<":[{"if":[false,{"var":"bad-name"},3]},4]}`,
			wantSQL: "3 < 4",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("%s < %s", testPlaceholder(d, 1), testPlaceholder(d, 2))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(3)}, {Name: "p2", Value: float64(4)}},
		},
		{
			name:    "value logical fallback folds before comparison",
			logic:   `{"==":[{"or":[0,5]},5]}`,
			wantSQL: "5 = 5",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("%s = %s", testPlaceholder(d, 1), testPlaceholder(d, 2))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(5)}, {Name: "p2", Value: float64(5)}},
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
