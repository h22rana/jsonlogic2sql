package jsonlogic2sql

import "testing"

func TestInArrayMembership_NullSafeRuntimeArray_AllDialectsAndModes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "needle", Type: FieldTypeString},
		{Name: "tags", Type: FieldTypeArray},
	})
	logic := `{"in":[{"var":"needle"},{"var":"tags"}]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			predicate := testNullSafeArrayMembershipSQL(d, "needle", "tags")
			gotCondition, err := tr.TranspileCondition(logic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			if gotCondition != predicate {
				t.Fatalf("TranspileCondition() = %q, want %q", gotCondition, predicate)
			}

			gotParamCondition, params, err := tr.TranspileParameterizedCondition(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotParamCondition != predicate {
				t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParamCondition, predicate)
			}
			if len(params) != 0 {
				t.Fatalf("condition params = %#v, want none", params)
			}

			valuePredicate := "CASE WHEN " + predicate + " THEN TRUE ELSE FALSE END"
			gotValue, err := tr.TranspileValue(logic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if gotValue != valuePredicate {
				t.Fatalf("TranspileValue() = %q, want %q", gotValue, valuePredicate)
			}

			gotParamValue, valueParams, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			if gotParamValue != valuePredicate {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParamValue, valuePredicate)
			}
			if len(valueParams) != 0 {
				t.Fatalf("value params = %#v, want none", valueParams)
			}
		})
	}
}

func TestInArrayMembership_InternalAliasAvoidsUserFieldCollision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		schema      *Schema
		logic       string
		valueSQL    string
		arraySQL    string
		tableAlias  string
		memberAlias string
	}{
		{
			name: "needle field matches base internal alias",
			schema: mustNewSchema([]FieldSchema{
				{Name: "__j2s_member", Type: FieldTypeString},
				{Name: "tags", Type: FieldTypeArray},
			}),
			logic:       `{"in":[{"var":"__j2s_member"},{"var":"tags"}]}`,
			valueSQL:    "__j2s_member",
			arraySQL:    "tags",
			tableAlias:  "__j2s_members",
			memberAlias: "__j2s_member_1",
		},
		{
			name: "array field matches base internal alias",
			schema: mustNewSchema([]FieldSchema{
				{Name: "needle", Type: FieldTypeString},
				{Name: "__j2s_member", Type: FieldTypeArray},
			}),
			logic:       `{"in":[{"var":"needle"},{"var":"__j2s_member"}]}`,
			valueSQL:    "needle",
			arraySQL:    "__j2s_member",
			tableAlias:  "__j2s_members",
			memberAlias: "__j2s_member_1",
		},
		{
			name: "needle field also matches first suffixed alias",
			schema: mustNewSchema([]FieldSchema{
				{Name: "__j2s_member_1", Type: FieldTypeString},
				{Name: "tags", Type: FieldTypeArray},
			}),
			logic:       `{"in":[{"var":"__j2s_member_1"},{"var":"tags"}]}`,
			valueSQL:    "__j2s_member_1",
			arraySQL:    "tags",
			tableAlias:  "__j2s_members",
			memberAlias: "__j2s_member_2",
		},
		{
			name: "needle field matches internal table alias",
			schema: mustNewSchema([]FieldSchema{
				{Name: "__j2s_members", Type: FieldTypeString},
				{Name: "tags", Type: FieldTypeArray},
			}),
			logic:       `{"in":[{"var":"__j2s_members"},{"var":"tags"}]}`,
			valueSQL:    "__j2s_members",
			arraySQL:    "tags",
			tableAlias:  "__j2s_members_1",
			memberAlias: "__j2s_member_1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			for _, d := range allDialects() {
				t.Run(d.String(), func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  tt.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error = %v", err)
					}

					want := testArrayMembershipSQLWithAliases(d, tt.tableAlias, tt.memberAlias, tt.valueSQL, tt.arraySQL)
					got, err := tr.TranspileCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if got != want {
						t.Fatalf("TranspileCondition() = %q, want %q", got, want)
					}

					gotParam, params, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if len(params) != 0 {
						t.Fatalf("params = %#v, want none", params)
					}
				})
			}
		})
	}
}

func TestInLiteralArrayMembership_StrictAndNullSafe(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "status", Type: FieldTypeString},
		{Name: "amount", Type: FieldTypeInteger},
	})

	tests := []struct {
		name            string
		logic           string
		wantSQL         string
		wantParam       string
		wantParamValues []QueryParam
	}{
		{
			name:            "string field null plus string literal",
			logic:           `{"in":[{"var":"status"},[null,"active"]]}`,
			wantSQL:         testArrayLiteralMembershipSQL("status", "NULL", "'active'"),
			wantParam:       testArrayLiteralMembershipSQL("status", "NULL", "@p1"),
			wantParamValues: []QueryParam{{Name: "p1", Value: "active"}},
		},
		{
			name:      "string field null-only literal array",
			logic:     `{"in":[{"var":"status"},[null]]}`,
			wantSQL:   "status IS NULL",
			wantParam: "status IS NULL",
		},
		{
			name:      "string field numeric literal mismatch folds false",
			logic:     `{"in":[{"var":"status"},[1,2]]}`,
			wantSQL:   "FALSE",
			wantParam: "FALSE",
		},
		{
			name:      "integer field string literal mismatch folds false",
			logic:     `{"in":[{"var":"amount"},["1","2"]]}`,
			wantSQL:   "FALSE",
			wantParam: "FALSE",
		},
		{
			name:            "integer field null plus numeric literal",
			logic:           `{"in":[{"var":"amount"},[null,2]]}`,
			wantSQL:         testArrayLiteralMembershipSQL("amount", "NULL", "2"),
			wantParam:       testArrayLiteralMembershipSQL("amount", "NULL", "@p1"),
			wantParamValues: []QueryParam{{Name: "p1", Value: float64(2)}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: DialectBigQuery,
				Schema:  schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			got, err := tr.TranspileCondition(tt.logic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			if got != tt.wantSQL {
				t.Fatalf("TranspileCondition() = %q, want %q", got, tt.wantSQL)
			}

			gotParam, params, err := tr.TranspileParameterizedCondition(tt.logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotParam != tt.wantParam {
				t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, tt.wantParam)
			}
			assertParams(t, params, tt.wantParamValues)
		})
	}
}

func TestInLiteralArrayMembership_EnumValidationKeepsStrictTypes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"active"}},
	})
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  schema,
	})
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() error = %v", err)
	}

	got, err := tr.TranspileCondition(`{"in":[{"var":"status"},[123]]}`)
	if err != nil {
		t.Fatalf("numeric mismatched enum literals should fold false, got error: %v", err)
	}
	if got != "FALSE" {
		t.Fatalf("TranspileCondition() = %q, want FALSE", got)
	}

	if _, err := tr.TranspileCondition(`{"in":[{"var":"status"},["archived"]]}`); err == nil {
		t.Fatal("invalid string enum literal should still be rejected")
	}
}
