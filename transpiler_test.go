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
			input:    `{"===": [{"var": "status"}, "active"]}`,
			expected: "WHERE status = 'active'",
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
			name:     "double negation",
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
			name:     "chained less than (between exclusive)",
			input:    `{"<": [0, {"var": "temp"}, 100]}`,
			expected: "WHERE (0 < temp AND temp < 100)",
			hasError: false,
		},
		{
			name:     "chained less than or equal (between inclusive)",
			input:    `{"<=": [0, {"var": "score"}, 100]}`,
			expected: "WHERE (0 <= score AND score <= 100)",
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
			expected: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18))",
			hasError: false,
		},
		{
			name:     "some elements",
			input:    `{"some": [{"var": "statuses"}, {"==": [{"var": "item"}, "active"]}]}`,
			expected: "WHERE EXISTS (SELECT 1 FROM UNNEST(statuses) AS elem WHERE elem = 'active')",
			hasError: false,
		},
		{
			name:     "none elements",
			input:    `{"none": [{"var": "values"}, {"==": [{"var": "item"}, "invalid"]}]}`,
			expected: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE elem = 'invalid')",
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
			name:     "substring",
			input:    `{"substr": [{"var": "text"}, 0, 5]}`,
			expected: "WHERE SUBSTR(text, 1, 5)",
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

// TestComprehensiveNestedExpressions tests deeply nested and complex expressions
func TestComprehensiveNestedExpressions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		{
			name:     "nested reduce in comparison",
			input:    `{">": [{"reduce": [{"filter": [{"var": "cars"}, {"==": [{"var": "vendor"}, "Toyota"]}]}, {"+": [1, {"var": "accumulator"}]}, 0]}, 2]}`,
			expected: "WHERE ARRAY_REDUCE(ARRAY_FILTER(cars, condition_placeholder), (1 + accumulator), reduction_placeholder) > 2",
			hasError: false,
		},
		{
			name:     "nested filter in reduce",
			input:    `{"reduce": [{"filter": [{"var": "items"}, {">": [{"var": "price"}, 100]}]}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}`,
			expected: "WHERE ARRAY_REDUCE(ARRAY_FILTER(items, condition_placeholder), (accumulator + current), reduction_placeholder)",
			hasError: false,
		},
		{
			name:     "nested some in and",
			input:    `{"and": [{"==": [{"var": "status"}, "active"]}, {"some": [{"var": "results"}, {"and": [{"==": [{"var": "product"}, "abc"]}, {">": [{"var": "score"}, 8]}]}]}]}`,
			expected: "WHERE (status = 'active' AND EXISTS (SELECT 1 FROM UNNEST(results) AS elem WHERE (product = 'abc' AND score > 8)))",
			hasError: false,
		},
		{
			name:     "complex nested expression",
			input:    `{"and": [{"==": [{"var": "color2"}, "orange"]}, {"==": [{"var": "slider"}, 35]}, {"some": [{"var": "results"}, {"and": [{"==": [{"var": "product"}, "abc"]}, {">": [{"var": "score"}, 8]}]}]}, {">": [{"reduce": [{"filter": [{"var": "cars"}, {"and": [{"==": [{"var": "vendor"}, "Toyota"]}, {">=": [{"var": "year"}, 2010]}]}]}, {"+": [1, {"var": "accumulator"}]}, 0]}, 2]}]}`,
			expected: "WHERE (color2 = 'orange' AND slider = 35 AND EXISTS (SELECT 1 FROM UNNEST(results) AS elem WHERE (product = 'abc' AND score > 8)) AND ARRAY_REDUCE(ARRAY_FILTER(cars, condition_placeholder), (1 + accumulator), reduction_placeholder) > 2)",
			hasError: false,
		},
		{
			name:     "nested comparison in filter",
			input:    `{"filter": [{"var": "products"}, {"and": [{">": [{"var": "price"}, 100]}, {"<": [{"var": "price"}, 1000]}]}]}`,
			expected: "WHERE ARRAY_FILTER(products, condition_placeholder)",
			hasError: false,
		},
		{
			name:     "nested arithmetic in reduce",
			input:    `{"reduce": [{"var": "numbers"}, {"+": [{"var": "accumulator"}, {"*": [{"var": "current"}, 2]}]}, 0]}`,
			expected: "WHERE ARRAY_REDUCE(numbers, (accumulator + (current * 2)), reduction_placeholder)",
			hasError: false,
		},
		{
			name:     "nested logical in some",
			input:    `{"some": [{"var": "items"}, {"or": [{"==": [{"var": "status"}, "active"]}, {">": [{"var": "priority"}, 5]}]}]}`,
			expected: "WHERE EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE (status = 'active' OR priority > 5))",
			hasError: false,
		},
		{
			name:     "nested all in comparison",
			input:    `{">": [{"all": [{"var": "scores"}, {">=": [{"var": "elem"}, 70]}]}, true]}`,
			expected: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(scores) AS elem WHERE NOT (elem >= 70)) > TRUE",
			hasError: false,
		},
		{
			name:     "deeply nested reduce filter",
			input:    `{"reduce": [{"filter": [{"var": "data"}, {"and": [{"some": [{"var": "tags"}, {"==": [{"var": "elem"}, "important"]}]}, {">": [{"var": "value"}, 0]}]}]}, {"+": [{"var": "accumulator"}, {"reduce": [{"var": "current.subitems"}, {"+": [{"var": "acc"}, {"var": "item"}]}, 0]}]}, 0]}`,
			expected: "WHERE ARRAY_REDUCE(ARRAY_FILTER(data, condition_placeholder), (accumulator + ARRAY_REDUCE(current.subitems, (acc + item), reduction_placeholder)), reduction_placeholder)",
			hasError: false,
		},
		{
			name:     "nested map in comparison",
			input:    `{">": [{"map": [{"var": "numbers"}, {"*": [{"var": "elem"}, 2]}]}, 10]}`,
			expected: "WHERE ARRAY_MAP(numbers, transformation_placeholder) > 10",
			hasError: false,
		},
		{
			name:     "nested comparison in numeric",
			input:    `{"+": [{">": [{"var": "a"}, 5]}, {"<": [{"var": "b"}, 10]}]}`,
			expected: "WHERE ('a' > '5' + 'b' < '10')",
			hasError: false,
		},
		{
			name:     "nested var in arithmetic",
			input:    `{"+": [{"var": "x"}, {"*": [{"var": "y"}, {"var": "z"}]}]}`,
			expected: "WHERE (x + (y * z))",
			hasError: false,
		},
		{
			name:     "nested if in comparison",
			input:    `{">": [{"if": [{">": [{"var": "x"}, 0]}, {"var": "positive"}, {"var": "negative"}]}, 0]}`,
			expected: "WHERE CASE WHEN x > 0 THEN positive ELSE negative END > 0",
			hasError: false,
		},
		{
			name:     "nested comparison in logical",
			input:    `{"and": [{">": [{"var": "a"}, 1]}, {"<": [{"var": "b"}, 10]}, {"==": [{"var": "c"}, "test"]}]}`,
			expected: "WHERE (a > 1 AND b < 10 AND c = 'test')",
			hasError: false,
		},
		{
			name:     "nested reduce with complex expression",
			input:    `{"reduce": [{"var": "items"}, {"+": [{"var": "accumulator"}, {"*": [{"var": "current.price"}, {"if": [{">": [{"var": "current.discount"}, 0]}, {"-": [1, {"var": "current.discount"}]}, 1]}]}]}, 0]}`,
			expected: "WHERE ARRAY_REDUCE(items, (accumulator + (current.price * CASE WHEN current.discount > 0 THEN (1 - current.discount) ELSE 1 END)), reduction_placeholder)",
			hasError: false,
		},
		{
			name:     "nested filter with or",
			input:    `{"filter": [{"var": "users"}, {"or": [{">=": [{"var": "age"}, 18]}, {"==": [{"var": "role"}, "admin"]}]}]}`,
			expected: "WHERE ARRAY_FILTER(users, condition_placeholder)",
			hasError: false,
		},
		{
			name:     "nested some with comparison",
			input:    `{"some": [{"var": "items"}, {">": [{"+": [{"var": "price"}, {"var": "tax"}]}, 100]}]}`,
			expected: "WHERE EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE (price + tax) > 100)",
			hasError: false,
		},
		{
			name:     "nested all with nested comparison",
			input:    `{"all": [{"var": "scores"}, {"and": [{">=": [{"var": "elem"}, 0]}, {"<=": [{"var": "elem"}, 100]}]}]}`,
			expected: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(scores) AS elem WHERE NOT ((elem >= 0 AND elem <= 100)))",
			hasError: false,
		},
		{
			name:     "nested none with complex",
			input:    `{"none": [{"var": "errors"}, {"or": [{"==": [{"var": "elem.type"}, "critical"]}, {">": [{"var": "elem.count"}, 10]}]}]}`,
			expected: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(errors) AS elem WHERE (elem.type = 'critical' OR elem.count > 10))",
			hasError: false,
		},
		{
			name:     "very deeply nested",
			input:    `{"and": [{"some": [{"filter": [{"var": "data"}, {">": [{"var": "value"}, 0]}]}, {"all": [{"var": "elem.items"}, {">=": [{"var": "elem.score"}, 50]}]}]}, {">": [{"reduce": [{"var": "totals"}, {"+": [{"var": "accumulator"}, {"*": [{"var": "current"}, {"if": [{">": [{"var": "current"}, 100]}, 2, 1]}]}]}, 0]}, 1000]}]}`,
			expected: "WHERE (EXISTS (SELECT 1 FROM UNNEST(ARRAY_FILTER(data, condition_placeholder)) AS elem WHERE NOT EXISTS (SELECT 1 FROM UNNEST(elem.elems) AS elem WHERE NOT (elem.score >= 50))) AND ARRAY_REDUCE(totals, (accumulator + (current * CASE WHEN current > 100 THEN 2 ELSE 1 END)), reduction_placeholder) > 1000)",
			hasError: false,
		},
		{
			name:     "multiple nested if conditions",
			input:    `{"if": [{"and": [{">": [{"var": "age"}, 18]}, {"==": [{"var": "country"}, "US"]}]}, {"if": [{">": [{"var": "score"}, 80]}, "excellent", "good"]}, "not eligible"]}`,
			expected: "WHERE CASE WHEN (age > 18 AND country = 'US') THEN CASE WHEN score > 80 THEN 'excellent' ELSE 'good' END ELSE 'not eligible' END",
			hasError: false,
		},
		{
			name:     "nested arithmetic with multiple operations",
			input:    `{"+": [{"*": [{"var": "price"}, {"var": "quantity"}]}, {"-": [{"var": "discount"}, {"%": [{"var": "tax"}, 10]}]}]}`,
			expected: "WHERE ((price * quantity) + (discount - (tax % 10)))",
			hasError: false,
		},
		{
			name:     "complex array operations",
			input:    `{"and": [{"some": [{"var": "items"}, {">": [{"var": "price"}, 100]}]}, {"all": [{"var": "tags"}, {"in": [{"var": "elem"}, ["important", "urgent"]]}]}]}`,
			expected: "WHERE (EXISTS (SELECT 1 FROM UNNEST(items) AS elem WHERE price > 100) AND NOT EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE NOT (elem IN ('important', 'urgent'))))",
			hasError: false,
		},
		{
			name:     "nested string operations",
			input:    `{"==": [{"cat": [{"var": "firstName"}, " ", {"var": "lastName"}]}, "John Doe"]}`,
			expected: "WHERE CONCAT(firstName, ' ', lastName) = John Doe",
			hasError: false,
		},
		{
			name:     "chained comparisons with variables",
			input:    `{"<": [{"var": "min"}, {"var": "value"}, {"var": "max"}]}`,
			expected: "WHERE (min < value AND value < max)",
			hasError: false,
		},
		{
			name:     "nested missing operations",
			input:    `{"and": [{"!": [{"missing": "email"}]}, {"missing_some": [1, ["phone", "address"]]}]}`,
			expected: "WHERE (NOT (email IS NULL) AND (phone IS NULL OR address IS NULL))",
			hasError: false,
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
