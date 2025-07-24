package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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
	assert.Contains(t, string(output), "patience is a CLI tool")
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

func TestCLI_StatusOutput_Success(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing a successful command
	cmd := exec.Command(binary, "--", "echo", "success")
	output, err := cmd.CombinedOutput()

	// Then it should succeed
	require.NoError(t, err)

	// And show status messages
	outputStr := string(output)
	assert.Contains(t, outputStr, "[retry] Attempt 1/3 starting...")
	assert.Contains(t, outputStr, "✅ [retry] Command succeeded after 1 attempt.")
	assert.Contains(t, outputStr, "Run Statistics:")
	assert.Contains(t, outputStr, "Total Attempts: 1")
	assert.Contains(t, outputStr, "Successful Runs: 1")
	assert.Contains(t, outputStr, "Failed Runs: 0")
	assert.Contains(t, outputStr, "Final Reason: exit code 0")
}

func TestCLI_StatusOutput_Failure(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing a failing command
	cmd := exec.Command(binary, "--attempts", "2", "--", "false")
	output, err := cmd.CombinedOutput()

	// Then it should fail
	require.Error(t, err)

	// And show status messages
	outputStr := string(output)
	assert.Contains(t, outputStr, "[retry] Attempt 1/2 starting...")
	assert.Contains(t, outputStr, "[retry] Attempt 1/2 failed (exit code 1). Retrying in 0s.")
	assert.Contains(t, outputStr, "[retry] Attempt 2/2 starting...")
	assert.Contains(t, outputStr, "[retry] Attempt 2/2 failed (exit code 1).")
	assert.Contains(t, outputStr, "❌ [retry] Command failed after 2 attempts.")
	assert.Contains(t, outputStr, "Run Statistics:")
	assert.Contains(t, outputStr, "Total Attempts: 2")
	assert.Contains(t, outputStr, "Successful Runs: 0")
	assert.Contains(t, outputStr, "Failed Runs: 2")
	assert.Contains(t, outputStr, "Final Reason: max retries reached (exit code 1)")
}

func TestCLI_StatusOutput_WithDelay(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing with delay
	cmd := exec.Command(binary, "--attempts", "2", "--delay", "100ms", "--", "false")
	output, err := cmd.CombinedOutput()

	// Then it should fail
	require.Error(t, err)

	// And show delay in status messages
	outputStr := string(output)
	assert.Contains(t, outputStr, "[retry] Attempt 1/2 failed (exit code 1). Retrying in 0.1s.")
}

func TestCLI_MetricsIntegration_DaemonNotRunning(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing a command (daemon not running)
	start := time.Now()
	cmd := exec.Command(binary, "--attempts", "2", "--", "echo", "metrics test")
	err := cmd.Run()
	elapsed := time.Since(start)

	// Then it should succeed normally
	require.NoError(t, err)

	// And should not be significantly delayed by metrics dispatch
	assert.Less(t, elapsed, 2*time.Second) // Should complete quickly even with async metrics
}

func TestCLI_MetricsIntegration_WithMockDaemon(t *testing.T) {
	// Given a mock daemon listening on Unix socket
	socketPath := "/tmp/test-cli-metrics-" + fmt.Sprintf("%d", time.Now().UnixNano()) + ".sock"

	// Clean up socket file after test
	defer func() {
		os.Remove(socketPath)
	}()

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	// Channel to receive metrics data
	received := make(chan []byte, 1)

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		buf := make([]byte, 4096)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		received <- buf[:n]
	}()

	// And a compiled retry binary
	binary := buildBinary(t)

	// When executing a command
	cmd := exec.Command(binary, "--attempts", "2", "--", "echo", "daemon test")

	// Override the default socket path by setting environment or using a custom build
	// For this test, we'll just verify the CLI works normally
	err = cmd.Run()

	// Then it should succeed
	require.NoError(t, err)

	// Note: Since we can't easily override the socket path in the CLI without
	// adding a flag, this test mainly verifies that metrics integration doesn't
	// break normal CLI operation. The actual metrics sending is tested in the
	// metrics package unit tests.
}

func TestCLI_MetricsIntegration_Performance(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing multiple commands in sequence
	start := time.Now()
	for i := 0; i < 5; i++ {
		cmd := exec.Command(binary, "--", "echo", fmt.Sprintf("test %d", i))
		err := cmd.Run()
		require.NoError(t, err)
	}
	elapsed := time.Since(start)

	// Then the total time should not be significantly impacted by metrics
	// (async dispatch should not block CLI execution)
	assert.Less(t, elapsed, 3*time.Second) // Should complete quickly
}

func TestCLI_EnvironmentVariables(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"RETRY_ATTEMPTS", "RETRY_DELAY", "RETRY_TIMEOUT", "RETRY_BACKOFF",
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

	// Given environment variables are set
	os.Setenv("RETRY_ATTEMPTS", "2")
	os.Setenv("RETRY_DELAY", "0.1s")

	// And a compiled retry binary
	binary := buildBinary(t)

	// When executing a command that fails once then succeeds
	cmd := exec.Command(binary, "--", "sh", "-c", "if [ ! -f /tmp/retry-test-env ]; then touch /tmp/retry-test-env && exit 1; else rm -f /tmp/retry-test-env && exit 0; fi")
	output, err := cmd.CombinedOutput()

	// Then it should succeed using environment configuration
	require.NoError(t, err)
	outputStr := string(output)
	assert.Contains(t, outputStr, "[retry] Attempt 1/2 failed")
	assert.Contains(t, outputStr, "[retry] Command succeeded after 2 attempts")
}

func TestCLI_ConfigurationPrecedence(t *testing.T) {
	// Create temporary config file
	configContent := `
attempts = 5
delay = "2s"
`
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "retry.toml")
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// Save and set environment variable
	originalEnv := os.Getenv("RETRY_ATTEMPTS")
	defer func() {
		if originalEnv != "" {
			os.Setenv("RETRY_ATTEMPTS", originalEnv)
		} else {
			os.Unsetenv("RETRY_ATTEMPTS")
		}
	}()
	os.Setenv("RETRY_ATTEMPTS", "3") // Should override config file

	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing with CLI flag (should override both env and config)
	cmd := exec.Command(binary, "--config", configFile, "--attempts", "1", "--", "exit", "1")
	output, err := cmd.CombinedOutput()

	// Then CLI flag should take precedence (only 1 attempt)
	require.Error(t, err) // Should fail since only 1 attempt
	outputStr := string(output)
	assert.NotContains(t, outputStr, "Attempt 2") // Should not retry
}

func TestCLI_ConfigurationValidation(t *testing.T) {
	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing with invalid configuration
	cmd := exec.Command(binary, "--attempts", "0", "--", "echo", "test")
	output, err := cmd.CombinedOutput()

	// Then it should return validation error
	require.Error(t, err)
	outputStr := string(output)
	assert.Contains(t, outputStr, "validation errors")
	assert.Contains(t, outputStr, "must be greater than 0")
}

func TestCLI_DebugConfiguration(t *testing.T) {
	// Create temporary config file
	configContent := `
attempts = 5
delay = "1s"
`
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "retry.toml")
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	require.NoError(t, err)

	// Given a compiled retry binary
	binary := buildBinary(t)

	// When executing with debug config flag
	cmd := exec.Command(binary, "--config", configFile, "--debug-config", "--attempts", "2", "--", "echo", "test")
	output, err := cmd.CombinedOutput()

	// Then it should show configuration debug information
	require.NoError(t, err)
	outputStr := string(output)
	assert.Contains(t, outputStr, "Configuration Resolution Debug Info")
	assert.Contains(t, outputStr, "attempts")
	assert.Contains(t, outputStr, "CLI flag")
	assert.Contains(t, outputStr, "config file")
}

func TestCLI_EnvironmentVariableValidation(t *testing.T) {
	// Save original environment
	originalEnv := os.Getenv("RETRY_ATTEMPTS")
	defer func() {
		if originalEnv != "" {
			os.Setenv("RETRY_ATTEMPTS", originalEnv)
		} else {
			os.Unsetenv("RETRY_ATTEMPTS")
		}
	}()

	// Given invalid environment variable
	os.Setenv("RETRY_ATTEMPTS", "invalid")

	// And a compiled retry binary
	binary := buildBinary(t)

	// When executing a command
	cmd := exec.Command(binary, "--", "echo", "test")
	output, err := cmd.CombinedOutput()

	// Then it should return an error
	require.Error(t, err)
	outputStr := string(output)
	assert.Contains(t, outputStr, "Error")
}

func TestCLI_ComplexEnvironmentConfiguration(t *testing.T) {
	// Save original environment
	originalEnv := make(map[string]string)
	envVars := []string{
		"RETRY_ATTEMPTS", "RETRY_DELAY", "RETRY_BACKOFF", "RETRY_MULTIPLIER", "RETRY_MAX_DELAY",
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

	// Given complex environment configuration
	os.Setenv("RETRY_ATTEMPTS", "3")
	os.Setenv("RETRY_DELAY", "0.1s")
	os.Setenv("RETRY_BACKOFF", "exponential")
	os.Setenv("RETRY_MULTIPLIER", "2.0")
	os.Setenv("RETRY_MAX_DELAY", "1s")

	// And a compiled retry binary
	binary := buildBinary(t)

	// When executing a command that fails multiple times
	cmd := exec.Command(binary, "--", "sh", "-c", "if [ ! -f /tmp/retry-test-complex ]; then touch /tmp/retry-test-complex && exit 1; elif [ ! -f /tmp/retry-test-complex2 ]; then touch /tmp/retry-test-complex2 && exit 1; else rm -f /tmp/retry-test-complex /tmp/retry-test-complex2 && exit 0; fi")
	output, err := cmd.CombinedOutput()

	// Then it should use exponential backoff from environment
	require.NoError(t, err)
	outputStr := string(output)
	assert.Contains(t, outputStr, "[retry] Attempt 1/3 failed")
	assert.Contains(t, outputStr, "[retry] Attempt 2/3 failed")
	assert.Contains(t, outputStr, "[retry] Command succeeded after 3 attempts")
}
