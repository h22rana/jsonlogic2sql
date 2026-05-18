package params

import (
	"testing"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

func TestNewParamCollector(t *testing.T) {
	pc := NewParamCollector(PlaceholderNamed)
	if pc.Count() != 0 {
		t.Errorf("expected 0 params, got %d", pc.Count())
	}
	if len(pc.Params()) != 0 {
		t.Errorf("expected empty params, got %v", pc.Params())
	}
	if pc.Style() != PlaceholderNamed {
		t.Errorf("expected PlaceholderNamed style, got %d", pc.Style())
	}
}

func TestParamCollectorAddNamed(t *testing.T) {
	pc := NewParamCollector(PlaceholderNamed)

	p1 := pc.Add("alice")
	if p1 != "@p1" {
		t.Errorf("expected @p1, got %s", p1)
	}

	p2 := pc.Add(42)
	if p2 != "@p2" {
		t.Errorf("expected @p2, got %s", p2)
	}

	p3 := pc.Add(3.14)
	if p3 != "@p3" {
		t.Errorf("expected @p3, got %s", p3)
	}

	if pc.Count() != 3 {
		t.Errorf("expected 3 params, got %d", pc.Count())
	}

	params := pc.Params()
	if len(params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(params))
	}

	if params[0].Name != "p1" || params[0].Value != "alice" {
		t.Errorf("param 0: expected {p1, alice}, got {%s, %v}", params[0].Name, params[0].Value)
	}
	if params[1].Name != "p2" || params[1].Value != 42 {
		t.Errorf("param 1: expected {p2, 42}, got {%s, %v}", params[1].Name, params[1].Value)
	}
	if params[2].Name != "p3" || params[2].Value != 3.14 {
		t.Errorf("param 2: expected {p3, 3.14}, got {%s, %v}", params[2].Name, params[2].Value)
	}
}

func TestParamCollectorAddPositional(t *testing.T) {
	pc := NewParamCollector(PlaceholderPositional)

	p1 := pc.Add("bob")
	if p1 != "$1" {
		t.Errorf("expected $1, got %s", p1)
	}

	p2 := pc.Add(99)
	if p2 != "$2" {
		t.Errorf("expected $2, got %s", p2)
	}

	if pc.Count() != 2 {
		t.Errorf("expected 2 params, got %d", pc.Count())
	}

	// Names are always p1, p2 regardless of style
	params := pc.Params()
	if params[0].Name != "p1" {
		t.Errorf("expected name p1, got %s", params[0].Name)
	}
	if params[1].Name != "p2" {
		t.Errorf("expected name p2, got %s", params[1].Name)
	}
}

func TestParamCollectorAddQuestion(t *testing.T) {
	pc := NewParamCollector(PlaceholderQuestion)

	p1 := pc.Add("val1")
	if p1 != "?" {
		t.Errorf("expected ?, got %s", p1)
	}

	p2 := pc.Add("val2")
	if p2 != "?" {
		t.Errorf("expected ?, got %s", p2)
	}
}

func TestParamCollectorOrdering(t *testing.T) {
	pc := NewParamCollector(PlaceholderNamed)
	pc.Add("first")
	pc.Add("second")
	pc.Add("third")

	params := pc.Params()
	if len(params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(params))
	}

	expected := []string{"first", "second", "third"}
	for i, p := range params {
		if p.Value != expected[i] {
			t.Errorf("param %d: expected value %q, got %q", i, expected[i], p.Value)
		}
	}
}

func TestStyleForDialect(t *testing.T) {
	tests := []struct {
		dialect  dialect.Dialect
		expected PlaceholderStyle
	}{
		{dialect.DialectBigQuery, PlaceholderNamed},
		{dialect.DialectSpanner, PlaceholderNamed},
		{dialect.DialectClickHouse, PlaceholderNamed},
		{dialect.DialectPostgreSQL, PlaceholderPositional},
		{dialect.DialectDuckDB, PlaceholderPositional},
	}

	for _, tt := range tests {
		t.Run(tt.dialect.String(), func(t *testing.T) {
			got := StyleForDialect(tt.dialect)
			if got != tt.expected {
				t.Errorf("StyleForDialect(%s) = %d, want %d", tt.dialect, got, tt.expected)
			}
		})
	}
}

func TestValidatePlaceholderRefsNamed(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		params    []QueryParam
		expectErr bool
	}{
		{
			name:      "single param found",
			sql:       "WHERE email = @p1",
			params:    []QueryParam{{Name: "p1", Value: "alice"}},
			expectErr: false,
		},
		{
			name:      "multiple params found",
			sql:       "WHERE email = @p1 AND age > @p2",
			params:    []QueryParam{{Name: "p1", Value: "alice"}, {Name: "p2", Value: 30}},
			expectErr: false,
		},
		{
			name:      "param missing from SQL",
			sql:       "WHERE email = @p1",
			params:    []QueryParam{{Name: "p1", Value: "alice"}, {Name: "p2", Value: 30}},
			expectErr: true,
		},
		{
			name:      "no params no error",
			sql:       "WHERE status IS NULL",
			params:    []QueryParam{},
			expectErr: false,
		},
		{
			name:      "@p1 should not match @p10",
			sql:       "WHERE x = @p10",
			params:    []QueryParam{{Name: "p1", Value: "val"}},
			expectErr: true,
		},
		{
			name:      "@p10 should match @p10",
			sql:       "WHERE x = @p10",
			params:    []QueryParam{{Name: "p10", Value: "val"}},
			expectErr: false,
		},
		{
			name:      "param at start of string",
			sql:       "@p1 = email",
			params:    []QueryParam{{Name: "p1", Value: "val"}},
			expectErr: false,
		},
		{
			name:      "param at end of string",
			sql:       "WHERE x = @p1",
			params:    []QueryParam{{Name: "p1", Value: "val"}},
			expectErr: false,
		},
		{
			name:      "param in parentheses",
			sql:       "WHERE x IN (@p1, @p2)",
			params:    []QueryParam{{Name: "p1", Value: 1}, {Name: "p2", Value: 2}},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlaceholderRefs(tt.sql, tt.params, PlaceholderNamed)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidatePlaceholderRefs() error = %v, wantErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestValidatePlaceholderRefsPositional(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		params    []QueryParam
		expectErr bool
	}{
		{
			name:      "single param found",
			sql:       "WHERE email = $1",
			params:    []QueryParam{{Name: "p1", Value: "alice"}},
			expectErr: false,
		},
		{
			name:      "multiple params found",
			sql:       "WHERE email = $1 AND age > $2",
			params:    []QueryParam{{Name: "p1", Value: "alice"}, {Name: "p2", Value: 30}},
			expectErr: false,
		},
		{
			name:      "$1 should not match $10",
			sql:       "WHERE x = $10",
			params:    []QueryParam{{Name: "p1", Value: "val"}},
			expectErr: true,
		},
		{
			name:      "$$1 should not match $1",
			sql:       "WHERE x = $$1",
			params:    []QueryParam{{Name: "p1", Value: "val"}},
			expectErr: true,
		},
		{
			name:      "param in parentheses",
			sql:       "WHERE x IN ($1, $2)",
			params:    []QueryParam{{Name: "p1", Value: 1}, {Name: "p2", Value: 2}},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlaceholderRefs(tt.sql, tt.params, PlaceholderPositional)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidatePlaceholderRefs() error = %v, wantErr %v", err, tt.expectErr)
			}
		})
	}
}

// KNOWN_LIMITATION: ValidatePlaceholderRefs can treat placeholder-like text
// inside SQL string literals as a real reference.
// TODO: Future tokenizer upgrade to skip quoted strings and SQL comments.
func TestValidatePlaceholderRefsFalsePositives(t *testing.T) {
	// A custom operator could emit SQL with @p1 inside a string literal.
	// The validator would incorrectly consider @p1 as referenced.
	sql := "WHERE x = '@p1' AND y = @p2"
	params := []QueryParam{
		{Name: "p1", Value: "not-actually-referenced"},
		{Name: "p2", Value: "referenced"},
	}

	err := ValidatePlaceholderRefs(sql, params, PlaceholderNamed)
	// The validator finds '@p1' inside the string literal and considers p1 as referenced.
	// This is a known false negative - no error, even though @p1 is inside quotes.
	if err != nil {
		t.Errorf("KNOWN_LIMITATION: expected no error (false negative), got: %v", err)
	}
}

// KNOWN_LIMITATION: ValidatePlaceholderRefs can treat placeholder-like text in
// SQL comments as a real reference.
// TODO: Future tokenizer upgrade to skip SQL comments.
func TestValidatePlaceholderRefsCommentFalsePositive(t *testing.T) {
	sql := "WHERE /* $1 */ x = $2"
	params := []QueryParam{
		{Name: "p1", Value: "inside-comment"},
		{Name: "p2", Value: "actual"},
	}

	err := ValidatePlaceholderRefs(sql, params, PlaceholderPositional)
	// The validator finds $1 inside the comment and considers p1 as referenced.
	if err != nil {
		t.Errorf("KNOWN_LIMITATION: expected no error (false negative), got: %v", err)
	}
}

func TestValueForPlaceholder(t *testing.T) {
	t.Run("named style", func(t *testing.T) {
		pc := NewParamCollector(PlaceholderNamed)
		pc.Add("hello")
		pc.Add(float64(42))

		val, ok := pc.ValueForPlaceholder("@p1")
		if !ok || val != "hello" {
			t.Errorf("ValueForPlaceholder(@p1) = %v, %v; want hello, true", val, ok)
		}
		val, ok = pc.ValueForPlaceholder("@p2")
		if !ok || val != float64(42) {
			t.Errorf("ValueForPlaceholder(@p2) = %v, %v; want 42, true", val, ok)
		}
		_, ok = pc.ValueForPlaceholder("@p3")
		if ok {
			t.Error("ValueForPlaceholder(@p3) should return false for missing placeholder")
		}
	})

	t.Run("positional style", func(t *testing.T) {
		pc := NewParamCollector(PlaceholderPositional)
		pc.Add("world")
		pc.Add(true)

		val, ok := pc.ValueForPlaceholder("$1")
		if !ok || val != "world" {
			t.Errorf("ValueForPlaceholder($1) = %v, %v; want world, true", val, ok)
		}
		val, ok = pc.ValueForPlaceholder("$2")
		if !ok || val != true {
			t.Errorf("ValueForPlaceholder($2) = %v, %v; want true, true", val, ok)
		}
		_, ok = pc.ValueForPlaceholder("@p1")
		if ok {
			t.Error("ValueForPlaceholder(@p1) should not match positional style")
		}
	})
}

func TestFindQuotedPlaceholderRef(t *testing.T) {
	t.Run("named placeholder inside quoted literal is detected", func(t *testing.T) {
		sql := "WHERE x = '@p1' AND y = @p2"
		params := []QueryParam{
			{Name: "p1", Value: "a"},
			{Name: "p2", Value: "b"},
		}
		got, ok := FindQuotedPlaceholderRef(sql, params, PlaceholderNamed)
		if !ok {
			t.Fatal("expected quoted placeholder to be detected")
		}
		if got != "@p1" {
			t.Fatalf("placeholder = %q, want %q", got, "@p1")
		}
	})

	t.Run("positional placeholder inside quoted literal is detected", func(t *testing.T) {
		sql := "WHERE x = '$1' AND y = $2"
		params := []QueryParam{
			{Name: "p1", Value: "a"},
			{Name: "p2", Value: "b"},
		}
		got, ok := FindQuotedPlaceholderRef(sql, params, PlaceholderPositional)
		if !ok {
			t.Fatal("expected quoted placeholder to be detected")
		}
		if got != "$1" {
			t.Fatalf("placeholder = %q, want %q", got, "$1")
		}
	})

	t.Run("placeholder-like text as part of larger token is ignored", func(t *testing.T) {
		sql := "WHERE email = 'name@p1.example' AND y = @p1"
		params := []QueryParam{{Name: "p1", Value: "a"}}
		if got, ok := FindQuotedPlaceholderRef(sql, params, PlaceholderNamed); ok {
			t.Fatalf("unexpected placeholder match %q", got)
		}
	})

	t.Run("escaped single quotes are handled", func(t *testing.T) {
		sql := "WHERE msg = 'it''s @p1' AND x = @p1"
		params := []QueryParam{{Name: "p1", Value: "a"}}
		got, ok := FindQuotedPlaceholderRef(sql, params, PlaceholderNamed)
		if !ok {
			t.Fatal("expected quoted placeholder to be detected")
		}
		if got != "@p1" {
			t.Fatalf("placeholder = %q, want %q", got, "@p1")
		}
	})

	t.Run("no placeholders in quoted literals", func(t *testing.T) {
		sql := "WHERE x = @p1 AND y = @p2"
		params := []QueryParam{
			{Name: "p1", Value: "a"},
			{Name: "p2", Value: "b"},
		}
		if got, ok := FindQuotedPlaceholderRef(sql, params, PlaceholderNamed); ok {
			t.Fatalf("unexpected placeholder match %q", got)
		}
	})
}
