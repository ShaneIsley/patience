# retry

A simple, reliable command-line tool for retrying commands until they succeed. Built with Go and designed to be your friendly companion when dealing with flaky commands, network requests, or any process that might need a second (or third, or fourth) chance.

## Why retry?

We've all been there – a deployment script fails because of a temporary network hiccup, a test flakes out randomly, or an API call times out just when you need it most. Instead of manually running the same command over and over, let `retry` handle the tedious work for you.

## Features

- **Simple and intuitive** – Just prefix your command with `retry`
- **Configurable attempts** – Set how many times to try
- **Smart delays** – Add fixed delays between attempts to avoid overwhelming services
- **Timeout protection** – Prevent commands from hanging indefinitely
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

# Add a 2-second delay between attempts
retry --attempts 3 --delay 2s -- ping -c 1 google.com

# Set a timeout for each attempt
retry --timeout 30s -- wget https://large-file.example.com/download

# Combine all options
retry --attempts 5 --delay 1s --timeout 10s -- your-flaky-command
```

## Command-Line Options

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--attempts` | `-a` | `3` | Maximum number of attempts |
| `--delay` | `-d` | `0` | Fixed delay between attempts (e.g., `1s`, `500ms`) |
| `--timeout` | `-t` | `0` | Timeout per attempt (e.g., `30s`, `5m`) |
| `--help` | `-h` | | Show help information |

## How It Works

1. **Run your command** – `retry` executes your command exactly as you would
2. **Check the result** – If it succeeds (exit code 0), we're done!
3. **Wait and retry** – If it fails, wait for the specified delay and try again
4. **Respect limits** – Stop after the maximum number of attempts
5. **Preserve exit codes** – The final exit code matches your command's result

## Exit Codes

- **0** – Command succeeded within the retry limit
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
- `pkg/backoff` – Delay strategies (currently fixed delay, extensible for exponential backoff)

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