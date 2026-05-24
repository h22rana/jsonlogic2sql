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

func TestParamCollectorAddClickHouse(t *testing.T) {
	pc := NewParamCollector(PlaceholderClickHouse)

	if got := pc.Add("alice"); got != "{p1:String}" {
		t.Fatalf("string placeholder = %q, want {p1:String}", got)
	}
	if got := pc.Add(42); got != "{p2:Int64}" {
		t.Fatalf("integer placeholder = %q, want {p2:Int64}", got)
	}
	if got := pc.Add(3.14); got != "{p3:Float64}" {
		t.Fatalf("float placeholder = %q, want {p3:Float64}", got)
	}
}

func TestParamCollectorAddClickHouseExactNumberString(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "int64 range", value: "9007199254740993", want: "{p1:Int64}"},
		{name: "uint64 range", value: "9223372036854775808", want: "{p1:UInt64}"},
		{name: "uint128 range", value: "18446744073709551616", want: "{p1:UInt128}"},
		{name: "int128 range", value: "-9223372036854775809", want: "{p1:Int128}"},
		{name: "float overflow literal", value: "1e400", want: "{p1:Float64}"},
		{name: "float underflow literal", value: "1e-400", want: "{p1:Float64}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewParamCollector(PlaceholderClickHouse)
			if got := pc.AddExactNumberString(tt.value); got != tt.want {
				t.Fatalf("AddExactNumberString(%q) = %q, want %q", tt.value, got, tt.want)
			}
			gotParams := pc.Params()
			if len(gotParams) != 1 || gotParams[0].Value != tt.value {
				t.Fatalf("Params() = %#v, want exact public value %q", gotParams, tt.value)
			}
			if pc.PlaceholderValueIsString(tt.want) {
				t.Fatalf("PlaceholderValueIsString(%q) = true, want false for exact numeric string", tt.want)
			}
		})
	}
}

func TestParamCollectorAddNumericString(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantSQL   string
		wantValue interface{}
	}{
		{
			name:      "integer",
			value:     "1",
			wantSQL:   "{p1:Int64}",
			wantValue: int64(1),
		},
		{
			name:      "float",
			value:     "1.5",
			wantSQL:   "{p1:Float64}",
			wantValue: 1.5,
		},
		{
			name:      "exact unsigned integer",
			value:     "9223372036854775808",
			wantSQL:   "{p1:UInt64}",
			wantValue: "9223372036854775808",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewParamCollector(PlaceholderClickHouse)
			gotSQL, ok := pc.AddNumericString(tt.value)
			if !ok {
				t.Fatalf("AddNumericString(%q) returned false", tt.value)
			}
			if gotSQL != tt.wantSQL {
				t.Fatalf("AddNumericString(%q) = %q, want %q", tt.value, gotSQL, tt.wantSQL)
			}
			gotParams := pc.Params()
			if len(gotParams) != 1 || gotParams[0].Value != tt.wantValue {
				t.Fatalf("Params() = %#v, want value %#v", gotParams, tt.wantValue)
			}
		})
	}
}

func TestParamCollectorAddNumericStringRejectsNonNumericStrings(t *testing.T) {
	pc := NewParamCollector(PlaceholderClickHouse)
	if gotSQL, ok := pc.AddNumericString("not numeric"); ok {
		t.Fatalf("AddNumericString(non-numeric) = %q, true; want false", gotSQL)
	}
}

func TestParamCollectorRewriteStringParamAsNumeric(t *testing.T) {
	tests := []struct {
		name      string
		style     PlaceholderStyle
		value     string
		wantSQL   string
		wantValue interface{}
	}{
		{
			name:      "named integer",
			style:     PlaceholderNamed,
			value:     "1",
			wantSQL:   "@p1",
			wantValue: int64(1),
		},
		{
			name:      "clickhouse integer",
			style:     PlaceholderClickHouse,
			value:     "1",
			wantSQL:   "{p1:Int64}",
			wantValue: int64(1),
		},
		{
			name:      "clickhouse float",
			style:     PlaceholderClickHouse,
			value:     "1.5",
			wantSQL:   "{p1:Float64}",
			wantValue: 1.5,
		},
		{
			name:      "clickhouse exact unsigned integer",
			style:     PlaceholderClickHouse,
			value:     "9223372036854775808",
			wantSQL:   "{p1:UInt64}",
			wantValue: "9223372036854775808",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := NewParamCollector(tt.style)
			_ = pc.Add(tt.value)

			gotSQL, ok := pc.RewriteStringParamAsNumeric(0, tt.value)
			if !ok {
				t.Fatalf("RewriteStringParamAsNumeric(%q) returned false", tt.value)
			}
			if gotSQL != tt.wantSQL {
				t.Fatalf("RewriteStringParamAsNumeric(%q) = %q, want %q", tt.value, gotSQL, tt.wantSQL)
			}

			gotParams := pc.Params()
			if len(gotParams) != 1 || gotParams[0].Value != tt.wantValue {
				t.Fatalf("Params() = %#v, want value %#v", gotParams, tt.wantValue)
			}
			if pc.PlaceholderValueIsString(gotSQL) {
				t.Fatalf("PlaceholderValueIsString(%q) = true, want false after numeric rewrite", gotSQL)
			}
		})
	}
}

func TestParamCollectorRewriteStringParamAsNumericRejectsMismatches(t *testing.T) {
	pc := NewParamCollector(PlaceholderClickHouse)
	_ = pc.Add("1")

	if gotSQL, ok := pc.RewriteStringParamAsNumeric(0, "2"); ok {
		t.Fatalf("RewriteStringParamAsNumeric with mismatched value = %q, true; want false", gotSQL)
	}
	if gotSQL, ok := pc.RewriteStringParamAsNumeric(1, "1"); ok {
		t.Fatalf("RewriteStringParamAsNumeric with out-of-range index = %q, true; want false", gotSQL)
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
		{dialect.DialectClickHouse, PlaceholderClickHouse},
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
		{
			name:      "quoted placeholder is ignored",
			sql:       "WHERE note = '@p1' AND y = @p2",
			params:    []QueryParam{{Name: "p1", Value: "inside-string"}, {Name: "p2", Value: "referenced"}},
			expectErr: true,
		},
		{
			name:      "double-quoted placeholder is ignored",
			sql:       `WHERE ident = "@p1" AND y = @p2`,
			params:    []QueryParam{{Name: "p1", Value: "inside-identifier"}, {Name: "p2", Value: "referenced"}},
			expectErr: true,
		},
		{
			name:      "backtick-quoted placeholder is ignored",
			sql:       "WHERE ident = `@p1` AND y = @p2",
			params:    []QueryParam{{Name: "p1", Value: "inside-identifier"}, {Name: "p2", Value: "referenced"}},
			expectErr: true,
		},
		{
			name:      "escaped backtick identifier placeholder is ignored",
			sql:       "WHERE ident = `field``@p1` AND y = @p2",
			params:    []QueryParam{{Name: "p1", Value: "inside-identifier"}, {Name: "p2", Value: "referenced"}},
			expectErr: true,
		},
		{
			name:      "escaped quote string placeholder is ignored",
			sql:       "WHERE note = 'it''s @p1' AND y = @p2",
			params:    []QueryParam{{Name: "p1", Value: "inside-string"}, {Name: "p2", Value: "referenced"}},
			expectErr: true,
		},
		{
			name:      "line comment placeholder is ignored",
			sql:       "WHERE -- @p1\n y = @p2",
			params:    []QueryParam{{Name: "p1", Value: "inside-comment"}, {Name: "p2", Value: "referenced"}},
			expectErr: true,
		},
		{
			name:      "block comment placeholder is ignored",
			sql:       "WHERE /* @p1 */ y = @p2",
			params:    []QueryParam{{Name: "p1", Value: "inside-comment"}, {Name: "p2", Value: "referenced"}},
			expectErr: true,
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
		{
			name:      "quoted placeholder is ignored",
			sql:       "WHERE x = '$1' AND y = $2",
			params:    []QueryParam{{Name: "p1", Value: "inside-string"}, {Name: "p2", Value: "actual"}},
			expectErr: true,
		},
		{
			name:      "double-quoted placeholder is ignored",
			sql:       `WHERE x = "$1" AND y = $2`,
			params:    []QueryParam{{Name: "p1", Value: "inside-identifier"}, {Name: "p2", Value: "actual"}},
			expectErr: true,
		},
		{
			name:      "escaped double-quoted identifier placeholder is ignored",
			sql:       `WHERE x = "field""$1" AND y = $2`,
			params:    []QueryParam{{Name: "p1", Value: "inside-identifier"}, {Name: "p2", Value: "actual"}},
			expectErr: true,
		},
		{
			name:      "backtick-quoted placeholder is ignored",
			sql:       "WHERE x = `$1` AND y = $2",
			params:    []QueryParam{{Name: "p1", Value: "inside-identifier"}, {Name: "p2", Value: "actual"}},
			expectErr: true,
		},
		{
			name:      "line comment placeholder is ignored",
			sql:       "WHERE -- $1\n x = $2",
			params:    []QueryParam{{Name: "p1", Value: "inside-comment"}, {Name: "p2", Value: "actual"}},
			expectErr: true,
		},
		{
			name:      "block comment placeholder is ignored",
			sql:       "WHERE /* $1 */ x = $2",
			params:    []QueryParam{{Name: "p1", Value: "inside-comment"}, {Name: "p2", Value: "actual"}},
			expectErr: true,
		},
		{
			name:      "dollar-quoted placeholder is ignored",
			sql:       "WHERE x = $tag$ $1 $tag$ AND y = $2",
			params:    []QueryParam{{Name: "p1", Value: "inside-dollar-quote"}, {Name: "p2", Value: "actual"}},
			expectErr: true,
		},
		{
			name:      "empty-tag dollar-quoted placeholder is ignored",
			sql:       "WHERE x = $$ $1 $$ AND y = $2",
			params:    []QueryParam{{Name: "p1", Value: "inside-dollar-quote"}, {Name: "p2", Value: "actual"}},
			expectErr: true,
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

func TestValidatePlaceholderRefsQuestion(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		params    []QueryParam
		expectErr bool
	}{
		{
			name:      "one question placeholder per param",
			sql:       "WHERE email = ? AND age > ?",
			params:    []QueryParam{{Name: "p1", Value: "alice"}, {Name: "p2", Value: 30}},
			expectErr: false,
		},
		{
			name:      "quoted question is ignored",
			sql:       "WHERE note = '?' AND email = ?",
			params:    []QueryParam{{Name: "p1", Value: "alice"}},
			expectErr: false,
		},
		{
			name:      "line comment question is ignored",
			sql:       "WHERE -- ?\n email = ?",
			params:    []QueryParam{{Name: "p1", Value: "alice"}},
			expectErr: false,
		},
		{
			name:      "block comment question is ignored",
			sql:       "WHERE /* ? */ email = ?",
			params:    []QueryParam{{Name: "p1", Value: "alice"}},
			expectErr: false,
		},
		{
			name:      "dollar-quoted question is ignored",
			sql:       "WHERE note = $tag$ ? $tag$ AND email = ?",
			params:    []QueryParam{{Name: "p1", Value: "alice"}},
			expectErr: false,
		},
		{
			name:      "too few bindable question placeholders",
			sql:       "WHERE email = ?",
			params:    []QueryParam{{Name: "p1", Value: "alice"}, {Name: "p2", Value: 30}},
			expectErr: true,
		},
		{
			name:      "only quoted question is not bindable",
			sql:       "WHERE note = '?'",
			params:    []QueryParam{{Name: "p1", Value: "alice"}},
			expectErr: true,
		},
		{
			name:      "no params no error",
			sql:       "WHERE note = '?'",
			params:    []QueryParam{},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlaceholderRefs(tt.sql, tt.params, PlaceholderQuestion)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidatePlaceholderRefs() error = %v, wantErr %v", err, tt.expectErr)
			}
		})
	}
}

func TestValidatePlaceholderRefsClickHouse(t *testing.T) {
	tests := []struct {
		name      string
		sql       string
		params    []QueryParam
		expectErr bool
	}{
		{
			name:      "single typed param found",
			sql:       "WHERE email = {p1:String}",
			params:    []QueryParam{{Name: "p1", Value: "alice"}},
			expectErr: false,
		},
		{
			name:      "numeric param found",
			sql:       "WHERE age > {p1:Int64}",
			params:    []QueryParam{{Name: "p1", Value: int64(30)}},
			expectErr: false,
		},
		{
			name:      "wrong type does not match",
			sql:       "WHERE age > {p1:String}",
			params:    []QueryParam{{Name: "p1", Value: int64(30)}},
			expectErr: true,
		},
		{
			name:      "quoted typed placeholder is ignored",
			sql:       "WHERE note = '{p1:String}' AND email = {p2:String}",
			params:    []QueryParam{{Name: "p1", Value: "inside-string"}, {Name: "p2", Value: "actual"}},
			expectErr: true,
		},
		{
			name:      "backtick quoted typed placeholder is ignored",
			sql:       "WHERE ident = `{p1:String}` AND email = {p2:String}",
			params:    []QueryParam{{Name: "p1", Value: "inside-identifier"}, {Name: "p2", Value: "actual"}},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePlaceholderRefs(tt.sql, tt.params, PlaceholderClickHouse)
			if (err != nil) != tt.expectErr {
				t.Errorf("ValidatePlaceholderRefs() error = %v, wantErr %v", err, tt.expectErr)
			}
		})
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

	t.Run("string inference excludes exact numeric strings", func(t *testing.T) {
		pc := NewParamCollector(PlaceholderNamed)
		pc.Add("actual string")
		pc.AddExactNumberString("9007199254740993")

		if !pc.PlaceholderValueIsString("@p1") {
			t.Error("PlaceholderValueIsString(@p1) = false, want true for actual string")
		}
		if pc.PlaceholderValueIsString("@p2") {
			t.Error("PlaceholderValueIsString(@p2) = true, want false for exact numeric string")
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

	t.Run("named placeholder inside double-quoted identifier is detected", func(t *testing.T) {
		sql := `WHERE x = "@p1" AND y = @p2`
		params := []QueryParam{
			{Name: "p1", Value: "a"},
			{Name: "p2", Value: "b"},
		}
		got, ok := FindQuotedPlaceholderRef(sql, params, PlaceholderNamed)
		if !ok {
			t.Fatal("expected double-quoted placeholder to be detected")
		}
		if got != "@p1" {
			t.Fatalf("placeholder = %q, want %q", got, "@p1")
		}
	})

	t.Run("named placeholder inside backtick-quoted identifier is detected", func(t *testing.T) {
		sql := "WHERE x = `@p1` AND y = @p2"
		params := []QueryParam{
			{Name: "p1", Value: "a"},
			{Name: "p2", Value: "b"},
		}
		got, ok := FindQuotedPlaceholderRef(sql, params, PlaceholderNamed)
		if !ok {
			t.Fatal("expected backtick-quoted placeholder to be detected")
		}
		if got != "@p1" {
			t.Fatalf("placeholder = %q, want %q", got, "@p1")
		}
	})

	t.Run("positional placeholder inside double-quoted identifier is detected", func(t *testing.T) {
		sql := `WHERE x = "$1" AND y = $2`
		params := []QueryParam{
			{Name: "p1", Value: "a"},
			{Name: "p2", Value: "b"},
		}
		got, ok := FindQuotedPlaceholderRef(sql, params, PlaceholderPositional)
		if !ok {
			t.Fatal("expected double-quoted placeholder to be detected")
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

	t.Run("escaped double quotes are handled", func(t *testing.T) {
		sql := `WHERE ident = "field""$1" AND x = $1`
		params := []QueryParam{{Name: "p1", Value: "a"}}
		got, ok := FindQuotedPlaceholderRef(sql, params, PlaceholderPositional)
		if !ok {
			t.Fatal("expected double-quoted placeholder to be detected")
		}
		if got != "$1" {
			t.Fatalf("placeholder = %q, want %q", got, "$1")
		}
	})

	t.Run("escaped backticks are handled", func(t *testing.T) {
		sql := "WHERE ident = `field``@p1` AND x = @p1"
		params := []QueryParam{{Name: "p1", Value: "a"}}
		got, ok := FindQuotedPlaceholderRef(sql, params, PlaceholderNamed)
		if !ok {
			t.Fatal("expected backtick-quoted placeholder to be detected")
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

	t.Run("positional placeholder inside dollar-quoted literal is detected", func(t *testing.T) {
		sql := "WHERE x = $tag$ $1 $tag$ AND y = $2"
		params := []QueryParam{
			{Name: "p1", Value: "a"},
			{Name: "p2", Value: "b"},
		}
		got, ok := FindQuotedPlaceholderRef(sql, params, PlaceholderPositional)
		if !ok {
			t.Fatal("expected dollar-quoted placeholder to be detected")
		}
		if got != "$1" {
			t.Fatalf("placeholder = %q, want %q", got, "$1")
		}
	})

	t.Run("positional placeholder inside empty-tag dollar-quoted literal is detected", func(t *testing.T) {
		sql := "WHERE x = $$ $1 $$ AND y = $2"
		params := []QueryParam{
			{Name: "p1", Value: "a"},
			{Name: "p2", Value: "b"},
		}
		got, ok := FindQuotedPlaceholderRef(sql, params, PlaceholderPositional)
		if !ok {
			t.Fatal("expected empty-tag dollar-quoted placeholder to be detected")
		}
		if got != "$1" {
			t.Fatalf("placeholder = %q, want %q", got, "$1")
		}
	})

	t.Run("scoped check ignores earlier named placeholder text", func(t *testing.T) {
		sql := "WHERE x = '@p1' AND y = @p1"
		params := []QueryParam{{Name: "p1", Value: "a"}}
		if got, ok := FindQuotedPlaceholderRefAfter(sql, params, PlaceholderNamed, 1); ok {
			t.Fatalf("unexpected earlier placeholder match %q", got)
		}
	})

	t.Run("scoped check detects later named placeholder text", func(t *testing.T) {
		sql := "WHERE x = '@p2' AND y = @p1"
		params := []QueryParam{
			{Name: "p1", Value: "a"},
			{Name: "p2", Value: "b"},
		}
		got, ok := FindQuotedPlaceholderRefAfter(sql, params, PlaceholderNamed, 1)
		if !ok {
			t.Fatal("expected later named placeholder to be detected")
		}
		if got != "@p2" {
			t.Fatalf("placeholder = %q, want %q", got, "@p2")
		}
	})

	t.Run("scoped check ignores earlier positional placeholder text", func(t *testing.T) {
		sql := "WHERE x = '$1' AND y = $1"
		params := []QueryParam{{Name: "p1", Value: "a"}}
		if got, ok := FindQuotedPlaceholderRefAfter(sql, params, PlaceholderPositional, 1); ok {
			t.Fatalf("unexpected earlier placeholder match %q", got)
		}
	})

	t.Run("scoped check detects later positional placeholder text", func(t *testing.T) {
		sql := "WHERE x = '$2' AND y = $1"
		params := []QueryParam{
			{Name: "p1", Value: "a"},
			{Name: "p2", Value: "b"},
		}
		got, ok := FindQuotedPlaceholderRefAfter(sql, params, PlaceholderPositional, 1)
		if !ok {
			t.Fatal("expected later positional placeholder to be detected")
		}
		if got != "$2" {
			t.Fatalf("placeholder = %q, want %q", got, "$2")
		}
	})
}
