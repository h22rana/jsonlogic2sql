package parser

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
)

func TestParser_parseOperator(t *testing.T) {
	p := newTestParser()

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
	p := newTestParser()

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

// --- Tests for parseOperator additional branches (59.3% -> higher) ---

func TestParser_parseOperator_AdditionalBranches(t *testing.T) {
	p := newTestParser()

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
			expected: "(f1 IS NULL AND f2 IS NULL)",
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
			expected: "CONCAT(COALESCE(CAST(first AS STRING), ''), ' ', COALESCE(CAST(last AS STRING), ''))",
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
			expected: "MOD(CAST(count AS NUMERIC), CAST(3 AS NUMERIC))",
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
		p := newTestParser()

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
		p := newTestParser()

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
		p := newTestParser()

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
		p := newTestParser()

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
		p := newTestParser()

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
		p := newTestParser()

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
		p := newTestParser()

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
		p := newTestParser()

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
		p := newTestParser()

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
		p := newTestParser()

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
		p := newTestParser()

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

// --- Tests for processArg additional branches (66.7% -> higher) ---

func TestParser_processArg_AdditionalBranches(t *testing.T) {
	t.Run("multi-key map is rejected", func(t *testing.T) {
		p := newTestParser()
		multiKeyMap := map[string]interface{}{"a": 1, "b": 2}
		if _, err := p.processArg(multiKeyMap, "$", 0); !isTranspileErrorCode(err, tperrors.ErrMultipleKeys) {
			t.Fatalf("processArg() error = %v, want %s", err, tperrors.ErrMultipleKeys)
		}
	})

	t.Run("array arg is processed recursively", func(t *testing.T) {
		p := newTestParser()
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
		p := newTestParser()
		result, err := p.processArg(42, "$", 0)
		if err != nil {
			t.Fatalf("processArg() unexpected error: %v", err)
		}
		if result != 42 {
			t.Errorf("processArg(42) = %v, expected 42", result)
		}
	})

	t.Run("custom operator in nested expression gets parsed to SQL", func(t *testing.T) {
		p := newTestParser()

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
		p := newTestParser()

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

// --- Tests for custom operator integrated in comparison context ---

func TestParser_CustomOperatorInComparison(t *testing.T) {
	p := newTestParser()

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
	p := newTestParser()

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
	p := newTestParser()

	p.SetCustomOperatorLookup(func(operatorName string) (CustomOperatorHandler, bool) {
		if operatorName == "isEmpty" {
			return &mockCustomHandler{
				forcePredicate: true,
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
	expected := "NOT (name IS NULL OR name = '')"
	if result != expected {
		t.Errorf("got %q, expected %q", result, expected)
	}
}

// --- Tests for ParseCondition with custom operator ---

func TestParser_ParseCondition_WithCustomOperator(t *testing.T) {
	p := newTestParser()

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

// --- Tests for isBuiltInOperator ---

func TestParser_isBuiltInOperator(t *testing.T) {
	p := newTestParser()

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
