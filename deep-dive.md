# patience: Backoff Theory Made Concrete

*2026-03-18T12:03:05Z by Showboat 0.6.1*
<!-- showboat-id: ed02cb88-61b8-412d-8b9a-37bb8ad32e36 -->

## The Problem

Most retry logic is written the same way: catch the error, wait a fixed number of seconds, try again. This works — until it doesn't. When 50 instances of your deployment script all fail at once and all wait the same two seconds before hammering the same database, you've turned a transient outage into a thundering herd.

`patience` is a Go CLI that wraps any shell command with configurable retry/backoff behavior. The interesting question isn't "how do you add retry?" — it's "which shape of delay do you need, and why?"

Different failure modes have different shapes:
- **Transient network hiccups** → brief, randomized delay (jitter)
- **Overloaded upstream** → exponential backoff so pressure decreases over time
- **Hard rate limits (e.g. 100 req/hour)** → proactive scheduling that can't be solved by back-pressure alone, because the rate limit applies across all your instances, not per-process

This last case is where patience's Diophantine strategy lives.

## The Strategy Interface

All 10 strategies conform to a single two-method contract defined in `pkg/backoff/strategy.go`:

```bash
sed -n '9,20p' pkg/backoff/strategy.go
```

```output
// Strategy defines the interface for backoff strategies
type Strategy interface {
	// Delay returns the delay duration for the given attempt number
	Delay(attempt int) time.Duration
}

// HTTPAwareStrategy defines the interface for strategies that can process HTTP command output
type HTTPAwareStrategy interface {
	Strategy
	// ProcessCommandOutput analyzes command output to extract HTTP retry timing
	ProcessCommandOutput(stdout, stderr string, exitCode int)
}
```

The core `Strategy` interface has exactly one method: `Delay(attempt int) time.Duration`. The executor calls it before sleeping between retries — that's the entire contract.

The 10 concrete strategies are:

| Strategy | Growth shape | Best for |
|---|---|---|
| Fixed | Constant | Known-slow dependencies |
| Linear | O(n) | Predictable retry windows |
| Exponential | O(multiplier^n) | Transient errors, standard choice |
| Jitter | Random ≤ exponential | Thundering herd prevention |
| DecorrelatedJitter | Random, previous-delay seeded | AWS-recommended, multi-client |
| Fibonacci | Sub-exponential | Middle ground, fast early / slow late |
| Polynomial | O(n^exponent) | Tunable growth rate |
| Adaptive | EMA-based, measured success rate | Self-tuning under real load |
| HTTPAware | Decorator wrapping any strategy | Server-directed timing |
| Diophantine | Proactive constraint satisfaction | Hard API rate limits, multi-instance |

`HTTPAwareStrategy` extends the base with `ProcessCommandOutput` — the executor feeds it each attempt's stdout/stderr so the strategy can parse HTTP headers before deciding the next delay. This is the pattern used to layer HTTP awareness on top of any base strategy without changing the executor loop.

## Strategy Deep Dive: Exponential vs Diophantine

### Exponential — the standard baseline

Exponential backoff is reactive: you wait longer after each failure. The math is simple enough to verify in your head.

```bash
sed -n '57,75p' pkg/backoff/strategy.go
```

```output
// Delay returns the exponentially increasing delay for the given attempt
func (e *Exponential) Delay(attempt int) time.Duration {
	if attempt <= 0 {
		return e.BaseDelay
	}

	// Calculate exponential delay: baseDelay * multiplier^(attempt-1)
	delay := float64(e.BaseDelay) * math.Pow(e.Multiplier, float64(attempt-1))

	// Convert back to duration
	result := time.Duration(delay)

	// Apply max delay cap if set
	if e.MaxDelay > 0 && result > e.MaxDelay {
		result = e.MaxDelay
	}

	return result
}
```

delay = baseDelay × multiplier^(attempt−1)

With baseDelay=1s and multiplier=2: attempt 1→1s, 2→2s, 3→4s, 4→8s. Predictable, geometric. The MaxDelay cap prevents pathological cases.

**What exponential can't solve:** suppose a rate-limited API allows 100 calls/hour. Exponential backoff only tells you how long to wait *after* you fail. If you have 5 parallel patience processes all retrying the same endpoint, their exponential backoffs are independent — no process knows what the others are doing. You can still collectively blow through 100 calls/hour before any single process detects a failure.

This is the problem Diophantine was designed for.

### Diophantine — proactive constraint satisfaction

The name refers to Diophantine inequalities: constraints over integers where you need to find a feasible solution. Here the constraint is: *the number of requests in any sliding window of length W must not exceed rate limit R*.

```bash
sed -n '24,55p' pkg/backoff/diophantine.go
```

```output
// CanScheduleRequest checks if a new request can be scheduled without violating the rate limit.
// It takes a list of existing request times and the time of the new request.
func (d *DiophantineStrategy) CanScheduleRequest(existing []time.Time, newRequestTime time.Time) bool {
	newRetries := make([]time.Time, len(d.retryOffsets))
	for i, offset := range d.retryOffsets {
		newRetries[i] = newRequestTime.Add(offset)
	}

	allRequests := make([]time.Time, 0, len(existing)+len(newRetries))
	allRequests = append(allRequests, existing...)
	allRequests = append(allRequests, newRetries...)
	sort.Slice(allRequests, func(i, j int) bool {
		return allRequests[i].Before(allRequests[j])
	})

	// Check every possible window by using each request time as a potential window start
	for i := 0; i < len(allRequests); i++ {
		windowStart := allRequests[i]
		windowEnd := windowStart.Add(d.window)
		count := 0

		// Count requests in this window [windowStart, windowEnd)
		for j := i; j < len(allRequests) && allRequests[j].Before(windowEnd); j++ {
			count++
		}

		if count > d.rateLimit {
			return false
		}
	}
	return true
}
```

The algorithm is worth reading carefully.

**Key insight:** it checks *future planned retries*, not just the current request. When called with `retryOffsets = [0, 30s, 60s, 90s]`, it materializes all four timestamps, merges them with every other instance's registered requests, sorts the combined list, and then slides a window of length W across every possible start position. If *any* window position would exceed R, the answer is false.

This is O(n²) in the number of in-flight requests, where n = rateLimit × activeInstances. For a limit of 100/hour across 5 instances that's 500 events max — trivially fast, and the window naturally purges expired slots.

The formal constraint being satisfied is:



where T is every request timestamp (potential window starts), R is the set of all registered request times, W is the window duration, and L is the rate limit. By checking every element of R as a potential window start, we guarantee the constraint holds for all possible windows — not just a fixed grid of checkpoints.

**Why not just use a token bucket?** A token bucket is the standard answer to "rate limit N requests per W". The difference is coordination: a token bucket per process would allow 5×N total requests. Diophantine's check runs against the *shared* registered-request list held by the daemon, so all instances collectively stay under N/W.

The algorithm is worth reading carefully.

**Key insight:** it checks *future planned retries*, not just the current request. When called with `retryOffsets = [0, 30s, 60s, 90s]`, it materializes all four timestamps, merges them with every other instance's registered requests, sorts the combined list, and then slides a window of length W across every possible start position. If *any* window position would exceed R, the answer is false.

This is O(n²) in the number of in-flight requests, where n = rateLimit × activeInstances. For a limit of 100/hour across 5 instances that's 500 events max — trivially fast, and the window naturally purges expired slots.

The formal constraint being satisfied is: for every timestamp t in the request set, the count of requests falling in [t, t+W) must not exceed the rate limit L. By using each request time as a potential window start, we guarantee the constraint holds for all possible windows — not just a fixed grid of checkpoints.

**Why not just use a token bucket?** A token bucket is the standard answer to "rate limit N requests per W". The difference is coordination: a token bucket per process would allow 5×N total requests. Diophantine's check runs against the *shared* registered-request list held by the daemon, so all instances collectively stay under the limit.

## The Unix Socket Daemon

The daemon (`cmd/patienced`) is what makes multi-instance coordination possible. Each `patience` process is a client; the daemon is the single source of truth for who has reserved rate-limit slots.

```bash
sed -n '63,99p' pkg/daemon/daemon.go
```

```output
func NewDaemon(config *Config) (*Daemon, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Validate MaxConnections
	if config.MaxConnections <= 0 {
		config.MaxConnections = 100 // Default fallback
	}
	if config.MaxConnections > 10000 {
		config.MaxConnections = 10000 // Reasonable upper limit
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Create structured logger
	logger := NewLogger("daemon", LogLevel(config.LogLevel))

	// Create metrics storage
	metricsStorage := storage.NewMetricsStorage(config.MaxMetrics, config.MetricsMaxAge)

	// Create worker pool for handling connections
	workerPool := NewWorkerPool(config.MaxConnections, metricsStorage, logger)

	daemon := &Daemon{
		config:        config,
		storage:       metricsStorage,
		logger:        logger,
		ctx:           ctx,
		cancel:        cancel,
		connectionSem: make(chan struct{}, config.MaxConnections),
		workerPool:    workerPool,
	}

	return daemon, nil
}
```

```bash
sed -n '115,130p' pkg/daemon/daemon.go
```

```output
	// Create Unix domain socket listener
	// NOTE: This duplicates functionality from UnixServer. Consider refactoring
	// to use UnixServer for socket handling to reduce code duplication.
	listener, err := net.Listen("unix", d.config.SocketPath)
	if err != nil {
		return fmt.Errorf("failed to create socket listener: %w", err)
	}
	d.listener = listener

	// Set socket permissions - restrict to owner only for security
	// Using 0600 ensures only the daemon owner can read/write to the socket,
	// preventing unauthorized users from sending metrics or interfering with rate limiting
	if err := os.Chmod(d.config.SocketPath, 0600); err != nil {
		d.logger.Warn("failed to set socket permissions", "error", err)
	}

```

The daemon uses a Unix domain socket at `/tmp/retry-daemon.sock` (configurable). Two design choices stand out:

**Why Unix sockets, not TCP?** Unix sockets are filesystem objects — access control is enforced by the OS via file permissions. The daemon sets `0600` immediately after binding, so only the owner can connect. For a local coordination primitive this is simpler and safer than binding to a loopback port and writing auth middleware.

**Discovery protocol:** clients don't need pre-configured daemon addresses. They try to connect to the default socket path. If the daemon isn't running, `net.DialTimeout` fails fast and the strategy falls back to local exponential backoff. Zero config for the common case; full coordination when the daemon is present.

```bash
sed -n '29,65p' pkg/daemon/client.go
```

```output
// connect establishes a connection to the daemon if not already connected
func (c *DaemonClient) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return nil // Already connected
	}

	conn, err := net.DialTimeout("unix", c.socketPath, c.connectionTimeout)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon at %s: %w", c.socketPath, err)
	}

	c.conn = conn
	return c.performHandshake()
}

// performHandshake performs the initial protocol handshake using type-safe protocol
func (c *DaemonClient) performHandshake() error {
	handshakeReq := HandshakeRequestJSON{
		Type:    "handshake",
		Version: "1.0",
		Client:  "patience-cli",
	}

	response, err := c.SendHandshakeTypeSafe(handshakeReq)
	if err != nil {
		return fmt.Errorf("handshake failed: %w", err)
	}

	if response.Status != "ok" {
		return fmt.Errorf("handshake rejected by daemon: %s", response.Message)
	}

	return nil
}
```

```bash
sed -n '100,145p' pkg/daemon/client.go
```

```output
// CanScheduleRequest asks the daemon if a request can be scheduled
func (c *DaemonClient) CanScheduleRequest(ctx context.Context, req *ScheduleRequest) (*ScheduleResponse, error) {
	// Check for connection timeout
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// Ensure connection is established
	if err := c.connect(); err != nil {
		return nil, err
	}

	// Create schedule request message
	scheduleReq := map[string]interface{}{
		"type":          "schedule_request",
		"resource_id":   req.ResourceID,
		"rate_limit":    req.RateLimit,
		"window_ms":     int64(req.Window / time.Millisecond),
		"retry_offsets": convertDurationsToMs(req.RetryOffsets),
		"request_time":  req.RequestTime.Unix(),
	}

	response, err := c.sendRequest(scheduleReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send schedule request: %w", err)
	}

	// Parse response
	canSchedule, _ := response["can_schedule"].(bool)
	reason, _ := response["reason"].(string)

	waitUntil := req.RequestTime.Add(time.Minute) // Default fallback
	if waitUntilStr, ok := response["wait_until"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, waitUntilStr); err == nil {
			waitUntil = parsed
		}
	}
	return &ScheduleResponse{
		CanSchedule: canSchedule,
		WaitUntil:   waitUntil,
		Reason:      reason,
	}, nil
}

```

**The coordination loop** (simplified):

1. Client calls `CanScheduleRequest`, passing `retryOffsets` — the full planned schedule for this instance
2. The daemon's `RequestScheduler` merges those offsets with every other registered instance's slots and runs `DiophantineStrategy.CanScheduleRequest`
3. If the answer is `true`, the daemon registers the slots (via `RegisterScheduledRequests`) and returns `can_schedule: true`
4. If `false`, it finds the next available slot using `GetNextAvailableSlot` and returns `wait_until`
5. The client sleeps until `wait_until`, then re-asks

The scheduler's `GetNextAvailableSlot` walks in 1-minute increments up to 24 hours until it finds a feasible window. This is linear search over a constraint-satisfaction problem — pragmatic given that real rate limits rarely push beyond a few hours of backlog.

Because the daemon holds all registered requests in memory keyed by `ResourceID`, every patience instance targeting the same resource sees a globally consistent view without any network round-trips to the upstream API.

## HTTP-Aware Retries: Production Pragmatism

Theory says "back off exponentially." Reality says the server already told you exactly when to retry. The `HTTPAware` strategy is a decorator that wraps any base strategy and overrides its delay whenever it finds timing information in the command's output.

```bash
sed -n '39,81p' pkg/backoff/http_aware.go
```

```output
func (h *HTTPAware) Delay(attempt int) time.Duration {
	// If we have HTTP timing information, use it
	if h.lastRetryAfter > 0 {
		return h.lastRetryAfter
	}

	// Otherwise, fall back to the base strategy
	return h.fallbackStrategy.Delay(attempt)
}

// ProcessCommandOutput analyzes command output to extract HTTP retry timing
func (h *HTTPAware) ProcessCommandOutput(stdout, stderr string, exitCode int) {
	// Reset previous timing
	h.lastRetryAfter = 0

	// Memory optimization: Limit processing to first 10KB of output to prevent memory issues
	const maxProcessingSize = 10 * 1024
	if len(stdout) > maxProcessingSize {
		stdout = stdout[:maxProcessingSize]
	}
	if len(stderr) > maxProcessingSize {
		stderr = stderr[:maxProcessingSize]
	}

	// Check both stdout and stderr for HTTP responses
	output := stdout + "\n" + stderr

	// Try to extract retry timing from various sources
	if delay := h.parseRetryAfterHeader(output); delay > 0 {
		h.lastRetryAfter = h.capDelay(delay)
		return
	}

	if delay := h.parseRateLimitHeaders(output); delay > 0 {
		h.lastRetryAfter = h.capDelay(delay)
		return
	}

	if delay := h.parseJSONResponse(output); delay > 0 {
		h.lastRetryAfter = h.capDelay(delay)
		return
	}
}
```

Three extraction paths, tried in priority order:

1. **`Retry-After` header** — standard HTTP/1.1, parsed with a compiled regex for zero-allocation hot-path performance
2. **`X-RateLimit-Retry-After` / `X-RateLimit-Reset`** — common vendor extensions; the Reset variant is a Unix timestamp, so the code converts it to `time.Until(resetTime)`
3. **JSON body** — walks balanced `{…}` objects looking for fields like `retry_after`, `retryAfterSeconds`, `retry_in`

The `maxProcessingSize = 10KB` cap on input is a deliberate engineering choice: a curl response with a rate-limit header is a few hundred bytes; a command that dumps a 50MB log to stderr shouldn't cause the retry loop to allocate 50MB just to scan it.

The `maxRetryAfter` cap (set at construction time) prevents a malicious or misconfigured server from forcing a week-long wait. Trust, but verify — and cap.

This is "production pragmatism layered on theory": the exponential fallback is the theoretically correct behavior; the HTTP parsing is the empirically correct behavior when you're talking to real APIs.

## Architecture: Separating Strategy Selection from Execution

The clean separation in patience is that **strategies know nothing about commands** — they only answer "how long should I wait?". The executor knows nothing about backoff math — it only asks "how long should I wait?" and sleeps.

```bash
sed -n '141,151p' pkg/executor/executor.go
```

```output
// Executor handles command execution with retry logic
type Executor struct {
	MaxAttempts     int
	Runner          CommandRunner
	BackoffStrategy backoff.Strategy
	Timeout         time.Duration
	Conditions      *conditions.Checker
	Reporter        *ui.Reporter
	DaemonClient    *daemon.DaemonClient // Optional daemon client for coordination
	ResourceID      string               // Resource identifier for rate limiting
}
```

```bash
sed -n '332,339p' pkg/executor/executor.go
```

```output
// Run executes the given command with retry logic and returns the result
// coordinateDaemon handles Diophantine strategy coordination with daemon
func (e *Executor) coordinateDaemon(strategy backoff.Strategy, command []string) error {
	if diophantineStrategy, ok := strategy.(*backoff.DiophantineStrategy); ok {
		return e.coordinateWithDaemon(diophantineStrategy, command)
	}
	return nil
}
```

The `Executor` struct holds a `backoff.Strategy` — the interface, not a concrete type. This is the separation point:

```
Executor.BackoffStrategy  →  backoff.Strategy interface
                              ├── Exponential
                              ├── Jitter
                              ├── Fibonacci
                              ├── HTTPAware (decorator)
                              ├── Diophantine
                              └── ... 7 others
```

The main retry loop calls `BackoffStrategy.Delay(attempt)` then sleeps. It never inspects which strategy it holds. The only place a type assertion appears is in two narrow extension points:

1. `coordinateDaemon` — type-asserts to `*DiophantineStrategy` to trigger daemon coordination (only Diophantine needs this)
2. `recordStrategyOutcome` — interface-asserts to an anonymous `RecordOutcome` interface (only Adaptive needs this)

Both assertions fail gracefully: if the strategy isn't the expected type, the code continues without coordination. This means every strategy is a drop-in replacement with zero executor changes — you pick the strategy at the CLI flag layer and the rest of the system adapts automatically.

The CLI parses `--backoff exponential|diophantine|http-aware|...`, constructs the appropriate strategy object with its parameters, and injects it into `NewExecutorWithBackoff`. Everything downstream is interface-typed.

## Closing Thoughts

The story patience tells is that retry/backoff isn't one problem — it's a family of related problems with different mathematical shapes:

- **Geometric problems** (transient errors) → exponential or jitter
- **Sequence problems** (warmup/cooldown) → Fibonacci or polynomial
- **Constraint-satisfaction problems** (hard rate limits, multi-instance) → Diophantine with daemon

The architecture reflects this: a thin interface hides the math, a decorator (`HTTPAware`) layers empiricism on top of theory, and a daemon externalizes the global state that no single process can maintain alone.

The Diophantine strategy is the most conceptually interesting piece precisely because it inverts the usual retry loop: instead of "try, fail, wait, retry," it asks "given everything everyone else has planned, can I fit my entire retry schedule into the remaining budget?" That's a coordination problem disguised as a retry problem, and patience solves it with a Unix socket and a sliding-window constraint check.

