package jsonlogic2sql

func mustNewSchema(fields []FieldSchema) *Schema {
	schema, err := NewSchema(fields)
	if err != nil {
		panic(err)
	}
	return schema
}

type testSchemaMode struct {
	name   string
	schema *Schema
}

func allSchemaModes(schema *Schema) []testSchemaMode {
	return []testSchemaMode{
		{name: "schema-less"},
		{name: "schema-aware", schema: schema},
	}
}
