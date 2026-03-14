# Patience: A Code Walkthrough

*2026-03-14T01:37:50Z by Showboat 0.6.1*
<!-- showboat-id: 40254663-18df-442d-93c7-3d68f70654fc -->

## What is Patience?

Patience is a production-grade command retry utility written in Go. It wraps shell commands and retries them intelligently when they fail, offering 10 different backoff strategies ranging from simple fixed delays to machine-learning-inspired adaptive timing and mathematical rate-limit modeling.

The core idea: instead of writing ad-hoc retry loops in your scripts, you wrap any command with `patience` and let it handle failure recovery with the right timing strategy for your use case.

Let's walk through how it all works, following the code from entry point to execution.

## Project Structure

The codebase is organized into two main areas: the CLI commands (`cmd/`) and the library packages (`pkg/`).

```bash
find /home/user/patience/cmd /home/user/patience/pkg -type f -name '*.go' ! -name '*_test.go' | sed 's|/home/user/patience/||' | sort
```

```output
cmd/patience/main.go
cmd/patience/subcommands.go
cmd/patienced/main.go
pkg/backoff/adaptive.go
pkg/backoff/diophantine.go
pkg/backoff/diophantine_discovery.go
pkg/backoff/effectiveness_tracker.go
pkg/backoff/http_aware.go
pkg/backoff/http_aware_adaptive.go
pkg/backoff/http_aware_selector.go
pkg/backoff/polynomial.go
pkg/backoff/strategy.go
pkg/conditions/conditions.go
pkg/config/config.go
pkg/daemon/client.go
pkg/daemon/daemon.go
pkg/daemon/logging.go
pkg/daemon/protocol.go
pkg/daemon/protocol_types.go
pkg/daemon/scheduler.go
pkg/daemon/server.go
pkg/daemon/unix_server.go
pkg/daemon/worker_pool.go
pkg/discovery/database.go
pkg/discovery/enhanced_parser.go
pkg/discovery/learner.go
pkg/discovery/parser.go
pkg/discovery/resource_grouper.go
pkg/discovery/service.go
pkg/discovery/types.go
pkg/executor/constants.go
pkg/executor/executor.go
pkg/metrics/metrics.go
pkg/monitoring/resources.go
pkg/patterns/http_pattern_matcher.go
pkg/patterns/json_matcher.go
pkg/patterns/matcher.go
pkg/patterns/multiline_matcher.go
pkg/patterns/pattern_set.go
pkg/storage/storage.go
pkg/ui/ui.go
```

The key packages:
- **cmd/patience/** -- the CLI entry point and strategy subcommands
- **cmd/patienced/** -- an optional metrics daemon
- **pkg/backoff/** -- all 10 backoff strategy implementations
- **pkg/executor/** -- the core retry engine
- **pkg/conditions/** -- success/failure pattern matching
- **pkg/config/** -- configuration loading with file/env/flag precedence
- **pkg/ui/** -- terminal status reporting
- **pkg/metrics/** -- metrics collection and transmission
- **pkg/daemon/** -- optional daemon for multi-instance coordination

## The Entry Point: main.go

Everything starts in `cmd/patience/main.go`. The `main()` function is minimal -- it just executes the root Cobra command:

```bash
sed -n '262,267p' /home/user/patience/cmd/patience/main.go
```

```output
func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

The `rootCmd` is defined as a Cobra command that serves as the top-level help and dispatcher. It lists all available strategies and provides usage examples:

```bash
sed -n '110,141p' /home/user/patience/cmd/patience/main.go
```

```output
var rootCmd = &cobra.Command{
	Use:   "patience STRATEGY [STRATEGY-OPTIONS] -- COMMAND [ARGS...]",
	Short: "Intelligent retry wrapper with adaptive backoff strategies",
	Long: `patience is a CLI tool that executes commands with intelligent retry strategies.
It supports multiple backoff strategies including HTTP-aware retries that respect
server timing hints.

Available Strategies:
  http-aware           HTTP response-aware delays (respects Retry-After headers)
  exponential          Exponentially increasing delays  
  linear               Linearly increasing delays
  fixed                Fixed delay between retries
  jitter               Random jitter around base delay
  decorrelated-jitter  AWS-style decorrelated jitter
  fibonacci            Fibonacci sequence delays

Use "patience STRATEGY --help" for strategy-specific options.

EXAMPLES:
  # HTTP-aware retry for API calls
  patience http-aware --fallback exponential -- curl -i https://api.github.com

  # Exponential backoff with custom parameters
  patience exponential --base-delay 1s --multiplier 2.0 -- curl https://httpbin.org/delay/2

  # Linear backoff for database connections
  patience linear --increment 5s --max-delay 60s -- psql -h db.example.com

  # Using abbreviations for brevity
  patience ha -f exp -- curl -i https://api.github.com
  patience exp -b 1s -x 2.0 -- curl https://httpbin.org/status/503`,
}
```

The `init()` function registers all 10 strategy subcommands with the root command. Each strategy is a separate Cobra subcommand (like `git commit`, `docker run`):

```bash
sed -n '143,155p' /home/user/patience/cmd/patience/main.go
```

```output
func init() {
	// Add strategy subcommands
	rootCmd.AddCommand(createHTTPAwareCommand())
	rootCmd.AddCommand(createExponentialCommand())
	rootCmd.AddCommand(createLinearCommand())
	rootCmd.AddCommand(createFixedCommand())
	rootCmd.AddCommand(createJitterCommand())
	rootCmd.AddCommand(createDecorrelatedJitterCommand())
	rootCmd.AddCommand(createFibonacciCommand())
	rootCmd.AddCommand(createPolynomialCommand())
	rootCmd.AddCommand(createAdaptiveCommand())
	rootCmd.AddCommand(createDiophantineCommand())
}
```

Each strategy subcommand is created by a `create*Command()` function that registers all 10 strategies. When a user runs `patience exponential -- curl http://...`, Cobra routes to the `exponential` subcommand's `RunE` handler.

## Strategy Subcommands and Common Configuration

Every strategy shares a common set of options via `CommonConfig`. This struct centralizes the retry behavior that applies regardless of which timing strategy is chosen:

```bash
sed -n '31,45p' /home/user/patience/cmd/patience/subcommands.go
```

```output
type CommonConfig struct {
	Attempts        int           `json:"attempts"`
	Timeout         time.Duration `json:"timeout"`
	SuccessPattern  string        `json:"success_pattern"`
	FailurePattern  string        `json:"failure_pattern"`
	CaseInsensitive bool          `json:"case_insensitive"`
	ConfigFile      string        `json:"-"` // Config file path (not serialized)
	DebugConfig     bool          `json:"-"` // Debug config flag (not serialized)

	// Daemon configuration
	DaemonEnabled   bool          `json:"daemon_enabled"`
	DaemonSocket    string        `json:"daemon_socket"`
	DaemonTimeout   time.Duration `json:"daemon_timeout"`
	DaemonAutoStart bool          `json:"daemon_auto_start"`
}
```

Key fields: `Attempts` (1-1000), `Timeout` per attempt, `SuccessPattern`/`FailurePattern` for regex-based outcome detection, and daemon-related fields for multi-instance coordination.

These common flags are attached to every strategy subcommand via `addCommonFlags()`:

```bash
sed -n '137,145p' /home/user/patience/cmd/patience/subcommands.go
```

```output
func addCommonFlags(cmd *cobra.Command, config *CommonConfig) {
	cmd.Flags().IntVarP(&config.Attempts, "attempts", "a", 3, "Maximum retry attempts (1-1000)")
	cmd.Flags().DurationVarP(&config.Timeout, "timeout", "t", 0, "Timeout per attempt (0 = no timeout)")
	cmd.Flags().StringVar(&config.SuccessPattern, "success-pattern", "", "Regex pattern for success detection")
	cmd.Flags().StringVar(&config.FailurePattern, "failure-pattern", "", "Regex pattern for failure detection")
	cmd.Flags().BoolVar(&config.CaseInsensitive, "case-insensitive", false, "Case-insensitive pattern matching")
	cmd.Flags().StringVar(&config.ConfigFile, "config", "", "Configuration file path")
	cmd.Flags().BoolVar(&config.DebugConfig, "debug-config", false, "Show configuration debug information")
}
```

Here's how a concrete strategy subcommand is built -- the exponential command is a good representative example. It defines its own strategy-specific config (`ExponentialConfig`), creates a Cobra command with strategy-specific flags (`--base-delay`, `--multiplier`, `--max-delay`), then appends the common flags:

```bash
sed -n '210,267p' /home/user/patience/cmd/patience/subcommands.go
```

```output
func createExponentialCommand() *cobra.Command {
	var strategyConfig ExponentialConfig
	var commonConfig CommonConfig = NewCommonConfig()

	cmd := &cobra.Command{
		Use:     "exponential [OPTIONS] -- COMMAND [ARGS...]",
		Aliases: []string{"exp"},
		Short:   "Exponentially increasing delays",
		Long: `Exponential backoff strategy with configurable base delay, multiplier, and maximum delay.
Each retry attempt increases the delay by the specified multiplier.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check if we have any arguments
			if len(args) == 0 {
				return fmt.Errorf("no command specified after '--'")
			}

			// Load configuration with precedence (file, env, flags)
			cfg, err := loadConfigWithPrecedence(cmd, &commonConfig)
			if err != nil {
				return err
			}

			// Update common config from loaded configuration
			commonConfig.Attempts = cfg.Attempts
			commonConfig.Timeout = cfg.Timeout
			commonConfig.SuccessPattern = cfg.SuccessPattern
			commonConfig.FailurePattern = cfg.FailurePattern
			commonConfig.CaseInsensitive = cfg.CaseInsensitive

			// Validate configurations
			if err := commonConfig.Validate(); err != nil {
				return err
			}

			if err := strategyConfig.Validate(); err != nil {
				return err
			}

			// Store parsed config and command for testing
			lastExponentialConfig = strategyConfig
			lastParsedCommand = args

			// Create strategy and execute
			return executeWithExponential(strategyConfig, commonConfig, args)
		},
	}

	// Add strategy-specific flags
	cmd.Flags().DurationVarP(&strategyConfig.BaseDelay, "base-delay", "b", 1*time.Second, "Base delay")
	cmd.Flags().Float64VarP(&strategyConfig.Multiplier, "multiplier", "x", 2.0, "Multiplier")
	cmd.Flags().DurationVarP(&strategyConfig.MaxDelay, "max-delay", "m", 60*time.Second, "Maximum delay")

	// Add common flags
	addCommonFlags(cmd, &commonConfig)

	return cmd
}
```

The flow inside each subcommand handler follows this pattern:
1. Validate arguments (require at least one command after `--`)
2. Load configuration with precedence: config file → environment variables → CLI flags
3. Validate both common and strategy-specific configs
4. Create the backoff strategy instance
5. Call `executeWithStrategy()` or an equivalent function

The `executeWithStrategy()` function is the common path for simpler strategies -- it wires up the executor and handles results:

```bash
sed -n '829,844p' /home/user/patience/cmd/patience/subcommands.go
```

```output
func executeWithStrategy(strategy backoff.Strategy, commonConfig CommonConfig, commandArgs []string) error {
	// Create executor
	exec, err := createExecutorFromConfig(strategy, commonConfig)
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	// Execute command
	result, err := exec.Run(commandArgs)
	if err != nil {
		return fmt.Errorf("execution error: %w", err)
	}

	// Handle results
	return handleExecutionResult(result, exec)
}
```

The `createExecutorFromConfig()` function is the factory that wires everything together -- strategy, timeout, condition checker, and UI reporter:

```bash
sed -n '404,432p' /home/user/patience/cmd/patience/subcommands.go
```

```output
func createExecutorFromConfig(strategy backoff.Strategy, config CommonConfig) (*executor.Executor, error) {
	// Create base executor with strategy and timeout
	var exec *executor.Executor

	if strategy != nil && config.Timeout > 0 {
		exec = executor.NewExecutorWithBackoffAndTimeout(config.Attempts, strategy, config.Timeout)
	} else if strategy != nil {
		exec = executor.NewExecutorWithBackoff(config.Attempts, strategy)
	} else if config.Timeout > 0 {
		exec = executor.NewExecutorWithTimeout(config.Attempts, config.Timeout)
	} else {
		exec = executor.NewExecutor(config.Attempts)
	}

	// Add condition checker if patterns specified
	if config.SuccessPattern != "" || config.FailurePattern != "" {
		checker, err := conditions.NewChecker(config.SuccessPattern, config.FailurePattern, config.CaseInsensitive)
		if err != nil {
			return nil, fmt.Errorf("failed to create condition checker: %w", err)
		}
		exec.Conditions = checker
	}

	// Add status reporter
	reporter := ui.NewReporter(os.Stderr)
	exec.Reporter = reporter

	return exec, nil
}
```

## The Configuration System

The config package (`pkg/config/config.go`) implements a layered configuration system using Viper. Configuration values are resolved with a clear precedence order: **defaults → config file → environment variables → CLI flags**.

The `Config` struct mirrors `CommonConfig` but is Viper-aware with `mapstructure` tags:

```bash
sed -n '15,31p' /home/user/patience/pkg/config/config.go
```

```output
type Config struct {
	Attempts        int           `mapstructure:"attempts"`
	Delay           time.Duration `mapstructure:"delay"`
	Timeout         time.Duration `mapstructure:"timeout"`
	BackoffType     string        `mapstructure:"backoff"`
	MaxDelay        time.Duration `mapstructure:"max_delay"`
	Multiplier      float64       `mapstructure:"multiplier"`
	SuccessPattern  string        `mapstructure:"success_pattern"`
	FailurePattern  string        `mapstructure:"failure_pattern"`
	CaseInsensitive bool          `mapstructure:"case_insensitive"`

	// Daemon configuration
	DaemonEnabled   bool          `mapstructure:"daemon_enabled"`
	DaemonSocket    string        `mapstructure:"daemon_socket"`
	DaemonTimeout   time.Duration `mapstructure:"daemon_timeout"`
	DaemonAutoStart bool          `mapstructure:"daemon_auto_start"`
}
```

The configuration auto-discovery searches for config files in the working directory:

```bash
sed -n '434,446p' /home/user/patience/pkg/config/config.go
```

```output
// It looks for .patience.toml, patience.toml, .patience.yaml, patience.yaml files
func FindConfigFile(dir string) string {
	configNames := []string{".patience.toml", "patience.toml", ".patience.yaml", "patience.yaml"}

	for _, name := range configNames {
		configPath := filepath.Join(dir, name)
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}

	return ""
}
```

Environment variables use the `PATIENCE_` prefix (e.g. `PATIENCE_ATTEMPTS=5`, `PATIENCE_TIMEOUT=10s`). The `MergeWithExplicitFlags()` method only applies CLI flag values that were explicitly set by the user, preventing defaults from overriding config file values -- a subtle but important detail for correct precedence.

The config system also includes a debug mode (`--debug-config`) that uses structured logging via `slog` to show exactly where each configuration value came from (default, config file, environment variable, or CLI flag).

## The Strategy Interface

The heart of patience's flexibility is the `Strategy` interface in `pkg/backoff/strategy.go` -- just one method:

```bash
sed -n '9,21p' /home/user/patience/pkg/backoff/strategy.go
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

Every backoff strategy implements this single-method interface. The `HTTPAwareStrategy` extends it for strategies that can also parse command output. Let's walk through each of the 10 strategy implementations.

### Fixed Strategy

The simplest -- returns the same delay every time:

```bash
sed -n '23,37p' /home/user/patience/pkg/backoff/strategy.go
```

```output
type Fixed struct {
	Duration time.Duration
}

// NewFixed creates a new Fixed backoff strategy
func NewFixed(duration time.Duration) *Fixed {
	return &Fixed{
		Duration: duration,
	}
}

// Delay returns the fixed duration for any attempt
func (f *Fixed) Delay(attempt int) time.Duration {
	return f.Duration
}
```

### Exponential Strategy

The classic exponential backoff: `delay = baseDelay × multiplier^(attempt-1)`, capped at `maxDelay`:

```bash
sed -n '58,75p' /home/user/patience/pkg/backoff/strategy.go
```

```output
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

With defaults (base=1s, multiplier=2.0, max=60s), this produces delays of: 1s, 2s, 4s, 8s, 16s, 32s, 60s, 60s...

### Linear Strategy

Delays grow by a constant increment: `delay = increment × attempt`:

```bash
sed -n '131,145p' /home/user/patience/pkg/backoff/strategy.go
```

```output
func (l *Linear) Delay(attempt int) time.Duration {
	if attempt <= 0 {
		return l.Increment
	}

	// Calculate linear delay: increment * attempt
	delay := time.Duration(attempt) * l.Increment

	// Apply max delay cap if set
	if l.MaxDelay > 0 && delay > l.MaxDelay {
		delay = l.MaxDelay
	}

	return delay
}
```

### Jitter Strategy

Adds full randomness to exponential backoff -- returns a random delay between 0 and the exponential value. This is critical for preventing the "thundering herd" problem where many clients retry at the same time:

```bash
sed -n '96,112p' /home/user/patience/pkg/backoff/strategy.go
```

```output
func (j *Jitter) Delay(attempt int) time.Duration {
	if attempt <= 0 {
		// For invalid attempts, return random delay between 0 and base delay
		return time.Duration(rand.Float64() * float64(j.BaseDelay))
	}

	// Calculate exponential delay: baseDelay * multiplier^(attempt-1)
	exponentialDelay := float64(j.BaseDelay) * math.Pow(j.Multiplier, float64(attempt-1))

	// Apply max delay cap if set
	if j.MaxDelay > 0 && time.Duration(exponentialDelay) > j.MaxDelay {
		exponentialDelay = float64(j.MaxDelay)
	}

	// Return random delay between 0 and exponential delay (full jitter)
	return time.Duration(rand.Float64() * exponentialDelay)
}
```

### Decorrelated Jitter Strategy

AWS's recommended algorithm: `random_between(base_delay, previous_delay × multiplier)`. Unlike regular jitter, this strategy is **stateful** -- it remembers the last delay and bases the next delay on it, creating better distribution:

```bash
sed -n '170,199p' /home/user/patience/pkg/backoff/strategy.go
```

```output
func (d *DecorrelatedJitter) Delay(attempt int) time.Duration {
	var upperBound time.Duration

	if attempt <= 0 || d.previousDelay == 0 {
		// For first attempt or invalid attempts, use base delay * multiplier as upper bound
		upperBound = time.Duration(float64(d.BaseDelay) * d.Multiplier)
	} else {
		// For subsequent attempts, use previous delay * multiplier as upper bound
		upperBound = time.Duration(float64(d.previousDelay) * d.Multiplier)
	}

	// Apply max delay cap if set
	if d.MaxDelay > 0 && upperBound > d.MaxDelay {
		upperBound = d.MaxDelay
	}

	// Ensure upper bound is at least base delay
	if upperBound < d.BaseDelay {
		upperBound = d.BaseDelay
	}

	// Calculate random delay between base delay and upper bound
	delayRange := upperBound - d.BaseDelay
	randomDelay := d.BaseDelay + time.Duration(rand.Float64()*float64(delayRange))

	// Store this delay as the previous delay for next calculation
	d.previousDelay = randomDelay

	return randomDelay
}
```

### Fibonacci Strategy

Delays follow the Fibonacci sequence (1, 1, 2, 3, 5, 8, 13, 21...) multiplied by a base delay. This provides growth between linear and exponential -- a good middle ground:

```bash
sed -n '220,252p' /home/user/patience/pkg/backoff/strategy.go
```

```output
func (f *Fibonacci) Delay(attempt int) time.Duration {
	if attempt <= 0 {
		return f.BaseDelay
	}

	// Calculate fibonacci number for the attempt
	fibNumber := fibonacci(attempt)

	// Calculate delay: baseDelay * fibonacci(attempt)
	delay := time.Duration(fibNumber) * f.BaseDelay

	// Apply max delay cap if set
	if f.MaxDelay > 0 && delay > f.MaxDelay {
		delay = f.MaxDelay
	}

	return delay
}

// fibonacci calculates the nth fibonacci number (1-based)
// Returns: 1, 1, 2, 3, 5, 8, 13, 21, 34, 55, ...
// Optimized iterative implementation with O(n) time complexity
func fibonacci(n int) int {
	if n <= 2 {
		return 1
	}

	a, b := 1, 1
	for i := 3; i <= n; i++ {
		a, b = b, a+b
	}
	return b
}
```

Note the O(n) iterative implementation of `fibonacci()` -- no recursion, no memoization needed since attempt numbers are small.

### Polynomial Strategy

`pkg/backoff/polynomial.go` implements configurable polynomial growth: `delay = base_delay × attempt^exponent`. The exponent parameter gives fine-grained control over the growth curve:

```bash
sed -n '9,54p' /home/user/patience/pkg/backoff/polynomial.go
```

```output
// PolynomialStrategy implements polynomial backoff: delay = base_delay * (attempt ^ exponent)
type PolynomialStrategy struct {
	baseDelay time.Duration
	exponent  float64
	maxDelay  time.Duration
}

// NewPolynomial creates a new polynomial backoff strategy
func NewPolynomial(baseDelay time.Duration, exponent float64, maxDelay time.Duration) (*PolynomialStrategy, error) {
	if baseDelay <= 0 {
		return nil, fmt.Errorf("base delay must be positive, got %v", baseDelay)
	}
	if exponent < 0 {
		return nil, fmt.Errorf("exponent must be non-negative, got %f", exponent)
	}
	if maxDelay <= 0 {
		return nil, fmt.Errorf("max delay must be positive, got %v", maxDelay)
	}
	if baseDelay > maxDelay {
		return nil, fmt.Errorf("base delay (%v) cannot be greater than max delay (%v)", baseDelay, maxDelay)
	}

	return &PolynomialStrategy{
		baseDelay: baseDelay,
		exponent:  exponent,
		maxDelay:  maxDelay,
	}, nil
}

// Delay calculates the delay for the given attempt using polynomial growth
func (p *PolynomialStrategy) Delay(attempt int) time.Duration {
	if attempt <= 0 {
		return p.baseDelay
	}

	// Calculate: base_delay * (attempt ^ exponent)
	multiplier := math.Pow(float64(attempt), p.exponent)
	delay := float64(p.baseDelay) * multiplier

	// Apply max delay cap
	if delay > float64(p.maxDelay) {
		return p.maxDelay
	}

	return time.Duration(delay)
}
```

With exponent=0.8 you get sublinear (gentle) growth. With exponent=2.0 you get quadratic growth. This is the only strategy that returns an error from its constructor -- it validates that base delay doesn't exceed max delay.

## HTTP-Aware Strategy

`pkg/backoff/http_aware.go` is the most practically useful strategy for API work. It parses the actual HTTP response output from commands like `curl -i` to extract server-specified retry timing. It checks three sources in order:

1. Standard `Retry-After` header
2. Rate limit headers (`X-RateLimit-Retry-After`, `X-RateLimit-Reset`)
3. JSON response bodies with retry fields (`retry_after`, `retryAfter`, `retry_in`, etc.)

If none of these are found, it falls back to a configurable strategy (default: exponential).

```bash
sed -n '49,81p' /home/user/patience/pkg/backoff/http_aware.go
```

```output
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

The header parsing uses pre-compiled regex patterns (set once in the constructor for performance). The JSON parser is particularly robust -- it uses brace-counting to find balanced JSON objects within the output, then checks each one for retry timing fields:

```bash
sed -n '131,193p' /home/user/patience/pkg/backoff/http_aware.go
```

```output
func (h *HTTPAware) parseJSONResponse(output string) time.Duration {
	// Limit search to first 10KB to avoid processing huge outputs
	const maxSearchSize = 10 * 1024
	if len(output) > maxSearchSize {
		output = output[:maxSearchSize]
	}

	// Look for JSON-like content
	if !strings.Contains(output, "{") {
		return 0
	}

	// Try to find balanced JSON objects and parse each one
	// This is more robust than first-{ to last-} which can span unrelated content
	retryFields := []string{"retry_after", "retry_after_seconds", "retryAfter", "retryAfterSeconds", "retry_in"}

	start := 0
	for {
		// Find next potential JSON start
		jsonStart := strings.Index(output[start:], "{")
		if jsonStart == -1 {
			break
		}
		jsonStart += start

		// Find matching closing brace using brace counting
		jsonEnd := findMatchingBrace(output, jsonStart)
		if jsonEnd == -1 {
			start = jsonStart + 1
			continue
		}

		jsonStr := output[jsonStart : jsonEnd+1]

		// Try to parse this JSON object
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
			start = jsonStart + 1
			continue
		}

		// Check for retry timing fields in this JSON object
		for _, field := range retryFields {
			if value, exists := data[field]; exists {
				switch v := value.(type) {
				case float64:
					return time.Duration(v) * time.Second
				case int:
					return time.Duration(v) * time.Second
				case string:
					if seconds, err := strconv.Atoi(v); err == nil {
						return time.Duration(seconds) * time.Second
					}
				}
			}
		}

		// This JSON didn't have retry fields, try next one
		start = jsonEnd + 1
	}

	return 0
}
```

## Adaptive Strategy

`pkg/backoff/adaptive.go` is the most sophisticated strategy. It uses machine learning concepts to learn which delay durations lead to successful retries. The key data structures:

```bash
sed -n '9,37p' /home/user/patience/pkg/backoff/adaptive.go
```

```output
// DelayBucket represents a range of delays for learning purposes
type DelayBucket struct {
	MinDelay     time.Duration
	MaxDelay     time.Duration
	SuccessRate  float64
	SampleCount  int
	TotalLatency time.Duration
}

// OutcomeRecord represents a single retry outcome for learning
type OutcomeRecord struct {
	Delay   time.Duration
	Success bool
	Latency time.Duration
}

// Adaptive implements a machine learning-inspired backoff strategy
// that learns from success/failure patterns to optimize retry timing
type Adaptive struct {
	fallbackStrategy Strategy
	learningRate     float64
	memoryWindow     int

	// Learning data structures (protected by mutex)
	mu             sync.RWMutex
	delayBuckets   map[int]*DelayBucket
	recentOutcomes []OutcomeRecord
	totalOutcomes  int
}
```

The strategy maintains delay buckets with exponential ranges (0-1s, 1-2s, 2-5s, 5-10s, 10-30s, 30-60s, 60-300s). Each bucket tracks its success rate using an exponential moving average (EMA). When choosing a delay, the strategy:

1. Falls back to the base strategy until it has at least 3 data points
2. Finds the bucket with the highest success rate (minimum 2 samples)
3. Picks the midpoint of that bucket's range
4. **Blends** the learned optimal delay with the fallback strategy using the learning rate

This blending is what makes it work well from the start -- with learning rate 0.1, the strategy is 90% fallback initially, gradually shifting weight toward learned behavior:

```bash
sed -n '148,159p' /home/user/patience/pkg/backoff/adaptive.go
```

```output
// blendDelays combines learned optimal delay with fallback using learning rate
func (a *Adaptive) blendDelays(optimal, fallback time.Duration) time.Duration {
	// Learning rate determines how much to trust learned vs fallback
	optimalWeight := a.learningRate
	fallbackWeight := 1.0 - a.learningRate

	blended := time.Duration(
		float64(optimal)*optimalWeight + float64(fallback)*fallbackWeight,
	)

	return blended
}
```

The EMA formula for updating success rates is: `new_rate = (1-α) × old_rate + α × outcome`, where α is the learning rate and outcome is 1.0 for success, 0.0 for failure. The `recentOutcomes` ring buffer (size = `memoryWindow`) ensures old data ages out, keeping the strategy responsive to changing conditions.

All mutable state is protected by a `sync.RWMutex` with careful locking discipline -- read locks for `Delay()`, write locks for `RecordOutcome()`, and double-checked locking for lazy bucket initialization.

## Diophantine Strategy

`pkg/backoff/diophantine.go` takes a completely different approach. Instead of reacting to failures, it **proactively prevents rate limit violations** using mathematical modeling. The core idea: given a rate limit (e.g. 100 requests per hour), can we add a new request (with its planned retries) without exceeding the limit in any time window?

```bash
sed -n '8,55p' /home/user/patience/pkg/backoff/diophantine.go
```

```output
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

The algorithm takes the new request's planned retry schedule (e.g. initial attempt + retries at +10m and +30m), merges them with all existing scheduled requests, sorts by time, then slides a window across all request times checking that no window contains more than `rateLimit` requests. The `Delay()` method is a simple exponential fallback for when the daemon isn't available.

This strategy is designed for use with the optional daemon (`patienced`) that coordinates across multiple patience instances.

## Condition Checking

`pkg/conditions/conditions.go` determines whether a command execution "succeeded." By default, success = exit code 0. But users can specify regex patterns for more nuanced detection:

```bash
sed -n '61,96p' /home/user/patience/pkg/conditions/conditions.go
```

```output
// CheckSuccess determines if a command execution was successful
// It checks patterns first, then falls back to exit code
func (c *Checker) CheckSuccess(exitCode int, stdout, stderr string) Result {
	// Check failure pattern first (takes precedence)
	if c.failurePattern != nil {
		if c.failurePattern.MatchString(stdout) || c.failurePattern.MatchString(stderr) {
			return Result{
				Success: false,
				Reason:  "failure pattern matched",
			}
		}
	}

	// Check success pattern
	if c.successPattern != nil {
		if c.successPattern.MatchString(stdout) || c.successPattern.MatchString(stderr) {
			return Result{
				Success: true,
				Reason:  "success pattern matched",
			}
		}
	}

	// Fall back to exit code
	if exitCode == 0 {
		return Result{
			Success: true,
			Reason:  "exit code 0",
		}
	} else {
		return Result{
			Success: false,
			Reason:  fmt.Sprintf("exit code %d", exitCode),
		}
	}
}
```

The precedence is deliberate: **failure pattern > success pattern > exit code**. This means you can match a failure pattern even when the exit code is 0 -- useful for commands that exit successfully but print error messages. Both patterns are checked against both stdout and stderr.

## The Executor: The Retry Engine

`pkg/executor/executor.go` is the core of patience. The `Executor` struct ties everything together:

```bash
sed -n '141,151p' /home/user/patience/pkg/executor/executor.go
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

The `Runner` field uses a `CommandRunner` interface, allowing the real `SystemCommandRunner` to be swapped out in tests. The `SystemCommandRunner` handles the gritty details of process execution:

```bash
sed -n '75,139p' /home/user/patience/pkg/executor/executor.go
```

```output
func (r *SystemCommandRunner) RunWithOutputAndContext(ctx context.Context, command []string) (CommandOutput, error) {
	if len(command) == 0 {
		return CommandOutput{ExitCode: -1}, nil
	}

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)

	// Process cleanup improvement: Set process group for better signal handling
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	// Inherit parent environment without modifications
	// Note: Previously set CURL_CA_BUNDLE="" which disabled TLS certificate validation,
	// creating a security vulnerability. Users should configure curl timeouts explicitly
	// via command arguments if needed (e.g., curl --connect-timeout 10)
	cmd.Env = os.Environ()

	// Capture stdout and stderr while also forwarding to terminal
	// Use limited buffers for large outputs
	stdoutBuf := &limitedBuffer{limit: DefaultMaxBufferSize}
	stderrBuf := &limitedBuffer{limit: DefaultMaxBufferSize}
	cmd.Stdout = io.MultiWriter(os.Stdout, stdoutBuf)
	cmd.Stderr = io.MultiWriter(os.Stderr, stderrBuf)

	// Ensure process group cleanup on context cancellation.
	// Using cmd.Cancel avoids the data race that occurs when accessing
	// cmd.Process from a separate goroutine while cmd.Run() is executing.
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			// Kill process group to ensure all child processes are terminated
			return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		}
		return nil
	}

	err := cmd.Run()

	output := CommandOutput{
		Stdout: stdoutBuf.String(),
		Stderr: stderrBuf.String(),
	}

	// Memory management optimization: Clear buffers after copying strings
	defer func() {
		stdoutBuf.Reset()
		stderrBuf.Reset()
		// Memory cleanup - removed manual GC call as per best practices
	}()

	if err != nil {
		// Check for context deadline exceeded (timeout)
		if ctx.Err() == context.DeadlineExceeded {
			output.ExitCode = -1
			return output, context.DeadlineExceeded
		}
		if exitError, ok := err.(*exec.ExitError); ok {
			output.ExitCode = exitError.ExitCode()
			return output, nil
		}
		output.ExitCode = -1
		return output, err
	}

	output.ExitCode = 0
	return output, nil
}
```

Several important details here:
- **Process groups**: `Setpgid: true` creates a new process group so that timeout kills reach child processes too. The `cmd.Cancel` callback sends SIGTERM to the entire group via negative PID.
- **Dual output**: `io.MultiWriter` sends output to both the terminal (so the user sees it in real-time) and to a `limitedBuffer` (for pattern matching and HTTP parsing).
- **limitedBuffer**: A custom buffer that silently stops accepting data after a size limit, preventing memory exhaustion from commands with huge output.
- **Security**: The environment is inherited cleanly -- a comment notes that a previous version incorrectly cleared `CURL_CA_BUNDLE` which disabled TLS validation.

Now the main event -- the `Run()` method, the retry loop:

```bash
sed -n '441,552p' /home/user/patience/pkg/executor/executor.go
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

	// All attempts failed - determine final reason
	finalReason := e.determineFinalReason(lastOutput, timedOut)
	stats.Finalize(false, finalReason)

	return e.buildFinalResult(false, e.MaxAttempts, lastOutput, timedOut, finalReason, stats, attemptMetrics, runStartTime, command, lastError), lastError
}
```

The retry loop is the heart of the whole system. Each iteration:

1. **Reports** attempt start via the UI reporter
2. **Executes** the command (with timeout if configured)
3. **Evaluates** the result using the condition checker (or exit code)
4. **Records** metrics for the attempt
5. **Feeds back** to adaptive/HTTP-aware strategies via `recordStrategyOutcome()` and `ProcessCommandOutput()`
6. **Decides** whether to stop (success, failure pattern matched, or last attempt)
7. **Calculates delay** from the backoff strategy and sleeps

The loop also correctly handles the "failure pattern matched" case as a hard stop -- if the output matches a failure pattern, there's no point retrying because the failure is deterministic.

## UI and Reporting

`pkg/ui/ui.go` handles all terminal output. The `Reporter` writes status messages to stderr (so they don't interfere with the command's stdout):

```bash
sed -n '43,77p' /home/user/patience/pkg/ui/ui.go
```

```output
func (r *Reporter) AttemptStart(attempt, maxAttempts int) {
	if r.quiet {
		return
	}
	fmt.Fprintf(r.writer, "[retry] Attempt %d/%d starting...\n", attempt, maxAttempts)
}

// AttemptFailure reports a failed attempt with reason and next delay
func (r *Reporter) AttemptFailure(attempt, maxAttempts int, reason string, nextDelay time.Duration) {
	if r.quiet {
		return
	}

	// High-frequency optimization: Use string builder for efficient string operations
	var builder strings.Builder
	builder.WriteString("[retry] Attempt ")
	builder.WriteString(strconv.Itoa(attempt))
	builder.WriteByte('/')
	builder.WriteString(strconv.Itoa(maxAttempts))
	builder.WriteString(" failed (")
	builder.WriteString(reason)
	builder.WriteString(")")

	if attempt == maxAttempts {
		// Last attempt, no retry
		builder.WriteString(".\n")
	} else {
		// Will retry
		builder.WriteString(". Retrying in ")
		builder.WriteString(r.formatDuration(nextDelay))
		builder.WriteString(".\n")
	}

	fmt.Fprint(r.writer, builder.String())
}
```

The `AttemptFailure` method uses `strings.Builder` instead of `fmt.Sprintf` -- a deliberate performance optimization for high-frequency calls. The `FinalSummary` method prints the overall outcome with statistics (total attempts, success/fail counts, duration).

The `RunStats` struct tracks timing across the entire retry run and individual attempts, used to produce the final summary.

## Metrics System

`pkg/metrics/metrics.go` collects detailed per-attempt and per-run metrics. These can optionally be sent to a daemon process for aggregation:

```bash
sed -n '37,89p' /home/user/patience/pkg/metrics/metrics.go
```

```output
type RunMetrics struct {
	Command              string          `json:"command"`
	CommandHash          string          `json:"command_hash"`
	FinalStatus          string          `json:"final_status"` // "succeeded" or "failed"
	TotalDurationSeconds float64         `json:"total_duration_seconds"`
	TotalAttempts        int             `json:"total_attempts"`
	SuccessfulAttempts   int             `json:"successful_attempts"`
	FailedAttempts       int             `json:"failed_attempts"`
	Attempts             []AttemptMetric `json:"attempts"`
	Timestamp            int64           `json:"timestamp"` // Unix timestamp
}

// NewRunMetrics creates a new RunMetrics instance
func NewRunMetrics(command []string, success bool, totalDuration time.Duration, attempts []AttemptMetric) *RunMetrics {
	commandStr := strings.Join(command, " ")

	var finalStatus string
	if success {
		finalStatus = "succeeded"
	} else {
		finalStatus = "failed"
	}

	successfulAttempts := 0
	failedAttempts := 0
	for _, attempt := range attempts {
		if attempt.Success {
			successfulAttempts++
		} else {
			failedAttempts++
		}
	}

	return &RunMetrics{
		Command:              commandStr,
		CommandHash:          generateCommandHash(command),
		FinalStatus:          finalStatus,
		TotalDurationSeconds: float64(totalDuration) / float64(time.Second),
		TotalAttempts:        len(attempts),
		SuccessfulAttempts:   successfulAttempts,
		FailedAttempts:       failedAttempts,
		Attempts:             attempts,
		Timestamp:            time.Now().Unix(),
	}
}

// generateCommandHash creates a consistent hash for a command
func generateCommandHash(command []string) string {
	commandStr := strings.Join(command, " ")
	hash := sha256.Sum256([]byte(commandStr))
	// Return first 8 characters of hex representation
	return fmt.Sprintf("%x", hash)[:8]
}
```

The `CommandHash` field uses the first 8 hex characters of a SHA-256 hash, providing a consistent identifier for grouping metrics from the same command across runs.

Metrics are sent asynchronously via Unix socket using fire-and-forget semantics -- the 100ms timeout ensures patience never blocks waiting for the metrics daemon:

```bash
sed -n '137,143p' /home/user/patience/pkg/metrics/metrics.go
```

```output
// SendMetricsAsync sends metrics to the daemon asynchronously (fire-and-forget)
func (c *Client) SendMetricsAsync(metrics *RunMetrics) {
	go func() {
		// Ignore errors in async mode - this is fire-and-forget
		_ = c.SendMetrics(metrics)
	}()
}
```

## Tying It All Together: handleExecutionResult

After the executor's retry loop completes, control returns to `handleExecutionResult()` in `subcommands.go`, which handles the final summary, metrics, and process exit:

```bash
sed -n '435,468p' /home/user/patience/cmd/patience/subcommands.go
```

```output
func handleExecutionResult(result *executor.Result, exec *executor.Executor) error {
	// Show final summary if we have statistics
	if result.Stats != nil && exec.Reporter != nil {
		exec.Reporter.FinalSummary(result.Stats)
	}

	// Send metrics to daemon asynchronously (fire-and-forget)
	if result.Metrics != nil {
		metricsClient := metrics.NewClient(metrics.DefaultSocketPath())
		metricsClient.SendMetricsAsync(result.Metrics)
	}

	// Exit with appropriate code based on success (skip during tests)
	if !testMode {
		if result.Success {
			os.Exit(0)
		} else {
			// If failure was due to pattern matching, use exit code 1
			// Otherwise use the original exit code
			if strings.Contains(result.Reason, "failure pattern matched") {
				os.Exit(1)
			} else {
				os.Exit(result.ExitCode)
			}
		}
	}

	// Return error for test mode to indicate failure
	if !result.Success {
		return fmt.Errorf("command failed: %s", result.Reason)
	}

	return nil
}
```

Notice how the exit code is carefully preserved: success exits 0, failure pattern matches always exit 1, and other failures propagate the original command's exit code. The `testMode` flag prevents `os.Exit()` during testing.

## Summary: The Complete Flow

When you run `patience exponential --base-delay 2s --attempts 5 -- curl https://api.example.com`:

1. **main.go**: `rootCmd.Execute()` dispatches to the `exponential` subcommand
2. **subcommands.go**: `createExponentialCommand()`'s handler loads config (file → env → flags), validates, creates an `Exponential` strategy (base=2s, mult=2.0, max=60s)
3. **subcommands.go**: `createExecutorFromConfig()` wires up the executor with the strategy, optional condition checker, and UI reporter
4. **executor.go**: `Run()` enters the retry loop:
   - Attempt 1: runs curl, checks exit code → fails (503)
   - Strategy calculates 2s delay, reporter prints `[retry] Attempt 1/5 failed (exit code 22). Retrying in 2s.`
   - Sleeps 2 seconds
   - Attempt 2: runs curl again → fails (503)
   - Strategy calculates 4s delay (2s × 2.0)
   - ...continues until success or attempt 5
5. **subcommands.go**: `handleExecutionResult()` prints the final summary, fires off metrics async, and exits with the appropriate code

### Design Principles

- **Interface-driven**: The `Strategy` interface is a single method. Adding a new strategy means implementing one function.
- **Separation of concerns**: Strategy (when to retry), Conditions (what counts as success), Executor (the retry loop), Reporter (output), Metrics (telemetry) are all independent.
- **Defense in depth**: Limited buffers prevent memory exhaustion, output processing is capped at 10KB, process groups ensure clean timeout kills, and TLS is never silently disabled.
- **Progressive complexity**: From `fixed` (one parameter) to `adaptive` (ML-based learning) to `diophantine` (mathematical rate limit modeling), users can pick the complexity level that matches their needs.
