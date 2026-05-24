package jsonlogic2sql

import (
	"fmt"
	"reflect"
	"testing"
)

func testOperatorArgs(values ...string) []OperatorArg {
	args := make([]OperatorArg, len(values))
	for i, value := range values {
		args[i] = OperatorArg{SQL: value, Kind: ExpressionKindValue, Type: ExpressionTypeUnknown}
	}
	return args
}

func transpileOperatorExpression(tr *Transpiler, logic string) (string, error) {
	sql, err := tr.TranspileCondition(logic)
	if IsErrorCode(err, ErrInvalidExpressionContext) {
		return tr.TranspileValue(logic)
	}
	return sql, err
}

// LengthOperator implements OperatorHandler for LENGTH SQL function.
type LengthOperator struct{}

func (l *LengthOperator) ToSQL(operator string, args []OperatorArg) (OperatorResult, error) {
	if len(args) != 1 {
		return OperatorResult{}, fmt.Errorf("length requires exactly 1 argument, got %d", len(args))
	}
	return ValueSQL(fmt.Sprintf("LENGTH(%s)", args[0].SQL), ExpressionTypeNumber), nil
}

// UpperOperator implements OperatorHandler for UPPER SQL function.
type UpperOperator struct{}

func (u *UpperOperator) ToSQL(operator string, args []OperatorArg) (OperatorResult, error) {
	if len(args) != 1 {
		return OperatorResult{}, fmt.Errorf("upper requires exactly 1 argument, got %d", len(args))
	}
	return ValueSQL(fmt.Sprintf("UPPER(%s)", args[0].SQL), ExpressionTypeString), nil
}

// ConcatWithSeparatorOperator joins arguments with a separator.
type ConcatWithSeparatorOperator struct {
	Separator string
}

func (c *ConcatWithSeparatorOperator) ToSQL(operator string, args []OperatorArg) (OperatorResult, error) {
	if len(args) < 2 {
		return OperatorResult{}, fmt.Errorf("concat_ws requires at least 2 arguments")
	}
	result := args[0].SQL
	for i := 1; i < len(args); i++ {
		result += fmt.Sprintf(" || '%s' || %s", c.Separator, args[i].SQL)
	}
	return ValueSQL(result, ExpressionTypeString), nil
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
		registry.RegisterFunc("custom", func(op string, args []OperatorArg) (OperatorResult, error) {
			return ValueSQL("CUSTOM()", ExpressionTypeUnknown), nil
		})

		handler, ok := registry.Get("custom")
		if !ok {
			t.Fatal("expected to find custom operator")
		}
		result, err := handler.ToSQL("custom", nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.SQL != "CUSTOM()" {
			t.Errorf("expected CUSTOM(), got %s", result.SQL)
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
		registry.Register("upper", &UpperOperator{})
		registry.Register("length", &LengthOperator{})

		list := registry.List()
		want := []string{"length", "upper"}
		if !reflect.DeepEqual(list, want) {
			t.Errorf("List() = %#v, want %#v", list, want)
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

	t.Run("Merge nil and self are no-op", func(t *testing.T) {
		registry := NewOperatorRegistry()
		registry.Register("length", &LengthOperator{})

		registry.Merge(nil)
		registry.Merge(registry)

		if got, want := registry.List(), []string{"length"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("List() after no-op merges = %#v, want %#v", got, want)
		}
	})
}

func TestValidateOperatorName(t *testing.T) {
	t.Run("valid custom names", func(t *testing.T) {
		valid := []string{"length", "toLower", "my_op", "_private", "Op2", "a", "!contains", "!startsWith"}
		for _, name := range valid {
			if err := validateOperatorName(name); err != nil {
				t.Errorf("unexpected error for valid name %q: %v", name, err)
			}
		}
	})

	t.Run("empty name", func(t *testing.T) {
		if err := validateOperatorName(""); err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("whitespace-only names", func(t *testing.T) {
		names := []string{" ", "  ", "\t", "\n", " \t\n "}
		for _, name := range names {
			if err := validateOperatorName(name); err == nil {
				t.Errorf("expected error for whitespace-only name %q", name)
			}
		}
	})

	t.Run("invalid format names", func(t *testing.T) {
		invalid := []string{"1op", "my-op", "my op", "op!", "op.name", "op/name", "op+1", " length", "length ", " length "}
		for _, name := range invalid {
			if err := validateOperatorName(name); err == nil {
				t.Errorf("expected error for invalid name %q", name)
			}
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
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		err := transpiler.RegisterOperatorFunc("length", func(op string, args []OperatorArg) (OperatorResult, error) {
			if len(args) != 1 {
				return OperatorResult{}, fmt.Errorf("length requires 1 argument")
			}
			return ValueSQL(fmt.Sprintf("LENGTH(%s)", args[0].SQL), ExpressionTypeNumber), nil
		})
		if err != nil {
			t.Fatalf("unexpected error registering operator: %v", err)
		}

		sql, err := transpileOperatorExpression(transpiler, `{"length": [{"var": "email"}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "LENGTH(email)"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("RegisterOperatorFunc rejects nil function", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		err := transpiler.RegisterOperatorFunc("bad", nil)
		if err == nil {
			t.Fatal("RegisterOperatorFunc() expected error, got nil")
		}
		if transpiler.HasCustomOperator("bad") {
			t.Fatal("unsupported function type should not be registered")
		}
	})

	t.Run("RegisterOperator with struct", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		err := transpiler.RegisterOperator("length", &LengthOperator{})
		if err != nil {
			t.Fatalf("unexpected error registering operator: %v", err)
		}

		sql, err := transpileOperatorExpression(transpiler, `{"length": [{"var": "name"}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "LENGTH(name)"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator with nested expression", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		err := transpiler.RegisterOperatorFunc("length", func(op string, args []OperatorArg) (OperatorResult, error) {
			return ValueSQL(fmt.Sprintf("LENGTH(%s)", args[0].SQL), ExpressionTypeNumber), nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// length of concatenated string
		sql, err := transpileOperatorExpression(transpiler, `{"length": [{"cat": [{"var": "first"}, {"var": "last"}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "LENGTH(CONCAT(COALESCE(first, ''), COALESCE(last, '')))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator in comparison", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		err := transpiler.RegisterOperatorFunc("length", func(op string, args []OperatorArg) (OperatorResult, error) {
			return ValueSQL(fmt.Sprintf("LENGTH(%s)", args[0].SQL), ExpressionTypeNumber), nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpileOperatorExpression(transpiler, `{">": [{"length": [{"var": "email"}]}, 10]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "LENGTH(email) > 10"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("upper operator", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		err := transpiler.RegisterOperator("upper", &UpperOperator{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpileOperatorExpression(transpiler, `{"==": [{"upper": [{"var": "name"}]}, "JOHN"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "UPPER(name) = 'JOHN'"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("multiple custom operators", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		transpiler.RegisterOperator("length", &LengthOperator{})
		transpiler.RegisterOperator("upper", &UpperOperator{})

		sql, err := transpileOperatorExpression(transpiler, `{"and": [{">": [{"length": [{"var": "name"}]}, 5]}, {"==": [{"upper": [{"var": "status"}]}, "ACTIVE"]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "(LENGTH(name) > 5 AND UPPER(status) = 'ACTIVE')"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator with multiple args", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		err := transpiler.RegisterOperatorFunc("coalesce", func(op string, args []OperatorArg) (OperatorResult, error) {
			result := "COALESCE("
			for i, arg := range args {
				if i > 0 {
					result += ", "
				}
				result += arg.SQL
			}
			result += ")"
			return ValueSQL(result, ExpressionTypeUnknown), nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpileOperatorExpression(transpiler, `{"coalesce": [{"var": "nickname"}, {"var": "name"}, "Unknown"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "COALESCE(nickname, name, 'Unknown')"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("reject built-in operator override", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		err := transpiler.RegisterOperatorFunc("and", func(op string, args []OperatorArg) (OperatorResult, error) {
			return PredicateSQL("CUSTOM_AND"), nil
		})
		if err == nil {
			t.Error("expected error when trying to override built-in operator")
		}
	})

	t.Run("HasCustomOperator", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		transpiler.RegisterOperator("length", &LengthOperator{})

		if !transpiler.HasCustomOperator("length") {
			t.Error("expected HasCustomOperator to return true")
		}
		if transpiler.HasCustomOperator("nonexistent") {
			t.Error("expected HasCustomOperator to return false for non-registered")
		}
	})

	t.Run("UnregisterOperator", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		transpiler.RegisterOperator("length", &LengthOperator{})

		if !transpiler.UnregisterOperator("length") {
			t.Error("expected UnregisterOperator to return true")
		}

		// Now it should fail to transpile
		_, err := transpiler.TranspileCondition(`{"length": [{"var": "email"}]}`)
		if err == nil {
			t.Error("expected error after unregistering operator")
		}
	})

	t.Run("ListCustomOperators", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		transpiler.RegisterOperator("upper", &UpperOperator{})
		transpiler.RegisterOperator("length", &LengthOperator{})

		list := transpiler.ListCustomOperators()
		want := []string{"length", "upper"}
		if !reflect.DeepEqual(list, want) {
			t.Errorf("ListCustomOperators() = %#v, want %#v", list, want)
		}
	})

	t.Run("ClearCustomOperators", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		transpiler.RegisterOperator("length", &LengthOperator{})
		transpiler.RegisterOperator("upper", &UpperOperator{})

		transpiler.ClearCustomOperators()
		if len(transpiler.ListCustomOperators()) != 0 {
			t.Error("expected no custom operators after clear")
		}
	})

	t.Run("custom operator with literal argument", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		err := transpiler.RegisterOperatorFunc("repeat", func(op string, args []OperatorArg) (OperatorResult, error) {
			if len(args) != 2 {
				return OperatorResult{}, fmt.Errorf("repeat requires 2 arguments")
			}
			return ValueSQL(fmt.Sprintf("REPEAT(%s, %s)", args[0].SQL, args[1].SQL), ExpressionTypeString), nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpileOperatorExpression(transpiler, `{"repeat": [{"var": "char"}, 5]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "REPEAT(char, 5)"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator with stateful handler", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		err := transpiler.RegisterOperator("concat_ws", &ConcatWithSeparatorOperator{Separator: ", "})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpileOperatorExpression(transpiler, `{"concat_ws": [{"var": "first"}, {"var": "last"}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "first || ', ' || last"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})
}

func TestCustomOperatorEdgeCases(t *testing.T) {
	t.Run("custom operator returning error", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		transpiler.RegisterOperatorFunc("failing", func(op string, args []OperatorArg) (OperatorResult, error) {
			return OperatorResult{}, fmt.Errorf("intentional failure")
		})

		_, err := transpiler.TranspileCondition(`{"failing": [{"var": "x"}]}`)
		if err == nil {
			t.Error("expected error from failing operator")
		}
	})

	t.Run("custom operator with no arguments", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		transpiler.RegisterOperatorFunc("now", func(op string, args []OperatorArg) (OperatorResult, error) {
			return ValueSQL("NOW()", ExpressionTypeUnknown), nil
		})

		sql, err := transpileOperatorExpression(transpiler, `{"now": []}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "NOW()"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator with single non-array argument", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		transpiler.RegisterOperatorFunc("single", func(op string, args []OperatorArg) (OperatorResult, error) {
			return ValueSQL(fmt.Sprintf("SINGLE(%s)", args[0].SQL), ExpressionTypeUnknown), nil
		})

		// When argument is not an array, it should still work
		sql, err := transpileOperatorExpression(transpiler, `{"single": {"var": "x"}}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "SINGLE(x)"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})
}

// Test dialectAwareFuncHandler directly.
func TestDialectAwareFuncHandler(t *testing.T) {
	t.Run("ToSQL returns error requiring dialect", func(t *testing.T) {
		handler := &dialectAwareFuncHandler{
			fn: func(op string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
				return ValueSQL("TEST()", ExpressionTypeUnknown), nil
			},
		}

		_, err := handler.ToSQL("test_op", testOperatorArgs("arg1"))
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
			fn: func(op string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
				return ValueSQL(fmt.Sprintf("DIALECT_%s(%s)", dialect.String(), args[0].SQL), ExpressionTypeUnknown), nil
			},
		}

		result, err := handler.ToSQLWithDialect("test_op", testOperatorArgs("col"), DialectBigQuery)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "DIALECT_BigQuery(col)"
		if result.SQL != expected {
			t.Errorf("expected %q, got %q", expected, result.SQL)
		}
	})

	t.Run("ToSQLWithDialect with multiple dialects", func(t *testing.T) {
		handler := &dialectAwareFuncHandler{
			fn: func(op string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
				var sql string
				switch dialect {
				case DialectBigQuery:
					sql = "BQ_FUNC()"
				case DialectSpanner:
					sql = "SPANNER_FUNC()"
				case DialectPostgreSQL:
					sql = "PG_FUNC()"
				case DialectDuckDB:
					sql = "DUCKDB_FUNC()"
				case DialectClickHouse:
					sql = "CH_FUNC()"
				default:
					return OperatorResult{}, fmt.Errorf("unsupported dialect: %s", dialect)
				}
				return ValueSQL(sql, ExpressionTypeUnknown), nil
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
			if result.SQL != tt.expected {
				t.Errorf("dialect %s: expected %q, got %q", tt.dialect, tt.expected, result.SQL)
			}
		}
	})

	t.Run("ToSQLWithDialect passes error from wrapped function", func(t *testing.T) {
		handler := &dialectAwareFuncHandler{
			fn: func(op string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
				return OperatorResult{}, fmt.Errorf("custom error from function")
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

func (d *DialectAwareTestHandler) ToSQLWithDialect(operator string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
	return ValueSQL(fmt.Sprintf("%s_%s(%s)", d.prefix, dialect.String(), args[0].SQL), ExpressionTypeUnknown), nil
}

// Test dialectAwareHandlerWrapper directly.
func TestDialectAwareHandlerWrapper(t *testing.T) {
	t.Run("ToSQL uses stored dialect", func(t *testing.T) {
		handler := &DialectAwareTestHandler{prefix: "TEST"}
		wrapper := &dialectAwareHandlerWrapper{
			handler: handler,
			dialect: DialectBigQuery,
		}

		result, err := wrapper.ToSQL("my_op", testOperatorArgs("column"))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "TEST_BigQuery(column)"
		if result.SQL != expected {
			t.Errorf("expected %q, got %q", expected, result.SQL)
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

			result, err := wrapper.ToSQL("op", testOperatorArgs("arg"))
			if err != nil {
				t.Fatalf("dialect %s: unexpected error: %v", tt.dialect, err)
			}
			if result.SQL != tt.expected {
				t.Errorf("dialect %s: expected %q, got %q", tt.dialect, tt.expected, result.SQL)
			}
		}
	})
}

// Test RegisterDialectAwareFunc method on OperatorRegistry.
func TestOperatorRegistry_RegisterDialectAwareFunc(t *testing.T) {
	t.Run("register and retrieve dialect-aware function", func(t *testing.T) {
		registry := NewOperatorRegistry()
		registry.RegisterDialectAwareFunc("now", func(op string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
			return ValueSQL(fmt.Sprintf("NOW_%s()", dialect.String()), ExpressionTypeUnknown), nil
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
		if result.SQL != expected {
			t.Errorf("expected %q, got %q", expected, result.SQL)
		}
	})

	t.Run("ToSQL on dialect-aware func returns error", func(t *testing.T) {
		registry := NewOperatorRegistry()
		registry.RegisterDialectAwareFunc("custom", func(op string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
			return ValueSQL("CUSTOM()", ExpressionTypeUnknown), nil
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
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		err := transpiler.RegisterDialectAwareOperatorFunc("now", func(op string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
			var sql string
			switch dialect {
			case DialectBigQuery:
				sql = "CURRENT_TIMESTAMP()"
			case DialectSpanner:
				sql = "CURRENT_TIMESTAMP()"
			default:
				return OperatorResult{}, fmt.Errorf("unsupported dialect: %s", dialect)
			}
			return ValueSQL(sql, ExpressionTypeUnknown), nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpileOperatorExpression(transpiler, `{"==": [{"now": []}, "2024-01-01"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "CURRENT_TIMESTAMP() = '2024-01-01'"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("RegisterDialectAwareOperatorFunc with Spanner", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectSpanner)
		err := transpiler.RegisterDialectAwareOperatorFunc("array_length", func(op string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
			var sql string
			switch dialect {
			case DialectBigQuery:
				sql = fmt.Sprintf("ARRAY_LENGTH(%s)", args[0].SQL)
			case DialectSpanner:
				sql = fmt.Sprintf("ARRAY_LENGTH(%s)", args[0].SQL)
			default:
				return OperatorResult{}, fmt.Errorf("unsupported dialect: %s", dialect)
			}
			return ValueSQL(sql, ExpressionTypeNumber), nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		sql, err := transpileOperatorExpression(transpiler, `{">": [{"array_length": [{"var": "items"}]}, 0]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "ARRAY_LENGTH(items) > 0"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("dialect-aware operator with different output per dialect", func(t *testing.T) {
		// Define a function that returns different SQL based on dialect
		stringContainsOp := func(op string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
			if len(args) != 2 {
				return OperatorResult{}, fmt.Errorf("string_contains requires 2 arguments")
			}
			var sql string
			switch dialect {
			case DialectBigQuery:
				sql = fmt.Sprintf("STRPOS(%s, %s) > 0", args[0].SQL, args[1].SQL)
			case DialectSpanner:
				sql = fmt.Sprintf("STRPOS(%s, %s) > 0", args[0].SQL, args[1].SQL)
			default:
				return OperatorResult{}, fmt.Errorf("unsupported dialect: %s", dialect)
			}
			return PredicateSQL(sql), nil
		}

		// Test with BigQuery
		bqTranspiler := mustTestTranspiler(t, DialectBigQuery)
		bqTranspiler.RegisterDialectAwareOperatorFunc("string_contains", stringContainsOp)
		bqSQL, err := transpileOperatorExpression(bqTranspiler, `{"string_contains": [{"var": "name"}, "test"]}`)
		if err != nil {
			t.Fatalf("BigQuery: unexpected error: %v", err)
		}
		if bqSQL != "STRPOS(name, 'test') > 0" {
			t.Errorf("BigQuery: expected STRPOS(name, 'test') > 0, got %s", bqSQL)
		}

		// Test with Spanner
		spannerTranspiler := mustTestTranspiler(t, DialectSpanner)
		spannerTranspiler.RegisterDialectAwareOperatorFunc("string_contains", stringContainsOp)
		spannerSQL, err := transpileOperatorExpression(spannerTranspiler, `{"string_contains": [{"var": "name"}, "test"]}`)
		if err != nil {
			t.Fatalf("Spanner: unexpected error: %v", err)
		}
		if spannerSQL != "STRPOS(name, 'test') > 0" {
			t.Errorf("Spanner: expected STRPOS(name, 'test') > 0, got %s", spannerSQL)
		}
	})

	t.Run("reject built-in operator override with dialect-aware", func(t *testing.T) {
		transpiler := mustTestTranspiler(t, DialectBigQuery)
		err := transpiler.RegisterDialectAwareOperatorFunc("and", func(op string, args []OperatorArg, dialect Dialect) (OperatorResult, error) {
			return PredicateSQL("CUSTOM_AND"), nil
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
		tr := mustTestTranspiler(t, dialect)
		tr.RegisterOperatorFunc("toLower", func(op string, args []OperatorArg) (OperatorResult, error) {
			if len(args) != 1 {
				return OperatorResult{}, fmt.Errorf("toLower requires 1 argument")
			}
			return ValueSQL(fmt.Sprintf("LOWER(%s)", args[0].SQL), ExpressionTypeString), nil
		})
		tr.RegisterOperatorFunc("toUpper", func(op string, args []OperatorArg) (OperatorResult, error) {
			if len(args) != 1 {
				return OperatorResult{}, fmt.Errorf("toUpper requires 1 argument")
			}
			return ValueSQL(fmt.Sprintf("UPPER(%s)", args[0].SQL), ExpressionTypeString), nil
		})
		tr.RegisterOperatorFunc("startsWith", func(op string, args []OperatorArg) (OperatorResult, error) {
			if len(args) != 2 {
				return OperatorResult{}, fmt.Errorf("startsWith requires 2 arguments")
			}
			return PredicateSQL(fmt.Sprintf("%s LIKE CONCAT(%s, '%%')", args[0].SQL, args[1].SQL)), nil
		})
		tr.RegisterOperatorFunc("!startsWith", func(op string, args []OperatorArg) (OperatorResult, error) {
			if len(args) != 2 {
				return OperatorResult{}, fmt.Errorf("!startsWith requires 2 arguments")
			}
			return PredicateSQL(fmt.Sprintf("%s NOT LIKE CONCAT(%s, '%%')", args[0].SQL, args[1].SQL)), nil
		})
		tr.RegisterOperatorFunc("endsWith", func(op string, args []OperatorArg) (OperatorResult, error) {
			if len(args) != 2 {
				return OperatorResult{}, fmt.Errorf("endsWith requires 2 arguments")
			}
			return PredicateSQL(fmt.Sprintf("%s LIKE CONCAT('%%', %s)", args[0].SQL, args[1].SQL)), nil
		})
		tr.RegisterOperatorFunc("!endsWith", func(op string, args []OperatorArg) (OperatorResult, error) {
			if len(args) != 2 {
				return OperatorResult{}, fmt.Errorf("!endsWith requires 2 arguments")
			}
			return PredicateSQL(fmt.Sprintf("%s NOT LIKE CONCAT('%%', %s)", args[0].SQL, args[1].SQL)), nil
		})
		tr.RegisterOperatorFunc("contains", func(op string, args []OperatorArg) (OperatorResult, error) {
			if len(args) != 2 {
				return OperatorResult{}, fmt.Errorf("contains requires 2 arguments")
			}
			return PredicateSQL(fmt.Sprintf("%s LIKE CONCAT('%%', %s, '%%')", args[0].SQL, args[1].SQL)), nil
		})
		tr.RegisterOperatorFunc("!contains", func(op string, args []OperatorArg) (OperatorResult, error) {
			if len(args) != 2 {
				return OperatorResult{}, fmt.Errorf("!contains requires 2 arguments")
			}
			return PredicateSQL(fmt.Sprintf("%s NOT LIKE CONCAT('%%', %s, '%%')", args[0].SQL, args[1].SQL)), nil
		})
		return tr
	}

	t.Run("custom operator inside cat", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := transpileOperatorExpression(tr, `{"cat": [{"toLower": [{"var": "firstName"}]}, " ", {"toUpper": [{"var": "lastName"}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "CONCAT(COALESCE(LOWER(firstName), ''), ' ', COALESCE(UPPER(lastName), ''))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside if then/else branches", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := transpileOperatorExpression(tr, `{"if": [{"==": [{"var": "type"}, "premium"]}, {"toUpper": [{"var": "name"}]}, {"toLower": [{"var": "name"}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "CASE WHEN type = 'premium' THEN UPPER(name) ELSE LOWER(name) END"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside and/or", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := transpileOperatorExpression(tr, `{"and": [{"startsWith": [{"var": "name"}, "A"]}, {"or": [{"endsWith": [{"var": "email"}, "@company.com"]}, {"!contains": [{"var": "desc"}, "spam"]}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "(name LIKE CONCAT('A', '%') AND (email LIKE CONCAT('%', '@company.com') OR desc NOT LIKE CONCAT('%', 'spam', '%')))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside all array operator", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := transpileOperatorExpression(tr, `{"all": [{"var": "tags"}, {"!contains": [{"var": ""}, "spam"]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "(ARRAY_LENGTH(tags) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem NOT LIKE CONCAT('%', 'spam', '%'))))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside some array operator", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := transpileOperatorExpression(tr, `{"some": [{"var": "emails"}, {"endsWith": [{"var": ""}, "@company.com"]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "EXISTS (SELECT 1 FROM UNNEST(emails) AS elem WHERE elem LIKE CONCAT('%', '@company.com'))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside none array operator", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := transpileOperatorExpression(tr, `{"none": [{"var": "names"}, {"startsWith": [{"var": ""}, "Bot"]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "NOT EXISTS (SELECT 1 FROM UNNEST(names) AS elem WHERE elem LIKE CONCAT('Bot', '%'))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside filter array operator", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.TranspileValue(`{"filter": [{"var": "users"}, {"and": [{"!startsWith": [{"var": "name"}, "Test"]}, {"!endsWith": [{"var": "email"}, "@temp.com"]}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "ARRAY(SELECT elem FROM UNNEST(users) AS elem WHERE (elem.name NOT LIKE CONCAT('Test', '%') AND elem.email NOT LIKE CONCAT('%', '@temp.com')))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside map array operator", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.TranspileValue(`{"map": [{"var": "names"}, {"toLower": [{"var": ""}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "ARRAY(SELECT LOWER(elem) FROM UNNEST(names) AS elem)"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("custom operator inside reduce array operator", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		if sql, err := tr.TranspileValue(`{"reduce": [{"var": "items"}, {"cat": [{"var": "accumulator"}, {"toUpper": [{"var": "current"}]}]}, ""]}`); err == nil {
			t.Fatalf("BigQuery general string reduce should be unsupported, got SQL: %s", sql)
		}

		tr = setupTranspiler(DialectClickHouse)
		sql, err := tr.TranspileValue(`{"reduce": [{"var": "items"}, {"cat": [{"var": "accumulator"}, {"toUpper": [{"var": "current"}]}]}, ""]}`)
		if err != nil {
			t.Fatalf("ClickHouse reduce error: %v", err)
		}
		expected := "arrayFold((acc, elem) -> CONCAT(COALESCE(acc, ''), COALESCE(UPPER(elem), '')), items, '')"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("deeply nested: and with all containing custom operators", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := transpileOperatorExpression(tr, `{"and": [{"all": [{"var": "tags"}, {"!contains": [{"var": ""}, "spam"]}]}, {"some": [{"var": "emails"}, {"endsWith": [{"var": ""}, "@valid.com"]}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "((ARRAY_LENGTH(tags) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem NOT LIKE CONCAT('%', 'spam', '%')))) AND EXISTS (SELECT 1 FROM UNNEST(emails) AS elem WHERE elem LIKE CONCAT('%', '@valid.com')))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("deeply nested: or with none containing custom operators", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := transpileOperatorExpression(tr, `{"or": [{"none": [{"var": "names"}, {"startsWith": [{"var": ""}, "Bot"]}]}, {"all": [{"var": "scores"}, {">": [{"var": ""}, 50]}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "(NOT EXISTS (SELECT 1 FROM UNNEST(names) AS elem WHERE elem LIKE CONCAT('Bot', '%')) OR (ARRAY_LENGTH(scores) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(scores) AS elem WHERE NOT (elem > 50))))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("triple nested: and with or containing all/some/none", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := transpileOperatorExpression(tr, `{"and": [{"or": [{"all": [{"var": "tags"}, {"!contains": [{"var": ""}, "spam"]}]}, {"none": [{"var": "emails"}, {"startsWith": [{"var": ""}, "blocked_"]}]}]}, {"some": [{"var": "scores"}, {">": [{"var": ""}, 100]}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "(((ARRAY_LENGTH(tags) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem NOT LIKE CONCAT('%', 'spam', '%')))) OR NOT EXISTS (SELECT 1 FROM UNNEST(emails) AS elem WHERE elem LIKE CONCAT('blocked_', '%'))) AND EXISTS (SELECT 1 FROM UNNEST(scores) AS elem WHERE elem > 100))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("filter with nested and/or and multiple custom operators", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.TranspileValue(`{"filter": [{"var": "transactions"}, {"and": [{"!startsWith": [{"var": "name"}, "VOID"]}, {"!endsWith": [{"var": "category"}, "_canceled"]}, {"!contains": [{"var": "email"}, "spam"]}]}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "ARRAY(SELECT elem FROM UNNEST(transactions) AS elem WHERE (elem.name NOT LIKE CONCAT('VOID', '%') AND elem.category NOT LIKE CONCAT('%', '_canceled') AND elem.email NOT LIKE CONCAT('%', 'spam', '%')))"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("if with all condition in then branch", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := tr.TranspileValue(`{"if": [{"all": [{"var": "scores"}, {">": [{"var": ""}, 50]}]}, {"var": "status"}, "FAILED"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "CASE WHEN (ARRAY_LENGTH(scores) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(scores) AS elem WHERE NOT (elem > 50))) THEN status ELSE 'FAILED' END"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("comparison with custom operator result", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := transpileOperatorExpression(tr, `{"==": [{"toLower": [{"var": "status"}]}, "active"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "LOWER(status) = 'active'"
		if sql != expected {
			t.Errorf("expected %s, got %s", expected, sql)
		}
	})

	t.Run("nested custom operators inside substr", func(t *testing.T) {
		tr := setupTranspiler(DialectBigQuery)
		sql, err := transpileOperatorExpression(tr, `{"!=": [{"substr": [{"toUpper": [{"var": "region"}]}, 0, 2]}, "XX"]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "SUBSTR(UPPER(region), 1, 2) != 'XX'"
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
		tr := mustTestTranspiler(t, dialect)
		tr.RegisterOperatorFunc("toLower", func(op string, args []OperatorArg) (OperatorResult, error) {
			return ValueSQL(fmt.Sprintf("LOWER(%s)", args[0].SQL), ExpressionTypeString), nil
		})
		tr.RegisterOperatorFunc("toUpper", func(op string, args []OperatorArg) (OperatorResult, error) {
			return ValueSQL(fmt.Sprintf("UPPER(%s)", args[0].SQL), ExpressionTypeString), nil
		})
		tr.RegisterOperatorFunc("!contains", func(op string, args []OperatorArg) (OperatorResult, error) {
			return PredicateSQL(fmt.Sprintf("%s NOT LIKE CONCAT('%%', %s, '%%')", args[0].SQL, args[1].SQL)), nil
		})
		tr.RegisterOperatorFunc("endsWith", func(op string, args []OperatorArg) (OperatorResult, error) {
			return PredicateSQL(fmt.Sprintf("%s LIKE CONCAT('%%', %s)", args[0].SQL, args[1].SQL)), nil
		})
		return tr
	}

	for _, d := range dialects {
		t.Run(d.name, func(t *testing.T) {
			tr := setupTranspiler(d.dialect)

			// Test: custom operator inside cat
			sql, err := tr.TranspileValue(`{"cat": ["Hello ", {"toUpper": [{"var": "name"}]}]}`)
			if err != nil {
				t.Errorf("[%s] cat with custom operator: unexpected error: %v", d.name, err)
			}
			if sql != "CONCAT('Hello ', COALESCE(UPPER(name), ''))" {
				t.Errorf("[%s] cat with custom operator: got %s", d.name, sql)
			}

			// Test: custom operator inside map
			sql, err = tr.TranspileValue(`{"map": [{"var": "tags"}, {"toLower": [{"var": ""}]}]}`)
			if err != nil {
				t.Errorf("[%s] map with custom operator: unexpected error: %v", d.name, err)
			}
			// ClickHouse uses arrayMap, others use UNNEST
			if d.dialect == DialectClickHouse {
				if sql != "arrayMap(elem -> LOWER(elem), tags)" {
					t.Errorf("[%s] map with custom operator: got %s", d.name, sql)
				}
			} else {
				expectedMap := testDuckDBUnnestSourceAliases(d.dialect, "ARRAY(SELECT LOWER(elem) FROM UNNEST(tags) AS elem)")
				if sql != expectedMap {
					t.Errorf("[%s] map with custom operator: got %s, want %s", d.name, sql, expectedMap)
				}
			}

			// Test: custom operator inside all
			sql, err = transpileOperatorExpression(tr, `{"all": [{"var": "tags"}, {"!contains": [{"var": ""}, "spam"]}]}`)
			if err != nil {
				t.Errorf("[%s] all with custom operator: unexpected error: %v", d.name, err)
			}
			// ClickHouse uses arrayAll, others use NOT EXISTS with dialect-specific array length.
			var expectedAll string
			switch d.dialect {
			case DialectClickHouse:
				expectedAll = "(length(tags) > 0 AND arrayAll(elem -> elem NOT LIKE CONCAT('%', 'spam', '%'), tags))"
			case DialectPostgreSQL:
				expectedAll = "(CARDINALITY(tags) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem NOT LIKE CONCAT('%', 'spam', '%'))))"
			case DialectDuckDB:
				expectedAll = testDuckDBUnnestSourceAliases(d.dialect, "(length(tags) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem NOT LIKE CONCAT('%', 'spam', '%'))))")
			default: // BigQuery, Spanner
				expectedAll = "(ARRAY_LENGTH(tags) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem NOT LIKE CONCAT('%', 'spam', '%'))))"
			}
			if sql != expectedAll {
				t.Errorf("[%s] all with custom operator: got %s", d.name, sql)
			}

			// Test: and with all and some containing custom operators
			sql, err = transpileOperatorExpression(tr, `{"and": [{"all": [{"var": "tags"}, {"!contains": [{"var": ""}, "spam"]}]}, {"some": [{"var": "emails"}, {"endsWith": [{"var": ""}, "@valid.com"]}]}]}`)
			if err != nil {
				t.Errorf("[%s] and with all/some: unexpected error: %v", d.name, err)
			}
			var expectedAnd string
			switch d.dialect {
			case DialectClickHouse:
				expectedAnd = "((length(tags) > 0 AND arrayAll(elem -> elem NOT LIKE CONCAT('%', 'spam', '%'), tags)) AND arrayExists(elem -> elem LIKE CONCAT('%', '@valid.com'), emails))"
			case DialectPostgreSQL:
				expectedAnd = "((CARDINALITY(tags) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem NOT LIKE CONCAT('%', 'spam', '%')))) AND EXISTS (SELECT 1 FROM UNNEST(emails) AS elem WHERE elem LIKE CONCAT('%', '@valid.com')))"
			case DialectDuckDB:
				expectedAnd = testDuckDBUnnestSourceAliases(d.dialect, "((length(tags) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem NOT LIKE CONCAT('%', 'spam', '%')))) AND EXISTS (SELECT 1 FROM UNNEST(emails) AS elem WHERE elem LIKE CONCAT('%', '@valid.com')))")
			default:
				expectedAnd = "((ARRAY_LENGTH(tags) > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem NOT LIKE CONCAT('%', 'spam', '%')))) AND EXISTS (SELECT 1 FROM UNNEST(emails) AS elem WHERE elem LIKE CONCAT('%', '@valid.com')))"
			}
			if sql != expectedAnd {
				t.Errorf("[%s] and with all/some: got %s, want %s", d.name, sql, expectedAnd)
			}
		})
	}
}
