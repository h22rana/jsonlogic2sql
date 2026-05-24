package jsonlogic2sql

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func decodeLogicMapLocal(t *testing.T, logic string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(logic), &m); err != nil {
		t.Fatalf("json.Unmarshal(map) failed: %v", err)
	}
	return m
}

func decodeLogicAnyLocal(t *testing.T, logic string) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(logic), &v); err != nil {
		t.Fatalf("json.Unmarshal(interface) failed: %v", err)
	}
	return v
}

func assertAccumulatorFieldSQL(t *testing.T, sql string) {
	t.Helper()
	if !strings.Contains(sql, "elem.accumulator") {
		t.Fatalf("expected accumulator to resolve as an element field, got: %s", sql)
	}
}

func assertAllAPIsResolveAccumulatorAsElementField(t *testing.T, tr *Transpiler, logic string, valueMode bool) {
	t.Helper()

	m := decodeLogicMapLocal(t, logic)
	logicAny := decodeLogicAnyLocal(t, logic)

	var (
		sql string
		err error
	)
	if valueMode {
		sql, err = tr.TranspileValue(logic)
	} else {
		sql, err = tr.TranspileCondition(logic)
	}
	if err != nil {
		t.Fatalf("inline transpilation error: %v", err)
	}
	assertAccumulatorFieldSQL(t, sql)

	if valueMode {
		sql, err = tr.TranspileValueFromMap(m)
	} else {
		sql, err = tr.TranspileConditionFromMap(m)
	}
	if err != nil {
		t.Fatalf("map transpilation error: %v", err)
	}
	assertAccumulatorFieldSQL(t, sql)

	if valueMode {
		sql, err = tr.TranspileValueFromInterface(logicAny)
	} else {
		sql, err = tr.TranspileConditionFromInterface(logicAny)
	}
	if err != nil {
		t.Fatalf("interface transpilation error: %v", err)
	}
	assertAccumulatorFieldSQL(t, sql)

	if valueMode {
		sql, _, err = tr.TranspileParameterizedValue(logic)
	} else {
		sql, _, err = tr.TranspileParameterizedCondition(logic)
	}
	if err != nil {
		t.Fatalf("parameterized transpilation error: %v", err)
	}
	assertAccumulatorFieldSQL(t, sql)

	if valueMode {
		sql, _, err = tr.TranspileParameterizedValueFromMap(m)
	} else {
		sql, _, err = tr.TranspileParameterizedConditionFromMap(m)
	}
	if err != nil {
		t.Fatalf("parameterized map transpilation error: %v", err)
	}
	assertAccumulatorFieldSQL(t, sql)

	if valueMode {
		sql, _, err = tr.TranspileParameterizedValueFromInterface(logicAny)
	} else {
		sql, _, err = tr.TranspileParameterizedConditionFromInterface(logicAny)
	}
	if err != nil {
		t.Fatalf("parameterized interface transpilation error: %v", err)
	}
	assertAccumulatorFieldSQL(t, sql)
}

func TestAccumulatorOutsideReduceResolvesAsElementField_AllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{
			Name: "bag.numbers",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "accumulator", Type: FieldTypeNumber},
			},
		},
	})

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	cases := []struct {
		name      string
		logic     string
		valueMode bool
	}{
		{
			name:      "map expression",
			logic:     `{"map":[{"var":"bag.numbers"},{"+":[{"var":"accumulator"},1]}]}`,
			valueMode: true,
		},
		{
			name:      "filter predicate",
			logic:     `{"filter":[{"var":"bag.numbers"},{">":[{"var":"accumulator"},0]}]}`,
			valueMode: true,
		},
		{
			name:  "all predicate",
			logic: `{"all":[{"var":"bag.numbers"},{">=":[{"var":"accumulator"},0]}]}`,
		},
		{
			name:  "some predicate",
			logic: `{"some":[{"var":"bag.numbers"},{"==":[{"var":"accumulator"},1]}]}`,
		},
		{
			name:  "none predicate",
			logic: `{"none":[{"var":"bag.numbers"},{"==":[{"var":"accumulator"},0]}]}`,
		},
	}

	for _, d := range dialects {
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
					assertAllAPIsResolveAccumulatorAsElementField(t, tr, tc.logic, tc.valueMode)
				})
			}
		})
	}
}

func TestReduceAccumulatorStillWorks_AllDialects(t *testing.T) {
	t.Parallel()

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}
	logic := `{"reduce":[{"var":"bag.numbers"},{"+":[{"var":"accumulator"},{"var":"current"}]},0]}`

	accWord := regexp.MustCompile(`\baccumulator\b`)

	for _, d := range dialects {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error: %v", err)
			}

			sql, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error: %v", err)
			}
			if accWord.MatchString(sql) {
				t.Fatalf("unexpected bare accumulator in inline SQL: %s", sql)
			}

			psql, params, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error: %v", err)
			}
			if accWord.MatchString(psql) {
				t.Fatalf("unexpected bare accumulator in parameterized SQL: %s", psql)
			}
			if len(params) == 0 {
				t.Fatalf("expected parameterized reduce to emit params, got none")
			}
		})
	}
}
