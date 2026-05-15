package jsonlogic2sql

import (
	"strings"
	"testing"
)

func testArrayScopeSchema() *Schema {
	return mustNewSchema([]FieldSchema{
		{Name: "numbers", Type: FieldTypeArray},
		{Name: "scores", Type: FieldTypeArray},
		{Name: "groups", Type: FieldTypeArray},
		{Name: "type", Type: FieldTypeString},
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

func TestTranspile_ArrayLambdaVarSemantics_AllDialectsSchemaModes(t *testing.T) {
	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-aware", schema: testArrayScopeSchema()},
		{name: "schema-less", schema: nil},
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

					bareReduceSQL, err := tr.TranspileValue(`{"reduce":[{"var":"numbers"},{"var":"type"},""]}`)
					if err != nil {
						t.Fatalf("reduce bare field transpilation error: %v", err)
					}
					if strings.Contains(bareReduceSQL, "elem.type") {
						t.Fatalf("expected reduce bare field to remain non-element scoped, got: %s", bareReduceSQL)
					}
				})
			}
		})
	}
}

func TestTranspile_ArrayLambdaRejectsLegacyElementAliases_AllDialectsSchemaModes(t *testing.T) {
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
	reduceRejected := []string{".type", "item.type", "elem.type", ""}

	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-aware", schema: testArrayScopeSchema()},
		{name: "schema-less", schema: nil},
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
