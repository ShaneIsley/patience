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
	"github.com/user/retry/pkg/metrics"
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
	return nil
}

var (
	flagConfig  config.Config
	configFile  string
	debugConfig bool
)

var rootCmd = &cobra.Command{
	Use:   "retry [flags] -- command [args...]",
	Short: "Retry a command until it succeeds or max attempts reached",
	Long: `retry is a CLI tool that executes a command and retries it on failure.
It supports configurable retry attempts, delays between retries, and timeouts.

Configuration precedence (highest to lowest):
1. CLI flags
2. Environment variables (RETRY_*)
3. Configuration file
4. Default values

Configuration can be loaded from a TOML file. The tool looks for configuration files
in the following order:
1. File specified by --config flag
2. .retry.toml in current directory
3. retry.toml in current directory
4. .retry.toml in home directory

Environment variables:
- RETRY_ATTEMPTS: Maximum number of attempts
- RETRY_DELAY: Base delay between attempts (e.g., "1s", "500ms")
- RETRY_TIMEOUT: Timeout per attempt (e.g., "30s", "1m")
- RETRY_BACKOFF: Backoff strategy ("fixed" or "exponential")
- RETRY_MAX_DELAY: Maximum delay for exponential backoff
- RETRY_MULTIPLIER: Multiplier for exponential backoff
- RETRY_SUCCESS_PATTERN: Regex pattern for success detection
- RETRY_FAILURE_PATTERN: Regex pattern for failure detection
- RETRY_CASE_INSENSITIVE: Case-insensitive pattern matching ("true" or "false")`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRetry,
}

func init() {
	// Configuration file flag
	rootCmd.Flags().StringVar(&configFile, "config", "", "Configuration file path")

	// Debug flag
	rootCmd.Flags().BoolVar(&debugConfig, "debug-config", false, "Show configuration resolution debug information")

	// CLI flags (these will override config file and environment values)
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

// loadConfiguration loads configuration with full precedence support
func loadConfiguration(cmd *cobra.Command) (*config.Config, error) {
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

	// Create explicit flags map and flag config
	var effectiveFlagConfig *config.Config
	var explicitFields map[string]bool

	if hasAnyFlagsSet(cmd) {
		effectiveFlagConfig = &flagConfig
		explicitFields = make(map[string]bool)

		// Track which flags were explicitly set
		if cmd.Flags().Changed("attempts") {
			explicitFields["attempts"] = true
		}
		if cmd.Flags().Changed("delay") {
			explicitFields["delay"] = true
		}
		if cmd.Flags().Changed("timeout") {
			explicitFields["timeout"] = true
		}
		if cmd.Flags().Changed("backoff") {
			explicitFields["backoff"] = true
		}
		if cmd.Flags().Changed("max-delay") {
			explicitFields["max_delay"] = true
		}
		if cmd.Flags().Changed("multiplier") {
			explicitFields["multiplier"] = true
		}
		if cmd.Flags().Changed("success-pattern") {
			explicitFields["success_pattern"] = true
		}
		if cmd.Flags().Changed("failure-pattern") {
			explicitFields["failure_pattern"] = true
		}
		if cmd.Flags().Changed("case-insensitive") {
			explicitFields["case_insensitive"] = true
		}
	}

	// Load configuration with full precedence support
	finalConfig, debugInfo, err := config.LoadWithPrecedenceAndExplicitFlags(configPath, effectiveFlagConfig, explicitFields, debugConfig)
	if err != nil {
		return nil, err
	}

	// Print debug information if requested
	if debugConfig && debugInfo != nil {
		debugInfo.PrintDebugInfo()
		fmt.Println() // Add blank line after debug info
	}

	return finalConfig, nil
}

// hasAnyFlagsSet checks if any CLI flags were set
func hasAnyFlagsSet(cmd *cobra.Command) bool {
	flagNames := []string{
		"attempts", "delay", "timeout", "backoff", "max-delay",
		"multiplier", "success-pattern", "failure-pattern", "case-insensitive",
	}

	for _, flagName := range flagNames {
		if cmd.Flags().Changed(flagName) {
			return true
		}
	}
	return false
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
