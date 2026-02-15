# AGENTS.md

This file provides development guidelines for AI agents working in this repository.

For detailed development guidelines, see [Development-Guidelines.md](Development-Guidelines.md).

## Build, Lint, and Test

- **Build:** `make build` or `go build -o patience ./cmd/patience`
- **Test:** `make test` or `go test ./...`
- **Run a single test:** `go test -v -run ^TestMyFunction$ ./path/to/package`
- **Test with race detection:** `go test -race ./...`
- **Lint:** `golangci-lint run --config=.golangci.yml`
- **Format:** `gofmt -w .` and `goimports -w .` (or run `pre-commit run --all-files`)

## CLI Architecture

The CLI uses a strategy-based subcommand architecture:

```bash
patience STRATEGY [OPTIONS] -- COMMAND [ARGS...]
```

Available strategies (with aliases): `http-aware` (`ha`), `exponential` (`exp`), `linear` (`lin`), `fixed` (`fix`), `jitter` (`jit`), `decorrelated-jitter` (`dj`), `fibonacci` (`fib`), `polynomial` (`poly`), `adaptive` (`adapt`), `diophantine` (`dio`).

## Code Style and Conventions

- **Imports:** Use `goimports` for formatting. No blank or dot imports.
- **Formatting:** Adhere to `gofmt` standards. Max line length is 120 characters.
- **Types:** Avoid `interface{}` where possible. Use specific types.
- **Naming:** Follow standard Go conventions (e.g., `camelCase` for private, `PascalCase` for public). Receivers should be named consistently. Error variables should be prefixed with `err`.
- **Error Handling:** Check all errors. Use `errors.As` and `errors.Is` for specific error types.
- **Complexity:** Keep functions under 120 lines and cyclomatic complexity below 15.
- **Constants:** Avoid magic numbers; define them as constants in `pkg/executor/constants.go`.
- **Concurrency:** Use `sync.Mutex` or `sync.RWMutex` to protect shared resources.
- **Pre-commit:** This project uses pre-commit hooks. Install with `pre-commit install`.

## Project Structure

- `/cmd/patience` - Main CLI with Cobra subcommand architecture
- `/cmd/patienced` - Optional metrics daemon
- `/pkg/backoff` - All backoff strategies (implement `backoff.Strategy` interface)
- `/pkg/executor` - Core retry logic and command execution
- `/pkg/config` - Configuration loading and validation
- `/pkg/conditions` - Pattern matching for success/failure detection
- `/pkg/metrics` - Metrics collection and daemon communication
- `/pkg/ui` - Terminal output and status reporting
- `/pkg/daemon` - Daemon server implementation
- `/pkg/storage` - In-memory metrics storage
