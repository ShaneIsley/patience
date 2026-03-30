# Backoff Theory Made Concrete: A Code Walkthrough of patience

*2026-03-30T00:06:22Z by Showboat 0.6.1*
<!-- showboat-id: 310327b2-abd4-448b-bb56-b6385d863210 -->

## The Problem: Not All Failures Are Created Equal

Every retry library implements exponential backoff. Few ask *why* exponential is the right choice — or when it isn't.

**patience** is a Go CLI tool for retrying failed commands. It ships 10 backoff strategies, not because more is better, but because different failure modes have fundamentally different shapes:

| Failure Mode | What's Happening | Right Strategy |
|---|---|---|
| Transient blip | Server hiccupped once | Fixed delay — just wait a beat |
| Overloaded service | Cascading load | Exponential — back off aggressively |
| Rate limit | Quota exhausted | Diophantine — *schedule proactively* |
| Flaky test | Non-deterministic | Jitter — decorrelate competing retriers |
| Cold start | Service warming up | Linear — give it incrementally more time |

The interesting architectural question isn't "how do you implement backoff?" — it's "how do you let the caller pick the right shape without coupling strategy selection to execution logic?"

This walkthrough follows that thread through the code: from the strategy interface that makes all 10 interchangeable, through two contrasting implementations (exponential and Diophantine), into the Unix socket daemon that enables multi-instance coordination, and out through the HTTP-aware layer that lets servers override client-side theory with real-world timing.

## The Strategy Interface: One Contract, Ten Shapes

The core abstraction is deliberately minimal — a single method:

```bash
sed -n '9,13p' pkg/backoff/strategy.go
```

```output
// Strategy defines the interface for backoff strategies
type Strategy interface {
	// Delay returns the delay duration for the given attempt number
	Delay(attempt int) time.Duration
}
```

The \`Strategy\` interface takes an attempt number and returns how long to wait. That's it. No configuration leaks through. No awareness of *what* command is running or *why* it failed. This constraint is the load-bearing design decision: it means the executor's retry loop is identical regardless of whether you're using constant delays or solving Diophantine inequalities.

There's a second interface for strategies that need to see HTTP response data:

```bash
sed -n '15,20p' pkg/backoff/strategy.go
```

```output
// HTTPAwareStrategy defines the interface for strategies that can process HTTP command output
type HTTPAwareStrategy interface {
	Strategy
	// ProcessCommandOutput analyzes command output to extract HTTP retry timing
	ProcessCommandOutput(stdout, stderr string, exitCode int)
}
```

\`HTTPAwareStrategy\` embeds \`Strategy\` — it doesn't replace it. The executor checks at runtime whether the strategy implements this optional interface via a type assertion, then feeds it command output if it does. This is Go's version of the open-closed principle: the retry loop doesn't need modification to support HTTP awareness; the strategy opts in.

All 10 strategies conform to this contract. Let's look at two that sit at opposite ends of the complexity spectrum.

## Deep Dive: Exponential Backoff — The Workhorse

Exponential backoff is the strategy most engineers reach for first, and for good reason: when a service is overloaded, linear retry just adds fuel. You need *multiplicative* retreat.

```bash
sed -n '38,75p' pkg/backoff/strategy.go
```

```output

// Exponential implements an exponential backoff strategy
type Exponential struct {
	BaseDelay  time.Duration
	Multiplier float64
	MaxDelay   time.Duration
}

// NewExponential creates a new Exponential backoff strategy
// baseDelay is the initial delay, multiplier is the factor to increase by each attempt
// maxDelay is the maximum delay (0 means no limit)
func NewExponential(baseDelay time.Duration, multiplier float64, maxDelay time.Duration) *Exponential {
	return &Exponential{
		BaseDelay:  baseDelay,
		Multiplier: multiplier,
		MaxDelay:   maxDelay,
	}
}

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

The formula is \`baseDelay × multiplier^(attempt-1)\`. With defaults of 1s base and 2x multiplier, the delay sequence is: 1s, 2s, 4s, 8s, 16s...

Three design details worth noting:

1. **MaxDelay cap**: Unbounded exponential growth hits absurd values fast — attempt 20 with a 2x multiplier is ~145 hours. The cap makes this production-safe.
2. **attempt-1 exponent**: Attempt 1 returns the base delay, not double it. The first retry shouldn't penalize you for the original failure.
3. **Stateless**: The \`Delay\` method is a pure function of the attempt number. No mutation, no side effects — it's safe to call from concurrent goroutines without synchronization.

This is exactly the right tool when a service is overloaded and you want clients to *spread out* over time. But it has a fundamental limitation: it's *reactive*. It only backs off *after* failure. For rate-limited APIs, you've already consumed a quota slot by the time you learn you should have waited.

That's the problem the Diophantine strategy solves.

## Deep Dive: The Diophantine Strategy — Proactive Rate Limiting

### The Mathematical Foundation

Most backoff strategies answer the question "how long should I wait *after* a failure?" The Diophantine strategy answers a different question: "given a rate limit of N requests per window W, and a set of planned retry offsets, *can I start this request without any sliding window of size W ever exceeding N total requests?*"

This is a constraint satisfaction problem. The name references Diophantine inequalities — constraints over integer (or in this case, discrete-time) variables where you need to verify that no feasible window placement violates the bound.

```bash
sed -n '1,55p' pkg/backoff/diophantine.go
```

```output
package backoff

import (
	"sort"
	"time"
)

// DiophantineStrategy implements a proactive scheduling strategy based on Diophantine inequalities.
type DiophantineStrategy struct {
	rateLimit    int
	window       time.Duration
	retryOffsets []time.Duration
}

// NewDiophantine creates a new Diophantine backoff strategy.
func NewDiophantine(rateLimit int, window time.Duration, retryOffsets []time.Duration) *DiophantineStrategy {
	return &DiophantineStrategy{
		rateLimit:    rateLimit,
		window:       window,
		retryOffsets: retryOffsets,
	}
}

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

### How the Algorithm Works

The \`CanScheduleRequest\` method is the heart of the strategy. Walk through it step by step:

1. **Project future requests**: Given the new request time and the configured retry offsets (e.g., "retry at +5s, +15s, +45s"), compute every timestamp this request *will* generate.

2. **Merge with existing schedule**: Combine projected retries with all requests already registered in the system. Sort by time.

3. **Sliding window check**: For every request in the merged timeline, treat it as the start of a window of size W. Count how many requests fall within \`[windowStart, windowStart + W)\`. If *any* window exceeds the rate limit, reject the request.

The key insight is in step 3: rather than checking one window, the algorithm checks *every possible* window position anchored at a request time. This is correct because the worst-case window — the one most likely to violate the limit — always starts at (or just before) a request timestamp. There's no point checking a window that starts in a gap between requests; sliding it forward to the next request can only increase the count.

This is an O(n²) check in the worst case (for each of n request times, scan forward through the remaining requests). For the typical use case — coordinating a handful of CLI instances with tens to low hundreds of tracked requests — this is more than fast enough. The constant factor is tiny: it's comparing timestamps, not doing network I/O.

### Fallback Behavior

The Diophantine strategy also implements the standard \`Delay(attempt int)\` interface for compatibility:

```bash
sed -n '57,82p' pkg/backoff/diophantine.go
```

```output
// Delay implements the Strategy interface for compatibility with existing executor
// For Diophantine strategy, this is a fallback that should rarely be used
// The main scheduling logic is in CanScheduleRequest
func (d *DiophantineStrategy) Delay(attempt int) time.Duration {
	// Simple exponential backoff as fallback when daemon is not available
	if attempt <= 0 {
		return time.Second
	}

	// Use the first retry offset as base delay, with exponential growth
	baseDelay := time.Second
	if len(d.retryOffsets) > 1 {
		baseDelay = d.retryOffsets[1] // Use second offset as it's usually the first retry
	}

	// Exponential backoff: baseDelay * 2^(attempt-1)
	delay := baseDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay > time.Hour {
			return time.Hour // Cap at 1 hour
		}
	}

	return delay
}
```

This is a pragmatic concession: when the daemon isn't running, the Diophantine strategy degrades gracefully to exponential backoff. No crash, no special error handling in the executor — it just becomes a slightly different exponential strategy. The caller never knows.

But the real power of the Diophantine approach only activates when multiple instances coordinate through the daemon.

## The Unix Socket Daemon: Multi-Instance Coordination

### Why a Daemon?

The Diophantine algorithm can check whether *one* instance's requests fit within a rate limit. But in production, you often have multiple patience instances retrying against the same API — parallel CI jobs, cron tasks, deployment scripts. Each instance has its own view of the schedule. Without coordination, they'll independently conclude "I have room" and collectively blow the limit.

The daemon solves this by providing a single point of truth: a shared request schedule that all instances register with and query before executing.

### Architecture: Socket → Protocol → Scheduler

The daemon uses a Unix domain socket for IPC. Three components handle the flow:

**1. Connection lifecycle** — The \`UnixServer\` manages socket creation, permission hardening, and connection pooling:

```bash
sed -n '56,81p' pkg/daemon/unix_server.go
```

```output
func (s *UnixServer) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)

	// Remove existing socket file if it exists
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return err
	}

	// Create Unix socket listener
	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return err
	}
	s.listener = listener

	// Set socket permissions for security
	if err := os.Chmod(s.socketPath, SocketPermissions); err != nil {
		s.listener.Close()
		return err
	}

	// Start accepting connections in a goroutine
	go s.acceptConnections()

	return nil
}
```

The socket is created with \`0600\` permissions — owner-only read/write. This is a defense-in-depth choice: the daemon coordinates rate limiting, and you don't want arbitrary users on a shared machine manipulating the schedule. The stale socket cleanup (\`os.Remove\` before \`net.Listen\`) handles the common case of a daemon that crashed without cleaning up.

**2. Wire protocol** — Newline-delimited JSON over the socket, with typed message structs:

```bash
sed -n '1,39p' pkg/daemon/protocol_types.go
```

```output
package daemon

import "time"

// JSON Protocol message types for type-safe daemon communication
// These replace map[string]interface{} usage throughout daemon package

// HandshakeRequestJSON represents a client handshake request in JSON protocol
type HandshakeRequestJSON struct {
	Type    string `json:"type"`
	Version string `json:"version"`
	Client  string `json:"client"`
}

// HandshakeResponseJSON represents a daemon handshake response in JSON protocol
type HandshakeResponseJSON struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// ScheduleRequestJSON represents a request to schedule a command execution in JSON protocol
type ScheduleRequestJSON struct {
	Type        string    `json:"type"`
	ResourceID  string    `json:"resource_id"`
	Command     []string  `json:"command"`
	RequestedAt time.Time `json:"requested_at"`
}

// ScheduleResponseJSON represents a response to a schedule request in JSON protocol
type ScheduleResponseJSON struct {
	Type         string    `json:"type"`
	Status       string    `json:"status"`
	CanSchedule  bool      `json:"can_schedule"`
	Reason       string    `json:"reason,omitempty"`
	Message      string    `json:"message,omitempty"`
	ScheduledAt  time.Time `json:"scheduled_at,omitempty"`
	ExpiresAt    time.Time `json:"expires_at,omitempty"`
}
```

The protocol follows a request-response pattern: handshake first (version negotiation), then schedule queries and registration. The \`type\` field acts as a discriminated union — the server parses it first to determine which struct to deserialize into. This avoids the fragility of \`map[string]interface{}\` dispatch while keeping the wire format human-readable (it's just JSON you can debug with \`socat\`).

**3. The scheduler** — Where the Diophantine algorithm meets shared state:

```bash
sed -n '78,110p' pkg/daemon/scheduler.go
```

```output
// CanScheduleWithStrategy checks if a new request can be scheduled using the given strategy
func (s *RequestScheduler) CanScheduleWithStrategy(resourceID string, strategy *backoff.DiophantineStrategy, requestTime time.Time) bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// Get existing requests as time.Time slice
	requests := s.requests[resourceID]
	existing := make([]time.Time, 0, len(requests))

	now := time.Now()
	for _, req := range requests {
		if req.ExpiresAt.After(now) {
			existing = append(existing, req.ScheduledAt)
		}
	}

	return strategy.CanScheduleRequest(existing, requestTime)
}

// GetNextAvailableSlot finds the next time when a request can be scheduled
func (s *RequestScheduler) GetNextAvailableSlot(resourceID string, strategy *backoff.DiophantineStrategy, preferredTime time.Time) time.Time {
	// Simple implementation: try every minute until we find an available slot
	candidate := preferredTime
	for i := 0; i < 1440; i++ { // Try for up to 24 hours
		if s.CanScheduleWithStrategy(resourceID, strategy, candidate) {
			return candidate
		}
		candidate = candidate.Add(time.Minute)
	}

	// If no slot found in 24 hours, return 24 hours from now
	return preferredTime.Add(24 * time.Hour)
}
```

The scheduler is the bridge between the daemon's shared state and the Diophantine algorithm. \`CanScheduleWithStrategy\` collects all non-expired requests for a resource, converts them to timestamps, and delegates to \`DiophantineStrategy.CanScheduleRequest\`. The read lock (\`RLock\`) allows concurrent schedule *checks* while writes (registering new requests) take the exclusive lock.

\`GetNextAvailableSlot\` is a linear scan — try each minute until one works. For a rate limit of 100/hour, the worst case is 100 iterations. This is intentionally simple; a binary search would be premature optimization for a use case where "check every minute for the next 24 hours" completes in microseconds.

### The Client-Side Flow

When the executor detects a Diophantine strategy, it coordinates with the daemon before executing:

```bash
sed -n '225,281p' pkg/executor/executor.go
```

```output
func (e *Executor) coordinateWithDaemon(strategy *backoff.DiophantineStrategy, command []string) error {
	// If no daemon client is configured, skip coordination (fallback mode)
	if e.DaemonClient == nil {
		return nil
	}

	// Determine resource ID (use configured ResourceID or derive from command)
	resourceID := e.ResourceID
	if resourceID == "" {
		resourceID = e.deriveResourceID(command)
	}

	// Create schedule request
	scheduleReq := &daemon.ScheduleRequest{
		ResourceID:   resourceID,
		RateLimit:    strategy.GetRateLimit(),
		Window:       strategy.GetWindow(),
		RetryOffsets: strategy.GetRetryOffsets(),
		RequestTime:  time.Now(),
	}

	// Ask daemon if we can schedule now
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := e.DaemonClient.CanScheduleRequest(ctx, scheduleReq)
	if err != nil {
		// If daemon communication fails, fall back to local-only mode
		if e.Reporter != nil {
			e.Reporter.ShowWarning("Daemon unavailable, using local scheduling")
		}
		return nil
	}

	// If we can't schedule now, wait until we can
	if !response.CanSchedule {
		waitTime := time.Until(response.WaitUntil)
		if waitTime > 0 {
			if e.Reporter != nil {
				e.Reporter.ShowWaiting(waitTime, "Waiting for rate limit slot...")
			}
			time.Sleep(waitTime)
		}
	}

	// Register our planned requests with the daemon
	plannedRequests := e.createPlannedRequests(resourceID, scheduleReq.RequestTime, strategy.GetRetryOffsets())
	err = e.DaemonClient.RegisterScheduledRequests(ctx, plannedRequests)
	if err != nil {
		// Registration failure is not critical, continue with execution
		if e.Reporter != nil {
			e.Reporter.ShowWarning("Failed to register requests with daemon")
		}
	}

	return nil
}
```

The coordination flow reveals a consistent design philosophy — **degrade gracefully, never block fatally**:

1. **No daemon? Skip coordination.** The nil check on \`DaemonClient\` means non-daemon mode just works.
2. **Daemon unreachable? Warn and continue.** A network error doesn't abort the command — it falls back to local-only scheduling.
3. **Registration fails? Warn and continue.** The command still executes; other instances just won't know about it.

This layered fallback means the daemon is purely additive. You get better rate limiting *with* it, but never worse behavior *without* it.

The \`deriveResourceID\` method is worth a glance — it uses command heuristics (curl URL → hostname, psql → "database") to automatically group related commands under the same rate limit without requiring explicit configuration. It's a small touch that makes the common case work without flags.

## HTTP-Aware Retries: When the Server Knows Better

All the strategies so far are client-side heuristics. But HTTP servers can *tell* you when to retry — via the \`Retry-After\` header, rate limit headers, or JSON response bodies. The \`HTTPAware\` strategy wraps any other strategy and overrides its delay when server-provided timing is available:

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

The priority cascade is deliberate: standard \`Retry-After\` header first, then vendor-specific rate limit headers (\`X-RateLimit-Reset\`), then JSON body fields (\`retry_after\`, \`retryAfter\`, etc.). Each source is more fragile and less standardized than the last, so the code trusts them in descending order.

Two production-hardening details stand out:

- **10KB output cap**: Without this, a command that dumps a multi-megabyte response would cause the regex engine to churn through the entire thing looking for headers. The cap keeps parsing time bounded regardless of command output size.
- **maxRetryAfter cap**: A malicious or buggy server could send \`Retry-After: 999999\`. The cap prevents a single response from stalling the retry loop for days.

The \`Delay\` method shows the layering clearly: if \`ProcessCommandOutput\` found server timing, use it; otherwise, delegate to whatever fallback strategy was configured (exponential, linear, etc.). The HTTP-aware layer is a *decorator*, not a replacement.

## Architecture: Strategy Selection Separated from Execution

The executor's retry loop is strategy-agnostic. Here's the critical section:

```bash
sed -n '441,545p' pkg/executor/executor.go
```

```output
func (e *Executor) Run(command []string) (*Result, error) {
	// Handle Diophantine strategy coordination with daemon
	if err := e.coordinateDaemon(e.BackoffStrategy, command); err != nil {
		return &Result{
			Success:      false,
			AttemptCount: 0,
			ExitCode:     -1,
			TimedOut:     false,
			Reason:       fmt.Sprintf("daemon coordination failed: %v", err),
		}, err
	}

	var lastOutput CommandOutput
	var lastError error
	var timedOut bool

	// Initialize execution tracking
	stats, attemptMetrics, runStartTime := e.initializeExecution(command)

	// Retry loop
	for attempt := 1; attempt <= e.MaxAttempts; attempt++ {
		// Report attempt start
		if e.Reporter != nil {
			e.Reporter.AttemptStart(attempt, e.MaxAttempts)
		}
		stats.RecordAttemptStart()

		// Record attempt start time for metrics
		attemptStartTime := time.Now()

		output, err, timeout := e.executeAttempt(command)
		lastOutput = output
		lastError = err
		if timeout {
			timedOut = true
		}

		// Record attempt duration for metrics
		attemptDuration := time.Since(attemptStartTime)

		if err != nil {
			return nil, err
		}

		// Check success conditions and determine if we should stop retrying
		conditionResult, shouldStop := e.processAttemptResult(output, attempt)

		// Record attempt result
		stats.RecordAttemptEnd(conditionResult.Success, conditionResult.Reason)

		// Record attempt metrics
		attemptMetrics = append(attemptMetrics, metrics.AttemptMetric{
			Duration: attemptDuration,
			ExitCode: output.ExitCode,
			Success:  conditionResult.Success,
		})

		// Record outcome for adaptive and HTTP-aware strategies
		e.recordStrategyOutcome(attempt, conditionResult.Success, attemptDuration)

		// Process command output for HTTP-aware strategies
		if httpAware, ok := e.BackoffStrategy.(interface {
			ProcessCommandOutput(stdout, stderr string, exitCode int)
		}); ok {
			httpAware.ProcessCommandOutput(output.Stdout, output.Stderr, output.ExitCode)
		}

		// If we should stop retrying (success or failure pattern matched)
		if shouldStop {
			stats.Finalize(conditionResult.Success, conditionResult.Reason)
			return e.buildFinalResult(conditionResult.Success, attempt, output, timedOut, conditionResult.Reason, stats, attemptMetrics, runStartTime, command, lastError), nil
		}

		// If this was the last attempt, break out of loop
		if attempt == e.MaxAttempts {
			// Report final failure (no retry)
			if e.Reporter != nil {
				failureReason := conditionResult.Reason
				if timedOut {
					failureReason = fmt.Sprintf("timeout: %s", e.Timeout)
				}
				e.Reporter.AttemptFailure(attempt, e.MaxAttempts, failureReason, 0)
			}
			break
		}

		// Calculate delay and report failure
		var delay time.Duration
		if e.BackoffStrategy != nil {
			delay = e.BackoffStrategy.Delay(attempt)
		}

		if e.Reporter != nil {
			failureReason := conditionResult.Reason
			if timedOut {
				failureReason = fmt.Sprintf("timeout: %s", e.Timeout)
			}
			e.Reporter.AttemptFailure(attempt, e.MaxAttempts, failureReason, delay)
		}

		// Wait before next attempt if backoff strategy is configured
		if delay > 0 {
			time.Sleep(delay)
		}
	}
```

The retry loop itself never mentions exponential, linear, Diophantine, or any specific strategy. It calls \`e.BackoffStrategy.Delay(attempt)\` and sleeps. The only strategy-specific code is the Diophantine daemon coordination at the top, which is checked via a type assertion (\`coordinateDaemon\` does an \`ok\` cast internally). HTTP-aware processing is similarly discovered at runtime via interface assertion.

This separation has a concrete benefit: adding strategy #11 requires zero changes to the executor. You implement \`Delay(attempt int) time.Duration\`, register a new subcommand, and the existing retry loop handles it.

### The Full Request Lifecycle

Putting it all together, here's the path a Diophantine-coordinated request takes:

    CLI parses flags → creates DiophantineStrategy(rateLimit, window, offsets)
                     → creates Executor with strategy + DaemonClient
                     → Executor.Run() called

    coordinateDaemon():
      1. Connect to daemon via Unix socket
      2. Handshake (version negotiation)
      3. Send ScheduleRequest with rate limit + offsets
      4. Daemon's RequestScheduler checks CanScheduleRequest
         → merges with all registered requests
         → runs sliding window check
      5. If denied: sleep until WaitUntil
      6. Register planned retries with daemon

    Retry loop:
      for attempt 1..N:
        execute command
        check success/failure conditions
        if HTTP-aware: parse Retry-After from output
        delay = strategy.Delay(attempt)
        sleep(delay)

The daemon coordination happens *once*, before the retry loop — it's about *admission control* ("should I start at all?"), not about per-retry timing. The per-retry delays still come from the strategy's \`Delay\` method, which in the Diophantine case falls back to exponential growth.

## Closing: Theory as a Design Tool

The 10 strategies in patience aren't 10 ways to do the same thing. They encode different models of failure:

- **Fixed/Linear**: The system needs time, and more time helps proportionally.
- **Exponential**: The system is overloaded, and concurrent retriers make it worse.
- **Jitter/Decorrelated Jitter**: Multiple retriers exist, and they need to avoid thundering herd.
- **Fibonacci**: You want growth between linear and exponential — aggressive but not *too* aggressive.
- **Polynomial**: You need tunable growth curves without the explosion of exponential.
- **HTTP-Aware**: The server is the authority on its own capacity.
- **Adaptive**: Past outcomes should inform future delays.
- **Diophantine**: The rate limit is known, retries are predictable, and you want to *prevent* violations rather than react to them.

The architecture makes these interchangeable at the interface level while allowing each to bring its own complexity. The Diophantine strategy brings a daemon; the HTTP-aware strategy brings output parsing; the exponential strategy brings nothing but arithmetic. The executor doesn't care — it calls \`Delay(attempt)\` and moves on.

This is what "backoff theory made concrete" looks like: not just implementing the formulas, but designing an abstraction that lets each formula live in its natural complexity without leaking into the rest of the system.
