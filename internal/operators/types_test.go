package operators

import (
	"testing"
)

func TestSQLResult(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantSQL bool
		wantVal string
	}{
		{
			name:    "simple SQL expression",
			sql:     "column_name",
			wantSQL: true,
			wantVal: "column_name",
		},
		{
			name:    "complex SQL expression",
			sql:     "CASE WHEN x > 0 THEN 'positive' ELSE 'negative' END",
			wantSQL: true,
			wantVal: "CASE WHEN x > 0 THEN 'positive' ELSE 'negative' END",
		},
		{
			name:    "empty SQL",
			sql:     "",
			wantSQL: true,
			wantVal: "",
		},
		{
			name:    "SQL with special characters",
			sql:     "(a + b) * c",
			wantSQL: true,
			wantVal: "(a + b) * c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SQLResult(tt.sql)
			if result.IsSQL != tt.wantSQL {
				t.Errorf("SQLResult().IsSQL = %v, want %v", result.IsSQL, tt.wantSQL)
			}
			if result.Value != tt.wantVal {
				t.Errorf("SQLResult().Value = %v, want %v", result.Value, tt.wantVal)
			}
		})
	}
}

func TestSQLFieldResult(t *testing.T) {
	result := SQLFieldResult("elem.a")
	if result.Value != "elem.a" {
		t.Errorf("SQLFieldResult().Value = %q, want %q", result.Value, "elem.a")
	}
	if !result.IsSQL {
		t.Error("SQLFieldResult().IsSQL = false, want true")
	}
	if !result.IsField {
		t.Error("SQLFieldResult().IsField = false, want true")
	}
}

func TestTypedSQLResult(t *testing.T) {
	result := TypedSQLResult("amount > 0", ExpressionKindPredicate, ExpressionTypeBoolean)
	if result.Value != "amount > 0" {
		t.Errorf("TypedSQLResult().Value = %q, want %q", result.Value, "amount > 0")
	}
	if !result.IsSQL {
		t.Error("TypedSQLResult().IsSQL = false, want true")
	}
	if !result.HasExpressionInfo {
		t.Error("TypedSQLResult().HasExpressionInfo = false, want true")
	}
	if result.Kind != ExpressionKindPredicate {
		t.Errorf("TypedSQLResult().Kind = %v, want %v", result.Kind, ExpressionKindPredicate)
	}
	if result.Type != ExpressionTypeBoolean {
		t.Errorf("TypedSQLResult().Type = %v, want %v", result.Type, ExpressionTypeBoolean)
	}
}

func TestLiteralResult(t *testing.T) {
	tests := []struct {
		name    string
		val     string
		wantSQL bool
		wantVal string
	}{
		{
			name:    "string literal",
			val:     "hello",
			wantSQL: false,
			wantVal: "hello",
		},
		{
			name:    "numeric string literal",
			val:     "42",
			wantSQL: false,
			wantVal: "42",
		},
		{
			name:    "empty literal",
			val:     "",
			wantSQL: false,
			wantVal: "",
		},
		{
			name:    "literal with quotes",
			val:     "it's a test",
			wantSQL: false,
			wantVal: "it's a test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LiteralResult(tt.val)
			if result.IsSQL != tt.wantSQL {
				t.Errorf("LiteralResult().IsSQL = %v, want %v", result.IsSQL, tt.wantSQL)
			}
			if result.Value != tt.wantVal {
				t.Errorf("LiteralResult().Value = %v, want %v", result.Value, tt.wantVal)
			}
		})
	}
}

func TestProcessedValue_String(t *testing.T) {
	tests := []struct {
		name string
		pv   ProcessedValue
		want string
	}{
		{
			name: "SQL result string",
			pv:   SQLResult("SELECT * FROM table"),
			want: "SELECT * FROM table",
		},
		{
			name: "literal result string",
			pv:   LiteralResult("test value"),
			want: "test value",
		},
		{
			name: "empty value",
			pv:   ProcessedValue{Value: "", IsSQL: false},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pv.String(); got != tt.want {
				t.Errorf("ProcessedValue.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProcessedValue_IsEmpty(t *testing.T) {
	tests := []struct {
		name string
		pv   ProcessedValue
		want bool
	}{
		{
			name: "empty SQL result",
			pv:   SQLResult(""),
			want: true,
		},
		{
			name: "empty literal result",
			pv:   LiteralResult(""),
			want: true,
		},
		{
			name: "non-empty SQL result",
			pv:   SQLResult("column"),
			want: false,
		},
		{
			name: "non-empty literal result",
			pv:   LiteralResult("value"),
			want: false,
		},
		{
			name: "whitespace only",
			pv:   LiteralResult(" "),
			want: false, // whitespace is not empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pv.IsEmpty(); got != tt.want {
				t.Errorf("ProcessedValue.IsEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProcessedValue_DirectConstruction(t *testing.T) {
	// Test direct struct construction
	pv := ProcessedValue{
		Value: "test",
		IsSQL: true,
	}

	if pv.Value != "test" {
		t.Errorf("Value = %v, want test", pv.Value)
	}
	if !pv.IsSQL {
		t.Error("IsSQL should be true")
	}
	if pv.String() != "test" {
		t.Errorf("String() = %v, want test", pv.String())
	}
	if pv.IsEmpty() {
		t.Error("IsEmpty() should be false")
	}
}
