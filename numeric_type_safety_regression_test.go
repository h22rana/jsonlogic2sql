package jsonlogic2sql

import (
	"fmt"
	"strings"
	"testing"
)

func TestNumericOperatorsRejectKnownNonNumericValueExpressionsAllDialects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		logic     string
		condition bool
	}{
		{
			name:  "array map in addition",
			logic: `{"+":[{"map":[[1,2],{"var":""}]},1]}`,
		},
		{
			name:  "array merge in addition",
			logic: `{"+":[{"merge":[[1],[2]]},1]}`,
		},
		{
			name:  "string cat in addition",
			logic: `{"+":[{"cat":["5"]},1]}`,
		},
		{
			name:      "array map in comparison arithmetic",
			logic:     `{">":[{"+":[{"map":[[1,2],{"var":""}]},1]},0]}`,
			condition: true,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, emptyTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					var sql string
					var inlineErr error
					if tc.condition {
						sql, inlineErr = tr.TranspileCondition(tc.logic)
					} else {
						sql, inlineErr = tr.TranspileValue(tc.logic)
					}
					if inlineErr == nil {
						t.Fatalf("inline SQL = %q, want numeric type error", sql)
					}
					if !strings.Contains(inlineErr.Error(), "numeric operation on incompatible") {
						t.Fatalf("inline error = %v, want numeric type error", inlineErr)
					}

					var params []QueryParam
					var paramErr error
					if tc.condition {
						sql, params, paramErr = tr.TranspileParameterizedCondition(tc.logic)
					} else {
						sql, params, paramErr = tr.TranspileParameterizedValue(tc.logic)
					}
					if paramErr == nil {
						t.Fatalf("parameterized SQL = %q params = %#v, want numeric type error", sql, params)
					}
					if !strings.Contains(paramErr.Error(), "numeric operation on incompatible") {
						t.Fatalf("parameterized error = %v, want numeric type error", paramErr)
					}
				})
			}
		})
	}
}

func TestNumericOperatorsValidateCustomValueResultTypesAllDialects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		op   string
		typ  ExpressionType
	}{
		{name: "custom string value", op: "string_value", typ: ExpressionTypeString},
		{name: "custom array value", op: "array_value", typ: ExpressionTypeArray},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, emptyTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			for _, tc := range cases {
				op := tc.op
				typ := tc.typ
				if regErr := tr.RegisterOperatorFunc(op, func(_ string, args []OperatorArg) (OperatorResult, error) {
					if len(args) != 1 {
						return OperatorResult{}, fmt.Errorf("%s requires one argument", op)
					}
					if typ == ExpressionTypeArray {
						return ArrayValueSQL(args[0].SQL, ExpressionTypeNumber), nil
					}
					return ValueSQL(args[0].SQL, typ), nil
				}); regErr != nil {
					t.Fatalf("RegisterOperatorFunc(%s) error = %v", op, regErr)
				}
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					logic := fmt.Sprintf(`{"+":[{"%s":[1]},1]}`, tc.op)
					sql, err := tr.TranspileValue(logic)
					if err == nil {
						t.Fatalf("inline SQL = %q, want numeric type error", sql)
					}
					if !strings.Contains(err.Error(), "numeric operation on incompatible") {
						t.Fatalf("inline error = %v, want numeric type error", err)
					}

					sql, params, err := tr.TranspileParameterizedValue(logic)
					if err == nil {
						t.Fatalf("parameterized SQL = %q params = %#v, want numeric type error", sql, params)
					}
					if !strings.Contains(err.Error(), "numeric operation on incompatible") {
						t.Fatalf("parameterized error = %v, want numeric type error", err)
					}
				})
			}
		})
	}
}

func TestNumericOperatorsKeepBooleanValueCoercionAllDialects(t *testing.T) {
	t.Parallel()

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, emptyTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			if _, err := tr.TranspileValue(`{"+":[{"some":[[1],true]},1]}`); err != nil {
				t.Fatalf("TranspileValue() boolean predicate arithmetic error = %v", err)
			}
			if _, _, err := tr.TranspileParameterizedValue(`{"+":[{"some":[[1],true]},1]}`); err != nil {
				t.Fatalf("TranspileParameterizedValue() boolean predicate arithmetic error = %v", err)
			}
		})
	}
}
