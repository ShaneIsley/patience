package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds the configuration for the retry CLI
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
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("invalid %s value '%v': %s", e.Field, e.Value, e.Message)
}

// ConfigSource represents where a configuration value came from
type ConfigSource int

const (
	SourceDefault ConfigSource = iota
	SourceConfigFile
	SourceEnvironment
	SourceCLIFlag
)

func (s ConfigSource) String() string {
	switch s {
	case SourceDefault:
		return "default"
	case SourceConfigFile:
		return "config file"
	case SourceEnvironment:
		return "environment variable"
	case SourceCLIFlag:
		return "CLI flag"
	default:
		return "unknown"
	}
}

// ConfigDebugInfo holds debugging information about configuration resolution
type ConfigDebugInfo struct {
	Sources map[string]ConfigSource
	Values  map[string]interface{}
}

// LoadFromFile loads configuration from a TOML file
func LoadFromFile(configFile string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Set config file
	v.SetConfigFile(configFile)
	v.SetConfigType("toml")

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Unmarshal into config struct
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &config, nil
}

// LoadWithEnvironment loads configuration with environment variable support
func LoadWithEnvironment() (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Configure environment variable support
	v.SetEnvPrefix("RETRY")
	v.AutomaticEnv()

	// Map environment variables to config keys
	envMappings := map[string]string{
		"RETRY_ATTEMPTS":         "attempts",
		"RETRY_DELAY":            "delay",
		"RETRY_TIMEOUT":          "timeout",
		"RETRY_BACKOFF":          "backoff",
		"RETRY_MAX_DELAY":        "max_delay",
		"RETRY_MULTIPLIER":       "multiplier",
		"RETRY_SUCCESS_PATTERN":  "success_pattern",
		"RETRY_FAILURE_PATTERN":  "failure_pattern",
		"RETRY_CASE_INSENSITIVE": "case_insensitive",
	}

	for envVar, configKey := range envMappings {
		v.BindEnv(configKey, envVar)
	}

	// Unmarshal into config struct
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &config, nil
}

// LoadWithPrecedence loads configuration with full precedence support
func LoadWithPrecedence(configFile string, flagConfig *Config, debug bool) (*Config, *ConfigDebugInfo, error) {
	var debugInfo *ConfigDebugInfo
	if debug {
		debugInfo = &ConfigDebugInfo{
			Sources: make(map[string]ConfigSource),
			Values:  make(map[string]interface{}),
		}
	}

	v := viper.New()

	// Set defaults
	setDefaults(v)
	if debug {
		recordDefaults(debugInfo)
	}

	// Load config file if specified
	if configFile != "" {
		v.SetConfigFile(configFile)
		v.SetConfigType("toml")
		if err := v.ReadInConfig(); err != nil {
			return nil, debugInfo, fmt.Errorf("failed to read config file: %w", err)
		}
		if debug {
			recordConfigFile(debugInfo, v)
		}
	}

	// Configure environment variable support
	v.SetEnvPrefix("RETRY")
	v.AutomaticEnv()

	// Map environment variables to config keys
	envMappings := map[string]string{
		"RETRY_ATTEMPTS":         "attempts",
		"RETRY_DELAY":            "delay",
		"RETRY_TIMEOUT":          "timeout",
		"RETRY_BACKOFF":          "backoff",
		"RETRY_MAX_DELAY":        "max_delay",
		"RETRY_MULTIPLIER":       "multiplier",
		"RETRY_SUCCESS_PATTERN":  "success_pattern",
		"RETRY_FAILURE_PATTERN":  "failure_pattern",
		"RETRY_CASE_INSENSITIVE": "case_insensitive",
	}

	for envVar, configKey := range envMappings {
		v.BindEnv(configKey, envVar)
	}

	if debug {
		recordEnvironment(debugInfo, envMappings)
	}

	// Unmarshal into config struct
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, debugInfo, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Apply CLI flag overrides
	if flagConfig != nil {
		config = *config.MergeWithFlags(flagConfig)
		if debug {
			recordFlags(debugInfo, flagConfig)
		}
	}

	// Validate final configuration
	if err := config.Validate(); err != nil {
		return nil, debugInfo, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &config, debugInfo, nil
}

// LoadWithPrecedenceAndExplicitFlags loads configuration with full precedence support and explicit flag handling
func LoadWithPrecedenceAndExplicitFlags(configFile string, flagConfig *Config, explicitFields map[string]bool, debug bool) (*Config, *ConfigDebugInfo, error) {
	var debugInfo *ConfigDebugInfo
	if debug {
		debugInfo = &ConfigDebugInfo{
			Sources: make(map[string]ConfigSource),
			Values:  make(map[string]interface{}),
		}
	}

	v := viper.New()

	// Set defaults
	setDefaults(v)
	if debug {
		recordDefaults(debugInfo)
	}

	// Load config file if specified
	if configFile != "" {
		v.SetConfigFile(configFile)
		v.SetConfigType("toml")
		if err := v.ReadInConfig(); err != nil {
			return nil, debugInfo, fmt.Errorf("failed to read config file: %w", err)
		}
		if debug {
			recordConfigFile(debugInfo, v)
		}
	}

	// Configure environment variable support
	v.SetEnvPrefix("RETRY")
	v.AutomaticEnv()

	// Map environment variables to config keys
	envMappings := map[string]string{
		"RETRY_ATTEMPTS":         "attempts",
		"RETRY_DELAY":            "delay",
		"RETRY_TIMEOUT":          "timeout",
		"RETRY_BACKOFF":          "backoff",
		"RETRY_MAX_DELAY":        "max_delay",
		"RETRY_MULTIPLIER":       "multiplier",
		"RETRY_SUCCESS_PATTERN":  "success_pattern",
		"RETRY_FAILURE_PATTERN":  "failure_pattern",
		"RETRY_CASE_INSENSITIVE": "case_insensitive",
	}

	for envVar, configKey := range envMappings {
		v.BindEnv(configKey, envVar)
	}

	if debug {
		recordEnvironment(debugInfo, envMappings)
	}

	// Unmarshal into config struct
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, debugInfo, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Apply CLI flag overrides with explicit field tracking
	if flagConfig != nil && explicitFields != nil {
		config = *config.MergeWithExplicitFlags(flagConfig, explicitFields)
		if debug {
			recordExplicitFlags(debugInfo, flagConfig, explicitFields)
		}
	}

	// Validate final configuration
	if err := config.Validate(); err != nil {
		return nil, debugInfo, fmt.Errorf("configuration validation failed: %w", err)
	}

	return &config, debugInfo, nil
}

// LoadWithDefaults returns a configuration with default values
func LoadWithDefaults() *Config {
	v := viper.New()
	setDefaults(v)

	var config Config
	v.Unmarshal(&config)
	return &config
}

// setDefaults sets the default configuration values
func setDefaults(v *viper.Viper) {
	v.SetDefault("attempts", 3)
	v.SetDefault("delay", time.Duration(0))
	v.SetDefault("timeout", time.Duration(0))
	v.SetDefault("backoff", "fixed")
	v.SetDefault("max_delay", time.Duration(0))
	v.SetDefault("multiplier", 2.0)
	v.SetDefault("success_pattern", "")
	v.SetDefault("failure_pattern", "")
	v.SetDefault("case_insensitive", false)
}

// MergeWithFlags merges the base configuration with flag overrides
// This version assumes flags only contains values that were explicitly set
func (c *Config) MergeWithFlags(flags *Config) *Config {
	result := *c // Copy base config

	// Override with flag values (assumes only explicitly set values are non-zero)
	// The caller should ensure that only explicitly set flags are passed
	if flags.Attempts != 0 {
		result.Attempts = flags.Attempts
	}
	if flags.Delay != 0 {
		result.Delay = flags.Delay
	}
	if flags.Timeout != 0 {
		result.Timeout = flags.Timeout
	}
	if flags.BackoffType != "" {
		result.BackoffType = flags.BackoffType
	}
	if flags.MaxDelay != 0 {
		result.MaxDelay = flags.MaxDelay
	}
	if flags.Multiplier != 0 {
		result.Multiplier = flags.Multiplier
	}
	if flags.SuccessPattern != "" {
		result.SuccessPattern = flags.SuccessPattern
	}
	if flags.FailurePattern != "" {
		result.FailurePattern = flags.FailurePattern
	}
	// Boolean values are handled by the caller

	return &result
}

// MergeWithExplicitFlags merges configuration with explicitly set flag values
// This version handles zero values correctly by using explicit field setting
func (c *Config) MergeWithExplicitFlags(flags *Config, explicitFields map[string]bool) *Config {
	result := *c // Copy base config

	// Override with flag values only if they were explicitly set
	if explicitFields["attempts"] {
		result.Attempts = flags.Attempts
	}
	if explicitFields["delay"] {
		result.Delay = flags.Delay
	}
	if explicitFields["timeout"] {
		result.Timeout = flags.Timeout
	}
	if explicitFields["backoff"] {
		result.BackoffType = flags.BackoffType
	}
	if explicitFields["max_delay"] {
		result.MaxDelay = flags.MaxDelay
	}
	if explicitFields["multiplier"] {
		result.Multiplier = flags.Multiplier
	}
	if explicitFields["success_pattern"] {
		result.SuccessPattern = flags.SuccessPattern
	}
	if explicitFields["failure_pattern"] {
		result.FailurePattern = flags.FailurePattern
	}
	if explicitFields["case_insensitive"] {
		result.CaseInsensitive = flags.CaseInsensitive
	}

	return &result
}

// FindConfigFile searches for a configuration file in the given directory
// It looks for .retry.toml, retry.toml, .retry.yaml, retry.yaml files
func FindConfigFile(dir string) string {
	configNames := []string{".retry.toml", "retry.toml", ".retry.yaml", "retry.yaml"}

	for _, name := range configNames {
		configPath := filepath.Join(dir, name)
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}

	return ""
}

// Validate validates the configuration and returns detailed error messages
func (c *Config) Validate() error {
	var errors []ValidationError

	// Validate attempts
	if c.Attempts <= 0 {
		errors = append(errors, ValidationError{
			Field:   "attempts",
			Value:   c.Attempts,
			Message: "must be greater than 0",
		})
	}
	if c.Attempts > 1000 {
		errors = append(errors, ValidationError{
			Field:   "attempts",
			Value:   c.Attempts,
			Message: "must be 1000 or less to prevent excessive resource usage",
		})
	}

	// Validate delay
	if c.Delay < 0 {
		errors = append(errors, ValidationError{
			Field:   "delay",
			Value:   c.Delay,
			Message: "must be non-negative",
		})
	}
	if c.Delay > 24*time.Hour {
		errors = append(errors, ValidationError{
			Field:   "delay",
			Value:   c.Delay,
			Message: "must be 24 hours or less",
		})
	}

	// Validate timeout
	if c.Timeout < 0 {
		errors = append(errors, ValidationError{
			Field:   "timeout",
			Value:   c.Timeout,
			Message: "must be non-negative (0 means no timeout)",
		})
	}
	if c.Timeout > 24*time.Hour {
		errors = append(errors, ValidationError{
			Field:   "timeout",
			Value:   c.Timeout,
			Message: "must be 24 hours or less",
		})
	}

	// Validate backoff type
	if c.BackoffType != "" && c.BackoffType != "fixed" && c.BackoffType != "exponential" {
		errors = append(errors, ValidationError{
			Field:   "backoff",
			Value:   c.BackoffType,
			Message: "must be 'fixed' or 'exponential'",
		})
	}

	// Validate max delay
	if c.MaxDelay < 0 {
		errors = append(errors, ValidationError{
			Field:   "max_delay",
			Value:   c.MaxDelay,
			Message: "must be non-negative (0 means no limit)",
		})
	}
	if c.MaxDelay > 24*time.Hour {
		errors = append(errors, ValidationError{
			Field:   "max_delay",
			Value:   c.MaxDelay,
			Message: "must be 24 hours or less",
		})
	}
	if c.MaxDelay > 0 && c.Delay > 0 && c.MaxDelay < c.Delay {
		errors = append(errors, ValidationError{
			Field:   "max_delay",
			Value:   c.MaxDelay,
			Message: "must be greater than or equal to base delay",
		})
	}

	// Validate multiplier
	if c.Multiplier < 1.0 {
		errors = append(errors, ValidationError{
			Field:   "multiplier",
			Value:   c.Multiplier,
			Message: "must be 1.0 or greater",
		})
	}
	if c.Multiplier > 10.0 {
		errors = append(errors, ValidationError{
			Field:   "multiplier",
			Value:   c.Multiplier,
			Message: "must be 10.0 or less to prevent excessive delays",
		})
	}

	// Return combined error if any validation failed
	if len(errors) > 0 {
		var messages []string
		for _, err := range errors {
			messages = append(messages, err.Error())
		}
		return fmt.Errorf("validation errors:\n  - %s", strings.Join(messages, "\n  - "))
	}

	return nil
}

// recordDefaults records default values in debug info
func recordDefaults(debug *ConfigDebugInfo) {
	debug.Sources["attempts"] = SourceDefault
	debug.Values["attempts"] = 3
	debug.Sources["delay"] = SourceDefault
	debug.Values["delay"] = time.Duration(0)
	debug.Sources["timeout"] = SourceDefault
	debug.Values["timeout"] = time.Duration(0)
	debug.Sources["backoff"] = SourceDefault
	debug.Values["backoff"] = "fixed"
	debug.Sources["max_delay"] = SourceDefault
	debug.Values["max_delay"] = time.Duration(0)
	debug.Sources["multiplier"] = SourceDefault
	debug.Values["multiplier"] = 2.0
	debug.Sources["success_pattern"] = SourceDefault
	debug.Values["success_pattern"] = ""
	debug.Sources["failure_pattern"] = SourceDefault
	debug.Values["failure_pattern"] = ""
	debug.Sources["case_insensitive"] = SourceDefault
	debug.Values["case_insensitive"] = false
}

// recordConfigFile records config file values in debug info
func recordConfigFile(debug *ConfigDebugInfo, v *viper.Viper) {
	configKeys := []string{
		"attempts", "delay", "timeout", "backoff", "max_delay",
		"multiplier", "success_pattern", "failure_pattern", "case_insensitive",
	}

	for _, key := range configKeys {
		if v.IsSet(key) {
			debug.Sources[key] = SourceConfigFile
			debug.Values[key] = v.Get(key)
		}
	}
}

// recordEnvironment records environment variable values in debug info
func recordEnvironment(debug *ConfigDebugInfo, envMappings map[string]string) {
	for envVar, configKey := range envMappings {
		if value := os.Getenv(envVar); value != "" {
			debug.Sources[configKey] = SourceEnvironment
			debug.Values[configKey] = value
		}
	}
}

// recordFlags records CLI flag values in debug info
func recordFlags(debug *ConfigDebugInfo, flags *Config) {
	if flags.Attempts != 0 {
		debug.Sources["attempts"] = SourceCLIFlag
		debug.Values["attempts"] = flags.Attempts
	}
	if flags.Delay != 0 {
		debug.Sources["delay"] = SourceCLIFlag
		debug.Values["delay"] = flags.Delay
	}
	if flags.Timeout != 0 {
		debug.Sources["timeout"] = SourceCLIFlag
		debug.Values["timeout"] = flags.Timeout
	}
	if flags.BackoffType != "" {
		debug.Sources["backoff"] = SourceCLIFlag
		debug.Values["backoff"] = flags.BackoffType
	}
	if flags.MaxDelay != 0 {
		debug.Sources["max_delay"] = SourceCLIFlag
		debug.Values["max_delay"] = flags.MaxDelay
	}
	if flags.Multiplier != 0 {
		debug.Sources["multiplier"] = SourceCLIFlag
		debug.Values["multiplier"] = flags.Multiplier
	}
	if flags.SuccessPattern != "" {
		debug.Sources["success_pattern"] = SourceCLIFlag
		debug.Values["success_pattern"] = flags.SuccessPattern
	}
	if flags.FailurePattern != "" {
		debug.Sources["failure_pattern"] = SourceCLIFlag
		debug.Values["failure_pattern"] = flags.FailurePattern
	}
}

// recordExplicitFlags records CLI flag values that were explicitly set in debug info
func recordExplicitFlags(debug *ConfigDebugInfo, flags *Config, explicitFields map[string]bool) {
	if explicitFields["attempts"] {
		debug.Sources["attempts"] = SourceCLIFlag
		debug.Values["attempts"] = flags.Attempts
	}
	if explicitFields["delay"] {
		debug.Sources["delay"] = SourceCLIFlag
		debug.Values["delay"] = flags.Delay
	}
	if explicitFields["timeout"] {
		debug.Sources["timeout"] = SourceCLIFlag
		debug.Values["timeout"] = flags.Timeout
	}
	if explicitFields["backoff"] {
		debug.Sources["backoff"] = SourceCLIFlag
		debug.Values["backoff"] = flags.BackoffType
	}
	if explicitFields["max_delay"] {
		debug.Sources["max_delay"] = SourceCLIFlag
		debug.Values["max_delay"] = flags.MaxDelay
	}
	if explicitFields["multiplier"] {
		debug.Sources["multiplier"] = SourceCLIFlag
		debug.Values["multiplier"] = flags.Multiplier
	}
	if explicitFields["success_pattern"] {
		debug.Sources["success_pattern"] = SourceCLIFlag
		debug.Values["success_pattern"] = flags.SuccessPattern
	}
	if explicitFields["failure_pattern"] {
		debug.Sources["failure_pattern"] = SourceCLIFlag
		debug.Values["failure_pattern"] = flags.FailurePattern
	}
	if explicitFields["case_insensitive"] {
		debug.Sources["case_insensitive"] = SourceCLIFlag
		debug.Values["case_insensitive"] = flags.CaseInsensitive
	}
}

// PrintDebugInfo prints configuration debug information
func (debug *ConfigDebugInfo) PrintDebugInfo() {
	fmt.Println("Configuration Resolution Debug Info:")
	fmt.Println("===================================")

	configKeys := []string{
		"attempts", "delay", "timeout", "backoff", "max_delay",
		"multiplier", "success_pattern", "failure_pattern", "case_insensitive",
	}

	for _, key := range configKeys {
		source := debug.Sources[key]
		value := debug.Values[key]
		fmt.Printf("%-20s: %-15v (from %s)\n", key, value, source)
	}
}
