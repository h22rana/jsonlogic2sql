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

func (a *ArrayOperator) unnestSourceSQL(array, alias string) string {
	if a.getDialect() == dialect.DialectDuckDB {
		return fmt.Sprintf("UNNEST(%s) AS %s(%s)", array, alias, alias)
	}
	return fmt.Sprintf("UNNEST(%s) AS %s", array, alias)
}

func (a *ArrayOperator) renderMapSQL(alias, transformation, array string) string {
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("arrayMap(%s -> %s, %s)", alias, transformation, array)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		return fmt.Sprintf("ARRAY(SELECT %s FROM %s)", transformation, a.unnestSourceSQL(array, alias))
	}
	return fmt.Sprintf("ARRAY(SELECT %s FROM %s)", transformation, a.unnestSourceSQL(array, alias))
}

func (a *ArrayOperator) renderFilterSQL(alias, array, condition string) string {
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("arrayFilter(%s -> %s, %s)", alias, condition, array)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		return fmt.Sprintf("ARRAY(SELECT %s FROM %s WHERE %s)", alias, a.unnestSourceSQL(array, alias), condition)
	}
	return fmt.Sprintf("ARRAY(SELECT %s FROM %s WHERE %s)", alias, a.unnestSourceSQL(array, alias), condition)
}

func (a *ArrayOperator) renderAllSQL(alias, array, condition string) string {
	lengthCheck := a.arrayLengthSQL(array)
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("(%s > 0 AND arrayAll(%s -> %s, %s))", lengthCheck, alias, condition, array)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		return fmt.Sprintf("(%s > 0 AND NOT EXISTS (SELECT 1 FROM %s WHERE NOT (%s)))", lengthCheck, a.unnestSourceSQL(array, alias), condition)
	}
	return fmt.Sprintf("(%s > 0 AND NOT EXISTS (SELECT 1 FROM %s WHERE NOT (%s)))", lengthCheck, a.unnestSourceSQL(array, alias), condition)
}

func (a *ArrayOperator) renderSomeSQL(alias, array, condition string) string {
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("arrayExists(%s -> %s, %s)", alias, condition, array)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		return fmt.Sprintf("EXISTS (SELECT 1 FROM %s WHERE %s)", a.unnestSourceSQL(array, alias), condition)
	}
	return fmt.Sprintf("EXISTS (SELECT 1 FROM %s WHERE %s)", a.unnestSourceSQL(array, alias), condition)
}

func (a *ArrayOperator) renderNoneSQL(alias, array, condition string) string {
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		return fmt.Sprintf("NOT arrayExists(%s -> %s, %s)", alias, condition, array)
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL, dialect.DialectDuckDB:
		return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM %s WHERE %s)", a.unnestSourceSQL(array, alias), condition)
	}
	return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM %s WHERE %s)", a.unnestSourceSQL(array, alias), condition)
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

func (a *ArrayOperator) renderGeneralReduceSQL(alias, array, reducer, initial string, initialType ExpressionType) (string, error) {
	switch a.getDialect() {
	case dialect.DialectClickHouse:
		initial = clickHouseArrayFoldInitialSQL(initial, initialType)
		return fmt.Sprintf("arrayFold((acc, %s) -> %s, %s, %s)", alias, reducer, array, initial), nil
	case dialect.DialectDuckDB:
		if containsSQLKeywordOutsideQuotedRegions(reducer, "SELECT") {
			return "", errUnsupportedDuckDBReduceSubquery
		}
		return fmt.Sprintf("list_reduce(%s, lambda acc, %s : %s, %s)", array, alias, reducer, initial), nil
	case dialect.DialectUnspecified, dialect.DialectBigQuery, dialect.DialectSpanner, dialect.DialectPostgreSQL:
		return "", errUnsupportedGeneralReduce
	}
	return "", errUnsupportedGeneralReduce
}

func clickHouseArrayFoldInitialSQL(initial string, initialType ExpressionType) string {
	if initialType == ExpressionTypeNumber {
		return fmt.Sprintf("toFloat64(%s)", initial)
	}
	return initial
}

func containsSQLKeywordOutsideQuotedRegions(sql, keyword string) bool {
	keyword = strings.ToUpper(keyword)
	for i := 0; i < len(sql); i++ {
		switch sql[i] {
		case '\'':
			i = skipSQLQuotedRegion(sql, i, '\'')
			continue
		case '"':
			i = skipSQLQuotedRegion(sql, i, '"')
			continue
		case '`':
			i = skipSQLQuotedRegion(sql, i, '`')
			continue
		case '-':
			if i+1 < len(sql) && sql[i+1] == '-' {
				i = skipSQLLineComment(sql, i)
				continue
			}
		case '/':
			if i+1 < len(sql) && sql[i+1] == '*' {
				i = skipSQLBlockComment(sql, i)
				continue
			}
		}
		if sqlKeywordAt(sql, keyword, i) {
			return true
		}
	}
	return false
}

func sqlKeywordAt(sql, keyword string, i int) bool {
	if i+len(keyword) > len(sql) || strings.ToUpper(sql[i:i+len(keyword)]) != keyword {
		return false
	}
	return sqlKeywordBoundary(sql, i-1) && sqlKeywordBoundary(sql, i+len(keyword))
}

func sqlKeywordBoundary(sql string, i int) bool {
	if i < 0 || i >= len(sql) {
		return true
	}
	ch := sql[i]
	return (ch < 'a' || ch > 'z') &&
		(ch < 'A' || ch > 'Z') &&
		(ch < '0' || ch > '9') &&
		ch != '_'
}

func skipSQLQuotedRegion(sql string, start int, quote byte) int {
	for i := start + 1; i < len(sql); i++ {
		if sql[i] != quote {
			continue
		}
		if i+1 < len(sql) && sql[i+1] == quote {
			i++
			continue
		}
		return i
	}
	return len(sql) - 1
}

func skipSQLLineComment(sql string, start int) int {
	for i := start + 2; i < len(sql); i++ {
		if sql[i] == '\n' || sql[i] == '\r' {
			return i
		}
	}
	return len(sql) - 1
}

func skipSQLBlockComment(sql string, start int) int {
	for i := start + 2; i+1 < len(sql); i++ {
		if sql[i] == '*' && sql[i+1] == '/' {
			return i + 1
		}
	}
	return len(sql) - 1
}
