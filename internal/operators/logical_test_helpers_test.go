package operators

type truthinessSchemaProvider struct {
	fields map[string]string // field name -> type
}

func (m *truthinessSchemaProvider) HasField(fieldName string) bool {
	_, exists := m.fields[fieldName]
	return exists
}

func (m *truthinessSchemaProvider) GetFieldType(fieldName string) string {
	return m.fields[fieldName]
}

func (m *truthinessSchemaProvider) ValidateField(_ string) error {
	return nil // Allow all fields for testing
}

func (m *truthinessSchemaProvider) IsArrayType(fieldName string) bool {
	return m.fields[fieldName] == "array"
}

func (m *truthinessSchemaProvider) IsStringType(fieldName string) bool {
	return m.fields[fieldName] == "string"
}

func (m *truthinessSchemaProvider) IsNumericType(fieldName string) bool {
	t := m.fields[fieldName]
	return t == "integer" || t == "number"
}

func (m *truthinessSchemaProvider) IsBooleanType(fieldName string) bool {
	return m.fields[fieldName] == "boolean"
}

func (m *truthinessSchemaProvider) IsEnumType(fieldName string) bool {
	return m.fields[fieldName] == "enum"
}

func (m *truthinessSchemaProvider) GetAllowedValues(_ string) []string {
	return nil
}

func (m *truthinessSchemaProvider) ValidateEnumValue(_, _ string) error {
	return nil
}
