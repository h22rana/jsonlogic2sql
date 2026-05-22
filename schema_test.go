package jsonlogic2sql

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaValidation(t *testing.T) {
	// Create a schema with some fields
	schema := mustNewSchema([]FieldSchema{
		{Name: "order.items.count", Type: FieldTypeInteger},
		{Name: "order.total.amount", Type: FieldTypeInteger},
		{Name: "user.name", Type: FieldTypeString},
		{Name: "user.tags", Type: FieldTypeArray},
	})

	// Test field validation
	tests := []struct {
		name        string
		fieldName   string
		shouldExist bool
		fieldType   FieldType
	}{
		{"existing integer field", "order.items.count", true, FieldTypeInteger},
		{"existing string field", "user.name", true, FieldTypeString},
		{"existing array field", "user.tags", true, FieldTypeArray},
		{"non-existent field", "nonexistent.field", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if schema.HasField(tt.fieldName) != tt.shouldExist {
				t.Errorf("HasField(%q) = %v, want %v", tt.fieldName, schema.HasField(tt.fieldName), tt.shouldExist)
			}

			if tt.shouldExist {
				if err := schema.ValidateField(tt.fieldName); err != nil {
					t.Errorf("ValidateField(%q) returned error: %v", tt.fieldName, err)
				}
				if schema.GetFieldTypeFieldType(tt.fieldName) != tt.fieldType {
					t.Errorf("GetFieldType(%q) = %v, want %v", tt.fieldName, schema.GetFieldTypeFieldType(tt.fieldName), tt.fieldType)
				}
			} else {
				if err := schema.ValidateField(tt.fieldName); err == nil {
					t.Errorf("ValidateField(%q) should return error for non-existent field", tt.fieldName)
				}
			}
		})
	}
}

func TestSchemaFromJSON(t *testing.T) {
	jsonData := `[
		{"name": "field1", "type": "string"},
		{"name": "field2", "type": "integer"}
	]`

	schema, err := NewSchemaFromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("NewSchemaFromJSON failed: %v", err)
	}

	if !schema.HasField("field1") {
		t.Error("field1 should exist in schema")
	}
	if !schema.HasField("field2") {
		t.Error("field2 should exist in schema")
	}
	if schema.IsStringType("field1") != true {
		t.Error("field1 should be string type")
	}
	if schema.IsNumericType("field2") != true {
		t.Error("field2 should be numeric type")
	}
}

func TestNestedSchemaObjectAndArrayEnumValidation(t *testing.T) {
	schemaJSON := `[
		{
			"name": "profile",
			"type": "object",
			"fields": [
				{"name": "country", "type": "string"},
				{"name": "status", "type": "enum", "allowedValues": ["active", "blocked"]}
			]
		},
		{
			"name": "payments",
			"type": "array",
			"elementFields": [
				{"name": "type", "type": "enum", "allowedValues": ["BALANCE", "CARD"]},
				{"name": "amount", "type": "number"},
				{
					"name": "details",
					"type": "object",
					"fields": [
						{"name": "issuer", "type": "string"},
						{
							"name": "events",
							"type": "array",
							"elementFields": [
								{"name": "code", "type": "enum", "allowedValues": ["AUTH", "CAPTURE"]}
							]
						}
					]
				}
			]
		}
	]`

	schema, err := NewSchemaFromJSON([]byte(schemaJSON))
	if err != nil {
		t.Fatalf("NewSchemaFromJSON() error = %v", err)
	}

	for _, fieldName := range []string{
		"profile",
		"profile.country",
		"profile.status",
		"payments",
		"payments.type",
		"payments.amount",
		"payments.details",
		"payments.details.issuer",
		"payments.details.events",
		"payments.details.events.code",
	} {
		if !schema.HasField(fieldName) {
			t.Fatalf("schema should expose flattened field %q", fieldName)
		}
	}
	for _, fieldName := range []string{"profile.country", "profile.status", "payments"} {
		if err := schema.ValidateField(fieldName); err != nil {
			t.Fatalf("ValidateField(%q) error = %v", fieldName, err)
		}
	}
	for _, fieldName := range []string{"payments.type", "payments.amount", "payments.details.issuer"} {
		if err := schema.ValidateField(fieldName); err == nil {
			t.Fatalf("ValidateField(%q) should reject array element fields outside array scope", fieldName)
		}
	}
	if !schema.IsEnumType("profile.status") || !schema.IsEnumType("payments.type") {
		t.Fatal("nested enum fields should keep enum metadata")
	}
	if err := schema.ValidateEnumValue("profile.status", "archived"); err == nil {
		t.Fatal("profile.status should reject an enum value outside allowedValues")
	}
	if got, err := schema.ResolveScopedField("payments", "details.issuer"); err != nil || got != "payments.details.issuer" {
		t.Fatalf("ResolveScopedField(payments, details.issuer) = %q, %v; want payments.details.issuer, nil", got, err)
	}
	if got, err := schema.ResolveScopedField("payments", "details.events"); err != nil || got != "payments.details.events" {
		t.Fatalf("ResolveScopedField(payments, details.events) = %q, %v; want payments.details.events, nil", got, err)
	}
	if _, err := schema.ResolveScopedField("payments", "details.events.code"); err == nil {
		t.Fatal("ResolveScopedField() should reject nested array element fields from the parent array scope")
	}
	if got, err := schema.ResolveScopedField("payments.details.events", "code"); err != nil || got != "payments.details.events.code" {
		t.Fatalf("ResolveScopedField(payments.details.events, code) = %q, %v; want payments.details.events.code, nil", got, err)
	}
	if _, err := schema.ResolveScopedField("payments", "unknown"); err == nil {
		t.Fatal("ResolveScopedField() should reject unknown array element fields")
	}

	validCases := []struct {
		name             string
		logic            string
		wantInlineSQL    string
		wantParamSQL     string
		wantParamLiteral string
	}{
		{
			name:             "object enum field",
			logic:            `{"==":[{"var":"profile.status"},"active"]}`,
			wantInlineSQL:    "profile.status = 'active'",
			wantParamSQL:     "profile.status = ",
			wantParamLiteral: "active",
		},
		{
			name:             "array element enum field",
			logic:            `{"some":[{"var":"payments"},{"==":[{"var":"type"},"BALANCE"]}]}`,
			wantInlineSQL:    "elem.type = 'BALANCE'",
			wantParamSQL:     "elem.type = ",
			wantParamLiteral: "BALANCE",
		},
		{
			name:             "object field inside array element",
			logic:            `{"some":[{"var":"payments"},{"==":[{"var":"details.issuer"},"visa"]}]}`,
			wantInlineSQL:    "elem.details.issuer = 'visa'",
			wantParamSQL:     "elem.details.issuer = ",
			wantParamLiteral: "visa",
		},
	}

	invalidCases := []struct {
		name      string
		logic     string
		wantError string
	}{
		{
			name:      "object enum rejects invalid value",
			logic:     `{"==":[{"var":"profile.status"},"archived"]}`,
			wantError: "invalid enum value 'archived' for field 'profile.status'",
		},
		{
			name:      "array element enum rejects invalid value",
			logic:     `{"some":[{"var":"payments"},{"==":[{"var":"type"},"CASH"]}]}`,
			wantError: "invalid enum value 'CASH' for field 'payments.type'",
		},
		{
			name:      "array element object rejects unknown field",
			logic:     `{"some":[{"var":"payments"},{"==":[{"var":"details.unknown"},"visa"]}]}`,
			wantError: "field 'details.unknown' is not defined in schema scope 'payments'",
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspilerWithConfig(&TranspilerConfig{
				Dialect: d,
				Schema:  schema,
			})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			for _, tc := range validCases {
				t.Run(tc.name, func(t *testing.T) {
					sql, err := tr.TranspileCondition(tc.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if !strings.Contains(sql, tc.wantInlineSQL) {
						t.Fatalf("TranspileCondition() = %q, want to contain %q", sql, tc.wantInlineSQL)
					}

					paramSQL, params, err := tr.TranspileParameterizedCondition(tc.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if !strings.Contains(paramSQL, tc.wantParamSQL) {
						t.Fatalf("TranspileParameterizedCondition() = %q, want to contain %q", paramSQL, tc.wantParamSQL)
					}
					if len(params) != 1 || params[0].Value != tc.wantParamLiteral {
						t.Fatalf("params = %#v, want one string param %q", params, tc.wantParamLiteral)
					}
				})
			}

			for _, tc := range invalidCases {
				t.Run(tc.name, func(t *testing.T) {
					if _, err := tr.TranspileCondition(tc.logic); err == nil || !strings.Contains(err.Error(), tc.wantError) {
						t.Fatalf("TranspileCondition() error = %v, want containing %q", err, tc.wantError)
					}
					paramSQL, params, err := tr.TranspileParameterizedCondition(tc.logic)
					if err == nil || !strings.Contains(err.Error(), tc.wantError) {
						t.Fatalf("TranspileParameterizedCondition() error = %v, want containing %q (SQL %q params %#v)",
							err, tc.wantError, paramSQL, params)
					}
				})
			}
		})
	}
}

func TestNestedSchemaShapeValidation(t *testing.T) {
	tests := []struct {
		name      string
		fields    []FieldSchema
		wantError string
	}{
		{
			name: "root name is required",
			fields: []FieldSchema{
				{Type: FieldTypeString},
			},
			wantError: "schema field requires non-empty name",
		},
		{
			name: "nested object child name is required",
			fields: []FieldSchema{
				{
					Name: "profile",
					Type: FieldTypeObject,
					Fields: []FieldSchema{
						{Type: FieldTypeString},
					},
				},
			},
			wantError: `schema field under "profile" requires non-empty name`,
		},
		{
			name: "array element child name is required",
			fields: []FieldSchema{
				{
					Name: "payments",
					Type: FieldTypeArray,
					ElementFields: []FieldSchema{
						{Type: FieldTypeString},
					},
				},
			},
			wantError: `schema field under "payments" requires non-empty name`,
		},
		{
			name: "type is required",
			fields: []FieldSchema{
				{Name: "profile.status"},
			},
			wantError: `schema field "profile.status" requires non-empty type`,
		},
		{
			name: "nested type is required",
			fields: []FieldSchema{
				{
					Name: "profile",
					Type: FieldTypeObject,
					Fields: []FieldSchema{
						{Name: "status"},
					},
				},
			},
			wantError: `schema field "profile.status" requires non-empty type`,
		},
		{
			name: "unsupported type is rejected",
			fields: []FieldSchema{
				{Name: "profile.status", Type: FieldType("varchar")},
			},
			wantError: `schema field "profile.status" has unsupported type "varchar"`,
		},
		{
			name: "root path segment cannot be empty",
			fields: []FieldSchema{
				{Name: "profile..status", Type: FieldTypeString},
			},
			wantError: `schema field "profile..status" contains an empty path segment`,
		},
		{
			name: "nested path segment cannot be empty",
			fields: []FieldSchema{
				{
					Name: "profile",
					Type: FieldTypeObject,
					Fields: []FieldSchema{
						{Name: ".status", Type: FieldTypeString},
					},
				},
			},
			wantError: `schema field "profile..status" contains an empty path segment`,
		},
		{
			name: "duplicate root field is rejected",
			fields: []FieldSchema{
				{Name: "profile.status", Type: FieldTypeString},
				{Name: "profile.status", Type: FieldTypeString},
			},
			wantError: `schema field "profile.status" is defined more than once`,
		},
		{
			name: "duplicate flattened nested field is rejected",
			fields: []FieldSchema{
				{Name: "profile.status", Type: FieldTypeString},
				{
					Name: "profile",
					Type: FieldTypeObject,
					Fields: []FieldSchema{
						{Name: "status", Type: FieldTypeString},
					},
				},
			},
			wantError: `schema field "profile.status" is defined more than once`,
		},
		{
			name: "enum allowedValues are required",
			fields: []FieldSchema{
				{Name: "profile.status", Type: FieldTypeEnum},
			},
			wantError: `schema enum field "profile.status" requires at least one allowedValues entry`,
		},
		{
			name: "nested enum allowedValues are required",
			fields: []FieldSchema{
				{
					Name: "profile",
					Type: FieldTypeObject,
					Fields: []FieldSchema{
						{Name: "status", Type: FieldTypeEnum},
					},
				},
			},
			wantError: `schema enum field "profile.status" requires at least one allowedValues entry`,
		},
		{
			name: "array element enum allowedValues are required",
			fields: []FieldSchema{
				{
					Name: "payments",
					Type: FieldTypeArray,
					ElementFields: []FieldSchema{
						{Name: "type", Type: FieldTypeEnum},
					},
				},
			},
			wantError: `schema enum field "payments.type" requires at least one allowedValues entry`,
		},
		{
			name: "duplicate enum allowedValues are rejected",
			fields: []FieldSchema{
				{Name: "profile.status", Type: FieldTypeEnum, AllowedValues: []string{"active", "active"}},
			},
			wantError: `schema enum field "profile.status" has duplicate allowed value "active"`,
		},
		{
			name: "nested duplicate enum allowedValues are rejected",
			fields: []FieldSchema{
				{
					Name: "payments",
					Type: FieldTypeArray,
					ElementFields: []FieldSchema{
						{Name: "type", Type: FieldTypeEnum, AllowedValues: []string{"CARD", "CARD"}},
					},
				},
			},
			wantError: `schema enum field "payments.type" has duplicate allowed value "CARD"`,
		},
		{
			name: "primitive allowedValues are rejected",
			fields: []FieldSchema{
				{Name: "profile.status", Type: FieldTypeString, AllowedValues: []string{"active"}},
			},
			wantError: `schema field "profile.status" uses allowedValues but has type "string"; allowedValues require enum type`,
		},
		{
			name: "array allowedValues are rejected",
			fields: []FieldSchema{
				{Name: "tags", Type: FieldTypeArray, AllowedValues: []string{"risk"}},
			},
			wantError: `schema field "tags" uses allowedValues but has type "array"; allowedValues require enum type`,
		},
		{
			name: "fields require object type",
			fields: []FieldSchema{
				{
					Name: "payments",
					Type: FieldTypeArray,
					Fields: []FieldSchema{
						{Name: "type", Type: FieldTypeString},
					},
				},
			},
			wantError: `schema field "payments" uses fields but has type "array"; fields require object type`,
		},
		{
			name: "elementFields require array type",
			fields: []FieldSchema{
				{
					Name: "profile",
					Type: FieldTypeObject,
					ElementFields: []FieldSchema{
						{Name: "status", Type: FieldTypeString},
					},
				},
			},
			wantError: `schema field "profile" uses elementFields but has type "object"; elementFields require array type`,
		},
		{
			name: "nested child shape is validated",
			fields: []FieldSchema{
				{
					Name: "profile",
					Type: FieldTypeObject,
					Fields: []FieldSchema{
						{
							Name: "tags",
							Type: FieldTypeString,
							ElementFields: []FieldSchema{
								{Name: "code", Type: FieldTypeString},
							},
						},
					},
				},
			},
			wantError: `schema field "profile.tags" uses elementFields but has type "string"; elementFields require array type`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSchemaFields(tt.fields); err == nil || err.Error() != tt.wantError {
				t.Fatalf("ValidateSchemaFields() error = %v, want %q", err, tt.wantError)
			}
			if _, err := NewSchema(tt.fields); err == nil || err.Error() != tt.wantError {
				t.Fatalf("NewSchema() error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestSchemaObjectAndArrayChildrenAreOptional(t *testing.T) {
	schema, err := NewSchema([]FieldSchema{
		{Name: "metadata", Type: FieldTypeObject},
		{Name: "tags", Type: FieldTypeArray},
		{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{""}},
	})
	if err != nil {
		t.Fatalf("NewSchema() error = %v", err)
	}
	if !schema.HasField("metadata") || !schema.HasField("tags") || !schema.HasField("status") {
		t.Fatalf("schema fields missing after construction: %#v", schema.GetFields())
	}
	if err := schema.ValidateEnumValue("status", ""); err != nil {
		t.Fatalf("empty string should be allowed when explicitly listed in enum allowedValues: %v", err)
	}
}

func TestSchemaWithTranspiler(t *testing.T) {
	// Create schema
	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeInteger},
		{Name: "status", Type: FieldTypeString},
	})

	// Create transpiler with schema
	transpiler, err := NewTranspiler(DialectBigQuery, defaultTestSchema())
	if err != nil {
		t.Fatalf("NewTranspiler() returned error: %v", err)
	}
	transpiler.SetSchema(schema)

	// Test valid field
	result, err := transpiler.TranspileCondition(`{"==": [{"var": "amount"}, 100]}`)
	if err != nil {
		t.Fatalf("Transpile with valid field failed: %v", err)
	}
	expected := "amount = 100"
	if result != expected {
		t.Errorf("TranspileCondition() = %q, want %q", result, expected)
	}

	// Test invalid field (should fail with schema validation)
	_, err = transpiler.TranspileCondition(`{"==": [{"var": "invalid_field"}, 100]}`)
	if err == nil {
		t.Error("Transpile with invalid field should fail with schema validation")
	}
}

func TestSchemaInOperator(t *testing.T) {
	// Create schema with array and string fields
	schema := mustNewSchema([]FieldSchema{
		{Name: "tags", Type: FieldTypeArray},
		{Name: "description", Type: FieldTypeString},
	})

	transpiler, err := NewTranspiler(DialectBigQuery, defaultTestSchema())
	if err != nil {
		t.Fatalf("NewTranspiler() returned error: %v", err)
	}
	transpiler.SetSchema(schema)

	// Test in operator with array field (right side is variable)
	result, err := transpiler.TranspileCondition(`{"in": ["tag1", {"var": "tags"}]}`)
	if err != nil {
		t.Fatalf("Transpile with array field failed: %v", err)
	}
	// Should use null-safe array membership to preserve JSONLogic null equality.
	expected := testNullSafeArrayMembershipSQL(DialectBigQuery, "'tag1'", "tags")
	if result != expected {
		t.Errorf("TranspileCondition() = %q, want %q", result, expected)
	}

	// Test in operator with string field (right side is variable)
	result, err = transpiler.TranspileCondition(`{"in": ["hello", {"var": "description"}]}`)
	if err != nil {
		t.Fatalf("Transpile with string field failed: %v", err)
	}
	// Should use string containment syntax: STRPOS(description, 'hello') > 0
	expected = "STRPOS(description, 'hello') > 0"
	if result != expected {
		t.Errorf("TranspileCondition() = %q, want %q", result, expected)
	}

	// Test in operator with array field (left side is variable, right side is array)
	result, err = transpiler.TranspileCondition(`{"in": [{"var": "tags"}, ["tag1", "tag2"]]}`)
	if err != nil {
		t.Fatalf("Transpile with array field (left var) failed: %v", err)
	}
	// JSONLogic array membership uses strict element equality. An array-valued
	// left operand cannot strictly equal scalar elements in a literal RHS array.
	expected = "FALSE"
	if result != expected {
		t.Errorf("TranspileCondition() = %q, want %q", result, expected)
	}
}

func TestSchemaInOperator_ArrayValuedLeftOperandFoldsFalseAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "tags", Type: FieldTypeArray},
	})
	logic := `{"in": [{"var": "tags"}, ["tag1", "tag2"]]}`

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

			got, err := tr.TranspileCondition(logic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			if got != "FALSE" {
				t.Fatalf("TranspileCondition() = %q, want FALSE", got)
			}

			gotParam, params, err := tr.TranspileParameterizedCondition(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotParam != "FALSE" {
				t.Fatalf("TranspileParameterizedCondition() = %q, want FALSE", gotParam)
			}
			if len(params) != 0 {
				t.Fatalf("params = %#v, want none", params)
			}
		})
	}
}

func TestSchemaRequiredAllowsLiteralOnlyEmptySchema(t *testing.T) {
	transpiler, err := NewTranspiler(DialectBigQuery, emptyTestSchema())
	if err != nil {
		t.Fatalf("NewTranspiler() returned error: %v", err)
	}

	if _, fieldErr := transpiler.TranspileCondition(`{"==": [{"var": "any_field"}, 100]}`); fieldErr == nil ||
		!strings.Contains(fieldErr.Error(), "field 'any_field' is not defined in schema") {
		t.Fatalf("field access with empty schema error = %v, want schema validation error", fieldErr)
	}

	result, err := transpiler.TranspileCondition(`{"==": [1, 1]}`)
	if err != nil {
		t.Fatalf("literal-only expression failed with empty schema: %v", err)
	}
	if result != "TRUE" {
		t.Errorf("TranspileCondition() = %q, want TRUE", result)
	}
}

func TestSchemaFromFile(t *testing.T) {
	// Create a temporary file with schema JSON
	tempDir := t.TempDir()
	schemaFile := filepath.Join(tempDir, "schema.json")

	schemaJSON := `[
		{"name": "user.name", "type": "string"},
		{"name": "user.age", "type": "integer"},
		{"name": "user.active", "type": "boolean"}
	]`

	err := os.WriteFile(schemaFile, []byte(schemaJSON), 0o600)
	if err != nil {
		t.Fatalf("Failed to create temp schema file: %v", err)
	}

	schema, err := NewSchemaFromFile(schemaFile)
	if err != nil {
		t.Fatalf("NewSchemaFromFile() failed: %v", err)
	}

	// Verify fields
	if !schema.HasField("user.name") {
		t.Error("user.name should exist in schema")
	}
	if !schema.HasField("user.age") {
		t.Error("user.age should exist in schema")
	}
	if !schema.HasField("user.active") {
		t.Error("user.active should exist in schema")
	}

	// Verify types
	if !schema.IsStringType("user.name") {
		t.Error("user.name should be string type")
	}
	if !schema.IsNumericType("user.age") {
		t.Error("user.age should be numeric type")
	}
	if !schema.IsBooleanType("user.active") {
		t.Error("user.active should be boolean type")
	}
}

func TestSchemaFromFile_NotFound(t *testing.T) {
	_, err := NewSchemaFromFile("/nonexistent/path/schema.json")
	if err == nil {
		t.Error("NewSchemaFromFile() should return error for non-existent file")
	}
}

func TestSchemaGetFields(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "field3", Type: FieldTypeBoolean},
		{Name: "field1", Type: FieldTypeString},
		{Name: "field2", Type: FieldTypeInteger},
	})

	fields := schema.GetFields()

	want := []string{"field1", "field2", "field3"}
	if !reflect.DeepEqual(fields, want) {
		t.Fatalf("GetFields() = %#v, want %#v", fields, want)
	}
}

func TestSchemaAllowedValuesAreDefensiveCopies(t *testing.T) {
	sourceAllowed := []string{"active", "pending"}
	sourceFields := []FieldSchema{
		{Name: "status", Type: FieldTypeEnum, AllowedValues: sourceAllowed},
	}
	schema := mustNewSchema(sourceFields)

	sourceAllowed[0] = "mutated"
	sourceFields[0].AllowedValues[1] = "mutated-again"
	if err := schema.ValidateEnumValue("status", "active"); err != nil {
		t.Fatalf("ValidateEnumValue(active) error after source mutation = %v", err)
	}

	allowed := schema.GetAllowedValues("status")
	allowed[0] = "changed-by-caller"
	if err := schema.ValidateEnumValue("status", "active"); err != nil {
		t.Fatalf("ValidateEnumValue(active) error after returned slice mutation = %v", err)
	}
	if got, want := schema.GetAllowedValues("status"), []string{"active", "pending"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("GetAllowedValues() = %#v, want %#v", got, want)
	}
}

func TestSchemaIsBooleanType(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "is_active", Type: FieldTypeBoolean},
		{Name: "name", Type: FieldTypeString},
		{Name: "count", Type: FieldTypeInteger},
	})

	tests := []struct {
		field    string
		expected bool
	}{
		{"is_active", true},
		{"name", false},
		{"count", false},
		{"nonexistent", false},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			if schema.IsBooleanType(tt.field) != tt.expected {
				t.Errorf("IsBooleanType(%q) = %v, want %v", tt.field, schema.IsBooleanType(tt.field), tt.expected)
			}
		})
	}
}

func TestSchemaGetAllowedValues(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"active", "pending", "closed"}},
		{Name: "name", Type: FieldTypeString},
	})

	// Enum field should return allowed values
	values := schema.GetAllowedValues("status")
	if len(values) != 3 {
		t.Errorf("GetAllowedValues(status) returned %d values, want 3", len(values))
	}

	// Non-enum field should return nil
	values = schema.GetAllowedValues("name")
	if values != nil {
		t.Errorf("GetAllowedValues(name) should return nil for non-enum field")
	}

	// Non-existent field should return nil
	values = schema.GetAllowedValues("nonexistent")
	if values != nil {
		t.Errorf("GetAllowedValues(nonexistent) should return nil for non-existent field")
	}
}

func TestSchemaConstructors_RejectQuotedFieldNames(t *testing.T) {
	tests := []struct {
		name   string
		fields []FieldSchema
	}{
		{
			"backtick in field name",
			[]FieldSchema{{Name: "data.`24h`.tx", Type: FieldTypeInteger}},
		},
		{
			"double quote in field name",
			[]FieldSchema{{Name: `data."24h".tx`, Type: FieldTypeInteger}},
		},
		{
			"backtick-wrapped segment",
			[]FieldSchema{{Name: "history.`7d`.count", Type: FieldTypeNumber}},
		},
		{
			"single quote in field name",
			[]FieldSchema{{Name: "data.'24h'.tx.sum", Type: FieldTypeInteger}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSchemaFields(tt.fields); err == nil {
				t.Errorf("ValidateSchemaFields() expected error for quoted field name %q, got nil", tt.fields[0].Name)
			}
			_, err := NewSchema(tt.fields)
			if err == nil {
				t.Errorf("NewSchema() expected error for quoted field name %q, got nil", tt.fields[0].Name)
			}
		})
	}
}

func TestSchemaConstructors_RejectUnsafeIdentifierSegments(t *testing.T) {
	tests := []struct {
		name   string
		fields []FieldSchema
	}{
		{
			name:   "semicolon in root field",
			fields: []FieldSchema{{Name: "metrics.24h;DROP.count", Type: FieldTypeInteger}},
		},
		{
			name: "sql comment in object child",
			fields: []FieldSchema{
				{
					Name: "profile",
					Type: FieldTypeObject,
					Fields: []FieldSchema{
						{Name: "name--", Type: FieldTypeString},
					},
				},
			},
		},
		{
			name: "block comment token in array element",
			fields: []FieldSchema{
				{
					Name: "items",
					Type: FieldTypeArray,
					ElementFields: []FieldSchema{
						{Name: "amount/*", Type: FieldTypeNumber},
					},
				},
			},
		},
		{
			name:   "parenthesis in field",
			fields: []FieldSchema{{Name: "name)OR(TRUE", Type: FieldTypeString}},
		},
		{
			name:   "whitespace in field",
			fields: []FieldSchema{{Name: "display name", Type: FieldTypeString}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSchemaFields(tt.fields); err == nil {
				t.Fatal("ValidateSchemaFields() expected unsafe identifier error, got nil")
			}
			if _, err := NewSchema(tt.fields); err == nil {
				t.Fatal("NewSchema() expected unsafe identifier error, got nil")
			}
		})
	}
}

func TestNewSchema_AcceptsRawFieldNames(t *testing.T) {
	schema, err := NewSchema([]FieldSchema{
		{Name: "fixture.history.24h.events.total", Type: FieldTypeInteger},
		{Name: "user.name", Type: FieldTypeString},
		{Name: "metrics.\uff124h.count", Type: FieldTypeInteger},
		{Name: "tags", Type: FieldTypeArray},
	})
	if err != nil {
		t.Fatalf("NewSchema() unexpected error for raw field names: %v", err)
	}
	if !schema.HasField("fixture.history.24h.events.total") {
		t.Error("schema should have field fixture.history.24h.events.total")
	}
	if !schema.HasField("metrics.\uff124h.count") {
		t.Error("schema should have field metrics.\uff124h.count")
	}
}

func TestNewSchemaFromJSON_RejectsQuotedFieldNames(t *testing.T) {
	_, err := NewSchemaFromJSON([]byte("[{\"name\":\"fixture.`24h`.events.total\",\"type\":\"integer\"}]"))
	if err == nil {
		t.Fatal("NewSchemaFromJSON() expected schema field validation error, got nil")
	}
}
