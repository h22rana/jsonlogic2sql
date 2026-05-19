package jsonlogic2sql

import (
	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
	"github.com/h22rana/jsonlogic2sql/internal/params"
)

// QueryParam represents a single bind parameter collected during parameterized transpilation.
// Name is the stable identifier (e.g., "p1", "p2"), and Value is the Go-native value to bind.
type QueryParam = params.QueryParam

// TranspileParameterizedCondition converts a JSON Logic string to a SQL condition
// (without the WHERE keyword) with bind parameter placeholders.
func (t *Transpiler) TranspileParameterizedCondition(jsonLogic string) (string, []QueryParam, error) {
	logic, err := decodeJSONLogic(jsonLogic)
	if err != nil {
		return "", nil, tperrors.NewInvalidJSON(err)
	}
	return t.parser.ParseConditionParameterized(logic)
}

// TranspileParameterizedConditionFromMap converts a pre-parsed JSON Logic map to
// a SQL condition (without the WHERE keyword) with bind parameter placeholders.
func (t *Transpiler) TranspileParameterizedConditionFromMap(logic map[string]interface{}) (string, []QueryParam, error) {
	return t.parser.ParseConditionParameterized(logic)
}

// TranspileParameterizedConditionFromInterface converts any JSON Logic interface{}
// to a SQL condition (without the WHERE keyword) with bind parameter placeholders.
func (t *Transpiler) TranspileParameterizedConditionFromInterface(logic interface{}) (string, []QueryParam, error) {
	return t.parser.ParseConditionParameterized(logic)
}

// TranspileParameterizedValue converts a JSON Logic string to a parameterized
// SQL value expression.
func (t *Transpiler) TranspileParameterizedValue(jsonLogic string) (string, []QueryParam, error) {
	logic, err := decodeJSONLogic(jsonLogic)
	if err != nil {
		return "", nil, tperrors.NewInvalidJSON(err)
	}
	return t.parser.ParseValueParameterized(logic)
}

// TranspileParameterizedValueFromMap converts a pre-parsed JSON Logic map to a
// parameterized SQL value expression.
func (t *Transpiler) TranspileParameterizedValueFromMap(logic map[string]interface{}) (string, []QueryParam, error) {
	return t.parser.ParseValueParameterized(logic)
}

// TranspileParameterizedValueFromInterface converts any JSON Logic interface{}
// to a parameterized SQL value expression.
func (t *Transpiler) TranspileParameterizedValueFromInterface(logic interface{}) (string, []QueryParam, error) {
	return t.parser.ParseValueParameterized(logic)
}

// Package-level convenience functions.

// TranspileParameterizedCondition converts a JSON Logic string to a SQL condition
// with bind parameter placeholders.
func TranspileParameterizedCondition(d Dialect, schema *Schema, jsonLogic string) (string, []QueryParam, error) {
	t, err := NewTranspiler(d, schema)
	if err != nil {
		return "", nil, err
	}
	return t.TranspileParameterizedCondition(jsonLogic)
}

// TranspileParameterizedConditionFromMap converts a pre-parsed JSON Logic map to
// a SQL condition (without the WHERE keyword) with bind parameter placeholders.
func TranspileParameterizedConditionFromMap(d Dialect, schema *Schema, logic map[string]interface{}) (string, []QueryParam, error) {
	t, err := NewTranspiler(d, schema)
	if err != nil {
		return "", nil, err
	}
	return t.TranspileParameterizedConditionFromMap(logic)
}

// TranspileParameterizedConditionFromInterface converts any JSON Logic interface{}
// to a SQL condition (without the WHERE keyword) with bind parameter placeholders.
func TranspileParameterizedConditionFromInterface(d Dialect, schema *Schema, logic interface{}) (string, []QueryParam, error) {
	t, err := NewTranspiler(d, schema)
	if err != nil {
		return "", nil, err
	}
	return t.TranspileParameterizedConditionFromInterface(logic)
}

// TranspileParameterizedValue converts a JSON Logic string to a parameterized
// SQL value expression.
func TranspileParameterizedValue(d Dialect, schema *Schema, jsonLogic string) (string, []QueryParam, error) {
	t, err := NewTranspiler(d, schema)
	if err != nil {
		return "", nil, err
	}
	return t.TranspileParameterizedValue(jsonLogic)
}

// TranspileParameterizedValueFromMap converts a pre-parsed JSON Logic map to a
// parameterized SQL value expression.
func TranspileParameterizedValueFromMap(d Dialect, schema *Schema, logic map[string]interface{}) (string, []QueryParam, error) {
	t, err := NewTranspiler(d, schema)
	if err != nil {
		return "", nil, err
	}
	return t.TranspileParameterizedValueFromMap(logic)
}

// TranspileParameterizedValueFromInterface converts any JSON Logic interface{}
// to a parameterized SQL value expression.
func TranspileParameterizedValueFromInterface(d Dialect, schema *Schema, logic interface{}) (string, []QueryParam, error) {
	t, err := NewTranspiler(d, schema)
	if err != nil {
		return "", nil, err
	}
	return t.TranspileParameterizedValueFromInterface(logic)
}
