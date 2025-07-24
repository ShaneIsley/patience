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
