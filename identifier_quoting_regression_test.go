package jsonlogic2sql

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestIdentifierQuotingRegression_NormalAndDeep_AllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "profile.status", Type: FieldTypeString},
		{Name: "metrics.24h.count", Type: FieldTypeInteger},
		{
			Name: "events",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "24h", Type: FieldTypeInteger},
				{Name: "24h.total", Type: FieldTypeInteger},
			},
		},
		{Name: "fixture.windowed_metrics.24h.events.total", Type: FieldTypeInteger},
		{Name: "fixture.windowed_metrics.7d.events.count", Type: FieldTypeInteger},
	})

	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-required", schema: schema},
	}

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	cases := []struct {
		name          string
		logic         string
		inlineSQL     map[Dialect]string
		paramSQL      map[Dialect]string
		expectedParam []QueryParam
	}{
		{
			name:  "normal nested identifier remains unquoted",
			logic: `{"==": [{"var": "profile.status"}, "active"]}`,
			inlineSQL: sameSQLAllDialects(
				"profile.status = 'active'",
			),
			paramSQL: map[Dialect]string{
				DialectBigQuery:   "profile.status = @p1",
				DialectSpanner:    "profile.status = @p1",
				DialectPostgreSQL: "profile.status = $1",
				DialectDuckDB:     "profile.status = $1",
				DialectClickHouse: "profile.status = {p1:String}",
			},
			expectedParam: []QueryParam{{Name: "p1", Value: "active"}},
		},
		{
			name:  "shallow numeric-leading segment is quoted",
			logic: `{">=": [{"var": "metrics.24h.count"}, 50000]}`,
			inlineSQL: map[Dialect]string{
				DialectBigQuery:   "metrics.`24h`.count >= 50000",
				DialectSpanner:    "metrics.`24h`.count >= 50000",
				DialectPostgreSQL: `metrics."24h".count >= 50000`,
				DialectDuckDB:     `metrics."24h".count >= 50000`,
				DialectClickHouse: "metrics.`24h`.count >= 50000",
			},
			paramSQL: map[Dialect]string{
				DialectBigQuery:   "metrics.`24h`.count >= @p1",
				DialectSpanner:    "metrics.`24h`.count >= @p1",
				DialectPostgreSQL: `metrics."24h".count >= $1`,
				DialectDuckDB:     `metrics."24h".count >= $1`,
				DialectClickHouse: "metrics.`24h`.count >= {p1:Float64}",
			},
			expectedParam: []QueryParam{{Name: "p1", Value: float64(50000)}},
		},
		{
			name:  "deeply nested numeric-leading segments are quoted independently",
			logic: `{"and":[{">=":[{"var":"fixture.windowed_metrics.24h.events.total"},50000]},{"<":[{"var":"fixture.windowed_metrics.7d.events.count"},100]}]}`,
			inlineSQL: map[Dialect]string{
				DialectBigQuery:   "(fixture.windowed_metrics.`24h`.events.total >= 50000 AND fixture.windowed_metrics.`7d`.events.count < 100)",
				DialectSpanner:    "(fixture.windowed_metrics.`24h`.events.total >= 50000 AND fixture.windowed_metrics.`7d`.events.count < 100)",
				DialectPostgreSQL: `(fixture.windowed_metrics."24h".events.total >= 50000 AND fixture.windowed_metrics."7d".events.count < 100)`,
				DialectDuckDB:     `(fixture.windowed_metrics."24h".events.total >= 50000 AND fixture.windowed_metrics."7d".events.count < 100)`,
				DialectClickHouse: "(fixture.windowed_metrics.`24h`.events.total >= 50000 AND fixture.windowed_metrics.`7d`.events.count < 100)",
			},
			paramSQL: map[Dialect]string{
				DialectBigQuery:   "(fixture.windowed_metrics.`24h`.events.total >= @p1 AND fixture.windowed_metrics.`7d`.events.count < @p2)",
				DialectSpanner:    "(fixture.windowed_metrics.`24h`.events.total >= @p1 AND fixture.windowed_metrics.`7d`.events.count < @p2)",
				DialectPostgreSQL: `(fixture.windowed_metrics."24h".events.total >= $1 AND fixture.windowed_metrics."7d".events.count < $2)`,
				DialectDuckDB:     `(fixture.windowed_metrics."24h".events.total >= $1 AND fixture.windowed_metrics."7d".events.count < $2)`,
				DialectClickHouse: "(fixture.windowed_metrics.`24h`.events.total >= {p1:Float64} AND fixture.windowed_metrics.`7d`.events.count < {p2:Float64})",
			},
			expectedParam: []QueryParam{
				{Name: "p1", Value: float64(50000)},
				{Name: "p2", Value: float64(100)},
			},
		},
		{
			name:  "deeply nested numeric-leading segments inside custom operators",
			logic: `{"and":[{"betweenInclusive":[{"var":"fixture.windowed_metrics.24h.events.total"},50000,100000]},{"isNonZero":[{"var":"fixture.windowed_metrics.7d.events.count"}]}]}`,
			inlineSQL: map[Dialect]string{
				DialectBigQuery:   "((fixture.windowed_metrics.`24h`.events.total BETWEEN 50000 AND 100000) AND (fixture.windowed_metrics.`7d`.events.count != 0))",
				DialectSpanner:    "((fixture.windowed_metrics.`24h`.events.total BETWEEN 50000 AND 100000) AND (fixture.windowed_metrics.`7d`.events.count != 0))",
				DialectPostgreSQL: `((fixture.windowed_metrics."24h".events.total BETWEEN 50000 AND 100000) AND (fixture.windowed_metrics."7d".events.count != 0))`,
				DialectDuckDB:     `((fixture.windowed_metrics."24h".events.total BETWEEN 50000 AND 100000) AND (fixture.windowed_metrics."7d".events.count != 0))`,
				DialectClickHouse: "((fixture.windowed_metrics.`24h`.events.total BETWEEN 50000 AND 100000) AND (fixture.windowed_metrics.`7d`.events.count != 0))",
			},
			paramSQL: map[Dialect]string{
				DialectBigQuery:   "((fixture.windowed_metrics.`24h`.events.total BETWEEN @p1 AND @p2) AND (fixture.windowed_metrics.`7d`.events.count != 0))",
				DialectSpanner:    "((fixture.windowed_metrics.`24h`.events.total BETWEEN @p1 AND @p2) AND (fixture.windowed_metrics.`7d`.events.count != 0))",
				DialectPostgreSQL: `((fixture.windowed_metrics."24h".events.total BETWEEN $1 AND $2) AND (fixture.windowed_metrics."7d".events.count != 0))`,
				DialectDuckDB:     `((fixture.windowed_metrics."24h".events.total BETWEEN $1 AND $2) AND (fixture.windowed_metrics."7d".events.count != 0))`,
				DialectClickHouse: "((fixture.windowed_metrics.`24h`.events.total BETWEEN {p1:Float64} AND {p2:Float64}) AND (fixture.windowed_metrics.`7d`.events.count != 0))",
			},
			expectedParam: []QueryParam{
				{Name: "p1", Value: float64(50000)},
				{Name: "p2", Value: float64(100000)},
			},
		},
		{
			name:  "deeply nested numeric-leading segment inside dialect-aware custom operator",
			logic: `{"dialectMetricPresent":[{"var":"fixture.windowed_metrics.24h.events.total"}]}`,
			inlineSQL: map[Dialect]string{
				DialectBigQuery:   "IFNULL(fixture.windowed_metrics.`24h`.events.total, 0) > 0",
				DialectSpanner:    "IFNULL(fixture.windowed_metrics.`24h`.events.total, 0) > 0",
				DialectPostgreSQL: `COALESCE(fixture.windowed_metrics."24h".events.total, 0) > 0`,
				DialectDuckDB:     `COALESCE(fixture.windowed_metrics."24h".events.total, 0) > 0`,
				DialectClickHouse: "ifNull(fixture.windowed_metrics.`24h`.events.total, 0) > 0",
			},
			paramSQL: map[Dialect]string{
				DialectBigQuery:   "IFNULL(fixture.windowed_metrics.`24h`.events.total, 0) > 0",
				DialectSpanner:    "IFNULL(fixture.windowed_metrics.`24h`.events.total, 0) > 0",
				DialectPostgreSQL: `COALESCE(fixture.windowed_metrics."24h".events.total, 0) > 0`,
				DialectDuckDB:     `COALESCE(fixture.windowed_metrics."24h".events.total, 0) > 0`,
				DialectClickHouse: "ifNull(fixture.windowed_metrics.`24h`.events.total, 0) > 0",
			},
			expectedParam: nil,
		},
		{
			name:  "array filter quotes numeric-leading element segment",
			logic: `{"filter":[{"var":"events"},{">=":[{"var":"24h"},1]}]}`,
			inlineSQL: map[Dialect]string{
				DialectBigQuery:   "ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE elem.`24h` >= 1)",
				DialectSpanner:    "ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE elem.`24h` >= 1)",
				DialectPostgreSQL: `ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE elem."24h" >= 1)`,
				DialectDuckDB:     `ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE elem."24h" >= 1)`,
				DialectClickHouse: "arrayFilter(elem -> elem.`24h` >= 1, events)",
			},
			paramSQL: map[Dialect]string{
				DialectBigQuery:   "ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE elem.`24h` >= @p1)",
				DialectSpanner:    "ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE elem.`24h` >= @p1)",
				DialectPostgreSQL: `ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE elem."24h" >= $1)`,
				DialectDuckDB:     `ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE elem."24h" >= $1)`,
				DialectClickHouse: "arrayFilter(elem -> elem.`24h` >= {p1:Float64}, events)",
			},
			expectedParam: []QueryParam{{Name: "p1", Value: float64(1)}},
		},
		{
			name:  "array map quotes numeric-leading element segment",
			logic: `{"map":[{"var":"events"},{"var":"24h.total"}]}`,
			inlineSQL: map[Dialect]string{
				DialectBigQuery:   "ARRAY(SELECT elem.`24h`.total FROM UNNEST(events) AS elem)",
				DialectSpanner:    "ARRAY(SELECT elem.`24h`.total FROM UNNEST(events) AS elem)",
				DialectPostgreSQL: `ARRAY(SELECT elem."24h".total FROM UNNEST(events) AS elem)`,
				DialectDuckDB:     `ARRAY(SELECT elem."24h".total FROM UNNEST(events) AS elem)`,
				DialectClickHouse: "arrayMap(elem -> elem.`24h`.total, events)",
			},
			paramSQL: map[Dialect]string{
				DialectBigQuery:   "ARRAY(SELECT elem.`24h`.total FROM UNNEST(events) AS elem)",
				DialectSpanner:    "ARRAY(SELECT elem.`24h`.total FROM UNNEST(events) AS elem)",
				DialectPostgreSQL: `ARRAY(SELECT elem."24h".total FROM UNNEST(events) AS elem)`,
				DialectDuckDB:     `ARRAY(SELECT elem."24h".total FROM UNNEST(events) AS elem)`,
				DialectClickHouse: "arrayMap(elem -> elem.`24h`.total, events)",
			},
			expectedParam: nil,
		},
		{
			name:  "array reduce aggregate quotes numeric-leading current segment",
			logic: `{"reduce":[{"var":"events"},{"+":[{"var":"accumulator"},{"var":"current.24h"}]},0]}`,
			inlineSQL: map[Dialect]string{
				DialectBigQuery:   "0 + COALESCE((SELECT SUM(elem.`24h`) FROM UNNEST(events) AS elem), 0)",
				DialectSpanner:    "0 + COALESCE((SELECT SUM(elem.`24h`) FROM UNNEST(events) AS elem), 0)",
				DialectPostgreSQL: `0 + COALESCE((SELECT SUM(elem."24h") FROM UNNEST(events) AS elem), 0)`,
				DialectDuckDB:     `0 + COALESCE((SELECT SUM(elem."24h") FROM UNNEST(events) AS elem), 0)`,
				DialectClickHouse: "0 + coalesce(arrayReduce('sum', arrayMap(x -> x.`24h`, events)), 0)",
			},
			paramSQL: map[Dialect]string{
				DialectBigQuery:   "@p1 + COALESCE((SELECT SUM(elem.`24h`) FROM UNNEST(events) AS elem), 0)",
				DialectSpanner:    "@p1 + COALESCE((SELECT SUM(elem.`24h`) FROM UNNEST(events) AS elem), 0)",
				DialectPostgreSQL: `$1 + COALESCE((SELECT SUM(elem."24h") FROM UNNEST(events) AS elem), 0)`,
				DialectDuckDB:     `$1 + COALESCE((SELECT SUM(elem."24h") FROM UNNEST(events) AS elem), 0)`,
				DialectClickHouse: "{p1:Float64} + coalesce(arrayReduce('sum', arrayMap(x -> x.`24h`, events)), 0)",
			},
			expectedParam: []QueryParam{{Name: "p1", Value: float64(0)}},
		},
		{
			name:  "array filter custom operator quotes numeric-leading element segment",
			logic: `{"filter":[{"var":"events"},{"isNonZero":[{"var":"24h"}]}]}`,
			inlineSQL: map[Dialect]string{
				DialectBigQuery:   "ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE (elem.`24h` != 0))",
				DialectSpanner:    "ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE (elem.`24h` != 0))",
				DialectPostgreSQL: `ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE (elem."24h" != 0))`,
				DialectDuckDB:     `ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE (elem."24h" != 0))`,
				DialectClickHouse: "arrayFilter(elem -> (elem.`24h` != 0), events)",
			},
			paramSQL: map[Dialect]string{
				DialectBigQuery:   "ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE (elem.`24h` != 0))",
				DialectSpanner:    "ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE (elem.`24h` != 0))",
				DialectPostgreSQL: `ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE (elem."24h" != 0))`,
				DialectDuckDB:     `ARRAY(SELECT elem FROM UNNEST(events) AS elem WHERE (elem."24h" != 0))`,
				DialectClickHouse: "arrayFilter(elem -> (elem.`24h` != 0), events)",
			},
			expectedParam: nil,
		},
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
					registerIdentifierQuotingCustomOperators(t, tr)

					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							assertIdentifierQuotingSQL(t, tr, d, tc.logic, tc.inlineSQL[d], tc.paramSQL[d], tc.expectedParam)
						})
					}
				})
			}
		})
	}
}

func TestIdentifierQuotingRegression_NonASCIIDigitLeadingSchemaSegment(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "metrics.\uff124h.count", Type: FieldTypeInteger},
	})
	logic := `{">=": [{"var": "metrics.\uff124h.count"}, 10]}`

	tests := []struct {
		dialect Dialect
		inline  string
		param   string
	}{
		{
			dialect: DialectBigQuery,
			inline:  "metrics.`\uff124h`.count >= 10",
			param:   "metrics.`\uff124h`.count >= @p1",
		},
		{
			dialect: DialectSpanner,
			inline:  "metrics.`\uff124h`.count >= 10",
			param:   "metrics.`\uff124h`.count >= @p1",
		},
		{
			dialect: DialectPostgreSQL,
			inline:  "metrics.\"\uff124h\".count >= 10",
			param:   "metrics.\"\uff124h\".count >= $1",
		},
		{
			dialect: DialectDuckDB,
			inline:  "metrics.\"\uff124h\".count >= 10",
			param:   "metrics.\"\uff124h\".count >= $1",
		},
		{
			dialect: DialectClickHouse,
			inline:  "metrics.`\uff124h`.count >= 10",
			param:   "metrics.`\uff124h`.count >= {p1:Float64}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.dialect.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: tt.dialect,
				Schema:  schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error: %v", err)
			}

			assertIdentifierQuotingSQL(t, tr, tt.dialect, logic, tt.inline, tt.param, []QueryParam{{Name: "p1", Value: float64(10)}})
		})
	}
}

func TestIdentifierQuotingRegression_UnicodeSchemaSegments_AllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "metrics.café.count", Type: FieldTypeInteger},
		{
			Name: "events",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "café", Type: FieldTypeString},
			},
		},
	})

	tests := []struct {
		dialect     Dialect
		rootInline  string
		rootParam   string
		arrayInline string
		arrayParam  string
	}{
		{
			dialect:     DialectBigQuery,
			rootInline:  "metrics.`café`.count >= 10",
			rootParam:   "metrics.`café`.count >= @p1",
			arrayInline: "ARRAY(SELECT elem.`café` FROM UNNEST(events) AS elem)",
			arrayParam:  "ARRAY(SELECT elem.`café` FROM UNNEST(events) AS elem)",
		},
		{
			dialect:     DialectSpanner,
			rootInline:  "metrics.`café`.count >= 10",
			rootParam:   "metrics.`café`.count >= @p1",
			arrayInline: "ARRAY(SELECT elem.`café` FROM UNNEST(events) AS elem)",
			arrayParam:  "ARRAY(SELECT elem.`café` FROM UNNEST(events) AS elem)",
		},
		{
			dialect:     DialectPostgreSQL,
			rootInline:  `metrics."café".count >= 10`,
			rootParam:   `metrics."café".count >= $1`,
			arrayInline: `ARRAY(SELECT elem."café" FROM UNNEST(events) AS elem)`,
			arrayParam:  `ARRAY(SELECT elem."café" FROM UNNEST(events) AS elem)`,
		},
		{
			dialect:     DialectDuckDB,
			rootInline:  `metrics."café".count >= 10`,
			rootParam:   `metrics."café".count >= $1`,
			arrayInline: `ARRAY(SELECT elem."café" FROM UNNEST(events) AS elem)`,
			arrayParam:  `ARRAY(SELECT elem."café" FROM UNNEST(events) AS elem)`,
		},
		{
			dialect:     DialectClickHouse,
			rootInline:  "metrics.`café`.count >= 10",
			rootParam:   "metrics.`café`.count >= {p1:Float64}",
			arrayInline: "arrayMap(elem -> elem.`café`, events)",
			arrayParam:  "arrayMap(elem -> elem.`café`, events)",
		},
	}

	rootLogic := `{">=": [{"var": "metrics.café.count"}, 10]}`
	arrayLogic := `{"map":[{"var":"events"},{"var":"café"}]}`

	for _, tt := range tests {
		t.Run(tt.dialect.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: tt.dialect,
				Schema:  schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error: %v", err)
			}

			assertIdentifierQuotingSQL(t, tr, tt.dialect, rootLogic, tt.rootInline, tt.rootParam, []QueryParam{{Name: "p1", Value: float64(10)}})
			assertIdentifierQuotingSQL(t, tr, tt.dialect, arrayLogic, tt.arrayInline, tt.arrayParam, nil)
		})
	}
}

func registerIdentifierQuotingCustomOperators(t *testing.T, tr *Transpiler) {
	t.Helper()

	if err := tr.RegisterOperatorFunc("betweenInclusive", func(_ string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 3 {
			return OperatorResult{}, fmt.Errorf("betweenInclusive expects 3 args")
		}
		return PredicateSQL(fmt.Sprintf("(%s BETWEEN %s AND %s)", args[0].SQL, args[1].SQL, args[2].SQL)), nil
	}); err != nil {
		t.Fatalf("RegisterOperatorFunc(betweenInclusive) error: %v", err)
	}

	if err := tr.RegisterOperatorFunc("isNonZero", func(_ string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 1 {
			return OperatorResult{}, fmt.Errorf("isNonZero expects 1 arg")
		}
		return PredicateSQL(fmt.Sprintf("(%s != 0)", args[0].SQL)), nil
	}); err != nil {
		t.Fatalf("RegisterOperatorFunc(isNonZero) error: %v", err)
	}

	if err := tr.RegisterDialectAwareOperatorFunc("dialectMetricPresent", func(_ string, args []OperatorArg, d Dialect) (OperatorResult, error) {
		if len(args) != 1 {
			return OperatorResult{}, fmt.Errorf("dialectMetricPresent expects 1 arg")
		}
		var sql string
		switch d {
		case DialectPostgreSQL, DialectDuckDB:
			sql = fmt.Sprintf("COALESCE(%s, 0) > 0", args[0].SQL)
		case DialectClickHouse:
			sql = fmt.Sprintf("ifNull(%s, 0) > 0", args[0].SQL)
		default:
			sql = fmt.Sprintf("IFNULL(%s, 0) > 0", args[0].SQL)
		}
		return PredicateSQL(sql), nil
	}); err != nil {
		t.Fatalf("RegisterDialectAwareOperatorFunc(dialectMetricPresent) error: %v", err)
	}
}

func TestIdentifierQuotingRegression_UnsafeSchemaRequiredIdentifiersRejected_AllDialects(t *testing.T) {
	t.Parallel()

	logic := `{"==": [{"var": "metrics.24h;DROP.count"}, 1]}`
	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	for _, d := range dialects {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error: %v", err)
			}

			if _, err := tr.TranspileCondition(logic); err == nil {
				t.Fatal("TranspileCondition() expected invalid identifier error, got nil")
			}
			if _, err := tr.TranspileCondition(logic); err == nil {
				t.Fatal("TranspileCondition() expected invalid identifier error, got nil")
			}
			if _, _, err := tr.TranspileParameterizedCondition(logic); err == nil {
				t.Fatal("TranspileParameterizedCondition() expected invalid identifier error, got nil")
			}
			if _, _, err := tr.TranspileParameterizedCondition(logic); err == nil {
				t.Fatal("TranspileParameterizedCondition() expected invalid identifier error, got nil")
			}
		})
	}
}

func assertIdentifierQuotingSQL(
	t *testing.T,
	tr *Transpiler,
	d Dialect,
	logic string,
	expectedInline string,
	expectedParamSQL string,
	expectedParams []QueryParam,
) {
	t.Helper()

	expectedInline = testDuckDBUnnestSourceAliases(d, expectedInline)
	expectedParamSQL = testDuckDBUnnestSourceAliases(d, expectedParamSQL)

	sql, err := tr.TranspileCondition(logic)
	valueMode := IsErrorCode(err, ErrInvalidExpressionContext)
	if valueMode {
		sql, err = tr.TranspileValue(logic)
	}
	if err != nil {
		t.Fatalf("transpile error: %v", err)
	}
	if sql != expectedInline {
		t.Fatalf("transpile for %s = %q, want %q", d, sql, expectedInline)
	}

	var cond string
	if valueMode {
		cond, err = tr.TranspileValue(logic)
	} else {
		cond, err = tr.TranspileCondition(logic)
	}
	if err != nil {
		t.Fatalf("transpile repeat error: %v", err)
	}
	if cond != expectedInline {
		t.Fatalf("TranspileCondition() for %s = %q, want %q", d, cond, expectedInline)
	}

	var paramSQL string
	var gotParams []QueryParam
	if valueMode {
		paramSQL, gotParams, err = tr.TranspileParameterizedValue(logic)
	} else {
		paramSQL, gotParams, err = tr.TranspileParameterizedCondition(logic)
	}
	if err != nil {
		t.Fatalf("parameterized transpile error: %v", err)
	}
	if paramSQL != expectedParamSQL {
		t.Fatalf("TranspileParameterizedCondition() for %s = %q, want %q", d, paramSQL, expectedParamSQL)
	}
	if !reflect.DeepEqual(gotParams, expectedParams) {
		t.Fatalf("TranspileParameterizedCondition() params for %s = %#v, want %#v", d, gotParams, expectedParams)
	}

	var paramCond string
	var condParams []QueryParam
	if valueMode {
		paramCond, condParams, err = tr.TranspileParameterizedValue(logic)
	} else {
		paramCond, condParams, err = tr.TranspileParameterizedCondition(logic)
	}
	if err != nil {
		t.Fatalf("parameterized transpile repeat error: %v", err)
	}
	if paramCond != expectedParamSQL {
		t.Fatalf("TranspileParameterizedCondition() for %s = %q, want %q", d, paramCond, expectedParamSQL)
	}
	if !reflect.DeepEqual(condParams, expectedParams) {
		t.Fatalf("TranspileParameterizedCondition() params for %s = %#v, want %#v", d, condParams, expectedParams)
	}

	var logicMap map[string]interface{}
	if unmarshalErr := json.Unmarshal([]byte(logic), &logicMap); unmarshalErr != nil {
		t.Fatalf("json.Unmarshal() error: %v", unmarshalErr)
	}

	var fromMapSQL string
	var fromMapParams []QueryParam
	if valueMode {
		fromMapSQL, fromMapParams, err = tr.TranspileParameterizedValueFromMap(logicMap)
	} else {
		fromMapSQL, fromMapParams, err = tr.TranspileParameterizedConditionFromMap(logicMap)
	}
	if err != nil {
		t.Fatalf("parameterized from map error: %v", err)
	}
	if fromMapSQL != expectedParamSQL {
		t.Fatalf("TranspileParameterizedConditionFromMap() for %s = %q, want %q", d, fromMapSQL, expectedParamSQL)
	}
	if !reflect.DeepEqual(fromMapParams, expectedParams) {
		t.Fatalf("TranspileParameterizedConditionFromMap() params for %s = %#v, want %#v", d, fromMapParams, expectedParams)
	}
}

func sameSQLAllDialects(sql string) map[Dialect]string {
	return map[Dialect]string{
		DialectBigQuery:   sql,
		DialectSpanner:    sql,
		DialectPostgreSQL: sql,
		DialectDuckDB:     sql,
		DialectClickHouse: sql,
	}
}
