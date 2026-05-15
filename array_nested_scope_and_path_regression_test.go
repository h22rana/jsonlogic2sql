package jsonlogic2sql

import (
	"fmt"
	"strings"
	"testing"
)

func TestNestedBareFieldUsesInnerAlias_AllDialects(t *testing.T) {
	t.Parallel()

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	logic := `{"map":[{"var":"groups"},{"filter":[{"var":"values"},{"and":[{"==":[{"var":""},1]},{">=":[{"var":"base"},0]}]}]}]}`

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
				t.Fatalf("unexpected outer alias in inner bare-field predicate: %s", sql)
			}
			if !strings.Contains(sql, "AND elem1.base") {
				t.Fatalf("expected inner alias for bare field in predicate, got: %s", sql)
			}

			psql, _, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error: %v", err)
			}
			if strings.Contains(psql, "AND elem.base") {
				t.Fatalf("unexpected outer alias in parameterized inner bare-field predicate: %s", psql)
			}
			if !strings.Contains(psql, "AND elem1.base") {
				t.Fatalf("expected inner alias for bare field in parameterized predicate, got: %s", psql)
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
			logic:      `{"all":[{"var":"bag.records"},{"some":[{"var":"values"},{">=":[{"var":""},0]}]}]}`,
			want:       "UNNEST(elem.values) AS elem1",
			notWant:    "UNNEST(elem.values) AS elem WHERE elem >= 0",
			clickWant:  "arrayExists(elem1 -> elem1 >=",
			clickAvoid: "arrayExists(elem -> elem >=",
		},
		{
			name:       "all logical predicate",
			logic:      `{"all":[{"var":"bag.records"},{"and":[{"some":[{"var":"values"},{">=":[{"var":""},0]}]},true]}]}`,
			want:       "UNNEST(elem.values) AS elem1",
			notWant:    "UNNEST(elem.values) AS elem WHERE elem >= 0",
			clickWant:  "arrayExists(elem1 -> elem1 >=",
			clickAvoid: "arrayExists(elem -> elem >=",
		},
		{
			name:       "filter predicate",
			logic:      `{"filter":[{"var":"bag.records"},{"some":[{"var":"values"},{">=":[{"var":""},0]}]}]}`,
			valueMode:  true,
			want:       "UNNEST(elem.values) AS elem1",
			notWant:    "UNNEST(elem.values) AS elem WHERE elem >= 0",
			clickWant:  "arrayExists(elem1 -> elem1 >=",
			clickAvoid: "arrayExists(elem -> elem >=",
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
	if !strings.Contains(sql, "elem1 >=") {
		t.Fatalf("expected inner element predicate, got: %s", sql)
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
			logic:    `{"map":[{"var":"bag.records"},{"oops":[{"var":""}]}]}`,
			wantPath: "$.map[1].oops",
		},
		{
			name:     "nested custom operator under logical in map transform",
			logic:    `{"map":[{"var":"bag.records"},{"and":[{"oops":[{"var":""}]},{">":[{"var":""},0]}]}]}`,
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
