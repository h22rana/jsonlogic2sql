package operators

import (
	"regexp"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

var arrayTestUnnestAliasPattern = regexp.MustCompile(`FROM UNNEST\(([^)]*)\) AS (elem[0-9]*)`)

func arrayTestDuckDBUnnestSourceAliases(d dialect.Dialect, sql string) string {
	if d != dialect.DialectDuckDB {
		return sql
	}
	return arrayTestUnnestAliasPattern.ReplaceAllString(sql, `FROM UNNEST($1) AS $2($2)`)
}

type arrayTestSchemaProvider struct {
	mockSchemaProvider
}

func (a *arrayTestSchemaProvider) GetFieldType(fieldName string) string {
	if a.IsArrayType(fieldName) {
		return "array"
	}
	return "number"
}

func (a *arrayTestSchemaProvider) IsArrayType(fieldName string) bool {
	switch fieldName {
	case "numbers", "scores", "values", "array1", "array2", "arr", "a", "b", "c", "d",
		"moreNumbers", "ages", "statuses", "prices", "items", "objectItems", "readings", "transactions",
		"logs", "flags", "temperatures":
		return true
	default:
		return false
	}
}

func (a *arrayTestSchemaProvider) HasArrayElementFields(fieldName string) bool {
	switch fieldName {
	case "objectItems", "readings", "transactions", "logs":
		return true
	default:
		return false
	}
}

func (a *arrayTestSchemaProvider) GetArrayElementType(fieldName string) string {
	switch fieldName {
	case "statuses":
		return "string"
	case "flags":
		return "boolean"
	case "objectItems", "readings", "transactions", "logs":
		return "object"
	default:
		if a.IsArrayType(fieldName) {
			return "number"
		}
		return ""
	}
}

func (a *arrayTestSchemaProvider) IsNumericType(fieldName string) bool {
	return !a.IsArrayType(fieldName)
}
