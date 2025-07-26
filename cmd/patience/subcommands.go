package main

import (
	"fmt"
	"time"

	"github.com/shaneisley/patience/pkg/backoff"
	"github.com/spf13/cobra"
)

// Global variables to store parsed configurations for testing
var (
	lastHTTPAwareConfig   HTTPAwareConfig
	lastExponentialConfig ExponentialConfig
	lastParsedCommand     []string
)

// HTTPAwareConfig holds configuration for HTTP-aware strategy
type HTTPAwareConfig struct {
	Fallback string
	MaxDelay time.Duration
}

// ExponentialConfig holds configuration for exponential strategy
type ExponentialConfig struct {
	BaseDelay  time.Duration
	Multiplier float64
	MaxDelay   time.Duration
}

// createHTTPAwareCommand creates the http-aware subcommand
func createHTTPAwareCommand() *cobra.Command {
	var config HTTPAwareConfig

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

			// Store parsed config and command for testing
			lastHTTPAwareConfig = config
			lastParsedCommand = args

			// Create strategy and execute
			return executeWithHTTPAware(config, args)
		},
	} // Add flags
	cmd.Flags().StringVarP(&config.Fallback, "fallback", "f", "exponential", "Fallback strategy when no HTTP info available")
	cmd.Flags().DurationVarP(&config.MaxDelay, "max-delay", "m", 30*time.Minute, "Maximum delay cap")

	return cmd
}

// createExponentialCommand creates the exponential subcommand
func createExponentialCommand() *cobra.Command {
	var config ExponentialConfig

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

			// Store parsed config and command for testing
			lastExponentialConfig = config
			lastParsedCommand = args

			// Create strategy and execute
			return executeWithExponential(config, args)
		},
	} // Add flags
	cmd.Flags().DurationVarP(&config.BaseDelay, "base-delay", "b", 1*time.Second, "Base delay")
	cmd.Flags().Float64VarP(&config.Multiplier, "multiplier", "x", 2.0, "Multiplier")
	cmd.Flags().DurationVarP(&config.MaxDelay, "max-delay", "m", 60*time.Second, "Maximum delay")

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
func executeWithHTTPAware(config HTTPAwareConfig, commandArgs []string) error {
	// Create fallback strategy
	var fallbackStrategy backoff.Strategy
	switch config.Fallback {
	case "exponential", "exp":
		fallbackStrategy = backoff.NewExponential(1*time.Second, 2.0, 60*time.Second)
	case "linear":
		fallbackStrategy = backoff.NewLinear(1*time.Second, 60*time.Second)
	case "fixed":
		fallbackStrategy = backoff.NewFixed(1 * time.Second)
	default:
		fallbackStrategy = backoff.NewExponential(1*time.Second, 2.0, 60*time.Second)
	}

	// Create HTTP-aware strategy
	strategy := backoff.NewHTTPAware(fallbackStrategy, config.MaxDelay)

	// For now, just return nil (will be implemented in integration phase)
	_ = strategy
	_ = commandArgs
	return nil
}

// executeWithExponential executes command with exponential strategy
func executeWithExponential(config ExponentialConfig, commandArgs []string) error {
	// Create exponential strategy
	strategy := backoff.NewExponential(config.BaseDelay, config.Multiplier, config.MaxDelay)

	// For now, just return nil (will be implemented in integration phase)
	_ = strategy
	_ = commandArgs
	return nil
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

// createLinearCommand creates the linear subcommand
func createLinearCommand() *cobra.Command {
	var config LinearConfig

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

			strategy := backoff.NewLinear(config.Increment, config.MaxDelay)
			return executeWithStrategy(strategy, args)
		},
	}

	cmd.Flags().DurationVarP(&config.Increment, "increment", "i", 1*time.Second, "Delay increment")
	cmd.Flags().DurationVarP(&config.MaxDelay, "max-delay", "m", 60*time.Second, "Maximum delay")

	return cmd
}

// createFixedCommand creates the fixed subcommand
func createFixedCommand() *cobra.Command {
	var config FixedConfig

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

			strategy := backoff.NewFixed(config.Delay)
			return executeWithStrategy(strategy, args)
		},
	}

	cmd.Flags().DurationVarP(&config.Delay, "delay", "d", 1*time.Second, "Fixed delay")

	return cmd
}

// createJitterCommand creates the jitter subcommand
func createJitterCommand() *cobra.Command {
	var config JitterConfig

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

			strategy := backoff.NewJitter(config.BaseDelay, config.Multiplier, config.MaxDelay)
			return executeWithStrategy(strategy, args)
		},
	}

	cmd.Flags().DurationVarP(&config.BaseDelay, "base-delay", "b", 1*time.Second, "Base delay")
	cmd.Flags().Float64VarP(&config.Multiplier, "multiplier", "x", 2.0, "Multiplier")
	cmd.Flags().DurationVarP(&config.MaxDelay, "max-delay", "m", 60*time.Second, "Maximum delay")

	return cmd
}

// createDecorrelatedJitterCommand creates the decorrelated-jitter subcommand
func createDecorrelatedJitterCommand() *cobra.Command {
	var config DecorrelatedJitterConfig

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

			strategy := backoff.NewDecorrelatedJitter(config.BaseDelay, config.Multiplier, config.MaxDelay)
			return executeWithStrategy(strategy, args)
		},
	}

	cmd.Flags().DurationVarP(&config.BaseDelay, "base-delay", "b", 1*time.Second, "Base delay")
	cmd.Flags().Float64VarP(&config.Multiplier, "multiplier", "x", 2.0, "Multiplier")
	cmd.Flags().DurationVarP(&config.MaxDelay, "max-delay", "m", 60*time.Second, "Maximum delay")

	return cmd
}

// createFibonacciCommand creates the fibonacci subcommand
func createFibonacciCommand() *cobra.Command {
	var config FibonacciConfig

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

			strategy := backoff.NewFibonacci(config.BaseDelay, config.MaxDelay)
			return executeWithStrategy(strategy, args)
		},
	}

	cmd.Flags().DurationVarP(&config.BaseDelay, "base-delay", "b", 1*time.Second, "Base delay")
	cmd.Flags().DurationVarP(&config.MaxDelay, "max-delay", "m", 60*time.Second, "Maximum delay")

	return cmd
}

// executeWithStrategy executes command with the given strategy
func executeWithStrategy(strategy backoff.Strategy, commandArgs []string) error {
	// For now, just return nil (will be fully implemented with executor integration)
	_ = strategy
	_ = commandArgs
	return nil
}
