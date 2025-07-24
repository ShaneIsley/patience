package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/user/retry/pkg/backoff"
	"github.com/user/retry/pkg/executor"
)

// Config holds the CLI configuration
type Config struct {
	Attempts int
	Delay    time.Duration
	Timeout  time.Duration
}

// createExecutor creates an executor based on the configuration
func createExecutor(config Config) *executor.Executor {
	if config.Delay > 0 && config.Timeout > 0 {
		strategy := backoff.NewFixed(config.Delay)
		return executor.NewExecutorWithBackoffAndTimeout(config.Attempts, strategy, config.Timeout)
	} else if config.Delay > 0 {
		strategy := backoff.NewFixed(config.Delay)
		return executor.NewExecutorWithBackoff(config.Attempts, strategy)
	} else if config.Timeout > 0 {
		return executor.NewExecutorWithTimeout(config.Attempts, config.Timeout)
	} else {
		return executor.NewExecutor(config.Attempts)
	}
}

// executeCommand runs the command with the given executor and exits with appropriate code
func executeCommand(exec *executor.Executor, args []string) error {
	result, err := exec.Run(args)
	if err != nil {
		return fmt.Errorf("execution error: %w", err)
	}

	// Exit with the same code as the target command
	if result.Success {
		os.Exit(0)
	} else {
		os.Exit(result.ExitCode)
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
	rootCmd.Flags().DurationVarP(&config.Delay, "delay", "d", 0, "Fixed delay between attempts")
	rootCmd.Flags().DurationVarP(&config.Timeout, "timeout", "t", 0, "Timeout per attempt")
}

func runRetry(cmd *cobra.Command, args []string) error {
	exec := createExecutor(config)
	return executeCommand(exec, args)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
