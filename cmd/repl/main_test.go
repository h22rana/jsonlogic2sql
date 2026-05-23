package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"

	"github.com/h22rana/jsonlogic2sql"
)

func replTestSchema(t *testing.T) *jsonlogic2sql.Schema {
	t.Helper()
	schema, err := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
		{Name: "col", Type: jsonlogic2sql.FieldTypeString},
		{Name: "column", Type: jsonlogic2sql.FieldTypeString},
		{Name: "desc", Type: jsonlogic2sql.FieldTypeString},
		{Name: "name", Type: jsonlogic2sql.FieldTypeString},
		{Name: "email", Type: jsonlogic2sql.FieldTypeString},
		{Name: "code", Type: jsonlogic2sql.FieldTypeString},
		{Name: "status", Type: jsonlogic2sql.FieldTypeString},
		{Name: "amount", Type: jsonlogic2sql.FieldTypeNumber},
		{Name: "country", Type: jsonlogic2sql.FieldTypeString},
		{Name: "deleted_at", Type: jsonlogic2sql.FieldTypeString},
		{Name: "active", Type: jsonlogic2sql.FieldTypeBoolean},
		{Name: "failedAttempts", Type: jsonlogic2sql.FieldTypeNumber},
		{Name: "transaction.amount", Type: jsonlogic2sql.FieldTypeNumber},
		{Name: "user.verified", Type: jsonlogic2sql.FieldTypeBoolean},
		{Name: "user.accountAgeDays", Type: jsonlogic2sql.FieldTypeNumber},
		{Name: "age", Type: jsonlogic2sql.FieldTypeNumber},
		{Name: "field", Type: jsonlogic2sql.FieldTypeString},
		{Name: "field1", Type: jsonlogic2sql.FieldTypeString},
		{Name: "field2", Type: jsonlogic2sql.FieldTypeString},
		{Name: "verified", Type: jsonlogic2sql.FieldTypeBoolean},
	})
	if err != nil {
		t.Fatalf("NewSchema() error: %v", err)
	}
	return schema
}

func TestUnescapeSQLString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain unquoted string", "hello", "hello"},
		{"unquoted with double single quotes", "it''s", "it''s"},
		{"quoted simple", "'hello'", "hello"},
		{"quoted with escaped quote", "'it''s'", "it's"},
		{"quoted with multiple escaped quotes", "'it''s a ''test'''", "it's a 'test'"},
		{"single quote char", "'", "'"},
		{"two single quotes (empty SQL string)", "''", ""},
		{"quoted empty string", "''", ""},
		{"column identifier", "my_column", "my_column"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := unescapeSQLString(tt.input)
			if got != tt.want {
				t.Errorf("unescapeSQLString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeLikePattern(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello", "hello"},
		{"single quote", "it's", "it''s"},
		{"percent", "100%", "100\\%"},
		{"underscore", "a_b", "a\\_b"},
		{"backslash", "a\\b", "a\\\\b"},
		{"mixed special chars", "it's 100% done_now", "it''s 100\\% done\\_now"},
		{"empty string", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeLikePattern(tt.input)
			if got != tt.want {
				t.Errorf("escapeLikePattern(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractFromArrayString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"not an array", "hello", "hello"},
		{"array with unquoted value", "[T]", "T"},
		{"array with quoted value", "['hello']", "hello"},
		{"array with quoted and escaped value", "['it''s']", "it's"},
		{"empty array", "[]", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFromArrayString(tt.input)
			if got != tt.want {
				t.Errorf("extractFromArrayString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseContainsArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []interface{}
		wantColumn string
		wantPat    string
	}{
		{
			name:       "normal order with quoted pattern",
			args:       []interface{}{"col", "'hello'"},
			wantColumn: "col",
			wantPat:    "'hello'",
		},
		{
			name:       "normal order with escaped quote",
			args:       []interface{}{"col", "'it''s'"},
			wantColumn: "col",
			wantPat:    "'it''s'",
		},
		{
			name:       "reversed order (quoted pattern first)",
			args:       []interface{}{"'it''s'", "col"},
			wantColumn: "col",
			wantPat:    "'it''s'",
		},
		{
			name:       "array pattern",
			args:       []interface{}{"col", "['it''s']"},
			wantColumn: "col",
			wantPat:    "'it''s'",
		},
		{
			name:       "postgres array pattern",
			args:       []interface{}{"col", "ARRAY['it''s']"},
			wantColumn: "col",
			wantPat:    "'it''s'",
		},
		{
			name:       "postgres array placeholder pattern",
			args:       []interface{}{"col", "ARRAY[$1]"},
			wantColumn: "col",
			wantPat:    "$1",
		},
		{
			name:       "placeholder arg (parameterized mode)",
			args:       []interface{}{"col", "@p1"},
			wantColumn: "col",
			wantPat:    "@p1",
		},
		{
			name:       "reversed placeholder arg (parameterized BigQuery)",
			args:       []interface{}{"@p1", "col"},
			wantColumn: "col",
			wantPat:    "@p1",
		},
		{
			name:       "reversed placeholder arg (parameterized PostgreSQL)",
			args:       []interface{}{"$1", "col"},
			wantColumn: "col",
			wantPat:    "$1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := make([]jsonlogic2sql.OperatorArg, len(tt.args))
			for i, arg := range tt.args {
				args[i] = jsonlogic2sql.OperatorArg{SQL: arg.(string)}
			}
			col, pat := parseContainsArgs(args)
			if col != tt.wantColumn || pat != tt.wantPat {
				t.Errorf("parseContainsArgs() = (%q, %q), want (%q, %q)",
					col, pat, tt.wantColumn, tt.wantPat)
			}
		})
	}
}

// setupTestTranspiler creates a transpiler with the same custom operators as the REPL.
func setupTestTranspiler(t *testing.T) *jsonlogic2sql.Transpiler {
	t.Helper()
	currentDialect = jsonlogic2sql.DialectBigQuery
	tr, err := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectBigQuery, replTestSchema(t))
	if err != nil {
		t.Fatalf("NewTranspiler: %v", err)
	}
	registerCustomOperators(tr)
	return tr
}

func replFieldEqualityScenarioSchema(t *testing.T) *jsonlogic2sql.Schema {
	t.Helper()
	schema, err := jsonlogic2sql.NewSchema([]jsonlogic2sql.FieldSchema{
		{
			Name: "request.methods",
			Type: jsonlogic2sql.FieldTypeArray,
			ElementFields: []jsonlogic2sql.FieldSchema{
				{Name: "type", Type: jsonlogic2sql.FieldTypeString},
			},
		},
		{Name: "request.channel", Type: jsonlogic2sql.FieldTypeString},
		{Name: "request.description", Type: jsonlogic2sql.FieldTypeString},
		{Name: "request.account_id", Type: jsonlogic2sql.FieldTypeString},
		{Name: "request.amount", Type: jsonlogic2sql.FieldTypeInteger},
		{Name: "request.numeric_user_id", Type: jsonlogic2sql.FieldTypeInteger},
		{Name: "metrics.window.2m.count", Type: jsonlogic2sql.FieldTypeInteger},
	})
	if err != nil {
		t.Fatalf("NewSchema() error: %v", err)
	}
	return schema
}

func fieldEqualityScenarioJSON(finalComparison string) string {
	return fmt.Sprintf(`{"and":[{"==":[{"var":"request.channel"},"Code"]},{"some":[{"var":"request.methods"},{"==":[{"var":"type"},"BALANCE"]}]},{"!=":[{"var":"request.description"},""]},{"!=":[{"var":"request.account_id"},""]},{">=":[{"var":"request.amount"},10000]},{"or":[{"contains":[{"var":"request.description"},"alpha"]},{"contains":[{"var":"request.description"},"beta"]},{"contains":[{"var":"request.description"},"gamma"]},{"contains":[{"var":"request.description"},"delta"]}]},%s]}`, finalComparison)
}

func scenarioCountFieldSQL(d jsonlogic2sql.Dialect) string {
	return fmt.Sprintf("metrics.window.%s.count", dialect.QuoteIdentifierSegment("2m", d))
}

func TestTransactionRuleFieldEqualityAcrossDialectsAndModes(t *testing.T) {
	schema := replFieldEqualityScenarioSchema(t)
	countField := `{"var":"metrics.window.2m.count"}`

	tests := []struct {
		name           string
		final          string
		wantFragments  []string
		blockFragments []string
		wantErr        string
	}{
		{
			name:  "integer integer loose equality",
			final: fmt.Sprintf(`{"==":[%s,{"var":"request.numeric_user_id"}]}`, countField),
			wantFragments: []string{
				"{count} IS NULL AND request.numeric_user_id IS NULL",
				"{count} = request.numeric_user_id",
			},
		},
		{
			name:  "integer integer strict equality",
			final: fmt.Sprintf(`{"===":[%s,{"var":"request.numeric_user_id"}]}`, countField),
			wantFragments: []string{
				"{count} IS NULL AND request.numeric_user_id IS NULL",
				"{count} = request.numeric_user_id",
			},
		},
		{
			name:  "integer string strict equality",
			final: fmt.Sprintf(`{"===":[%s,{"var":"request.channel"}]}`, countField),
			wantFragments: []string{
				"{count} IS NULL AND request.channel IS NULL",
			},
			blockFragments: []string{
				"{count} = request.channel",
			},
		},
		{
			name:    "integer string loose equality",
			final:   fmt.Sprintf(`{"==":[%s,{"var":"request.channel"}]}`, countField),
			wantErr: "loose equality between number field",
		},
	}

	for _, d := range []jsonlogic2sql.Dialect{
		jsonlogic2sql.DialectBigQuery,
		jsonlogic2sql.DialectSpanner,
		jsonlogic2sql.DialectPostgreSQL,
		jsonlogic2sql.DialectDuckDB,
		jsonlogic2sql.DialectClickHouse,
	} {
		t.Run(d.String(), func(t *testing.T) {
			currentDialect = d
			tr, err := jsonlogic2sql.NewTranspiler(d, schema)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}
			registerCustomOperators(tr)

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					logic := fieldEqualityScenarioJSON(tt.final)
					countSQL := scenarioCountFieldSQL(d)
					wantFragments := make([]string, 0, len(tt.wantFragments))
					for _, fragment := range tt.wantFragments {
						wantFragments = append(wantFragments, strings.ReplaceAll(fragment, "{count}", countSQL))
					}
					blockFragments := make([]string, 0, len(tt.blockFragments))
					for _, fragment := range tt.blockFragments {
						blockFragments = append(blockFragments, strings.ReplaceAll(fragment, "{count}", countSQL))
					}
					apis := map[string]func() (string, error){
						"condition": func() (string, error) {
							return tr.TranspileCondition(logic)
						},
						"parameterized condition": func() (string, error) {
							sql, _, err := tr.TranspileParameterizedCondition(logic)
							return sql, err
						},
						"value": func() (string, error) {
							return tr.TranspileValue(logic)
						},
						"parameterized value": func() (string, error) {
							sql, _, err := tr.TranspileParameterizedValue(logic)
							return sql, err
						},
					}

					for apiName, run := range apis {
						sql, err := run()
						if tt.wantErr != "" {
							if err == nil {
								t.Fatalf("%s SQL = %q, want error containing %q", apiName, sql, tt.wantErr)
							}
							if !strings.Contains(err.Error(), tt.wantErr) {
								t.Fatalf("%s error = %v, want %q", apiName, err, tt.wantErr)
							}
							continue
						}
						if err != nil {
							t.Fatalf("%s error = %v", apiName, err)
						}
						for _, fragment := range wantFragments {
							if !strings.Contains(sql, fragment) {
								t.Fatalf("%s SQL missing %q:\n%s", apiName, fragment, sql)
							}
						}
						for _, fragment := range blockFragments {
							if strings.Contains(sql, fragment) {
								t.Fatalf("%s SQL contains blocked fragment %q:\n%s", apiName, fragment, sql)
							}
						}
					}
				})
			}
		})
	}
}

func TestLikeOperatorsQuoteEscaping(t *testing.T) {
	tr := setupTestTranspiler(t)

	tests := []struct {
		name     string
		jsonExpr string
		want     string
	}{
		{
			name:     "startsWith with apostrophe",
			jsonExpr: `{"startsWith": [{"var": "column"}, "it's"]}`,
			want:     "column LIKE 'it''s%'",
		},
		{
			name:     "!startsWith with apostrophe",
			jsonExpr: `{"!startsWith": [{"var": "column"}, "it's"]}`,
			want:     "column NOT LIKE 'it''s%'",
		},
		{
			name:     "endsWith with apostrophe",
			jsonExpr: `{"endsWith": [{"var": "column"}, "it's"]}`,
			want:     "column LIKE '%it''s'",
		},
		{
			name:     "!endsWith with apostrophe",
			jsonExpr: `{"!endsWith": [{"var": "column"}, "it's"]}`,
			want:     "column NOT LIKE '%it''s'",
		},
		{
			name:     "contains with apostrophe",
			jsonExpr: `{"contains": [{"var": "column"}, "it's"]}`,
			want:     "column LIKE '%it''s%'",
		},
		{
			name:     "!contains with apostrophe",
			jsonExpr: `{"!contains": [{"var": "column"}, "it's"]}`,
			want:     "column NOT LIKE '%it''s%'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tr.TranspileCondition(tt.jsonExpr)
			if err != nil {
				t.Fatalf("Transpile error: %v", err)
			}
			if got != tt.want {
				t.Errorf("TranspileCondition(%s)\n  got:  %s\n  want: %s", tt.jsonExpr, got, tt.want)
			}
		})
	}
}

func TestLikeOperatorsPlainStrings(t *testing.T) {
	tr := setupTestTranspiler(t)

	tests := []struct {
		name     string
		jsonExpr string
		want     string
	}{
		{
			name:     "startsWith plain",
			jsonExpr: `{"startsWith": [{"var": "name"}, "Alice"]}`,
			want:     "name LIKE 'Alice%'",
		},
		{
			name:     "endsWith plain",
			jsonExpr: `{"endsWith": [{"var": "email"}, "@company.com"]}`,
			want:     "email LIKE '%@company.com'",
		},
		{
			name:     "contains plain",
			jsonExpr: `{"contains": [{"var": "desc"}, "hello"]}`,
			want:     "desc LIKE '%hello%'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tr.TranspileCondition(tt.jsonExpr)
			if err != nil {
				t.Fatalf("Transpile error: %v", err)
			}
			if got != tt.want {
				t.Errorf("TranspileCondition(%s)\n  got:  %s\n  want: %s", tt.jsonExpr, got, tt.want)
			}
		})
	}
}

func TestLikeOperatorsWildcardEscaping(t *testing.T) {
	tr := setupTestTranspiler(t)

	tests := []struct {
		name     string
		jsonExpr string
		want     string
	}{
		{
			name:     "contains percent",
			jsonExpr: `{"contains": [{"var": "col"}, "100%"]}`,
			want:     `col LIKE '%100\%%'`,
		},
		{
			name:     "startsWith underscore",
			jsonExpr: `{"startsWith": [{"var": "col"}, "_private"]}`,
			want:     `col LIKE '\_private%'`,
		},
		{
			name:     "endsWith mixed apostrophe and wildcard",
			jsonExpr: `{"endsWith": [{"var": "col"}, "it's 100%"]}`,
			want:     `col LIKE '%it''s 100\%'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tr.TranspileCondition(tt.jsonExpr)
			if err != nil {
				t.Fatalf("Transpile error: %v", err)
			}
			if got != tt.want {
				t.Errorf("TranspileCondition(%s)\n  got:  %s\n  want: %s", tt.jsonExpr, got, tt.want)
			}
		})
	}
}

func TestContainsReversedWithApostrophe(t *testing.T) {
	tr := setupTestTranspiler(t)

	got, err := tr.TranspileCondition(`{"contains": ["it's", {"var": "column"}]}`)
	if err != nil {
		t.Fatalf("Transpile error: %v", err)
	}
	want := "column LIKE '%it''s%'"
	if got != want {
		t.Errorf("reversed contains\n  got:  %s\n  want: %s", got, want)
	}
}

func TestPrintParams(t *testing.T) {
	tests := []struct {
		name   string
		params []jsonlogic2sql.QueryParam
		want   string
	}{
		{
			name:   "no params",
			params: nil,
			want:   "Params: (none)\n",
		},
		{
			name:   "empty slice",
			params: []jsonlogic2sql.QueryParam{},
			want:   "Params: (none)\n",
		},
		{
			name: "single string param",
			params: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: "active"},
			},
			want: `Params: [{p1: "active"}]` + "\n",
		},
		{
			name: "single numeric param",
			params: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: float64(1000)},
			},
			want: "Params: [{p1: 1000}]\n",
		},
		{
			name: "multiple mixed params",
			params: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: "active"},
				{Name: "p2", Value: float64(1000)},
			},
			want: `Params: [{p1: "active"}, {p2: 1000}]` + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			printParams(tt.params)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			_, _ = buf.ReadFrom(r)
			got := buf.String()
			if got != tt.want {
				t.Errorf("printParams() output:\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}

func TestParamsModeToggle(t *testing.T) {
	origMode := paramsMode
	defer func() { paramsMode = origMode }()

	paramsMode = false
	paramsMode = !paramsMode
	if !paramsMode {
		t.Error("expected paramsMode to be true after toggle")
	}
	paramsMode = !paramsMode
	if paramsMode {
		t.Error("expected paramsMode to be false after second toggle")
	}
}

func TestSelectExpressionMode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "default condition", input: "\n", want: false},
		{name: "condition number", input: "1\n", want: false},
		{name: "condition name", input: "condition\n", want: false},
		{name: "predicate alias", input: "predicate\n", want: false},
		{name: "value number", input: "2\n", want: true},
		{name: "value name", input: "value\n", want: true},
		{name: "invalid defaults condition", input: "other\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(tt.input))
			if got := selectExpressionMode(scanner); got != tt.want {
				t.Fatalf("selectExpressionMode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExpressionModeName(t *testing.T) {
	origMode := valueMode
	defer func() { valueMode = origMode }()

	valueMode = false
	if got := expressionModeName(); got != modeCondition {
		t.Fatalf("expressionModeName() = %q, want %q", got, modeCondition)
	}
	valueMode = true
	if got := expressionModeName(); got != modeValue {
		t.Fatalf("expressionModeName() = %q, want %q", got, modeValue)
	}
}

func TestTranspileParameterized_BigQuery(t *testing.T) {
	tr := setupTestTranspiler(t)

	tests := []struct {
		name       string
		jsonExpr   string
		wantSQL    string
		wantParams []jsonlogic2sql.QueryParam
	}{
		{
			name:     "simple equality",
			jsonExpr: `{"==": [{"var": "status"}, "active"]}`,
			wantSQL:  "status = @p1",
			wantParams: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: "active"},
			},
		},
		{
			name:     "numeric comparison",
			jsonExpr: `{">": [{"var": "amount"}, 1000]}`,
			wantSQL:  "amount > @p1",
			wantParams: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: float64(1000)},
			},
		},
		{
			name:     "AND with mixed types",
			jsonExpr: `{"and": [{"==": [{"var": "status"}, "pending"]}, {">": [{"var": "amount"}, 5000]}]}`,
			wantSQL:  "(status = @p1 AND amount > @p2)",
			wantParams: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: "pending"},
				{Name: "p2", Value: float64(5000)},
			},
		},
		{
			name:     "IN array",
			jsonExpr: `{"in": [{"var": "country"}, ["US", "CA"]]}`,
			wantSQL:  "country IN (@p1, @p2)",
			wantParams: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: "US"},
				{Name: "p2", Value: "CA"},
			},
		},
		{
			name:       "null comparison produces no params",
			jsonExpr:   `{"==": [{"var": "deleted_at"}, null]}`,
			wantSQL:    "deleted_at IS NULL",
			wantParams: nil,
		},
		{
			name:       "boolean comparison produces no params",
			jsonExpr:   `{"==": [{"var": "active"}, true]}`,
			wantSQL:    "active = TRUE",
			wantParams: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, params, err := tr.TranspileParameterizedCondition(tt.jsonExpr)
			if err != nil {
				t.Fatalf("TranspileParameterized error: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("SQL:\n  got:  %s\n  want: %s", sql, tt.wantSQL)
			}
			if tt.wantParams == nil {
				if len(params) != 0 {
					t.Errorf("Params: got %v, want nil/empty", params)
				}
			} else {
				if len(params) != len(tt.wantParams) {
					t.Fatalf("Params length: got %d, want %d", len(params), len(tt.wantParams))
				}
				for i, want := range tt.wantParams {
					if params[i].Name != want.Name {
						t.Errorf("Param[%d].Name: got %q, want %q", i, params[i].Name, want.Name)
					}
					if fmt.Sprintf("%v", params[i].Value) != fmt.Sprintf("%v", want.Value) {
						t.Errorf("Param[%d].Value: got %v, want %v", i, params[i].Value, want.Value)
					}
				}
			}
		})
	}
}

func TestTranspileParameterized_PostgreSQL(t *testing.T) {
	tr, err := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectPostgreSQL, replTestSchema(t))
	if err != nil {
		t.Fatalf("NewTranspiler: %v", err)
	}

	sql, params, err := tr.TranspileParameterizedCondition(`{"==": [{"var": "email"}, "alice@example.com"]}`)
	if err != nil {
		t.Fatalf("TranspileParameterized error: %v", err)
	}

	wantSQL := "email = $1"
	if sql != wantSQL {
		t.Errorf("SQL: got %q, want %q", sql, wantSQL)
	}
	if len(params) != 1 || params[0].Name != "p1" || params[0].Value != "alice@example.com" {
		t.Errorf("Params: got %v, want [{p1 alice@example.com}]", params)
	}
}

func TestTranspileParameterized_CustomOperator(t *testing.T) {
	tr := setupTestTranspiler(t)

	sql, _, err := tr.TranspileParameterizedValue(`{"toLower": [{"var": "name"}]}`)
	if err != nil {
		t.Fatalf("TranspileParameterized error: %v", err)
	}

	wantSQL := "LOWER(name)"
	if sql != wantSQL {
		t.Errorf("SQL: got %q, want %q", sql, wantSQL)
	}
}

func TestTranspileParameterized_Error(t *testing.T) {
	tr := setupTestTranspiler(t)

	_, _, err := tr.TranspileParameterizedCondition(`{invalid json}`)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}

	_, _, err = tr.TranspileParameterizedCondition(`{"unknownOp": [1, 2]}`)
	if err == nil {
		t.Fatal("expected error for unknown operator")
	}
}

func TestTranspileParameterized_LikeOperators(t *testing.T) {
	tests := []struct {
		name       string
		jsonExpr   string
		wantSQL    string
		wantParams []jsonlogic2sql.QueryParam
	}{
		{
			name:     "startsWith parameterized",
			jsonExpr: `{"startsWith": [{"var": "name"}, "Al"]}`,
			wantSQL:  "name LIKE CONCAT(REPLACE(REPLACE(REPLACE(CAST(@p1 AS STRING), '\\\\', '\\\\\\\\'), '%', '\\%'), '_', '\\_'), '%')",
			wantParams: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: "Al"},
			},
		},
		{
			name:     "!startsWith parameterized",
			jsonExpr: `{"!startsWith": [{"var": "name"}, "Al"]}`,
			wantSQL:  "name NOT LIKE CONCAT(REPLACE(REPLACE(REPLACE(CAST(@p1 AS STRING), '\\\\', '\\\\\\\\'), '%', '\\%'), '_', '\\_'), '%')",
			wantParams: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: "Al"},
			},
		},
		{
			name:     "endsWith parameterized",
			jsonExpr: `{"endsWith": [{"var": "email"}, "@example.com"]}`,
			wantSQL:  "email LIKE CONCAT('%', REPLACE(REPLACE(REPLACE(CAST(@p1 AS STRING), '\\\\', '\\\\\\\\'), '%', '\\%'), '_', '\\_'))",
			wantParams: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: "@example.com"},
			},
		},
		{
			name:     "!endsWith parameterized",
			jsonExpr: `{"!endsWith": [{"var": "email"}, "@example.com"]}`,
			wantSQL:  "email NOT LIKE CONCAT('%', REPLACE(REPLACE(REPLACE(CAST(@p1 AS STRING), '\\\\', '\\\\\\\\'), '%', '\\%'), '_', '\\_'))",
			wantParams: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: "@example.com"},
			},
		},
		{
			name:     "contains parameterized",
			jsonExpr: `{"contains": [{"var": "desc"}, "hello"]}`,
			wantSQL:  "desc LIKE CONCAT('%', REPLACE(REPLACE(REPLACE(CAST(@p1 AS STRING), '\\\\', '\\\\\\\\'), '%', '\\%'), '_', '\\_'), '%')",
			wantParams: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: "hello"},
			},
		},
		{
			name:     "!contains parameterized",
			jsonExpr: `{"!contains": [{"var": "desc"}, "hello"]}`,
			wantSQL:  "desc NOT LIKE CONCAT('%', REPLACE(REPLACE(REPLACE(CAST(@p1 AS STRING), '\\\\', '\\\\\\\\'), '%', '\\%'), '_', '\\_'), '%')",
			wantParams: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: "hello"},
			},
		},
	}

	tr := setupTestTranspiler(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sql, params, err := tr.TranspileParameterizedCondition(tt.jsonExpr)
			if err != nil {
				t.Fatalf("TranspileParameterized error: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("SQL:\n  got:  %s\n  want: %s", sql, tt.wantSQL)
			}
			if len(params) != len(tt.wantParams) {
				t.Fatalf("Params length: got %d, want %d", len(params), len(tt.wantParams))
			}
			for i, want := range tt.wantParams {
				if params[i].Name != want.Name {
					t.Errorf("Param[%d].Name: got %q, want %q", i, params[i].Name, want.Name)
				}
				if fmt.Sprintf("%v", params[i].Value) != fmt.Sprintf("%v", want.Value) {
					t.Errorf("Param[%d].Value: got %v, want %v", i, params[i].Value, want.Value)
				}
			}
		})
	}
}

func TestTranspileParameterized_LikeOperators_PlaceholderNotQuoted(t *testing.T) {
	tests := []struct {
		name         string
		dialect      jsonlogic2sql.Dialect
		jsonExpr     string
		placeholder  string
		quotedShould string
	}{
		{
			name:         "bigquery startsWith placeholder not quoted",
			dialect:      jsonlogic2sql.DialectBigQuery,
			jsonExpr:     `{"startsWith": [{"var": "name"}, "Al"]}`,
			placeholder:  "@p1",
			quotedShould: "'@p1%",
		},
		{
			name:         "postgres startsWith placeholder not quoted",
			dialect:      jsonlogic2sql.DialectPostgreSQL,
			jsonExpr:     `{"startsWith": [{"var": "name"}, "Al"]}`,
			placeholder:  "$1",
			quotedShould: "'$1%",
		},
		{
			name:         "duckdb contains placeholder not quoted",
			dialect:      jsonlogic2sql.DialectDuckDB,
			jsonExpr:     `{"contains": [{"var": "name"}, "Al"]}`,
			placeholder:  "$1",
			quotedShould: "'%$1%",
		},
		{
			name:         "clickhouse contains placeholder not quoted",
			dialect:      jsonlogic2sql.DialectClickHouse,
			jsonExpr:     `{"contains": [{"var": "name"}, "Al"]}`,
			placeholder:  "{p1:String}",
			quotedShould: "'%{p1:String}%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentDialect = tt.dialect
			tr, err := jsonlogic2sql.NewTranspiler(tt.dialect, replTestSchema(t))
			if err != nil {
				t.Fatalf("NewTranspiler: %v", err)
			}
			registerCustomOperators(tr)

			sql, _, err := tr.TranspileParameterizedCondition(tt.jsonExpr)
			if err != nil {
				t.Fatalf("TranspileParameterized error: %v", err)
			}

			if strings.Contains(sql, tt.quotedShould) {
				t.Fatalf("placeholder appears quoted in LIKE pattern, SQL: %s", sql)
			}
			if !strings.Contains(sql, tt.placeholder) {
				t.Fatalf("SQL %q does not contain expected placeholder %q", sql, tt.placeholder)
			}
		})
	}
}

func TestLikeOperatorsNonParamStillWork(t *testing.T) {
	tr := setupTestTranspiler(t)

	tests := []struct {
		name     string
		jsonExpr string
		want     string
	}{
		{
			name:     "startsWith non-param",
			jsonExpr: `{"startsWith": [{"var": "name"}, "Alice"]}`,
			want:     "name LIKE 'Alice%'",
		},
		{
			name:     "endsWith non-param",
			jsonExpr: `{"endsWith": [{"var": "email"}, "@company.com"]}`,
			want:     "email LIKE '%@company.com'",
		},
		{
			name:     "contains non-param",
			jsonExpr: `{"contains": [{"var": "desc"}, "hello"]}`,
			want:     "desc LIKE '%hello%'",
		},
		{
			name:     "startsWith with apostrophe non-param",
			jsonExpr: `{"startsWith": [{"var": "column"}, "it's"]}`,
			want:     "column LIKE 'it''s%'",
		},
		{
			name:     "contains with apostrophe non-param",
			jsonExpr: `{"contains": [{"var": "column"}, "it's"]}`,
			want:     "column LIKE '%it''s%'",
		},
		{
			name:     "contains reversed args non-param",
			jsonExpr: `{"contains": ["foo", {"var": "name"}]}`,
			want:     "name LIKE '%foo%'",
		},
		{
			name:     "!contains reversed args non-param",
			jsonExpr: `{"!contains": ["bar", {"var": "col"}]}`,
			want:     "col NOT LIKE '%bar%'",
		},
		{
			name:     "contains with numeric pattern non-param",
			jsonExpr: `{"contains": [{"var": "desc"}, 1000]}`,
			want:     "desc LIKE '%1000%'",
		},
		{
			name:     "startsWith with numeric pattern non-param",
			jsonExpr: `{"startsWith": [{"var": "code"}, 404]}`,
			want:     "code LIKE '404%'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tr.TranspileCondition(tt.jsonExpr)
			if err != nil {
				t.Fatalf("Transpile error: %v", err)
			}
			if got != tt.want {
				t.Errorf("TranspileCondition(%s)\n  got:  %s\n  want: %s", tt.jsonExpr, got, tt.want)
			}
		})
	}
}

func TestContainsReversedArgsParameterized(t *testing.T) {
	escReplace := "REPLACE(REPLACE(REPLACE(CAST(@p1 AS STRING), '\\\\', '\\\\\\\\'), '%', '\\%'), '_', '\\_')"

	tests := []struct {
		name       string
		jsonExpr   string
		wantSQL    string
		wantParams []jsonlogic2sql.QueryParam
	}{
		{
			name:     "contains reversed parameterized",
			jsonExpr: `{"contains": ["foo", {"var": "name"}]}`,
			wantSQL:  "name LIKE CONCAT('%', " + escReplace + ", '%')",
			wantParams: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: "foo"},
			},
		},
		{
			name:     "!contains reversed parameterized",
			jsonExpr: `{"!contains": ["bar", {"var": "col"}]}`,
			wantSQL:  "col NOT LIKE CONCAT('%', " + escReplace + ", '%')",
			wantParams: []jsonlogic2sql.QueryParam{
				{Name: "p1", Value: "bar"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := setupTestTranspiler(t)
			sql, params, err := tr.TranspileParameterizedCondition(tt.jsonExpr)
			if err != nil {
				t.Fatalf("TranspileParameterized error: %v", err)
			}
			if sql != tt.wantSQL {
				t.Errorf("SQL:\n  got:  %s\n  want: %s", sql, tt.wantSQL)
			}
			if len(params) != len(tt.wantParams) {
				t.Fatalf("Params length: got %d, want %d", len(params), len(tt.wantParams))
			}
			for i, want := range tt.wantParams {
				if params[i].Name != want.Name || fmt.Sprintf("%v", params[i].Value) != fmt.Sprintf("%v", want.Value) {
					t.Errorf("Param[%d]: got {%s %v}, want {%s %v}", i, params[i].Name, params[i].Value, want.Name, want.Value)
				}
			}
		})
	}
}

func TestContainsArrayPatternParameterized(t *testing.T) {
	tr := setupTestTranspiler(t)

	// Non-param: array pattern ["x"] should produce the same LIKE as scalar "x"
	sqlNonParam, err := tr.TranspileCondition(`{"contains": [{"var": "col"}, ["x"]]}`)
	if err != nil {
		t.Fatalf("Transpile error: %v", err)
	}
	wantNonParam := "col LIKE '%x%'"
	if sqlNonParam != wantNonParam {
		t.Errorf("Non-param SQL:\n  got:  %s\n  want: %s", sqlNonParam, wantNonParam)
	}

	// Param: array pattern ["x"] should produce valid LIKE and bind the scalar element, not the slice.
	sqlParam, params, err := tr.TranspileParameterizedCondition(`{"contains": [{"var": "col"}, ["x"]]}`)
	if err != nil {
		t.Fatalf("TranspileParameterized error: %v", err)
	}
	wantParam := "col LIKE CONCAT('%', REPLACE(REPLACE(REPLACE(CAST(@p1 AS STRING), '\\\\', '\\\\\\\\'), '%', '\\%'), '_', '\\_'), '%')"
	if sqlParam != wantParam {
		t.Errorf("Param SQL:\n  got:  %s\n  want: %s", sqlParam, wantParam)
	}
	if len(params) != 1 || params[0].Value != "x" {
		t.Fatalf("Params = %#v, want scalar x", params)
	}
}

func TestContainsArrayPatternPostgreSQL(t *testing.T) {
	origDialect := currentDialect
	t.Cleanup(func() { currentDialect = origDialect })

	currentDialect = jsonlogic2sql.DialectPostgreSQL
	tr, err := jsonlogic2sql.NewTranspiler(jsonlogic2sql.DialectPostgreSQL, replTestSchema(t))
	if err != nil {
		t.Fatalf("NewTranspiler: %v", err)
	}
	registerCustomOperators(tr)

	sqlNonParam, err := tr.TranspileCondition(`{"contains": [{"var": "col"}, ["x"]]}`)
	if err != nil {
		t.Fatalf("Transpile error: %v", err)
	}
	wantNonParam := "col LIKE '%x%'"
	if sqlNonParam != wantNonParam {
		t.Errorf("Non-param SQL:\n  got:  %s\n  want: %s", sqlNonParam, wantNonParam)
	}

	sqlNotContains, err := tr.TranspileCondition(`{"!contains": [{"var": "col"}, ["x"]]}`)
	if err != nil {
		t.Fatalf("Transpile !contains error: %v", err)
	}
	wantNotContains := "col NOT LIKE '%x%'"
	if sqlNotContains != wantNotContains {
		t.Errorf("!contains SQL:\n  got:  %s\n  want: %s", sqlNotContains, wantNotContains)
	}

	sqlParam, params, err := tr.TranspileParameterizedCondition(`{"contains": [{"var": "col"}, ["x"]]}`)
	if err != nil {
		t.Fatalf("TranspileParameterized error: %v", err)
	}
	wantParam := "col LIKE CONCAT('%', REPLACE(REPLACE(REPLACE(CAST($1 AS TEXT), '\\\\', '\\\\\\\\'), '%', '\\%'), '_', '\\_'), '%')"
	if sqlParam != wantParam {
		t.Errorf("Param SQL:\n  got:  %s\n  want: %s", sqlParam, wantParam)
	}
	if len(params) != 1 || params[0].Value != "x" {
		t.Fatalf("Params = %#v, want scalar x", params)
	}

	sqlParamNotContains, params, err := tr.TranspileParameterizedCondition(`{"!contains": [{"var": "col"}, ["x"]]}`)
	if err != nil {
		t.Fatalf("TranspileParameterized !contains error: %v", err)
	}
	wantParamNotContains := "col NOT LIKE CONCAT('%', REPLACE(REPLACE(REPLACE(CAST($1 AS TEXT), '\\\\', '\\\\\\\\'), '%', '\\%'), '_', '\\_'), '%')"
	if sqlParamNotContains != wantParamNotContains {
		t.Errorf("Param !contains SQL:\n  got:  %s\n  want: %s", sqlParamNotContains, wantParamNotContains)
	}
	if len(params) != 1 || params[0].Value != "x" {
		t.Fatalf("!contains params = %#v, want scalar x", params)
	}
}

func TestRegexpContainsBigQuery_NonParameterized(t *testing.T) {
	tr := setupTestTranspiler(t)

	sql, err := tr.TranspileCondition(`{"regexpContains": [{"var": "email"}, "^foo.*bar$"]}`)
	if err != nil {
		t.Fatalf("Transpile error: %v", err)
	}

	want := "REGEXP_CONTAINS(email, r'^foo.*bar$')"
	if sql != want {
		t.Errorf("SQL:\n  got:  %s\n  want: %s", sql, want)
	}
}

func TestRegexpContainsBigQuery_Parameterized(t *testing.T) {
	tr := setupTestTranspiler(t)

	sql, params, err := tr.TranspileParameterizedCondition(`{"regexpContains": [{"var": "email"}, "^foo.*bar$"]}`)
	if err != nil {
		t.Fatalf("TranspileParameterized error: %v", err)
	}

	wantSQL := "REGEXP_CONTAINS(email, @p1)"
	if sql != wantSQL {
		t.Errorf("SQL:\n  got:  %s\n  want: %s", sql, wantSQL)
	}
	wantParams := []jsonlogic2sql.QueryParam{
		{Name: "p1", Value: "^foo.*bar$"},
	}
	if len(params) != len(wantParams) {
		t.Fatalf("Params length: got %d, want %d", len(params), len(wantParams))
	}
	for i, want := range wantParams {
		if params[i].Name != want.Name || fmt.Sprintf("%v", params[i].Value) != fmt.Sprintf("%v", want.Value) {
			t.Errorf("Param[%d]: got {%s %v}, want {%s %v}", i, params[i].Name, params[i].Value, want.Name, want.Value)
		}
	}
}

func TestNormalizeWaveDashUsesDialectSafeRegex(t *testing.T) {
	origDialect := currentDialect
	t.Cleanup(func() { currentDialect = origDialect })

	tests := []struct {
		name    string
		dialect jsonlogic2sql.Dialect
		want    string
	}{
		{
			name:    "bigquery raw re2 escape",
			dialect: jsonlogic2sql.DialectBigQuery,
			want:    `REGEXP_REPLACE(col, r'[\x{301C}\x{FF5E}]', '~')`,
		},
		{
			name:    "spanner raw re2 escape",
			dialect: jsonlogic2sql.DialectSpanner,
			want:    `REGEXP_REPLACE(col, r'[\x{301C}\x{FF5E}]', '~')`,
		},
		{
			name:    "postgres unicode escape string",
			dialect: jsonlogic2sql.DialectPostgreSQL,
			want:    `REGEXP_REPLACE(col, U&'[\301C\FF5E]', '~', 'g')`,
		},
		{
			name:    "duckdb global re2 replace",
			dialect: jsonlogic2sql.DialectDuckDB,
			want:    `regexp_replace(col, '[\x{301C}\x{FF5E}]', '~', 'g')`,
		},
		{
			name:    "clickhouse escaped re2 replace all",
			dialect: jsonlogic2sql.DialectClickHouse,
			want:    `replaceRegexpAll(col, '[\\x{301C}\\x{FF5E}]', '~')`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			currentDialect = tt.dialect
			tr, err := jsonlogic2sql.NewTranspiler(tt.dialect, replTestSchema(t))
			if err != nil {
				t.Fatalf("NewTranspiler: %v", err)
			}
			registerCustomOperators(tr)

			sql, err := tr.TranspileValue(`{"normalizeWaveDash": [{"var": "col"}]}`)
			if err != nil {
				t.Fatalf("TranspileValue error: %v", err)
			}
			if sql != tt.want {
				t.Fatalf("SQL:\n  got:  %s\n  want: %s", sql, tt.want)
			}
			if strings.Contains(sql, `\u301C`) || strings.Contains(sql, `\uFF5E`) {
				t.Fatalf("SQL still uses non-portable unicode escapes: %s", sql)
			}

			paramSQL, params, err := tr.TranspileParameterizedValue(`{"normalizeWaveDash": [{"var": "col"}]}`)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue error: %v", err)
			}
			if paramSQL != tt.want {
				t.Fatalf("Parameterized SQL:\n  got:  %s\n  want: %s", paramSQL, tt.want)
			}
			if len(params) != 0 {
				t.Fatalf("Params = %#v, want none", params)
			}
		})
	}
}

func TestReplExamplesTranspileInDefaultConditionMode(t *testing.T) {
	tr := setupTestTranspiler(t)

	for _, example := range replExamples() {
		t.Run(example.name, func(t *testing.T) {
			if _, err := tr.TranspileCondition(example.json); err != nil {
				t.Fatalf("example %q does not transpile in condition mode: %v", example.json, err)
			}
		})
	}
}
