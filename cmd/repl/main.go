package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/h22rana/jsonlogic2sql"
)

func main() {
	fmt.Println("JSON Logic to SQL Transpiler REPL")
	fmt.Println("Type ':help' for commands, ':quit' to exit")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	transpiler := jsonlogic2sql.NewTranspiler()

	// startsWith operator is basically column LIKE 'value%'
	// args[0] is the column name (SQL), args[1] is the pattern (already quoted SQL string)
	transpiler.RegisterOperatorFunc("startsWith", func(op string, args []interface{}) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("startsWith requires exactly 2 arguments")
		}
		column := args[0].(string)
		pattern := args[1].(string)
		// Extract value from quoted string (e.g., "'T'" -> "T")
		if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
			pattern = pattern[1 : len(pattern)-1]
		}
		return fmt.Sprintf("%s LIKE '%s%%'", column, pattern), nil
	})

	// endsWith operator is basically column LIKE '%value'
	transpiler.RegisterOperatorFunc("endsWith", func(op string, args []interface{}) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("endsWith requires exactly 2 arguments")
		}
		column := args[0].(string)
		pattern := args[1].(string)
		// Extract value from quoted string
		if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
			pattern = pattern[1 : len(pattern)-1]
		}
		return fmt.Sprintf("%s LIKE '%%%s'", column, pattern), nil
	})

	// contains operator is basically column LIKE '%value%'
	// Supports: {"contains": [{"var": "field"}, "T"]} or {"contains": [{"var": "field"}, ["T"]]}
	// Also handles reversed: {"contains": ["T", {"var": "field"}]}
	transpiler.RegisterOperatorFunc("contains", func(op string, args []interface{}) (string, error) {
		if len(args) != 2 {
			return "", fmt.Errorf("contains requires exactly 2 arguments")
		}

		var column, pattern string
		arg0Str, arg0IsStr := args[0].(string)
		arg1Str, arg1IsStr := args[1].(string)

		// Helper function to extract value from array string representation like "[T]"
		extractFromArrayString := func(s string) string {
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

		if arg0IsStr && arg1IsStr {
			// Check if either argument is an array string representation
			if strings.HasPrefix(arg1Str, "[") && strings.HasSuffix(arg1Str, "]") {
				// Second arg is an array, extract first element
				column = arg0Str
				pattern = extractFromArrayString(arg1Str)
			} else if strings.HasPrefix(arg0Str, "[") && strings.HasSuffix(arg0Str, "]") {
				// First arg is an array (reversed case)
				column = arg1Str
				pattern = extractFromArrayString(arg0Str)
			} else {
				// Check if arguments are reversed (pattern first, column second)
				arg0Quoted := len(arg0Str) >= 2 && arg0Str[0] == '\'' && arg0Str[len(arg0Str)-1] == '\''
				arg1Quoted := len(arg1Str) >= 2 && arg1Str[0] == '\'' && arg1Str[len(arg1Str)-1] == '\''

				if arg0Quoted && !arg1Quoted {
					// Reversed: pattern is first, column is second
					column = arg1Str
					pattern = arg0Str
				} else {
					// Normal: column is first, pattern is second
					column = arg0Str
					pattern = arg1Str
				}
			}
		} else {
			// Default: first is column, second is pattern
			column = args[0].(string)
			pattern = args[1].(string)
			// Check if pattern is an array string and extract value
			pattern = extractFromArrayString(pattern)
		}

		// Extract value from quoted string pattern
		if len(pattern) >= 2 && pattern[0] == '\'' && pattern[len(pattern)-1] == '\'' {
			pattern = pattern[1 : len(pattern)-1]
		}
		return fmt.Sprintf("%s LIKE '%%%s%%'", column, pattern), nil
	})

	for {
		fmt.Print("jsonlogic> ")
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
			handleCommand(input)
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

func handleCommand(input string) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return
	}

	command := parts[0]

	switch command {
	case ":help":
		showHelp()
	case ":examples":
		showExamples()
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
}

func showHelp() {
	fmt.Println("Available commands:")
	fmt.Println("  :help     - Show this help message")
	fmt.Println("  :examples - Show example JSON Logic expressions")
	fmt.Println("  :clear    - Clear the screen")
	fmt.Println("  :quit     - Exit the REPL")
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
