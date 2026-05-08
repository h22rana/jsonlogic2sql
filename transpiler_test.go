package jsonlogic2sql

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"testing"
)

func TestNewTranspiler(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() returned error: %v", err)
	}
	if tr == nil {
		t.Fatal("NewTranspiler() returned nil")
		return
	}
	if tr.parser == nil {
		t.Fatal("parser is nil")
	}
}

func TestTranspiler_NullSafeFieldEquality(t *testing.T) {
	defaultTr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error = %v", err)
	}
	defaultSQL, err := defaultTr.Transpile(`{"==": [{"var": "a"}, {"var": "b"}]}`)
	if err != nil {
		t.Fatalf("Transpile() default error = %v", err)
	}
	if defaultSQL != "WHERE a = b" {
		t.Fatalf("Transpile() default = %q, want %q", defaultSQL, "WHERE a = b")
	}

	configTr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect:               DialectBigQuery,
		NullSafeFieldEquality: true,
	})
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() error = %v", err)
	}
	configSQL, err := configTr.Transpile(`{"==": [{"var": "a"}, {"var": "b"}]}`)
	if err != nil {
		t.Fatalf("Transpile() config error = %v", err)
	}
	if want := "WHERE ((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b))"; configSQL != want {
		t.Fatalf("Transpile() config = %q, want %q", configSQL, want)
	}

	setterTr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() setter error = %v", err)
	}
	setterTr.SetNullSafeFieldEquality(true)
	if !setterTr.config.NullSafeFieldEquality || !setterTr.operatorConfig.NullSafeFieldEquality {
		t.Fatal("SetNullSafeFieldEquality(true) did not update public and operator config")
	}
	setterSQL, err := setterTr.Transpile(`{"!==": [{"var": "a"}, {"var": "b"}]}`)
	if err != nil {
		t.Fatalf("Transpile() setter error = %v", err)
	}
	if want := "WHERE ((a IS NULL AND b IS NOT NULL) OR (a IS NOT NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a <> b))"; setterSQL != want {
		t.Fatalf("Transpile() setter = %q, want %q", setterSQL, want)
	}
}

func TestTranspiler_NullSafeFieldEquality_AllDialects(t *testing.T) {
	for _, d := range []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	} {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect:               d,
				NullSafeFieldEquality: true,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}
			got, err := tr.Transpile(`{"===": [{"var": "a"}, {"var": "b"}]}`)
			if err != nil {
				t.Fatalf("Transpile() error = %v", err)
			}
			if want := "WHERE ((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b))"; got != want {
				t.Fatalf("Transpile() = %q, want %q", got, want)
			}
		})
	}
}

func TestTranspiler_NullSafeFieldEquality_DeeplyNested(t *testing.T) {
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect:               DialectBigQuery,
		NullSafeFieldEquality: true,
	})
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() error = %v", err)
	}

	logic := `{"and":[{"or":[{"===":[{"var":"a"},{"var":"b"}]},{"!=":[{"var":"c"},{"var":"d"}]}]},{"and":[{"==":[{"var":["e","left"]},{"var":["f","right"]}]},{">":[{"var":"score"},10]}]}]}`

	got, err := tr.Transpile(logic)
	if err != nil {
		t.Fatalf("Transpile() error = %v", err)
	}
	want := "WHERE (" +
		"(((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b)) OR ((c IS NULL AND d IS NOT NULL) OR (c IS NOT NULL AND d IS NULL) OR (c IS NOT NULL AND d IS NOT NULL AND c != d)))" +
		" AND " +
		"(((COALESCE(e, 'left') IS NULL AND COALESCE(f, 'right') IS NULL) OR (COALESCE(e, 'left') IS NOT NULL AND COALESCE(f, 'right') IS NOT NULL AND COALESCE(e, 'left') = COALESCE(f, 'right'))) AND score > 10)" +
		")"
	if got != want {
		t.Fatalf("Transpile() = %q, want %q", got, want)
	}

	gotParamSQL, gotParams, err := tr.TranspileParameterized(logic)
	if err != nil {
		t.Fatalf("TranspileParameterized() error = %v", err)
	}
	wantParamSQL := "WHERE (" +
		"(((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b)) OR ((c IS NULL AND d IS NOT NULL) OR (c IS NOT NULL AND d IS NULL) OR (c IS NOT NULL AND d IS NOT NULL AND c != d)))" +
		" AND " +
		"(((COALESCE(e, @p1) IS NULL AND COALESCE(f, @p2) IS NULL) OR (COALESCE(e, @p1) IS NOT NULL AND COALESCE(f, @p2) IS NOT NULL AND COALESCE(e, @p1) = COALESCE(f, @p2))) AND score > @p3)" +
		")"
	if gotParamSQL != wantParamSQL {
		t.Fatalf("TranspileParameterized() SQL = %q, want %q", gotParamSQL, wantParamSQL)
	}
	wantParams := []QueryParam{
		{Name: "p1", Value: "left"},
		{Name: "p2", Value: "right"},
		{Name: "p3", Value: float64(10)},
	}
	if !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("TranspileParameterized() params = %#v, want %#v", gotParams, wantParams)
	}
}

func TestTranspiler_NullSafeFieldEquality_ReviewRegressions(t *testing.T) {
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect:               DialectBigQuery,
		NullSafeFieldEquality: true,
	})
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() error = %v", err)
	}

	got, err := tr.Transpile(`{"!":{"==":[{"var":"a"},{"var":"b"}]}}`)
	if err != nil {
		t.Fatalf("Transpile() negated equality error = %v", err)
	}
	want := "WHERE NOT (((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b)))"
	if got != want {
		t.Fatalf("Transpile() negated equality = %q, want %q", got, want)
	}

	arrayLogic := `{"some":[{"var":"items"},{"==":[{"var":"current.a"},{"var":"current.b"}]}]}`
	got, err = tr.Transpile(arrayLogic)
	if err != nil {
		t.Fatalf("Transpile() scoped array equality error = %v", err)
	}
	want = "WHERE EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE ((elem.a IS NULL AND elem.b IS NULL) OR (elem.a IS NOT NULL AND elem.b IS NOT NULL AND elem.a = elem.b)))"
	if got != want {
		t.Fatalf("Transpile() scoped array equality = %q, want %q", got, want)
	}

	gotParamSQL, gotParams, err := tr.TranspileParameterized(arrayLogic)
	if err != nil {
		t.Fatalf("TranspileParameterized() scoped array equality error = %v", err)
	}
	if gotParamSQL != want {
		t.Fatalf("TranspileParameterized() scoped array equality = %q, want %q", gotParamSQL, want)
	}
	if len(gotParams) != 0 {
		t.Fatalf("TranspileParameterized() params = %#v, want none", gotParams)
	}

	schema := mustNewSchema([]FieldSchema{
		{Name: "items", Type: FieldTypeArray},
		{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"active", "inactive"}},
	})
	enumTr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect:               DialectBigQuery,
		Schema:                schema,
		NullSafeFieldEquality: true,
	})
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() enum schema error = %v", err)
	}

	enumCases := []struct {
		name  string
		logic string
		want  string
	}{
		{
			name:  "scoped field left and enum field right",
			logic: `{"some":[{"var":"items"},{"==":[{"var":"current.status"},{"var":"status"}]}]}`,
			want:  "WHERE EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE ((elem.status IS NULL AND status IS NULL) OR (elem.status IS NOT NULL AND status IS NOT NULL AND elem.status = status)))",
		},
		{
			name:  "enum field left and scoped field right",
			logic: `{"some":[{"var":"items"},{"==":[{"var":"status"},{"var":"current.status"}]}]}`,
			want:  "WHERE EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE ((status IS NULL AND elem.status IS NULL) OR (status IS NOT NULL AND elem.status IS NOT NULL AND status = elem.status)))",
		},
	}

	for _, tc := range enumCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := enumTr.Transpile(tc.logic)
			if err != nil {
				t.Fatalf("Transpile() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("Transpile() = %q, want %q", got, tc.want)
			}

			gotParamSQL, gotParams, err := enumTr.TranspileParameterized(tc.logic)
			if err != nil {
				t.Fatalf("TranspileParameterized() error = %v", err)
			}
			if gotParamSQL != tc.want {
				t.Fatalf("TranspileParameterized() SQL = %q, want %q", gotParamSQL, tc.want)
			}
			if len(gotParams) != 0 {
				t.Fatalf("TranspileParameterized() params = %#v, want none", gotParams)
			}
		})
	}
}

func TestTranspiler_NullSafeFieldEquality_AllDialectsSchemaModesNestedConditions(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "a", Type: FieldTypeString},
		{Name: "b", Type: FieldTypeString},
		{Name: "c", Type: FieldTypeString},
		{Name: "d", Type: FieldTypeString},
		{Name: "e", Type: FieldTypeString},
		{Name: "f", Type: FieldTypeString},
		{Name: "items", Type: FieldTypeArray},
		{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"active", "inactive"}},
	})
	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-less"},
		{name: "schema-aware", schema: schema},
	}

	logic := `{"and":[{"or":[{"==":[{"var":"a"},{"var":"b"}]},{"!=":[{"var":"c"},{"var":"d"}]}]},{"!":{"===":[{"var":"e"},{"var":"f"}]}},{"all":[{"var":"items"},{"or":[{"==":[{"var":"current.left"},{"var":"current.right"}]},{"!==":[{"var":"current.status"},{"var":"status"}]}]}]}]}`

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			for _, d := range allDialects() {
				t.Run(d.String(), func(t *testing.T) {
					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect:               d,
						Schema:                mode.schema,
						NullSafeFieldEquality: true,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}

					out := runAllAPIVariants(t, tr, logic)
					if len(out.params) != 0 || len(out.condParams) != 0 {
						t.Fatalf("params = %#v condParams = %#v, want none", out.params, out.condParams)
					}

					assertContains(t, out.inlineSQL, "((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b))")
					assertContains(t, out.inlineSQL, "((c IS NULL AND d IS NOT NULL) OR (c IS NOT NULL AND d IS NULL) OR (c IS NOT NULL AND d IS NOT NULL AND c != d))")
					assertContains(t, out.inlineSQL, "NOT (((e IS NULL AND f IS NULL) OR (e IS NOT NULL AND f IS NOT NULL AND e = f)))")
					assertContains(t, out.inlineSQL, "((elem.left IS NULL AND elem.right IS NULL) OR (elem.left IS NOT NULL AND elem.right IS NOT NULL AND elem.left = elem.right))")
					assertContains(t, out.inlineSQL, "((elem.status IS NULL AND status IS NOT NULL) OR (elem.status IS NOT NULL AND status IS NULL) OR (elem.status IS NOT NULL AND status IS NOT NULL AND elem.status <> status))")

					if d == DialectClickHouse {
						assertContains(t, out.inlineSQL, "arrayAll(elem ->")
					} else {
						assertContains(t, out.inlineSQL, "NOT EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE NOT")
					}
					if out.paramSQL != out.inlineSQL {
						t.Fatalf("parameterized SQL = %q, want inline SQL %q", out.paramSQL, out.inlineSQL)
					}
				})
			}
		})
	}
}

func TestTranspiler_NullSafeFieldEquality_CustomOperatorInteraction(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "left", Type: FieldTypeString},
		{Name: "right", Type: FieldTypeString},
		{Name: "name", Type: FieldTypeString},
		{Name: "normalized_name", Type: FieldTypeString},
		{Name: "items", Type: FieldTypeArray},
	})
	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-less"},
		{name: "schema-aware", schema: schema},
	}

	logic := `{"and":[{"==":[{"var":"left"},{"var":"right"}]},{"==":[{"lower":[{"var":"name"}]},{"var":"normalized_name"}]},{"some":[{"var":"items"},{"==":[{"lower":[{"var":"current.code"}]},{"var":"current.normalized"}]}]}]}`

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			for _, d := range allDialects() {
				t.Run(d.String(), func(t *testing.T) {
					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect:               d,
						Schema:                mode.schema,
						NullSafeFieldEquality: true,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}
					registerNullSafeFieldEqualityCustomOperators(t, tr)

					out := runAllAPIVariants(t, tr, logic)
					if len(out.params) != 0 || len(out.condParams) != 0 {
						t.Fatalf("params = %#v condParams = %#v, want none", out.params, out.condParams)
					}

					assertContains(t, out.inlineSQL, "((left IS NULL AND right IS NULL) OR (left IS NOT NULL AND right IS NOT NULL AND left = right))")
					assertContains(t, out.inlineSQL, "LOWER(name) = normalized_name")
					assertContains(t, out.inlineSQL, "LOWER(elem.code) = elem.normalized")
					assertNotContains(t, out.inlineSQL, "LOWER(name) IS NULL")
					assertNotContains(t, out.inlineSQL, "LOWER(name) IS NOT NULL")
					assertNotContains(t, out.inlineSQL, "LOWER(elem.code) IS NULL")
					assertNotContains(t, out.inlineSQL, "LOWER(elem.code) IS NOT NULL")

					if d == DialectClickHouse {
						assertContains(t, out.inlineSQL, "arrayExists(elem -> LOWER(elem.code) = elem.normalized, items)")
					} else {
						assertContains(t, out.inlineSQL, "EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE LOWER(elem.code) = elem.normalized)")
					}
					if out.paramSQL != out.inlineSQL {
						t.Fatalf("parameterized SQL = %q, want inline SQL %q", out.paramSQL, out.inlineSQL)
					}
				})
			}
		})
	}
}

func registerNullSafeFieldEqualityCustomOperators(t *testing.T, tr *Transpiler) {
	t.Helper()
	if err := tr.RegisterOperatorFunc("lower", func(_ string, args []interface{}) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("lower expects 1 argument")
		}
		return fmt.Sprintf("LOWER(%v)", args[0]), nil
	}); err != nil {
		t.Fatalf("RegisterOperatorFunc(lower) error = %v", err)
	}
}

func TestTranspiler_Transpile(t *testing.T) {
	tr, _ := NewTranspiler(DialectBigQuery)

	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    `{">": [{"var": "amount"}, 1000]}`,
			expected: "WHERE amount > 1000",
			hasError: false,
		},
		{
			name:     "and operation",
			input:    `{"and": [{"==": [{"var": "status"}, "pending"]}, {">": [{"var": "amount"}, 5000]}]}`,
			expected: "WHERE (status = 'pending' AND amount > 5000)",
			hasError: false,
		},
		{
			name:     "or operation",
			input:    `{"or": [{">=": [{"var": "failedAttempts"}, 5]}, {"in": [{"var": "country"}, ["CN", "RU"]]}]}`,
			expected: "WHERE (failedAttempts >= 5 OR country IN ('CN', 'RU'))",
			hasError: false,
		},
		{
			name:     "nested conditions",
			input:    `{"and": [{">": [{"var": "transaction.amount"}, 10000]}, {"or": [{"==": [{"var": "user.verified"}, false]}, {"<": [{"var": "user.accountAgeDays"}, 7]}]}]}`,
			expected: "WHERE (transaction.amount > 10000 AND (user.verified = FALSE OR user.accountAgeDays < 7))",
			hasError: false,
		},
		{
			name:     "if operation",
			input:    `{"if": [{">": [{"var": "age"}, 18]}, "adult", "minor"]}`,
			expected: "WHERE CASE WHEN age > 18 THEN 'adult' ELSE 'minor' END",
			hasError: false,
		},
		{
			name:     "missing operation",
			input:    `{"missing": "field"}`,
			expected: "WHERE field IS NULL",
			hasError: false,
		},
		{
			name:     "missing_some operation",
			input:    `{"missing_some": [1, ["field1", "field2"]]}`,
			expected: "WHERE (field1 IS NULL OR field2 IS NULL)",
			hasError: false,
		},
		{
			name:     "invalid JSON",
			input:    `{invalid json}`,
			expected: "",
			hasError: true,
		},
		{
			name:     "unsupported operator",
			input:    `{"unsupported": [1, 2]}`,
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.Transpile(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Transpile() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Transpile() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("Transpile() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestTranspiler_InStringExpressionContainment_NoSchema(t *testing.T) {
	tests := []struct {
		name      string
		dialect   Dialect
		jsonLogic string
		wantSQL   string
	}{
		{
			name:      "bigquery",
			dialect:   DialectBigQuery,
			jsonLogic: `{"in": [{"cat": [{"substr": [{"var": "profile.first"}, 0, 2]}, "-x"]}, {"var": "profile.name"}]}`,
			wantSQL:   "WHERE STRPOS(profile.name, CONCAT(SUBSTR(profile.first, 1, 2), '-x')) > 0",
		},
		{
			name:      "spanner",
			dialect:   DialectSpanner,
			jsonLogic: `{"in": [{"cat": [{"substr": [{"var": "profile.first"}, 0, 2]}, "-x"]}, {"var": "profile.name"}]}`,
			wantSQL:   "WHERE STRPOS(profile.name, CONCAT(SUBSTR(profile.first, 1, 2), '-x')) > 0",
		},
		{
			name:      "postgresql",
			dialect:   DialectPostgreSQL,
			jsonLogic: `{"in": [{"cat": [{"substr": [{"var": "profile.first"}, 0, 2]}, "-x"]}, {"var": "profile.name"}]}`,
			wantSQL:   "WHERE POSITION(CONCAT(SUBSTR(profile.first, 1, 2), '-x') IN profile.name) > 0",
		},
		{
			name:      "duckdb",
			dialect:   DialectDuckDB,
			jsonLogic: `{"in": [{"cat": [{"substr": [{"var": "profile.first"}, 0, 2]}, "-x"]}, {"var": "profile.name"}]}`,
			wantSQL:   "WHERE STRPOS(profile.name, CONCAT(SUBSTR(profile.first, 1, 2), '-x')) > 0",
		},
		{
			name:      "clickhouse",
			dialect:   DialectClickHouse,
			jsonLogic: `{"in": [{"cat": [{"substr": [{"var": "profile.first"}, 0, 2]}, "-x"]}, {"var": "profile.name"}]}`,
			wantSQL:   "WHERE position(profile.name, CONCAT(substring(profile.first, 1, 2), '-x')) > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := NewTranspiler(tt.dialect)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			gotSQL, err := tr.Transpile(tt.jsonLogic)
			if err != nil {
				t.Fatalf("Transpile() error = %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("Transpile() = %q, want %q", gotSQL, tt.wantSQL)
			}
		})
	}
}

func TestTranspiler_TranspileFromMap(t *testing.T) {
	tr, _ := NewTranspiler(DialectBigQuery)

	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "WHERE amount > 1000",
			hasError: false,
		},
		{
			name:     "and operation",
			input:    map[string]interface{}{"and": []interface{}{map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "pending"}}, map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 5000}}}},
			expected: "WHERE (status = 'pending' AND amount > 5000)",
			hasError: false,
		},
		{
			name:     "unsupported operator",
			input:    map[string]interface{}{"unsupported": []interface{}{1, 2}},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.TranspileFromMap(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("TranspileFromMap() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("TranspileFromMap() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileFromMap() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestTranspiler_TranspileFromInterface(t *testing.T) {
	tr, _ := NewTranspiler(DialectBigQuery)

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "WHERE amount > 1000",
			hasError: false,
		},
		{
			name:     "primitive value",
			input:    "hello",
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.TranspileFromInterface(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("TranspileFromInterface() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("TranspileFromInterface() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileFromInterface() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestTranspileFromMap_RejectsInvalidJSONNumberLiterals(t *testing.T) {
	tr, _ := NewTranspiler(DialectBigQuery)

	tests := []struct {
		name  string
		input map[string]interface{}
	}{
		{
			name: "comparison literal injection",
			input: map[string]interface{}{
				"==": []interface{}{
					map[string]interface{}{"var": "x"},
					json.Number("1 OR 1=1"),
				},
			},
		},
		{
			name: "numeric expression literal injection",
			input: map[string]interface{}{
				"+": []interface{}{
					json.Number("1 OR 1=1"),
					1,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tr.TranspileFromMap(tt.input)
			if err == nil {
				t.Fatal("expected error for invalid json.Number literal")
			}
		})
	}
}

func TestTranspileFromInterface_RejectsInvalidJSONNumberLiterals(t *testing.T) {
	tr, _ := NewTranspiler(DialectBigQuery)

	logic := map[string]interface{}{
		"==": []interface{}{
			map[string]interface{}{"var": "x"},
			json.Number("1 OR 1=1"),
		},
	}

	_, err := tr.TranspileFromInterface(logic)
	if err == nil {
		t.Fatal("expected error for invalid json.Number literal")
	}
}

func TestTranspileFromMap_SchemaEqualityRejectsInvalidJSONNumberBeforeFold(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "code", Type: FieldTypeString},
		{Name: "amount", Type: FieldTypeInteger},
	})
	tr, _ := NewTranspiler(DialectBigQuery)
	tr.SetSchema(schema)

	tests := []struct {
		name  string
		input map[string]interface{}
	}{
		{
			name: "malformed json number literal before strict fold",
			input: map[string]interface{}{
				"!==": []interface{}{
					map[string]interface{}{"var": "code"},
					json.Number("0 OR 1=1"),
				},
			},
		},
		{
			name: "malformed json number default before strict fold",
			input: map[string]interface{}{
				"!==": []interface{}{
					map[string]interface{}{"var": []interface{}{"amount", json.Number("0 OR 1=1")}},
					"abc",
				},
			},
		},
		{
			name: "malformed json number default before loose fold",
			input: map[string]interface{}{
				"!=": []interface{}{
					map[string]interface{}{"var": []interface{}{"amount", json.Number("0 OR 1=1")}},
					"abc",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tr.TranspileFromMap(tt.input); err == nil {
				t.Fatal("TranspileFromMap() expected error for invalid json.Number")
			}
			if _, err := tr.TranspileFromInterface(tt.input); err == nil {
				t.Fatal("TranspileFromInterface() expected error for invalid json.Number")
			}
			if _, _, err := tr.TranspileParameterizedFromInterface(tt.input); err == nil {
				t.Fatal("TranspileParameterizedFromInterface() expected error for invalid json.Number")
			}
		})
	}
}

func TestTranspile_SchemaEqualityValidatesEnumDefaultsForVarOperands(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"active"}},
		{Name: "other", Type: FieldTypeString},
	})
	tr, _ := NewTranspiler(DialectBigQuery)
	tr.SetSchema(schema)

	tests := []struct {
		name  string
		logic string
	}{
		{
			name:  "invalid enum default on left var",
			logic: `{"==":[{"var":["status","bogus"]},{"var":"other"}]}`,
		},
		{
			name:  "invalid enum default on right var",
			logic: `{"==":[{"var":"other"},{"var":["status","bogus"]}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tr.Transpile(tt.logic); err == nil {
				t.Fatal("Transpile() expected error for invalid enum default")
			}
			if _, _, err := tr.TranspileParameterized(tt.logic); err == nil {
				t.Fatal("TranspileParameterized() expected error for invalid enum default")
			}
		})
	}
}

func TestTranspileFromInterface_SchemaEqualityPreservesFloat32ForEnum(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"1.2"}},
	})
	tr, _ := NewTranspiler(DialectBigQuery)
	tr.SetSchema(schema)
	logic := map[string]interface{}{
		"==": []interface{}{
			map[string]interface{}{"var": "status"},
			float32(1.2),
		},
	}

	gotSQL, err := tr.TranspileFromInterface(logic)
	if err != nil {
		t.Fatalf("TranspileFromInterface() error = %v", err)
	}
	if wantSQL := "WHERE status = '1.2'"; gotSQL != wantSQL {
		t.Fatalf("TranspileFromInterface() SQL = %q, want %q", gotSQL, wantSQL)
	}

	gotParamSQL, gotParams, err := tr.TranspileParameterizedFromInterface(logic)
	if err != nil {
		t.Fatalf("TranspileParameterizedFromInterface() error = %v", err)
	}
	if wantSQL := "WHERE status = @p1"; gotParamSQL != wantSQL {
		t.Fatalf("TranspileParameterizedFromInterface() SQL = %q, want %q", gotParamSQL, wantSQL)
	}
	wantParams := []QueryParam{{Name: "p1", Value: "1.2"}}
	if !reflect.DeepEqual(gotParams, wantParams) {
		t.Fatalf("TranspileParameterizedFromInterface() params = %v, want %v", gotParams, wantParams)
	}
}

func TestTranspileFromInterface_SchemaNumberStrictEqualityFoldsNonFinite(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "score", Type: FieldTypeNumber},
	})
	tr, _ := NewTranspiler(DialectBigQuery)
	tr.SetSchema(schema)

	tests := []struct {
		name    string
		op      string
		value   float64
		wantSQL string
	}{
		{name: "strict NaN", op: "===", value: math.NaN(), wantSQL: "WHERE FALSE"},
		{name: "strict not NaN", op: "!==", value: math.NaN(), wantSQL: "WHERE TRUE"},
		{name: "strict infinity", op: "===", value: math.Inf(1), wantSQL: "WHERE FALSE"},
		{name: "strict not infinity", op: "!==", value: math.Inf(1), wantSQL: "WHERE TRUE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logic := map[string]interface{}{
				tt.op: []interface{}{
					map[string]interface{}{"var": "score"},
					tt.value,
				},
			}

			gotSQL, err := tr.TranspileFromInterface(logic)
			if err != nil {
				t.Fatalf("TranspileFromInterface() error = %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Fatalf("TranspileFromInterface() SQL = %q, want %q", gotSQL, tt.wantSQL)
			}

			gotParamSQL, gotParams, err := tr.TranspileParameterizedFromInterface(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedFromInterface() error = %v", err)
			}
			if gotParamSQL != tt.wantSQL {
				t.Fatalf("TranspileParameterizedFromInterface() SQL = %q, want %q", gotParamSQL, tt.wantSQL)
			}
			if len(gotParams) != 0 {
				t.Fatalf("TranspileParameterizedFromInterface() params = %v, want none", gotParams)
			}
		})
	}
}

func TestTranspile_SchemaStringEqualityCanonicalizesNonFiniteNumbers(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "code", Type: FieldTypeString},
		{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"Infinity", "-Infinity"}},
		{Name: "limited_status", Type: FieldTypeEnum, AllowedValues: []string{"active"}},
	})
	tr, _ := NewTranspiler(DialectBigQuery)
	tr.SetSchema(schema)

	jsonSQL, err := tr.Transpile(`{"==": [{"var": "code"}, 1e400]}`)
	if err != nil {
		t.Fatalf("Transpile() error = %v", err)
	}
	if want := "WHERE code = 'Infinity'"; jsonSQL != want {
		t.Fatalf("Transpile() SQL = %q, want %q", jsonSQL, want)
	}

	jsonParamSQL, jsonParams, err := tr.TranspileParameterized(`{"==": [{"var": "code"}, -1e400]}`)
	if err != nil {
		t.Fatalf("TranspileParameterized() error = %v", err)
	}
	if want := "WHERE code = @p1"; jsonParamSQL != want {
		t.Fatalf("TranspileParameterized() SQL = %q, want %q", jsonParamSQL, want)
	}
	if want := []QueryParam{{Name: "p1", Value: "-Infinity"}}; !reflect.DeepEqual(jsonParams, want) {
		t.Fatalf("TranspileParameterized() params = %v, want %v", jsonParams, want)
	}

	interfaceSQL, interfaceParams, err := tr.TranspileParameterizedFromInterface(map[string]interface{}{
		"!=": []interface{}{map[string]interface{}{"var": "code"}, math.Inf(1)},
	})
	if err != nil {
		t.Fatalf("TranspileParameterizedFromInterface() error = %v", err)
	}
	if want := "WHERE code != @p1"; interfaceSQL != want {
		t.Fatalf("TranspileParameterizedFromInterface() SQL = %q, want %q", interfaceSQL, want)
	}
	if want := []QueryParam{{Name: "p1", Value: "Infinity"}}; !reflect.DeepEqual(interfaceParams, want) {
		t.Fatalf("TranspileParameterizedFromInterface() params = %v, want %v", interfaceParams, want)
	}

	enumSQL, err := tr.TranspileFromInterface(map[string]interface{}{
		"==": []interface{}{map[string]interface{}{"var": "status"}, math.Inf(1)},
	})
	if err != nil {
		t.Fatalf("TranspileFromInterface() enum error = %v", err)
	}
	if want := "WHERE status = 'Infinity'"; enumSQL != want {
		t.Fatalf("TranspileFromInterface() enum SQL = %q, want %q", enumSQL, want)
	}

	_, err = tr.TranspileFromInterface(map[string]interface{}{
		"==": []interface{}{map[string]interface{}{"var": "limited_status"}, math.Inf(1)},
	})
	if err == nil {
		t.Fatal("TranspileFromInterface() expected enum validation error")
	}
}

func TestTranspileFromMap_CustomOperatorRejectsInvalidJSONNumberLiterals(t *testing.T) {
	tr, _ := NewTranspiler(DialectBigQuery)
	_ = tr.RegisterOperatorFunc("identity", func(_ string, args []interface{}) (string, error) {
		return args[0].(string), nil
	})

	logic := map[string]interface{}{
		"identity": []interface{}{json.Number("1 OR 1=1")},
	}
	_, err := tr.TranspileFromMap(logic)
	if err == nil {
		t.Fatal("expected error for invalid json.Number literal in custom operator input")
	}
}

func TestTranspile(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    `{">": [{"var": "amount"}, 1000]}`,
			expected: "WHERE amount > 1000",
			hasError: false,
		},
		{
			name:     "invalid JSON",
			input:    `{invalid json}`,
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Transpile(DialectBigQuery, tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Transpile() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Transpile() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("Transpile() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestTranspileFromMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "WHERE amount > 1000",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TranspileFromMap(DialectBigQuery, tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("TranspileFromMap() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("TranspileFromMap() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileFromMap() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestTranspileFromInterface(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "WHERE amount > 1000",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TranspileFromInterface(DialectBigQuery, tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("TranspileFromInterface() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("TranspileFromInterface() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileFromInterface() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

// Test all JSON Logic operators comprehensively.
func TestAllOperators(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		// Data Access Operations
		{
			name:     "var simple",
			input:    `{"var": "name"}`,
			expected: "WHERE name",
			hasError: false,
		},
		{
			name:     "var with default",
			input:    `{"var": ["status", "pending"]}`,
			expected: "WHERE COALESCE(status, 'pending')",
			hasError: false,
		},
		{
			name:     "missing field",
			input:    `{"missing": "email"}`,
			expected: "WHERE email IS NULL",
			hasError: false,
		},
		{
			name:     "missing some fields",
			input:    `{"missing_some": [1, ["field1", "field2"]]}`,
			expected: "WHERE (field1 IS NULL OR field2 IS NULL)",
			hasError: false,
		},
		{
			name:     "missing with array of fields",
			input:    `{"missing": ["email", "phone", "address"]}`,
			expected: "WHERE (email IS NULL OR phone IS NULL OR address IS NULL)",
			hasError: false,
		},

		// Logic and Boolean Operations
		{
			name:     "equality",
			input:    `{"==": [{"var": "status"}, "active"]}`,
			expected: "WHERE status = 'active'",
			hasError: false,
		},
		{
			name:     "equality with null",
			input:    `{"==": [{"var": "deleted_at"}, null]}`,
			expected: "WHERE deleted_at IS NULL",
			hasError: false,
		},
		{
			name:     "inequality with null",
			input:    `{"!=": [{"var": "field"}, null]}`,
			expected: "WHERE field IS NOT NULL",
			hasError: false,
		},
		{
			name:     "strict equality",
			input:    `{"===": [{"var": "status"}, "active"]}`,
			expected: "WHERE status = 'active'",
			hasError: false,
		},
		{
			name:     "inequality",
			input:    `{"!=": [{"var": "status"}, "inactive"]}`,
			expected: "WHERE status != 'inactive'",
			hasError: false,
		},
		{
			name:     "strict inequality",
			input:    `{"!==": [{"var": "count"}, 0]}`,
			expected: "WHERE count <> 0",
			hasError: false,
		},
		{
			name:     "logical not",
			input:    `{"!": [{"var": "isDeleted"}]}`,
			expected: "WHERE NOT (isDeleted)",
			hasError: false,
		},
		{
			name:     "double negation",
			input:    `{"!!": [{"var": "value"}]}`,
			expected: "WHERE (value IS NOT NULL AND value != FALSE AND value != 0 AND value != '')",
			hasError: false,
		},
		{
			name:     "logical and",
			input:    `{"and": [{"==": [{"var": "status"}, "active"]}, {">": [{"var": "score"}, 100]}]}`,
			expected: "WHERE (status = 'active' AND score > 100)",
			hasError: false,
		},
		{
			name:     "logical or",
			input:    `{"or": [{"==": [{"var": "role"}, "admin"]}, {"==": [{"var": "role"}, "user"]}]}`,
			expected: "WHERE (role = 'admin' OR role = 'user')",
			hasError: false,
		},
		{
			name:     "conditional if",
			input:    `{"if": [{">": [{"var": "age"}, 18]}, "adult", "minor"]}`,
			expected: "WHERE CASE WHEN age > 18 THEN 'adult' ELSE 'minor' END",
			hasError: false,
		},

		// Numeric Operations
		{
			name:     "greater than",
			input:    `{">": [{"var": "amount"}, 1000]}`,
			expected: "WHERE amount > 1000",
			hasError: false,
		},
		{
			name:     "greater than or equal",
			input:    `{">=": [{"var": "score"}, 80]}`,
			expected: "WHERE score >= 80",
			hasError: false,
		},
		{
			name:     "less than",
			input:    `{"<": [{"var": "age"}, 65]}`,
			expected: "WHERE age < 65",
			hasError: false,
		},
		{
			name:     "less than or equal",
			input:    `{"<=": [{"var": "count"}, 10]}`,
			expected: "WHERE count <= 10",
			hasError: false,
		},
		{
			name:     "chained less than (between exclusive)",
			input:    `{"<": [0, {"var": "temp"}, 100]}`,
			expected: "WHERE (0 < temp AND temp < 100)",
			hasError: false,
		},
		{
			name:     "chained less than or equal (between inclusive)",
			input:    `{"<=": [0, {"var": "score"}, 100]}`,
			expected: "WHERE (0 <= score AND score <= 100)",
			hasError: false,
		},
		{
			name:     "max",
			input:    `{"max": [{"var": "score1"}, {"var": "score2"}, {"var": "score3"}]}`,
			expected: "WHERE GREATEST(score1, score2, score3)",
			hasError: false,
		},
		{
			name:     "min",
			input:    `{"min": [{"var": "price1"}, {"var": "price2"}]}`,
			expected: "WHERE LEAST(price1, price2)",
			hasError: false,
		},
		{
			name:     "addition",
			input:    `{"+": [{"var": "price"}, {"var": "tax"}]}`,
			expected: "WHERE (price + tax)",
			hasError: false,
		},
		{
			name:     "subtraction",
			input:    `{"-": [{"var": "total"}, {"var": "discount"}]}`,
			expected: "WHERE (total - discount)",
			hasError: false,
		},
		{
			name:     "multiplication",
			input:    `{"*": [{"var": "price"}, 1.2]}`,
			expected: "WHERE (price * 1.2)",
			hasError: false,
		},
		{
			name:     "division",
			input:    `{"/": [{"var": "total"}, 2]}`,
			expected: "WHERE (total / 2)",
			hasError: false,
		},
		{
			name:     "modulo",
			input:    `{"%": [{"var": "count"}, 3]}`,
			expected: "WHERE (count % 3)",
			hasError: false,
		},

		// Array Operations
		{
			name:     "in array",
			input:    `{"in": [{"var": "country"}, ["US", "CA", "MX"]]}`,
			expected: "WHERE country IN ('US', 'CA', 'MX')",
			hasError: false,
		},
		{
			name:     "map array",
			input:    `{"map": [{"var": "numbers"}, {"+": [{"var": "item"}, 1]}]}`,
			expected: "WHERE ARRAY(SELECT (elem + 1) FROM UNNEST(numbers) AS elem)",
			hasError: false,
		},
		{
			name:     "filter array",
			input:    `{"filter": [{"var": "scores"}, {">": [{"var": "item"}, 70]}]}`,
			expected: "WHERE ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem > 70)",
			hasError: false,
		},
		{
			name:     "reduce array",
			input:    `{"reduce": [{"var": "numbers"}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}`,
			expected: "WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST(numbers) AS elem), 0)",
			hasError: false,
		},
		{
			name:     "all elements",
			input:    `{"all": [{"var": "ages"}, {">=": [{"var": "item"}, 18]}]}`,
			expected: "WHERE (ARRAY_LENGTH(ages) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18)))",
			hasError: false,
		},
		{
			name:     "some elements",
			input:    `{"some": [{"var": "statuses"}, {"==": [{"var": "item"}, "active"]}]}`,
			expected: "WHERE EXISTS (SELECT 1 FROM UNNEST(statuses) AS elem WHERE elem = 'active')",
			hasError: false,
		},
		{
			name:     "none elements",
			input:    `{"none": [{"var": "values"}, {"==": [{"var": "item"}, "invalid"]}]}`,
			expected: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE elem = 'invalid')",
			hasError: false,
		},
		{
			name:     "merge arrays",
			input:    `{"merge": [{"var": "array1"}, {"var": "array2"}]}`,
			expected: "WHERE ARRAY_CONCAT(array1, array2)",
			hasError: false,
		},

		// String Operations
		{
			name:     "concatenate strings",
			input:    `{"cat": [{"var": "firstName"}, " ", {"var": "lastName"}]}`,
			expected: "WHERE CONCAT(firstName, ' ', lastName)",
			hasError: false,
		},
		{
			name:     "substring",
			input:    `{"substr": [{"var": "text"}, 0, 5]}`,
			expected: "WHERE SUBSTR(text, 1, 5)",
			hasError: false,
		},

		// Complex Nested Examples
		{
			name:     "nested conditions",
			input:    `{"and": [{">": [{"var": "transaction.amount"}, 10000]}, {"or": [{"==": [{"var": "user.verified"}, false]}, {"<": [{"var": "user.accountAgeDays"}, 7]}]}]}`,
			expected: "WHERE (transaction.amount > 10000 AND (user.verified = FALSE OR user.accountAgeDays < 7))",
			hasError: false,
		},
		{
			name:     "complex conditional",
			input:    `{"if": [{"and": [{">=": [{"var": "age"}, 18]}, {"==": [{"var": "country"}, "US"]}]}, "eligible", "ineligible"]}`,
			expected: "WHERE CASE WHEN (age >= 18 AND country = 'US') THEN 'eligible' ELSE 'ineligible' END",
			hasError: false,
		},
		{
			name:     "multiple numeric operations",
			input:    `{"and": [{">": [{"var": "totalPrice"}, 100]}, {"<": [{"var": "totalQuantity"}, 1000]}]}`,
			expected: "WHERE (totalPrice > 100 AND totalQuantity < 1000)",
			hasError: false,
		},
		{
			name:     "mixed operations",
			input:    `{"and": [{"in": [{"var": "status"}, ["active", "pending"]]}, {"!": [{"missing": "email"}]}, {">=": [{"var": "score"}, 80]}]}`,
			expected: "WHERE (status IN ('active', 'pending') AND NOT (email IS NULL) AND score >= 80)",
			hasError: false,
		},

		// Error Cases
		{
			name:     "unsupported operator",
			input:    `{"unsupported": [1, 2]}`,
			expected: "",
			hasError: true,
		},
		{
			name:     "invalid JSON",
			input:    `{invalid json}`,
			expected: "",
			hasError: true,
		},
		{
			name:     "empty input",
			input:    ``,
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Transpile(DialectBigQuery, tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Transpile() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Transpile() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("Transpile() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

// TestComprehensiveNestedExpressions tests deeply nested and complex expressions.
func TestComprehensiveNestedExpressions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "nested reduce in comparison",
			input:    `{">": [{"reduce": [{"filter": [{"var": "cars"}, {"==": [{"var": "vendor"}, "Toyota"]}]}, {"+": [1, {"var": "accumulator"}]}, 0]}, 2]}`,
			expected: "WHERE (SELECT (1 + 0) FROM UNNEST(ARRAY(SELECT elem FROM UNNEST(cars) AS elem WHERE vendor = 'Toyota')) AS elem) > 2",
			hasError: false,
		},
		{
			name:     "nested filter in reduce",
			input:    `{"reduce": [{"filter": [{"var": "items"}, {">": [{"var": "price"}, 100]}]}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}`,
			expected: "WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST(ARRAY(SELECT elem FROM UNNEST(items) AS elem WHERE price > 100)) AS elem), 0)",
			hasError: false,
		},
		{
			name:     "nested some in and",
			input:    `{"and": [{"==": [{"var": "status"}, "active"]}, {"some": [{"var": "results"}, {"and": [{"==": [{"var": "product"}, "abc"]}, {">": [{"var": "score"}, 8]}]}]}]}`,
			expected: "WHERE (status = 'active' AND EXISTS (SELECT 1 FROM UNNEST(results) AS elem WHERE (product = 'abc' AND score > 8)))",
			hasError: false,
		},
		{
			name:     "complex nested expression",
			input:    `{"and": [{"==": [{"var": "color2"}, "orange"]}, {"==": [{"var": "slider"}, 35]}, {"some": [{"var": "results"}, {"and": [{"==": [{"var": "product"}, "abc"]}, {">": [{"var": "score"}, 8]}]}]}, {">": [{"reduce": [{"filter": [{"var": "cars"}, {"and": [{"==": [{"var": "vendor"}, "Toyota"]}, {">=": [{"var": "year"}, 2010]}]}]}, {"+": [1, {"var": "accumulator"}]}, 0]}, 2]}]}`,
			expected: "WHERE (color2 = 'orange' AND slider = 35 AND EXISTS (SELECT 1 FROM UNNEST(results) AS elem WHERE (product = 'abc' AND score > 8)) AND (SELECT (1 + 0) FROM UNNEST(ARRAY(SELECT elem FROM UNNEST(cars) AS elem WHERE (vendor = 'Toyota' AND year >= 2010))) AS elem) > 2)",
			hasError: false,
		},
		{
			name:     "nested comparison in filter",
			input:    `{"filter": [{"var": "products"}, {"and": [{">": [{"var": "price"}, 100]}, {"<": [{"var": "price"}, 1000]}]}]}`,
			expected: "WHERE ARRAY(SELECT elem FROM UNNEST(products) AS elem WHERE (price > 100 AND price < 1000))",
			hasError: false,
		},
		{
			name:     "nested arithmetic in reduce",
			input:    `{"reduce": [{"var": "numbers"}, {"+": [{"var": "accumulator"}, {"*": [{"var": "current"}, 2]}]}, 0]}`,
			expected: "WHERE (SELECT (0 + (elem * 2)) FROM UNNEST(numbers) AS elem)",
			hasError: false,
		},
		{
			name:     "nested logical in some",
			input:    `{"some": [{"var": "items"}, {"or": [{"==": [{"var": "status"}, "active"]}, {">": [{"var": "priority"}, 5]}]}]}`,
			expected: "WHERE EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE (status = 'active' OR priority > 5))",
			hasError: false,
		},
		{
			name:     "nested all in comparison",
			input:    `{">": [{"all": [{"var": "scores"}, {">=": [{"var": "elem"}, 70]}]}, true]}`,
			expected: "WHERE (ARRAY_LENGTH(scores) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(scores) AS elem WHERE NOT (elem >= 70))) > TRUE",
			hasError: false,
		},
		{
			name:     "deeply nested reduce filter",
			input:    `{"reduce": [{"filter": [{"var": "data"}, {"and": [{"some": [{"var": "tags"}, {"==": [{"var": "elem"}, "important"]}]}, {">": [{"var": "value"}, 0]}]}]}, {"+": [{"var": "accumulator"}, {"reduce": [{"var": "current.subitems"}, {"+": [{"var": "acc"}, {"var": "item"}]}, 0]}]}, 0]}`,
			expected: "WHERE (SELECT (0 + (SELECT (acc + elem) FROM UNNEST(elem.subitems) AS elem)) FROM UNNEST(ARRAY(SELECT elem FROM UNNEST(data) AS elem WHERE (EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE elem = 'important') AND value > 0))) AS elem)",
			hasError: false,
		},
		{
			name:     "nested map in comparison",
			input:    `{">": [{"map": [{"var": "numbers"}, {"*": [{"var": "elem"}, 2]}]}, 10]}`,
			expected: "WHERE ARRAY(SELECT (elem * 2) FROM UNNEST(numbers) AS elem) > 10",
			hasError: false,
		},
		{
			name:     "nested comparison in numeric",
			input:    `{"+": [{">": [{"var": "a"}, 5]}, {"<": [{"var": "b"}, 10]}]}`,
			expected: "WHERE ((a > 5) + (b < 10))",
			hasError: false,
		},
		{
			name:     "nested var in arithmetic",
			input:    `{"+": [{"var": "x"}, {"*": [{"var": "y"}, {"var": "z"}]}]}`,
			expected: "WHERE (x + (y * z))",
			hasError: false,
		},
		{
			name:     "nested if in comparison",
			input:    `{">": [{"if": [{">": [{"var": "x"}, 0]}, {"var": "positive"}, {"var": "negative"}]}, 0]}`,
			expected: "WHERE CASE WHEN x > 0 THEN positive ELSE negative END > 0",
			hasError: false,
		},
		{
			name:     "nested comparison in logical",
			input:    `{"and": [{">": [{"var": "a"}, 1]}, {"<": [{"var": "b"}, 10]}, {"==": [{"var": "c"}, "test"]}]}`,
			expected: "WHERE (a > 1 AND b < 10 AND c = 'test')",
			hasError: false,
		},
		{
			name:     "nested reduce with complex expression",
			input:    `{"reduce": [{"var": "items"}, {"+": [{"var": "accumulator"}, {"*": [{"var": "current.price"}, {"if": [{">": [{"var": "current.discount"}, 0]}, {"-": [1, {"var": "current.discount"}]}, 1]}]}]}, 0]}`,
			expected: "WHERE (SELECT (0 + (elem.price * CASE WHEN elem.discount > 0 THEN (1 - elem.discount) ELSE 1 END)) FROM UNNEST(items) AS elem)",
			hasError: false,
		},
		{
			name:     "nested filter with or",
			input:    `{"filter": [{"var": "users"}, {"or": [{">=": [{"var": "age"}, 18]}, {"==": [{"var": "role"}, "admin"]}]}]}`,
			expected: "WHERE ARRAY(SELECT elem FROM UNNEST(users) AS elem WHERE (age >= 18 OR role = 'admin'))",
			hasError: false,
		},
		{
			name:     "nested some with comparison",
			input:    `{"some": [{"var": "items"}, {">": [{"+": [{"var": "price"}, {"var": "tax"}]}, 100]}]}`,
			expected: "WHERE EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE (price + tax) > 100)",
			hasError: false,
		},
		{
			name:     "nested all with nested comparison",
			input:    `{"all": [{"var": "scores"}, {"and": [{">=": [{"var": "elem"}, 0]}, {"<=": [{"var": "elem"}, 100]}]}]}`,
			expected: "WHERE (ARRAY_LENGTH(scores) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(scores) AS elem WHERE NOT ((elem >= 0 AND elem <= 100))))",
			hasError: false,
		},
		{
			name:     "nested none with complex",
			input:    `{"none": [{"var": "errors"}, {"or": [{"==": [{"var": "elem.type"}, "critical"]}, {">": [{"var": "elem.count"}, 10]}]}]}`,
			expected: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(errors) AS elem WHERE (elem.type = 'critical' OR elem.count > 10))",
			hasError: false,
		},
		{
			name:     "very deeply nested",
			input:    `{"and": [{"some": [{"filter": [{"var": "data"}, {">": [{"var": "value"}, 0]}]}, {"all": [{"var": "elem.items"}, {">=": [{"var": "elem.score"}, 50]}]}]}, {">": [{"reduce": [{"var": "totals"}, {"+": [{"var": "accumulator"}, {"*": [{"var": "current"}, {"if": [{">": [{"var": "current"}, 100]}, 2, 1]}]}]}, 0]}, 1000]}]}`,
			expected: "WHERE (EXISTS (SELECT 1 FROM UNNEST(ARRAY(SELECT elem FROM UNNEST(data) AS elem WHERE value > 0)) AS elem WHERE (ARRAY_LENGTH(elem.items) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(elem.items) AS elem WHERE NOT (elem.score >= 50)))) AND (SELECT (0 + (elem * CASE WHEN elem > 100 THEN 2 ELSE 1 END)) FROM UNNEST(totals) AS elem) > 1000)",
			hasError: false,
		},
		{
			name:     "multiple nested if conditions",
			input:    `{"if": [{"and": [{">": [{"var": "age"}, 18]}, {"==": [{"var": "country"}, "US"]}]}, {"if": [{">": [{"var": "score"}, 80]}, "excellent", "good"]}, "not eligible"]}`,
			expected: "WHERE CASE WHEN (age > 18 AND country = 'US') THEN CASE WHEN score > 80 THEN 'excellent' ELSE 'good' END ELSE 'not eligible' END",
			hasError: false,
		},
		{
			name:     "nested arithmetic with multiple operations",
			input:    `{"+": [{"*": [{"var": "price"}, {"var": "quantity"}]}, {"-": [{"var": "discount"}, {"%": [{"var": "tax"}, 10]}]}]}`,
			expected: "WHERE ((price * quantity) + (discount - (tax % 10)))",
			hasError: false,
		},
		{
			name:     "complex array operations",
			input:    `{"and": [{"some": [{"var": "items"}, {">": [{"var": "price"}, 100]}]}, {"all": [{"var": "tags"}, {"in": [{"var": "elem"}, ["important", "urgent"]]}]}]}`,
			expected: "WHERE (EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE price > 100) AND (ARRAY_LENGTH(tags) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem IN ('important', 'urgent')))))",
			hasError: false,
		},
		{
			name:     "nested string operations",
			input:    `{"==": [{"cat": [{"var": "firstName"}, " ", {"var": "lastName"}]}, "John Doe"]}`,
			expected: "WHERE CONCAT(firstName, ' ', lastName) = 'John Doe'",
			hasError: false,
		},
		{
			name:     "chained comparisons with variables",
			input:    `{"<": [{"var": "min"}, {"var": "value"}, {"var": "max"}]}`,
			expected: "WHERE (min < value AND value < max)",
			hasError: false,
		},
		{
			name:     "nested missing operations",
			input:    `{"and": [{"!": [{"missing": "email"}]}, {"missing_some": [1, ["phone", "address"]]}]}`,
			expected: "WHERE (NOT (email IS NULL) AND (phone IS NULL OR address IS NULL))",
			hasError: false,
		},
		// Additional NULL comparison edge cases
		{
			name:     "strict equality with null",
			input:    `{"===": [{"var": "field"}, null]}`,
			expected: "WHERE field IS NULL",
			hasError: false,
		},
		{
			name:     "null on left side of equality",
			input:    `{"==": [null, {"var": "field"}]}`,
			expected: "WHERE field IS NULL",
			hasError: false,
		},
		{
			name:     "null on left side of inequality",
			input:    `{"!=": [null, {"var": "field"}]}`,
			expected: "WHERE field IS NOT NULL",
			hasError: false,
		},
		{
			name:     "both null equality",
			input:    `{"==": [null, null]}`,
			expected: "WHERE NULL IS NULL",
			hasError: false,
		},
		{
			name:     "both null inequality",
			input:    `{"!=": [null, null]}`,
			expected: "WHERE NULL IS NOT NULL",
			hasError: false,
		},
		// Missing operator edge cases
		{
			name:     "missing with single element array",
			input:    `{"missing": ["email"]}`,
			expected: "WHERE (email IS NULL)",
			hasError: false,
		},
		{
			name:     "missing with two fields",
			input:    `{"missing": ["email", "phone"]}`,
			expected: "WHERE (email IS NULL OR phone IS NULL)",
			hasError: false,
		},
		{
			name:     "NOT with missing array",
			input:    `{"!": [{"missing": ["email", "phone"]}]}`,
			expected: "WHERE NOT ((email IS NULL OR phone IS NULL))",
			hasError: false,
		},
		// Complex NULL scenarios
		{
			name:     "null comparison in and",
			input:    `{"and": [{"==": [{"var": "deleted_at"}, null]}, {"!=": [{"var": "archived_at"}, null]}]}`,
			expected: "WHERE (deleted_at IS NULL AND archived_at IS NOT NULL)",
			hasError: false,
		},
		{
			name:     "null comparison in or",
			input:    `{"or": [{"==": [{"var": "field1"}, null]}, {"==": [{"var": "field2"}, null]}]}`,
			expected: "WHERE (field1 IS NULL OR field2 IS NULL)",
			hasError: false,
		},
		{
			name:     "null comparison with var default",
			input:    `{"==": [{"var": ["deleted_at", null]}, null]}`,
			expected: "WHERE COALESCE(deleted_at, NULL) IS NULL",
			hasError: false,
		},
		// Edge case: NULL in arithmetic (should error or handle gracefully)
		{
			name:     "null in arithmetic should handle",
			input:    `{"+": [{"var": "value"}, null]}`,
			expected: "WHERE (value + NULL)",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Transpile(DialectBigQuery, tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Transpile() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Transpile() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("Transpile() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

// TestAdditionalEdgeCases tests additional edge cases for comprehensive coverage.
func TestAdditionalEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		// NULL comparison tests
		{
			name:     "equal to null uses IS NULL",
			input:    `{"==": [{"var": "deleted_at"}, null]}`,
			expected: "WHERE deleted_at IS NULL",
			hasError: false,
		},
		{
			name:     "not equal to null uses IS NOT NULL",
			input:    `{"!=": [{"var": "field"}, null]}`,
			expected: "WHERE field IS NOT NULL",
			hasError: false,
		},
		{
			name:     "strict not equal to null uses IS NOT NULL",
			input:    `{"!==": [{"var": "value"}, null]}`,
			expected: "WHERE value IS NOT NULL",
			hasError: false,
		},

		// Missing operator with array
		{
			name:     "missing with array of fields",
			input:    `{"missing": ["email", "phone", "address"]}`,
			expected: "WHERE (email IS NULL OR phone IS NULL OR address IS NULL)",
			hasError: false,
		},

		// Cat with nested if
		{
			name:     "cat with nested if expression",
			input:    `{"cat": [{"if": [{"==": [{"var": "gender"}, "M"]}, "Mr. ", "Ms. "]}, {"var": "first_name"}, " ", {"var": "last_name"}]}`,
			expected: "WHERE CONCAT(CASE WHEN (gender = 'M') THEN 'Mr. ' ELSE 'Ms. ' END, first_name, ' ', last_name)",
			hasError: false,
		},

		// NOT with missing
		{
			name:     "NOT with missing",
			input:    `{"!": [{"missing": "email"}]}`,
			expected: "WHERE NOT (email IS NULL)",
			hasError: false,
		},

		// Double NOT
		{
			name:     "double NOT",
			input:    `{"!": [{"!": [{"var": "flag"}]}]}`,
			expected: "WHERE NOT (NOT (flag))",
			hasError: false,
		},

		// Five-value chained comparison
		{
			name:     "five value chained comparison",
			input:    `{"<": [1, {"var": "a"}, {"var": "b"}, {"var": "c"}, 100]}`,
			expected: "WHERE (1 < a AND a < b AND b < c AND c < 100)",
			hasError: false,
		},

		// Nested max/min
		{
			name:     "nested max min",
			input:    `{"max": [{"min": [{"var": "a"}, {"var": "b"}]}, {"min": [{"var": "c"}, {"var": "d"}]}]}`,
			expected: "WHERE GREATEST(LEAST(a, b), LEAST(c, d))",
			hasError: false,
		},

		// Complex negation
		{
			name:     "complex negation",
			input:    `{"!": [{"or": [{"==": [{"var": "a"}, 1]}, {"==": [{"var": "b"}, 2]}]}]}`,
			expected: "WHERE NOT ((a = 1 OR b = 2))",
			hasError: false,
		},

		// Between pattern
		{
			name:     "between pattern with and",
			input:    `{"and": [{">=": [{"var": "value"}, 10]}, {"<=": [{"var": "value"}, 20]}]}`,
			expected: "WHERE (value >= 10 AND value <= 20)",
			hasError: false,
		},

		// Triple nested if
		{
			name:     "triple nested if",
			input:    `{"if": [{"var": "a"}, {"if": [{"var": "b"}, {"if": [{"var": "c"}, "deep", "c_false"]}, "b_false"]}, "a_false"]}`,
			expected: "WHERE CASE WHEN a THEN CASE WHEN b THEN CASE WHEN c THEN 'deep' ELSE 'c_false' END ELSE 'b_false' END ELSE 'a_false' END",
			hasError: false,
		},

		// Comparison with var default
		{
			name:     "comparison with var default",
			input:    `{"==": [{"var": ["status", "unknown"]}, "active"]}`,
			expected: "WHERE COALESCE(status, 'unknown') = 'active'",
			hasError: false,
		},

		// Nested all with some
		{
			name:     "nested all with some",
			input:    `{"all": [{"var": "groups"}, {"some": [{"var": "members"}, {"==": [{"var": "role"}, "admin"]}]}]}`,
			expected: "WHERE (ARRAY_LENGTH(groups) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(groups) AS elem WHERE NOT (EXISTS (SELECT 1 FROM UNNEST(members) AS elem WHERE role = 'admin'))))",
			hasError: false,
		},

		// If with arithmetic result
		{
			name:     "if with arithmetic result",
			input:    `{"if": [{">": [{"var": "qty"}, 100]}, {"*": [{"var": "price"}, 0.9]}, {"*": [{"var": "price"}, 1.0]}]}`,
			expected: "WHERE CASE WHEN qty > 100 THEN (price * 0.9) ELSE (price * 1.0) END",
			hasError: false,
		},

		// Empty string in cat
		{
			name:     "empty string in cat",
			input:    `{"cat": ["", {"var": "name"}, ""]}`,
			expected: "WHERE CONCAT('', name, '')",
			hasError: false,
		},

		// Negative index in substr
		{
			name:     "negative index in substr",
			input:    `{"substr": [{"var": "text"}, -5, 3]}`,
			expected: "WHERE SUBSTR(text, -4, 3)",
			hasError: false,
		},

		// In with string (substring check)
		{
			name:     "in with string for substring check",
			input:    `{"in": ["test", "this is a test string"]}`,
			expected: "WHERE STRPOS('this is a test string', 'test') > 0",
			hasError: false,
		},

		// Or with literals
		{
			name:     "or with literals",
			input:    `{"or": [false, {"var": "flag"}, true]}`,
			expected: "WHERE (FALSE OR flag OR TRUE)",
			hasError: false,
		},

		// And with falsy values
		{
			name:     "and with falsy values",
			input:    `{"and": [false, null, 0, ""]}`,
			expected: "WHERE (FALSE AND NULL AND 0 AND '')",
			hasError: false,
		},

		// Type tagging edge cases - strings with SQL keywords should be quoted as literals
		{
			name:     "string with AND keyword should be quoted",
			input:    `{"==": [{"var": "name"}, "JOHN AND JANE"]}`,
			expected: "WHERE name = 'JOHN AND JANE'",
			hasError: false,
		},
		{
			name:     "string with OR keyword should be quoted",
			input:    `{"==": [{"var": "status"}, "PASS OR FAIL"]}`,
			expected: "WHERE status = 'PASS OR FAIL'",
			hasError: false,
		},
		{
			name:     "string with parentheses should be quoted",
			input:    `{"==": [{"var": "desc"}, "Item (Large)"]}`,
			expected: "WHERE desc = 'Item (Large)'",
			hasError: false,
		},
		{
			name:     "string with LIKE keyword should be quoted",
			input:    `{"==": [{"var": "phrase"}, "I LIKE PIZZA"]}`,
			expected: "WHERE phrase = 'I LIKE PIZZA'",
			hasError: false,
		},
		{
			name:     "string with SELECT keyword should be quoted",
			input:    `{"==": [{"var": "action"}, "SELECT ITEM"]}`,
			expected: "WHERE action = 'SELECT ITEM'",
			hasError: false,
		},
		{
			name:     "string with CASE keyword should be quoted",
			input:    `{"==": [{"var": "product"}, "PHONE CASE"]}`,
			expected: "WHERE product = 'PHONE CASE'",
			hasError: false,
		},
		{
			name:     "Japanese with SQL-like parentheses should be quoted",
			input:    `{"==": [{"var": "shop"}, "SPA(スパ)"]}`,
			expected: "WHERE shop = 'SPA(スパ)'",
			hasError: false,
		},
		{
			name:     "string with equals sign should be quoted",
			input:    `{"==": [{"var": "formula"}, "x = y + z"]}`,
			expected: "WHERE formula = 'x = y + z'",
			hasError: false,
		},
		{
			name:     "string with greater than sign should be quoted",
			input:    `{"==": [{"var": "comparison"}, "A > B"]}`,
			expected: "WHERE comparison = 'A > B'",
			hasError: false,
		},
		{
			name:     "string with IN keyword should be quoted",
			input:    `{"==": [{"var": "location"}, "STORE IN MALL"]}`,
			expected: "WHERE location = 'STORE IN MALL'",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Transpile(DialectBigQuery, tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Transpile() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Transpile() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("Transpile() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

// TestArrayOperatorsDialectSupport tests map, filter, reduce operators for BigQuery, Spanner, PostgreSQL, and DuckDB dialects.
func TestArrayOperatorsDialectSupport(t *testing.T) {
	dialects := []struct {
		name    string
		dialect Dialect
	}{
		{"BigQuery", DialectBigQuery},
		{"Spanner", DialectSpanner},
		{"PostgreSQL", DialectPostgreSQL},
		{"DuckDB", DialectDuckDB},
	}

	for _, d := range dialects {
		t.Run(d.name, func(t *testing.T) {
			tr, err := NewTranspiler(d.dialect)
			if err != nil {
				t.Fatalf("Failed to create transpiler: %v", err)
			}

			tests := []struct {
				name     string
				input    string
				expected string
			}{
				// Map operator tests
				{
					name:     "map with var array and addition",
					input:    `{"map": [{"var": "numbers"}, {"+": [{"var": "item"}, 1]}]}`,
					expected: "WHERE ARRAY(SELECT (elem + 1) FROM UNNEST(numbers) AS elem)",
				},
				{
					name:     "map with var array and multiplication",
					input:    `{"map": [{"var": "prices"}, {"*": [{"var": "item"}, 2]}]}`,
					expected: "WHERE ARRAY(SELECT (elem * 2) FROM UNNEST(prices) AS elem)",
				},
				{
					name:     "map with literal array",
					input:    `{"map": [[1, 2, 3], {"+": [{"var": "item"}, 10]}]}`,
					expected: "WHERE ARRAY(SELECT (elem + 10) FROM UNNEST([1, 2, 3]) AS elem)",
				},

				// Filter operator tests
				{
					name:     "filter with var array and greater than",
					input:    `{"filter": [{"var": "scores"}, {">": [{"var": "item"}, 70]}]}`,
					expected: "WHERE ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem > 70)",
				},
				{
					name:     "filter with var array and equality",
					input:    `{"filter": [{"var": "statuses"}, {"==": [{"var": "item"}, "active"]}]}`,
					expected: "WHERE ARRAY(SELECT elem FROM UNNEST(statuses) AS elem WHERE elem = 'active')",
				},
				{
					name:     "filter with literal array",
					input:    `{"filter": [[1, 2, 3, 4, 5], {">=": [{"var": "item"}, 3]}]}`,
					expected: "WHERE ARRAY(SELECT elem FROM UNNEST([1, 2, 3, 4, 5]) AS elem WHERE elem >= 3)",
				},

				// Reduce operator tests - SUM pattern
				{
					name:     "reduce with SUM pattern",
					input:    `{"reduce": [{"var": "numbers"}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}`,
					expected: "WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST(numbers) AS elem), 0)",
				},
				{
					name:     "reduce with SUM pattern and non-zero initial",
					input:    `{"reduce": [{"var": "values"}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 100]}`,
					expected: "WHERE 100 + COALESCE((SELECT SUM(elem) FROM UNNEST(values) AS elem), 0)",
				},
				{
					name:     "reduce with literal array and SUM pattern",
					input:    `{"reduce": [[10, 20, 30], {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}`,
					expected: "WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST([10, 20, 30]) AS elem), 0)",
				},

				// Reduce operator tests - MIN pattern
				{
					name:     "reduce with MIN pattern",
					input:    `{"reduce": [{"var": "values"}, {"min": [{"var": "accumulator"}, {"var": "current"}]}, 999999]}`,
					expected: "WHERE 999999 + COALESCE((SELECT MIN(elem) FROM UNNEST(values) AS elem), 0)",
				},

				// Reduce operator tests - MAX pattern
				{
					name:     "reduce with MAX pattern",
					input:    `{"reduce": [{"var": "values"}, {"max": [{"var": "accumulator"}, {"var": "current"}]}, 0]}`,
					expected: "WHERE 0 + COALESCE((SELECT MAX(elem) FROM UNNEST(values) AS elem), 0)",
				},

				// Reduce operator tests - general pattern
				{
					name:     "reduce with multiplication pattern",
					input:    `{"reduce": [{"var": "numbers"}, {"*": [{"var": "accumulator"}, {"var": "current"}]}, 1]}`,
					expected: "WHERE (SELECT (1 * elem) FROM UNNEST(numbers) AS elem)",
				},

				// All operator tests - dialect-specific array length function
				{
					name:  "all elements satisfy condition",
					input: `{"all": [{"var": "ages"}, {">=": [{"var": "item"}, 18]}]}`,
					expected: func() string {
						switch d.dialect {
						case DialectPostgreSQL:
							return "WHERE (CARDINALITY(ages) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18)))"
						case DialectDuckDB:
							return "WHERE (length(ages) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18)))"
						default: // BigQuery, Spanner
							return "WHERE (ARRAY_LENGTH(ages) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18)))"
						}
					}(),
				},

				// Some operator tests
				{
					name:     "some elements satisfy condition",
					input:    `{"some": [{"var": "statuses"}, {"==": [{"var": "item"}, "active"]}]}`,
					expected: "WHERE EXISTS (SELECT 1 FROM UNNEST(statuses) AS elem WHERE elem = 'active')",
				},

				// None operator tests
				{
					name:     "no elements satisfy condition",
					input:    `{"none": [{"var": "values"}, {"==": [{"var": "item"}, "invalid"]}]}`,
					expected: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE elem = 'invalid')",
				},

				// Note: merge tests are in TestMergeOperatorDialectSpecific due to dialect-specific output

				// Combined/nested array operations
				{
					name:     "map in comparison",
					input:    `{">": [{"map": [{"var": "numbers"}, {"*": [{"var": "item"}, 2]}]}, 10]}`,
					expected: "WHERE ARRAY(SELECT (elem * 2) FROM UNNEST(numbers) AS elem) > 10",
				},
				{
					name:     "filter in and condition",
					input:    `{"and": [{"==": [{"var": "status"}, "active"]}, {"some": [{"var": "tags"}, {"==": [{"var": "item"}, "premium"]}]}]}`,
					expected: "WHERE (status = 'active' AND EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE elem = 'premium'))",
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					result, err := tr.Transpile(tt.input)
					if err != nil {
						t.Errorf("[%s] Transpile() unexpected error = %v", d.name, err)
						return
					}
					if result != tt.expected {
						t.Errorf("[%s] Transpile() = %v, expected %v", d.name, result, tt.expected)
					}
				})
			}
		})
	}
}

// TestMergeOperatorDialectSpecific tests the merge operator with dialect-specific SQL output.
func TestMergeOperatorDialectSpecific(t *testing.T) {
	tests := []struct {
		name     string
		dialect  Dialect
		input    string
		expected string
	}{
		// BigQuery
		{
			name:     "BigQuery merge two arrays",
			dialect:  DialectBigQuery,
			input:    `{"merge": [{"var": "arr1"}, {"var": "arr2"}]}`,
			expected: "WHERE ARRAY_CONCAT(arr1, arr2)",
		},
		{
			name:     "BigQuery merge three arrays",
			dialect:  DialectBigQuery,
			input:    `{"merge": [{"var": "a"}, {"var": "b"}, {"var": "c"}]}`,
			expected: "WHERE ARRAY_CONCAT(a, b, c)",
		},
		// Spanner
		{
			name:     "Spanner merge two arrays",
			dialect:  DialectSpanner,
			input:    `{"merge": [{"var": "arr1"}, {"var": "arr2"}]}`,
			expected: "WHERE ARRAY_CONCAT(arr1, arr2)",
		},
		// PostgreSQL - uses || operator
		{
			name:     "PostgreSQL merge two arrays",
			dialect:  DialectPostgreSQL,
			input:    `{"merge": [{"var": "arr1"}, {"var": "arr2"}]}`,
			expected: "WHERE (arr1 || arr2)",
		},
		{
			name:     "PostgreSQL merge three arrays",
			dialect:  DialectPostgreSQL,
			input:    `{"merge": [{"var": "a"}, {"var": "b"}, {"var": "c"}]}`,
			expected: "WHERE (a || b || c)",
		},
		{
			name:     "PostgreSQL merge single array",
			dialect:  DialectPostgreSQL,
			input:    `{"merge": [{"var": "arr"}]}`,
			expected: "WHERE arr",
		},
		// DuckDB - uses ARRAY_CONCAT like BigQuery/Spanner
		{
			name:     "DuckDB merge two arrays",
			dialect:  DialectDuckDB,
			input:    `{"merge": [{"var": "arr1"}, {"var": "arr2"}]}`,
			expected: "WHERE ARRAY_CONCAT(arr1, arr2)",
		},
		{
			name:     "DuckDB merge three arrays",
			dialect:  DialectDuckDB,
			input:    `{"merge": [{"var": "a"}, {"var": "b"}, {"var": "c"}]}`,
			expected: "WHERE ARRAY_CONCAT(a, b, c)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := NewTranspiler(tt.dialect)
			if err != nil {
				t.Fatalf("Failed to create transpiler: %v", err)
			}

			result, err := tr.Transpile(tt.input)
			if err != nil {
				t.Errorf("Transpile() unexpected error = %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("Transpile() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

// TestTranspileCondition tests the TranspileCondition method that returns SQL without WHERE keyword.
func TestTranspileCondition(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple comparison without WHERE",
			input:    `{">": [{"var": "amount"}, 1000]}`,
			expected: "amount > 1000",
		},
		{
			name:     "and operation without WHERE",
			input:    `{"and": [{"==": [{"var": "status"}, "pending"]}, {">": [{"var": "amount"}, 5000]}]}`,
			expected: "(status = 'pending' AND amount > 5000)",
		},
		{
			name:     "complex nested condition without WHERE",
			input:    `{"or": [{">=": [{"var": "failedAttempts"}, 5]}, {"in": [{"var": "country"}, ["CN", "RU"]]}]}`,
			expected: "(failedAttempts >= 5 OR country IN ('CN', 'RU'))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.TranspileCondition(tt.input)
			if err != nil {
				t.Errorf("TranspileCondition() unexpected error = %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("TranspileCondition() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

// TestTranspileConditionFromMap tests the TranspileConditionFromMap method.
func TestTranspileConditionFromMap(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	logic := map[string]interface{}{
		">": []interface{}{
			map[string]interface{}{"var": "amount"},
			1000,
		},
	}

	result, err := tr.TranspileConditionFromMap(logic)
	if err != nil {
		t.Errorf("TranspileConditionFromMap() unexpected error = %v", err)
		return
	}

	expected := "amount > 1000"
	if result != expected {
		t.Errorf("TranspileConditionFromMap() = %q, expected %q", result, expected)
	}
}

// TestTranspileConditionConvenienceFunction tests the standalone TranspileCondition function.
func TestTranspileConditionConvenienceFunction(t *testing.T) {
	result, err := TranspileCondition(DialectBigQuery, `{"==": [{"var": "status"}, "active"]}`)
	if err != nil {
		t.Errorf("TranspileCondition() unexpected error = %v", err)
		return
	}

	expected := "status = 'active'"
	if result != expected {
		t.Errorf("TranspileCondition() = %q, expected %q", result, expected)
	}
}

// TestTranspileVsTranspileCondition verifies the difference between Transpile and TranspileCondition.
func TestTranspileVsTranspileCondition(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	input := `{">": [{"var": "amount"}, 1000]}`

	// Transpile should include WHERE
	withWhere, err := tr.Transpile(input)
	if err != nil {
		t.Fatalf("Transpile() error: %v", err)
	}
	if withWhere != "WHERE amount > 1000" {
		t.Errorf("Transpile() = %q, expected %q", withWhere, "WHERE amount > 1000")
	}

	// TranspileCondition should NOT include WHERE
	withoutWhere, err := tr.TranspileCondition(input)
	if err != nil {
		t.Fatalf("TranspileCondition() error: %v", err)
	}
	if withoutWhere != "amount > 1000" {
		t.Errorf("TranspileCondition() = %q, expected %q", withoutWhere, "amount > 1000")
	}
}

func TestNewTranspilerWithConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *TranspilerConfig
		wantError bool
	}{
		{
			name:      "BigQuery dialect",
			config:    &TranspilerConfig{Dialect: DialectBigQuery},
			wantError: false,
		},
		{
			name:      "Spanner dialect",
			config:    &TranspilerConfig{Dialect: DialectSpanner},
			wantError: false,
		},
		{
			name:      "PostgreSQL dialect",
			config:    &TranspilerConfig{Dialect: DialectPostgreSQL},
			wantError: false,
		},
		{
			name:      "DuckDB dialect",
			config:    &TranspilerConfig{Dialect: DialectDuckDB},
			wantError: false,
		},
		{
			name:      "ClickHouse dialect",
			config:    &TranspilerConfig{Dialect: DialectClickHouse},
			wantError: false,
		},
		{
			name:      "unspecified dialect",
			config:    &TranspilerConfig{},
			wantError: true,
		},
		{
			name:      "nil config",
			config:    nil,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := NewTranspilerWithConfig(tt.config)
			if tt.wantError {
				if err == nil {
					t.Error("NewTranspilerWithConfig() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("NewTranspilerWithConfig() unexpected error: %v", err)
				}
				if tr == nil {
					t.Error("NewTranspilerWithConfig() returned nil transpiler")
				}
			}
		})
	}
}

func TestNewTranspilerWithConfig_WithSchema(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeInteger},
		{Name: "status", Type: FieldTypeString},
	})

	config := &TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  schema,
	}

	tr, err := NewTranspilerWithConfig(config)
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() unexpected error: %v", err)
	}

	// Valid field should work
	result, err := tr.Transpile(`{">": [{"var": "amount"}, 100]}`)
	if err != nil {
		t.Errorf("Transpile() unexpected error: %v", err)
	}
	if result != "WHERE amount > 100" {
		t.Errorf("Transpile() = %q, want %q", result, "WHERE amount > 100")
	}

	// Invalid field should error
	_, err = tr.Transpile(`{">": [{"var": "invalid_field"}, 100]}`)
	if err == nil {
		t.Error("Transpile() should error for invalid field when schema is set")
	}
}

func TestNewTranspilerWithConfig_NilSchemaPointer_UsesNoSchemaValidation(t *testing.T) {
	var nilSchema *Schema
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  nilSchema,
	})
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() unexpected error: %v", err)
	}

	_, err = tr.Transpile(`{"==": [{"var": "bad field"}, 1]}`)
	if err == nil {
		t.Fatal("expected invalid identifier error with nil schema")
	}
}

func TestTranspiler_SetSchema_NilRestoresIdentifierValidation(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() unexpected error: %v", err)
	}

	// With schema set, unusual names are allowed and validated by schema.
	tr.SetSchema(mustNewSchema([]FieldSchema{
		{Name: "bad field", Type: FieldTypeNumber},
	}))

	if _, err := tr.Transpile(`{"==": [{"var": "bad field"}, 1]}`); err != nil {
		t.Fatalf("Transpile() with schema unexpected error: %v", err)
	}

	// Clearing schema should restore no-schema identifier safety checks.
	tr.SetSchema(nil)
	if _, err := tr.Transpile(`{"==": [{"var": "bad field"}, 1]}`); err == nil {
		t.Fatal("expected invalid identifier error after SetSchema(nil)")
	}
}

func TestTranspile_PreservesLargeJSONIntegerLiterals(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() unexpected error: %v", err)
	}

	sql1, err := tr.Transpile(`{"==": [{"var": "amount"}, 9223372036854775808]}`)
	if err != nil {
		t.Fatalf("Transpile() unexpected error: %v", err)
	}
	sql2, err := tr.Transpile(`{"==": [{"var": "amount"}, 9223372036854775809]}`)
	if err != nil {
		t.Fatalf("Transpile() unexpected error: %v", err)
	}

	want1 := "WHERE amount = 9223372036854775808"
	want2 := "WHERE amount = 9223372036854775809"
	if sql1 != want1 {
		t.Errorf("sql1 = %q, want %q", sql1, want1)
	}
	if sql2 != want2 {
		t.Errorf("sql2 = %q, want %q", sql2, want2)
	}
	if sql1 == sql2 {
		t.Errorf("expected distinct SQL for distinct large integer literals, got %q", sql1)
	}
}

func TestTranspiler_GetDialect(t *testing.T) {
	tests := []struct {
		name    string
		dialect Dialect
	}{
		{"BigQuery", DialectBigQuery},
		{"Spanner", DialectSpanner},
		{"PostgreSQL", DialectPostgreSQL},
		{"DuckDB", DialectDuckDB},
		{"ClickHouse", DialectClickHouse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := NewTranspiler(tt.dialect)
			if err != nil {
				t.Fatalf("NewTranspiler() error: %v", err)
			}

			if tr.GetDialect() != tt.dialect {
				t.Errorf("GetDialect() = %v, want %v", tr.GetDialect(), tt.dialect)
			}
		})
	}
}

func TestTranspiler_TranspileConditionFromInterface(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error: %v", err)
	}

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name: "simple comparison",
			input: map[string]interface{}{
				">": []interface{}{
					map[string]interface{}{"var": "amount"},
					1000,
				},
			},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name: "equality",
			input: map[string]interface{}{
				"==": []interface{}{
					map[string]interface{}{"var": "status"},
					"active",
				},
			},
			expected: "status = 'active'",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.TranspileConditionFromInterface(tt.input)
			if tt.hasError {
				if err == nil {
					t.Error("TranspileConditionFromInterface() expected error")
				}
			} else {
				if err != nil {
					t.Errorf("TranspileConditionFromInterface() unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileConditionFromInterface() = %q, want %q", result, tt.expected)
				}
			}
		})
	}
}

// DialectAwareTestOperator implements DialectAwareOperatorHandler for testing.
type DialectAwareTestOperator struct{}

func (d *DialectAwareTestOperator) ToSQLWithDialect(operator string, args []interface{}, dialect Dialect) (string, error) {
	if len(args) != 1 {
		return "", nil
	}
	switch dialect {
	case DialectBigQuery:
		return "BIGQUERY_" + args[0].(string), nil
	case DialectSpanner:
		return "SPANNER_" + args[0].(string), nil
	default:
		return "DEFAULT_" + args[0].(string), nil
	}
}

func TestTranspiler_RegisterDialectAwareOperator(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error: %v", err)
	}

	err = tr.RegisterDialectAwareOperator("testOp", &DialectAwareTestOperator{})
	if err != nil {
		t.Fatalf("RegisterDialectAwareOperator() error: %v", err)
	}

	// Verify operator is registered
	if !tr.HasCustomOperator("testOp") {
		t.Error("testOp should be registered")
	}

	// Test that it works
	result, err := tr.Transpile(`{"testOp": [{"var": "field"}]}`)
	if err != nil {
		t.Errorf("Transpile() unexpected error: %v", err)
	}
	if result != "WHERE BIGQUERY_field" {
		t.Errorf("Transpile() = %q, want %q", result, "WHERE BIGQUERY_field")
	}
}

// TestPackageLevel_TranspileConditionFromMap tests the package-level TranspileConditionFromMap function.
func TestPackageLevel_TranspileConditionFromMap(t *testing.T) {
	tests := []struct {
		name     string
		dialect  Dialect
		logic    map[string]interface{}
		expected string
		hasError bool
	}{
		{
			name:    "simple comparison BigQuery",
			dialect: DialectBigQuery,
			logic: map[string]interface{}{
				">": []interface{}{
					map[string]interface{}{"var": "amount"},
					float64(100),
				},
			},
			expected: "amount > 100",
			hasError: false,
		},
		{
			name:    "simple comparison Spanner",
			dialect: DialectSpanner,
			logic: map[string]interface{}{
				"==": []interface{}{
					map[string]interface{}{"var": "status"},
					"active",
				},
			},
			expected: "status = 'active'",
			hasError: false,
		},
		{
			name:    "simple comparison PostgreSQL",
			dialect: DialectPostgreSQL,
			logic: map[string]interface{}{
				"<": []interface{}{
					map[string]interface{}{"var": "count"},
					float64(50),
				},
			},
			expected: "count < 50",
			hasError: false,
		},
		{
			name:    "simple comparison DuckDB",
			dialect: DialectDuckDB,
			logic: map[string]interface{}{
				">=": []interface{}{
					map[string]interface{}{"var": "value"},
					float64(10),
				},
			},
			expected: "value >= 10",
			hasError: false,
		},
		{
			name:    "simple comparison ClickHouse",
			dialect: DialectClickHouse,
			logic: map[string]interface{}{
				"<=": []interface{}{
					map[string]interface{}{"var": "score"},
					float64(75),
				},
			},
			expected: "score <= 75",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TranspileConditionFromMap(tt.dialect, tt.logic)
			if tt.hasError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileConditionFromMap() = %q, want %q", result, tt.expected)
				}
			}
		})
	}
}

// TestPackageLevel_TranspileConditionFromInterface tests the package-level TranspileConditionFromInterface function.
func TestPackageLevel_TranspileConditionFromInterface(t *testing.T) {
	tests := []struct {
		name     string
		dialect  Dialect
		logic    interface{}
		expected string
		hasError bool
	}{
		{
			name:    "from map interface BigQuery",
			dialect: DialectBigQuery,
			logic: map[string]interface{}{
				"!=": []interface{}{
					map[string]interface{}{"var": "type"},
					"unknown",
				},
			},
			expected: "type != 'unknown'",
			hasError: false,
		},
		{
			name:    "from map interface Spanner",
			dialect: DialectSpanner,
			logic: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{
						">": []interface{}{
							map[string]interface{}{"var": "a"},
							float64(1),
						},
					},
					map[string]interface{}{
						"<": []interface{}{
							map[string]interface{}{"var": "b"},
							float64(10),
						},
					},
				},
			},
			expected: "(a > 1 AND b < 10)",
			hasError: false,
		},
		{
			name:    "from map interface DuckDB",
			dialect: DialectDuckDB,
			logic: map[string]interface{}{
				"or": []interface{}{
					map[string]interface{}{
						"==": []interface{}{
							map[string]interface{}{"var": "x"},
							float64(0),
						},
					},
					map[string]interface{}{
						"==": []interface{}{
							map[string]interface{}{"var": "y"},
							float64(0),
						},
					},
				},
			},
			expected: "(x = 0 OR y = 0)",
			hasError: false,
		},
		{
			name:    "from map interface ClickHouse",
			dialect: DialectClickHouse,
			logic: map[string]interface{}{
				"in": []interface{}{
					map[string]interface{}{"var": "status"},
					[]interface{}{"a", "b", "c"},
				},
			},
			expected: "status IN ('a', 'b', 'c')",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TranspileConditionFromInterface(tt.dialect, tt.logic)
			if tt.hasError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileConditionFromInterface() = %q, want %q", result, tt.expected)
				}
			}
		})
	}
}

func TestNestedComparisonSchemaCoercion(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "status", Type: FieldTypeString},
		{Name: "amount", Type: FieldTypeInteger},
	})
	config := &TranspilerConfig{Dialect: DialectBigQuery, Schema: schema}
	tr, err := NewTranspilerWithConfig(config)
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() error: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "baseline: number coerced to string for string field",
			input:    `{"==": [{"var": "status"}, 123]}`,
			expected: "WHERE status = '123'",
		},
		{
			name:     "nested in numeric: coercion still applies",
			input:    `{"+": [{"==": [{"var": "status"}, 123]}, 0]}`,
			expected: "WHERE ((status = '123') + 0)",
		},
		{
			name:     "nested in if in numeric: coercion still applies",
			input:    `{"+": [{"if": [{"==": [{"var": "status"}, 456]}, 1, 0]}, 0]}`,
			expected: "WHERE (CASE WHEN status = '456' THEN 1 ELSE 0 END + 0)",
		},
		{
			name:     "nested: string coerced to number for integer field",
			input:    `{"+": [{">": [{"var": "amount"}, "50"]}, 0]}`,
			expected: "WHERE ((amount > 50) + 0)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.Transpile(tt.input)
			if err != nil {
				t.Fatalf("Transpile() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("Transpile() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestNumericStringInjectionPrevention(t *testing.T) {
	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "multiplication with injection string",
			input:    `{"*": ["1 OR 1=1", 2]}`,
			expected: "WHERE ('1 OR 1=1' * 2)",
		},
		{
			name:     "addition with injection string",
			input:    `{"+": ["1; DROP TABLE users", 2]}`,
			expected: "WHERE ('1; DROP TABLE users' + 2)",
		},
		{
			name:     "numeric string coerced correctly",
			input:    `{"*": ["3", 4]}`,
			expected: "WHERE (3 * 4)",
		},
		{
			name:     "float string coerced correctly",
			input:    `{"+": ["1.5", 2]}`,
			expected: "WHERE (1.5 + 2)",
		},
		{
			name:     "non-numeric string safely quoted",
			input:    `{"+": ["hello", 1]}`,
			expected: "WHERE ('hello' + 1)",
		},
		{
			name:     "single quotes escaped in string",
			input:    `{"*": ["it's dangerous", 1]}`,
			expected: "WHERE ('it''s dangerous' * 1)",
		},
		{
			name:     "nested comparison in numeric preserves column names",
			input:    `{"+": [{"if": [{"==": [{"var": "status"}, "active"]}, 1, 0]}, 0]}`,
			expected: "WHERE (CASE WHEN status = 'active' THEN 1 ELSE 0 END + 0)",
		},
	}

	for _, d := range dialects {
		t.Run(d.String(), func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					result, err := Transpile(d, tt.input)
					if err != nil {
						t.Fatalf("Transpile() error = %v", err)
					}
					if result != tt.expected {
						t.Errorf("Transpile() = %q, want %q", result, tt.expected)
					}
				})
			}
		})
	}
}
