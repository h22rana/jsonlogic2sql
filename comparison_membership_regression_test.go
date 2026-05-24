package jsonlogic2sql

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestArrayLiteralExpressionMembershipUsesNullSafeEqualityAllDialectsAndModes(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "a", Type: FieldTypeString},
		{Name: "b", Type: FieldTypeString},
	})
	conditionLogic := `{"in":[{"var":"a"},[{"var":"b"}]]}`
	valueLogic := fmt.Sprintf(`{"if":[%s,"hit","miss"]}`, conditionLogic)
	wantCondition := testNullSafeArrayMemberEqualitySQL("b", "a")
	wantValue := fmt.Sprintf("CASE WHEN %s THEN 'hit' ELSE 'miss' END", wantCondition)

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{Dialect: d, Schema: schema})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			gotCondition, err := tr.TranspileCondition(conditionLogic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			if gotCondition != wantCondition {
				t.Fatalf("TranspileCondition() = %q, want %q", gotCondition, wantCondition)
			}

			gotParameterizedCondition, conditionParams, err := tr.TranspileParameterizedCondition(conditionLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			if gotParameterizedCondition != wantCondition {
				t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParameterizedCondition, wantCondition)
			}
			if len(conditionParams) != 0 {
				t.Fatalf("TranspileParameterizedCondition() params = %#v, want none", conditionParams)
			}

			gotValue, err := tr.TranspileValue(valueLogic)
			if err != nil {
				t.Fatalf("TranspileValue() error = %v", err)
			}
			if gotValue != wantValue {
				t.Fatalf("TranspileValue() = %q, want %q", gotValue, wantValue)
			}

			gotParameterizedValue, valueParams, err := tr.TranspileParameterizedValue(valueLogic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error = %v", err)
			}
			wantParameterizedValue := fmt.Sprintf(
				"CASE WHEN %s THEN %s ELSE %s END",
				wantCondition,
				testStringPlaceholder(d, 1),
				testStringPlaceholder(d, 2),
			)
			if gotParameterizedValue != wantParameterizedValue {
				t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParameterizedValue, wantParameterizedValue)
			}
			wantValueParams := []QueryParam{
				{Name: "p1", Value: "hit"},
				{Name: "p2", Value: "miss"},
			}
			if !reflect.DeepEqual(valueParams, wantValueParams) {
				t.Fatalf("TranspileParameterizedValue() params = %#v, want %#v", valueParams, wantValueParams)
			}
		})
	}
}

func TestArrayLiteralMixedExpressionMembershipKeepsLiteralINFastPathAllDialects(t *testing.T) {
	t.Parallel()

	schema := mustNewSchema([]FieldSchema{
		{Name: "a", Type: FieldTypeString},
		{Name: "b", Type: FieldTypeString},
	})
	logic := `{"in":[{"var":"a"},[{"var":"b"},"x"]]}`
	wantExpressionPredicate := testNullSafeArrayMemberEqualitySQL("b", "a")

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			t.Parallel()

			tr, err := NewTranspilerWithConfig(&TranspilerConfig{Dialect: d, Schema: schema})
			if err != nil {
				t.Fatalf("NewTranspilerWithConfig() error = %v", err)
			}

			gotInline, err := tr.TranspileCondition(logic)
			if err != nil {
				t.Fatalf("TranspileCondition() error = %v", err)
			}
			wantInline := fmt.Sprintf("(%s OR a IN ('x'))", wantExpressionPredicate)
			if gotInline != wantInline {
				t.Fatalf("TranspileCondition() = %q, want %q", gotInline, wantInline)
			}

			gotParameterized, gotParams, err := tr.TranspileParameterizedCondition(logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedCondition() error = %v", err)
			}
			wantParameterized := fmt.Sprintf("(%s OR a IN (%s))", wantExpressionPredicate, testStringPlaceholder(d, 1))
			if gotParameterized != wantParameterized {
				t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParameterized, wantParameterized)
			}
			if !reflect.DeepEqual(gotParams, []QueryParam{{Name: "p1", Value: "x"}}) {
				t.Fatalf("TranspileParameterizedCondition() params = %#v, want p1=x", gotParams)
			}
			if strings.Contains(gotParameterized, fmt.Sprintf("a IN (b, %s)", testStringPlaceholder(d, 1))) {
				t.Fatalf("TranspileParameterizedCondition() used plain IN for expression member: %q", gotParameterized)
			}
		})
	}
}
