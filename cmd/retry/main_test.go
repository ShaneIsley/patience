package main

import (
	"os"
	"os/exec"
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
