BINARY ?= asana-cli
MAIN ?= ./cmd/asana-cli
PKGS ?= ./...
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -s -w -X github.com/thaodangspace/asana-cli/cli.version=$(VERSION)

.DEFAULT_GOAL := help

.PHONY: help build install test test-race fmt fmt-check vet tidy mod-check check ci docs-install docs-dev docs-build docs-preview clean snapshot

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the asana-cli binary
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) $(MAIN)

install: ## Install the CLI with go install
	go install -trimpath -ldflags "$(LDFLAGS)" $(MAIN)

test: ## Run tests
	env ASANA_CONFIG=/dev/null ASANA_ACCESS_TOKEN= ASANA_DEFAULT_WORKSPACE= ASANA_API_BASE= go test -count=1 $(PKGS)

test-race: ## Run tests with the race detector
	env ASANA_CONFIG=/dev/null ASANA_ACCESS_TOKEN= ASANA_DEFAULT_WORKSPACE= ASANA_API_BASE= go test -count=1 -race $(PKGS)

fmt: ## Format Go code
	go fmt $(PKGS)

fmt-check: ## Verify Go code is formatted
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		printf '%s\n' "$$unformatted"; \
		exit 1; \
	fi

vet: ## Run go vet
	go vet $(PKGS)

tidy: ## Tidy Go modules
	go mod tidy

mod-check: ## Verify go.mod and go.sum are tidy
	go mod tidy
	git diff --exit-code -- go.mod go.sum

check: fmt tidy vet test ## Format, tidy, vet, and test

ci: fmt-check mod-check vet test-race ## Run the read-only CI quality checks locally

docs-install: ## Install documentation site dependencies
	npm --prefix docs ci

docs-dev: ## Start the documentation site development server
	npm --prefix docs run dev

docs-build: docs-install ## Build the static documentation site
	npm --prefix docs run build

docs-preview: ## Preview the production documentation build
	npm --prefix docs run preview

clean: ## Remove build artifacts
	rm -f $(BINARY)
	rm -rf dist

snapshot: ## Build a local GoReleaser snapshot
	goreleaser release --snapshot --clean
