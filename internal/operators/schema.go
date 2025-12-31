package operators

// SchemaProvider provides schema information for field validation and type checking
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
}
