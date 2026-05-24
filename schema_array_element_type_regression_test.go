package jsonlogic2sql

import (
	"strings"
	"testing"
)

func arrayElementTypeRegressionSchema() *Schema {
	return mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
		{Name: "age", Type: FieldTypeInteger},
		{Name: "score", Type: FieldTypeNumber},
		{Name: "label", Type: FieldTypeString},
		{Name: "ints", Type: FieldTypeArray, ElementType: FieldTypeInteger},
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

func TestSchemaArrayIntegerMergeNullUsesIntegerNullAllDialects(t *testing.T) {
	t.Parallel()

	schema := arrayElementTypeRegressionSchema()
	tests := []struct {
		name      string
		logic     string
		wantToken map[Dialect]string
	}{
		{
			name:  "prepend null to integer array field",
			logic: `{"merge":[null,{"var":"ints"}]}`,
			wantToken: map[Dialect]string{
				DialectBigQuery:   "CAST(NULL AS INT64)",
				DialectSpanner:    "CAST(NULL AS INT64)",
				DialectPostgreSQL: "CAST(NULL AS BIGINT)",
				DialectDuckDB:     "CAST(NULL AS BIGINT)",
				DialectClickHouse: "CAST(NULL AS Nullable(Int64))",
			},
		},
		{
			name:  "append null to filtered integer array",
			logic: `{"merge":[{"filter":[{"var":"ints"},{"var":""}]},null]}`,
			wantToken: map[Dialect]string{
				DialectBigQuery:   "CAST(NULL AS INT64)",
				DialectSpanner:    "CAST(NULL AS INT64)",
				DialectPostgreSQL: "CAST(NULL AS BIGINT)",
				DialectDuckDB:     "CAST(NULL AS BIGINT)",
				DialectClickHouse: "CAST(NULL AS Nullable(Int64))",
			},
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

					sql, err := tr.TranspileValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if !strings.Contains(sql, tt.wantToken[d]) {
						t.Fatalf("TranspileValue() = %q, want integer NULL token %q", sql, tt.wantToken[d])
					}
					if strings.Contains(sql, "FLOAT64") || strings.Contains(sql, "DOUBLE PRECISION") || strings.Contains(sql, "Nullable(Float64)") {
						t.Fatalf("TranspileValue() = %q, should not render floating-point NULL for integer array merge", sql)
					}

					paramSQL, params, err := tr.TranspileParameterizedValue(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if len(params) != 0 {
						t.Fatalf("TranspileParameterizedValue() params = %#v, want none", params)
					}
					if !strings.Contains(paramSQL, tt.wantToken[d]) {
						t.Fatalf("TranspileParameterizedValue() = %q, want integer NULL token %q", paramSQL, tt.wantToken[d])
					}
				})
			}
		})
	}
}

func TestSchemaArrayElementTypesRejectIntegerNumberMergeAllDialects(t *testing.T) {
	t.Parallel()

	schema := arrayElementTypeRegressionSchema()
	logic := `{"merge":[{"var":"ints"},{"var":"numbers"}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			if sql, err := tr.TranspileValue(logic); err == nil ||
				!strings.Contains(err.Error(), "incompatible array element schema types") {
				t.Fatalf("TranspileValue() = %q, error = %v; want integer/number schema type error", sql, err)
			}

			if sql, _, err := tr.TranspileParameterizedValue(logic); err == nil ||
				!strings.Contains(err.Error(), "incompatible array element schema types") {
				t.Fatalf("TranspileParameterizedValue() = %q, error = %v; want integer/number schema type error", sql, err)
			}
		})
	}
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
		{name: "integer number equality", logic: `{"==":[{"var":"ints"},{"var":"numbers"}]}`},
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
			name:      "integer array rejects fractional default element",
			logic:     `{"var":["ints",[1.5]]}`,
			wantError: "default value for integer array field 'ints' element 0 must be an integer or null",
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

func TestSchemaArrayElementTypesRejectIntegerNumberValueBranchesAllDialects(t *testing.T) {
	t.Parallel()

	schema := arrayElementTypeRegressionSchema()
	logic := `{"if":[{"var":"flag"},{"var":"ints"},{"var":"numbers"}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			if sql, err := tr.TranspileValue(logic); err == nil ||
				!strings.Contains(err.Error(), "compatible element schema types") {
				t.Fatalf("TranspileValue() = %q, error = %v; want integer/number branch type error", sql, err)
			}

			if sql, _, err := tr.TranspileParameterizedValue(logic); err == nil ||
				!strings.Contains(err.Error(), "compatible element schema types") {
				t.Fatalf("TranspileParameterizedValue() = %q, error = %v; want integer/number branch type error", sql, err)
			}
		})
	}
}

func TestSchemaArrayElementTypesAllowNullMembershipNeedlesAllDialects(t *testing.T) {
	t.Parallel()

	schema := arrayElementTypeRegressionSchema()
	logic := `{"in":[null,{"var":"ints"}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			sql, err := tr.TranspileCondition(logic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			if sql == "FALSE" || !strings.Contains(sql, "IS NULL") {
				t.Fatalf("TranspileCondition() = %q, want null-safe array membership", sql)
			}

			paramSQL, params, err := tr.TranspileParameterizedCondition(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			for _, param := range params {
				if param.Value == nil {
					continue
				}
				t.Fatalf("TranspileParameterizedCondition() params = %#v, want no non-nil params for literal null needle", params)
			}
			if paramSQL == "FALSE" || !strings.Contains(paramSQL, "IS NULL") {
				t.Fatalf("TranspileParameterizedCondition() = %q, want null-safe array membership", paramSQL)
			}
		})
	}
}

func TestSchemaArrayElementTypesAllowIntegerArrayNumericMembershipAllDialects(t *testing.T) {
	t.Parallel()

	schema := arrayElementTypeRegressionSchema()
	tests := []struct {
		name         string
		logic        string
		inlineNeedle string
		paramNeedle  func(Dialect) string
		wantParam    bool
	}{
		{
			name:         "integer literal",
			logic:        `{"in":[1,{"var":"ints"}]}`,
			inlineNeedle: "1",
			paramNeedle:  func(d Dialect) string { return testPlaceholder(d, 1) },
			wantParam:    true,
		},
		{
			name:         "integer field",
			logic:        `{"in":[{"var":"age"},{"var":"ints"}]}`,
			inlineNeedle: "age",
			paramNeedle:  func(Dialect) string { return "age" },
		},
		{
			name:         "number field",
			logic:        `{"in":[{"var":"score"},{"var":"ints"}]}`,
			inlineNeedle: "score",
			paramNeedle:  func(Dialect) string { return "score" },
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
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if want := testNullSafeArrayMembershipSQL(d, tt.inlineNeedle, "ints"); sql != want {
						t.Fatalf("TranspileCondition() = %q, want %q", sql, want)
					}

					paramSQL, params, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if want := testNullSafeArrayMembershipSQL(d, tt.paramNeedle(d), "ints"); paramSQL != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", paramSQL, want)
					}
					if tt.wantParam {
						if len(params) != 1 {
							t.Fatalf("TranspileParameterizedCondition() params = %#v, want one numeric param", params)
						}
						if value, ok := params[0].Value.(float64); !ok || value != 1 {
							t.Fatalf("TranspileParameterizedCondition() params = %#v, want float64(1)", params)
						}
					} else if len(params) != 0 {
						t.Fatalf("TranspileParameterizedCondition() params = %#v, want none", params)
					}
				})
			}
		})
	}
}

func TestSchemaArrayElementTypesRejectImpossibleIntegerArrayMembershipAllDialects(t *testing.T) {
	t.Parallel()

	schema := arrayElementTypeRegressionSchema()
	tests := []struct {
		name  string
		logic string
	}{
		{
			name:  "fractional literal",
			logic: `{"in":[1.5,{"var":"ints"}]}`,
		},
		{
			name:  "integer literal outside int64 range",
			logic: `{"in":[9223372036854775808,{"var":"ints"}]}`,
		},
		{
			name:  "string field with null default",
			logic: `{"in":[{"var":["label",null]},{"var":"ints"}]}`,
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
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if sql != "FALSE" {
						t.Fatalf("TranspileCondition() = %q, want FALSE", sql)
					}

					paramSQL, params, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if paramSQL != "FALSE" {
						t.Fatalf("TranspileParameterizedCondition() = %q, want FALSE", paramSQL)
					}
					if len(params) != 0 {
						t.Fatalf("TranspileParameterizedCondition() params = %#v, want none for folded FALSE", params)
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

func TestSchemaArrayElementTypesAllowEquivalentConciseAndExplicitObjectArraysAllDialects(t *testing.T) {
	t.Parallel()

	logic := `{"some":[{"if":[{"var":"flag"},{"var":"conciseItems"},{"var":"explicitItems"}]},{"some":[{"var":"kids"},{"==":[{"var":"name"},"target"]}]}]}`
	schema := mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
		{
			Name: "conciseItems",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{
					Name: "kids",
					Type: FieldTypeArray,
					ElementFields: []FieldSchema{
						{Name: "name", Type: FieldTypeString},
					},
				},
			},
		},
		{
			Name: "explicitItems",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{
					Name:        "kids",
					Type:        FieldTypeArray,
					ElementType: FieldTypeObject,
					ElementFields: []FieldSchema{
						{Name: "name", Type: FieldTypeString},
					},
				},
			},
		},
	})

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			sql, err := tr.TranspileCondition(logic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			if !strings.Contains(sql, "kids") || !strings.Contains(sql, "name") {
				t.Fatalf("TranspileCondition() = %q, want nested kids.name access", sql)
			}

			paramSQL, params, err := tr.TranspileParameterizedCondition(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if !strings.Contains(paramSQL, "kids") || !strings.Contains(paramSQL, "name") {
				t.Fatalf("TranspileParameterizedCondition() = %q, want nested kids.name access", paramSQL)
			}
			if len(params) != 1 || params[0].Name != "p1" || params[0].Value != "target" {
				t.Fatalf("TranspileParameterizedCondition() params = %#v, want p1=target", params)
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
