package jsonlogic2sql

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRegressionMatrix_DataAccess_AllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "profile.name", Type: FieldTypeString},
		{Name: "profile.nick", Type: FieldTypeString},
		{Name: "profile.email", Type: FieldTypeString},
		{Name: "profile.phone", Type: FieldTypeString},
		{Name: "profile.city", Type: FieldTypeString},
		{Name: "flags.active", Type: FieldTypeBoolean},
	})

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	type dataCase struct {
		name                 string
		logic                string
		expectSchemaAwareErr bool
		expectSchemaLessErr  bool
	}

	cases := []dataCase{
		{name: "var_simple", logic: `{"==":[{"var":"profile.name"},"alice"]}`},
		{name: "var_default", logic: `{"eqGuest":[{"var":["profile.nick","guest"]}]}`},
		{name: "missing_single", logic: `{"missing":"profile.email"}`},
		{name: "missing_array", logic: `{"missing":["profile.email","profile.phone"]}`},
		{name: "missing_some_1", logic: `{"missing_some":[1,["profile.email","profile.phone"]]}`},
		{name: "missing_some_2", logic: `{"missing_some":[2,["profile.email","profile.phone","profile.city"]]}`},
		{
			name:  "nested_builtin",
			logic: `{"and":[{"!":{"missing":"profile.email"}},{"or":[{"==":[{"if":[{"missing_some":[2,["profile.email","profile.phone","profile.city"]]},"NEED_DATA","OK"]},"NEED_DATA"]},{"==":[{"var":"flags.active"},true]}]}]}`,
		},
		{
			name:  "nested_custom",
			logic: `{"and":[{"isPresent":[{"var":"profile.name"}]},{"eqGuest":[{"var":["profile.nick","guest"]}]},{"!":{"missing_some":[2,["profile.email","profile.phone","profile.city"]]}}]}`,
		},
		{name: "unknown_field", logic: `{"missing":"unknown.field"}`, expectSchemaAwareErr: true},
	}

	modes := []struct {
		name   string
		schema *Schema
		aware  bool
	}{
		{name: "schema-required", schema: schema, aware: true},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()

			for _, d := range dialects {
				t.Run(d.String(), func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error: %v", err)
					}
					registerDataAccessMatrixCustomOps(t, tr)

					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							// schema-required/schema-required expected error paths
							if mode.aware && tc.expectSchemaAwareErr {
								assertDataAccessAllErrorPaths(t, tr, tc.logic)
								return
							}
							if !mode.aware && tc.expectSchemaLessErr {
								assertDataAccessAllErrorPaths(t, tr, tc.logic)
								return
							}

							sql, err := tr.TranspileCondition(tc.logic)
							if err != nil {
								t.Fatalf("TranspileCondition() error: %v", err)
							}
							cond := sql
							if strings.TrimSpace(cond) == "" || strings.HasPrefix(cond, "WHERE ") {
								t.Fatalf("TranspileCondition() returned invalid condition SQL: %q", cond)
							}

							psql, params, err := tr.TranspileParameterizedCondition(tc.logic)
							if err != nil {
								t.Fatalf("TranspileParameterizedCondition() error: %v", err)
							}
							pcond := psql
							if strings.TrimSpace(pcond) == "" || strings.HasPrefix(pcond, "WHERE ") {
								t.Fatalf("TranspileParameterizedCondition() returned invalid condition SQL: %q", pcond)
							}

							logicMap := parseDataAccessLogicMap(t, tc.logic)
							pFromMap, mapParams, err := tr.TranspileParameterizedConditionFromMap(logicMap)
							if err != nil {
								t.Fatalf("TranspileParameterizedConditionFromMap() error: %v", err)
							}
							pFromAny, anyParams, err := tr.TranspileParameterizedConditionFromInterface(logicMap)
							if err != nil {
								t.Fatalf("TranspileParameterizedConditionFromInterface() error: %v", err)
							}
							if pFromMap != psql || pFromAny != psql {
								t.Fatalf("param SQL mismatch map/interface variants:\npsql=%q\nfromMap=%q\nfromAny=%q", psql, pFromMap, pFromAny)
							}
							if !reflect.DeepEqual(mapParams, params) || !reflect.DeepEqual(anyParams, params) {
								t.Fatalf("param value mismatch map/interface variants:\nparams=%#v\nmap=%#v\nany=%#v", params, mapParams, anyParams)
							}

							pcFromMap, mapCondParams, err := tr.TranspileParameterizedConditionFromMap(logicMap)
							if err != nil {
								t.Fatalf("TranspileParameterizedConditionFromMap() error: %v", err)
							}
							pcFromAny, anyCondParams, err := tr.TranspileParameterizedConditionFromInterface(logicMap)
							if err != nil {
								t.Fatalf("TranspileParameterizedConditionFromInterface() error: %v", err)
							}
							if pcFromMap != pcond || pcFromAny != pcond {
								t.Fatalf("param condition mismatch map/interface variants:\npcond=%q\nfromMap=%q\nfromAny=%q", pcond, pcFromMap, pcFromAny)
							}
							if !reflect.DeepEqual(mapCondParams, params) || !reflect.DeepEqual(anyCondParams, params) {
								t.Fatalf("param condition values mismatch map/interface variants:\nparams=%#v\nmap=%#v\nany=%#v", params, mapCondParams, anyCondParams)
							}

							validateDataAccessCase(t, tc.name, d, mode.aware, cond, pcond, params)
						})
					}
				})
			}
		})
	}
}

func registerDataAccessMatrixCustomOps(t *testing.T, tr *Transpiler) {
	t.Helper()

	mustRegister := func(name string, f OperatorFunc) {
		t.Helper()
		if err := tr.RegisterOperatorFunc(name, f); err != nil {
			t.Fatalf("RegisterOperatorFunc(%q) error: %v", name, err)
		}
	}

	mustRegister("isPresent", func(_ string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 1 {
			return OperatorResult{}, fmt.Errorf("isPresent expects 1 arg")
		}
		return PredicateSQL(fmt.Sprintf("(%s IS NOT NULL)", args[0].SQL)), nil
	})

	mustRegister("eqGuest", func(_ string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 1 {
			return OperatorResult{}, fmt.Errorf("eqGuest expects 1 arg")
		}
		return PredicateSQL(fmt.Sprintf("(%s = 'guest')", args[0].SQL)), nil
	})
}

func assertDataAccessAllErrorPaths(t *testing.T, tr *Transpiler, logic string) {
	t.Helper()

	logicMap := parseDataAccessLogicMap(t, logic)

	if _, err := tr.TranspileCondition(logic); err == nil {
		t.Fatal("expected TranspileCondition() error, got nil")
	}
	if _, err := tr.TranspileCondition(logic); err == nil {
		t.Fatal("expected TranspileCondition() error, got nil")
	}
	if _, _, err := tr.TranspileParameterizedCondition(logic); err == nil {
		t.Fatal("expected TranspileParameterizedCondition() error, got nil")
	}
	if _, _, err := tr.TranspileParameterizedCondition(logic); err == nil {
		t.Fatal("expected TranspileParameterizedCondition() error, got nil")
	}
	if _, _, err := tr.TranspileParameterizedConditionFromMap(logicMap); err == nil {
		t.Fatal("expected TranspileParameterizedConditionFromMap() error, got nil")
	}
	if _, _, err := tr.TranspileParameterizedConditionFromInterface(logicMap); err == nil {
		t.Fatal("expected TranspileParameterizedConditionFromInterface() error, got nil")
	}
	if _, _, err := tr.TranspileParameterizedConditionFromMap(logicMap); err == nil {
		t.Fatal("expected TranspileParameterizedConditionFromMap() error, got nil")
	}
	if _, _, err := tr.TranspileParameterizedConditionFromInterface(logicMap); err == nil {
		t.Fatal("expected TranspileParameterizedConditionFromInterface() error, got nil")
	}
}

func validateDataAccessCase(
	t *testing.T,
	name string,
	d Dialect,
	schemaAware bool,
	cond string,
	pcond string,
	params []QueryParam,
) {
	t.Helper()

	requireContains := func(parts ...string) {
		t.Helper()
		for _, p := range parts {
			if !strings.Contains(cond, p) {
				t.Fatalf("missing SQL fragment %q in %q", p, cond)
			}
		}
	}

	switch name {
	case "var_simple":
		requireContains("profile.name = 'alice'")
		if len(params) != 1 {
			t.Fatalf("expected 1 param, got %d: %#v", len(params), params)
		}
		if params[0].Value != "alice" {
			t.Fatalf("expected param value alice, got %v", params[0].Value)
		}

	case "var_default":
		requireContains("COALESCE(profile.nick, 'guest') = 'guest'")
		if len(params) != 1 {
			t.Fatalf("expected 1 param, got %d: %#v", len(params), params)
		}
		if params[0].Value != "guest" {
			t.Fatalf("expected param value guest, got %v", params[0].Value)
		}

	case "missing_single":
		if cond != "profile.email IS NULL" {
			t.Fatalf("unexpected SQL for missing_single: %q", cond)
		}
		if len(params) != 0 {
			t.Fatalf("missing should not allocate params: %#v", params)
		}

	case "missing_array":
		requireContains("profile.email IS NULL", "profile.phone IS NULL", " OR ")
		if len(params) != 0 {
			t.Fatalf("missing array should not allocate params: %#v", params)
		}

	case "missing_some_1":
		requireContains("profile.email IS NULL", "profile.phone IS NULL", " OR ")
		if len(params) != 0 {
			t.Fatalf("missing_some min1 should not allocate params: %#v", params)
		}

	case "missing_some_2":
		requireContains(
			"CASE WHEN profile.email IS NULL THEN 1 ELSE 0 END",
			"CASE WHEN profile.phone IS NULL THEN 1 ELSE 0 END",
			"CASE WHEN profile.city IS NULL THEN 1 ELSE 0 END",
			") >= 2",
		)
		if len(params) != 1 {
			t.Fatalf("expected 1 param for missing_some min2, got %d: %#v", len(params), params)
		}
		if !strings.Contains(pcond, firstDataAccessPlaceholder(d)) {
			t.Fatalf("expected placeholder %q in %q", firstDataAccessPlaceholder(d), pcond)
		}

	case "nested_builtin":
		requireContains(
			"NOT (profile.email IS NULL)",
			"CASE WHEN",
			"THEN 'NEED_DATA' ELSE 'OK' END = 'NEED_DATA'",
			"flags.active = TRUE",
		)
		if len(params) == 0 {
			t.Fatalf("expected params for nested_builtin")
		}

	case "nested_custom":
		requireContains(
			"profile.name IS NOT NULL",
			"COALESCE(profile.nick, 'guest') = 'guest'",
			"NOT (",
			"CASE WHEN profile.email IS NULL THEN 1 ELSE 0 END",
		)
		if len(params) == 0 {
			t.Fatalf("expected params for nested_custom")
		}

	case "unknown_field":
		if schemaAware {
			t.Fatalf("unknown_field should have errored in schema-required mode")
		}
		if cond != "unknown.field IS NULL" {
			t.Fatalf("unexpected schema-required SQL for unknown_field: %q", cond)
		}
	}
}

func parseDataAccessLogicMap(t *testing.T, logic string) map[string]interface{} {
	t.Helper()

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(logic), &m); err != nil {
		t.Fatalf("json.Unmarshal() error for %q: %v", logic, err)
	}
	return m
}

func firstDataAccessPlaceholder(d Dialect) string {
	if d == DialectPostgreSQL || d == DialectDuckDB {
		return "$1"
	}
	return "@p1"
}
