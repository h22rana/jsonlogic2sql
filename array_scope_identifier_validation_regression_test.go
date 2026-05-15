package jsonlogic2sql

import (
	"encoding/json"
	"strings"
	"testing"
)

func decodeLogicMapForArrayScopeIDTest(t *testing.T, logic string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(logic), &m); err != nil {
		t.Fatalf("json.Unmarshal(map) failed: %v", err)
	}
	return m
}

func decodeLogicAnyForArrayScopeIDTest(t *testing.T, logic string) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(logic), &v); err != nil {
		t.Fatalf("json.Unmarshal(any) failed: %v", err)
	}
	return v
}

func assertUnsafeVarRejected(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected rejection error, got nil")
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "invalid identifier") ||
		strings.Contains(msg, "not defined in schema") ||
		strings.Contains(msg, "unsupported array-scope variable") {
		return
	}
	t.Fatalf("expected invalid identifier, schema validation, or unsupported array-scope variable error, got: %v", err)
}

func TestArrayScopeIdentifierValidationRejectsMaliciousPaths_AllDialects(t *testing.T) {
	t.Parallel()

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	cases := []struct {
		name  string
		logic string
	}{
		{
			name:  "map rewritten item dotted payload",
			logic: `{"map":[{"var":"bag.numbers"},{"var":"item.x) OR 1=1 --"}]}`,
		},
		{
			name:  "filter rewritten current dotted payload",
			logic: `{"filter":[{"var":"bag.numbers"},{">=":[{"var":"current.x) OR 1=1 --"},0]}]}`,
		},
		{
			name:  "map direct elem dotted payload",
			logic: `{"map":[{"var":"bag.numbers"},{"var":"elem.x) OR 1=1 --"}]}`,
		},
		{
			name:  "map source internal elem dotted payload",
			logic: `{"map":[{"var":"elem.x) OR 1=1 --"},{"var":""}]}`,
		},
		{
			name:  "reduce initial internal elem dotted payload",
			logic: `{"reduce":[{"var":"bag.numbers"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"elem.x) OR 1=1 --"}]}`,
		},
		{
			name:  "map source internal elem dotted payload with default array-form var",
			logic: `{"map":[{"var":["elem.x) OR 1=1 --",[]]},{"var":""}]}`,
		},
	}

	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-less", schema: nil},
		{name: "schema-aware", schema: mustNewSchema([]FieldSchema{{Name: "bag.numbers", Type: FieldTypeArray}})},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()
			for _, d := range dialects {
				t.Run(d.String(), func(t *testing.T) {
					t.Parallel()
					tr, err := NewTranspilerWithConfig(&TranspilerConfig{Dialect: d, Schema: mode.schema})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error: %v", err)
					}

					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							m := decodeLogicMapForArrayScopeIDTest(t, tc.logic)
							logicAny := decodeLogicAnyForArrayScopeIDTest(t, tc.logic)

							sql, err := tr.TranspileValue(tc.logic)
							assertUnsafeVarRejected(t, err)
							if strings.Contains(sql, "OR 1=1") {
								t.Fatalf("unexpected injectable SQL emitted: %s", sql)
							}

							psql, _, err := tr.TranspileParameterizedValue(tc.logic)
							assertUnsafeVarRejected(t, err)
							if strings.Contains(psql, "OR 1=1") {
								t.Fatalf("unexpected injectable SQL emitted in parameterized mode: %s", psql)
							}

							_, err = tr.TranspileValueFromMap(m)
							assertUnsafeVarRejected(t, err)

							_, err = tr.TranspileValueFromInterface(logicAny)
							assertUnsafeVarRejected(t, err)

							_, _, err = tr.TranspileParameterizedValueFromMap(m)
							assertUnsafeVarRejected(t, err)

							_, _, err = tr.TranspileParameterizedValueFromInterface(logicAny)
							assertUnsafeVarRejected(t, err)
						})
					}
				})
			}
		})
	}
}

func TestArrayScopeIdentifierValidationAllowsSafeDottedPaths(t *testing.T) {
	t.Parallel()

	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error: %v", err)
	}
	logic := `{"map":[{"var":"bag.numbers"},{"var":"safe_field"}]}`

	sql, err := tr.TranspileValue(logic)
	if err != nil {
		t.Fatalf("unexpected inline error: %v", err)
	}
	if !strings.Contains(sql, "elem.safe_field") {
		t.Fatalf("expected rewritten safe element path, got: %s", sql)
	}

	psql, _, err := tr.TranspileParameterizedValue(logic)
	if err != nil {
		t.Fatalf("unexpected parameterized error: %v", err)
	}
	if !strings.Contains(psql, "elem.safe_field") {
		t.Fatalf("expected rewritten safe element path in parameterized SQL, got: %s", psql)
	}
}

func TestRootArrayOperandDoesNotTreatElemAsInScopeAlias_WithSchema(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{{Name: "bag.numbers", Type: FieldTypeArray}})
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{Dialect: DialectBigQuery, Schema: schema})
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() error: %v", err)
	}

	logic := `{"map":[{"var":"elem.values"},{"var":""}]}`
	logicMap := decodeLogicMapForArrayScopeIDTest(t, logic)
	logicAny := decodeLogicAnyForArrayScopeIDTest(t, logic)

	sql, err := tr.TranspileValue(logic)
	if err == nil {
		t.Fatalf("expected schema validation error, got SQL: %s", sql)
	}
	if strings.Contains(sql, "UNNEST(elem.values)") {
		t.Fatalf("unexpected unbound elem alias SQL emitted: %s", sql)
	}

	psql, _, err := tr.TranspileParameterizedValue(logic)
	if err == nil {
		t.Fatalf("expected schema validation error (param), got SQL: %s", psql)
	}
	if strings.Contains(psql, "UNNEST(elem.values)") {
		t.Fatalf("unexpected unbound elem alias SQL emitted (param): %s", psql)
	}

	_, err = tr.TranspileValueFromMap(logicMap)
	if err == nil {
		t.Fatal("expected schema validation error from map input")
	}

	_, err = tr.TranspileValueFromInterface(logicAny)
	if err == nil {
		t.Fatal("expected schema validation error from interface input")
	}

	_, _, err = tr.TranspileParameterizedValueFromMap(logicMap)
	if err == nil {
		t.Fatal("expected schema validation error from parameterized map input")
	}

	_, _, err = tr.TranspileParameterizedValueFromInterface(logicAny)
	if err == nil {
		t.Fatal("expected schema validation error from parameterized interface input")
	}
}

func TestNestedArrayOperandUsesBareOuterFieldsInChildScope(t *testing.T) {
	t.Parallel()

	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error: %v", err)
	}

	logic := `{"map":[{"var":"groups"},{"reduce":[{"var":"values"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"base"}]}]}`

	sql, err := tr.TranspileValue(logic)
	if err != nil {
		t.Fatalf("unexpected inline error: %v", err)
	}
	if !strings.Contains(sql, "UNNEST(elem.values)") {
		t.Fatalf("expected nested outer elem alias in array operand, got: %s", sql)
	}

	psql, _, err := tr.TranspileParameterizedValue(logic)
	if err != nil {
		t.Fatalf("unexpected parameterized error: %v", err)
	}
	if !strings.Contains(psql, "UNNEST(elem.values)") {
		t.Fatalf("expected nested outer elem alias in parameterized array operand, got: %s", psql)
	}
}
