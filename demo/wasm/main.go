//go:build js && wasm

package main

import (
	"encoding/json"
	"syscall/js"

	jsonlogic2sql "github.com/h22rana/jsonlogic2sql"
)

// transpilers holds active transpiler instances keyed by an ID.
var transpilers = map[int]*jsonlogic2sql.Transpiler{}
var nextID = 1

func emptySchema() (*jsonlogic2sql.Schema, error) {
	return jsonlogic2sql.NewSchema(nil)
}

func dialetFromString(s string) (jsonlogic2sql.Dialect, bool) {
	switch s {
	case "bigquery":
		return jsonlogic2sql.DialectBigQuery, true
	case "spanner":
		return jsonlogic2sql.DialectSpanner, true
	case "postgresql":
		return jsonlogic2sql.DialectPostgreSQL, true
	case "duckdb":
		return jsonlogic2sql.DialectDuckDB, true
	case "clickhouse":
		return jsonlogic2sql.DialectClickHouse, true
	default:
		return 0, false
	}
}

// newTranspiler(dialect: string) => {id: number} | {error: string}
func newTranspiler(_ js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return map[string]interface{}{"error": "dialect argument required"}
	}
	dialectStr := args[0].String()
	dialect, ok := dialetFromString(dialectStr)
	if !ok {
		return map[string]interface{}{"error": "unsupported dialect: " + dialectStr}
	}
	schema, err := emptySchema()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	t, err := jsonlogic2sql.NewTranspiler(dialect, schema)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	id := nextID
	nextID++
	transpilers[id] = t
	return map[string]interface{}{"id": id}
}

// setSchema(id: number, schemaJSON: string) => {ok: true} | {error: string}
func setSchema(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return map[string]interface{}{"error": "id and schemaJSON arguments required"}
	}
	id := args[0].Int()
	schemaJSON := args[1].String()

	t, ok := transpilers[id]
	if !ok {
		return map[string]interface{}{"error": "transpiler not found"}
	}

	schema, err := jsonlogic2sql.NewSchemaFromJSON([]byte(schemaJSON))
	if err != nil {
		return map[string]interface{}{"error": "invalid schema: " + err.Error()}
	}
	if err := t.SetSchema(schema); err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"ok": true}
}

// transpileValue(id: number, jsonLogic: string) => {sql: string} | {error: string}
func transpileValue(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return map[string]interface{}{"error": "id and jsonLogic arguments required"}
	}
	id := args[0].Int()
	jsonLogic := args[1].String()

	t, ok := transpilers[id]
	if !ok {
		return map[string]interface{}{"error": "transpiler not found"}
	}

	sql, err := t.TranspileValue(jsonLogic)
	if err != nil {
		errResult := map[string]interface{}{"error": err.Error()}
		if tErr, ok := jsonlogic2sql.AsTranspileError(err); ok {
			errResult["code"] = string(tErr.Code)
			errResult["operator"] = tErr.Operator
			errResult["path"] = tErr.Path
		}
		return errResult
	}
	return map[string]interface{}{"sql": sql}
}

// transpileCondition(id: number, jsonLogic: string) => {sql: string} | {error: string}
func transpileCondition(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return map[string]interface{}{"error": "id and jsonLogic arguments required"}
	}
	id := args[0].Int()
	jsonLogic := args[1].String()

	t, ok := transpilers[id]
	if !ok {
		return map[string]interface{}{"error": "transpiler not found"}
	}

	sql, err := t.TranspileCondition(jsonLogic)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"sql": sql}
}

// transpileParameterizedValue(id: number, jsonLogic: string) => {sql: string, params: string} | {error: string}
func transpileParameterizedValue(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return map[string]interface{}{"error": "id and jsonLogic arguments required"}
	}
	id := args[0].Int()
	jsonLogic := args[1].String()

	t, ok := transpilers[id]
	if !ok {
		return map[string]interface{}{"error": "transpiler not found"}
	}

	sql, params, err := t.TranspileParameterizedValue(jsonLogic)
	if err != nil {
		errResult := map[string]interface{}{"error": err.Error()}
		if tErr, ok := jsonlogic2sql.AsTranspileError(err); ok {
			errResult["code"] = string(tErr.Code)
			errResult["operator"] = tErr.Operator
			errResult["path"] = tErr.Path
		}
		return errResult
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return map[string]interface{}{"error": "failed to encode params: " + err.Error()}
	}
	return map[string]interface{}{"sql": sql, "params": string(paramsJSON)}
}

// transpileParameterizedCondition(id: number, jsonLogic: string) => {sql: string, params: string} | {error: string}
func transpileParameterizedCondition(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return map[string]interface{}{"error": "id and jsonLogic arguments required"}
	}
	id := args[0].Int()
	jsonLogic := args[1].String()

	t, ok := transpilers[id]
	if !ok {
		return map[string]interface{}{"error": "transpiler not found"}
	}

	sql, params, err := t.TranspileParameterizedCondition(jsonLogic)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return map[string]interface{}{"error": "failed to encode params: " + err.Error()}
	}
	return map[string]interface{}{"sql": sql, "params": string(paramsJSON)}
}

// quickTranspileParameterizedValue(dialect: string, jsonLogic: string) => {sql: string, params: string} | {error: string}
func quickTranspileParameterizedValue(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return map[string]interface{}{"error": "dialect and jsonLogic arguments required"}
	}
	dialectStr := args[0].String()
	jsonLogic := args[1].String()

	dialect, ok := dialetFromString(dialectStr)
	if !ok {
		return map[string]interface{}{"error": "unsupported dialect: " + dialectStr}
	}

	schema, err := emptySchema()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	sql, params, err := jsonlogic2sql.TranspileParameterizedValue(dialect, schema, jsonLogic)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return map[string]interface{}{"error": "failed to encode params: " + err.Error()}
	}
	return map[string]interface{}{"sql": sql, "params": string(paramsJSON)}
}

// quickTranspileParameterizedCondition(dialect: string, jsonLogic: string) => {sql: string, params: string} | {error: string}
func quickTranspileParameterizedCondition(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return map[string]interface{}{"error": "dialect and jsonLogic arguments required"}
	}
	dialectStr := args[0].String()
	jsonLogic := args[1].String()

	dialect, ok := dialetFromString(dialectStr)
	if !ok {
		return map[string]interface{}{"error": "unsupported dialect: " + dialectStr}
	}

	schema, err := emptySchema()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	sql, params, err := jsonlogic2sql.TranspileParameterizedCondition(dialect, schema, jsonLogic)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return map[string]interface{}{"error": "failed to encode params: " + err.Error()}
	}
	return map[string]interface{}{"sql": sql, "params": string(paramsJSON)}
}

// quickTranspileValue(dialect: string, jsonLogic: string) => {sql: string} | {error: string}
func quickTranspileValue(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return map[string]interface{}{"error": "dialect and jsonLogic arguments required"}
	}
	dialectStr := args[0].String()
	jsonLogic := args[1].String()

	dialect, ok := dialetFromString(dialectStr)
	if !ok {
		return map[string]interface{}{"error": "unsupported dialect: " + dialectStr}
	}

	schema, err := emptySchema()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	sql, err := jsonlogic2sql.TranspileValue(dialect, schema, jsonLogic)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"sql": sql}
}

// quickTranspileCondition(dialect: string, jsonLogic: string) => {sql: string} | {error: string}
func quickTranspileCondition(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return map[string]interface{}{"error": "dialect and jsonLogic arguments required"}
	}
	dialectStr := args[0].String()
	jsonLogic := args[1].String()

	dialect, ok := dialetFromString(dialectStr)
	if !ok {
		return map[string]interface{}{"error": "unsupported dialect: " + dialectStr}
	}

	schema, err := emptySchema()
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}

	sql, err := jsonlogic2sql.TranspileCondition(dialect, schema, jsonLogic)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"sql": sql}
}

// getDialects() => string[] - returns list of supported dialects.
func getDialects(_ js.Value, _ []js.Value) interface{} {
	dialects := []interface{}{"bigquery", "spanner", "postgresql", "duckdb", "clickhouse"}
	return dialects
}

// destroyTranspiler(id: number) => {ok: true}
func destroyTranspiler(_ js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return map[string]interface{}{"error": "id argument required"}
	}
	id := args[0].Int()
	delete(transpilers, id)
	return map[string]interface{}{"ok": true}
}

// getSamples() => JSON string of sample expressions.
func getSamples(_ js.Value, _ []js.Value) interface{} {
	samples := []map[string]string{
		{"name": "Simple equality", "mode": "condition", "jsonLogic": `{"==": [{"var": "status"}, "active"]}`},
		{"name": "Greater than", "mode": "condition", "jsonLogic": `{">": [{"var": "amount"}, 1000]}`},
		{"name": "AND condition", "mode": "condition", "jsonLogic": `{"and": [{">": [{"var": "amount"}, 5000]}, {"==": [{"var": "status"}, "pending"]}]}`},
		{"name": "OR condition", "mode": "condition", "jsonLogic": `{"or": [{">=": [{"var": "failedAttempts"}, 5]}, {"in": [{"var": "country"}, ["CN", "RU"]]}]}`},
		{"name": "IN array", "mode": "condition", "jsonLogic": `{"in": [{"var": "country"}, ["US", "CA", "MX"]]}`},
		{"name": "NOT IN", "mode": "condition", "jsonLogic": `{"!": {"in": [{"var": "status"}, ["blocked", "suspended"]]}}`},
		{"name": "NULL check", "mode": "condition", "jsonLogic": `{"==": [{"var": "deleted_at"}, null]}`},
		{"name": "Null-safe fields", "mode": "condition", "jsonLogic": `{"==": [{"var": "primary_email"}, {"var": "backup_email"}]}`},
		{"name": "Array element enum", "mode": "condition", "jsonLogic": `{"some": [{"var": "payment_methods"}, {"==": [{"var": "type"}, "BALANCE"]}]}`},
		{"name": "Chained comparison", "mode": "condition", "jsonLogic": `{"<": [18, {"var": "age"}, 65]}`},
		{"name": "Nested arithmetic", "mode": "condition", "jsonLogic": `{">": [{"+": [{"var": "base"}, {"*": [{"var": "bonus"}, 0.1]}]}, 1000]}`},
		{"name": "Value fallback", "mode": "value", "jsonLogic": `{"or": [false, "", "unknown"]}`},
		{"name": "Conditional value", "mode": "value", "jsonLogic": `{"if": [{">": [{"var": "age"}, 18]}, "adult", "minor"]}`},
		{"name": "String value", "mode": "value", "jsonLogic": `{"cat": ["Order ", {"var": "status"}]}`},
	}
	data, err := json.Marshal(samples)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func main() {
	c := make(chan struct{})

	// Register all functions on the global jsonlogic2sql object
	jsObj := map[string]interface{}{
		"newTranspiler":                        js.FuncOf(newTranspiler),
		"setSchema":                            js.FuncOf(setSchema),
		"transpileValue":                       js.FuncOf(transpileValue),
		"transpileCondition":                   js.FuncOf(transpileCondition),
		"transpileParameterizedValue":          js.FuncOf(transpileParameterizedValue),
		"transpileParameterizedCondition":      js.FuncOf(transpileParameterizedCondition),
		"quickTranspileValue":                  js.FuncOf(quickTranspileValue),
		"quickTranspileCondition":              js.FuncOf(quickTranspileCondition),
		"quickTranspileParameterizedValue":     js.FuncOf(quickTranspileParameterizedValue),
		"quickTranspileParameterizedCondition": js.FuncOf(quickTranspileParameterizedCondition),
		"destroyTranspiler":                    js.FuncOf(destroyTranspiler),
		"getDialects":                          js.FuncOf(getDialects),
		"getSamples":                           js.FuncOf(getSamples),
	}

	js.Global().Set("jsonlogic2sql", js.ValueOf(jsObj))

	// Signal that WASM is ready (without eval, safe under strict CSP)
	if cb := js.Global().Get("onJsonlogic2sqlReady"); cb.Truthy() {
		cb.Invoke()
	}

	<-c // Block forever
}
