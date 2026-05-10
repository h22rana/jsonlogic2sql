package operators

import (
	"fmt"
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
)

const (
	arraySourceArgIndex        = 0
	arrayExpressionArgIndex    = 1
	arrayReduceInitialArgIndex = 2

	binaryArrayOperatorArgCount = 2
	reduceOperatorArgCount      = 3
)

func (a *ArrayOperator) arrayLengthSQL(array string) string {
	if a.config == nil {
		return fmt.Sprintf("ARRAY_LENGTH(%s)", array)
	}
	return a.config.ArrayLengthFunc(array)
}

func (a *ArrayOperator) renderMapSQL(alias, transformation, array string) string {
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("arrayMap(%s -> %s, %s)", alias, transformation, array)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		return fmt.Sprintf("ARRAY(SELECT %s FROM UNNEST(%s) AS %s)", transformation, array, alias)
	}
	return fmt.Sprintf("ARRAY(SELECT %s FROM UNNEST(%s) AS %s)", transformation, array, alias)
}

func (a *ArrayOperator) renderFilterSQL(alias, array, condition string) string {
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("arrayFilter(%s -> %s, %s)", alias, condition, array)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		return fmt.Sprintf("ARRAY(SELECT %s FROM UNNEST(%s) AS %s WHERE %s)", alias, array, alias, condition)
	}
	return fmt.Sprintf("ARRAY(SELECT %s FROM UNNEST(%s) AS %s WHERE %s)", alias, array, alias, condition)
}

func (a *ArrayOperator) renderAllSQL(alias, array, condition string) string {
	lengthCheck := a.arrayLengthSQL(array)
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("(%s > 0 AND arrayAll(%s -> %s, %s))", lengthCheck, alias, condition, array)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		return fmt.Sprintf("(%s > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(%s) AS %s WHERE NOT (%s)))", lengthCheck, array, alias, condition)
	}
	return fmt.Sprintf("(%s > 0 AND NOT EXISTS (SELECT 1 FROM UNNEST(%s) AS %s WHERE NOT (%s)))", lengthCheck, array, alias, condition)
}

func (a *ArrayOperator) renderSomeSQL(alias, array, condition string) string {
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("arrayExists(%s -> %s, %s)", alias, condition, array)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		return fmt.Sprintf("EXISTS (SELECT 1 FROM UNNEST(%s) AS %s WHERE %s)", array, alias, condition)
	}
	return fmt.Sprintf("EXISTS (SELECT 1 FROM UNNEST(%s) AS %s WHERE %s)", array, alias, condition)
}

func (a *ArrayOperator) renderNoneSQL(alias, array, condition string) string {
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("NOT arrayExists(%s -> %s, %s)", alias, condition, array)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM UNNEST(%s) AS %s WHERE %s)", array, alias, condition)
	}
	return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM UNNEST(%s) AS %s WHERE %s)", array, alias, condition)
}

func (a *ArrayOperator) renderMergeSQL(arrays []string) (string, error) {
	if len(arrays) == 0 {
		return a.emptyArrayLiteralSQL()
	}

	switch d := a.getDialect(); d {
	case dialect.DialectPostgreSQL:
		if len(arrays) == 1 {
			return arrays[0], nil
		}
		return fmt.Sprintf("(%s)", strings.Join(arrays, " || ")), nil
	case dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectDuckDB:
		return fmt.Sprintf("ARRAY_CONCAT(%s)", strings.Join(arrays, ", ")), nil
	case dialect.DialectClickHouse:
		return fmt.Sprintf("arrayConcat(%s)", strings.Join(arrays, ", ")), nil
	case dialect.DialectUnspecified:
		return "", fmt.Errorf("merge: dialect not specified")
	default:
		return "", fmt.Errorf("merge: unsupported dialect %s", d)
	}
}
