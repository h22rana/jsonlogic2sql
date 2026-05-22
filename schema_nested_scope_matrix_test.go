package jsonlogic2sql

import (
	"fmt"
	"strings"
	"testing"
)

func nestedScopeAuditSchema() *Schema {
	accountElementFields := nestedScopeAuditAccountElementFields()
	return mustNewSchema([]FieldSchema{
		{Name: "status", Type: FieldTypeString},
		{Name: "useBackup", Type: FieldTypeBoolean},
		{Name: "useMetrics", Type: FieldTypeBoolean},
		{Name: "useAlt", Type: FieldTypeBoolean},
		{Name: "useProfile", Type: FieldTypeBoolean},
		{
			Name: "customer",
			Type: FieldTypeObject,
			Fields: []FieldSchema{
				{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"active", "blocked"}},
				{
					Name: "profile",
					Type: FieldTypeObject,
					Fields: []FieldSchema{
						{Name: "country", Type: FieldTypeString},
						{Name: "tier", Type: FieldTypeEnum, AllowedValues: []string{"gold", "silver"}},
					},
				},
			},
		},
		{
			Name: "profile",
			Type: FieldTypeObject,
			Fields: []FieldSchema{
				{Name: "status", Type: FieldTypeString},
			},
		},
		{
			Name:          "accounts",
			Type:          FieldTypeArray,
			ElementFields: accountElementFields,
		},
		{
			Name:          "backupAccounts",
			Type:          FieldTypeArray,
			ElementFields: accountElementFields,
		},
		{
			Name: "metricAccounts",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "status", Type: FieldTypeNumber},
			},
		},
		{
			Name: "altAccounts",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"active", "archived"}},
			},
		},
		{
			Name: "lightAccounts",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"active", "blocked"}},
			},
		},
		{
			Name: "parentGroupA",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{
					Name: "children",
					Type: FieldTypeArray,
					ElementFields: []FieldSchema{
						{Name: "y", Type: FieldTypeString},
					},
				},
			},
		},
		{
			Name: "parentGroupB",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{
					Name: "children",
					Type: FieldTypeArray,
					ElementFields: []FieldSchema{
						{Name: "z", Type: FieldTypeString},
					},
				},
			},
		},
	})
}

func nestedScopeAuditAccountElementFields() []FieldSchema {
	return []FieldSchema{
		{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"active", "blocked"}},
		{
			Name: "profile",
			Type: FieldTypeObject,
			Fields: []FieldSchema{
				{Name: "region", Type: FieldTypeString},
				{Name: "tier", Type: FieldTypeEnum, AllowedValues: []string{"gold", "silver"}},
			},
		},
		{
			Name: "transactions",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "amount", Type: FieldTypeNumber},
				{
					Name: "method",
					Type: FieldTypeObject,
					Fields: []FieldSchema{
						{Name: "type", Type: FieldTypeEnum, AllowedValues: []string{"BALANCE", "CARD"}},
						{Name: "issuer", Type: FieldTypeString},
					},
				},
				{
					Name: "flags",
					Type: FieldTypeArray,
					ElementFields: []FieldSchema{
						{Name: "code", Type: FieldTypeString},
					},
				},
			},
		},
	}
}

func TestNestedSchemaScopeAudit_AllDialects(t *testing.T) {
	t.Parallel()

	validCases := []struct {
		name      string
		logic     string
		valueRoot bool
		want      []string
		wantNot   []string
		paramLen  int
	}{
		{
			name:     "root object enum",
			logic:    `{"==":[{"var":"customer.status"},"active"]}`,
			want:     []string{"customer.status ="},
			paramLen: 1,
		},
		{
			name:     "array element enum",
			logic:    `{"some":[{"var":"accounts"},{"==":[{"var":"status"},"active"]}]}`,
			want:     []string{"elem.status ="},
			paramLen: 1,
		},
		{
			name:     "array element nested object enum",
			logic:    `{"some":[{"var":"accounts"},{"==":[{"var":"profile.tier"},"gold"]}]}`,
			want:     []string{"elem.profile.tier ="},
			paramLen: 1,
		},
		{
			name:  "nested array element object enum and numeric field",
			logic: `{"some":[{"var":"accounts"},{"some":[{"var":"transactions"},{"and":[{"==":[{"var":"method.type"},"BALANCE"]},{">":[{"var":"amount"},0]}]}]}]}`,
			want: []string{
				"elem1.method.type =",
				"elem1.amount >",
			},
			paramLen: 2,
		},
		{
			name:      "filter scoped missing nested object field",
			logic:     `{"filter":[{"var":"accounts"},{"missing":["profile.region"]}]}`,
			valueRoot: true,
			want:      []string{"elem.profile.region IS NULL"},
		},
		{
			name:      "map stringifies nested object and enum fields",
			logic:     `{"map":[{"var":"accounts"},{"cat":[{"var":"profile.region"},":",{"var":"status"}]}]}`,
			valueRoot: true,
			want:      []string{"elem.profile.region", "elem.status"},
			paramLen:  1,
		},
		{
			name:      "three-level nested array source keeps scoped schema",
			logic:     `{"map":[{"var":"accounts"},{"map":[{"var":"transactions"},{"filter":[{"var":"flags"},{"==":[{"var":"code"},"risk"]}]}]}]}`,
			valueRoot: true,
			want:      []string{"elem.transactions", "elem1.flags", "elem2.code ="},
			paramLen:  1,
		},
		{
			name:      "map over filtered array preserves element schema",
			logic:     `{"map":[{"filter":[{"var":"accounts"},{"==":[{"var":"status"},"active"]}]},{"var":"profile.tier"}]}`,
			valueRoot: true,
			want:      []string{"elem.status =", "elem.profile.tier"},
			paramLen:  1,
		},
		{
			name:     "some over filtered array preserves element schema",
			logic:    `{"some":[{"filter":[{"var":"accounts"},{"==":[{"var":"status"},"active"]}]},{"==":[{"var":"profile.tier"},"gold"]}]}`,
			want:     []string{"elem.status =", "elem.profile.tier ="},
			paramLen: 2,
		},
		{
			name:      "filter over identity map preserves element schema",
			logic:     `{"filter":[{"map":[{"var":"accounts"},{"var":""}]},{"==":[{"var":"profile.region"},"JP"]}]}`,
			valueRoot: true,
			want:      []string{"elem.profile.region ="},
			paramLen:  1,
		},
		{
			name:      "nested map over filtered scoped array preserves element schema",
			logic:     `{"map":[{"var":"accounts"},{"map":[{"filter":[{"var":"transactions"},{">":[{"var":"amount"},0]}]},{"var":"method.type"}]}]}`,
			valueRoot: true,
			want:      []string{"elem2.amount >", "elem1.method.type"},
			paramLen:  1,
		},
		{
			name:      "map over constant if array source preserves element schema",
			logic:     `{"map":[{"if":[true,{"var":"accounts"},[]]},{"var":"profile.tier"}]}`,
			valueRoot: true,
			want:      []string{"elem.profile.tier"},
		},
		{
			name:      "map over fallback or array source preserves element schema",
			logic:     `{"map":[{"or":[false,{"var":"accounts"}]},{"var":"profile.region"}]}`,
			valueRoot: true,
			want:      []string{"elem.profile.region"},
		},
		{
			name:      "map over dynamic if compatible array sources preserves element schema",
			logic:     `{"map":[{"if":[{"var":"useBackup"},{"var":"accounts"},{"var":"backupAccounts"}]},{"var":"profile.tier"}]}`,
			valueRoot: true,
			want:      []string{"CASE WHEN useBackup IS TRUE THEN accounts ELSE backupAccounts END", "elem.profile.tier"},
		},
		{
			name:      "nested map over dynamic if compatible array sources preserves nested element schema",
			logic:     `{"map":[{"if":[{"var":"useBackup"},{"var":"accounts"},{"var":"backupAccounts"}]},{"map":[{"var":"transactions"},{"var":"method.type"}]}]}`,
			valueRoot: true,
			want:      []string{"elem.transactions", "elem1.method.type"},
		},
		{
			name:     "some over dynamic if compatible array sources validates scoped enum",
			logic:    `{"some":[{"if":[{"var":"useBackup"},{"var":"accounts"},{"var":"backupAccounts"}]},{"==":[{"var":"status"},"active"]}]}`,
			want:     []string{"elem.status ="},
			paramLen: 1,
		},
		{
			name:      "map over overflowed truthy if source chooses reachable schema",
			logic:     `{"map":[{"if":[1e9999,{"var":"accounts"},{"var":"metricAccounts"}]},{"var":"status"}]}`,
			valueRoot: true,
			want:      []string{"accounts", "elem.status"},
			wantNot:   []string{"metricAccounts"},
		},
		{
			name:      "map over underflowed falsy if source chooses reachable schema",
			logic:     `{"map":[{"if":[1e-9999,{"var":"accounts"},{"var":"metricAccounts"}]},{"var":"status"}]}`,
			valueRoot: true,
			want:      []string{"metricAccounts", "elem.status"},
			wantNot:   []string{"UNNEST(accounts)", "arrayMap(elem -> elem.status, accounts)"},
		},
	}

	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-required", schema: nestedScopeAuditSchema()},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			t.Parallel()

			for _, d := range allDialects() {
				t.Run(d.String(), func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{Dialect: d, Schema: mode.schema})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}

					for _, tc := range validCases {
						t.Run(tc.name, func(t *testing.T) {
							t.Parallel()

							sql, params, inlineErr := transpileAuditCase(t, tr, tc.logic, tc.valueRoot, false)
							if inlineErr != nil {
								t.Fatalf("inline transpilation error = %v", inlineErr)
							}
							assertSQLContainsAll(t, sql, tc.want)
							assertSQLContainsNone(t, sql, tc.wantNot)
							if len(params) != 0 {
								t.Fatalf("inline params = %#v, want none", params)
							}

							paramSQL, params, paramErr := transpileAuditCase(t, tr, tc.logic, tc.valueRoot, true)
							if paramErr != nil {
								t.Fatalf("parameterized transpilation error = %v", paramErr)
							}
							assertSQLContainsAll(t, paramSQL, tc.want)
							assertSQLContainsNone(t, paramSQL, tc.wantNot)
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

func TestNestedSchemaScopeAuditRejectsInvalidSchemaRequiredCases_AllDialects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		logic     string
		valueRoot bool
		wantError string
	}{
		{
			name:      "root object enum rejects invalid value",
			logic:     `{"==":[{"var":"customer.status"},"archived"]}`,
			wantError: "invalid enum value 'archived' for field 'customer.status'",
		},
		{
			name:      "array element enum rejects invalid value",
			logic:     `{"some":[{"var":"accounts"},{"==":[{"var":"status"},"archived"]}]}`,
			wantError: "invalid enum value 'archived' for field 'accounts.status'",
		},
		{
			name:      "array element nested object enum rejects invalid value",
			logic:     `{"some":[{"var":"accounts"},{"==":[{"var":"profile.tier"},"platinum"]}]}`,
			wantError: "invalid enum value 'platinum' for field 'accounts.profile.tier'",
		},
		{
			name:      "nested array enum rejects invalid in value",
			logic:     `{"some":[{"var":"accounts"},{"some":[{"var":"transactions"},{"in":[{"var":"method.type"},["BALANCE","CASH"]]}]}]}`,
			wantError: "invalid enum value 'CASH' for field 'accounts.transactions.method.type'",
		},
		{
			name:      "unknown nested object field is rejected",
			logic:     `{"some":[{"var":"accounts"},{"==":[{"var":"profile.unknown"},"JP"]}]}`,
			wantError: "field 'profile.unknown' is not defined in schema scope 'accounts'",
		},
		{
			name:      "scoped missing rejects unknown nested object field",
			logic:     `{"filter":[{"var":"accounts"},{"missing":["profile.unknown"]}]}`,
			valueRoot: true,
			wantError: "field 'profile.unknown' is not defined in schema scope 'accounts'",
		},
		{
			name:      "array operator rejects non-array nested object field source",
			logic:     `{"some":[{"var":"accounts"},{"some":[{"var":"profile.region"},true]}]}`,
			wantError: "array operation on non-array field 'accounts.profile.region' (type: string)",
		},
		{
			name:      "map over filtered array rejects unknown scoped field",
			logic:     `{"map":[{"filter":[{"var":"accounts"},{"==":[{"var":"status"},"active"]}]},{"var":"unknown"}]}`,
			valueRoot: true,
			wantError: "field 'unknown' is not defined in schema scope 'accounts'",
		},
		{
			name:      "defaulted scoped enum var validates default value",
			logic:     `{"some":[{"var":"accounts"},{"==":[{"var":["status","archived"]},"active"]}]}`,
			wantError: "invalid enum value 'archived' for field 'accounts.status'",
		},
		{
			name:      "dynamic array source rejects incompatible scoped field types",
			logic:     `{"map":[{"if":[{"var":"useMetrics"},{"var":"accounts"},{"var":"metricAccounts"}]},{"var":"status"}]}`,
			valueRoot: true,
			wantError: "field 'status' has incompatible schema types across array source scopes",
		},
		{
			name:      "dynamic array source rejects incompatible scoped enum values",
			logic:     `{"some":[{"if":[{"var":"useAlt"},{"var":"accounts"},{"var":"altAccounts"}]},{"==":[{"var":"status"},"active"]}]}`,
			wantError: "field 'status' has incompatible enum values across array source scopes",
		},
		{
			name:      "dynamic array source rejects field missing from one source scope",
			logic:     `{"map":[{"if":[{"var":"useBackup"},{"var":"accounts"},{"var":"lightAccounts"}]},{"var":"profile.tier"}]}`,
			valueRoot: true,
			wantError: "field 'profile.tier' is not defined in schema scope 'lightAccounts'",
		},
		{
			name:      "dynamic array source rejects object branch even when scoped field exists",
			logic:     `{"map":[{"if":[{"var":"useProfile"},{"var":"accounts"},{"var":"profile"}]},{"var":"status"}]}`,
			valueRoot: true,
			wantError: "array operation on non-array field 'profile' (type: object)",
		},
		{
			name:      "dynamic nested array source rejects field missing from later source scope",
			logic:     `{"map":[{"if":[{"var":"useBackup"},{"var":"parentGroupA"},{"var":"parentGroupB"}]},{"map":[{"var":"children"},{"var":"y"}]}]}`,
			valueRoot: true,
			wantError: "field 'y' is not defined in schema scope 'parentGroupB.children'",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{Dialect: d, Schema: nestedScopeAuditSchema()})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					_, _, err := transpileAuditCase(t, tr, tc.logic, tc.valueRoot, false)
					if err == nil || !strings.Contains(err.Error(), tc.wantError) {
						t.Fatalf("inline error = %v, want containing %q", err, tc.wantError)
					}

					sql, params, err := transpileAuditCase(t, tr, tc.logic, tc.valueRoot, true)
					if err == nil || !strings.Contains(err.Error(), tc.wantError) {
						t.Fatalf("parameterized error = %v, want containing %q (SQL %q params %#v)",
							err, tc.wantError, sql, params)
					}
				})
			}
		})
	}
}

func TestNestedSchemaScopeRejectsDynamicNonArraySourcesForAllArrayOperators_AllDialects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		logic     string
		valueRoot bool
	}{
		{
			name:      "map",
			logic:     `{"map":[{"if":[{"var":"useProfile"},{"var":"accounts"},{"var":"profile"}]},{"var":"status"}]}`,
			valueRoot: true,
		},
		{
			name:      "filter",
			logic:     `{"filter":[{"if":[{"var":"useProfile"},{"var":"accounts"},{"var":"profile"}]},true]}`,
			valueRoot: true,
		},
		{
			name:  "all",
			logic: `{"all":[{"if":[{"var":"useProfile"},{"var":"accounts"},{"var":"profile"}]},true]}`,
		},
		{
			name:  "some",
			logic: `{"some":[{"if":[{"var":"useProfile"},{"var":"accounts"},{"var":"profile"}]},true]}`,
		},
		{
			name:  "none",
			logic: `{"none":[{"if":[{"var":"useProfile"},{"var":"accounts"},{"var":"profile"}]},true]}`,
		},
		{
			name:      "reduce",
			logic:     `{"reduce":[{"if":[{"var":"useProfile"},{"var":"accounts"},{"var":"profile"}]},{"var":"accumulator"},0]}`,
			valueRoot: true,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  nestedScopeAuditSchema(),
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					_, _, err := transpileAuditCase(t, tr, tc.logic, tc.valueRoot, false)
					assertDynamicNonArraySourceError(t, err)

					sql, params, err := transpileAuditCase(t, tr, tc.logic, tc.valueRoot, true)
					if err == nil {
						t.Fatalf("parameterized transpilation unexpectedly succeeded: SQL %q params %#v", sql, params)
					}
					assertDynamicNonArraySourceError(t, err)
				})
			}
		})
	}
}

func TestNestedSchemaScopeRejectsUnknownElementSchemaForScopedFields_AllDialects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		logic     string
		valueRoot bool
	}{
		{
			name:      "map",
			logic:     `{"map":[{"map":[{"var":"accounts"},{"var":"status"}]},{"var":"status"}]}`,
			valueRoot: true,
		},
		{
			name:      "filter",
			logic:     `{"filter":[{"map":[{"var":"accounts"},{"var":"status"}]},{"var":"status"}]}`,
			valueRoot: true,
		},
		{
			name:  "all",
			logic: `{"all":[{"map":[{"var":"accounts"},{"var":"status"}]},{"var":"status"}]}`,
		},
		{
			name:  "some",
			logic: `{"some":[{"map":[{"var":"accounts"},{"var":"status"}]},{"var":"status"}]}`,
		},
		{
			name:  "none",
			logic: `{"none":[{"map":[{"var":"accounts"},{"var":"status"}]},{"var":"status"}]}`,
		},
		{
			name:      "reduce",
			logic:     `{"reduce":[{"map":[{"var":"accounts"},{"var":"status"}]},{"var":"current.status"},0]}`,
			valueRoot: true,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  nestedScopeAuditSchema(),
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					_, _, err := transpileAuditCase(t, tr, tc.logic, tc.valueRoot, false)
					assertUnknownElementSchemaError(t, err)

					sql, params, err := transpileAuditCase(t, tr, tc.logic, tc.valueRoot, true)
					if err == nil {
						t.Fatalf("parameterized transpilation unexpectedly succeeded: SQL %q params %#v", sql, params)
					}
					assertUnknownElementSchemaError(t, err)
				})
			}
		})
	}
}

func TestNestedSchemaScopedFieldsInsideCustomOperators_AllDialects(t *testing.T) {
	t.Parallel()

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  nestedScopeAuditSchema(),
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}
			registerNestedScopeAuditCustomOperators(t, tr)

			logic := `{"some":[{"var":"accounts"},{"==":[{"upper":[{"var":"profile.region"}]},"JP"]}]}`
			sql, err := tr.TranspileCondition(logic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			assertSQLContainsAll(t, sql, []string{"UPPER(elem.profile.region)", "= 'JP'"})

			paramSQL, params, err := tr.TranspileParameterizedCondition(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			assertSQLContainsAll(t, paramSQL, []string{"UPPER(elem.profile.region)", "="})
			if len(params) != 1 || params[0].Value != "JP" {
				t.Fatalf("params = %#v, want one JP parameter", params)
			}

			invalid := `{"some":[{"var":"accounts"},{"==":[{"upper":[{"var":"profile.unknown"}]},"JP"]}]}`
			if _, err := tr.TranspileCondition(invalid); err == nil ||
				!strings.Contains(err.Error(), "field 'profile.unknown' is not defined in schema scope 'accounts'") {
				t.Fatalf("custom operator invalid scoped field error = %v", err)
			}
		})
	}
}

func registerNestedScopeAuditCustomOperators(t *testing.T, tr *Transpiler) {
	t.Helper()

	if err := tr.RegisterOperatorFunc("upper", func(_ string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 1 {
			return OperatorResult{}, fmt.Errorf("upper requires exactly 1 argument")
		}
		if args[0].Kind != ExpressionKindValue || args[0].Type != ExpressionTypeString {
			return OperatorResult{}, fmt.Errorf("upper requires a string value argument")
		}
		return ValueSQL(fmt.Sprintf("UPPER(%s)", args[0].SQL), ExpressionTypeString), nil
	}); err != nil {
		t.Fatalf("RegisterOperatorFunc(upper) error = %v", err)
	}
}

func transpileAuditCase(t *testing.T, tr *Transpiler, logic string, valueRoot, parameterized bool) (string, []QueryParam, error) {
	t.Helper()

	if parameterized {
		if valueRoot {
			return tr.TranspileParameterizedValue(logic)
		}
		return tr.TranspileParameterizedCondition(logic)
	}
	if valueRoot {
		sql, err := tr.TranspileValue(logic)
		return sql, nil, err
	}
	sql, err := tr.TranspileCondition(logic)
	return sql, nil, err
}

func assertSQLContainsAll(t *testing.T, sql string, fragments []string) {
	t.Helper()
	for _, fragment := range fragments {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("SQL = %q, want to contain %q", sql, fragment)
		}
	}
}

func assertSQLContainsNone(t *testing.T, sql string, fragments []string) {
	t.Helper()
	for _, fragment := range fragments {
		if strings.Contains(sql, fragment) {
			t.Fatalf("SQL = %q, should not contain %q", sql, fragment)
		}
	}
}

func assertDynamicNonArraySourceError(t *testing.T, err error) {
	t.Helper()
	const want = "array operation on non-array field 'profile' (type: object)"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want containing %q", err, want)
	}
}

func assertUnknownElementSchemaError(t *testing.T, err error) {
	t.Helper()
	const want = "cannot be validated because the array element schema is unknown"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want containing %q", err, want)
	}
}
