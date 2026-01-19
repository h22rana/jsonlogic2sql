package jsonlogic2sql

import (
	"testing"
)

func TestNewTranspiler(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() returned error: %v", err)
	}
	if tr == nil {
		t.Fatal("NewTranspiler() returned nil")
	}
	if tr.parser == nil {
		t.Fatal("parser is nil")
	}
}

func TestTranspiler_Transpile(t *testing.T) {
	tr, _ := NewTranspiler(DialectBigQuery)

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
	tr, _ := NewTranspiler(DialectBigQuery)

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
	tr, _ := NewTranspiler(DialectBigQuery)

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
			result, err := Transpile(DialectBigQuery, tt.input)

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
			result, err := TranspileFromMap(DialectBigQuery, tt.input)

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
			result, err := TranspileFromInterface(DialectBigQuery, tt.input)

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

// Test all JSON Logic operators comprehensively.
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
		{
			name:     "missing with array of fields",
			input:    `{"missing": ["email", "phone", "address"]}`,
			expected: "WHERE (email IS NULL OR phone IS NULL OR address IS NULL)",
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
			name:     "equality with null",
			input:    `{"==": [{"var": "deleted_at"}, null]}`,
			expected: "WHERE deleted_at IS NULL",
			hasError: false,
		},
		{
			name:     "inequality with null",
			input:    `{"!=": [{"var": "field"}, null]}`,
			expected: "WHERE field IS NOT NULL",
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
			expected: "WHERE ARRAY(SELECT (elem + 1) FROM UNNEST(numbers) AS elem)",
			hasError: false,
		},
		{
			name:     "filter array",
			input:    `{"filter": [{"var": "scores"}, {">": [{"var": "item"}, 70]}]}`,
			expected: "WHERE ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem > 70)",
			hasError: false,
		},
		{
			name:     "reduce array",
			input:    `{"reduce": [{"var": "numbers"}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}`,
			expected: "WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST(numbers) AS elem), 0)",
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
			result, err := Transpile(DialectBigQuery, tt.input)

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

// TestComprehensiveNestedExpressions tests deeply nested and complex expressions.
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
			expected: "WHERE (SELECT (1 + 0) FROM UNNEST(ARRAY(SELECT elem FROM UNNEST(cars) AS elem WHERE vendor = 'Toyota')) AS elem) > 2",
			hasError: false,
		},
		{
			name:     "nested filter in reduce",
			input:    `{"reduce": [{"filter": [{"var": "items"}, {">": [{"var": "price"}, 100]}]}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}`,
			expected: "WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST(ARRAY(SELECT elem FROM UNNEST(items) AS elem WHERE price > 100)) AS elem), 0)",
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
			expected: "WHERE (color2 = 'orange' AND slider = 35 AND EXISTS (SELECT 1 FROM UNNEST(results) AS elem WHERE (product = 'abc' AND score > 8)) AND (SELECT (1 + 0) FROM UNNEST(ARRAY(SELECT elem FROM UNNEST(cars) AS elem WHERE (vendor = 'Toyota' AND year >= 2010))) AS elem) > 2)",
			hasError: false,
		},
		{
			name:     "nested comparison in filter",
			input:    `{"filter": [{"var": "products"}, {"and": [{">": [{"var": "price"}, 100]}, {"<": [{"var": "price"}, 1000]}]}]}`,
			expected: "WHERE ARRAY(SELECT elem FROM UNNEST(products) AS elem WHERE (price > 100 AND price < 1000))",
			hasError: false,
		},
		{
			name:     "nested arithmetic in reduce",
			input:    `{"reduce": [{"var": "numbers"}, {"+": [{"var": "accumulator"}, {"*": [{"var": "current"}, 2]}]}, 0]}`,
			expected: "WHERE (SELECT (0 + (current * 2)) FROM UNNEST(numbers) AS elem)",
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
			expected: "WHERE (SELECT (0 + (SELECT (acc + elem) FROM UNNEST(current.subelems) AS elem)) FROM UNNEST(ARRAY(SELECT elem FROM UNNEST(data) AS elem WHERE (EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE elem = 'important') AND value > 0))) AS elem)",
			hasError: false,
		},
		{
			name:     "nested map in comparison",
			input:    `{">": [{"map": [{"var": "numbers"}, {"*": [{"var": "elem"}, 2]}]}, 10]}`,
			expected: "WHERE ARRAY(SELECT (elem * 2) FROM UNNEST(numbers) AS elem) > 10",
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
			expected: "WHERE (SELECT (0 + (current.price * CASE WHEN current.discount > 0 THEN (1 - current.discount) ELSE 1 END)) FROM UNNEST(items) AS elem)",
			hasError: false,
		},
		{
			name:     "nested filter with or",
			input:    `{"filter": [{"var": "users"}, {"or": [{">=": [{"var": "age"}, 18]}, {"==": [{"var": "role"}, "admin"]}]}]}`,
			expected: "WHERE ARRAY(SELECT elem FROM UNNEST(users) AS elem WHERE (age >= 18 OR role = 'admin'))",
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
			expected: "WHERE (EXISTS (SELECT 1 FROM UNNEST(ARRAY(SELECT elem FROM UNNEST(data) AS elem WHERE value > 0)) AS elem WHERE NOT EXISTS (SELECT 1 FROM UNNEST(elem.elems) AS elem WHERE NOT (elem.score >= 50))) AND (SELECT (0 + (current * CASE WHEN current > 100 THEN 2 ELSE 1 END)) FROM UNNEST(totals) AS elem) > 1000)",
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
			expected: "WHERE CONCAT(firstName, ' ', lastName) = 'John Doe'",
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
		// Additional NULL comparison edge cases
		{
			name:     "strict equality with null",
			input:    `{"===": [{"var": "field"}, null]}`,
			expected: "WHERE field IS NULL",
			hasError: false,
		},
		{
			name:     "null on left side of equality",
			input:    `{"==": [null, {"var": "field"}]}`,
			expected: "WHERE field IS NULL",
			hasError: false,
		},
		{
			name:     "null on left side of inequality",
			input:    `{"!=": [null, {"var": "field"}]}`,
			expected: "WHERE field IS NOT NULL",
			hasError: false,
		},
		{
			name:     "both null equality",
			input:    `{"==": [null, null]}`,
			expected: "WHERE NULL IS NULL",
			hasError: false,
		},
		{
			name:     "both null inequality",
			input:    `{"!=": [null, null]}`,
			expected: "WHERE NULL IS NOT NULL",
			hasError: false,
		},
		// Missing operator edge cases
		{
			name:     "missing with single element array",
			input:    `{"missing": ["email"]}`,
			expected: "WHERE (email IS NULL)",
			hasError: false,
		},
		{
			name:     "missing with two fields",
			input:    `{"missing": ["email", "phone"]}`,
			expected: "WHERE (email IS NULL OR phone IS NULL)",
			hasError: false,
		},
		{
			name:     "NOT with missing array",
			input:    `{"!": [{"missing": ["email", "phone"]}]}`,
			expected: "WHERE NOT ((email IS NULL OR phone IS NULL))",
			hasError: false,
		},
		// Complex NULL scenarios
		{
			name:     "null comparison in and",
			input:    `{"and": [{"==": [{"var": "deleted_at"}, null]}, {"!=": [{"var": "archived_at"}, null]}]}`,
			expected: "WHERE (deleted_at IS NULL AND archived_at IS NOT NULL)",
			hasError: false,
		},
		{
			name:     "null comparison in or",
			input:    `{"or": [{"==": [{"var": "field1"}, null]}, {"==": [{"var": "field2"}, null]}]}`,
			expected: "WHERE (field1 IS NULL OR field2 IS NULL)",
			hasError: false,
		},
		{
			name:     "null comparison with var default",
			input:    `{"==": [{"var": ["deleted_at", null]}, null]}`,
			expected: "WHERE COALESCE(deleted_at, NULL) IS NULL",
			hasError: false,
		},
		// Edge case: NULL in arithmetic (should error or handle gracefully)
		{
			name:     "null in arithmetic should handle",
			input:    `{"+": [{"var": "value"}, null]}`,
			expected: "WHERE (value + NULL)",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Transpile(DialectBigQuery, tt.input)

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

// TestAdditionalEdgeCases tests additional edge cases for comprehensive coverage.
func TestAdditionalEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		hasError bool
	}{
		// NULL comparison tests
		{
			name:     "equal to null uses IS NULL",
			input:    `{"==": [{"var": "deleted_at"}, null]}`,
			expected: "WHERE deleted_at IS NULL",
			hasError: false,
		},
		{
			name:     "not equal to null uses IS NOT NULL",
			input:    `{"!=": [{"var": "field"}, null]}`,
			expected: "WHERE field IS NOT NULL",
			hasError: false,
		},
		{
			name:     "strict not equal to null uses IS NOT NULL",
			input:    `{"!==": [{"var": "value"}, null]}`,
			expected: "WHERE value IS NOT NULL",
			hasError: false,
		},

		// Missing operator with array
		{
			name:     "missing with array of fields",
			input:    `{"missing": ["email", "phone", "address"]}`,
			expected: "WHERE (email IS NULL OR phone IS NULL OR address IS NULL)",
			hasError: false,
		},

		// Cat with nested if
		{
			name:     "cat with nested if expression",
			input:    `{"cat": [{"if": [{"==": [{"var": "gender"}, "M"]}, "Mr. ", "Ms. "]}, {"var": "first_name"}, " ", {"var": "last_name"}]}`,
			expected: "WHERE CONCAT(CASE WHEN (gender = 'M') THEN 'Mr. ' ELSE 'Ms. ' END, first_name, ' ', last_name)",
			hasError: false,
		},

		// NOT with missing
		{
			name:     "NOT with missing",
			input:    `{"!": [{"missing": "email"}]}`,
			expected: "WHERE NOT (email IS NULL)",
			hasError: false,
		},

		// Double NOT
		{
			name:     "double NOT",
			input:    `{"!": [{"!": [{"var": "flag"}]}]}`,
			expected: "WHERE NOT (NOT (flag))",
			hasError: false,
		},

		// Five-value chained comparison
		{
			name:     "five value chained comparison",
			input:    `{"<": [1, {"var": "a"}, {"var": "b"}, {"var": "c"}, 100]}`,
			expected: "WHERE (1 < a AND a < b AND b < c AND c < 100)",
			hasError: false,
		},

		// Nested max/min
		{
			name:     "nested max min",
			input:    `{"max": [{"min": [{"var": "a"}, {"var": "b"}]}, {"min": [{"var": "c"}, {"var": "d"}]}]}`,
			expected: "WHERE GREATEST(LEAST(a, b), LEAST(c, d))",
			hasError: false,
		},

		// Complex negation
		{
			name:     "complex negation",
			input:    `{"!": [{"or": [{"==": [{"var": "a"}, 1]}, {"==": [{"var": "b"}, 2]}]}]}`,
			expected: "WHERE NOT ((a = 1 OR b = 2))",
			hasError: false,
		},

		// Between pattern
		{
			name:     "between pattern with and",
			input:    `{"and": [{">=": [{"var": "value"}, 10]}, {"<=": [{"var": "value"}, 20]}]}`,
			expected: "WHERE (value >= 10 AND value <= 20)",
			hasError: false,
		},

		// Triple nested if
		{
			name:     "triple nested if",
			input:    `{"if": [{"var": "a"}, {"if": [{"var": "b"}, {"if": [{"var": "c"}, "deep", "c_false"]}, "b_false"]}, "a_false"]}`,
			expected: "WHERE CASE WHEN a THEN CASE WHEN b THEN CASE WHEN c THEN 'deep' ELSE 'c_false' END ELSE 'b_false' END ELSE 'a_false' END",
			hasError: false,
		},

		// Comparison with var default
		{
			name:     "comparison with var default",
			input:    `{"==": [{"var": ["status", "unknown"]}, "active"]}`,
			expected: "WHERE COALESCE(status, 'unknown') = 'active'",
			hasError: false,
		},

		// Nested all with some
		{
			name:     "nested all with some",
			input:    `{"all": [{"var": "groups"}, {"some": [{"var": "members"}, {"==": [{"var": "role"}, "admin"]}]}]}`,
			expected: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(groups) AS elem WHERE NOT (EXISTS (SELECT 1 FROM UNNEST(members) AS elem WHERE role = 'admin')))",
			hasError: false,
		},

		// If with arithmetic result
		{
			name:     "if with arithmetic result",
			input:    `{"if": [{">": [{"var": "qty"}, 100]}, {"*": [{"var": "price"}, 0.9]}, {"*": [{"var": "price"}, 1.0]}]}`,
			expected: "WHERE CASE WHEN qty > 100 THEN (price * 0.9) ELSE (price * 1) END",
			hasError: false,
		},

		// Empty string in cat
		{
			name:     "empty string in cat",
			input:    `{"cat": ["", {"var": "name"}, ""]}`,
			expected: "WHERE CONCAT('', name, '')",
			hasError: false,
		},

		// Negative index in substr
		{
			name:     "negative index in substr",
			input:    `{"substr": [{"var": "text"}, -5, 3]}`,
			expected: "WHERE SUBSTR(text, -4, 3)",
			hasError: false,
		},

		// In with string (substring check)
		{
			name:     "in with string for substring check",
			input:    `{"in": ["test", "this is a test string"]}`,
			expected: "WHERE POSITION('test' IN 'this is a test string') > 0",
			hasError: false,
		},

		// Or with literals
		{
			name:     "or with literals",
			input:    `{"or": [false, {"var": "flag"}, true]}`,
			expected: "WHERE (FALSE OR flag OR TRUE)",
			hasError: false,
		},

		// And with falsy values
		{
			name:     "and with falsy values",
			input:    `{"and": [false, null, 0, ""]}`,
			expected: "WHERE (FALSE AND NULL AND 0 AND '')",
			hasError: false,
		},

		// Type tagging edge cases - strings with SQL keywords should be quoted as literals
		{
			name:     "string with AND keyword should be quoted",
			input:    `{"==": [{"var": "name"}, "JOHN AND JANE"]}`,
			expected: "WHERE name = 'JOHN AND JANE'",
			hasError: false,
		},
		{
			name:     "string with OR keyword should be quoted",
			input:    `{"==": [{"var": "status"}, "PASS OR FAIL"]}`,
			expected: "WHERE status = 'PASS OR FAIL'",
			hasError: false,
		},
		{
			name:     "string with parentheses should be quoted",
			input:    `{"==": [{"var": "desc"}, "Item (Large)"]}`,
			expected: "WHERE desc = 'Item (Large)'",
			hasError: false,
		},
		{
			name:     "string with LIKE keyword should be quoted",
			input:    `{"==": [{"var": "phrase"}, "I LIKE PIZZA"]}`,
			expected: "WHERE phrase = 'I LIKE PIZZA'",
			hasError: false,
		},
		{
			name:     "string with SELECT keyword should be quoted",
			input:    `{"==": [{"var": "action"}, "SELECT ITEM"]}`,
			expected: "WHERE action = 'SELECT ITEM'",
			hasError: false,
		},
		{
			name:     "string with CASE keyword should be quoted",
			input:    `{"==": [{"var": "product"}, "PHONE CASE"]}`,
			expected: "WHERE product = 'PHONE CASE'",
			hasError: false,
		},
		{
			name:     "Japanese with SQL-like parentheses should be quoted",
			input:    `{"==": [{"var": "shop"}, "SPA(スパ)"]}`,
			expected: "WHERE shop = 'SPA(スパ)'",
			hasError: false,
		},
		{
			name:     "string with equals sign should be quoted",
			input:    `{"==": [{"var": "formula"}, "x = y + z"]}`,
			expected: "WHERE formula = 'x = y + z'",
			hasError: false,
		},
		{
			name:     "string with greater than sign should be quoted",
			input:    `{"==": [{"var": "comparison"}, "A > B"]}`,
			expected: "WHERE comparison = 'A > B'",
			hasError: false,
		},
		{
			name:     "string with IN keyword should be quoted",
			input:    `{"==": [{"var": "location"}, "STORE IN MALL"]}`,
			expected: "WHERE location = 'STORE IN MALL'",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Transpile(DialectBigQuery, tt.input)

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

// TestArrayOperatorsDialectSupport tests map, filter, reduce operators for BigQuery, Spanner, PostgreSQL, and DuckDB dialects.
func TestArrayOperatorsDialectSupport(t *testing.T) {
	dialects := []struct {
		name    string
		dialect Dialect
	}{
		{"BigQuery", DialectBigQuery},
		{"Spanner", DialectSpanner},
		{"PostgreSQL", DialectPostgreSQL},
		{"DuckDB", DialectDuckDB},
	}

	for _, d := range dialects {
		t.Run(d.name, func(t *testing.T) {
			tr, err := NewTranspiler(d.dialect)
			if err != nil {
				t.Fatalf("Failed to create transpiler: %v", err)
			}

			tests := []struct {
				name     string
				input    string
				expected string
			}{
				// Map operator tests
				{
					name:     "map with var array and addition",
					input:    `{"map": [{"var": "numbers"}, {"+": [{"var": "item"}, 1]}]}`,
					expected: "WHERE ARRAY(SELECT (elem + 1) FROM UNNEST(numbers) AS elem)",
				},
				{
					name:     "map with var array and multiplication",
					input:    `{"map": [{"var": "prices"}, {"*": [{"var": "item"}, 2]}]}`,
					expected: "WHERE ARRAY(SELECT (elem * 2) FROM UNNEST(prices) AS elem)",
				},
				{
					name:     "map with literal array",
					input:    `{"map": [[1, 2, 3], {"+": [{"var": "item"}, 10]}]}`,
					expected: "WHERE ARRAY(SELECT (elem + 10) FROM UNNEST([1 2 3]) AS elem)",
				},

				// Filter operator tests
				{
					name:     "filter with var array and greater than",
					input:    `{"filter": [{"var": "scores"}, {">": [{"var": "item"}, 70]}]}`,
					expected: "WHERE ARRAY(SELECT elem FROM UNNEST(scores) AS elem WHERE elem > 70)",
				},
				{
					name:     "filter with var array and equality",
					input:    `{"filter": [{"var": "statuses"}, {"==": [{"var": "item"}, "active"]}]}`,
					expected: "WHERE ARRAY(SELECT elem FROM UNNEST(statuses) AS elem WHERE elem = 'active')",
				},
				{
					name:     "filter with literal array",
					input:    `{"filter": [[1, 2, 3, 4, 5], {">=": [{"var": "item"}, 3]}]}`,
					expected: "WHERE ARRAY(SELECT elem FROM UNNEST([1 2 3 4 5]) AS elem WHERE elem >= 3)",
				},

				// Reduce operator tests - SUM pattern
				{
					name:     "reduce with SUM pattern",
					input:    `{"reduce": [{"var": "numbers"}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}`,
					expected: "WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST(numbers) AS elem), 0)",
				},
				{
					name:     "reduce with SUM pattern and non-zero initial",
					input:    `{"reduce": [{"var": "values"}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 100]}`,
					expected: "WHERE 100 + COALESCE((SELECT SUM(elem) FROM UNNEST(values) AS elem), 0)",
				},
				{
					name:     "reduce with literal array and SUM pattern",
					input:    `{"reduce": [[10, 20, 30], {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}`,
					expected: "WHERE 0 + COALESCE((SELECT SUM(elem) FROM UNNEST([10 20 30]) AS elem), 0)",
				},

				// Reduce operator tests - MIN pattern
				{
					name:     "reduce with MIN pattern",
					input:    `{"reduce": [{"var": "values"}, {"min": [{"var": "accumulator"}, {"var": "current"}]}, 999999]}`,
					expected: "WHERE 999999 + COALESCE((SELECT MIN(elem) FROM UNNEST(values) AS elem), 0)",
				},

				// Reduce operator tests - MAX pattern
				{
					name:     "reduce with MAX pattern",
					input:    `{"reduce": [{"var": "values"}, {"max": [{"var": "accumulator"}, {"var": "current"}]}, 0]}`,
					expected: "WHERE 0 + COALESCE((SELECT MAX(elem) FROM UNNEST(values) AS elem), 0)",
				},

				// Reduce operator tests - general pattern
				{
					name:     "reduce with multiplication pattern",
					input:    `{"reduce": [{"var": "numbers"}, {"*": [{"var": "accumulator"}, {"var": "current"}]}, 1]}`,
					expected: "WHERE (SELECT (1 * current) FROM UNNEST(numbers) AS elem)",
				},

				// All operator tests
				{
					name:     "all elements satisfy condition",
					input:    `{"all": [{"var": "ages"}, {">=": [{"var": "item"}, 18]}]}`,
					expected: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(ages) AS elem WHERE NOT (elem >= 18))",
				},

				// Some operator tests
				{
					name:     "some elements satisfy condition",
					input:    `{"some": [{"var": "statuses"}, {"==": [{"var": "item"}, "active"]}]}`,
					expected: "WHERE EXISTS (SELECT 1 FROM UNNEST(statuses) AS elem WHERE elem = 'active')",
				},

				// None operator tests
				{
					name:     "no elements satisfy condition",
					input:    `{"none": [{"var": "values"}, {"==": [{"var": "item"}, "invalid"]}]}`,
					expected: "WHERE NOT EXISTS (SELECT 1 FROM UNNEST(values) AS elem WHERE elem = 'invalid')",
				},

				// Note: merge tests are in TestMergeOperatorDialectSpecific due to dialect-specific output

				// Combined/nested array operations
				{
					name:     "map in comparison",
					input:    `{">": [{"map": [{"var": "numbers"}, {"*": [{"var": "item"}, 2]}]}, 10]}`,
					expected: "WHERE ARRAY(SELECT (elem * 2) FROM UNNEST(numbers) AS elem) > 10",
				},
				{
					name:     "filter in and condition",
					input:    `{"and": [{"==": [{"var": "status"}, "active"]}, {"some": [{"var": "tags"}, {"==": [{"var": "item"}, "premium"]}]}]}`,
					expected: "WHERE (status = 'active' AND EXISTS (SELECT 1 FROM UNNEST(tags) AS elem WHERE elem = 'premium'))",
				},
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					result, err := tr.Transpile(tt.input)
					if err != nil {
						t.Errorf("[%s] Transpile() unexpected error = %v", d.name, err)
						return
					}
					if result != tt.expected {
						t.Errorf("[%s] Transpile() = %v, expected %v", d.name, result, tt.expected)
					}
				})
			}
		})
	}
}

// TestMergeOperatorDialectSpecific tests the merge operator with dialect-specific SQL output.
func TestMergeOperatorDialectSpecific(t *testing.T) {
	tests := []struct {
		name     string
		dialect  Dialect
		input    string
		expected string
	}{
		// BigQuery
		{
			name:     "BigQuery merge two arrays",
			dialect:  DialectBigQuery,
			input:    `{"merge": [{"var": "arr1"}, {"var": "arr2"}]}`,
			expected: "WHERE ARRAY_CONCAT(arr1, arr2)",
		},
		{
			name:     "BigQuery merge three arrays",
			dialect:  DialectBigQuery,
			input:    `{"merge": [{"var": "a"}, {"var": "b"}, {"var": "c"}]}`,
			expected: "WHERE ARRAY_CONCAT(a, b, c)",
		},
		// Spanner
		{
			name:     "Spanner merge two arrays",
			dialect:  DialectSpanner,
			input:    `{"merge": [{"var": "arr1"}, {"var": "arr2"}]}`,
			expected: "WHERE ARRAY_CONCAT(arr1, arr2)",
		},
		// PostgreSQL - uses || operator
		{
			name:     "PostgreSQL merge two arrays",
			dialect:  DialectPostgreSQL,
			input:    `{"merge": [{"var": "arr1"}, {"var": "arr2"}]}`,
			expected: "WHERE (arr1 || arr2)",
		},
		{
			name:     "PostgreSQL merge three arrays",
			dialect:  DialectPostgreSQL,
			input:    `{"merge": [{"var": "a"}, {"var": "b"}, {"var": "c"}]}`,
			expected: "WHERE (a || b || c)",
		},
		{
			name:     "PostgreSQL merge single array",
			dialect:  DialectPostgreSQL,
			input:    `{"merge": [{"var": "arr"}]}`,
			expected: "WHERE arr",
		},
		// DuckDB - uses ARRAY_CONCAT like BigQuery/Spanner
		{
			name:     "DuckDB merge two arrays",
			dialect:  DialectDuckDB,
			input:    `{"merge": [{"var": "arr1"}, {"var": "arr2"}]}`,
			expected: "WHERE ARRAY_CONCAT(arr1, arr2)",
		},
		{
			name:     "DuckDB merge three arrays",
			dialect:  DialectDuckDB,
			input:    `{"merge": [{"var": "a"}, {"var": "b"}, {"var": "c"}]}`,
			expected: "WHERE ARRAY_CONCAT(a, b, c)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := NewTranspiler(tt.dialect)
			if err != nil {
				t.Fatalf("Failed to create transpiler: %v", err)
			}

			result, err := tr.Transpile(tt.input)
			if err != nil {
				t.Errorf("Transpile() unexpected error = %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("Transpile() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

// TestTranspileCondition tests the TranspileCondition method that returns SQL without WHERE keyword.
func TestTranspileCondition(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple comparison without WHERE",
			input:    `{">": [{"var": "amount"}, 1000]}`,
			expected: "amount > 1000",
		},
		{
			name:     "and operation without WHERE",
			input:    `{"and": [{"==": [{"var": "status"}, "pending"]}, {">": [{"var": "amount"}, 5000]}]}`,
			expected: "(status = 'pending' AND amount > 5000)",
		},
		{
			name:     "complex nested condition without WHERE",
			input:    `{"or": [{">=": [{"var": "failedAttempts"}, 5]}, {"in": [{"var": "country"}, ["CN", "RU"]]}]}`,
			expected: "(failedAttempts >= 5 OR country IN ('CN', 'RU'))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.TranspileCondition(tt.input)
			if err != nil {
				t.Errorf("TranspileCondition() unexpected error = %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("TranspileCondition() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

// TestTranspileConditionFromMap tests the TranspileConditionFromMap method.
func TestTranspileConditionFromMap(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	logic := map[string]interface{}{
		">": []interface{}{
			map[string]interface{}{"var": "amount"},
			1000,
		},
	}

	result, err := tr.TranspileConditionFromMap(logic)
	if err != nil {
		t.Errorf("TranspileConditionFromMap() unexpected error = %v", err)
		return
	}

	expected := "amount > 1000"
	if result != expected {
		t.Errorf("TranspileConditionFromMap() = %q, expected %q", result, expected)
	}
}

// TestTranspileConditionConvenienceFunction tests the standalone TranspileCondition function.
func TestTranspileConditionConvenienceFunction(t *testing.T) {
	result, err := TranspileCondition(DialectBigQuery, `{"==": [{"var": "status"}, "active"]}`)
	if err != nil {
		t.Errorf("TranspileCondition() unexpected error = %v", err)
		return
	}

	expected := "status = 'active'"
	if result != expected {
		t.Errorf("TranspileCondition() = %q, expected %q", result, expected)
	}
}

// TestTranspileVsTranspileCondition verifies the difference between Transpile and TranspileCondition.
func TestTranspileVsTranspileCondition(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("Failed to create transpiler: %v", err)
	}

	input := `{">": [{"var": "amount"}, 1000]}`

	// Transpile should include WHERE
	withWhere, err := tr.Transpile(input)
	if err != nil {
		t.Fatalf("Transpile() error: %v", err)
	}
	if withWhere != "WHERE amount > 1000" {
		t.Errorf("Transpile() = %q, expected %q", withWhere, "WHERE amount > 1000")
	}

	// TranspileCondition should NOT include WHERE
	withoutWhere, err := tr.TranspileCondition(input)
	if err != nil {
		t.Fatalf("TranspileCondition() error: %v", err)
	}
	if withoutWhere != "amount > 1000" {
		t.Errorf("TranspileCondition() = %q, expected %q", withoutWhere, "amount > 1000")
	}
}

func TestNewTranspilerWithConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    *TranspilerConfig
		wantError bool
	}{
		{
			name:      "BigQuery dialect",
			config:    &TranspilerConfig{Dialect: DialectBigQuery},
			wantError: false,
		},
		{
			name:      "Spanner dialect",
			config:    &TranspilerConfig{Dialect: DialectSpanner},
			wantError: false,
		},
		{
			name:      "PostgreSQL dialect",
			config:    &TranspilerConfig{Dialect: DialectPostgreSQL},
			wantError: false,
		},
		{
			name:      "DuckDB dialect",
			config:    &TranspilerConfig{Dialect: DialectDuckDB},
			wantError: false,
		},
		{
			name:      "ClickHouse dialect",
			config:    &TranspilerConfig{Dialect: DialectClickHouse},
			wantError: false,
		},
		{
			name:      "unspecified dialect",
			config:    &TranspilerConfig{},
			wantError: true,
		},
		{
			name:      "nil config",
			config:    nil,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := NewTranspilerWithConfig(tt.config)
			if tt.wantError {
				if err == nil {
					t.Error("NewTranspilerWithConfig() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("NewTranspilerWithConfig() unexpected error: %v", err)
				}
				if tr == nil {
					t.Error("NewTranspilerWithConfig() returned nil transpiler")
				}
			}
		})
	}
}

func TestNewTranspilerWithConfig_WithSchema(t *testing.T) {
	schema := NewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeInteger},
		{Name: "status", Type: FieldTypeString},
	})

	config := &TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  schema,
	}

	tr, err := NewTranspilerWithConfig(config)
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() unexpected error: %v", err)
	}

	// Valid field should work
	result, err := tr.Transpile(`{">": [{"var": "amount"}, 100]}`)
	if err != nil {
		t.Errorf("Transpile() unexpected error: %v", err)
	}
	if result != "WHERE amount > 100" {
		t.Errorf("Transpile() = %q, want %q", result, "WHERE amount > 100")
	}

	// Invalid field should error
	_, err = tr.Transpile(`{">": [{"var": "invalid_field"}, 100]}`)
	if err == nil {
		t.Error("Transpile() should error for invalid field when schema is set")
	}
}

func TestTranspiler_GetDialect(t *testing.T) {
	tests := []struct {
		name    string
		dialect Dialect
	}{
		{"BigQuery", DialectBigQuery},
		{"Spanner", DialectSpanner},
		{"PostgreSQL", DialectPostgreSQL},
		{"DuckDB", DialectDuckDB},
		{"ClickHouse", DialectClickHouse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, err := NewTranspiler(tt.dialect)
			if err != nil {
				t.Fatalf("NewTranspiler() error: %v", err)
			}

			if tr.GetDialect() != tt.dialect {
				t.Errorf("GetDialect() = %v, want %v", tr.GetDialect(), tt.dialect)
			}
		})
	}
}

func TestTranspiler_TranspileConditionFromInterface(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error: %v", err)
	}

	tests := []struct {
		name     string
		input    interface{}
		expected string
		hasError bool
	}{
		{
			name: "simple comparison",
			input: map[string]interface{}{
				">": []interface{}{
					map[string]interface{}{"var": "amount"},
					1000,
				},
			},
			expected: "amount > 1000",
			hasError: false,
		},
		{
			name: "equality",
			input: map[string]interface{}{
				"==": []interface{}{
					map[string]interface{}{"var": "status"},
					"active",
				},
			},
			expected: "status = 'active'",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tr.TranspileConditionFromInterface(tt.input)
			if tt.hasError {
				if err == nil {
					t.Error("TranspileConditionFromInterface() expected error")
				}
			} else {
				if err != nil {
					t.Errorf("TranspileConditionFromInterface() unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileConditionFromInterface() = %q, want %q", result, tt.expected)
				}
			}
		})
	}
}

// DialectAwareTestOperator implements DialectAwareOperatorHandler for testing.
type DialectAwareTestOperator struct{}

func (d *DialectAwareTestOperator) ToSQLWithDialect(operator string, args []interface{}, dialect Dialect) (string, error) {
	if len(args) != 1 {
		return "", nil
	}
	switch dialect {
	case DialectBigQuery:
		return "BIGQUERY_" + args[0].(string), nil
	case DialectSpanner:
		return "SPANNER_" + args[0].(string), nil
	default:
		return "DEFAULT_" + args[0].(string), nil
	}
}

func TestTranspiler_RegisterDialectAwareOperator(t *testing.T) {
	tr, err := NewTranspiler(DialectBigQuery)
	if err != nil {
		t.Fatalf("NewTranspiler() error: %v", err)
	}

	err = tr.RegisterDialectAwareOperator("testOp", &DialectAwareTestOperator{})
	if err != nil {
		t.Fatalf("RegisterDialectAwareOperator() error: %v", err)
	}

	// Verify operator is registered
	if !tr.HasCustomOperator("testOp") {
		t.Error("testOp should be registered")
	}

	// Test that it works
	result, err := tr.Transpile(`{"testOp": [{"var": "field"}]}`)
	if err != nil {
		t.Errorf("Transpile() unexpected error: %v", err)
	}
	if result != "WHERE BIGQUERY_field" {
		t.Errorf("Transpile() = %q, want %q", result, "WHERE BIGQUERY_field")
	}
}

// TestPackageLevel_TranspileConditionFromMap tests the package-level TranspileConditionFromMap function.
func TestPackageLevel_TranspileConditionFromMap(t *testing.T) {
	tests := []struct {
		name     string
		dialect  Dialect
		logic    map[string]interface{}
		expected string
		hasError bool
	}{
		{
			name:    "simple comparison BigQuery",
			dialect: DialectBigQuery,
			logic: map[string]interface{}{
				">": []interface{}{
					map[string]interface{}{"var": "amount"},
					float64(100),
				},
			},
			expected: "amount > 100",
			hasError: false,
		},
		{
			name:    "simple comparison Spanner",
			dialect: DialectSpanner,
			logic: map[string]interface{}{
				"==": []interface{}{
					map[string]interface{}{"var": "status"},
					"active",
				},
			},
			expected: "status = 'active'",
			hasError: false,
		},
		{
			name:    "simple comparison PostgreSQL",
			dialect: DialectPostgreSQL,
			logic: map[string]interface{}{
				"<": []interface{}{
					map[string]interface{}{"var": "count"},
					float64(50),
				},
			},
			expected: "count < 50",
			hasError: false,
		},
		{
			name:    "simple comparison DuckDB",
			dialect: DialectDuckDB,
			logic: map[string]interface{}{
				">=": []interface{}{
					map[string]interface{}{"var": "value"},
					float64(10),
				},
			},
			expected: "value >= 10",
			hasError: false,
		},
		{
			name:    "simple comparison ClickHouse",
			dialect: DialectClickHouse,
			logic: map[string]interface{}{
				"<=": []interface{}{
					map[string]interface{}{"var": "score"},
					float64(75),
				},
			},
			expected: "score <= 75",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TranspileConditionFromMap(tt.dialect, tt.logic)
			if tt.hasError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileConditionFromMap() = %q, want %q", result, tt.expected)
				}
			}
		})
	}
}

// TestPackageLevel_TranspileConditionFromInterface tests the package-level TranspileConditionFromInterface function.
func TestPackageLevel_TranspileConditionFromInterface(t *testing.T) {
	tests := []struct {
		name     string
		dialect  Dialect
		logic    interface{}
		expected string
		hasError bool
	}{
		{
			name:    "from map interface BigQuery",
			dialect: DialectBigQuery,
			logic: map[string]interface{}{
				"!=": []interface{}{
					map[string]interface{}{"var": "type"},
					"unknown",
				},
			},
			expected: "type != 'unknown'",
			hasError: false,
		},
		{
			name:    "from map interface Spanner",
			dialect: DialectSpanner,
			logic: map[string]interface{}{
				"and": []interface{}{
					map[string]interface{}{
						">": []interface{}{
							map[string]interface{}{"var": "a"},
							float64(1),
						},
					},
					map[string]interface{}{
						"<": []interface{}{
							map[string]interface{}{"var": "b"},
							float64(10),
						},
					},
				},
			},
			expected: "(a > 1 AND b < 10)",
			hasError: false,
		},
		{
			name:    "from map interface DuckDB",
			dialect: DialectDuckDB,
			logic: map[string]interface{}{
				"or": []interface{}{
					map[string]interface{}{
						"==": []interface{}{
							map[string]interface{}{"var": "x"},
							float64(0),
						},
					},
					map[string]interface{}{
						"==": []interface{}{
							map[string]interface{}{"var": "y"},
							float64(0),
						},
					},
				},
			},
			expected: "(x = 0 OR y = 0)",
			hasError: false,
		},
		{
			name:    "from map interface ClickHouse",
			dialect: DialectClickHouse,
			logic: map[string]interface{}{
				"in": []interface{}{
					map[string]interface{}{"var": "status"},
					[]interface{}{"a", "b", "c"},
				},
			},
			expected: "status IN ('a', 'b', 'c')",
			hasError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := TranspileConditionFromInterface(tt.dialect, tt.logic)
			if tt.hasError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("TranspileConditionFromInterface() = %q, want %q", result, tt.expected)
				}
			}
		})
	}
}
