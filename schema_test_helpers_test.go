package jsonlogic2sql

func mustNewSchema(fields []FieldSchema) *Schema {
	schema, err := NewSchema(fields)
	if err != nil {
		panic(err)
	}
	return schema
}
