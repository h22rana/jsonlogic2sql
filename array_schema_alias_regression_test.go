package jsonlogic2sql

import (
	"strings"
	"testing"
)

func testArrayScopeSchema() *Schema {
	return mustNewSchema([]FieldSchema{
		{
			Name: "numbers",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "type", Type: FieldTypeString},
				{Name: "item", Type: FieldTypeString},
				{Name: "current", Type: FieldTypeString},
				{Name: "elem", Type: FieldTypeString},
				{Name: "name", Type: FieldTypeArray},
				{Name: "email", Type: FieldTypeString},
				{Name: "phone", Type: FieldTypeString},
			},
		},
		{Name: "scores", Type: FieldTypeArray},
		{
			Name: "groups",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "base", Type: FieldTypeNumber},
				{Name: "flag", Type: FieldTypeBoolean},
				{Name: "tags", Type: FieldTypeArray},
				{Name: "values", Type: FieldTypeArray},
			},
		},
	})
}

func allDialects() []Dialect {
	return []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}
}

func TestTranspile_ArrayScopeVarsWithSchema(t *testing.T) {
	tests := []struct {
		name        string
		jsonLogic   string
		valueRoot   bool
		mustContain []string
	}{
		{
			name:      "map uses empty var",
			jsonLogic: `{"map":[{"var":"numbers"},{"*":[{"var":""},2]}]}`,
			valueRoot: true,
			mustContain: []string{
				"elem",
			},
		},
		{
			name:      "filter uses empty var",
			jsonLogic: `{"filter":[{"var":"numbers"},{">":[{"var":""},1]}]}`,
			valueRoot: true,
			mustContain: []string{
				"elem",
			},
		},
		{
			name:      "all supports array-form var default",
			jsonLogic: `{"all":[{"var":"scores"},{">":[{"var":["",0]},50]}]}`,
			mustContain: []string{
				"COALESCE(elem, 0)",
			},
		},
		{
			name:      "reduce supports array-form current default",
			jsonLogic: `{"reduce":[{"var":"numbers"},{"+":[{"var":"accumulator"},{"var":["current",0]}]},1]}`,
			valueRoot: true,
			mustContain: []string{
				"COALESCE(elem, 0)",
			},
		},
		{
			name:      "nested reduce initial uses outer scoped field",
			jsonLogic: `{"map":[{"var":"groups"},{"reduce":[{"var":"values"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"base"}]}]}`,
			valueRoot: true,
			mustContain: []string{
				"elem.base",
			},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  testArrayScopeSchema(),
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error: %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					var sql string
					var err error
					if tt.valueRoot {
						sql, err = tr.TranspileValue(tt.jsonLogic)
					} else {
						sql, err = tr.TranspileCondition(tt.jsonLogic)
					}
					if err != nil {
						t.Fatalf("transpile error: %v", err)
					}
					for _, frag := range tt.mustContain {
						if !strings.Contains(sql, frag) {
							t.Fatalf("expected SQL to contain %q, got: %s", frag, sql)
						}
					}
				})
			}
		})
	}
}

func TestTranspileParameterized_ArrayScopeVarsWithSchema(t *testing.T) {
	tests := []struct {
		name           string
		jsonLogic      string
		valueRoot      bool
		mustContainSQL string
		wantParamCount int
	}{
		{
			name:           "map uses empty var",
			jsonLogic:      `{"map":[{"var":"numbers"},{"*":[{"var":""},2]}]}`,
			valueRoot:      true,
			mustContainSQL: "elem",
			wantParamCount: 1,
		},
		{
			name:           "all supports array-form var default",
			jsonLogic:      `{"all":[{"var":"scores"},{">":[{"var":["",0]},50]}]}`,
			mustContainSQL: "COALESCE(elem",
			wantParamCount: 2,
		},
		{
			name:           "reduce supports array-form current default",
			jsonLogic:      `{"reduce":[{"var":"numbers"},{"+":[{"var":"accumulator"},{"var":["current",0]}]},1]}`,
			valueRoot:      true,
			mustContainSQL: "COALESCE(elem",
			wantParamCount: 2,
		},
		{
			name:           "nested reduce initial uses outer scoped field",
			jsonLogic:      `{"map":[{"var":"groups"},{"reduce":[{"var":"values"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"base"}]}]}`,
			valueRoot:      true,
			mustContainSQL: "elem.base",
			wantParamCount: 0,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  testArrayScopeSchema(),
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error: %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					var sql string
					var params []QueryParam
					var err error
					if tt.valueRoot {
						sql, params, err = tr.TranspileParameterizedValue(tt.jsonLogic)
					} else {
						sql, params, err = tr.TranspileParameterizedCondition(tt.jsonLogic)
					}
					if err != nil {
						t.Fatalf("parameterized transpile error: %v", err)
					}
					if !strings.Contains(sql, tt.mustContainSQL) {
						t.Fatalf("expected SQL to contain %q, got: %s", tt.mustContainSQL, sql)
					}
					if got := len(params); got != tt.wantParamCount {
						t.Fatalf("param count = %d, want %d (params=%v)", got, tt.wantParamCount, params)
					}
				})
			}
		})
	}
}

func TestArrayPredicateLambdasUseTruthinessAllDialectsSchemaRequired(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{
			Name: "items",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "active", Type: FieldTypeBoolean},
				{Name: "name", Type: FieldTypeString},
				{Name: "score", Type: FieldTypeNumber},
				{Name: "tags", Type: FieldTypeArray},
			},
		},
	})

	arrayLength := func(d Dialect, expr string) string {
		switch d {
		case DialectPostgreSQL:
			return "CARDINALITY(" + expr + ")"
		case DialectDuckDB, DialectClickHouse:
			return "length(" + expr + ")"
		default:
			return "ARRAY_LENGTH(" + expr + ")"
		}
	}

	tests := []struct {
		name      string
		valueRoot bool
		logic     string
		want      func(Dialect) string
	}{
		{
			name:      "filter boolean field",
			valueRoot: true,
			logic:     `{"filter":[{"var":"items"},{"var":"active"}]}`,
			want: func(Dialect) string {
				return "elem.active IS TRUE"
			},
		},
		{
			name:  "some string field",
			logic: `{"some":[{"var":"items"},{"var":"name"}]}`,
			want: func(Dialect) string {
				return "(elem.name IS NOT NULL AND elem.name != '')"
			},
		},
		{
			name:  "all number field",
			logic: `{"all":[{"var":"items"},{"var":"score"}]}`,
			want: func(Dialect) string {
				return "(elem.score IS NOT NULL AND elem.score != 0)"
			},
		},
		{
			name:  "none array field",
			logic: `{"none":[{"var":"items"},{"var":"tags"}]}`,
			want: func(d Dialect) string {
				return "(elem.tags IS NOT NULL AND " + arrayLength(d, "elem.tags") + " > 0)"
			},
		},
		{
			name:      "filter truthiness-only logical",
			valueRoot: true,
			logic:     `{"filter":[{"var":"items"},{"and":[{"var":"active"},"kept"]}]}`,
			want: func(Dialect) string {
				return "elem.active IS TRUE"
			},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					t.Parallel()

					typedSchemaTr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}

					var got string
					var gotParams []QueryParam
					if tt.valueRoot {
						got, err = typedSchemaTr.TranspileValue(tt.logic)
					} else {
						got, err = typedSchemaTr.TranspileCondition(tt.logic)
					}
					if err != nil {
						t.Fatalf("schema-required inline error = %v", err)
					}
					if want := tt.want(d); !strings.Contains(got, want) {
						t.Fatalf("schema-required inline SQL = %q, want to contain %q", got, want)
					}

					if tt.valueRoot {
						got, gotParams, err = typedSchemaTr.TranspileParameterizedValue(tt.logic)
					} else {
						got, gotParams, err = typedSchemaTr.TranspileParameterizedCondition(tt.logic)
					}
					if err != nil {
						t.Fatalf("schema-required parameterized error = %v", err)
					}
					if want := tt.want(d); !strings.Contains(got, want) {
						t.Fatalf("schema-required parameterized SQL = %q, want to contain %q", got, want)
					}
					if len(gotParams) != 0 {
						t.Fatalf("schema-required params = %#v, want none", gotParams)
					}
				})
			}
		})
	}
}

func TestArrayScopedDefaultedVarsPreserveSchemaEqualityMetadata(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{
			Name: "items",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "amount", Type: FieldTypeNumber},
			},
		},
	})

	cases := []struct {
		name  string
		logic string
	}{
		{
			name:  "loose equality",
			logic: `{"some":[{"var":"items"},{"==":[{"var":["amount","abc"]},"abc"]}]}`,
		},
		{
			name:  "strict equality",
			logic: `{"some":[{"var":"items"},{"===":[{"var":["amount","abc"]},"abc"]}]}`,
		},
		{
			name:  "loose inequality",
			logic: `{"some":[{"var":"items"},{"!=":[{"var":["amount","abc"]},"abc"]}]}`,
		},
		{
			name:  "strict inequality",
			logic: `{"some":[{"var":"items"},{"!==":[{"var":["amount","abc"]},"abc"]}]}`,
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
				t.Fatalf("NewTranspilerWithConfig() error: %v", err)
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					sql, err := tr.TranspileCondition(tc.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error: %v", err)
					}
					if strings.Contains(sql, "WHERE FALSE") || strings.Contains(sql, "WHERE TRUE") {
						t.Fatalf("inline scoped default comparison folded away: %s", sql)
					}
					if !strings.Contains(sql, "COALESCE(elem.amount, 'abc')") {
						t.Fatalf("inline SQL did not preserve scoped default comparison, got: %s", sql)
					}

					paramSQL, params, err := tr.TranspileParameterizedCondition(tc.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error: %v", err)
					}
					if strings.Contains(paramSQL, "WHERE FALSE") || strings.Contains(paramSQL, "WHERE TRUE") {
						t.Fatalf("parameterized scoped default comparison folded away: %s params=%#v", paramSQL, params)
					}
					if !strings.Contains(paramSQL, "COALESCE(elem.amount, ") {
						t.Fatalf("parameterized SQL did not preserve scoped default comparison, got: %s params=%#v", paramSQL, params)
					}
					if len(params) != 2 {
						t.Fatalf("params = %#v, want default and comparison literal", params)
					}
				})
			}
		})
	}
}

func TestTranspile_ArrayNestedScopeUsesDistinctAliases(t *testing.T) {
	tests := []struct {
		name        string
		jsonLogic   string
		expectElem1 bool
	}{
		{
			name:        "nested reduce in map",
			jsonLogic:   `{"map":[{"var":"groups"},{"reduce":[{"var":"values"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"base"}]}]}`,
			expectElem1: true,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  testArrayScopeSchema(),
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error: %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					sql, err := tr.TranspileValue(tt.jsonLogic)
					if err != nil {
						t.Fatalf("TranspileValue() error: %v", err)
					}
					if tt.expectElem1 && !strings.Contains(sql, "elem1") {
						allowClickHouseOptimizedReduce := d == DialectClickHouse &&
							strings.Contains(sql, "arrayReduce('sum', elem.values)") &&
							strings.Contains(sql, "elem.base +")
						if !allowClickHouseOptimizedReduce {
							t.Fatalf("expected nested alias elem1, got: %s", sql)
						}
					}
					if strings.Contains(sql, "UNNEST(elem.values) AS elem)") {
						t.Fatalf("found alias-shadow SQL: %s", sql)
					}
					if strings.Contains(sql, "arrayFold((acc, elem) ->") {
						t.Fatalf("found clickhouse alias-shadow SQL: %s", sql)
					}
				})
			}
		})
	}
}

func TestTranspileParameterized_ArrayNestedScopeUsesDistinctAliases(t *testing.T) {
	tests := []struct {
		name        string
		jsonLogic   string
		expectElem1 bool
	}{
		{
			name:        "nested reduce in map",
			jsonLogic:   `{"map":[{"var":"groups"},{"reduce":[{"var":"values"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"base"}]}]}`,
			expectElem1: true,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  testArrayScopeSchema(),
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error: %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					sql, _, err := tr.TranspileParameterizedValue(tt.jsonLogic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error: %v", err)
					}
					if tt.expectElem1 && !strings.Contains(sql, "elem1") {
						allowClickHouseOptimizedReduce := d == DialectClickHouse &&
							strings.Contains(sql, "arrayReduce('sum', elem.values)") &&
							strings.Contains(sql, "elem.base +")
						if !allowClickHouseOptimizedReduce {
							t.Fatalf("expected nested alias elem1, got: %s", sql)
						}
					}
					if strings.Contains(sql, "UNNEST(elem.values) AS elem)") {
						t.Fatalf("found alias-shadow SQL: %s", sql)
					}
					if strings.Contains(sql, "arrayFold((acc, elem) ->") {
						t.Fatalf("found clickhouse alias-shadow SQL: %s", sql)
					}
				})
			}
		})
	}
}

func TestTranspile_ArrayScopedVarsPreserveSchemaValidation(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{
			Name: "groups",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "values", Type: FieldTypeString},
				{Name: "flag", Type: FieldTypeBoolean},
				{Name: "tags", Type: FieldTypeArray},
			},
		},
	})

	tests := []struct {
		name      string
		logic     string
		wantError string
	}{
		{
			name:      "nested map rejects scoped string source",
			logic:     `{"map":[{"var":"groups"},{"map":[{"var":"values"},{"var":""}]}]}`,
			wantError: "array operation on non-array field 'groups.values'",
		},
		{
			name:      "filter rejects scoped boolean ordering",
			logic:     `{"filter":[{"var":"groups"},{">":[{"var":"flag"},0]}]}`,
			wantError: "ordering comparison '>' on incompatible field 'groups.flag'",
		},
		{
			name:      "reduce rejects current boolean ordering",
			logic:     `{"reduce":[{"var":"groups"},{"if":[{">":[{"var":"current.flag"},0]},{"var":"accumulator"},{"var":"accumulator"}]},0]}`,
			wantError: "ordering comparison '>' on incompatible field 'groups.flag'",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error: %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if _, err := tr.TranspileValue(tt.logic); err == nil || !strings.Contains(err.Error(), tt.wantError) {
						t.Fatalf("TranspileValue() error = %v, want containing %q", err, tt.wantError)
					}
					sql, params, err := tr.TranspileParameterizedValue(tt.logic)
					if err == nil || !strings.Contains(err.Error(), tt.wantError) {
						t.Fatalf("TranspileParameterizedValue() error = %v, want containing %q (SQL %q params %#v)",
							err, tt.wantError, sql, params)
					}
				})
			}
		})
	}
}

func TestTranspile_ArrayScopeUnknownFieldsRejectedWithSchema_AllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{
			Name: "numbers",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "type", Type: FieldTypeString},
			},
		},
	})

	cases := []struct {
		name      string
		logic     string
		valueRoot bool
	}{
		{
			name:      "map bare field",
			logic:     `{"map":[{"var":"numbers"},{"var":"unknown"}]}`,
			valueRoot: true,
		},
		{
			name:      "map defaulted field",
			logic:     `{"map":[{"var":"numbers"},{"var":["unknown","x"]}]}`,
			valueRoot: true,
		},
		{
			name:      "filter missing field",
			logic:     `{"filter":[{"var":"numbers"},{"missing":"unknown"}]}`,
			valueRoot: true,
		},
		{
			name:      "reduce current field",
			logic:     `{"reduce":[{"var":"numbers"},{"cat":[{"var":"accumulator"},{"var":"current.unknown"}]},""]}`,
			valueRoot: true,
		},
		{
			name:  "some predicate field",
			logic: `{"some":[{"var":"numbers"},{"==":[{"var":"unknown"},"x"]}]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			typedSchemaTr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error: %v", err)
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					assertUnknownScopedFieldRejected(t, typedSchemaTr, tc.logic, tc.valueRoot)
				})
			}
		})
	}
}

func TestTranspile_ArrayElementFieldsAreScopedOnly_AllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{
			Name: "accounts",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "status", Type: FieldTypeString},
				{Name: "elem", Type: FieldTypeObject, Fields: []FieldSchema{
					{Name: "x", Type: FieldTypeString},
				}},
				{Name: "elem2", Type: FieldTypeObject, Fields: []FieldSchema{
					{Name: "x", Type: FieldTypeString},
				}},
				{Name: "elem10", Type: FieldTypeObject, Fields: []FieldSchema{
					{Name: "score", Type: FieldTypeNumber},
				}},
			},
		},
	})

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{Dialect: d, Schema: schema})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error: %v", err)
			}

			for _, logic := range []string{
				`{"==":[{"var":"accounts.status"},"active"]}`,
				`{"==":[{"var":"accounts.elem2.x"},"active"]}`,
			} {
				if _, err := tr.TranspileCondition(logic); err == nil {
					t.Fatalf("TranspileCondition(%s) should reject root access to array element field", logic)
				}
				if _, _, err := tr.TranspileParameterizedCondition(logic); err == nil {
					t.Fatalf("TranspileParameterizedCondition(%s) should reject root access to array element field", logic)
				}
			}

			for _, tc := range []struct {
				name  string
				logic string
				want  string
			}{
				{
					name:  "map base elem-prefixed field",
					logic: `{"map":[{"var":"accounts"},{"var":"elem.x"}]}`,
					want:  "elem.elem.x",
				},
				{
					name:  "map numbered elem-prefixed field",
					logic: `{"map":[{"var":"accounts"},{"var":"elem2.x"}]}`,
					want:  "elem.elem2.x",
				},
				{
					name:  "some numbered elem-prefixed field",
					logic: `{"some":[{"var":"accounts"},{"==":[{"var":"elem2.x"},"active"]}]}`,
					want:  "elem.elem2.x",
				},
				{
					name:  "reduce current numbered elem-prefixed field",
					logic: `{"reduce":[{"var":"accounts"},{"cat":[{"var":"accumulator"},{"var":"current.elem2.x"}]},""]}`,
					want:  "elem.elem2.x",
				},
				{
					name:  "map multi-digit elem-prefixed field",
					logic: `{"map":[{"var":"accounts"},{"var":"elem10.score"}]}`,
					want:  "elem.elem10.score",
				},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					assertArrayElementScopedSQLContains(t, tr, tc.logic, tc.want)
				})
			}
			assertArrayScopeAliasRejected(t, tr, `{"map":[{"var":"accounts"},{"var":"elem.type"}]}`, true)
		})
	}
}

func TestTranspile_ArrayNestedElementFieldsRequireNestedScope_AllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{
			Name: "items",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{
					Name: "values",
					Type: FieldTypeArray,
					ElementFields: []FieldSchema{
						{Name: "tag", Type: FieldTypeString},
						{Name: "score", Type: FieldTypeNumber},
					},
				},
				{
					Name: "details",
					Type: FieldTypeObject,
					Fields: []FieldSchema{
						{Name: "issuer", Type: FieldTypeString},
						{
							Name: "events",
							Type: FieldTypeArray,
							ElementFields: []FieldSchema{
								{Name: "code", Type: FieldTypeEnum, AllowedValues: []string{"AUTH", "CAPTURE"}},
							},
						},
					},
				},
			},
		},
	})

	invalid := []string{
		`{"map":[{"var":"items"},{"var":"values.tag"}]}`,
		`{"map":[{"var":"items"},{"var":"details.events.code"}]}`,
	}
	valid := []struct {
		name  string
		logic string
		want  []string
	}{
		{
			name:  "direct object descendant in parent scope",
			logic: `{"map":[{"var":"items"},{"var":"details.issuer"}]}`,
			want:  []string{"elem.details.issuer"},
		},
		{
			name:  "nested array element field after nested lambda",
			logic: `{"map":[{"var":"items"},{"map":[{"var":"values"},{"var":"tag"}]}]}`,
			want:  []string{"elem.values", "elem1.tag"},
		},
		{
			name:  "nested object array enum after nested lambda",
			logic: `{"map":[{"var":"items"},{"filter":[{"var":"details.events"},{"==":[{"var":"code"},"AUTH"]}]}]}`,
			want:  []string{"elem.details.events", "elem1.code"},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{Dialect: d, Schema: schema})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error: %v", err)
			}

			for _, logic := range invalid {
				t.Run("reject/"+logic, func(t *testing.T) {
					t.Parallel()
					assertArrayNestedElementFieldRejected(t, tr, logic)
				})
			}
			for _, tc := range valid {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					assertArrayElementScopedSQLContainsAll(t, tr, tc.logic, tc.want)
				})
			}
		})
	}
}

func assertArrayNestedElementFieldRejected(t *testing.T, tr *Transpiler, logic string) {
	t.Helper()

	if sql, err := tr.TranspileValue(logic); err == nil || !strings.Contains(err.Error(), "is not defined in schema scope") {
		t.Fatalf("TranspileValue() SQL = %q, error = %v, want scoped schema error", sql, err)
	}
	if sql, params, err := tr.TranspileParameterizedValue(logic); err == nil ||
		!strings.Contains(err.Error(), "is not defined in schema scope") {
		t.Fatalf("TranspileParameterizedValue() SQL = %q params = %#v, error = %v, want scoped schema error",
			sql, params, err)
	}
}

func assertArrayElementScopedSQLContainsAll(t *testing.T, tr *Transpiler, logic string, wants []string) {
	t.Helper()

	inlineSQL, err := tr.TranspileValue(logic)
	if err != nil {
		t.Fatalf("TranspileValue() error = %v", err)
	}
	for _, want := range wants {
		if !strings.Contains(inlineSQL, want) {
			t.Fatalf("TranspileValue() SQL = %q, want to contain %q", inlineSQL, want)
		}
	}

	paramSQL, _, err := tr.TranspileParameterizedValue(logic)
	if err != nil {
		t.Fatalf("TranspileParameterizedValue() error = %v", err)
	}
	for _, want := range wants {
		if !strings.Contains(paramSQL, want) {
			t.Fatalf("TranspileParameterizedValue() SQL = %q, want to contain %q", paramSQL, want)
		}
	}
}

func assertArrayElementScopedSQLContains(t *testing.T, tr *Transpiler, logic, want string) {
	t.Helper()

	inlineSQL, inlineErr := tr.TranspileValue(logic)
	if inlineErr != nil {
		inlineSQL, inlineErr = tr.TranspileCondition(logic)
	}
	if inlineErr != nil {
		t.Fatalf("inline transpilation error = %v", inlineErr)
	}
	if !strings.Contains(inlineSQL, want) {
		t.Fatalf("inline SQL = %q, want to contain %q", inlineSQL, want)
	}

	paramSQL, _, paramErr := tr.TranspileParameterizedValue(logic)
	if paramErr != nil {
		paramSQL, _, paramErr = tr.TranspileParameterizedCondition(logic)
	}
	if paramErr != nil {
		t.Fatalf("parameterized transpilation error = %v", paramErr)
	}
	if !strings.Contains(paramSQL, want) {
		t.Fatalf("parameterized SQL = %q, want to contain %q", paramSQL, want)
	}
}

func assertUnknownScopedFieldRejected(t *testing.T, tr *Transpiler, logic string, valueRoot bool) {
	t.Helper()

	var err error
	if valueRoot {
		_, err = tr.TranspileValue(logic)
	} else {
		_, err = tr.TranspileCondition(logic)
	}
	if err == nil || !strings.Contains(err.Error(), "field 'unknown' is not defined in schema") {
		t.Fatalf("inline error = %v, want unknown field schema error", err)
	}

	if valueRoot {
		_, _, err = tr.TranspileParameterizedValue(logic)
	} else {
		_, _, err = tr.TranspileParameterizedCondition(logic)
	}
	if err == nil || !strings.Contains(err.Error(), "field 'unknown' is not defined in schema") {
		t.Fatalf("parameterized error = %v, want unknown field schema error", err)
	}
}

func TestTranspile_ArrayLambdaVarSemantics_AllDialectsSchemaRequired(t *testing.T) {
	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-required", schema: testArrayScopeSchema()},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			for _, d := range allDialects() {
				t.Run(d.String(), func(t *testing.T) {
					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error: %v", err)
					}

					bareFieldSQL, err := tr.TranspileValue(`{"map":[{"var":"numbers"},{"var":["type","unknown"]}]}`)
					if err != nil {
						t.Fatalf("bare field transpilation error: %v", err)
					}
					if !strings.Contains(bareFieldSQL, "COALESCE(elem.type") {
						t.Fatalf("expected bare field to resolve against element, got: %s", bareFieldSQL)
					}

					exactNamesSQL, err := tr.TranspileValue(`{"map":[{"var":"numbers"},{"cat":[{"var":"item"},{"var":"current"},{"var":"elem"}]}]}`)
					if err != nil {
						t.Fatalf("exact name transpilation error: %v", err)
					}
					for _, want := range []string{"elem.item", "elem.current", "elem.elem"} {
						if !strings.Contains(exactNamesSQL, want) {
							t.Fatalf("expected exact name to resolve as element field %q, got: %s", want, exactNamesSQL)
						}
					}

					allowedReduceSQL, err := tr.TranspileValue(`{"reduce":[{"var":"numbers"},{"cat":[{"var":"accumulator"},{"var":"current.type"}]},""]}`)
					if err != nil {
						t.Fatalf("reduce current.type transpilation error: %v", err)
					}
					if !strings.Contains(allowedReduceSQL, "elem.type") {
						t.Fatalf("expected reduce current.type to resolve against element, got: %s", allowedReduceSQL)
					}

					assertArrayScopeAliasRejected(t, tr, `{"reduce":[{"var":"numbers"},{"var":"type"},""]}`, true)
				})
			}
		})
	}
}

func TestTranspile_ArrayLambdaRejectsLegacyElementAliases_AllDialectsSchemaRequired(t *testing.T) {
	aliases := []string{".type", "item.type", "current.type", "elem.type"}
	elementOperators := []struct {
		name      string
		logicFor  func(alias string) string
		valueRoot bool
	}{
		{name: "map", valueRoot: true, logicFor: func(alias string) string {
			return `{"map":[{"var":"numbers"},{"var":"` + alias + `"}]}`
		}},
		{name: "filter", valueRoot: true, logicFor: func(alias string) string {
			return `{"filter":[{"var":"numbers"},{"==":[{"var":"` + alias + `"},1]}]}`
		}},
		{name: "all", logicFor: func(alias string) string {
			return `{"all":[{"var":"numbers"},{"==":[{"var":"` + alias + `"},1]}]}`
		}},
		{name: "some", logicFor: func(alias string) string {
			return `{"some":[{"var":"numbers"},{"==":[{"var":"` + alias + `"},1]}]}`
		}},
		{name: "none", logicFor: func(alias string) string {
			return `{"none":[{"var":"numbers"},{"==":[{"var":"` + alias + `"},1]}]}`
		}},
	}
	reduceRejected := []string{".type", "item.type", "elem.type", "", "acc", "type", "current."}

	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-required", schema: testArrayScopeSchema()},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			for _, d := range allDialects() {
				t.Run(d.String(), func(t *testing.T) {
					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error: %v", err)
					}

					for _, op := range elementOperators {
						for _, alias := range aliases {
							t.Run(op.name+"/"+alias, func(t *testing.T) {
								logic := op.logicFor(alias)
								assertArrayScopeAliasRejected(t, tr, logic, op.valueRoot)
							})
						}
					}

					for _, alias := range reduceRejected {
						t.Run("reduce/"+alias, func(t *testing.T) {
							logic := `{"reduce":[{"var":"numbers"},{"var":"` + alias + `"},""]}`
							assertArrayScopeAliasRejected(t, tr, logic, true)
						})
					}
				})
			}
		})
	}
}

func TestTranspile_ArrayLambdaRejectsTrailingDotReduceCurrent_AllDialects(t *testing.T) {
	tests := []struct {
		name  string
		logic string
	}{
		{
			name:  "general reduce",
			logic: `{"reduce":[{"var":"numbers"},{"cat":[{"var":"accumulator"},{"var":"current."}]},""]}`,
		},
		{
			name:  "aggregate reduce",
			logic: `{"reduce":[{"var":"numbers"},{"+":[{"var":"accumulator"},{"var":"current."}]},0]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspilerWithConfig(&TranspilerConfig{Dialect: d, Schema: defaultTestSchema()})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error: %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					assertArrayScopeAliasRejected(t, tr, tt.logic, true)
				})
			}
		})
	}
}

func assertArrayScopeAliasRejected(t *testing.T, tr *Transpiler, logic string, valueRoot bool) {
	t.Helper()

	var err error
	if valueRoot {
		_, err = tr.TranspileValue(logic)
	} else {
		_, err = tr.TranspileCondition(logic)
	}
	assertUnsupportedArrayScopeVar(t, err)

	if valueRoot {
		_, _, err = tr.TranspileParameterizedValue(logic)
	} else {
		_, _, err = tr.TranspileParameterizedCondition(logic)
	}
	assertUnsupportedArrayScopeVar(t, err)
}

func assertUnsupportedArrayScopeVar(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected unsupported array-scope variable error, got nil")
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "unsupported array-scope") && !strings.Contains(msg, "unsupported reduce-scope") {
		t.Fatalf("expected unsupported array/reduce scope error, got: %v", err)
	}
}
