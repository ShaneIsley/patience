# patience

A modern, intelligent command-line tool for practicing patience with adaptive backoff strategies. Built with Go and designed to be your patient companion when dealing with flaky commands, network requests, or any process that might need a second (or third, or fourth) chance.

**Author:** Shane Isley  
**Repository:** [github.com/shaneisley/patience](https://github.com/shaneisley/patience)  
**License:** MIT

## Why patience?

We've all been there – a deployment script fails because of a temporary network hiccup, a test flakes out randomly, or an API call times out just when you need it most. Instead of manually running the same command over and over, let `patience` handle the tedious work for you with grace and wisdom.

## Features

- **Strategy-based interface** – Choose the right backoff strategy for your use case
- **HTTP-aware patience** – Respects `Retry-After` headers and server timing hints
- **10 backoff strategies** – From simple fixed delays to machine learning adaptive strategies
- **Intelligent pattern matching** – Define success/failure based on output patterns, not just exit codes
- **Timeout protection** – Prevent commands from hanging indefinitely
- **Preserves behavior** – Your command's output and exit codes work exactly as expected
- **Zero dependencies** – Single binary that works anywhere
- **Metrics Daemon (Optional)** – Collect and visualize patience metrics with the [`patienced` daemon](DAEMON.md)

## Documentation

📚 **Complete Documentation Suite:**
- **[Quick Start](#quick-start)** - Get running in 2 minutes
- **[Migration Guide](#migration-guide)** - Switch from other retry tools
- **[examples.md](examples.md)** - Real-world usage scenarios and patterns
- **[Architecture.md](Architecture.md)** - System design and technical decisions
- **[DAEMON.md](DAEMON.md)** - Optional metrics collection and monitoring
- **[Development-Guidelines.md](Development-Guidelines.md)** - TDD process and contribution standards
- **[DOCUMENTATION.md](DOCUMENTATION.md)** - Documentation maintenance and standards

## Quick Start

Get up and running with patience in under 2 minutes:

### 1. Install
```bash
git clone https://github.com/shaneisley/patience.git
cd patience
go build -o patience ./cmd/patience
```

### 2. Try It Out
```bash
# Basic retry with exponential backoff
./patience exponential -- curl https://httpbin.org/status/500

# HTTP-aware retry (respects server timing)
./patience http-aware -- curl -i https://httpbin.org/delay/2

# Success! Your command now has patience
```

### 3. Common Use Cases

**API Calls with Rate Limiting:**
```bash
patience http-aware -- curl -H "Authorization: Bearer $TOKEN" https://api.github.com/user
```

**Database Connections:**
```bash
patience linear --increment 2s --max-delay 30s -- psql -h db.example.com -c "SELECT 1"
```

**Deployment Scripts:**
```bash
patience exponential --success-pattern "deployment successful" -- kubectl apply -f app.yaml
```

**Flaky Tests:**
```bash
patience fixed --attempts 5 --delay 1s -- npm test
```

### 4. Next Steps
- See [Strategy Details](#strategy-details) for choosing the right backoff strategy
- Check [Pattern Matching](#pattern-matching) for advanced success/failure detection
- Explore [Configuration](#configuration) for persistent settings

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

# Test with a command that fails (will have patience 3 times by default)
./patience exponential -- false

# Test HTTP-aware strategy
./patience http-aware -- curl -i https://httpbin.org/status/200
```

## Basic Usage

The basic syntax is: `patience STRATEGY [OPTIONS] -- COMMAND [ARGS...]`

### Quick Start Examples

```bash
# HTTP-aware patience for API calls (respects Retry-After headers)
patience http-aware -- curl -i https://api.github.com/user

# Exponential backoff with custom parameters
patience exponential --base-delay 1s --multiplier 2.0 -- curl https://api.stripe.com

# Linear backoff for database connections
patience linear --increment 2s --max-delay 30s -- psql -h db.example.com

# Fixed delay for simple patience
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
| `fixed` | `fix` | Fixed delay between patience | Simple patience, testing |
| `jitter` | `jit` | Random jitter around exponential | Distributed systems, load balancing |
| `decorrelated-jitter` | `dj` | AWS-style decorrelated jitter | High-scale distributed systems |
| `fibonacci` | `fib` | Fibonacci sequence delays | Moderate growth, gradual recovery |
| `polynomial` | `poly` | Polynomial growth with configurable exponent | Customizable growth patterns |
| `adaptive` | `adapt` | Machine learning adaptive strategy | Commands with changing patterns |

### Common Options (Available for All Strategies)

```bash
# Set maximum patience attempts
patience exponential --attempts 5 -- command

# Add timeout per attempt
patience linear --timeout 30s -- command

# Pattern matching - succeed when output contains pattern
patience fixed --success-pattern "deployment successful" -- deploy.sh

# Pattern matching - fail when output contains pattern
patience exponential --failure-pattern "(?i)error|failed" -- health-check.sh

# Case-insensitive pattern matching
patience http-aware --success-pattern "SUCCESS" --case-insensitive -- deployment-script
```

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

The HTTP-aware strategy is patience's flagship feature - it intelligently parses HTTP responses to determine optimal patience timing.

```bash
# Basic HTTP-aware patience
patience http-aware -- curl -i https://api.github.com/user

# With fallback strategy when no HTTP info available
patience http-aware --fallback exponential -- curl https://api.example.com

# Set maximum delay cap
patience http-aware --max-delay 5m -- curl https://api.slow-service.com
```

**How it works:**
- Parses `Retry-After` headers from HTTP responses
- Extracts patience timing from JSON responses (`retry_after`, `retryAfter` fields)
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
patience linear --increment 2s -- gradual-patience

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
patience fibonacci --base-delay 1s -- moderate-growth-patience
```

#### Polynomial Backoff (`polynomial`, `poly`)
Uses polynomial growth with configurable exponent for fine-tuned delay patterns.

```bash
# Quadratic backoff (1s, 4s, 9s, 16s...)
patience polynomial --base-delay 1s --exponent 2.0 -- database-connection

# Moderate growth (1s, 2.8s, 5.2s, 8s...)
patience polynomial --base-delay 1s --exponent 1.5 -- api-call

# Gentle sublinear growth
patience polynomial --base-delay 1s --exponent 0.8 -- frequent-operation
```

#### Adaptive Strategy (`adaptive`, `adapt`)
Machine learning-inspired strategy that learns from success/failure patterns to optimize timing.

```bash
# Basic adaptive with exponential fallback
patience adaptive --learning-rate 0.1 --memory-window 50 -- flaky-service

# Fast learning for rapidly changing conditions
patience adaptive --learning-rate 0.5 --fallback fixed -- dynamic-api

# Conservative learning with large memory
patience adaptive --learning-rate 0.05 --memory-window 200 -- database-operation
```

### Strategy Comparison

| Strategy | Growth Pattern | Use Case | Example Delays (1s base) |
|----------|----------------|----------|---------------------------|
| `http-aware` | Server-directed | HTTP APIs, web services | Varies based on server response |
| `exponential` | Exponential | Network calls, APIs | 1s, 2s, 4s, 8s |
| `linear` | Linear | Rate-limited APIs | 1s, 2s, 3s, 4s |
| `fixed` | Constant | Simple patience, testing | 1s, 1s, 1s, 1s |
| `jitter` | Random exponential | Distributed systems | 0.3s, 1.8s, 0.9s, 5.2s |
| `decorrelated-jitter` | Smart random | AWS services, high-scale | 1.2s, 2.8s, 1.9s, 4.1s |
| `fibonacci` | Fibonacci | Moderate growth | 1s, 1s, 2s, 3s, 5s, 8s |
| `polynomial` | Polynomial | Customizable growth | 1s, 4s, 9s, 16s (exponent=2.0) |
| `adaptive` | Learning-based | Changing patterns | Adapts based on success/failure |

## Configuration

### Configuration Files

`patience` supports configuration files for setting default values. Configuration files use TOML format and follow this precedence order:

1. **CLI flags** (highest priority)
2. **Environment variables** 
3. **Configuration file**
4. **Default values** (lowest priority)

#### Auto-discovery

`patience` automatically looks for configuration files in the current directory:
- `.patience.toml`
- `patience.toml`

#### Manual Configuration

Use the `--config` flag to specify a configuration file:

```bash
patience exponential --config /path/to/config.toml -- command
```

#### Example Configuration File

```toml
# .patience.toml
attempts = 5
timeout = "30s"
success_pattern = "deployment successful|build completed"
failure_pattern = "(?i)error|failed|timeout"
case_insensitive = true

# Strategy-specific settings (used when no CLI flags provided)
base_delay = "1s"
multiplier = 2.0
max_delay = "10s"
```

### Environment Variables

All configuration options can be set via environment variables with the `PATIENCE_` prefix:

```bash
export PATIENCE_ATTEMPTS=5
export PATIENCE_TIMEOUT=30s
export PATIENCE_SUCCESS_PATTERN="deployment successful"
export PATIENCE_FAILURE_PATTERN="(?i)error|failed"
export PATIENCE_CASE_INSENSITIVE=true

# Strategy-specific variables
export PATIENCE_BASE_DELAY=1s
export PATIENCE_MULTIPLIER=2.0
export PATIENCE_MAX_DELAY=10s

patience exponential -- command
```

### Debug Configuration

Use `--debug-config` to see how configuration values are resolved:

```bash
patience exponential --debug-config -- command
```

This shows the source of each configuration value (CLI flag, environment variable, config file, or default).

## Command-Line Options

### Common Options (Available for All Strategies)

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--attempts` | `-a` | `3` | Maximum number of attempts (1-1000) |
| `--timeout` | `-t` | `0` | Timeout per attempt (e.g., `30s`, `5m`). Note: ~10-20ms overhead |
| `--success-pattern` | | | Regex pattern indicating success in stdout/stderr |
| `--failure-pattern` | | | Regex pattern indicating failure in stdout/stderr |
| `--case-insensitive` | | `false` | Make pattern matching case-insensitive |
| `--config` | | | Configuration file path |
| `--debug-config` | | `false` | Show configuration debug information |
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
| `--base-delay` | `-b` | `1s` | Base delay for first patience |
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

#### Polynomial Strategy
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--base-delay` | `-b` | `1s` | Base delay for polynomial calculation |
| `--exponent` | `-e` | `2.0` | Polynomial exponent (controls growth rate) |
| `--max-delay` | `-m` | `60s` | Maximum delay cap |

#### Adaptive Strategy
| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--learning-rate` | `-r` | `0.1` | Learning rate for adaptation (0.01-1.0) |
| `--memory-window` | `-w` | `50` | Number of recent outcomes to remember (5-10000) |
| `--fallback` | `-f` | `exponential` | Fallback strategy when learning data insufficient |

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
- **Non-zero** – Command failed after all patience attempts (matches the command's final exit code)

**Note:** `patience` exits with the result of the first successful attempt, not the last attempt.

## Behavior

**Important:** `patience` stops immediately when a command succeeds - it does not execute remaining attempts.

- ✅ **Exits on first success** - If attempt 1 succeeds, attempts 2-N are never executed
- 🔄 **Only has patience on failure** - Success means the job is complete
- 📊 **Preserves exit codes** - Your command's original behavior is maintained
- ⏱️ **Efficient execution** - No wasted time on unnecessary attempts

### Examples:
```bash
# If API is up on attempt 1, attempts 2-5 are skipped
patience exponential --attempts 5 -- curl https://api.example.com/health

# Only has patience while the service is starting up
patience linear --attempts 10 --increment 1s -- nc -z localhost 8080

# This stops immediately if the first curl succeeds
patience http-aware --attempts 5 -- curl https://api.example.com
# Output: "✅ Command succeeded after 1 attempt" (attempts 2-5 never run)
```

## Migration Guide

Switching from other retry tools? Here's how to migrate common patterns to patience:

### From `retry` (bash script)

**Old:**
```bash
retry -t 5 -d 2 curl https://api.example.com
```

**New:**
```bash
patience fixed --attempts 5 --delay 2s -- curl https://api.example.com
```

### From `retries` (Python)

**Old:**
```python
@retries(max_attempts=3, delay=1, backoff=2)
def api_call():
    return requests.get('https://api.example.com')
```

**New:**
```bash
patience exponential --attempts 3 --base-delay 1s --multiplier 2 -- curl https://api.example.com
```

### From `exponential-backoff` (npm)

**Old:**
```bash
exponential-backoff --initial-delay 1000 --max-delay 30000 -- curl api.com
```

**New:**
```bash
patience exponential --base-delay 1s --max-delay 30s -- curl api.com
```

### From `while` loops

**Old:**
```bash
while ! curl https://api.example.com; do
  echo "Retrying in 5 seconds..."
  sleep 5
done
```

**New:**
```bash
patience fixed --delay 5s -- curl https://api.example.com
```

### From AWS CLI retry

**Old:**
```bash
aws s3 cp file.txt s3://bucket/ --cli-read-timeout 0 --cli-connect-timeout 60
```

**New:**
```bash
patience exponential --timeout 60s -- aws s3 cp file.txt s3://bucket/
```

### From `timeout` + manual retry

**Old:**
```bash
for i in {1..3}; do
  timeout 30 command && break
  sleep $((i * 2))
done
```

**New:**
```bash
patience exponential --attempts 3 --base-delay 2s --timeout 30s -- command
```

### Key Advantages of patience

1. **HTTP Intelligence**: Automatically respects `Retry-After` headers
2. **Pattern Matching**: Success/failure based on output, not just exit codes
3. **Strategy Variety**: 10 different backoff strategies for different use cases
4. **Real-time Feedback**: Clear progress reporting during retries
5. **Configuration**: Persistent settings via config files
6. **Metrics**: Optional daemon for long-term retry analytics

## Examples

Check out [examples.md](examples.md) for real-world usage scenarios and common patterns.

## Development

This project follows Test-Driven Development (TDD) principles and is built incrementally. The codebase includes:

- **Comprehensive test coverage** – Unit tests for all core functionality
- **Integration tests** – End-to-end CLI testing
- **Clean architecture** – Modular design with clear separation of concerns

See [Development-Guidelines.md](Development-Guidelines.md) for details on our TDD process and code style.

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
- `pkg/executor` – Core patience logic and command execution
- `pkg/backoff` – Backoff strategies including HTTP-aware intelligence
- `pkg/conditions` – Pattern matching for success/failure detection
- `pkg/metrics` – Metrics collection and daemon communication
- `pkg/ui` – Terminal output and status reporting
- `pkg/config` – Configuration loading and validation

See [Architecture.md](Architecture.md) for a detailed breakdown of the components.

## Contributing

We welcome contributions! The project follows conventional commit messages and maintains high test coverage. Feel free to:

- Report bugs or suggest features via [GitHub Issues](https://github.com/shaneisley/patience/issues)
- Submit pull requests with tests
- Improve documentation
- Share your use cases

**Documentation:**
- [Development Guidelines](Development-Guidelines.md) - TDD process, code style, and contribution standards
- [Architecture](Architecture.md) - System design and technical decisions
- [Documentation Maintenance](DOCUMENTATION.md) - How to maintain and improve documentation
- [Daemon Setup](DAEMON.md) - Optional metrics collection and monitoring

**Getting Started:**
- See [Development](#development) section for setup instructions
- Check [examples.md](examples.md) for real-world usage patterns
- Review existing tests for contribution examples

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
