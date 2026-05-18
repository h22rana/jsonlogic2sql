package jsonlogic2sql

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

// FieldType represents the type of a field in the schema.
type FieldType string

// Field type constants for schema validation.
const (
	FieldTypeString  FieldType = "string"
	FieldTypeInteger FieldType = "integer"
	FieldTypeNumber  FieldType = "number"
	FieldTypeBoolean FieldType = "boolean"
	FieldTypeArray   FieldType = "array"
	FieldTypeObject  FieldType = "object"
	FieldTypeEnum    FieldType = "enum"
)

// FieldSchema represents the schema/metadata for a single field.
type FieldSchema struct {
	Name          string        `json:"name"`
	Type          FieldType     `json:"type"`
	AllowedValues []string      `json:"allowedValues,omitempty"` // For enum types: list of valid values
	Fields        []FieldSchema `json:"fields,omitempty"`        // Nested object fields
	ElementFields []FieldSchema `json:"elementFields,omitempty"` // Nested fields on array elements
}

// Schema represents the collection of field schemas.
type Schema struct {
	fields        map[string]FieldSchema // Map field name to schema for O(1) lookup
	validationErr error
}

// NewSchema creates a new schema from a slice of field schemas.
// Returns an error if any field name contains quote characters (backtick,
// double quote, or single quote). Schema field names must be raw, unquoted
// identifiers; the transpiler handles quoting automatically.
func NewSchema(fields []FieldSchema) (*Schema, error) {
	if err := ValidateSchemaFields(fields); err != nil {
		return nil, err
	}
	return newSchemaUnchecked(fields), nil
}

func newSchemaUnchecked(fields []FieldSchema) *Schema {
	s := &Schema{
		fields: make(map[string]FieldSchema),
	}
	for _, field := range fields {
		s.addField("", field)
	}
	return s
}

func (s *Schema) addField(prefix string, field FieldSchema) {
	fieldName := joinSchemaPath(prefix, field.Name)
	stored := field
	stored.Name = fieldName
	s.fields[fieldName] = stored

	for _, child := range field.Fields {
		s.addField(fieldName, child)
	}
	for _, child := range field.ElementFields {
		s.addField(fieldName, child)
	}
}

// ValidateSchemaFields validates schema field definitions.
// Field names must be raw, unquoted identifiers; the transpiler applies SQL
// identifier quoting automatically based on the target dialect.
func ValidateSchemaFields(fields []FieldSchema) error {
	for _, field := range fields {
		if err := validateSchemaField("", field); err != nil {
			return err
		}
	}
	return nil
}

func validateSchemaField(prefix string, field FieldSchema) error {
	if strings.TrimSpace(field.Name) == "" {
		if prefix == "" {
			return fmt.Errorf("schema field requires non-empty name")
		}
		return fmt.Errorf("schema field under %q requires non-empty name", prefix)
	}
	if field.Type == "" {
		return fmt.Errorf("schema field %q requires non-empty type", joinSchemaPath(prefix, field.Name))
	}
	if !isSupportedFieldType(field.Type) {
		return fmt.Errorf("schema field %q has unsupported type %q", joinSchemaPath(prefix, field.Name), field.Type)
	}

	fieldName := joinSchemaPath(prefix, field.Name)
	for _, seg := range strings.Split(fieldName, ".") {
		if dialect.ContainsQuoteCharacters(seg) {
			return fmt.Errorf(
				"schema field %q contains quote characters; "+
					"use raw identifiers; the transpiler handles quoting automatically", fieldName)
		}
	}
	if len(field.Fields) > 0 && field.Type != FieldTypeObject {
		return fmt.Errorf("schema field %q uses fields but has type %q; fields require object type", fieldName, field.Type)
	}
	if len(field.ElementFields) > 0 && field.Type != FieldTypeArray {
		return fmt.Errorf("schema field %q uses elementFields but has type %q; elementFields require array type", fieldName, field.Type)
	}
	if field.Type == FieldTypeEnum {
		if len(field.AllowedValues) == 0 {
			return fmt.Errorf("schema enum field %q requires at least one allowedValues entry", fieldName)
		}
	} else if len(field.AllowedValues) > 0 {
		return fmt.Errorf("schema field %q uses allowedValues but has type %q; allowedValues require enum type", fieldName, field.Type)
	}
	for _, child := range field.Fields {
		if err := validateSchemaField(fieldName, child); err != nil {
			return err
		}
	}
	for _, child := range field.ElementFields {
		if err := validateSchemaField(fieldName, child); err != nil {
			return err
		}
	}
	return nil
}

func isSupportedFieldType(fieldType FieldType) bool {
	switch fieldType {
	case FieldTypeString,
		FieldTypeInteger,
		FieldTypeNumber,
		FieldTypeBoolean,
		FieldTypeArray,
		FieldTypeObject,
		FieldTypeEnum:
		return true
	default:
		return false
	}
}

func joinSchemaPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	if name == "" {
		return prefix
	}
	return prefix + "." + name
}

// NewSchemaFromJSON creates a new schema from a JSON byte slice.
func NewSchemaFromJSON(data []byte) (*Schema, error) {
	var fields []FieldSchema
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("invalid schema JSON: %w", err)
	}
	return NewSchema(fields)
}

// NewSchemaFromFile loads a schema from a JSON file.
func NewSchemaFromFile(path string) (*Schema, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file: %w", err)
	}
	return NewSchemaFromJSON(data)
}

// HasField checks if a field exists in the schema.
func (s *Schema) HasField(fieldName string) bool {
	if s == nil {
		return true // No schema means all fields are allowed
	}
	_, exists := s.fields[fieldName]
	return exists
}

// ValidateField checks if a field exists in the schema and returns an error if not.
func (s *Schema) ValidateField(fieldName string) error {
	if s == nil {
		return nil // No schema means no validation
	}
	if s.validationErr != nil {
		return s.validationErr
	}
	if !s.HasField(fieldName) {
		return fmt.Errorf("field '%s' is not defined in schema", fieldName)
	}
	return nil
}

// ResolveScopedField resolves a field name relative to an array element or
// object schema scope. For example, field "type" in scope "payments" resolves
// to "payments.type" and validates against that nested schema entry.
func (s *Schema) ResolveScopedField(scopePath, fieldName string) (string, error) {
	if s == nil {
		return fieldName, nil
	}
	if s.validationErr != nil {
		return "", s.validationErr
	}
	if fieldName == "" {
		return scopePath, nil
	}
	if scopePath == "" {
		if err := s.ValidateField(fieldName); err != nil {
			return "", err
		}
		return fieldName, nil
	}
	scopedName := joinSchemaPath(scopePath, fieldName)
	if s.HasField(scopedName) {
		return scopedName, nil
	}
	return "", fmt.Errorf("field '%s' is not defined in schema scope '%s'", fieldName, scopePath)
}

// GetFields returns all field names in the schema.
func (s *Schema) GetFields() []string {
	if s == nil {
		return nil
	}
	fields := make([]string, 0, len(s.fields))
	for name := range s.fields {
		fields = append(fields, name)
	}
	return fields
}

// IsArrayType checks if a field is of array type.
func (s *Schema) IsArrayType(fieldName string) bool {
	return s.GetFieldTypeFieldType(fieldName) == FieldTypeArray
}

// IsStringType checks if a field is of string type.
func (s *Schema) IsStringType(fieldName string) bool {
	return s.GetFieldTypeFieldType(fieldName) == FieldTypeString
}

// IsNumericType checks if a field is of numeric type (integer or number).
func (s *Schema) IsNumericType(fieldName string) bool {
	fieldType := s.GetFieldTypeFieldType(fieldName)
	return fieldType == FieldTypeInteger || fieldType == FieldTypeNumber
}

// IsBooleanType checks if a field is of boolean type.
func (s *Schema) IsBooleanType(fieldName string) bool {
	return s.GetFieldTypeFieldType(fieldName) == FieldTypeBoolean
}

// IsEnumType checks if a field is of enum type.
func (s *Schema) IsEnumType(fieldName string) bool {
	return s.GetFieldTypeFieldType(fieldName) == FieldTypeEnum
}

// GetAllowedValues returns the allowed values for an enum field
// Returns nil if the field is not an enum or doesn't exist.
func (s *Schema) GetAllowedValues(fieldName string) []string {
	if s == nil {
		return nil
	}
	if field, exists := s.fields[fieldName]; exists {
		return field.AllowedValues
	}
	return nil
}

// ValidateEnumValue checks if a value is valid for an enum field
// Returns nil if valid, error if invalid.
func (s *Schema) ValidateEnumValue(fieldName, value string) error {
	if s == nil {
		return nil // No schema means no validation
	}

	if !s.IsEnumType(fieldName) {
		return nil // Not an enum field, no validation needed
	}

	allowedValues := s.GetAllowedValues(fieldName)
	if len(allowedValues) == 0 {
		return nil // No allowed values defined, skip validation
	}

	for _, allowed := range allowedValues {
		if value == allowed {
			return nil // Value is valid
		}
	}

	return fmt.Errorf("invalid enum value '%s' for field '%s': allowed values are %v", value, fieldName, allowedValues)
}

// GetFieldType returns the type of a field as a string.
// This implements the operators.SchemaProvider interface.
func (s *Schema) GetFieldType(fieldName string) string {
	return string(s.GetFieldTypeFieldType(fieldName))
}

// GetFieldTypeFieldType returns the type of a field as FieldType (internal use).
func (s *Schema) GetFieldTypeFieldType(fieldName string) FieldType {
	if s == nil {
		return "" // No schema means unknown type
	}
	if field, exists := s.fields[fieldName]; exists {
		return field.Type
	}
	return ""
}
