package parser

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
)

func TestNewParser(t *testing.T) {
	p := newTestParser()
	if p == nil {
		t.Fatal("newTestParser() returned nil")
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

func TestContainsBarePostgreSQLEmptyArrayLiteral(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want bool
	}{
		{name: "bare array", sql: "UNNEST(ARRAY[])", want: true},
		{name: "typed cast suffix", sql: "UNNEST(ARRAY[]::INT[])", want: false},
		{name: "typed CAST expression", sql: "UNNEST(CAST(ARRAY[] AS INT[]))", want: false},
		{name: "single quoted text", sql: "SELECT 'ARRAY[]'", want: false},
		{name: "escaped single quoted text", sql: "SELECT 'it''s ARRAY[]'", want: false},
		{name: "double quoted identifier", sql: `SELECT "ARRAY[]"`, want: false},
		{name: "escaped double quoted identifier", sql: `SELECT "field""ARRAY[]"`, want: false},
		{name: "backtick quoted identifier", sql: "SELECT `ARRAY[]`", want: false},
		{name: "line comment", sql: "-- ARRAY[]\nSELECT TRUE", want: false},
		{name: "block comment", sql: "/* ARRAY[] */ SELECT TRUE", want: false},
		{name: "dollar quoted literal", sql: "SELECT $tag$ ARRAY[] $tag$", want: false},
		{name: "empty tag dollar quoted literal", sql: "SELECT $$ ARRAY[] $$", want: false},
		{name: "identifier prefix", sql: "SELECT myARRAY[]", want: false},
		{name: "identifier suffix", sql: "SELECT ARRAY[]suffix", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsBarePostgreSQLEmptyArrayLiteral(tt.sql); got != tt.want {
				t.Fatalf("containsBarePostgreSQLEmptyArrayLiteral(%q) = %v, want %v", tt.sql, got, tt.want)
			}
		})
	}
}

func TestParser_Parse(t *testing.T) {
	p := newTestParser()

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
				"map": []interface{}{map[string]interface{}{"var": "numbers"}, map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": ""}, 1}}},
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

func TestParser_ParseCondition(t *testing.T) {
	p := newTestParser()

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
				// Verify it does not include a WHERE prefix.
				if strings.HasPrefix(result, "WHERE ") {
					t.Errorf("ParseCondition() should not have WHERE prefix, got %q", result)
				}
			}
		})
	}
}

func TestParser_SetCustomOperatorLookup(t *testing.T) {
	t.Run("sets custom operator lookup and validates through validator", func(t *testing.T) {
		p := newTestParser()

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
		p := newTestParser()

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
		p := newTestParser()

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

func TestParser_SetSchema(t *testing.T) {
	t.Run("sets schema on shared config", func(t *testing.T) {
		p := newTestParser()

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
		config := operators.NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlyParserSchema{})
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

	t.Run("set schema to nil installs empty schema", func(t *testing.T) {
		p := newTestParser()

		schema := &mockSchemaProvider{
			fields: map[string]string{"amount": "number"},
		}
		p.SetSchema(schema)
		if p.config.Schema == nil {
			t.Fatal("SetSchema() should have set schema")
		}

		p.SetSchema(nil)
		if p.config.Schema == nil {
			t.Fatal("SetSchema(nil) should install an empty schema provider")
		}
		if p.config.Schema.HasField("amount") {
			t.Error("SetSchema(nil) should not preserve previous schema fields")
		}
		if err := p.config.Schema.ValidateField("amount"); err == nil {
			t.Error("SetSchema(nil) empty schema should reject field access")
		}
	})
}

func TestParser_wrapOperatorError(t *testing.T) {
	p := newTestParser()

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

func TestNewParser_WithExplicitConfig(t *testing.T) {
	config := operators.NewOperatorConfig(dialect.DialectPostgreSQL, &fieldOnlyParserSchema{})
	p := NewParser(config)
	if p == nil {
		t.Fatal("NewParser() with config returned nil")
		return
	}
	if p.config.GetDialect() != dialect.DialectPostgreSQL {
		t.Errorf("dialect = %v, expected PostgreSQL", p.config.GetDialect())
	}
}
