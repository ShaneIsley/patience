# patience

Run a command again until it succeeds.

```
patience [flags] -- COMMAND [ARGS...]
```

patience runs a command. If the command fails, patience waits and runs it again. It stops at
the first success, after a set number of attempts, when a time budget runs out, or when you
press Ctrl-C. It is one static binary for Linux and macOS with no runtime dependencies.

## Why

Most retries start as a shell loop:

```text
for i in 1 2 3; do curl -fsS "$url" && break; sleep 5; done
```

The loop has the same gaps every time someone writes it. The delay is fixed, so a hundred
clients that failed together retry together. A hung attempt hangs the loop. The loop exits 0
even when every attempt failed, because `$?` is the status of the last `sleep`. Each gap gets
patched by hand, in each script, differently.

patience is that loop written once, with the gaps closed:

- Delays grow between attempts, with optional jitter so clients spread out.
- Each attempt can have a timeout, and the whole run a budget.
- Ctrl-C and SIGTERM reach the command and every process it started.
- stdin, stdout and stderr pass straight through. The command's exit code is patience's exit code.
- When the exit code lies, a pattern in the output can decide success or failure instead.

## When to use it

- A network command in a script or CI job fails now and then: `curl`, `git fetch`,
  `docker pull`, `apt-get update`, `terraform apply`.
- Something has to be up before the next step can run: a database, a container, a port.
- An API rate-limits you and expects callers to back off.
- A tool prints its real result but exits 0 either way, or exits non-zero after succeeding.
- A command sometimes hangs and should be killed and tried again.

When not to: a tool with a retry of its own that understands the protocol will do better.
`curl --retry` honours `Retry-After` headers, which patience cannot see. A service that must stay
up belongs under a supervisor such as systemd or a container runtime. And a command that is not
safe to repeat, such as a payment without an idempotency key, should not be retried by anything.

## Install

```text
go install github.com/shaneisley/patience/cmd/patience@latest
```

Or build from a checkout with `make build`, which writes a static `patience` binary to the
repository root.

## Quick start

Retry a health check up to five times, waiting 1s, 2s, 4s and 8s between attempts:

```text
patience -n 5 -- curl -fsS https://example.com/health
```

Wait for a database to accept connections, checking every two seconds for up to a minute:

```text
patience -n 0 -b constant -d 2s --max-time 1m -- pg_isready -h db
```

Pull an image, giving each attempt 30 seconds and randomising the delays so that many hosts
starting at once do not retry in step:

```text
patience -n 4 -t 30s -j 1 -- docker pull registry.example.com/app:latest
```

Here is what a run looks like. Progress goes to stderr, prefixed `patience:`. The command's
own output is passed through as it is produced:

```sh
# expect: exit 7
patience -n 3 -d 10ms -- sh -c 'echo trying >&2; exit 7'
# stderr:
#   trying
#   patience: attempt 1 failed (exit code 7); retrying in 10ms
#   trying
#   patience: attempt 2 failed (exit code 7); retrying in 20ms
#   trying
#   patience: attempt 3 failed (exit code 7)
#   patience: giving up: all 3 attempts failed (exit code 7)
```

A command that succeeds on the first try produces no output from patience unless you pass `-v`.
`-q` silences progress entirely.

Because progress never touches stdout, patience sits inside a pipeline without changing what
flows through it:

```sh
seq 5 | patience -- wc -l
# prints 5
```

The "wait for something" case, with a background job standing in for the slow service:

```sh
d=$(mktemp -d)
(sleep 0.2; touch "$d/ready") &
patience -n 0 -b constant -d 50ms --max-time 5s -- test -f "$d/ready" && rm -r "$d"
# stderr: a few "failed (exit code 1); retrying in 50ms" lines, then "attempt N succeeded"
```

Every example in this file that is fenced as `sh` is run by the test suite against the
current binary, with `# expect: exit N` giving the expected exit status.

## Flags and defaults

Without flags, patience makes 3 attempts with exponential backoff starting at 1s, capped
at 1m, no jitter, no timeout, and retries on any non-zero exit code.

### Flags

```
Retry control
  -n, --attempts N         total attempts; 0 = unlimited (default 3)
  -t, --timeout D          per-attempt timeout; 0 = none (default 0)
      --max-time D         overall budget including delays; 0 = none (default 0)
      --retry-on CODES     retry only on these exit codes, e.g. 1,75 (default: any non-zero)

Backoff
  -b, --backoff NAME       constant | linear | exponential | decorrelated (default exponential)
  -d, --delay D            base delay; increment for linear (default 1s)
  -m, --max-delay D        cap on any single delay; 0 = none (default 1m)
  -x, --multiplier F       growth factor for exponential and decorrelated (default 2)
  -j, --jitter F           0..1: randomise each delay downward by up to this fraction; 1 = full jitter (default 0)

Outcome
      --success-pattern RE attempt succeeds if its output matches (overrides exit code)
      --failure-pattern RE attempt fails if its output matches (overrides success pattern)
  -i, --ignore-case        patterns are case-insensitive

Other
      --foreground         run the command in patience's own process group (see Signals)
  -q, --quiet              no progress on stderr
  -v, --verbose            report every attempt and a final summary
      --version
```

Durations take Go syntax: `500ms`, `2s`, `1m30s`. A bare number is an error, not seconds.

Every flag can be set by an environment variable named `PATIENCE_<FLAG>`, with dashes as
underscores. Flags win over the environment. This is the way to set defaults for a whole CI job:

```sh
PATIENCE_ATTEMPTS=2 PATIENCE_DELAY=0 PATIENCE_QUIET=true patience -- false
# expect: exit 1
```

## Choosing a backoff

The delay after failed attempt *k* (counting from 1), before the cap and jitter:

| `--backoff`    | delay                              | with `-d 1s -x 2`         |
|----------------|------------------------------------|---------------------------|
| `constant`     | `d`                                | 1s, 1s, 1s, 1s            |
| `linear`       | `d · k`                            | 1s, 2s, 3s, 4s            |
| `exponential`  | `d · x^(k−1)`                      | 1s, 2s, 4s, 8s            |
| `decorrelated` | `U[d, min(cap, prev · x)]`         | random, AWS-style         |

Every delay is then capped at `--max-delay`. Then, for all but `decorrelated`, jitter is applied:
with `-j F` the delay becomes `v·(1−F) + U[0, F·v]`. So `-j 1` is full jitter (anywhere from 0
to v), `-j 0.5` is equal jitter (v/2 to v), and `-j 0.2` shortens the delay by at most 20%.

Which to pick:

- `exponential` (the default) suits most things. Transient faults usually clear within a few
  seconds, and the growing delay stops a long outage from turning into a tight loop.
- `constant` suits polling for something that is expected to come up, where there is no
  reason to wait longer each time.
- `linear` suits a resource that recovers at a steady rate, such as a queue draining.
- `decorrelated` is the AWS "decorrelated jitter" sequence, for many clients hammering one
  service. Its reference implementation uses `-x 3`. It ignores `-j`.

Add jitter whenever more than one client can be retrying against the same service. Without it,
clients that fail together stay synchronised on every retry.

```sh
patience -n 4 -b linear -d 20ms -- false
# expect: exit 1
# stderr shows: retrying in 20ms, 40ms, 60ms
```

## Deciding success and failure

By default an attempt succeeds when the command exits 0. Two patterns override that. They are
checked line by line against both stdout and stderr as the command runs:

1. `--failure-pattern` matched: the attempt failed (exit code 1 if the command exited 0).
2. Otherwise `--success-pattern` matched: the attempt succeeded.
3. Otherwise the exit code decides.

```sh
# A deploy tool that prints success but exits 1:
patience -n 1 --success-pattern 'deployment successful' -- sh -c 'echo deployment successful; exit 1'
# expect: exit 0
```

```sh
# A health check that exits 0 but reports an error:
patience -n 1 --failure-pattern '(?i)error' -- sh -c 'echo ERROR: service unavailable'
# expect: exit 1
```

Patterns are Go regular expressions (RE2 syntax). `-i` makes them case-insensitive. A line
longer than 64 KiB is matched in 64 KiB pieces.

### Retrying only some failures

Most commands use exit codes to say *why* they failed, and only some of those reasons are
worth another try. `--retry-on` lists the codes that are:

```sh
# Exit 3 is not in the list, so it is not retried.
patience -n 5 -d 0 --retry-on 1,75 -- sh -c 'exit 3'
# expect: exit 3
```

A command that cannot be started at all, because it is not found or not executable, exits
127 or 126 and is never retried.

## Timeouts and budgets

`--timeout` limits each attempt. A timed-out attempt is sent SIGTERM, then SIGKILL two seconds
later if it is still running. It counts as a failure with exit code 124, as in `timeout(1)`:

```sh
patience -n 1 -t 100ms -- sleep 5
# expect: exit 124
```

`--max-time` bounds the whole run, delays included. patience will not start a sleep that would
end past the budget, but it never cuts short a running attempt:

```sh
patience -n 0 -b constant -d 50ms --max-time 175ms -- false
# expect: exit 1
# attempts at 0ms, 50ms, 100ms and 150ms; the sleep that would end at 200ms is not started
```

`-n 0` retries until success, `--max-time`, or a signal. Combine it with `--max-time` unless
you want the run to be stopped by hand.

## Exit status

| exit    | meaning                                                      |
|---------|--------------------------------------------------------------|
| 0       | an attempt succeeded                                         |
| N       | the last attempt failed with the command's exit code N       |
| 1       | the last attempt was failed by `--failure-pattern` (exit was 0) |
| 124     | the last attempt timed out                                   |
| 126/127 | the command is not executable / not found (no retries)       |
| 128+S   | patience was interrupted by signal S (130 for Ctrl-C)        |
| 2       | usage error (bad flag, no command)                           |

## Signals

By default the command runs in its own process group. SIGINT and SIGTERM sent to patience are
forwarded to that group, so Ctrl-C stops the command and everything it started. A second signal
kills them. patience then exits 128+signal without retrying.

`--foreground` runs the command in patience's own process group instead. Use it for commands
that need the terminal, such as prompts, editors, or `ssh` without a command. The terminal then
delivers Ctrl-C to the command directly, but a `--timeout` kills only the command itself, not
the processes it started. This is the same trade-off as `timeout --foreground`.

## Development

```text
make build      # static binary
make test       # go test ./...  (no network, no fixed paths, builds into a temp dir)
make race       # go test -race -count=3 ./...
make lint       # gofmt, go vet, staticcheck
make doctest    # runs every sh block in this file against the built binary
```

`CONTRIBUTING.md` has the rules. `PLAN.md` records why the tool looks the way it does, and
`CHANGELOG.md` lists what changed.

## License

MIT, see `LICENSE`.
