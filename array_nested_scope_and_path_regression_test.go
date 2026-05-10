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

func TestNestedArrayPredicatePreservesOuterScope_AllDialects(t *testing.T) {
	t.Parallel()

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	cases := []struct {
		name       string
		logic      string
		valueMode  bool
		want       string
		notWant    string
		clickWant  string
		clickAvoid string
	}{
		{
			name:       "all predicate",
			logic:      `{"all":[{"var":"bag.records"},{"some":[{"var":"item.values"},{">=":[{"var":"current"},{"var":"item.base"}]}]}]}`,
			want:       "UNNEST(elem.values) AS elem1",
			notWant:    "UNNEST(elem.values) AS elem WHERE elem >= elem.base",
			clickWant:  "arrayExists(elem1 -> elem1 >= elem.base, elem.values)",
			clickAvoid: "arrayExists(elem -> elem >= elem.base, elem.values)",
		},
		{
			name:       "all logical predicate",
			logic:      `{"all":[{"var":"bag.records"},{"and":[{"some":[{"var":"item.values"},{">=":[{"var":"current"},{"var":"item.base"}]}]},true]}]}`,
			want:       "UNNEST(elem.values) AS elem1",
			notWant:    "UNNEST(elem.values) AS elem WHERE elem >= elem.base",
			clickWant:  "arrayExists(elem1 -> elem1 >= elem.base, elem.values)",
			clickAvoid: "arrayExists(elem -> elem >= elem.base, elem.values)",
		},
		{
			name:       "filter predicate",
			logic:      `{"filter":[{"var":"bag.records"},{"some":[{"var":"item.values"},{">=":[{"var":"current"},{"var":"item.base"}]}]}]}`,
			valueMode:  true,
			want:       "UNNEST(elem.values) AS elem1",
			notWant:    "UNNEST(elem.values) AS elem WHERE elem >= elem.base",
			clickWant:  "arrayExists(elem1 -> elem1 >= elem.base, elem.values)",
			clickAvoid: "arrayExists(elem -> elem >= elem.base, elem.values)",
		},
	}

	for _, d := range dialects {
		for _, tc := range cases {
			t.Run(d.String()+"/"+tc.name, func(t *testing.T) {
				t.Parallel()

				tr, err := NewTranspiler(d)
				if err != nil {
					t.Fatalf("NewTranspiler() error: %v", err)
				}

				var sql string
				if tc.valueMode {
					sql, err = tr.TranspileValue(tc.logic)
				} else {
					sql, err = tr.TranspileCondition(tc.logic)
				}
				if err != nil {
					t.Fatalf("inline transpilation error: %v", err)
				}
				assertNestedArrayPredicateScope(t, d, sql, tc.want, tc.notWant, tc.clickWant, tc.clickAvoid)

				var paramSQL string
				if tc.valueMode {
					paramSQL, _, err = tr.TranspileParameterizedValue(tc.logic)
				} else {
					paramSQL, _, err = tr.TranspileParameterizedCondition(tc.logic)
				}
				if err != nil {
					t.Fatalf("parameterized transpilation error: %v", err)
				}
				assertNestedArrayPredicateScope(t, d, paramSQL, tc.want, tc.notWant, tc.clickWant, tc.clickAvoid)
			})
		}
	}
}

func assertNestedArrayPredicateScope(
	t *testing.T,
	d Dialect,
	sql string,
	want string,
	notWant string,
	clickWant string,
	clickAvoid string,
) {
	t.Helper()
	if d == DialectClickHouse {
		if !strings.Contains(sql, clickWant) {
			t.Fatalf("expected ClickHouse SQL to contain %q, got: %s", clickWant, sql)
		}
		if strings.Contains(sql, clickAvoid) {
			t.Fatalf("unexpected ClickHouse scope collapse %q in SQL: %s", clickAvoid, sql)
		}
		return
	}
	if !strings.Contains(sql, want) {
		t.Fatalf("expected SQL to contain %q, got: %s", want, sql)
	}
	if !strings.Contains(sql, "elem1 >= elem.base") {
		t.Fatalf("expected inner current with outer item reference, got: %s", sql)
	}
	if strings.Contains(sql, notWant) {
		t.Fatalf("unexpected nested scope collapse %q in SQL: %s", notWant, sql)
	}
}

func TestCustomOperatorPathInsideArrayContexts_InlineAndParam(t *testing.T) {
	t.Parallel()

	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error: %v", err)
	}
	tr.RegisterOperatorFunc("oops", func(_ string, _ []OperatorArg) (OperatorResult, error) {
		return OperatorResult{}, fmt.Errorf("boom")
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
