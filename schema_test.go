package jsonlogic2sql

import (
	"os"
	"path/filepath"
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

func TestSchemaWithTranspiler(t *testing.T) {
	// Create schema
	schema := mustNewSchema([]FieldSchema{
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

	transpiler, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() returned error: %v", err)
	}
	transpiler.SetSchema(schema)

	// Test in operator with array field (right side is variable)
	result, err := transpiler.TranspileCondition(`{"in": ["tag1", {"var": "tags"}]}`)
	if err != nil {
		t.Fatalf("Transpile with array field failed: %v", err)
	}
	// Should use dialect-specific array membership syntax (BigQuery uses IN UNNEST)
	expected := "'tag1' IN UNNEST(tags)"
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
	// Should use array membership syntax: tags IN ('tag1', 'tag2')
	expected = "tags IN ('tag1', 'tag2')"
	if result != expected {
		t.Errorf("TranspileCondition() = %q, want %q", result, expected)
	}
}

func TestSchemaOptional(t *testing.T) {
	// Test that transpiler works without schema (backward compatibility)
	transpiler, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() returned error: %v", err)
	}

	result, err := transpiler.TranspileCondition(`{"==": [{"var": "any_field"}, 100]}`)
	if err != nil {
		t.Fatalf("Transpile without schema failed: %v", err)
	}
	expected := "any_field = 100"
	if result != expected {
		t.Errorf("TranspileCondition() = %q, want %q", result, expected)
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
		{Name: "field1", Type: FieldTypeString},
		{Name: "field2", Type: FieldTypeInteger},
		{Name: "field3", Type: FieldTypeBoolean},
	})

	fields := schema.GetFields()

	if len(fields) != 3 {
		t.Errorf("GetFields() returned %d fields, want 3", len(fields))
	}

	// Check that all fields are present (order may vary)
	fieldMap := make(map[string]bool)
	for _, f := range fields {
		fieldMap[f] = true
	}

	if !fieldMap["field1"] {
		t.Error("field1 should be in GetFields() result")
	}
	if !fieldMap["field2"] {
		t.Error("field2 should be in GetFields() result")
	}
	if !fieldMap["field3"] {
		t.Error("field3 should be in GetFields() result")
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
			_, err = NewValidatedSchema(tt.fields)
			if err == nil {
				t.Errorf("NewValidatedSchema() expected error for quoted field name %q, got nil", tt.fields[0].Name)
			}
		})
	}
}

func TestNewSchema_AcceptsRawFieldNames(t *testing.T) {
	schema, err := NewSchema([]FieldSchema{
		{Name: "fixture.history.24h.events.total", Type: FieldTypeInteger},
		{Name: "user.name", Type: FieldTypeString},
		{Name: "tags", Type: FieldTypeArray},
	})
	if err != nil {
		t.Fatalf("NewSchema() unexpected error for raw field names: %v", err)
	}
	if !schema.HasField("fixture.history.24h.events.total") {
		t.Error("schema should have field fixture.history.24h.events.total")
	}
}

func TestNewSchemaFromJSON_RejectsQuotedFieldNames(t *testing.T) {
	_, err := NewSchemaFromJSON([]byte("[{\"name\":\"fixture.`24h`.events.total\",\"type\":\"integer\"}]"))
	if err == nil {
		t.Fatal("NewSchemaFromJSON() expected schema field validation error, got nil")
	}
}

func TestNewValidatedSchema_AcceptsRawFieldNames(t *testing.T) {
	schema, err := NewValidatedSchema([]FieldSchema{
		{Name: "fixture.history.24h.events.total", Type: FieldTypeInteger},
		{Name: "user.name", Type: FieldTypeString},
		{Name: "tags", Type: FieldTypeArray},
	})
	if err != nil {
		t.Fatalf("NewValidatedSchema() unexpected error for raw field names: %v", err)
	}
	if !schema.HasField("fixture.history.24h.events.total") {
		t.Error("schema should have field fixture.history.24h.events.total")
	}
}
