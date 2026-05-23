package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/h22rana/jsonlogic2sql"
)

const (
	modeCondition = "condition"
	modeValue     = "value"
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

// unescapeSQLString decodes a SQL string literal back to its raw value.
// For quoted inputs (e.g., "'it”s'"), it strips the outer single quotes and
// unescapes ” → '. Unquoted inputs pass through unchanged.
func unescapeSQLString(s string) string {
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		s = s[1 : len(s)-1]
		s = strings.ReplaceAll(s, "''", "'")
	}
	return s
}

func isArrayString(s string) bool {
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		return true
	}
	return strings.HasPrefix(strings.ToUpper(s), "ARRAY[") && strings.HasSuffix(s, "]")
}

// extractFromArrayString extracts value from array string representations like
// "[T]" and PostgreSQL "ARRAY[T]".
func extractFromArrayString(s string) string {
	switch {
	case strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]"):
		return unquoteArrayElement(s[1 : len(s)-1])
	case strings.HasPrefix(strings.ToUpper(s), "ARRAY[") && strings.HasSuffix(s, "]"):
		return unquoteArrayElement(s[len("ARRAY[") : len(s)-1])
	default:
		return s
	}
}

func unquoteArrayElement(inner string) string {
	if len(inner) >= 2 && inner[0] == '\'' && inner[len(inner)-1] == '\'' {
		inner = inner[1 : len(inner)-1]
		return strings.ReplaceAll(inner, "''", "'")
	}
	return inner
}

// isSQLStringLiteral returns true if s is a SQL-quoted string literal (e.g., 'hello').
// In non-parameterized mode, string args arrive as SQL literals like 'Al'.
// In parameterized mode, they arrive as placeholders like @p1 or $1.
func isSQLStringLiteral(s string) bool {
	return len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\''
}

// castToString returns a SQL CAST expression that coerces expr to a string
// type appropriate for the given dialect, ensuring REPLACE receives text input
// even when the bind parameter is numeric.
func castToString(expr string, d jsonlogic2sql.Dialect) string {
	castType := "STRING"
	switch d {
	case jsonlogic2sql.DialectPostgreSQL, jsonlogic2sql.DialectDuckDB:
		castType = "TEXT"
	case jsonlogic2sql.DialectClickHouse:
		castType = "String"
	}
	return fmt.Sprintf("CAST(%s AS %s)", expr, castType)
}

// escapeLikeExpr wraps a SQL expression with REPLACE calls that escape
// LIKE metacharacters (%, _) at query execution time. The expression is
// first cast to a string type so that numeric bind parameters do not
// cause runtime errors in REPLACE.
func escapeLikeExpr(expr string, d jsonlogic2sql.Dialect) string {
	return fmt.Sprintf(
		"REPLACE(REPLACE(REPLACE(%s, '\\\\', '\\\\\\\\'), '%%', '\\%%'), '_', '\\_')",
		castToString(expr, d),
	)
}

// buildLikeSQL builds a LIKE/NOT LIKE expression.
//
// Three pattern argument forms are handled:
//  1. SQL string literal ('hello'): unquote, escape LIKE wildcards, inline.
//  2. Bind placeholder (@p1, $1, or {p1:String}):
//     CAST to string, wrap with REPLACE for runtime escaping.
//  3. Unquoted primitive (1000, TRUE): treat as a literal value, escape and inline.
func buildLikeSQL(column, patternArg, prefix, suffix string, negate bool, d jsonlogic2sql.Dialect) string {
	keyword := "LIKE"
	if negate {
		keyword = "NOT LIKE"
	}
	if isSQLStringLiteral(patternArg) {
		raw := unescapeSQLString(patternArg)
		return fmt.Sprintf("%s %s '%s%s%s'", column, keyword, prefix, escapeLikePattern(raw), suffix)
	}
	if isPlaceholder(patternArg) {
		escaped := escapeLikeExpr(patternArg, d)
		parts := make([]string, 0, 3)
		if prefix != "" {
			parts = append(parts, fmt.Sprintf("'%s'", prefix))
		}
		parts = append(parts, escaped)
		if suffix != "" {
			parts = append(parts, fmt.Sprintf("'%s'", suffix))
		}
		if len(parts) == 1 {
			return fmt.Sprintf("%s %s %s", column, keyword, parts[0])
		}
		return fmt.Sprintf("%s %s CONCAT(%s)", column, keyword, strings.Join(parts, ", "))
	}
	// Unquoted primitive (numeric, boolean, etc.): treat as literal value.
	return fmt.Sprintf("%s %s '%s%s%s'", column, keyword, prefix, escapeLikePattern(patternArg), suffix)
}

func normalizeWaveDashSQL(expr string, d jsonlogic2sql.Dialect) (string, error) {
	switch d {
	case jsonlogic2sql.DialectBigQuery, jsonlogic2sql.DialectSpanner:
		return fmt.Sprintf("REGEXP_REPLACE(%s, r'[\\x{301C}\\x{FF5E}]', '~')", expr), nil
	case jsonlogic2sql.DialectPostgreSQL:
		return fmt.Sprintf("REGEXP_REPLACE(%s, U&'[\\301C\\FF5E]', '~', 'g')", expr), nil
	case jsonlogic2sql.DialectDuckDB:
		return fmt.Sprintf("regexp_replace(%s, '[\\x{301C}\\x{FF5E}]', '~', 'g')", expr), nil
	case jsonlogic2sql.DialectClickHouse:
		return fmt.Sprintf("replaceRegexpAll(%s, '[\\\\x{301C}\\\\x{FF5E}]', '~')", expr), nil
	default:
		return "", fmt.Errorf("unsupported dialect: %v", d)
	}
}

// placeholderRe matches generated placeholders plus ClickHouse's typed
// placeholder form for custom operator interoperability.
var placeholderRe = regexp.MustCompile(`^(?:@p\d+|\$\d+|\{p\d+:[A-Za-z][A-Za-z0-9_]*(?:\([^{}]*\))?\})$`)

// isPlaceholder reports whether s looks like a bind-parameter placeholder.
func isPlaceholder(s string) bool {
	return placeholderRe.MatchString(s)
}

// isBoundValue reports whether s is a SQL string literal or a bind placeholder.
// Used to distinguish "value" arguments from column identifiers in custom operators.
func isBoundValue(s string) bool {
	return isSQLStringLiteral(s) || isPlaceholder(s)
}

// parseContainsArgs parses the arguments for contains/!contains operators.
// Returns the column and the raw pattern argument (preserving SQL quoting
// so that buildLikeSQL can distinguish literals from placeholders).
func parseContainsArgs(args []jsonlogic2sql.OperatorArg) (column, pattern string) {
	arg0Str := args[0].SQL
	arg1Str := args[1].SQL

	if isArrayString(arg1Str) {
		column = arg0Str
		inner := extractFromArrayString(arg1Str)
		if isPlaceholder(inner) {
			return column, inner
		}
		return column, fmt.Sprintf("'%s'", strings.ReplaceAll(inner, "'", "''"))
	}
	if isArrayString(arg0Str) {
		column = arg1Str
		inner := extractFromArrayString(arg0Str)
		if isPlaceholder(inner) {
			return column, inner
		}
		return column, fmt.Sprintf("'%s'", strings.ReplaceAll(inner, "'", "''"))
	}
	if isBoundValue(arg0Str) && !isBoundValue(arg1Str) {
		return arg1Str, arg0Str
	}

	column = arg0Str
	pattern = arg1Str
	if isArrayString(pattern) {
		inner := extractFromArrayString(pattern)
		if isPlaceholder(inner) {
			return column, inner
		}
		return column, fmt.Sprintf("'%s'", strings.ReplaceAll(inner, "'", "''"))
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

// currentSchema holds the loaded schema to preserve validation across dialect switches.
var currentSchema *jsonlogic2sql.Schema

// paramsMode controls whether output uses parameterized placeholders.
var paramsMode bool

// valueMode controls whether input is transpiled as a SQL value expression.
// When false, input is transpiled as a SQL condition/predicate expression.
var valueMode bool

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

func selectExpressionMode(scanner *bufio.Scanner) bool {
	fmt.Println("Select expression mode:")
	fmt.Println("  1. Condition/predicate")
	fmt.Println("  2. Value")
	fmt.Print("\nEnter choice [1-2] (default: 1 for condition): ")

	if !scanner.Scan() {
		return false
	}

	input := strings.TrimSpace(scanner.Text())
	if input == "" {
		return false
	}

	switch strings.ToLower(input) {
	case "1", modeCondition, "predicate":
		return false
	case "2", modeValue:
		return true
	default:
		fmt.Println("Invalid choice, defaulting to condition")
		return false
	}
}

func expressionModeName() string {
	if valueMode {
		return modeValue
	}
	return modeCondition
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

// maxInputSize is the maximum size for a single line of input (1MB).
// Default bufio.Scanner limit is 64KB which truncates large JSON inputs.
const maxInputSize = 1024 * 1024

func main() {
	fmt.Println("JSON Logic to SQL Transpiler REPL")
	fmt.Println("==================================")
	fmt.Println()

	scanner := bufio.NewScanner(os.Stdin)
	// Increase buffer size to handle large JSON inputs
	scanner.Buffer(make([]byte, maxInputSize), maxInputSize)

	// Prompt user to select dialect
	currentDialect = selectDialect(scanner)
	fmt.Println()

	valueMode = selectExpressionMode(scanner)
	fmt.Println()

	currentSchema = promptSchema(scanner)

	transpiler, err := jsonlogic2sql.NewTranspilerWithConfig(&jsonlogic2sql.TranspilerConfig{
		Dialect: currentDialect,
		Schema:  currentSchema,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create transpiler: %v\n", err)
		os.Exit(1)
	}

	// Register all custom operators
	registerCustomOperators(transpiler)

	fmt.Printf("\nUsing %s dialect | Mode: %s\n", getDialectName(currentDialect), expressionModeName())
	fmt.Println("Type ':help' for commands, ':quit' to exit")
	fmt.Println()

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

		processInput(input, transpiler)
		fmt.Println()
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
}

func processInput(input string, transpiler *jsonlogic2sql.Transpiler) {
	if paramsMode {
		var sql string
		var params []jsonlogic2sql.QueryParam
		var err error
		if valueMode {
			sql, params, err = transpiler.TranspileParameterizedValue(input)
		} else {
			sql, params, err = transpiler.TranspileParameterizedCondition(input)
		}
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
		fmt.Printf("SQL:    %s\n", sql)
		printParams(params)
		return
	}

	var result string
	var err error
	if valueMode {
		result, err = transpiler.TranspileValue(input)
	} else {
		result, err = transpiler.TranspileCondition(input)
	}
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("SQL: %s\n", result)
}

// promptSchema loads a schema from a user-provided path, or returns an empty
// schema for literal-only expressions when the user leaves the path blank.
func promptSchema(scanner *bufio.Scanner) *jsonlogic2sql.Schema {
	fmt.Print("Enter schema path (leave empty for literal-only empty schema): ")
	if !scanner.Scan() {
		return emptySchema()
	}
	schemaPath := strings.TrimSpace(scanner.Text())
	if schemaPath == "" {
		return emptySchema()
	}

	data, err := os.ReadFile(filepath.Clean(schemaPath))
	if err != nil {
		fmt.Printf("Error reading schema file: %v\n\n", err)
		return emptySchema()
	}

	schema, err := jsonlogic2sql.NewSchemaFromJSON(data)
	if err != nil {
		fmt.Printf("Error parsing schema file: %v\n\n", err)
		return emptySchema()
	}

	fmt.Printf("Schema loaded: %s\n\n", schemaPath)
	return schema
}

func emptySchema() *jsonlogic2sql.Schema {
	schema, err := jsonlogic2sql.NewSchema(nil)
	if err != nil {
		panic(err)
	}
	return schema
}

// printParams formats and prints the collected query parameters.
func printParams(params []jsonlogic2sql.QueryParam) {
	if len(params) == 0 {
		fmt.Println("Params: (none)")
		return
	}
	fmt.Printf("Params: [")
	for i, p := range params {
		if i > 0 {
			fmt.Print(", ")
		}
		switch v := p.Value.(type) {
		case string:
			fmt.Printf("{%s: %q}", p.Name, v)
		default:
			fmt.Printf("{%s: %v}", p.Name, v)
		}
	}
	fmt.Println("]")
}

func handleCommand(input string, transpiler *jsonlogic2sql.Transpiler, scanner *bufio.Scanner) *jsonlogic2sql.Transpiler {
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
	case ":params":
		paramsMode = !paramsMode
		if paramsMode {
			fmt.Println("Parameterized mode: ON (output uses bind placeholders)")
		} else {
			fmt.Println("Parameterized mode: OFF (output uses inlined literals)")
		}
	case ":mode":
		handleModeCommand(parts)
	case ":condition":
		valueMode = false
		fmt.Printf("Expression mode: %s\n", expressionModeName())
	case ":value":
		valueMode = true
		fmt.Printf("Expression mode: %s\n", expressionModeName())
	case ":schema":
		handleSchemaCommand(parts, transpiler)
	case ":file":
		handleFileInput(parts, transpiler)
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

func handleModeCommand(parts []string) {
	if len(parts) < 2 {
		fmt.Printf("Expression mode: %s\n", expressionModeName())
		return
	}
	switch parts[1] {
	case modeCondition, "predicate":
		valueMode = false
		fmt.Printf("Expression mode: %s\n", expressionModeName())
	case modeValue:
		valueMode = true
		fmt.Printf("Expression mode: %s\n", expressionModeName())
	default:
		fmt.Println("Usage: :mode condition|value")
	}
}

// handleFileInput handles the :file command to read JSON from a file.
// This is useful for large JSON inputs that exceed terminal line limits (~4096 bytes).
func handleFileInput(parts []string, transpiler *jsonlogic2sql.Transpiler) {
	if len(parts) < 2 {
		fmt.Println("Usage: :file <path>")
		fmt.Println("Example: :file input.json")
		return
	}

	filePath := parts[1]
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	input := strings.TrimSpace(string(data))
	if input == "" {
		fmt.Println("File is empty")
		return
	}

	processInput(input, transpiler)
	fmt.Println()
}

// handleDialectChange handles the :dialect command to switch SQL dialects.
func handleDialectChange(scanner *bufio.Scanner) *jsonlogic2sql.Transpiler {
	fmt.Println()
	newDialect := selectDialect(scanner)
	currentDialect = newDialect

	transpiler, err := jsonlogic2sql.NewTranspilerWithConfig(&jsonlogic2sql.TranspilerConfig{
		Dialect: currentDialect,
		Schema:  currentSchema,
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

// handleSchemaCommand handles the :schema command to load a schema from a file.
// This enables schema validation and type-aware SQL generation in the REPL.
func handleSchemaCommand(parts []string, transpiler *jsonlogic2sql.Transpiler) {
	if len(parts) < 2 {
		fmt.Println("Usage: :schema <path>")
		fmt.Println("Example: :schema schema.json")
		return
	}

	schemaPath := parts[1]
	data, err := os.ReadFile(filepath.Clean(schemaPath))
	if err != nil {
		fmt.Printf("Error reading schema file: %v\n", err)
		return
	}

	schema, err := jsonlogic2sql.NewSchemaFromJSON(data)
	if err != nil {
		fmt.Printf("Error parsing schema file: %v\n", err)
		return
	}

	currentSchema = schema
	if err := transpiler.SetSchema(schema); err != nil {
		fmt.Printf("Error setting schema: %v\n", err)
		return
	}
	fmt.Printf("Schema loaded: %s\n\n", schemaPath)
}

// registerCustomOperators registers all custom operators for the REPL.
//
//nolint:funlen // This function registers many operators and is long by design.
func registerCustomOperators(transpiler *jsonlogic2sql.Transpiler) {
	// ========================================================================
	// Basic String Pattern Matching Operators
	// ========================================================================

	// startsWith operator: column LIKE 'value%'.
	_ = transpiler.RegisterOperatorFunc("startsWith", func(_ string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
		if len(args) != 2 {
			return jsonlogic2sql.OperatorResult{}, fmt.Errorf("startsWith requires exactly 2 arguments")
		}
		sql := buildLikeSQL(args[0].SQL, args[1].SQL, "", "%", false, currentDialect)
		return jsonlogic2sql.PredicateSQL(sql), nil
	})

	// !startsWith operator: column NOT LIKE 'value%'.
	_ = transpiler.RegisterOperatorFunc("!startsWith", func(_ string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
		if len(args) != 2 {
			return jsonlogic2sql.OperatorResult{}, fmt.Errorf("!startsWith requires exactly 2 arguments")
		}
		sql := buildLikeSQL(args[0].SQL, args[1].SQL, "", "%", true, currentDialect)
		return jsonlogic2sql.PredicateSQL(sql), nil
	})

	// endsWith operator: column LIKE '%value'.
	_ = transpiler.RegisterOperatorFunc("endsWith", func(_ string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
		if len(args) != 2 {
			return jsonlogic2sql.OperatorResult{}, fmt.Errorf("endsWith requires exactly 2 arguments")
		}
		sql := buildLikeSQL(args[0].SQL, args[1].SQL, "%", "", false, currentDialect)
		return jsonlogic2sql.PredicateSQL(sql), nil
	})

	// !endsWith operator: column NOT LIKE '%value'.
	_ = transpiler.RegisterOperatorFunc("!endsWith", func(_ string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
		if len(args) != 2 {
			return jsonlogic2sql.OperatorResult{}, fmt.Errorf("!endsWith requires exactly 2 arguments")
		}
		sql := buildLikeSQL(args[0].SQL, args[1].SQL, "%", "", true, currentDialect)
		return jsonlogic2sql.PredicateSQL(sql), nil
	})

	// contains operator: column LIKE '%value%'.
	// Supports reversed args: {"contains": ["T", {"var": "field"}]}.
	_ = transpiler.RegisterOperatorFunc("contains", func(_ string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
		if len(args) != 2 {
			return jsonlogic2sql.OperatorResult{}, fmt.Errorf("contains requires exactly 2 arguments")
		}
		column, pattern := parseContainsArgs(args)
		sql := buildLikeSQL(column, pattern, "%", "%", false, currentDialect)
		return jsonlogic2sql.PredicateSQL(sql), nil
	})

	// !contains operator: column NOT LIKE '%value%'.
	_ = transpiler.RegisterOperatorFunc("!contains", func(_ string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
		if len(args) != 2 {
			return jsonlogic2sql.OperatorResult{}, fmt.Errorf("!contains requires exactly 2 arguments")
		}
		column, pattern := parseContainsArgs(args)
		sql := buildLikeSQL(column, pattern, "%", "%", true, currentDialect)
		return jsonlogic2sql.PredicateSQL(sql), nil
	})

	// ========================================================================
	// String Transformation Operators
	// ========================================================================

	// normalizeNFKC operator is basically NORMALIZE(column, 'NFKC').
	_ = transpiler.RegisterOperatorFunc("normalizeNFKC", func(_ string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
		if len(args) != 1 {
			return jsonlogic2sql.OperatorResult{}, fmt.Errorf("normalizeNFKC requires exactly 1 argument")
		}
		sql := fmt.Sprintf("NORMALIZE(%s, 'NFKC')", args[0].SQL)
		return jsonlogic2sql.ValueSQL(sql, jsonlogic2sql.ExpressionTypeString), nil
	})

	_ = transpiler.RegisterDialectAwareOperatorFunc("normalizeWaveDash",
		func(_ string, args []jsonlogic2sql.OperatorArg, dialect jsonlogic2sql.Dialect) (jsonlogic2sql.OperatorResult, error) {
			if len(args) != 1 {
				return jsonlogic2sql.OperatorResult{}, fmt.Errorf("normalizeWaveDash requires exactly 1 argument")
			}
			sql, err := normalizeWaveDashSQL(args[0].SQL, dialect)
			if err != nil {
				return jsonlogic2sql.OperatorResult{}, err
			}
			return jsonlogic2sql.ValueSQL(sql, jsonlogic2sql.ExpressionTypeString), nil
		})

	// toLower operator is basically LOWER(column).
	_ = transpiler.RegisterOperatorFunc("toLower", func(_ string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
		if len(args) != 1 {
			return jsonlogic2sql.OperatorResult{}, fmt.Errorf("toLower requires exactly 1 argument")
		}
		sql := fmt.Sprintf("LOWER(%s)", args[0].SQL)
		return jsonlogic2sql.ValueSQL(sql, jsonlogic2sql.ExpressionTypeString), nil
	})

	// toUpper operator is basically UPPER(column).
	_ = transpiler.RegisterOperatorFunc("toUpper", func(_ string, args []jsonlogic2sql.OperatorArg) (jsonlogic2sql.OperatorResult, error) {
		if len(args) != 1 {
			return jsonlogic2sql.OperatorResult{}, fmt.Errorf("toUpper requires exactly 1 argument")
		}
		sql := fmt.Sprintf("UPPER(%s)", args[0].SQL)
		return jsonlogic2sql.ValueSQL(sql, jsonlogic2sql.ExpressionTypeString), nil
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
		func(_ string, args []jsonlogic2sql.OperatorArg, dialect jsonlogic2sql.Dialect) (jsonlogic2sql.OperatorResult, error) {
			if len(args) != 0 {
				return jsonlogic2sql.OperatorResult{}, fmt.Errorf("currentTimestamp takes no arguments")
			}
			var sql string
			switch dialect {
			case jsonlogic2sql.DialectBigQuery, jsonlogic2sql.DialectSpanner:
				sql = "CURRENT_TIMESTAMP()"
			case jsonlogic2sql.DialectPostgreSQL, jsonlogic2sql.DialectDuckDB:
				sql = "CURRENT_TIMESTAMP"
			case jsonlogic2sql.DialectClickHouse:
				sql = "now()"
			default:
				return jsonlogic2sql.OperatorResult{}, fmt.Errorf("unsupported dialect: %v", dialect)
			}
			return jsonlogic2sql.ValueSQL(sql, jsonlogic2sql.ExpressionTypeUnknown), nil
		})

	// dateDiff operator calculates the difference between two dates (in days).
	// BigQuery: DATE_DIFF(date1, date2, DAY)
	// Spanner: DATE_DIFF(date1, date2, DAY)
	// PostgreSQL: (date1 - date2) -- returns integer days
	// DuckDB: date_diff('day', date2, date1) -- note: part first, then dates
	// ClickHouse: dateDiff('day', date2, date1) -- same as DuckDB
	// Example: {">": [{"dateDiff": [{"var": "end_date"}, {"var": "start_date"}]}, 30]}
	_ = transpiler.RegisterDialectAwareOperatorFunc("dateDiff",
		func(_ string, args []jsonlogic2sql.OperatorArg, dialect jsonlogic2sql.Dialect) (jsonlogic2sql.OperatorResult, error) {
			if len(args) != 2 {
				return jsonlogic2sql.OperatorResult{}, fmt.Errorf("dateDiff requires exactly 2 arguments")
			}
			date1 := args[0].SQL
			date2 := args[1].SQL
			var sql string
			switch dialect {
			case jsonlogic2sql.DialectBigQuery, jsonlogic2sql.DialectSpanner:
				sql = fmt.Sprintf("DATE_DIFF(%s, %s, DAY)", date1, date2)
			case jsonlogic2sql.DialectPostgreSQL:
				// PostgreSQL: subtracting dates returns integer days
				sql = fmt.Sprintf("(%s - %s)", date1, date2)
			case jsonlogic2sql.DialectDuckDB, jsonlogic2sql.DialectClickHouse:
				// DuckDB/ClickHouse: dateDiff('part', start, end) - note different argument order
				sql = fmt.Sprintf("dateDiff('day', %s, %s)", date2, date1)
			default:
				return jsonlogic2sql.OperatorResult{}, fmt.Errorf("unsupported dialect: %v", dialect)
			}
			return jsonlogic2sql.ValueSQL(sql, jsonlogic2sql.ExpressionTypeNumber), nil
		})

	// arrayLength operator returns the length of an array.
	// BigQuery: ARRAY_LENGTH(array)
	// Spanner: ARRAY_LENGTH(array)
	// PostgreSQL: CARDINALITY(array)
	// DuckDB: ARRAY_LENGTH(array)
	// ClickHouse: length(array)
	// Example: {">": [{"arrayLength": [{"var": "tags"}]}, 0]}
	_ = transpiler.RegisterDialectAwareOperatorFunc("arrayLength",
		func(_ string, args []jsonlogic2sql.OperatorArg, dialect jsonlogic2sql.Dialect) (jsonlogic2sql.OperatorResult, error) {
			if len(args) != 1 {
				return jsonlogic2sql.OperatorResult{}, fmt.Errorf("arrayLength requires exactly 1 argument")
			}
			arr := args[0].SQL
			var sql string
			switch dialect {
			case jsonlogic2sql.DialectBigQuery, jsonlogic2sql.DialectSpanner, jsonlogic2sql.DialectDuckDB:
				sql = fmt.Sprintf("ARRAY_LENGTH(%s)", arr)
			case jsonlogic2sql.DialectPostgreSQL:
				sql = fmt.Sprintf("CARDINALITY(%s)", arr)
			case jsonlogic2sql.DialectClickHouse:
				sql = fmt.Sprintf("length(%s)", arr)
			default:
				return jsonlogic2sql.OperatorResult{}, fmt.Errorf("unsupported dialect: %v", dialect)
			}
			return jsonlogic2sql.ValueSQL(sql, jsonlogic2sql.ExpressionTypeNumber), nil
		})

	// regexpContains operator checks if a string matches a regex pattern.
	// BigQuery: REGEXP_CONTAINS(string, pattern)
	// Spanner: REGEXP_CONTAINS(string, pattern)
	// PostgreSQL: string ~ pattern
	// DuckDB: regexp_matches(string, pattern)
	// ClickHouse: match(string, pattern)
	// Example: {"regexpContains": [{"var": "email"}, "^[a-z]+@example\\.com$"]}
	_ = transpiler.RegisterDialectAwareOperatorFunc("regexpContains",
		func(_ string, args []jsonlogic2sql.OperatorArg, dialect jsonlogic2sql.Dialect) (jsonlogic2sql.OperatorResult, error) {
			if len(args) != 2 {
				return jsonlogic2sql.OperatorResult{}, fmt.Errorf("regexpContains requires exactly 2 arguments")
			}
			str := args[0].SQL
			pattern := args[1].SQL
			var sql string
			switch dialect {
			case jsonlogic2sql.DialectBigQuery:
				// BigQuery raw string prefix (r'...') is only valid for literals.
				// Placeholders/expressions must be passed without the raw prefix.
				if isSQLStringLiteral(pattern) {
					sql = fmt.Sprintf("REGEXP_CONTAINS(%s, r%s)", str, pattern)
					return jsonlogic2sql.PredicateSQL(sql), nil
				}
				sql = fmt.Sprintf("REGEXP_CONTAINS(%s, %s)", str, pattern)
			case jsonlogic2sql.DialectSpanner:
				sql = fmt.Sprintf("REGEXP_CONTAINS(%s, %s)", str, pattern)
			case jsonlogic2sql.DialectPostgreSQL:
				sql = fmt.Sprintf("%s ~ %s", str, pattern)
			case jsonlogic2sql.DialectDuckDB:
				sql = fmt.Sprintf("regexp_matches(%s, %s)", str, pattern)
			case jsonlogic2sql.DialectClickHouse:
				sql = fmt.Sprintf("match(%s, %s)", str, pattern)
			default:
				return jsonlogic2sql.OperatorResult{}, fmt.Errorf("unsupported dialect: %v", dialect)
			}
			return jsonlogic2sql.PredicateSQL(sql), nil
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
		func(_ string, args []jsonlogic2sql.OperatorArg, dialect jsonlogic2sql.Dialect) (jsonlogic2sql.OperatorResult, error) {
			if len(args) != 2 {
				return jsonlogic2sql.OperatorResult{}, fmt.Errorf("safeDivide requires exactly 2 arguments")
			}
			numerator := args[0].SQL
			denominator := args[1].SQL
			var sql string
			switch dialect {
			case jsonlogic2sql.DialectBigQuery:
				// BigQuery has built-in SAFE_DIVIDE that returns NULL on division by zero
				sql = fmt.Sprintf("SAFE_DIVIDE(%s, %s)", numerator, denominator)
			case jsonlogic2sql.DialectSpanner, jsonlogic2sql.DialectPostgreSQL, jsonlogic2sql.DialectDuckDB:
				// Spanner, PostgreSQL, and DuckDB don't have SAFE_DIVIDE, use CASE expression
				sql = fmt.Sprintf("CASE WHEN %s = 0 THEN NULL ELSE %s / %s END", denominator, numerator, denominator)
			case jsonlogic2sql.DialectClickHouse:
				// ClickHouse uses if() function for conditional expressions
				sql = fmt.Sprintf("if(%s = 0, NULL, %s / %s)", denominator, numerator, denominator)
			default:
				return jsonlogic2sql.OperatorResult{}, fmt.Errorf("unsupported dialect: %v", dialect)
			}
			return jsonlogic2sql.ValueSQL(sql, jsonlogic2sql.ExpressionTypeNumber), nil
		})
}

func showHelp() {
	fmt.Println("Available commands:")
	fmt.Println("  :help          - Show this help message")
	fmt.Println("  :examples      - Show example JSON Logic expressions")
	fmt.Println("  :dialect       - Change the SQL dialect")
	fmt.Println("  :mode <mode>   - Set expression mode: condition or value")
	fmt.Println("  :condition     - Use condition/predicate expression mode")
	fmt.Println("  :value         - Use value expression mode")
	fmt.Println("  :params        - Toggle parameterized query output (bind placeholders)")
	fmt.Println("  :schema <path> - Load schema for validation and type-aware SQL")
	fmt.Println("  :file <path>   - Read JSON Logic from a file (for large inputs)")
	fmt.Println("  :clear         - Clear the screen")
	fmt.Println("  :quit          - Exit the REPL")
	fmt.Println()
	paramStatus := "OFF"
	if paramsMode {
		paramStatus = "ON"
	}
	mode := modeCondition
	if valueMode {
		mode = modeValue
	}
	fmt.Printf("Current dialect: %s | Mode: %s | Params: %s\n", getDialectName(currentDialect), mode, paramStatus)
	fmt.Println()
	fmt.Println("Enter JSON Logic expressions to convert them to SQL.")
	fmt.Println("Example: {\">\": [{\"var\": \"amount\"}, 1000]}")
	fmt.Println()
	fmt.Println("Note: For large JSON inputs (>4KB), use :file to avoid terminal limits.")
}

type replExample struct {
	name string
	json string
	sql  string
}

func replExamples() []replExample {
	return []replExample{
		{
			name: "Simple Comparison",
			json: `{">": [{"var": "amount"}, 1000]}`,
			sql:  "amount > 1000",
		},
		{
			name: "Multiple Conditions (AND)",
			json: `{"and": [{">": [{"var": "amount"}, 5000]}, {"==": [{"var": "status"}, "pending"]}]}`,
			sql:  "(amount > 5000 AND status = 'pending')",
		},
		{
			name: "Multiple Conditions (OR)",
			json: `{"or": [{">=": [{"var": "failedAttempts"}, 5]}, {"in": [{"var": "country"}, ["CN", "RU"]]}]}`,
			sql:  "(failedAttempts >= 5 OR country IN ('CN', 'RU'))",
		},
		{
			name: "Nested Conditions",
			json: `{"and": [{">": [{"var": "transaction.amount"}, 10000]}, {"or": [{"==": [{"var": "user.verified"}, false]}, {"<": [{"var": "user.accountAgeDays"}, 7]}]}]}`,
			sql:  "(transaction_amount > 10000 AND (user_verified = FALSE OR user_accountAgeDays < 7))",
		},
		{
			name: "IF Condition",
			json: `{"if": [{">": [{"var": "age"}, 18]}, {"==": [{"var": "status"}, "adult"]}, {"==": [{"var": "status"}, "minor"]}]}`,
			sql:  "CASE WHEN age > 18 THEN status = 'adult' ELSE status = 'minor' END",
		},
		{
			name: "Missing Field Check",
			json: `{"missing": ["field"]}`,
			sql:  "field IS NULL",
		},
		{
			name: "Missing Some Fields",
			json: `{"missing_some": [1, ["field1", "field2"]]}`,
			sql:  "(field1 IS NULL + field2 IS NULL) >= 1",
		},
		{
			name: "NOT Operation",
			json: `{"!": [{"==": [{"var": "verified"}, true]}]}`,
			sql:  "NOT (verified = TRUE)",
		},
	}
}

func showExamples() {
	examples := replExamples()
	fmt.Println("Example JSON Logic expressions:")
	fmt.Println()

	for i, example := range examples {
		fmt.Printf("%d. %s\n", i+1, example.name)
		fmt.Printf("   JSON: %s\n", example.json)
		fmt.Printf("   SQL:  %s\n", example.sql)
		fmt.Println()
	}
}
