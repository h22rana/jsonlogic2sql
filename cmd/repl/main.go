package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/h22rana/jsonlogic2sql"
)

// escapeLikePattern escapes special characters for SQL LIKE patterns.
// Escapes: single quotes ('), percent (%), and underscore (_)
// For BigQuery/Spanner, use backslash escaping for LIKE wildcards.
func escapeLikePattern(pattern string) string {
	// First escape single quotes (SQL string escaping)
	pattern = strings.ReplaceAll(pattern, "'", "''")
	// Then escape LIKE wildcards (backslash escaping for BigQuery/Spanner)
	pattern = strings.ReplaceAll(pattern, "\\", "\\\\") // Escape existing backslashes first
	pattern = strings.ReplaceAll(pattern, "%", "\\%")
	pattern = strings.ReplaceAll(pattern, "_", "\\_")
	return pattern
}

// extractFromArrayString extracts value from array string representation like "[T]".
func extractFromArrayString(s string) string {
	// If it's an array representation like "[T]", extract "T"
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		inner := s[1 : len(s)-1] // Remove "[" and "]"
		// Remove quotes if present
		if len(inner) >= 2 && inner[0] == '\'' && inner[len(inner)-1] == '\'' {
			return inner[1 : len(inner)-1]
		}
		return inner
	}
	return s
}

// parseContainsArgs parses the arguments for contains/!contains operators.
// Returns the column and pattern to use in the LIKE clause.
func parseContainsArgs(args []interface{}) (column, pattern string) {
	arg0Str, arg0IsStr := args[0].(string)
	arg1Str, arg1IsStr := args[1].(string)

	if arg0IsStr && arg1IsStr {
		// Check if either argument is an array string representation.
		if strings.HasPrefix(arg1Str, "[") && strings.HasSuffix(arg1Str, "]") {
			// Second arg is an array, extract first element.
			column = arg0Str
			pattern = extractFromArrayString(arg1Str)
		} else if strings.HasPrefix(arg0Str, "[") && strings.HasSuffix(arg0Str, "]") {
			// First arg is an array (reversed case).
			column = arg1Str
			pattern = extractFromArrayString(arg0Str)
		} else {
			// Check if arguments are reversed (pattern first, column second).
			arg0Quoted := len(arg0Str) >= 2 && arg0Str[0] == '\'' && arg0Str[len(arg0Str)-1] == '\''
			arg1Quoted := len(arg1Str) >= 2 && arg1Str[0] == '\'' && arg1Str[len(arg1Str)-1] == '\''

			if arg0Quoted && !arg1Quoted {
				// Reversed: pattern is first, column is second.
				column = arg1Str
				pattern = arg0Str
			} else {
				// Normal: column is first, pattern is second.
				column = arg0Str
				pattern = arg1Str
			}
		}
	} else {
		// Default: first is column, second is pattern.
		column = args[0].(string)
		pattern = args[1].(string)
		// Check if pattern is an array string and extract value.
		pattern = extractFromArrayString(pattern)
	}

	// Extract value from quoted string pattern.
	if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
		pattern = pattern[1 : len(pattern)-1]
	}
	return column, pattern
}

// dialects defines the available SQL dialects with their display names.
var dialects = []struct {
	dialect jsonlogic2sql.Dialect
	name    string
}{
	{jsonlogic2sql.DialectBigQuery, "BigQuery"},
	{jsonlogic2sql.DialectSpanner, "Spanner"},
	{jsonlogic2sql.DialectPostgreSQL, "PostgreSQL"},
	{jsonlogic2sql.DialectDuckDB, "DuckDB"},
	{jsonlogic2sql.DialectClickHouse, "ClickHouse"},
}

// currentDialect holds the currently selected dialect.
var currentDialect jsonlogic2sql.Dialect

// selectDialect prompts the user to select a SQL dialect.
func selectDialect(scanner *bufio.Scanner) jsonlogic2sql.Dialect {
	fmt.Println("Select SQL dialect:")
	for i, d := range dialects {
		fmt.Printf("  %d. %s\n", i+1, d.name)
	}
	fmt.Print("\nEnter choice [1-5] (default: 1 for BigQuery): ")

	if !scanner.Scan() {
		return jsonlogic2sql.DialectBigQuery
	}

	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return jsonlogic2sql.DialectBigQuery
	}

	var choice int
	if _, err := fmt.Sscanf(input, "%d", &choice); err != nil || choice < 1 || choice > len(dialects) {
		fmt.Println("Invalid choice, defaulting to BigQuery")
		return jsonlogic2sql.DialectBigQuery
	}

	return dialects[choice-1].dialect
}

// getDialectName returns the display name for a dialect.
func getDialectName(d jsonlogic2sql.Dialect) string {
	for _, dialect := range dialects {
		if dialect.dialect == d {
			return dialect.name
		}
	}
	return "Unknown"
}

func main() {
	fmt.Println("JSON Logic to SQL Transpiler REPL")
	fmt.Println("==================================")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)

	// Prompt user to select dialect
	currentDialect = selectDialect(scanner)
	fmt.Printf("\nUsing %s dialect\n", getDialectName(currentDialect))
	fmt.Println("Type ':help' for commands, ':quit' to exit")
	fmt.Println()

	transpiler, err := jsonlogic2sql.NewTranspilerWithConfig(&jsonlogic2sql.TranspilerConfig{
		Dialect: currentDialect,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create transpiler: %v\n", err)
		os.Exit(1)
	}

	// Register all custom operators
	registerCustomOperators(transpiler)

	for {
		fmt.Printf("[%s] jsonlogic> ", getDialectName(currentDialect))
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())

		// Handle empty input
		if input == "" {
			continue
		}

		// Handle commands
		if strings.HasPrefix(input, ":") {
			newTranspiler := handleCommand(input, transpiler, scanner)
			if newTranspiler != nil {
				transpiler = newTranspiler
			}
			continue
		}

		// Process JSON Logic input
		result, err := transpiler.Transpile(input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
		} else {
			fmt.Printf("SQL: %s\n", result)
		}
		fmt.Println()
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
}

func handleCommand(input string, _ *jsonlogic2sql.Transpiler, scanner *bufio.Scanner) *jsonlogic2sql.Transpiler {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}

	command := parts[0]

	switch command {
	case ":help":
		showHelp()
	case ":examples":
		showExamples()
	case ":dialect":
		return handleDialectChange(scanner)
	case ":quit", ":exit":
		fmt.Println("Goodbye!")
		os.Exit(0)
	case ":clear":
		// Clear screen (works on most terminals)
		fmt.Print("\033[2J\033[H")
	default:
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Type ':help' for available commands")
	}
	return nil
}

// handleDialectChange handles the :dialect command to switch SQL dialects.
func handleDialectChange(scanner *bufio.Scanner) *jsonlogic2sql.Transpiler {
	fmt.Println()
	newDialect := selectDialect(scanner)
	currentDialect = newDialect

	transpiler, err := jsonlogic2sql.NewTranspilerWithConfig(&jsonlogic2sql.TranspilerConfig{
		Dialect: currentDialect,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create transpiler: %v\n", err)
		return nil
	}

	// Re-register all custom operators for the new transpiler
	registerCustomOperators(transpiler)

	fmt.Printf("\nSwitched to %s dialect\n\n", getDialectName(currentDialect))
	return transpiler
}

// registerCustomOperators registers all custom operators for the REPL.
//
//nolint:funlen // This function registers many operators and is long by design.
func registerCustomOperators(transpiler *jsonlogic2sql.Transpiler) {
	// ========================================================================
	// Basic String Pattern Matching Operators
	// ========================================================================

	// startsWith operator is basically column LIKE 'value%'.
	// args[0] is the column name (SQL), args[1] is the pattern (already quoted SQL string).
	_ = transpiler.RegisterOperatorFunc("startsWith", func(_ string, args []any) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("startsWith requires exactly 2 arguments")
		}
		column := args[0].(string)
		pattern := args[1].(string)
		// Extract value from quoted string (e.g., "'T'" -> "T")
		if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
			pattern = pattern[1 : len(pattern)-1]
		}
		return fmt.Sprintf("%s LIKE '%s%%'", column, escapeLikePattern(pattern)), nil
	})

	// !startsWith operator is basically column NOT LIKE 'value%'.
	_ = transpiler.RegisterOperatorFunc("!startsWith", func(_ string, args []any) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("!startsWith requires exactly 2 arguments")
		}
		column := args[0].(string)
		pattern := args[1].(string)
		// Extract value from quoted string (e.g., "'T'" -> "T")
		if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
			pattern = pattern[1 : len(pattern)-1]
		}
		return fmt.Sprintf("%s NOT LIKE '%s%%'", column, escapeLikePattern(pattern)), nil
	})

	// endsWith operator is basically column LIKE '%value'.
	_ = transpiler.RegisterOperatorFunc("endsWith", func(_ string, args []any) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("endsWith requires exactly 2 arguments")
		}
		column := args[0].(string)
		pattern := args[1].(string)
		// Extract value from quoted string
		if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
			pattern = pattern[1 : len(pattern)-1]
		}
		return fmt.Sprintf("%s LIKE '%%%s'", column, escapeLikePattern(pattern)), nil
	})

	// !endsWith operator is basically column NOT LIKE '%value'.
	_ = transpiler.RegisterOperatorFunc("!endsWith", func(_ string, args []any) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("!endsWith requires exactly 2 arguments")
		}
		column := args[0].(string)
		pattern := args[1].(string)
		// Extract value from quoted string (e.g., "'T'" -> "T")
		if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
			pattern = pattern[1 : len(pattern)-1]
		}
		return fmt.Sprintf("%s NOT LIKE '%%%s'", column, escapeLikePattern(pattern)), nil
	})

	// contains operator is basically column LIKE '%value%'.
	// Supports: {"contains": [{"var": "field"}, "T"]} or {"contains": [{"var": "field"}, ["T"]]}.
	// Also handles reversed: {"contains": ["T", {"var": "field"}]}.
	_ = transpiler.RegisterOperatorFunc("contains", func(_ string, args []any) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("contains requires exactly 2 arguments")
		}
		column, pattern := parseContainsArgs(args)
		return fmt.Sprintf("%s LIKE '%%%s%%'", column, escapeLikePattern(pattern)), nil
	})

	// !contains operator is basically column NOT LIKE '%value%'.
	_ = transpiler.RegisterOperatorFunc("!contains", func(_ string, args []any) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("!contains requires exactly 2 arguments")
		}
		column, pattern := parseContainsArgs(args)
		return fmt.Sprintf("%s NOT LIKE '%%%s%%'", column, escapeLikePattern(pattern)), nil
	})

	// ========================================================================
	// String Transformation Operators
	// ========================================================================

	// normalizeNFKC operator is basically NORMALIZE(column, 'NFKC').
	_ = transpiler.RegisterOperatorFunc("normalizeNFKC", func(_ string, args []any) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("normalizeNFKC requires exactly 1 argument")
		}
		column := args[0].(string)
		return fmt.Sprintf("NORMALIZE(%s, 'NFKC')", column), nil
	})

	// normalizeWaveDash operator converts wave dash variants to ASCII tilde
	// U+301C (〜) wave dash → ~
	// U+FF5E (～) fullwidth tilde → ~
	// SQL: REGEXP_REPLACE(column, '[〜～]', '~')
	_ = transpiler.RegisterOperatorFunc("normalizeWaveDash", func(_ string, args []any) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("normalizeWaveDash requires exactly 1 argument")
		}
		column := args[0].(string)
		// REGEXP_REPLACE to convert both wave dash (U+301C) and fullwidth tilde (U+FF5E) to ASCII tilde
		return fmt.Sprintf("REGEXP_REPLACE(%s, '[〜～]', '~')", column), nil
	})

	// toLower operator is basically LOWER(column).
	_ = transpiler.RegisterOperatorFunc("toLower", func(_ string, args []any) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("toLower requires exactly 1 argument")
		}
		column := args[0].(string)
		return fmt.Sprintf("LOWER(%s)", column), nil
	})

	// toUpper operator is basically UPPER(column).
	_ = transpiler.RegisterOperatorFunc("toUpper", func(_ string, args []any) (string, error) {
		if len(args) != 1 {
			return "", fmt.Errorf("toUpper requires exactly 1 argument")
		}
		column := args[0].(string)
		return fmt.Sprintf("UPPER(%s)", column), nil
	})

	// ========================================================================
	// Dialect-Aware Custom Operators
	// ========================================================================
	// These operators demonstrate how to register operators that generate
	// different SQL based on the target dialect (BigQuery, Spanner, PostgreSQL, DuckDB, or ClickHouse).

	// currentTimestamp operator returns the current timestamp.
	// BigQuery: CURRENT_TIMESTAMP()
	// Spanner: CURRENT_TIMESTAMP()
	// PostgreSQL: CURRENT_TIMESTAMP
	// DuckDB: CURRENT_TIMESTAMP
	// ClickHouse: now()
	// Example: {"==": [{"currentTimestamp": []}, {"var": "created_at"}]}
	_ = transpiler.RegisterDialectAwareOperatorFunc("currentTimestamp",
		func(_ string, args []any, dialect jsonlogic2sql.Dialect) (string, error) {
			if len(args) != 0 {
				return "", fmt.Errorf("currentTimestamp takes no arguments")
			}
			switch dialect {
			case jsonlogic2sql.DialectBigQuery, jsonlogic2sql.DialectSpanner:
				return "CURRENT_TIMESTAMP()", nil
			case jsonlogic2sql.DialectPostgreSQL, jsonlogic2sql.DialectDuckDB:
				return "CURRENT_TIMESTAMP", nil
			case jsonlogic2sql.DialectClickHouse:
				return "now()", nil
			default:
				return "", fmt.Errorf("unsupported dialect: %v", dialect)
			}
		})

	// dateDiff operator calculates the difference between two dates (in days).
	// BigQuery: DATE_DIFF(date1, date2, DAY)
	// Spanner: DATE_DIFF(date1, date2, DAY)
	// PostgreSQL: (date1 - date2) -- returns integer days
	// DuckDB: date_diff('day', date2, date1) -- note: part first, then dates
	// ClickHouse: dateDiff('day', date2, date1) -- same as DuckDB
	// Example: {">": [{"dateDiff": [{"var": "end_date"}, {"var": "start_date"}]}, 30]}
	_ = transpiler.RegisterDialectAwareOperatorFunc("dateDiff",
		func(_ string, args []any, dialect jsonlogic2sql.Dialect) (string, error) {
			if len(args) != 2 {
				return "", fmt.Errorf("dateDiff requires exactly 2 arguments")
			}
			date1 := args[0].(string)
			date2 := args[1].(string)
			switch dialect {
			case jsonlogic2sql.DialectBigQuery, jsonlogic2sql.DialectSpanner:
				return fmt.Sprintf("DATE_DIFF(%s, %s, DAY)", date1, date2), nil
			case jsonlogic2sql.DialectPostgreSQL:
				// PostgreSQL: subtracting dates returns integer days
				return fmt.Sprintf("(%s - %s)", date1, date2), nil
			case jsonlogic2sql.DialectDuckDB, jsonlogic2sql.DialectClickHouse:
				// DuckDB/ClickHouse: dateDiff('part', start, end) - note different argument order
				return fmt.Sprintf("dateDiff('day', %s, %s)", date2, date1), nil
			default:
				return "", fmt.Errorf("unsupported dialect: %v", dialect)
			}
		})

	// arrayLength operator returns the length of an array.
	// BigQuery: ARRAY_LENGTH(array)
	// Spanner: ARRAY_LENGTH(array)
	// PostgreSQL: CARDINALITY(array)
	// DuckDB: ARRAY_LENGTH(array)
	// ClickHouse: length(array)
	// Example: {">": [{"arrayLength": [{"var": "tags"}]}, 0]}
	_ = transpiler.RegisterDialectAwareOperatorFunc("arrayLength",
		func(_ string, args []any, dialect jsonlogic2sql.Dialect) (string, error) {
			if len(args) != 1 {
				return "", fmt.Errorf("arrayLength requires exactly 1 argument")
			}
			arr := args[0].(string)
			switch dialect {
			case jsonlogic2sql.DialectBigQuery, jsonlogic2sql.DialectSpanner, jsonlogic2sql.DialectDuckDB:
				return fmt.Sprintf("ARRAY_LENGTH(%s)", arr), nil
			case jsonlogic2sql.DialectPostgreSQL:
				return fmt.Sprintf("CARDINALITY(%s)", arr), nil
			case jsonlogic2sql.DialectClickHouse:
				return fmt.Sprintf("length(%s)", arr), nil
			default:
				return "", fmt.Errorf("unsupported dialect: %v", dialect)
			}
		})

	// regexpContains operator checks if a string matches a regex pattern.
	// BigQuery: REGEXP_CONTAINS(string, pattern)
	// Spanner: REGEXP_CONTAINS(string, pattern)
	// PostgreSQL: string ~ pattern
	// DuckDB: regexp_matches(string, pattern)
	// ClickHouse: match(string, pattern)
	// Example: {"regexpContains": [{"var": "email"}, "^[a-z]+@example\\.com$"]}
	_ = transpiler.RegisterDialectAwareOperatorFunc("regexpContains",
		func(_ string, args []any, dialect jsonlogic2sql.Dialect) (string, error) {
			if len(args) != 2 {
				return "", fmt.Errorf("regexpContains requires exactly 2 arguments")
			}
			str := args[0].(string)
			pattern := args[1].(string)
			switch dialect {
			case jsonlogic2sql.DialectBigQuery:
				return fmt.Sprintf("REGEXP_CONTAINS(%s, r%s)", str, pattern), nil
			case jsonlogic2sql.DialectSpanner:
				return fmt.Sprintf("REGEXP_CONTAINS(%s, %s)", str, pattern), nil
			case jsonlogic2sql.DialectPostgreSQL:
				return fmt.Sprintf("%s ~ %s", str, pattern), nil
			case jsonlogic2sql.DialectDuckDB:
				return fmt.Sprintf("regexp_matches(%s, %s)", str, pattern), nil
			case jsonlogic2sql.DialectClickHouse:
				return fmt.Sprintf("match(%s, %s)", str, pattern), nil
			default:
				return "", fmt.Errorf("unsupported dialect: %v", dialect)
			}
		})

	// safeDivide operator performs division that returns NULL on division by zero.
	// This demonstrates a real dialect difference:
	// BigQuery: SAFE_DIVIDE(numerator, denominator) - built-in function
	// Spanner: CASE WHEN denominator = 0 THEN NULL ELSE numerator / denominator END
	// PostgreSQL: CASE WHEN denominator = 0 THEN NULL ELSE numerator / denominator END
	// DuckDB: CASE WHEN denominator = 0 THEN NULL ELSE numerator / denominator END
	// ClickHouse: if(denominator = 0, NULL, numerator / denominator)
	// Example: {"safeDivide": [{"var": "total"}, {"var": "count"}]}
	_ = transpiler.RegisterDialectAwareOperatorFunc("safeDivide",
		func(_ string, args []any, dialect jsonlogic2sql.Dialect) (string, error) {
			if len(args) != 2 {
				return "", fmt.Errorf("safeDivide requires exactly 2 arguments")
			}
			numerator := args[0].(string)
			denominator := args[1].(string)
			switch dialect {
			case jsonlogic2sql.DialectBigQuery:
				// BigQuery has built-in SAFE_DIVIDE that returns NULL on division by zero
				return fmt.Sprintf("SAFE_DIVIDE(%s, %s)", numerator, denominator), nil
			case jsonlogic2sql.DialectSpanner, jsonlogic2sql.DialectPostgreSQL, jsonlogic2sql.DialectDuckDB:
				// Spanner, PostgreSQL, and DuckDB don't have SAFE_DIVIDE, use CASE expression
				return fmt.Sprintf("CASE WHEN %s = 0 THEN NULL ELSE %s / %s END", denominator, numerator, denominator), nil
			case jsonlogic2sql.DialectClickHouse:
				// ClickHouse uses if() function for conditional expressions
				return fmt.Sprintf("if(%s = 0, NULL, %s / %s)", denominator, numerator, denominator), nil
			default:
				return "", fmt.Errorf("unsupported dialect: %v", dialect)
			}
		})
}

func showHelp() {
	fmt.Println("Available commands:")
	fmt.Println("  :help     - Show this help message")
	fmt.Println("  :examples - Show example JSON Logic expressions")
	fmt.Println("  :dialect  - Change the SQL dialect")
	fmt.Println("  :clear    - Clear the screen")
	fmt.Println("  :quit     - Exit the REPL")
	fmt.Println()
	fmt.Printf("Current dialect: %s\n", getDialectName(currentDialect))
	fmt.Println()
	fmt.Println("Enter JSON Logic expressions to convert them to SQL WHERE clauses.")
	fmt.Println("Example: {\">\": [{\"var\": \"amount\"}, 1000]}")
}

func showExamples() {
	examples := []struct {
		name string
		json string
		sql  string
	}{
		{
			name: "Simple Comparison",
			json: `{">": [{"var": "amount"}, 1000]}`,
			sql:  "WHERE amount > 1000",
		},
		{
			name: "Multiple Conditions (AND)",
			json: `{"and": [{">": [{"var": "amount"}, 5000]}, {"==": [{"var": "status"}, "pending"]}]}`,
			sql:  "WHERE (amount > 5000 AND status = 'pending')",
		},
		{
			name: "Multiple Conditions (OR)",
			json: `{"or": [{">=": [{"var": "failedAttempts"}, 5]}, {"in": [{"var": "country"}, ["CN", "RU"]]}]}`,
			sql:  "WHERE (failedAttempts >= 5 OR country IN ('CN', 'RU'))",
		},
		{
			name: "Nested Conditions",
			json: `{"and": [{">": [{"var": "transaction.amount"}, 10000]}, {"or": [{"==": [{"var": "user.verified"}, false]}, {"<": [{"var": "user.accountAgeDays"}, 7]}]}]}`,
			sql:  "WHERE (transaction_amount > 10000 AND (user_verified = FALSE OR user_accountAgeDays < 7))",
		},
		{
			name: "IF Statement",
			json: `{"if": [{">": [{"var": "age"}, 18]}, "adult", "minor"]}`,
			sql:  "WHERE CASE WHEN age > 18 THEN 'adult' ELSE 'minor' END",
		},
		{
			name: "Missing Field Check",
			json: `{"missing": ["field"]}`,
			sql:  "WHERE field IS NULL",
		},
		{
			name: "Missing Some Fields",
			json: `{"missing_some": [1, ["field1", "field2"]]}`,
			sql:  "WHERE (field1 IS NULL + field2 IS NULL) >= 1",
		},
		{
			name: "NOT Operation",
			json: `{"!": [{"==": [{"var": "verified"}, true]}]}`,
			sql:  "WHERE NOT (verified = TRUE)",
		},
	}

	fmt.Println("Example JSON Logic expressions:")
	fmt.Println()

	for i, example := range examples {
		fmt.Printf("%d. %s\n", i+1, example.name)
		fmt.Printf("   JSON: %s\n", example.json)
		fmt.Printf("   SQL:  %s\n", example.sql)
		fmt.Println()
	}
}
