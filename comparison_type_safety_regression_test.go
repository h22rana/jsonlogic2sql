package jsonlogic2sql

import (
	"strings"
	"testing"
)

func comparisonTypeSafetySchema() *Schema {
	return mustNewSchema([]FieldSchema{
		{Name: "n", Type: FieldTypeNumber},
		{Name: "s", Type: FieldTypeString},
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "nums", Type: FieldTypeArray},
		{Name: "other_nums", Type: FieldTypeArray},
	})
}

func TestOrderingComparisonsRejectKnownIncompatibleOperandsAllDialects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		logic string
	}{
		{
			name:  "number field versus string field",
			logic: `{">":[{"var":"n"},{"var":"s"}]}`,
		},
		{
			name:  "string value expression versus number literal",
			logic: `{">":[{"cat":["a","b"]},1]}`,
		},
		{
			name:  "number value expression versus string value expression",
			logic: `{">":[{"+":[1,2]},{"cat":["x"]}]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, comparisonTypeSafetySchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					sql, err := tr.TranspileCondition(tc.logic)
					if err == nil {
						t.Fatalf("TranspileCondition() SQL = %q, want ordering type error", sql)
					}
					if !strings.Contains(err.Error(), "ordering comparison") {
						t.Fatalf("TranspileCondition() error = %v, want ordering type error", err)
					}

					sql, params, err := tr.TranspileParameterizedCondition(tc.logic)
					if err == nil {
						t.Fatalf("TranspileParameterizedCondition() SQL = %q params = %#v, want ordering type error", sql, params)
					}
					if !strings.Contains(err.Error(), "ordering comparison") {
						t.Fatalf("TranspileParameterizedCondition() error = %v, want ordering type error", err)
					}
				})
			}
		})
	}
}

func TestOrderingComparisonsCoerceBooleanLiteralsAllDialects(t *testing.T) {
	t.Parallel()

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, comparisonTypeSafetySchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			if _, err := tr.TranspileCondition(`{">":[true,0]}`); err != nil {
				t.Fatalf("TranspileCondition() boolean literal ordering error = %v", err)
			}
			if _, _, err := tr.TranspileParameterizedCondition(`{">":[true,0]}`); err != nil {
				t.Fatalf("TranspileParameterizedCondition() boolean literal ordering error = %v", err)
			}
		})
	}
}

func TestBigQueryRejectsArrayFieldEqualityAllModes(t *testing.T) {
	t.Parallel()

	tr, err := NewTranspiler(DialectBigQuery, comparisonTypeSafetySchema())
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	for _, op := range []string{"==", "===", "!=", "!=="} {
		t.Run(op, func(t *testing.T) {
			t.Parallel()

			logic := `{"` + op + `":[{"var":"nums"},{"var":"other_nums"}]}`

			sql, err := tr.TranspileCondition(logic)
			if err == nil {
				t.Fatalf("TranspileCondition() SQL = %q, want BigQuery array equality error", sql)
			}
			if !strings.Contains(err.Error(), "equality between array fields") {
				t.Fatalf("TranspileCondition() error = %v, want array equality error", err)
			}

			sql, params, err := tr.TranspileParameterizedCondition(logic)
			if err == nil {
				t.Fatalf("TranspileParameterizedCondition() SQL = %q params = %#v, want BigQuery array equality error", sql, params)
			}
			if !strings.Contains(err.Error(), "equality between array fields") {
				t.Fatalf("TranspileParameterizedCondition() error = %v, want array equality error", err)
			}
		})
	}
}
