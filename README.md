# retry

A simple, reliable command-line tool for retrying commands until they succeed. Built with Go and designed to be your friendly companion when dealing with flaky commands, network requests, or any process that might need a second (or third, or fourth) chance.

## Why retry?

We've all been there – a deployment script fails because of a temporary network hiccup, a test flakes out randomly, or an API call times out just when you need it most. Instead of manually running the same command over and over, let `retry` handle the tedious work for you.

## Features

- **Simple and intuitive** – Just prefix your command with `retry`
- **Configurable attempts** – Set how many times to try
- **Smart backoff strategies** – Choose between fixed delays or exponential backoff
- **Timeout protection** – Prevent commands from hanging indefinitely
- **Pattern matching** – Define success/failure based on output patterns, not just exit codes
- **Preserves behavior** – Your command's output and exit codes work exactly as expected
- **Zero dependencies** – Single binary that works anywhere

## Installation

### From Source

```bash
git clone https://github.com/user/retry.git
cd retry
go build -o retry ./cmd/retry
```

### Quick Test

```bash
# Test with a command that always succeeds
./retry -- echo "Hello, World!"

# Test with a command that fails (will retry 3 times by default)
./retry -- false
```

## Basic Usage

The basic syntax is simple: `retry [flags] -- command [args...]`

```bash
# Retry a flaky curl command up to 5 times
retry --attempts 5 -- curl https://api.example.com/status

# Add a 2-second fixed delay between attempts
retry --attempts 3 --delay 2s -- ping -c 1 google.com

# Use exponential backoff (1s, 2s, 4s, 8s...)
retry --attempts 5 --delay 1s --backoff exponential -- flaky-api-call

# Set a timeout for each attempt
retry --timeout 30s -- wget https://large-file.example.com/download

# Combine all options with exponential backoff and max delay
retry --attempts 5 --delay 500ms --backoff exponential --max-delay 10s --timeout 30s -- deployment-script

# Pattern matching - succeed when output contains "success" (even if exit code is non-zero)
retry --success-pattern "deployment successful" -- deploy.sh

# Pattern matching - fail when output contains "error" (even if exit code is zero)
retry --failure-pattern "(?i)error|failed" -- health-check.sh

# Case-insensitive pattern matching
retry --success-pattern "SUCCESS" --case-insensitive -- deployment-script

## Pattern Matching

Many real-world commands don't use exit codes properly. A deployment script might print "deployment successful" but exit with code 1, or a health check might exit with code 0 but print "Error: service unavailable". Pattern matching solves this by letting you define success and failure based on the command's output.

### Success Patterns

Use `--success-pattern` to define when a command should be considered successful, regardless of exit code:

```bash
# Deployment tools that don't use exit codes properly
retry --success-pattern "deployment successful" -- kubectl apply -f deployment.yaml

# API responses that indicate success
retry --success-pattern "\"status\":\"ok\"" -- curl -s https://api.example.com/status

# Multiple success indicators (regex OR)
retry --success-pattern "(success|completed|ready)" -- health-check.sh
```

### Failure Patterns

Use `--failure-pattern` to define when a command should be considered failed, even with exit code 0:

```bash
# Catch error messages in output
retry --failure-pattern "(?i)error|failed|timeout" -- flaky-script.sh

# Specific failure conditions
retry --failure-pattern "connection refused|network unreachable" -- network-test.sh

# JSON error responses
retry --failure-pattern "\"error\":" -- api-call.sh
```

### Pattern Precedence

Patterns are evaluated in this order:
1. **Failure pattern match** → Command fails (exit code 1)
2. **Success pattern match** → Command succeeds (exit code 0)  
3. **Exit code** → Standard behavior (0 = success, non-zero = failure)

### Case-Insensitive Matching

Add `--case-insensitive` to make pattern matching case-insensitive:

```bash
# Matches "SUCCESS", "success", "Success", etc.
retry --success-pattern "success" --case-insensitive -- deployment.sh
```

### Regex Support

Both success and failure patterns support full regex syntax:

```bash
# Match specific formats
retry --success-pattern "build #\d+ completed" -- build-script.sh

# Word boundaries
retry --failure-pattern "\berror\b" -- log-parser.sh

# Capture groups and alternatives
retry --success-pattern "(deployed|updated) successfully" -- deploy.sh
```

## Command-Line Options

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--attempts` | `-a` | `3` | Maximum number of attempts |
| `--delay` | `-d` | `0` | Base delay between attempts (e.g., `1s`, `500ms`) |
| `--backoff` | | `fixed` | Backoff strategy: `fixed` or `exponential` |
| `--multiplier` | | `2.0` | Multiplier for exponential backoff |
| `--max-delay` | | `0` | Maximum delay for exponential backoff (0 = no limit) |
| `--timeout` | `-t` | `0` | Timeout per attempt (e.g., `30s`, `5m`) |
| `--success-pattern` | | | Regex pattern indicating success in stdout/stderr |
| `--failure-pattern` | | | Regex pattern indicating failure in stdout/stderr |
| `--case-insensitive` | | `false` | Make pattern matching case-insensitive |
| `--help` | `-h` | | Show help information |

## How It Works

1. **Run your command** – `retry` executes your command exactly as you would
2. **Check the result** – Determine success using pattern matching (if configured) or exit code
3. **Pattern precedence** – Failure patterns override success patterns, which override exit codes
4. **Calculate delay** – Use fixed delay or exponential backoff based on attempt number
5. **Wait and retry** – If it fails, wait for the calculated delay and try again
6. **Respect limits** – Stop after the maximum number of attempts or max delay reached
7. **Preserve exit codes** – The final exit code matches your command's result

## Exit Codes

- **0** – Command succeeded within the retry limit (by exit code or success pattern)
- **1** – Command failed due to failure pattern match
- **Non-zero** – Command failed after all retry attempts (matches the command's exit code)

## Examples

Check out [examples.md](examples.md) for real-world usage scenarios and common patterns.

## Development

This project follows Test-Driven Development (TDD) principles and is built incrementally. The codebase includes:

- **Comprehensive test coverage** – Unit tests for all core functionality
- **Integration tests** – End-to-end CLI testing
- **Clean architecture** – Modular design with clear separation of concerns

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run CLI integration tests
go test ./cmd/retry -v
```

### Building

```bash
# Build for current platform
go build -o retry ./cmd/retry

# Build for multiple platforms
GOOS=linux GOARCH=amd64 go build -o retry-linux ./cmd/retry
GOOS=darwin GOARCH=amd64 go build -o retry-darwin ./cmd/retry
GOOS=windows GOARCH=amd64 go build -o retry.exe ./cmd/retry
```

## Architecture

The project is organized into clean, testable packages:

- `cmd/retry` – CLI interface using Cobra
- `pkg/executor` – Core retry logic and command execution
- `pkg/backoff` – Backoff strategies (fixed delay and exponential backoff)
- `pkg/conditions` – Pattern matching for success/failure detection

## Contributing

We welcome contributions! The project follows conventional commit messages and maintains high test coverage. Feel free to:

- Report bugs or suggest features via issues
- Submit pull requests with tests
- Improve documentation
- Share your use cases

## License

MIT License – see LICENSE file for details.

## Acknowledgments

Built with:
- [Cobra](https://github.com/spf13/cobra) for CLI framework
- [Testify](https://github.com/stretchr/testify) for testing utilities
- The Go standard library for robust, concurrent execution

---

*Happy retrying! 🔄*