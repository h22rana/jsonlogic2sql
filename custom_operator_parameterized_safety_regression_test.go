package jsonlogic2sql

import (
	"strings"
	"testing"
)

func TestParameterizedCustomPredicateConstantsPreserveDroppedParamDetection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		logic     string
		valueMode bool
	}{
		{
			name:  "direct predicate",
			logic: `{"always":["x"]}`,
		},
		{
			name:  "double bang truthiness",
			logic: `{"!!":{"always":["x"]}}`,
		},
		{
			name:  "and skips truthy constant",
			logic: `{"and":[{"always":["x"]},true]}`,
		},
		{
			name:  "nested and preserves dropped-param marker",
			logic: `{"and":[{"and":[{"always":["x"]},true]},true]}`,
		},
		{
			name:  "or short-circuits truthy constant",
			logic: `{"or":[{"always":["x"]},false]}`,
		},
		{
			name:  "nested or preserves dropped-param marker",
			logic: `{"or":[{"or":[{"always":["x"]},false]},false]}`,
		},
		{
			name:  "double bang nested logical preserves dropped-param marker",
			logic: `{"!!":{"and":[{"always":["x"]},true]}}`,
		},
		{
			name:  "outer logical preserves nested double bang marker",
			logic: `{"and":[{"!!":{"and":[{"always":["x"]},true]}},true]}`,
		},
		{
			name:      "value double bang truthiness",
			logic:     `{"!!":{"always":["x"]}}`,
			valueMode: true,
		},
		{
			name:      "value and skips truthy constant",
			logic:     `{"and":[{"always":["x"]},true]}`,
			valueMode: true,
		},
		{
			name:      "value nested and preserves dropped-param marker",
			logic:     `{"and":[{"and":[{"always":["x"]},true]},true]}`,
			valueMode: true,
		},
		{
			name:      "value or preserves nested logical marker",
			logic:     `{"or":[{"and":[{"always":["x"]},true]},"fallback"]}`,
			valueMode: true,
		},
		{
			name:      "value or short-circuits truthy constant",
			logic:     `{"or":[{"always":["x"]},false]}`,
			valueMode: true,
		},
		{
			name:      "cat stringified logical skips truthy constant",
			logic:     `{"cat":[{"and":[{"always":["x"]},"ok"]}]}`,
			valueMode: true,
		},
		{
			name:      "cat stringified nested logical preserves dropped-param marker",
			logic:     `{"cat":[{"and":[{"and":[{"always":["x"]},true]},"ok"]}]}`,
			valueMode: true,
		},
		{
			name:  "comparison folds custom predicate operand",
			logic: `{"==":[{"always":["x"]},true]}`,
		},
		{
			name:  "and skips folded custom comparison",
			logic: `{"and":[{"==":[{"always":["x"]},true]},true]}`,
		},
		{
			name:      "value comparison folds custom predicate operand",
			logic:     `{"==":[{"always":["x"]},true]}`,
			valueMode: true,
		},
		{
			name:      "value or short-circuits folded custom comparison",
			logic:     `{"or":[{"==":[{"always":["x"]},true]},"fallback"]}`,
			valueMode: true,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					tr, err := NewTranspilerWithConfig(&TranspilerConfig{Dialect: d, Schema: defaultTestSchema()})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error: %v", err)
					}
					if regErr := tr.RegisterOperatorFunc("always", func(_ string, _ []OperatorArg) (OperatorResult, error) {
						return PredicateSQL("TRUE"), nil
					}); regErr != nil {
						t.Fatalf("RegisterOperatorFunc(always) error: %v", regErr)
					}

					var sql string
					var params []QueryParam
					if tc.valueMode {
						sql, params, err = tr.TranspileParameterizedValue(tc.logic)
					} else {
						sql, params, err = tr.TranspileParameterizedCondition(tc.logic)
					}
					if !IsErrorCode(err, ErrUnreferencedPlaceholder) {
						t.Fatalf("parameterized transpilation error = %v, want %s (SQL %q params %#v)",
							err, ErrUnreferencedPlaceholder, sql, params)
					}
					if !strings.Contains(err.Error(), "custom operator may have dropped an argument") {
						t.Fatalf("error missing custom-operator safety message: %v", err)
					}
				})
			}
		})
	}
}

func TestParameterizedCustomValueFoldedComparisonRollsBackParserDroppedParams(t *testing.T) {
	t.Parallel()

	logic := `{"===":[{"idstr":["x"]},1]}`

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{Dialect: d, Schema: defaultTestSchema()})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error: %v", err)
			}
			if regErr := tr.RegisterOperatorFunc("idstr", func(_ string, args []OperatorArg) (OperatorResult, error) {
				return ValueSQL(args[0].SQL, ExpressionTypeString), nil
			}); regErr != nil {
				t.Fatalf("RegisterOperatorFunc(idstr) error: %v", regErr)
			}

			gotCondition, conditionParams, err := tr.TranspileParameterizedCondition(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotCondition != "FALSE" {
				t.Fatalf("TranspileParameterizedCondition() = %q, want FALSE", gotCondition)
			}
			if len(conditionParams) != 0 {
				t.Fatalf("condition params = %#v, want none", conditionParams)
			}

			gotValue, valueParams, err := tr.TranspileParameterizedValue(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			if gotValue != "FALSE" {
				t.Fatalf("TranspileParameterizedValue() = %q, want FALSE", gotValue)
			}
			if len(valueParams) != 0 {
				t.Fatalf("value params = %#v, want none", valueParams)
			}
		})
	}
}

func TestParameterizedCustomNullValueFoldedComparisonPreservesDroppedParamDetection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		logic     string
		valueMode bool
	}{
		{
			name:  "condition strict null equality",
			logic: `{"===":[{"nuller":["x"]},null]}`,
		},
		{
			name:      "value strict null equality",
			logic:     `{"===":[{"nuller":["x"]},null]}`,
			valueMode: true,
		},
		{
			name:  "condition loose null equality",
			logic: `{"==":[{"nuller":["x"]},null]}`,
		},
		{
			name:      "value loose null equality",
			logic:     `{"==":[{"nuller":["x"]},null]}`,
			valueMode: true,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					tr, err := NewTranspilerWithConfig(&TranspilerConfig{Dialect: d, Schema: defaultTestSchema()})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error: %v", err)
					}
					if regErr := tr.RegisterOperatorFunc("nuller", func(_ string, _ []OperatorArg) (OperatorResult, error) {
						return ValueSQL("NULL", ExpressionTypeNull), nil
					}); regErr != nil {
						t.Fatalf("RegisterOperatorFunc(nuller) error: %v", regErr)
					}

					var sql string
					var params []QueryParam
					if tc.valueMode {
						sql, params, err = tr.TranspileParameterizedValue(tc.logic)
					} else {
						sql, params, err = tr.TranspileParameterizedCondition(tc.logic)
					}
					if !IsErrorCode(err, ErrUnreferencedPlaceholder) {
						t.Fatalf("parameterized transpilation error = %v, want %s (SQL %q params %#v)",
							err, ErrUnreferencedPlaceholder, sql, params)
					}
					if !strings.Contains(err.Error(), "custom operator may have dropped an argument") {
						t.Fatalf("error missing custom-operator safety message: %v", err)
					}
				})
			}
		})
	}
}

func TestParameterizedCustomOperatorRejectsPostgreSQLDollarQuotedPlaceholders(t *testing.T) {
	t.Parallel()

	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectPostgreSQL,
		Schema:  defaultTestSchema(),
	})
	if err != nil {
		t.Fatalf("NewTranspilerWithConfig() error: %v", err)
	}
	if regErr := tr.RegisterOperatorFunc("dollarquote", func(_ string, args []OperatorArg) (OperatorResult, error) {
		return ValueSQL("$tag$ "+args[0].SQL+" $tag$", ExpressionTypeString), nil
	}); regErr != nil {
		t.Fatalf("RegisterOperatorFunc(dollarquote) error: %v", regErr)
	}

	sql, gotParams, err := tr.TranspileParameterizedValue(`{"dollarquote":["x"]}`)
	if !IsErrorCode(err, ErrCustomOperatorFailed) {
		t.Fatalf("TranspileParameterizedValue() SQL = %q params %#v error = %v, want %s",
			sql, gotParams, err, ErrCustomOperatorFailed)
	}
	if !strings.Contains(err.Error(), "placeholder $1 appears inside a quoted SQL region") {
		t.Fatalf("error = %v, want dollar-quoted placeholder message", err)
	}
}
