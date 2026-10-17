package ui

import (
	"bytes"
	"testing"
	"time"

	"github.com/shaneisley/patience/internal/run"
	"github.com/stretchr/testify/assert"
)

func TestLevels(t *testing.T) {
	fail := run.Outcome{ExitCode: 1, Reason: "exit code 1"}
	ok := run.Outcome{Success: true, Reason: "exit code 0"}
	res := run.Result{Attempts: 2, ExitCode: 1, Reason: "all 2 attempts failed (exit code 1)", Elapsed: 1500 * time.Millisecond}

	var b bytes.Buffer
	q := Reporter{W: &b, Level: Quiet}
	q.AttemptStart(1, 2)
	q.AttemptEnd(1, fail, true, time.Second)
	q.Done(res)
	assert.Empty(t, b.String())

	b.Reset()
	n := Reporter{W: &b, Level: Normal}
	n.AttemptStart(1, 2)
	n.AttemptEnd(1, fail, true, 1500*time.Millisecond)
	n.AttemptEnd(2, fail, false, 0)
	n.Done(res)
	assert.Equal(t, "patience: attempt 1 failed (exit code 1); retrying in 1.5s\n"+
		"patience: attempt 2 failed (exit code 1)\n"+
		"patience: giving up: all 2 attempts failed (exit code 1)\n", b.String())

	b.Reset()
	n.AttemptEnd(1, ok, false, 0) // first-try success is silent at Normal
	n.Done(run.Result{Attempts: 1})
	assert.Empty(t, b.String())
	n.AttemptEnd(3, ok, false, 0) // later success is reported
	assert.Equal(t, "patience: attempt 3 succeeded (exit code 0)\n", b.String())

	b.Reset()
	v := Reporter{W: &b, Level: Verbose}
	v.AttemptStart(1, 0)
	v.AttemptEnd(1, ok, false, 0)
	v.Done(run.Result{Attempts: 1, Elapsed: 20 * time.Millisecond})
	assert.Equal(t, "patience: attempt 1 starting\n"+
		"patience: attempt 1 succeeded (exit code 0)\n"+
		"patience: 1 attempt(s) in 20ms, exit 0\n", b.String())
}

func TestFmtDur(t *testing.T) {
	assert.Equal(t, "0s", fmtDur(0))
	assert.Equal(t, "250ms", fmtDur(250*time.Millisecond))
	assert.Equal(t, "1.5s", fmtDur(1500*time.Millisecond))
	assert.Equal(t, "2m0s", fmtDur(2*time.Minute))
	assert.Equal(t, "3/5", counter(3, 5))
}
