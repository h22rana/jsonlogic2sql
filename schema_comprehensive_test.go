package jsonlogic2sql

import (
	"fmt"
	"strings"
	"testing"
)

// setupTestTranspiler creates a transpiler with custom operators registered
// similar to how the REPL does it
func setupTestTranspiler() *Transpiler {
	transpiler := NewTranspiler()

	// startsWith operator: column LIKE 'value%'
	transpiler.RegisterOperatorFunc("startsWith", func(op string, args []interface{}) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("startsWith requires exactly 2 arguments")
		}
		column := args[0].(string)
		pattern := args[1].(string)
		// Extract value from quoted string (e.g., "'T'" -> "T")
		if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
			pattern = pattern[1 : len(pattern)-1]
		}
		return fmt.Sprintf("%s LIKE '%s%%'", column, pattern), nil
	})

	// endsWith operator: column LIKE '%value'
	transpiler.RegisterOperatorFunc("endsWith", func(op string, args []interface{}) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("endsWith requires exactly 2 arguments")
		}
		column := args[0].(string)
		pattern := args[1].(string)
		// Extract value from quoted string
		if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
			pattern = pattern[1 : len(pattern)-1]
		}
		return fmt.Sprintf("%s LIKE '%%%s'", column, pattern), nil
	})

	// contains operator: column LIKE '%value%'
	transpiler.RegisterOperatorFunc("contains", func(op string, args []interface{}) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("contains requires exactly 2 arguments")
		}

		var column, pattern string
		arg0Str, arg0IsStr := args[0].(string)
		arg1Str, arg1IsStr := args[1].(string)

		// Helper function to extract value from array string representation like "[T]"
		extractFromArrayString := func(s string) string {
			if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
				inner := s[1 : len(s)-1]
				if len(inner) >= 2 && inner[0] == '\'' && inner[len(inner)-1] == '\'' {
					return inner[1 : len(inner)-1]
				}
				return inner
			}
			return s
		}

		if arg0IsStr && arg1IsStr {
			if strings.HasPrefix(arg1Str, "[") && strings.HasSuffix(arg1Str, "]") {
				column = arg0Str
				pattern = extractFromArrayString(arg1Str)
			} else if strings.HasPrefix(arg0Str, "[") && strings.HasSuffix(arg0Str, "]") {
				column = arg1Str
				pattern = extractFromArrayString(arg0Str)
			} else {
				arg0Quoted := len(arg0Str) >= 2 && arg0Str[0] == '\'' && arg0Str[len(arg0Str)-1] == '\''
				arg1Quoted := len(arg1Str) >= 2 && arg1Str[0] == '\'' && arg1Str[len(arg1Str)-1] == '\''

				if arg0Quoted && !arg1Quoted {
					column = arg1Str
					pattern = arg0Str
				} else {
					column = arg0Str
					pattern = arg1Str
				}
			}
		} else {
			column = args[0].(string)
			pattern = args[1].(string)
			pattern = extractFromArrayString(pattern)
		}

		if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
			pattern = pattern[1 : len(pattern)-1]
		}
		return fmt.Sprintf("%s LIKE '%%%s%%'", column, pattern), nil
	})

	return transpiler
}

// TestSchemaValidationComprehensive tests schema validation with various edge cases
func TestSchemaValidationComprehensive(t *testing.T) {
	// Load schema from JSON with generic field names
	schemaJSON := `[
		{"name": "order.history.daily.total", "type": "integer"},
		{"name": "order.history.daily.count", "type": "integer"},
		{"name": "request.params.category_code", "type": "string"},
		{"name": "request.params.is_verified", "type": "boolean"},
		{"name": "request.params.input_mode", "type": "string"},
		{"name": "request.params.amount", "type": "integer"},
		{"name": "user.tags", "type": "array"},
		{"name": "user.description", "type": "string"}
	]`

	schema, err := NewSchemaFromJSON([]byte(schemaJSON))
	if err != nil {
		t.Fatalf("Failed to create schema from JSON: %v", err)
	}

	tests := []struct {
		name        string
		jsonLogic   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid field reference",
			jsonLogic:   `{"==": [{"var": "request.params.category_code"}, "1"]}`,
			expectError: false,
		},
		{
			name:        "valid nested field reference",
			jsonLogic:   `{">=":[{"var":"order.history.daily.total"},"50000"]}`,
			expectError: false,
		},
		{
			name:        "invalid field reference",
			jsonLogic:   `{"==": [{"var": "nonexistent.field"}, "value"]}`,
			expectError: true,
			errorMsg:    "not defined in schema",
		},
		{
			name:        "invalid field in complex expression",
			jsonLogic:   `{"and":[{"==":[{"var":"request.params.category_code"},"1"]},{"==":[{"var":"invalid.field"},"test"]}]}`,
			expectError: true,
			errorMsg:    "not defined in schema",
		},
		{
			name:        "valid boolean field",
			jsonLogic:   `{"==": [{"var": "request.params.is_verified"}, false]}`,
			expectError: false,
		},
		{
			name:        "valid integer comparison with string value",
			jsonLogic:   `{">=":[{"var":"order.history.daily.total"},"50000"]}`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transpiler := NewTranspiler()
			transpiler.SetSchema(schema)

			result, err := transpiler.Transpile(tt.jsonLogic)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got no error. Result: %s", tt.errorMsg, result)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestSchemaTypeAwareBehavior tests the type-aware "in" operator behavior
func TestSchemaTypeAwareBehavior(t *testing.T) {
	schema := NewSchema([]FieldSchema{
		{Name: "tags", Type: FieldTypeArray},
		{Name: "description", Type: FieldTypeString},
		{Name: "status", Type: FieldTypeString},
	})

	transpiler := NewTranspiler()
	transpiler.SetSchema(schema)

	tests := []struct {
		name      string
		jsonLogic string
		expected  string
	}{
		{
			name:      "in with array type field",
			jsonLogic: `{"in": ["tag1", {"var": "tags"}]}`,
			expected:  "WHERE 'tag1' IN tags",
		},
		{
			name:      "in with string type field (string containment)",
			jsonLogic: `{"in": ["hello", {"var": "description"}]}`,
			expected:  "WHERE STRPOS(description, 'hello') > 0",
		},
		{
			name:      "in with literal array",
			jsonLogic: `{"in": [{"var": "status"}, ["active", "pending"]]}`,
			expected:  "WHERE status IN ('active', 'pending')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpiler.Transpile(tt.jsonLogic)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected: %s\nGot: %s", tt.expected, result)
			}
		})
	}
}

// TestCustomOperatorsStartsWithEndsWithContains tests the custom operators
func TestCustomOperatorsStartsWithEndsWithContains(t *testing.T) {
	transpiler := setupTestTranspiler()

	tests := []struct {
		name      string
		jsonLogic string
		expected  string
	}{
		{
			name:      "simple startsWith",
			jsonLogic: `{"startsWith": [{"var": "request.params.input_mode"}, "T"]}`,
			expected:  "WHERE request.params.input_mode LIKE 'T%'",
		},
		{
			name:      "simple endsWith",
			jsonLogic: `{"endsWith": [{"var": "request.params.input_mode"}, "T"]}`,
			expected:  "WHERE request.params.input_mode LIKE '%T'",
		},
		{
			name:      "simple contains",
			jsonLogic: `{"contains": [{"var": "request.params.input_mode"}, "T"]}`,
			expected:  "WHERE request.params.input_mode LIKE '%T%'",
		},
		{
			name:      "contains with array notation",
			jsonLogic: `{"contains": [{"var": "request.params.input_mode"}, ["T"]]}`,
			expected:  "WHERE request.params.input_mode LIKE '%T%'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpiler.Transpile(tt.jsonLogic)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected: %s\nGot: %s", tt.expected, result)
			}
		})
	}
}

// TestNegationOfCustomOperators tests the negation of custom operators with !
func TestNegationOfCustomOperators(t *testing.T) {
	transpiler := setupTestTranspiler()

	tests := []struct {
		name        string
		jsonLogic   string
		shouldMatch string // partial match expected in the output
	}{
		{
			name:        "negated startsWith",
			jsonLogic:   `{"!": {"startsWith": [{"var": "request.params.input_mode"}, "T"]}}`,
			shouldMatch: "NOT",
		},
		{
			name:        "negated endsWith",
			jsonLogic:   `{"!": {"endsWith": [{"var": "request.params.input_mode"}, "T"]}}`,
			shouldMatch: "NOT",
		},
		{
			name:        "negated contains",
			jsonLogic:   `{"!": {"contains": [{"var": "request.params.input_mode"}, "T"]}}`,
			shouldMatch: "NOT",
		},
		{
			name:        "negated in with array",
			jsonLogic:   `{"!": {"in": ["T", {"var": "request.params.input_mode"}]}}`,
			shouldMatch: "NOT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpiler.Transpile(tt.jsonLogic)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if !strings.Contains(result, tt.shouldMatch) {
				t.Errorf("Expected output to contain '%s', got: %s", tt.shouldMatch, result)
			}
		})
	}
}

// TestComplexNestedExpressions tests the complex nested expressions
func TestComplexNestedExpressions(t *testing.T) {
	transpiler := setupTestTranspiler()

	tests := []struct {
		name        string
		jsonLogic   string
		description string
	}{
		{
			name:        "does not contain",
			jsonLogic:   `{"and":[{"==":[{"var":"request.params.category_code"},"1"]},{">=":[{"var":"order.history.daily.total"},"50000"]},{"or":[{"==":[{"var":"request.params.is_verified"},false]},{"and":[{"!":{"in":["T",{"var":"request.params.input_mode"}]}}]}]}]}`,
			description: "Complex AND with nested OR and negated IN",
		},
		{
			name:        "contains",
			jsonLogic:   `{"and":[{"==":[{"var":"request.params.category_code"},"1"]},{">=":[{"var":"order.history.daily.total"},"50000"]},{"or":[{"==":[{"var":"request.params.is_verified"},false]},{"and":[{"in":[{"var":"request.params.input_mode"},["T"]]}]}]}]}`,
			description: "Complex AND with IN array",
		},
		{
			name:        "does not end with",
			jsonLogic:   `{"and":[{"==":[{"var":"request.params.category_code"},"1"]},{">=":[{"var":"order.history.daily.total"},"50000"]},{"or":[{"==":[{"var":"request.params.is_verified"},false]},{"and":[{"!":{"endsWith":[{"var":"request.params.input_mode"},"T"]}}]}]}]}`,
			description: "Complex AND with negated endsWith",
		},
		{
			name:        "ends with",
			jsonLogic:   `{"and":[{"==":[{"var":"request.params.category_code"},"1"]},{">=":[{"var":"order.history.daily.total"},"50000"]},{"or":[{"==":[{"var":"request.params.is_verified"},false]},{"and":[{"endsWith":[{"var":"request.params.input_mode"},"T"]}]}]}]}`,
			description: "Complex AND with endsWith",
		},
		{
			name:        "does not begin with",
			jsonLogic:   `{"and":[{"==":[{"var":"request.params.category_code"},"1"]},{">=":[{"var":"order.history.daily.total"},"50000"]},{"or":[{"==":[{"var":"request.params.is_verified"},false]},{"and":[{"!":{"startsWith":[{"var":"request.params.input_mode"},"T"]}}]}]}]}`,
			description: "Complex AND with negated startsWith",
		},
		{
			name:        "begins with",
			jsonLogic:   `{"and":[{"==":[{"var":"request.params.category_code"},"1"]},{">=":[{"var":"order.history.daily.total"},"50000"]},{"or":[{"==":[{"var":"request.params.is_verified"},false]},{"and":[{"startsWith":[{"var":"request.params.input_mode"},"T"]}]}]}]}`,
			description: "Complex AND with startsWith",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpiler.Transpile(tt.jsonLogic)
			if err != nil {
				t.Fatalf("Test '%s' (%s) failed with error: %v", tt.name, tt.description, err)
			}
			// At minimum, should produce valid SQL
			if !strings.HasPrefix(result, "WHERE ") {
				t.Errorf("Expected result to start with 'WHERE ', got: %s", result)
			}
			t.Logf("Test '%s': %s", tt.name, result)
		})
	}
}

// TestSchemaWithCustomOperators tests schema validation with custom operators
func TestSchemaWithCustomOperators(t *testing.T) {
	schemaJSON := `[
		{"name": "request.params.input_mode", "type": "string"},
		{"name": "request.params.category_code", "type": "string"},
		{"name": "order.history.daily.total", "type": "integer"},
		{"name": "request.params.is_verified", "type": "boolean"}
	]`

	schema, err := NewSchemaFromJSON([]byte(schemaJSON))
	if err != nil {
		t.Fatalf("Failed to create schema: %v", err)
	}

	transpiler := setupTestTranspiler()
	transpiler.SetSchema(schema)

	tests := []struct {
		name        string
		jsonLogic   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid startsWith with schema",
			jsonLogic:   `{"startsWith": [{"var": "request.params.input_mode"}, "T"]}`,
			expectError: false,
		},
		{
			name:        "invalid field with startsWith",
			jsonLogic:   `{"startsWith": [{"var": "invalid.field"}, "T"]}`,
			expectError: true,
			errorMsg:    "not defined in schema",
		},
		{
			name:        "valid complex expression with schema",
			jsonLogic:   `{"and":[{"==":[{"var":"request.params.category_code"},"1"]},{"startsWith":[{"var":"request.params.input_mode"},"T"]}]}`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpiler.Transpile(tt.jsonLogic)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got no error. Result: %s", tt.errorMsg, result)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestSchemaBackwardCompatibility tests that the transpiler works without schema
func TestSchemaBackwardCompatibility(t *testing.T) {
	transpiler := setupTestTranspiler()
	// No schema set - should accept any field

	tests := []struct {
		name      string
		jsonLogic string
		expected  string
	}{
		{
			name:      "any field without schema",
			jsonLogic: `{"==": [{"var": "any.random.field"}, "value"]}`,
			expected:  "WHERE any.random.field = 'value'",
		},
		{
			name:      "nested fields without schema",
			jsonLogic: `{"and":[{"==":[{"var":"field1"},"a"]},{"==":[{"var":"field2"},"b"]}]}`,
			expected:  "WHERE (field1 = 'a' AND field2 = 'b')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpiler.Transpile(tt.jsonLogic)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected: %s\nGot: %s", tt.expected, result)
			}
		})
	}
}

// TestSchemaFromFileExample tests loading schema from file (requires a test schema file)
func TestSchemaFromFileExample(t *testing.T) {
	// Create a test schema JSON for this test
	testSchemaJSON := `[
		{"name": "order.total", "type": "integer"},
		{"name": "user.name", "type": "string"},
		{"name": "user.active", "type": "boolean"}
	]`

	schema, err := NewSchemaFromJSON([]byte(testSchemaJSON))
	if err != nil {
		t.Fatalf("Failed to create test schema: %v", err)
	}

	// Test some fields
	tests := []struct {
		fieldName   string
		shouldExist bool
		fieldType   FieldType
	}{
		{"order.total", true, FieldTypeInteger},
		{"user.name", true, FieldTypeString},
		{"user.active", true, FieldTypeBoolean},
		{"nonexistent.field", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.fieldName, func(t *testing.T) {
			if schema.HasField(tt.fieldName) != tt.shouldExist {
				t.Errorf("HasField(%q) = %v, want %v", tt.fieldName, schema.HasField(tt.fieldName), tt.shouldExist)
			}
			if tt.shouldExist && schema.GetFieldTypeFieldType(tt.fieldName) != tt.fieldType {
				t.Errorf("GetFieldType(%q) = %v, want %v", tt.fieldName, schema.GetFieldTypeFieldType(tt.fieldName), tt.fieldType)
			}
		})
	}
}

// TestInOperatorWithSchemaIntegration tests the IN operator behavior with schema-based type detection
func TestInOperatorWithSchemaIntegration(t *testing.T) {
	schema := NewSchema([]FieldSchema{
		{Name: "user.roles", Type: FieldTypeArray},
		{Name: "user.bio", Type: FieldTypeString},
		{Name: "status", Type: FieldTypeString},
	})

	transpiler := NewTranspiler()
	transpiler.SetSchema(schema)

	tests := []struct {
		name      string
		jsonLogic string
		expected  string
	}{
		{
			name:      "in with array field uses IN syntax",
			jsonLogic: `{"in": ["admin", {"var": "user.roles"}]}`,
			expected:  "WHERE 'admin' IN user.roles",
		},
		{
			name:      "in with string field uses STRPOS",
			jsonLogic: `{"in": ["developer", {"var": "user.bio"}]}`,
			expected:  "WHERE STRPOS(user.bio, 'developer') > 0",
		},
		{
			name:      "in with literal array on right side",
			jsonLogic: `{"in": [{"var": "status"}, ["active", "pending", "approved"]]}`,
			expected:  "WHERE status IN ('active', 'pending', 'approved')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpiler.Transpile(tt.jsonLogic)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected: %s\nGot: %s", tt.expected, result)
			}
		})
	}
}

// TestEdgeCasesWithSchema tests various edge cases
func TestEdgeCasesWithSchema(t *testing.T) {
	schema := NewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeInteger},
		{Name: "name", Type: FieldTypeString},
		{Name: "active", Type: FieldTypeBoolean},
	})

	transpiler := NewTranspiler()
	transpiler.SetSchema(schema)

	tests := []struct {
		name        string
		jsonLogic   string
		expectError bool
	}{
		{
			name:        "comparison with null",
			jsonLogic:   `{"==": [{"var": "name"}, null]}`,
			expectError: false,
		},
		{
			name:        "boolean comparison",
			jsonLogic:   `{"==": [{"var": "active"}, true]}`,
			expectError: false,
		},
		{
			name:        "integer comparison with numeric string",
			jsonLogic:   `{">": [{"var": "amount"}, "100"]}`,
			expectError: false,
		},
		{
			name:        "multiple operators combined",
			jsonLogic:   `{"and": [{"==": [{"var": "active"}, true]}, {">": [{"var": "amount"}, 0]}]}`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpiler.Transpile(tt.jsonLogic)
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got result: %s", result)
			} else if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}
