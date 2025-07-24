package ui

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestReporter_AttemptStart(t *testing.T) {
	// Given a reporter with a buffer
	var buf bytes.Buffer
	reporter := NewReporter(&buf)

	// When reporting attempt start
	reporter.AttemptStart(1, 3)

	// Then it should output the correct message
	output := buf.String()
	assert.Contains(t, output, "[retry] Attempt 1/3 starting...")
}

func TestReporter_AttemptFailure_ExitCode(t *testing.T) {
	// Given a reporter with a buffer
	var buf bytes.Buffer
	reporter := NewReporter(&buf)

	// When reporting attempt failure with exit code
	reporter.AttemptFailure(1, 3, "exit code 1", 2*time.Second)

	// Then it should output the correct message
	output := buf.String()
	assert.Contains(t, output, "[retry] Attempt 1/3 failed (exit code 1). Retrying in 2s.")
}

func TestReporter_AttemptFailure_Timeout(t *testing.T) {
	// Given a reporter with a buffer
	var buf bytes.Buffer
	reporter := NewReporter(&buf)

	// When reporting attempt failure with timeout
	reporter.AttemptFailure(2, 5, "timeout: 10s", 4500*time.Millisecond)

	// Then it should output the correct message
	output := buf.String()
	assert.Contains(t, output, "[retry] Attempt 2/5 failed (timeout: 10s). Retrying in 4.5s.")
}

func TestReporter_AttemptFailure_PatternMatch(t *testing.T) {
	// Given a reporter with a buffer
	var buf bytes.Buffer
	reporter := NewReporter(&buf)

	// When reporting attempt failure with pattern match
	reporter.AttemptFailure(3, 5, "failure pattern matched", 1*time.Second)

	// Then it should output the correct message
	output := buf.String()
	assert.Contains(t, output, "[retry] Attempt 3/5 failed (failure pattern matched). Retrying in 1s.")
}

func TestReporter_AttemptFailure_LastAttempt(t *testing.T) {
	// Given a reporter with a buffer
	var buf bytes.Buffer
	reporter := NewReporter(&buf)

	// When reporting failure on the last attempt (no retry)
	reporter.AttemptFailure(3, 3, "exit code 1", 0)

	// Then it should not mention retrying
	output := buf.String()
	assert.Contains(t, output, "[retry] Attempt 3/3 failed (exit code 1).")
	assert.NotContains(t, output, "Retrying")
}

func TestReporter_FinalSuccess(t *testing.T) {
	// Given a reporter with a buffer
	var buf bytes.Buffer
	reporter := NewReporter(&buf)

	// And run statistics
	stats := &RunStats{
		TotalAttempts:  3,
		SuccessfulRuns: 1,
		FailedRuns:     2,
		TotalDuration:  5*time.Second + 500*time.Millisecond,
		FinalReason:    "exit code 0",
		Success:        true,
	}

	// When reporting final success
	reporter.FinalSummary(stats)

	// Then it should output success message and statistics
	output := buf.String()
	assert.Contains(t, output, "✅ [retry] Command succeeded after 3 attempts.")
	assert.Contains(t, output, "Total Attempts: 3")
	assert.Contains(t, output, "Successful Runs: 1")
	assert.Contains(t, output, "Failed Runs: 2")
	assert.Contains(t, output, "Total Duration: 5.5s")
	assert.Contains(t, output, "Final Reason: exit code 0")
}

func TestReporter_FinalFailure(t *testing.T) {
	// Given a reporter with a buffer
	var buf bytes.Buffer
	reporter := NewReporter(&buf)

	// And run statistics
	stats := &RunStats{
		TotalAttempts:  5,
		SuccessfulRuns: 0,
		FailedRuns:     5,
		TotalDuration:  15*time.Second + 750*time.Millisecond,
		FinalReason:    "max retries reached",
		Success:        false,
	}

	// When reporting final failure
	reporter.FinalSummary(stats)

	// Then it should output failure message and statistics
	output := buf.String()
	assert.Contains(t, output, "❌ [retry] Command failed after 5 attempts.")
	assert.Contains(t, output, "Total Attempts: 5")
	assert.Contains(t, output, "Successful Runs: 0")
	assert.Contains(t, output, "Failed Runs: 5")
	assert.Contains(t, output, "Total Duration: 15.75s")
	assert.Contains(t, output, "Final Reason: max retries reached")
}

func TestReporter_FinalSuccess_SingleAttempt(t *testing.T) {
	// Given a reporter with a buffer
	var buf bytes.Buffer
	reporter := NewReporter(&buf)

	// And run statistics for single successful attempt
	stats := &RunStats{
		TotalAttempts:  1,
		SuccessfulRuns: 1,
		FailedRuns:     0,
		TotalDuration:  2 * time.Second,
		FinalReason:    "success pattern matched",
		Success:        true,
	}

	// When reporting final success
	reporter.FinalSummary(stats)

	// Then it should use singular form
	output := buf.String()
	assert.Contains(t, output, "✅ [retry] Command succeeded after 1 attempt.")
}

func TestReporter_DurationFormatting(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{"milliseconds", 500 * time.Millisecond, "0.5s"},
		{"seconds", 2 * time.Second, "2s"},
		{"seconds with milliseconds", 2*time.Second + 500*time.Millisecond, "2.5s"},
		{"minutes", 90 * time.Second, "1m30s"},
		{"hours", 3661 * time.Second, "1h1m1s"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given a reporter
			var buf bytes.Buffer
			reporter := NewReporter(&buf)

			// When formatting duration
			formatted := reporter.formatDuration(tt.duration)

			// Then it should match expected format
			assert.Equal(t, tt.expected, formatted)
		})
	}
}

func TestReporter_Quiet_Mode(t *testing.T) {
	// Given a reporter in quiet mode
	var buf bytes.Buffer
	reporter := NewReporter(&buf)
	reporter.SetQuiet(true)

	// When reporting attempt start and failure
	reporter.AttemptStart(1, 3)
	reporter.AttemptFailure(1, 3, "exit code 1", 2*time.Second)

	// Then it should not output real-time messages
	output := buf.String()
	assert.Empty(t, output)

	// But final summary should still be shown
	stats := &RunStats{
		TotalAttempts:  3,
		SuccessfulRuns: 1,
		FailedRuns:     2,
		TotalDuration:  5 * time.Second,
		FinalReason:    "exit code 0",
		Success:        true,
	}
	reporter.FinalSummary(stats)

	output = buf.String()
	assert.Contains(t, output, "✅ [retry] Command succeeded after 3 attempts.")
}

func TestRunStats_CalculateStats(t *testing.T) {
	// Given a new run stats tracker
	stats := NewRunStats()

	// When recording attempts
	stats.RecordAttemptStart()
	stats.RecordAttemptEnd(false, "exit code 1")

	stats.RecordAttemptStart()
	stats.RecordAttemptEnd(false, "timeout: 5s")

	stats.RecordAttemptStart()
	stats.RecordAttemptEnd(true, "exit code 0")

	// And finalizing
	stats.Finalize(true, "exit code 0")

	// Then it should have correct statistics
	assert.Equal(t, 3, stats.TotalAttempts)
	assert.Equal(t, 1, stats.SuccessfulRuns)
	assert.Equal(t, 2, stats.FailedRuns)
	assert.True(t, stats.Success)
	assert.Equal(t, "exit code 0", stats.FinalReason)
	assert.True(t, stats.TotalDuration > 0)
}
