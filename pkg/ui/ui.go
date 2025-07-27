package ui

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Reporter handles status reporting and terminal output
type Reporter struct {
	writer io.Writer
	quiet  bool
}

// RunStats tracks statistics for a retry run
type RunStats struct {
	TotalAttempts    int
	SuccessfulRuns   int
	FailedRuns       int
	TotalDuration    time.Duration
	FinalReason      string
	Success          bool
	startTime        time.Time
	attemptStartTime time.Time
}

// NewReporter creates a new status reporter
func NewReporter(writer io.Writer) *Reporter {
	return &Reporter{
		writer: writer,
		quiet:  false,
	}
}

// SetQuiet enables or disables quiet mode (suppresses real-time messages)
func (r *Reporter) SetQuiet(quiet bool) {
	r.quiet = quiet
}

// AttemptStart reports the start of a retry attempt
func (r *Reporter) AttemptStart(attempt, maxAttempts int) {
	if r.quiet {
		return
	}
	fmt.Fprintf(r.writer, "[retry] Attempt %d/%d starting...\n", attempt, maxAttempts)
}

// AttemptFailure reports a failed attempt with reason and next delay
func (r *Reporter) AttemptFailure(attempt, maxAttempts int, reason string, nextDelay time.Duration) {
	if r.quiet {
		return
	}

	// High-frequency optimization: Use string builder for efficient string operations
	var builder strings.Builder
	builder.WriteString("[retry] Attempt ")
	builder.WriteString(strconv.Itoa(attempt))
	builder.WriteByte('/')
	builder.WriteString(strconv.Itoa(maxAttempts))
	builder.WriteString(" failed (")
	builder.WriteString(reason)
	builder.WriteString(")")

	if attempt == maxAttempts {
		// Last attempt, no retry
		builder.WriteString(".\n")
	} else {
		// Will retry
		builder.WriteString(". Retrying in ")
		builder.WriteString(r.formatDuration(nextDelay))
		builder.WriteString(".\n")
	}

	fmt.Fprint(r.writer, builder.String())
}

// FinalSummary reports the final outcome and statistics
func (r *Reporter) FinalSummary(stats *RunStats) {
	// Success/failure message with emoji
	if stats.Success {
		if stats.TotalAttempts == 1 {
			fmt.Fprintf(r.writer, "✅ [retry] Command succeeded after 1 attempt.\n")
		} else {
			fmt.Fprintf(r.writer, "✅ [retry] Command succeeded after %d attempts.\n", stats.TotalAttempts)
		}
	} else {
		if stats.TotalAttempts == 1 {
			fmt.Fprintf(r.writer, "❌ [retry] Command failed after 1 attempt.\n")
		} else {
			fmt.Fprintf(r.writer, "❌ [retry] Command failed after %d attempts.\n", stats.TotalAttempts)
		}
	}

	// Run statistics
	fmt.Fprintf(r.writer, "\nRun Statistics:\n")
	fmt.Fprintf(r.writer, "  Total Attempts: %d\n", stats.TotalAttempts)
	fmt.Fprintf(r.writer, "  Successful Runs: %d\n", stats.SuccessfulRuns)
	fmt.Fprintf(r.writer, "  Failed Runs: %d\n", stats.FailedRuns)
	fmt.Fprintf(r.writer, "  Total Duration: %s\n", r.formatDuration(stats.TotalDuration))
	fmt.Fprintf(r.writer, "  Final Reason: %s\n", stats.FinalReason)
}

// formatDuration formats a duration in a human-readable way
func (r *Reporter) formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}

	// Handle sub-second durations
	if d < time.Second {
		return fmt.Sprintf("%.1fs", float64(d)/float64(time.Second))
	}

	// Handle durations with fractional seconds
	if d < time.Minute {
		seconds := float64(d) / float64(time.Second)
		if seconds == float64(int(seconds)) {
			return fmt.Sprintf("%.0fs", seconds)
		}
		// Format with minimal decimal places, removing trailing zeros
		formatted := fmt.Sprintf("%.2f", seconds)
		// Remove trailing zeros
		formatted = strings.TrimRight(formatted, "0")
		// Remove trailing decimal point if all decimals were zeros
		formatted = strings.TrimRight(formatted, ".")
		return formatted + "s"
	}

	// Handle longer durations
	hours := d / time.Hour
	minutes := (d % time.Hour) / time.Minute
	seconds := (d % time.Minute) / time.Second

	if hours > 0 {
		if minutes > 0 && seconds > 0 {
			return fmt.Sprintf("%dh%dm%ds", hours, minutes, seconds)
		} else if minutes > 0 {
			return fmt.Sprintf("%dh%dm", hours, minutes)
		} else if seconds > 0 {
			return fmt.Sprintf("%dh%ds", hours, seconds)
		}
		return fmt.Sprintf("%dh", hours)
	}

	if minutes > 0 {
		if seconds > 0 {
			return fmt.Sprintf("%dm%ds", minutes, seconds)
		}
		return fmt.Sprintf("%dm", minutes)
	}

	return fmt.Sprintf("%ds", seconds)
}

// NewRunStats creates a new run statistics tracker
func NewRunStats() *RunStats {
	return &RunStats{
		startTime: time.Now(),
	}
}

// RecordAttemptStart records the start of an attempt
func (s *RunStats) RecordAttemptStart() {
	s.attemptStartTime = time.Now()
	s.TotalAttempts++
}

// RecordAttemptEnd records the end of an attempt
func (s *RunStats) RecordAttemptEnd(success bool, reason string) {
	if success {
		s.SuccessfulRuns++
	} else {
		s.FailedRuns++
	}
}

// Finalize calculates final statistics
func (s *RunStats) Finalize(success bool, finalReason string) {
	s.Success = success
	s.FinalReason = finalReason
	s.TotalDuration = time.Since(s.startTime)
}
