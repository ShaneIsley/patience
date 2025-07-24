package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildBinary builds the retry binary for testing
func buildBinary(t *testing.T) string {
	t.Helper()

	// Build the binary
	cmd := exec.Command("go", "build", "-o", "retry-test", ".")
	cmd.Dir = "."
	err := cmd.Run()
	require.NoError(t, err, "Failed to build retry binary")

	// Clean up binary after test
	t.Cleanup(func() {
		os.Remove("retry-test")
	})

	return "./retry-test"
}

func TestCLI_RunSimpleSuccessCommand(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing: retry -- /bin/true
	cmd := exec.Command(binary, "--", "true")
	err := cmd.Run()

	// Then the CLI should exit with code 0
	require.NoError(t, err)

	// And the exit code should be 0 (success)
	assert.Equal(t, 0, cmd.ProcessState.ExitCode())
}

func TestCLI_RunSimpleFailCommand(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing: retry --attempts 2 -- /bin/false
	cmd := exec.Command(binary, "--attempts", "2", "--", "false")
	err := cmd.Run()

	// Then the CLI should exit with a non-zero code
	require.Error(t, err)

	// And the exit code should be 1 (failure)
	if exitError, ok := err.(*exec.ExitError); ok {
		assert.Equal(t, 1, exitError.ExitCode())
	} else {
		t.Fatalf("Expected ExitError, got %T", err)
	}
}

func TestCLI_WithDelayFlag(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing with delay flag: retry --attempts 2 --delay 10ms -- false
	cmd := exec.Command(binary, "--attempts", "2", "--delay", "10ms", "--", "false")
	err := cmd.Run()

	// Then it should still fail but take some time due to delay
	require.Error(t, err)
	if exitError, ok := err.(*exec.ExitError); ok {
		assert.Equal(t, 1, exitError.ExitCode())
	}
}

func TestCLI_WithTimeoutFlag(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing with timeout: retry --timeout 50ms -- sleep 0.2
	cmd := exec.Command(binary, "--timeout", "50ms", "--", "sleep", "0.2")
	err := cmd.Run()

	// Then it should fail due to timeout
	require.Error(t, err)
	if exitError, ok := err.(*exec.ExitError); ok {
		// Should exit with -1 (timeout exit code)
		assert.Equal(t, 255, exitError.ExitCode()) // -1 becomes 255 in exit code
	}
}

func TestCLI_HelpFlag(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing with --help
	cmd := exec.Command(binary, "--help")
	output, err := cmd.Output()

	// Then it should succeed and show help
	require.NoError(t, err)
	assert.Contains(t, string(output), "retry is a CLI tool")
	assert.Contains(t, string(output), "--attempts")
	assert.Contains(t, string(output), "--delay")
	assert.Contains(t, string(output), "--timeout")
}

func TestCLI_InvalidCommand(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing without command arguments
	cmd := exec.Command(binary)
	err := cmd.Run()

	// Then it should fail with usage error
	require.Error(t, err)
}

func TestCLI_ExponentialBackoff(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing with exponential backoff
	cmd := exec.Command(binary, "--attempts", "2", "--delay", "50ms", "--backoff", "exponential", "--", "false")
	err := cmd.Run()

	// Then it should still fail but use exponential delays
	require.Error(t, err)
	if exitError, ok := err.(*exec.ExitError); ok {
		assert.Equal(t, 1, exitError.ExitCode())
	}
}

func TestCLI_ExponentialBackoffWithMaxDelay(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing with exponential backoff and max delay
	cmd := exec.Command(binary,
		"--attempts", "3",
		"--delay", "100ms",
		"--backoff", "exponential",
		"--max-delay", "150ms",
		"--multiplier", "2.0",
		"--", "false")
	err := cmd.Run()

	// Then it should fail with capped delays
	require.Error(t, err)
	if exitError, ok := err.(*exec.ExitError); ok {
		assert.Equal(t, 1, exitError.ExitCode())
	}
}

func TestCLI_SuccessPattern(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing with success pattern that matches
	cmd := exec.Command(binary,
		"--success-pattern", "success",
		"--", "sh", "-c", "echo 'deployment success'; exit 1")
	err := cmd.Run()

	// Then it should succeed despite exit code 1
	require.NoError(t, err)
}

func TestCLI_FailurePattern(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing with failure pattern that matches
	cmd := exec.Command(binary,
		"--failure-pattern", "(?i)error",
		"--", "sh", "-c", "echo 'Error occurred'; exit 0")
	err := cmd.Run()

	// Then it should fail despite exit code 0
	require.Error(t, err)
	if exitError, ok := err.(*exec.ExitError); ok {
		assert.Equal(t, 1, exitError.ExitCode()) // Pattern match failure uses exit code 1
	}
}

func TestCLI_CaseInsensitivePattern(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing with case-insensitive success pattern
	cmd := exec.Command(binary,
		"--success-pattern", "SUCCESS",
		"--case-insensitive",
		"--", "sh", "-c", "echo 'deployment success'; exit 1")
	err := cmd.Run()

	// Then it should succeed with case-insensitive matching
	require.NoError(t, err)
}

func TestCLI_ConfigFile(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// And a configuration file
	configContent := `
attempts = 5
delay = "100ms"
backoff = "exponential"
success_pattern = "success"
`
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "retry.toml")
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// When executing with config file
	cmd := exec.Command(binary,
		"--config", configFile,
		"--", "sh", "-c", "echo 'deployment success'; exit 1")
	err = cmd.Run()

	// Then it should succeed using config file settings
	require.NoError(t, err)
}

func TestCLI_ConfigFileWithFlagOverride(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// And a configuration file with success pattern
	configContent := `
attempts = 2
success_pattern = "success"
`
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "retry.toml")
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// When executing with config file but overriding attempts via flag
	cmd := exec.Command(binary,
		"--config", configFile,
		"--attempts", "1", // Override config file value
		"--", "sh", "-c", "echo 'deployment success'; exit 1")
	err = cmd.Run()

	// Then it should succeed using flag override (1 attempt is enough due to success pattern)
	require.NoError(t, err)
}

func TestCLI_AutoDiscoverConfigFile(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// And a configuration file in current directory
	configContent := `
attempts = 1
success_pattern = "success"
`
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, ".retry.toml")
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// Get absolute path to binary
	absBinary, err := filepath.Abs(binary)
	require.NoError(t, err)

	// When executing from the directory with config file (no --config flag)
	cmd := exec.Command(absBinary, "--", "sh", "-c", "echo 'deployment success'; exit 1")
	cmd.Dir = tmpDir // Run from the temp directory
	err = cmd.Run()

	// Then it should succeed using auto-discovered config file
	require.NoError(t, err)
}

func TestCLI_InvalidConfigFile(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// And an invalid configuration file
	configContent := `
attempts = "invalid"
[broken toml
`
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "retry.toml")
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// When executing with invalid config file
	cmd := exec.Command(binary,
		"--config", configFile,
		"--", "true")
	err = cmd.Run()

	// Then it should fail with config error
	require.Error(t, err)
}
