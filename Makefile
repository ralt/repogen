.PHONY: all build test test-unit test-integration test-packages clean install help

# Default target
all: build

# Build the repogen binary
build:
	@echo "Building repogen..."
	@go build -o repogen ./cmd/repogen
	@echo "✓ Build complete: ./repogen"

# Run all tests
test: test-unit test-integration

# Run unit tests only
test-unit:
	@echo "Running unit tests..."
	@go test -v -short ./...

# Build test packages (requires Docker or native tools)
test-packages:
	@echo "Building test packages..."
	@chmod +x test/build-test-packages.sh
	@./test/build-test-packages.sh

# Build test packages using Docker (if native tools not available)
test-packages-docker:
	@echo "Building test packages in Docker..."
	@chmod +x test/build-test-packages.sh
	@echo "Building Debian packages..."
	@docker run --rm -v $(PWD):/work -w /work debian:bookworm bash -c "apt-get update -qq && apt-get install -y -qq dpkg-dev > /dev/null 2>&1 && ./test/build-test-packages.sh"
	@echo "Building RPM packages..."
	@docker run --rm -v $(PWD):/work -w /work fedora:latest bash -c "dnf install -y -q rpm-build > /dev/null 2>&1 && ./test/build-test-packages.sh"
	@echo "✓ All test packages built successfully"

# Run integration tests (requires Docker)
test-integration: test-packages
	@echo "Running integration tests..."
	@go test -v -timeout 15m ./test

# Quick integration test (skip if Docker not available)
test-integration-quick:
	@echo "Running integration tests (quick)..."
	@go test -v -timeout 10m -short ./test || echo "Integration tests skipped"

# Clean build artifacts and test outputs
clean:
	@echo "Cleaning..."
	@rm -f repogen
	@rm -rf test/integration-output
	@rm -rf test/fixtures/deb-build test/fixtures/rpm-build test/fixtures/apk-build test/fixtures/bottle-build
	@echo "✓ Clean complete"

# Install to system
install: build
	@echo "Installing repogen to /usr/local/bin..."
	@sudo cp repogen /usr/local/bin/
	@echo "✓ Installation complete"

# Uninstall from system
uninstall:
	@echo "Removing repogen from /usr/local/bin..."
	@sudo rm -f /usr/local/bin/repogen
	@echo "✓ Uninstall complete"

# Run linter
lint:
	@echo "Running linter..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found, install from https://golangci-lint.run/"; exit 1)
	@golangci-lint run ./...

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "✓ Format complete"

# Update dependencies
deps:
	@echo "Updating dependencies..."
	@go mod tidy
	@go mod download
	@echo "✓ Dependencies updated"

# Display help
help:
	@echo "Repogen Makefile targets:"
	@echo ""
	@echo "  make build                 - Build the repogen binary"
	@echo "  make test                  - Run all tests"
	@echo "  make test-unit             - Run unit tests only"
	@echo "  make test-integration      - Run Docker-based integration tests"
	@echo "  make test-packages         - Build test packages for integration tests"
	@echo "  make test-packages-docker  - Build test packages using Docker"
	@echo "  make clean                 - Clean build artifacts"
	@echo "  make install               - Install to /usr/local/bin"
	@echo "  make uninstall             - Remove from /usr/local/bin"
	@echo "  make lint                  - Run linter"
	@echo "  make fmt                   - Format code"
	@echo "  make deps                  - Update dependencies"
	@echo "  make help                  - Show this help message"
	@echo ""
	@echo "Requirements for integration tests:"
	@echo "  - Docker (for running tests in containers)"
	@echo "  - dpkg-deb (for building Debian packages)"
	@echo "  - rpmbuild (for building RPM packages)"
	@echo ""
	@echo "Quick start:"
	@echo "  make build          # Build the binary"
	@echo "  make test-packages  # Build test packages"
	@echo "  make test           # Run all tests"
