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
			name:  "or short-circuits truthy constant",
			logic: `{"or":[{"always":["x"]},false]}`,
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
					tr, err := NewTranspilerWithConfig(&TranspilerConfig{Dialect: d})
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
