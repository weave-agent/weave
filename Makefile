GO_FILES ?= ./bus/... ./cmd/... ./config/... ./launcher/... ./sdk/...
GOLANGCI_LINT ?= golangci-lint
EXT_DIRS := $(sort $(dir $(wildcard extensions/*/*/go.mod extensions/*/go.mod)))

.DEFAULT_GOAL := help
.PHONY: help tools gen fmt lint fix test bench

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

##@ Tools
tools: ## Install/update development tools (moq, golangci-lint)
	go install github.com/matryer/moq@latest
	@brew install golangci-lint 2>/dev/null || brew upgrade golangci-lint

##@ Generate code
gen: ## Regenerate mocks, generated code
	@echo "Removing old generated files..."
	find . -name "*_mock.go" -delete
	@echo "Regenerating code..."
	go generate ./...
	cd extensions/loop && go generate ./...

##@ Formatting
fmt: ## Format code (gofumpt, goimports, go fix)
	@echo "Running golangci-lint formatters (gofumpt, goimports, swaggo)..."
	$(GOLANGCI_LINT) fmt --config .golangci.yml ./...
	@for dir in $(EXT_DIRS); do \
		(cd $$dir && $(GOLANGCI_LINT) fmt --config $(CURDIR)/.golangci.yml ./...) || exit 1; \
	done
	@echo "Running modernize..."
	go fix ./...

##@ Linting
lint: ## Run golangci-lint
	@echo "lint via golangci-lint"
	$(GOLANGCI_LINT) run \
		--config .golangci.yml \
		$(GO_FILES)
	@for dir in $(EXT_DIRS); do \
		echo "lint $$dir"; \
		(cd $$dir && $(GOLANGCI_LINT) run --config $(CURDIR)/.golangci.yml ./...) || exit 1; \
	done

##@ Fix (format + auto-fix linter issues)
fix: ## Auto-fix linter issues
	@echo "Running golangci-lint with --fix..."
	$(GOLANGCI_LINT) run --fix --config .golangci.yml $(GO_FILES)
	@for dir in $(EXT_DIRS); do \
		echo "fix $$dir"; \
		(cd $$dir && $(GOLANGCI_LINT) run --fix --config $(CURDIR)/.golangci.yml ./...) || exit 1; \
	done

##@ Testing
test: ## Run all tests (root + extension modules)
	go test ./...
	@for dir in $(EXT_DIRS); do \
		echo "test $$dir"; \
		(cd $$dir && go test ./...) || exit 1; \
	done

##@ Benchmarking
bench: ## Run build benchmarks (cold/warm/partial, with and without TUI)
	go test -bench=. -benchmem -run=NONE -timeout 600s ./launcher/
