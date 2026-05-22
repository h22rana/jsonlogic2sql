package parser

import (
	"strings"

	"github.com/h22rana/jsonlogic2sql/internal/dialect"
	tperrors "github.com/h22rana/jsonlogic2sql/internal/errors"
)

func (p *Parser) rejectUnsupportedPostgreSQLEmptyArrayResult(res expressionResult) error {
	if !p.isUnsupportedPostgreSQLEmptyArrayResult(res) {
		return nil
	}
	return tperrors.New(tperrors.ErrInvalidArgument, "", "$",
		"empty PostgreSQL array literals require an explicit element type")
}

func (p *Parser) isUnsupportedPostgreSQLEmptyArrayResult(res expressionResult) bool {
	return p.config.GetDialect() == dialect.DialectPostgreSQL && containsBarePostgreSQLEmptyArrayLiteral(res.SQL)
}

func containsBarePostgreSQLEmptyArrayLiteral(sql string) bool {
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false

	for i := 0; i < len(sql); i++ {
		ch := sql[i]

		switch {
		case !inSingleQuote && !inDoubleQuote && !inBacktick && i+1 < len(sql) && ch == '-' && sql[i+1] == '-':
			i = skipSQLLineComment(sql, i+2)
			continue
		case !inSingleQuote && !inDoubleQuote && !inBacktick && i+1 < len(sql) && ch == '/' && sql[i+1] == '*':
			i = skipSQLBlockComment(sql, i+2)
			continue
		case !inSingleQuote && !inDoubleQuote && !inBacktick && ch == '$':
			if end, ok := skipPostgreSQLDollarQuotedLiteral(sql, i); ok {
				i = end
				continue
			}
		case inSingleQuote:
			if ch == '\'' {
				if i+1 < len(sql) && sql[i+1] == '\'' {
					i++
					continue
				}
				inSingleQuote = false
			}
			continue
		case inDoubleQuote:
			if ch == '"' {
				if i+1 < len(sql) && sql[i+1] == '"' {
					i++
					continue
				}
				inDoubleQuote = false
			}
			continue
		case inBacktick:
			if ch == '`' {
				inBacktick = false
			}
			continue
		case ch == '\'':
			inSingleQuote = true
			continue
		case ch == '"':
			inDoubleQuote = true
			continue
		case ch == '`':
			inBacktick = true
			continue
		}

		if i+len("ARRAY[]") > len(sql) || !strings.EqualFold(sql[i:i+len("ARRAY[]")], "ARRAY[]") {
			continue
		}
		if i > 0 && isSQLIdentifierChar(sql[i-1]) {
			continue
		}
		if i+len("ARRAY[]") < len(sql) && isSQLIdentifierChar(sql[i+len("ARRAY[]")]) {
			continue
		}
		if isTypedPostgreSQLEmptyArrayLiteral(sql, i) {
			continue
		}
		return true
	}

	return false
}

func skipSQLLineComment(sql string, start int) int {
	for i := start; i < len(sql); i++ {
		if sql[i] == '\n' || sql[i] == '\r' {
			return i
		}
	}
	return len(sql) - 1
}

func skipSQLBlockComment(sql string, start int) int {
	for i := start; i+1 < len(sql); i++ {
		if sql[i] == '*' && sql[i+1] == '/' {
			return i + 1
		}
	}
	return len(sql) - 1
}

func skipPostgreSQLDollarQuotedLiteral(sql string, start int) (int, bool) {
	delimiter, ok := dollarQuoteDelimiterAt(sql, start)
	if !ok {
		return 0, false
	}
	contentStart := start + len(delimiter)
	closingOffset := strings.Index(sql[contentStart:], delimiter)
	if closingOffset < 0 {
		return len(sql) - 1, true
	}
	return contentStart + closingOffset + len(delimiter) - 1, true
}

func dollarQuoteDelimiterAt(sql string, start int) (string, bool) {
	if start >= len(sql) || sql[start] != '$' || start+1 >= len(sql) {
		return "", false
	}
	if sql[start+1] == '$' {
		return "$$", true
	}
	if !isDollarQuoteTagFirstChar(sql[start+1]) {
		return "", false
	}
	for i := start + 2; i < len(sql); i++ {
		if sql[i] == '$' {
			return sql[start : i+1], true
		}
		if !isDollarQuoteTagChar(sql[i]) {
			return "", false
		}
	}
	return "", false
}

func isDollarQuoteTagFirstChar(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isDollarQuoteTagChar(ch byte) bool {
	return isDollarQuoteTagFirstChar(ch) || (ch >= '0' && ch <= '9')
}

func isTypedPostgreSQLEmptyArrayLiteral(sql string, start int) bool {
	end := start + len("ARRAY[]")
	next := skipSQLSpaces(sql, end)
	if strings.HasPrefix(sql[next:], "::") {
		return true
	}
	return isCastPostgreSQLEmptyArrayLiteral(sql, start, end)
}

func isCastPostgreSQLEmptyArrayLiteral(sql string, start, end int) bool {
	open := skipSQLSpacesBackward(sql, start-1)
	if open < 0 || sql[open] != '(' {
		return false
	}

	wordEnd := skipSQLSpacesBackward(sql, open-1) + 1
	if wordEnd <= 0 {
		return false
	}
	wordStart := wordEnd - 1
	for wordStart >= 0 && isSQLIdentifierChar(sql[wordStart]) {
		wordStart--
	}
	if !strings.EqualFold(sql[wordStart+1:wordEnd], "CAST") {
		return false
	}

	return hasSQLWordAt(sql, skipSQLSpaces(sql, end), "AS")
}

func skipSQLSpaces(sql string, pos int) int {
	for pos < len(sql) && (sql[pos] == ' ' || sql[pos] == '\t' || sql[pos] == '\n' || sql[pos] == '\r') {
		pos++
	}
	return pos
}

func skipSQLSpacesBackward(sql string, pos int) int {
	for pos >= 0 && (sql[pos] == ' ' || sql[pos] == '\t' || sql[pos] == '\n' || sql[pos] == '\r') {
		pos--
	}
	return pos
}

func hasSQLWordAt(sql string, pos int, word string) bool {
	if pos+len(word) > len(sql) || !strings.EqualFold(sql[pos:pos+len(word)], word) {
		return false
	}
	if pos > 0 && isSQLIdentifierChar(sql[pos-1]) {
		return false
	}
	if pos+len(word) < len(sql) && isSQLIdentifierChar(sql[pos+len(word)]) {
		return false
	}
	return true
}

func isSQLIdentifierChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9') ||
		ch == '_'
}
