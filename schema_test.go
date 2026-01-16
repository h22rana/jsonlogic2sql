package jsonlogic2sql

import (
	"testing"
)

func TestSchemaValidation(t *testing.T) {
	// Create a schema with some fields
	schema := NewSchema([]FieldSchema{
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

func TestSchemaWithTranspiler(t *testing.T) {
	// Create schema
	schema := NewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeInteger},
		{Name: "status", Type: FieldTypeString},
	})

	// Create transpiler with schema
	transpiler, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() returned error: %v", err)
	}
	transpiler.SetSchema(schema)

	// Test valid field
	result, err := transpiler.Transpile(`{"==": [{"var": "amount"}, 100]}`)
	if err != nil {
		t.Fatalf("Transpile with valid field failed: %v", err)
	}
	expected := "WHERE amount = 100"
	if result != expected {
		t.Errorf("Transpile() = %q, want %q", result, expected)
	}

	// Test invalid field (should fail with schema validation)
	_, err = transpiler.Transpile(`{"==": [{"var": "invalid_field"}, 100]}`)
	if err == nil {
		t.Error("Transpile with invalid field should fail with schema validation")
	}
}

func TestSchemaInOperator(t *testing.T) {
	// Create schema with array and string fields
	schema := NewSchema([]FieldSchema{
		{Name: "tags", Type: FieldTypeArray},
		{Name: "description", Type: FieldTypeString},
	})

	transpiler, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() returned error: %v", err)
	}
	transpiler.SetSchema(schema)

	// Test in operator with array field (right side is variable)
	result, err := transpiler.Transpile(`{"in": ["tag1", {"var": "tags"}]}`)
	if err != nil {
		t.Fatalf("Transpile with array field failed: %v", err)
	}
	// Should use array membership syntax: 'tag1' IN tags
	expected := "WHERE 'tag1' IN tags"
	if result != expected {
		t.Errorf("Transpile() = %q, want %q", result, expected)
	}

	// Test in operator with string field (right side is variable)
	result, err = transpiler.Transpile(`{"in": ["hello", {"var": "description"}]}`)
	if err != nil {
		t.Fatalf("Transpile with string field failed: %v", err)
	}
	// Should use string containment syntax: STRPOS(description, 'hello') > 0
	expected = "WHERE STRPOS(description, 'hello') > 0"
	if result != expected {
		t.Errorf("Transpile() = %q, want %q", result, expected)
	}

	// Test in operator with array field (left side is variable, right side is array)
	result, err = transpiler.Transpile(`{"in": [{"var": "tags"}, ["tag1", "tag2"]]}`)
	if err != nil {
		t.Fatalf("Transpile with array field (left var) failed: %v", err)
	}
	// Should use array membership syntax: tags IN ('tag1', 'tag2')
	expected = "WHERE tags IN ('tag1', 'tag2')"
	if result != expected {
		t.Errorf("Transpile() = %q, want %q", result, expected)
	}
}

func TestSchemaOptional(t *testing.T) {
	// Test that transpiler works without schema (backward compatibility)
	transpiler, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() returned error: %v", err)
	}

	result, err := transpiler.Transpile(`{"==": [{"var": "any_field"}, 100]}`)
	if err != nil {
		t.Fatalf("Transpile without schema failed: %v", err)
	}
	expected := "WHERE any_field = 100"
	if result != expected {
		t.Errorf("Transpile() = %q, want %q", result, expected)
	}
}
