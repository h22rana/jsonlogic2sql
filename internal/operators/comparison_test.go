package operators

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

func TestComparisonOperator_ToSQL(t *testing.T) {
	op := NewComparisonOperator(nil)

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
			expected: "TRUE = FALSE",
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
			expected: "",
			hasError: true,
		},
		{
			name:     "in with string containment",
			operator: "in",
			args:     []interface{}{map[string]interface{}{"var": "field"}, "not-array"},
			expected: "POSITION(field IN 'not-array') > 0",
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

func TestComparisonOperator_valueToSQL(t *testing.T) {
	op := NewComparisonOperator(nil)

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
				config = NewOperatorConfig(tt.dialect, nil)
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
	config := NewOperatorConfig(dialect.DialectBigQuery, nil)
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
	config := NewOperatorConfig(dialect.DialectBigQuery, nil)
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
	config := NewOperatorConfig(dialect.DialectBigQuery, nil)
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
