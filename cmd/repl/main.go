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
		result, err := jsonlogic2sql.Transpile(input)
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
