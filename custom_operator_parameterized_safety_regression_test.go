package jsonlogic2sql

import (
	"strings"
	"testing"
)

func TestParameterizedCustomPredicateConstantsPreserveDroppedParamDetection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                         string
		logic                        string
		valueMode                    bool
		skipArrayLiteralElemDialects bool
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
			name:  "and skips folded if with custom condition",
			logic: `{"and":[{"if":[{"always":["x"]},true,false]},true]}`,
		},
		{
			name:  "double bang folded if preserves custom condition marker",
			logic: `{"!!":{"if":[{"always":["x"]},true,false]}}`,
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
		{
			name:      "value double bang folded if preserves custom condition marker",
			logic:     `{"!!":{"if":[{"always":["x"]},true,false]}}`,
			valueMode: true,
		},
		{
			name:      "value filter lambda preserves custom predicate marker",
			logic:     `{"filter":[[1],{"always":["x"]}]}`,
			valueMode: true,
		},
		{
			name:  "some lambda preserves custom predicate marker",
			logic: `{"some":[[1],{"always":["x"]}]}`,
		},
		{
			name:  "all lambda preserves custom predicate marker",
			logic: `{"all":[[1],{"always":["x"]}]}`,
		},
		{
			name:  "none lambda preserves custom predicate marker",
			logic: `{"none":[[1],{"always":["x"]}]}`,
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
					var transpileErr error
					if tc.valueMode {
						sql, params, transpileErr = tr.TranspileParameterizedValue(tc.logic)
					} else {
						sql, params, transpileErr = tr.TranspileParameterizedCondition(tc.logic)
					}
					if !IsErrorCode(transpileErr, ErrUnreferencedPlaceholder) {
						t.Fatalf("parameterized transpilation error = %v, want %s (SQL %q params %#v)",
							transpileErr, ErrUnreferencedPlaceholder, sql, params)
					}
					if !strings.Contains(transpileErr.Error(), "custom operator may have dropped an argument") {
						t.Fatalf("error missing custom-operator safety message: %v", transpileErr)
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
		name                         string
		logic                        string
		valueMode                    bool
		skipArrayLiteralElemDialects bool
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
					var transpileErr error
					if tc.valueMode {
						sql, params, transpileErr = tr.TranspileParameterizedValue(tc.logic)
					} else {
						sql, params, transpileErr = tr.TranspileParameterizedCondition(tc.logic)
					}
					if !IsErrorCode(transpileErr, ErrUnreferencedPlaceholder) {
						t.Fatalf("parameterized transpilation error = %v, want %s (SQL %q params %#v)",
							transpileErr, ErrUnreferencedPlaceholder, sql, params)
					}
					if !strings.Contains(transpileErr.Error(), "custom operator may have dropped an argument") {
						t.Fatalf("error missing custom-operator safety message: %v", transpileErr)
					}
				})
			}
		})
	}
}

func TestCustomNullValueShortCircuitsValueModeAllDialects(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		logic       string
		wantSQL     string
		wantParamFn func(Dialect) string
		wantParams  []QueryParam
	}{
		{
			name:    "and returns custom null before unreachable field",
			logic:   `{"and":[{"nuller":[]},{"var":"missing"}]}`,
			wantSQL: "NULL",
			wantParamFn: func(Dialect) string {
				return "NULL"
			},
		},
		{
			name:    "or skips custom null before fallback",
			logic:   `{"or":[{"nuller":[]},"fallback"]}`,
			wantSQL: "'fallback'",
			wantParamFn: func(d Dialect) string {
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "fallback"}},
		},
		{
			name:    "if skips then branch for custom null condition",
			logic:   `{"if":[{"nuller":[]},{"var":"missing"},"fallback"]}`,
			wantSQL: "'fallback'",
			wantParamFn: func(d Dialect) string {
				return testStringPlaceholder(d, 1)
			},
			wantParams: []QueryParam{{Name: "p1", Value: "fallback"}},
		},
		{
			name:    "cat stringifies short-circuited custom null logical",
			logic:   `{"cat":[{"and":[{"nuller":[]},{"var":"missing"}]}]}`,
			wantSQL: "CONCAT('')",
			wantParamFn: func(Dialect) string {
				return "CONCAT('')"
			},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
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

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					gotSQL, err := tr.TranspileValue(tc.logic)
					if err != nil {
						t.Fatalf("TranspileValue() error = %v", err)
					}
					if gotSQL != tc.wantSQL {
						t.Fatalf("TranspileValue() = %q, want %q", gotSQL, tc.wantSQL)
					}

					gotParamSQL, gotParams, err := tr.TranspileParameterizedValue(tc.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedValue() error = %v", err)
					}
					if want := tc.wantParamFn(d); gotParamSQL != want {
						t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParamSQL, want)
					}
					if len(gotParams) != len(tc.wantParams) {
						t.Fatalf("params = %#v, want %#v", gotParams, tc.wantParams)
					}
					for i := range gotParams {
						if gotParams[i] != tc.wantParams[i] {
							t.Fatalf("params = %#v, want %#v", gotParams, tc.wantParams)
						}
					}
				})
			}
		})
	}
}

func TestParameterizedCustomNullValueShortCircuitPreservesDroppedParamDetection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		logic string
	}{
		{
			name:  "and custom null hides unreachable field but preserves dropped arg",
			logic: `{"and":[{"nuller":["x"]},{"var":"missing"}]}`,
		},
		{
			name:  "or custom null hides unreachable field but preserves dropped arg",
			logic: `{"or":[{"nuller":["x"]},"fallback"]}`,
		},
		{
			name:  "if custom null condition hides then field but preserves dropped arg",
			logic: `{"if":[{"nuller":["x"]},{"var":"missing"},"fallback"]}`,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
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

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()

					sql, params, err := tr.TranspileParameterizedValue(tc.logic)
					if !IsErrorCode(err, ErrUnreferencedPlaceholder) {
						t.Fatalf("TranspileParameterizedValue() SQL = %q params = %#v error = %v, want %s",
							sql, params, err, ErrUnreferencedPlaceholder)
					}
					if !strings.Contains(err.Error(), "custom operator may have dropped an argument") {
						t.Fatalf("error missing custom-operator safety message: %v", err)
					}
				})
			}
		})
	}
}

func TestParameterizedCustomDroppedParamsInsideArrayLiteralsAreDetected(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name                         string
		logic                        string
		valueMode                    bool
		skipArrayLiteralElemDialects bool
	}{
		{
			name:      "value logical array literal truthiness",
			logic:     `{"and":[[{"always":["secret"]}],"fallback"]}`,
			valueMode: true,
		},
		{
			name:  "condition double bang array literal truthiness",
			logic: `{"!!":[[{"always":["secret"]}]]}`,
		},
		{
			name:  "condition not array literal truthiness",
			logic: `{"!":[[{"always":["secret"]}]]}`,
		},
		{
			name:      "value if array literal condition",
			logic:     `{"if":[[{"always":["secret"]}],"yes","no"]}`,
			valueMode: true,
		},
		{
			name:      "value logical array literal with cat-preserved custom value",
			logic:     `{"and":[[{"cat":[{"dropstr":["secret"]}]}],"fallback"]}`,
			valueMode: true,
		},
		{
			name:      "value logical array literal with numeric-preserved custom value",
			logic:     `{"and":[[{"+":[{"dropnum":["secret"]},1]}],"fallback"]}`,
			valueMode: true,
		},
		{
			name:      "value logical array literal with substr-preserved custom value",
			logic:     `{"and":[[{"substr":["abcdef",{"dropnum":["secret"]},2]}],"fallback"]}`,
			valueMode: true,
		},
		{
			name:                         "value logical array literal with map-preserved custom value",
			logic:                        `{"and":[[{"map":[[1],{"dropstr":["secret"]}]}],"fallback"]}`,
			valueMode:                    true,
			skipArrayLiteralElemDialects: true,
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
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
			if regErr := tr.RegisterOperatorFunc("dropstr", func(_ string, _ []OperatorArg) (OperatorResult, error) {
				return ValueSQL("'safe'", ExpressionTypeString), nil
			}); regErr != nil {
				t.Fatalf("RegisterOperatorFunc(dropstr) error: %v", regErr)
			}
			if regErr := tr.RegisterOperatorFunc("dropnum", func(_ string, _ []OperatorArg) (OperatorResult, error) {
				return ValueSQL("1", ExpressionTypeNumber), nil
			}); regErr != nil {
				t.Fatalf("RegisterOperatorFunc(dropnum) error: %v", regErr)
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					if tc.skipArrayLiteralElemDialects && dialectRejectsArrayLiteralElements(d) {
						t.Skipf("%s rejects array literals containing array-valued elements before placeholder validation", d)
					}

					var sql string
					var params []QueryParam
					var transpileErr error
					if tc.valueMode {
						sql, params, transpileErr = tr.TranspileParameterizedValue(tc.logic)
					} else {
						sql, params, transpileErr = tr.TranspileParameterizedCondition(tc.logic)
					}
					if !IsErrorCode(transpileErr, ErrUnreferencedPlaceholder) {
						t.Fatalf("parameterized transpilation error = %v, want %s (SQL %q params %#v)",
							transpileErr, ErrUnreferencedPlaceholder, sql, params)
					}
					if !strings.Contains(transpileErr.Error(), "custom operator may have dropped an argument") {
						t.Fatalf("error missing custom-operator safety message: %v", transpileErr)
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
