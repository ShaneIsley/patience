//go:build unix

// Package run executes a command repeatedly according to a backoff schedule
// and decides each attempt's outcome from its exit code or its output.
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"slices"
	"syscall"
	"time"
)

// Exit codes patience uses for its own conditions. They follow timeout(1) and
// the shell: 124 timed out, 126 not executable, 127 not found.
const (
	ExitTimeout    = 124
	ExitNotExec    = 126
	ExitNotFound   = 127
	ExitPatternErr = 1
)

// defaultKillGrace is how long a timed-out attempt gets after SIGTERM before SIGKILL.
const defaultKillGrace = 2 * time.Second

// Options controls a Run.
type Options struct {
	Attempts  int           // total attempts; 0 = unlimited
	Timeout   time.Duration // per attempt; 0 = none
	KillGrace time.Duration // after a timeout's SIGTERM, how long before SIGKILL; 0 = 2s
	MaxTime   time.Duration // overall budget including delays; 0 = none
	RetryOn   []int         // retry only on these exit codes; nil = any non-zero

	Success *regexp.Regexp // output matching this makes the attempt succeed
	Failure *regexp.Regexp // output matching this makes the attempt fail (wins over Success)

	Next func(attempt int) time.Duration // delay after failed attempt k (1-based)

	// Foreground runs the command in patience's own process group so it can
	// use the terminal; a timeout then kills only the direct child, and the
	// terminal delivers Ctrl-C to it directly.
	Foreground bool

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer

	Report Reporter // may be nil

	// Test hooks. Nil means the real clock and time.Sleep.
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error
}

// Reporter receives progress. Methods are called from the Run goroutine.
type Reporter interface {
	AttemptStart(attempt, total int) // total is 0 when unlimited
	AttemptEnd(attempt int, o Outcome, retrying bool, delay time.Duration)
	Done(r Result)
}

// Outcome is one attempt's result.
type Outcome struct {
	ExitCode int           // what the command (or patience's mapping) produced
	Signal   os.Signal     // non-nil if the command died from a signal
	TimedOut bool          // killed by --timeout
	Success  bool          // final verdict after patterns and exit code
	Reason   string        // human-readable verdict
	Elapsed  time.Duration // wall time of this attempt
	ExecErr  error         // could not start the command (126/127); never retried
}

// Result is the whole run.
type Result struct {
	Attempts    int
	Last        Outcome
	ExitCode    int           // what patience should exit with
	Elapsed     time.Duration // whole run including delays
	Reason      string        // why the run stopped
	Interrupted os.Signal     // non-nil if patience itself was signalled
}

// Run executes cmd until it succeeds or the options say stop. It returns an
// error only for programming errors (empty command); every command failure is
// reported through Result.
func Run(ctx context.Context, cmd []string, o Options) (Result, error) {
	if len(cmd) == 0 {
		return Result{}, errors.New("no command given")
	}
	o = o.withDefaults()

	// Signals are owned here for the whole run, not per attempt, so a Ctrl-C
	// during a backoff sleep is seen too. The first SIGINT/SIGTERM is forwarded
	// to the child's process group; the second kills it.
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigs)

	start := o.Now()
	var r Result
	for attempt := 1; ; attempt++ {
		r.Attempts = attempt
		o.report(func(rep Reporter) { rep.AttemptStart(attempt, o.Attempts) })

		out, interrupted := o.runOnce(ctx, cmd, sigs)
		r.Last = out
		r.Elapsed = o.Now().Sub(start)
		if interrupted != nil {
			r.interrupt(interrupted)
			break
		}
		if out.Success {
			r.ExitCode, r.Reason = 0, out.Reason
			o.report(func(rep Reporter) { rep.AttemptEnd(attempt, out, false, 0) })
			break
		}

		stop, delay := o.decide(attempt, out, r.Elapsed)
		o.report(func(rep Reporter) { rep.AttemptEnd(attempt, out, stop == "", delay) })
		if stop != "" {
			r.ExitCode, r.Reason = out.ExitCode, stop
			break
		}
		if sig := o.wait(ctx, delay, sigs); sig != nil {
			r.interrupt(sig)
			break
		}
	}
	r.Elapsed = o.Now().Sub(start)
	o.report(func(rep Reporter) { rep.Done(r) })
	return r, nil
}

func (o Options) withDefaults() Options {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Sleep == nil {
		o.Sleep = sleepCtx
	}
	if o.KillGrace <= 0 {
		o.KillGrace = defaultKillGrace
	}
	if o.Stdin == nil {
		o.Stdin = os.Stdin
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.Next == nil {
		o.Next = func(int) time.Duration { return 0 }
	}
	return o
}

func (o Options) report(f func(Reporter)) {
	if o.Report != nil {
		f(o.Report)
	}
}

func (r *Result) interrupt(sig os.Signal) {
	r.Interrupted = sig
	r.ExitCode = exitForSignal(sig)
	r.Reason = "interrupted by " + sig.String()
}

// decide returns why to stop after a failed attempt (empty = retry) and the
// delay to wait before the next one.
func (o Options) decide(attempt int, out Outcome, elapsed time.Duration) (stop string, delay time.Duration) {
	switch {
	case out.ExecErr != nil:
		return out.Reason, 0
	case o.Attempts > 0 && attempt >= o.Attempts:
		return fmt.Sprintf("all %d attempts failed (%s)", o.Attempts, out.Reason), 0
	case o.RetryOn != nil && !slices.Contains(o.RetryOn, out.ExitCode):
		return fmt.Sprintf("exit code %d is not in --retry-on", out.ExitCode), 0
	}
	delay = o.Next(attempt)
	if o.MaxTime > 0 && elapsed+delay >= o.MaxTime {
		return fmt.Sprintf("--max-time %v reached (%s)", o.MaxTime, out.Reason), delay
	}
	return "", delay
}

// wait sleeps for d unless a signal or ctx cancellation arrives first.
func (o Options) wait(ctx context.Context, d time.Duration, sigs <-chan os.Signal) os.Signal {
	if d <= 0 {
		select {
		case s := <-sigs:
			return s
		default:
			return nil
		}
	}
	sctx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() { _ = o.Sleep(sctx, d); close(done) }()
	select {
	case s := <-sigs:
		cancel()
		<-done // the sleep goroutine exits once cancelled
		return s
	case <-done:
		return nil
	}
}

// runOnce starts the command, streams its output, applies the timeout and
// forwards signals. It returns the outcome and, if patience itself was
// signalled, that signal.
func (o Options) runOnce(ctx context.Context, argv []string, sigs <-chan os.Signal) (Outcome, os.Signal) {
	m := &matcher{success: o.Success, failure: o.Failure}
	stdout := newScanWriter(o.Stdout, m)
	stderr := newScanWriter(o.Stderr, m)

	c := exec.Command(argv[0], argv[1:]...) //nolint:gosec // running the user's command is the purpose
	c.Stdin = o.Stdin
	c.Stdout = stdout
	c.Stderr = stderr
	if !o.Foreground {
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	t0 := o.Now()
	if err := c.Start(); err != nil {
		return execFailure(err, argv[0]), nil
	}
	s := o.supervise(ctx, c, sigs)
	flush(stdout)
	flush(stderr)

	out := Outcome{Elapsed: o.Now().Sub(t0), TimedOut: s.timedOut}
	if s.cancelled {
		out.ExitCode, out.Reason = ExitTimeout, "cancelled"
		return out, nil
	}
	out.ExitCode, out.Signal = exitStatus(s.waitErr)

	// In foreground mode the terminal sends Ctrl-C straight to the child; a
	// child that died of SIGINT means the user wants out.
	interrupted := s.interrupted
	if o.Foreground && interrupted == nil && out.Signal == syscall.SIGINT {
		interrupted = syscall.SIGINT
	}
	if interrupted != nil {
		return out, interrupted
	}
	o.classify(&out, m)
	return out, nil
}

type supervision struct {
	waitErr     error
	interrupted os.Signal
	timedOut    bool
	cancelled   bool
}

// supervise waits for the started command while handling the timeout,
// forwarded signals and context cancellation.
func (o Options) supervise(ctx context.Context, c *exec.Cmd, sigs <-chan os.Signal) supervision {
	kill := func(sig syscall.Signal) {
		if o.Foreground {
			_ = c.Process.Signal(sig)
		} else {
			_ = syscall.Kill(-c.Process.Pid, sig)
		}
	}
	waitDone := make(chan error, 1)
	go func() { waitDone <- c.Wait() }()

	var timer <-chan time.Time
	if o.Timeout > 0 {
		tm := time.NewTimer(o.Timeout)
		defer tm.Stop()
		timer = tm.C
	}

	// grace fires KillGrace after SIGTERM; it is armed only on timeout and
	// dies with this function, so a command that exits in time is never
	// followed by a SIGKILL to a process group that may since have been reused.
	var grace <-chan time.Time
	var s supervision
	signalled := 0
	for {
		select {
		case s.waitErr = <-waitDone:
			return s
		case <-timer:
			s.timedOut = true
			timer = nil
			kill(syscall.SIGTERM)
			gt := time.NewTimer(o.KillGrace)
			defer gt.Stop()
			grace = gt.C
		case <-grace:
			grace = nil
			kill(syscall.SIGKILL)
		case sig := <-sigs:
			s.interrupted = sig
			signalled++
			switch {
			case signalled > 1:
				kill(syscall.SIGKILL)
			case !o.Foreground:
				kill(sig.(syscall.Signal))
				// In foreground mode the terminal already delivered it.
			}
		case <-ctx.Done():
			kill(syscall.SIGKILL)
			s.waitErr = <-waitDone // reap; the status is irrelevant once cancelled
			s.cancelled = true
			return s
		}
	}
}

// classify sets Success and Reason from the timeout, patterns and exit status.
func (o Options) classify(out *Outcome, m *matcher) {
	switch {
	case out.TimedOut:
		out.ExitCode = ExitTimeout
		out.Reason = fmt.Sprintf("timed out after %v", o.Timeout)
	case m.sawFailure:
		out.Reason = "failure pattern matched"
		if out.ExitCode == 0 {
			out.ExitCode = ExitPatternErr
		}
	case m.sawSuccess:
		out.Success = true
		out.Reason = "success pattern matched"
	case out.Signal != nil:
		out.Reason = "killed by " + out.Signal.String()
	case out.ExitCode == 0:
		out.Success = true
		out.Reason = "exit code 0"
	default:
		out.Reason = fmt.Sprintf("exit code %d", out.ExitCode)
	}
}

func flush(w io.Writer) {
	if sw, ok := w.(*scanWriter); ok {
		sw.flush()
	}
}

func execFailure(err error, name string) Outcome {
	code := ExitNotFound
	reason := "not found"
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.ENOEXEC) {
		code = ExitNotExec
		reason = "not executable"
	}
	return Outcome{ExitCode: code, ExecErr: err, Reason: fmt.Sprintf("%s: %s", name, reason)}
}

// exitStatus maps a Wait error to (exit code, signal).
func exitStatus(err error) (int, os.Signal) {
	if err == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
			return 128 + int(ws.Signal()), ws.Signal()
		}
		return ee.ExitCode(), nil
	}
	// I/O error copying output: a failure, but not an exec error.
	return 1, nil
}

func exitForSignal(s os.Signal) int {
	if ss, ok := s.(syscall.Signal); ok {
		return 128 + int(ss)
	}
	return 130
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
