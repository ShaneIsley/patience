# AGENTS.md - Development Guide for patience CLI

## Build/Test Commands
- `go build ./...` - Build all packages
- `go build -o patience ./cmd/patience` - Build main CLI binary
- `go test ./...` - Run all tests
- `go test -v ./pkg/executor` - Run tests for specific package
- `go test -v ./pkg/backoff -run TestHTTPAware` - Run HTTP-aware strategy tests
- `go test -v ./cmd/patience` - Run CLI integration tests
- `go test -race ./...` - Run tests with race detection
- `go test -cover ./...` - Run tests with coverage reporting
- `go mod tidy` - Clean up dependencies
- `gofmt -w .` - Format all Go files
- `goimports -w .` - Format and organize imports
- `golangci-lint run` - Run linter (requires .golangci.yml config)

## CLI Interface (Current Implementation)

### Subcommand Architecture
The CLI uses a strategy-based subcommand architecture:

```bash
# Basic syntax
patience STRATEGY [OPTIONS] -- COMMAND [ARGS...]

# Available strategies with aliases
patience http-aware (ha)           # HTTP response-aware delays
patience exponential (exp)         # Exponentially increasing delays  
patience linear (lin)              # Linearly increasing delays
patience fixed (fix)               # Fixed delay between retries
patience jitter (jit)              # Random jitter around base delay
patience decorrelated-jitter (dj)  # AWS-style decorrelated jitter
patience fibonacci (fib)           # Fibonacci sequence delays
```

### Common Flags (All Strategies)
- `--attempts, -a` - Maximum retry attempts (default: 3)
- `--timeout, -t` - Timeout per attempt
- `--success-pattern` - Regex pattern for success detection
- `--failure-pattern` - Regex pattern for failure detection
- `--case-insensitive` - Case-insensitive pattern matching

### Strategy-Specific Flags
Each strategy has unique configuration options. Use `patience STRATEGY --help` for details.

### Testing the CLI
```bash
# Test basic functionality
./patience exponential --attempts 3 --base-delay 1s -- echo "test"

# Test HTTP-aware with real API
./patience http-aware -- curl -i https://api.github.com/user

# Test pattern matching
./patience fixed --success-pattern "SUCCESS" -- echo "SUCCESS: done"

# Test failure scenarios
./patience exponential --attempts 2 -- false
```

## Code Style Guidelines
- **Formatting**: All code MUST be formatted with `gofmt` and `goimports`
- **Testing**: Follow TDD (Red-Green-Refactor). Use `testify/require` for critical checks, `testify/assert` for others
- **Naming**: Use clear, descriptive names. Test functions start with `Test`, files end with `_test.go`
- **Interfaces**: Accept interfaces, return structs. Use interfaces to define behavior, not data
- **Error Handling**: Handle errors explicitly, never discard. Use `errors` package to wrap with context
- **Comments**: All exported functions/types need godoc comments. Use complete sentences
- **Packages**: Single purpose packages. Avoid generic utils packages
- **Subcommands**: Each strategy subcommand should have its own configuration struct and validation
- **HTTP Parsing**: Use Go standard library for HTTP response parsing, no external dependencies
- **Strategy Pattern**: All backoff strategies implement the `backoff.Strategy` interface

## Project Structure
- `/cmd/patience` - Main CLI package with subcommand architecture using Cobra
  - `main.go` - Root command and strategy registration
  - `subcommands.go` - All strategy subcommand implementations
- `/cmd/patienced` - Optional daemon for metrics aggregation
- `/pkg/executor` - Core retry logic and command execution
- `/pkg/config` - Configuration loading and validation
- `/pkg/backoff` - All backoff strategies including HTTP-aware intelligence
  - `strategy.go` - Base strategy interface
  - `http_aware.go` - HTTP response parsing and adaptive timing
  - `[other strategies]` - Mathematical backoff implementations
- `/pkg/conditions` - Success/failure condition checking with regex support
- `/pkg/metrics` - Metrics collection and daemon communication
- `/pkg/ui` - Terminal output and status reporting
- `/pkg/storage` - Configuration and state persistence
- `/scripts` - Installation, testing, and deployment scripts
- `/benchmarks` - Performance testing infrastructure
- `/examples` - Real-world usage examples and integration tests