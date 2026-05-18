package jsonlogic2sql

import (
	"fmt"
	"strings"
	"testing"
)

func nestedScopeAuditSchema() *Schema {
	return mustNewSchema([]FieldSchema{
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
			Name: "accounts",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
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
			},
		},
	})
}

func TestNestedSchemaScopeAudit_AllDialects(t *testing.T) {
	t.Parallel()

	validCases := []struct {
		name      string
		logic     string
		valueRoot bool
		want      []string
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
	}

	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-aware", schema: nestedScopeAuditSchema()},
		{name: "schema-less"},
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
							if len(params) != 0 {
								t.Fatalf("inline params = %#v, want none", params)
							}

							paramSQL, params, paramErr := transpileAuditCase(t, tr, tc.logic, tc.valueRoot, true)
							if paramErr != nil {
								t.Fatalf("parameterized transpilation error = %v", paramErr)
							}
							assertSQLContainsAll(t, paramSQL, tc.want)
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

func TestNestedSchemaScopeAuditRejectsInvalidSchemaAwareCases_AllDialects(t *testing.T) {
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
			name:      "defaulted scoped enum var validates default value",
			logic:     `{"some":[{"var":"accounts"},{"==":[{"var":["status","archived"]},"active"]}]}`,
			wantError: "invalid enum value 'archived' for field 'accounts.status'",
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
