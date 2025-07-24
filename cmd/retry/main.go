package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/user/retry/pkg/backoff"
	"github.com/user/retry/pkg/conditions"
	"github.com/user/retry/pkg/config"
	"github.com/user/retry/pkg/executor"
	"github.com/user/retry/pkg/ui"
)

// createExecutor creates an executor based on the configuration
func createExecutor(cfg *config.Config) (*executor.Executor, error) {
	var strategy backoff.Strategy

	// Create backoff strategy if delay is specified
	if cfg.Delay > 0 {
		switch cfg.BackoffType {
		case "exponential":
			strategy = backoff.NewExponential(cfg.Delay, cfg.Multiplier, cfg.MaxDelay)
		default: // "fixed" or empty
			strategy = backoff.NewFixed(cfg.Delay)
		}
	}

	// Create condition checker if patterns are specified
	var checker *conditions.Checker
	if cfg.SuccessPattern != "" || cfg.FailurePattern != "" {
		var err error
		checker, err = conditions.NewChecker(cfg.SuccessPattern, cfg.FailurePattern, cfg.CaseInsensitive)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern: %w", err)
		}
	}

	// Create base executor
	var exec *executor.Executor
	if strategy != nil && cfg.Timeout > 0 {
		exec = executor.NewExecutorWithBackoffAndTimeout(cfg.Attempts, strategy, cfg.Timeout)
	} else if strategy != nil {
		exec = executor.NewExecutorWithBackoff(cfg.Attempts, strategy)
	} else if cfg.Timeout > 0 {
		exec = executor.NewExecutorWithTimeout(cfg.Attempts, cfg.Timeout)
	} else {
		exec = executor.NewExecutor(cfg.Attempts)
	}

	// Add condition checker if configured
	exec.Conditions = checker

	// Add status reporter
	reporter := ui.NewReporter(os.Stderr)
	exec.Reporter = reporter

	return exec, nil
}

// executeCommand runs the command with the given executor and exits with appropriate code
func executeCommand(exec *executor.Executor, args []string) error {
	result, err := exec.Run(args)
	if err != nil {
		return fmt.Errorf("execution error: %w", err)
	}

	// Show final summary if we have statistics
	if result.Stats != nil && exec.Reporter != nil {
		exec.Reporter.FinalSummary(result.Stats)
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
	return nil
}

var (
	flagConfig config.Config
	configFile string
)

var rootCmd = &cobra.Command{
	Use:   "retry [flags] -- command [args...]",
	Short: "Retry a command until it succeeds or max attempts reached",
	Long: `retry is a CLI tool that executes a command and retries it on failure.
It supports configurable retry attempts, delays between retries, and timeouts.

Configuration can be loaded from a TOML file. The tool looks for configuration files
in the following order:
1. File specified by --config flag
2. .retry.toml in current directory
3. retry.toml in current directory
4. .retry.toml in home directory

CLI flags override configuration file values.`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRetry,
}

func init() {
	// Configuration file flag
	rootCmd.Flags().StringVar(&configFile, "config", "", "Configuration file path")

	// CLI flags (these will override config file values)
	rootCmd.Flags().IntVarP(&flagConfig.Attempts, "attempts", "a", 0, "Maximum number of attempts")
	rootCmd.Flags().DurationVarP(&flagConfig.Delay, "delay", "d", 0, "Base delay between attempts")
	rootCmd.Flags().DurationVarP(&flagConfig.Timeout, "timeout", "t", 0, "Timeout per attempt")
	rootCmd.Flags().StringVar(&flagConfig.BackoffType, "backoff", "", "Backoff strategy: fixed or exponential")
	rootCmd.Flags().DurationVar(&flagConfig.MaxDelay, "max-delay", 0, "Maximum delay for exponential backoff (0 = no limit)")
	rootCmd.Flags().Float64Var(&flagConfig.Multiplier, "multiplier", 0, "Multiplier for exponential backoff")
	rootCmd.Flags().StringVar(&flagConfig.SuccessPattern, "success-pattern", "", "Regex pattern that indicates success in stdout/stderr")
	rootCmd.Flags().StringVar(&flagConfig.FailurePattern, "failure-pattern", "", "Regex pattern that indicates failure in stdout/stderr")
	rootCmd.Flags().BoolVar(&flagConfig.CaseInsensitive, "case-insensitive", false, "Make pattern matching case-insensitive")
}

// loadConfiguration loads configuration from file and merges with CLI flags
func loadConfiguration(cmd *cobra.Command) (*config.Config, error) {
	var baseConfig *config.Config

	// Determine config file to use
	var configPath string
	if configFile != "" {
		// Use explicitly specified config file
		configPath = configFile
	} else {
		// Search for config file in standard locations
		cwd, _ := os.Getwd()
		if found := config.FindConfigFile(cwd); found != "" {
			configPath = found
		} else {
			// Check home directory
			if homeDir, err := os.UserHomeDir(); err == nil {
				if found := config.FindConfigFile(homeDir); found != "" {
					configPath = found
				}
			}
		}
	}

	// Load configuration from file or use defaults
	if configPath != "" {
		var err error
		baseConfig, err = config.LoadFromFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load config file %s: %w", configPath, err)
		}
	} else {
		baseConfig = config.LoadWithDefaults()
	}

	// Merge with CLI flags (flags override config file)
	// Handle boolean flags specially since false is a valid override
	finalConfig := baseConfig.MergeWithFlags(&flagConfig)

	// Special handling for boolean flags that were explicitly set
	if cmd.Flags().Changed("case-insensitive") {
		finalConfig.CaseInsensitive = flagConfig.CaseInsensitive
	}

	return finalConfig, nil
}
func runRetry(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := loadConfiguration(cmd)
	if err != nil {
		return err
	}

	// Create executor
	exec, err := createExecutor(cfg)
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
