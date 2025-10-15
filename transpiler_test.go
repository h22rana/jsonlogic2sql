package jsonlogic2sql

import (
	"testing"
)

func TestNewTranspiler(t *testing.T) {
	tr := NewTranspiler()
	if tr == nil {
		t.Fatal("NewTranspiler() returned nil")
	}
	if tr.parser == nil {
		t.Fatal("parser is nil")
	}
}

func TestTranspiler_Transpile(t *testing.T) {
	tr := NewTranspiler()

	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    `{">": [{"var": "amount"}, 1000]}`,
			expected: "WHERE amount > 1000",
			hasError: false,
		},
		{
			name:     "and operation",
			input:    `{"and": [{"==": [{"var": "status"}, "pending"]}, {">": [{"var": "amount"}, 5000]}]}`,
			expected: "WHERE (status = 'pending' AND amount > 5000)",
			hasError: false,
		},
		{
			name:     "or operation",
			input:    `{"or": [{">=": [{"var": "failedAttempts"}, 5]}, {"in": [{"var": "country"}, ["CN", "RU"]]}]}`,
			expected: "WHERE (failedAttempts >= 5 OR country IN ('CN', 'RU'))",
			hasError: false,
		},
		{
			name:     "nested conditions",
			input:    `{"and": [{">": [{"var": "transaction.amount"}, 10000]}, {"or": [{"==": [{"var": "user.verified"}, false]}, {"<": [{"var": "user.accountAgeDays"}, 7]}]}]}`,
			expected: "WHERE (transaction.amount > 10000 AND (user.verified = FALSE OR user.accountAgeDays < 7))",
			hasError: false,
		},
		{
			name:     "if operation",
			input:    `{"if": [{">": [{"var": "age"}, 18]}, "adult", "minor"]}`,
			expected: "WHERE CASE WHEN age > 18 THEN 'adult' ELSE 'minor' END",
			hasError: false,
		},
		{
			name:     "missing operation",
			input:    `{"missing": "field"}`,
			expected: "WHERE field IS NULL",
			hasError: false,
		},
		{
			name:     "missing_some operation",
			input:    `{"missing_some": [1, ["field1", "field2"]]}`,
			expected: "WHERE (field1 IS NULL OR field2 IS NULL)",
			hasError: false,
		},
		{
			name:     "invalid JSON",
			input:    `{invalid json}`,
			expected: "",
			hasError: true,
		},
		{
			name:     "unsupported operator",
			input:    `{"unsupported": [1, 2]}`,
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.Transpile(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Transpile() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Transpile() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("Transpile() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestTranspiler_TranspileFromMap(t *testing.T) {
	tr := NewTranspiler()

	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "WHERE amount > 1000",
			hasError: false,
		},
		{
			name:     "and operation",
			input:    map[string]interface{}{"and": []interface{}{map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "pending"}}, map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 5000}}}},
			expected: "WHERE (status = 'pending' AND amount > 5000)",
			hasError: false,
		},
		{
			name:     "unsupported operator",
			input:    map[string]interface{}{"unsupported": []interface{}{1, 2}},
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.TranspileFromMap(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("TranspileFromMap() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("TranspileFromMap() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileFromMap() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestTranspiler_TranspileFromInterface(t *testing.T) {
	tr := NewTranspiler()

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "WHERE amount > 1000",
			hasError: false,
		},
		{
			name:     "primitive value",
			input:    "hello",
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.TranspileFromInterface(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("TranspileFromInterface() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("TranspileFromInterface() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileFromInterface() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestTranspile(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    `{">": [{"var": "amount"}, 1000]}`,
			expected: "WHERE amount > 1000",
			hasError: false,
		},
		{
			name:     "invalid JSON",
			input:    `{invalid json}`,
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Transpile(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Transpile() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Transpile() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("Transpile() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestTranspileFromMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "WHERE amount > 1000",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TranspileFromMap(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("TranspileFromMap() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("TranspileFromMap() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileFromMap() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

func TestTranspileFromInterface(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name:     "simple comparison",
			input:    map[string]interface{}{">": []interface{}{map[string]interface{}{"var": "amount"}, 1000}},
			expected: "WHERE amount > 1000",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TranspileFromInterface(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("TranspileFromInterface() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("TranspileFromInterface() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileFromInterface() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}

// Test all JSON Logic operators comprehensively
func TestAllOperators(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		// Data Access Operations
		{
			name:     "var simple",
			input:    `{"var": "name"}`,
			expected: "WHERE name",
			hasError: false,
		},
		{
			name:     "var with default",
			input:    `{"var": ["status", "pending"]}`,
			expected: "WHERE COALESCE(status, 'pending')",
			hasError: false,
		},
		{
			name:     "missing field",
			input:    `{"missing": "email"}`,
			expected: "WHERE email IS NULL",
			hasError: false,
		},
		{
			name:     "missing some fields",
			input:    `{"missing_some": [1, ["field1", "field2"]]}`,
			expected: "WHERE (field1 IS NULL OR field2 IS NULL)",
			hasError: false,
		},

		// Logic and Boolean Operations
		{
			name:     "equality",
			input:    `{"==": [{"var": "status"}, "active"]}`,
			expected: "WHERE status = 'active'",
			hasError: false,
		},
		{
			name:     "strict equality",
			input:    `{"===": [{"var": "count"}, 5]}`,
			expected: "WHERE count = 5",
			hasError: false,
		},
		{
			name:     "inequality",
			input:    `{"!=": [{"var": "status"}, "inactive"]}`,
			expected: "WHERE status != 'inactive'",
			hasError: false,
		},
		{
			name:     "strict inequality",
			input:    `{"!==": [{"var": "count"}, 0]}`,
			expected: "WHERE count <> 0",
			hasError: false,
		},
		{
			name:     "logical not",
			input:    `{"!": [{"var": "isDeleted"}]}`,
			expected: "WHERE NOT (isDeleted)",
			hasError: false,
		},
		{
			name:     "double not",
			input:    `{"!!": [{"var": "value"}]}`,
			expected: "WHERE (value IS NOT NULL AND value != FALSE AND value != 0 AND value != '')",
			hasError: false,
		},
		{
			name:     "logical and",
			input:    `{"and": [{"==": [{"var": "status"}, "active"]}, {">": [{"var": "score"}, 100]}]}`,
			expected: "WHERE (status = 'active' AND score > 100)",
			hasError: false,
		},
		{
			name:     "logical or",
			input:    `{"or": [{"==": [{"var": "role"}, "admin"]}, {"==": [{"var": "role"}, "user"]}]}`,
			expected: "WHERE (role = 'admin' OR role = 'user')",
			hasError: false,
		},
		{
			name:     "conditional if",
			input:    `{"if": [{">": [{"var": "age"}, 18]}, "adult", "minor"]}`,
			expected: "WHERE CASE WHEN age > 18 THEN 'adult' ELSE 'minor' END",
			hasError: false,
		},

		// Numeric Operations
		{
			name:     "greater than",
			input:    `{">": [{"var": "amount"}, 1000]}`,
			expected: "WHERE amount > 1000",
			hasError: false,
		},
		{
			name:     "greater than or equal",
			input:    `{">=": [{"var": "score"}, 80]}`,
			expected: "WHERE score >= 80",
			hasError: false,
		},
		{
			name:     "less than",
			input:    `{"<": [{"var": "age"}, 65]}`,
			expected: "WHERE age < 65",
			hasError: false,
		},
		{
			name:     "less than or equal",
			input:    `{"<=": [{"var": "count"}, 10]}`,
			expected: "WHERE count <= 10",
			hasError: false,
		},
		{
			name:     "between",
			input:    `{"between": [{"var": "age"}, 18, 65]}`,
			expected: "WHERE (age BETWEEN 18 AND 65)",
			hasError: false,
		},
		{
			name:     "max",
			input:    `{"max": [{"var": "score1"}, {"var": "score2"}, {"var": "score3"}]}`,
			expected: "WHERE GREATEST(score1, score2, score3)",
			hasError: false,
		},
		{
			name:     "min",
			input:    `{"min": [{"var": "price1"}, {"var": "price2"}]}`,
			expected: "WHERE LEAST(price1, price2)",
			hasError: false,
		},
		{
			name:     "addition",
			input:    `{"+": [{"var": "price"}, {"var": "tax"}]}`,
			expected: "WHERE (price + tax)",
			hasError: false,
		},
		{
			name:     "subtraction",
			input:    `{"-": [{"var": "total"}, {"var": "discount"}]}`,
			expected: "WHERE (total - discount)",
			hasError: false,
		},
		{
			name:     "multiplication",
			input:    `{"*": [{"var": "price"}, 1.2]}`,
			expected: "WHERE (price * 1.2)",
			hasError: false,
		},
		{
			name:     "division",
			input:    `{"/": [{"var": "total"}, 2]}`,
			expected: "WHERE (total / 2)",
			hasError: false,
		},
		{
			name:     "modulo",
			input:    `{"%": [{"var": "count"}, 3]}`,
			expected: "WHERE (count % 3)",
			hasError: false,
		},

		// Array Operations
		{
			name:     "in array",
			input:    `{"in": [{"var": "country"}, ["US", "CA", "MX"]]}`,
			expected: "WHERE country IN ('US', 'CA', 'MX')",
			hasError: false,
		},
		{
			name:     "map array",
			input:    `{"map": [{"var": "numbers"}, {"+": [{"var": "item"}, 1]}]}`,
			expected: "WHERE ARRAY_MAP(numbers, transformation_placeholder)",
			hasError: false,
		},
		{
			name:     "filter array",
			input:    `{"filter": [{"var": "scores"}, {">": [{"var": "item"}, 70]}]}`,
			expected: "WHERE ARRAY_FILTER(scores, condition_placeholder)",
			hasError: false,
		},
		{
			name:     "reduce array",
			input:    `{"reduce": [{"var": "numbers"}, 0, {"+": [{"var": "accumulator"}, {"var": "item"}]}]}`,
			expected: "WHERE ARRAY_REDUCE(numbers, 0, reduction_placeholder)",
			hasError: false,
		},
		{
			name:     "all elements",
			input:    `{"all": [{"var": "ages"}, {">=": [{"var": "item"}, 18]}]}`,
			expected: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (item >= 18))",
			hasError: false,
		},
		{
			name:     "some elements",
			input:    `{"some": [{"var": "statuses"}, {"==": [{"var": "item"}, "active"]}]}`,
			expected: "WHERE EXISTS (SELECT 1 FROM UNNEST(statuses) AS elem WHERE item = 'active')",
			hasError: false,
		},
		{
			name:     "none elements",
			input:    `{"none": [{"var": "values"}, {"==": [{"var": "item"}, "invalid"]}]}`,
			expected: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE item = 'invalid')",
			hasError: false,
		},
		{
			name:     "merge arrays",
			input:    `{"merge": [{"var": "array1"}, {"var": "array2"}]}`,
			expected: "WHERE ARRAY_CONCAT(array1, array2)",
			hasError: false,
		},

		// String Operations
		{
			name:     "concatenate strings",
			input:    `{"cat": [{"var": "firstName"}, " ", {"var": "lastName"}]}`,
			expected: "WHERE CONCAT(firstName, ' ', lastName)",
			hasError: false,
		},
		{
			name:     "substring with length",
			input:    `{"substr": [{"var": "email"}, 1, 10]}`,
			expected: "WHERE SUBSTRING(email, 1 + 1, 10)",
			hasError: false,
		},
		{
			name:     "substring without length",
			input:    `{"substr": [{"var": "email"}, 5]}`,
			expected: "WHERE SUBSTRING(email, 5 + 1)",
			hasError: false,
		},

		// Complex Nested Examples
		{
			name:     "nested conditions",
			input:    `{"and": [{">": [{"var": "transaction.amount"}, 10000]}, {"or": [{"==": [{"var": "user.verified"}, false]}, {"<": [{"var": "user.accountAgeDays"}, 7]}]}]}`,
			expected: "WHERE (transaction.amount > 10000 AND (user.verified = FALSE OR user.accountAgeDays < 7))",
			hasError: false,
		},
		{
			name:     "complex conditional",
			input:    `{"if": [{"and": [{">=": [{"var": "age"}, 18]}, {"==": [{"var": "country"}, "US"]}]}, "eligible", "ineligible"]}`,
			expected: "WHERE CASE WHEN (age >= 18 AND country = 'US') THEN 'eligible' ELSE 'ineligible' END",
			hasError: false,
		},
		{
			name:     "multiple numeric operations",
			input:    `{"and": [{">": [{"var": "totalPrice"}, 100]}, {"<": [{"var": "totalQuantity"}, 1000]}]}`,
			expected: "WHERE (totalPrice > 100 AND totalQuantity < 1000)",
			hasError: false,
		},
		{
			name:     "mixed operations",
			input:    `{"and": [{"in": [{"var": "status"}, ["active", "pending"]]}, {"!": [{"missing": "email"}]}, {">=": [{"var": "score"}, 80]}]}`,
			expected: "WHERE (status IN ('active', 'pending') AND NOT (email IS NULL) AND score >= 80)",
			hasError: false,
		},

		// Error Cases
		{
			name:     "unsupported operator",
			input:    `{"unsupported": [1, 2]}`,
			expected: "",
			hasError: true,
		},
		{
			name:     "invalid JSON",
			input:    `{invalid json}`,
			expected: "",
			hasError: true,
		},
		{
			name:     "empty input",
			input:    ``,
			expected: "",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Transpile(tt.input)

			if tt.hasError {
				if err == nil {
					t.Errorf("Transpile() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Transpile() unexpected error = %v", err)
				}
				if result != tt.expected {
					t.Errorf("Transpile() = %v, expected %v", result, tt.expected)
				}
			}
		})
	}
}
