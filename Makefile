.PHONY: build install go-install clean test vet lint fmt help man install-man e2e

# Binary name
BINARY_NAME=agr

# Build directory
BUILD_DIR=build

# Go parameters
GOCMD=go
GO_BUILDVCS_FLAG ?= -buildvcs=false
GOBUILD=$(GOCMD) build $(GO_BUILDVCS_FLAG)
GOINSTALL=$(GOCMD) install $(GO_BUILDVCS_FLAG)
GORUN=$(GOCMD) run $(GO_BUILDVCS_FLAG)
GOTEST=$(GOCMD) test
GOMOD=$(GOCMD) mod
GOVET=$(GOCMD) vet
GOLANGCI_LINT=golangci-lint
TMP_BASE ?= $(if $(TMPDIR),$(TMPDIR),/tmp)
GOCACHE_DIR ?= $(TMP_BASE)/agr-go-cache
GOTMPDIR_DIR ?= $(TMP_BASE)/agr-go-tmp
GOLANGCI_LINT_CACHE_DIR ?= $(TMP_BASE)/agr-golangci-lint-cache
GO_RUN_ENV = GOCACHE=$(GOCACHE_DIR) GOTMPDIR=$(GOTMPDIR_DIR)
GOLANGCI_LINT_ENV = $(GO_RUN_ENV) GOLANGCI_LINT_CACHE=$(GOLANGCI_LINT_CACHE_DIR)

# Version info (can be overridden)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Build flags
LDFLAGS=-ldflags "-s -w -X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

# Default target
all: build

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(GOCACHE_DIR) $(GOTMPDIR_DIR)
	$(GO_RUN_ENV) $(GOBUILD) $(LDFLAGS) -o $(BINARY_NAME) ./cmd/agr

## build-all: Build for multiple platforms
build-all: build-linux build-darwin build-windows

build-linux:
	@echo "Building for Linux..."
	@mkdir -p $(BUILD_DIR)
	@mkdir -p $(GOCACHE_DIR) $(GOTMPDIR_DIR)
	$(GO_RUN_ENV) GOOS=linux GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 ./cmd/agr
	$(GO_RUN_ENV) GOOS=linux GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 ./cmd/agr

build-darwin:
	@echo "Building for macOS..."
	@mkdir -p $(BUILD_DIR)
	@mkdir -p $(GOCACHE_DIR) $(GOTMPDIR_DIR)
	$(GO_RUN_ENV) GOOS=darwin GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/agr
	$(GO_RUN_ENV) GOOS=darwin GOARCH=arm64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/agr

build-windows:
	@echo "Building for Windows..."
	@mkdir -p $(BUILD_DIR)
	@mkdir -p $(GOCACHE_DIR) $(GOTMPDIR_DIR)
	$(GO_RUN_ENV) GOOS=windows GOARCH=amd64 $(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe ./cmd/agr

## install: Install the binary to /usr/local/bin
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	@sudo cp $(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "Done."

## go-install: Install via go install with version metadata injected
go-install:
	@echo "Installing $(BINARY_NAME) via go install with version info..."
	@mkdir -p $(GOCACHE_DIR) $(GOTMPDIR_DIR)
	$(GO_RUN_ENV) $(GOINSTALL) $(LDFLAGS) ./cmd/agr
	@echo "Done. Binary installed to $$(go env GOPATH)/bin/$(BINARY_NAME)"

## uninstall: Remove the binary from /usr/local/bin
uninstall:
	@echo "Removing $(BINARY_NAME) from /usr/local/bin..."
	@sudo rm -f /usr/local/bin/$(BINARY_NAME)
	@echo "Done."

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -f $(BINARY_NAME)
	@rm -rf $(BUILD_DIR)
	@echo "Done."

## test: Run tests
test:
	@mkdir -p $(GOCACHE_DIR) $(GOTMPDIR_DIR)
	$(GO_RUN_ENV) $(GOTEST) -v ./...

## vet: Run go vet
vet:
	mkdir -p $(GOCACHE_DIR) $(GOTMPDIR_DIR)
	$(GO_RUN_ENV) $(GOVET) ./...

## lint: Run golangci-lint
lint:
	mkdir -p $(GOCACHE_DIR) $(GOTMPDIR_DIR) $(GOLANGCI_LINT_CACHE_DIR)
	$(GOLANGCI_LINT_ENV) $(GOLANGCI_LINT) run

## fmt: Run gofmt
fmt:
	gofmt -w .

## e2e: Run lifecycle tests (requires credentials via env or ~/.agr/config.toml)
e2e:
	@mkdir -p $(GOCACHE_DIR) $(GOTMPDIR_DIR)
	$(GO_RUN_ENV) $(GOTEST) -v -timeout 20m ./tests/lifecycle/...

## deps: Download dependencies
deps:
	@mkdir -p $(GOCACHE_DIR) $(GOTMPDIR_DIR)
	$(GO_RUN_ENV) $(GOMOD) download

## tidy: Tidy go.mod
tidy:
	@mkdir -p $(GOCACHE_DIR) $(GOTMPDIR_DIR)
	$(GO_RUN_ENV) $(GOMOD) tidy

## man: Generate man pages (maintainer-only docgen, NextPlan §9.5)
man:
	@echo "Generating man pages..."
	@mkdir -p $(GOCACHE_DIR) $(GOTMPDIR_DIR)
	$(GO_RUN_ENV) $(GORUN) ./cmd/internal/docgen man --dir man
	@echo "Done."

## install-man: Install man pages to system
install-man: man
	@echo "Installing man pages to /usr/local/share/man/man1/..."
	@sudo mkdir -p /usr/local/share/man/man1
	@sudo cp man/*.1 /usr/local/share/man/man1/
	@echo "Done. Use 'man agr' to view."

## uninstall-man: Remove man pages from system
uninstall-man:
	@echo "Removing man pages..."
	@sudo rm -f /usr/local/share/man/man1/agr*.1
	@echo "Done."

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed 's/^/  /'
