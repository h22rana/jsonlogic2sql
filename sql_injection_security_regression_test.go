package jsonlogic2sql

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const injectionPayload = "x' OR 1=1; DROP TABLE users; --"

func securityRegressionSchema() *Schema {
	return mustNewSchema([]FieldSchema{
		{Name: "name", Type: FieldTypeString},
		{Name: "status", Type: FieldTypeString},
		{Name: "amount", Type: FieldTypeInteger},
		{Name: "tags", Type: FieldTypeArray},
		{
			Name: "items",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "name", Type: FieldTypeString},
				{Name: "amount", Type: FieldTypeNumber},
			},
		},
	})
}

func TestSQLInjectionSecurity_LiteralPayloadsEscapedOrParameterized_AllDialects(t *testing.T) {
	t.Parallel()

	escapedPayload := "x'' OR 1=1; DROP TABLE users; --"
	cases := []struct {
		name       string
		mode       string
		logic      string
		inlineWant string
		inlineBy   map[Dialect]string
		paramWant  map[Dialect]string
		paramsWant []QueryParam
		register   func(t *testing.T, tr *Transpiler)
	}{
		{
			name:       "condition equality escapes string literal",
			mode:       "condition",
			logic:      `{"==":[{"var":"name"},"` + injectionPayload + `"]}`,
			inlineWant: "name = '" + escapedPayload + "'",
			paramWant:  placeholderSQLByDialect("name = "),
			paramsWant: []QueryParam{{Name: "p1", Value: injectionPayload}},
		},
		{
			name:       "condition in array escapes string literal",
			mode:       "condition",
			logic:      `{"in":[{"var":"name"},["safe","` + injectionPayload + `"]]}`,
			inlineWant: "name IN ('safe', '" + escapedPayload + "')",
			paramWant: map[Dialect]string{
				DialectBigQuery:   "name IN (@p1, @p2)",
				DialectSpanner:    "name IN (@p1, @p2)",
				DialectPostgreSQL: "name IN ($1, $2)",
				DialectDuckDB:     "name IN ($1, $2)",
				DialectClickHouse: "name IN (@p1, @p2)",
			},
			paramsWant: []QueryParam{
				{Name: "p1", Value: "safe"},
				{Name: "p2", Value: injectionPayload},
			},
		},
		{
			name:       "value cat escapes string literal",
			mode:       "value",
			logic:      `{"cat":["prefix:", "` + injectionPayload + `", {"var":"name"}]}`,
			inlineWant: "CONCAT('prefix:', '" + escapedPayload + "', COALESCE(name, ''))",
			paramWant:  catPayloadSQLByDialect(),
			paramsWant: []QueryParam{
				{Name: "p1", Value: "prefix:"},
				{Name: "p2", Value: injectionPayload},
			},
		},
		{
			name:       "array scoped default escapes string literal",
			mode:       "value",
			logic:      `{"map":[{"var":"items"},{"var":["name","` + injectionPayload + `"]}]}`,
			inlineWant: "ARRAY(SELECT COALESCE(elem.name, '" + escapedPayload + "') FROM UNNEST(items) AS elem)",
			inlineBy: map[Dialect]string{
				DialectClickHouse: "arrayMap(elem -> COALESCE(elem.name, '" + escapedPayload + "'), items)",
			},
			paramWant: map[Dialect]string{
				DialectBigQuery:   "ARRAY(SELECT COALESCE(elem.name, @p1) FROM UNNEST(items) AS elem)",
				DialectSpanner:    "ARRAY(SELECT COALESCE(elem.name, @p1) FROM UNNEST(items) AS elem)",
				DialectPostgreSQL: "ARRAY(SELECT COALESCE(elem.name, $1) FROM UNNEST(items) AS elem)",
				DialectDuckDB:     "ARRAY(SELECT COALESCE(elem.name, $1) FROM UNNEST(items) AS elem)",
				DialectClickHouse: "arrayMap(elem -> COALESCE(elem.name, @p1), items)",
			},
			paramsWant: []QueryParam{{Name: "p1", Value: injectionPayload}},
		},
		{
			name:       "custom operator receives escaped literal SQL",
			mode:       "value",
			logic:      `{"safeWrap":["` + injectionPayload + `"]}`,
			inlineWant: "SAFE_WRAP('" + escapedPayload + "')",
			paramWant:  placeholderSQLByDialect("SAFE_WRAP(", ")"),
			paramsWant: []QueryParam{{Name: "p1", Value: injectionPayload}},
			register: func(t *testing.T, tr *Transpiler) {
				t.Helper()
				if err := tr.RegisterOperatorFunc("safeWrap", func(_ string, args []OperatorArg) (OperatorResult, error) {
					if len(args) != 1 {
						return OperatorResult{}, fmt.Errorf("safeWrap expects one argument")
					}
					return ValueSQL(fmt.Sprintf("SAFE_WRAP(%s)", args[0].SQL), args[0].Type), nil
				}); err != nil {
					t.Fatalf("RegisterOperatorFunc(safeWrap) error = %v", err)
				}
			},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					tr, err := NewTranspiler(d, securityRegressionSchema())
					if err != nil {
						t.Fatalf("NewTranspiler() error = %v", err)
					}
					if tc.register != nil {
						tc.register(t, tr)
					}

					var inlineSQL string
					if tc.mode == "condition" {
						inlineSQL, err = tr.TranspileCondition(tc.logic)
					} else {
						inlineSQL, err = tr.TranspileValue(tc.logic)
					}
					if err != nil {
						t.Fatalf("inline transpilation error = %v", err)
					}
					inlineWant := tc.inlineWant
					if byDialect := tc.inlineBy[d]; byDialect != "" {
						inlineWant = byDialect
					}
					if inlineSQL != inlineWant {
						t.Fatalf("inline SQL = %q, want %q", inlineSQL, inlineWant)
					}

					var paramSQL string
					var params []QueryParam
					if tc.mode == "condition" {
						paramSQL, params, err = tr.TranspileParameterizedCondition(tc.logic)
					} else {
						paramSQL, params, err = tr.TranspileParameterizedValue(tc.logic)
					}
					if err != nil {
						t.Fatalf("parameterized transpilation error = %v", err)
					}
					if want := tc.paramWant[d]; paramSQL != want {
						t.Fatalf("parameterized SQL = %q, want %q", paramSQL, want)
					}
					if strings.Contains(paramSQL, injectionPayload) {
						t.Fatalf("parameterized SQL leaked raw payload: %s", paramSQL)
					}
					if !reflect.DeepEqual(params, tc.paramsWant) {
						t.Fatalf("params = %#v, want %#v", params, tc.paramsWant)
					}
				})
			}
		})
	}
}

func TestSQLInjectionSecurity_FieldAndOperatorPayloadsRejected_AllDialects(t *testing.T) {
	t.Parallel()

	fieldPayloadCases := []struct {
		name  string
		mode  string
		logic string
	}{
		{
			name:  "root var statement payload",
			mode:  "condition",
			logic: `{"==":[{"var":"name;DROP_TABLE.users"}, "x"]}`,
		},
		{
			name:  "defaulted var statement payload",
			mode:  "value",
			logic: `{"var":["name;DROP_TABLE.users", "x"]}`,
		},
		{
			name:  "missing field statement payload",
			mode:  "condition",
			logic: `{"missing":["name;DROP_TABLE.users"]}`,
		},
		{
			name:  "array scoped field statement payload",
			mode:  "value",
			logic: `{"map":[{"var":"items"},{"var":"name;DROP_TABLE.users"}]}`,
		},
		{
			name:  "malformed multi-key object cannot smuggle extra expression",
			mode:  "condition",
			logic: `{"==":[{"var":"name","extra":{"var":"amount"}}, "x"]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, securityRegressionSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tc := range fieldPayloadCases {
				t.Run(tc.name, func(t *testing.T) {
					var sql string
					if tc.mode == "condition" {
						sql, err = tr.TranspileCondition(tc.logic)
					} else {
						sql, err = tr.TranspileValue(tc.logic)
					}
					assertSecurityRejection(t, sql, err)

					var paramSQL string
					if tc.mode == "condition" {
						paramSQL, _, err = tr.TranspileParameterizedCondition(tc.logic)
					} else {
						paramSQL, _, err = tr.TranspileParameterizedValue(tc.logic)
					}
					assertSecurityRejection(t, paramSQL, err)
				})
			}
		})
	}

	t.Run("custom operator names reject injection syntax", func(t *testing.T) {
		tr, err := NewTranspiler(DialectBigQuery, securityRegressionSchema())
		if err != nil {
			t.Fatalf("NewTranspiler() error = %v", err)
		}
		names := []string{
			"evil;DROP",
			"evil--comment",
			"evil/*comment*/",
			"evil)OR(TRUE",
			"evil.name",
			"evil name",
		}
		for _, name := range names {
			err := tr.RegisterOperatorFunc(name, func(_ string, _ []OperatorArg) (OperatorResult, error) {
				return PredicateSQL("TRUE"), nil
			})
			if err == nil {
				t.Fatalf("RegisterOperatorFunc(%q) expected validation error, got nil", name)
			}
		}
	})
}

func placeholderSQLByDialect(parts ...string) map[Dialect]string {
	if len(parts) == 0 {
		parts = []string{"", ""}
	}
	if len(parts) == 1 {
		parts = append(parts, "")
	}
	return map[Dialect]string{
		DialectBigQuery:   parts[0] + "@p1" + strings.Join(parts[1:], ""),
		DialectSpanner:    parts[0] + "@p1" + strings.Join(parts[1:], ""),
		DialectPostgreSQL: parts[0] + "$1" + strings.Join(parts[1:], ""),
		DialectDuckDB:     parts[0] + "$1" + strings.Join(parts[1:], ""),
		DialectClickHouse: parts[0] + "@p1" + strings.Join(parts[1:], ""),
	}
}

func catPayloadSQLByDialect() map[Dialect]string {
	return map[Dialect]string{
		DialectBigQuery:   "CONCAT(@p1, @p2, COALESCE(name, ''))",
		DialectSpanner:    "CONCAT(@p1, @p2, COALESCE(name, ''))",
		DialectPostgreSQL: "CONCAT($1, $2, COALESCE(name, ''))",
		DialectDuckDB:     "CONCAT($1, $2, COALESCE(name, ''))",
		DialectClickHouse: "CONCAT(@p1, @p2, COALESCE(name, ''))",
	}
}

func assertSecurityRejection(t *testing.T, sql string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected security validation error, got SQL: %s", sql)
	}
	if strings.Contains(sql, "DROP") || strings.Contains(sql, "OR 1=1") || strings.Contains(sql, "--") {
		t.Fatalf("rejected expression still returned suspicious SQL %q with error %v", sql, err)
	}
}
