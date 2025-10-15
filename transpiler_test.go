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
			input:    `{"missing": ["field"]}`,
			expected: "WHERE field IS NULL",
			hasError: false,
		},
		{
			name:     "missing_some operation",
			input:    `{"missing_some": [1, ["field1", "field2"]]}`,
			expected: "WHERE (field1 IS NULL + field2 IS NULL) >= 1",
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

// Test the examples from the original request
func TestOriginalExamples(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Simple Comparison",
			input:    `{">": [{"var": "amount"}, 1000]}`,
			expected: "WHERE amount > 1000",
		},
		{
			name:     "Multiple Conditions (AND)",
			input:    `{"and": [{">": [{"var": "amount"}, 5000]}, {"==": [{"var": "status"}, "pending"]}]}`,
			expected: "WHERE (amount > 5000 AND status = 'pending')",
		},
		{
			name:     "Multiple Conditions (OR)",
			input:    `{"or": [{">=": [{"var": "failedAttempts"}, 5]}, {"in": [{"var": "country"}, ["CN", "RU"]]}]}`,
			expected: "WHERE (failedAttempts >= 5 OR country IN ('CN', 'RU'))",
		},
		{
			name:     "Nested Conditions",
			input:    `{"and": [{">": [{"var": "transaction.amount"}, 10000]}, {"or": [{"==": [{"var": "user.verified"}, false]}, {"<": [{"var": "user.accountAgeDays"}, 7]}]}]}`,
			expected: "WHERE (transaction.amount > 10000 AND (user.verified = FALSE OR user.accountAgeDays < 7))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Transpile(tt.input)
			if err != nil {
				t.Errorf("Transpile() unexpected error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("Transpile() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
