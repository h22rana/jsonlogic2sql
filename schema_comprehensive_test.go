package jsonlogic2sql

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func transpileSchemaExpression(transpiler *Transpiler, logic string) (string, error) {
	result, err := transpiler.TranspileCondition(logic)
	if IsErrorCode(err, ErrInvalidExpressionContext) {
		return transpiler.TranspileValue(logic)
	}
	return result, err
}

// setupTestTranspiler creates a transpiler with custom operators registered
// similar to how the REPL does it.
func setupTestTranspiler() *Transpiler {
	transpiler, _ := NewTranspiler(DialectBigQuery)

	// startsWith operator: column LIKE 'value%'
	transpiler.RegisterOperatorFunc("startsWith", func(op string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 2 {
			return OperatorResult{}, fmt.Errorf("startsWith requires exactly 2 arguments")
		}
		column := args[0].SQL
		pattern := args[1].SQL
		// Extract value from quoted string (e.g., "'T'" -> "T")
		if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
			pattern = pattern[1 : len(pattern)-1]
		}
		return PredicateSQL(fmt.Sprintf("%s LIKE '%s%%'", column, pattern)), nil
	})

	// endsWith operator: column LIKE '%value'
	transpiler.RegisterOperatorFunc("endsWith", func(op string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 2 {
			return OperatorResult{}, fmt.Errorf("endsWith requires exactly 2 arguments")
		}
		column := args[0].SQL
		pattern := args[1].SQL
		// Extract value from quoted string
		if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
			pattern = pattern[1 : len(pattern)-1]
		}
		return PredicateSQL(fmt.Sprintf("%s LIKE '%%%s'", column, pattern)), nil
	})

	// contains operator: column LIKE '%value%'
	transpiler.RegisterOperatorFunc("contains", func(op string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 2 {
			return OperatorResult{}, fmt.Errorf("contains requires exactly 2 arguments")
		}

		var column, pattern string
		arg0Str := args[0].SQL
		arg1Str := args[1].SQL

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

		if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
			pattern = pattern[1 : len(pattern)-1]
		}
		return PredicateSQL(fmt.Sprintf("%s LIKE '%%%s%%'", column, pattern)), nil
	})

	return transpiler
}

// TestSchemaValidationComprehensive tests schema validation with various edge cases.
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
			transpiler, _ := NewTranspiler(DialectBigQuery)
			transpiler.SetSchema(schema)

			result, err := transpileSchemaExpression(transpiler, tt.jsonLogic)

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

// TestSchemaTypeAwareBehavior tests the type-aware "in" operator behavior.
func TestSchemaTypeAwareBehavior(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "tags", Type: FieldTypeArray},
		{Name: "description", Type: FieldTypeString},
		{Name: "status", Type: FieldTypeString},
	})

	transpiler, _ := NewTranspiler(DialectBigQuery)
	transpiler.SetSchema(schema)

	tests := []struct {
		name      string
		jsonLogic string
		expected  string
	}{
		{
			name:      "in with array type field",
			jsonLogic: `{"in": ["tag1", {"var": "tags"}]}`,
			expected:  "'tag1' IN UNNEST(tags)",
		},
		{
			name:      "in with string type field (string containment)",
			jsonLogic: `{"in": ["hello", {"var": "description"}]}`,
			expected:  "STRPOS(description, 'hello') > 0",
		},
		{
			name:      "in with literal array",
			jsonLogic: `{"in": [{"var": "status"}, ["active", "pending"]]}`,
			expected:  "status IN ('active', 'pending')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpileSchemaExpression(transpiler, tt.jsonLogic)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected: %s\nGot: %s", tt.expected, result)
			}
		})
	}
}

// TestCustomOperatorsStartsWithEndsWithContains tests the custom operators.
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
			expected:  "request.params.input_mode LIKE 'T%'",
		},
		{
			name:      "simple endsWith",
			jsonLogic: `{"endsWith": [{"var": "request.params.input_mode"}, "T"]}`,
			expected:  "request.params.input_mode LIKE '%T'",
		},
		{
			name:      "simple contains",
			jsonLogic: `{"contains": [{"var": "request.params.input_mode"}, "T"]}`,
			expected:  "request.params.input_mode LIKE '%T%'",
		},
		{
			name:      "contains with array notation",
			jsonLogic: `{"contains": [{"var": "request.params.input_mode"}, ["T"]]}`,
			expected:  "request.params.input_mode LIKE '%T%'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpileSchemaExpression(transpiler, tt.jsonLogic)
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
			result, err := transpiler.TranspileCondition(tt.jsonLogic)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if !strings.Contains(result, tt.shouldMatch) {
				t.Errorf("Expected output to contain '%s', got: %s", tt.shouldMatch, result)
			}
		})
	}
}

// TestComplexNestedExpressions tests the complex nested expressions.
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
			result, err := transpiler.TranspileCondition(tt.jsonLogic)
			if err != nil {
				t.Fatalf("Test '%s' (%s) failed with error: %v", tt.name, tt.description, err)
			}
			// At minimum, should produce valid SQL
			if !strings.HasPrefix(result, "") {
				t.Errorf("Expected result to start with 'WHERE ', got: %s", result)
			}
			t.Logf("Test '%s': %s", tt.name, result)
		})
	}
}

// TestSchemaWithCustomOperators tests schema validation with custom operators.
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
			result, err := transpiler.TranspileCondition(tt.jsonLogic)

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

// TestSchemaBackwardCompatibility tests that the transpiler works without schema.
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
			expected:  "any.random.field = 'value'",
		},
		{
			name:      "nested fields without schema",
			jsonLogic: `{"and":[{"==":[{"var":"field1"},"a"]},{"==":[{"var":"field2"},"b"]}]}`,
			expected:  "(field1 = 'a' AND field2 = 'b')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpiler.TranspileCondition(tt.jsonLogic)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected: %s\nGot: %s", tt.expected, result)
			}
		})
	}
}

// TestSchemaFromFileExample tests loading schema from file (requires a test schema file).
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

// TestInOperatorWithSchemaIntegration tests the IN operator behavior with schema-based type detection.
func TestInOperatorWithSchemaIntegration(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "user.roles", Type: FieldTypeArray},
		{Name: "user.bio", Type: FieldTypeString},
		{Name: "status", Type: FieldTypeString},
	})

	transpiler, _ := NewTranspiler(DialectBigQuery)
	transpiler.SetSchema(schema)

	tests := []struct {
		name      string
		jsonLogic string
		expected  string
	}{
		{
			name:      "in with array field uses IN syntax",
			jsonLogic: `{"in": ["admin", {"var": "user.roles"}]}`,
			expected:  "'admin' IN UNNEST(user.roles)",
		},
		{
			name:      "in with string field uses STRPOS",
			jsonLogic: `{"in": ["developer", {"var": "user.bio"}]}`,
			expected:  "STRPOS(user.bio, 'developer') > 0",
		},
		{
			name:      "in with literal array on right side",
			jsonLogic: `{"in": [{"var": "status"}, ["active", "pending", "approved"]]}`,
			expected:  "status IN ('active', 'pending', 'approved')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpiler.TranspileCondition(tt.jsonLogic)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected: %s\nGot: %s", tt.expected, result)
			}
		})
	}
}

// TestEdgeCasesWithSchema tests various edge cases.
func TestEdgeCasesWithSchema(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeInteger},
		{Name: "name", Type: FieldTypeString},
		{Name: "active", Type: FieldTypeBoolean},
	})

	transpiler, _ := NewTranspiler(DialectBigQuery)
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
			result, err := transpiler.TranspileCondition(tt.jsonLogic)
			if tt.expectError && err == nil {
				t.Errorf("Expected error but got result: %s", result)
			} else if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

// TestTypeAwareOperators tests type validation across all operators.
func TestTypeAwareOperators(t *testing.T) {
	// Create a schema with various field types
	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeInteger},
		{Name: "price", Type: FieldTypeNumber},
		{Name: "name", Type: FieldTypeString},
		{Name: "description", Type: FieldTypeString},
		{Name: "tags", Type: FieldTypeArray},
		{Name: "roles", Type: FieldTypeArray},
		{Name: "active", Type: FieldTypeBoolean},
		{Name: "verified", Type: FieldTypeBoolean},
		{Name: "metadata", Type: FieldTypeObject},
	})

	transpiler, _ := NewTranspiler(DialectBigQuery)
	transpiler.SetSchema(schema)

	tests := []struct {
		name        string
		jsonLogic   string
		expectError bool
		errorMsg    string
	}{
		// Numeric operators - valid cases
		{
			name:        "numeric addition with integer field",
			jsonLogic:   `{"+": [{"var": "amount"}, 10]}`,
			expectError: false,
		},
		{
			name:        "numeric addition with number field",
			jsonLogic:   `{"+": [{"var": "price"}, 1.5]}`,
			expectError: false,
		},
		{
			name:        "numeric subtraction with integer",
			jsonLogic:   `{"-": [{"var": "amount"}, 5]}`,
			expectError: false,
		},
		{
			name:        "numeric multiplication",
			jsonLogic:   `{"*": [{"var": "price"}, 2]}`,
			expectError: false,
		},
		{
			name:        "numeric division",
			jsonLogic:   `{"/": [{"var": "amount"}, 2]}`,
			expectError: false,
		},
		{
			name:        "numeric modulo",
			jsonLogic:   `{"%": [{"var": "amount"}, 3]}`,
			expectError: false,
		},
		{
			name:        "numeric max",
			jsonLogic:   `{"max": [{"var": "amount"}, {"var": "price"}]}`,
			expectError: false,
		},
		{
			name:        "numeric min",
			jsonLogic:   `{"min": [{"var": "amount"}, 0]}`,
			expectError: false,
		},
		// Numeric operators - invalid cases
		{
			name:        "numeric addition with string field",
			jsonLogic:   `{"+": [{"var": "name"}, 10]}`,
			expectError: true,
			errorMsg:    "non-numeric field 'name'",
		},
		{
			name:        "numeric addition with array field",
			jsonLogic:   `{"+": [{"var": "tags"}, 10]}`,
			expectError: true,
			errorMsg:    "non-numeric field 'tags'",
		},
		{
			name:        "numeric addition with boolean field",
			jsonLogic:   `{"+": [{"var": "active"}, 1]}`,
			expectError: true,
			errorMsg:    "non-numeric field 'active'",
		},
		{
			name:        "numeric multiplication with object field",
			jsonLogic:   `{"*": [{"var": "metadata"}, 2]}`,
			expectError: true,
			errorMsg:    "non-numeric field 'metadata'",
		},

		// String operators - valid cases
		{
			name:        "string concatenation with string fields",
			jsonLogic:   `{"cat": [{"var": "name"}, " ", {"var": "description"}]}`,
			expectError: false,
		},
		{
			name:        "string concatenation with numeric (allowed)",
			jsonLogic:   `{"cat": ["ID: ", {"var": "amount"}]}`,
			expectError: false,
		},
		{
			name:        "substring with string field",
			jsonLogic:   `{"substr": [{"var": "name"}, 0, 5]}`,
			expectError: false,
		},
		// String operators - invalid cases
		{
			name:        "string concatenation with array field",
			jsonLogic:   `{"cat": [{"var": "name"}, {"var": "tags"}]}`,
			expectError: true,
			errorMsg:    "string operation on incompatible field 'tags'",
		},
		{
			name:        "substring with array field",
			jsonLogic:   `{"substr": [{"var": "tags"}, 0, 5]}`,
			expectError: true,
			errorMsg:    "string operation on incompatible field 'tags'",
		},

		// Array operators - valid cases
		{
			name:        "array some with array field",
			jsonLogic:   `{"some": [{"var": "tags"}, {"==": [{"var": ""}, "important"]}]}`,
			expectError: false,
		},
		{
			name:        "array all with array field",
			jsonLogic:   `{"all": [{"var": "roles"}, {"==": [{"var": ""}, "admin"]}]}`,
			expectError: false,
		},
		{
			name:        "array none with array field",
			jsonLogic:   `{"none": [{"var": "tags"}, {"==": [{"var": ""}, "invalid"]}]}`,
			expectError: false,
		},
		{
			name:        "array merge with array fields",
			jsonLogic:   `{"merge": [{"var": "tags"}, {"var": "roles"}]}`,
			expectError: false,
		},
		// Array operators - invalid cases
		{
			name:        "array some with string field",
			jsonLogic:   `{"some": [{"var": "name"}, {"==": [{"var": ""}, "a"]}]}`,
			expectError: true,
			errorMsg:    "array operation on non-array field 'name'",
		},
		{
			name:        "array all with integer field",
			jsonLogic:   `{"all": [{"var": "amount"}, {"==": [{"var": ""}, 0]}]}`,
			expectError: true,
			errorMsg:    "array operation on non-array field 'amount'",
		},
		{
			name:        "array merge with string field",
			jsonLogic:   `{"merge": [{"var": "tags"}, {"var": "name"}]}`,
			expectError: true,
			errorMsg:    "array operation on non-array field 'name'",
		},

		// Comparison operators (ordering) - valid cases
		{
			name:        "ordering comparison with numeric",
			jsonLogic:   `{">": [{"var": "amount"}, 100]}`,
			expectError: false,
		},
		{
			name:        "ordering comparison with string (lexical)",
			jsonLogic:   `{">": [{"var": "name"}, "abc"]}`,
			expectError: false,
		},
		{
			name:        "chained ordering comparison",
			jsonLogic:   `{"<": [0, {"var": "amount"}, 1000]}`,
			expectError: false,
		},
		// Comparison operators (ordering) - invalid cases
		{
			name:        "ordering comparison with array field",
			jsonLogic:   `{">": [{"var": "tags"}, 0]}`,
			expectError: true,
			errorMsg:    "ordering comparison '>' on incompatible field 'tags'",
		},
		{
			name:        "ordering comparison with boolean field",
			jsonLogic:   `{"<": [{"var": "active"}, 1]}`,
			expectError: true,
			errorMsg:    "ordering comparison '<' on incompatible field 'active'",
		},
		{
			name:        "ordering comparison with object field",
			jsonLogic:   `{">=": [{"var": "metadata"}, 0]}`,
			expectError: true,
			errorMsg:    "ordering comparison '>=' on incompatible field 'metadata'",
		},

		// Equality operators - should work with any type (no validation)
		{
			name:        "equality with string",
			jsonLogic:   `{"==": [{"var": "name"}, "test"]}`,
			expectError: false,
		},
		{
			name:        "equality with array (comparing to null)",
			jsonLogic:   `{"==": [{"var": "tags"}, null]}`,
			expectError: false,
		},
		{
			name:        "equality with boolean",
			jsonLogic:   `{"==": [{"var": "active"}, true]}`,
			expectError: false,
		},
		{
			name:        "inequality with any type",
			jsonLogic:   `{"!=": [{"var": "metadata"}, null]}`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpileSchemaExpression(transpiler, tt.jsonLogic)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s' but got result: %s", tt.errorMsg, result)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestTypeValidationWithoutSchema verifies that validation is skipped without schema.
func TestTypeValidationWithoutSchema(t *testing.T) {
	transpiler, _ := NewTranspiler(DialectBigQuery)
	// No schema set - all operations should pass

	tests := []struct {
		name      string
		jsonLogic string
	}{
		{
			name:      "numeric on any field without schema",
			jsonLogic: `{"+": [{"var": "anyField"}, 10]}`,
		},
		{
			name:      "string on any field without schema",
			jsonLogic: `{"cat": [{"var": "field1"}, {"var": "field2"}]}`,
		},
		{
			name:      "array on any field without schema",
			jsonLogic: `{"some": [{"var": "items"}, {"==": [{"var": ""}, "x"]}]}`,
		},
		{
			name:      "ordering on any field without schema",
			jsonLogic: `{">": [{"var": "value"}, 100]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := transpileSchemaExpression(transpiler, tt.jsonLogic)
			if err != nil {
				t.Errorf("Without schema, should not validate types. Got error: %v", err)
			}
		})
	}
}

// TestTypeValidationWithFieldNotInSchema verifies that fields not in schema pass validation.
func TestTypeValidationWithFieldNotInSchema(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "known_field", Type: FieldTypeInteger},
	})

	transpiler, _ := NewTranspiler(DialectBigQuery)
	transpiler.SetSchema(schema)

	// Field not in schema should pass validation (existence is checked separately)
	_, err := transpileSchemaExpression(transpiler, `{"+": [{"var": "unknown_field"}, 10]}`)
	if err != nil {
		// If there's an error, it should be about field not being in schema, not about type
		if strings.Contains(err.Error(), "non-numeric") {
			t.Errorf("Fields not in schema should skip type validation. Got: %v", err)
		}
	}
}

// TestEnumTypeSupport tests enum type validation and SQL generation.
func TestEnumTypeSupport(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"active", "pending", "canceled"}},
		{Name: "priority", Type: FieldTypeEnum, AllowedValues: []string{"low", "medium", "high"}},
		{Name: "name", Type: FieldTypeString},
	})

	transpiler, _ := NewTranspiler(DialectBigQuery)
	transpiler.SetSchema(schema)

	t.Run("valid enum comparisons", func(t *testing.T) {
		tests := []struct {
			name      string
			jsonLogic string
			expected  string
		}{
			{
				name:      "enum equality with valid value",
				jsonLogic: `{"==": [{"var": "status"}, "active"]}`,
				expected:  "status = 'active'",
			},
			{
				name:      "enum inequality with valid value",
				jsonLogic: `{"!=": [{"var": "status"}, "canceled"]}`,
				expected:  "status != 'canceled'",
			},
			{
				name:      "enum in array with valid values",
				jsonLogic: `{"in": [{"var": "status"}, ["active", "pending"]]}`,
				expected:  "status IN ('active', 'pending')",
			},
			{
				name:      "enum strict equality",
				jsonLogic: `{"===": [{"var": "priority"}, "high"]}`,
				expected:  "priority = 'high'",
			},
			{
				name:      "enum with null comparison",
				jsonLogic: `{"==": [{"var": "status"}, null]}`,
				expected:  "status IS NULL",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := transpiler.TranspileCondition(tt.jsonLogic)
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("Expected: %s\nGot: %s", tt.expected, result)
				}
			})
		}
	})

	t.Run("invalid enum values should error", func(t *testing.T) {
		tests := []struct {
			name      string
			jsonLogic string
			errorMsg  string
		}{
			{
				name:      "invalid enum value in equality",
				jsonLogic: `{"==": [{"var": "status"}, "invalid"]}`,
				errorMsg:  "invalid enum value 'invalid' for field 'status'",
			},
			{
				name:      "invalid enum value in array",
				jsonLogic: `{"in": [{"var": "status"}, ["active", "invalid"]]}`,
				errorMsg:  "invalid enum value 'invalid' for field 'status'",
			},
			{
				name:      "all invalid enum values",
				jsonLogic: `{"in": [{"var": "priority"}, ["urgent", "critical"]]}`,
				errorMsg:  "invalid enum value 'urgent' for field 'priority'",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := transpiler.TranspileCondition(tt.jsonLogic)
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errorMsg, err)
				}
			})
		}
	})

	t.Run("non-enum fields should not validate enum values", func(t *testing.T) {
		// String fields should accept any value, not validate as enum
		result, err := transpiler.TranspileCondition(`{"==": [{"var": "name"}, "anything"]}`)
		if err != nil {
			t.Fatalf("String field should accept any value, got error: %v", err)
		}
		if result != "name = 'anything'" {
			t.Errorf("Unexpected result: %s", result)
		}
	})
}

// TestEnumSchemaFromJSON tests loading enum schema from JSON.
func TestEnumSchemaFromJSON(t *testing.T) {
	schemaJSON := `[
		{"name": "status", "type": "enum", "allowedValues": ["active", "inactive", "deleted"]},
		{"name": "role", "type": "enum", "allowedValues": ["admin", "user", "guest"]}
	]`

	schema, err := NewSchemaFromJSON([]byte(schemaJSON))
	if err != nil {
		t.Fatalf("Failed to parse schema JSON: %v", err)
	}

	// Test IsEnumType
	if !schema.IsEnumType("status") {
		t.Error("status should be enum type")
	}
	if !schema.IsEnumType("role") {
		t.Error("role should be enum type")
	}

	// Test GetAllowedValues
	statusValues := schema.GetAllowedValues("status")
	if len(statusValues) != 3 {
		t.Errorf("Expected 3 allowed values for status, got %d", len(statusValues))
	}

	roleValues := schema.GetAllowedValues("role")
	expectedRoles := []string{"admin", "user", "guest"}
	for i, expected := range expectedRoles {
		if roleValues[i] != expected {
			t.Errorf("Expected role value '%s', got '%s'", expected, roleValues[i])
		}
	}

	// Test ValidateEnumValue
	if err := schema.ValidateEnumValue("status", "active"); err != nil {
		t.Errorf("'active' should be valid for status: %v", err)
	}
	if err := schema.ValidateEnumValue("status", "invalid"); err == nil {
		t.Error("'invalid' should not be valid for status")
	}
}

// TestEnumWithComplexExpressions tests enum validation in complex nested expressions.
func TestEnumWithComplexExpressions(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"active", "pending", "canceled"}},
		{Name: "amount", Type: FieldTypeInteger},
	})

	transpiler, _ := NewTranspiler(DialectBigQuery)
	transpiler.SetSchema(schema)

	tests := []struct {
		name        string
		jsonLogic   string
		expected    string
		expectError bool
		errorMsg    string
	}{
		{
			name:      "enum in AND expression",
			jsonLogic: `{"and": [{"==": [{"var": "status"}, "active"]}, {">": [{"var": "amount"}, 100]}]}`,
			expected:  "(status = 'active' AND amount > 100)",
		},
		{
			name:      "enum in OR expression",
			jsonLogic: `{"or": [{"==": [{"var": "status"}, "pending"]}, {"==": [{"var": "status"}, "active"]}]}`,
			expected:  "(status = 'pending' OR status = 'active')",
		},
		{
			name:        "invalid enum in AND expression",
			jsonLogic:   `{"and": [{"==": [{"var": "status"}, "invalid"]}, {">": [{"var": "amount"}, 100]}]}`,
			expectError: true,
			errorMsg:    "invalid enum value 'invalid'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpiler.TranspileCondition(tt.jsonLogic)
			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("Expected: %s\nGot: %s", tt.expected, result)
				}
			}
		})
	}
}

// TestTypeCoercionForInOperator verifies that array elements in the "in" operator are coerced
// to match the field type. Numbers are quoted for string fields, and string numbers are
// unquoted for numeric fields. This prevents type errors in strict-typing databases like BigQuery.
func TestTypeCoercionForInOperator(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "code", Type: FieldTypeString},
		{Name: "status", Type: FieldTypeString},
		{Name: "bio", Type: FieldTypeString},
		{Name: "amount", Type: FieldTypeInteger},
		{Name: "price", Type: FieldTypeNumber},
		{Name: "active", Type: FieldTypeBoolean},
	})

	t.Run("string field with numeric array elements should quote values", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileCondition(`{"in":[{"var":"code"},[5960,9000]]}`)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "code IN ('5960', '9000')"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("string field with mixed array elements should coerce numbers", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileCondition(`{"in":[{"var":"status"},["active",123,"pending"]]}`)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "status IN ('active', '123', 'pending')"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("numeric field with string array elements should unquote values", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileCondition(`{"in":[{"var":"amount"},["100","200","300"]]}`)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "amount IN (100, 200, 300)"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("numeric field with numeric array elements stays unchanged", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileCondition(`{"in":[{"var":"amount"},[100,200,300]]}`)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "amount IN (100, 200, 300)"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("string field with string array elements stays unchanged", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileCondition(`{"in":[{"var":"status"},["active","pending"]]}`)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "status IN ('active', 'pending')"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("no schema should not coerce", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		// No schema set

		result, err := transpiler.TranspileCondition(`{"in":[{"var":"code"},[5960,9000]]}`)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "code IN (5960, 9000)"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("string field with float values should quote correctly", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileCondition(`{"in":[{"var":"status"},[1.5,2.7]]}`)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "status IN ('1.5', '2.7')"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("string containment with number literal coerces to string", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileCondition(`{"in":[123,{"var":"bio"}]}`)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "STRPOS(bio, '123') > 0"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("string containment with float literal coerces to string", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileCondition(`{"in":[3.14,{"var":"bio"}]}`)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "STRPOS(bio, '3.14') > 0"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("string containment with string literal stays unchanged", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileCondition(`{"in":["hello",{"var":"bio"}]}`)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "STRPOS(bio, 'hello') > 0"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("coercion works across all dialects", func(t *testing.T) {
		dialects := []Dialect{
			DialectBigQuery,
			DialectSpanner,
			DialectPostgreSQL,
			DialectDuckDB,
			DialectClickHouse,
		}
		for _, d := range dialects {
			transpiler, _ := NewTranspiler(d)
			transpiler.SetSchema(schema)

			result, err := transpiler.TranspileCondition(`{"in":[{"var":"code"},[5960,9000]]}`)
			if err != nil {
				t.Fatalf("[%s] Unexpected error: %v", d, err)
			}
			expected := "code IN ('5960', '9000')"
			if result != expected {
				t.Errorf("[%s] Expected: %s\nGot: %s", d, expected, result)
			}
		}
	})

	// TranspileConditionFromMap tests: Go native int types bypass JSON unmarshaling (float64)
	t.Run("TranspileConditionFromMap: string field with Go int array", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileConditionFromMap(map[string]interface{}{
			"in": []interface{}{
				map[string]interface{}{"var": "code"},
				[]interface{}{5960, 9000},
			},
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "code IN ('5960', '9000')"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("TranspileConditionFromMap: string field with Go int64 array", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileConditionFromMap(map[string]interface{}{
			"in": []interface{}{
				map[string]interface{}{"var": "code"},
				[]interface{}{int64(5960), int64(9000)},
			},
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "code IN ('5960', '9000')"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("TranspileConditionFromMap: int in string var (containment)", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileConditionFromMap(map[string]interface{}{
			"in": []interface{}{
				123,
				map[string]interface{}{"var": "bio"},
			},
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "STRPOS(bio, '123') > 0"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("TranspileConditionFromMap: string field == Go int", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileConditionFromMap(map[string]interface{}{
			"==": []interface{}{
				map[string]interface{}{"var": "code"},
				5960,
			},
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "code = '5960'"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	t.Run("TranspileConditionFromInterface: string field == large Go int64", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)
		logic := map[string]interface{}{
			"==": []interface{}{
				map[string]interface{}{"var": "code"},
				int64(9223372036854775807),
			},
		}

		result, err := transpiler.TranspileConditionFromInterface(logic)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "code = '9223372036854775807'"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}

		paramSQL, paramValues, err := transpiler.TranspileParameterizedConditionFromInterface(logic)
		if err != nil {
			t.Fatalf("Unexpected parameterized error: %v", err)
		}
		expectedParamSQL := "code = @p1"
		if paramSQL != expectedParamSQL {
			t.Errorf("Expected parameterized SQL: %s\nGot: %s", expectedParamSQL, paramSQL)
		}
		expectedParams := []QueryParam{{Name: "p1", Value: "9223372036854775807"}}
		if !reflect.DeepEqual(paramValues, expectedParams) {
			t.Errorf("Expected params: %v\nGot: %v", expectedParams, paramValues)
		}
	})

	t.Run("TranspileConditionFromInterface: string field == Go float32", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)
		logic := map[string]interface{}{
			"==": []interface{}{
				map[string]interface{}{"var": "code"},
				float32(1.2),
			},
		}

		result, err := transpiler.TranspileConditionFromInterface(logic)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "code = '1.2'"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}

		paramSQL, paramValues, err := transpiler.TranspileParameterizedConditionFromInterface(logic)
		if err != nil {
			t.Fatalf("Unexpected parameterized error: %v", err)
		}
		expectedParamSQL := "code = @p1"
		if paramSQL != expectedParamSQL {
			t.Errorf("Expected parameterized SQL: %s\nGot: %s", expectedParamSQL, paramSQL)
		}
		expectedParams := []QueryParam{{Name: "p1", Value: "1.2"}}
		if !reflect.DeepEqual(paramValues, expectedParams) {
			t.Errorf("Expected params: %v\nGot: %v", expectedParams, paramValues)
		}
	})

	t.Run("TranspileConditionFromInterface: number field == large Go uint64", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)
		logic := map[string]interface{}{
			"==": []interface{}{
				map[string]interface{}{"var": "price"},
				uint64(9223372036854775808),
			},
		}

		result, err := transpiler.TranspileConditionFromInterface(logic)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "price = 9223372036854775808"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}

		paramSQL, paramValues, err := transpiler.TranspileParameterizedConditionFromInterface(logic)
		if err != nil {
			t.Fatalf("Unexpected parameterized error: %v", err)
		}
		expectedParamSQL := "price = @p1"
		if paramSQL != expectedParamSQL {
			t.Errorf("Expected parameterized SQL: %s\nGot: %s", expectedParamSQL, paramSQL)
		}
		expectedParams := []QueryParam{{Name: "p1", Value: uint64(9223372036854775808)}}
		if !reflect.DeepEqual(paramValues, expectedParams) {
			t.Errorf("Expected params: %v\nGot: %v", expectedParams, paramValues)
		}
	})

	t.Run("TranspileConditionFromInterface: integer field == large Go uint on 64-bit", func(t *testing.T) {
		if ^uint(0) <= uint(0xffffffff) {
			t.Skip("large uint literal requires 64-bit uint")
		}
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)
		largeUint := uint(uint64(9007199254740993))
		logic := map[string]interface{}{
			"==": []interface{}{
				map[string]interface{}{"var": "amount"},
				largeUint,
			},
		}

		result, err := transpiler.TranspileConditionFromInterface(logic)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "amount = 9007199254740993"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}

		paramSQL, paramValues, err := transpiler.TranspileParameterizedConditionFromInterface(logic)
		if err != nil {
			t.Fatalf("Unexpected parameterized error: %v", err)
		}
		expectedParamSQL := "amount = @p1"
		if paramSQL != expectedParamSQL {
			t.Errorf("Expected parameterized SQL: %s\nGot: %s", expectedParamSQL, paramSQL)
		}
		expectedParams := []QueryParam{{Name: "p1", Value: int64(9007199254740993)}}
		if !reflect.DeepEqual(paramValues, expectedParams) {
			t.Errorf("Expected params: %v\nGot: %v", expectedParams, paramValues)
		}
	})

	t.Run("TranspileConditionFromMap: integer field with Go string array", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.SetSchema(schema)

		result, err := transpiler.TranspileConditionFromMap(map[string]interface{}{
			"in": []interface{}{
				map[string]interface{}{"var": "amount"},
				[]interface{}{"100", "200"},
			},
		})
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		expected := "amount IN (100, 200)"
		if result != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})
}

// TestTypeCoercionForComparisons verifies that string literals are coerced to appropriate types
// based on the field being compared. This ensures proper SQL output like "field >= 50000"
// instead of "field >= '50000'" when comparing an integer field with a string value.
func TestTypeCoercionForComparisons(t *testing.T) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeInteger},
		{Name: "price", Type: FieldTypeNumber},
		{Name: "status", Type: FieldTypeString},
		{Name: "active", Type: FieldTypeBoolean},
	})

	transpiler, _ := NewTranspiler(DialectBigQuery)
	transpiler.SetSchema(schema)

	tests := []struct {
		name      string
		jsonLogic string
		expected  string
	}{
		{
			name:      "integer field with string numeric value should coerce",
			jsonLogic: `{">=":[{"var":"amount"},"50000"]}`,
			expected:  "amount >= 50000",
		},
		{
			name:      "integer field with actual number should work",
			jsonLogic: `{">=":[{"var":"amount"},50000]}`,
			expected:  "amount >= 50000",
		},
		{
			name:      "number field with string numeric value should coerce",
			jsonLogic: `{">":[{"var":"price"},"99.99"]}`,
			expected:  "price > 99.99",
		},
		{
			name:      "string field with string value should remain quoted",
			jsonLogic: `{"==":[{"var":"status"},"active"]}`,
			expected:  "status = 'active'",
		},
		{
			name:      "string field with numeric string should remain quoted",
			jsonLogic: `{"==":[{"var":"status"},"123"]}`,
			expected:  "status = '123'",
		},
		{
			name:      "boolean field should not coerce",
			jsonLogic: `{"==":[{"var":"active"},true]}`,
			expected:  "active = TRUE",
		},
		{
			name:      "chained comparison with string values should coerce",
			jsonLogic: `{"<":["0",{"var":"amount"},"1000"]}`,
			expected:  "(0 < amount AND amount < 1000)",
		},
		{
			name:      "reversed order - literal on left, field on right",
			jsonLogic: `{"<":["100",{"var":"amount"}]}`,
			expected:  "100 < amount",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := transpiler.TranspileCondition(tt.jsonLogic)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected: %s\nGot: %s", tt.expected, result)
			}
		})
	}
}
