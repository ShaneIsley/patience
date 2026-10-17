package backoff

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixed(v float64) func() float64 { return func() float64 { return v } }

func TestParseKind(t *testing.T) {
	for _, k := range Kinds {
		got, err := ParseKind(string(k))
		require.NoError(t, err)
		assert.Equal(t, k, got)
	}
	got, err := ParseKind("EXPONENTIAL")
	require.NoError(t, err)
	assert.Equal(t, Exponential, got)
	_, err = ParseKind("fibonacci")
	assert.ErrorContains(t, err, `unknown backoff "fibonacci"`)
	assert.ErrorContains(t, err, "constant, linear, exponential, decorrelated")
}

func TestValidate(t *testing.T) {
	ok := Params{Kind: Exponential, Delay: time.Second, MaxDelay: time.Minute, Multiplier: 2}
	require.NoError(t, ok.Validate())

	cases := map[string]Params{
		"bad kind":       {Kind: "poly", Delay: time.Second, Multiplier: 2},
		"negative delay": {Kind: Constant, Delay: -time.Second, Multiplier: 2},
		"negative cap":   {Kind: Constant, Delay: time.Second, MaxDelay: -1, Multiplier: 2},
		"zero mult":      {Kind: Exponential, Delay: time.Second, Multiplier: 0},
		"negative mult":  {Kind: Exponential, Delay: time.Second, Multiplier: -2},
		"nan mult":       {Kind: Exponential, Delay: time.Second, Multiplier: math.NaN()},
		"inf mult":       {Kind: Exponential, Delay: time.Second, Multiplier: math.Inf(1)},
		"jitter > 1":     {Kind: Exponential, Delay: time.Second, Multiplier: 2, Jitter: 1.5},
		"jitter < 0":     {Kind: Exponential, Delay: time.Second, Multiplier: 2, Jitter: -0.1},
		"jitter nan":     {Kind: Exponential, Delay: time.Second, Multiplier: 2, Jitter: math.NaN()},
	}
	for name, p := range cases {
		t.Run(name, func(t *testing.T) { assert.Error(t, p.Validate()) })
	}
}

func TestSequences(t *testing.T) {
	s := func(ds ...float64) []time.Duration {
		out := make([]time.Duration, len(ds))
		for i, d := range ds {
			out[i] = time.Duration(d * float64(time.Second))
		}
		return out
	}
	cases := []struct {
		name string
		p    Params
		want []time.Duration
	}{
		{"constant", Params{Kind: Constant, Delay: time.Second, Multiplier: 2}, s(1, 1, 1, 1, 1)},
		{"linear", Params{Kind: Linear, Delay: time.Second, Multiplier: 2}, s(1, 2, 3, 4, 5)},
		{"linear capped", Params{Kind: Linear, Delay: time.Second, MaxDelay: 3 * time.Second, Multiplier: 2}, s(1, 2, 3, 3, 3)},
		{"exponential", Params{Kind: Exponential, Delay: time.Second, Multiplier: 2}, s(1, 2, 4, 8, 16)},
		{"exponential x1.5", Params{Kind: Exponential, Delay: time.Second, Multiplier: 1.5}, s(1, 1.5, 2.25, 3.375, 5.0625)},
		{"exponential capped", Params{Kind: Exponential, Delay: time.Second, MaxDelay: 5 * time.Second, Multiplier: 2}, s(1, 2, 4, 5, 5)},
		{"exponential shrinking", Params{Kind: Exponential, Delay: 8 * time.Second, Multiplier: 0.5}, s(8, 4, 2, 1, 0.5)},
		{"zero base", Params{Kind: Exponential, Delay: 0, Multiplier: 2}, s(0, 0, 0, 0, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sch := New(c.p, nil)
			for k := 1; k <= len(c.want); k++ {
				assert.Equal(t, c.want[k-1], sch.Next(k), "attempt %d", k)
			}
		})
	}
}

func TestNextBelowOne(t *testing.T) {
	sch := New(Params{Kind: Linear, Delay: time.Second, Multiplier: 2}, nil)
	assert.Equal(t, time.Second, sch.Next(0))
	assert.Equal(t, time.Second, sch.Next(-5))
}

func TestJitterEndpoints(t *testing.T) {
	base := Params{Kind: Exponential, Delay: time.Second, Multiplier: 2}
	// Attempt 3 → v = 4s.
	for _, c := range []struct {
		jitter, rnd float64
		want        time.Duration
	}{
		{1, 0, 0}, // full jitter, low end
		{1, 0.999999, 3999996 * time.Microsecond}, // full jitter, high end
		{0.5, 0, 2 * time.Second},                 // equal jitter floor
		{0.5, 0.5, 3 * time.Second},               // equal jitter middle
		{0.2, 0, 3200 * time.Millisecond},         // 20% wobble floor
		{0, 0.7, 4 * time.Second},                 // no jitter ignores rnd
	} {
		p := base
		p.Jitter = c.jitter
		got := New(p, fixed(c.rnd)).Next(3)
		assert.InDelta(t, float64(c.want), float64(got), float64(time.Microsecond), "jitter=%v rnd=%v", c.jitter, c.rnd)
	}
}

func TestDecorrelated(t *testing.T) {
	// AWS reference: sleep = min(cap, random_between(base, sleep * 3)), sleep starts at base.
	p := Params{Kind: Decorrelated, Delay: time.Second, MaxDelay: 20 * time.Second, Multiplier: 3}
	// rnd = 1 (just under) → always the upper bound: 3, 9, 20, 20.
	sch := New(p, fixed(0.9999999))
	for i, want := range []float64{3, 9, 20, 20} {
		got := sch.Next(i + 1)
		assert.InDelta(t, want, got.Seconds(), 1e-5, "attempt %d", i+1)
	}
	// rnd = 0 → always the base.
	sch = New(p, fixed(0))
	for k := 1; k <= 4; k++ {
		assert.Equal(t, time.Second, sch.Next(k))
	}
	// Jitter is ignored for decorrelated.
	p.Jitter = 1
	assert.Equal(t, time.Second, New(p, fixed(0)).Next(1))
	// Multiplier < 1 never drops below base.
	p.Multiplier = 0.5
	assert.Equal(t, time.Second, New(p, fixed(0.5)).Next(5))
}

// TestBounds is the property test from PLAN.md step 3: for every schedule and
// extreme parameters, v·(1−J) ≤ delay ≤ cap and the result is never negative.
func TestBounds(t *testing.T) {
	delays := []time.Duration{0, time.Nanosecond, time.Millisecond, time.Second, 100 * 365 * 24 * time.Hour}
	caps := []time.Duration{0, time.Nanosecond, time.Minute, 100 * 365 * 24 * time.Hour}
	mults := []float64{0.1, 1, 2, 3, 1e6}
	jitters := []float64{0, 0.5, 1}
	rng := rand.New(rand.NewPCG(1, 2))
	for _, kind := range Kinds {
		for _, d := range delays {
			for _, c := range caps {
				for _, m := range mults {
					for _, j := range jitters {
						p := Params{Kind: kind, Delay: d, MaxDelay: c, Multiplier: m, Jitter: j}
						require.NoError(t, p.Validate())
						sch := New(p, rng.Float64)
						for k := 1; k <= 10000; k *= 10 {
							got := sch.Next(k)
							if got < 0 {
								t.Fatalf("%+v attempt %d: negative delay %v", p, k, got)
							}
							if c > 0 && got > c {
								t.Fatalf("%+v attempt %d: %v exceeds cap %v", p, k, got, c)
							}
							if kind != Decorrelated && c == 0 && m >= 1 {
								// Uncapped, non-shrinking: delay never below v·(1−J) where v ≥ d.
								floor := time.Duration(float64(d) * (1 - j))
								if got < floor-time.Nanosecond {
									t.Fatalf("%+v attempt %d: %v below floor %v", p, k, got, floor)
								}
							}
						}
					}
				}
			}
		}
	}
}

func TestSaturation(t *testing.T) {
	// 1s · 10^100 must saturate, not overflow.
	p := Params{Kind: Exponential, Delay: time.Second, Multiplier: 10}
	assert.Equal(t, time.Duration(math.MaxInt64), New(p, nil).Next(101))
	// Linear with a huge increment likewise.
	p = Params{Kind: Linear, Delay: 100 * 365 * 24 * time.Hour, Multiplier: 2}
	assert.Equal(t, time.Duration(math.MaxInt64), New(p, nil).Next(1000))
	// And the cap still wins when present.
	p.MaxDelay = time.Minute
	assert.Equal(t, time.Minute, New(p, nil).Next(1000))
}

// TestFullJitterMean samples full jitter and checks the mean is v/2 within 1%
// (PLAN.md step 3 gate).
func TestFullJitterMean(t *testing.T) {
	const n = 100000
	p := Params{Kind: Constant, Delay: 2 * time.Second, Multiplier: 2, Jitter: 1}
	rng := rand.New(rand.NewPCG(7, 11))
	sch := New(p, rng.Float64)
	var sum float64
	var minD, maxD time.Duration = time.Hour, 0
	for i := 0; i < n; i++ {
		d := sch.Next(1)
		sum += d.Seconds()
		minD, maxD = min(minD, d), max(maxD, d)
	}
	mean := sum / n
	assert.InDelta(t, 1.0, mean, 0.01, "mean of U[0,2s] should be 1s")
	assert.Less(t, minD, 10*time.Millisecond)
	assert.Greater(t, maxD, 1990*time.Millisecond)
}
