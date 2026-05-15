package jsonlogic2sql

import (
	"strings"
	"testing"
)

func TestTranspile_ArrayLambdaMissingUsesElementScope(t *testing.T) {
	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-aware", schema: testArrayScopeSchema()},
		{name: "schema-less", schema: nil},
	}

	tests := []struct {
		name           string
		logic          string
		valueRoot      bool
		wantFragments  []string
		blockFragments []string
		wantParamCount int
	}{
		{
			name:      "filter missing single field",
			logic:     `{"filter":[{"var":"numbers"},{"missing":"name"}]}`,
			valueRoot: true,
			wantFragments: []string{
				"elem.name IS NULL",
			},
			blockFragments: []string{
				"WHERE name IS NULL",
				"-> name IS NULL",
			},
		},
		{
			name:  "some missing field array",
			logic: `{"some":[{"var":"numbers"},{"missing":["name","email"]}]}`,
			wantFragments: []string{
				"elem.name IS NULL",
				"elem.email IS NULL",
			},
			blockFragments: []string{
				"WHERE (name IS NULL",
				"-> (name IS NULL",
			},
		},
		{
			name:  "some missing_some field array",
			logic: `{"some":[{"var":"numbers"},{"missing_some":[2,["name","email","phone"]]}]}`,
			wantFragments: []string{
				"CASE WHEN elem.name IS NULL THEN 1 ELSE 0 END",
				"CASE WHEN elem.email IS NULL THEN 1 ELSE 0 END",
				"CASE WHEN elem.phone IS NULL THEN 1 ELSE 0 END",
			},
			blockFragments: []string{
				"CASE WHEN name IS NULL",
				"CASE WHEN email IS NULL",
				"CASE WHEN phone IS NULL",
			},
			wantParamCount: 1,
		},
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

					for _, tt := range tests {
						t.Run(tt.name+"/inline", func(t *testing.T) {
							sql := transpileArrayMissingScopeCase(t, tr, tt.logic, tt.valueRoot)
							assertSQLFragments(t, sql, tt.wantFragments, tt.blockFragments)
						})

						t.Run(tt.name+"/parameterized", func(t *testing.T) {
							sql, params := transpileParameterizedArrayMissingScopeCase(t, tr, tt.logic, tt.valueRoot)
							assertSQLFragments(t, sql, tt.wantFragments, tt.blockFragments)
							if got := len(params); got != tt.wantParamCount {
								t.Fatalf("param count = %d, want %d (params=%v)", got, tt.wantParamCount, params)
							}
						})
					}
				})
			}
		})
	}
}

func TestTranspile_ArrayLambdaMissingRejectsImplementationAliases(t *testing.T) {
	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-aware", schema: testArrayScopeSchema()},
		{name: "schema-less", schema: nil},
	}
	aliases := []string{".name", "item.name", "current.name", "elem.name"}

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

					for _, alias := range aliases {
						t.Run(alias+"/missing", func(t *testing.T) {
							logic := `{"some":[{"var":"numbers"},{"missing":"` + alias + `"}]}`
							sql, err := tr.TranspileCondition(logic)
							assertArrayMissingScopeAliasError(t, sql, err)
							_, _, err = tr.TranspileParameterizedCondition(logic)
							assertArrayMissingScopeAliasError(t, "", err)
						})

						t.Run(alias+"/missing_some", func(t *testing.T) {
							logic := `{"some":[{"var":"numbers"},{"missing_some":[1,["` + alias + `"]]}]}`
							sql, err := tr.TranspileCondition(logic)
							assertArrayMissingScopeAliasError(t, sql, err)
							_, _, err = tr.TranspileParameterizedCondition(logic)
							assertArrayMissingScopeAliasError(t, "", err)
						})
					}
				})
			}
		})
	}
}

func transpileArrayMissingScopeCase(t *testing.T, tr *Transpiler, logic string, valueRoot bool) string {
	t.Helper()
	if valueRoot {
		sql, err := tr.TranspileValue(logic)
		if err != nil {
			t.Fatalf("TranspileValue() error: %v", err)
		}
		return sql
	}
	sql, err := tr.TranspileCondition(logic)
	if err != nil {
		t.Fatalf("TranspileCondition() error: %v", err)
	}
	return sql
}

func transpileParameterizedArrayMissingScopeCase(t *testing.T, tr *Transpiler, logic string, valueRoot bool) (string, []QueryParam) {
	t.Helper()
	if valueRoot {
		sql, params, err := tr.TranspileParameterizedValue(logic)
		if err != nil {
			t.Fatalf("TranspileParameterizedValue() error: %v", err)
		}
		return sql, params
	}
	sql, params, err := tr.TranspileParameterizedCondition(logic)
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error: %v", err)
	}
	return sql, params
}

func assertSQLFragments(t *testing.T, sql string, wantFragments, blockFragments []string) {
	t.Helper()
	for _, want := range wantFragments {
		if !strings.Contains(sql, want) {
			t.Fatalf("expected SQL to contain %q, got: %s", want, sql)
		}
	}
	for _, block := range blockFragments {
		if strings.Contains(sql, block) {
			t.Fatalf("SQL contains unscoped fragment %q: %s", block, sql)
		}
	}
}

func assertArrayMissingScopeAliasError(t *testing.T, _ string, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected unsupported array-scope variable error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported array-scope variable") {
		t.Fatalf("expected unsupported array-scope variable error, got: %v", err)
	}
}
