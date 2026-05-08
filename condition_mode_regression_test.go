package jsonlogic2sql

import (
	"fmt"
	"reflect"
	"testing"
)

func TestTranspileCondition_PredicateIfAcceptsBooleanConstants(t *testing.T) {
	tests := []struct {
		name      string
		logic     string
		want      string
		wantParam func(Dialect) string
		params    []QueryParam
	}{
		{
			name:  "boolean branch arms",
			logic: `{"if":[{">":[{"var":"age"},18]},true,false]}`,
			want:  "CASE WHEN age > 18 THEN TRUE ELSE FALSE END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN age > %s THEN TRUE ELSE FALSE END", testPlaceholder(d, 1))
			},
			params: []QueryParam{{Name: "p1", Value: float64(18)}},
		},
		{
			name:  "boolean condition",
			logic: `{"if":[true,{">":[{"var":"age"},18]},false]}`,
			want:  "CASE WHEN TRUE THEN age > 18 ELSE FALSE END",
			wantParam: func(d Dialect) string {
				return fmt.Sprintf("CASE WHEN TRUE THEN age > %s ELSE FALSE END", testPlaceholder(d, 1))
			},
			params: []QueryParam{{Name: "p1", Value: float64(18)}},
		},
	}

	for _, d := range allDialects() {
		t.Run(d.String(), func(t *testing.T) {
			tr, err := NewTranspiler(d)
			if err != nil {
				t.Fatalf("NewTranspiler() error = %v", err)
			}

			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					got, err := tr.TranspileCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileCondition() error = %v", err)
					}
					if got != tt.want {
						t.Fatalf("TranspileCondition() = %q, want %q", got, tt.want)
					}

					gotParam, gotParams, err := tr.TranspileParameterizedCondition(tt.logic)
					if err != nil {
						t.Fatalf("TranspileParameterizedCondition() error = %v", err)
					}
					if want := tt.wantParam(d); gotParam != want {
						t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
					}
					if !reflect.DeepEqual(gotParams, tt.params) {
						t.Fatalf("params = %#v, want %#v", gotParams, tt.params)
					}
				})
			}
		})
	}
}
