package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_LoadFromFile(t *testing.T) {
	// Given a TOML configuration file
	configContent := `
attempts = 5
delay = "2s"
timeout = "30s"
backoff = "exponential"
max_delay = "10s"
multiplier = 2.5
success_pattern = "deployment successful"
failure_pattern = "(?i)error|failed"
case_insensitive = true
`

	// Create temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "retry.toml")
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// When loading configuration from file
	config, err := LoadFromFile(configFile)

	// Then it should load all values correctly
	require.NoError(t, err)
	assert.Equal(t, 5, config.Attempts)
	assert.Equal(t, 2*time.Second, config.Delay)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, "exponential", config.BackoffType)
	assert.Equal(t, 10*time.Second, config.MaxDelay)
	assert.Equal(t, 2.5, config.Multiplier)
	assert.Equal(t, "deployment successful", config.SuccessPattern)
	assert.Equal(t, "(?i)error|failed", config.FailurePattern)
	assert.True(t, config.CaseInsensitive)
}

func TestConfig_LoadFromFileWithDefaults(t *testing.T) {
	// Given a minimal TOML configuration file
	configContent := `
attempts = 10
delay = "1s"
`

	// Create temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "retry.toml")
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// When loading configuration from file
	config, err := LoadFromFile(configFile)

	// Then it should load specified values and use defaults for others
	require.NoError(t, err)
	assert.Equal(t, 10, config.Attempts)
	assert.Equal(t, 1*time.Second, config.Delay)
	assert.Equal(t, time.Duration(0), config.Timeout)  // Default
	assert.Equal(t, "fixed", config.BackoffType)       // Default
	assert.Equal(t, time.Duration(0), config.MaxDelay) // Default
	assert.Equal(t, 2.0, config.Multiplier)            // Default
	assert.Equal(t, "", config.SuccessPattern)         // Default
	assert.Equal(t, "", config.FailurePattern)         // Default
	assert.False(t, config.CaseInsensitive)            // Default
}

func TestConfig_LoadFromNonExistentFile(t *testing.T) {
	// When loading configuration from non-existent file
	config, err := LoadFromFile("/non/existent/file.toml")

	// Then it should return an error
	require.Error(t, err)
	assert.Nil(t, config)
}

func TestConfig_LoadFromInvalidTOML(t *testing.T) {
	// Given an invalid TOML file
	configContent := `
attempts = 5
delay = "invalid duration"
[invalid toml
`

	// Create temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "retry.toml")
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// When loading configuration from invalid file
	config, err := LoadFromFile(configFile)

	// Then it should return an error
	require.Error(t, err)
	assert.Nil(t, config)
}

func TestConfig_LoadWithDefaults(t *testing.T) {
	// When loading configuration with defaults only
	config := LoadWithDefaults()

	// Then it should return default values
	require.NotNil(t, config)
	assert.Equal(t, 3, config.Attempts)
	assert.Equal(t, time.Duration(0), config.Delay)
	assert.Equal(t, time.Duration(0), config.Timeout)
	assert.Equal(t, "fixed", config.BackoffType)
	assert.Equal(t, time.Duration(0), config.MaxDelay)
	assert.Equal(t, 2.0, config.Multiplier)
	assert.Equal(t, "", config.SuccessPattern)
	assert.Equal(t, "", config.FailurePattern)
	assert.False(t, config.CaseInsensitive)
}

func TestConfig_MergeWithFlags(t *testing.T) {
	// Given a base configuration
	baseConfig := &Config{
		Attempts:        5,
		Delay:           2 * time.Second,
		Timeout:         30 * time.Second,
		BackoffType:     "exponential",
		MaxDelay:        10 * time.Second,
		Multiplier:      2.5,
		SuccessPattern:  "success",
		FailurePattern:  "error",
		CaseInsensitive: true,
	}

	// And flag overrides (some values set, others zero/empty)
	flagConfig := &Config{
		Attempts:        10,               // Override
		Delay:           0,                // Don't override (zero value)
		Timeout:         60 * time.Second, // Override
		BackoffType:     "",               // Don't override (empty)
		MaxDelay:        0,                // Don't override (zero value)
		Multiplier:      0,                // Don't override (zero value)
		SuccessPattern:  "deployment ok",  // Override
		FailurePattern:  "",               // Don't override (empty)
		CaseInsensitive: false,            // This is tricky - false could be intentional
	}

	// When merging configurations
	result := baseConfig.MergeWithFlags(flagConfig)

	// Then flag values should override base values where non-zero/non-empty
	assert.Equal(t, 10, result.Attempts)                    // Overridden
	assert.Equal(t, 2*time.Second, result.Delay)            // Kept from base
	assert.Equal(t, 60*time.Second, result.Timeout)         // Overridden
	assert.Equal(t, "exponential", result.BackoffType)      // Kept from base
	assert.Equal(t, 10*time.Second, result.MaxDelay)        // Kept from base
	assert.Equal(t, 2.5, result.Multiplier)                 // Kept from base
	assert.Equal(t, "deployment ok", result.SuccessPattern) // Overridden
	assert.Equal(t, "error", result.FailurePattern)         // Kept from base
	assert.True(t, result.CaseInsensitive)                  // Kept from base (bool handling)
}

func TestConfig_FindConfigFile(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Create config file in temp directory
	configFile := filepath.Join(tmpDir, ".retry.toml")
	err := os.WriteFile(configFile, []byte("attempts = 5"), 0644)
	require.NoError(t, err)

	// When finding config file in the directory
	found := FindConfigFile(tmpDir)

	// Then it should find the config file
	assert.Equal(t, configFile, found)
}

func TestConfig_FindConfigFileNotFound(t *testing.T) {
	// When finding config file in directory without config
	tmpDir := t.TempDir()
	found := FindConfigFile(tmpDir)

	// Then it should return empty string
	assert.Equal(t, "", found)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid config",
			config: Config{
				Attempts:        3,
				Delay:           time.Second,
				Timeout:         30 * time.Second,
				BackoffType:     "exponential",
				MaxDelay:        10 * time.Second,
				Multiplier:      2.0,
				SuccessPattern:  "success",
				FailurePattern:  "error",
				CaseInsensitive: true,
			},
			expectError: false,
		},
		{
			name: "invalid attempts - zero",
			config: Config{
				Attempts: 0,
			},
			expectError: true,
			errorMsg:    "must be greater than 0",
		},
		{
			name: "invalid attempts - too high",
			config: Config{
				Attempts: 1001,
			},
			expectError: true,
			errorMsg:    "must be 1000 or less",
		},
		{
			name: "invalid delay - negative",
			config: Config{
				Attempts: 3,
				Delay:    -time.Second,
			},
			expectError: true,
			errorMsg:    "must be non-negative",
		},
		{
			name: "invalid timeout - negative",
			config: Config{
				Attempts: 3,
				Timeout:  -time.Second,
			},
			expectError: true,
			errorMsg:    "must be non-negative",
		},
		{
			name: "invalid backoff type",
			config: Config{
				Attempts:    3,
				BackoffType: "invalid",
			},
			expectError: true,
			errorMsg:    "must be 'fixed' or 'exponential'",
		},
		{
			name: "invalid max delay - less than delay",
			config: Config{
				Attempts: 3,
				Delay:    5 * time.Second,
				MaxDelay: 2 * time.Second,
			},
			expectError: true,
			errorMsg:    "must be greater than or equal to base delay",
		},
		{
			name: "invalid multiplier - too low",
			config: Config{
				Attempts:   3,
				Multiplier: 0.5,
			},
			expectError: true,
			errorMsg:    "must be 1.0 or greater",
		},
		{
			name: "invalid multiplier - too high",
			config: Config{
				Attempts:   3,
				Multiplier: 11.0,
			},
			expectError: true,
			errorMsg:    "must be 10.0 or less",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_LoadWithEnvironment(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"RETRY_ATTEMPTS", "RETRY_DELAY", "RETRY_TIMEOUT", "RETRY_BACKOFF",
		"RETRY_MAX_DELAY", "RETRY_MULTIPLIER", "RETRY_SUCCESS_PATTERN",
		"RETRY_FAILURE_PATTERN", "RETRY_CASE_INSENSITIVE",
	}

	for _, envVar := range envVars {
		originalEnv[envVar] = os.Getenv(envVar)
		os.Unsetenv(envVar)
	}

	// Restore environment after test
	defer func() {
		for _, envVar := range envVars {
			if value, exists := originalEnv[envVar]; exists && value != "" {
				os.Setenv(envVar, value)
			} else {
				os.Unsetenv(envVar)
			}
		}
	}()

	// Set test environment variables
	os.Setenv("RETRY_ATTEMPTS", "5")
	os.Setenv("RETRY_DELAY", "2s")
	os.Setenv("RETRY_TIMEOUT", "30s")
	os.Setenv("RETRY_BACKOFF", "exponential")
	os.Setenv("RETRY_MAX_DELAY", "10s")
	os.Setenv("RETRY_MULTIPLIER", "2.5")
	os.Setenv("RETRY_SUCCESS_PATTERN", "deployment successful")
	os.Setenv("RETRY_FAILURE_PATTERN", "(?i)error|failed")
	os.Setenv("RETRY_CASE_INSENSITIVE", "true")

	// When loading configuration with environment variables
	config, err := LoadWithEnvironment()

	// Then it should load all values from environment
	require.NoError(t, err)
	assert.Equal(t, 5, config.Attempts)
	assert.Equal(t, 2*time.Second, config.Delay)
	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, "exponential", config.BackoffType)
	assert.Equal(t, 10*time.Second, config.MaxDelay)
	assert.Equal(t, 2.5, config.Multiplier)
	assert.Equal(t, "deployment successful", config.SuccessPattern)
	assert.Equal(t, "(?i)error|failed", config.FailurePattern)
	assert.True(t, config.CaseInsensitive)
}

func TestConfig_LoadWithEnvironment_PartialOverride(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{"RETRY_ATTEMPTS", "RETRY_DELAY"}

	for _, envVar := range envVars {
		originalEnv[envVar] = os.Getenv(envVar)
		os.Unsetenv(envVar)
	}

	// Restore environment after test
	defer func() {
		for _, envVar := range envVars {
			if value, exists := originalEnv[envVar]; exists && value != "" {
				os.Setenv(envVar, value)
			} else {
				os.Unsetenv(envVar)
			}
		}
	}()

	// Set only some environment variables
	os.Setenv("RETRY_ATTEMPTS", "10")
	os.Setenv("RETRY_DELAY", "1s")

	// When loading configuration with partial environment override
	config, err := LoadWithEnvironment()

	// Then it should use env vars where set and defaults elsewhere
	require.NoError(t, err)
	assert.Equal(t, 10, config.Attempts)              // From env
	assert.Equal(t, 1*time.Second, config.Delay)      // From env
	assert.Equal(t, time.Duration(0), config.Timeout) // Default
	assert.Equal(t, "fixed", config.BackoffType)      // Default
}

func TestConfig_LoadWithPrecedence(t *testing.T) {
	// Create temporary config file
	configContent := `
attempts = 5
delay = "2s"
timeout = "30s"
backoff = "exponential"
`
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "retry.toml")
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// Save and set environment variables
	originalEnv := os.Getenv("RETRY_ATTEMPTS")
	defer func() {
		if originalEnv != "" {
			os.Setenv("RETRY_ATTEMPTS", originalEnv)
		} else {
			os.Unsetenv("RETRY_ATTEMPTS")
		}
	}()
	os.Setenv("RETRY_ATTEMPTS", "7") // Should override config file

	// Create flag config (should override both env and config file)
	flagConfig := &Config{
		Attempts: 10, // Should override env and config file
		Delay:    0,  // Should not override (zero value)
	}

	// When loading with precedence
	config, debugInfo, err := LoadWithPrecedence(configFile, flagConfig, true)

	// Then precedence should be: CLI > env > config file > defaults
	require.NoError(t, err)
	require.NotNil(t, debugInfo)

	assert.Equal(t, 10, config.Attempts)               // From CLI flag
	assert.Equal(t, 2*time.Second, config.Delay)       // From config file
	assert.Equal(t, 30*time.Second, config.Timeout)    // From config file
	assert.Equal(t, "exponential", config.BackoffType) // From config file

	// Check debug info
	assert.Equal(t, SourceCLIFlag, debugInfo.Sources["attempts"])
	assert.Equal(t, SourceConfigFile, debugInfo.Sources["delay"])
	assert.Equal(t, SourceConfigFile, debugInfo.Sources["timeout"])
	assert.Equal(t, SourceConfigFile, debugInfo.Sources["backoff"])
}

func TestConfig_LoadWithPrecedence_ValidationError(t *testing.T) {
	// Create config with invalid values
	flagConfig := &Config{
		Attempts: -1, // Invalid
	}

	// When loading with invalid configuration
	config, debugInfo, err := LoadWithPrecedence("", flagConfig, false)

	// Then it should return validation error
	require.Error(t, err)
	assert.Nil(t, config)
	assert.Nil(t, debugInfo)
	assert.Contains(t, err.Error(), "validation errors")
	assert.Contains(t, err.Error(), "must be greater than 0")
}

func TestConfig_LoadWithEnvironment_InvalidValues(t *testing.T) {
	// Save original environment
	originalEnv := os.Getenv("RETRY_ATTEMPTS")
	defer func() {
		if originalEnv != "" {
			os.Setenv("RETRY_ATTEMPTS", originalEnv)
		} else {
			os.Unsetenv("RETRY_ATTEMPTS")
		}
	}()

	// Set invalid environment variable
	os.Setenv("RETRY_ATTEMPTS", "invalid")

	// When loading configuration with invalid environment variable
	config, err := LoadWithEnvironment()

	// Then it should return an error
	require.Error(t, err)
	assert.Nil(t, config)
}

func TestConfigDebugInfo_PrintDebugInfo(t *testing.T) {
	// Given debug info with various sources
	debugInfo := &ConfigDebugInfo{
		Sources: map[string]ConfigSource{
			"attempts": SourceCLIFlag,
			"delay":    SourceConfigFile,
			"timeout":  SourceEnvironment,
			"backoff":  SourceDefault,
		},
		Values: map[string]interface{}{
			"attempts": 10,
			"delay":    "2s",
			"timeout":  "30s",
			"backoff":  "fixed",
		},
	}

	// When printing debug info (just verify it doesn't panic)
	// In a real test, we might capture stdout, but for now just ensure no panic
	require.NotPanics(t, func() {
		debugInfo.PrintDebugInfo()
	})
}
