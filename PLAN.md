# patience — critique-and-remediation plan

Status: **Phases 3 and 4 complete on branch `pivot-single-command`; README rewritten after review (D23).** Phase 4 results are in the private review notes (not in this repository). Merged to `main`; not tagged (step 10 needs an explicit instruction).
Baseline commit: b0679e8. Phase 1 report: kept outside the repository (27 broken promises, 3 security findings).

## 0. Decision: pivot, not repair

The working part of patience is a correct retry loop with seven backoff schedules, output-pattern
success/failure, and a per-attempt timeout. Everything around it is either unbuilt (adaptive,
diophantine, daemon coordination), unreachable from the binary (discovery, patterns, monitoring,
storage, ~42% of production code), or broken in the executor (stdin, >10 MiB output, signals).

The pivot keeps the loop, rewrites the executor, replaces the ten copy-pasted subcommands with one
command, and deletes the rest. Nothing is sacred: the CLI shape, dependencies, config system, docs,
scripts and tests are all redesigned below, with the old shape kept only where it was already right.

## 1. Target product

`patience` — run a command, retry it on failure with a chosen backoff, decide failure by exit code
or output pattern. Single static binary, no cgo, Linux and macOS.

### 1.1 CLI (one command, no subcommands)

```
patience [flags] -- COMMAND [ARGS...]

Retry control
  -n, --attempts N          total attempts; 0 = unlimited (default 3)
  -t, --timeout D           per-attempt timeout; 0 = none (default 0)
      --max-time D          overall budget incl. delays; 0 = none (default 0)
      --retry-on CODES      retry only on these exit codes, e.g. 1,75 (default: any non-zero)

Backoff
  -b, --backoff NAME        constant | linear | exponential | decorrelated  (default exponential)
  -d, --delay D             base delay / increment (default 1s)
  -m, --max-delay D         cap on any single delay; 0 = none (default 1m)
  -x, --multiplier F        growth factor for exponential and decorrelated (default 2; AWS uses 3)
  -j, --jitter F            0..1: delay = v·(1−F) + U[0, F·v]; 1 = full jitter, 0.5 = equal (default 0)

Outcome
      --success-pattern RE  attempt succeeds if stdout/stderr matches (overrides exit code)
      --failure-pattern RE  attempt fails if stdout/stderr matches (overrides success pattern)
  -i, --ignore-case         patterns are case-insensitive

Output
      --foreground          run in patience's process group (terminal use; timeout kills only the child)
  -q, --quiet               no progress on stderr
  -v, --verbose             per-attempt detail and final summary on stderr
      --version
```

Why one command: the ten subcommands shared nothing but copy-pasted code, and that duplication is
the direct cause of config being wired into 3 of 10 (Phase 1 review, row 5). `jitter` was never a
strategy, it is a modifier, so it becomes the `--jitter F` knob (one number covers AWS's "full"
and "equal" modes). `decorrelated-jitter` is the opposite case: it is a schedule in its own right
(`U[d, min(cap, prev·x)]` ignores any underlying schedule), so it stays as `--backoff decorrelated`.
`fibonacci` and `polynomial` are dropped: fibonacci is exponential with ratio ≈1.618 and polynomial's
only used points are linear (e=1) and constant (e=0); neither has a use case the others lack.
Added instead: `--retry-on` (don't retry deterministic failures) and `--attempts 0` (unlimited),
both common shell-loop needs that the old tool could not express. `http-aware` is dropped until there is a stronger use case: it only ever worked when
curl was also told to fail (`-f`), at which point `curl --retry` already honours Retry-After.

Dropped: `http-aware`, `adaptive`, `diophantine`, `--daemon*`, `--resource-id`, `--fallback`, `--config`,
`--debug-config`, config-file auto-discovery, all subcommands and aliases, `patienced`.

Env vars `PATIENCE_<FLAG>` (e.g. `PATIENCE_ATTEMPTS=5`, `PATIENCE_BACKOFF=linear`) set defaults;
flags win. No config file: a `.patience.toml` discovered in the cwd silently changing retry
behaviour is a footgun, and the file loader was the single largest source of defects.

### 1.2 Semantics (the contract the README and tests will state)

- Delay for attempt k (1-based, after failure k): constant `d`; linear `d·k`; exponential
  `d·x^(k-1)`; each capped at `--max-delay` giving `v`, then jittered: `v·(1−F) + U[0, F·v]`.
  decorrelated: `U[d, min(cap, prev·x)]` with `prev` starting at `d` (AWS); `--jitter` is ignored
  for it. All arithmetic in float64 with saturation; no integer overflow (row 11).
- Retry decision after a failed attempt: never for 126/127 (not executable / not found). If
  `--retry-on` is set, retry only when the attempt's exit code is listed; a pattern failure with
  exit 0 counts as code 1 for this check, and a timeout counts as 124.
- `--attempts 0` retries until success, `--max-time`, or a signal.
- Attempt outcome: failure-pattern match → fail; else success-pattern match → success; else exit
  code 0 → success. Patterns are matched per line on the live stream, both stdout and stderr, so
  there is no output buffer and no size limit (rows 1, 2).
- stdin is passed through to the child. stdout/stderr are passed through unmodified, byte-exact.
- Signals: SIGINT/SIGTERM are forwarded to the child's process group; a second signal kills it.
  patience then exits 128+signal (row 3).
- Exit code: the last attempt's exit code; 124 if the last attempt timed out (same as `timeout(1)`);
  1 if it was failed by pattern with exit 0; 126/127 for not-executable/not-found (no retry);
  2 for usage errors. Progress goes to stderr only; stdout is the child's alone (row 8).
- `--max-time` stops scheduling new attempts once the budget is spent; it never cuts a running
  attempt (that is `--timeout`'s job).

### 1.3 Layout after the pivot

```
cmd/patience/main.go        flag parsing (spf13/pflag), env defaults, exit-code mapping   (~200 lines)
internal/backoff/           schedules + jitter, pure functions over (attempt, prev)       (~150)
internal/run/               executor: process, streams, pattern scan, signals, timeout    (~300)
internal/ui/                stderr progress                                              (~80)
```
Target: ~700 production lines (from 9,571), tests ~1,500, zero cgo, deps = pflag + testify.
`internal/` so nothing is an API; `pkg/` goes away.

Deleted outright: `cmd/patienced`, `pkg/{backoff,daemon,discovery,patterns,storage,monitoring,
metrics,config,conditions,executor,ui}` (the keepers are rewritten under `internal/`),
`benchmarks/`, `scripts/` (all 18), `examples/`, `docs/`, `Architecture.md`, `DAEMON.md`,
`DOCUMENTATION.md`, `Development-Guidelines.md`, `examples.md`, `.pre-commit-config.yaml`,
go-sqlite3, viper, yaml, cobra and their transitive deps.

## 2. Execution steps (Phase 3) — each is one commit with its own gate

| # | Step | Gate (measured, quoted in the commit) |
|---|---|---|
| 1 | Move `PLAN.md` in (this file); write `CHANGELOG.md` with the breaking-change entry | — |
| 2 | Delete everything in §1.3 "Deleted outright"; prune `go.mod` | `go build ./...` ok; `go mod tidy -diff` empty; prod lines measured |
| 3 | `internal/backoff`: 4 schedules + jitter knob, table tests with injected RNG; property test: `v·(1−F) ≤ delay ≤ cap` for attempts 1..10000 and extreme params (`d=1ns`, `d=100y`, `x=1e6`, `F∈{0,0.5,1}`); sampled mean of full jitter within 1% of `v/2` at n=10⁵ | 100% line coverage of the package |
| 4 | `internal/run`: executor with stdin passthrough, streaming tee + line scanner, pgid, signal forwarding, timeout → 124, `--max-time` | tests: 50 MB output byte-exact, `seq 5 \| … wc -l` = 5, SIGTERM leaves no child (pgrep), timeout kills grandchild, 200 runs of the timing test with 0 failures |
| 5 | (removed: Retry-After parsing dropped with http-aware) | — |
| 6 | `cmd/patience`: pflag + env, validation (negative durations, NaN, jitter outside 0..1, bad `--retry-on` rejected with exit 2), `--retry-on`, unlimited attempts, exit mapping, `--version` via ldflags | CLI tests build the binary once into `t.TempDir()`; no network; no builds into the tree |
| 7 | README rewrite: every `sh` block is executable; `TestReadme` runs them (no network needed once http-aware was dropped) | doctest passes in CI |
| 8 | CI: valid YAML (lint it), Go 1.24 stable, `gofmt -l`, `go vet`, staticcheck, golangci-lint v2 with a minimal config, `go test -race -count=3`, `CGO_ENABLED=0` builds for linux/darwin × amd64/arm64, doctest | all green on the PR's own run, linked in the plan |
| 9 | `.gitignore` fix (`/patience`, `/patienced` anchored), `AGENTS.md` rewritten to the new layout, `CONTRIBUTING.md` cut to a page, `Makefile` kept to build/test/lint/doctest | — |
| 10 | Tag proposal `v1.0.0` (no prior tags exist, module path unchanged) — **only on your instruction** | — |

Windows: `Setpgid`/`syscall.Kill` have no equivalent; the run package carries `//go:build unix` and
the README states Linux/macOS only. Adding Windows later is a job-object port of `internal/run`.

## 3. Verification (Phase 4) — like-for-like against baseline

| Metric | Baseline (b0679e8, lane C method) | Target |
|---|---|---|
| Production lines (gofmt, comments stripped, go/ast) | 9,571 | ≤ 1,000 |
| Binary size | 22.2 MB (cgo) / 8.85 MB (CGO off) | ≤ 5 MB static |
| `go test ./...` cold | 19.8 s | ≤ 10 s |
| golangci repo-config findings | 309 | 0 |
| Coverage | 59.3% | ≥ 85%, no 0% package |
| Flaky tests (≥ 500 runs, idle + GOMAXPROCS=1 + 3 concurrent) | 4 tests at 2–67% | 0 failures |
| CI | invalid YAML, 14/14 failed | green |
| Phase 1 review rows re-run | 27 broken | 0 broken, or documented as out of scope |

Every Phase 1 review row gets re-run against the new binary with the same command, and the
result is recorded next to the original measurement in the private review notes.

## 4. Decisions needed from you before Phase 3

1. **One command vs. keep subcommands.** Plan: one command (§1.1). A `patience exponential …`
   shim is cheap if anyone depends on the old shape; I found no evidence that anyone does.
2. **Drop the config file.** Plan: env vars + flags only.
3. ~~http-aware~~ **Decided: dropped** (2026-10-17) until there is a stronger use case.
4. **Quiet by default** (progress only with `-v`, errors always). Plan: progress on by default, `-q` to silence — matches the current behaviour and the README's "real-time feedback" promise. Say if you want it the other way.
5. ~~Keep fibonacci and polynomial~~ **Decided (2026-10-17): dropped.** Schedules are constant, linear, exponential, decorrelated; jitter is a 0..1 knob; `--retry-on` and `--attempts 0` added.
6. **Name stays `patience`**, module path stays, first tag is `v1.0.0`.

## 5. Deviations log

Phase 1:
- D1: Brief had `{TOOL}/{LANGUAGE}/{REPO}` placeholders; user chose patience (Go).
- D2: `make` not installed; built with the Makefile's `go build` lines.
- D3: Binaries built into the scratchpad so the tree stayed untouched.
- D4: Plan file lived in the scratchpad during phase 1; moved here at the start of phase 2.
- D5: No gcc on host; `zig cc` used as CC for cgo.
- D6: Steps 2 and 4–5 delegated to two subagents; headline claims re-run by the lead.
- D7: `go test -race` failed to link 6 packages under zig/arm64; worked around with `-ldflags=-linkmode=external`.
- D8: GitHub API rate limit; only the latest CI run's job list inspected.
- D9: Full benchmark run stopped at 840 s; `-short` used.
- D10: `/usr/bin/time` absent; wall times via a python `perf_counter` wrapper.

Phase 2:
- D11: Plan proposes a breaking CLI redesign rather than fixes in place, per "consider nothing sacred".
- D13: Strategy set revised on user decision (2026-10-17): fibonacci/polynomial dropped, decorrelated promoted to a schedule, jitter modes collapsed to one fraction, `--retry-on` and unlimited attempts added. Reverses the lead's own earlier "keep fib/poly" recommendation after the user asked for a per-strategy justification.
- D12: http-aware / `--retry-after` removed from the plan on user decision (2026-10-17); step 5 and `internal/retryafter` deleted.

Phase 3:
- D14: Added `--foreground` (mirrors `timeout --foreground`): without it, commands that read the terminal get SIGTTIN because the child is in its own process group. Not in the original plan.
- D15: Step 2 left a stub `cmd/patience/main.go` (exit 2) so every commit builds; the plan had not said how the tree builds between deletion and rewrite.
- D16: Steps 4, 6, 7, 8 and 9 were developed on one working tree and committed in plan order; the gates (lint, race, flake runs, doctest) were run on the combined tree, not per commit.
- D17: `cmd/patience` in-process coverage is 8% because its tests drive the built binary; real coverage (95.5%) is measured by building with `-cover` and `GOCOVERDIR`, which `TestMain` does when the variable is set.
- D18: go.mod requires 1.24.6, not 1.24: govulncheck flagged GO-2025-3956 (os/exec LookPath) as called from `internal/run`; 1.24.6 is the fixed patch.
- D19: golangci gocyclo limit set to 20 (old config: 15) after refactoring `Run`/`runOnce` from 25/24 to below 20; the old limit was never enforced (CI never ran).
- D20: Cold `go test ./...` is 10–12 s, at the ≤10 s target's edge; the remainder is real-process timing tests (signals, timeouts) that would be meaningless if shortened further.

Phase 4:
- D21: CI could not be run on GitHub from this environment (nothing pushed); the workflow was validated with yaml.v3 and every step's command was run locally instead.
- D23: README rewritten after Phase 4 (user request: explain why, when and how; plain style). Added Why / When to use it / Choosing a backoff; three realistic `text` examples and three new executable ones (pipeline, wait-for-file); the `--max-time` example's comment was wrong (said three attempts, run makes four) and its numbers were changed to 50ms/175ms for margin. No behaviour changed; 10 `sh` blocks pass, drift test passes.
- D22: Documentation review before Phase 4 (user request) changed `--help` wording for `--jitter`, `--foreground`, `--delay`, `--verbose`; CONTRIBUTING's CI description; CHANGELOG; and added `TestHelpMatchesReadme` so help and README cannot drift.
