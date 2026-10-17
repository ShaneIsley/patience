// Package ui prints retry progress to stderr.
package ui

import (
	"fmt"
	"io"
	"time"

	"github.com/shaneisley/patience/internal/run"
)

// Level selects how much is printed.
type Level int

// Levels, from silent to chatty.
const (
	Quiet   Level = iota // nothing
	Normal               // one line per failed attempt, one on stop
	Verbose              // also attempt starts and a final summary
)

// Reporter implements run.Reporter.
type Reporter struct {
	W     io.Writer
	Level Level
}

// AttemptStart is called before each attempt.
func (r Reporter) AttemptStart(attempt, total int) {
	if r.Level < Verbose {
		return
	}
	fmt.Fprintf(r.W, "patience: attempt %s starting\n", counter(attempt, total))
}

// AttemptEnd is called after each attempt with its outcome and the delay before the next.
func (r Reporter) AttemptEnd(attempt int, o run.Outcome, retrying bool, delay time.Duration) {
	if r.Level < Normal {
		return
	}
	if o.Success {
		if r.Level >= Verbose || attempt > 1 {
			fmt.Fprintf(r.W, "patience: attempt %d succeeded (%s)\n", attempt, o.Reason)
		}
		return
	}
	if retrying {
		fmt.Fprintf(r.W, "patience: attempt %d failed (%s); retrying in %s\n", attempt, o.Reason, fmtDur(delay))
		return
	}
	fmt.Fprintf(r.W, "patience: attempt %d failed (%s)\n", attempt, o.Reason)
}

// Done is called once when the run stops.
func (r Reporter) Done(res run.Result) {
	if r.Level < Normal {
		return
	}
	if res.ExitCode != 0 {
		fmt.Fprintf(r.W, "patience: giving up: %s\n", res.Reason)
	}
	if r.Level >= Verbose {
		fmt.Fprintf(r.W, "patience: %d attempt(s) in %s, exit %d\n", res.Attempts, fmtDur(res.Elapsed), res.ExitCode)
	}
}

func counter(attempt, total int) string {
	if total <= 0 {
		return fmt.Sprintf("%d", attempt)
	}
	return fmt.Sprintf("%d/%d", attempt, total)
}

// fmtDur prints durations the way people read them: 1.5s, 250ms, 2m0s.
func fmtDur(d time.Duration) string {
	switch {
	case d == 0:
		return "0s"
	case d < time.Second:
		return d.Round(time.Millisecond).String()
	case d < time.Minute:
		return d.Round(100 * time.Millisecond).String()
	default:
		return d.Round(time.Second).String()
	}
}
