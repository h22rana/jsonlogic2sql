package jsonlogic2sql

import (
	"fmt"
	"strings"
	"testing"
)

func typeMetadataRegressionSchema() *Schema {
	return mustNewSchema([]FieldSchema{
		{Name: "flag", Type: FieldTypeBoolean},
		{
			Name: "items",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "x", Type: FieldTypeNumber},
				{Name: "name", Type: FieldTypeString},
				{
					Name: "children",
					Type: FieldTypeArray,
					ElementFields: []FieldSchema{
						{Name: "y", Type: FieldTypeNumber},
					},
				},
			},
		},
		{
			Name: "items2",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "x", Type: FieldTypeNumber},
				{
					Name: "children",
					Type: FieldTypeArray,
					ElementFields: []FieldSchema{
						{Name: "label", Type: FieldTypeString},
					},
				},
			},
		},
	})
}

func expectValueAndParamErrorContains(t *testing.T, tr *Transpiler, logic, want string) {
	t.Helper()

	sql, err := tr.TranspileValue(logic)
	if err == nil {
		t.Fatalf("TranspileValue() SQL = %q, want error containing %q", sql, want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("TranspileValue() error = %v, want containing %q", err, want)
	}

	sql, params, err := tr.TranspileParameterizedValue(logic)
	if err == nil {
		t.Fatalf("TranspileParameterizedValue() SQL = %q params = %#v, want error containing %q", sql, params, want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("TranspileParameterizedValue() error = %v, want containing %q", err, want)
	}
}

func expectConditionAndParamErrorContains(t *testing.T, tr *Transpiler, logic, want string) {
	t.Helper()

	sql, err := tr.TranspileCondition(logic)
	if err == nil {
		t.Fatalf("TranspileCondition() SQL = %q, want error containing %q", sql, want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("TranspileCondition() error = %v, want containing %q", err, want)
	}

	sql, params, err := tr.TranspileParameterizedCondition(logic)
	if err == nil {
		t.Fatalf("TranspileParameterizedCondition() SQL = %q params = %#v, want error containing %q", sql, params, want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("TranspileParameterizedCondition() error = %v, want containing %q", err, want)
	}
}

func TestTypeMetadataRejectsObjectArrayScalarArrayBranchesAllDialects(t *testing.T) {
	t.Parallel()

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, typeMetadataRegressionSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			expectValueAndParamErrorContains(t, tr,
				`{"if":[{"var":"flag"},{"var":"items"},[1]]}`,
				"array value branches must have compatible element types",
			)
		})
	}
}

func TestTypeMetadataRejectsIncompatibleDynamicObjectArraySourcesAllDialects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		logic   string
		wantErr string
	}{
		{
			name:    "filter source branches",
			logic:   `{"filter":[{"if":[{"var":"flag"},{"var":"items"},{"var":"items2"}]},{">":[{"var":"x"},0]}]}`,
			wantErr: "compatible object element schemas",
		},
		{
			name:    "merge arguments",
			logic:   `{"merge":[{"var":"items"},{"var":"items2"}]}`,
			wantErr: "not defined in schema scope",
		},
		{
			name:    "value if branches",
			logic:   `{"if":[{"var":"flag"},{"var":"items"},{"var":"items2"}]}`,
			wantErr: "compatible object element schemas",
		},
		{
			name:    "value and branches",
			logic:   `{"and":[{"var":"items"},{"var":"items2"}]}`,
			wantErr: "compatible object element schemas",
		},
		{
			name:    "value or branches",
			logic:   `{"or":[{"var":"items"},{"var":"items2"}]}`,
			wantErr: "compatible object element schemas",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, typeMetadataRegressionSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					expectValueAndParamErrorContains(t, tr, tc.logic, tc.wantErr)
				})
			}
		})
	}
}

func TestTypeMetadataRejectsIncompatibleObjectArrayLiteralElements(t *testing.T) {
	t.Parallel()

	logic := `[{"var":"items"},{"var":"items2"}]`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			if testRejectsNestedArrayValues(d) {
				t.Skipf("%s rejects nested array values before object-element schema compatibility is relevant", d)
			}

			tr, err := NewTranspiler(d, typeMetadataRegressionSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			expectValueAndParamErrorContains(t, tr, logic, "compatible object element schemas")
		})
	}
}

func TestTypeMetadataRejectsArrayScalarLooseEqualityAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray, ElementType: FieldTypeNumber},
		{Name: "age", Type: FieldTypeNumber},
	})
	cases := []struct {
		name    string
		logic   string
		wantErr string
	}{
		{
			name:    "array field equals scalar",
			logic:   `{"==":[{"var":"arr"},1]}`,
			wantErr: "array equality",
		},
		{
			name:    "defaulted array field equals scalar",
			logic:   `{"==":[{"var":["arr",[1]]},1]}`,
			wantErr: "array equality",
		},
		{
			name:    "scalar not equals defaulted array field",
			logic:   `{"!=":[1,{"var":["arr",[1]]}]}`,
			wantErr: "array equality",
		},
		{
			name:    "defaulted array field equals scalar field",
			logic:   `{"==":[{"var":["arr",[1]]},{"var":"age"}]}`,
			wantErr: "array field",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					expectConditionAndParamErrorContains(t, tr, tc.logic, tc.wantErr)
					expectValueAndParamErrorContains(t, tr, tc.logic, tc.wantErr)
				})
			}
		})
	}
}

func TestTypeMetadataFoldsArrayScalarStrictEqualityAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "arr", Type: FieldTypeArray, ElementType: FieldTypeNumber},
		{Name: "age", Type: FieldTypeNumber},
	})
	cases := []struct {
		name  string
		logic string
		want  string
	}{
		{
			name:  "array field strict equals scalar",
			logic: `{"===":[{"var":"arr"},1]}`,
			want:  "FALSE",
		},
		{
			name:  "defaulted array field strict equals scalar",
			logic: `{"===":[{"var":["arr",[1]]},1]}`,
			want:  "FALSE",
		},
		{
			name:  "scalar strict not equals defaulted array field",
			logic: `{"!==":[1,{"var":["arr",[1]]}]}`,
			want:  "TRUE",
		},
		{
			name:  "defaulted array field strict equals scalar field",
			logic: `{"===":[{"var":["arr",[1]]},{"var":"age"}]}`,
			want:  "FALSE",
		},
		{
			name:  "scalar field strict not equals defaulted array field",
			logic: `{"!==":[{"var":"age"},{"var":["arr",[1]]}]}`,
			want:  "TRUE",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					sql, err := tr.TranspileCondition(tc.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if sql != tc.want {
						t.Fatalf("TranspileCondition() SQL = %q, want %q", sql, tc.want)
					}

					sql, params, err := tr.TranspileParameterizedCondition(tc.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if sql != tc.want {
						t.Fatalf("TranspileParameterizedCondition() SQL = %q, want %q", sql, tc.want)
					}
					if len(params) != 0 {
						t.Fatalf("TranspileParameterizedCondition() params = %#v, want none", params)
					}

					sql, err = tr.TranspileValue(tc.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if sql != tc.want {
						t.Fatalf("TranspileValue() SQL = %q, want %q", sql, tc.want)
					}

					sql, params, err = tr.TranspileParameterizedValue(tc.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if sql != tc.want {
						t.Fatalf("TranspileParameterizedValue() SQL = %q, want %q", sql, tc.want)
					}
					if len(params) != 0 {
						t.Fatalf("TranspileParameterizedValue() params = %#v, want none", params)
					}
				})
			}
		})
	}
}

func TestTypeMetadataSupportsArrayDefaultsAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "tags", Type: FieldTypeArray},
	})
	cases := []struct {
		name  string
		logic string
	}{
		{
			name:  "empty array var default",
			logic: `{"var":["tags",[]]}`,
		},
		{
			name:  "non-empty array var default",
			logic: `{"var":["tags",["x"]]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					_, inlineErr := tr.TranspileValue(tc.logic)
					_, _, paramErr := tr.TranspileParameterizedValue(tc.logic)
					if d == DialectPostgreSQL && tc.name == "empty array var default" {
						for mode, modeErr := range map[string]error{"inline": inlineErr, "parameterized": paramErr} {
							if modeErr == nil || !strings.Contains(modeErr.Error(), "empty PostgreSQL array literals require an explicit element type") {
								t.Fatalf("%s error = %v, want PostgreSQL empty-array type error", mode, modeErr)
							}
						}
						return
					}
					if inlineErr != nil {
						t.Fatalf("TranspileValue() error = %v", inlineErr)
					}
					if paramErr != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", paramErr)
					}
				})
			}

			_, err = tr.TranspileCondition(`{"in":["x",{"var":["tags",[]]}]}`)
			_, _, paramErr := tr.TranspileParameterizedCondition(`{"in":["x",{"var":["tags",[]]}]}`)
			if d == DialectPostgreSQL {
				for mode, err := range map[string]error{"inline": err, "parameterized": paramErr} {
					if err == nil || !strings.Contains(err.Error(), "empty PostgreSQL array literals require an explicit element type") {
						t.Fatalf("%s in-default error = %v, want PostgreSQL empty-array type error", mode, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("TranspileCondition(in default) error = %v", err)
			}
			if paramErr != nil {
				t.Fatalf("TranspileParameterizedCondition(in default) error = %v", paramErr)
			}
		})
	}
}

func TestTypeMetadataDefaultedFieldFieldEqualityAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeNumber},
		{Name: "score", Type: FieldTypeNumber},
	})
	cases := []string{
		`{"==":[{"var":["amount","missing"]},{"var":["score","missing"]}]}`,
		`{"!=":[{"var":["amount","missing"]},{"var":["score","missing"]}]}`,
		`{"===":[{"var":["amount","missing"]},{"var":["score","missing"]}]}`,
		`{"!==":[{"var":["amount","missing"]},{"var":["score","other"]}]}`,
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			for _, logic := range cases {
				t.Run(logic, func(t *testing.T) {
					t.Parallel()
					sql, err := tr.TranspileCondition(logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if strings.Contains(sql, "COALESCE(") || strings.Contains(sql, "coalesce(") {
						t.Fatalf("TranspileCondition() SQL = %q, should split null/default branches instead of COALESCE", sql)
					}
					paramSQL, params, err := tr.TranspileParameterizedCondition(logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if strings.Contains(paramSQL, "COALESCE(") || strings.Contains(paramSQL, "coalesce(") {
						t.Fatalf("TranspileParameterizedCondition() SQL = %q, should split null/default branches instead of COALESCE", paramSQL)
					}
					if len(params) != 0 {
						t.Fatalf("TranspileParameterizedCondition() params = %#v, want no params for static default branches", params)
					}
				})
			}
		})
	}
}

func TestTypeMetadataRejectsObjectArrayEqualityAndScalarMembershipAllDialects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		logic   string
		wantErr string
	}{
		{
			name:    "incompatible object array equality",
			logic:   `{"==":[{"var":"items"},{"var":"items2"}]}`,
			wantErr: "object-array fields",
		},
		{
			name:    "scalar in object array",
			logic:   `{"in":[1,{"var":"items"}]}`,
			wantErr: "",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, typeMetadataRegressionSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					wantErr := tc.wantErr
					if d == DialectBigQuery && tc.name == "incompatible object array equality" {
						wantErr = "equality between array fields"
					}

					sql, err := tr.TranspileCondition(tc.logic)
					if wantErr == "" {
						if err != nil {
							t.Fatalf("TranspileCondition() error = %v", err)
						}
						if sql != "FALSE" {
							t.Fatalf("TranspileCondition() SQL = %q, want FALSE", sql)
						}
					} else if err == nil || !strings.Contains(err.Error(), wantErr) {
						t.Fatalf("TranspileCondition() SQL = %q error = %v, want error containing %q", sql, err, wantErr)
					}

					paramSQL, _, paramErr := tr.TranspileParameterizedCondition(tc.logic)
					if wantErr == "" {
						if paramErr != nil {
							t.Fatalf("TranspileParameterizedCondition() error = %v", paramErr)
						}
						if paramSQL != "FALSE" {
							t.Fatalf("TranspileParameterizedCondition() SQL = %q, want FALSE", paramSQL)
						}
					} else if paramErr == nil || !strings.Contains(paramErr.Error(), wantErr) {
						t.Fatalf("TranspileParameterizedCondition() SQL = %q error = %v, want error containing %q",
							paramSQL, paramErr, wantErr)
					}
				})
			}
		})
	}
}

func TestTypeMetadataRejectsNestedArrayMapForUnsupportedDialects(t *testing.T) {
	t.Parallel()

	logic := `{"map":[[1,2],[{"var":""}]]}`
	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, emptyTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			_, inlineErr := tr.TranspileValue(logic)
			_, _, paramErr := tr.TranspileParameterizedValue(logic)
			if testRejectsNestedArrayValues(d) {
				want := "does not support array literals whose elements are arrays"
				if d == DialectPostgreSQL {
					want = "does not support array values whose elements are arrays"
				}
				for mode, err := range map[string]error{"inline": inlineErr, "parameterized": paramErr} {
					if err == nil {
						t.Fatalf("%s mode succeeded, want nested-array dialect error", mode)
					}
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("%s mode error = %v, want nested-array dialect error", mode, err)
					}
				}
				return
			}
			if inlineErr != nil {
				t.Fatalf("TranspileValue() error = %v", inlineErr)
			}
			if paramErr != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", paramErr)
			}
		})
	}
}

func TestTypeMetadataRejectsPostgreSQLRaggedNestedArrayLiterals(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		logic string
	}{
		{
			name:  "direct value literal",
			logic: `[[1],[2,3]]`,
		},
		{
			name:  "array default",
			logic: `{"var":["tags",[[1],[2,3]]]}`,
		},
		{
			name:  "array operator source",
			logic: `{"map":[[[1],[2,3]],{"var":""}]}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(DialectPostgreSQL, mustNewSchema([]FieldSchema{
				{Name: "tags", Type: FieldTypeArray},
			}))
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			_, err = tr.TranspileValue(tc.logic)
			if err == nil || !strings.Contains(err.Error(), "rectangular") {
				t.Fatalf("TranspileValue() error = %v, want rectangular-array error", err)
			}

			_, _, err = tr.TranspileParameterizedValue(tc.logic)
			if err == nil || !strings.Contains(err.Error(), "rectangular") {
				t.Fatalf("TranspileParameterizedValue() error = %v, want rectangular-array error", err)
			}
		})
	}
}

func TestTypeMetadataPreservesCustomArrayElementTypesAllDialects(t *testing.T) {
	t.Parallel()

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, emptyTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			if err := tr.RegisterOperatorFunc("idarray", func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 1 {
					return OperatorResult{}, fmt.Errorf("idarray requires one argument")
				}
				res := ArrayValueSQL(args[0].SQL, args[0].ArrayElementType)
				res.ArrayElementTypes = args[0].ArrayElementTypes
				return res, nil
			}); err != nil {
				t.Fatalf("RegisterOperatorFunc() error = %v", err)
			}

			logic := `{"filter":[{"idarray":[[1,2,0]]},{"var":""}]}`
			if _, err := tr.TranspileValue(logic); err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if _, _, err := tr.TranspileParameterizedValue(logic); err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
		})
	}
}

func TestTypeMetadataPreservesCustomArraySchemaScopesAllDialects(t *testing.T) {
	t.Parallel()

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, typeMetadataRegressionSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			if err := tr.RegisterOperatorFunc("idarray", func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 1 {
					return OperatorResult{}, fmt.Errorf("idarray requires one argument")
				}
				res := ArrayValueSQL(args[0].SQL, args[0].ArrayElementType)
				res.ArrayElementTypes = args[0].ArrayElementTypes
				res.ArrayElementSchemaScopes = args[0].ArrayElementSchemaScopes
				return res, nil
			}); err != nil {
				t.Fatalf("RegisterOperatorFunc() error = %v", err)
			}

			logic := `{"map":[{"idarray":[{"var":"items"}]},{"var":"x"}]}`
			if _, err := tr.TranspileValue(logic); err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if _, _, err := tr.TranspileParameterizedValue(logic); err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
		})
	}
}

func TestTypeMetadataRejectsNonEmptyObjectArrayDefaultsAllDialects(t *testing.T) {
	t.Parallel()

	logic := `{"map":[{"var":["items",[1]]},{"var":"x"}]}`
	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, typeMetadataRegressionSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			expectValueAndParamErrorContains(t, tr, logic, "object-array field 'items'")
		})
	}
}

func TestTypeMetadataRejectsCurrentObjectElementScalarDefaultsAllDialects(t *testing.T) {
	t.Parallel()

	logic := `{"map":[{"var":"items"},{"var":["","fallback"]}]}`
	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, typeMetadataRegressionSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			expectValueAndParamErrorContains(t, tr, logic, "array element")
		})
	}
}

func TestTypeMetadataPreservesCurrentObjectScopesThroughValueBranchesAllDialects(t *testing.T) {
	t.Parallel()

	logic := `{"filter":[{"map":[{"var":"items"},{"if":[{">":[{"var":"x"},0]},{"var":""},{"var":""}]}]},{">":[{"var":"x"},0]}]}`
	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, typeMetadataRegressionSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			if sql, err := tr.TranspileValue(logic); err != nil {
				t.Fatalf("TranspileValue() SQL = %q, error = %v", sql, err)
			}
			if sql, _, err := tr.TranspileParameterizedValue(logic); err != nil {
				t.Fatalf("TranspileParameterizedValue() SQL = %q, error = %v", sql, err)
			}
		})
	}
}

func TestTypeMetadataAllowsCurrentObjectElementNullDefaultsAllDialects(t *testing.T) {
	t.Parallel()

	logic := `{"filter":[{"map":[{"var":"items"},{"var":["",null]}]},{">":[{"var":"x"},0]}]}`
	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, typeMetadataRegressionSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			if sql, err := tr.TranspileValue(logic); err != nil {
				t.Fatalf("TranspileValue() SQL = %q, error = %v", sql, err)
			} else if strings.Contains(sql, "COALESCE(elem, NULL)") || strings.Contains(sql, "COALESCE(elem,NULL)") {
				t.Fatalf("TranspileValue() SQL = %q, want current object null default simplified", sql)
			}
			if sql, _, err := tr.TranspileParameterizedValue(logic); err != nil {
				t.Fatalf("TranspileParameterizedValue() SQL = %q, error = %v", sql, err)
			} else if strings.Contains(sql, "COALESCE(elem, NULL)") || strings.Contains(sql, "COALESCE(elem,NULL)") {
				t.Fatalf("TranspileParameterizedValue() SQL = %q, want current object null default simplified", sql)
			}
		})
	}
}

func TestTypeMetadataPreservesArrayReduceResultSchemaScopes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "groups", Type: FieldTypeArray, ElementFields: []FieldSchema{
			{Name: "items", Type: FieldTypeArray, ElementFields: []FieldSchema{
				{Name: "x", Type: FieldTypeNumber},
			}},
		}},
	})
	logic := `{"map":[{"reduce":[{"var":"groups"},{"merge":[{"var":"accumulator"},{"var":"current.items"}]},[]]},{"var":"x"}]}`
	for _, d := range []Dialect{DialectDuckDB, DialectClickHouse} {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			if _, err := tr.TranspileValue(logic); err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if _, _, err := tr.TranspileParameterizedValue(logic); err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
		})
	}
}

func TestTypeMetadataPreservesNestedArrayMetadataForCustomOperators(t *testing.T) {
	t.Parallel()

	logic := `{"inspect":[{"map":[{"var":"items"},{"filter":[{"var":"children"},{">":[{"var":"y"},0]}]}]}]}`
	for _, d := range []Dialect{DialectDuckDB, DialectClickHouse} {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, typeMetadataRegressionSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			if err := tr.RegisterOperatorFunc("inspect", func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 1 {
					return OperatorResult{}, fmt.Errorf("inspect requires one argument")
				}
				wantTypes := []ExpressionType{ExpressionTypeArray, ExpressionTypeObject}
				if !samePublicExpressionTypes(args[0].ArrayElementTypes, wantTypes) {
					return OperatorResult{}, fmt.Errorf("array element types = %v, want %v", args[0].ArrayElementTypes, wantTypes)
				}
				if !sameStrings(args[0].ArrayElementSchemaScopes, []string{"items.children"}) {
					return OperatorResult{}, fmt.Errorf("array element scopes = %v, want [items.children]", args[0].ArrayElementSchemaScopes)
				}
				res := ArrayValueSQL(args[0].SQL, args[0].ArrayElementType)
				res.ArrayElementTypes = args[0].ArrayElementTypes
				res.ArrayElementSchemaScopes = args[0].ArrayElementSchemaScopes
				return res, nil
			}); err != nil {
				t.Fatalf("RegisterOperatorFunc() error = %v", err)
			}

			if _, err := tr.TranspileValue(logic); err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if _, _, err := tr.TranspileParameterizedValue(logic); err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
		})
	}
}

func TestTypeMetadataRejectsObjectArrayBranchWithUnknownSchemaScopeAllDialects(t *testing.T) {
	t.Parallel()

	logic := `{"map":[{"if":[{"var":"flag"},{"var":"items"},{"externalItems":[]}]},{"var":"x"}]}`
	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, typeMetadataRegressionSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			if err := tr.RegisterOperatorFunc("externalItems", func(_ string, _ []OperatorArg) (OperatorResult, error) {
				return ArrayValueSQL("external_items", ExpressionTypeObject), nil
			}); err != nil {
				t.Fatalf("RegisterOperatorFunc() error = %v", err)
			}

			expectValueAndParamErrorContains(t, tr, logic, "known object element schema and unknown object element schema")
		})
	}
}

func samePublicExpressionTypes(got, want []ExpressionType) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
