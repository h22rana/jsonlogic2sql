package jsonlogic2sql

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func matrixSchema() *Schema {
	return mustNewSchema([]FieldSchema{
		{Name: "bag.records", Type: FieldTypeArray},
		{Name: "bag.numbers", Type: FieldTypeArray},
		{Name: "bag.words", Type: FieldTypeArray},
		{Name: "bag.flags", Type: FieldTypeArray},
		{Name: "metrics.amount", Type: FieldTypeNumber},
		{Name: "profile.name", Type: FieldTypeString},
	})
}

type apiOutput struct {
	inlineSQL  string
	condSQL    string
	paramSQL   string
	paramCond  string
	params     []QueryParam
	condParams []QueryParam
}

func decodeLogicMap(t *testing.T, logic string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(logic), &m); err != nil {
		t.Fatalf("json.Unmarshal(map) failed: %v", err)
	}
	return m
}

func decodeLogicAny(t *testing.T, logic string) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(logic), &v); err != nil {
		t.Fatalf("json.Unmarshal(interface) failed: %v", err)
	}
	return v
}

func runAllAPIVariants(t *testing.T, tr *Transpiler, logic string) apiOutput {
	t.Helper()

	inlineSQL, err := tr.TranspileCondition(logic)
	valueMode := IsErrorCode(err, ErrInvalidExpressionContext)
	if valueMode {
		inlineSQL, err = tr.TranspileValue(logic)
	}
	if err != nil {
		t.Fatalf("TranspileCondition() error: %v", err)
	}
	condSQL := inlineSQL
	if !valueMode {
		condSQL, err = tr.TranspileCondition(logic)
		if err != nil {
			t.Fatalf("TranspileCondition() error: %v", err)
		}
	}
	var paramSQL string
	var params []QueryParam
	if valueMode {
		paramSQL, params, err = tr.TranspileParameterizedValue(logic)
	} else {
		paramSQL, params, err = tr.TranspileParameterizedCondition(logic)
	}
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error: %v", err)
	}
	paramCond := paramSQL
	condParams := params
	if !valueMode {
		paramCond, condParams, err = tr.TranspileParameterizedCondition(logic)
		if err != nil {
			t.Fatalf("TranspileParameterizedCondition() error: %v", err)
		}
	}

	logicMap := decodeLogicMap(t, logic)
	logicAny := decodeLogicAny(t, logic)

	var inlineFromMap, inlineFromAny string
	if valueMode {
		inlineFromMap, err = tr.TranspileValueFromMap(logicMap)
	} else {
		inlineFromMap, err = tr.TranspileConditionFromMap(logicMap)
	}
	if err != nil {
		t.Fatalf("TranspileFromMap variant error: %v", err)
	}
	if valueMode {
		inlineFromAny, err = tr.TranspileValueFromInterface(logicAny)
	} else {
		inlineFromAny, err = tr.TranspileConditionFromInterface(logicAny)
	}
	if err != nil {
		t.Fatalf("TranspileFromInterface variant error: %v", err)
	}
	condFromMap := inlineFromMap
	condFromAny := inlineFromAny

	var paramFromMap, paramFromAny string
	var mapParams, anyParams []QueryParam
	if valueMode {
		paramFromMap, mapParams, err = tr.TranspileParameterizedValueFromMap(logicMap)
	} else {
		paramFromMap, mapParams, err = tr.TranspileParameterizedConditionFromMap(logicMap)
	}
	if err != nil {
		t.Fatalf("TranspileParameterizedFromMap variant error: %v", err)
	}
	if valueMode {
		paramFromAny, anyParams, err = tr.TranspileParameterizedValueFromInterface(logicAny)
	} else {
		paramFromAny, anyParams, err = tr.TranspileParameterizedConditionFromInterface(logicAny)
	}
	if err != nil {
		t.Fatalf("TranspileParameterizedFromInterface variant error: %v", err)
	}
	paramCondFromMap := paramFromMap
	paramCondFromAny := paramFromAny
	mapCondParams := mapParams
	anyCondParams := anyParams

	if inlineFromMap != inlineSQL || inlineFromAny != inlineSQL {
		t.Fatalf("inline API mismatch: direct=%q fromMap=%q fromAny=%q", inlineSQL, inlineFromMap, inlineFromAny)
	}
	if condFromMap != condSQL || condFromAny != condSQL {
		t.Fatalf("condition API mismatch: direct=%q fromMap=%q fromAny=%q", condSQL, condFromMap, condFromAny)
	}
	if paramFromMap != paramSQL || paramFromAny != paramSQL {
		t.Fatalf("parameterized API SQL mismatch: direct=%q fromMap=%q fromAny=%q", paramSQL, paramFromMap, paramFromAny)
	}
	if paramCondFromMap != paramCond || paramCondFromAny != paramCond {
		t.Fatalf("parameterized condition SQL mismatch: direct=%q fromMap=%q fromAny=%q", paramCond, paramCondFromMap, paramCondFromAny)
	}
	if len(mapParams) != len(params) || len(anyParams) != len(params) {
		t.Fatalf("parameterized API param count mismatch: direct=%d fromMap=%d fromAny=%d", len(params), len(mapParams), len(anyParams))
	}
	if len(mapCondParams) != len(condParams) || len(anyCondParams) != len(condParams) {
		t.Fatalf("parameterized condition API param count mismatch: direct=%d fromMap=%d fromAny=%d", len(condParams), len(mapCondParams), len(anyCondParams))
	}
	if condSQL != inlineSQL {
		t.Fatalf("condition mismatch: inline=%q cond=%q", inlineSQL, condSQL)
	}
	if paramCond != paramSQL {
		t.Fatalf("parameterized condition mismatch: inline=%q cond=%q", paramSQL, paramCond)
	}

	return apiOutput{
		inlineSQL:  inlineSQL,
		condSQL:    condSQL,
		paramSQL:   paramSQL,
		paramCond:  paramCond,
		params:     params,
		condParams: condParams,
	}
}

func assertPlaceholderStyle(t *testing.T, d Dialect, sql string, paramCount int) {
	t.Helper()
	if paramCount == 0 {
		return
	}
	switch d {
	case DialectPostgreSQL, DialectDuckDB:
		if !strings.Contains(sql, "$1") {
			t.Fatalf("expected $ placeholders for %s, got: %s", d, sql)
		}
	default:
		if !strings.Contains(sql, "@p1") {
			t.Fatalf("expected @p placeholders for %s, got: %s", d, sql)
		}
	}
}

func assertContains(t *testing.T, sql, fragment string) {
	t.Helper()
	if !strings.Contains(sql, fragment) {
		t.Fatalf("expected SQL to contain %q, got: %s", fragment, sql)
	}
}

func assertNotContains(t *testing.T, sql, fragment string) {
	t.Helper()
	if strings.Contains(sql, fragment) {
		t.Fatalf("expected SQL not to contain %q, got: %s", fragment, sql)
	}
}

func TestArrayEdgeMatrix_AllDialects_SchemaAndNoSchema(t *testing.T) {
	type matrixCase struct {
		name      string
		logic     string
		wantParam int
		validate  func(t *testing.T, d Dialect, out apiOutput)
	}

	cases := []matrixCase{
		{
			name:      "small map",
			logic:     `{"map":[{"var":"bag.numbers"},{"*":[{"var":""},2]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayMap(elem -> (elem * 2), bag.numbers)")
				} else {
					assertContains(t, inline, "UNNEST(bag.numbers) AS elem")
					assertContains(t, inline, "(elem * 2)")
				}
			},
		},
		{
			name:      "all length function by dialect",
			logic:     `{"all":[{"var":"bag.numbers"},{">=":[{"var":""},0]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				switch d {
				case DialectBigQuery, DialectSpanner:
					assertContains(t, inline, "ARRAY_LENGTH(bag.numbers)")
				case DialectPostgreSQL:
					assertContains(t, inline, "CARDINALITY(bag.numbers)")
				case DialectDuckDB, DialectClickHouse:
					assertContains(t, inline, "length(bag.numbers)")
				}
			},
		},
		{
			name:      "merge dialect behavior",
			logic:     `{"merge":[{"var":"bag.numbers"},{"var":"bag.words"}]}`,
			wantParam: 0,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				switch d {
				case DialectPostgreSQL:
					assertContains(t, inline, "bag.numbers || bag.words")
				case DialectClickHouse:
					assertContains(t, inline, "arrayConcat(bag.numbers, bag.words)")
				default:
					assertContains(t, inline, "ARRAY_CONCAT(bag.numbers, bag.words)")
				}
			},
		},
		{
			name:      "reduce array-form current default",
			logic:     `{"reduce":[{"var":"bag.numbers"},{"+":[{"var":"accumulator"},{"var":["current",0]}]},1]}`,
			wantParam: 2,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				assertContains(t, inline, "COALESCE(elem, 0)")
			},
		},
		{
			name:      "nested map with outer scoped source",
			logic:     `{"map":[{"var":"bag.records"},{"map":[{"var":"values"},{"var":""}]}]}`,
			wantParam: 0,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayMap(elem1 -> elem1, elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "SELECT elem1")
					assertNotContains(t, inline, "UNNEST(elem.values) AS elem)")
				}
			},
		},
		{
			name:      "nested filter with outer scoped source",
			logic:     `{"map":[{"var":"bag.records"},{"filter":[{"var":"values"},{">=":[{"var":""},0]}]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayFilter(elem1 -> elem1 >= 0, elem.values)")
					assertNotContains(t, inline, "arrayFilter(elem -> elem >= 0, elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "elem1 >= 0")
					assertNotContains(t, inline, "UNNEST(elem.values) AS elem WHERE elem >= 0")
				}
			},
		},
		{
			name:      "nested all with outer scoped source",
			logic:     `{"map":[{"var":"bag.records"},{"all":[{"var":"values"},{">=":[{"var":""},0]}]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayAll(elem1 -> elem1 >= 0, elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "elem1 >= 0")
				}
			},
		},
		{
			name:      "nested some with outer scoped source",
			logic:     `{"map":[{"var":"bag.records"},{"some":[{"var":"values"},{">=":[{"var":""},0]}]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayExists(elem1 -> elem1 >= 0, elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "elem1 >= 0")
				}
			},
		},
		{
			name:      "nested none with outer scoped source",
			logic:     `{"map":[{"var":"bag.records"},{"none":[{"var":"values"},{"<":[{"var":""},0]}]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayExists(elem1 -> elem1 < 0, elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "elem1 < 0")
				}
			},
		},
		{
			name:      "nested reduce with outer scoped source and initial",
			logic:     `{"map":[{"var":"bag.records"},{"reduce":[{"var":"values"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"base"}]}]}`,
			wantParam: 0,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayMap(elem -> elem.base + coalesce(arrayReduce('sum', elem.values), 0), bag.records)")
				} else {
					assertContains(t, inline, "UNNEST(bag.records) AS elem")
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "elem.base + COALESCE((SELECT SUM(elem1)")
				}
			},
		},
		{
			name:      "very deep mixed nesting",
			logic:     `{"and":[{"some":[{"map":[{"var":"bag.records"},{"filter":[{"var":"values"},{">=":[{"var":""},0]}]}]},{"all":[{"var":""},{">=":[{"var":""},0]}]}]},{">=":[{"var":"metrics.amount"},100]}]}`,
			wantParam: 3,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				assertContains(t, inline, "metrics.amount >= 100")
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayFilter(elem1 -> elem1 >= 0, elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "elem1 >= 0")
				}
			},
		},
	}

	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-aware", schema: matrixSchema()},
		{name: "schema-less", schema: nil},
	}

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			for _, d := range dialects {
				t.Run(d.String(), func(t *testing.T) {
					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error: %v", err)
					}

					for _, c := range cases {
						t.Run(c.name, func(t *testing.T) {
							out := runAllAPIVariants(t, tr, c.logic)
							if len(out.params) != c.wantParam {
								t.Fatalf("param count mismatch: got=%d want=%d sql=%s", len(out.params), c.wantParam, out.paramSQL)
							}
							if len(out.condParams) != c.wantParam {
								t.Fatalf("condition param count mismatch: got=%d want=%d sql=%s", len(out.condParams), c.wantParam, out.paramCond)
							}
							assertPlaceholderStyle(t, d, out.paramSQL, c.wantParam)
							if c.validate != nil {
								c.validate(t, d, out)
							}
						})
					}
				})
			}
		})
	}
}

func TestArrayEdgeMatrix_SchemaVsNoSchemaValidation(t *testing.T) {
	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	schemaTrByDialect := make(map[Dialect]*Transpiler, len(dialects))
	noSchemaTrByDialect := make(map[Dialect]*Transpiler, len(dialects))

	for _, d := range dialects {
		withSchema, err := NewTranspilerWithConfig(&TranspilerConfig{
			Dialect: d,
			Schema:  matrixSchema(),
		})
		if err != nil {
			t.Fatalf("with schema transpiler init failed: %v", err)
		}
		noSchema, err := NewTranspilerWithConfig(&TranspilerConfig{
			Dialect: d,
			Schema:  nil,
		})
		if err != nil {
			t.Fatalf("no schema transpiler init failed: %v", err)
		}
		schemaTrByDialect[d] = withSchema
		noSchemaTrByDialect[d] = noSchema
	}

	tests := []struct {
		name          string
		logic         string
		wantSchemaErr string
		wantNoSchema  map[Dialect]string
	}{
		{
			name:          "unknown field rejected with schema",
			logic:         `{"map":[{"var":"unknown.arr"},{"+":[{"var":""},1]}]}`,
			wantSchemaErr: "is not defined in schema",
			wantNoSchema: map[Dialect]string{
				DialectBigQuery:   "ARRAY(SELECT (elem + 1) FROM UNNEST(unknown.arr) AS elem)",
				DialectSpanner:    "ARRAY(SELECT (elem + 1) FROM UNNEST(unknown.arr) AS elem)",
				DialectPostgreSQL: "ARRAY(SELECT (elem + 1) FROM UNNEST(unknown.arr) AS elem)",
				DialectDuckDB:     "ARRAY(SELECT (elem + 1) FROM UNNEST(unknown.arr) AS elem)",
				DialectClickHouse: "arrayMap(elem -> (elem + 1), unknown.arr)",
			},
		},
		{
			name:          "non-array field rejected with schema",
			logic:         `{"map":[{"var":"metrics.amount"},{"+":[{"var":""},1]}]}`,
			wantSchemaErr: "array operation on non-array field",
			wantNoSchema: map[Dialect]string{
				DialectBigQuery:   "ARRAY(SELECT (elem + 1) FROM UNNEST(metrics.amount) AS elem)",
				DialectSpanner:    "ARRAY(SELECT (elem + 1) FROM UNNEST(metrics.amount) AS elem)",
				DialectPostgreSQL: "ARRAY(SELECT (elem + 1) FROM UNNEST(metrics.amount) AS elem)",
				DialectDuckDB:     "ARRAY(SELECT (elem + 1) FROM UNNEST(metrics.amount) AS elem)",
				DialectClickHouse: "arrayMap(elem -> (elem + 1), metrics.amount)",
			},
		},
	}

	for _, d := range dialects {
		t.Run(d.String(), func(t *testing.T) {
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					_, err := schemaTrByDialect[d].TranspileValue(tc.logic)
					if err == nil || !strings.Contains(err.Error(), tc.wantSchemaErr) {
						t.Fatalf("expected schema error containing %q, got: %v", tc.wantSchemaErr, err)
					}

					// No-schema mode should accept and produce SQL shape.
					sql, err := noSchemaTrByDialect[d].TranspileValue(tc.logic)
					if err != nil {
						t.Fatalf("no-schema mode should pass, got error: %v", err)
					}
					if sql != tc.wantNoSchema[d] {
						t.Fatalf("no-schema SQL = %q, want %q", sql, tc.wantNoSchema[d])
					}
				})
			}
		})
	}
}

func TestArrayEdgeMatrix_PackageFunctionsSmoke(t *testing.T) {
	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}
	logic := `{"map":[{"var":"bag.numbers"},{"*":[{"var":""},2]}]}`
	logicMap := decodeLogicMap(t, logic)
	logicAny := decodeLogicAny(t, logic)

	for _, d := range dialects {
		t.Run(d.String(), func(t *testing.T) {
			sql1, err := TranspileValue(d, logic)
			if err != nil {
				t.Fatalf("TranspileValue() error: %v", err)
			}
			sql2, err := TranspileValueFromMap(d, logicMap)
			if err != nil {
				t.Fatalf("TranspileValueFromMap() error: %v", err)
			}
			sql3, err := TranspileValueFromInterface(d, logicAny)
			if err != nil {
				t.Fatalf("TranspileValueFromInterface() error: %v", err)
			}
			cond, err := TranspileValue(d, logic)
			if err != nil {
				t.Fatalf("TranspileValue() error: %v", err)
			}
			if sql1 != sql2 || sql1 != sql3 {
				t.Fatalf("package transpile mismatch: direct=%q map=%q any=%q", sql1, sql2, sql3)
			}
			if sql1 != cond {
				t.Fatalf("package condition mismatch: sql=%q cond=%q", sql1, cond)
			}

			psql, params, err := TranspileParameterizedValue(d, logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error: %v", err)
			}
			if len(params) != 1 {
				t.Fatalf("expected 1 param, got %d", len(params))
			}
			pcond, cparams, err := TranspileParameterizedValue(d, logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error: %v", err)
			}
			if psql != pcond {
				t.Fatalf("package parameterized condition mismatch: sql=%q cond=%q", psql, pcond)
			}
			if len(cparams) != len(params) {
				t.Fatalf("package parameterized param len mismatch: cond=%d sql=%d", len(cparams), len(params))
			}
			assertPlaceholderStyle(t, d, psql, 1)
		})
	}
}

func BenchmarkArrayEdgeMatrix_DeepNesting(b *testing.B) {
	logic := `{"and":[{"some":[{"map":[{"var":"bag.records"},{"filter":[{"var":"values"},{">=":[{"var":""},0]}]}]},{"all":[{"var":""},{">=":[{"var":""},0]}]}]},{">=":[{"var":"metrics.amount"},100]}]}`
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  matrixSchema(),
	})
	if err != nil {
		b.Fatalf("failed to init transpiler: %v", err)
	}
	for i := 0; i < b.N; i++ {
		if _, err := tr.TranspileCondition(logic); err != nil {
			b.Fatalf("TranspileCondition() error: %v", err)
		}
	}
}

func TestArrayEdgeMatrix_ErrorMessagesAreStable(t *testing.T) {
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  matrixSchema(),
	})
	if err != nil {
		t.Fatalf("failed to init transpiler: %v", err)
	}
	_, err = tr.TranspileCondition(`{"reduce":[{"var":"bag.numbers"},{"var":"current"}]}`)
	if err == nil {
		t.Fatal("expected reduce arity error")
	}
	if !strings.Contains(err.Error(), "reduce operator requires at least 3 arguments") {
		t.Fatalf("unexpected reduce arity error: %v", err)
	}
}

func TestArrayEdgeMatrix_NoPanicOnComplexInputs(t *testing.T) {
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  matrixSchema(),
	})
	if err != nil {
		t.Fatalf("failed to init transpiler: %v", err)
	}

	inputs := []string{
		`{"map":[{"var":"bag.records"},{"map":[{"var":"values"},{"if":[{">":[{"var":""},10]},{"var":""},0]}]}]}`,
		`{"filter":[{"map":[{"var":"bag.records"},{"reduce":[{"var":"values"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"base"}]}]},{">":[{"var":""},0]}]}`,
		`{"all":[{"filter":[{"var":"bag.numbers"},{">":[{"var":""},0]}]},{">":[{"var":""},0]}]}`,
	}

	for i, logic := range inputs {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			if strings.HasPrefix(logic, `{"all"`) {
				if _, err := tr.TranspileCondition(logic); err != nil {
					t.Fatalf("TranspileCondition() failed for complex input: %v", err)
				}
				if _, _, err := tr.TranspileParameterizedCondition(logic); err != nil {
					t.Fatalf("TranspileParameterizedCondition() failed for complex input: %v", err)
				}
				return
			}
			if _, err := tr.TranspileValue(logic); err != nil {
				t.Fatalf("TranspileValue() failed for complex input: %v", err)
			}
			if _, _, err := tr.TranspileParameterizedValue(logic); err != nil {
				t.Fatalf("TranspileParameterizedValue() failed for complex input: %v", err)
			}
		})
	}
}
