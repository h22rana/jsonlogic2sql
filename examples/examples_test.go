package examples

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql"
)

// TestOriginalExamples tests the examples provided in the original request
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
			result, err := jsonlogic2sql.Transpile(tt.input)
			if err != nil {
				t.Errorf("Transpile() unexpected error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("Transpile() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestAdditionalExamples tests additional JSON Logic patterns
func TestAdditionalExamples(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "IF Statement",
			input:    `{"if": [{">": [{"var": "age"}, 18]}, "adult", "minor"]}`,
			expected: "WHERE CASE WHEN age > 18 THEN 'adult' ELSE 'minor' END",
		},
		{
			name:     "Missing Field Check",
			input:    `{"missing": ["field"]}`,
			expected: "WHERE field IS NULL",
		},
		{
			name:     "Missing Some Fields",
			input:    `{"missing_some": [1, ["field1", "field2"]]}`,
			expected: "WHERE (field1 IS NULL + field2 IS NULL) >= 1",
		},
		{
			name:     "NOT Operation",
			input:    `{"!": [{"==": [{"var": "verified"}, true]}]}`,
			expected: "WHERE NOT (verified = TRUE)",
		},
		{
			name:     "Double NOT (Boolean Conversion)",
			input:    `{"!!": [{"var": "value"}]}`,
			expected: "WHERE CASE WHEN value THEN TRUE ELSE FALSE END",
		},
		{
			name:     "Variable with Default",
			input:    `{"var": ["status", "pending"]}`,
			expected: "WHERE COALESCE(status, 'pending')",
		},
		{
			name:     "Complex Nested Logic",
			input:    `{"and": [{"!": [{"missing": "email"}]}, {"or": [{"==": [{"var": "role"}, "admin"]}, {"and": [{"==": [{"var": "role"}, "user"]}, {">": [{"var": "score"}, 100]}]}]}]}`,
			expected: "WHERE (NOT (email IS NULL) AND (role = 'admin' OR (role = 'user' AND score > 100)))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := jsonlogic2sql.Transpile(tt.input)
			if err != nil {
				t.Errorf("Transpile() unexpected error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("Transpile() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

// TestErrorCases tests error handling
func TestErrorCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Invalid JSON",
			input: `{invalid json}`,
		},
		{
			name:  "Unsupported Operator",
			input: `{"unsupported": [1, 2]}`,
		},
		{
			name:  "Empty Expression",
			input: ``,
		},
		{
			name:  "Null Expression",
			input: `null`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := jsonlogic2sql.Transpile(tt.input)
			if err == nil {
				t.Errorf("Transpile() expected error for %s, got result: %s", tt.name, result)
			}
		})
	}
}
