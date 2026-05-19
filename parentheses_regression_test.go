package jsonlogic2sql

import (
	"reflect"
	"testing"
)

func TestTranspile_ParenthesesNormalizationTrickyCases(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery, defaultTestSchema())
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	tests := []struct {
		name  string
		logic string
		value bool
		want  string
	}{
		{
			name:  "concat comparison with quoted closing parenthesis",
			logic: `{"cat":[{"==":[{"var":"amount"},")"]}]}`,
			value: true,
			want:  "CONCAT('false')",
		},
		{
			name:  "not preserves nested and precedence inside or",
			logic: `{"!":{"or":[{"==":[{"var":"a"},1]},{"and":[{"==":[{"var":"b"},2]},{"==":[{"var":"c"},3]}]}]}}`,
			want:  "NOT (a = 1 OR (b = 2 AND c = 3))",
		},
		{
			name:  "concat if keeps one grouped or condition",
			logic: `{"cat":[{"if":[{"or":[{"==":[{"var":"type"},"A"]},{"==":[{"var":"type"},"B"]}]},"yes","no"]}]}`,
			value: true,
			want:  "CONCAT(COALESCE(CASE WHEN (type = 'A' OR type = 'B') THEN 'yes' ELSE 'no' END, ''))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			var err error
			if tt.value {
				got, err = tr.TranspileValue(tt.logic)
			} else {
				got, err = tr.TranspileCondition(tt.logic)
			}
			if err != nil {
				t.Fatalf("transpile error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("SQL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTranspileParameterized_ParenthesesNormalizationTrickyCases(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery, defaultTestSchema())
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}

	gotSQL, gotParams, err := tr.TranspileParameterizedValue(`{"cat":[{"==":[{"var":"amount"},")"]}]}`)
	if err != nil {
		t.Fatalf("TranspileParameterizedValue() error = %v", err)
	}
	if wantSQL := "CONCAT('false')"; gotSQL != wantSQL {
		t.Fatalf("TranspileParameterizedValue() SQL = %q, want %q", gotSQL, wantSQL)
	}
	if wantParams := []QueryParam(nil); !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
	}

	gotSQL, gotParams, err = tr.TranspileParameterizedCondition(
		`{"!":{"or":[{"==":[{"var":"a"},1]},{"and":[{"==":[{"var":"b"},2]},{"==":[{"var":"c"},3]}]}]}}`,
	)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error = %v", err)
	}
	if wantSQL := "NOT (a = @p1 OR (b = @p2 AND c = @p3))"; gotSQL != wantSQL {
		t.Fatalf("TranspileParameterizedCondition() SQL = %q, want %q", gotSQL, wantSQL)
	}
	wantParams := []QueryParam{
		{Name: "p1", Value: float64(1)},
		{Name: "p2", Value: float64(2)},
		{Name: "p3", Value: float64(3)},
	}
	if !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("params = %#v, want %#v", gotParams, wantParams)
	}
}

func TestTranspileValue_CatComparisonStringifiesSchemaRequiredBoolean(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeNumber},
	})
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  schema,
	})
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() error = %v", err)
	}

	logic := `{"cat":[{"==":[{"var":"amount"},")"]}]}`
	gotSQL, err := tr.TranspileValue(logic)
	if err != nil {
		t.Fatalf("TranspileValue() error = %v", err)
	}
	if wantSQL := "CONCAT('false')"; gotSQL != wantSQL {
		t.Fatalf("TranspileValue() SQL = %q, want %q", gotSQL, wantSQL)
	}

	gotParamSQL, gotParams, err := tr.TranspileParameterizedValue(logic)
	if err != nil {
		t.Fatalf("TranspileParameterizedValue() error = %v", err)
	}
	if wantSQL := "CONCAT('false')"; gotParamSQL != wantSQL {
		t.Fatalf("TranspileParameterizedValue() SQL = %q, want %q", gotParamSQL, wantSQL)
	}
	if len(gotParams) != 0 {
		t.Fatalf("params = %#v, want none", gotParams)
	}
}
