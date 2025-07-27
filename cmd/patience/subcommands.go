package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/shaneisley/patience/pkg/backoff"
	"github.com/shaneisley/patience/pkg/conditions"
	"github.com/shaneisley/patience/pkg/executor"
	"github.com/shaneisley/patience/pkg/metrics"
	"github.com/shaneisley/patience/pkg/ui"
	"github.com/spf13/cobra"
)

// Global variables to store parsed configurations for testing
var (
	lastHTTPAwareConfig   HTTPAwareConfig
	lastExponentialConfig ExponentialConfig
	lastParsedCommand     []string
)

// CommonConfig holds configuration options common to all strategies
type CommonConfig struct {
	Attempts        int           `json:"attempts"`
	Timeout         time.Duration `json:"timeout"`
	SuccessPattern  string        `json:"success_pattern"`
	FailurePattern  string        `json:"failure_pattern"`
	CaseInsensitive bool          `json:"case_insensitive"`
}

// Validate validates the common configuration
func (c CommonConfig) Validate() error {
	if c.Attempts < 1 || c.Attempts > 1000 {
		return fmt.Errorf("attempts must be between 1 and 1000, got %d", c.Attempts)
	}

	if c.Timeout < 0 {
		return fmt.Errorf("timeout must be non-negative, got %v", c.Timeout)
	}

	// Validate regex patterns
	if c.SuccessPattern != "" {
		if _, err := regexp.Compile(c.SuccessPattern); err != nil {
			return fmt.Errorf("invalid success pattern: %w", err)
		}
	}

	if c.FailurePattern != "" {
		if _, err := regexp.Compile(c.FailurePattern); err != nil {
			return fmt.Errorf("invalid failure pattern: %w", err)
		}
	}

	return nil
}

// NewCommonConfig creates a new CommonConfig with default values
func NewCommonConfig() CommonConfig {
	return CommonConfig{
		Attempts:        3,
		Timeout:         0, // No timeout by default
		SuccessPattern:  "",
		FailurePattern:  "",
		CaseInsensitive: false,
	}
}

// HTTPAwareConfig holds configuration for HTTP-aware strategy
type HTTPAwareConfig struct {
	Fallback string
	MaxDelay time.Duration
}

// Validate validates the HTTP-aware configuration
func (h HTTPAwareConfig) Validate() error {
	if h.MaxDelay < 0 {
		return fmt.Errorf("max-delay must be non-negative, got %v", h.MaxDelay)
	}

	validFallbacks := []string{"exponential", "exp", "linear", "lin", "fixed", "fix", "jitter", "jit", "decorrelated-jitter", "dj", "fibonacci", "fib"}
	for _, valid := range validFallbacks {
		if h.Fallback == valid {
			return nil
		}
	}

	return fmt.Errorf("unknown fallback strategy: %s", h.Fallback)
}

// ExponentialConfig holds configuration for exponential strategy
type ExponentialConfig struct {
	BaseDelay  time.Duration
	Multiplier float64
	MaxDelay   time.Duration
}

// Validate validates the exponential configuration
func (e ExponentialConfig) Validate() error {
	if e.BaseDelay < 0 {
		return fmt.Errorf("base-delay must be non-negative, got %v", e.BaseDelay)
	}

	if e.Multiplier <= 0 {
		return fmt.Errorf("multiplier must be positive, got %f", e.Multiplier)
	}

	if e.MaxDelay < 0 {
		return fmt.Errorf("max-delay must be non-negative, got %v", e.MaxDelay)
	}

	return nil
}

// addCommonFlags adds common configuration flags to a command
func addCommonFlags(cmd *cobra.Command, config *CommonConfig) {
	cmd.Flags().IntVarP(&config.Attempts, "attempts", "a", 3, "Maximum retry attempts (1-1000)")
	cmd.Flags().DurationVarP(&config.Timeout, "timeout", "t", 0, "Timeout per attempt (0 = no timeout)")
	cmd.Flags().StringVar(&config.SuccessPattern, "success-pattern", "", "Regex pattern for success detection")
	cmd.Flags().StringVar(&config.FailurePattern, "failure-pattern", "", "Regex pattern for failure detection")
	cmd.Flags().BoolVar(&config.CaseInsensitive, "case-insensitive", false, "Case-insensitive pattern matching")
}

// createHTTPAwareCommand creates the http-aware subcommand
func createHTTPAwareCommand() *cobra.Command {
	var strategyConfig HTTPAwareConfig
	var commonConfig CommonConfig = NewCommonConfig()

	cmd := &cobra.Command{
		Use:     "http-aware [OPTIONS] -- COMMAND [ARGS...]",
		Aliases: []string{"ha"},
		Short:   "HTTP response-aware delays",
		Long: `HTTP response-aware retry strategy that parses server responses to determine
optimal retry timing. Works with curl and other HTTP tools.

Respects Retry-After headers and JSON retry fields from HTTP responses.
Falls back to specified strategy when no HTTP information is available.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Check if we have any arguments
			if len(args) == 0 {
				return fmt.Errorf("no command specified after '--'")
			}

			// Validate configurations
			if err := commonConfig.Validate(); err != nil {
				return err
			}

			if err := strategyConfig.Validate(); err != nil {
				return err
			}

			// Store parsed config and command for testing
			lastHTTPAwareConfig = strategyConfig
			lastParsedCommand = args

			// Execute with integrated executor
			return executeWithHTTPAware(strategyConfig, commonConfig, args)
		},
	}

	// Add strategy-specific flags
	cmd.Flags().StringVarP(&strategyConfig.Fallback, "fallback", "f", "exponential", "Fallback strategy when no HTTP info available")
	cmd.Flags().DurationVarP(&strategyConfig.MaxDelay, "max-delay", "m", 30*time.Minute, "Maximum delay cap")

	// Add common flags
	addCommonFlags(cmd, &commonConfig)

	return cmd
}

// createExponentialCommand creates the exponential subcommand
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

// parseCommandArgs finds the -- separator and returns the command arguments
func parseCommandArgs(args []string) ([]string, error) {
	for i, arg := range args {
		if arg == "--" {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("no command specified after '--'")
			}
			return args[i+1:], nil
		}
	}
	return nil, fmt.Errorf("command separator '--' not found")
}

// executeWithHTTPAware executes command with HTTP-aware strategy
func executeWithHTTPAware(strategyConfig HTTPAwareConfig, commonConfig CommonConfig, commandArgs []string) error {
	// Create fallback strategy
	fallbackStrategy, err := createFallbackStrategy(strategyConfig.Fallback)
	if err != nil {
		return fmt.Errorf("failed to create fallback strategy: %w", err)
	}

	// Create HTTP-aware strategy
	strategy := backoff.NewHTTPAware(fallbackStrategy, strategyConfig.MaxDelay)

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

// executeWithExponential executes command with exponential strategy
func executeWithExponential(strategyConfig ExponentialConfig, commonConfig CommonConfig, commandArgs []string) error {
	// Create exponential strategy
	strategy := backoff.NewExponential(strategyConfig.BaseDelay, strategyConfig.Multiplier, strategyConfig.MaxDelay)

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

// createExecutorFromConfig creates an executor from strategy and common configuration
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

// handleExecutionResult handles the result of command execution
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

	// Exit with appropriate code based on success
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

	return nil // This line won't be reached due to os.Exit
}

// createFallbackStrategy creates a fallback strategy from the given type
func createFallbackStrategy(fallbackType string) (backoff.Strategy, error) {
	switch fallbackType {
	case "exponential", "exp":
		return backoff.NewExponential(1*time.Second, 2.0, 60*time.Second), nil
	case "linear", "lin":
		return backoff.NewLinear(1*time.Second, 60*time.Second), nil
	case "fixed", "fix":
		return backoff.NewFixed(1 * time.Second), nil
	case "jitter", "jit":
		return backoff.NewJitter(1*time.Second, 2.0, 60*time.Second), nil
	case "decorrelated-jitter", "dj":
		return backoff.NewDecorrelatedJitter(1*time.Second, 2.0, 60*time.Second), nil
	case "fibonacci", "fib":
		return backoff.NewFibonacci(1*time.Second, 60*time.Second), nil
	default:
		return nil, fmt.Errorf("unknown fallback strategy: %s", fallbackType)
	}
}

// Test helper functions
func getLastParsedHTTPAwareConfig() HTTPAwareConfig {
	return lastHTTPAwareConfig
}

func getLastParsedExponentialConfig() ExponentialConfig {
	return lastExponentialConfig
}

func getLastParsedCommand() []string {
	return lastParsedCommand
}

// createTestRootCommand creates a root command for testing
func createTestRootCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "patience",
		Short: "Intelligent retry wrapper with adaptive backoff strategies",
	}

	// Add subcommands
	rootCmd.AddCommand(createHTTPAwareCommand())
	rootCmd.AddCommand(createExponentialCommand())

	return rootCmd
}

// parseArgsWithSeparator handles the -- separator manually
func parseArgsWithSeparator(allArgs []string) ([]string, error) {
	// Find the -- separator in the original arguments
	for i, arg := range allArgs {
		if arg == "--" {
			if i+1 >= len(allArgs) {
				return nil, fmt.Errorf("no command specified after '--'")
			}
			return allArgs[i+1:], nil
		}
	}
	return nil, fmt.Errorf("command separator '--' not found")
}

// createStrategyFromConfig creates a strategy from the given configuration
func createStrategyFromConfig(strategyType string, config interface{}) (backoff.Strategy, error) {
	switch strategyType {
	case "http-aware", "ha":
		httpConfig, ok := config.(HTTPAwareConfig)
		if !ok {
			return nil, fmt.Errorf("invalid config type for http-aware strategy")
		}

		// Create fallback strategy
		var fallbackStrategy backoff.Strategy
		switch httpConfig.Fallback {
		case "exponential", "exp":
			fallbackStrategy = backoff.NewExponential(1*time.Second, 2.0, 60*time.Second)
		case "linear":
			fallbackStrategy = backoff.NewLinear(1*time.Second, 60*time.Second)
		case "fixed":
			fallbackStrategy = backoff.NewFixed(1 * time.Second)
		default:
			fallbackStrategy = backoff.NewExponential(1*time.Second, 2.0, 60*time.Second)
		}

		return backoff.NewHTTPAware(fallbackStrategy, httpConfig.MaxDelay), nil

	case "exponential", "exp":
		expConfig, ok := config.(ExponentialConfig)
		if !ok {
			return nil, fmt.Errorf("invalid config type for exponential strategy")
		}

		return backoff.NewExponential(expConfig.BaseDelay, expConfig.Multiplier, expConfig.MaxDelay), nil

	default:
		return nil, fmt.Errorf("unknown strategy type: %s", strategyType)
	}
}

// getStrategyTypeName returns the type name of a strategy for testing
func getStrategyTypeName(strategy backoff.Strategy) string {
	return fmt.Sprintf("%T", strategy)
}

// Additional strategy configurations
type LinearConfig struct {
	Increment time.Duration
	MaxDelay  time.Duration
}

type FixedConfig struct {
	Delay time.Duration
}

type JitterConfig struct {
	BaseDelay  time.Duration
	Multiplier float64
	MaxDelay   time.Duration
}

type DecorrelatedJitterConfig struct {
	BaseDelay  time.Duration
	Multiplier float64
	MaxDelay   time.Duration
}

type FibonacciConfig struct {
	BaseDelay time.Duration
	MaxDelay  time.Duration
}

// PolynomialConfig holds configuration for polynomial backoff strategy
type PolynomialConfig struct {
	BaseDelay time.Duration
	Exponent  float64
	MaxDelay  time.Duration
}

// AdaptiveConfig holds configuration for adaptive backoff strategy
type AdaptiveConfig struct {
	LearningRate     float64
	MemoryWindow     int
	FallbackStrategy string
	FallbackConfig   interface{}
}

// createLinearCommand creates the linear subcommand
func createLinearCommand() *cobra.Command {
	var strategyConfig LinearConfig
	var commonConfig CommonConfig = NewCommonConfig()

	cmd := &cobra.Command{
		Use:     "linear [OPTIONS] -- COMMAND [ARGS...]",
		Aliases: []string{"lin"},
		Short:   "Linearly increasing delays",
		Long:    "Linear backoff strategy with configurable increment and maximum delay.",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no command specified after '--'")
			}

			// Validate configurations
			if err := commonConfig.Validate(); err != nil {
				return err
			}

			strategy := backoff.NewLinear(strategyConfig.Increment, strategyConfig.MaxDelay)
			return executeWithStrategy(strategy, commonConfig, args)
		},
	}

	cmd.Flags().DurationVarP(&strategyConfig.Increment, "increment", "i", 1*time.Second, "Delay increment")
	cmd.Flags().DurationVarP(&strategyConfig.MaxDelay, "max-delay", "m", 60*time.Second, "Maximum delay")

	// Add common flags
	addCommonFlags(cmd, &commonConfig)

	return cmd
}

// createFixedCommand creates the fixed subcommand
func createFixedCommand() *cobra.Command {
	var strategyConfig FixedConfig
	var commonConfig CommonConfig = NewCommonConfig()

	cmd := &cobra.Command{
		Use:     "fixed [OPTIONS] -- COMMAND [ARGS...]",
		Aliases: []string{"fix"},
		Short:   "Fixed delay between retries",
		Long:    "Fixed backoff strategy with constant delay between retries.",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no command specified after '--'")
			}

			// Validate configurations
			if err := commonConfig.Validate(); err != nil {
				return err
			}

			strategy := backoff.NewFixed(strategyConfig.Delay)
			return executeWithStrategy(strategy, commonConfig, args)
		},
	}

	cmd.Flags().DurationVarP(&strategyConfig.Delay, "delay", "d", 1*time.Second, "Fixed delay")

	// Add common flags
	addCommonFlags(cmd, &commonConfig)

	return cmd
}

// createJitterCommand creates the jitter subcommand
func createJitterCommand() *cobra.Command {
	var strategyConfig JitterConfig
	var commonConfig CommonConfig = NewCommonConfig()

	cmd := &cobra.Command{
		Use:     "jitter [OPTIONS] -- COMMAND [ARGS...]",
		Aliases: []string{"jit"},
		Short:   "Random jitter around base delay",
		Long:    "Jitter backoff strategy with random delays around a base value.",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no command specified after '--'")
			}

			// Validate configurations
			if err := commonConfig.Validate(); err != nil {
				return err
			}

			strategy := backoff.NewJitter(strategyConfig.BaseDelay, strategyConfig.Multiplier, strategyConfig.MaxDelay)
			return executeWithStrategy(strategy, commonConfig, args)
		},
	}

	cmd.Flags().DurationVarP(&strategyConfig.BaseDelay, "base-delay", "b", 1*time.Second, "Base delay")
	cmd.Flags().Float64VarP(&strategyConfig.Multiplier, "multiplier", "x", 2.0, "Multiplier")
	cmd.Flags().DurationVarP(&strategyConfig.MaxDelay, "max-delay", "m", 60*time.Second, "Maximum delay")

	// Add common flags
	addCommonFlags(cmd, &commonConfig)

	return cmd
}

// createDecorrelatedJitterCommand creates the decorrelated-jitter subcommand
func createDecorrelatedJitterCommand() *cobra.Command {
	var strategyConfig DecorrelatedJitterConfig
	var commonConfig CommonConfig = NewCommonConfig()

	cmd := &cobra.Command{
		Use:     "decorrelated-jitter [OPTIONS] -- COMMAND [ARGS...]",
		Aliases: []string{"dj"},
		Short:   "AWS-style decorrelated jitter",
		Long:    "Decorrelated jitter backoff strategy as used by AWS services.",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no command specified after '--'")
			}

			// Validate configurations
			if err := commonConfig.Validate(); err != nil {
				return err
			}

			strategy := backoff.NewDecorrelatedJitter(strategyConfig.BaseDelay, strategyConfig.Multiplier, strategyConfig.MaxDelay)
			return executeWithStrategy(strategy, commonConfig, args)
		},
	}

	cmd.Flags().DurationVarP(&strategyConfig.BaseDelay, "base-delay", "b", 1*time.Second, "Base delay")
	cmd.Flags().Float64VarP(&strategyConfig.Multiplier, "multiplier", "x", 2.0, "Multiplier")
	cmd.Flags().DurationVarP(&strategyConfig.MaxDelay, "max-delay", "m", 60*time.Second, "Maximum delay")

	// Add common flags
	addCommonFlags(cmd, &commonConfig)

	return cmd
}

// createFibonacciCommand creates the fibonacci subcommand
func createFibonacciCommand() *cobra.Command {
	var strategyConfig FibonacciConfig
	var commonConfig CommonConfig = NewCommonConfig()

	cmd := &cobra.Command{
		Use:     "fibonacci [OPTIONS] -- COMMAND [ARGS...]",
		Aliases: []string{"fib"},
		Short:   "Fibonacci sequence delays",
		Long:    "Fibonacci backoff strategy following the Fibonacci sequence.",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no command specified after '--'")
			}

			// Validate configurations
			if err := commonConfig.Validate(); err != nil {
				return err
			}

			strategy := backoff.NewFibonacci(strategyConfig.BaseDelay, strategyConfig.MaxDelay)
			return executeWithStrategy(strategy, commonConfig, args)
		},
	}

	cmd.Flags().DurationVarP(&strategyConfig.BaseDelay, "base-delay", "b", 1*time.Second, "Base delay")
	cmd.Flags().DurationVarP(&strategyConfig.MaxDelay, "max-delay", "m", 60*time.Second, "Maximum delay")

	// Add common flags
	addCommonFlags(cmd, &commonConfig)

	return cmd
}

// executeWithStrategy executes command with the given strategy
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

// createPolynomialCommand creates the polynomial subcommand
func createPolynomialCommand() *cobra.Command {
	var strategyConfig PolynomialConfig
	var commonConfig CommonConfig = NewCommonConfig()

	cmd := &cobra.Command{
		Use:     "polynomial [OPTIONS] -- COMMAND [ARGS...]",
		Aliases: []string{"poly"},
		Short:   "Polynomial backoff strategy with configurable growth rate",
		Long: `Polynomial backoff strategy uses the formula: delay = base_delay * (attempt ^ exponent)

The exponent parameter controls the growth rate:
- exponent < 1.0: Sublinear growth (gentle increase)
- exponent = 1.0: Linear growth (same as linear strategy)  
- exponent = 1.5: Moderate growth (balanced approach)
- exponent = 2.0: Quadratic growth (rapid increase)
- exponent > 2.0: Aggressive growth (approaches exponential)

Examples:
  # Quadratic backoff for database connections
  patience polynomial --base-delay 500ms --exponent 2.0 --max-delay 30s -- psql -h db.example.com
  
  # Moderate growth for API calls
  patience poly -b 1s -e 1.5 -m 60s -- curl https://api.example.com
  
  # Gentle sublinear growth for frequent operations
  patience polynomial --exponent 0.8 -- frequent-operation`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no command specified after '--'")
			}

			// Validate configurations
			if err := commonConfig.Validate(); err != nil {
				return err
			}

			// Validate polynomial-specific configuration
			if strategyConfig.BaseDelay <= 0 {
				return fmt.Errorf("base delay must be positive")
			}
			if strategyConfig.Exponent < 0 {
				return fmt.Errorf("exponent must be non-negative")
			}
			if strategyConfig.MaxDelay <= 0 {
				return fmt.Errorf("max delay must be positive")
			}
			if strategyConfig.BaseDelay > strategyConfig.MaxDelay {
				return fmt.Errorf("base delay cannot be greater than max delay")
			}

			// Create strategy
			strategy, err := backoff.NewPolynomial(strategyConfig.BaseDelay, strategyConfig.Exponent, strategyConfig.MaxDelay)
			if err != nil {
				return fmt.Errorf("failed to create polynomial strategy: %w", err)
			}

			return executeWithStrategy(strategy, commonConfig, args)
		},
	}

	// Set default values
	strategyConfig.BaseDelay = 1 * time.Second
	strategyConfig.Exponent = 2.0
	strategyConfig.MaxDelay = 60 * time.Second

	// Add strategy-specific flags
	cmd.Flags().DurationVarP(&strategyConfig.BaseDelay, "base-delay", "b", strategyConfig.BaseDelay,
		"Base delay for polynomial calculation")
	cmd.Flags().Float64VarP(&strategyConfig.Exponent, "exponent", "e", strategyConfig.Exponent,
		"Polynomial exponent (controls growth rate)")
	cmd.Flags().DurationVarP(&strategyConfig.MaxDelay, "max-delay", "m", strategyConfig.MaxDelay,
		"Maximum delay cap")

	// Add common flags
	addCommonFlags(cmd, &commonConfig)

	return cmd
}

// createAdaptiveCommand creates the adaptive subcommand
func createAdaptiveCommand() *cobra.Command {
	var strategyConfig AdaptiveConfig
	var commonConfig CommonConfig = NewCommonConfig()

	cmd := &cobra.Command{
		Use:     "adaptive [OPTIONS] -- COMMAND [ARGS...]",
		Aliases: []string{"adapt"},
		Short:   "Machine learning-inspired adaptive backoff strategy",
		Long: `Adaptive backoff strategy learns from success/failure patterns to optimize retry timing.

The strategy tracks outcomes and adjusts delays based on what works best for your specific command.
It uses a fallback strategy when insufficient learning data is available.

Key parameters:
- learning-rate: How quickly to adapt (0.01-1.0, default 0.1)
- memory-window: Number of recent outcomes to remember (5-10000, default 50)
- fallback: Strategy to use when learning data is insufficient

Examples:
  # Basic adaptive with exponential fallback
  patience adaptive --learning-rate 0.1 --memory-window 50 -- curl https://api.example.com
  
  # Fast learning for rapidly changing conditions
  patience adapt --learning-rate 0.5 --fallback fixed -- flaky-command
  
  # Conservative learning with large memory
  patience adaptive -r 0.05 -w 200 --fallback linear -- database-operation`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("no command specified after '--'")
			}

			// Validate configurations
			if err := commonConfig.Validate(); err != nil {
				return err
			}

			// Validate adaptive-specific configuration
			if strategyConfig.LearningRate <= 0 || strategyConfig.LearningRate > 1.0 {
				return fmt.Errorf("learning rate must be between 0.01 and 1.0, got %f", strategyConfig.LearningRate)
			}
			if strategyConfig.MemoryWindow <= 0 || strategyConfig.MemoryWindow > 10000 {
				return fmt.Errorf("memory window must be between 1 and 10000, got %d", strategyConfig.MemoryWindow)
			}

			// Create fallback strategy
			var fallbackStrategy backoff.Strategy
			switch strategyConfig.FallbackStrategy {
			case "exponential", "exp":
				fallbackStrategy = backoff.NewExponential(1*time.Second, 2.0, 60*time.Second)
			case "linear", "lin":
				fallbackStrategy = backoff.NewLinear(1*time.Second, 60*time.Second)
			case "fixed", "fix":
				fallbackStrategy = backoff.NewFixed(1 * time.Second)
			case "jitter", "jit":
				fallbackStrategy = backoff.NewJitter(1*time.Second, 2.0, 60*time.Second)
			case "decorrelated-jitter", "dj":
				fallbackStrategy = backoff.NewDecorrelatedJitter(1*time.Second, 2.0, 60*time.Second)
			case "fibonacci", "fib":
				fallbackStrategy = backoff.NewFibonacci(1*time.Second, 60*time.Second)
			case "polynomial", "poly":
				poly, err := backoff.NewPolynomial(1*time.Second, 2.0, 60*time.Second)
				if err != nil {
					return fmt.Errorf("failed to create polynomial fallback: %w", err)
				}
				fallbackStrategy = poly
			default:
				fallbackStrategy = backoff.NewExponential(1*time.Second, 2.0, 60*time.Second)
			}

			// Create adaptive strategy
			strategy, err := backoff.NewAdaptive(fallbackStrategy, strategyConfig.LearningRate, strategyConfig.MemoryWindow)
			if err != nil {
				return fmt.Errorf("failed to create adaptive strategy: %w", err)
			}

			return executeWithStrategy(strategy, commonConfig, args)
		},
	}

	// Set default values
	strategyConfig.LearningRate = 0.1
	strategyConfig.MemoryWindow = 50
	strategyConfig.FallbackStrategy = "exponential"

	// Add strategy-specific flags
	cmd.Flags().Float64VarP(&strategyConfig.LearningRate, "learning-rate", "r", strategyConfig.LearningRate,
		"Learning rate for adaptation (0.01-1.0)")
	cmd.Flags().IntVarP(&strategyConfig.MemoryWindow, "memory-window", "w", strategyConfig.MemoryWindow,
		"Number of recent outcomes to remember (5-10000)")
	cmd.Flags().StringVarP(&strategyConfig.FallbackStrategy, "fallback", "f", strategyConfig.FallbackStrategy,
		"Fallback strategy (exponential, linear, fixed, jitter, decorrelated-jitter, fibonacci, polynomial)")

	// Add common flags
	addCommonFlags(cmd, &commonConfig)

	return cmd
}
