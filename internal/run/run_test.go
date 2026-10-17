//go:build unix

package run

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sh runs a shell snippet as the command under test.
func sh(script string) []string { return []string{"sh", "-c", script} }

func noSleep(context.Context, time.Duration) error { return nil }

func runQuiet(t *testing.T, cmd []string, o Options) Result {
	t.Helper()
	if o.Stdout == nil {
		o.Stdout = io.Discard
	}
	if o.Stderr == nil {
		o.Stderr = io.Discard
	}
	if o.Stdin == nil {
		o.Stdin = strings.NewReader("")
	}
	if o.Sleep == nil {
		o.Sleep = noSleep
	}
	r, err := Run(context.Background(), cmd, o)
	require.NoError(t, err)
	return r
}

func TestEmptyCommand(t *testing.T) {
	_, err := Run(context.Background(), nil, Options{})
	assert.Error(t, err)
}

func TestSuccessFirstTry(t *testing.T) {
	var out bytes.Buffer
	r := runQuiet(t, sh("echo hi"), Options{Attempts: 3, Stdout: &out})
	assert.Equal(t, 0, r.ExitCode)
	assert.Equal(t, 1, r.Attempts)
	assert.Equal(t, "hi\n", out.String())
}

func TestExitCodePreserved(t *testing.T) {
	r := runQuiet(t, sh("exit 42"), Options{Attempts: 3})
	assert.Equal(t, 42, r.ExitCode)
	assert.Equal(t, 3, r.Attempts)
	assert.Contains(t, r.Reason, "all 3 attempts failed")
}

func TestEventualSuccess(t *testing.T) {
	// Fails twice using a counter file, then succeeds.
	f := filepath.Join(t.TempDir(), "n")
	script := fmt.Sprintf(`n=$(cat %[1]s 2>/dev/null || echo 0); n=$((n+1)); echo $n > %[1]s; [ $n -ge 3 ]`, f)
	var delays []time.Duration
	r := runQuiet(t, sh(script), Options{
		Attempts: 5,
		Next: func(k int) time.Duration {
			d := time.Duration(k) * time.Millisecond
			delays = append(delays, d)
			return d
		},
	})
	assert.Equal(t, 0, r.ExitCode)
	assert.Equal(t, 3, r.Attempts)
	assert.Equal(t, []time.Duration{time.Millisecond, 2 * time.Millisecond}, delays)
}

func TestUnlimitedAttempts(t *testing.T) {
	f := filepath.Join(t.TempDir(), "n")
	script := fmt.Sprintf(`n=$(cat %[1]s 2>/dev/null || echo 0); n=$((n+1)); echo $n > %[1]s; [ $n -ge 25 ]`, f)
	r := runQuiet(t, sh(script), Options{Attempts: 0})
	assert.Equal(t, 0, r.ExitCode)
	assert.Equal(t, 25, r.Attempts)
}

func TestStdinPassthrough(t *testing.T) {
	var out bytes.Buffer
	r := runQuiet(t, []string{"wc", "-l"}, Options{Attempts: 1, Stdin: strings.NewReader("a\nb\nc\nd\ne\n"), Stdout: &out})
	assert.Equal(t, 0, r.ExitCode)
	assert.Equal(t, "5", strings.TrimSpace(out.String()))
}

// TestLargeOutputByteExact is the large-output gate: 50 MB through
// patience must arrive unchanged and the command must not be killed.
func TestLargeOutputByteExact(t *testing.T) {
	const n = 50 * 1024 * 1024
	h := sha256.New()
	var count int64
	w := io.MultiWriter(h, countWriter{&count})
	r := runQuiet(t, sh(fmt.Sprintf("head -c %d /dev/zero | tr '\\0' 'x'", n)), Options{
		Attempts: 1, Stdout: w,
		Success: regexp.MustCompile("never"), // force the scanner on
	})
	assert.Equal(t, 0, r.ExitCode, r.Reason)
	assert.Equal(t, int64(n), count)
	want := sha256.New()
	chunk := bytes.Repeat([]byte("x"), 1<<20)
	for i := 0; i < 50; i++ {
		want.Write(chunk)
	}
	assert.Equal(t, want.Sum(nil), h.Sum(nil))
}

type countWriter struct{ n *int64 }

func (c countWriter) Write(p []byte) (int, error) { *c.n += int64(len(p)); return len(p), nil }

func TestPatterns(t *testing.T) {
	re := regexp.MustCompile
	cases := []struct {
		name     string
		script   string
		o        Options
		wantCode int
		wantWhy  string
	}{
		{"success pattern beats exit code", "echo deployed; exit 1", Options{Success: re("deployed")}, 0, "success pattern matched"},
		{"failure pattern beats exit 0", "echo ERROR x; exit 0", Options{Failure: re("ERROR")}, ExitPatternErr, "failure pattern matched"},
		{"failure keeps nonzero code", "echo ERROR; exit 7", Options{Failure: re("ERROR")}, 7, "failure pattern matched"},
		{"failure beats success", "echo ok bad; exit 0", Options{Success: re("ok"), Failure: re("bad")}, 1, "failure pattern matched"},
		{"stderr is scanned", "echo ok >&2; exit 1", Options{Success: re("ok")}, 0, "success pattern matched"},
		{"match across writes", "printf o; sleep 0.02; printf 'k\\n'; exit 1", Options{Success: re("^ok$")}, 0, "success pattern matched"},
		{"trailing partial line", "printf 'done'; exit 1", Options{Success: re("done")}, 0, "success pattern matched"},
		{"no newline, very long line", "head -c 200000 /dev/zero | tr '\\0' a; printf NEEDLE; exit 1", Options{Success: re("NEEDLE")}, 0, "success pattern matched"},
		{"case sensitive by default", "echo Success; exit 1", Options{Success: re("success")}, 1, "exit code 1"},
		{"case insensitive flag", "echo Success; exit 1", Options{Success: re("(?i)success")}, 0, "success pattern matched"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.o.Attempts = 1
			r := runQuiet(t, sh(c.script), c.o)
			assert.Equal(t, c.wantCode, r.ExitCode)
			assert.Equal(t, c.wantWhy, r.Last.Reason)
		})
	}
}

func TestRetryOn(t *testing.T) {
	r := runQuiet(t, sh("exit 3"), Options{Attempts: 5, RetryOn: []int{1, 75}})
	assert.Equal(t, 1, r.Attempts)
	assert.Equal(t, 3, r.ExitCode)
	assert.Contains(t, r.Reason, "not in --retry-on")

	r = runQuiet(t, sh("exit 75"), Options{Attempts: 3, RetryOn: []int{1, 75}})
	assert.Equal(t, 3, r.Attempts)

	// Pattern failure with exit 0 counts as 1; timeout counts as 124.
	r = runQuiet(t, sh("echo bad"), Options{Attempts: 3, RetryOn: []int{1}, Failure: regexp.MustCompile("bad")})
	assert.Equal(t, 3, r.Attempts)
	r = runQuiet(t, sh("sleep 5"), Options{Attempts: 3, RetryOn: []int{124}, Timeout: 50 * time.Millisecond})
	assert.Equal(t, 3, r.Attempts)
	assert.Equal(t, ExitTimeout, r.ExitCode)
}

func TestExecErrorsNotRetried(t *testing.T) {
	r := runQuiet(t, []string{"/no/such/binary"}, Options{Attempts: 3})
	assert.Equal(t, ExitNotFound, r.ExitCode)
	assert.Equal(t, 1, r.Attempts)
	assert.Contains(t, r.Reason, "not found")

	f := filepath.Join(t.TempDir(), "noexec")
	require.NoError(t, os.WriteFile(f, []byte("#!/bin/sh\n"), 0o644))
	r = runQuiet(t, []string{f}, Options{Attempts: 3})
	assert.Equal(t, ExitNotExec, r.ExitCode)
	assert.Equal(t, 1, r.Attempts)
}

func TestTimeoutKillsProcessTree(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "late")
	// A grandchild sleeps; if it survives the timeout it writes the marker.
	script := fmt.Sprintf("sh -c 'sleep 0.6; touch %s' & wait", marker)
	start := time.Now()
	r := runQuiet(t, sh(script), Options{Attempts: 1, Timeout: 100 * time.Millisecond})
	assert.Equal(t, ExitTimeout, r.ExitCode)
	assert.True(t, r.Last.TimedOut)
	assert.Less(t, time.Since(start), time.Second, "timeout should not wait for the sleep")
	time.Sleep(800 * time.Millisecond)
	_, err := os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "grandchild survived the timeout")
}

func TestTimeoutEscalatesToKill(t *testing.T) {
	// A child ignoring SIGTERM must be SIGKILLed after killGrace.
	old := killGrace
	killGrace = 200 * time.Millisecond
	defer func() { killGrace = old }()
	start := time.Now()
	r := runQuiet(t, sh("trap '' TERM; sleep 30"), Options{Attempts: 1, Timeout: 50 * time.Millisecond})
	assert.Equal(t, ExitTimeout, r.ExitCode)
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestMaxTime(t *testing.T) {
	now := time.Unix(0, 0)
	clock := func() time.Time { return now }
	sleep := func(_ context.Context, d time.Duration) error { now = now.Add(d); return nil }
	r := runQuiet(t, sh("exit 1"), Options{
		Attempts: 0, MaxTime: 10 * time.Second,
		Next:  func(int) time.Duration { return 3 * time.Second },
		Now:   clock,
		Sleep: sleep,
	})
	// 0s run, +3, +3, +3 = 9s; the next sleep would reach 12 ≥ 10, so stop after attempt 4.
	assert.Equal(t, 4, r.Attempts)
	assert.Equal(t, 1, r.ExitCode)
	assert.Contains(t, r.Reason, "--max-time")
}

// TestSignalForwarded is the signal-forwarding gate: SIGTERM to patience must
// reach the child and leave nothing running.
func TestSignalForwarded(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			marker := filepath.Join(t.TempDir(), "survived")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan Result, 1)
			go func() {
				r, _ := Run(ctx, sh(fmt.Sprintf("sleep 0.8 && touch %s", marker)), Options{
					Attempts: 3, Stdout: io.Discard, Stderr: io.Discard, Stdin: strings.NewReader(""),
				})
				done <- r
			}()
			time.Sleep(200 * time.Millisecond)
			require.NoError(t, syscall.Kill(os.Getpid(), sig))
			select {
			case r := <-done:
				assert.Equal(t, 128+int(sig), r.ExitCode)
				assert.Equal(t, sig, r.Interrupted)
				assert.Equal(t, 1, r.Attempts, "must not retry after a signal")
			case <-time.After(3 * time.Second):
				t.Fatal("Run did not return after signal")
			}
			time.Sleep(1000 * time.Millisecond)
			_, err := os.Stat(marker)
			assert.True(t, os.IsNotExist(err), "child survived %s", sig)
		})
	}
}

func TestSignalDuringBackoffSleep(t *testing.T) {
	done := make(chan Result, 1)
	go func() {
		r, _ := Run(context.Background(), sh("exit 1"), Options{
			Attempts: 5, Next: func(int) time.Duration { return 10 * time.Second },
			Stdout: io.Discard, Stderr: io.Discard, Stdin: strings.NewReader(""),
		})
		done <- r
	}()
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGINT))
	select {
	case r := <-done:
		assert.Equal(t, 130, r.ExitCode)
		assert.Equal(t, 1, r.Attempts)
		assert.Less(t, time.Since(start), time.Second)
	case <-time.After(3 * time.Second):
		t.Fatal("sleep was not interrupted")
	}
}

func TestChildKilledBySignalIsFailure(t *testing.T) {
	r := runQuiet(t, sh("kill -USR1 $$"), Options{Attempts: 2})
	assert.Equal(t, 128+int(syscall.SIGUSR1), r.ExitCode)
	assert.Equal(t, 2, r.Attempts)
	assert.Contains(t, r.Last.Reason, "killed by")
}

func TestForegroundMode(t *testing.T) {
	// Child shares our process group; a timeout still ends the attempt.
	r := runQuiet(t, sh("sleep 5"), Options{Attempts: 1, Timeout: 50 * time.Millisecond, Foreground: true})
	assert.Equal(t, ExitTimeout, r.ExitCode)
	// Verify the child really is in our group by reading its pgid.
	var pg bytes.Buffer
	r = runQuiet(t, sh("ps -o pgid= -p $$"), Options{Attempts: 1, Foreground: true, Stdout: &pg})
	require.Equal(t, 0, r.ExitCode)
	got, err := strconv.Atoi(strings.TrimSpace(pg.String()))
	require.NoError(t, err)
	assert.Equal(t, syscall.Getpgrp(), got)
	pg.Reset()
	r = runQuiet(t, sh("ps -o pgid= -p $$"), Options{Attempts: 1, Stdout: &pg})
	require.Equal(t, 0, r.ExitCode)
	got, _ = strconv.Atoi(strings.TrimSpace(pg.String()))
	assert.NotEqual(t, syscall.Getpgrp(), got, "default mode must use its own group")
}

type recReporter struct{ events []string }

func (r *recReporter) AttemptStart(a, n int) {
	r.events = append(r.events, fmt.Sprintf("start %d/%d", a, n))
}
func (r *recReporter) AttemptEnd(a int, o Outcome, retrying bool, d time.Duration) {
	r.events = append(r.events, fmt.Sprintf("end %d %s retry=%v %v", a, o.Reason, retrying, d))
}
func (r *recReporter) Done(res Result) { r.events = append(r.events, "done "+res.Reason) }

func TestReporterSequence(t *testing.T) {
	rep := &recReporter{}
	runQuiet(t, sh("exit 2"), Options{Attempts: 2, Report: rep, Next: func(int) time.Duration { return time.Second }})
	assert.Equal(t, []string{
		"start 1/2", "end 1 exit code 2 retry=true 1s",
		"start 2/2", "end 2 exit code 2 retry=false 0s",
		"done all 2 attempts failed (exit code 2)",
	}, rep.events)
}

func TestExitStatusMapping(t *testing.T) {
	code, sig := exitStatus(nil)
	assert.Equal(t, 0, code)
	assert.Nil(t, sig)
	err := exec.Command("sh", "-c", "exit 9").Run()
	code, sig = exitStatus(err)
	assert.Equal(t, 9, code)
	assert.Nil(t, sig)
	code, sig = exitStatus(fmt.Errorf("io"))
	assert.Equal(t, 1, code)
	assert.Nil(t, sig)
}

func TestSecondSignalKills(t *testing.T) {
	// Child ignores SIGTERM; the first signal is forwarded and ignored, the
	// second must SIGKILL it.
	done := make(chan Result, 1)
	go func() {
		r, _ := Run(context.Background(), sh("trap '' TERM; sleep 30"), Options{
			Attempts: 1, Stdout: io.Discard, Stderr: io.Discard, Stdin: strings.NewReader(""),
		})
		done <- r
	}()
	time.Sleep(200 * time.Millisecond)
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))
	time.Sleep(200 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("returned after one ignored SIGTERM")
	default:
	}
	require.NoError(t, syscall.Kill(os.Getpid(), syscall.SIGTERM))
	select {
	case r := <-done:
		assert.Equal(t, 143, r.ExitCode)
	case <-time.After(3 * time.Second):
		t.Fatal("second signal did not kill the child")
	}
}

func TestContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Result, 1)
	go func() {
		r, _ := Run(ctx, sh("sleep 30"), Options{Attempts: 3, Stdout: io.Discard, Stderr: io.Discard, Stdin: strings.NewReader("")})
		done <- r
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case r := <-done:
		assert.Equal(t, "cancelled", r.Last.Reason)
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not stop the run")
	}
}

func TestForegroundChildInterrupted(t *testing.T) {
	// In foreground mode the terminal would deliver SIGINT to the child; here
	// the child interrupts itself, which must stop the run with 130.
	r := runQuiet(t, sh("kill -INT $$"), Options{Attempts: 3, Foreground: true})
	assert.Equal(t, 130, r.ExitCode)
	assert.Equal(t, 1, r.Attempts)
	assert.Equal(t, syscall.SIGINT, r.Interrupted)
}

func TestSleepCtxCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Error(t, sleepCtx(ctx, time.Minute))
	assert.NoError(t, sleepCtx(context.Background(), time.Millisecond))
}

func TestExitForSignalNonSyscall(t *testing.T) {
	assert.Equal(t, 130, exitForSignal(os.Interrupt))
}
