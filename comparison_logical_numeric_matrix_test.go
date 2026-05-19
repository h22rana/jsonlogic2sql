package jsonlogic2sql

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRegressionMatrix_ComparisonLogicalNumeric_AllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "profile.name", Type: FieldTypeString},
		{Name: "profile.age", Type: FieldTypeNumber},
		{Name: "profile.score", Type: FieldTypeNumber},
		{Name: "profile.tags", Type: FieldTypeArray},
		{Name: "flags.active", Type: FieldTypeBoolean},
	})

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	type regressionCase struct {
		name              string
		logic             string
		valueRoot         bool
		schemaRequiredErr ErrorCode
	}

	cases := []regressionCase{
		{
			name:              "builtin_deep_mix",
			logic:             `{"and":[{">=":[{"+":[{"var":"profile.age"},5]},21]},{"<":[{"var":"profile.score"},{"max":[80,{"*":[2,10]}]}]},{"!==":[{"var":"profile.name"},null]},{"==":[{"if":[{"!!":[{"var":"flags.active"}]},"Y","N"]},"Y"]}]}`,
			schemaRequiredErr: ErrInvalidExpressionContext,
		},
		{
			name:  "chained_compare",
			logic: `{"<":[10,{"var":"profile.age"},30]}`,
		},
		{
			name:      "numeric_max_min",
			logic:     `{"-":[{"max":[{"var":"profile.score"},50,75]},{"min":[10,{"var":"profile.age"},20]}]}`,
			valueRoot: true,
		},
		{
			name:  "custom_nested_mix",
			logic: `{"and":[{"between":[{"add2":[{"var":"profile.age"},2]},18,65]},{"==":[{"lower":[{"var":"profile.name"}]},"alice"]},{"!":{"<":[{"var":"profile.score"},40]}}]}`,
		},
		{
			name:      "custom_if_numeric",
			logic:     `{"if":[{"between":[{"var":"profile.score"},60,90]},{"+":[{"mul":[2,3]},{"var":"profile.age"}]},{"-":[{"var":"profile.age"},1]}]}`,
			valueRoot: true,
		},
		{
			name:  "schema_string_coercion",
			logic: `{"==":[{"var":"profile.name"},123]}`,
		},
	}

	modes := []struct {
		name        string
		schema      *Schema
		schemaAware bool
	}{
		{name: "schema-required", schema: schema, schemaAware: true},
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
					registerMatrixCustomOps(t, tr)

					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							var sql string
							var err error
							if tc.valueRoot {
								sql, err = tr.TranspileValue(tc.logic)
							} else {
								sql, err = tr.TranspileCondition(tc.logic)
							}
							if mode.schema == nil && tc.schemaRequiredErr != "" {
								if !IsErrorCode(err, tc.schemaRequiredErr) {
									t.Fatalf("transpile error = %v, want %s", err, tc.schemaRequiredErr)
								}
								logicMap := parseJSONLogicMap(t, tc.logic)
								if tc.valueRoot {
									_, _, err = tr.TranspileParameterizedValue(tc.logic)
									if !IsErrorCode(err, tc.schemaRequiredErr) {
										t.Fatalf("parameterized transpile error = %v, want %s", err, tc.schemaRequiredErr)
									}
									_, _, err = tr.TranspileParameterizedValueFromMap(logicMap)
									if !IsErrorCode(err, tc.schemaRequiredErr) {
										t.Fatalf("parameterized from map error = %v, want %s", err, tc.schemaRequiredErr)
									}
									_, _, err = tr.TranspileParameterizedValueFromInterface(logicMap)
									if !IsErrorCode(err, tc.schemaRequiredErr) {
										t.Fatalf("parameterized from interface error = %v, want %s", err, tc.schemaRequiredErr)
									}
								} else {
									_, _, err = tr.TranspileParameterizedCondition(tc.logic)
									if !IsErrorCode(err, tc.schemaRequiredErr) {
										t.Fatalf("parameterized transpile error = %v, want %s", err, tc.schemaRequiredErr)
									}
									_, _, err = tr.TranspileParameterizedConditionFromMap(logicMap)
									if !IsErrorCode(err, tc.schemaRequiredErr) {
										t.Fatalf("parameterized from map error = %v, want %s", err, tc.schemaRequiredErr)
									}
									_, _, err = tr.TranspileParameterizedConditionFromInterface(logicMap)
									if !IsErrorCode(err, tc.schemaRequiredErr) {
										t.Fatalf("parameterized from interface error = %v, want %s", err, tc.schemaRequiredErr)
									}
								}
								return
							}
							if err != nil {
								t.Fatalf("transpile error: %v", err)
							}

							var repeat string
							if !tc.valueRoot {
								repeat, err = tr.TranspileCondition(tc.logic)
							} else {
								repeat, err = tr.TranspileValue(tc.logic)
							}
							if err != nil {
								t.Fatalf("transpile repeat error: %v", err)
							}

							if sql != repeat {
								t.Fatalf("repeat transpile mismatch: sql=%q repeat=%q", sql, repeat)
							}

							var psql string
							var params []QueryParam
							if tc.valueRoot {
								psql, params, err = tr.TranspileParameterizedValue(tc.logic)
							} else {
								psql, params, err = tr.TranspileParameterizedCondition(tc.logic)
							}
							if err != nil {
								t.Fatalf("parameterized transpile error: %v", err)
							}
							var pcond string
							var cparams []QueryParam
							if tc.valueRoot {
								pcond, cparams, err = tr.TranspileParameterizedValue(tc.logic)
							} else {
								pcond, cparams, err = tr.TranspileParameterizedCondition(tc.logic)
							}
							if err != nil {
								t.Fatalf("parameterized transpile repeat error: %v", err)
							}
							if psql != pcond {
								t.Fatalf("param repeat transpile mismatch: psql=%q pcond=%q", psql, pcond)
							}
							if !reflect.DeepEqual(params, cparams) {
								t.Fatalf("param list mismatch between repeated parameterized transpiles:\nparams=%#v\ncparams=%#v", params, cparams)
							}

							logicMap := parseJSONLogicMap(t, tc.logic)

							var pFromMap string
							var mapParams []QueryParam
							if tc.valueRoot {
								pFromMap, mapParams, err = tr.TranspileParameterizedValueFromMap(logicMap)
							} else {
								pFromMap, mapParams, err = tr.TranspileParameterizedConditionFromMap(logicMap)
							}
							if err != nil {
								t.Fatalf("parameterized from map error: %v", err)
							}
							var pFromAny string
							var anyParams []QueryParam
							if tc.valueRoot {
								pFromAny, anyParams, err = tr.TranspileParameterizedValueFromInterface(logicMap)
							} else {
								pFromAny, anyParams, err = tr.TranspileParameterizedConditionFromInterface(logicMap)
							}
							if err != nil {
								t.Fatalf("parameterized from interface error: %v", err)
							}
							if pFromMap != psql || pFromAny != psql {
								t.Fatalf("param SQL mismatch map/interface variants:\npsql=%q\nfromMap=%q\nfromAny=%q", psql, pFromMap, pFromAny)
							}
							if !reflect.DeepEqual(mapParams, params) || !reflect.DeepEqual(anyParams, params) {
								t.Fatalf("param value mismatch map/interface variants:\nparams=%#v\nmap=%#v\nany=%#v", params, mapParams, anyParams)
							}

							var pcFromMap string
							var mapCondParams []QueryParam
							if tc.valueRoot {
								pcFromMap, mapCondParams, err = tr.TranspileParameterizedValueFromMap(logicMap)
							} else {
								pcFromMap, mapCondParams, err = tr.TranspileParameterizedConditionFromMap(logicMap)
							}
							if err != nil {
								t.Fatalf("parameterized repeat from map error: %v", err)
							}
							var pcFromAny string
							var anyCondParams []QueryParam
							if tc.valueRoot {
								pcFromAny, anyCondParams, err = tr.TranspileParameterizedValueFromInterface(logicMap)
							} else {
								pcFromAny, anyCondParams, err = tr.TranspileParameterizedConditionFromInterface(logicMap)
							}
							if err != nil {
								t.Fatalf("parameterized repeat from interface error: %v", err)
							}
							if pcFromMap != pcond || pcFromAny != pcond {
								t.Fatalf("param condition mismatch map/interface variants:\npcond=%q\nfromMap=%q\nfromAny=%q", pcond, pcFromMap, pcFromAny)
							}
							if !reflect.DeepEqual(mapCondParams, params) || !reflect.DeepEqual(anyCondParams, params) {
								t.Fatalf("param condition values mismatch map/interface variants:\nparams=%#v\nmap=%#v\nany=%#v", params, mapCondParams, anyCondParams)
							}

							validateComparisonLogicalNumericCase(t, tc.name, d, mode.schemaAware, sql, pcond, params)
						})
					}
				})
			}
		})
	}
}

func registerMatrixCustomOps(t *testing.T, tr *Transpiler) {
	t.Helper()

	mustRegister := func(name string, f OperatorFunc) {
		t.Helper()
		if err := tr.RegisterOperatorFunc(name, f); err != nil {
			t.Fatalf("RegisterOperatorFunc(%q) error: %v", name, err)
		}
	}

	mustRegister("add2", func(_ string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 2 {
			return OperatorResult{}, fmt.Errorf("add2 expects 2 args")
		}
		return ValueSQL(fmt.Sprintf("(%s + %s)", args[0].SQL, args[1].SQL), ExpressionTypeNumber), nil
	})

	mustRegister("between", func(_ string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 3 {
			return OperatorResult{}, fmt.Errorf("between expects 3 args")
		}
		return PredicateSQL(fmt.Sprintf("(%s >= %s AND %s <= %s)", args[0].SQL, args[1].SQL, args[0].SQL, args[2].SQL)), nil
	})

	mustRegister("lower", func(_ string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 1 {
			return OperatorResult{}, fmt.Errorf("lower expects 1 arg")
		}
		return ValueSQL(fmt.Sprintf("LOWER(%s)", args[0].SQL), ExpressionTypeString), nil
	})

	mustRegister("mul", func(_ string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 2 {
			return OperatorResult{}, fmt.Errorf("mul expects 2 args")
		}
		return ValueSQL(fmt.Sprintf("(%s * %s)", args[0].SQL, args[1].SQL), ExpressionTypeNumber), nil
	})
}

func validateComparisonLogicalNumericCase(
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
	case "builtin_deep_mix":
		requireContains(
			"(profile.age + 5)",
			"GREATEST(80, (2 * 10))",
			"profile.name IS NOT NULL",
			"CASE WHEN",
			"THEN 'Y' ELSE 'N' END = 'Y'",
		)
		if len(params) == 0 {
			t.Fatalf("expected parameterized output to contain params for %s", name)
		}
		if !strings.Contains(pcond, firstPlaceholder(d)) {
			t.Fatalf("expected placeholder %q in %q", firstPlaceholder(d), pcond)
		}

	case "chained_compare":
		requireContains("10 < profile.age", "profile.age < 30")
		if len(params) != 2 {
			t.Fatalf("expected 2 params for chained comparison, got %d: %#v", len(params), params)
		}

	case "numeric_max_min":
		requireContains("GREATEST(profile.score, 50, 75)", "LEAST(10, profile.age, 20)")

	case "custom_nested_mix":
		requireContains(
			"((profile.age + 2) >= 18 AND (profile.age + 2) <= 65)",
			"LOWER(profile.name)",
			"NOT (profile.score < 40)",
		)
		if len(params) == 0 {
			t.Fatalf("expected parameterized output to contain params for %s", name)
		}

	case "custom_if_numeric":
		requireContains(
			"CASE WHEN",
			"(profile.score >= 60 AND profile.score <= 90)",
			"((2 * 3) + profile.age)",
			"(profile.age - 1)",
		)

	case "schema_string_coercion":
		if len(params) != 1 {
			t.Fatalf("expected 1 param for schema_string_coercion, got %d: %#v", len(params), params)
		}

		if schemaAware {
			requireContains("profile.name = '123'")
			if _, ok := params[0].Value.(string); !ok {
				t.Fatalf("expected schema-required param type string, got %T (%v)", params[0].Value, params[0].Value)
			}
			return
		}

		requireContains("profile.name = 123")
		if _, ok := params[0].Value.(float64); !ok {
			t.Fatalf("expected schema-required param type float64, got %T (%v)", params[0].Value, params[0].Value)
		}
	}
}

func parseJSONLogicMap(t *testing.T, logic string) map[string]interface{} {
	t.Helper()

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(logic), &m); err != nil {
		t.Fatalf("json.Unmarshal() error for %q: %v", logic, err)
	}
	return m
}

func firstPlaceholder(d Dialect) string {
	if d == DialectPostgreSQL || d == DialectDuckDB {
		return "$1"
	}
	return "@p1"
}
