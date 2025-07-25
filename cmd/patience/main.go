package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/shaneisley/patience/pkg/backoff"
	"github.com/shaneisley/patience/pkg/conditions"
	"github.com/shaneisley/patience/pkg/config"
	"github.com/shaneisley/patience/pkg/executor"
	"github.com/shaneisley/patience/pkg/metrics"
	"github.com/shaneisley/patience/pkg/ui"
	"github.com/spf13/cobra"
)

// createExecutor creates an executor based on the configuration
func createExecutor(cfg *config.Config) (*executor.Executor, error) {
	var strategy backoff.Strategy

	// Create backoff strategy if delay is specified
	if cfg.Delay > 0 {
		switch cfg.BackoffType {
		case "exponential":
			strategy = backoff.NewExponential(cfg.Delay, cfg.Multiplier, cfg.MaxDelay)
		case "jitter":
			strategy = backoff.NewJitter(cfg.Delay, cfg.Multiplier, cfg.MaxDelay)
		case "linear":
			strategy = backoff.NewLinear(cfg.Delay, cfg.MaxDelay)
		case "decorrelated-jitter":
			strategy = backoff.NewDecorrelatedJitter(cfg.Delay, cfg.Multiplier, cfg.MaxDelay)
		case "fibonacci":
			strategy = backoff.NewFibonacci(cfg.Delay, cfg.MaxDelay)
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
	Use:   "patience [flags] -- command [args...]",
	Short: "Patiently patience a command until it succeeds or max attempts reached",
	Long: `patience is a CLI tool that executes a command and retries it on failure with grace.
It supports configurable patience attempts, delays between retries, and timeouts.

Configuration precedence (highest to lowest):
1. CLI flags
2. Environment variables (PATIENCE_*)
3. Configuration file
4. Default values

Configuration can be loaded from a TOML file. The tool looks for configuration files
in the following order:
1. File specified by --config flag
2. .patience.toml in current directory
3. patience.toml in current directory
4. .patience.toml in home directory

Environment variables:
- PATIENCE_ATTEMPTS: Maximum number of attempts
- PATIENCE_DELAY: Base delay between attempts (e.g., "1s", "500ms")
- PATIENCE_TIMEOUT: Timeout per attempt (e.g., "30s", "1m")
- PATIENCE_BACKOFF: Backoff strategy ("fixed", "exponential", "jitter", "linear", "decorrelated-jitter", or "fibonacci")
- PATIENCE_MAX_DELAY: Maximum delay for exponential backoff
- PATIENCE_MULTIPLIER: Multiplier for exponential backoff
- PATIENCE_SUCCESS_PATTERN: Regex pattern for success detection
- PATIENCE_FAILURE_PATTERN: Regex pattern for failure detection
- PATIENCE_CASE_INSENSITIVE: Case-insensitive pattern matching ("true" or "false")

EXAMPLES:
  # Basic retry with exponential backoff
  patience --attempts 5 --delay 1s --backoff exponential -- curl -f https://api.example.com

  # Pattern-based success detection
  patience --success-pattern "deployment successful" -- ./deploy.sh

  # Distributed system with jitter to prevent thundering herd
  patience --backoff jitter --delay 1s --max-delay 10s -- api-call

  # Environment variable usage
  PATIENCE_ATTEMPTS=5 PATIENCE_BACKOFF=fibonacci patience -- flaky-command

  # Configuration file with flag override
  patience --config myproject.toml --attempts 10 -- integration-test`,
	Args: cobra.MinimumNArgs(1),
	RunE: runRetry,
}

func init() {
	// Configuration file flag
	rootCmd.Flags().StringVar(&configFile, "config", "", "Configuration file path")

	// Debug flag
	rootCmd.Flags().BoolVar(&debugConfig, "debug-config", false, "Show configuration resolution debug information")

	// CLI flags (these will override config file and environment values)
	rootCmd.Flags().IntVarP(&flagConfig.Attempts, "attempts", "a", 0, "Maximum retry attempts (default: 3, range: 1-1000)")
	rootCmd.Flags().DurationVarP(&flagConfig.Delay, "delay", "d", 0, "Base delay between attempts (default: 0 = no delay, examples: 1s, 500ms)")
	rootCmd.Flags().DurationVarP(&flagConfig.Timeout, "timeout", "t", 0, "Timeout per attempt (default: 0 = no timeout, examples: 30s, 1m)")
	rootCmd.Flags().StringVar(&flagConfig.BackoffType, "backoff", "", "Backoff strategy (default: fixed)\n                                 Options: fixed, exponential, jitter, linear, decorrelated-jitter, fibonacci")
	rootCmd.Flags().DurationVar(&flagConfig.MaxDelay, "max-delay", 0, "Maximum delay cap (default: 0 = no limit, valid with: exponential, jitter, linear, decorrelated-jitter, fibonacci)")
	rootCmd.Flags().Float64Var(&flagConfig.Multiplier, "multiplier", 0, "Backoff multiplier (default: 2.0, valid with: exponential, jitter, decorrelated-jitter)")
	rootCmd.Flags().StringVar(&flagConfig.SuccessPattern, "success-pattern", "", "Regex pattern for success detection in command output")
	rootCmd.Flags().StringVar(&flagConfig.FailurePattern, "failure-pattern", "", "Regex pattern for failure detection in command output")
	rootCmd.Flags().BoolVar(&flagConfig.CaseInsensitive, "case-insensitive", false, "Case-insensitive pattern matching")
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
