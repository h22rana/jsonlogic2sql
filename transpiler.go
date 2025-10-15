package jsonlogic2sql

import (
	"encoding/json"
	"fmt"

	"github.com/h22rana/jsonlogic2sql/internal/parser"
)

// TranspilerConfig holds configuration options for the transpiler
type TranspilerConfig struct {
	UseANSINotEqual bool // true: <>, false: !=
}

// Transpiler provides the main API for converting JSON Logic to SQL WHERE clauses
type Transpiler struct {
	parser *parser.Parser
	config *TranspilerConfig
}

// NewTranspiler creates a new transpiler instance
func NewTranspiler() *Transpiler {
	return &Transpiler{
		parser: parser.NewParser(),
		config: &TranspilerConfig{
			UseANSINotEqual: true, // Default to ANSI SQL <>
		},
	}
}

// NewTranspilerWithConfig creates a new transpiler instance with custom configuration
func NewTranspilerWithConfig(config *TranspilerConfig) *Transpiler {
	return &Transpiler{
		parser: parser.NewParser(),
		config: config,
	}
}

// Transpile converts a JSON Logic string to a SQL WHERE clause
func (t *Transpiler) Transpile(jsonLogic string) (string, error) {
	var logic interface{}
	if err := json.Unmarshal([]byte(jsonLogic), &logic); err != nil {
		return "", fmt.Errorf("invalid JSON: %v", err)
	}

	return t.parser.Parse(logic)
}

// TranspileFromMap converts a pre-parsed JSON Logic map to a SQL WHERE clause
func (t *Transpiler) TranspileFromMap(logic map[string]interface{}) (string, error) {
	return t.parser.Parse(logic)
}

// TranspileFromInterface converts any JSON Logic interface{} to a SQL WHERE clause
func (t *Transpiler) TranspileFromInterface(logic interface{}) (string, error) {
	return t.parser.Parse(logic)
}

// Convenience functions for direct usage without creating a Transpiler instance

// Transpile converts a JSON Logic string to a SQL WHERE clause
func Transpile(jsonLogic string) (string, error) {
	t := NewTranspiler()
	return t.Transpile(jsonLogic)
}

// TranspileFromMap converts a pre-parsed JSON Logic map to a SQL WHERE clause
func TranspileFromMap(logic map[string]interface{}) (string, error) {
	t := NewTranspiler()
	return t.TranspileFromMap(logic)
}

// TranspileFromInterface converts any JSON Logic interface{} to a SQL WHERE clause
func TranspileFromInterface(logic interface{}) (string, error) {
	t := NewTranspiler()
	return t.TranspileFromInterface(logic)
}
