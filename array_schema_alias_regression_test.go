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
			name:      "map supports item alias",
			jsonLogic: `{"map":[{"var":"numbers"},{"*":[{"var":"item"},2]}]}`,
			valueRoot: true,
			mustContain: []string{
				"elem",
			},
		},
		{
			name:      "filter supports current alias",
			jsonLogic: `{"filter":[{"var":"numbers"},{">":[{"var":"current"},1]}]}`,
			valueRoot: true,
			mustContain: []string{
				"elem",
			},
		},
		{
			name:      "all supports array-form var default",
			jsonLogic: `{"all":[{"var":"scores"},{">":[{"var":["item",0]},50]}]}`,
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
			name:      "nested reduce initial uses outer item alias",
			jsonLogic: `{"map":[{"var":"groups"},{"reduce":[{"var":"item.values"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"item.base"}]}]}`,
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
			name:           "map supports item alias",
			jsonLogic:      `{"map":[{"var":"numbers"},{"*":[{"var":"item"},2]}]}`,
			valueRoot:      true,
			mustContainSQL: "elem",
			wantParamCount: 1,
		},
		{
			name:           "all supports array-form var default",
			jsonLogic:      `{"all":[{"var":"scores"},{">":[{"var":["item",0]},50]}]}`,
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
			name:           "nested reduce initial uses outer item alias",
			jsonLogic:      `{"map":[{"var":"groups"},{"reduce":[{"var":"item.values"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"item.base"}]}]}`,
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
			jsonLogic:   `{"map":[{"var":"groups"},{"reduce":[{"var":"item.values"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"item.base"}]}]}`,
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
			jsonLogic:   `{"map":[{"var":"groups"},{"reduce":[{"var":"item.values"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"item.base"}]}]}`,
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
