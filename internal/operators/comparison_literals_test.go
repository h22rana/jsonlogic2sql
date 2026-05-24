package operators

import (
	"encoding/json"
	"testing"
)

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

func TestFoldLiteralComparison_OrderingUsesJSONLogicCoercion(t *testing.T) {
	tests := []struct {
		name      string
		operator  string
		args      []interface{}
		want      bool
		wantKnown bool
	}{
		{
			name:      "string number coerces for mixed comparison",
			operator:  OpLessThan,
			args:      []interface{}{"2", float64(10)},
			want:      true,
			wantKnown: true,
		},
		{
			name:      "number string coerces false",
			operator:  OpLessThan,
			args:      []interface{}{float64(10), "2"},
			wantKnown: true,
		},
		{
			name:      "invalid numeric string folds false",
			operator:  OpLessThan,
			args:      []interface{}{"abc", float64(10)},
			wantKnown: true,
		},
		{
			name:      "string string remains lexical",
			operator:  OpLessThan,
			args:      []interface{}{"2", "10"},
			wantKnown: true,
		},
		{
			name:      "null coerces to zero",
			operator:  OpLessThan,
			args:      []interface{}{nil, float64(1)},
			want:      true,
			wantKnown: true,
		},
		{
			name:      "boolean coerces to number",
			operator:  OpLessThan,
			args:      []interface{}{true, "2"},
			want:      true,
			wantKnown: true,
		},
		{
			name:      "chained ordering folds all adjacent pairs",
			operator:  OpLessThan,
			args:      []interface{}{"1", "2", float64(3)},
			want:      true,
			wantKnown: true,
		},
		{
			name:      "chained ordering stops on false pair",
			operator:  OpLessThan,
			args:      []interface{}{"2", "10", float64(20)},
			wantKnown: true,
		},
		{
			name:      "inclusive chained ordering folds booleans",
			operator:  OpLessThanOrEqual,
			args:      []interface{}{nil, false, true, "2"},
			want:      true,
			wantKnown: true,
		},
		{
			name:      "dynamic operand remains unknown",
			operator:  OpLessThan,
			args:      []interface{}{map[string]interface{}{OpVar: "x"}, float64(10)},
			wantKnown: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, known, err := FoldLiteralComparison(tt.operator, tt.args)
			if err != nil {
				t.Fatalf("FoldLiteralComparison() error = %v", err)
			}
			if known != tt.wantKnown {
				t.Fatalf("FoldLiteralComparison() known = %v, want %v", known, tt.wantKnown)
			}
			if got != tt.want {
				t.Fatalf("FoldLiteralComparison() = %v, want %v", got, tt.want)
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
			name: "empty string needle matches empty string haystack",
			args: []interface{}{"", ""},
			want: true,
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
			name: "empty array needle matches empty string haystack",
			args: []interface{}{[]interface{}{}, ""},
			want: true,
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
