package jsonlogic2sql

import (
	"fmt"
	"testing"
)

// LengthOperator implements OperatorHandler for LENGTH SQL function.
type LengthOperator struct{}

func (l *LengthOperator) ToSQL(operator string, args []interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("length requires exactly 1 argument, got %d", len(args))
	}
	return fmt.Sprintf("LENGTH(%s)", args[0]), nil
}

// UpperOperator implements OperatorHandler for UPPER SQL function.
type UpperOperator struct{}

func (u *UpperOperator) ToSQL(operator string, args []interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("upper requires exactly 1 argument, got %d", len(args))
	}
	return fmt.Sprintf("UPPER(%s)", args[0]), nil
}

// ConcatWithSeparatorOperator joins arguments with a separator.
type ConcatWithSeparatorOperator struct {
	Separator string
}

func (c *ConcatWithSeparatorOperator) ToSQL(operator string, args []interface{}) (string, error) {
	if len(args) < 2 {
		return "", fmt.Errorf("concat_ws requires at least 2 arguments")
	}
	result := fmt.Sprintf("%s", args[0])
	for i := 1; i < len(args); i++ {
		result += fmt.Sprintf(" || '%s' || %s", c.Separator, args[i])
	}
	return result, nil
}

func TestOperatorRegistry(t *testing.T) {
	t.Run("Register and Get", func(t *testing.T) {
		registry := NewOperatorRegistry()
		registry.Register("length", &LengthOperator{})

		handler, ok := registry.Get("length")
		if !ok {
			t.Fatal("expected to find length operator")
		}
		if handler == nil {
			t.Fatal("expected non-nil handler")
		}
	})

	t.Run("RegisterFunc", func(t *testing.T) {
		registry := NewOperatorRegistry()
		registry.RegisterFunc("custom", func(op string, args []interface{}) (string, error) {
			return "CUSTOM()", nil
		})

		handler, ok := registry.Get("custom")
		if !ok {
			t.Fatal("expected to find custom operator")
		}
		result, err := handler.ToSQL("custom", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != "CUSTOM()" {
			t.Errorf("expected CUSTOM(), got %s", result)
		}
	})

	t.Run("Has", func(t *testing.T) {
		registry := NewOperatorRegistry()
		registry.Register("length", &LengthOperator{})

		if !registry.Has("length") {
			t.Error("expected Has to return true for registered operator")
		}
		if registry.Has("nonexistent") {
			t.Error("expected Has to return false for non-registered operator")
		}
	})

	t.Run("Unregister", func(t *testing.T) {
		registry := NewOperatorRegistry()
		registry.Register("length", &LengthOperator{})

		if !registry.Unregister("length") {
			t.Error("expected Unregister to return true for registered operator")
		}
		if registry.Has("length") {
			t.Error("expected operator to be removed after Unregister")
		}
		if registry.Unregister("nonexistent") {
			t.Error("expected Unregister to return false for non-registered operator")
		}
	})

	t.Run("List", func(t *testing.T) {
		registry := NewOperatorRegistry()
		registry.Register("length", &LengthOperator{})
		registry.Register("upper", &UpperOperator{})

		list := registry.List()
		if len(list) != 2 {
			t.Errorf("expected 2 operators, got %d", len(list))
		}
	})

	t.Run("Clear", func(t *testing.T) {
		registry := NewOperatorRegistry()
		registry.Register("length", &LengthOperator{})
		registry.Register("upper", &UpperOperator{})

		registry.Clear()
		if len(registry.List()) != 0 {
			t.Error("expected registry to be empty after Clear")
		}
	})

	t.Run("Clone", func(t *testing.T) {
		registry := NewOperatorRegistry()
		registry.Register("length", &LengthOperator{})

		clone := registry.Clone()
		if !clone.Has("length") {
			t.Error("expected clone to have length operator")
		}

		// Modify original, clone should not be affected
		registry.Register("upper", &UpperOperator{})
		if clone.Has("upper") {
			t.Error("clone should not be affected by changes to original")
		}
	})

	t.Run("Merge", func(t *testing.T) {
		registry1 := NewOperatorRegistry()
		registry1.Register("length", &LengthOperator{})

		registry2 := NewOperatorRegistry()
		registry2.Register("upper", &UpperOperator{})

		registry1.Merge(registry2)
		if !registry1.Has("length") {
			t.Error("expected registry1 to still have length")
		}
		if !registry1.Has("upper") {
			t.Error("expected registry1 to have upper after merge")
		}
	})
}

func TestValidateOperatorName(t *testing.T) {
	t.Run("valid custom name", func(t *testing.T) {
		if err := validateOperatorName("length"); err != nil {
			t.Errorf("unexpected error for valid name: %v", err)
		}
	})

	t.Run("built-in operator", func(t *testing.T) {
		builtIns := []string{"var", "==", "and", "or", "+", "-", "cat", "in", "if"}
		for _, op := range builtIns {
			if err := validateOperatorName(op); err == nil {
				t.Errorf("expected error for built-in operator: %s", op)
			}
		}
	})
}

func TestTranspilerCustomOperators(t *testing.T) {
	t.Run("RegisterOperatorFunc simple", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		err := transpiler.RegisterOperatorFunc("length", func(op string, args []interface{}) (string, error) {
			if len(args) != 1 {
				return "", fmt.Errorf("length requires 1 argument")
			}
			return fmt.Sprintf("LENGTH(%s)", args[0]), nil
		})
		if err != nil {
			t.Fatalf("unexpected error registering operator: %v", err)
		}

		sql, err := transpiler.Transpile(`{"length": [{"var": "email"}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE LENGTH(email)"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("RegisterOperator with struct", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		err := transpiler.RegisterOperator("length", &LengthOperator{})
		if err != nil {
			t.Fatalf("unexpected error registering operator: %v", err)
		}

		sql, err := transpiler.Transpile(`{"length": [{"var": "name"}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE LENGTH(name)"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator with nested expression", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		err := transpiler.RegisterOperatorFunc("length", func(op string, args []interface{}) (string, error) {
			return fmt.Sprintf("LENGTH(%s)", args[0]), nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// length of concatenated string
		sql, err := transpiler.Transpile(`{"length": [{"cat": [{"var": "first"}, {"var": "last"}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE LENGTH(CONCAT(first, last))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator in comparison", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		err := transpiler.RegisterOperatorFunc("length", func(op string, args []interface{}) (string, error) {
			return fmt.Sprintf("LENGTH(%s)", args[0]), nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpiler.Transpile(`{">": [{"length": [{"var": "email"}]}, 10]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE LENGTH(email) > 10"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("upper operator", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		err := transpiler.RegisterOperator("upper", &UpperOperator{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpiler.Transpile(`{"==": [{"upper": [{"var": "name"}]}, "JOHN"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE UPPER(name) = 'JOHN'"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("multiple custom operators", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.RegisterOperator("length", &LengthOperator{})
		transpiler.RegisterOperator("upper", &UpperOperator{})

		sql, err := transpiler.Transpile(`{"and": [{">": [{"length": [{"var": "name"}]}, 5]}, {"==": [{"upper": [{"var": "status"}]}, "ACTIVE"]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE (LENGTH(name) > 5 AND UPPER(status) = 'ACTIVE')"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator with multiple args", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		err := transpiler.RegisterOperatorFunc("coalesce", func(op string, args []interface{}) (string, error) {
			result := "COALESCE("
			for i, arg := range args {
				if i > 0 {
					result += ", "
				}
				result += fmt.Sprintf("%s", arg)
			}
			result += ")"
			return result, nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpiler.Transpile(`{"coalesce": [{"var": "nickname"}, {"var": "name"}, "Unknown"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE COALESCE(nickname, name, 'Unknown')"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("reject built-in operator override", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		err := transpiler.RegisterOperatorFunc("and", func(op string, args []interface{}) (string, error) {
			return "CUSTOM_AND", nil
		})
		if err == nil {
			t.Error("expected error when trying to override built-in operator")
		}
	})

	t.Run("HasCustomOperator", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.RegisterOperator("length", &LengthOperator{})

		if !transpiler.HasCustomOperator("length") {
			t.Error("expected HasCustomOperator to return true")
		}
		if transpiler.HasCustomOperator("nonexistent") {
			t.Error("expected HasCustomOperator to return false for non-registered")
		}
	})

	t.Run("UnregisterOperator", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.RegisterOperator("length", &LengthOperator{})

		if !transpiler.UnregisterOperator("length") {
			t.Error("expected UnregisterOperator to return true")
		}

		// Now it should fail to transpile
		_, err := transpiler.Transpile(`{"length": [{"var": "email"}]}`)
		if err == nil {
			t.Error("expected error after unregistering operator")
		}
	})

	t.Run("ListCustomOperators", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.RegisterOperator("length", &LengthOperator{})
		transpiler.RegisterOperator("upper", &UpperOperator{})

		list := transpiler.ListCustomOperators()
		if len(list) != 2 {
			t.Errorf("expected 2 operators, got %d", len(list))
		}
	})

	t.Run("ClearCustomOperators", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.RegisterOperator("length", &LengthOperator{})
		transpiler.RegisterOperator("upper", &UpperOperator{})

		transpiler.ClearCustomOperators()
		if len(transpiler.ListCustomOperators()) != 0 {
			t.Error("expected no custom operators after clear")
		}
	})

	t.Run("custom operator with literal argument", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		err := transpiler.RegisterOperatorFunc("repeat", func(op string, args []interface{}) (string, error) {
			if len(args) != 2 {
				return "", fmt.Errorf("repeat requires 2 arguments")
			}
			return fmt.Sprintf("REPEAT(%s, %s)", args[0], args[1]), nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpiler.Transpile(`{"repeat": [{"var": "char"}, 5]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE REPEAT(char, 5)"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator with stateful handler", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		err := transpiler.RegisterOperator("concat_ws", &ConcatWithSeparatorOperator{Separator: ", "})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpiler.Transpile(`{"concat_ws": [{"var": "first"}, {"var": "last"}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE first || ', ' || last"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})
}

func TestCustomOperatorEdgeCases(t *testing.T) {
	t.Run("custom operator returning error", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.RegisterOperatorFunc("failing", func(op string, args []interface{}) (string, error) {
			return "", fmt.Errorf("intentional failure")
		})

		_, err := transpiler.Transpile(`{"failing": [{"var": "x"}]}`)
		if err == nil {
			t.Error("expected error from failing operator")
		}
	})

	t.Run("custom operator with no arguments", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.RegisterOperatorFunc("now", func(op string, args []interface{}) (string, error) {
			return "NOW()", nil
		})

		sql, err := transpiler.Transpile(`{"now": []}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE NOW()"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator with single non-array argument", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		transpiler.RegisterOperatorFunc("single", func(op string, args []interface{}) (string, error) {
			return fmt.Sprintf("SINGLE(%s)", args[0]), nil
		})

		// When argument is not an array, it should still work
		sql, err := transpiler.Transpile(`{"single": {"var": "x"}}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE SINGLE(x)"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})
}

// Test dialectAwareFuncHandler directly.
func TestDialectAwareFuncHandler(t *testing.T) {
	t.Run("ToSQL returns error requiring dialect", func(t *testing.T) {
		handler := &dialectAwareFuncHandler{
			fn: func(op string, args []interface{}, dialect Dialect) (string, error) {
				return "TEST()", nil
			},
		}

		_, err := handler.ToSQL("test_op", []interface{}{"arg1"})
		if err == nil {
			t.Error("expected error from ToSQL on dialectAwareFuncHandler")
		}
		expectedMsg := "operator test_op requires dialect - use ToSQLWithDialect instead"
		if err.Error() != expectedMsg {
			t.Errorf("expected error message %q, got %q", expectedMsg, err.Error())
		}
	})

	t.Run("ToSQLWithDialect delegates to wrapped function", func(t *testing.T) {
		handler := &dialectAwareFuncHandler{
			fn: func(op string, args []interface{}, dialect Dialect) (string, error) {
				return fmt.Sprintf("DIALECT_%s(%s)", dialect.String(), args[0]), nil
			},
		}

		result, err := handler.ToSQLWithDialect("test_op", []interface{}{"col"}, DialectBigQuery)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "DIALECT_BigQuery(col)"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("ToSQLWithDialect with multiple dialects", func(t *testing.T) {
		handler := &dialectAwareFuncHandler{
			fn: func(op string, args []interface{}, dialect Dialect) (string, error) {
				switch dialect {
				case DialectBigQuery:
					return "BQ_FUNC()", nil
				case DialectSpanner:
					return "SPANNER_FUNC()", nil
				case DialectPostgreSQL:
					return "PG_FUNC()", nil
				case DialectDuckDB:
					return "DUCKDB_FUNC()", nil
				case DialectClickHouse:
					return "CH_FUNC()", nil
				default:
					return "", fmt.Errorf("unsupported dialect: %s", dialect)
				}
			},
		}

		tests := []struct {
			dialect  Dialect
			expected string
		}{
			{DialectBigQuery, "BQ_FUNC()"},
			{DialectSpanner, "SPANNER_FUNC()"},
			{DialectPostgreSQL, "PG_FUNC()"},
			{DialectDuckDB, "DUCKDB_FUNC()"},
			{DialectClickHouse, "CH_FUNC()"},
		}

		for _, tt := range tests {
			result, err := handler.ToSQLWithDialect("test_op", nil, tt.dialect)
			if err != nil {
				t.Fatalf("dialect %s: unexpected error: %v", tt.dialect, err)
			}
			if result != tt.expected {
				t.Errorf("dialect %s: expected %q, got %q", tt.dialect, tt.expected, result)
			}
		}
	})

	t.Run("ToSQLWithDialect passes error from wrapped function", func(t *testing.T) {
		handler := &dialectAwareFuncHandler{
			fn: func(op string, args []interface{}, dialect Dialect) (string, error) {
				return "", fmt.Errorf("custom error from function")
			},
		}

		_, err := handler.ToSQLWithDialect("test_op", nil, DialectBigQuery)
		if err == nil {
			t.Error("expected error from wrapped function")
		}
		if err.Error() != "custom error from function" {
			t.Errorf("expected 'custom error from function', got %q", err.Error())
		}
	})
}

// DialectAwareTestHandler implements DialectAwareOperatorHandler for testing.
type DialectAwareTestHandler struct {
	prefix string
}

func (d *DialectAwareTestHandler) ToSQLWithDialect(operator string, args []interface{}, dialect Dialect) (string, error) {
	return fmt.Sprintf("%s_%s(%s)", d.prefix, dialect.String(), args[0]), nil
}

// Test dialectAwareHandlerWrapper directly.
func TestDialectAwareHandlerWrapper(t *testing.T) {
	t.Run("ToSQL uses stored dialect", func(t *testing.T) {
		handler := &DialectAwareTestHandler{prefix: "TEST"}
		wrapper := &dialectAwareHandlerWrapper{
			handler: handler,
			dialect: DialectBigQuery,
		}

		result, err := wrapper.ToSQL("my_op", []interface{}{"column"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "TEST_BigQuery(column)"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("ToSQL with different dialects", func(t *testing.T) {
		handler := &DialectAwareTestHandler{prefix: "FUNC"}

		tests := []struct {
			dialect  Dialect
			expected string
		}{
			{DialectBigQuery, "FUNC_BigQuery(arg)"},
			{DialectSpanner, "FUNC_Spanner(arg)"},
			{DialectPostgreSQL, "FUNC_PostgreSQL(arg)"},
			{DialectDuckDB, "FUNC_DuckDB(arg)"},
			{DialectClickHouse, "FUNC_ClickHouse(arg)"},
		}

		for _, tt := range tests {
			wrapper := &dialectAwareHandlerWrapper{
				handler: handler,
				dialect: tt.dialect,
			}

			result, err := wrapper.ToSQL("op", []interface{}{"arg"})
			if err != nil {
				t.Fatalf("dialect %s: unexpected error: %v", tt.dialect, err)
			}
			if result != tt.expected {
				t.Errorf("dialect %s: expected %q, got %q", tt.dialect, tt.expected, result)
			}
		}
	})
}

// Test RegisterDialectAwareFunc method on OperatorRegistry.
func TestOperatorRegistry_RegisterDialectAwareFunc(t *testing.T) {
	t.Run("register and retrieve dialect-aware function", func(t *testing.T) {
		registry := NewOperatorRegistry()
		registry.RegisterDialectAwareFunc("now", func(op string, args []interface{}, dialect Dialect) (string, error) {
			return fmt.Sprintf("NOW_%s()", dialect.String()), nil
		})

		handler, ok := registry.Get("now")
		if !ok {
			t.Fatal("expected to find 'now' operator")
		}

		// Check it implements DialectAwareOperatorHandler
		dialectHandler, ok := handler.(DialectAwareOperatorHandler)
		if !ok {
			t.Fatal("expected handler to implement DialectAwareOperatorHandler")
		}

		result, err := dialectHandler.ToSQLWithDialect("now", nil, DialectPostgreSQL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "NOW_PostgreSQL()"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("ToSQL on dialect-aware func returns error", func(t *testing.T) {
		registry := NewOperatorRegistry()
		registry.RegisterDialectAwareFunc("custom", func(op string, args []interface{}, dialect Dialect) (string, error) {
			return "CUSTOM()", nil
		})

		handler, _ := registry.Get("custom")
		_, err := handler.ToSQL("custom", nil)
		if err == nil {
			t.Error("expected error from ToSQL on dialect-aware handler")
		}
	})
}

func TestDialectAwareOperators(t *testing.T) {
	t.Run("RegisterDialectAwareOperatorFunc with BigQuery", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		err := transpiler.RegisterDialectAwareOperatorFunc("now", func(op string, args []interface{}, dialect Dialect) (string, error) {
			switch dialect {
			case DialectBigQuery:
				return "CURRENT_TIMESTAMP()", nil
			case DialectSpanner:
				return "CURRENT_TIMESTAMP()", nil
			default:
				return "", fmt.Errorf("unsupported dialect: %s", dialect)
			}
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpiler.Transpile(`{"==": [{"now": []}, "2024-01-01"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE CURRENT_TIMESTAMP() = '2024-01-01'"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("RegisterDialectAwareOperatorFunc with Spanner", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectSpanner)
		err := transpiler.RegisterDialectAwareOperatorFunc("array_length", func(op string, args []interface{}, dialect Dialect) (string, error) {
			switch dialect {
			case DialectBigQuery:
				return fmt.Sprintf("ARRAY_LENGTH(%s)", args[0]), nil
			case DialectSpanner:
				return fmt.Sprintf("ARRAY_LENGTH(%s)", args[0]), nil
			default:
				return "", fmt.Errorf("unsupported dialect: %s", dialect)
			}
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpiler.Transpile(`{">": [{"array_length": [{"var": "items"}]}, 0]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE ARRAY_LENGTH(items) > 0"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("dialect-aware operator with different output per dialect", func(t *testing.T) {
		// Define a function that returns different SQL based on dialect
		stringContainsOp := func(op string, args []interface{}, dialect Dialect) (string, error) {
			if len(args) != 2 {
				return "", fmt.Errorf("string_contains requires 2 arguments")
			}
			switch dialect {
			case DialectBigQuery:
				return fmt.Sprintf("STRPOS(%s, %s) > 0", args[0], args[1]), nil
			case DialectSpanner:
				return fmt.Sprintf("STRPOS(%s, %s) > 0", args[0], args[1]), nil
			default:
				return "", fmt.Errorf("unsupported dialect: %s", dialect)
			}
		}

		// Test with BigQuery
		bqTranspiler, _ := NewTranspiler(DialectBigQuery)
		bqTranspiler.RegisterDialectAwareOperatorFunc("string_contains", stringContainsOp)
		bqSQL, err := bqTranspiler.Transpile(`{"string_contains": [{"var": "name"}, "test"]}`)
		if err != nil {
			t.Fatalf("BigQuery: unexpected error: %v", err)
		}
		if bqSQL != "WHERE STRPOS(name, 'test') > 0" {
			t.Errorf("BigQuery: expected 'WHERE STRPOS(name, 'test') > 0', got %s", bqSQL)
		}

		// Test with Spanner
		spannerTranspiler, _ := NewTranspiler(DialectSpanner)
		spannerTranspiler.RegisterDialectAwareOperatorFunc("string_contains", stringContainsOp)
		spannerSQL, err := spannerTranspiler.Transpile(`{"string_contains": [{"var": "name"}, "test"]}`)
		if err != nil {
			t.Fatalf("Spanner: unexpected error: %v", err)
		}
		if spannerSQL != "WHERE STRPOS(name, 'test') > 0" {
			t.Errorf("Spanner: expected 'WHERE STRPOS(name, 'test') > 0', got %s", spannerSQL)
		}
	})

	t.Run("reject built-in operator override with dialect-aware", func(t *testing.T) {
		transpiler, _ := NewTranspiler(DialectBigQuery)
		err := transpiler.RegisterDialectAwareOperatorFunc("and", func(op string, args []interface{}, dialect Dialect) (string, error) {
			return "CUSTOM_AND", nil
		})
		if err == nil {
			t.Error("expected error when trying to override built-in operator with dialect-aware function")
		}
	})
}

// TestDeeplyNestedCustomOperators tests custom operators in deeply nested contexts.
func TestDeeplyNestedCustomOperators(t *testing.T) {
	// Helper to create a transpiler with common custom operators
	setupTranspiler := func(dialect Dialect) *Transpiler {
		tr, _ := NewTranspiler(dialect)
		tr.RegisterOperatorFunc("toLower", func(op string, args []interface{}) (string, error) {
			if len(args) != 1 {
				return "", fmt.Errorf("toLower requires 1 argument")
			}
			return fmt.Sprintf("LOWER(%s)", args[0]), nil
		})
		tr.RegisterOperatorFunc("toUpper", func(op string, args []interface{}) (string, error) {
			if len(args) != 1 {
				return "", fmt.Errorf("toUpper requires 1 argument")
			}
			return fmt.Sprintf("UPPER(%s)", args[0]), nil
		})
		tr.RegisterOperatorFunc("startsWith", func(op string, args []interface{}) (string, error) {
			if len(args) != 2 {
				return "", fmt.Errorf("startsWith requires 2 arguments")
			}
			return fmt.Sprintf("%s LIKE '%s%%'", args[0], args[1]), nil
		})
		tr.RegisterOperatorFunc("!startsWith", func(op string, args []interface{}) (string, error) {
			if len(args) != 2 {
				return "", fmt.Errorf("!startsWith requires 2 arguments")
			}
			return fmt.Sprintf("%s NOT LIKE '%s%%'", args[0], args[1]), nil
		})
		tr.RegisterOperatorFunc("endsWith", func(op string, args []interface{}) (string, error) {
			if len(args) != 2 {
				return "", fmt.Errorf("endsWith requires 2 arguments")
			}
			return fmt.Sprintf("%s LIKE '%%%s'", args[0], args[1]), nil
		})
		tr.RegisterOperatorFunc("!endsWith", func(op string, args []interface{}) (string, error) {
			if len(args) != 2 {
				return "", fmt.Errorf("!endsWith requires 2 arguments")
			}
			return fmt.Sprintf("%s NOT LIKE '%%%s'", args[0], args[1]), nil
		})
		tr.RegisterOperatorFunc("contains", func(op string, args []interface{}) (string, error) {
			if len(args) != 2 {
				return "", fmt.Errorf("contains requires 2 arguments")
			}
			return fmt.Sprintf("%s LIKE '%%%s%%'", args[0], args[1]), nil
		})
		tr.RegisterOperatorFunc("!contains", func(op string, args []interface{}) (string, error) {
			if len(args) != 2 {
				return "", fmt.Errorf("!contains requires 2 arguments")
			}
			return fmt.Sprintf("%s NOT LIKE '%%%s%%'", args[0], args[1]), nil
		})
		return tr
	}

	t.Run("custom operator inside cat", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"cat": [{"toLower": [{"var": "firstName"}]}, " ", {"toUpper": [{"var": "lastName"}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE CONCAT(LOWER(firstName), ' ', UPPER(lastName))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside if then/else branches", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"if": [{"==": [{"var": "type"}, "premium"]}, {"toUpper": [{"var": "name"}]}, {"toLower": [{"var": "name"}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE CASE WHEN type = 'premium' THEN UPPER(name) ELSE LOWER(name) END"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside and/or", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"and": [{"startsWith": [{"var": "name"}, "A"]}, {"or": [{"endsWith": [{"var": "email"}, "@company.com"]}, {"!contains": [{"var": "desc"}, "spam"]}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Note: string literals come pre-quoted from the parser
		expected := "WHERE (name LIKE ''A'%' AND (email LIKE '%'@company.com'' OR desc NOT LIKE '%'spam'%'))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside all array operator", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"all": [{"var": "tags"}, {"!contains": [{"var": "item"}, "spam"]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Note: string literals come pre-quoted from the parser
		expected := "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem NOT LIKE '%'spam'%'))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside some array operator", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"some": [{"var": "emails"}, {"endsWith": [{"var": "item"}, "@company.com"]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Note: string literals come pre-quoted from the parser
		expected := "WHERE EXISTS (SELECT 1 FROM UNNEST(emails) AS elem WHERE elem LIKE '%'@company.com'')"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside none array operator", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"none": [{"var": "names"}, {"startsWith": [{"var": "item"}, "Bot"]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Note: string literals come pre-quoted from the parser
		expected := "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(names) AS elem WHERE elem LIKE ''Bot'%')"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside filter array operator", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"filter": [{"var": "users"}, {"and": [{"!startsWith": [{"var": "item.name"}, "Test"]}, {"!endsWith": [{"var": "item.email"}, "@temp.com"]}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Note: string literals come pre-quoted from the parser
		expected := "WHERE ARRAY(SELECT elem FROM UNNEST(users) AS elem WHERE (elem.name NOT LIKE ''Test'%' AND elem.email NOT LIKE '%'@temp.com''))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside map array operator", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"map": [{"var": "names"}, {"toLower": [{"var": "item"}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE ARRAY(SELECT LOWER(elem) FROM UNNEST(names) AS elem)"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside reduce array operator", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"reduce": [{"var": "items"}, {"cat": [{"var": "accumulator"}, {"toUpper": [{"var": "current"}]}]}, ""]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE (SELECT CONCAT('', UPPER(elem)) FROM UNNEST(items) AS elem)"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("deeply nested: and with all containing custom operators", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"and": [{"all": [{"var": "tags"}, {"!contains": [{"var": "item"}, "spam"]}]}, {"some": [{"var": "emails"}, {"endsWith": [{"var": "item"}, "@valid.com"]}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Note: string literals come pre-quoted from the parser
		expected := "WHERE (NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem NOT LIKE '%'spam'%')) AND EXISTS (SELECT 1 FROM UNNEST(emails) AS elem WHERE elem LIKE '%'@valid.com''))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("deeply nested: or with none containing custom operators", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"or": [{"none": [{"var": "names"}, {"startsWith": [{"var": "item"}, "Bot"]}]}, {"all": [{"var": "scores"}, {">": [{"var": "item"}, 50]}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Note: string literals come pre-quoted from the parser
		expected := "WHERE (NOT EXISTS (SELECT 1 FROM UNNEST(names) AS elem WHERE elem LIKE ''Bot'%') OR NOT EXISTS (SELECT 1 FROM UNNEST(scores) AS elem WHERE NOT (elem > 50)))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("triple nested: and with or containing all/some/none", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"and": [{"or": [{"all": [{"var": "tags"}, {"!contains": [{"var": "item"}, "spam"]}]}, {"none": [{"var": "emails"}, {"startsWith": [{"var": "item"}, "blocked_"]}]}]}, {"some": [{"var": "scores"}, {">": [{"var": "item"}, 100]}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Note: string literals come pre-quoted from the parser
		expected := "WHERE ((NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem NOT LIKE '%'spam'%')) OR NOT EXISTS (SELECT 1 FROM UNNEST(emails) AS elem WHERE elem LIKE ''blocked_'%')) AND EXISTS (SELECT 1 FROM UNNEST(scores) AS elem WHERE elem > 100))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("filter with nested and/or and multiple custom operators", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"filter": [{"var": "transactions"}, {"and": [{"!startsWith": [{"var": "item.name"}, "VOID"]}, {"!endsWith": [{"var": "item.category"}, "_canceled"]}, {"!contains": [{"var": "item.email"}, "spam"]}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Note: string literals come pre-quoted from the parser
		expected := "WHERE ARRAY(SELECT elem FROM UNNEST(transactions) AS elem WHERE (elem.name NOT LIKE ''VOID'%' AND elem.category NOT LIKE '%'_canceled'' AND elem.email NOT LIKE '%'spam'%'))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("if with all condition in then branch", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"if": [{"all": [{"var": "scores"}, {">": [{"var": "item"}, 50]}]}, {"var": "status"}, "FAILED"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE CASE WHEN NOT EXISTS (SELECT 1 FROM UNNEST(scores) AS elem WHERE NOT (elem > 50)) THEN status ELSE 'FAILED' END"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("comparison with custom operator result", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"==": [{"toLower": [{"var": "status"}]}, "active"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE LOWER(status) = 'active'"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("nested custom operators inside substr", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.Transpile(`{"!=": [{"substr": [{"toUpper": [{"var": "region"}]}, 0, 2]}, "XX"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "WHERE SUBSTR(UPPER(region), 1, 2) != 'XX'"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})
}

// TestDeeplyNestedCustomOperatorsMultiDialect tests deeply nested custom operators across all dialects.
func TestDeeplyNestedCustomOperatorsMultiDialect(t *testing.T) {
	dialects := []struct {
		dialect Dialect
		name    string
	}{
		{DialectBigQuery, "BigQuery"},
		{DialectSpanner, "Spanner"},
		{DialectPostgreSQL, "PostgreSQL"},
		{DialectDuckDB, "DuckDB"},
		{DialectClickHouse, "ClickHouse"},
	}

	// Helper to create a transpiler with common custom operators
	setupTranspiler := func(dialect Dialect) *Transpiler {
		tr, _ := NewTranspiler(dialect)
		tr.RegisterOperatorFunc("toLower", func(op string, args []interface{}) (string, error) {
			return fmt.Sprintf("LOWER(%s)", args[0]), nil
		})
		tr.RegisterOperatorFunc("toUpper", func(op string, args []interface{}) (string, error) {
			return fmt.Sprintf("UPPER(%s)", args[0]), nil
		})
		tr.RegisterOperatorFunc("!contains", func(op string, args []interface{}) (string, error) {
			return fmt.Sprintf("%s NOT LIKE '%%%s%%'", args[0], args[1]), nil
		})
		tr.RegisterOperatorFunc("endsWith", func(op string, args []interface{}) (string, error) {
			return fmt.Sprintf("%s LIKE '%%%s'", args[0], args[1]), nil
		})
		return tr
	}

	for _, d := range dialects {
		t.Run(d.name, func(t *testing.T) {
			tr := setupTranspiler(d.dialect)

			// Test: custom operator inside cat
			sql, err := tr.Transpile(`{"cat": ["Hello ", {"toUpper": [{"var": "name"}]}]}`)
			if err != nil {
				t.Errorf("[%s] cat with custom operator: unexpected error: %v", d.name, err)
			}
			if sql != "WHERE CONCAT('Hello ', UPPER(name))" {
				t.Errorf("[%s] cat with custom operator: got %s", d.name, sql)
			}

			// Test: custom operator inside map
			sql, err = tr.Transpile(`{"map": [{"var": "tags"}, {"toLower": [{"var": "item"}]}]}`)
			if err != nil {
				t.Errorf("[%s] map with custom operator: unexpected error: %v", d.name, err)
			}
			// ClickHouse uses arrayMap, others use UNNEST
			if d.dialect == DialectClickHouse {
				if sql != "WHERE arrayMap(elem -> LOWER(elem), tags)" {
					t.Errorf("[%s] map with custom operator: got %s", d.name, sql)
				}
			} else {
				if sql != "WHERE ARRAY(SELECT LOWER(elem) FROM UNNEST(tags) AS elem)" {
					t.Errorf("[%s] map with custom operator: got %s", d.name, sql)
				}
			}

			// Test: custom operator inside all
			sql, err = tr.Transpile(`{"all": [{"var": "tags"}, {"!contains": [{"var": "item"}, "spam"]}]}`)
			if err != nil {
				t.Errorf("[%s] all with custom operator: unexpected error: %v", d.name, err)
			}
			// ClickHouse uses arrayAll, others use NOT EXISTS
			// Note: string literals come pre-quoted from the parser
			if d.dialect == DialectClickHouse {
				if sql != "WHERE arrayAll(elem -> elem NOT LIKE '%'spam'%', tags)" {
					t.Errorf("[%s] all with custom operator: got %s", d.name, sql)
				}
			} else {
				if sql != "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem NOT LIKE '%'spam'%'))" {
					t.Errorf("[%s] all with custom operator: got %s", d.name, sql)
				}
			}

			// Test: and with all and some containing custom operators
			sql, err = tr.Transpile(`{"and": [{"all": [{"var": "tags"}, {"!contains": [{"var": "item"}, "spam"]}]}, {"some": [{"var": "emails"}, {"endsWith": [{"var": "item"}, "@valid.com"]}]}]}`)
			if err != nil {
				t.Errorf("[%s] and with all/some: unexpected error: %v", d.name, err)
			}
			// Just verify no error - detailed output differs by dialect
			if sql == "" {
				t.Errorf("[%s] and with all/some: got empty result", d.name)
			}
		})
	}
}
