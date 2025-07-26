# patience

A modern, intelligent command-line tool for retrying commands with adaptive backoff strategies. Built with Go and designed to be your patient companion when dealing with flaky commands, network requests, or any process that might need a second (or third, or fourth) chance.

**Author:** Shane Isley  
**Repository:** [github.com/shaneisley/patience](https://github.com/shaneisley/patience)  
**License:** MIT

## Why patience?

We've all been there – a deployment script fails because of a temporary network hiccup, a test flakes out randomly, or an API call times out just when you need it most. Instead of manually running the same command over and over, let `patience` handle the tedious work for you with grace and wisdom.

## Features

- **Strategy-based interface** – Choose the right backoff strategy for your use case
- **HTTP-aware retries** – Respects `Retry-After` headers and server timing hints
- **7 backoff strategies** – From simple fixed delays to AWS-recommended decorrelated jitter
- **Intelligent pattern matching** – Define success/failure based on output patterns, not just exit codes
- **Timeout protection** – Prevent commands from hanging indefinitely
- **Preserves behavior** – Your command's output and exit codes work exactly as expected
- **Zero dependencies** – Single binary that works anywhere

## Installation

### From Source

```bash
git clone https://github.com/shaneisley/patience.git
cd patience
go build -o patience ./cmd/patience
```

### Quick Test

```bash
# Test with a command that always succeeds
./patience fixed -- echo "Hello, World!"

# Test with a command that fails (will retry 3 times by default)
./patience exponential -- false

# Test HTTP-aware strategy
./patience http-aware -- curl -i https://httpbin.org/status/200
```

## Basic Usage

The basic syntax is: `patience STRATEGY [OPTIONS] -- COMMAND [ARGS...]`

### Quick Start Examples

```bash
# HTTP-aware retry for API calls (respects Retry-After headers)
patience http-aware -- curl -i https://api.github.com/user

# Exponential backoff with custom parameters
patience exponential --base-delay 1s --multiplier 2.0 -- curl https://api.stripe.com

# Linear backoff for database connections
patience linear --increment 2s --max-delay 30s -- psql -h db.example.com

# Fixed delay for simple retries
patience fixed --delay 5s -- flaky-script.sh

# Using abbreviations for brevity
patience ha -f exp -- curl -i https://api.github.com
patience exp -b 1s -x 2.0 -- curl https://api.stripe.com
```

### Available Strategies

| Strategy | Alias | Description | Best For |
|----------|-------|-------------|----------|
| `http-aware` | `ha` | Respects HTTP `Retry-After` headers | API calls, HTTP requests |
| `exponential` | `exp` | Exponentially increasing delays | Network operations, external services |
| `linear` | `lin` | Linearly increasing delays | Rate-limited APIs, predictable timing |
| `fixed` | `fix` | Fixed delay between retries | Simple retries, testing |
| `jitter` | `jit` | Random jitter around exponential | Distributed systems, load balancing |
| `decorrelated-jitter` | `dj` | AWS-style decorrelated jitter | High-scale distributed systems |
| `fibonacci` | `fib` | Fibonacci sequence delays | Moderate growth, gradual recovery |

### Common Options (Available for All Strategies)

```bash
# Set maximum retry attempts
patience exponential --attempts 5 -- command

# Add timeout per attempt
patience linear --timeout 30s -- command

# Pattern matching - succeed when output contains pattern
patience fixed --success-pattern "deployment successful" -- deploy.sh

# Pattern matching - fail when output contains pattern
patience exponential --failure-pattern "(?i)error|failed" -- health-check.sh

# Case-insensitive pattern matching
patience http-aware --success-pattern "SUCCESS" --case-insensitive -- deployment-script

## Pattern Matching

Many real-world commands don't use exit codes properly. A deployment script might print "deployment successful" but exit with code 1, or a health check might exit with code 0 but print "Error: service unavailable". Pattern matching solves this by letting you define success and failure based on the command's output.

### Success Patterns

Use `--success-pattern` to define when a command should be considered successful, regardless of exit code:

```bash
# Deployment tools that don't use exit codes properly
patience --success-pattern "deployment successful" -- kubectl apply -f deployment.yaml

# API responses that indicate success
patience --success-pattern "\"status\":\"ok\"" -- curl -s https://api.example.com/status

# Multiple success indicators (regex OR)
patience --success-pattern "(success|completed|ready)" -- health-check.sh
```

### Failure Patterns

Use `--failure-pattern` to define when a command should be considered failed, even with exit code 0:

```bash
# Catch error messages in output
patience --failure-pattern "(?i)error|failed|timeout" -- flaky-script.sh

# Specific failure conditions
patience --failure-pattern "connection refused|network unreachable" -- network-test.sh

# JSON error responses
patience --failure-pattern "\"error\":" -- api-call.sh
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
patience --success-pattern "success" --case-insensitive -- deployment.sh
```

### Regex Support

Both success and failure patterns support full regex syntax:

```bash
# Match specific formats
patience --success-pattern "build #\d+ completed" -- build-script.sh

# Word boundaries
patience --failure-pattern "\berror\b" -- log-parser.sh

# Capture groups and alternatives
patience --success-pattern "(deployed|updated) successfully" -- deploy.sh
```

## Strategy Details

### HTTP-Aware Strategy (`http-aware`, `ha`)

The HTTP-aware strategy is patience's flagship feature - it intelligently parses HTTP responses to determine optimal retry timing.

```bash
# Basic HTTP-aware retry
patience http-aware -- curl -i https://api.github.com/user

# With fallback strategy when no HTTP info available
patience http-aware --fallback exponential -- curl https://api.example.com

# Set maximum delay cap
patience http-aware --max-delay 5m -- curl https://api.slow-service.com
```

**How it works:**
- Parses `Retry-After` headers from HTTP responses
- Extracts retry timing from JSON responses (`retry_after`, `retryAfter` fields)
- Falls back to specified strategy when no HTTP timing information is available
- Validated with 7 major APIs: GitHub, Twitter, AWS, Stripe, Discord, Reddit, Slack

### Mathematical Strategies

#### Exponential Backoff (`exponential`, `exp`)
Doubles the delay after each failed attempt - industry standard for network operations.

```bash
# Basic exponential backoff (1s, 2s, 4s, 8s...)
patience exponential --base-delay 1s -- api-call

# Custom multiplier (1s, 1.5s, 2.25s, 3.375s...)
patience exponential --base-delay 1s --multiplier 1.5 -- api-call

# With maximum delay cap
patience exponential --base-delay 1s --max-delay 10s -- api-call
```

#### Linear Backoff (`linear`, `lin`)
Increases delay by a fixed increment each attempt - predictable timing.

```bash
# Linear progression (2s, 4s, 6s, 8s...)
patience linear --increment 2s -- gradual-retry

# With maximum delay cap
patience linear --increment 1s --max-delay 30s -- rate-limited-api
```

#### Fixed Delay (`fixed`, `fix`)
Waits the same amount of time between each attempt - simple and predictable.

```bash
# Wait 3 seconds between each attempt
patience fixed --delay 3s -- flaky-command
```

#### Jitter (`jitter`, `jit`)
Adds randomness to exponential backoff to prevent thundering herd problems.

```bash
# Random delays between 0 and exponential backoff time
patience jitter --base-delay 1s --multiplier 2.0 -- distributed-api-call
```

#### Decorrelated Jitter (`decorrelated-jitter`, `dj`)
AWS-recommended strategy that uses the previous delay to calculate the next delay.

```bash
# Smart jitter based on previous delay
patience decorrelated-jitter --base-delay 1s --multiplier 3.0 -- aws-api-call
```

#### Fibonacci Backoff (`fibonacci`, `fib`)
Uses the Fibonacci sequence for delays - moderate growth between linear and exponential.

```bash
# Fibonacci sequence delays (1s, 1s, 2s, 3s, 5s, 8s...)
patience fibonacci --base-delay 1s -- moderate-growth-retry
```

### Strategy Comparison

| Strategy | Growth Pattern | Use Case | Example Delays (1s base) |
|----------|----------------|----------|---------------------------|
| `http-aware` | Server-directed | HTTP APIs, web services | Varies based on server response |
| `exponential` | Exponential | Network calls, APIs | 1s, 2s, 4s, 8s |
| `linear` | Linear | Rate-limited APIs | 1s, 2s, 3s, 4s |
| `fixed` | Constant | Simple retries, testing | 1s, 1s, 1s, 1s |
| `jitter` | Random exponential | Distributed systems | 0.3s, 1.8s, 0.9s, 5.2s |
| `decorrelated-jitter` | Smart random | AWS services, high-scale | 1.2s, 2.8s, 1.9s, 4.1s |
| `fibonacci` | Fibonacci | Moderate growth | 1s, 1s, 2s, 3s, 5s, 8s |

## Command-Line Options

### Common Options (Available for All Strategies)

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--attempts` | `-a` | `3` | Maximum number of attempts (1-1000) |
| `--timeout` | `-t` | `0` | Timeout per attempt (e.g., `30s`, `5m`) |
| `--success-pattern` | | | Regex pattern indicating success in stdout/stderr |
| `--failure-pattern` | | | Regex pattern indicating failure in stdout/stderr |
| `--case-insensitive` | | `false` | Make pattern matching case-insensitive |
| `--help` | `-h` | | Show help information |

### Strategy-Specific Options

#### HTTP-Aware Strategy
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--fallback` | `-f` | `exponential` | Fallback strategy when no HTTP info available |
| `--max-delay` | `-m` | `30m` | Maximum delay cap |

#### Exponential Strategy
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--base-delay` | `-b` | `1s` | Base delay for first retry |
| `--multiplier` | `-x` | `2.0` | Multiplier for exponential growth |
| `--max-delay` | `-m` | `60s` | Maximum delay cap |

#### Linear Strategy
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--increment` | `-i` | `1s` | Delay increment per attempt |
| `--max-delay` | `-m` | `60s` | Maximum delay cap |

#### Fixed Strategy
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--delay` | `-d` | `1s` | Fixed delay between attempts |

#### Jitter Strategy
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--base-delay` | `-b` | `1s` | Base delay for calculations |
| `--multiplier` | `-x` | `2.0` | Multiplier for jitter range |
| `--max-delay` | `-m` | `60s` | Maximum delay cap |

#### Decorrelated Jitter Strategy
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--base-delay` | `-b` | `1s` | Base delay for calculations |
| `--multiplier` | `-x` | `2.0` | Multiplier for jitter calculations |
| `--max-delay` | `-m` | `60s` | Maximum delay cap |

#### Fibonacci Strategy
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--base-delay` | `-b` | `1s` | Base delay for Fibonacci sequence |
| `--max-delay` | `-m` | `60s` | Maximum delay cap |

## How It Works

1. **Run your command** – `patience` executes your command exactly as you would
2. **Check the result** – Determine success using pattern matching (if configured) or exit code
3. **Pattern precedence** – Failure patterns override success patterns, which override exit codes
4. **Exit on success** – If the command succeeds, `patience` exits immediately (remaining attempts are skipped)
5. **Calculate delay** – Use the configured backoff strategy (fixed, exponential, jitter, linear, decorrelated-jitter, or fibonacci) based on attempt number
6. **Wait patiently** – If it fails, wait for the calculated delay and try again with grace
7. **Respect limits** – Stop after the maximum number of attempts or max delay reached
8. **Preserve exit codes** – The final exit code matches your command's result

## Exit Codes

- **0** – Command succeeded on any attempt (remaining attempts skipped)
- **1** – Command failed due to failure pattern match
- **Non-zero** – Command failed after all retry attempts (matches the command's final exit code)

**Note:** `patience` exits with the result of the first successful attempt, not the last attempt.

## Behavior

**Important:** `patience` stops immediately when a command succeeds - it does not execute remaining attempts.

- ✅ **Exits on first success** - If attempt 1 succeeds, attempts 2-N are never executed
- 🔄 **Only retries on failure** - Success means the job is complete
- 📊 **Preserves exit codes** - Your command's original behavior is maintained
- ⏱️ **Efficient execution** - No wasted time on unnecessary attempts

### Examples:
```bash
# If API is up on attempt 1, attempts 2-5 are skipped
patience exponential --attempts 5 -- curl https://api.example.com/health

# Only retries while the service is starting up
patience linear --attempts 10 --increment 1s -- nc -z localhost 8080

# This stops immediately if the first curl succeeds
patience http-aware --attempts 5 -- curl https://api.example.com
# Output: "✅ Command succeeded after 1 attempt" (attempts 2-5 never run)
```

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
go test ./cmd/patience -v

# Run HTTP-aware strategy tests
go test ./pkg/backoff -v -run TestHTTPAware
```

### Building

```bash
# Build for current platform
go build -o patience ./cmd/patience

# Build for multiple platforms
GOOS=linux GOARCH=amd64 go build -o patience-linux ./cmd/patience
GOOS=darwin GOARCH=amd64 go build -o patience-darwin ./cmd/patience
GOOS=windows GOARCH=amd64 go build -o patience.exe ./cmd/patience
```

## Architecture

The project is organized into clean, testable packages:

- `cmd/patience` – CLI interface with subcommand architecture using Cobra
- `pkg/executor` – Core retry logic and command execution
- `pkg/backoff` – Backoff strategies including HTTP-aware intelligence
- `pkg/conditions` – Pattern matching for success/failure detection
- `pkg/metrics` – Metrics collection and daemon communication
- `pkg/ui` – Terminal output and status reporting
- `pkg/config` – Configuration loading and validation

## Contributing

We welcome contributions! The project follows conventional commit messages and maintains high test coverage. Feel free to:

- Report bugs or suggest features via [GitHub Issues](https://github.com/shaneisley/patience/issues)
- Submit pull requests with tests
- Improve documentation
- Share your use cases

### Contact

- **Author:** Shane Isley
- **GitHub:** [@shaneisley](https://github.com/shaneisley)
- **Repository:** [github.com/shaneisley/patience](https://github.com/shaneisley/patience)

## License

MIT License – see LICENSE file for details.

## Acknowledgments

Built with:
- [Cobra](https://github.com/spf13/cobra) for CLI framework
- [Testify](https://github.com/stretchr/testify) for testing utilities
- The Go standard library for robust, concurrent execution

---

*Practice patience! 🧘*