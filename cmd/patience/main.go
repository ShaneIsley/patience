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
  patience exponential --base-delay 1s --multiplier 2.0 -- curl https://api.stripe.com

  # Linear backoff for database connections
  patience linear --increment 5s --max-delay 60s -- psql -h db.example.com

  # Using abbreviations for brevity
  patience ha -f exp -- curl -i https://api.github.com
  patience exp -b 1s -x 2.0 -- curl https://api.stripe.com`,
}

func init() {
	// Add strategy subcommands
	rootCmd.AddCommand(createHTTPAwareCommand())
	rootCmd.AddCommand(createExponentialCommand())
	rootCmd.AddCommand(createLinearCommand())
	rootCmd.AddCommand(createFixedCommand())
	rootCmd.AddCommand(createJitterCommand())
	rootCmd.AddCommand(createDecorrelatedJitterCommand())
	rootCmd.AddCommand(createFibonacciCommand())
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
