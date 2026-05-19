package operators

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

func testRuntimeStringContainmentSQL(d dialect.Dialect, haystack, needle string) string {
	var containment string
	switch d {
	case dialect.DialectPostgreSQL:
		containment = fmt.Sprintf("POSITION(%s IN %s) > 0", needle, haystack)
	case dialect.DialectClickHouse:
		containment = fmt.Sprintf("position(%s, %s) > 0", haystack, needle)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectDuckDB:
		containment = fmt.Sprintf("STRPOS(%s, %s) > 0", haystack, needle)
	}
	return fmt.Sprintf(
		"((%s = '' AND (%s IS NOT NULL AND %s != '')) OR (%s != '' AND %s))",
		needle,
		haystack,
		haystack,
		needle,
		containment,
	)
}

func TestComparisonOperator_ToSQL(t *testing.T) {
	op := NewComparisonOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		expected string
		hasError bool
	}{
		// Equality tests
		{
			name:     "equality with numbers",
			operator: "==",
			args:     []interface{}{1, 2},
			expected: "1 = 2",
			hasError: false,
		},
		{
			name:     "equality with strings",
			operator: "==",
			args:     []interface{}{"hello", "world"},
			expected: "'hello' = 'world'",
			hasError: false,
		},
		{
			name:     "equality with var and literal",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "status"}, "pending"},
			expected: "status = 'pending'",
			hasError: false,
		},
		{
			name:     "equality with dotted var",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "user.status"}, "active"},
			expected: "user.status = 'active'",
			hasError: false,
		},
		// Inequality tests
		{
			name:     "inequality with numbers",
			operator: "!=",
			args:     []interface{}{1, 2},
			expected: "1 != 2",
			hasError: false,
		},

		// Greater than tests
		{
			name:     "greater than with numbers",
			operator: ">",
			args:     []interface{}{5, 3},
			expected: "5 > 3",
			hasError: false,
		},
		{
			name:     "greater than with var",
			operator: ">",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, 1000},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name:     "greater than with dotted var",
			operator: ">",
			args:     []interface{}{map[string]interface{}{"var": "transaction.amount"}, 5000},
			expected: "transaction.amount > 5000",
			hasError: false,
		},

		// Greater than or equal tests
		{
			name:     "greater than or equal",
			operator: ">=",
			args:     []interface{}{5, 5},
			expected: "5 >= 5",
			hasError: false,
		},
		{
			name:     "greater than or equal with var",
			operator: ">=",
			args:     []interface{}{map[string]interface{}{"var": "failedAttempts"}, 5},
			expected: "failedAttempts >= 5",
			hasError: false,
		},

		// Less than tests
		{
			name:     "less than with numbers",
			operator: "<",
			args:     []interface{}{3, 5},
			expected: "3 < 5",
			hasError: false,
		},
		{
			name:     "less than with var",
			operator: "<",
			args:     []interface{}{map[string]interface{}{"var": "age"}, 18},
			expected: "age < 18",
			hasError: false,
		},

		// Less than or equal tests
		{
			name:     "less than or equal",
			operator: "<=",
			args:     []interface{}{5, 5},
			expected: "5 <= 5",
			hasError: false,
		},
		{
			name:     "less than or equal with var",
			operator: "<=",
			args:     []interface{}{map[string]interface{}{"var": "user.accountAgeDays"}, 7},
			expected: "user.accountAgeDays <= 7",
			hasError: false,
		},

		// Boolean tests
		{
			name:     "equality with booleans",
			operator: "==",
			args:     []interface{}{true, false},
			expected: "FALSE",
			hasError: false,
		},
		{
			name:     "equality with var and boolean",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "verified"}, false},
			expected: "verified = FALSE",
			hasError: false,
		},

		// Null tests
		{
			name:     "equality with null",
			operator: "==",
			args:     []interface{}{nil, nil},
			expected: "NULL IS NULL",
			hasError: false,
		},
		{
			name:     "inequality with null",
			operator: "!=",
			args:     []interface{}{map[string]interface{}{"var": "field"}, nil},
			expected: "field IS NOT NULL",
			hasError: false,
		},
		{
			name:     "equality with var and null",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "deleted_at"}, nil},
			expected: "deleted_at IS NULL",
			hasError: false,
		},
		{
			name:     "equality with null and var",
			operator: "==",
			args:     []interface{}{nil, map[string]interface{}{"var": "deleted_at"}},
			expected: "deleted_at IS NULL",
			hasError: false,
		},
		{
			name:     "strict equality with null",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "field"}, nil},
			expected: "field IS NULL",
			hasError: false,
		},
		{
			name:     "strict inequality with null",
			operator: "!==",
			args:     []interface{}{map[string]interface{}{"var": "field"}, nil},
			expected: "field IS NOT NULL",
			hasError: false,
		},

		// in operator tests
		{
			name:     "in with string array",
			operator: "in",
			args:     []interface{}{map[string]interface{}{"var": "country"}, []interface{}{"CN", "RU"}},
			expected: "country IN ('CN', 'RU')",
			hasError: false,
		},
		{
			name:     "in with number array",
			operator: "in",
			args:     []interface{}{map[string]interface{}{"var": "status"}, []interface{}{1, 2, 3}},
			expected: "status IN (1, 2, 3)",
			hasError: false,
		},
		{
			name:     "in with empty array",
			operator: "in",
			args:     []interface{}{map[string]interface{}{"var": "field"}, []interface{}{}},
			expected: "FALSE",
			hasError: false,
		},
		{
			name:     "in with string containment",
			operator: "in",
			args:     []interface{}{map[string]interface{}{"var": "field"}, "not-array"},
			expected: "STRPOS('not-array', field) > 0",
			hasError: false,
		},

		// Error cases
		{
			name:     "too few arguments",
			operator: "==",
			args:     []interface{}{1},
			expected: "",
			hasError: true,
		},
		{
			name:     "too many arguments",
			operator: "==",
			args:     []interface{}{1, 2, 3},
			expected: "",
			hasError: true,
		},
		{
			name:     "unsupported operator",
			operator: "unsupported",
			args:     []interface{}{1, 2},
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

func TestComparisonOperator_ToSQL_NullSafeFieldEquality(t *testing.T) {
	defaultConfig := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	defaultOp := NewComparisonOperator(defaultConfig)
	got, err := defaultOp.ToSQL("==", []interface{}{
		map[string]interface{}{"var": "a"},
		map[string]interface{}{"var": "b"},
	})
	if err != nil {
		t.Fatalf("ToSQL() default unexpected error = %v", err)
	}
	if want := "((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b))"; got != want {
		t.Fatalf("ToSQL() default = %q, want %q", got, want)
	}

	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		wantSQL  string
	}{
		{
			name:     "loose equality var to var",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				map[string]interface{}{"var": "b"},
			},
			wantSQL: "((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b))",
		},
		{
			name:     "strict equality var to var",
			operator: "===",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				map[string]interface{}{"var": "b"},
			},
			wantSQL: "((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b))",
		},
		{
			name:     "loose inequality var to var",
			operator: "!=",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				map[string]interface{}{"var": "b"},
			},
			wantSQL: "((a IS NULL AND b IS NOT NULL) OR (a IS NOT NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a != b))",
		},
		{
			name:     "strict inequality var to var",
			operator: "!==",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				map[string]interface{}{"var": "b"},
			},
			wantSQL: "((a IS NULL AND b IS NOT NULL) OR (a IS NOT NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a <> b))",
		},
		{
			name:     "defaulted vars use rendered operands",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": []interface{}{"a", "left-default"}},
				map[string]interface{}{"var": []interface{}{"b", "right-default"}},
			},
			wantSQL: "((COALESCE(a, 'left-default') IS NULL AND COALESCE(b, 'right-default') IS NULL) OR (COALESCE(a, 'left-default') IS NOT NULL AND COALESCE(b, 'right-default') IS NOT NULL AND COALESCE(a, 'left-default') = COALESCE(b, 'right-default')))",
		},
		{
			name:     "field literal remains unchanged",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				"x",
			},
			wantSQL: "a = 'x'",
		},
		{
			name:     "literal literal remains unchanged",
			operator: "==",
			args:     []interface{}{1, 1},
			wantSQL:  "1 = 1",
		},
		{
			name:     "field null remains null comparison",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				nil,
			},
			wantSQL: "a IS NULL",
		},
		{
			name:     "schema-required strict field literal remains unchanged",
			operator: "===",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				"1",
			},
			wantSQL: "a = '1'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := op.ToSQL(tt.operator, tt.args)
			if err != nil {
				t.Fatalf("ToSQL() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Fatalf("ToSQL() = %q, want %q", got, tt.wantSQL)
			}
		})
	}
}

func TestComparisonOperator_valueToSQL(t *testing.T) {
	op := NewComparisonOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "literal string",
			input:    "hello",
			expected: "'hello'",
			hasError: false,
		},
		{
			name:     "literal number",
			input:    42,
			expected: "42",
			hasError: false,
		},
		{
			name:     "literal boolean",
			input:    true,
			expected: "TRUE",
			hasError: false,
		},
		{
			name:     "var expression",
			input:    map[string]interface{}{"var": "amount"},
			expected: "amount",
			hasError: false,
		},
		{
			name:     "dotted var expression",
			input:    map[string]interface{}{"var": "user.name"},
			expected: "user.name",
			hasError: false,
		},
		{
			name:     "var with default",
			input:    map[string]interface{}{"var": []interface{}{"status", "pending"}},
			expected: "COALESCE(status, 'pending')",
			hasError: false,
		},
		{
			name:     "non-var object",
			input:    map[string]interface{}{"other": "value"},
			expected: "",
			hasError: true,
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
					t.Errorf("valueToSQL() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestComparisonOperator_strposFunc(t *testing.T) {
	tests := []struct {
		name     string
		dialect  dialect.Dialect
		haystack string
		needle   string
		expected string
	}{
		{
			name:     "BigQuery dialect",
			dialect:  dialect.DialectBigQuery,
			haystack: "description",
			needle:   "'test'",
			expected: "STRPOS(description, 'test')",
		},
		{
			name:     "Spanner dialect",
			dialect:  dialect.DialectSpanner,
			haystack: "description",
			needle:   "'test'",
			expected: "STRPOS(description, 'test')",
		},
		{
			name:     "DuckDB dialect",
			dialect:  dialect.DialectDuckDB,
			haystack: "description",
			needle:   "'test'",
			expected: "STRPOS(description, 'test')",
		},
		{
			name:     "PostgreSQL dialect",
			dialect:  dialect.DialectPostgreSQL,
			haystack: "description",
			needle:   "'test'",
			expected: "POSITION('test' IN description)",
		},
		{
			name:     "ClickHouse dialect",
			dialect:  dialect.DialectClickHouse,
			haystack: "description",
			needle:   "'test'",
			expected: "position(description, 'test')",
		},
		{
			name:     "Unspecified dialect defaults to STRPOS",
			dialect:  dialect.DialectUnspecified,
			haystack: "col",
			needle:   "'val'",
			expected: "STRPOS(col, 'val')",
		},
		{
			name:     "nil config defaults to STRPOS",
			dialect:  dialect.DialectUnspecified,
			haystack: "field",
			needle:   "'search'",
			expected: "STRPOS(field, 'search')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var config *OperatorConfig
			if tt.name != "nil config defaults to STRPOS" {
				config = NewOperatorConfig(tt.dialect, &fieldOnlySchemaProvider{})
			}
			op := NewComparisonOperator(config)
			result := op.strposFunc(tt.haystack, tt.needle)
			if result != tt.expected {
				t.Errorf("strposFunc() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestComparisonOperator_processArithmeticExpression(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name     string
		operator string
		args     interface{}
		expected string
		hasError bool
	}{
		{
			name:     "addition",
			operator: "+",
			args:     []interface{}{1, 2},
			expected: "(1 + 2)",
			hasError: false,
		},
		{
			name:     "subtraction",
			operator: "-",
			args:     []interface{}{5, 3},
			expected: "(5 - 3)",
			hasError: false,
		},
		{
			name:     "multiplication",
			operator: "*",
			args:     []interface{}{2, 4},
			expected: "(2 * 4)",
			hasError: false,
		},
		{
			name:     "division",
			operator: "/",
			args:     []interface{}{10, 2},
			expected: "(10 / 2)",
			hasError: false,
		},
		{
			name:     "modulo",
			operator: "%",
			args:     []interface{}{7, 3},
			expected: "(7 % 3)",
			hasError: false,
		},
		{
			name:     "unary minus (negation)",
			operator: "-",
			args:     []interface{}{42},
			expected: "(-42)",
			hasError: false,
		},
		{
			name:     "unary plus (cast to number)",
			operator: "+",
			args:     []interface{}{"42"},
			expected: "CAST('42' AS NUMERIC)",
			hasError: false,
		},
		{
			name:     "multiple operands addition",
			operator: "+",
			args:     []interface{}{1, 2, 3},
			expected: "(1 + 2 + 3)",
			hasError: false,
		},
		{
			name:     "unsupported operator",
			operator: "^",
			args:     []interface{}{2, 3},
			expected: "",
			hasError: true,
		},
		{
			name:     "non-array args",
			operator: "+",
			args:     "invalid",
			expected: "",
			hasError: true,
		},
		{
			name:     "insufficient args for binary op",
			operator: "*",
			args:     []interface{}{5},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.processArithmeticExpression(tt.operator, tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("processArithmeticExpression() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("processArithmeticExpression() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("processArithmeticExpression() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestComparisonOperator_processComparisonExpression(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name     string
		operator string
		args     interface{}
		expected string
		hasError bool
	}{
		{
			name:     "greater than",
			operator: ">",
			args:     []interface{}{5, 3},
			expected: "(5 > 3)",
			hasError: false,
		},
		{
			name:     "greater than or equal",
			operator: ">=",
			args:     []interface{}{5, 5},
			expected: "(5 >= 5)",
			hasError: false,
		},
		{
			name:     "less than",
			operator: "<",
			args:     []interface{}{3, 5},
			expected: "(3 < 5)",
			hasError: false,
		},
		{
			name:     "less than or equal",
			operator: "<=",
			args:     []interface{}{3, 3},
			expected: "(3 <= 3)",
			hasError: false,
		},
		{
			name:     "equality",
			operator: "==",
			args:     []interface{}{1, 1},
			expected: "(1 = 1)",
			hasError: false,
		},
		{
			name:     "strict equality",
			operator: "===",
			args:     []interface{}{1, 1},
			expected: "(1 = 1)",
			hasError: false,
		},
		{
			name:     "inequality",
			operator: "!=",
			args:     []interface{}{1, 2},
			expected: "(1 != 2)",
			hasError: false,
		},
		{
			name:     "strict inequality",
			operator: "!==",
			args:     []interface{}{1, 2},
			expected: "(1 <> 2)",
			hasError: false,
		},
		{
			name:     "unsupported comparison",
			operator: "<>",
			args:     []interface{}{1, 2},
			expected: "",
			hasError: true,
		},
		{
			name:     "non-array args",
			operator: ">",
			args:     "invalid",
			expected: "",
			hasError: true,
		},
		{
			name:     "wrong number of args",
			operator: ">",
			args:     []interface{}{1, 2, 3},
			expected: "",
			hasError: true,
		},
		{
			name:     "single arg (insufficient)",
			operator: ">",
			args:     []interface{}{1},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.processComparisonExpression(tt.operator, tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("processComparisonExpression() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("processComparisonExpression() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("processComparisonExpression() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestComparisonOperator_processMinMaxExpression(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name     string
		operator string
		args     interface{}
		expected string
		hasError bool
	}{
		{
			name:     "max with two args",
			operator: "max",
			args:     []interface{}{5, 10},
			expected: "GREATEST(5, 10)",
			hasError: false,
		},
		{
			name:     "max with three args",
			operator: "max",
			args:     []interface{}{5, 10, 15},
			expected: "GREATEST(5, 10, 15)",
			hasError: false,
		},
		{
			name:     "min with two args",
			operator: "min",
			args:     []interface{}{5, 10},
			expected: "LEAST(5, 10)",
			hasError: false,
		},
		{
			name:     "min with three args",
			operator: "min",
			args:     []interface{}{5, 10, 3},
			expected: "LEAST(5, 10, 3)",
			hasError: false,
		},
		{
			name:     "unsupported min/max operator",
			operator: "avg",
			args:     []interface{}{1, 2},
			expected: "",
			hasError: true,
		},
		{
			name:     "non-array args",
			operator: "max",
			args:     "invalid",
			expected: "",
			hasError: true,
		},
		{
			name:     "insufficient args",
			operator: "max",
			args:     []interface{}{5},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.processMinMaxExpression(tt.operator, tt.args)
			if tt.hasError {
				if err == nil {
					t.Errorf("processMinMaxExpression() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("processMinMaxExpression() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("processMinMaxExpression() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

// comparisonSchemaProvider is a configurable schema provider for comparison tests.
type comparisonSchemaProvider struct {
	fields      map[string]string   // field name -> type
	enumValues  map[string][]string // field name -> allowed values
	knownFields map[string]bool     // fields that exist
	validateErr error
}

func newComparisonSchemaProvider(fields map[string]string) *comparisonSchemaProvider {
	known := make(map[string]bool)
	for k := range fields {
		known[k] = true
	}
	return &comparisonSchemaProvider{
		fields:      fields,
		enumValues:  make(map[string][]string),
		knownFields: known,
	}
}

func (m *comparisonSchemaProvider) HasField(fieldName string) bool {
	return m.knownFields[fieldName]
}

func (m *comparisonSchemaProvider) GetFieldType(fieldName string) string {
	return m.fields[fieldName]
}

func (m *comparisonSchemaProvider) ValidateField(fieldName string) error {
	if m.validateErr != nil {
		return m.validateErr
	}
	if !m.HasField(fieldName) {
		return fmt.Errorf("field '%s' is not defined in schema", fieldName)
	}
	return nil
}

func (m *comparisonSchemaProvider) IsArrayType(fieldName string) bool {
	return m.fields[fieldName] == "array"
}

func (m *comparisonSchemaProvider) IsStringType(fieldName string) bool {
	return m.fields[fieldName] == "string"
}

func (m *comparisonSchemaProvider) IsNumericType(fieldName string) bool {
	t := m.fields[fieldName]
	return t == "integer" || t == "number"
}

func (m *comparisonSchemaProvider) IsBooleanType(fieldName string) bool {
	return m.fields[fieldName] == "boolean"
}

func (m *comparisonSchemaProvider) IsEnumType(fieldName string) bool {
	_, ok := m.enumValues[fieldName]
	return ok
}

func (m *comparisonSchemaProvider) GetAllowedValues(fieldName string) []string {
	return m.enumValues[fieldName]
}

func (m *comparisonSchemaProvider) ValidateEnumValue(fieldName, value string) error {
	allowed := m.enumValues[fieldName]
	for _, v := range allowed {
		if v == value {
			return nil
		}
	}
	return fmt.Errorf("invalid enum value '%s' for field '%s'", value, fieldName)
}

func TestJSNumberFromString(t *testing.T) {
	tests := []struct {
		input      string
		wantOK     bool
		wantString string
		wantInt    bool
	}{
		{input: "", wantOK: true, wantString: "0", wantInt: true},
		{input: " ", wantOK: true, wantString: "0", wantInt: true},
		{input: "  5  ", wantOK: true, wantString: "5", wantInt: true},
		{input: "010", wantOK: true, wantString: "10", wantInt: true},
		{input: "00010", wantOK: true, wantString: "10", wantInt: true},
		{input: "+5", wantOK: true, wantString: "5", wantInt: true},
		{input: "-5", wantOK: true, wantString: "-5", wantInt: true},
		{input: "5.", wantOK: true, wantString: "5", wantInt: true},
		{input: ".5", wantOK: true, wantString: "0.5", wantInt: false},
		{input: "1e-7", wantOK: true, wantString: "1e-7", wantInt: false},
		{input: "1e-6", wantOK: true, wantString: "0.000001", wantInt: false},
		{input: "1e20", wantOK: true, wantString: "100000000000000000000", wantInt: true},
		{input: "1e21", wantOK: true, wantString: "1e+21", wantInt: true},
		{input: "5e2", wantOK: true, wantString: "500", wantInt: true},
		{input: "9007199254740993", wantOK: true, wantString: "9007199254740992", wantInt: true},
		{input: "9223372036854775808", wantOK: true, wantString: "9223372036854776000", wantInt: true},
		{input: "0x10", wantOK: true, wantString: "16", wantInt: true},
		{input: "0X10", wantOK: true, wantString: "16", wantInt: true},
		{input: "0x7fffffffffffffff", wantOK: true, wantString: "9223372036854776000", wantInt: true},
		{input: "0x8000000000000000", wantOK: true, wantString: "9223372036854776000", wantInt: true},
		{input: "0xffffffffffffffff", wantOK: true, wantString: "18446744073709552000", wantInt: true},
		{input: "0o10", wantOK: true, wantString: "8", wantInt: true},
		{input: "0O10", wantOK: true, wantString: "8", wantInt: true},
		{input: "0o1000000000000000000000", wantOK: true, wantString: "9223372036854776000", wantInt: true},
		{input: "0b10", wantOK: true, wantString: "2", wantInt: true},
		{input: "0B10", wantOK: true, wantString: "2", wantInt: true},
		{input: "0b1000000000000000000000000000000000000000000000000000000000000000", wantOK: true, wantString: "9223372036854776000", wantInt: true},
		{input: "5x", wantOK: false},
		{input: ".", wantOK: false},
		{input: "0x1.5p3", wantOK: false},
		{input: "5_000", wantOK: false},
		{input: "NaN", wantOK: false},
		{input: "Inf", wantOK: false},
		{input: "Infinity", wantOK: false},
		{input: "+0x10", wantOK: false},
		{input: "-0x10", wantOK: false},
		{input: "+0o10", wantOK: false},
		{input: "-0b10", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := jsNumberFromString(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("jsNumberFromString(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.integral != tt.wantInt {
				t.Fatalf("jsNumberFromString(%q) integral = %v, want %v", tt.input, got.integral, tt.wantInt)
			}
			if gotString := jsNumberToCanonicalString(got); gotString != tt.wantString {
				t.Fatalf("jsNumberFromString(%q) canonical = %q, want %q", tt.input, gotString, tt.wantString)
			}
		})
	}
}

func TestFoldLiteralComparison_OverflowJSONNumbersAreUnknown(t *testing.T) {
	tests := []struct {
		name     string
		operator string
		args     []interface{}
	}{
		{
			name:     "equality",
			operator: "==",
			args:     []interface{}{json.Number("1e400"), json.Number("1e400")},
		},
		{
			name:     "inequality",
			operator: "!=",
			args:     []interface{}{json.Number("1e400"), json.Number("1e400")},
		},
		{
			name:     "strict equality",
			operator: "===",
			args:     []interface{}{json.Number("1e400"), json.Number("1e400")},
		},
		{
			name:     "ordering",
			operator: ">",
			args:     []interface{}{json.Number("1e400"), json.Number("1")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, known, err := FoldLiteralComparison(tt.operator, tt.args)
			if err != nil {
				t.Fatalf("FoldLiteralComparison() error = %v", err)
			}
			if known {
				t.Fatal("FoldLiteralComparison() known = true, want false")
			}
		})
	}
}

func TestFoldLiteralComparison_ArrayMembershipUsesStrictEquality(t *testing.T) {
	tests := []struct {
		name string
		args []interface{}
		want bool
	}{
		{
			name: "same number matches",
			args: []interface{}{float64(1), []interface{}{float64(1)}},
			want: true,
		},
		{
			name: "string number does not match number",
			args: []interface{}{"1", []interface{}{float64(1)}},
		},
		{
			name: "false does not match zero",
			args: []interface{}{false, []interface{}{float64(0)}},
		},
		{
			name: "null matches null",
			args: []interface{}{nil, []interface{}{nil}},
			want: true,
		},
		{
			name: "empty array haystack is false",
			args: []interface{}{"x", []interface{}{}},
		},
		{
			name: "numeric haystack is false",
			args: []interface{}{"3", float64(12345)},
		},
		{
			name: "boolean haystack is false",
			args: []interface{}{"true", true},
		},
		{
			name: "string haystack stringifies number needle",
			args: []interface{}{float64(3), "12345"},
			want: true,
		},
		{
			name: "string haystack stringifies boolean needle",
			args: []interface{}{true, "true"},
			want: true,
		},
		{
			name: "string haystack stringifies null needle",
			args: []interface{}{nil, "null"},
			want: true,
		},
		{
			name: "empty string haystack is false",
			args: []interface{}{"", ""},
		},
		{
			name: "empty string needle matches non-empty haystack",
			args: []interface{}{"", "x"},
			want: true,
		},
		{
			name: "array needle uses javascript string form",
			args: []interface{}{[]interface{}{float64(1), float64(2)}, "x1,2y"},
			want: true,
		},
		{
			name: "empty array needle is false with empty string haystack",
			args: []interface{}{[]interface{}{}, ""},
		},
		{
			name: "empty array needle matches non-empty string haystack",
			args: []interface{}{[]interface{}{}, "abc"},
			want: true,
		},
		{
			name: "string haystack keeps mismatched stringified number false",
			args: []interface{}{float64(0), "false"},
		},
		{
			name: "null haystack is false",
			args: []interface{}{"x", nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, known, err := FoldLiteralComparison("in", tt.args)
			if err != nil {
				t.Fatalf("FoldLiteralComparison() error = %v", err)
			}
			if !known {
				t.Fatal("FoldLiteralComparison() known = false, want true")
			}
			if got != tt.want {
				t.Fatalf("FoldLiteralComparison() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestComparisonOperator_ToSQL_EqualitySemanticsWithSchema(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"amount":         "integer",
		"score":          "number",
		"active":         "boolean",
		"code":           "string",
		"other":          "string",
		"status":         "enum",
		"limited_status": "enum",
	})
	schema.enumValues["status"] = []string{"active", "1", "1.2", "9223372036854775807", "Infinity", "-Infinity"}
	schema.enumValues["limited_status"] = []string{"active"}
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name      string
		operator  string
		args      []interface{}
		wantSQL   string
		wantError bool
	}{
		{
			name:     "numeric field trims string literal",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, " 5 "},
			wantSQL:  "amount = 5",
		},
		{
			name:     "numeric field parses leading zero as decimal",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "010"},
			wantSQL:  "amount = 10",
		},
		{
			name:     "numeric field parses hex string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "0x10"},
			wantSQL:  "amount = 16",
		},
		{
			name:     "numeric field parses octal string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "0o10"},
			wantSQL:  "amount = 8",
		},
		{
			name:     "numeric field parses binary string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "0b10"},
			wantSQL:  "amount = 2",
		},
		{
			name:     "integer field unsafe decimal string rounds like javascript",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "9007199254740993"},
			wantSQL:  "amount = 9007199254740992",
		},
		{
			name:     "numeric field invalid string folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "abc"},
			wantSQL:  "FALSE",
		},
		{
			name:     "string field preserves Go int64 literal exactly",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, int64(9223372036854775807)},
			wantSQL:  "code = '9223372036854775807'",
		},
		{
			name:     "enum field validates exact Go int64 string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "status"}, int64(9223372036854775807)},
			wantSQL:  "status = '9223372036854775807'",
		},
		{
			name:     "string field preserves Go float32 literal formatting",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, float32(1.2)},
			wantSQL:  "code = '1.2'",
		},
		{
			name:     "enum field validates Go float32 literal formatting",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "status"}, float32(1.2)},
			wantSQL:  "status = '1.2'",
		},
		{
			name:     "string field json overflow canonicalizes infinity",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e400")},
			wantSQL:  "code = 'Infinity'",
		},
		{
			name:     "string field json overflow canonicalizes infinity for not equal",
			operator: "!=",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e400")},
			wantSQL:  "code != 'Infinity'",
		},
		{
			name:     "string field json negative overflow canonicalizes negative infinity",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("-1e400")},
			wantSQL:  "code = '-Infinity'",
		},
		{
			name:     "string field json underflow canonicalizes zero",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e-400")},
			wantSQL:  "code = '0'",
		},
		{
			name:     "string field native infinity canonicalizes",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, math.Inf(1)},
			wantSQL:  "code = 'Infinity'",
		},
		{
			name:     "string field native NaN still folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, math.NaN()},
			wantSQL:  "FALSE",
		},
		{
			name:     "enum field validates infinity canonical string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "status"}, math.Inf(1)},
			wantSQL:  "status = 'Infinity'",
		},
		{
			name:      "enum field rejects infinity canonical string",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": "limited_status"}, math.Inf(1)},
			wantError: true,
		},
		{
			name:     "integer field preserves large uint64 literal within int64 range",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, uint64(9007199254740993)},
			wantSQL:  "amount = 9007199254740993",
		},
		{
			name:     "number field preserves large uint64 literal above int64 range",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, uint64(9223372036854775808)},
			wantSQL:  "score = 9223372036854775808",
		},
		{
			name:     "strict number field folds NaN false",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "score"}, math.NaN()},
			wantSQL:  "FALSE",
		},
		{
			name:     "strict number field folds NaN true for not equal",
			operator: "!==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, math.NaN()},
			wantSQL:  "TRUE",
		},
		{
			name:     "strict number field folds infinity false",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "score"}, math.Inf(1)},
			wantSQL:  "FALSE",
		},
		{
			name:     "strict number field folds infinity true for not equal",
			operator: "!==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, math.Inf(1)},
			wantSQL:  "TRUE",
		},
		{
			name:     "integer field uint64 above int64 range folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, uint64(9223372036854775808)},
			wantSQL:  "FALSE",
		},
		{
			name:      "malformed json number literal errors before strict fold",
			operator:  "!==",
			args:      []interface{}{map[string]interface{}{"var": "code"}, json.Number("0 OR 1=1")},
			wantError: true,
		},
		{
			name:     "defaulted numeric var skips equality folding",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"amount", "abc"}}, "abc"},
			wantSQL:  "COALESCE(amount, 'abc') = 'abc'",
		},
		{
			name:     "defaulted numeric var coerces numeric string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"amount", 0}}, "50"},
			wantSQL:  "COALESCE(amount, 0) = 50",
		},
		{
			name:     "defaulted numeric var coerces numeric string with field on right",
			operator: "==",
			args:     []interface{}{"50", map[string]interface{}{"var": []interface{}{"amount", 0}}},
			wantSQL:  "50 = COALESCE(amount, 0)",
		},
		{
			name:     "defaulted numeric var invalid string folds false when default cannot match",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"amount", 0}}, "abc"},
			wantSQL:  "FALSE",
		},
		{
			name:     "defaulted numeric var invalid string folds false with field on right when default cannot match",
			operator: "==",
			args:     []interface{}{"abc", map[string]interface{}{"var": []interface{}{"amount", 0}}},
			wantSQL:  "FALSE",
		},
		{
			name:     "defaulted strict numeric mismatch folds false when default cannot match",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"amount", 0}}, "50"},
			wantSQL:  "FALSE",
		},
		{
			name:     "defaulted strict numeric mismatch folds false with field on right when default cannot match",
			operator: "===",
			args:     []interface{}{"50", map[string]interface{}{"var": []interface{}{"amount", 0}}},
			wantSQL:  "FALSE",
		},
		{
			name:     "defaulted strict numeric mismatch preserves expression when default can match",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"amount", "50"}}, "50"},
			wantSQL:  "COALESCE(amount, '50') = '50'",
		},
		{
			name:     "defaulted strict numeric mismatch preserves expression with field on right when default can match",
			operator: "===",
			args:     []interface{}{"50", map[string]interface{}{"var": []interface{}{"amount", "50"}}},
			wantSQL:  "'50' = COALESCE(amount, '50')",
		},
		{
			name:      "defaulted string field boolean literal errors",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"code", ""}}, true},
			wantError: true,
		},
		{
			name:      "defaulted string field boolean literal errors with field on right",
			operator:  "==",
			args:      []interface{}{true, map[string]interface{}{"var": []interface{}{"code", ""}}},
			wantError: true,
		},
		{
			name:      "defaulted enum validates visible default",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"status", "unknown"}}, "active"},
			wantError: true,
		},
		{
			name:      "defaulted enum validates visible default before null equality",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"status", "unknown"}}, nil},
			wantError: true,
		},
		{
			name:      "defaulted enum validates visible default with var on right",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"status", "unknown"}}, map[string]interface{}{"var": "other"}},
			wantError: true,
		},
		{
			name:      "defaulted enum validates visible default with var on left",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": "other"}, map[string]interface{}{"var": []interface{}{"status", "unknown"}}},
			wantError: true,
		},
		{
			name:     "defaulted enum preserves valid default with var on right",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": []interface{}{"status", "active"}}, map[string]interface{}{"var": "other"}},
			wantSQL:  "((COALESCE(status, 'active') IS NULL AND other IS NULL) OR (COALESCE(status, 'active') IS NOT NULL AND other IS NOT NULL AND COALESCE(status, 'active') = other))",
		},
		{
			name:      "defaulted var malformed json number default errors before strict fold",
			operator:  "!==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"amount", json.Number("0 OR 1=1")}}, "abc"},
			wantError: true,
		},
		{
			name:      "defaulted var malformed json number default errors before loose fold",
			operator:  "!=",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"amount", json.Number("0 OR 1=1")}}, "abc"},
			wantError: true,
		},
		{
			name:     "numeric field invalid string folds true for not equal",
			operator: "!=",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "abc"},
			wantSQL:  "TRUE",
		},
		{
			name:     "integer field max int64 literal remains equality",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, json.Number("9223372036854775807")},
			wantSQL:  "amount = 9223372036854775807",
		},
		{
			name:     "integer field min int64 literal remains equality",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, json.Number("-9223372036854775808")},
			wantSQL:  "amount = -9223372036854775808",
		},
		{
			name:     "integer field min int64 string remains equality",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "-9223372036854775808"},
			wantSQL:  "amount = -9223372036854775808",
		},
		{
			name:     "integer field min int64 string remains inequality",
			operator: "!=",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "-9223372036854775808"},
			wantSQL:  "amount != -9223372036854775808",
		},
		{
			name:     "integer field above int64 range folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, json.Number("9223372036854775808")},
			wantSQL:  "FALSE",
		},
		{
			name:     "integer field below int64 range folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, json.Number("-9223372036854775809")},
			wantSQL:  "FALSE",
		},
		{
			name:     "integer field above int64 range folds true for not equal",
			operator: "!=",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, json.Number("9223372036854775808")},
			wantSQL:  "TRUE",
		},
		{
			name:     "strict integer field above int64 range folds false",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, json.Number("9223372036854775808")},
			wantSQL:  "FALSE",
		},
		{
			name:     "integer field fractional literal folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, 5.5},
			wantSQL:  "FALSE",
		},
		{
			name:     "number field fractional string remains numeric",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, ".5"},
			wantSQL:  "score = 0.5",
		},
		{
			name:     "number field oversized hex string remains numeric",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, "0x8000000000000000"},
			wantSQL:  "score = 9.223372036854776e+18",
		},
		{
			name:     "number field oversized binary string remains numeric",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, "0b1000000000000000000000000000000000000000000000000000000000000000"},
			wantSQL:  "score = 9.223372036854776e+18",
		},
		{
			name:     "number field large json number literal preserves exact text",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, json.Number("9223372036854775808")},
			wantSQL:  "score = 9223372036854775808",
		},
		{
			name:     "number field underflow json number literal preserves exact text",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "score"}, json.Number("1e-400")},
			wantSQL:  "score = 1e-400",
		},
		{
			name:     "integer field max int64 hex string folds false after javascript rounding",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "0x7fffffffffffffff"},
			wantSQL:  "FALSE",
		},
		{
			name:     "integer field oversized hex string folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "0x8000000000000000"},
			wantSQL:  "FALSE",
		},
		{
			name:     "boolean field numeric one",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "active"}, 1},
			wantSQL:  "active = TRUE",
		},
		{
			name:     "boolean field string zero",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "active"}, "0"},
			wantSQL:  "active = FALSE",
		},
		{
			name:     "boolean field empty string is false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "active"}, ""},
			wantSQL:  "active = FALSE",
		},
		{
			name:     "boolean field non boolean number folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "active"}, "2"},
			wantSQL:  "FALSE",
		},
		{
			name:     "boolean field non numeric string folds false",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "active"}, "true"},
			wantSQL:  "FALSE",
		},
		{
			name:     "string field string literal remains equality",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, "abc"},
			wantSQL:  "code = 'abc'",
		},
		{
			name:     "string field numeric literal canonicalizes",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("5.0")},
			wantSQL:  "code = '5'",
		},
		{
			name:     "string field small exponent canonicalizes like javascript",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e-7")},
			wantSQL:  "code = '1e-7'",
		},
		{
			name:     "string field decimal threshold canonicalizes like javascript",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e-6")},
			wantSQL:  "code = '0.000001'",
		},
		{
			name:     "string field large numeric literal canonicalizes like javascript",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "code"}, json.Number("9223372036854775808")},
			wantSQL:  "code = '9223372036854776000'",
		},
		{
			name:      "string field boolean literal errors",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": "code"}, true},
			wantError: true,
		},
		{
			name:     "enum field numeric literal validates as canonical string",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "status"}, 1},
			wantSQL:  "status = '1'",
		},
		{
			name:      "enum field boolean literal errors",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": "status"}, false},
			wantError: true,
		},
		{
			name:     "strict numeric field string literal folds false",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "5"},
			wantSQL:  "FALSE",
		},
		{
			name:     "strict numeric field string literal folds true for not equal",
			operator: "!==",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, "5"},
			wantSQL:  "TRUE",
		},
		{
			name:     "strict string field numeric literal folds false",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "code"}, 5},
			wantSQL:  "FALSE",
		},
		{
			name:     "strict boolean field numeric literal folds false",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "active"}, 1},
			wantSQL:  "FALSE",
		},
		{
			name:     "strict boolean field boolean literal remains equality",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "active"}, true},
			wantSQL:  "active = TRUE",
		},
		{
			name:     "null comparison remains authoritative",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, nil},
			wantSQL:  "amount IS NULL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := op.ToSQL(tt.operator, tt.args)
			if tt.wantError {
				if err == nil {
					t.Fatalf("ToSQL() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ToSQL() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Fatalf("ToSQL() = %q, want %q", got, tt.wantSQL)
			}
		})
	}
}

func TestComparisonOperator_ToSQL_NullSafeFieldEqualityPreservesSchemaLiteralSemantics(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"amount": "integer",
		"code":   "string",
	})
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		wantSQL  string
	}{
		{
			name:     "strict field literal mismatch still folds",
			operator: "===",
			args: []interface{}{
				map[string]interface{}{"var": "amount"},
				"5",
			},
			wantSQL: "FALSE",
		},
		{
			name:     "loose numeric invalid string still folds",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "amount"},
				"abc",
			},
			wantSQL: "FALSE",
		},
		{
			name:     "field null still emits IS NULL",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "amount"},
				nil,
			},
			wantSQL: "amount IS NULL",
		},
		{
			name:     "field literal coercion still applies",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "code"},
				5,
			},
			wantSQL: "code = '5'",
		},
		{
			name:     "field field uses null-safe fallback",
			operator: "===",
			args: []interface{}{
				map[string]interface{}{"var": "amount"},
				map[string]interface{}{"var": "code"},
			},
			wantSQL: "((amount IS NULL AND code IS NULL) OR (amount IS NOT NULL AND code IS NOT NULL AND amount = code))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := op.ToSQL(tt.operator, tt.args)
			if err != nil {
				t.Fatalf("ToSQL() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Fatalf("ToSQL() = %q, want %q", got, tt.wantSQL)
			}
		})
	}
}

func TestComparisonOperator_ToSQL_EqualityConstantFoldsValidateSchema(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"bad`field": "integer",
	})
	schema.validateErr = fmt.Errorf("schema field %q contains quote characters", "bad`field")
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []interface{}
	}{
		{
			name:     "loose impossible comparison validates schema",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "bad`field"}, "abc"},
		},
		{
			name:     "strict type mismatch validates schema",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "bad`field"}, "5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := op.ToSQL(tt.operator, tt.args)
			if err == nil {
				t.Fatalf("ToSQL() = %q, expected schema validation error", got)
			}
			if !strings.Contains(err.Error(), "contains quote characters") {
				t.Fatalf("ToSQL() error = %v, want schema validation error", err)
			}
		})
	}
}

func TestComparisonOperator_arrayMembershipSQL(t *testing.T) {
	tests := []struct {
		name     string
		dialect  dialect.Dialect
		valueSQL string
		arraySQL string
		expected string
	}{
		{
			name:     "BigQuery - IN UNNEST",
			dialect:  dialect.DialectBigQuery,
			valueSQL: "'test'",
			arraySQL: "tags",
			expected: "'test' IN UNNEST(tags)",
		},
		{
			name:     "Spanner - IN UNNEST",
			dialect:  dialect.DialectSpanner,
			valueSQL: "'test'",
			arraySQL: "tags",
			expected: "'test' IN UNNEST(tags)",
		},
		{
			name:     "PostgreSQL - ANY",
			dialect:  dialect.DialectPostgreSQL,
			valueSQL: "'test'",
			arraySQL: "tags",
			expected: "'test' = ANY(tags)",
		},
		{
			name:     "DuckDB - list_contains",
			dialect:  dialect.DialectDuckDB,
			valueSQL: "'test'",
			arraySQL: "tags",
			expected: "list_contains(tags, 'test')",
		},
		{
			name:     "ClickHouse - has",
			dialect:  dialect.DialectClickHouse,
			valueSQL: "'test'",
			arraySQL: "tags",
			expected: "has(tags, 'test')",
		},
		{
			name:     "Unspecified dialect - fallback to IN UNNEST",
			dialect:  dialect.DialectUnspecified,
			valueSQL: "42",
			arraySQL: "numbers",
			expected: "42 IN UNNEST(numbers)",
		},
		{
			name:     "nil config - fallback to IN UNNEST",
			dialect:  dialect.Dialect(0), // placeholder, will use nil config
			valueSQL: "'val'",
			arraySQL: "arr",
			expected: "'val' IN UNNEST(arr)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var op *ComparisonOperator
			if tt.name == "nil config - fallback to IN UNNEST" {
				op = NewComparisonOperator(testFieldOnlyConfig())
			} else {
				config := NewOperatorConfig(tt.dialect, &fieldOnlySchemaProvider{})
				op = NewComparisonOperator(config)
			}
			result := op.arrayMembershipSQL(tt.valueSQL, tt.arraySQL)
			if result != tt.expected {
				t.Errorf("arrayMembershipSQL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestComparisonOperator_coerceValueForComparison(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"age":         "integer",
		"price":       "number",
		"name":        "string",
		"description": "string",
		"is_active":   "boolean",
		"tags":        "array",
	})

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name      string
		value     interface{}
		fieldName string
		expected  interface{}
	}{
		// String-to-number coercion for numeric fields
		{
			name:      "string integer to numeric field",
			value:     "50000",
			fieldName: "age",
			expected:  int64(50000),
		},
		{
			name:      "string float to numeric field",
			value:     "3.14",
			fieldName: "price",
			expected:  float64(3.14),
		},
		{
			name:      "non-numeric string to numeric field (no coercion)",
			value:     "hello",
			fieldName: "age",
			expected:  "hello",
		},
		// Number-to-string coercion for string fields
		{
			name:      "float64 integer to string field",
			value:     float64(5960),
			fieldName: "name",
			expected:  "5960",
		},
		{
			name:      "float64 fractional to string field",
			value:     float64(3.14),
			fieldName: "name",
			expected:  "3.14",
		},
		{
			name:      "float32 to string field",
			value:     float32(1.5),
			fieldName: "name",
			expected:  "1.5",
		},
		{
			name:      "int to string field",
			value:     42,
			fieldName: "name",
			expected:  "42",
		},
		{
			name:      "int8 to string field",
			value:     int8(10),
			fieldName: "name",
			expected:  "10",
		},
		{
			name:      "int16 to string field",
			value:     int16(100),
			fieldName: "name",
			expected:  "100",
		},
		{
			name:      "int32 to string field",
			value:     int32(1000),
			fieldName: "name",
			expected:  "1000",
		},
		{
			name:      "int64 to string field",
			value:     int64(9999),
			fieldName: "name",
			expected:  "9999",
		},
		{
			name:      "uint to string field",
			value:     uint(7),
			fieldName: "name",
			expected:  "7",
		},
		{
			name:      "uint8 to string field",
			value:     uint8(8),
			fieldName: "name",
			expected:  "8",
		},
		{
			name:      "uint16 to string field",
			value:     uint16(16),
			fieldName: "name",
			expected:  "16",
		},
		{
			name:      "uint32 to string field",
			value:     uint32(32),
			fieldName: "name",
			expected:  "32",
		},
		{
			name:      "uint64 to string field",
			value:     uint64(64),
			fieldName: "name",
			expected:  "64",
		},
		// No coercion cases
		{
			name:      "empty field name returns value as-is",
			value:     "test",
			fieldName: "",
			expected:  "test",
		},
		{
			name:      "boolean field - no coercion",
			value:     "true",
			fieldName: "is_active",
			expected:  "true",
		},
		{
			name:      "array field - no coercion",
			value:     "something",
			fieldName: "tags",
			expected:  "something",
		},
		{
			name:      "number already a number for numeric field",
			value:     42,
			fieldName: "age",
			expected:  42,
		},
		{
			name:      "string already a string for string field",
			value:     "hello",
			fieldName: "name",
			expected:  "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := op.coerceValueForComparison(tt.value, tt.fieldName)
			if fmt.Sprintf("%v", result) != fmt.Sprintf("%v", tt.expected) {
				t.Errorf("coerceValueForComparison() = %v (%T), want %v (%T)", result, result, tt.expected, tt.expected)
			}
		})
	}

	// A field-only schema has no type metadata, so no coercion is applied.
	opFieldOnly := NewComparisonOperator(testFieldOnlyConfig())
	result := opFieldOnly.coerceValueForComparison("50000", "age")
	if result != "50000" {
		t.Errorf("coerceValueForComparison() with field-only schema = %v, want '50000'", result)
	}
}

func TestComparisonOperator_validateEnumValue(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"status":  "string",
		"country": "string",
		"age":     "integer",
	})
	schema.enumValues["status"] = []string{"active", "inactive", "pending"}
	schema.enumValues["country"] = []string{"US", "UK", "JP"}

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name      string
		value     interface{}
		fieldName string
		hasError  bool
	}{
		{
			name:      "valid enum value",
			value:     "active",
			fieldName: "status",
			hasError:  false,
		},
		{
			name:      "another valid enum value",
			value:     "pending",
			fieldName: "status",
			hasError:  false,
		},
		{
			name:      "invalid enum value",
			value:     "deleted",
			fieldName: "status",
			hasError:  true,
		},
		{
			name:      "valid country enum value",
			value:     "US",
			fieldName: "country",
			hasError:  false,
		},
		{
			name:      "invalid country enum value",
			value:     "XX",
			fieldName: "country",
			hasError:  true,
		},
		{
			name:      "null value skips validation",
			value:     nil,
			fieldName: "status",
			hasError:  false,
		},
		{
			name:      "non-enum field skips validation",
			value:     "anything",
			fieldName: "age",
			hasError:  false,
		},
		{
			name:      "empty field name skips validation",
			value:     "something",
			fieldName: "",
			hasError:  false,
		},
		{
			name:      "non-string value converted to string for validation",
			value:     123,
			fieldName: "status",
			hasError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := op.validateEnumValue(tt.value, tt.fieldName)
			if tt.hasError {
				if err == nil {
					t.Errorf("validateEnumValue() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("validateEnumValue() unexpected error = %v", err)
				}
			}
		})
	}

	// A field-only schema has no enum metadata, so enum validation is a no-op.
	opFieldOnly := NewComparisonOperator(testFieldOnlyConfig())
	if err := opFieldOnly.validateEnumValue("anything", "status"); err != nil {
		t.Errorf("validateEnumValue() with field-only schema should return nil, got %v", err)
	}
}

func TestComparisonOperator_extractFieldName(t *testing.T) {
	op := NewComparisonOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		varName  interface{}
		expected string
	}{
		{
			name:     "string var name",
			varName:  "fieldName",
			expected: "fieldName",
		},
		{
			name:     "array with string first element",
			varName:  []interface{}{"fieldName", "default"},
			expected: "fieldName",
		},
		{
			name:     "array with non-string first element",
			varName:  []interface{}{123, "default"},
			expected: "",
		},
		{
			name:     "empty array",
			varName:  []interface{}{},
			expected: "",
		},
		{
			name:     "numeric var name",
			varName:  42,
			expected: "",
		},
		{
			name:     "nil var name",
			varName:  nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := op.extractFieldName(tt.varName)
			if result != tt.expected {
				t.Errorf("extractFieldName() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestComparisonOperator_valueToSQL_Extended(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		// ProcessedValue - SQL
		{
			name:     "ProcessedValue with SQL",
			input:    ProcessedValue{Value: "some_column > 5", IsSQL: true},
			expected: "some_column > 5",
			hasError: false,
		},
		// ProcessedValue - Literal
		{
			name:     "ProcessedValue with literal string",
			input:    ProcessedValue{Value: "hello", IsSQL: false},
			expected: "'hello'",
			hasError: false,
		},
		// nil value
		{
			name:     "nil value",
			input:    nil,
			expected: "NULL",
			hasError: false,
		},
		// boolean false
		{
			name:     "boolean false",
			input:    false,
			expected: "FALSE",
			hasError: false,
		},
		// empty var name (current element reference)
		{
			name:     "empty var name returns elem",
			input:    map[string]interface{}{"var": ""},
			expected: "elem",
			hasError: false,
		},
		// Arithmetic expression inside comparison
		{
			name:     "addition expression",
			input:    map[string]interface{}{"+": []interface{}{map[string]interface{}{"var": "x"}, 10}},
			expected: "(x + 10)",
			hasError: false,
		},
		{
			name:     "subtraction expression",
			input:    map[string]interface{}{"-": []interface{}{map[string]interface{}{"var": "x"}, 5}},
			expected: "(x - 5)",
			hasError: false,
		},
		{
			name:     "multiplication expression",
			input:    map[string]interface{}{"*": []interface{}{map[string]interface{}{"var": "x"}, 2}},
			expected: "(x * 2)",
			hasError: false,
		},
		{
			name:     "division expression",
			input:    map[string]interface{}{"/": []interface{}{map[string]interface{}{"var": "x"}, 2}},
			expected: "(x / 2)",
			hasError: false,
		},
		{
			name:     "modulo expression",
			input:    map[string]interface{}{"%": []interface{}{map[string]interface{}{"var": "x"}, 3}},
			expected: "(x % 3)",
			hasError: false,
		},
		// Comparison expression inside comparison
		{
			name:     "nested greater than expression",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "a"}, 5}},
			expected: "(a > 5)",
			hasError: false,
		},
		// Min/Max expression
		{
			name:     "max expression",
			input:    map[string]interface{}{"max": []interface{}{map[string]interface{}{"var": "a"}, 100}},
			expected: "GREATEST(a, 100)",
			hasError: false,
		},
		{
			name:     "min expression",
			input:    map[string]interface{}{"min": []interface{}{map[string]interface{}{"var": "a"}, 0}},
			expected: "LEAST(a, 0)",
			hasError: false,
		},
		// If expression
		{
			name: "if expression",
			input: map[string]interface{}{"if": []interface{}{
				map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "x"}, 0}},
				"positive",
				"negative",
			}},
			expected: "CASE WHEN x > 0 THEN 'positive' ELSE 'negative' END",
			hasError: false,
		},
		{
			name:     "if expression non-array args error",
			input:    map[string]interface{}{"if": "invalid"},
			expected: "",
			hasError: true,
		},
		// Cat/Substr string operations
		{
			name:     "cat expression",
			input:    map[string]interface{}{"cat": []interface{}{"hello", " ", "world"}},
			expected: "CONCAT('hello', ' ', 'world')",
			hasError: false,
		},
		{
			name:     "cat expression non-array error",
			input:    map[string]interface{}{"cat": "invalid"},
			expected: "",
			hasError: true,
		},
		{
			name:     "substr expression",
			input:    map[string]interface{}{"substr": []interface{}{"hello", 1, 3}},
			expected: "SUBSTR('hello', 2, 3)",
			hasError: false,
		},
		{
			name:     "substr expression non-array error",
			input:    map[string]interface{}{"substr": "invalid"},
			expected: "",
			hasError: true,
		},
		// Array should error
		{
			name:     "array value should error",
			input:    []interface{}{1, 2, 3},
			expected: "",
			hasError: true,
		},
		// Array operators
		{
			name: "reduce expression non-array error",
			input: map[string]interface{}{
				"reduce": "invalid",
			},
			expected: "",
			hasError: true,
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

func TestComparisonOperator_handleIn_WithVarRightSide(t *testing.T) {
	// Test in with var on right side and schema indicating array type
	schema := newComparisonSchemaProvider(map[string]string{
		"tags":        "array",
		"description": "string",
		"name":        "string",
	})

	tests := []struct {
		name     string
		dialect  dialect.Dialect
		leftArg  interface{}
		rightArg interface{}
		expected string
		hasError bool
	}{
		// Array field on right side: use arrayMembershipSQL
		{
			name:     "BigQuery in with array var on right",
			dialect:  dialect.DialectBigQuery,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "tags"},
			expected: "'test' IN UNNEST(tags)",
			hasError: false,
		},
		{
			name:     "PostgreSQL in with array var on right",
			dialect:  dialect.DialectPostgreSQL,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "tags"},
			expected: "'test' = ANY(tags)",
			hasError: false,
		},
		{
			name:     "DuckDB in with array var on right",
			dialect:  dialect.DialectDuckDB,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "tags"},
			expected: "list_contains(tags, 'test')",
			hasError: false,
		},
		{
			name:     "ClickHouse in with array var on right",
			dialect:  dialect.DialectClickHouse,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "tags"},
			expected: "has(tags, 'test')",
			hasError: false,
		},
		// String field on right side: use STRPOS
		{
			name:     "BigQuery in with string var on right",
			dialect:  dialect.DialectBigQuery,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "description"},
			expected: "STRPOS(description, 'test') > 0",
			hasError: false,
		},
		{
			name:     "PostgreSQL in with string var on right",
			dialect:  dialect.DialectPostgreSQL,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "description"},
			expected: "POSITION('test' IN description) > 0",
			hasError: false,
		},
		{
			name:     "ClickHouse in with string var on right",
			dialect:  dialect.DialectClickHouse,
			leftArg:  "test",
			rightArg: map[string]interface{}{"var": "description"},
			expected: "position(description, 'test') > 0",
			hasError: false,
		},
		// Non-container right-hand literals are known false in JSONLogic.
		{
			name:     "in with number on right side folds false",
			dialect:  dialect.DialectBigQuery,
			leftArg:  "3",
			rightArg: float64(12345),
			expected: "FALSE",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := NewOperatorConfig(tt.dialect, schema)
			op := NewComparisonOperator(config)
			result, err := op.ToSQL("in", []interface{}{tt.leftArg, tt.rightArg})
			if tt.hasError {
				if err == nil {
					t.Errorf("ToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("ToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("ToSQL() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestComparisonOperator_handleIn_SchemaRequired_VarRightSide(t *testing.T) {
	// With field-only schema metadata, test heuristic based on left side being a literal.
	op := NewComparisonOperator(testFieldOnlyConfig())

	tests := []struct {
		name     string
		leftArg  interface{}
		rightArg interface{}
		expected string
		hasError bool
	}{
		{
			name:     "literal string left and var right - string containment",
			leftArg:  "search",
			rightArg: map[string]interface{}{"var": "field"},
			expected: "STRPOS(field, 'search') > 0",
			hasError: false,
		},
		{
			name:     "var left and var right - array membership fallback",
			leftArg:  map[string]interface{}{"var": "item"},
			rightArg: map[string]interface{}{"var": "collection"},
			expected: "item IN UNNEST(collection)",
			hasError: false,
		},
		{
			name:     "boolean right side folds false",
			leftArg:  "test",
			rightArg: true,
			expected: "FALSE",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := op.ToSQL("in", []interface{}{tt.leftArg, tt.rightArg})
			if tt.hasError {
				if err == nil {
					t.Errorf("ToSQL() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("ToSQL() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("ToSQL() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestComparisonOperator_handleIn_SchemaRequired_StringExpressionHeuristic(t *testing.T) {
	leftStringExpr := map[string]interface{}{
		OpCat: []interface{}{
			map[string]interface{}{
				OpSubstr: []interface{}{
					map[string]interface{}{OpVar: "profile.first"},
					float64(0),
					float64(2),
				},
			},
			"-x",
		},
	}
	leftNumericExpr := map[string]interface{}{
		"+": []interface{}{float64(1), float64(2)},
	}
	rightVar := map[string]interface{}{OpVar: "profile.name"}

	tests := []struct {
		name          string
		d             dialect.Dialect
		leftArg       interface{}
		expectedSQL   string
		expectedError bool
	}{
		{
			name:    "bigquery nested string expression uses containment",
			d:       dialect.DialectBigQuery,
			leftArg: leftStringExpr,
			expectedSQL: testRuntimeStringContainmentSQL(
				dialect.DialectBigQuery,
				"profile.name",
				"CONCAT(COALESCE(SUBSTR(profile.first, 1, 2), ''), '-x')",
			),
		},
		{
			name:    "postgres nested string expression uses containment",
			d:       dialect.DialectPostgreSQL,
			leftArg: leftStringExpr,
			expectedSQL: testRuntimeStringContainmentSQL(
				dialect.DialectPostgreSQL,
				"profile.name",
				"CONCAT(COALESCE(SUBSTR(profile.first, 1, 2), ''), '-x')",
			),
		},
		{
			name:    "clickhouse nested string expression uses containment",
			d:       dialect.DialectClickHouse,
			leftArg: leftStringExpr,
			expectedSQL: testRuntimeStringContainmentSQL(
				dialect.DialectClickHouse,
				"profile.name",
				"CONCAT(COALESCE(substring(profile.first, 1, 2), ''), '-x')",
			),
		},
		{
			name:        "bigquery numeric expression still uses membership fallback",
			d:           dialect.DialectBigQuery,
			leftArg:     leftNumericExpr,
			expectedSQL: "(1 + 2) IN UNNEST(profile.name)",
		},
		{
			name:        "postgres numeric expression still uses membership fallback",
			d:           dialect.DialectPostgreSQL,
			leftArg:     leftNumericExpr,
			expectedSQL: "(1 + 2) = ANY(profile.name)",
		},
		{
			name:        "clickhouse numeric expression still uses membership fallback",
			d:           dialect.DialectClickHouse,
			leftArg:     leftNumericExpr,
			expectedSQL: "has(profile.name, (1 + 2))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := NewComparisonOperator(NewOperatorConfig(tt.d, &fieldOnlySchemaProvider{}))
			got, err := op.ToSQL("in", []interface{}{tt.leftArg, rightVar})
			if tt.expectedError {
				if err == nil {
					t.Fatal("ToSQL() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ToSQL() error = %v", err)
			}
			if got != tt.expectedSQL {
				t.Fatalf("ToSQL() = %q, want %q", got, tt.expectedSQL)
			}
		})
	}
}

func TestComparisonOperator_handleIn_WithEnumValidation(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"status": "string",
	})
	schema.enumValues["status"] = []string{"active", "inactive", "pending"}

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	// Valid enum values in array
	result, err := op.ToSQL("in", []interface{}{
		map[string]interface{}{"var": "status"},
		[]interface{}{"active", "pending"},
	})
	if err != nil {
		t.Errorf("ToSQL() unexpected error = %v", err)
	}
	expected := "status IN ('active', 'pending')"
	if result != expected {
		t.Errorf("ToSQL() = %v, want %v", result, expected)
	}

	// Invalid enum values in array
	_, err = op.ToSQL("in", []interface{}{
		map[string]interface{}{"var": "status"},
		[]interface{}{"active", "deleted"},
	})
	if err == nil {
		t.Errorf("ToSQL() expected error for invalid enum value, got nil")
	}
}

func TestComparisonOperator_ToSQL_WithSchemaCoercion(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"age":  "integer",
		"name": "string",
	})

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []interface{}
		expected string
		hasError bool
	}{
		{
			name:     "string to number coercion - left field numeric, right literal string",
			operator: ">=",
			args:     []interface{}{map[string]interface{}{"var": "age"}, "50000"},
			expected: "age >= 50000",
			hasError: false,
		},
		{
			name:     "number to string coercion - left field string, right literal number",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "name"}, float64(5960)},
			expected: "name = '5960'",
			hasError: false,
		},
		{
			name:     "coercion for right field and left literal",
			operator: "==",
			args:     []interface{}{"50000", map[string]interface{}{"var": "age"}},
			expected: "50000 = age",
			hasError: false,
		},
		{
			name:     "coercion for right field string and left literal number",
			operator: "==",
			args:     []interface{}{float64(42), map[string]interface{}{"var": "name"}},
			expected: "'42' = name",
			hasError: false,
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
					t.Errorf("ToSQL() = %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func TestComparisonOperator_handleIn_WithSchemaArrayVar(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"tags": "array",
	})

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	// Test in with array-type field on right side (array membership)
	result, err := op.ToSQL("in", []interface{}{
		"test",
		map[string]interface{}{"var": "tags"},
	})
	if err != nil {
		t.Errorf("ToSQL() unexpected error = %v", err)
	}
	expected := "'test' IN UNNEST(tags)"
	if result != expected {
		t.Errorf("ToSQL() = %v, want %v", result, expected)
	}

	result, err = op.ToSQL("in", []interface{}{
		map[string]interface{}{"var": "tags"},
		[]interface{}{"test", "other"},
	})
	if err != nil {
		t.Errorf("ToSQL() unexpected error for array-valued left operand = %v", err)
	}
	if expected := "FALSE"; result != expected {
		t.Errorf("ToSQL() = %v, want %v", result, expected)
	}
}

func TestComparisonOperator_valueToSQL_ExpressionParserCallback(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	config.SetExpressionParser(func(expr any, path string) (string, error) {
		return "CUSTOM_FUNC()", nil
	})
	op := NewComparisonOperator(config)

	// Unknown operator with expression parser set should delegate
	result, err := op.valueToSQL(map[string]interface{}{"customOp": []interface{}{1, 2}})
	if err != nil {
		t.Errorf("valueToSQL() unexpected error = %v", err)
	}
	if result != "CUSTOM_FUNC()" {
		t.Errorf("valueToSQL() = %v, want CUSTOM_FUNC()", result)
	}
}

func TestComparisonOperator_handleIn_StringCoercion(t *testing.T) {
	// Test that number left side gets coerced to string when right side is a string field
	schema := newComparisonSchemaProvider(map[string]string{
		"name": "string",
	})

	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	// Number left side with string var on right - should coerce number to string
	result, err := op.ToSQL("in", []interface{}{
		float64(123),
		map[string]interface{}{"var": "name"},
	})
	if err != nil {
		t.Errorf("ToSQL() unexpected error = %v", err)
	}
	expected := "STRPOS(name, '123') > 0"
	if result != expected {
		t.Errorf("ToSQL() = %v, want %v", result, expected)
	}
}

func assertQueryParams(t *testing.T, got, want []params.QueryParam) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("Params() len %d, want %d\ngot: %#v", len(got), len(want), got)
	}
	for i := range got {
		if got[i].Name != want[i].Name || !reflect.DeepEqual(got[i].Value, want[i].Value) {
			t.Fatalf("Params()[%d] = {Name:%q Value:%#v (%T)}, want {Name:%q Value:%#v (%T)}",
				i, got[i].Name, got[i].Value, got[i].Value,
				want[i].Name, want[i].Value, want[i].Value)
		}
	}
}

func TestComparisonOperator_ToSQLParam(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		operator   string
		args       []interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:     "equality with var and literal",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "status"}, "pending"},
			wantSQL:  "status = @p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "pending"},
			},
		},
		{
			name:     "equality with numbers",
			operator: "==",
			args:     []interface{}{1, 2},
			wantSQL:  "@p1 = @p2",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 1},
				{Name: "p2", Value: 2},
			},
		},
		{
			name:       "equality with null",
			operator:   "==",
			args:       []interface{}{nil, nil},
			wantSQL:    "NULL IS NULL",
			wantParams: nil,
		},
		{
			name:       "var and null",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "field"}, nil},
			wantSQL:    "field IS NULL",
			wantParams: nil,
		},
		{
			name:       "inequality with var and null",
			operator:   "!=",
			args:       []interface{}{map[string]interface{}{"var": "field"}, nil},
			wantSQL:    "field IS NOT NULL",
			wantParams: nil,
		},
		{
			name:       "strict equality !==  with null",
			operator:   "!==",
			args:       []interface{}{map[string]interface{}{"var": "field"}, nil},
			wantSQL:    "field IS NOT NULL",
			wantParams: nil,
		},
		{
			name:     "greater than with var and number",
			operator: ">",
			args:     []interface{}{map[string]interface{}{"var": "amount"}, 1000},
			wantSQL:  "amount > @p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 1000},
			},
		},
		{
			name:     "in with array",
			operator: "in",
			args:     []interface{}{map[string]interface{}{"var": "country"}, []interface{}{"CN", "RU"}},
			wantSQL:  "country IN (@p1, @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "CN"},
				{Name: "p2", Value: "RU"},
			},
		},
		{
			name:     "chained comparison",
			operator: "<",
			args: []interface{}{
				10,
				map[string]interface{}{"var": "age"},
				30,
			},
			wantSQL: "(@p1 < age AND age < @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 10},
				{Name: "p2", Value: 30},
			},
		},
		{
			name:     "too few arguments",
			operator: "==",
			args:     []interface{}{1},
			wantErr:  true,
		},
		{
			name:     "in with empty array",
			operator: "in",
			args:     []interface{}{map[string]interface{}{"var": "field"}, []interface{}{}},
			wantSQL:  "FALSE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ToSQLParam() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ToSQLParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("ToSQLParam() SQL = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_ToSQLParam_NullSafeFieldEquality(t *testing.T) {
	defaultConfig := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	defaultOp := NewComparisonOperator(defaultConfig)
	pc := params.NewParamCollector(params.PlaceholderNamed)
	got, err := defaultOp.ToSQLParam("==", []interface{}{
		map[string]interface{}{"var": "a"},
		map[string]interface{}{"var": "b"},
	}, pc)
	if err != nil {
		t.Fatalf("ToSQLParam() default unexpected error = %v", err)
	}
	if want := "((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b))"; got != want {
		t.Fatalf("ToSQLParam() default = %q, want %q", got, want)
	}
	assertQueryParams(t, pc.Params(), nil)

	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		operator   string
		args       []interface{}
		wantSQL    string
		wantParams []params.QueryParam
	}{
		{
			name:     "loose equality var to var has no params",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				map[string]interface{}{"var": "b"},
			},
			wantSQL:    "((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b))",
			wantParams: nil,
		},
		{
			name:     "strict equality var to var has no params",
			operator: "===",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				map[string]interface{}{"var": "b"},
			},
			wantSQL:    "((a IS NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a = b))",
			wantParams: nil,
		},
		{
			name:     "loose inequality var to var has no params",
			operator: "!=",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				map[string]interface{}{"var": "b"},
			},
			wantSQL:    "((a IS NULL AND b IS NOT NULL) OR (a IS NOT NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a != b))",
			wantParams: nil,
		},
		{
			name:     "strict inequality var to var has no params",
			operator: "!==",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				map[string]interface{}{"var": "b"},
			},
			wantSQL:    "((a IS NULL AND b IS NOT NULL) OR (a IS NOT NULL AND b IS NULL) OR (a IS NOT NULL AND b IS NOT NULL AND a <> b))",
			wantParams: nil,
		},
		{
			name:     "defaulted vars preserve default param ordering",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": []interface{}{"a", "left-default"}},
				map[string]interface{}{"var": []interface{}{"b", "right-default"}},
			},
			wantSQL: "((COALESCE(a, @p1) IS NULL AND COALESCE(b, @p2) IS NULL) OR (COALESCE(a, @p1) IS NOT NULL AND COALESCE(b, @p2) IS NOT NULL AND COALESCE(a, @p1) = COALESCE(b, @p2)))",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "left-default"},
				{Name: "p2", Value: "right-default"},
			},
		},
		{
			name:     "field literal remains parameterized comparison",
			operator: "==",
			args: []interface{}{
				map[string]interface{}{"var": "a"},
				"x",
			},
			wantSQL: "a = @p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "x"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if err != nil {
				t.Fatalf("ToSQLParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Fatalf("ToSQLParam() = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_valueToSQLParam(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		input      interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:    "literal string",
			input:   "hello",
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "hello"},
			},
		},
		{
			name:    "literal number",
			input:   42,
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 42},
			},
		},
		{
			name:       "literal boolean",
			input:      true,
			wantSQL:    "TRUE",
			wantParams: nil,
		},
		{
			name:       "var expression",
			input:      map[string]interface{}{"var": "amount"},
			wantSQL:    "amount",
			wantParams: nil,
		},
		{
			name:       "ProcessedValue SQL",
			input:      ProcessedValue{Value: "some_col + 1", IsSQL: true},
			wantSQL:    "some_col + 1",
			wantParams: nil,
		},
		{
			name:    "ProcessedValue literal",
			input:   ProcessedValue{Value: "inner", IsSQL: false},
			wantSQL: "@p1",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: "inner"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.valueToSQLParam(tt.input, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("valueToSQLParam() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("valueToSQLParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("valueToSQLParam() = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_handleInParam(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"tags":        "array",
		"description": "string",
		"region":      "string",
		"amount":      "number",
		"flag":        "boolean",
	})
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	t.Run("array type var on right uses arrayMembershipSQL", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam("needle", map[string]interface{}{"var": "tags"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "@p1 IN UNNEST(tags)"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{{Name: "p1", Value: "needle"}})
	})

	t.Run("array type var on left with literal list folds false", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(map[string]interface{}{"var": "tags"}, []interface{}{"needle", "other"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		if want := "FALSE"; got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), nil)
	})

	t.Run("string type var on right uses strposFunc with parameterized left", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam("probe", map[string]interface{}{"var": "description"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "STRPOS(description, @p1) > 0"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{{Name: "p1", Value: "probe"}})
	})

	t.Run("array of literals parameterized", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		leftOriginal := map[string]interface{}{"var": "region"}
		got, err := op.handleInParam(leftOriginal, []interface{}{"EU", "APAC", "US"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "region IN (@p1, @p2, @p3)"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{
			{Name: "p1", Value: "EU"},
			{Name: "p2", Value: "APAC"},
			{Name: "p3", Value: "US"},
		})
	})

	t.Run("numeric right-hand literal folds false without params", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(map[string]interface{}{"var": "region"}, float64(12345), pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		if want := "FALSE"; got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), nil)
	})

	t.Run("empty right-hand array folds false without params", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(map[string]interface{}{"var": "region"}, []interface{}{}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		if want := "FALSE"; got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), nil)
	})

	t.Run("string containment with field-only schema", func(t *testing.T) {
		fieldOnlyConfig := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
		fieldOnlyOp := NewComparisonOperator(fieldOnlyConfig)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := fieldOnlyOp.handleInParam("foo", map[string]interface{}{"var": "bar"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "STRPOS(bar, @p1) > 0"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{{Name: "p1", Value: "foo"}})
	})

	t.Run("schema coercion for string field", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(float64(123), map[string]interface{}{"var": "description"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "STRPOS(description, @p1) > 0"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		if len(pc.Params()) != 1 {
			t.Fatalf("expected 1 param, got %d: %v", len(pc.Params()), pc.Params())
		}
		if pc.Params()[0].Value != "123" {
			t.Errorf("param value = %v (%T), want string \"123\"", pc.Params()[0].Value, pc.Params()[0].Value)
		}
	})

	t.Run("schema numeric field needle casts without unused left param", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(map[string]interface{}{"var": "amount"}, "12345", pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := testRuntimeStringContainmentSQL(
			dialect.DialectBigQuery,
			"@p1",
			"COALESCE(CAST(amount AS STRING), 'null')",
		)
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{{Name: "p1", Value: "12345"}})
	})

	t.Run("schema boolean field needle stringifies without unused left param", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(map[string]interface{}{"var": "flag"}, "true", pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := testRuntimeStringContainmentSQL(
			dialect.DialectBigQuery,
			"@p1",
			"CASE WHEN flag IS TRUE THEN 'true' WHEN flag IS FALSE THEN 'false' ELSE 'null' END",
		)
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{{Name: "p1", Value: "true"}})
	})

	t.Run("literal number needle stringifies for string haystack", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam(float64(3), "12345", pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "STRPOS(@p1, @p2) > 0"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{
			{Name: "p1", Value: "12345"},
			{Name: "p2", Value: "3"},
		})
	})

	t.Run("empty string needle checks haystack is non-empty without params", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam("", map[string]interface{}{"var": "description"}, pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "(description IS NOT NULL AND description != '')"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), nil)
	})

	t.Run("array literal needle stringifies for string haystack", func(t *testing.T) {
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := op.handleInParam([]interface{}{float64(1), float64(2)}, "x1,2y", pc)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "STRPOS(@p1, @p2) > 0"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
		assertQueryParams(t, pc.Params(), []params.QueryParam{
			{Name: "p1", Value: "x1,2y"},
			{Name: "p2", Value: "1,2"},
		})
	})

	t.Run("ProcessedValue SQL literal treated as string containment", func(t *testing.T) {
		schemaRequiredConfig := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
		schemaRequiredOp := NewComparisonOperator(schemaRequiredConfig)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		got, err := schemaRequiredOp.handleInParam(
			ProcessedValue{IsSQL: true, Value: "'foo'"},
			map[string]interface{}{"var": "col"},
			pc,
		)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "STRPOS(col, 'foo') > 0"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
	})

	t.Run("ProcessedValue placeholder for string param uses string containment", func(t *testing.T) {
		schemaRequiredConfig := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
		schemaRequiredOp := NewComparisonOperator(schemaRequiredConfig)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		pc.Add("hello") // @p1 = "hello" (string)
		got, err := schemaRequiredOp.handleInParam(
			ProcessedValue{IsSQL: true, Value: "@p1"},
			map[string]interface{}{"var": "col"},
			pc,
		)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := testRuntimeStringContainmentSQL(dialect.DialectBigQuery, "col", "@p1")
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
	})

	t.Run("ProcessedValue placeholder for numeric param uses array membership", func(t *testing.T) {
		schemaRequiredConfig := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
		schemaRequiredOp := NewComparisonOperator(schemaRequiredConfig)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		pc.Add(float64(42)) // @p1 = 42 (numeric)
		got, err := schemaRequiredOp.handleInParam(
			ProcessedValue{IsSQL: true, Value: "@p1"},
			map[string]interface{}{"var": "col"},
			pc,
		)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "@p1 IN UNNEST(col)"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
	})

	t.Run("ProcessedValue SQL expression uses array membership", func(t *testing.T) {
		schemaRequiredConfig := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
		schemaRequiredOp := NewComparisonOperator(schemaRequiredConfig)
		pc := params.NewParamCollector(params.PlaceholderNamed)
		pc.Add("hello") // @p1 = "hello"
		got, err := schemaRequiredOp.handleInParam(
			ProcessedValue{IsSQL: true, Value: "LOWER(@p1)"},
			map[string]interface{}{"var": "col"},
			pc,
		)
		if err != nil {
			t.Fatalf("handleInParam() error = %v", err)
		}
		want := "LOWER(@p1) IN UNNEST(col)"
		if got != want {
			t.Errorf("handleInParam() = %q, want %q", got, want)
		}
	})
}

func TestComparisonOperator_handleInParam_SchemaRequired_StringExpressionHeuristic(t *testing.T) {
	leftStringExpr := map[string]interface{}{
		OpCat: []interface{}{
			map[string]interface{}{
				OpSubstr: []interface{}{
					map[string]interface{}{OpVar: "profile.first"},
					float64(0),
					float64(2),
				},
			},
			"-x",
		},
	}
	leftNumericExpr := map[string]interface{}{
		"+": []interface{}{float64(1), float64(2)},
	}
	rightVar := map[string]interface{}{OpVar: "profile.name"}

	tests := []struct {
		name       string
		d          dialect.Dialect
		style      params.PlaceholderStyle
		leftArg    interface{}
		wantSQL    string
		wantParams []params.QueryParam
	}{
		{
			name:    "bigquery nested string expression uses containment",
			d:       dialect.DialectBigQuery,
			style:   params.PlaceholderNamed,
			leftArg: leftStringExpr,
			wantSQL: testRuntimeStringContainmentSQL(
				dialect.DialectBigQuery,
				"profile.name",
				"CONCAT(COALESCE(SUBSTR(profile.first, (@p1 + 1), @p2), ''), @p3)",
			),
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: "-x"},
			},
		},
		{
			name:    "postgres nested string expression uses containment",
			d:       dialect.DialectPostgreSQL,
			style:   params.PlaceholderPositional,
			leftArg: leftStringExpr,
			wantSQL: testRuntimeStringContainmentSQL(
				dialect.DialectPostgreSQL,
				"profile.name",
				"CONCAT(COALESCE(SUBSTR(profile.first, ($1 + 1), $2), ''), $3)",
			),
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: "-x"},
			},
		},
		{
			name:    "clickhouse nested string expression uses containment",
			d:       dialect.DialectClickHouse,
			style:   params.PlaceholderNamed,
			leftArg: leftStringExpr,
			wantSQL: testRuntimeStringContainmentSQL(
				dialect.DialectClickHouse,
				"profile.name",
				"CONCAT(COALESCE(substring(profile.first, (@p1 + 1), @p2), ''), @p3)",
			),
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(0)},
				{Name: "p2", Value: float64(2)},
				{Name: "p3", Value: "-x"},
			},
		},
		{
			name:    "bigquery numeric expression still uses membership fallback",
			d:       dialect.DialectBigQuery,
			style:   params.PlaceholderNamed,
			leftArg: leftNumericExpr,
			wantSQL: "(@p1 + @p2) IN UNNEST(profile.name)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
			},
		},
		{
			name:    "postgres numeric expression still uses membership fallback",
			d:       dialect.DialectPostgreSQL,
			style:   params.PlaceholderPositional,
			leftArg: leftNumericExpr,
			wantSQL: "($1 + $2) = ANY(profile.name)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
			},
		},
		{
			name:    "clickhouse numeric expression still uses membership fallback",
			d:       dialect.DialectClickHouse,
			style:   params.PlaceholderNamed,
			leftArg: leftNumericExpr,
			wantSQL: "has(profile.name, (@p1 + @p2))",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: float64(1)},
				{Name: "p2", Value: float64(2)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := NewComparisonOperator(NewOperatorConfig(tt.d, &fieldOnlySchemaProvider{}))
			pc := params.NewParamCollector(tt.style)
			got, err := op.ToSQLParam("in", []interface{}{tt.leftArg, rightVar}, pc)
			if err != nil {
				t.Fatalf("ToSQLParam() error = %v", err)
			}
			if got != tt.wantSQL {
				t.Fatalf("ToSQLParam() = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_InDoesNotMutateInputArray(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"region": "string",
	})
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	t.Run("non-parameterized path", func(t *testing.T) {
		arr := []interface{}{float64(1), float64(2)}
		original := append([]interface{}(nil), arr...)

		_, err := op.ToSQL("in", []interface{}{
			map[string]interface{}{"var": "region"},
			arr,
		})
		if err != nil {
			t.Fatalf("ToSQL(in) unexpected error: %v", err)
		}
		if !reflect.DeepEqual(arr, original) {
			t.Fatalf("input array mutated: got %#v, want %#v", arr, original)
		}
	})

	t.Run("parameterized path", func(t *testing.T) {
		arr := []interface{}{float64(1), float64(2)}
		original := append([]interface{}(nil), arr...)
		pc := params.NewParamCollector(params.PlaceholderNamed)

		_, err := op.handleInParam(
			map[string]interface{}{"var": "region"},
			arr,
			pc,
		)
		if err != nil {
			t.Fatalf("handleInParam() unexpected error: %v", err)
		}
		if !reflect.DeepEqual(arr, original) {
			t.Fatalf("input array mutated: got %#v, want %#v", arr, original)
		}
	})
}

func TestComparisonOperator_processArithmeticExpressionParam(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		operator   string
		args       interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:     "addition",
			operator: "+",
			args:     []interface{}{1, 2},
			wantSQL:  "(@p1 + @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 1},
				{Name: "p2", Value: 2},
			},
		},
		{
			name:     "unary minus",
			operator: "-",
			args:     []interface{}{42},
			wantSQL:  "(-@p1)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 42},
			},
		},
		{
			name:     "unsupported operator",
			operator: "^",
			args:     []interface{}{2, 3},
			wantErr:  true,
		},
		{
			name:     "non-array args",
			operator: "+",
			args:     "invalid",
			wantErr:  true,
		},
		{
			name:     "insufficient args for binary op",
			operator: "*",
			args:     []interface{}{5},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.processArithmeticExpressionParam(tt.operator, tt.args, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("processArithmeticExpressionParam() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("processArithmeticExpressionParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("processArithmeticExpressionParam() = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_processComparisonExpressionParam(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		operator   string
		args       interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:     "greater than",
			operator: ">",
			args:     []interface{}{5, 3},
			wantSQL:  "(@p1 > @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 5},
				{Name: "p2", Value: 3},
			},
		},
		{
			name:     "unsupported comparison",
			operator: "<>",
			args:     []interface{}{1, 2},
			wantErr:  true,
		},
		{
			name:     "non-array args",
			operator: ">",
			args:     "invalid",
			wantErr:  true,
		},
		{
			name:     "wrong number of args",
			operator: ">",
			args:     []interface{}{1, 2, 3},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.processComparisonExpressionParam(tt.operator, tt.args, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("processComparisonExpressionParam() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("processComparisonExpressionParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("processComparisonExpressionParam() = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_processMinMaxExpressionParam(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		operator   string
		args       interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantErr    bool
	}{
		{
			name:     "max",
			operator: "max",
			args:     []interface{}{5, 10},
			wantSQL:  "GREATEST(@p1, @p2)",
			wantParams: []params.QueryParam{
				{Name: "p1", Value: 5},
				{Name: "p2", Value: 10},
			},
		},
		{
			name:     "unsupported min/max operator",
			operator: "avg",
			args:     []interface{}{1, 2},
			wantErr:  true,
		},
		{
			name:     "non-array args",
			operator: "max",
			args:     "invalid",
			wantErr:  true,
		},
		{
			name:     "insufficient args",
			operator: "max",
			args:     []interface{}{5},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.processMinMaxExpressionParam(tt.operator, tt.args, pc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("processMinMaxExpressionParam() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("processMinMaxExpressionParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Errorf("processMinMaxExpressionParam() = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_ToSQLParam_WithSchemaCoercion(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"age": "integer",
	})
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	pc := params.NewParamCollector(params.PlaceholderNamed)
	got, err := op.ToSQLParam(">=", []interface{}{
		map[string]interface{}{"var": "age"},
		"50000",
	}, pc)
	if err != nil {
		t.Fatalf("ToSQLParam() unexpected error = %v", err)
	}
	if got != "age >= @p1" {
		t.Errorf("ToSQLParam() = %q, want %q", got, "age >= @p1")
	}
	assertQueryParams(t, pc.Params(), []params.QueryParam{
		{Name: "p1", Value: int64(50000)},
	})
}

func TestComparisonOperator_ToSQLParam_EqualitySemanticsWithSchema(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"amount":         "integer",
		"score":          "number",
		"active":         "boolean",
		"code":           "string",
		"other":          "string",
		"status":         "enum",
		"limited_status": "enum",
	})
	schema.enumValues["status"] = []string{"active", "1", "1.2", "9223372036854775807", "Infinity", "-Infinity"}
	schema.enumValues["limited_status"] = []string{"active"}
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name       string
		operator   string
		args       []interface{}
		wantSQL    string
		wantParams []params.QueryParam
		wantError  bool
	}{
		{
			name:       "numeric string literal is coerced",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "010"},
			wantSQL:    "amount = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: int64(10)}},
		},
		{
			name:       "numeric unsafe decimal string rounds like javascript",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "9007199254740993"},
			wantSQL:    "amount = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: int64(9007199254740992)}},
		},
		{
			name:       "numeric invalid string folds without params",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "abc"},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "string field preserves Go int64 literal exactly",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, int64(9223372036854775807)},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "9223372036854775807"}},
		},
		{
			name:       "enum field validates exact Go int64 string",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "status"}, int64(9223372036854775807)},
			wantSQL:    "status = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "9223372036854775807"}},
		},
		{
			name:       "string field preserves Go float32 literal formatting",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, float32(1.2)},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "1.2"}},
		},
		{
			name:       "enum field validates Go float32 literal formatting",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "status"}, float32(1.2)},
			wantSQL:    "status = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "1.2"}},
		},
		{
			name:       "string field json overflow canonicalizes infinity",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e400")},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "Infinity"}},
		},
		{
			name:       "string field json overflow canonicalizes infinity for not equal",
			operator:   "!=",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e400")},
			wantSQL:    "code != @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "Infinity"}},
		},
		{
			name:       "string field json negative overflow canonicalizes negative infinity",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("-1e400")},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "-Infinity"}},
		},
		{
			name:       "string field json underflow canonicalizes zero",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e-400")},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "0"}},
		},
		{
			name:       "string field native infinity canonicalizes",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, math.Inf(1)},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "Infinity"}},
		},
		{
			name:       "string field native NaN still folds without params",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, math.NaN()},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "enum field validates infinity canonical string",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "status"}, math.Inf(1)},
			wantSQL:    "status = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "Infinity"}},
		},
		{
			name:      "enum field rejects infinity canonical string",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": "limited_status"}, math.Inf(1)},
			wantError: true,
		},
		{
			name:       "integer field preserves large uint64 literal within int64 range",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, uint64(9007199254740993)},
			wantSQL:    "amount = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: int64(9007199254740993)}},
		},
		{
			name:       "number field preserves large uint64 literal above int64 range",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "score"}, uint64(9223372036854775808)},
			wantSQL:    "score = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: uint64(9223372036854775808)}},
		},
		{
			name:       "strict number field folds NaN without params",
			operator:   "===",
			args:       []interface{}{map[string]interface{}{"var": "score"}, math.NaN()},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "strict number field folds NaN true for not equal without params",
			operator:   "!==",
			args:       []interface{}{map[string]interface{}{"var": "score"}, math.NaN()},
			wantSQL:    "TRUE",
			wantParams: nil,
		},
		{
			name:       "strict number field folds infinity without params",
			operator:   "===",
			args:       []interface{}{map[string]interface{}{"var": "score"}, math.Inf(1)},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "strict number field folds infinity true for not equal without params",
			operator:   "!==",
			args:       []interface{}{map[string]interface{}{"var": "score"}, math.Inf(1)},
			wantSQL:    "TRUE",
			wantParams: nil,
		},
		{
			name:       "integer field uint64 above int64 range folds without params",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, uint64(9223372036854775808)},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:      "malformed json number literal errors before strict fold",
			operator:  "!==",
			args:      []interface{}{map[string]interface{}{"var": "code"}, json.Number("0 OR 1=1")},
			wantError: true,
		},
		{
			name:       "defaulted numeric var skips equality folding",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": []interface{}{"amount", "abc"}}, "abc"},
			wantSQL:    "COALESCE(amount, @p1) = @p2",
			wantParams: []params.QueryParam{{Name: "p1", Value: "abc"}, {Name: "p2", Value: "abc"}},
		},
		{
			name:       "defaulted numeric var coerces numeric string",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": []interface{}{"amount", 0}}, "50"},
			wantSQL:    "COALESCE(amount, @p1) = @p2",
			wantParams: []params.QueryParam{{Name: "p1", Value: 0}, {Name: "p2", Value: int64(50)}},
		},
		{
			name:       "defaulted numeric var coerces numeric string with field on right",
			operator:   "==",
			args:       []interface{}{"50", map[string]interface{}{"var": []interface{}{"amount", 0}}},
			wantSQL:    "@p1 = COALESCE(amount, @p2)",
			wantParams: []params.QueryParam{{Name: "p1", Value: int64(50)}, {Name: "p2", Value: 0}},
		},
		{
			name:       "defaulted numeric var invalid string folds without params when default cannot match",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": []interface{}{"amount", 0}}, "abc"},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "defaulted numeric var invalid string folds without params with field on right when default cannot match",
			operator:   "==",
			args:       []interface{}{"abc", map[string]interface{}{"var": []interface{}{"amount", 0}}},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "defaulted strict numeric mismatch folds without params when default cannot match",
			operator:   "===",
			args:       []interface{}{map[string]interface{}{"var": []interface{}{"amount", 0}}, "50"},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "defaulted strict numeric mismatch folds without params with field on right when default cannot match",
			operator:   "===",
			args:       []interface{}{"50", map[string]interface{}{"var": []interface{}{"amount", 0}}},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "defaulted strict numeric mismatch preserves params when default can match",
			operator:   "===",
			args:       []interface{}{map[string]interface{}{"var": []interface{}{"amount", "50"}}, "50"},
			wantSQL:    "COALESCE(amount, @p1) = @p2",
			wantParams: []params.QueryParam{{Name: "p1", Value: "50"}, {Name: "p2", Value: "50"}},
		},
		{
			name:       "defaulted strict numeric mismatch preserves params with field on right when default can match",
			operator:   "===",
			args:       []interface{}{"50", map[string]interface{}{"var": []interface{}{"amount", "50"}}},
			wantSQL:    "@p1 = COALESCE(amount, @p2)",
			wantParams: []params.QueryParam{{Name: "p1", Value: "50"}, {Name: "p2", Value: "50"}},
		},
		{
			name:      "defaulted string field boolean literal errors",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"code", ""}}, true},
			wantError: true,
		},
		{
			name:      "defaulted string field boolean literal errors with field on right",
			operator:  "==",
			args:      []interface{}{true, map[string]interface{}{"var": []interface{}{"code", ""}}},
			wantError: true,
		},
		{
			name:      "defaulted enum validates visible default",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"status", "unknown"}}, "active"},
			wantError: true,
		},
		{
			name:      "defaulted enum validates visible default before null equality",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"status", "unknown"}}, nil},
			wantError: true,
		},
		{
			name:      "defaulted enum validates visible default with var on right",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"status", "unknown"}}, map[string]interface{}{"var": "other"}},
			wantError: true,
		},
		{
			name:      "defaulted enum validates visible default with var on left",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": "other"}, map[string]interface{}{"var": []interface{}{"status", "unknown"}}},
			wantError: true,
		},
		{
			name:       "defaulted enum preserves valid default with var on right",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": []interface{}{"status", "active"}}, map[string]interface{}{"var": "other"}},
			wantSQL:    "((COALESCE(status, @p1) IS NULL AND other IS NULL) OR (COALESCE(status, @p1) IS NOT NULL AND other IS NOT NULL AND COALESCE(status, @p1) = other))",
			wantParams: []params.QueryParam{{Name: "p1", Value: "active"}},
		},
		{
			name:      "defaulted var malformed json number default errors before strict fold",
			operator:  "!==",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"amount", json.Number("0 OR 1=1")}}, "abc"},
			wantError: true,
		},
		{
			name:      "defaulted var malformed json number default errors before loose fold",
			operator:  "!=",
			args:      []interface{}{map[string]interface{}{"var": []interface{}{"amount", json.Number("0 OR 1=1")}}, "abc"},
			wantError: true,
		},
		{
			name:       "numeric above int64 range folds without params",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, json.Number("9223372036854775808")},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "numeric min int64 string remains param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "-9223372036854775808"},
			wantSQL:    "amount = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: int64(-9223372036854775807 - 1)}},
		},
		{
			name:       "numeric min int64 string not equal remains param",
			operator:   "!=",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "-9223372036854775808"},
			wantSQL:    "amount != @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: int64(-9223372036854775807 - 1)}},
		},
		{
			name:       "numeric above int64 range folds true for not equal without params",
			operator:   "!=",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, json.Number("9223372036854775808")},
			wantSQL:    "TRUE",
			wantParams: nil,
		},
		{
			name:       "number oversized radix string becomes numeric param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "score"}, "0x8000000000000000"},
			wantSQL:    "score = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: float64(9223372036854775808)}},
		},
		{
			name:       "number large json number literal preserves exact param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "score"}, json.Number("9223372036854775808")},
			wantSQL:    "score = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "9223372036854775808"}},
		},
		{
			name:       "number underflow json number literal preserves exact param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "score"}, json.Number("1e-400")},
			wantSQL:    "score = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "1e-400"}},
		},
		{
			name:       "integer max int64 hex string folds after javascript rounding",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "0x7fffffffffffffff"},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "integer oversized radix string folds without params",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "0x8000000000000000"},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:       "boolean string literal coerces inline",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "active"}, "0"},
			wantSQL:    "active = FALSE",
			wantParams: nil,
		},
		{
			name:       "string numeric literal becomes string param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("5.0")},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "5"}},
		},
		{
			name:       "string small exponent literal becomes javascript canonical string param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("1e-7")},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "1e-7"}},
		},
		{
			name:       "string large numeric literal becomes javascript canonical string param",
			operator:   "==",
			args:       []interface{}{map[string]interface{}{"var": "code"}, json.Number("9223372036854775808")},
			wantSQL:    "code = @p1",
			wantParams: []params.QueryParam{{Name: "p1", Value: "9223372036854776000"}},
		},
		{
			name:       "strict mismatch folds without params",
			operator:   "===",
			args:       []interface{}{map[string]interface{}{"var": "amount"}, "5"},
			wantSQL:    "FALSE",
			wantParams: nil,
		},
		{
			name:      "string boolean literal errors before adding params",
			operator:  "==",
			args:      []interface{}{map[string]interface{}{"var": "code"}, true},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if tt.wantError {
				if err == nil {
					t.Fatalf("ToSQLParam() expected error, got nil")
				}
				if len(pc.Params()) != 0 {
					t.Fatalf("ToSQLParam() params on error = %#v, want none", pc.Params())
				}
				return
			}
			if err != nil {
				t.Fatalf("ToSQLParam() unexpected error = %v", err)
			}
			if got != tt.wantSQL {
				t.Fatalf("ToSQLParam() = %q, want %q", got, tt.wantSQL)
			}
			assertQueryParams(t, pc.Params(), tt.wantParams)
		})
	}
}

func TestComparisonOperator_ToSQLParam_EqualityConstantFoldsValidateSchema(t *testing.T) {
	schema := newComparisonSchemaProvider(map[string]string{
		"bad`field": "integer",
	})
	schema.validateErr = fmt.Errorf("schema field %q contains quote characters", "bad`field")
	config := NewOperatorConfig(dialect.DialectBigQuery, schema)
	op := NewComparisonOperator(config)

	tests := []struct {
		name     string
		operator string
		args     []interface{}
	}{
		{
			name:     "loose impossible comparison validates schema",
			operator: "==",
			args:     []interface{}{map[string]interface{}{"var": "bad`field"}, "abc"},
		},
		{
			name:     "strict type mismatch validates schema",
			operator: "===",
			args:     []interface{}{map[string]interface{}{"var": "bad`field"}, "5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := params.NewParamCollector(params.PlaceholderNamed)
			got, err := op.ToSQLParam(tt.operator, tt.args, pc)
			if err == nil {
				t.Fatalf("ToSQLParam() = %q, expected schema validation error", got)
			}
			if !strings.Contains(err.Error(), "contains quote characters") {
				t.Fatalf("ToSQLParam() error = %v, want schema validation error", err)
			}
			if len(pc.Params()) != 0 {
				t.Fatalf("ToSQLParam() params on validation error = %#v, want none", pc.Params())
			}
		})
	}
}

func TestComparisonOperator_valueToSQLParam_ExpressionParserCallback(t *testing.T) {
	config := NewOperatorConfig(dialect.DialectBigQuery, &fieldOnlySchemaProvider{})
	config.SetParamExpressionParser(func(expr any, path string, pc *params.ParamCollector) (string, error) {
		return "CUSTOM_PARAM()", nil
	})
	op := NewComparisonOperator(config)

	pc := params.NewParamCollector(params.PlaceholderNamed)
	result, err := op.valueToSQLParam(map[string]interface{}{"customOp": []interface{}{1, 2}}, pc)
	if err != nil {
		t.Fatalf("valueToSQLParam() unexpected error = %v", err)
	}
	if result != "CUSTOM_PARAM()" {
		t.Errorf("valueToSQLParam() = %q, want CUSTOM_PARAM()", result)
	}
	if len(pc.Params()) != 0 {
		t.Errorf("expected no params, got %#v", pc.Params())
	}
}
