# AGENTS.md

This file provides development guidelines for AI agents working in this repository.

## Build, Lint, and Test

- **Build:** `make build`
- **Test:** `make test`
- **Run a single test:** `go test -v -run ^TestMyFunction$ ./path/to/package`
- **Lint:** `golangci-lint run --config=.golangci.yml`
- **Format:** `gofmt -w .` and `goimports -w .` (or run `pre-commit run --all-files`)

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
