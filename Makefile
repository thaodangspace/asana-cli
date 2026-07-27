BINARY ?= asana-cli
MAIN ?= ./cmd/asana-cli
PKGS ?= ./...
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS ?= -s -w -X github.com/dtonair/asana-cli/cli.version=$(VERSION)

.DEFAULT_GOAL := help

.PHONY: help build install test test-race fmt vet tidy check docs-install docs-dev docs-build docs-preview clean snapshot

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage: make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the asana-cli binary
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) $(MAIN)

install: ## Install the CLI with go install
	go install -trimpath -ldflags "$(LDFLAGS)" $(MAIN)

test: ## Run tests
	env ASANA_CONFIG=/dev/null ASANA_ACCESS_TOKEN= ASANA_DEFAULT_WORKSPACE= ASANA_API_BASE= go test $(PKGS)

test-race: ## Run tests with the race detector
	env ASANA_CONFIG=/dev/null ASANA_ACCESS_TOKEN= ASANA_DEFAULT_WORKSPACE= ASANA_API_BASE= go test -race $(PKGS)

fmt: ## Format Go code
	go fmt $(PKGS)

vet: ## Run go vet
	go vet $(PKGS)

tidy: ## Tidy Go modules
	go mod tidy

check: fmt tidy vet test ## Format, tidy, vet, and test

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
