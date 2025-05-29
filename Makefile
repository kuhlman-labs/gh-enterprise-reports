# gh-enterprise-reports Makefile
# Go-based GitHub CLI extension for generating enterprise reports

# Variables
BINARY_NAME=gh-enterprise-reports
MAIN_FILE=main.go
GO_VERSION=1.24.0
MODULE_NAME=github.com/kuhlman-labs/gh-enterprise-reports

# Build variables
BUILD_DIR=build
DIST_DIR=dist
VERSION?=$(shell git describe --tags --always --dirty)
COMMIT?=$(shell git rev-parse --short HEAD)
BUILD_DATE?=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

# LDFLAGS for build info
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)"

# Go variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Cross-compilation targets
PLATFORMS=darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64

.PHONY: all build clean test deps fmt vet lint install uninstall run help cross-compile release dev-setup

# Default target
all: clean deps fmt vet test build

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)

# Build for development (no optimizations)
build-dev:
	@echo "Building $(BINARY_NAME) for development..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	@rm -rf $(BUILD_DIR)
	@rm -rf $(DIST_DIR)
	@rm -f $(BINARY_NAME)

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -coverprofile=coverage.out ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Format Go code
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

# Vet Go code
vet:
	@echo "Vetting code..."
	$(GOVET) ./...

# Run golangci-lint (requires golangci-lint to be installed)
lint:
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Run 'make dev-setup'" && exit 1)
	golangci-lint run

# Install the binary to GOPATH/bin
install: build
	@echo "Installing $(BINARY_NAME) to $(shell go env GOPATH)/bin..."
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(shell go env GOPATH)/bin/

# Uninstall the binary from GOPATH/bin
uninstall:
	@echo "Uninstalling $(BINARY_NAME)..."
	@rm -f $(shell go env GOPATH)/bin/$(BINARY_NAME)

# Install as GitHub CLI extension
gh-install: build
	@echo "Installing as GitHub CLI extension..."
	@mkdir -p ~/.local/share/gh/extensions/gh-enterprise-reports
	@cp $(BUILD_DIR)/$(BINARY_NAME) ~/.local/share/gh/extensions/gh-enterprise-reports/
	@echo "Extension installed. You can now run: gh enterprise-reports"

# Uninstall GitHub CLI extension
gh-uninstall:
	@echo "Uninstalling GitHub CLI extension..."
	@rm -rf ~/.local/share/gh/extensions/gh-enterprise-reports
	@echo "Extension uninstalled."

# Run the application
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)

# Run with arguments (use: make run-with ARGS="--help")
run-with: build
	@echo "Running $(BINARY_NAME) with args: $(ARGS)"
	./$(BUILD_DIR)/$(BINARY_NAME) $(ARGS)

# Cross-compile for multiple platforms
cross-compile:
	@echo "Cross-compiling for multiple platforms..."
	@mkdir -p $(DIST_DIR)
	@for platform in $(PLATFORMS); do \
		os=$$(echo $$platform | cut -d'/' -f1); \
		arch=$$(echo $$platform | cut -d'/' -f2); \
		output_name=$(DIST_DIR)/$(BINARY_NAME)-$$os-$$arch; \
		if [ $$os = "windows" ]; then output_name=$$output_name.exe; fi; \
		echo "Building for $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch $(GOBUILD) $(LDFLAGS) -o $$output_name $(MAIN_FILE); \
	done

# Create release archives
release: cross-compile
	@echo "Creating release archives..."
	@cd $(DIST_DIR) && for file in *; do \
		if [ -f "$$file" ]; then \
			if [[ "$$file" == *".exe" ]]; then \
				zip "$${file%.*}.zip" "$$file"; \
			else \
				tar -czf "$$file.tar.gz" "$$file"; \
			fi; \
		fi; \
	done
	@echo "Release archives created in $(DIST_DIR)/"

# Initialize development environment
dev-setup:
	@echo "Setting up development environment..."
	@echo "Installing development tools..."
	@which golangci-lint > /dev/null || curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(shell go env GOPATH)/bin
	@echo "Development environment ready!"

# Generate a config template
init-config:
	@echo "Generating config template..."
	./$(BUILD_DIR)/$(BINARY_NAME) init --output config-template.yml

# Check for Go version compatibility
check-go-version:
	@echo "Checking Go version compatibility..."
	@go version | grep -q "go$(GO_VERSION)" || (echo "Warning: This project requires Go $(GO_VERSION)" && go version)

# Security audit
security:
	@echo "Running security audit..."
	@which gosec > /dev/null || (echo "Installing gosec..." && go run github.com/securego/gosec/v2/cmd/gosec@latest ./...)

# Update dependencies
update-deps:
	@echo "Updating dependencies..."
	$(GOGET) -u ./...
	$(GOMOD) tidy

# Show help
help:
	@echo "Available targets:"
	@echo "  all              - Clean, get deps, format, vet, test, and build"
	@echo "  build            - Build the binary"
	@echo "  build-dev        - Build the binary for development"
	@echo "  clean            - Clean build artifacts"
	@echo "  test             - Run tests"
	@echo "  test-coverage    - Run tests with coverage report"
	@echo "  deps             - Download and tidy dependencies"
	@echo "  fmt              - Format Go code"
	@echo "  vet              - Vet Go code"
	@echo "  lint             - Run golangci-lint"
	@echo "  install          - Install binary to GOPATH/bin"
	@echo "  uninstall        - Uninstall binary from GOPATH/bin"
	@echo "  gh-install       - Install as GitHub CLI extension"
	@echo "  gh-uninstall     - Uninstall GitHub CLI extension"
	@echo "  run              - Build and run the application"
	@echo "  run-with         - Build and run with arguments (use ARGS=)"
	@echo "  cross-compile    - Cross-compile for multiple platforms"
	@echo "  release          - Create release archives for all platforms"
	@echo "  dev-setup        - Set up development environment"
	@echo "  init-config      - Generate a config template"
	@echo "  check-go-version - Check Go version compatibility"
	@echo "  security         - Run security audit with gosec"
	@echo "  update-deps      - Update all dependencies"
	@echo "  help             - Show this help message" 