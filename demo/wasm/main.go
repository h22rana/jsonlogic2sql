//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"syscall/js"

	jsonlogic2sql "github.com/h22rana/jsonlogic2sql"
)

// transpilers holds active transpiler instances keyed by an ID.
var transpilers = map[int]*jsonlogic2sql.Transpiler{}
var nextID = 1

const (
	dialectIDBigQuery   = "bigquery"
	dialectIDSpanner    = "spanner"
	dialectIDPostgreSQL = "postgresql"
	dialectIDDuckDB     = "duckdb"
	dialectIDClickHouse = "clickhouse"
)

var wasmDialects = []struct {
	id      string
	dialect jsonlogic2sql.Dialect
}{
	{dialectIDBigQuery, jsonlogic2sql.DialectBigQuery},
	{dialectIDSpanner, jsonlogic2sql.DialectSpanner},
	{dialectIDPostgreSQL, jsonlogic2sql.DialectPostgreSQL},
	{dialectIDDuckDB, jsonlogic2sql.DialectDuckDB},
	{dialectIDClickHouse, jsonlogic2sql.DialectClickHouse},
}

var defaultDemoSchemaFields = []jsonlogic2sql.FieldSchema{
	{Name: "amount", Type: jsonlogic2sql.FieldTypeInteger},
	{Name: "status", Type: jsonlogic2sql.FieldTypeString},
	{Name: "failedAttempts", Type: jsonlogic2sql.FieldTypeInteger},
	{Name: "country", Type: jsonlogic2sql.FieldTypeString},
	{Name: "deleted_at", Type: jsonlogic2sql.FieldTypeString},
	{Name: "primary_email", Type: jsonlogic2sql.FieldTypeString},
	{Name: "backup_email", Type: jsonlogic2sql.FieldTypeString},
	{Name: "age", Type: jsonlogic2sql.FieldTypeInteger},
	{Name: "base", Type: jsonlogic2sql.FieldTypeNumber},
	{Name: "bonus", Type: jsonlogic2sql.FieldTypeNumber},
	{Name: "merchant_code", Type: jsonlogic2sql.FieldTypeString},
	{Name: "price", Type: jsonlogic2sql.FieldTypeNumber},
	{Name: "tags", Type: jsonlogic2sql.FieldTypeArray},
	{Name: "description", Type: jsonlogic2sql.FieldTypeString},
	{
		Name: "user",
		Type: jsonlogic2sql.FieldTypeObject,
		Fields: []jsonlogic2sql.FieldSchema{
			{Name: "verified", Type: jsonlogic2sql.FieldTypeBoolean},
			{Name: "roles", Type: jsonlogic2sql.FieldTypeArray},
			{Name: "age", Type: jsonlogic2sql.FieldTypeInteger},
		},
	},
	{
		Name: "payment_methods",
		Type: jsonlogic2sql.FieldTypeArray,
		ElementFields: []jsonlogic2sql.FieldSchema{
			{Name: "type", Type: jsonlogic2sql.FieldTypeEnum, AllowedValues: []string{"BALANCE", "CARD"}},
			{Name: "amount", Type: jsonlogic2sql.FieldTypeNumber},
			{
				Name: "details",
				Type: jsonlogic2sql.FieldTypeObject,
				Fields: []jsonlogic2sql.FieldSchema{
					{Name: "issuer", Type: jsonlogic2sql.FieldTypeString},
				},
			},
		},
	},
}

func schemaFromJSONString(schemaJSON string) (*jsonlogic2sql.Schema, error) {
	if strings.TrimSpace(schemaJSON) == "" {
		return nil, fmt.Errorf("schemaJSON argument required")
	}
	return jsonlogic2sql.NewSchemaFromJSON([]byte(schemaJSON))
}

func dialetFromString(s string) (jsonlogic2sql.Dialect, bool) {
	for _, candidate := range wasmDialects {
		if s == candidate.id {
			return candidate.dialect, true
		}
	}
	return 0, false
}

// newTranspiler(dialect: string, schemaJSON: string) => {id: number} | {error: string}
func newTranspiler(_ js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return map[string]interface{}{"error": "dialect and schemaJSON arguments required"}
	}
	dialectStr := args[0].String()
	schemaJSON := args[1].String()
	dialect, ok := dialetFromString(dialectStr)
	if !ok {
		return map[string]interface{}{"error": "unsupported dialect: " + dialectStr}
	}
	schema, err := schemaFromJSONString(schemaJSON)
	if err != nil {
		return map[string]interface{}{"error": "invalid schema: " + err.Error()}
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

	schema, err := schemaFromJSONString(schemaJSON)
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

// quickTranspileParameterizedValue(dialect: string, schemaJSON: string, jsonLogic: string) => {sql: string, params: string} | {error: string}
func quickTranspileParameterizedValue(_ js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		return map[string]interface{}{"error": "dialect, schemaJSON, and jsonLogic arguments required"}
	}
	dialectStr := args[0].String()
	schemaJSON := args[1].String()
	jsonLogic := args[2].String()

	dialect, ok := dialetFromString(dialectStr)
	if !ok {
		return map[string]interface{}{"error": "unsupported dialect: " + dialectStr}
	}

	schema, err := schemaFromJSONString(schemaJSON)
	if err != nil {
		return map[string]interface{}{"error": "invalid schema: " + err.Error()}
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

// quickTranspileParameterizedCondition(dialect: string, schemaJSON: string, jsonLogic: string) => {sql: string, params: string} | {error: string}
func quickTranspileParameterizedCondition(_ js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		return map[string]interface{}{"error": "dialect, schemaJSON, and jsonLogic arguments required"}
	}
	dialectStr := args[0].String()
	schemaJSON := args[1].String()
	jsonLogic := args[2].String()

	dialect, ok := dialetFromString(dialectStr)
	if !ok {
		return map[string]interface{}{"error": "unsupported dialect: " + dialectStr}
	}

	schema, err := schemaFromJSONString(schemaJSON)
	if err != nil {
		return map[string]interface{}{"error": "invalid schema: " + err.Error()}
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

// quickTranspileValue(dialect: string, schemaJSON: string, jsonLogic: string) => {sql: string} | {error: string}
func quickTranspileValue(_ js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		return map[string]interface{}{"error": "dialect, schemaJSON, and jsonLogic arguments required"}
	}
	dialectStr := args[0].String()
	schemaJSON := args[1].String()
	jsonLogic := args[2].String()

	dialect, ok := dialetFromString(dialectStr)
	if !ok {
		return map[string]interface{}{"error": "unsupported dialect: " + dialectStr}
	}

	schema, err := schemaFromJSONString(schemaJSON)
	if err != nil {
		return map[string]interface{}{"error": "invalid schema: " + err.Error()}
	}

	sql, err := jsonlogic2sql.TranspileValue(dialect, schema, jsonLogic)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"sql": sql}
}

// quickTranspileCondition(dialect: string, schemaJSON: string, jsonLogic: string) => {sql: string} | {error: string}
func quickTranspileCondition(_ js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		return map[string]interface{}{"error": "dialect, schemaJSON, and jsonLogic arguments required"}
	}
	dialectStr := args[0].String()
	schemaJSON := args[1].String()
	jsonLogic := args[2].String()

	dialect, ok := dialetFromString(dialectStr)
	if !ok {
		return map[string]interface{}{"error": "unsupported dialect: " + dialectStr}
	}

	schema, err := schemaFromJSONString(schemaJSON)
	if err != nil {
		return map[string]interface{}{"error": "invalid schema: " + err.Error()}
	}

	sql, err := jsonlogic2sql.TranspileCondition(dialect, schema, jsonLogic)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}
	}
	return map[string]interface{}{"sql": sql}
}

// getDialects() => string[] - returns list of supported dialects.
func getDialects(_ js.Value, _ []js.Value) interface{} {
	dialects := make([]interface{}, len(wasmDialects))
	for i, candidate := range wasmDialects {
		dialects[i] = candidate.id
	}
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

// getDefaultSchema() => JSON string of the schema used by built-in samples.
func getDefaultSchema(_ js.Value, _ []js.Value) interface{} {
	data, err := json.MarshalIndent(defaultDemoSchemaFields, "", "  ")
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
		"getDefaultSchema":                     js.FuncOf(getDefaultSchema),
	}

	js.Global().Set("jsonlogic2sql", js.ValueOf(jsObj))

	// Signal that WASM is ready (without eval, safe under strict CSP)
	if cb := js.Global().Get("onJsonlogic2sqlReady"); cb.Truthy() {
		cb.Invoke()
	}

	<-c // Block forever
}
