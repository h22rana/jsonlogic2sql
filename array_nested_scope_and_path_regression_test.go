package jsonlogic2sql

import (
	"fmt"
	"strings"
	"testing"
)

func TestNestedCurrentDottedUsesInnerAlias_AllDialects(t *testing.T) {
	t.Parallel()

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	logic := `{"map":[{"var":"groups"},{"filter":[{"var":"item.values"},{"and":[{"==":[{"var":"current"},1]},{">=":[{"var":"current.base"},0]}]}]}]}`

	for _, d := range dialects {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error: %v", err)
			}

			sql, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error: %v", err)
			}
			if strings.Contains(sql, "AND elem.base") {
				t.Fatalf("unexpected outer alias in inner current.* predicate: %s", sql)
			}
			if !strings.Contains(sql, "AND elem1.base") {
				t.Fatalf("expected inner alias for current.* in predicate, got: %s", sql)
			}

			psql, _, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error: %v", err)
			}
			if strings.Contains(psql, "AND elem.base") {
				t.Fatalf("unexpected outer alias in parameterized inner current.* predicate: %s", psql)
			}
			if !strings.Contains(psql, "AND elem1.base") {
				t.Fatalf("expected inner alias for current.* in parameterized predicate, got: %s", psql)
			}
		})
	}
}

func TestCustomOperatorPathInsideArrayContexts_InlineAndParam(t *testing.T) {
	t.Parallel()

	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error: %v", err)
	}
	tr.RegisterOperatorFunc("oops", func(_ string, _ []interface{}) (string, error) {
		return "", fmt.Errorf("boom")
	})

	cases := []struct {
		name     string
		logic    string
		wantPath string
	}{
		{
			name:     "direct custom operator in map transform",
			logic:    `{"map":[{"var":"bag.records"},{"oops":[{"var":"item"}]}]}`,
			wantPath: "$.map[1].oops",
		},
		{
			name:     "nested custom operator under logical in map transform",
			logic:    `{"map":[{"var":"bag.records"},{"and":[{"oops":[{"var":"item"}]},{">":[{"var":"current"},0]}]}]}`,
			wantPath: "$.map[1].and[0].oops",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := tr.TranspileValue(tc.logic)
			if err == nil {
				t.Fatalf("expected inline error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Fatalf("inline error missing expected path %q: %v", tc.wantPath, err)
			}

			_, _, err = tr.TranspileParameterizedValue(tc.logic)
			if err == nil {
				t.Fatalf("expected parameterized error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Fatalf("parameterized error missing expected path %q: %v", tc.wantPath, err)
			}
		})
	}
}
