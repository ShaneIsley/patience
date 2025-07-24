package conditions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConditions_SuccessPattern(t *testing.T) {
	// Given a condition checker with success pattern
	checker, err := NewChecker("deployment successful", "", false)
	require.NoError(t, err)

	// When checking output that matches the success pattern
	result := checker.CheckSuccess(0, "deployment successful", "")

	// Then it should indicate success
	assert.True(t, result.Success)
	assert.Equal(t, "success pattern matched", result.Reason)
}

func TestConditions_SuccessPatternNoMatch(t *testing.T) {
	// Given a condition checker with success pattern
	checker, err := NewChecker("deployment successful", "", false)
	require.NoError(t, err)

	// When checking output that doesn't match the success pattern
	result := checker.CheckSuccess(0, "deployment failed", "")

	// Then it should indicate success based on exit code (0)
	assert.True(t, result.Success)
	assert.Equal(t, "exit code 0", result.Reason)
}

func TestConditions_FailurePattern(t *testing.T) {
	// Given a condition checker with failure pattern
	checker, err := NewChecker("", "(?i)(error|failed)", false)
	require.NoError(t, err)

	// When checking output that matches the failure pattern
	result := checker.CheckSuccess(0, "", "Error: connection failed")

	// Then it should indicate failure despite exit code 0
	assert.False(t, result.Success)
	assert.Equal(t, "failure pattern matched", result.Reason)
}

func TestConditions_CaseInsensitive(t *testing.T) {
	// Given a condition checker with case-insensitive matching
	checker, err := NewChecker("SUCCESS", "", true)
	require.NoError(t, err)

	// When checking output with different case
	result := checker.CheckSuccess(0, "deployment success", "")

	// Then it should match case-insensitively
	assert.True(t, result.Success)
	assert.Equal(t, "success pattern matched", result.Reason)
}

func TestConditions_InvalidRegex(t *testing.T) {
	// When creating a checker with invalid regex
	_, err := NewChecker("[invalid", "", false)

	// Then it should return an error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid success pattern")
}

func TestConditions_NoPatterns(t *testing.T) {
	// Given a condition checker with no patterns
	checker, err := NewChecker("", "", false)
	require.NoError(t, err)

	// When checking with exit code 0
	result := checker.CheckSuccess(0, "some output", "")
	assert.True(t, result.Success)
	assert.Equal(t, "exit code 0", result.Reason)

	// When checking with non-zero exit code
	result = checker.CheckSuccess(1, "some output", "")
	assert.False(t, result.Success)
	assert.Equal(t, "exit code 1", result.Reason)
}
