# AGENTS.md

Guidance for AI agents working in this repository. Read `CONTRIBUTING.md` first; it is short.

- Build: `make build` (static, `CGO_ENABLED=0`). Test: `make test`; one test: `go test -run '^TestName$' ./internal/run`.
- Lint: `make lint`. Docs check: `make doctest` executes the README's `sh` blocks.
- The CLI is a single command: `patience [flags] -- COMMAND`. There are no subcommands, no config
  file, no daemon. Do not reintroduce them; `PLAN.md` explains why they were removed.
- Keep `internal/backoff` pure (no clock, no I/O). Keep process handling in `internal/run` only.
- Tests: no network, no fixed `/tmp` paths, no building into the source tree, generous timing bounds.
- Match the existing style: `gofmt`, short doc comments on exported names, errors wrapped with
  context, no `interface{}`.
- Update `CHANGELOG.md` and the README (with an executable example) for any user-visible change.
