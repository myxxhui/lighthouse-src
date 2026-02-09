# Lighthouse Makefile
# Infrastructure Decision Cockpit

.PHONY: help build test clean lint vet

# Default target
help:
	@echo "Lighthouse Makefile"
	@echo "Available targets:"
	@echo "  build     - Build the server binary"
	@echo "  test      - Run all tests"
	@echo "  clean     - Clean build artifacts"
	@echo "  lint      - Run linter"
	@echo "  vet       - Run go vet"
	@echo "  help      - Show this help"

# Build the server
build:
	go build -o bin/lighthouse-server ./cmd/server

# Run tests
test:
	go test -v ./...

# Clean build artifacts
clean:
	rm -rf bin/
	go clean

# Run linter
lint:
	golint ./...

# Run go vet
vet:
	go vet ./...

# Initialize Go module (if needed)
init:
	go mod init github.com/myxxhui/lighthouse-src

# Format code
fmt:
	gofmt -w .

# Generate dependencies
tidy:
	go mod tidy