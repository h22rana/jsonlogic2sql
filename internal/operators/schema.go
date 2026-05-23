package operators

import "fmt"

// SchemaProvider provides schema information for field validation and type checking.
type SchemaProvider interface {
	// HasField checks if a field exists in the schema
	HasField(fieldName string) bool
	// GetFieldType returns the type of a field as a string, or empty string if not found
	GetFieldType(fieldName string) string
	// ValidateField checks if a field exists and returns an error if not
	ValidateField(fieldName string) error
	// IsArrayType checks if a field is of array type
	IsArrayType(fieldName string) bool
	// IsStringType checks if a field is of string type
	IsStringType(fieldName string) bool
	// IsNumericType checks if a field is of numeric type (integer or number)
	IsNumericType(fieldName string) bool
	// IsBooleanType checks if a field is of boolean type
	IsBooleanType(fieldName string) bool
	// IsEnumType checks if a field is of enum type
	IsEnumType(fieldName string) bool
	// GetAllowedValues returns the allowed values for an enum field
	GetAllowedValues(fieldName string) []string
	// ValidateEnumValue checks if a value is valid for an enum field
	ValidateEnumValue(fieldName, value string) error
}

// ScopedSchemaProvider is implemented by schemas that can resolve fields
// relative to a nested object or array-element schema scope.
type ScopedSchemaProvider interface {
	SchemaProvider
	ResolveScopedField(scopePath, fieldName string) (string, error)
}

// ArrayElementSchemaProvider is implemented by schemas that can distinguish
// scalar array fields from arrays whose elements expose object fields.
type ArrayElementSchemaProvider interface {
	SchemaProvider
	HasArrayElementFields(fieldName string) bool
}

// ArrayElementTypeProvider is implemented by schemas that declare scalar array
// element types for array-column membership and reduce aggregate validation.
type ArrayElementTypeProvider interface {
	SchemaProvider
	GetArrayElementType(fieldName string) string
}

// ArrayElementSchemaSignatureProvider is implemented by schemas that can
// compare full array-element object schemas, not just individual field names.
type ArrayElementSchemaSignatureProvider interface {
	SchemaProvider
	ArrayElementSchemaSignature(fieldName string) string
}

// ArrayElementSchemaComparator is implemented by schemas that can report the
// exact field-level reason two array-element schemas are incompatible.
type ArrayElementSchemaComparator interface {
	SchemaProvider
	ValidateArrayElementSchemasCompatible(leftField, rightField string) error
}

func schemaArrayElementType(schema SchemaProvider, fieldName string) string {
	provider, ok := schema.(ArrayElementTypeProvider)
	if !ok {
		return ""
	}
	return provider.GetArrayElementType(fieldName)
}

func schemaArrayElementExpressionType(schema SchemaProvider, fieldName string) (ExpressionType, bool) {
	typ := schemaFieldTypeExpressionType(schemaArrayElementType(schema, fieldName))
	return typ, typ != ExpressionTypeUnknown
}

func validateSchemaAllowedValue(schema SchemaProvider, fieldName, value string) error {
	allowedValues := schema.GetAllowedValues(fieldName)
	if len(allowedValues) == 0 {
		return nil
	}
	for _, allowed := range allowedValues {
		if value == allowed {
			return nil
		}
	}
	return errInvalidSchemaEnumValue(fieldName, value, allowedValues)
}

func validateSchemaEnumArrayElementValue(schema SchemaProvider, fieldName, value string) error {
	if schemaArrayElementType(schema, fieldName) != "enum" {
		return nil
	}
	return validateSchemaAllowedValue(schema, fieldName, value)
}

func errInvalidSchemaEnumValue(fieldName, value string, allowedValues []string) error {
	return fmt.Errorf("invalid enum value '%s' for field '%s': allowed values are %v", value, fieldName, allowedValues)
}
