package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/user/retry/pkg/backoff"
	"github.com/user/retry/pkg/conditions"
	"github.com/user/retry/pkg/executor"
)

// Config holds the CLI configuration
type Config struct {
	Attempts        int
	Delay           time.Duration
	Timeout         time.Duration
	BackoffType     string
	MaxDelay        time.Duration
	Multiplier      float64
	SuccessPattern  string
	FailurePattern  string
	CaseInsensitive bool
}

// createExecutor creates an executor based on the configuration
func createExecutor(config Config) (*executor.Executor, error) {
	var strategy backoff.Strategy

	// Create backoff strategy if delay is specified
	if config.Delay > 0 {
		switch config.BackoffType {
		case "exponential":
			strategy = backoff.NewExponential(config.Delay, config.Multiplier, config.MaxDelay)
		default: // "fixed" or empty
			strategy = backoff.NewFixed(config.Delay)
		}
	}

	// Create condition checker if patterns are specified
	var checker *conditions.Checker
	if config.SuccessPattern != "" || config.FailurePattern != "" {
		var err error
		checker, err = conditions.NewChecker(config.SuccessPattern, config.FailurePattern, config.CaseInsensitive)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern: %w", err)
		}
	}

	// Create base executor
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

	// Add condition checker if configured
	exec.Conditions = checker

	return exec, nil
}

// executeCommand runs the command with the given executor and exits with appropriate code
func executeCommand(exec *executor.Executor, args []string) error {
	result, err := exec.Run(args)
	if err != nil {
		return fmt.Errorf("execution error: %w", err)
	}

	// Exit with appropriate code based on success
	if result.Success {
		os.Exit(0)
	} else {
		// If failure was due to pattern matching, use exit code 1
		// Otherwise use the original exit code
		if result.Reason == "failure pattern matched" {
			os.Exit(1)
		} else {
			os.Exit(result.ExitCode)
		}
	}

	return nil
}

var (
	config Config
)

var rootCmd = &cobra.Command{
	Use:   "retry [flags] -- command [args...]",
	Short: "Retry a command until it succeeds or max attempts reached",
	Long: `retry is a CLI tool that executes a command and retries it on failure.
It supports configurable retry attempts, delays between retries, and timeouts.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRetry,
}

func init() {
	rootCmd.Flags().IntVarP(&config.Attempts, "attempts", "a", 3, "Maximum number of attempts")
	rootCmd.Flags().DurationVarP(&config.Delay, "delay", "d", 0, "Base delay between attempts")
	rootCmd.Flags().DurationVarP(&config.Timeout, "timeout", "t", 0, "Timeout per attempt")
	rootCmd.Flags().StringVar(&config.BackoffType, "backoff", "fixed", "Backoff strategy: fixed or exponential")
	rootCmd.Flags().DurationVar(&config.MaxDelay, "max-delay", 0, "Maximum delay for exponential backoff (0 = no limit)")
	rootCmd.Flags().Float64Var(&config.Multiplier, "multiplier", 2.0, "Multiplier for exponential backoff")
	rootCmd.Flags().StringVar(&config.SuccessPattern, "success-pattern", "", "Regex pattern that indicates success in stdout/stderr")
	rootCmd.Flags().StringVar(&config.FailurePattern, "failure-pattern", "", "Regex pattern that indicates failure in stdout/stderr")
	rootCmd.Flags().BoolVar(&config.CaseInsensitive, "case-insensitive", false, "Make pattern matching case-insensitive")
}

func runRetry(cmd *cobra.Command, args []string) error {
	exec, err := createExecutor(config)
	if err != nil {
		return err
	}
	return executeCommand(exec, args)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
