# Changelog

## Unreleased — pivot to a single command (breaking)

This release replaces the whole command surface. There were no tagged releases before it, so
nothing is deprecated; the old shape is simply gone. See `PLAN.md` for the reasoning and the
measured defects that drove it.

### Removed
- All strategy subcommands (`patience exponential …` etc.) and their aliases.
- `http-aware`, `adaptive` and `diophantine` strategies; `fibonacci` and `polynomial` schedules.
- `patienced`, daemon coordination, metrics, `--daemon*`, `--resource-id`, `--fallback`.
- Config files (`--config`, `.patience.toml` discovery, `--debug-config`).
- cgo and the sqlite, viper, yaml and cobra dependencies.
- `scripts/`, `benchmarks/`, `examples/`, `docs/`.

### Added
- One command: `patience [flags] -- COMMAND`.
- `--backoff constant|linear|exponential|decorrelated`, `--jitter F` (0..1).
- `--retry-on CODES`, `--attempts 0` (unlimited), `--max-time`.
- `--version`.
- `--foreground`: run the command in patience's own process group (as `timeout --foreground`).

### Changed
- Progress lines are prefixed `patience:`; a first-try success prints nothing unless `-v`.
- Durations must carry a unit (`5s`), as before, but every flag now validates its range.
- Requires Go 1.24.6 or later to build (os/exec LookPath fix, GO-2025-3956).
- Windows is not supported; the build fails with a clear message.

### Fixed
- stdin is passed to the command.
- Output of any size passes through unmodified; the command is no longer killed at 10 MiB.
- SIGINT/SIGTERM are forwarded to the command; it is no longer orphaned.
- Exit codes: 124 on timeout, 126/127 for exec failures (no retry), 2 for usage errors.
- Environment variables apply to every option.
- Delays can no longer overflow to negative or zero values.
- After a timeout, the SIGKILL that follows SIGTERM is cancelled once the command exits, so
  it can no longer be sent to a process group that was reused in the meantime.
