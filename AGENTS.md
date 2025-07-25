# AGENTS.md - Development Guide for patience CLI

## Build/Test Commands
- `go build ./...` - Build all packages
- `go test ./...` - Run all tests
- `go test -v ./pkg/executor` - Run tests for specific package
- `go test -race ./...` - Run tests with race detection
- `go mod tidy` - Clean up dependencies
- `gofmt -w .` - Format all Go files
- `goimports -w .` - Format and organize imports
- `golangci-lint run` - Run linter (requires .golangci.yml config)

## Code Style Guidelines
- **Formatting**: All code MUST be formatted with `gofmt` and `goimports`
- **Testing**: Follow TDD (Red-Green-Refactor). Use `testify/require` for critical checks, `testify/assert` for others
- **Naming**: Use clear, descriptive names. Test functions start with `Test`, files end with `_test.go`
- **Interfaces**: Accept interfaces, return structs. Use interfaces to define behavior, not data
- **Error Handling**: Handle errors explicitly, never discard. Use `errors` package to wrap with context
- **Comments**: All exported functions/types need godoc comments. Use complete sentences
- **Packages**: Single purpose packages. Avoid generic utils packages

## Project Structure
- `/cmd/patience` - Main CLI package using Cobra
- `/pkg/executor` - Core retry logic and command execution
- `/pkg/config` - Configuration loading and validation
- `/pkg/backoff` - Backoff strategies (exponential, fixed, jitter)
- `/pkg/conditions` - Success/failure condition checking
- `/pkg/metrics` - Metrics collection and daemon communication
- `/pkg/ui` - Terminal output and status reporting