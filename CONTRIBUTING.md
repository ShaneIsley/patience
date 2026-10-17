# Contributing

patience is deliberately small: one command, four backoff schedules, pattern matching, timeouts.
A change that adds a concept needs a use case that the existing flags cannot express; see
`PLAN.md` §1 for the reasoning behind the current surface and the features that were removed.

## Workflow

```
make test      # go test ./...   (no network, no fixed paths, builds into a temp dir)
make race      # go test -race -count=3 ./...
make lint      # gofmt, go vet, staticcheck
make doctest   # runs every ```sh block in README.md against the built binary
```

CI runs gofmt, vet, the race suite and the README test on Linux and macOS; golangci-lint,
staticcheck and govulncheck on Linux; and cross-builds linux/darwin × amd64/arm64 with
`CGO_ENABLED=0`.

## Rules

- Every behaviour the README states has a test. Every README example is a `sh` block that
  `TestReadme` executes, with `# expect: exit N` when the exit code is not 0; illustrative
  commands that cannot run in CI go in `text` blocks.
- Tests must not use the network, fixed `/tmp` paths, or build into the source tree.
- Timing assertions use generous bounds (≥ 5× the expected value) and are run 200 times
  before merging: `go test -count=200 -run 'TestTimeout|TestMaxTime' ./internal/run`.
- No cgo, no new dependencies without a reason stated in the PR.
- `CHANGELOG.md` gets a line for every user-visible change.

## Layout

```
cmd/patience/       flag parsing, env defaults, exit-code mapping, CLI and README tests
internal/backoff/   delay schedules and jitter (pure functions, injectable RNG)
internal/run/       process execution, output streaming and pattern scan, signals, timeouts
internal/ui/        stderr progress
```

Packages are `internal/`; nothing is a public API.
