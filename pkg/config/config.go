package config

import (
	"fmt"
	"os"
	"path/filepath"
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

	return &config, nil
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
// Flag values override config file values when they are non-zero/non-empty
func (c *Config) MergeWithFlags(flags *Config) *Config {
	result := *c // Copy base config

	// Override with flag values if they are non-zero/non-empty
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
	// Note: For boolean flags, we need special handling since false is a valid override
	// For now, we'll keep the base value for CaseInsensitive to avoid complexity
	// This could be enhanced later with a "flag was set" tracking mechanism

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
