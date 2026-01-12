package jsonlogic2sql

import (
	"encoding/json"
	"fmt"
	"os"
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
	Name          string    `json:"name"`
	Type          FieldType `json:"type"`
	AllowedValues []string  `json:"allowedValues,omitempty"` // For enum types: list of valid values
}

// Schema represents the collection of field schemas.
type Schema struct {
	fields map[string]FieldSchema // Map field name to schema for O(1) lookup
}

// NewSchema creates a new schema from a slice of field schemas.
func NewSchema(fields []FieldSchema) *Schema {
	s := &Schema{
		fields: make(map[string]FieldSchema),
	}
	for _, field := range fields {
		s.fields[field.Name] = field
	}
	return s
}

// NewSchemaFromJSON creates a new schema from a JSON byte slice.
func NewSchemaFromJSON(data []byte) (*Schema, error) {
	var fields []FieldSchema
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("invalid schema JSON: %w", err)
	}
	return NewSchema(fields), nil
}

// NewSchemaFromFile loads a schema from a JSON file.
func NewSchemaFromFile(filepath string) (*Schema, error) {
	data, err := os.ReadFile(filepath)
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
	if !s.HasField(fieldName) {
		return fmt.Errorf("field '%s' is not defined in schema", fieldName)
	}
	return nil
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
