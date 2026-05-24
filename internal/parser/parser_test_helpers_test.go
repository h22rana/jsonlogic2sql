package parser

import (
	"fmt"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/operators"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// --- Helper types for custom operator tests ---

// mockCustomHandler implements CustomOperatorHandler for testing.
type mockCustomHandler struct {
	toSQL          func(operator string, args []interface{}) (string, error)
	forcePredicate bool
	resultKind     operators.ExpressionKind
	resultType     operators.ExpressionType
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
	if m.forcePredicate || m.resultKind == operators.ExpressionKindPredicate {
		return operators.PredicateSQL(sql), nil
	}
	return operators.ValueSQL(sql, m.resultType), nil
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

type fieldOnlyParserSchema struct{}

func (m *fieldOnlyParserSchema) HasField(_ string) bool { return true }

func (m *fieldOnlyParserSchema) GetFieldType(_ string) string { return "" }

func (m *fieldOnlyParserSchema) ValidateField(_ string) error { return nil }

func (m *fieldOnlyParserSchema) IsArrayType(_ string) bool { return false }

func (m *fieldOnlyParserSchema) IsStringType(_ string) bool { return false }

func (m *fieldOnlyParserSchema) IsNumericType(_ string) bool { return false }

func (m *fieldOnlyParserSchema) IsBooleanType(_ string) bool { return false }

func (m *fieldOnlyParserSchema) IsEnumType(_ string) bool { return false }

func (m *fieldOnlyParserSchema) GetAllowedValues(_ string) []string { return nil }

func (m *fieldOnlyParserSchema) ValidateEnumValue(_, _ string) error { return nil }

func newTestParser() *Parser {
	return NewParser(operators.NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlyParserSchema{}))
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
