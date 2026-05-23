package jsonlogic2sql

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func matrixSchema() *Schema {
	return mustNewSchema([]FieldSchema{
		{
			Name: "bag.records",
			Type: FieldTypeArray,
			ElementFields: []FieldSchema{
				{Name: "base", Type: FieldTypeNumber},
				{Name: "values", Type: FieldTypeArray, ElementType: FieldTypeNumber},
			},
		},
		{Name: "bag.numbers", Type: FieldTypeArray, ElementType: FieldTypeNumber},
		{Name: "bag.moreNumbers", Type: FieldTypeArray, ElementType: FieldTypeNumber},
		{Name: "bag.words", Type: FieldTypeArray, ElementType: FieldTypeString},
		{Name: "bag.flags", Type: FieldTypeArray, ElementType: FieldTypeBoolean},
		{Name: "metrics.amount", Type: FieldTypeNumber},
		{Name: "profile.name", Type: FieldTypeString},
	})
}

type apiOutput struct {
	inlineSQL  string
	condSQL    string
	paramSQL   string
	paramCond  string
	params     []QueryParam
	condParams []QueryParam
}

func decodeLogicMap(t *testing.T, logic string) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(logic), &m); err != nil {
		t.Fatalf("json.Unmarshal(map) failed: %v", err)
	}
	return m
}

func decodeLogicAny(t *testing.T, logic string) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(logic), &v); err != nil {
		t.Fatalf("json.Unmarshal(interface) failed: %v", err)
	}
	return v
}

func runAllAPIVariants(t *testing.T, tr *Transpiler, logic string) apiOutput {
	t.Helper()

	inlineSQL, err := tr.TranspileCondition(logic)
	valueMode := IsErrorCode(err, ErrInvalidExpressionContext)
	if valueMode {
		inlineSQL, err = tr.TranspileValue(logic)
	}
	if err != nil {
		t.Fatalf("TranspileCondition() error: %v", err)
	}
	condSQL := inlineSQL
	if !valueMode {
		condSQL, err = tr.TranspileCondition(logic)
		if err != nil {
			t.Fatalf("TranspileCondition() error: %v", err)
		}
	}
	var paramSQL string
	var params []QueryParam
	if valueMode {
		paramSQL, params, err = tr.TranspileParameterizedValue(logic)
	} else {
		paramSQL, params, err = tr.TranspileParameterizedCondition(logic)
	}
	if err != nil {
		t.Fatalf("TranspileParameterizedCondition() error: %v", err)
	}
	paramCond := paramSQL
	condParams := params
	if !valueMode {
		paramCond, condParams, err = tr.TranspileParameterizedCondition(logic)
		if err != nil {
			t.Fatalf("TranspileParameterizedCondition() error: %v", err)
		}
	}

	logicMap := decodeLogicMap(t, logic)
	logicAny := decodeLogicAny(t, logic)

	var inlineFromMap, inlineFromAny string
	if valueMode {
		inlineFromMap, err = tr.TranspileValueFromMap(logicMap)
	} else {
		inlineFromMap, err = tr.TranspileConditionFromMap(logicMap)
	}
	if err != nil {
		t.Fatalf("TranspileFromMap variant error: %v", err)
	}
	if valueMode {
		inlineFromAny, err = tr.TranspileValueFromInterface(logicAny)
	} else {
		inlineFromAny, err = tr.TranspileConditionFromInterface(logicAny)
	}
	if err != nil {
		t.Fatalf("TranspileFromInterface variant error: %v", err)
	}
	condFromMap := inlineFromMap
	condFromAny := inlineFromAny

	var paramFromMap, paramFromAny string
	var mapParams, anyParams []QueryParam
	if valueMode {
		paramFromMap, mapParams, err = tr.TranspileParameterizedValueFromMap(logicMap)
	} else {
		paramFromMap, mapParams, err = tr.TranspileParameterizedConditionFromMap(logicMap)
	}
	if err != nil {
		t.Fatalf("TranspileParameterizedFromMap variant error: %v", err)
	}
	if valueMode {
		paramFromAny, anyParams, err = tr.TranspileParameterizedValueFromInterface(logicAny)
	} else {
		paramFromAny, anyParams, err = tr.TranspileParameterizedConditionFromInterface(logicAny)
	}
	if err != nil {
		t.Fatalf("TranspileParameterizedFromInterface variant error: %v", err)
	}
	paramCondFromMap := paramFromMap
	paramCondFromAny := paramFromAny
	mapCondParams := mapParams
	anyCondParams := anyParams

	if inlineFromMap != inlineSQL || inlineFromAny != inlineSQL {
		t.Fatalf("inline API mismatch: direct=%q fromMap=%q fromAny=%q", inlineSQL, inlineFromMap, inlineFromAny)
	}
	if condFromMap != condSQL || condFromAny != condSQL {
		t.Fatalf("condition API mismatch: direct=%q fromMap=%q fromAny=%q", condSQL, condFromMap, condFromAny)
	}
	if paramFromMap != paramSQL || paramFromAny != paramSQL {
		t.Fatalf("parameterized API SQL mismatch: direct=%q fromMap=%q fromAny=%q", paramSQL, paramFromMap, paramFromAny)
	}
	if paramCondFromMap != paramCond || paramCondFromAny != paramCond {
		t.Fatalf("parameterized condition SQL mismatch: direct=%q fromMap=%q fromAny=%q", paramCond, paramCondFromMap, paramCondFromAny)
	}
	if len(mapParams) != len(params) || len(anyParams) != len(params) {
		t.Fatalf("parameterized API param count mismatch: direct=%d fromMap=%d fromAny=%d", len(params), len(mapParams), len(anyParams))
	}
	if len(mapCondParams) != len(condParams) || len(anyCondParams) != len(condParams) {
		t.Fatalf("parameterized condition API param count mismatch: direct=%d fromMap=%d fromAny=%d", len(condParams), len(mapCondParams), len(anyCondParams))
	}
	if condSQL != inlineSQL {
		t.Fatalf("condition mismatch: inline=%q cond=%q", inlineSQL, condSQL)
	}
	if paramCond != paramSQL {
		t.Fatalf("parameterized condition mismatch: inline=%q cond=%q", paramSQL, paramCond)
	}

	return apiOutput{
		inlineSQL:  inlineSQL,
		condSQL:    condSQL,
		paramSQL:   paramSQL,
		paramCond:  paramCond,
		params:     params,
		condParams: condParams,
	}
}

func assertAllAPIVariantsErrorContains(t *testing.T, tr *Transpiler, logic, want string) {
	t.Helper()

	errs := make([]error, 0, 4)
	if sql, err := tr.TranspileCondition(logic); err == nil {
		t.Fatalf("TranspileCondition() succeeded with %q, want error containing %q", sql, want)
	} else {
		errs = append(errs, err)
	}
	if sql, err := tr.TranspileValue(logic); err == nil {
		t.Fatalf("TranspileValue() succeeded with %q, want error containing %q", sql, want)
	} else {
		errs = append(errs, err)
	}
	if sql, params, err := tr.TranspileParameterizedCondition(logic); err == nil {
		t.Fatalf("TranspileParameterizedCondition() succeeded with %q params=%v, want error containing %q", sql, params, want)
	} else {
		errs = append(errs, err)
	}
	if sql, params, err := tr.TranspileParameterizedValue(logic); err == nil {
		t.Fatalf("TranspileParameterizedValue() succeeded with %q params=%v, want error containing %q", sql, params, want)
	} else {
		errs = append(errs, err)
	}

	for _, err := range errs {
		if strings.Contains(err.Error(), want) {
			return
		}
	}
	t.Fatalf("no API variant error contained %q; errors=%v", want, errs)
}

func assertPlaceholderStyle(t *testing.T, d Dialect, sql string, paramCount int) {
	t.Helper()
	if paramCount == 0 {
		return
	}
	switch d {
	case DialectPostgreSQL, DialectDuckDB:
		if !strings.Contains(sql, "$1") {
			t.Fatalf("expected $ placeholders for %s, got: %s", d, sql)
		}
	case DialectClickHouse:
		if !strings.Contains(sql, "{p1:") {
			t.Fatalf("expected ClickHouse typed placeholders for %s, got: %s", d, sql)
		}
	default:
		if !strings.Contains(sql, "@p1") {
			t.Fatalf("expected @p placeholders for %s, got: %s", d, sql)
		}
	}
}

func assertContains(t *testing.T, sql, fragment string) {
	t.Helper()
	if !strings.Contains(sql, fragment) {
		t.Fatalf("expected SQL to contain %q, got: %s", fragment, sql)
	}
}

func assertNotContains(t *testing.T, sql, fragment string) {
	t.Helper()
	if strings.Contains(sql, fragment) {
		t.Fatalf("expected SQL not to contain %q, got: %s", fragment, sql)
	}
}

func TestLiteralScalarArrayLambdaTruthiness_AllDialectsAndModes(t *testing.T) {
	t.Parallel()

	literalArray := func(d Dialect, elems ...string) string {
		if d == DialectPostgreSQL {
			return fmt.Sprintf("ARRAY[%s]", strings.Join(elems, ", "))
		}
		return fmt.Sprintf("[%s]", strings.Join(elems, ", "))
	}
	unnest := func(d Dialect, array string) string {
		if d == DialectDuckDB {
			return fmt.Sprintf("UNNEST(%s) AS elem(elem)", array)
		}
		return fmt.Sprintf("UNNEST(%s) AS elem", array)
	}
	arrayLength := func(d Dialect, array string) string {
		switch d {
		case DialectPostgreSQL:
			return fmt.Sprintf("CARDINALITY(%s)", array)
		case DialectDuckDB, DialectClickHouse:
			return fmt.Sprintf("length(%s)", array)
		default:
			return fmt.Sprintf("ARRAY_LENGTH(%s)", array)
		}
	}
	paramArray := func(d Dialect, count int, placeholder func(Dialect, int) string) string {
		elems := make([]string, count)
		for i := range elems {
			elems[i] = placeholder(d, i+1)
		}
		return literalArray(d, elems...)
	}
	valuePredicate := func(predicate string) string {
		switch {
		case strings.HasPrefix(predicate, "(") && strings.HasSuffix(predicate, ")"):
			return strings.TrimSuffix(strings.TrimPrefix(predicate, "("), ")")
		default:
			return predicate
		}
	}

	type lambdaCase struct {
		name           string
		logic          string
		valueOnly      bool
		wantInline     func(Dialect) string
		wantParam      func(Dialect) string
		wantParamCount int
	}

	cases := []lambdaCase{
		{
			name:           "filter numeric current element truthiness",
			logic:          `{"filter":[[1,2,0],{"var":""}]}`,
			valueOnly:      true,
			wantParamCount: 3,
			wantInline: func(d Dialect) string {
				array := literalArray(d, "1", "2", "0")
				condition := "(elem IS NOT NULL AND elem != 0)"
				if d == DialectClickHouse {
					return fmt.Sprintf("arrayFilter(elem -> %s, %s)", condition, array)
				}
				return fmt.Sprintf("ARRAY(SELECT elem FROM %s WHERE %s)", unnest(d, array), condition)
			},
			wantParam: func(d Dialect) string {
				array := paramArray(d, 3, testPlaceholder)
				condition := "(elem IS NOT NULL AND elem != 0)"
				if d == DialectClickHouse {
					return fmt.Sprintf("arrayFilter(elem -> %s, %s)", condition, array)
				}
				return fmt.Sprintf("ARRAY(SELECT elem FROM %s WHERE %s)", unnest(d, array), condition)
			},
		},
		{
			name:           "some boolean current element truthiness",
			logic:          `{"some":[[true,false],{"var":""}]}`,
			wantParamCount: 0,
			wantInline: func(d Dialect) string {
				array := literalArray(d, "TRUE", "FALSE")
				if d == DialectClickHouse {
					return fmt.Sprintf("arrayExists(elem -> elem IS TRUE, %s)", array)
				}
				return fmt.Sprintf("EXISTS (SELECT 1 FROM %s WHERE elem IS TRUE)", unnest(d, array))
			},
			wantParam: func(d Dialect) string {
				array := literalArray(d, "TRUE", "FALSE")
				if d == DialectClickHouse {
					return fmt.Sprintf("arrayExists(elem -> elem IS TRUE, %s)", array)
				}
				return fmt.Sprintf("EXISTS (SELECT 1 FROM %s WHERE elem IS TRUE)", unnest(d, array))
			},
		},
		{
			name:           "all string current element truthiness",
			logic:          `{"all":[["x",""],{"var":""}]}`,
			wantParamCount: 2,
			wantInline: func(d Dialect) string {
				array := literalArray(d, "'x'", "''")
				condition := "(elem IS NOT NULL AND elem != '')"
				if d == DialectClickHouse {
					return fmt.Sprintf("(%s > 0 AND arrayAll(elem -> %s, %s))", arrayLength(d, array), condition, array)
				}
				return fmt.Sprintf("(%s > 0 AND NOT EXISTS (SELECT 1 FROM %s WHERE NOT (%s)))", arrayLength(d, array), unnest(d, array), condition)
			},
			wantParam: func(d Dialect) string {
				array := paramArray(d, 2, testStringPlaceholder)
				condition := "(elem IS NOT NULL AND elem != '')"
				if d == DialectClickHouse {
					return fmt.Sprintf("(%s > 0 AND arrayAll(elem -> %s, %s))", arrayLength(d, array), condition, array)
				}
				return fmt.Sprintf("(%s > 0 AND NOT EXISTS (SELECT 1 FROM %s WHERE NOT (%s)))", arrayLength(d, array), unnest(d, array), condition)
			},
		},
		{
			name:           "none null current element truthiness",
			logic:          `{"none":[[null],{"var":""}]}`,
			wantParamCount: 0,
			wantInline: func(d Dialect) string {
				array := literalArray(d, "NULL")
				if d == DialectClickHouse {
					return fmt.Sprintf("NOT arrayExists(elem -> FALSE, %s)", array)
				}
				return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM %s WHERE FALSE)", unnest(d, array))
			},
			wantParam: func(d Dialect) string {
				array := literalArray(d, "NULL")
				if d == DialectClickHouse {
					return fmt.Sprintf("NOT arrayExists(elem -> FALSE, %s)", array)
				}
				return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM %s WHERE FALSE)", unnest(d, array))
			},
		},
		{
			name:           "map value logical numeric current element truthiness",
			logic:          `{"map":[[1,0],{"or":[{"var":""},5]}]}`,
			valueOnly:      true,
			wantParamCount: 3,
			wantInline: func(d Dialect) string {
				array := literalArray(d, "1", "0")
				transformation := "CASE WHEN (elem IS NOT NULL AND elem != 0) THEN elem ELSE 5 END"
				if d == DialectClickHouse {
					return fmt.Sprintf("arrayMap(elem -> %s, %s)", transformation, array)
				}
				return fmt.Sprintf("ARRAY(SELECT %s FROM %s)", transformation, unnest(d, array))
			},
			wantParam: func(d Dialect) string {
				array := literalArray(d, testPlaceholder(d, 1), testPlaceholder(d, 2))
				transformation := fmt.Sprintf("CASE WHEN (elem IS NOT NULL AND elem != 0) THEN elem ELSE %s END", testPlaceholder(d, 3))
				if d == DialectClickHouse {
					return fmt.Sprintf("arrayMap(elem -> %s, %s)", transformation, array)
				}
				return fmt.Sprintf("ARRAY(SELECT %s FROM %s)", transformation, unnest(d, array))
			},
		},
	}

	schemas := []struct {
		name   string
		schema *Schema
	}{
		{name: "empty-schema", schema: emptyTestSchema()},
		{name: "default-schema", schema: defaultTestSchema()},
	}

	for _, schemaCase := range schemas {
		t.Run(schemaCase.name, func(t *testing.T) {
			t.Parallel()
			for _, d := range allDialects() {
				t.Run(d.String(), func(t *testing.T) {
					t.Parallel()
					tr, err := NewTranspiler(d, schemaCase.schema)
					if err != nil {
						t.Fatalf("NewTranspiler() error = %v", err)
					}

					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							t.Parallel()

							if tc.valueOnly {
								got, err := tr.TranspileValue(tc.logic)
								if err != nil {
									t.Fatalf("TranspileValue() error = %v", err)
								}
								if want := tc.wantInline(d); got != want {
									t.Fatalf("TranspileValue() = %q, want %q", got, want)
								}

								gotParam, gotParams, err := tr.TranspileParameterizedValue(tc.logic)
								if err != nil {
									t.Fatalf("TranspileParameterizedValue() error = %v", err)
								}
								if want := tc.wantParam(d); gotParam != want {
									t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotParam, want)
								}
								if len(gotParams) != tc.wantParamCount {
									t.Fatalf("TranspileParameterizedValue() params = %#v, want %d params", gotParams, tc.wantParamCount)
								}
								return
							}

							got, err := tr.TranspileCondition(tc.logic)
							if err != nil {
								t.Fatalf("TranspileCondition() error = %v", err)
							}
							if want := tc.wantInline(d); got != want {
								t.Fatalf("TranspileCondition() = %q, want %q", got, want)
							}

							gotParam, gotParams, err := tr.TranspileParameterizedCondition(tc.logic)
							if err != nil {
								t.Fatalf("TranspileParameterizedCondition() error = %v", err)
							}
							if want := tc.wantParam(d); gotParam != want {
								t.Fatalf("TranspileParameterizedCondition() = %q, want %q", gotParam, want)
							}
							if len(gotParams) != tc.wantParamCount {
								t.Fatalf("TranspileParameterizedCondition() params = %#v, want %d params", gotParams, tc.wantParamCount)
							}

							gotValue, err := tr.TranspileValue(tc.logic)
							if err != nil {
								t.Fatalf("TranspileValue() error = %v", err)
							}
							if want := "CASE WHEN " + valuePredicate(tc.wantInline(d)) + " THEN TRUE ELSE FALSE END"; gotValue != want {
								t.Fatalf("TranspileValue() = %q, want %q", gotValue, want)
							}

							gotValueParam, gotValueParams, err := tr.TranspileParameterizedValue(tc.logic)
							if err != nil {
								t.Fatalf("TranspileParameterizedValue() error = %v", err)
							}
							if want := "CASE WHEN " + valuePredicate(tc.wantParam(d)) + " THEN TRUE ELSE FALSE END"; gotValueParam != want {
								t.Fatalf("TranspileParameterizedValue() = %q, want %q", gotValueParam, want)
							}
							if len(gotValueParams) != tc.wantParamCount {
								t.Fatalf("TranspileParameterizedValue() params = %#v, want %d params", gotValueParams, tc.wantParamCount)
							}
						})
					}
				})
			}
		})
	}
}

func TestArrayEdgeMatrix_AllDialects_SchemaAndSchemaRequired(t *testing.T) {
	type matrixCase struct {
		name                         string
		logic                        string
		wantParam                    int
		rejectGoogleNestedArrayValue bool
		validate                     func(t *testing.T, d Dialect, out apiOutput)
	}

	cases := []matrixCase{
		{
			name:      "small map",
			logic:     `{"map":[{"var":"bag.numbers"},{"*":[{"var":""},2]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayMap(elem -> (elem * 2), bag.numbers)")
				} else {
					assertContains(t, inline, "UNNEST(bag.numbers) AS elem")
					assertContains(t, inline, "(elem * 2)")
				}
			},
		},
		{
			name:      "all length function by dialect",
			logic:     `{"all":[{"var":"bag.numbers"},{">=":[{"var":""},0]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				switch d {
				case DialectBigQuery, DialectSpanner:
					assertContains(t, inline, "ARRAY_LENGTH(bag.numbers)")
				case DialectPostgreSQL:
					assertContains(t, inline, "CARDINALITY(bag.numbers)")
				case DialectDuckDB, DialectClickHouse:
					assertContains(t, inline, "length(bag.numbers)")
				}
			},
		},
		{
			name:      "merge dialect behavior",
			logic:     `{"merge":[{"var":"bag.numbers"},{"var":"bag.moreNumbers"}]}`,
			wantParam: 0,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				switch d {
				case DialectPostgreSQL:
					assertContains(t, inline, "bag.numbers || bag.moreNumbers")
				case DialectClickHouse:
					assertContains(t, inline, "arrayConcat(bag.numbers, bag.moreNumbers)")
				default:
					assertContains(t, inline, "ARRAY_CONCAT(bag.numbers, bag.moreNumbers)")
				}
			},
		},
		{
			name:      "reduce array-form current default",
			logic:     `{"reduce":[{"var":"bag.numbers"},{"+":[{"var":"accumulator"},{"var":["current",0]}]},1]}`,
			wantParam: 2,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "COALESCE(x, 0)")
				} else {
					assertContains(t, inline, "COALESCE(elem, 0)")
				}
			},
		},
		{
			name:                         "nested map with outer scoped source",
			logic:                        `{"map":[{"var":"bag.records"},{"map":[{"var":"values"},{"var":""}]}]}`,
			wantParam:                    0,
			rejectGoogleNestedArrayValue: true,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayMap(elem1 -> elem1, elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "SELECT elem1")
					assertNotContains(t, inline, "UNNEST(elem.values) AS elem)")
				}
			},
		},
		{
			name:                         "nested filter with outer scoped source",
			logic:                        `{"map":[{"var":"bag.records"},{"filter":[{"var":"values"},{">=":[{"var":""},0]}]}]}`,
			wantParam:                    1,
			rejectGoogleNestedArrayValue: true,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayFilter(elem1 -> elem1 >= 0, elem.values)")
					assertNotContains(t, inline, "arrayFilter(elem -> elem >= 0, elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "elem1 >= 0")
					assertNotContains(t, inline, "UNNEST(elem.values) AS elem WHERE elem >= 0")
				}
			},
		},
		{
			name:      "nested all with outer scoped source",
			logic:     `{"map":[{"var":"bag.records"},{"all":[{"var":"values"},{">=":[{"var":""},0]}]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayAll(elem1 -> elem1 >= 0, elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "elem1 >= 0")
				}
			},
		},
		{
			name:      "nested some with outer scoped source",
			logic:     `{"map":[{"var":"bag.records"},{"some":[{"var":"values"},{">=":[{"var":""},0]}]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayExists(elem1 -> elem1 >= 0, elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "elem1 >= 0")
				}
			},
		},
		{
			name:      "nested none with outer scoped source",
			logic:     `{"map":[{"var":"bag.records"},{"none":[{"var":"values"},{"<":[{"var":""},0]}]}]}`,
			wantParam: 1,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayExists(elem1 -> elem1 < 0, elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "elem1 < 0")
				}
			},
		},
		{
			name:      "nested reduce with outer scoped source and initial",
			logic:     `{"map":[{"var":"bag.records"},{"reduce":[{"var":"values"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"base"}]}]}`,
			wantParam: 0,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayMap(elem -> elem.base + coalesce(arrayReduce('sum', elem.values), 0), bag.records)")
				} else {
					assertContains(t, inline, "UNNEST(bag.records) AS elem")
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "elem.base + COALESCE((SELECT SUM(elem1)")
				}
			},
		},
		{
			name:                         "very deep mixed nesting",
			logic:                        `{"and":[{"some":[{"map":[{"var":"bag.records"},{"filter":[{"var":"values"},{">=":[{"var":""},0]}]}]},{"all":[{"var":""},{">=":[{"var":""},0]}]}]},{">=":[{"var":"metrics.amount"},100]}]}`,
			wantParam:                    3,
			rejectGoogleNestedArrayValue: true,
			validate: func(t *testing.T, d Dialect, out apiOutput) {
				t.Helper()
				inline := out.inlineSQL
				assertContains(t, inline, "metrics.amount >= 100")
				if d == DialectClickHouse {
					assertContains(t, inline, "arrayFilter(elem1 -> elem1 >= 0, elem.values)")
				} else {
					assertContains(t, inline, "UNNEST(elem.values) AS elem1")
					assertContains(t, inline, "elem1 >= 0")
				}
			},
		},
	}

	modes := []struct {
		name   string
		schema *Schema
	}{
		{name: "schema-required", schema: matrixSchema()},
	}

	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			for _, d := range dialects {
				t.Run(d.String(), func(t *testing.T) {
					tr, err := NewTranspilerWithConfig(&TranspilerConfig{
						Dialect: d,
						Schema:  mode.schema,
					})
					if err != nil {
						t.Fatalf("NewTranspilerWithConfig() error: %v", err)
					}

					for _, c := range cases {
						t.Run(c.name, func(t *testing.T) {
							if c.rejectGoogleNestedArrayValue && testRejectsNestedArrayValues(d) {
								if d == DialectPostgreSQL {
									assertAllAPIVariantsErrorContains(t, tr, c.logic, "PostgreSQL")
								} else {
									assertAllAPIVariantsErrorContains(t, tr, c.logic, "does not support array literals whose elements are arrays")
								}
								return
							}
							out := runAllAPIVariants(t, tr, c.logic)
							if len(out.params) != c.wantParam {
								t.Fatalf("param count mismatch: got=%d want=%d sql=%s", len(out.params), c.wantParam, out.paramSQL)
							}
							if len(out.condParams) != c.wantParam {
								t.Fatalf("condition param count mismatch: got=%d want=%d sql=%s", len(out.condParams), c.wantParam, out.paramCond)
							}
							assertPlaceholderStyle(t, d, out.paramSQL, c.wantParam)
							if c.validate != nil {
								c.validate(t, d, out)
							}
						})
					}
				})
			}
		})
	}
}

func TestArrayEdgeMatrix_SchemaVsSchemaRequiredValidation(t *testing.T) {
	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}

	schemaTrByDialect := make(map[Dialect]*Transpiler, len(dialects))
	emptySchemaTrByDialect := make(map[Dialect]*Transpiler, len(dialects))

	for _, d := range dialects {
		withSchema, err := NewTranspilerWithConfig(&TranspilerConfig{
			Dialect: d,
			Schema:  matrixSchema(),
		})
		if err != nil {
			t.Fatalf("with schema transpiler init failed: %v", err)
		}
		emptySchemaTr, err := NewTranspilerWithConfig(&TranspilerConfig{
			Dialect: d,
			Schema:  emptyTestSchema(),
		})
		if err != nil {
			t.Fatalf("empty schema transpiler init failed: %v", err)
		}
		schemaTrByDialect[d] = withSchema
		emptySchemaTrByDialect[d] = emptySchemaTr
	}

	tests := []struct {
		name          string
		logic         string
		wantSchemaErr string
	}{
		{
			name:          "unknown field rejected with schema",
			logic:         `{"map":[{"var":"unknown.arr"},{"+":[{"var":""},1]}]}`,
			wantSchemaErr: "is not defined in schema",
		},
		{
			name:          "non-array field rejected with schema",
			logic:         `{"map":[{"var":"metrics.amount"},{"+":[{"var":""},1]}]}`,
			wantSchemaErr: "array operation on non-array field",
		},
	}

	for _, d := range dialects {
		t.Run(d.String(), func(t *testing.T) {
			for _, tc := range tests {
				t.Run(tc.name, func(t *testing.T) {
					_, err := schemaTrByDialect[d].TranspileValue(tc.logic)
					if err == nil || !strings.Contains(err.Error(), tc.wantSchemaErr) {
						t.Fatalf("expected schema error containing %q, got: %v", tc.wantSchemaErr, err)
					}

					_, err = emptySchemaTrByDialect[d].TranspileValue(tc.logic)
					if err == nil || !strings.Contains(err.Error(), "is not defined in schema") {
						t.Fatalf("expected empty-schema field validation error, got: %v", err)
					}
				})
			}
		})
	}
}

func TestArrayEdgeMatrix_PackageFunctionsSmoke(t *testing.T) {
	dialects := []Dialect{
		DialectBigQuery,
		DialectSpanner,
		DialectPostgreSQL,
		DialectDuckDB,
		DialectClickHouse,
	}
	logic := `{"map":[{"var":"bag.numbers"},{"*":[{"var":""},2]}]}`
	logicMap := decodeLogicMap(t, logic)
	logicAny := decodeLogicAny(t, logic)

	for _, d := range dialects {
		t.Run(d.String(), func(t *testing.T) {
			sql1, err := TranspileValue(d, matrixSchema(), logic)
			if err != nil {
				t.Fatalf("TranspileValue() error: %v", err)
			}
			sql2, err := TranspileValueFromMap(d, matrixSchema(), logicMap)
			if err != nil {
				t.Fatalf("TranspileValueFromMap() error: %v", err)
			}
			sql3, err := TranspileValueFromInterface(d, matrixSchema(), logicAny)
			if err != nil {
				t.Fatalf("TranspileValueFromInterface() error: %v", err)
			}
			cond, err := TranspileValue(d, matrixSchema(), logic)
			if err != nil {
				t.Fatalf("TranspileValue() error: %v", err)
			}
			if sql1 != sql2 || sql1 != sql3 {
				t.Fatalf("package transpile mismatch: direct=%q map=%q any=%q", sql1, sql2, sql3)
			}
			if sql1 != cond {
				t.Fatalf("package condition mismatch: sql=%q cond=%q", sql1, cond)
			}

			psql, params, err := TranspileParameterizedValue(d, matrixSchema(), logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error: %v", err)
			}
			if len(params) != 1 {
				t.Fatalf("expected 1 param, got %d", len(params))
			}
			pcond, cparams, err := TranspileParameterizedValue(d, matrixSchema(), logic)
			if err != nil {
				t.Fatalf("TranspileParameterizedValue() error: %v", err)
			}
			if psql != pcond {
				t.Fatalf("package parameterized condition mismatch: sql=%q cond=%q", psql, pcond)
			}
			if len(cparams) != len(params) {
				t.Fatalf("package parameterized param len mismatch: cond=%d sql=%d", len(cparams), len(params))
			}
			assertPlaceholderStyle(t, d, psql, 1)
		})
	}
}

func BenchmarkArrayEdgeMatrix_DeepNesting(b *testing.B) {
	logic := `{"and":[{"some":[{"var":"bag.records"},{"all":[{"var":"values"},{">=":[{"var":""},0]}]}]},{">=":[{"var":"metrics.amount"},100]}]}`
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  matrixSchema(),
	})
	if err != nil {
		b.Fatalf("failed to init transpiler: %v", err)
	}
	for i := 0; i < b.N; i++ {
		if _, err := tr.TranspileCondition(logic); err != nil {
			b.Fatalf("TranspileCondition() error: %v", err)
		}
	}
}

func TestArrayEdgeMatrix_ErrorMessagesAreStable(t *testing.T) {
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  matrixSchema(),
	})
	if err != nil {
		t.Fatalf("failed to init transpiler: %v", err)
	}
	_, err = tr.TranspileCondition(`{"reduce":[{"var":"bag.numbers"},{"var":"current"}]}`)
	if err == nil {
		t.Fatal("expected reduce arity error")
	}
	if !strings.Contains(err.Error(), "reduce operator requires at least 3 arguments") {
		t.Fatalf("unexpected reduce arity error: %v", err)
	}
}

func TestArrayEdgeMatrix_NoPanicOnComplexInputs(t *testing.T) {
	tr, err := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  matrixSchema(),
	})
	if err != nil {
		t.Fatalf("failed to init transpiler: %v", err)
	}

	inputs := []struct {
		logic        string
		wantValueErr bool
	}{
		{
			logic:        `{"map":[{"var":"bag.records"},{"map":[{"var":"values"},{"if":[{">":[{"var":""},10]},{"var":""},0]}]}]}`,
			wantValueErr: true,
		},
		{logic: `{"filter":[{"map":[{"var":"bag.records"},{"reduce":[{"var":"values"},{"+":[{"var":"accumulator"},{"var":"current"}]},{"var":"base"}]}]},{">":[{"var":""},0]}]}`},
		{logic: `{"all":[{"filter":[{"var":"bag.numbers"},{">":[{"var":""},0]}]},{">":[{"var":""},0]}]}`},
	}

	for i, input := range inputs {
		t.Run(fmt.Sprintf("case_%d", i), func(t *testing.T) {
			logic := input.logic
			if strings.HasPrefix(logic, `{"all"`) {
				if _, err := tr.TranspileCondition(logic); err != nil {
					t.Fatalf("TranspileCondition() failed for complex input: %v", err)
				}
				if _, _, err := tr.TranspileParameterizedCondition(logic); err != nil {
					t.Fatalf("TranspileParameterizedCondition() failed for complex input: %v", err)
				}
				return
			}
			if _, err := tr.TranspileValue(logic); err != nil {
				if input.wantValueErr && strings.Contains(err.Error(), "does not support array literals whose elements are arrays") {
					return
				}
				t.Fatalf("TranspileValue() failed for complex input: %v", err)
			}
			if input.wantValueErr {
				t.Fatal("TranspileValue() succeeded, want nested-array dialect error")
			}
			if _, _, err := tr.TranspileParameterizedValue(logic); err != nil {
				t.Fatalf("TranspileParameterizedValue() failed for complex input: %v", err)
			}
		})
	}
}
