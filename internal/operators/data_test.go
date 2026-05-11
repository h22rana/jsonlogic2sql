package operators

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func TestDataOperator_ToSQL(t *testing.T) {
	op := NewDataOperator(nil)

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		expected string
		hasError bool
	}{
		// var operator tests
		{
			name:     "var with simple string",
			operator: "var",
			args:     []interface{}{"amount"},
			expected: "amount",
			hasError: false,
		},
		{
			name:     "var with dotted string",
			operator: "var",
			args:     []interface{}{"transaction.amount"},
			expected: "transaction.amount",
			hasError: false,
		},
		{
			name:     "var with nested dotted string",
			operator: "var",
			args:     []interface{}{"user.account.age"},
			expected: "user.account.age",
			hasError: false,
		},
		{
			name:     "var with array and default",
			operator: "var",
			args:     []interface{}{[]interface{}{"amount", 0}},
			expected: "COALESCE(amount, 0)",
			hasError: false,
		},
		{
			name:     "var with array and string default",
			operator: "var",
			args:     []interface{}{[]interface{}{"status", "pending"}},
			expected: "COALESCE(status, 'pending')",
			hasError: false,
		},
		{
			name:     "var with array and boolean default",
			operator: "var",
			args:     []interface{}{[]interface{}{"verified", true}},
			expected: "COALESCE(verified, TRUE)",
			hasError: false,
		},
		{
			name:     "var with array and null default",
			operator: "var",
			args:     []interface{}{[]interface{}{"field", nil}},
			expected: "COALESCE(field, NULL)",
			hasError: false,
		},
		{
			name:     "var with empty array",
			operator: "var",
			args:     []interface{}{[]interface{}{}},
			expected: "",
			hasError: true,
		},
		{
			name:     "var with too many array entries",
			operator: "var",
			args:     []interface{}{[]interface{}{"amount", 0, 1}},
			expected: "",
			hasError: true,
		},
		{
			name:     "var with non-string first arg",
			operator: "var",
			args:     []interface{}{[]interface{}{123, 0}},
			expected: "",
			hasError: true,
		},
		{
			name:     "var with no args",
			operator: "var",
			args:     []interface{}{},
			expected: "",
			hasError: true,
		},
		{
			name:     "non-finite float literal",
			operator: "var",
			args:     []interface{}{[]interface{}{"amount", math.Inf(1)}},
			expected: "",
			hasError: true,
		},

		// missing operator tests
		{
			name:     "missing with simple string",
			operator: "missing",
			args:     []interface{}{"field"},
			expected: "field IS NULL",
			hasError: false,
		},
		{
			name:     "missing with dotted string",
			operator: "missing",
			args:     []interface{}{"user.name"},
			expected: "user.name IS NULL",
			hasError: false,
		},
		{
			name:     "missing with wrong arg count",
			operator: "missing",
			args:     []interface{}{"field", "extra"},
			expected: "",
			hasError: true,
		},
		{
			name:     "missing with non-string arg",
			operator: "missing",
			args:     []interface{}{123},
			expected: "",
			hasError: true,
		},
		{
			name:     "missing with array of fields",
			operator: "missing",
			args:     []interface{}{[]interface{}{"email", "phone", "address"}},
			expected: "(email IS NULL OR phone IS NULL OR address IS NULL)",
			hasError: false,
		},
		{
			name:     "missing with array of single field",
			operator: "missing",
			args:     []interface{}{[]interface{}{"email"}},
			expected: "(email IS NULL)",
			hasError: false,
		},
		{
			name:     "missing with empty array",
			operator: "missing",
			args:     []interface{}{[]interface{}{}},
			expected: "",
			hasError: true,
		},

		// missing_some operator tests
		{
			name:     "missing_some with single field",
			operator: "missing_some",
			args:     []interface{}{1, []interface{}{"field"}},
			expected: "(field IS NULL)",
			hasError: false,
		},
		{
			name:     "missing_some with multiple fields",
			operator: "missing_some",
			args:     []interface{}{2, []interface{}{"field1", "field2", "field3"}},
			expected: "(CASE WHEN field1 IS NULL THEN 1 ELSE 0 END + CASE WHEN field2 IS NULL THEN 1 ELSE 0 END + CASE WHEN field3 IS NULL THEN 1 ELSE 0 END) >= 2",
			hasError: false,
		},
		{
			name:     "missing_some with dotted fields",
			operator: "missing_some",
			args:     []interface{}{1, []interface{}{"user.name", "user.email"}},
			expected: "(user.name IS NULL OR user.email IS NULL)",
			hasError: false,
		},
		{
			name:     "missing_some with wrong arg count",
			operator: "missing_some",
			args:     []interface{}{1},
			expected: "",
			hasError: true,
		},
		{
			name:     "missing_some with non-number first arg",
			operator: "missing_some",
			args:     []interface{}{"1", []interface{}{"field"}},
			expected: "",
			hasError: true,
		},
		{
			name:     "missing_some with non-array second arg",
			operator: "missing_some",
			args:     []interface{}{1, "field"},
			expected: "",
			hasError: true,
		},
		{
			name:     "missing_some with empty array",
			operator: "missing_some",
			args:     []interface{}{1, []interface{}{}},
			expected: "",
			hasError: true,
		},
		{
			name:     "missing_some with non-string in array",
			operator: "missing_some",
			args:     []interface{}{1, []interface{}{"field", 123}},
			expected: "",
			hasError: true,
		},

		// unsupported operator
		{
			name:     "unsupported operator",
			operator: "unsupported",
			args:     []interface{}{"test"},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.ToSQL(tt.operator, tt.args)

			if tt.hasError {
				if err == nil {
					t.Errorf("ToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("ToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("ToSQL() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestDataOperator_convertVarName(t *testing.T) {
	op := NewDataOperator(nil)

	valid := []struct {
		input    string
		expected string
	}{
		{"amount", "amount"},
		{"transaction.amount", "transaction.amount"},
		{"user.account.age", "user.account.age"},
		{"a.b.c.d", "a.b.c.d"},
		{"simple", "simple"},
		{"_private", "_private"},
		{"field_name", "field_name"},
		{"Field123", "Field123"},
		{"24h", "`24h`"},
		{"data.24h.tx", "data.`24h`.tx"},
	}

	t.Run("valid identifiers", func(t *testing.T) {
		for _, tt := range valid {
			t.Run(tt.input, func(t *testing.T) {
				result, err := op.convertVarName(tt.input)
				if err != nil {
					t.Errorf("convertVarName(%q) unexpected error: %v", tt.input, err)
				}
				if result != tt.expected {
					t.Errorf("convertVarName(%q) = %q, expected %q", tt.input, result, tt.expected)
				}
			})
		}
	})

	t.Run("invalid identifiers rejected without schema", func(t *testing.T) {
		invalid := []string{
			"",
			"1; DROP TABLE users; --",
			"' OR 1=1 --",
			"field name",
			"field\ttab",
			"field;name",
			"(expression)",
			"field..name",
			".field",
			"field.",
		}

		for _, input := range invalid {
			t.Run(input, func(t *testing.T) {
				_, err := op.convertVarName(input)
				if err == nil {
					t.Errorf("convertVarName(%q) expected error, got none", input)
				}
			})
		}
	})

	t.Run("schema bypasses identifier validation", func(t *testing.T) {
		schema := &dataSchemaProvider{}
		opWithSchema := NewDataOperator(NewOperatorConfig(0, schema))
		// With schema, unusual but raw names are quoted after schema validation.
		result, err := opWithSchema.convertVarName("my field")
		if err != nil {
			t.Errorf("convertVarName with schema unexpected error: %v", err)
		}
		if result != "`my field`" {
			t.Errorf("convertVarName with schema = %q, expected %q", result, "`my field`")
		}
	})
}

func TestDataOperator_convertVarName_rejectsPreQuoted(t *testing.T) {
	op := NewDataOperator(nil)

	tests := []struct {
		name  string
		input string
	}{
		{"backtick quoted segment", "data.`24h`.tx"},
		{"double-quote quoted segment", `data."24h".tx`},
		{"embedded backtick", "col`name"},
		{"embedded double quote", `col"name`},
		{"single-quote quoted segment", "data.'24h'.tx"},
		{"embedded single quote", "col'name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := op.convertVarName(tt.input)
			if err == nil {
				t.Errorf("convertVarName(%q) expected error for pre-quoted input, got nil", tt.input)
			}
		})
	}
}

func TestDataOperator_ToSQL_rejectsPreQuotedBeforeSchemaValidation(t *testing.T) {
	op := NewDataOperator(NewOperatorConfig(0, &rejectingDataSchemaProvider{}))

	_, err := op.ToSQL("var", []interface{}{"data.`24h`.tx"})
	if err == nil {
		t.Fatal("ToSQL() expected error for pre-quoted input, got nil")
	}
	if !strings.Contains(err.Error(), "contains quote characters") {
		t.Fatalf("ToSQL() error = %q, want quote-character validation", err.Error())
	}
}

// dataSchemaProvider is a minimal schema provider for data operator tests.
type dataSchemaProvider struct{}

func (m *dataSchemaProvider) HasField(_ string) bool              { return true }
func (m *dataSchemaProvider) GetFieldType(_ string) string        { return "string" }
func (m *dataSchemaProvider) ValidateField(_ string) error        { return nil }
func (m *dataSchemaProvider) IsArrayType(_ string) bool           { return false }
func (m *dataSchemaProvider) IsStringType(_ string) bool          { return true }
func (m *dataSchemaProvider) IsNumericType(_ string) bool         { return false }
func (m *dataSchemaProvider) IsBooleanType(_ string) bool         { return false }
func (m *dataSchemaProvider) IsEnumType(_ string) bool            { return false }
func (m *dataSchemaProvider) GetAllowedValues(_ string) []string  { return nil }
func (m *dataSchemaProvider) ValidateEnumValue(_, _ string) error { return nil }

type rejectingDataSchemaProvider struct {
	dataSchemaProvider
}

func (m *rejectingDataSchemaProvider) ValidateField(_ string) error {
	return errors.New("schema validation called before quote validation")
}

func TestDataOperator_valueToSQL(t *testing.T) {
	op := NewDataOperator(nil)

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{"string", "hello", "'hello'", false},
		{"string with quotes", "he'llo", "'he''llo'", false},
		{"integer", 42, "42", false},
		{"float", 3.14, "3.14", false},
		{"boolean true", true, "TRUE", false},
		{"boolean false", false, "FALSE", false},
		{"null", nil, "NULL", false},
		{"int64", int64(123), "123", false},
		{"float32", float32(1.5), "1.5", false},
		{"json.Number integer", json.Number("123"), "123", false},
		{"json.Number float", json.Number("1.25"), "1.25", false},
		{"json.Number scientific", json.Number("1e3"), "1e3", false},
		{"json.Number invalid SQL fragment", json.Number("1 OR 1=1"), "", true},
		{"unsupported type", []string{"a"}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.valueToSQL(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("valueToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("valueToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("valueToSQL() = %s, expected %s", result, tt.expected)
				}
			}
		})
	}
}

func TestDataOperator_getNumber(t *testing.T) {
	op := NewDataOperator(nil)

	tests := []struct {
		name     string
		input    interface{}
		expected float64
		hasError bool
	}{
		{
			name:     "float64",
			input:    float64(3.14),
			expected: 3.14,
			hasError: false,
		},
		{
			name:     "float32",
			input:    float32(2.5),
			expected: float64(float32(2.5)),
			hasError: false,
		},
		{
			name:     "int",
			input:    42,
			expected: 42.0,
			hasError: false,
		},
		{
			name:     "int8",
			input:    int8(10),
			expected: 10.0,
			hasError: false,
		},
		{
			name:     "int16",
			input:    int16(100),
			expected: 100.0,
			hasError: false,
		},
		{
			name:     "int32",
			input:    int32(1000),
			expected: 1000.0,
			hasError: false,
		},
		{
			name:     "int64",
			input:    int64(9999),
			expected: 9999.0,
			hasError: false,
		},
		{
			name:     "uint",
			input:    uint(7),
			expected: 7.0,
			hasError: false,
		},
		{
			name:     "uint8",
			input:    uint8(8),
			expected: 8.0,
			hasError: false,
		},
		{
			name:     "uint16",
			input:    uint16(16),
			expected: 16.0,
			hasError: false,
		},
		{
			name:     "uint32",
			input:    uint32(32),
			expected: 32.0,
			hasError: false,
		},
		{
			name:     "uint64",
			input:    uint64(64),
			expected: 64.0,
			hasError: false,
		},
		{
			name:     "string - not a number",
			input:    "not a number",
			expected: 0,
			hasError: true,
		},
		{
			name:     "bool - not a number",
			input:    true,
			expected: 0,
			hasError: true,
		},
		{
			name:     "nil - not a number",
			input:    nil,
			expected: 0,
			hasError: true,
		},
		{
			name:     "slice - not a number",
			input:    []int{1, 2},
			expected: 0,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.getNumber(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("getNumber() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("getNumber() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("getNumber() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestDataOperator_valueToSQL_ProcessedValue(t *testing.T) {
	op := NewDataOperator(nil)

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "ProcessedValue with SQL",
			input:    ProcessedValue{Value: "COUNT(*)", IsSQL: true},
			expected: "COUNT(*)",
			hasError: false,
		},
		{
			name:     "ProcessedValue with literal string",
			input:    ProcessedValue{Value: "hello", IsSQL: false},
			expected: "'hello'",
			hasError: false,
		},
		{
			name:     "ProcessedValue with literal number string",
			input:    ProcessedValue{Value: "42", IsSQL: false},
			expected: "'42'",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.valueToSQL(tt.input)
			if tt.hasError {
				if err == nil {
					t.Errorf("valueToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("valueToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("valueToSQL() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestDataOperator_handleVar_EmptyVarName(t *testing.T) {
	op := NewDataOperator(nil)

	// Empty var name represents the current element in array operations
	result, err := op.ToSQL("var", []interface{}{""})
	if err != nil {
		t.Errorf("ToSQL() unexpected error = %v", err)
	}
	if result != ElemVar {
		t.Errorf("ToSQL() = %v, want %v", result, ElemVar)
	}
}

func TestDataOperator_handleVar_NonStringNonArrayArg(t *testing.T) {
	op := NewDataOperator(nil)

	// Non-string, non-array argument
	_, err := op.ToSQL("var", []interface{}{42})
	if err == nil {
		t.Errorf("ToSQL() expected error for non-string/non-array arg, got nil")
	}
}

func assertCollectedParams(t *testing.T, pc *params.ParamCollector, want []params.QueryParam) {
	t.Helper()
	got := pc.Params()
	if len(got) != len(want) {
		t.Fatalf("Params() len = %d, want %d; got %#v", len(got), len(want), got)
	}
	for i := range got {
		if got[i].Name != want[i].Name {
			t.Errorf("Params()[%d].Name = %q, want %q", i, got[i].Name, want[i].Name)
		}
		if !reflect.DeepEqual(got[i].Value, want[i].Value) {
			t.Errorf("Params()[%d].Value = %#v (%T), want %#v (%T)",
				i, got[i].Value, got[i].Value, want[i].Value, want[i].Value)
		}
	}
	if pc.Count() != len(want) {
		t.Errorf("Count() = %d, want %d", pc.Count(), len(want))
	}
}

func TestDataOperator_ToSQLParam(t *testing.T) {
	op := NewDataOperator(nil)

	tests := []struct {
		name        string
		operator    string
		args        []interface{}
		expectedSQL string
		wantParams  []params.QueryParam
		hasError    bool
	}{
		{
			name:        "var with simple string",
			operator:    "var",
			args:        []interface{}{"amount"},
			expectedSQL: "amount",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "var with dotted string",
			operator:    "var",
			args:        []interface{}{"transaction.amount"},
			expectedSQL: "transaction.amount",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "var with array and default string",
			operator:    "var",
			args:        []interface{}{[]interface{}{"amount", "pending"}},
			expectedSQL: "COALESCE(amount, @p1)",
			wantParams:  []params.QueryParam{{Name: "p1", Value: "pending"}},
			hasError:    false,
		},
		{
			name:        "var with array and default number",
			operator:    "var",
			args:        []interface{}{[]interface{}{"amount", float64(0)}},
			expectedSQL: "COALESCE(amount, @p1)",
			wantParams:  []params.QueryParam{{Name: "p1", Value: float64(0)}},
			hasError:    false,
		},
		{
			name:        "var with array and default bool",
			operator:    "var",
			args:        []interface{}{[]interface{}{"verified", true}},
			expectedSQL: "COALESCE(verified, TRUE)",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "var with array and default nil",
			operator:    "var",
			args:        []interface{}{[]interface{}{"field", nil}},
			expectedSQL: "COALESCE(field, NULL)",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "missing with simple string",
			operator:    "missing",
			args:        []interface{}{"field"},
			expectedSQL: "field IS NULL",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "missing_some with count 1",
			operator:    "missing_some",
			args:        []interface{}{1, []interface{}{"field1", "field2", "field3"}},
			expectedSQL: "(field1 IS NULL OR field2 IS NULL OR field3 IS NULL)",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:     "missing_some with count 2",
			operator: "missing_some",
			args:     []interface{}{2, []interface{}{"field1", "field2", "field3"}},
			expectedSQL: "(CASE WHEN field1 IS NULL THEN 1 ELSE 0 END + " +
				"CASE WHEN field2 IS NULL THEN 1 ELSE 0 END + " +
				"CASE WHEN field3 IS NULL THEN 1 ELSE 0 END) >= @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: float64(2)}},
			hasError:   false,
		},
		{
			name:        "unsupported operator",
			operator:    "unsupported",
			args:        []interface{}{"test"},
			expectedSQL: "",
			wantParams:  nil,
			hasError:    true,
		},
		{
			name:        "var with empty array",
			operator:    "var",
			args:        []interface{}{[]interface{}{}},
			expectedSQL: "",
			wantParams:  nil,
			hasError:    true,
		},
		{
			name:        "var with too many array entries",
			operator:    "var",
			args:        []interface{}{[]interface{}{"amount", float64(0), float64(1)}},
			expectedSQL: "",
			wantParams:  nil,
			hasError:    true,
		},
		{
			name:        "var with non-string first arg",
			operator:    "var",
			args:        []interface{}{[]interface{}{123, 0}},
			expectedSQL: "",
			wantParams:  nil,
			hasError:    true,
		},
		{
			name:        "var with no args",
			operator:    "var",
			args:        []interface{}{},
			expectedSQL: "",
			wantParams:  nil,
			hasError:    true,
		},
		{
			name:        "non-finite float literal",
			operator:    "var",
			args:        []interface{}{[]interface{}{"amount", math.NaN()}},
			expectedSQL: "",
			wantParams:  nil,
			hasError:    true,
		},
		{
			name:        "missing with wrong arg count",
			operator:    "missing",
			args:        []interface{}{"field", "extra"},
			expectedSQL: "",
			wantParams:  nil,
			hasError:    true,
		},
		{
			name:        "missing_some with wrong arg count",
			operator:    "missing_some",
			args:        []interface{}{1},
			expectedSQL: "",
			wantParams:  nil,
			hasError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			result, err := op.ToSQLParam(tt.operator, tt.args, pc)

			if tt.hasError {
				if err == nil {
					t.Errorf("ToSQLParam() expected error, got nil")
				}
				if pc.Count() != 0 {
					t.Errorf("ToSQLParam() on error expected Count() 0, got %d", pc.Count())
				}
				return
			}
			if err != nil {
				t.Fatalf("ToSQLParam() unexpected error = %v", err)
			}
			if result != tt.expectedSQL {
				t.Errorf("ToSQLParam() = %q, want %q", result, tt.expectedSQL)
			}
			assertCollectedParams(t, pc, tt.wantParams)
		})
	}
}

func TestDataOperator_valueToSQLParam(t *testing.T) {
	op := NewDataOperator(nil)

	tests := []struct {
		name        string
		input       interface{}
		expectedSQL string
		wantParams  []params.QueryParam
		hasError    bool
	}{
		{
			name:        "string",
			input:       "hello",
			expectedSQL: "@p1",
			wantParams:  []params.QueryParam{{Name: "p1", Value: "hello"}},
			hasError:    false,
		},
		{
			name:        "integer",
			input:       42,
			expectedSQL: "@p1",
			wantParams:  []params.QueryParam{{Name: "p1", Value: 42}},
			hasError:    false,
		},
		{
			name:        "float64",
			input:       3.14,
			expectedSQL: "@p1",
			wantParams:  []params.QueryParam{{Name: "p1", Value: 3.14}},
			hasError:    false,
		},
		{
			name:        "float32",
			input:       float32(1.5),
			expectedSQL: "@p1",
			wantParams:  []params.QueryParam{{Name: "p1", Value: float64(1.5)}},
			hasError:    false,
		},
		{
			name:        "int64",
			input:       int64(123),
			expectedSQL: "@p1",
			wantParams:  []params.QueryParam{{Name: "p1", Value: int64(123)}},
			hasError:    false,
		},
		{
			name:        "boolean true",
			input:       true,
			expectedSQL: "TRUE",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "boolean false",
			input:       false,
			expectedSQL: "FALSE",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "nil",
			input:       nil,
			expectedSQL: "NULL",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "ProcessedValue SQL",
			input:       ProcessedValue{Value: "COUNT(*)", IsSQL: true},
			expectedSQL: "COUNT(*)",
			wantParams:  nil,
			hasError:    false,
		},
		{
			name:        "ProcessedValue literal string",
			input:       ProcessedValue{Value: "hello", IsSQL: false},
			expectedSQL: "@p1",
			wantParams:  []params.QueryParam{{Name: "p1", Value: "hello"}},
			hasError:    false,
		},
		{
			name:        "json.Number valid",
			input:       json.Number("123"),
			expectedSQL: "@p1",
			wantParams:  []params.QueryParam{{Name: "p1", Value: float64(123)}},
			hasError:    false,
		},
		{
			name:        "json.Number invalid SQL fragment",
			input:       json.Number("1 OR 1=1"),
			expectedSQL: "",
			wantParams:  nil,
			hasError:    true,
		},
		{
			name:        "unsupported type",
			input:       []string{"a"},
			expectedSQL: "",
			wantParams:  nil,
			hasError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			result, err := op.valueToSQLParam(tt.input, pc)

			if tt.hasError {
				if err == nil {
					t.Errorf("valueToSQLParam() expected error, got nil")
				}
				if pc.Count() != 0 {
					t.Errorf("valueToSQLParam() on error expected Count() 0, got %d", pc.Count())
				}
				return
			}
			if err != nil {
				t.Fatalf("valueToSQLParam() unexpected error = %v", err)
			}
			if result != tt.expectedSQL {
				t.Errorf("valueToSQLParam() = %q, want %q", result, tt.expectedSQL)
			}
			assertCollectedParams(t, pc, tt.wantParams)
		})
	}
}

func TestDataOperator_valueToSQLParam_PositionalStyle(t *testing.T) {
	op := NewDataOperator(nil)
	pc := params.NewParamCollector(params.PlaceholderPositional)

	s1, err := op.valueToSQLParam("first", pc)
	if err != nil {
		t.Fatalf("valueToSQLParam first: %v", err)
	}
	s2, err := op.valueToSQLParam(2, pc)
	if err != nil {
		t.Fatalf("valueToSQLParam second: %v", err)
	}

	if s1 != "$1" {
		t.Errorf("first placeholder = %q, want $1", s1)
	}
	if s2 != "$2" {
		t.Errorf("second placeholder = %q, want $2", s2)
	}
	assertCollectedParams(t, pc, []params.QueryParam{
		{Name: "p1", Value: "first"},
		{Name: "p2", Value: 2},
	})
}
