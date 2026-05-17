.DEFAULT_GOAL := help
.PHONY: help tools gen fmt lint tidy fix test bench

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

##@ Tools
tools: ## Install/update development tools (moq, golangci-lint)
	go install github.com/matryer/moq@latest
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest

##@ Generate code
gen: ## Regenerate mocks, generated code
	@echo "Removing old generated files..."
	find . -name "*_mock.go" -delete
	@echo "Regenerating code..."
	go generate ./...

##@ Formatting
fmt: ## Format code (gofumpt, goimports, go fix)
	@echo "Running golangci-lint formatters (gofumpt, goimports)..."
	golangci-lint fmt --config .golangci.yml ./...
	@echo "Running modernize..."
	go fix ./...

##@ Linting
lint: ## Run golangci-lint
	golangci-lint run --config .golangci.yml ./...

##@ Dependencies
tidy: ## Run go mod tidy
	go mod tidy

##@ Fix (format + auto-fix linter issues)
fix: ## Auto-fix linter issues
	golangci-lint run --fix --config .golangci.yml ./...

##@ Testing
test: ## Run all tests
	go test ./...

##@ Benchmarking
bench: ## Run build benchmarks (cold/warm/partial, with and without TUI)
	go test -bench=. -benchmem -run=NONE -timeout 600s ./internal/launcher/
