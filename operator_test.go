package jsonlogic2sql

import (
	"fmt"
	"testing"
)

// LengthOperator implements OperatorHandler for LENGTH SQL function
type LengthOperator struct{}

func (l *LengthOperator) ToSQL(operator string, args []interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("length requires exactly 1 argument, got %d", len(args))
	}
	return fmt.Sprintf("LENGTH(%s)", args[0]), nil
}

// UpperOperator implements OperatorHandler for UPPER SQL function
type UpperOperator struct{}

func (u *UpperOperator) ToSQL(operator string, args []interface{}) (string, error) {
	if len(args) != 1 {
		return "", fmt.Errorf("upper requires exactly 1 argument, got %d", len(args))
	}
	return fmt.Sprintf("UPPER(%s)", args[0]), nil
}

// ConcatWithSeparatorOperator joins arguments with a separator
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
		transpiler := NewTranspiler()
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
		transpiler := NewTranspiler()
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
		transpiler := NewTranspiler()
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
		transpiler := NewTranspiler()
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
		transpiler := NewTranspiler()
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
		transpiler := NewTranspiler()
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
		transpiler := NewTranspiler()
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
		transpiler := NewTranspiler()
		err := transpiler.RegisterOperatorFunc("and", func(op string, args []interface{}) (string, error) {
			return "CUSTOM_AND", nil
		})
		if err == nil {
			t.Error("expected error when trying to override built-in operator")
		}
	})

	t.Run("HasCustomOperator", func(t *testing.T) {
		transpiler := NewTranspiler()
		transpiler.RegisterOperator("length", &LengthOperator{})

		if !transpiler.HasCustomOperator("length") {
			t.Error("expected HasCustomOperator to return true")
		}
		if transpiler.HasCustomOperator("nonexistent") {
			t.Error("expected HasCustomOperator to return false for non-registered")
		}
	})

	t.Run("UnregisterOperator", func(t *testing.T) {
		transpiler := NewTranspiler()
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
		transpiler := NewTranspiler()
		transpiler.RegisterOperator("length", &LengthOperator{})
		transpiler.RegisterOperator("upper", &UpperOperator{})

		list := transpiler.ListCustomOperators()
		if len(list) != 2 {
			t.Errorf("expected 2 operators, got %d", len(list))
		}
	})

	t.Run("ClearCustomOperators", func(t *testing.T) {
		transpiler := NewTranspiler()
		transpiler.RegisterOperator("length", &LengthOperator{})
		transpiler.RegisterOperator("upper", &UpperOperator{})

		transpiler.ClearCustomOperators()
		if len(transpiler.ListCustomOperators()) != 0 {
			t.Error("expected no custom operators after clear")
		}
	})

	t.Run("custom operator with literal argument", func(t *testing.T) {
		transpiler := NewTranspiler()
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
		transpiler := NewTranspiler()
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
		transpiler := NewTranspiler()
		transpiler.RegisterOperatorFunc("failing", func(op string, args []interface{}) (string, error) {
			return "", fmt.Errorf("intentional failure")
		})

		_, err := transpiler.Transpile(`{"failing": [{"var": "x"}]}`)
		if err == nil {
			t.Error("expected error from failing operator")
		}
	})

	t.Run("custom operator with no arguments", func(t *testing.T) {
		transpiler := NewTranspiler()
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
		transpiler := NewTranspiler()
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
