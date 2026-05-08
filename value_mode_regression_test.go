package jsonlogic2sql

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

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
			wantSQL:    "CASE WHEN TRUE THEN @p1 ELSE @p2 END",
			wantParams: []QueryParam{{Name: "p1", Value: "yes"}, {Name: "p2", Value: "no"}},
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
			name:    "speculative predicate parse rollback before value fallback",
			logic:   `{"if":[{"or":[{"==":[{"var":"status"},"active"]},true]},"yes","no"]}`,
			wantSQL: "CASE WHEN CASE WHEN status = @p1 THEN status = @p1 ELSE TRUE END IS TRUE THEN @p2 ELSE @p3 END",
			wantParams: []QueryParam{
				{Name: "p1", Value: "active"},
				{Name: "p2", Value: "yes"},
				{Name: "p3", Value: "no"},
			},
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

func testPlaceholder(d Dialect, index int) string {
	switch d {
	case DialectPostgreSQL, DialectDuckDB:
		return fmt.Sprintf("$%d", index)
	default:
		return fmt.Sprintf("@p%d", index)
	}
}
