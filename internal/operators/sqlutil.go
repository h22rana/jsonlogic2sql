package operators

import "strings"

// StripRedundantOuterParens removes enclosing parentheses that wrap the whole
// SQL expression. Call this only at SQL grammar boundaries where a surrounding
// construct already delimits the expression, such as function arguments,
// CASE WHEN conditions, or NOT operands.
func StripRedundantOuterParens(sql string) string {
	trimmed := strings.TrimSpace(sql)
	for hasRedundantOuterParens(trimmed) {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	return trimmed
}

func hasRedundantOuterParens(sql string) bool {
	if len(sql) < 2 || sql[0] != '(' || sql[len(sql)-1] != ')' {
		return false
	}

	depth := 0
	inString := false
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		if ch == '\'' {
			if inString && i+1 < len(sql) && sql[i+1] == '\'' {
				i++
				continue
			}
			inString = !inString
			continue
		}
		if inString {
			continue
		}

		switch ch {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i != len(sql)-1 {
				return false
			}
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0 && !inString
}
