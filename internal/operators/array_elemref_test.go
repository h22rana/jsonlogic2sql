package operators

import (
	"strings"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

type elementRefSchemaProvider struct {
	fieldOnlySchemaProvider
}

func (p *elementRefSchemaProvider) GetFieldType(fieldName string) string {
	if p.IsArrayType(fieldName) {
		return "array"
	}
	switch fieldName {
	case "currently":
		return "boolean"
	case "base", "current_balance", "amount", "score", "item_count", "items":
		return "number"
	default:
		return ""
	}
}

func (p *elementRefSchemaProvider) IsArrayType(fieldName string) bool {
	switch fieldName {
	case "data", "groups", "numbers", "orders", "records", "results", "scores", "values", "amounts", "nums", "vals":
		return true
	default:
		return false
	}
}

func (p *elementRefSchemaProvider) IsNumericType(fieldName string) bool {
	switch p.GetFieldType(fieldName) {
	case "number", "integer":
		return true
	default:
		return false
	}
}

func (p *elementRefSchemaProvider) IsBooleanType(fieldName string) bool {
	return p.GetFieldType(fieldName) == "boolean"
}

func (p *elementRefSchemaProvider) IsStringType(fieldName string) bool {
	return p.GetFieldType(fieldName) == "string"
}

func (p *elementRefSchemaProvider) HasArrayElementFields(fieldName string) bool {
	switch fieldName {
	case "data", "groups", "orders", "records", "results", "scores":
		return true
	default:
		return false
	}
}

func (p *elementRefSchemaProvider) GetArrayElementType(fieldName string) string {
	if p.HasArrayElementFields(fieldName) {
		return "object"
	}
	if p.IsArrayType(fieldName) {
		return "number"
	}
	return ""
}

// TestArrayOperator_ElementRefNoCorruption verifies that field names containing
// "item" or "current" as substrings are NOT corrupted by array-scope mapping.
func TestArrayOperator_ElementRefNoCorruption(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &elementRefSchemaProvider{})
	op := NewArrayOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []any
		contains string
		absent   string
	}{
		{
			name:     "current.current_balance correctly mapped in reduce",
			operator: "reduce",
			args: []any{
				map[string]any{"var": "orders"},
				map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current.current_balance"}}},
				0,
			},
			contains: "elem.current_balance",
			absent:   "current.current_balance",
		},
		{
			name:     "item_count not corrupted in all",
			operator: "all",
			args: []any{
				map[string]any{"var": "scores"},
				map[string]any{">": []any{map[string]any{"var": "item_count"}, 0}},
			},
			contains: "elem.item_count",
			absent:   "elem_count",
		},
		{
			name:     "items field not corrupted in filter",
			operator: "filter",
			args: []any{
				map[string]any{"var": "data"},
				map[string]any{">": []any{map[string]any{"var": "items"}, 0}},
			},
			contains: "elem.items",
		},
		{
			name:     "currently field not corrupted in some",
			operator: "some",
			args: []any{
				map[string]any{"var": "records"},
				map[string]any{"==": []any{map[string]any{"var": "currently"}, true}},
			},
			contains: "elem.currently",
		},
		{
			name:     "current.amount correctly mapped in reduce",
			operator: "reduce",
			args: []any{
				map[string]any{"var": "orders"},
				map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current.amount"}}},
				0,
			},
			contains: "elem.amount",
			absent:   "current.amount",
		},
		{
			name:     "bare score correctly mapped in all",
			operator: "all",
			args: []any{
				map[string]any{"var": "results"},
				map[string]any{">=": []any{map[string]any{"var": "score"}, 50}},
			},
			contains: "elem.score",
		},
		{
			name:     "empty var maps to element in none",
			operator: "none",
			args: []any{
				map[string]any{"var": "values"},
				map[string]any{"==": []any{map[string]any{"var": ""}, 0}},
			},
			contains: "elem = 0",
		},
		{
			name:     "empty var maps to element in map",
			operator: "map",
			args: []any{
				map[string]any{"var": "values"},
				map[string]any{"*": []any{map[string]any{"var": ""}, 2}},
			},
			contains: "(elem * 2)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.ToSQL(tt.operator, tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("expected SQL to contain %q, got: %s", tt.contains, result)
			}
			if tt.absent != "" && strings.Contains(result, tt.absent) {
				t.Errorf("expected SQL NOT to contain %q, got: %s", tt.absent, result)
			}
		})
	}
}

// TestArrayOperator_DottedIdentifierPreservation verifies that dotted identifiers
// like "account.current" and "order.item" are NOT corrupted by the post-SQL safety net.
func TestArrayOperator_DottedIdentifierPreservation(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &elementRefSchemaProvider{})
	config.SetExpressionParser(func(expr any, path string) (string, error) {
		if m, ok := expr.(map[string]interface{}); ok {
			for _, args := range m {
				if arr, ok := args.([]interface{}); ok && len(arr) > 0 {
					if s, ok := arr[0].(string); ok {
						return s, nil
					}
				}
			}
		}
		return "", nil
	})
	op := NewArrayOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []any
		contains string
		absent   string
	}{
		{
			name:     "account.current preserved in map",
			operator: "map",
			args: []any{
				map[string]any{"var": "records"},
				map[string]any{"getField": []any{"account.current"}},
			},
			contains: "account.current",
			absent:   "account.elem",
		},
		{
			name:     "order.item preserved in all",
			operator: "all",
			args: []any{
				map[string]any{"var": "orders"},
				map[string]any{">": []any{map[string]any{"getField": []any{"order.item"}}, 0}},
			},
			contains: "order.item",
			absent:   "order.elem",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.ToSQL(tt.operator, tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("expected SQL to contain %q, got: %s", tt.contains, result)
			}
			if tt.absent != "" && strings.Contains(result, tt.absent) {
				t.Errorf("expected SQL NOT to contain %q, got: %s", tt.absent, result)
			}
		})
	}
}

// TestArrayOperator_CustomOpLiteralSQL verifies that custom operators emitting
// literal legacy alias SQL are no longer rewritten after SQL generation.
func TestArrayOperator_CustomOpLiteralSQL(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &elementRefSchemaProvider{})
	config.SetExpressionParser(func(expr any, path string) (string, error) {
		if m, ok := expr.(map[string]interface{}); ok {
			for op := range m {
				switch op {
				case "emit_item":
					return "item", nil
				case "emit_current_expr":
					return "(current + 1)", nil
				case "emit_current_dot":
					return "current.price", nil
				case "emit_current_numeric_dot":
					return "current.24h", nil
				}
			}
		}
		return "", nil
	})
	op := NewArrayOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []any
		contains string
		absent   string
	}{
		{
			name:     "literal item preserved in map",
			operator: "map",
			args:     []any{map[string]any{"var": "nums"}, map[string]any{"emit_item": []any{}}},
			contains: "SELECT item FROM",
		},
		{
			name:     "literal current in expression preserved in filter",
			operator: "filter",
			args:     []any{map[string]any{"var": "vals"}, map[string]any{">": []any{map[string]any{"emit_current_expr": []any{}}, 0}}},
			contains: "(current + 1)",
		},
		{
			name:     "literal current.price preserved in some",
			operator: "some",
			args:     []any{map[string]any{"var": "nums"}, map[string]any{">": []any{map[string]any{"emit_current_dot": []any{}}, 0}}},
			contains: "current.price",
		},
		{
			name:     "literal current numeric-leading path preserved in some",
			operator: "some",
			args:     []any{map[string]any{"var": "nums"}, map[string]any{">": []any{map[string]any{"emit_current_numeric_dot": []any{}}, 0}}},
			contains: "current.24h",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.ToSQL(tt.operator, tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("expected SQL to contain %q, got: %s", tt.contains, result)
			}
			if tt.absent != "" && strings.Contains(result, tt.absent) {
				t.Errorf("expected SQL NOT to contain %q, got: %s", tt.absent, result)
			}
		})
	}
}

// TestArrayOperator_ReduceAccumulatorEdgeCases tests reduce-specific edge cases
// including initial value preservation and nested reduces.
func TestArrayOperator_ReduceAccumulatorEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		dialect  dialect.Dialect
		operator string
		args     []any
		contains string
		absent   string
		wantErr  string
	}{
		{
			name:     "standalone reduce initial current.amount preserved",
			dialect:  dialect.DialectClickHouse,
			operator: "reduce",
			args: []any{
				map[string]any{"var": "numbers"},
				map[string]any{"var": "accumulator"},
				map[string]any{"var": "current.amount"},
			},
			contains: "current.amount",
			absent:   "elem.amount",
		},
		{
			name:     "nested reduce initial rewritten by outer map",
			dialect:  dialect.DialectBigQuery,
			operator: "map",
			args: []any{
				map[string]any{"var": "groups"},
				map[string]any{
					"reduce": []any{
						map[string]any{"var": "values"},
						map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}},
						map[string]any{"var": "base"},
					},
				},
			},
			contains: "elem.base",
		},
		{
			name:     "reduce aggregate rejects non-numeric string initial",
			dialect:  dialect.DialectBigQuery,
			operator: "reduce",
			args: []any{
				map[string]any{"var": "amounts"},
				map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": "current"}}},
				"$0.00",
			},
			wantErr: "numeric reduce aggregate initial must be numeric",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewOperatorConfig(tt.dialect, &elementRefSchemaProvider{})
			op := NewArrayOperator(config)
			result, err := op.ToSQL(tt.operator, tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q (SQL %q)", err, tt.wantErr, result)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("expected SQL to contain %q, got: %s", tt.contains, result)
			}
			if tt.absent != "" && strings.Contains(result, tt.absent) {
				t.Errorf("expected SQL NOT to contain %q, got: %s", tt.absent, result)
			}
		})
	}
}

// TestArrayOperator_ClickHouseElementRefRewrite verifies element reference
// rewriting works correctly with ClickHouse-specific array syntax.
func TestArrayOperator_ClickHouseElementRefRewrite(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectClickHouse, &elementRefSchemaProvider{})
	op := NewArrayOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []any
		contains string
		absent   string
	}{
		{
			name:     "item replaced in ClickHouse arrayMap",
			operator: "map",
			args:     []any{map[string]any{"var": "numbers"}, map[string]any{"*": []any{map[string]any{"var": ""}, 2}}},
			contains: "arrayMap(elem -> (elem * 2)",
			absent:   "item",
		},
		{
			name:     "current_balance preserved in ClickHouse arrayFilter",
			operator: "filter",
			args: []any{
				map[string]any{"var": "data"},
				map[string]any{">": []any{map[string]any{"var": "current_balance"}, 0}},
			},
			contains: "elem.current_balance",
			absent:   "elem_balance",
		},
		{
			name:     "bare score mapped in ClickHouse arrayAll",
			operator: "all",
			args: []any{
				map[string]any{"var": "results"},
				map[string]any{">=": []any{map[string]any{"var": "score"}, 50}},
			},
			contains: "elem.score",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.ToSQL(tt.operator, tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("expected SQL to contain %q, got: %s", tt.contains, result)
			}
			if tt.absent != "" && strings.Contains(result, tt.absent) {
				t.Errorf("expected SQL NOT to contain %q, got: %s", tt.absent, result)
			}
		})
	}
}

// TestArrayOperator_ArrayFormVarRewrite verifies scoped defaulted var expressions.
func TestArrayOperator_ArrayFormVarRewrite(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &elementRefSchemaProvider{})
	op := NewArrayOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []any
		contains string
		absent   string
	}{
		{
			name:     "array-form current with default in reduce",
			operator: "reduce",
			args: []any{
				map[string]any{"var": "amounts"},
				map[string]any{"+": []any{map[string]any{"var": "accumulator"}, map[string]any{"var": []any{"current", 0}}}},
				0,
			},
			contains: "COALESCE(elem, 0)",
			absent:   "COALESCE(current, 0)",
		},
		{
			name:     "array-form empty var with default in all",
			operator: "all",
			args: []any{
				map[string]any{"var": "scores"},
				map[string]any{">": []any{map[string]any{"var": []any{"", 0}}, 50}},
			},
			contains: "COALESCE(elem, 0)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.ToSQL(tt.operator, tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("expected SQL to contain %q, got: %s", tt.contains, result)
			}
			if tt.absent != "" && strings.Contains(result, tt.absent) {
				t.Errorf("expected SQL NOT to contain %q, got: %s", tt.absent, result)
			}
		})
	}
}
