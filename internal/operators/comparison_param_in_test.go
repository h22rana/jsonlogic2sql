package operators

import (
	"reflect"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestComparisonOperator_handleInParam(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"tags":        "array",
		"description": "string",
		"region":      "string",
		"amount":      "number",
		"flag":        "boolean",
	})
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	t.Run("array type var on right uses arrayMembershipSQL", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam("needle", map[string]interface{}{"var": "tags"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := testNullSafeArrayMembershipSQL(dialect.DialectBigQuery, "@p1", "tags")
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{{Name: "p1", Value: "needle"}})
	})

	t.Run("array type var on left with literal list folds false", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(map[string]interface{}{"var": "tags"}, []interface{}{"needle", "other"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		if want := "FALSE"; got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), nil)
	})

	t.Run("string type var on right uses strposFunc with parameterized left", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam("probe", map[string]interface{}{"var": "description"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "STRPOS(description, @p1) > 0"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{{Name: "p1", Value: "probe"}})
	})

	t.Run("array of literals parameterized", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		leftOriginal := map[string]interface{}{"var": "region"}
		got, err := op.handleInParam(leftOriginal, []interface{}{"EU", "APAC", "US"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "region IN (@p1, @p2, @p3)"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{
			{Name: "p1", Value: "EU"},
			{Name: "p2", Value: "APAC"},
			{Name: "p3", Value: "US"},
		})
	})

	t.Run("numeric right-hand literal folds false without params", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(map[string]interface{}{"var": "region"}, float64(12345), pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		if want := "FALSE"; got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), nil)
	})

	t.Run("empty right-hand array folds false without params", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(map[string]interface{}{"var": "region"}, []interface{}{}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		if want := "FALSE"; got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), nil)
	})

	t.Run("string containment with field-only schema", func(t *testing.T) {
		fieldOnlyConfig := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
		fieldOnlyOp := NewComparisonOperator(fieldOnlyConfig)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := fieldOnlyOp.handleInParam("foo", map[string]interface{}{"var": "bar"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "STRPOS(bar, @p1) > 0"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{{Name: "p1", Value: "foo"}})
	})

	t.Run("schema coercion for string field", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(float64(123), map[string]interface{}{"var": "description"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "STRPOS(description, @p1) > 0"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		if len(pc.Params()) != 1 {
			t.Fatalf("expected 1 param, got %d: %v", len(pc.Params()), pc.Params())
		}
		if pc.Params()[0].Value != "123" {
			t.Errorf("param value = %v (%T), want string \"123\"", pc.Params()[0].Value, pc.Params()[0].Value)
		}
	})

	t.Run("schema numeric field needle casts without unused left param", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(map[string]interface{}{"var": "amount"}, "12345", pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := testRuntimeStringContainmentSQL(
			dialect.DialectBigQuery,
			"@p1",
			"COALESCE(CAST(amount AS STRING), 'null')",
		)
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{{Name: "p1", Value: "12345"}})
	})

	t.Run("schema boolean field needle stringifies without unused left param", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(map[string]interface{}{"var": "flag"}, "true", pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := testRuntimeStringContainmentSQL(
			dialect.DialectBigQuery,
			"@p1",
			"CASE WHEN flag IS TRUE THEN 'true' WHEN flag IS FALSE THEN 'false' ELSE 'null' END",
		)
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{{Name: "p1", Value: "true"}})
	})

	t.Run("literal number needle stringifies for string haystack", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(float64(3), "12345", pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "STRPOS(@p1, @p2) > 0"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{
			{Name: "p1", Value: "12345"},
			{Name: "p2", Value: "3"},
		})
	})

	t.Run("empty string needle checks haystack is non-null without params", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam("", map[string]interface{}{"var": "description"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "(description IS NOT NULL)"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), nil)
	})

	t.Run("array literal needle stringifies for string haystack", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam([]interface{}{float64(1), float64(2)}, "x1,2y", pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "STRPOS(@p1, @p2) > 0"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{
			{Name: "p1", Value: "x1,2y"},
			{Name: "p2", Value: "1,2"},
		})
	})

	t.Run("ProcessedValue SQL literal treated as string containment", func(t *testing.T) {
		schemaRequiredConfig := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
		schemaRequiredOp := NewComparisonOperator(schemaRequiredConfig)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := schemaRequiredOp.handleInParam(
			ProcessedValue{IsSQL: true, Value: "'foo'"},
			map[string]interface{}{"var": "col"},
			pc,
		)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "STRPOS(col, 'foo') > 0"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
	})

	t.Run("ProcessedValue placeholder for string param uses string containment", func(t *testing.T) {
		schemaRequiredConfig := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
		schemaRequiredOp := NewComparisonOperator(schemaRequiredConfig)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		pc.Add("hello") // @p1 = "hello" (string)
		got, err := schemaRequiredOp.handleInParam(
			ProcessedValue{IsSQL: true, Value: "@p1"},
			map[string]interface{}{"var": "col"},
			pc,
		)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := testRuntimeStringContainmentSQL(dialect.DialectBigQuery, "col", "@p1")
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
	})

	t.Run("ProcessedValue placeholder for numeric param uses array membership", func(t *testing.T) {
		schemaRequiredConfig := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
		schemaRequiredOp := NewComparisonOperator(schemaRequiredConfig)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		pc.Add(float64(42)) // @p1 = 42 (numeric)
		got, err := schemaRequiredOp.handleInParam(
			ProcessedValue{IsSQL: true, Value: "@p1"},
			map[string]interface{}{"var": "col"},
			pc,
		)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := testNullSafeArrayMembershipSQL(dialect.DialectBigQuery, "@p1", "col")
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
	})

	t.Run("ProcessedValue SQL expression uses array membership", func(t *testing.T) {
		schemaRequiredConfig := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
		schemaRequiredOp := NewComparisonOperator(schemaRequiredConfig)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		pc.Add("hello") // @p1 = "hello"
		got, err := schemaRequiredOp.handleInParam(
			ProcessedValue{IsSQL: true, Value: "LOWER(@p1)"},
			map[string]interface{}{"var": "col"},
			pc,
		)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := testNullSafeArrayMembershipSQL(dialect.DialectBigQuery, "LOWER(@p1)", "col")
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
	})
}

func TestComparisonOperator_handleInParam_SchemaRequired_StringExpressionHeuristic(t *testing.T) {
	leftStringExpr := map[string]interface{}{
		OpCat: []interface{}{
			map[string]interface{}{
				OpSubstr: []interface{}{
					map[string]interface{}{OpVar: "profile.first"},
					float64(0),
					float64(2),
				},
			},
			"-x",
		},
	}
	leftNumericExpr := map[string]interface{}{
		"+": []interface{}{float64(1), float64(2)},
	}
	rightVar := map[string]interface{}{OpVar: "profile.name"}

	tests := []struct {
		name       string
		d          dialect.Dialect
		style      params.PlaceholderStyle
		leftArg    interface{}
		wantSQL    string
		wantParams []params.QueryParam
	}{
		{
			name:    "bigquery nested string expression uses containment",
			d:       dialect.DialectBigQuery,
			style:   params.PlaceholderNamed,
			leftArg: leftStringExpr,
			wantSQL: testRuntimeStringContainmentSQL(
				dialect.DialectBigQuery,
				"profile.name",
				"CONCAT(COALESCE(SUBSTR(profile.first, (@p1 + 1), @p2), ''), @p3)",
			),
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: "-x"},
			},
		},
		{
			name:    "postgres nested string expression uses containment",
			d:       dialect.DialectPostgreSQL,
			style:   params.PlaceholderPositional,
			leftArg: leftStringExpr,
			wantSQL: testRuntimeStringContainmentSQL(
				dialect.DialectPostgreSQL,
				"profile.name",
				"CONCAT(COALESCE(SUBSTR(profile.first, ($1 + 1), $2), ''), $3)",
			),
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: "-x"},
			},
		},
		{
			name:    "clickhouse nested string expression uses containment",
			d:       dialect.DialectClickHouse,
			style:   params.PlaceholderNamed,
			leftArg: leftStringExpr,
			wantSQL: testRuntimeStringContainmentSQL(
				dialect.DialectClickHouse,
				"profile.name",
				"CONCAT(COALESCE(substring(profile.first, (@p1 + 1), @p2), ''), @p3)",
			),
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: "-x"},
			},
		},
		{
			name:    "bigquery numeric expression uses null-safe membership fallback",
			d:       dialect.DialectBigQuery,
			style:   params.PlaceholderNamed,
			leftArg: leftNumericExpr,
			wantSQL: testNullSafeArrayMembershipSQL(dialect.DialectBigQuery, "(@p1 + @p2)", "profile.name"),
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
			},
		},
		{
			name:    "postgres numeric expression uses null-safe membership fallback",
			d:       dialect.DialectPostgreSQL,
			style:   params.PlaceholderPositional,
			leftArg: leftNumericExpr,
			wantSQL: testNullSafeArrayMembershipSQL(dialect.DialectPostgreSQL, "($1 + $2)", "profile.name"),
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
			},
		},
		{
			name:    "clickhouse numeric expression uses null-safe membership fallback",
			d:       dialect.DialectClickHouse,
			style:   params.PlaceholderNamed,
			leftArg: leftNumericExpr,
			wantSQL: testNullSafeArrayMembershipSQL(dialect.DialectClickHouse, "(@p1 + @p2)", "profile.name"),
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := NewComparisonOperator(NewOperatorConfig(tt.d, &fieldOnlySchemaProvider{}))
			pc := params.NewParamCollector(tt.style)
			got, err := op.ToSQLParam("in", []interface{}{tt.leftArg, rightVar}, pc)
			if err != nil {
				t.Fatalf("ToSQLParam() error = %v", err)
			}
			if got != tt.wantSQL {
				t.Fatalf("ToSQLParam() = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_InDoesNotMutateInputArray(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"region": "string",
	})
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	t.Run("non-parameterized path", func(t *testing.T) {
		arr := []interface{}{float64(1), float64(2)}
		original := append([]interface{}(nil), arr...)

		_, err := op.ToSQL("in", []interface{}{
			map[string]interface{}{"var": "region"},
			arr,
		})
		if err != nil {
			t.Fatalf("ToSQL(in) unexpected error: %v", err)
		}
		if !reflect.DeepEqual(arr, original) {
			t.Fatalf("input array mutated: got %#v, want %#v", arr, original)
		}
	})

	t.Run("parameterized path", func(t *testing.T) {
		arr := []interface{}{float64(1), float64(2)}
		original := append([]interface{}(nil), arr...)
		pc := params.NewParamCollector(params.PlaceholderNamed)

		_, err := op.handleInParam(
			map[string]interface{}{"var": "region"},
			arr,
			pc,
		)
		if err != nil {
			t.Fatalf("handleInParam() unexpected error: %v", err)
		}
		if !reflect.DeepEqual(arr, original) {
			t.Fatalf("input array mutated: got %#v, want %#v", arr, original)
		}
	})
}
