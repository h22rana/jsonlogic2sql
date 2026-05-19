package jsonlogic2sql

import (
	"fmt"
	"testing"
)

// BenchmarkSimpleComparison benchmarks a simple equality comparison.
func BenchmarkSimpleComparison(b *testing.B) {
	tr, _ := NewTranspiler(DialectBigQuery, defaultTestSchema())
	input := `{"==": [{"var": "status"}, "active"]}`
	b.ResetTimer()
	for b.Loop() {
		_, _ = tr.TranspileCondition(input)
	}
}

// BenchmarkChainedComparison benchmarks a between-style chained comparison.
func BenchmarkChainedComparison(b *testing.B) {
	tr, _ := NewTranspiler(DialectBigQuery, defaultTestSchema())
	input := `{"<=": [18, {"var": "age"}, 65]}`
	b.ResetTimer()
	for b.Loop() {
		_, _ = tr.TranspileCondition(input)
	}
}

// BenchmarkNestedLogical benchmarks nested AND/OR with multiple conditions.
func BenchmarkNestedLogical(b *testing.B) {
	tr, _ := NewTranspiler(DialectBigQuery, defaultTestSchema())
	input := `{"and": [{">=": [{"var": "age"}, 18]}, {"or": [{"==": [{"var": "role"}, "admin"]}, {"==": [{"var": "role"}, "moderator"]}]}, {"!=": [{"var": "status"}, "banned"]}]}`
	b.ResetTimer()
	for b.Loop() {
		_, _ = tr.TranspileCondition(input)
	}
}

// BenchmarkArithmeticExpression benchmarks nested arithmetic operations.
func BenchmarkArithmeticExpression(b *testing.B) {
	tr, _ := NewTranspiler(DialectBigQuery, defaultTestSchema())
	input := `{">": [{"+": [{"var": "price"}, {"*": [{"var": "tax_rate"}, {"var": "price"}]}]}, 100]}`
	b.ResetTimer()
	for b.Loop() {
		_, _ = tr.TranspileCondition(input)
	}
}

// BenchmarkArrayAll benchmarks the all array operator.
func BenchmarkArrayAll(b *testing.B) {
	tr, _ := NewTranspiler(DialectBigQuery, defaultTestSchema())
	input := `{"all": [{"var": "scores"}, {">=": [{"var": ""}, 70]}]}`
	b.ResetTimer()
	for b.Loop() {
		_, _ = tr.TranspileCondition(input)
	}
}

// BenchmarkArrayReduce benchmarks the reduce operator with SUM pattern.
func BenchmarkArrayReduce(b *testing.B) {
	tr, _ := NewTranspiler(DialectBigQuery, defaultTestSchema())
	input := `{"reduce": [{"var": "amounts"}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}`
	b.ResetTimer()
	for b.Loop() {
		_, _ = tr.TranspileCondition(input)
	}
}

// BenchmarkStringConcat benchmarks string concatenation.
func BenchmarkStringConcat(b *testing.B) {
	tr, _ := NewTranspiler(DialectBigQuery, defaultTestSchema())
	input := `{"cat": [{"var": "first_name"}, " ", {"var": "last_name"}]}`
	b.ResetTimer()
	for b.Loop() {
		_, _ = tr.TranspileCondition(input)
	}
}

// BenchmarkIfCondition benchmarks the if/ternary operator.
func BenchmarkIfCondition(b *testing.B) {
	tr, _ := NewTranspiler(DialectBigQuery, defaultTestSchema())
	input := `{"if": [{">": [{"var": "score"}, 90]}, "A", {">": [{"var": "score"}, 80]}, "B", {">": [{"var": "score"}, 70]}, "C", "F"]}`
	b.ResetTimer()
	for b.Loop() {
		_, _ = tr.TranspileCondition(input)
	}
}

// BenchmarkDeeplyNested benchmarks a deeply nested expression combining multiple operator types.
func BenchmarkDeeplyNested(b *testing.B) {
	tr, _ := NewTranspiler(DialectBigQuery, defaultTestSchema())
	input := `{"and": [{"some": [{"filter": [{"var": "data"}, {">": [{"var": "value"}, 0]}]}, {">": [{"var": "score"}, 50]}]}, {">": [{"reduce": [{"var": "totals"}, {"+": [{"var": "accumulator"}, {"var": "current"}]}, 0]}, 1000]}]}`
	b.ResetTimer()
	for b.Loop() {
		_, _ = tr.TranspileCondition(input)
	}
}

// BenchmarkWithSchema benchmarks transpilation with schema validation enabled.
func BenchmarkWithSchema(b *testing.B) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "age", Type: FieldTypeInteger},
		{Name: "name", Type: FieldTypeString},
		{Name: "status", Type: FieldTypeEnum, AllowedValues: []string{"active", "inactive", "banned"}},
		{Name: "scores", Type: FieldTypeArray},
	})
	tr, _ := NewTranspilerWithConfig(&TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  schema,
	})
	input := `{"and": [{">=": [{"var": "age"}, 18]}, {"==": [{"var": "status"}, "active"]}, {"some": [{"var": "scores"}, {">": [{"var": ""}, 70]}]}]}`
	b.ResetTimer()
	for b.Loop() {
		_, _ = tr.TranspileCondition(input)
	}
}

// BenchmarkDialects benchmarks the same expression across all dialects.
func BenchmarkDialects(b *testing.B) {
	input := `{"and": [{">=": [{"var": "age"}, 18]}, {"in": [{"var": "status"}, ["active", "pending"]]}, {"some": [{"var": "tags"}, {"==": [{"var": ""}, "vip"]}]}]}`

	dialects := []struct {
		name    string
		dialect Dialect
	}{
		{"BigQuery", DialectBigQuery},
		{"Spanner", DialectSpanner},
		{"PostgreSQL", DialectPostgreSQL},
		{"DuckDB", DialectDuckDB},
		{"ClickHouse", DialectClickHouse},
	}

	for _, d := range dialects {
		b.Run(d.name, func(b *testing.B) {
			tr, _ := NewTranspiler(d.dialect, defaultTestSchema())
			b.ResetTimer()
			for b.Loop() {
				_, _ = tr.TranspileCondition(input)
			}
		})
	}
}

// BenchmarkInOperator benchmarks the in operator with a large array.
func BenchmarkInOperator(b *testing.B) {
	tr, _ := NewTranspiler(DialectBigQuery, defaultTestSchema())
	input := `{"in": [{"var": "code"}, ["A001", "A002", "A003", "A004", "A005", "A006", "A007", "A008", "A009", "A010", "A011", "A012", "A013", "A014", "A015", "A016", "A017", "A018", "A019", "A020"]]}`
	b.ResetTimer()
	for b.Loop() {
		_, _ = tr.TranspileCondition(input)
	}
}

// BenchmarkTranspileCondition benchmarks TranspileCondition (without WHERE prefix).
func BenchmarkTranspileCondition(b *testing.B) {
	tr, _ := NewTranspiler(DialectBigQuery, defaultTestSchema())
	input := `{"and": [{">=": [{"var": "age"}, 18]}, {"==": [{"var": "active"}, true]}]}`
	b.ResetTimer()
	for b.Loop() {
		_, _ = tr.TranspileCondition(input)
	}
}

// BenchmarkTranspileConditionFromMap benchmarks TranspileConditionFromMap with pre-parsed input.
func BenchmarkTranspileConditionFromMap(b *testing.B) {
	tr, _ := NewTranspiler(DialectBigQuery, defaultTestSchema())
	input := map[string]interface{}{
		"and": []interface{}{
			map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": "age"}, 18}},
			map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "active"}},
		},
	}
	b.ResetTimer()
	for b.Loop() {
		_, _ = tr.TranspileConditionFromMap(input)
	}
}

// BenchmarkTranspileParameterizedCondition targets bind collection, placeholder
// validation, and predicate parsing in the common WHERE-clause path.
func BenchmarkTranspileParameterizedCondition(b *testing.B) {
	tr := mustBenchmarkTranspiler(b, DialectBigQuery)
	input := `{"and":[{">=":[{"var":"age"},18]},{"in":[{"var":"status"},["active","pending","review"]]},{"==":[{"var":"verified"},true]}]}`

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, _, err := tr.TranspileParameterizedCondition(input); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTranspileParameterizedValue covers value-mode CASE/concat output plus
// bind collection, which exercises a different parser path than conditions.
func BenchmarkTranspileParameterizedValue(b *testing.B) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "amount", Type: FieldTypeNumber},
		{Name: "nickname", Type: FieldTypeString},
	})
	tr := mustBenchmarkTranspilerWithConfig(b, &TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  schema,
	})
	input := `{"cat":[{"if":[{">":[{"var":"amount"},100]},"high","low"]},":",{"or":["",{"var":"nickname"},"anonymous"]}]}`

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, _, err := tr.TranspileParameterizedValue(input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTranspileValueFallbackWithSchema(b *testing.B) {
	schema := mustNewSchema([]FieldSchema{
		{Name: "nickname", Type: FieldTypeString},
	})
	tr := mustBenchmarkTranspilerWithConfig(b, &TranspilerConfig{
		Dialect: DialectBigQuery,
		Schema:  schema,
	})
	input := `{"or":["",{"var":"nickname"},"anonymous"]}`

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := tr.TranspileValue(input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNullSafeFieldEquality(b *testing.B) {
	tr := mustBenchmarkTranspilerWithConfig(b, &TranspilerConfig{
		Dialect:               DialectBigQuery,
		NullSafeFieldEquality: true,
	})
	input := `{"and":[{"==":[{"var":"left"},{"var":"right"}]},{"!==":[{"var":["primary",null]},{"var":["secondary",null]}]}]}`

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := tr.TranspileCondition(input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCustomOperators(b *testing.B) {
	tr := mustBenchmarkTranspiler(b, DialectBigQuery)
	registerBenchmarkCustomOperators(b, tr)

	benchmarks := map[string]string{
		"Condition": `{"and":[{">":[{"strlen":[{"var":"name"}]},3]},{"isPositive":[{"var":"score"}]}]}`,
		"Value":     `{"cat":[{"lower":[{"var":"name"}]},"-",{"if":[{"isPositive":[{"var":"score"}]},"positive","other"]}]}`,
	}

	for name, input := range benchmarks {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				var err error
				if name == "Condition" {
					_, err = tr.TranspileCondition(input)
				} else {
					_, err = tr.TranspileValue(input)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkArrayMapValueLambda(b *testing.B) {
	tr := mustBenchmarkTranspiler(b, DialectBigQuery)
	input := `{"map":[{"var":"items"},{"if":[{">":[{"var":"score"},0]},{"var":"score"},0]}]}`

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := tr.TranspileValue(input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParameterizedArrayScopedDefaults(b *testing.B) {
	tr := mustBenchmarkTranspiler(b, DialectBigQuery)
	input := `{"map":[{"var":"items"},{"cat":[{"var":["label","unknown"]},"-",{"var":["code","na"]}]}]}`

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, _, err := tr.TranspileParameterizedValue(input); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParameterizedValueDialects(b *testing.B) {
	input := `{"map":[{"filter":[{"var":"items"},{">":[{"var":"score"},10]}]},{"cat":[{"var":["label","unknown"]},":",{"var":"score"}]}]}`

	for _, d := range benchmarkDialects() {
		b.Run(d.name, func(b *testing.B) {
			tr := mustBenchmarkTranspiler(b, d.dialect)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, _, err := tr.TranspileParameterizedValue(input); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkConditionInputForms(b *testing.B) {
	tr := mustBenchmarkTranspiler(b, DialectBigQuery)
	jsonInput := `{"and":[{">=":[{"var":"age"},18]},{"==":[{"var":"status"},"active"]}]}`
	mapInput := map[string]interface{}{
		"and": []interface{}{
			map[string]interface{}{">=": []interface{}{map[string]interface{}{"var": "age"}, 18}},
			map[string]interface{}{"==": []interface{}{map[string]interface{}{"var": "status"}, "active"}},
		},
	}

	b.Run("JSONString", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := tr.TranspileCondition(jsonInput); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Map", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := tr.TranspileConditionFromMap(mapInput); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("Interface", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if _, err := tr.TranspileConditionFromInterface(mapInput); err != nil {
				b.Fatal(err)
			}
		}
	})
}

type benchmarkDialectCase struct {
	name    string
	dialect Dialect
}

func benchmarkDialects() []benchmarkDialectCase {
	return []benchmarkDialectCase{
		{"BigQuery", DialectBigQuery},
		{"Spanner", DialectSpanner},
		{"PostgreSQL", DialectPostgreSQL},
		{"DuckDB", DialectDuckDB},
		{"ClickHouse", DialectClickHouse},
	}
}

func mustBenchmarkTranspiler(b *testing.B, d Dialect) *Transpiler {
	b.Helper()
	tr, err := NewTranspiler(d, defaultTestSchema())
	if err != nil {
		b.Fatal(err)
	}
	return tr
}

func mustBenchmarkTranspilerWithConfig(b *testing.B, config *TranspilerConfig) *Transpiler {
	b.Helper()
	tr, err := NewTranspilerWithConfig(config)
	if err != nil {
		b.Fatal(err)
	}
	return tr
}

func registerBenchmarkCustomOperators(b *testing.B, tr *Transpiler) {
	b.Helper()

	if err := tr.RegisterOperatorFunc("strlen", func(_ string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 1 {
			return OperatorResult{}, fmt.Errorf("strlen requires exactly 1 argument")
		}
		return ValueSQL(fmt.Sprintf("LENGTH(%s)", args[0].SQL), ExpressionTypeNumber), nil
	}); err != nil {
		b.Fatal(err)
	}

	if err := tr.RegisterOperatorFunc("lower", func(_ string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 1 {
			return OperatorResult{}, fmt.Errorf("lower requires exactly 1 argument")
		}
		return ValueSQL(fmt.Sprintf("LOWER(%s)", args[0].SQL), ExpressionTypeString), nil
	}); err != nil {
		b.Fatal(err)
	}

	if err := tr.RegisterOperatorFunc("isPositive", func(_ string, args []OperatorArg) (OperatorResult, error) {
		if len(args) != 1 {
			return OperatorResult{}, fmt.Errorf("isPositive requires exactly 1 argument")
		}
		return PredicateSQL(fmt.Sprintf("%s > 0", args[0].SQL)), nil
	}); err != nil {
		b.Fatal(err)
	}
}
