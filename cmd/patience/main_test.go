package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bin is built once per test binary into a temp dir (never into the tree).
var bin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "patience-test-")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(dir, "patience")
	args := []string{"build", "-ldflags", "-X main.version=test", "-o", bin}
	if os.Getenv("GOCOVERDIR") != "" {
		args = append(args, "-cover") // the binary then writes coverage for every run
	}
	build := exec.Command("go", append(args, ".")...)
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	code           int
	stdout, stderr string
	wall           time.Duration
}

func patience(t *testing.T, stdin string, env []string, args ...string) result {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "GOCOVERDIR=" + os.Getenv("GOCOVERDIR")}, env...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	start := time.Now()
	err := cmd.Run()
	code := 0
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v", err)
	}
	return result{code, out.String(), errb.String(), time.Since(start)}
}

func TestVersionAndHelp(t *testing.T) {
	r := patience(t, "", nil, "--version")
	assert.Equal(t, 0, r.code)
	assert.Equal(t, "patience test\n", r.stdout)

	r = patience(t, "", nil, "--help")
	assert.Equal(t, 0, r.code)
	assert.Contains(t, r.stdout, "Usage: patience [flags] -- COMMAND")
	assert.Contains(t, r.stdout, "--retry-on")
	assert.Empty(t, r.stderr)
}

func TestUsageErrors(t *testing.T) {
	cases := [][]string{
		{},                                  // no command
		{"--", ""},                          // empty command
		{"--bogus", "--", "true"},           // unknown flag
		{"-n", "-1", "--", "true"},          // negative attempts
		{"-d", "-1s", "--", "true"},         // negative delay
		{"-d", "5", "--", "true"},           // missing unit
		{"-x", "0", "--", "true"},           // zero multiplier
		{"-x", "nan", "--", "true"},         // NaN
		{"-j", "1.5", "--", "true"},         // jitter out of range
		{"-b", "fibonacci", "--", "true"},   // removed schedule
		{"--retry-on", "1,x", "--", "true"}, // bad code list
		{"--retry-on", "300", "--", "true"}, // out of range
		{"--success-pattern", "([", "--", "true"},
		{"-t", "-1s", "--", "true"},
		{"--max-time", "-1s", "--", "true"},
	}
	for _, args := range cases {
		r := patience(t, "", nil, args...)
		assert.Equal(t, 2, r.code, "args %q: stderr %q", args, r.stderr)
		assert.Contains(t, r.stderr, "patience:", "args %q", args)
		assert.Empty(t, r.stdout, "usage errors must not touch stdout")
	}
}

func TestPassthrough(t *testing.T) {
	r := patience(t, "x\ny\nz\n", nil, "-n", "1", "--", "sh", "-c", "wc -l; echo err >&2; exit 0")
	assert.Equal(t, 0, r.code)
	assert.Equal(t, "3", strings.TrimSpace(r.stdout))
	assert.Equal(t, "err\n", r.stderr, "first-try success is silent on stderr")
}

func TestExitCodes(t *testing.T) {
	r := patience(t, "", nil, "-n", "2", "-d", "0", "--", "sh", "-c", "exit 42")
	assert.Equal(t, 42, r.code)
	assert.Contains(t, r.stderr, "attempt 1 failed (exit code 42); retrying in 0s")
	assert.Contains(t, r.stderr, "giving up: all 2 attempts failed")

	r = patience(t, "", nil, "-n", "1", "-t", "100ms", "--", "sleep", "5")
	assert.Equal(t, 124, r.code)

	r = patience(t, "", nil, "--", "/no/such/cmd")
	assert.Equal(t, 127, r.code)
	assert.NotContains(t, r.stderr, "retrying")

	r = patience(t, "", nil, "-n", "3", "-d", "0", "--failure-pattern", "bad", "--", "sh", "-c", "echo bad")
	assert.Equal(t, 1, r.code)
}

func TestDelaysAreApplied(t *testing.T) {
	r := patience(t, "", nil, "-n", "4", "-b", "linear", "-d", "50ms", "--", "false")
	assert.Equal(t, 1, r.code)
	// 50 + 100 + 150 = 300ms of delay.
	assert.GreaterOrEqual(t, r.wall, 300*time.Millisecond)
	assert.Less(t, r.wall, 2*time.Second)
	assert.Contains(t, r.stderr, "retrying in 50ms")
	assert.Contains(t, r.stderr, "retrying in 100ms")
	assert.Contains(t, r.stderr, "retrying in 150ms")
}

func TestEnvDefaultsAndPrecedence(t *testing.T) {
	env := []string{"PATIENCE_ATTEMPTS=2", "PATIENCE_DELAY=0", "PATIENCE_BACKOFF=constant", "PATIENCE_QUIET=true"}
	r := patience(t, "", env, "--", "false")
	assert.Equal(t, 1, r.code)
	assert.Empty(t, r.stderr, "PATIENCE_QUIET should silence progress")

	// Flag beats env.
	r = patience(t, "", env, "-n", "3", "-v", "--", "false")
	assert.Equal(t, 3, strings.Count(r.stderr, "starting"))

	// Malformed env is a usage error, not silently ignored.
	r = patience(t, "", []string{"PATIENCE_ATTEMPTS=lots"}, "--", "true")
	assert.Equal(t, 2, r.code)
	assert.Contains(t, r.stderr, "PATIENCE_ATTEMPTS")
}

func TestQuietAndVerbose(t *testing.T) {
	r := patience(t, "", nil, "-q", "-n", "2", "-d", "0", "--", "false")
	assert.Empty(t, r.stderr)
	r = patience(t, "", nil, "-v", "-n", "1", "--", "true")
	assert.Contains(t, r.stderr, "attempt 1/1 starting")
	assert.Contains(t, r.stderr, "1 attempt(s) in")
}

func TestRetryOnFlag(t *testing.T) {
	r := patience(t, "", nil, "-n", "5", "-d", "0", "--retry-on", "1,75", "--", "sh", "-c", "exit 3")
	assert.Equal(t, 3, r.code)
	assert.Contains(t, r.stderr, "not in --retry-on")
	assert.NotContains(t, r.stderr, "retrying")
}

func TestIgnoreCase(t *testing.T) {
	r := patience(t, "", nil, "-n", "1", "-i", "--success-pattern", "DONE", "--", "sh", "-c", "echo done; exit 1")
	assert.Equal(t, 0, r.code)
}

func TestCommandFlagsNotParsed(t *testing.T) {
	// Flags after -- belong to the command.
	r := patience(t, "", nil, "--", "sh", "-c", "echo --version -n")
	assert.Equal(t, 0, r.code)
	assert.Equal(t, "--version -n\n", r.stdout)
}

func TestNoDashSeparator(t *testing.T) {
	// Without --, flags stop at the first non-flag argument.
	r := patience(t, "", nil, "-n", "1", "echo", "hi")
	assert.Equal(t, 0, r.code)
	assert.Equal(t, "hi\n", r.stdout)
}

func TestParseCodes(t *testing.T) {
	got, err := parseCodes(" 1, 75 ,124")
	require.NoError(t, err)
	assert.Equal(t, []int{1, 75, 124}, got)
	_, err = parseCodes(",")
	assert.Error(t, err)
}

// TestHelpMatchesReadme fails when a flag exists in --help but not in the
// README's flag block, or the other way round.
func TestHelpMatchesReadme(t *testing.T) {
	help := patience(t, "", nil, "--help").stdout
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	require.NoError(t, err)
	flagRe := regexp.MustCompile(`--[a-z][a-z-]+`)
	seen := func(s string) map[string]bool {
		m := map[string]bool{}
		for _, f := range flagRe.FindAllString(s, -1) {
			m[f] = true
		}
		return m
	}
	inHelp := seen(help)
	start := strings.Index(string(readme), "### Flags")
	end := strings.Index(string(readme)[start:], "Durations take")
	inReadme := seen(string(readme)[start : start+end])
	for f := range inHelp {
		assert.True(t, inReadme[f], "%s is in --help but not in README flags", f)
	}
	for f := range inReadme {
		assert.True(t, inHelp[f], "%s is in README flags but not in --help", f)
	}
}
