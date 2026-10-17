// Command patience runs a command and retries it on failure with a chosen backoff.
//
//	patience [flags] -- COMMAND [ARGS...]
package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"

	"github.com/shaneisley/patience/internal/backoff"
	"github.com/shaneisley/patience/internal/run"
	"github.com/shaneisley/patience/internal/ui"
)

// version is set with -ldflags "-X main.version=v1.2.3".
var version = "dev"

const envPrefix = "PATIENCE_"

// exitUsage is the exit code for bad flags or no command, as in the shell.
const exitUsage = 2

func main() {
	os.Exit(realMain(os.Args[1:], os.Stdout, os.Stderr))
}

type cli struct {
	attempts   int
	timeout    time.Duration
	maxTime    time.Duration
	retryOn    string
	backoff    string
	delay      time.Duration
	maxDelay   time.Duration
	multiplier float64
	jitter     float64
	success    string
	failure    string
	ignoreCase bool
	foreground bool
	quiet      bool
	verbose    bool
	version    bool
}

func newFlags(c *cli) *pflag.FlagSet {
	f := pflag.NewFlagSet("patience", pflag.ContinueOnError)
	f.SortFlags = false
	f.IntVarP(&c.attempts, "attempts", "n", 3, "total attempts; 0 = unlimited")
	f.DurationVarP(&c.timeout, "timeout", "t", 0, "per-attempt timeout; 0 = none")
	f.DurationVar(&c.maxTime, "max-time", 0, "overall budget including delays; 0 = none")
	f.StringVar(&c.retryOn, "retry-on", "", "retry only on these exit codes, e.g. 1,75 (default: any non-zero)")
	f.StringVarP(&c.backoff, "backoff", "b", "exponential", "constant | linear | exponential | decorrelated")
	f.DurationVarP(&c.delay, "delay", "d", time.Second, "base delay; increment for linear")
	f.DurationVarP(&c.maxDelay, "max-delay", "m", time.Minute, "cap on any single delay; 0 = none")
	f.Float64VarP(&c.multiplier, "multiplier", "x", 2, "growth factor for exponential and decorrelated")
	f.Float64VarP(&c.jitter, "jitter", "j", 0, "0..1: randomise each delay downward by up to this fraction; 1 = full jitter")
	f.StringVar(&c.success, "success-pattern", "", "regexp: attempt succeeds if output matches (overrides exit code)")
	f.StringVar(&c.failure, "failure-pattern", "", "regexp: attempt fails if output matches (overrides success pattern)")
	f.BoolVarP(&c.ignoreCase, "ignore-case", "i", false, "patterns are case-insensitive")
	f.BoolVar(&c.foreground, "foreground", false, "run the command in patience's own process group (terminal use; a timeout then kills only the direct child)")
	f.BoolVarP(&c.quiet, "quiet", "q", false, "no progress on stderr")
	f.BoolVarP(&c.verbose, "verbose", "v", false, "report every attempt and a final summary")
	f.BoolVar(&c.version, "version", false, "print version and exit")
	return f
}

func usage(f *pflag.FlagSet) string {
	var b strings.Builder
	b.WriteString("Usage: patience [flags] -- COMMAND [ARGS...]\n\n")
	b.WriteString("Run COMMAND, retrying on failure with a backoff delay between attempts.\n")
	b.WriteString("Every flag can also be set with an environment variable PATIENCE_<FLAG>,\n")
	b.WriteString("e.g. PATIENCE_ATTEMPTS=5 PATIENCE_BACKOFF=linear; flags win.\n\n")
	b.WriteString("Flags:\n")
	b.WriteString(f.FlagUsages())
	b.WriteString("\nExit status: the command's exit code on failure; 124 if the last attempt timed out;\n")
	b.WriteString("126/127 if it could not be run (never retried); 2 for usage errors; 128+N on signal N.\n")
	return b.String()
}

func realMain(args []string, stdout, stderr *os.File) int {
	var c cli
	f := newFlags(&c)
	f.Usage = func() {} // we print usage ourselves
	f.SetOutput(stderr)

	// Environment defaults, applied before parsing so flags override them.
	if err := applyEnv(f); err != nil {
		fmt.Fprintf(stderr, "patience: %v\n", err)
		return exitUsage
	}

	if err := f.Parse(args); err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			fmt.Fprint(stdout, usage(f))
			return 0
		}
		fmt.Fprintf(stderr, "patience: %v\n%s", err, usage(f))
		return exitUsage
	}
	if c.version {
		fmt.Fprintf(stdout, "patience %s\n", version)
		return 0
	}
	cmd := f.Args()
	if f.ArgsLenAtDash() >= 0 {
		cmd = cmd[f.ArgsLenAtDash():]
	}
	if len(cmd) == 0 || cmd[0] == "" {
		fmt.Fprintf(stderr, "patience: no command given\n%s", usage(f))
		return exitUsage
	}

	opts, err := c.options()
	if err != nil {
		fmt.Fprintf(stderr, "patience: %v\n", err)
		return exitUsage
	}
	opts.Stdin = os.Stdin
	opts.Stdout = stdout
	opts.Stderr = stderr
	level := ui.Normal
	if c.quiet {
		level = ui.Quiet
	}
	if c.verbose {
		level = ui.Verbose
	}
	opts.Report = ui.Reporter{W: stderr, Level: level}

	res, err := run.Run(context.Background(), cmd, opts)
	if err != nil {
		fmt.Fprintf(stderr, "patience: %v\n", err)
		return exitUsage
	}
	return res.ExitCode
}

// applyEnv sets flag defaults from PATIENCE_* variables. Unknown variables are
// ignored; a malformed value is an error so it is not silently dropped.
func applyEnv(f *pflag.FlagSet) error {
	var err error
	f.VisitAll(func(fl *pflag.Flag) {
		if err != nil {
			return
		}
		name := envPrefix + strings.ToUpper(strings.ReplaceAll(fl.Name, "-", "_"))
		v, ok := os.LookupEnv(name)
		if !ok {
			return
		}
		if e := fl.Value.Set(v); e != nil {
			err = fmt.Errorf("%s=%q: %w", name, v, e)
		}
	})
	return err
}

func (c cli) options() (run.Options, error) {
	var o run.Options
	if c.attempts < 0 {
		return o, fmt.Errorf("--attempts must be 0 or more, got %d", c.attempts)
	}
	if c.timeout < 0 {
		return o, fmt.Errorf("--timeout must not be negative, got %v", c.timeout)
	}
	if c.maxTime < 0 {
		return o, fmt.Errorf("--max-time must not be negative, got %v", c.maxTime)
	}
	kind, err := backoff.ParseKind(c.backoff)
	if err != nil {
		return o, err
	}
	p := backoff.Params{Kind: kind, Delay: c.delay, MaxDelay: c.maxDelay, Multiplier: c.multiplier, Jitter: c.jitter}
	if err := p.Validate(); err != nil {
		return o, err
	}
	if math.IsNaN(c.jitter) {
		return o, fmt.Errorf("--jitter must be a number")
	}
	sched := backoff.New(p, nil)

	o.Attempts = c.attempts
	o.Timeout = c.timeout
	o.MaxTime = c.maxTime
	o.Next = sched.Next
	o.Foreground = c.foreground

	if c.retryOn != "" {
		codes, err := parseCodes(c.retryOn)
		if err != nil {
			return o, err
		}
		o.RetryOn = codes
	}
	flags := ""
	if c.ignoreCase {
		flags = "(?i)"
	}
	if c.success != "" {
		re, err := regexp.Compile(flags + c.success)
		if err != nil {
			return o, fmt.Errorf("--success-pattern: %w", err)
		}
		o.Success = re
	}
	if c.failure != "" {
		re, err := regexp.Compile(flags + c.failure)
		if err != nil {
			return o, fmt.Errorf("--failure-pattern: %w", err)
		}
		o.Failure = re
	}
	return o, nil
}

func parseCodes(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 || n > 255 {
			return nil, fmt.Errorf("--retry-on: %q is not an exit code (0-255)", part)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--retry-on: no exit codes given")
	}
	return out, nil
}
