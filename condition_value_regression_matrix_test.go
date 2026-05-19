package jsonlogic2sql

import (
	"fmt"
	"reflect"
	"testing"
)

func conditionValueRegressionSchema() *Schema {
	return mustNewSchema([]FieldSchema{
		{Name: "a", Type: FieldTypeNumber},
		{Name: "b", Type: FieldTypeNumber},
		{Name: "c", Type: FieldTypeNumber},
		{Name: "amount", Type: FieldTypeNumber},
		{Name: "arr", Type: FieldTypeArray},
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "x", Type: FieldTypeNumber},
	})
}

func identityArrayMapSQLForDialect(d Dialect, array string) string {
	if d == DialectClickHouse {
		return fmt.Sprintf("arrayMap(elem -> elem, %s)", array)
	}
	return fmt.Sprintf("ARRAY(SELECT elem FROM UNNEST(%s) AS elem)", array)
}

func regressionParamsEqual(got, want []QueryParam) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	return reflect.DeepEqual(got, want)
}

func TestRegressionMatrix_ConditionValue_AllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := conditionValueRegressionSchema()

	valueCases := []struct {
		name       string
		logic      string
		wantSQL    func(Dialect) string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "if value condition uses typed truthiness",
			logic: `{"if":[{"var":"flag"},"yes","no"]}`,
			wantSQL: func(_ Dialect) string {
				condition := "flag IS TRUE"
				return fmt.Sprintf("CASE WHEN %s THEN 'yes' ELSE 'no' END", condition)
			},
			wantParam: func(d Dialect) string {
				condition := "flag IS TRUE"
				return fmt.Sprintf("CASE WHEN %s THEN %s ELSE %s END", condition, testPlaceholder(d, 1), testPlaceholder(d, 2))
			},
			wantParams: []QueryParam{{Name: "p1", Value: "yes"}, {Name: "p2", Value: "no"}},
		},
		{
			name:  "nested value logical under numeric operator",
			logic: `{"+":[{"*":[{"or":[0,5]},2]},1]}`,
			wantSQL: func(Dialect) string {
				return "((5 * 2) + 1)"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("((%s * %s) + %s)", testPlaceholder(d, 1), testPlaceholder(d, 2), testPlaceholder(d, 3))
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(5)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: float64(1)},
			},
		},
		{
			name:  "array map transformation uses value logical semantics",
			logic: `{"map":[{"var":"arr"},{"or":[0,{"var":""}]}]}`,
			wantSQL: func(d Dialect) string {
				return identityArrayMapSQLForDialect(d, "arr")
			},
			wantParam: func(d Dialect) string {
				return identityArrayMapSQLForDialect(d, "arr")
			},
		},
		{
			name:  "array literal source evaluates value expression elements",
			logic: `{"map":[[{"var":"amount"},5],{"var":""}]}`,
			wantSQL: func(d Dialect) string {
				array := "[amount, 5]"
				if d == DialectPostgreSQL {
					array = "ARRAY[amount, 5]"
				}
				return identityArrayMapSQLForDialect(d, array)
			},
			wantParam: func(d Dialect) string {
				array := fmt.Sprintf("[amount, %s]", testPlaceholder(d, 1))
				if d == DialectPostgreSQL {
					array = fmt.Sprintf("ARRAY[amount, %s]", testPlaceholder(d, 1))
				}
				return identityArrayMapSQLForDialect(d, array)
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(5)}},
		},
		{
			name:  "custom predicate stringifies in value context",
			logic: `{"cat":[{"isPositive":[{"var":"amount"}]}]}`,
			wantSQL: func(Dialect) string {
				return "CONCAT(CASE WHEN amount > 0 THEN 'true' ELSE 'false' END)"
			},
			wantParam: func(Dialect) string {
				return "CONCAT(CASE WHEN amount > 0 THEN 'true' ELSE 'false' END)"
			},
		},
	}

	conditionCases := []struct {
		name       string
		logic      string
		wantSQL    func(Dialect) string
		wantParam  func(Dialect) string
		wantParams []QueryParam
	}{
		{
			name:  "predicate if skips unreachable branch",
			logic: `{"if":[true,{">":[{"var":"x"},1]},{">":[{"var":"missing"},1]}]}`,
			wantSQL: func(Dialect) string {
				return "x > 1"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("x > %s", testPlaceholder(d, 1))
			},
			wantParams: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
		{
			name:  "decisive boolean constant short-circuits predicate",
			logic: `{"and":[false,{">":[{"var":"missing"},1]}]}`,
			wantSQL: func(Dialect) string {
				return "FALSE"
			},
			wantParam: func(Dialect) string {
				return "FALSE"
			},
		},
		{
			name:  "comparison operands use value semantics",
			logic: `{"==":[{"if":["nonempty","x","y"]},"x"]}`,
			wantSQL: func(Dialect) string {
				return "TRUE"
			},
			wantParam: func(Dialect) string {
				return "TRUE"
			},
		},
		{
			name:  "parenthesized not preserves nested precedence",
			logic: `{"!":{"or":[{"==":[{"var":"a"},1]},{"and":[{"==":[{"var":"b"},2]},{"==":[{"var":"c"},3]}]}]}}`,
			wantSQL: func(Dialect) string {
				return "NOT (a = 1 OR (b = 2 AND c = 3))"
			},
			wantParam: func(d Dialect) string {
				return fmt.Sprintf(
					"NOT (a = %s OR (b = %s AND c = %s))",
					testPlaceholder(d, 1),
					testPlaceholder(d, 2),
					testPlaceholder(d, 3),
				)
			},
			wantParams: []QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: float64(3)},
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
			if err := tr.RegisterOperatorFunc("isPositive", func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 1 {
					return OperatorResult{}, fmt.Errorf("isPositive requires exactly 1 argument")
				}
				return PredicateSQL(fmt.Sprintf("%s > 0", args[0].SQL)), nil
			}); err != nil {
				t.Fatalf("RegisterOperatorFunc() error = %v", err)
			}

			for _, tc := range valueCases {
				t.Run("value/"+tc.name, func(t *testing.T) {
					gotSQL, err := tr.TranspileValue(tc.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if want := tc.wantSQL(d); gotSQL != want {
						t.Fatalf("TranspileValue() = %q, want %q", gotSQL, want)
					}

					gotParamSQL, gotParams, err := tr.TranspileParameterizedValue(tc.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if want := tc.wantParam(d); gotParamSQL != want {
						t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParamSQL, want)
					}
					if !regressionParamsEqual(gotParams, tc.wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, tc.wantParams)
					}
				})
			}

			for _, tc := range conditionCases {
				t.Run("condition/"+tc.name, func(t *testing.T) {
					gotSQL, err := tr.TranspileCondition(tc.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if want := tc.wantSQL(d); gotSQL != want {
						t.Fatalf("TranspileCondition() = %q, want %q", gotSQL, want)
					}

					gotParamSQL, gotParams, err := tr.TranspileParameterizedCondition(tc.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if want := tc.wantParam(d); gotParamSQL != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParamSQL, want)
					}
					if !regressionParamsEqual(gotParams, tc.wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, tc.wantParams)
					}
				})
			}
		})
	}
}
