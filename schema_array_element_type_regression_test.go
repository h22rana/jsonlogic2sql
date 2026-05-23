package jsonlogic2sql

import (
	"strings"
	"testing"
)

func arrayElementTypeRegressionSchema() *Schema {
	return mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "numbers", Type: FieldTypeArray, ElementType: FieldTypeNumber},
		{Name: "tags", Type: FieldTypeArray, ElementType: FieldTypeString},
		{Name: "states", Type: FieldTypeArray, ElementType: FieldTypeEnum, AllowedValues: []string{"active", "pending"}},
		{
			Name: "leftItems",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "labels", Type: FieldTypeArray, ElementType: FieldTypeString},
				{Name: "scores", Type: FieldTypeArray, ElementType: FieldTypeNumber},
				{Name: "statuses", Type: FieldTypeArray, ElementType: FieldTypeEnum, AllowedValues: []string{"active", "pending"}},
			},
		},
		{
			Name: "rightItems",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "labels", Type: FieldTypeArray, ElementType: FieldTypeNumber},
			},
		},
	})
}

func TestSchemaArrayElementTypesRejectInvalidFieldEqualityAllDialects(t *testing.T) {
	t.Parallel()

	schema := arrayElementTypeRegressionSchema()
	tests := []struct {
		name  string
		logic string
	}{
		{name: "loose equality", logic: `{"==":[{"var":"numbers"},{"var":"tags"}]}`},
		{name: "strict equality", logic: `{"===":[{"var":"numbers"},{"var":"tags"}]}`},
		{name: "loose inequality", logic: `{"!=":[{"var":"numbers"},{"var":"tags"}]}`},
		{name: "strict inequality", logic: `{"!==":[{"var":"numbers"},{"var":"tags"}]}`},
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

					if sql, err := tr.TranspileCondition(tt.logic); err == nil ||
						!strings.Contains(err.Error(), "incompatible array element types") {
						t.Fatalf("TranspileCondition() = %q, error = %v; want incompatible array element type error", sql, err)
					}

					if sql, _, err := tr.TranspileParameterizedCondition(tt.logic); err == nil ||
						!strings.Contains(err.Error(), "incompatible array element types") {
						t.Fatalf("TranspileParameterizedCondition() = %q, error = %v; want incompatible array element type error", sql, err)
					}
				})
			}
		})
	}
}

func TestSchemaArrayElementTypesAllowCompatibleFieldEqualityAllDialects(t *testing.T) {
	t.Parallel()

	schema := arrayElementTypeRegressionSchema()
	logic := `{"==":[{"var":"numbers"},{"var":"numbers"}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			sql, err := tr.TranspileCondition(logic)
			if d == DialectBigQuery {
				if err == nil || !strings.Contains(err.Error(), "equality between array fields") {
					t.Fatalf("BigQuery TranspileCondition() = %q, error = %v; want array equality rejection", sql, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}

			paramSQL, _, err := tr.TranspileParameterizedCondition(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if paramSQL != sql {
				t.Fatalf("TranspileParameterizedCondition() = %q, want %q", paramSQL, sql)
			}
		})
	}
}

func TestSchemaArrayDefaultsValidateElementTypesAllDialects(t *testing.T) {
	t.Parallel()

	schema := arrayElementTypeRegressionSchema()
	tests := []struct {
		name      string
		logic     string
		wantError string
	}{
		{
			name:      "number array rejects string default element",
			logic:     `{"var":["numbers",["x"]]}`,
			wantError: "default value for array field 'numbers' element 0 has incompatible type string; expected number or null",
		},
		{
			name:      "enum array rejects invalid default element",
			logic:     `{"var":["states",["archived"]]}`,
			wantError: "invalid enum value 'archived' for field 'states'",
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

					if sql, err := tr.TranspileValue(tt.logic); err == nil || !strings.Contains(err.Error(), tt.wantError) {
						t.Fatalf("TranspileValue() = %q, error = %v; want %q", sql, err, tt.wantError)
					}

					if sql, _, err := tr.TranspileParameterizedValue(tt.logic); err == nil || !strings.Contains(err.Error(), tt.wantError) {
						t.Fatalf("TranspileParameterizedValue() = %q, error = %v; want %q", sql, err, tt.wantError)
					}
				})
			}
		})
	}
}

func TestSchemaArrayDefaultsValidateScopedArrayFieldElementsAllDialects(t *testing.T) {
	t.Parallel()

	schema := arrayElementTypeRegressionSchema()
	tests := []struct {
		name      string
		logic     string
		wantError string
	}{
		{
			name:      "scoped number array rejects string default element",
			logic:     `{"map":[{"var":"leftItems"},{"var":["scores",["x"]]}]}`,
			wantError: "default value for array field 'leftItems.scores' element 0 has incompatible type string; expected number or null",
		},
		{
			name:      "scoped enum array rejects invalid default element",
			logic:     `{"map":[{"var":"leftItems"},{"var":["statuses",["archived"]]}]}`,
			wantError: "invalid enum value 'archived' for field 'leftItems.statuses'",
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

					if sql, err := tr.TranspileValue(tt.logic); err == nil || !strings.Contains(err.Error(), tt.wantError) {
						t.Fatalf("TranspileValue() = %q, error = %v; want %q", sql, err, tt.wantError)
					}

					if sql, _, err := tr.TranspileParameterizedValue(tt.logic); err == nil || !strings.Contains(err.Error(), tt.wantError) {
						t.Fatalf("TranspileParameterizedValue() = %q, error = %v; want %q", sql, err, tt.wantError)
					}
				})
			}
		})
	}
}

func TestSchemaArrayElementTypesRejectIncompatibleDynamicNestedArraysAllDialects(t *testing.T) {
	t.Parallel()

	logic := `{"some":[{"if":[{"var":"flag"},{"var":"leftItems"},{"var":"rightItems"}]},{"some":[{"var":"labels"},{"==":[{"var":""},"active"]}]}]}`
	schema := arrayElementTypeRegressionSchema()

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			if sql, err := tr.TranspileCondition(logic); err == nil ||
				!strings.Contains(err.Error(), "field 'labels' has incompatible array element types across array source scopes") {
				t.Fatalf("TranspileCondition() = %q, error = %v; want nested array element type compatibility error", sql, err)
			}

			if sql, _, err := tr.TranspileParameterizedCondition(logic); err == nil ||
				!strings.Contains(err.Error(), "field 'labels' has incompatible array element types across array source scopes") {
				t.Fatalf("TranspileParameterizedCondition() = %q, error = %v; want nested array element type compatibility error", sql, err)
			}
		})
	}
}

func TestSchemaEnumArrayMembershipValidatesLiteralNeedlesAllDialects(t *testing.T) {
	t.Parallel()

	schema := arrayElementTypeRegressionSchema()
	tests := []struct {
		name      string
		logic     string
		wantError string
	}{
		{
			name:  "valid literal needle",
			logic: `{"in":["active",{"var":"states"}]}`,
		},
		{
			name:  "incompatible numeric needle folds false",
			logic: `{"in":[1,{"var":"states"}]}`,
		},
		{
			name:      "invalid literal needle",
			logic:     `{"in":["archived",{"var":"states"}]}`,
			wantError: "invalid enum value 'archived' for field 'states'",
		},
		{
			name:      "invalid processed literal needle",
			logic:     `{"in":[{"if":[true,"archived","active"]},{"var":"states"}]}`,
			wantError: "invalid enum value 'archived' for field 'states'",
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

					sql, err := tr.TranspileCondition(tt.logic)
					if tt.wantError == "" {
						if err != nil {
							t.Fatalf("TranspileCondition() error = %v", err)
						}
						if tt.name == "incompatible numeric needle folds false" && sql != "FALSE" {
							t.Fatalf("TranspileCondition() = %q, want FALSE", sql)
						}
					} else if err == nil || !strings.Contains(err.Error(), tt.wantError) {
						t.Fatalf("TranspileCondition() = %q, error = %v; want %q", sql, err, tt.wantError)
					}

					paramSQL, _, err := tr.TranspileParameterizedCondition(tt.logic)
					if tt.wantError == "" {
						if err != nil {
							t.Fatalf("TranspileParameterizedCondition() error = %v", err)
						}
						if tt.name == "incompatible numeric needle folds false" && paramSQL != "FALSE" {
							t.Fatalf("TranspileParameterizedCondition() = %q, want FALSE", paramSQL)
						}
					} else if err == nil || !strings.Contains(err.Error(), tt.wantError) {
						t.Fatalf("TranspileParameterizedCondition() = %q, error = %v; want %q", paramSQL, err, tt.wantError)
					}
				})
			}
		})
	}
}
