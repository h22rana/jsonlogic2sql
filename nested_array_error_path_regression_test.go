package jsonlogic2sql

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func decodeLogicMapForPathTest(t *testing.T, logic string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(logic), &m); err != nil {
		t.Fatalf("json.Unmarshal(map) failed: %v", err)
	}
	return m
}

func assertNestedPathNotTruncated(t *testing.T, err error, parentOp string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "oops") {
		t.Fatalf("expected error to mention custom operator, got: %v", err)
	}
	if strings.Contains(msg, "$[1].oops") {
		t.Fatalf("path is truncated (missing parent context): %v", err)
	}
	if !strings.Contains(msg, "$."+parentOp) {
		t.Fatalf("expected parent operator path in error, got: %v", err)
	}
	if !strings.Contains(msg, ".map") {
		t.Fatalf("expected nested map segment in error path, got: %v", err)
	}
}

func TestNestedArrayCustomOperatorErrorPath_Preserved_InlineAndParam(t *testing.T) {
	t.Parallel()

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	cases := []struct {
		name     string
		parentOp string
		logic    string
	}{
		{
			name:     "array under comparison",
			parentOp: "==",
			logic:    `{"==":[{"map":[{"var":"nums"},{"oops":[{"var":""}]}]},1]}`,
		},
		{
			name:     "array under logical",
			parentOp: "and",
			logic:    `{"and":[true,{"map":[{"var":"nums"},{"oops":[{"var":""}]}]}]}`,
		},
	}

	for _, d := range dialects {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspiler(d, defaultTestSchema())
			if err != nil {
				t.Fatalf("NewTranspiler() error: %v", err)
			}
			tr.RegisterOperatorFunc("oops", func(_ string, _ []OperatorArg) (OperatorResult, error) {
				return OperatorResult{}, fmt.Errorf("boom")
			})

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					logicMap := decodeLogicMapForPathTest(t, tc.logic)

					var err error
					if tc.parentOp == "and" {
						_, err = tr.TranspileValue(tc.logic)
					} else {
						_, err = tr.TranspileCondition(tc.logic)
					}
					assertNestedPathNotTruncated(t, err, tc.parentOp)

					if tc.parentOp == "and" {
						_, _, err = tr.TranspileParameterizedValue(tc.logic)
					} else {
						_, _, err = tr.TranspileParameterizedCondition(tc.logic)
					}
					assertNestedPathNotTruncated(t, err, tc.parentOp)

					if tc.parentOp == "and" {
						_, err = tr.TranspileValueFromMap(logicMap)
					} else {
						_, err = tr.TranspileConditionFromMap(logicMap)
					}
					assertNestedPathNotTruncated(t, err, tc.parentOp)

					if tc.parentOp == "and" {
						_, _, err = tr.TranspileParameterizedValueFromMap(logicMap)
					} else {
						_, _, err = tr.TranspileParameterizedConditionFromMap(logicMap)
					}
					assertNestedPathNotTruncated(t, err, tc.parentOp)
				})
			}
		})
	}
}
