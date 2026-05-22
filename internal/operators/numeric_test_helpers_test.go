package operators

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/params"
)

type numericSchemaProvider struct {
	fields map[string]string
}

func (m *numericSchemaProvider) HasField(fieldName string) bool {
	_, ok := m.fields[fieldName]
	return ok
}

func (m *numericSchemaProvider) GetFieldType(fieldName string) string {
	return m.fields[fieldName]
}

func (m *numericSchemaProvider) ValidateField(_ string) error {
	return nil
}

func (m *numericSchemaProvider) IsArrayType(fieldName string) bool {
	return m.fields[fieldName] == "array"
}

func (m *numericSchemaProvider) IsStringType(fieldName string) bool {
	return m.fields[fieldName] == "string"
}

func (m *numericSchemaProvider) IsNumericType(fieldName string) bool {
	t := m.fields[fieldName]
	return t == "integer" || t == "number"
}

func (m *numericSchemaProvider) IsBooleanType(fieldName string) bool {
	return m.fields[fieldName] == "boolean"
}

func (m *numericSchemaProvider) IsEnumType(_ string) bool {
	return false
}

func (m *numericSchemaProvider) GetAllowedValues(_ string) []string {
	return nil
}

func (m *numericSchemaProvider) ValidateEnumValue(_, _ string) error {
	return nil
}

func assertNumericQueryParams(t *testing.T, got, want []params.QueryParam) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("params: got %d entries, want %d: %#v vs %#v", len(got), len(want), got, want)
	}
	for i := range got {
		if got[i].Name != want[i].Name {
			t.Errorf("param[%d].Name = %q, want %q", i, got[i].Name, want[i].Name)
		}
		gv, wv := got[i].Value, want[i].Value
		if !numericParamValuesEqual(gv, wv) {
			t.Errorf("param[%d].Value = %#v (%T), want %#v (%T)", i, gv, gv, wv, wv)
		}
	}
}

// numericParamValuesEqual treats float64 and int whole numbers as equal for JSON/Go literal drift.
func numericParamValuesEqual(a, b interface{}) bool {
	if a == b {
		return true
	}
	af, aok := toFloat64ForCompare(a)
	bf, bok := toFloat64ForCompare(b)
	return aok && bok && af == bf
}

func toFloat64ForCompare(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	default:
		return 0, false
	}
}
