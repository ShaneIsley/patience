// Package backoff computes the delay before each retry.
//
// All arithmetic is done in float64 seconds and saturated at MaxDelay (or at
// the largest time.Duration when there is no cap), so no parameter combination
// can overflow to a negative or zero delay.
package backoff

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"
	"time"
)

// Kind names a schedule.
type Kind string

// The schedules; see README "Backoff schedules" for the formulas.
const (
	Constant     Kind = "constant"     // d
	Linear       Kind = "linear"       // d·k
	Exponential  Kind = "exponential"  // d·x^(k-1)
	Decorrelated Kind = "decorrelated" // U[d, min(cap, prev·x)]  (AWS)
)

// Kinds lists the schedules in the order shown in help text.
var Kinds = []Kind{Constant, Linear, Exponential, Decorrelated}

// ParseKind accepts a schedule name, case-insensitively.
func ParseKind(s string) (Kind, error) {
	for _, k := range Kinds {
		if strings.EqualFold(s, string(k)) {
			return k, nil
		}
	}
	return "", fmt.Errorf("unknown backoff %q (want one of %s)", s, joinKinds())
}

func joinKinds() string {
	names := make([]string, len(Kinds))
	for i, k := range Kinds {
		names[i] = string(k)
	}
	return strings.Join(names, ", ")
}

// Params configures a schedule. Zero MaxDelay means no cap.
type Params struct {
	Kind       Kind
	Delay      time.Duration // base delay (constant, exponential, decorrelated) or increment (linear)
	MaxDelay   time.Duration // cap on any single delay; 0 = none
	Multiplier float64       // growth factor for exponential and decorrelated
	Jitter     float64       // 0..1; delay = v·(1−J) + U[0, J·v]; ignored by decorrelated
}

// Validate reports the first invalid parameter.
func (p Params) Validate() error {
	if _, err := ParseKind(string(p.Kind)); err != nil {
		return err
	}
	if p.Delay < 0 {
		return fmt.Errorf("delay must not be negative, got %v", p.Delay)
	}
	if p.MaxDelay < 0 {
		return fmt.Errorf("max-delay must not be negative, got %v", p.MaxDelay)
	}
	if math.IsNaN(p.Multiplier) || math.IsInf(p.Multiplier, 0) || p.Multiplier <= 0 {
		return fmt.Errorf("multiplier must be a positive number, got %v", p.Multiplier)
	}
	if math.IsNaN(p.Jitter) || p.Jitter < 0 || p.Jitter > 1 {
		return fmt.Errorf("jitter must be between 0 and 1, got %v", p.Jitter)
	}
	return nil
}

// Schedule produces successive delays. It is not safe for concurrent use.
type Schedule struct {
	p    Params
	rnd  func() float64 // uniform in [0,1)
	prev float64        // seconds; decorrelated state
}

// New returns a schedule. rnd may be nil, in which case math/rand/v2 is used.
// Params are assumed valid; call Validate first.
func New(p Params, rnd func() float64) *Schedule {
	if rnd == nil {
		rnd = rand.Float64
	}
	return &Schedule{p: p, rnd: rnd, prev: p.Delay.Seconds()}
}

// Next returns the delay to wait after failed attempt k (k ≥ 1).
// For k ≤ 0 it returns the base delay.
func (s *Schedule) Next(k int) time.Duration {
	if k < 1 {
		k = 1
	}
	d := s.p.Delay.Seconds()
	capSec := s.p.MaxDelay.Seconds()
	if s.p.MaxDelay == 0 {
		capSec = maxSeconds
	}

	var v float64
	switch s.p.Kind {
	case Constant:
		v = d
	case Linear:
		v = d * float64(k)
	case Exponential:
		v = d * math.Pow(s.p.Multiplier, float64(k-1))
	case Decorrelated:
		upper := math.Min(capSec, s.prev*s.p.Multiplier)
		if upper < d {
			upper = d
		}
		v = math.Min(d+s.rnd()*(upper-d), capSec)
		s.prev = v
		return toDuration(v)
	}
	v = math.Min(v, capSec)
	if s.p.Jitter > 0 {
		v = v*(1-s.p.Jitter) + s.rnd()*s.p.Jitter*v
	}
	return toDuration(v)
}

// maxSeconds is the largest whole number of seconds a time.Duration can hold.
const maxSeconds = float64(math.MaxInt64 / int64(time.Second))

func toDuration(sec float64) time.Duration {
	if math.IsNaN(sec) || sec <= 0 {
		return 0
	}
	if sec >= maxSeconds {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(sec * float64(time.Second))
}
