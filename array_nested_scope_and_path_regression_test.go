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

			tr, err := NewTranspiler(d, defaultTestSchema())
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

				tr, err := NewTranspiler(d, defaultTestSchema())
				if err != nil {
					t.Fatalf("NewTranspiler() error: %v", err)
				}

				var (
					sql       string
					inlineErr error
				)
				if tc.valueMode {
					sql, inlineErr = tr.TranspileValue(tc.logic)
				} else {
					sql, inlineErr = tr.TranspileCondition(tc.logic)
				}
				if inlineErr != nil {
					t.Fatalf("inline transpilation error: %v", inlineErr)
				}
				assertNestedArrayPredicateScope(t, d, sql, tc.want, tc.notWant, tc.clickWant, tc.clickAvoid)

				var (
					paramSQL string
					paramErr error
				)
				if tc.valueMode {
					paramSQL, _, paramErr = tr.TranspileParameterizedValue(tc.logic)
				} else {
					paramSQL, _, paramErr = tr.TranspileParameterizedCondition(tc.logic)
				}
				if paramErr != nil {
					t.Fatalf("parameterized transpilation error: %v", paramErr)
				}
				assertNestedArrayPredicateScope(t, d, paramSQL, tc.want, tc.notWant, tc.clickWant, tc.clickAvoid)
			})
		}
	}
}

func TestScopedNestedArrayPredicatesAreTwoValuedInValueMode_AllDialects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		logic string
	}{
		{
			name:  "map all",
			logic: `{"map":[{"var":"groups"},{"all":[{"var":"items"},true]}]}`,
		},
		{
			name:  "map some",
			logic: `{"map":[{"var":"groups"},{"some":[{"var":"items"},true]}]}`,
		},
		{
			name:  "map none",
			logic: `{"map":[{"var":"groups"},{"none":[{"var":"items"},true]}]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error: %v", err)
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					sql, err := tr.TranspileValue(tc.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error: %v", err)
					}
					assertNestedPredicateValueMaterialized(t, sql)

					paramSQL, params, err := tr.TranspileParameterizedValue(tc.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error: %v", err)
					}
					assertNestedPredicateValueMaterialized(t, paramSQL)
					if len(params) != 0 {
						t.Fatalf("params = %#v, want none", params)
					}
				})
			}
		})
	}
}

func assertNestedPredicateValueMaterialized(t *testing.T, sql string) {
	t.Helper()
	for _, want := range []string{"CASE WHEN", "elem.items", "THEN TRUE ELSE FALSE END"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("SQL = %q, want to contain %q", sql, want)
		}
	}
	if strings.Contains(sql, "SELECT (ARRAY_LENGTH(elem.items)") ||
		strings.Contains(sql, "SELECT (CARDINALITY(elem.items)") ||
		strings.Contains(sql, "SELECT (length(elem.items)") ||
		strings.Contains(sql, "elem -> (length(elem.items)") {
		t.Fatalf("nested predicate was not materialized as a value: %s", sql)
	}
}

func TestScopedNestedArrayValuesUseTruthinessInArrayLambdaPredicates_AllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{
			Name: "records",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "values", Type: FieldTypeArray},
			},
		},
	})

	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-required", schema: schema},
	}

	cases := []struct {
		name      string
		logic     string
		condition bool
		kind      string
		paramLen  int
	}{
		{
			name:  "filter map predicate",
			logic: `{"filter":[{"var":"records"},{"map":[{"var":"values"},{"var":""}]}]}`,
			kind:  "array",
		},
		{
			name:      "some map predicate",
			logic:     `{"some":[{"var":"records"},{"map":[{"var":"values"},{"var":""}]}]}`,
			condition: true,
			kind:      "array",
		},
		{
			name:     "filter reduce predicate",
			logic:    `{"filter":[{"var":"records"},{"reduce":[{"var":"values"},{"+":[{"var":"accumulator"},{"var":"current"}]},0]}]}`,
			kind:     "number",
			paramLen: 1,
		},
		{
			name:      "all reduce predicate",
			logic:     `{"all":[{"var":"records"},{"reduce":[{"var":"values"},{"+":[{"var":"accumulator"},{"var":"current"}]},0]}]}`,
			condition: true,
			kind:      "number",
			paramLen:  1,
		},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()
			for _, d := range allDialects() {
				t.Run(d.String(), func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error: %v", err)
					}

					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							t.Parallel()

							var (
								sql       string
								inlineErr error
							)
							if tc.condition {
								sql, inlineErr = tr.TranspileCondition(tc.logic)
							} else {
								sql, inlineErr = tr.TranspileValue(tc.logic)
							}
							if inlineErr != nil {
								t.Fatalf("inline transpilation error: %v", inlineErr)
							}
							assertScopedArrayValueTruthiness(t, d, sql, tc.kind)

							var (
								paramSQL string
								params   []QueryParam
								paramErr error
							)
							if tc.condition {
								paramSQL, params, paramErr = tr.TranspileParameterizedCondition(tc.logic)
							} else {
								paramSQL, params, paramErr = tr.TranspileParameterizedValue(tc.logic)
							}
							if paramErr != nil {
								t.Fatalf("parameterized transpilation error: %v", paramErr)
							}
							assertScopedArrayValueTruthiness(t, d, paramSQL, tc.kind)
							if len(params) != tc.paramLen {
								t.Fatalf("params = %#v, want len %d", params, tc.paramLen)
							}
						})
					}
				})
			}
		})
	}
}

func assertScopedArrayValueTruthiness(t *testing.T, d Dialect, sql, kind string) {
	t.Helper()
	if !strings.Contains(sql, "elem.values") {
		t.Fatalf("SQL = %q, want scoped nested source elem.values", sql)
	}
	if !strings.Contains(sql, "IS NOT NULL") {
		t.Fatalf("SQL = %q, want local value expression guarded by IS NOT NULL", sql)
	}

	switch kind {
	case "array":
		wantLength := "ARRAY_LENGTH("
		switch d {
		case DialectPostgreSQL:
			wantLength = "CARDINALITY("
		case DialectDuckDB, DialectClickHouse:
			wantLength = "length("
		}
		if !strings.Contains(sql, wantLength) || !strings.Contains(sql, "> 0") {
			t.Fatalf("SQL = %q, want array truthiness via %s... > 0", sql, wantLength)
		}
		for _, raw := range []string{"WHERE ARRAY(SELECT", "-> arrayMap("} {
			if strings.Contains(sql, raw) {
				t.Fatalf("SQL = %q, contains raw array value predicate %q", sql, raw)
			}
		}
	case "number":
		if !strings.Contains(sql, "!= 0") {
			t.Fatalf("SQL = %q, want numeric truthiness via != 0", sql)
		}
		for _, raw := range []string{
			"WHERE 0 +", "WHERE @p1 +", "WHERE $1 +",
			"-> 0 +", "-> @p1 +", "-> $1 +",
		} {
			if strings.Contains(sql, raw) {
				t.Fatalf("SQL = %q, contains raw numeric value predicate %q", sql, raw)
			}
		}
	default:
		t.Fatalf("unsupported truthiness kind %q", kind)
	}
}

func TestReduceNestedArrayOperatorsUseChildAliases_AllDialectsSchemaModes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{
			Name: "groups",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{
					Name: "values",
					Type: FieldTypeArray,
					ElementFields: []FieldSchema{
						{Name: "amount", Type: FieldTypeNumber},
						{Name: "score", Type: FieldTypeNumber},
						{
							Name: "tags",
							Type: FieldTypeArray,
							ElementFields: []FieldSchema{
								{Name: "score", Type: FieldTypeNumber},
							},
						},
					},
				},
			},
		},
	})

	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-required", schema: schema},
	}

	cases := []struct {
		name      string
		logic     string
		want      []string
		notWant   []string
		clickWant []string
		clickDeny []string
	}{
		{
			name:  "map source current field and transform bare field",
			logic: `{"reduce":[{"var":"groups"},{"map":[{"var":"current.values"},{"merge":[{"var":"tags"},{"var":"tags"}]}]},[]]}`,
			want: []string{
				"UNNEST(elem.values) AS elem1",
				"elem1.tags",
			},
			notWant: []string{
				"UNNEST(elem.values) AS elem)",
				"ARRAY_CONCAT(elem.tags, elem.tags)",
				"elem.tags || elem.tags",
			},
			clickWant: []string{
				"arrayMap(elem1 -> arrayConcat(elem1.tags, elem1.tags), elem.values)",
			},
			clickDeny: []string{
				"arrayMap(elem -> arrayConcat(elem.tags, elem.tags), elem.values)",
			},
		},
		{
			name:  "filter source current field and predicate bare field",
			logic: `{"reduce":[{"var":"groups"},{"filter":[{"var":"current.values"},{">":[{"var":"score"},0]}]},[]]}`,
			want: []string{
				"UNNEST(elem.values) AS elem1",
				"elem1.score >",
			},
			notWant: []string{
				"UNNEST(elem.values) AS elem WHERE elem.score >",
			},
			clickWant: []string{
				"arrayFilter(elem1 -> elem1.score >",
				", elem.values)",
			},
			clickDeny: []string{
				"arrayFilter(elem -> elem.score >",
			},
		},
		{
			name:  "some source current field and predicate bare field",
			logic: `{"reduce":[{"var":"groups"},{"some":[{"var":"current.values"},{">":[{"var":"score"},0]}]},false]}`,
			want: []string{
				"UNNEST(elem.values) AS elem1",
				"elem1.score >",
			},
			notWant: []string{
				"UNNEST(elem.values) AS elem WHERE elem.score >",
			},
			clickWant: []string{
				"arrayExists(elem1 -> elem1.score >",
				", elem.values)",
			},
			clickDeny: []string{
				"arrayExists(elem -> elem.score >",
			},
		},
		{
			name:  "all source current field and predicate bare field",
			logic: `{"reduce":[{"var":"groups"},{"all":[{"var":"current.values"},{">":[{"var":"score"},0]}]},false]}`,
			want: []string{
				"UNNEST(elem.values) AS elem1",
				"elem1.score >",
			},
			notWant: []string{
				"UNNEST(elem.values) AS elem WHERE NOT (elem.score >",
			},
			clickWant: []string{
				"arrayAll(elem1 -> elem1.score >",
				", elem.values)",
			},
			clickDeny: []string{
				"arrayAll(elem -> elem.score >",
			},
		},
		{
			name:  "none source current field and predicate bare field",
			logic: `{"reduce":[{"var":"groups"},{"none":[{"var":"current.values"},{">":[{"var":"score"},0]}]},false]}`,
			want: []string{
				"UNNEST(elem.values) AS elem1",
				"elem1.score >",
			},
			notWant: []string{
				"UNNEST(elem.values) AS elem WHERE elem.score >",
			},
			clickWant: []string{
				"arrayExists(elem1 -> elem1.score >",
				", elem.values)",
			},
			clickDeny: []string{
				"arrayExists(elem -> elem.score >",
			},
		},
		{
			name:  "nested reduce source current field and reducer current field",
			logic: `{"reduce":[{"var":"groups"},{"+":[{"var":"accumulator"},{"reduce":[{"var":"current.values"},{"+":[{"var":"accumulator"},{"var":"current.amount"}]},0]}]},0]}`,
			want: []string{
				"UNNEST(elem.values) AS elem1",
				"SUM(elem1.amount)",
			},
			notWant: []string{
				"SUM(elem.amount)",
				"UNNEST(elem.values) AS elem)",
			},
			clickWant: []string{
				"arrayMap(x -> x.amount, elem.values)",
			},
			clickDeny: []string{
				"arrayMap(x -> elem.amount, elem.values)",
			},
		},
		{
			name:  "three-level map filter keeps elem elem1 elem2 distinct",
			logic: `{"reduce":[{"var":"groups"},{"map":[{"var":"current.values"},{"filter":[{"var":"tags"},{">":[{"var":"score"},0]}]}]},[]]}`,
			want: []string{
				"UNNEST(elem.values) AS elem1",
				"UNNEST(elem1.tags) AS elem2",
				"elem2.score >",
			},
			notWant: []string{
				"UNNEST(elem.values) AS elem)",
				"UNNEST(elem1.tags) AS elem1",
				"elem1.score >",
			},
			clickWant: []string{
				"arrayMap(elem1 -> arrayFilter(elem2 -> elem2.score >",
				", elem1.tags), elem.values)",
			},
			clickDeny: []string{
				"arrayMap(elem -> arrayFilter(elem -> elem.score >",
			},
		},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()
			for _, d := range allDialects() {
				t.Run(d.String(), func(t *testing.T) {
					t.Parallel()
					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error: %v", err)
					}

					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							t.Parallel()
							sql, inlineErr := tr.TranspileValue(tc.logic)
							if inlineErr != nil {
								t.Fatalf("TranspileValue() error: %v", inlineErr)
							}
							assertReduceNestedArrayAliases(t, d, sql, tc)

							paramSQL, _, paramErr := tr.TranspileParameterizedValue(tc.logic)
							if paramErr != nil {
								t.Fatalf("TranspileParameterizedValue() error: %v", paramErr)
							}
							assertReduceNestedArrayAliases(t, d, paramSQL, tc)
						})
					}
				})
			}
		})
	}
}

func assertReduceNestedArrayAliases(t *testing.T, d Dialect, sql string, tc struct {
	name      string
	logic     string
	want      []string
	notWant   []string
	clickWant []string
	clickDeny []string
},
) {
	t.Helper()
	wants := tc.want
	denies := tc.notWant
	if d == DialectClickHouse {
		wants = tc.clickWant
		denies = tc.clickDeny
	}
	for _, want := range wants {
		if !strings.Contains(sql, want) {
			t.Fatalf("%s: expected SQL to contain %q, got: %s", tc.name, want, sql)
		}
	}
	for _, deny := range denies {
		if strings.Contains(sql, deny) {
			t.Fatalf("%s: unexpected alias-shadow fragment %q in SQL: %s", tc.name, deny, sql)
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

	tr, err := NewTranspiler(DialectBigQuery, defaultTestSchema())
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

			_, inlineErr := tr.TranspileValue(tc.logic)
			if inlineErr == nil {
				t.Fatalf("expected inline error for %s, got nil", tc.name)
			}
			if !strings.Contains(inlineErr.Error(), tc.wantPath) {
				t.Fatalf("inline error missing expected path %q: %v", tc.wantPath, inlineErr)
			}

			_, _, paramErr := tr.TranspileParameterizedValue(tc.logic)
			if paramErr == nil {
				t.Fatalf("expected parameterized error for %s, got nil", tc.name)
			}
			if !strings.Contains(paramErr.Error(), tc.wantPath) {
				t.Fatalf("parameterized error missing expected path %q: %v", tc.wantPath, paramErr)
			}
		})
	}
}
