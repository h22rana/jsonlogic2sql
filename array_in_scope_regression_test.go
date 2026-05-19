package jsonlogic2sql

import (
	"strings"
	"testing"
)

func TestTranspile_ArrayLambdaInUsesElementScopeRightOperand(t *testing.T) {
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

					inlineSQL, err := tr.TranspileValue(`{"filter":[{"var":"numbers"},{"in":[123,{"var":"name"}]}]}`)
					if err != nil {
						t.Fatalf("TranspileValue() error: %v", err)
					}
					assertArrayInMembershipSQL(t, d, inlineSQL, "123")

					paramSQL, params, err := tr.TranspileParameterizedValue(`{"filter":[{"var":"numbers"},{"in":[123,{"var":"name"}]}]}`)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error: %v", err)
					}
					if len(params) != 1 {
						t.Fatalf("param count = %d, want 1 (params=%v)", len(params), params)
					}
					assertArrayInMembershipSQL(t, d, paramSQL, firstPlaceholderForDialect(d))
				})
			}
		})
	}
}

func assertArrayInMembershipSQL(t *testing.T, d Dialect, sql, valueSQL string) {
	t.Helper()
	if !strings.Contains(sql, "elem.name") {
		t.Fatalf("expected element-scoped RHS elem.name, got: %s", sql)
	}
	switch d {
	case DialectPostgreSQL:
		assertSQLFragments(t, sql, []string{valueSQL + " = ANY(elem.name)"}, []string{" = ANY(name)"})
	case DialectDuckDB:
		assertSQLFragments(t, sql, []string{"list_contains(elem.name, " + valueSQL + ")"}, []string{"list_contains(name,"})
	case DialectClickHouse:
		assertSQLFragments(t, sql, []string{"has(elem.name, " + valueSQL + ")"}, []string{"has(name,"})
	default:
		assertSQLFragments(t, sql, []string{valueSQL + " IN UNNEST(elem.name)"}, []string{" IN UNNEST(name)"})
	}
}

func firstPlaceholderForDialect(d Dialect) string {
	switch d {
	case DialectPostgreSQL, DialectDuckDB:
		return "$1"
	default:
		return "@p1"
	}
}
