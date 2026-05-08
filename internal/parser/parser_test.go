package parser

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestNewParser(t *testing.T) {
	p := NewParser(nil)
	if p == nil {
		t.Fatal("NewParser(nil) returned nil")
		return
	}
	if p.validator == nil {
		t.Fatal("validator is nil")
	}
	if p.dataOp == nil {
		t.Fatal("dataOp is nil")
	}
	if p.comparisonOp == nil {
		t.Fatal("comparisonOp is nil")
	}
	if p.logicalOp == nil {
		t.Fatal("logicalOp is nil")
	}
}

func TestParser_Parse(t *testing.T) {
	p := NewParser(nil)

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		// Simple comparisons
		{
			name:     "simple greater than",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name:     "simple equality",
			input:    map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "pending"}},
			expected: "status = 'pending'",
			hasError: false,
		},
		{
			name:     "simple inequality",
			input:    map[string]interface{}{"!=": []interface{}{map[string]interface{}{"var": "verified"}, false}},
			expected: "verified != FALSE",
			hasError: false,
		},

		// AND operations
		{
			name: "and with two conditions",
			input: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 5000}},
					map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "pending"}},
				},
			},
			expected: "(amount > 5000 AND status = 'pending')",
			hasError: false,
		},
		{
			name: "and with three conditions",
			input: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
					map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "active"}},
					map[string]interface{}{"!=": []interface{}{map[string]interface{}{"var": "verified"}, false}},
				},
			},
			expected: "(amount > 1000 AND status = 'active' AND verified != FALSE)",
			hasError: false,
		},

		// OR operations
		{
			name: "or with two conditions",
			input: map[string]interface{}{
				"or": []interface{}{
					map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": "failedAttempts"}, 5}},
					map[string]interface{}{"in": []interface{}{map[string]interface{}{"var": "country"}, []interface{}{"CN", "RU"}}},
				},
			},
			expected: "(failedAttempts >= 5 OR country IN ('CN', 'RU'))",
			hasError: false,
		},

		// NOT operations
		{
			name:     "not operation",
			input:    map[string]interface{}{"!": []interface{}{map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "verified"}, true}}}},
			expected: "NOT (verified = TRUE)",
			hasError: false,
		},

		// IF operations
		{
			name: "if with condition and then",
			input: map[string]interface{}{
				"if": []interface{}{
					map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "age"}, 18}},
					"adult",
				},
			},
			expected: "CASE WHEN age > 18 THEN 'adult' ELSE NULL END",
			hasError: false,
		},
		{
			name: "if with condition, then, and else",
			input: map[string]interface{}{
				"if": []interface{}{
					map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "age"}, 18}},
					"adult",
					"minor",
				},
			},
			expected: "CASE WHEN age > 18 THEN 'adult' ELSE 'minor' END",
			hasError: false,
		},

		// Nested operations
		{
			name: "nested and/or",
			input: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "transaction.amount"}, 10000}},
					map[string]interface{}{"or": []interface{}{
						map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "user.verified"}, false}},
						map[string]interface{}{"<": []interface{}{map[string]interface{}{"var": "user.accountAgeDays"}, 7}},
					}},
				},
			},
			expected: "(transaction.amount > 10000 AND (user.verified = FALSE OR user.accountAgeDays < 7))",
			hasError: false,
		},

		// Missing operations
		{
			name:     "missing operation",
			input:    map[string]interface{}{"missing": "field"},
			expected: "field IS NULL",
			hasError: false,
		},
		{
			name:     "missing_some operation",
			input:    map[string]interface{}{"missing_some": []interface{}{1, []interface{}{"field1", "field2"}}},
			expected: "(field1 IS NULL OR field2 IS NULL)",
			hasError: false,
		},

		// IN operations
		{
			name:     "in operation with strings",
			input:    map[string]interface{}{"in": []interface{}{map[string]interface{}{"var": "country"}, []interface{}{"CN", "RU"}}},
			expected: "country IN ('CN', 'RU')",
			hasError: false,
		},
		{
			name:     "in operation with numbers",
			input:    map[string]interface{}{"in": []interface{}{map[string]interface{}{"var": "status"}, []interface{}{1, 2, 3}}},
			expected: "status IN (1, 2, 3)",
			hasError: false,
		},

		// Error cases
		{
			name:     "primitive value",
			input:    "hello",
			expected: "",
			hasError: true,
		},
		{
			name:     "array value",
			input:    []interface{}{1, 2, 3},
			expected: "",
			hasError: true,
		},
		{
			name:     "multiple keys in object",
			input:    map[string]interface{}{"a": 1, "b": 2},
			expected: "",
			hasError: true,
		},
		{
			name:     "unsupported operator",
			input:    map[string]interface{}{"unsupported": []interface{}{1, 2}},
			expected: "",
			hasError: true,
		},
		{
			name:     "invalid comparison args",
			input:    map[string]interface{}{">": "not-array"},
			expected: "",
			hasError: true,
		},
		{
			name:     "invalid logical args",
			input:    map[string]interface{}{"and": "not-array"},
			expected: "",
			hasError: true,
		},

		// Numeric operations
		{
			name: "addition operation",
			input: map[string]interface{}{
				"+": []interface{}{5, 3},
			},
			expected: "(5 + 3)",
			hasError: false,
		},
		{
			name: "multiplication with var",
			input: map[string]interface{}{
				"*": []interface{}{map[string]interface{}{"var": "price"}, 1.2},
			},
			expected: "(price * 1.2)",
			hasError: false,
		},
		{
			name: "max operation",
			input: map[string]interface{}{
				"max": []interface{}{10, 20, 15},
			},
			expected: "GREATEST(10, 20, 15)",
			hasError: false,
		},

		// Array operations
		{
			name: "merge operation",
			input: map[string]interface{}{
				"merge": []interface{}{[]interface{}{1, 2}, []interface{}{3, 4}},
			},
			expected: "ARRAY_CONCAT([1, 2], [3, 4])",
			hasError: false,
		},
		{
			name: "map operation",
			input: map[string]interface{}{
				"map": []interface{}{map[string]interface{}{"var": "numbers"}, map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": "item"}, 1}}},
			},
			expected: "ARRAY(SELECT (elem + 1) FROM UNNEST(numbers) AS elem)",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Parse(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Parse() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Parse() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("Parse() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestParser_parseExpression(t *testing.T) {
	p := NewParser(nil)

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name:     "primitive value",
			input:    "hello",
			expected: "",
			hasError: true,
		},
		{
			name:     "array value",
			input:    []interface{}{1, 2, 3},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.parseExpression(tt.input, "$")

			if tt.hasError {
				if err == nil {
					t.Errorf("parseExpression() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("parseExpression() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("parseExpression() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestParser_parseOperator(t *testing.T) {
	p := NewParser(nil)

	tests := []struct {
		name     string
		operator string
		args     interface{}
		expected string
		hasError bool
	}{
		{
			name:     "var operator",
			operator: "var",
			args:     "amount",
			expected: "amount",
			hasError: false,
		},
		{
			name:     "comparison operator",
			operator: ">",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, 1000},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name:     "logical operator",
			operator: "and",
			args:     []interface{}{map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}}},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name:     "unsupported operator",
			operator: "unsupported",
			args:     []interface{}{1, 2},
			expected: "",
			hasError: true,
		},
		{
			name:     "comparison with non-array args",
			operator: ">",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "logical with non-array args",
			operator: "and",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.parseOperator(tt.operator, tt.args, "$")

			if tt.hasError {
				if err == nil {
					t.Errorf("parseOperator() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("parseOperator() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("parseOperator() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestParser_isPrimitive(t *testing.T) {
	p := NewParser(nil)

	tests := []struct {
		input    interface{}
		expected bool
	}{
		{"hello", true},
		{42, true},
		{true, true},
		{false, true},
		{nil, true},
		{3.14, true},
		{int64(123), true},
		{[]interface{}{1, 2}, false},
		{map[string]interface{}{"a": 1}, false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			result := p.isPrimitive(tt.input)
			if result != tt.expected {
				t.Errorf("isPrimitive(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// --- Helper types for custom operator tests ---

// mockCustomHandler implements CustomOperatorHandler for testing.
type mockCustomHandler struct {
	toSQL func(operator string, args []interface{}) (string, error)
}

func (m *mockCustomHandler) ToSQL(operator string, args []operators.OperatorArg) (operators.OperatorResult, error) {
	legacyArgs := make([]interface{}, len(args))
	for i, arg := range args {
		legacyArgs[i] = arg.SQL
	}
	sql, err := m.toSQL(operator, legacyArgs)
	if err != nil {
		return operators.OperatorResult{}, err
	}
	return operators.PredicateSQL(sql), nil
}

// mockSchemaProvider implements operators.SchemaProvider for testing.
type mockSchemaProvider struct {
	fields map[string]string // field name -> type
}

func (m *mockSchemaProvider) HasField(fieldName string) bool {
	_, ok := m.fields[fieldName]
	return ok
}

func (m *mockSchemaProvider) GetFieldType(fieldName string) string {
	return m.fields[fieldName]
}

func (m *mockSchemaProvider) ValidateField(fieldName string) error {
	if _, ok := m.fields[fieldName]; !ok {
		return fmt.Errorf("field '%s' is not defined in schema", fieldName)
	}
	return nil
}

func (m *mockSchemaProvider) IsArrayType(fieldName string) bool {
	return m.fields[fieldName] == "array"
}

func (m *mockSchemaProvider) IsStringType(fieldName string) bool {
	return m.fields[fieldName] == "string"
}

func (m *mockSchemaProvider) IsNumericType(fieldName string) bool {
	t := m.fields[fieldName]
	return t == "integer" || t == "number"
}

func (m *mockSchemaProvider) IsBooleanType(fieldName string) bool {
	return m.fields[fieldName] == "boolean"
}

func (m *mockSchemaProvider) IsEnumType(fieldName string) bool {
	return m.fields[fieldName] == "enum"
}

func (m *mockSchemaProvider) GetAllowedValues(fieldName string) []string {
	return nil
}

func (m *mockSchemaProvider) ValidateEnumValue(fieldName, value string) error {
	return nil
}

// --- Tests for ParseCondition (0% coverage) ---

func TestParser_ParseCondition(t *testing.T) {
	p := NewParser(nil)

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison returns condition without WHERE",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name:     "equality condition without WHERE",
			input:    map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "active"}},
			expected: "status = 'active'",
			hasError: false,
		},
		{
			name: "and condition without WHERE",
			input: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "age"}, 18}},
					map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "active"}, true}},
				},
			},
			expected: "(age > 18 AND active = TRUE)",
			hasError: false,
		},
		{
			name:     "validation error on primitive",
			input:    "hello",
			expected: "",
			hasError: true,
		},
		{
			name:     "validation error on empty array",
			input:    []interface{}{},
			expected: "",
			hasError: true,
		},
		{
			name:     "unsupported operator error",
			input:    map[string]interface{}{"unknownOp": []interface{}{1, 2}},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.ParseCondition(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("ParseCondition() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("ParseCondition() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("ParseCondition() = %q, expected %q", result, tt.expected)
				}
				// Verify it does NOT have "" prefix
				if strings.HasPrefix(result, "WHERE ") {
					t.Errorf("ParseCondition() should not have WHERE prefix, got %q", result)
				}
			}
		})
	}
}

// --- Tests for SetCustomOperatorLookup (0% coverage) ---

func TestParser_SetCustomOperatorLookup(t *testing.T) {
	t.Run("sets custom operator lookup and validates through validator", func(t *testing.T) {
		p := NewParser(nil)

		lengthHandler := &mockCustomHandler{
			toSQL: func(op string, args []interface{}) (string, error) {
				return fmt.Sprintf("LENGTH(%s)", args[0]), nil
			},
		}

		lookup := func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "length" {
				return lengthHandler, true
			}
			return nil, false
		}

		p.SetCustomOperatorLookup(lookup)

		// Now custom operator should be accepted by the parser
		result, err := p.Parse(map[string]interface{}{
			"length": []interface{}{map[string]interface{}{"var": "name"}},
		})
		if err != nil {
			t.Fatalf("Parse() unexpected error: %v", err)
		}
		expected := "LENGTH(name)"
		if result != expected {
			t.Errorf("Parse() = %q, expected %q", result, expected)
		}
	})

	t.Run("nil lookup function in validator checker returns false", func(t *testing.T) {
		p := NewParser(nil)

		// SetCustomOperatorLookup with nil triggers the internal checker
		p.SetCustomOperatorLookup(nil)

		// Unknown operator should still be rejected
		_, err := p.Parse(map[string]interface{}{
			"length": []interface{}{map[string]interface{}{"var": "name"}},
		})
		if err == nil {
			t.Error("Parse() expected error for unregistered custom operator")
		}
	})

	t.Run("custom operator with single non-array arg", func(t *testing.T) {
		p := NewParser(nil)

		singleHandler := &mockCustomHandler{
			toSQL: func(op string, args []interface{}) (string, error) {
				return fmt.Sprintf("SINGLE(%s)", args[0]), nil
			},
		}

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "single" {
				return singleHandler, true
			}
			return nil, false
		})

		// Single non-array argument: {"single": {"var": "x"}}
		result, err := p.Parse(map[string]interface{}{
			"single": map[string]interface{}{"var": "x"},
		})
		if err != nil {
			t.Fatalf("Parse() unexpected error: %v", err)
		}
		expected := "SINGLE(x)"
		if result != expected {
			t.Errorf("Parse() = %q, expected %q", result, expected)
		}
	})
}

// --- Tests for SetSchema (0% coverage) ---

func TestParser_SetSchema(t *testing.T) {
	t.Run("sets schema on shared config", func(t *testing.T) {
		p := NewParser(nil)

		schema := &mockSchemaProvider{
			fields: map[string]string{
				"amount": "number",
				"status": "string",
			},
		}

		p.SetSchema(schema)

		// Verify schema is set on config
		if p.config.Schema == nil {
			t.Fatal("SetSchema() did not set schema on config")
		}
		if !p.config.Schema.HasField("amount") {
			t.Error("SetSchema() schema should have field 'amount'")
		}
		if p.config.Schema.HasField("nonexistent") {
			t.Error("SetSchema() schema should not have field 'nonexistent'")
		}
	})

	t.Run("schema affects field validation in operators", func(t *testing.T) {
		config := operators.NewOperatorConfig(dialect.DialectBigQuery, nil)
		p := NewParser(config)

		schema := &mockSchemaProvider{
			fields: map[string]string{
				"amount": "number",
			},
		}

		p.SetSchema(schema)

		// Querying a valid field should work
		result, err := p.Parse(map[string]interface{}{
			">": []interface{}{map[string]interface{}{"var": "amount"}, 100},
		})
		if err != nil {
			t.Fatalf("Parse() unexpected error for valid field: %v", err)
		}
		if result != "amount > 100" {
			t.Errorf("Parse() = %q, expected %q", result, "amount > 100")
		}
	})

	t.Run("set schema to nil clears schema", func(t *testing.T) {
		p := NewParser(nil)

		schema := &mockSchemaProvider{
			fields: map[string]string{"amount": "number"},
		}
		p.SetSchema(schema)
		if p.config.Schema == nil {
			t.Fatal("SetSchema() should have set schema")
		}

		p.SetSchema(nil)
		if p.config.Schema != nil {
			t.Error("SetSchema(nil) should have cleared schema")
		}
	})
}

// --- Tests for wrapOperatorError (33.3% -> higher coverage) ---

func TestParser_wrapOperatorError(t *testing.T) {
	p := NewParser(nil)

	t.Run("nil error returns nil", func(t *testing.T) {
		result := p.wrapOperatorError("==", "$", nil)
		if result != nil {
			t.Errorf("wrapOperatorError(nil) = %v, expected nil", result)
		}
	})

	t.Run("TranspileError passes through unchanged", func(t *testing.T) {
		original := tperrors.New(tperrors.ErrInsufficientArgs, ">", "$.>", "not enough args")
		result := p.wrapOperatorError(">", "$.>", original)
		// Verify it's the same error by checking it matches with errors.Is
		if !errors.Is(result, original) {
			t.Error("wrapOperatorError() should pass through TranspileError unchanged")
		}
		var tpErr *tperrors.TranspileError
		if !errors.As(result, &tpErr) {
			t.Error("result should be a TranspileError")
		}
		if tpErr.Code != tperrors.ErrInsufficientArgs {
			t.Errorf("code = %q, expected %q", tpErr.Code, tperrors.ErrInsufficientArgs)
		}
	})

	t.Run("plain error gets wrapped in TranspileError", func(t *testing.T) {
		plainErr := fmt.Errorf("something went wrong")
		result := p.wrapOperatorError("==", "$.==", plainErr)
		if result == nil {
			t.Fatal("wrapOperatorError() should not return nil for non-nil error")
		}
		var tpErr *tperrors.TranspileError
		if !errors.As(result, &tpErr) {
			t.Fatal("result should be a TranspileError")
		}
		if tpErr.Code != tperrors.ErrInvalidArgument {
			t.Errorf("code = %q, expected %q", tpErr.Code, tperrors.ErrInvalidArgument)
		}
		if tpErr.Operator != "==" {
			t.Errorf("operator = %q, expected %q", tpErr.Operator, "==")
		}
		if tpErr.Path != "$.==" {
			t.Errorf("path = %q, expected %q", tpErr.Path, "$.==")
		}
		if !errors.Is(tpErr.Cause, plainErr) {
			t.Error("cause should be the original error")
		}
	})
}

// --- Tests for parseOperator additional branches (59.3% -> higher) ---

func TestParser_parseOperator_AdditionalBranches(t *testing.T) {
	p := NewParser(nil)

	tests := []struct {
		name     string
		operator string
		args     interface{}
		expected string
		hasError bool
	}{
		// missing operator
		{
			name:     "missing operator with string arg",
			operator: "missing",
			args:     "fieldName",
			expected: "fieldName IS NULL",
			hasError: false,
		},
		// missing_some with valid array args
		{
			name:     "missing_some with valid args",
			operator: "missing_some",
			args:     []interface{}{1, []interface{}{"f1", "f2"}},
			expected: "(f1 IS NULL OR f2 IS NULL)",
			hasError: false,
		},
		// missing_some with non-array args
		{
			name:     "missing_some with non-array args",
			operator: "missing_some",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		// Numeric operators with non-array args
		{
			name:     "addition with non-array args",
			operator: "+",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "subtraction with non-array args",
			operator: "-",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "multiplication with non-array args",
			operator: "*",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "division with non-array args",
			operator: "/",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "modulo with non-array args",
			operator: "%",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "max with non-array args",
			operator: "max",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "min with non-array args",
			operator: "min",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		// Array operators with non-array args
		{
			name:     "map with non-array args",
			operator: "map",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "filter with non-array args",
			operator: "filter",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "reduce with non-array args",
			operator: "reduce",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "all with non-array args",
			operator: "all",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "some with non-array args",
			operator: "some",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "none with non-array args",
			operator: "none",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "merge with non-array args",
			operator: "merge",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		// String operators with non-array args
		{
			name:     "cat with non-array args",
			operator: "cat",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		{
			name:     "substr with non-array args",
			operator: "substr",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		// String operators with valid array args
		{
			name:     "cat with valid args",
			operator: "cat",
			args:     []interface{}{map[string]interface{}{"var": "first"}, " ", map[string]interface{}{"var": "last"}},
			expected: "CONCAT(first, ' ', last)",
			hasError: false,
		},
		{
			name:     "substr with valid args",
			operator: "substr",
			args:     []interface{}{map[string]interface{}{"var": "name"}, 0, 5},
			expected: "SUBSTR(name, 1, 5)",
			hasError: false,
		},
		// Unary operators (! and !!) with non-array args
		{
			name:     "not with non-array single expression arg",
			operator: "!",
			args:     map[string]interface{}{"var": "active"},
			expected: "NOT (active)",
			hasError: false,
		},
		{
			name:     "double-bang with non-array single expression arg",
			operator: "!!",
			args:     map[string]interface{}{"var": "name"},
			expected: "(name IS NOT NULL AND name != FALSE AND name != 0 AND name != '')",
			hasError: false,
		},
		// Strict equality / inequality
		{
			name:     "strict equality",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "status"}, "active"},
			expected: "status = 'active'",
			hasError: false,
		},
		{
			name:     "strict inequality",
			operator: "!==",
			args:     []interface{}{map[string]interface{}{"var": "status"}, "inactive"},
			expected: "status <> 'inactive'",
			hasError: false,
		},
		// Numeric operators with valid array args
		{
			name:     "subtraction operator",
			operator: "-",
			args:     []interface{}{map[string]interface{}{"var": "total"}, 10},
			expected: "(total - 10)",
			hasError: false,
		},
		{
			name:     "division operator",
			operator: "/",
			args:     []interface{}{map[string]interface{}{"var": "total"}, 2},
			expected: "(total / 2)",
			hasError: false,
		},
		{
			name:     "modulo operator",
			operator: "%",
			args:     []interface{}{map[string]interface{}{"var": "count"}, 3},
			expected: "(count % 3)",
			hasError: false,
		},
		{
			name:     "min operator",
			operator: "min",
			args:     []interface{}{5, 10, 3},
			expected: "LEAST(5, 10, 3)",
			hasError: false,
		},
		// less than or equal
		{
			name:     "less than or equal",
			operator: "<=",
			args:     []interface{}{map[string]interface{}{"var": "score"}, 100},
			expected: "score <= 100",
			hasError: false,
		},
		// less than
		{
			name:     "less than",
			operator: "<",
			args:     []interface{}{map[string]interface{}{"var": "age"}, 21},
			expected: "age < 21",
			hasError: false,
		},
		// greater than or equal
		{
			name:     "greater than or equal",
			operator: ">=",
			args:     []interface{}{map[string]interface{}{"var": "priority"}, 5},
			expected: "priority >= 5",
			hasError: false,
		},
		// if operator
		{
			name:     "if operator with array args",
			operator: "if",
			args:     []interface{}{map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "age"}, 18}}, "adult", "minor"},
			expected: "CASE WHEN age > 18 THEN 'adult' ELSE 'minor' END",
			hasError: false,
		},
		// if operator with non-array args
		{
			name:     "if with non-array args",
			operator: "if",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		// or with non-array args
		{
			name:     "or with non-array args",
			operator: "or",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		// in with non-array args
		{
			name:     "in with non-array args",
			operator: "in",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
		// equality with non-array args
		{
			name:     "equality with non-array args",
			operator: "==",
			args:     "not-array",
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.parseOperator(tt.operator, tt.args, "$")

			if tt.hasError {
				if err == nil {
					t.Errorf("parseOperator(%q) expected error, got nil", tt.operator)
				}
			} else {
				if err != nil {
					t.Errorf("parseOperator(%q) unexpected error = %v", tt.operator, err)
				}
				if result != tt.expected {
					t.Errorf("parseOperator(%q) = %q, expected %q", tt.operator, result, tt.expected)
				}
			}
		})
	}
}

// --- Tests for custom operator flow (processCustomOperatorArgs, processArgToSQL) ---

func TestParser_CustomOperatorFlow(t *testing.T) {
	t.Run("custom operator with var arg processes to SQL", func(t *testing.T) {
		p := NewParser(nil)

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "length" {
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return fmt.Sprintf("LENGTH(%s)", args[0]), nil
					},
				}, true
			}
			return nil, false
		})

		result, err := p.Parse(map[string]interface{}{
			"length": []interface{}{map[string]interface{}{"var": "name"}},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "LENGTH(name)"
		if result != expected {
			t.Errorf("got %q, expected %q", result, expected)
		}
	})

	t.Run("custom operator with literal string arg", func(t *testing.T) {
		p := NewParser(nil)

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "repeat" {
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return fmt.Sprintf("REPEAT(%s, %s)", args[0], args[1]), nil
					},
				}, true
			}
			return nil, false
		})

		result, err := p.Parse(map[string]interface{}{
			"repeat": []interface{}{map[string]interface{}{"var": "name"}, "hello"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "REPEAT(name, 'hello')"
		if result != expected {
			t.Errorf("got %q, expected %q", result, expected)
		}
	})

	t.Run("custom operator with literal bool arg", func(t *testing.T) {
		p := NewParser(nil)

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "flagCheck" {
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return fmt.Sprintf("FLAG(%s, %s)", args[0], args[1]), nil
					},
				}, true
			}
			return nil, false
		})

		result, err := p.Parse(map[string]interface{}{
			"flagCheck": []interface{}{map[string]interface{}{"var": "field"}, true},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "FLAG(field, TRUE)"
		if result != expected {
			t.Errorf("got %q, expected %q", result, expected)
		}
	})

	t.Run("custom operator with literal false arg", func(t *testing.T) {
		p := NewParser(nil)

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "flagCheck" {
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return fmt.Sprintf("FLAG(%s, %s)", args[0], args[1]), nil
					},
				}, true
			}
			return nil, false
		})

		result, err := p.Parse(map[string]interface{}{
			"flagCheck": []interface{}{map[string]interface{}{"var": "field"}, false},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "FLAG(field, FALSE)"
		if result != expected {
			t.Errorf("got %q, expected %q", result, expected)
		}
	})

	t.Run("custom operator with nil arg", func(t *testing.T) {
		p := NewParser(nil)

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "nullCheck" {
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return fmt.Sprintf("CHECK(%s, %s)", args[0], args[1]), nil
					},
				}, true
			}
			return nil, false
		})

		result, err := p.Parse(map[string]interface{}{
			"nullCheck": []interface{}{map[string]interface{}{"var": "field"}, nil},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "CHECK(field, NULL)"
		if result != expected {
			t.Errorf("got %q, expected %q", result, expected)
		}
	})

	t.Run("custom operator with numeric arg", func(t *testing.T) {
		p := NewParser(nil)

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "power" {
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return fmt.Sprintf("POWER(%s, %s)", args[0], args[1]), nil
					},
				}, true
			}
			return nil, false
		})

		result, err := p.Parse(map[string]interface{}{
			"power": []interface{}{map[string]interface{}{"var": "base"}, 3.14},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "POWER(base, 3.14)"
		if result != expected {
			t.Errorf("got %q, expected %q", result, expected)
		}
	})

	t.Run("custom operator returning error", func(t *testing.T) {
		p := NewParser(nil)

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "failing" {
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return "", fmt.Errorf("intentional failure")
					},
				}, true
			}
			return nil, false
		})

		_, err := p.Parse(map[string]interface{}{
			"failing": []interface{}{map[string]interface{}{"var": "x"}},
		})
		if err == nil {
			t.Fatal("expected error from failing custom operator")
		}
		var tpErr *tperrors.TranspileError
		if !errors.As(err, &tpErr) {
			t.Fatalf("expected TranspileError, got %T", err)
		}
		if tpErr.Code != tperrors.ErrCustomOperatorFailed {
			t.Errorf("error code = %q, expected %q", tpErr.Code, tperrors.ErrCustomOperatorFailed)
		}
	})

	t.Run("custom operator with nested custom operator in args", func(t *testing.T) {
		p := NewParser(nil)

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			switch operatorName {
			case "toLower":
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return fmt.Sprintf("LOWER(%s)", args[0]), nil
					},
				}, true
			case "length":
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return fmt.Sprintf("LENGTH(%s)", args[0]), nil
					},
				}, true
			}
			return nil, false
		})

		// {"length": [{"toLower": [{"var": "name"}]}]}
		result, err := p.Parse(map[string]interface{}{
			"length": []interface{}{
				map[string]interface{}{
					"toLower": []interface{}{map[string]interface{}{"var": "name"}},
				},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "LENGTH(LOWER(name))"
		if result != expected {
			t.Errorf("got %q, expected %q", result, expected)
		}
	})

	t.Run("custom operator with single non-array arg (processCustomOperatorArgs single path)", func(t *testing.T) {
		p := NewParser(nil)

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "stringify" {
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return fmt.Sprintf("CAST(%s AS STRING)", args[0]), nil
					},
				}, true
			}
			return nil, false
		})

		// Single non-array argument: {"stringify": {"var": "count"}}
		result, err := p.Parse(map[string]interface{}{
			"stringify": map[string]interface{}{"var": "count"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "CAST(count AS STRING)"
		if result != expected {
			t.Errorf("got %q, expected %q", result, expected)
		}
	})

	t.Run("custom operator with primitive single arg", func(t *testing.T) {
		p := NewParser(nil)

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "literal" {
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return fmt.Sprintf("LITERAL(%s)", args[0]), nil
					},
				}, true
			}
			return nil, false
		})

		// Single primitive arg: {"literal": "hello"}
		result, err := p.Parse(map[string]interface{}{
			"literal": "hello",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "LITERAL('hello')"
		if result != expected {
			t.Errorf("got %q, expected %q", result, expected)
		}
	})

	t.Run("custom operator with arg processing error", func(t *testing.T) {
		p := NewParser(nil)

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "myop" {
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return "OK", nil
					},
				}, true
			}
			return nil, false
		})

		// Nested invalid expression in args: unsupported operator inside custom op args
		_, err := p.Parse(map[string]interface{}{
			"myop": []interface{}{
				map[string]interface{}{"unknownBuiltIn": []interface{}{1}},
			},
		})
		if err == nil {
			t.Fatal("expected error for invalid nested expression in custom operator args")
		}
	})
}

// --- Tests for primitiveToSQL (0% coverage) ---

func TestParser_primitiveToSQL(t *testing.T) {
	p := NewParser(nil)

	tests := []struct {
		name     string
		input    interface{}
		expected interface{}
	}{
		{
			name:     "string value",
			input:    "hello",
			expected: "'hello'",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "''",
		},
		{
			name:     "bool true",
			input:    true,
			expected: "TRUE",
		},
		{
			name:     "bool false",
			input:    false,
			expected: "FALSE",
		},
		{
			name:     "nil value",
			input:    nil,
			expected: "NULL",
		},
		{
			name:     "integer value",
			input:    42,
			expected: "42",
		},
		{
			name:     "float value",
			input:    3.14,
			expected: "3.14",
		},
		{
			name:     "negative number",
			input:    -7,
			expected: "-7",
		},
		{
			name:     "zero",
			input:    0,
			expected: "0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.primitiveToSQL(tt.input)
			if result != tt.expected {
				t.Errorf("primitiveToSQL(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// --- Tests for processArg additional branches (66.7% -> higher) ---

func TestParser_processArg_AdditionalBranches(t *testing.T) {
	t.Run("multi-key map is returned as-is", func(t *testing.T) {
		p := NewParser(nil)
		multiKeyMap := map[string]interface{}{"a": 1, "b": 2}
		result, err := p.processArg(multiKeyMap, "$", 0)
		if err != nil {
			t.Fatalf("processArg() unexpected error: %v", err)
		}
		if resultMap, ok := result.(map[string]interface{}); !ok {
			t.Errorf("processArg() returned %T, expected map[string]interface{}", result)
		} else {
			if len(resultMap) != 2 {
				t.Errorf("processArg() returned map with %d keys, expected 2", len(resultMap))
			}
		}
	})

	t.Run("array arg is processed recursively", func(t *testing.T) {
		p := NewParser(nil)
		arrArg := []interface{}{1, "hello", true}
		result, err := p.processArg(arrArg, "$", 0)
		if err != nil {
			t.Fatalf("processArg() unexpected error: %v", err)
		}
		resultArr, ok := result.([]interface{})
		if !ok {
			t.Fatalf("processArg() returned %T, expected []interface{}", result)
		}
		if len(resultArr) != 3 {
			t.Errorf("processArg() returned array with %d elements, expected 3", len(resultArr))
		}
	})

	t.Run("primitive arg is returned as-is", func(t *testing.T) {
		p := NewParser(nil)
		result, err := p.processArg(42, "$", 0)
		if err != nil {
			t.Fatalf("processArg() unexpected error: %v", err)
		}
		if result != 42 {
			t.Errorf("processArg(42) = %v, expected 42", result)
		}
	})

	t.Run("custom operator in nested expression gets parsed to SQL", func(t *testing.T) {
		p := NewParser(nil)

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "toLower" {
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return fmt.Sprintf("LOWER(%s)", args[0]), nil
					},
				}, true
			}
			return nil, false
		})

		// Process a custom operator expression through processArg
		arg := map[string]interface{}{
			"toLower": []interface{}{map[string]interface{}{"var": "name"}},
		}
		result, err := p.processArg(arg, "$", 0)
		if err != nil {
			t.Fatalf("processArg() unexpected error: %v", err)
		}

		// Should be a ProcessedValue (SQLResult)
		pv, ok := result.(operators.ProcessedValue)
		if !ok {
			t.Fatalf("processArg() returned %T, expected operators.ProcessedValue", result)
		}
		if !pv.IsSQL {
			t.Error("processArg() returned ProcessedValue with IsSQL=false, expected true")
		}
		if pv.Value != "LOWER(name)" {
			t.Errorf("processArg() SQL = %q, expected %q", pv.Value, "LOWER(name)")
		}
	})

	t.Run("built-in operator with nested custom operator gets processed", func(t *testing.T) {
		p := NewParser(nil)

		p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
			if operatorName == "toUpper" {
				return &mockCustomHandler{
					toSQL: func(op string, args []interface{}) (string, error) {
						return fmt.Sprintf("UPPER(%s)", args[0]), nil
					},
				}, true
			}
			return nil, false
		})

		// {"==": [{"toUpper": [{"var": "name"}]}, "JOHN"]} -- the == arg with nested custom op
		arg := map[string]interface{}{
			"==": []interface{}{
				map[string]interface{}{"toUpper": []interface{}{map[string]interface{}{"var": "name"}}},
				"JOHN",
			},
		}
		result, err := p.processArg(arg, "$", 0)
		if err != nil {
			t.Fatalf("processArg() unexpected error: %v", err)
		}

		// Should be a map with the built-in operator but processed args
		resultMap, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("processArg() returned %T, expected map[string]interface{}", result)
		}
		if _, exists := resultMap["=="]; !exists {
			t.Error("processArg() result should contain '==' key")
		}
	})
}

// --- Tests for parseExpression with invalid type ---

func TestParser_parseExpression_InvalidType(t *testing.T) {
	p := NewParser(nil)

	// Test with a type that is not primitive, not array, and not map
	// A channel satisfies this
	ch := make(chan int)
	_, err := p.parseExpression(ch, "$")
	if err == nil {
		t.Fatal("parseExpression() expected error for channel type")
	}
	var tpErr *tperrors.TranspileError
	if !errors.As(err, &tpErr) {
		t.Fatalf("expected TranspileError, got %T: %v", err, err)
	}
	if tpErr.Code != tperrors.ErrInvalidExpression {
		t.Errorf("error code = %q, expected %q", tpErr.Code, tperrors.ErrInvalidExpression)
	}
}

// --- Tests for custom operator integrated in comparison context ---

func TestParser_CustomOperatorInComparison(t *testing.T) {
	p := NewParser(nil)

	p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
		if operatorName == "length" {
			return &mockCustomHandler{
				toSQL: func(op string, args []interface{}) (string, error) {
					return fmt.Sprintf("LENGTH(%s)", args[0]), nil
				},
			}, true
		}
		return nil, false
	})

	// {">" : [{"length": [{"var": "name"}]}, 5]}
	result, err := p.Parse(map[string]interface{}{
		">": []interface{}{
			map[string]interface{}{
				"length": []interface{}{map[string]interface{}{"var": "name"}},
			},
			5,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "LENGTH(name) > 5"
	if result != expected {
		t.Errorf("got %q, expected %q", result, expected)
	}
}

// --- Tests for custom operator in logical context ---

func TestParser_CustomOperatorInLogicalContext(t *testing.T) {
	p := NewParser(nil)

	p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
		switch operatorName {
		case "toLower":
			return &mockCustomHandler{
				toSQL: func(op string, args []interface{}) (string, error) {
					return fmt.Sprintf("LOWER(%s)", args[0]), nil
				},
			}, true
		case "length":
			return &mockCustomHandler{
				toSQL: func(op string, args []interface{}) (string, error) {
					return fmt.Sprintf("LENGTH(%s)", args[0]), nil
				},
			}, true
		}
		return nil, false
	})

	// {"and": [{">": [{"length": [{"var": "name"}]}, 3]}, {"==": [{"toLower": [{"var": "status"}]}, "active"]}]}
	result, err := p.Parse(map[string]interface{}{
		"and": []interface{}{
			map[string]interface{}{
				">": []interface{}{
					map[string]interface{}{"length": []interface{}{map[string]interface{}{"var": "name"}}},
					3,
				},
			},
			map[string]interface{}{
				"==": []interface{}{
					map[string]interface{}{"toLower": []interface{}{map[string]interface{}{"var": "status"}}},
					"active",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "(LENGTH(name) > 3 AND LOWER(status) = 'active')"
	if result != expected {
		t.Errorf("got %q, expected %q", result, expected)
	}
}

// --- Tests for custom operator in unary (!) context with non-array arg ---

func TestParser_CustomOperatorInUnaryContext(t *testing.T) {
	p := NewParser(nil)

	p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
		if operatorName == "isEmpty" {
			return &mockCustomHandler{
				toSQL: func(op string, args []interface{}) (string, error) {
					return fmt.Sprintf("(%s IS NULL OR %s = '')", args[0], args[0]), nil
				},
			}, true
		}
		return nil, false
	})

	// {"!": [{"isEmpty": [{"var": "name"}]}]}
	result, err := p.Parse(map[string]interface{}{
		"!": []interface{}{
			map[string]interface{}{
				"isEmpty": []interface{}{map[string]interface{}{"var": "name"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "NOT ((name IS NULL OR name = ''))"
	if result != expected {
		t.Errorf("got %q, expected %q", result, expected)
	}
}

// --- Tests for ParseCondition with custom operator ---

func TestParser_ParseCondition_WithCustomOperator(t *testing.T) {
	p := NewParser(nil)

	p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
		if operatorName == "length" {
			return &mockCustomHandler{
				toSQL: func(op string, args []interface{}) (string, error) {
					return fmt.Sprintf("LENGTH(%s)", args[0]), nil
				},
			}, true
		}
		return nil, false
	})

	result, err := p.ParseCondition(map[string]interface{}{
		">": []interface{}{
			map[string]interface{}{"length": []interface{}{map[string]interface{}{"var": "name"}}},
			5,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "LENGTH(name) > 5"
	if result != expected {
		t.Errorf("got %q, expected %q", result, expected)
	}
	if strings.HasPrefix(result, "WHERE ") {
		t.Error("ParseCondition should not have WHERE prefix")
	}
}

// --- Tests for NewParser with explicit config ---

func TestNewParser_WithExplicitConfig(t *testing.T) {
	config := operators.NewOperatorConfig(dialect.DialectPostgreSQL, nil)
	p := NewParser(config)
	if p == nil {
		t.Fatal("NewParser() with config returned nil")
		return
	}
	if p.config.GetDialect() != dialect.DialectPostgreSQL {
		t.Errorf("dialect = %v, expected PostgreSQL", p.config.GetDialect())
	}
}

// --- Tests for isBuiltInOperator ---

func TestParser_isBuiltInOperator(t *testing.T) {
	p := NewParser(nil)

	builtInOps := []string{
		"var", "missing", "missing_some",
		"==", "===", "!=", "!==", ">", ">=", "<", "<=", "in",
		"and", "or", "!", "!!", "if",
		"+", "-", "*", "/", "%", "max", "min",
		"cat", "substr",
		"map", "filter", "reduce", "all", "some", "none", "merge",
	}

	for _, op := range builtInOps {
		if !p.isBuiltInOperator(op) {
			t.Errorf("isBuiltInOperator(%q) = false, expected true", op)
		}
	}

	nonBuiltIn := []string{"length", "toLower", "customOp", "unknownOp"}
	for _, op := range nonBuiltIn {
		if p.isBuiltInOperator(op) {
			t.Errorf("isBuiltInOperator(%q) = true, expected false", op)
		}
	}
}

// assertQueryParams compares collected bind parameters by Name and Value.
func assertQueryParams(t *testing.T, got, want []params.QueryParam) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("params length: got %d, want %d (got=%+v)", len(got), len(want), got)
	}
	for i := range got {
		if got[i].Name != want[i].Name {
			t.Errorf("param[%d].Name: got %q, want %q", i, got[i].Name, want[i].Name)
		}
		if got[i].Value != want[i].Value {
			t.Errorf("param[%d].Value: got %v (%T), want %v (%T)", i, got[i].Value, got[i].Value, want[i].Value, want[i].Value)
		}
	}
}

func TestParser_ParseParameterized(t *testing.T) {
	p := NewParser(operators.NewOperatorConfig(dialect.DialectBigQuery, nil))

	tests := []struct {
		name       string
		input      interface{}
		wantSQL    string
		wantParams []params.QueryParam
	}{
		{
			name: "simple equality",
			input: map[string]interface{}{
				"==": []interface{}{map[string]interface{}{"var": "email"}, "alice"},
			},
			wantSQL:    "email = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "alice"}},
		},
		{
			name: "greater than",
			input: map[string]interface{}{
				">": []interface{}{map[string]interface{}{"var": "age"}, float64(18)},
			},
			wantSQL:    "age > @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: float64(18)}},
		},
		{
			name: "in array",
			input: map[string]interface{}{
				"in": []interface{}{
					map[string]interface{}{"var": "x"},
					[]interface{}{float64(1), float64(2)},
				},
			},
			wantSQL: "x IN (@p1, @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
			},
		},
		{
			name: "and with two conditions",
			input: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{
						"==": []interface{}{map[string]interface{}{"var": "status"}, "active"},
					},
					map[string]interface{}{
						">": []interface{}{map[string]interface{}{"var": "age"}, float64(18)},
					},
				},
			},
			wantSQL: "(status = @p1 AND age > @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "active"},
				{Name: "p2", Value: float64(18)},
			},
		},
		{
			name: "null comparison",
			input: map[string]interface{}{
				"==": []interface{}{map[string]interface{}{"var": "field"}, nil},
			},
			wantSQL:    "field IS NULL",
			wantParams: nil,
		},
		{
			name: "boolean comparison",
			input: map[string]interface{}{
				"==": []interface{}{map[string]interface{}{"var": "active"}, true},
			},
			wantSQL:    "active = TRUE",
			wantParams: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSQL, gotParams, err := p.ParseParameterized(tt.input)
			if err != nil {
				t.Fatalf("ParseParameterized: unexpected error: %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("SQL: got %q, want %q", gotSQL, tt.wantSQL)
			}
			assertQueryParams(t, gotParams, tt.wantParams)
		})
	}
}

func TestParser_ParseParameterized_PostgreSQL(t *testing.T) {
	p := NewParser(operators.NewOperatorConfig(dialect.DialectPostgreSQL, nil))

	tests := []struct {
		name       string
		input      interface{}
		wantSQL    string
		wantParams []params.QueryParam
	}{
		{
			name: "simple equality",
			input: map[string]interface{}{
				"==": []interface{}{map[string]interface{}{"var": "email"}, "alice"},
			},
			wantSQL:    "email = $1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "alice"}},
		},
		{
			name: "greater than",
			input: map[string]interface{}{
				">": []interface{}{map[string]interface{}{"var": "age"}, float64(18)},
			},
			wantSQL:    "age > $1",
			wantParams: []params.QueryParam{{Name: "p1", Value: float64(18)}},
		},
		{
			name: "in array",
			input: map[string]interface{}{
				"in": []interface{}{
					map[string]interface{}{"var": "x"},
					[]interface{}{float64(1), float64(2)},
				},
			},
			wantSQL: "x IN ($1, $2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
			},
		},
		{
			name: "and with two conditions",
			input: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{
						"==": []interface{}{map[string]interface{}{"var": "status"}, "active"},
					},
					map[string]interface{}{
						">": []interface{}{map[string]interface{}{"var": "age"}, float64(18)},
					},
				},
			},
			wantSQL: "(status = $1 AND age > $2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "active"},
				{Name: "p2", Value: float64(18)},
			},
		},
		{
			name: "null comparison",
			input: map[string]interface{}{
				"==": []interface{}{map[string]interface{}{"var": "field"}, nil},
			},
			wantSQL:    "field IS NULL",
			wantParams: nil,
		},
		{
			name: "boolean comparison",
			input: map[string]interface{}{
				"==": []interface{}{map[string]interface{}{"var": "active"}, true},
			},
			wantSQL:    "active = TRUE",
			wantParams: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotSQL, gotParams, err := p.ParseParameterized(tt.input)
			if err != nil {
				t.Fatalf("ParseParameterized: unexpected error: %v", err)
			}
			if gotSQL != tt.wantSQL {
				t.Errorf("SQL: got %q, want %q", gotSQL, tt.wantSQL)
			}
			assertQueryParams(t, gotParams, tt.wantParams)
		})
	}
}

func TestParser_ParseConditionParameterized(t *testing.T) {
	p := NewParser(operators.NewOperatorConfig(dialect.DialectBigQuery, nil))

	input := map[string]interface{}{
		"==": []interface{}{map[string]interface{}{"var": "x"}, "val"},
	}
	gotSQL, gotParams, err := p.ParseConditionParameterized(input)
	if err != nil {
		t.Fatalf("ParseConditionParameterized: unexpected error: %v", err)
	}
	if strings.HasPrefix(gotSQL, "WHERE ") {
		t.Errorf("condition must not have WHERE prefix, got %q", gotSQL)
	}
	wantSQL := "x = @p1"
	if gotSQL != wantSQL {
		t.Errorf("SQL: got %q, want %q", gotSQL, wantSQL)
	}
	assertQueryParams(t, gotParams, []params.QueryParam{{Name: "p1", Value: "val"}})
}

func TestParser_ParseParameterized_Errors(t *testing.T) {
	p := NewParser(operators.NewOperatorConfig(dialect.DialectBigQuery, nil))

	t.Run("nil logic", func(t *testing.T) {
		_, _, err := p.ParseParameterized(nil)
		if err == nil {
			t.Fatal("expected error for nil logic")
		}
	})

	t.Run("empty map", func(t *testing.T) {
		_, _, err := p.ParseParameterized(map[string]interface{}{})
		if err == nil {
			t.Fatal("expected error for empty operator map")
		}
	})

	t.Run("top level primitive", func(t *testing.T) {
		_, _, err := p.ParseParameterized(float64(42))
		if err == nil {
			t.Fatal("expected error for primitive root expression")
		}
	})

	t.Run("invalid expression type", func(t *testing.T) {
		_, _, err := p.ParseParameterized(struct{}{})
		if err == nil {
			t.Fatal("expected error for unsupported root type")
		}
	})
}

func TestParser_ParseParameterized_CustomOperator(t *testing.T) {
	p := NewParser(operators.NewOperatorConfig(dialect.DialectBigQuery, nil))

	p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
		if operatorName == "twice" {
			return &mockCustomHandler{
				toSQL: func(op string, args []interface{}) (string, error) {
					return fmt.Sprintf("(%s + %s)", args[0], args[0]), nil
				},
			}, true
		}
		return nil, false
	})

	input := map[string]interface{}{
		"==": []interface{}{
			map[string]interface{}{"twice": []interface{}{map[string]interface{}{"var": "n"}}},
			float64(2),
		},
	}
	gotSQL, gotParams, err := p.ParseParameterized(input)
	if err != nil {
		t.Fatalf("ParseParameterized: unexpected error: %v", err)
	}
	wantSQL := "(n + n) = @p1"
	if gotSQL != wantSQL {
		t.Errorf("SQL: got %q, want %q", gotSQL, wantSQL)
	}
	if !strings.Contains(gotSQL, "@p1") {
		t.Errorf("expected placeholder @p1 in SQL: %q", gotSQL)
	}
	assertQueryParams(t, gotParams, []params.QueryParam{{Name: "p1", Value: float64(2)}})
}

// TestParser_primitiveToSQLParam exercises primitiveToSQLParam indirectly: strings and
// numbers become bind params; bool and nil stay inline in SQL.
func TestParser_primitiveToSQLParam(t *testing.T) {
	p := NewParser(operators.NewOperatorConfig(dialect.DialectBigQuery, nil))

	t.Run("string literal via equality", func(t *testing.T) {
		_, gotParams, err := p.ParseParameterized(map[string]interface{}{
			"==": []interface{}{map[string]interface{}{"var": "k"}, "v"},
		})
		if err != nil {
			t.Fatal(err)
		}
		assertQueryParams(t, gotParams, []params.QueryParam{{Name: "p1", Value: "v"}})
	})

	t.Run("integer literal via equality", func(t *testing.T) {
		_, gotParams, err := p.ParseParameterized(map[string]interface{}{
			"==": []interface{}{map[string]interface{}{"var": "k"}, int64(99)},
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(gotParams) != 1 || gotParams[0].Name != "p1" {
			t.Fatalf("params: %+v", gotParams)
		}
		switch v := gotParams[0].Value.(type) {
		case int64:
			if v != 99 {
				t.Errorf("Value: got %d, want 99", v)
			}
		default:
			t.Errorf("Value type: got %T, want int64", gotParams[0].Value)
		}
	})

	t.Run("float literal via greater than", func(t *testing.T) {
		_, gotParams, err := p.ParseParameterized(map[string]interface{}{
			">": []interface{}{map[string]interface{}{"var": "k"}, float64(0.5)},
		})
		if err != nil {
			t.Fatal(err)
		}
		assertQueryParams(t, gotParams, []params.QueryParam{{Name: "p1", Value: float64(0.5)}})
	})
}
