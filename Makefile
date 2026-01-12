.PHONY: build test test-verbose lint lint/fix run clean help

# Default target
all: test build

# Build the REPL binary (reproducible build)
build:
	@echo "Building REPL binary..."
	@go build -trimpath -buildvcs=false -ldflags="-buildid=" -o bin/repl ./cmd/repl
	@echo "Binary built at bin/repl"

# Run all tests
test:
	@echo "Running tests..."
	@go test ./...

# Run tests with verbose output
test-verbose:
	@echo "Running tests with verbose output..."
	@go test -v ./...


# Run linter
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run $(args) && go list ./...; \
	else \
		echo "golangci-lint not found. Install it with: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8.0"; \
	fi

lint/fix:
	@make lint args='--fix -v' cons_args='-v'

# Run the REPL
run: build
	@echo "Starting REPL..."
	@./bin/repl

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@go clean

# Install dependencies
deps:
	@echo "Installing dependencies..."
	@go mod download
	@go mod tidy

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	@go vet ./...

# Check for security vulnerabilities
security:
	@echo "Checking for security vulnerabilities..."
	@if command -v gosec >/dev/null 2>&1; then \
		gosec ./...; \
	else \
		echo "gosec not found. Install it with: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest"; \
	fi

# Benchmark tests
bench:
	@echo "Running benchmarks..."
	@go test -bench=. ./...

# Show help
help:
	@echo "Available targets:"
	@echo "  build        - Build the REPL binary"
	@echo "  test         - Run all tests"
	@echo "  test-verbose - Run tests with verbose output"
	@echo "  lint         - Run linter"
	@echo "  run          - Build and run the REPL"
	@echo "  clean        - Clean build artifacts"
	@echo "  deps         - Install dependencies"
	@echo "  fmt          - Format code"
	@echo "  vet          - Run go vet"
	@echo "  security     - Check for security vulnerabilities"
	@echo "  bench        - Run benchmark tests"
	@echo "  help         - Show this help message"
