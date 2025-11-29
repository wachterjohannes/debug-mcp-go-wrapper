.PHONY: build test clean run install

# Binary name
BINARY=debug-mcp-wrapper

# Build the binary
build:
	go build -o $(BINARY) cmd/debug-mcp-wrapper/main.go

# Run tests
test:
	go test ./... -v

# Clean build artifacts
clean:
	rm -f $(BINARY)
	go clean

# Run the wrapper (requires --cwd argument)
run:
	go run cmd/debug-mcp-wrapper/main.go

# Install dependencies
install:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Run linter
lint:
	go vet ./...

# Build for multiple platforms
build-all:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY)-linux-amd64 cmd/debug-mcp-wrapper/main.go
	GOOS=darwin GOARCH=amd64 go build -o $(BINARY)-darwin-amd64 cmd/debug-mcp-wrapper/main.go
	GOOS=darwin GOARCH=arm64 go build -o $(BINARY)-darwin-arm64 cmd/debug-mcp-wrapper/main.go
	GOOS=windows GOARCH=amd64 go build -o $(BINARY)-windows-amd64.exe cmd/debug-mcp-wrapper/main.go
