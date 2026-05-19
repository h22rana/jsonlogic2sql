package jsonlogic2sql

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

func registerArrayEdgeCustomOperators(t *testing.T, tr *Transpiler) {
	t.Helper()

	registrations := []struct {
		name string
		fn   OperatorFunc
	}{
		{
			name: "double",
			fn: func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 1 {
					return OperatorResult{}, fmt.Errorf("double requires 1 argument")
				}
				return ValueSQL(fmt.Sprintf("(%s * 2)", args[0].SQL), ExpressionTypeNumber), nil
			},
		},
		{
			name: "plus",
			fn: func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 2 {
					return OperatorResult{}, fmt.Errorf("plus requires 2 arguments")
				}
				return ValueSQL(fmt.Sprintf("(%s + %s)", args[0].SQL, args[1].SQL), ExpressionTypeNumber), nil
			},
		},
		{
			name: "gte",
			fn: func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 2 {
					return OperatorResult{}, fmt.Errorf("gte requires 2 arguments")
				}
				return PredicateSQL(fmt.Sprintf("(%s >= %s)", args[0].SQL, args[1].SQL)), nil
			},
		},
		{
			name: "isPositive",
			fn: func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 1 {
					return OperatorResult{}, fmt.Errorf("isPositive requires 1 argument")
				}
				return PredicateSQL(fmt.Sprintf("(%s > 0)", args[0].SQL)), nil
			},
		},
		{
			name: "emit_item",
			fn: func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 0 {
					return OperatorResult{}, fmt.Errorf("emit_item requires 0 arguments")
				}
				// Intentional raw SQL: custom operators are responsible for emitted SQL.
				return ValueSQL("item", ExpressionTypeUnknown), nil
			},
		},
		{
			name: "emit_current",
			fn: func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 0 {
					return OperatorResult{}, fmt.Errorf("emit_current requires 0 arguments")
				}
				// Intentional raw SQL: custom operators are responsible for emitted SQL.
				return ValueSQL("current", ExpressionTypeUnknown), nil
			},
		},
		{
			name: "emit_current_balance",
			fn: func(_ string, args []OperatorArg) (OperatorResult, error) {
				if len(args) != 0 {
					return OperatorResult{}, fmt.Errorf("emit_current_balance requires 0 arguments")
				}
				// Must remain untouched; contains current as substring, not as element placeholder.
				return ValueSQL("current_balance", ExpressionTypeUnknown), nil
			},
		},
	}

	for _, reg := range registrations {
		if err := tr.RegisterOperatorFunc(reg.name, reg.fn); err != nil {
			t.Fatalf("RegisterOperatorFunc(%q) failed: %v", reg.name, err)
		}
	}
}

func assertNoWholeWordToken(t *testing.T, sql, token string) {
	t.Helper()
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(token) + `\b`)
	if pattern.MatchString(sql) {
		t.Fatalf("expected SQL not to contain whole-word token %q, got: %s", token, sql)
	}
}

func TestCustomOperatorArrayEdgeMatrix_AllDialects_SchemaAndSchemaRequired(t *testing.T) {
	type matrixCase struct {
		name      string
		logic     string
		wantParam int
		validate  func(t *testing.T, d Dialect, out apiOutput)
	}

	cases := []matrixCase{
		{
			name:      "map with custom operator and direct item var",
			logic:     `{"map":[{"var":"bag.numbers"},{"double":[{"var":""}]}]}`,
			wantParam: 0,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayMap(elem -> (elem * 2), bag.numbers)")
				} else {
					assertContains(t, inline, "UNNEST(bag.numbers) AS elem")
					assertContains(t, inline, "(elem * 2)")
				}
			},
		},
		{
			name:      "map with custom operator and raw item SQL",
			logic:     `{"map":[{"var":"bag.numbers"},{"double":[{"emit_item":[]}]}]}`,
			wantParam: 0,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayMap(elem -> (item * 2), bag.numbers)")
				} else {
					assertContains(t, inline, "UNNEST(bag.numbers) AS elem")
					assertContains(t, inline, "(item * 2)")
				}
			},
		},
		{
			name:      "map raw item SQL preserved",
			logic:     `{"map":[{"var":"bag.numbers"},{"emit_item":[]}]}`,
			wantParam: 0,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayMap(elem -> item, bag.numbers)")
				} else {
					assertContains(t, inline, "SELECT item FROM UNNEST(bag.numbers) AS elem")
				}
			},
		},
		{
			name:      "all with custom predicate and direct item var",
			logic:     `{"all":[{"var":"bag.numbers"},{"isPositive":[{"var":""}]}]}`,
			wantParam: 0,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				assertContains(t, inline, "(elem > 0)")
				switch d {
				case DialectBigQuery, DialectSpanner:
					assertContains(t, inline, "ARRAY_LENGTH(bag.numbers)")
				case DialectPostgreSQL:
					assertContains(t, inline, "CARDINALITY(bag.numbers)")
				case DialectDuckDB, DialectClickHouse:
					assertContains(t, inline, "length(bag.numbers)")
				}
			},
		},
		{
			name:      "all with custom predicate and raw item SQL",
			logic:     `{"all":[{"var":"bag.numbers"},{"isPositive":[{"emit_item":[]}]}]}`,
			wantParam: 0,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				assertContains(t, inline, "(item > 0)")
				switch d {
				case DialectBigQuery, DialectSpanner:
					assertContains(t, inline, "ARRAY_LENGTH(bag.numbers)")
				case DialectPostgreSQL:
					assertContains(t, inline, "CARDINALITY(bag.numbers)")
				case DialectDuckDB, DialectClickHouse:
					assertContains(t, inline, "length(bag.numbers)")
				}
			},
		},
		{
			name:      "nested filter mixed scope with direct vars",
			logic:     `{"map":[{"var":"bag.records"},{"filter":[{"var":"values"},{"gte":[{"var":""},0]}]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayFilter(elem1 -> (elem1 >= 0), elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "(elem1 >= 0)")
				}
				assertNoWholeWordToken(t, inline, "current")
			},
		},
		{
			name:      "nested filter with custom operator",
			logic:     `{"map":[{"var":"bag.records"},{"filter":[{"var":"values"},{"gte":[{"var":""},0]}]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayFilter(elem1 -> (elem1 >= 0), elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "(elem1 >= 0)")
				}
				assertNoWholeWordToken(t, inline, "current")
			},
		},
		{
			name:      "nested reduce with direct accumulator/current vars",
			logic:     `{"map":[{"var":"bag.records"},{"reduce":[{"var":"values"},{"plus":[{"var":"accumulator"},{"var":"current"}]},{"var":"base"}]}]}`,
			wantParam: 0,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				assertContains(t, inline, "elem.base")
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayMap(elem -> arrayFold((acc, elem1) -> (elem.base + elem1), elem.values, elem.base), bag.records)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "(elem.base + elem1)")
				}
				assertNoWholeWordToken(t, inline, "current")
				assertNoWholeWordToken(t, inline, "accumulator")
			},
		},
		{
			name:      "nested reduce mixed scope with custom operators",
			logic:     `{"map":[{"var":"bag.records"},{"reduce":[{"var":"values"},{"+":[{"var":"accumulator"},{"double":[{"var":"current"}]}]},{"var":"base"}]}]}`,
			wantParam: 0,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				assertContains(t, inline, "elem.base")
				if d != DialectClickHouse {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
				}
				assertNoWholeWordToken(t, inline, "current")
				assertNoWholeWordToken(t, inline, "accumulator")
			},
		},
		{
			name:      "reduce direct accumulator/current vars",
			logic:     `{"reduce":[{"var":"bag.numbers"},{"plus":[{"var":"accumulator"},{"var":"current"}]},0]}`,
			wantParam: 1,
			validate: func(t *testing.T, _ Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				assertContains(t, inline, "elem")
				assertNoWholeWordToken(t, inline, "current")
				assertNoWholeWordToken(t, inline, "accumulator")
			},
		},
		{
			name:      "reduce raw current SQL preserved",
			logic:     `{"reduce":[{"var":"bag.numbers"},{"+":[{"var":"accumulator"},{"emit_current":[]}]},0]}`,
			wantParam: 1,
			validate: func(t *testing.T, _ Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				assertContains(t, inline, "current")
				assertNoWholeWordToken(t, inline, "accumulator")
			},
		},
		{
			name:      "word-boundary safety for current substring",
			logic:     `{"map":[{"var":"bag.numbers"},{"emit_current_balance":[]}]}`,
			wantParam: 0,
			validate: func(t *testing.T, _ Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				assertContains(t, inline, "current_balance")
				assertNotContains(t, inline, "elem_balance")
			},
		},
		{
			name:      "parameterized custom operator receives placeholder",
			logic:     `{"filter":[{"var":"bag.numbers"},{"gte":[{"var":""},10]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				assertContains(t, inline, "(elem >= 10)")
				assertPlaceholderStyle(t, d, out.paramSQL, 1)
			},
		},
		{
			name:      "deep nested custom reducer with scoped fields",
			logic:     `{"and":[{"some":[{"map":[{"var":"bag.records"},{"reduce":[{"var":"values"},{"plus":[{"var":"accumulator"},{"var":"current"}]},{"var":"base"}]}]},{">=":[{"var":""},0]}]},{">=":[{"var":"metrics.amount"},100]}]}`,
			wantParam: 2,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				assertContains(t, inline, "metrics.amount >= 100")
				assertNoWholeWordToken(t, inline, "current")
				assertNoWholeWordToken(t, inline, "accumulator")
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayMap(elem -> arrayFold((acc, elem1) -> (elem.base + elem1), elem.values, elem.base), bag.records)")
					assertContains(t, inline, "arrayExists(elem -> elem >= 0")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "(elem.base + elem1)")
				}
			},
		},
	}

	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-required", schema: matrixSchema()},
	}

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			for _, d := range dialects {
				t.Run(d.String(), func(t *testing.T) {
					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error: %v", err)
					}
					registerArrayEdgeCustomOperators(t, tr)

					for _, c := range cases {
						t.Run(c.name, func(t *testing.T) {
							out := runAllAPIVariants(t, tr, c.logic)
							if len(out.params) != c.wantParam {
								t.Fatalf("param count mismatch: got=%d want=%d sql=%s", len(out.params), c.wantParam, out.paramSQL)
							}
							if len(out.condParams) != c.wantParam {
								t.Fatalf("condition param count mismatch: got=%d want=%d sql=%s", len(out.condParams), c.wantParam, out.paramCond)
							}
							if c.validate != nil {
								c.validate(t, d, out)
							}
						})
					}
				})
			}
		})
	}
}

func TestCustomOperatorArrayEdgeMatrix_SchemaValidationParity(t *testing.T) {
	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}
	logic := `{"map":[{"var":"unknown.values"},{"double":[{"var":""}]}]}`

	for _, d := range dialects {
		t.Run(d.String(), func(t *testing.T) {
			withSchema, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  matrixSchema(),
			})
			if err != nil {
				t.Fatalf("with-schema transpiler init failed: %v", err)
			}
			emptySchemaTr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  emptyTestSchema(),
			})
			if err != nil {
				t.Fatalf("empty schema transpiler init failed: %v", err)
			}
			registerArrayEdgeCustomOperators(t, withSchema)
			registerArrayEdgeCustomOperators(t, emptySchemaTr)

			_, err = withSchema.TranspileValue(logic)
			if err == nil || !strings.Contains(err.Error(), "is not defined in schema") {
				t.Fatalf("expected schema validation error, got: %v", err)
			}

			_, err = emptySchemaTr.TranspileValue(logic)
			if err == nil || !strings.Contains(err.Error(), "is not defined in schema") {
				t.Fatalf("expected empty-schema validation error, got: %v", err)
			}
		})
	}
}
