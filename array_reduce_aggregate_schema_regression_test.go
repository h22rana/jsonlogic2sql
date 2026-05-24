package jsonlogic2sql

import (
	"strings"
	"testing"
)

func reduceAggregateSchema(t *testing.T) *Schema {
	t.Helper()
	return mustNewSchema([]FieldSchema{
		{
			Name: "items",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "price", Type: FieldTypeNumber},
				{Name: "name", Type: FieldTypeString},
				{Name: "active", Type: FieldTypeBoolean},
				{Name: "tags", Type: FieldTypeArray, ElementType: FieldTypeString},
				{
					Name: "profile",
					Type: FieldTypeObject,
					Fields: []FieldSchema{
						{Name: "tier", Type: FieldTypeString},
					},
				},
			},
		},
		{Name: "numbers", Type: FieldTypeArray, ElementType: FieldTypeNumber},
		{Name: "names", Type: FieldTypeArray, ElementType: FieldTypeString},
	})
}

func TestReduceAggregateRejectsNonNumericSchemaElementValuesAllDialects(t *testing.T) {
	tests := []struct {
		name  string
		logic string
	}{
		{
			name:  "object element current",
			logic: `{"reduce":[{"var":"items"},{"+":[{"var":"accumulator"},{"var":"current"}]},0]}`,
		},
		{
			name:  "string element field",
			logic: `{"reduce":[{"var":"items"},{"+":[{"var":"accumulator"},{"var":"current.name"}]},0]}`,
		},
		{
			name:  "boolean element field",
			logic: `{"reduce":[{"var":"items"},{"max":[{"var":"accumulator"},{"var":"current.active"}]},0]}`,
		},
		{
			name:  "array element field",
			logic: `{"reduce":[{"var":"items"},{"+":[{"var":"accumulator"},{"var":"current.tags"}]},0]}`,
		},
		{
			name:  "string scalar array current",
			logic: `{"reduce":[{"var":"names"},{"+":[{"var":"accumulator"},{"var":"current"}]},0]}`,
		},
		{
			name:  "object element field",
			logic: `{"reduce":[{"var":"items"},{"+":[{"var":"accumulator"},{"var":"current.profile"}]},0]}`,
		},
		{
			name:  "non-numeric default for numeric field",
			logic: `{"reduce":[{"var":"items"},{"+":[{"var":"accumulator"},{"var":["current.price","missing"]}]},0]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d, reduceAggregateSchema(t))
			if err != nil {
				t.Fatalf("NewTranspiler: %v", err)
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if got, err := tr.TranspileValue(tt.logic); err == nil {
						t.Fatalf("TranspileValue() = %q, want numeric aggregate error", got)
					} else if !strings.Contains(err.Error(), "numeric reduce aggregate") {
						t.Fatalf("TranspileValue() error = %v, want numeric aggregate error", err)
					}

					if got, params, err := tr.TranspileParameterizedValue(tt.logic); err == nil {
						t.Fatalf("TranspileParameterizedValue() = %q params %#v, want numeric aggregate error", got, params)
					} else if !strings.Contains(err.Error(), "numeric reduce aggregate") {
						t.Fatalf("TranspileParameterizedValue() error = %v, want numeric aggregate error", err)
					}
				})
			}
		})
	}
}

func TestReduceAggregateAllowsNumericSchemaElementFieldsAllDialects(t *testing.T) {
	logic := `{"reduce":[{"var":"items"},{"+":[{"var":"accumulator"},{"var":["current.price",5]}]},0]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d, reduceAggregateSchema(t))
			if err != nil {
				t.Fatalf("NewTranspiler: %v", err)
			}

			sql, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if !strings.Contains(sql, "price") {
				t.Fatalf("TranspileValue() = %q, want price aggregate", sql)
			}

			paramSQL, params, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			if !strings.Contains(paramSQL, "price") {
				t.Fatalf("TranspileParameterizedValue() = %q, want price aggregate", paramSQL)
			}
			if len(params) != 2 {
				t.Fatalf("params = %#v, want initial and default parameters", params)
			}
		})
	}
}

func TestReduceAggregateParameterizedNumericStringInitialsUseNumericParamsAllDialects(t *testing.T) {
	tests := []struct {
		name              string
		initial           string
		wantParamValue    interface{}
		wantClickHouseSQL string
	}{
		{
			name:              "integer string",
			initial:           "1",
			wantParamValue:    int64(1),
			wantClickHouseSQL: "{p1:Int64}",
		},
		{
			name:              "negative integer string",
			initial:           "-2",
			wantParamValue:    int64(-2),
			wantClickHouseSQL: "{p1:Int64}",
		},
		{
			name:              "float string",
			initial:           "1.5",
			wantParamValue:    1.5,
			wantClickHouseSQL: "{p1:Float64}",
		},
		{
			name:              "exact unsigned integer string",
			initial:           "9223372036854775808",
			wantParamValue:    "9223372036854775808",
			wantClickHouseSQL: "{p1:UInt64}",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d, reduceAggregateSchema(t))
			if err != nil {
				t.Fatalf("NewTranspiler: %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					logic := `{"reduce":[{"var":"numbers"},{"+":[{"var":"accumulator"},{"var":"current"}]},"` +
						tt.initial + `"]}`

					sql, params, err := tr.TranspileParameterizedValue(logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if len(params) == 0 {
						t.Fatalf("params = %#v, want numeric initial parameter", params)
					}
					if params[0].Value != tt.wantParamValue {
						t.Fatalf("initial param value = %#v, want %#v", params[0].Value, tt.wantParamValue)
					}
					if strings.Contains(sql, "CAST(") && strings.Contains(sql, ":String") {
						t.Fatalf("SQL = %q, want numeric initial placeholder without string cast", sql)
					}
					if d == DialectClickHouse {
						if !strings.Contains(sql, tt.wantClickHouseSQL) {
							t.Fatalf("ClickHouse SQL = %q, want %s", sql, tt.wantClickHouseSQL)
						}
						if strings.Contains(sql, "{p1:String}") {
							t.Fatalf("ClickHouse SQL = %q, want numeric p1 placeholder", sql)
						}
					}
				})
			}
		})
	}
}

func TestReduceParameterizedEmptyArrayKeepsNumericStringInitialAsStringAllDialects(t *testing.T) {
	logic := `{"reduce":[[],{"+":[{"var":"accumulator"},{"var":"current"}]},"1"]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d, reduceAggregateSchema(t))
			if err != nil {
				t.Fatalf("NewTranspiler: %v", err)
			}

			sql, params, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			if len(params) != 1 || params[0].Value != "1" {
				t.Fatalf("params = %#v, want string initial parameter", params)
			}
			if d == DialectClickHouse && sql != "{p1:String}" {
				t.Fatalf("ClickHouse SQL = %q, want {p1:String}", sql)
			}
		})
	}
}
