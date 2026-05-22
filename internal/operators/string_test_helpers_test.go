package operators

type stringSchemaProvider struct {
	fields map[string]string
}

func (m *stringSchemaProvider) HasField(fieldName string) bool {
	_, ok := m.fields[fieldName]
	return ok
}

func (m *stringSchemaProvider) GetFieldType(fieldName string) string {
	return m.fields[fieldName]
}

func (m *stringSchemaProvider) ValidateField(_ string) error {
	return nil
}

func (m *stringSchemaProvider) IsArrayType(fieldName string) bool {
	return m.fields[fieldName] == "array"
}

func (m *stringSchemaProvider) IsStringType(fieldName string) bool {
	return m.fields[fieldName] == "string"
}

func (m *stringSchemaProvider) IsNumericType(fieldName string) bool {
	t := m.fields[fieldName]
	return t == "integer" || t == "number"
}

func (m *stringSchemaProvider) IsBooleanType(fieldName string) bool {
	return m.fields[fieldName] == "boolean"
}

func (m *stringSchemaProvider) IsEnumType(_ string) bool {
	return false
}

func (m *stringSchemaProvider) GetAllowedValues(_ string) []string {
	return nil
}

func (m *stringSchemaProvider) ValidateEnumValue(_, _ string) error {
	return nil
}
