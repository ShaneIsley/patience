package backoff

import (
	"math"
	"math/rand"
	"time"
)

// Strategy defines the interface for backoff strategies
type Strategy interface {
	// Delay returns the duration to wait before the next attempt
	// attempt is 1-based (1 for first retry, 2 for second retry, etc.)
	Delay(attempt int) time.Duration
}

// Fixed implements a fixed delay strategy
type Fixed struct {
	Duration time.Duration
}

// NewFixed creates a new Fixed backoff strategy
func NewFixed(duration time.Duration) *Fixed {
	return &Fixed{
		Duration: duration,
	}
}

// Delay returns the fixed duration for any attempt
func (f *Fixed) Delay(attempt int) time.Duration {
	return f.Duration
}

// Exponential implements an exponential backoff strategy
type Exponential struct {
	BaseDelay  time.Duration
	Multiplier float64
	MaxDelay   time.Duration
}

// NewExponential creates a new Exponential backoff strategy
// baseDelay is the initial delay, multiplier is the factor to increase by each attempt
// maxDelay is the maximum delay (0 means no limit)
func NewExponential(baseDelay time.Duration, multiplier float64, maxDelay time.Duration) *Exponential {
	return &Exponential{
		BaseDelay:  baseDelay,
		Multiplier: multiplier,
		MaxDelay:   maxDelay,
	}
}

// Delay returns the exponentially increasing delay for the given attempt
func (e *Exponential) Delay(attempt int) time.Duration {
	if attempt <= 0 {
		return e.BaseDelay
	}

	// Calculate exponential delay: baseDelay * multiplier^(attempt-1)
	delay := float64(e.BaseDelay) * math.Pow(e.Multiplier, float64(attempt-1))

	// Convert back to duration
	result := time.Duration(delay)

	// Apply max delay cap if set
	if e.MaxDelay > 0 && result > e.MaxDelay {
		result = e.MaxDelay
	}

	return result
}

// Jitter implements a jitter backoff strategy that adds randomness to exponential backoff
type Jitter struct {
	BaseDelay  time.Duration
	Multiplier float64
	MaxDelay   time.Duration
}

// NewJitter creates a new Jitter backoff strategy
// baseDelay is the initial delay, multiplier is the factor to increase by each attempt
// maxDelay is the maximum delay (0 means no limit)
func NewJitter(baseDelay time.Duration, multiplier float64, maxDelay time.Duration) *Jitter {
	return &Jitter{
		BaseDelay:  baseDelay,
		Multiplier: multiplier,
		MaxDelay:   maxDelay,
	}
}

// Delay returns a random delay between 0 and the exponential delay for the given attempt
func (j *Jitter) Delay(attempt int) time.Duration {
	if attempt <= 0 {
		// For invalid attempts, return random delay between 0 and base delay
		return time.Duration(rand.Float64() * float64(j.BaseDelay))
	}

	// Calculate exponential delay: baseDelay * multiplier^(attempt-1)
	exponentialDelay := float64(j.BaseDelay) * math.Pow(j.Multiplier, float64(attempt-1))

	// Apply max delay cap if set
	if j.MaxDelay > 0 && time.Duration(exponentialDelay) > j.MaxDelay {
		exponentialDelay = float64(j.MaxDelay)
	}

	// Return random delay between 0 and exponential delay (full jitter)
	return time.Duration(rand.Float64() * exponentialDelay)
}

// Linear implements a linear backoff strategy with predictable incremental delays
type Linear struct {
	Increment time.Duration
	MaxDelay  time.Duration
}

// NewLinear creates a new Linear backoff strategy
// increment is the amount to increase delay by each attempt
// maxDelay is the maximum delay (0 means no limit)
func NewLinear(increment time.Duration, maxDelay time.Duration) *Linear {
	return &Linear{
		Increment: increment,
		MaxDelay:  maxDelay,
	}
}

// Delay returns the linearly increasing delay for the given attempt
func (l *Linear) Delay(attempt int) time.Duration {
	if attempt <= 0 {
		return l.Increment
	}

	// Calculate linear delay: increment * attempt
	delay := time.Duration(attempt) * l.Increment

	// Apply max delay cap if set
	if l.MaxDelay > 0 && delay > l.MaxDelay {
		delay = l.MaxDelay
	}

	return delay
}

// DecorrelatedJitter implements the AWS-recommended decorrelated jitter strategy
// that uses the previous delay to calculate the next delay, creating better distribution
type DecorrelatedJitter struct {
	BaseDelay     time.Duration
	Multiplier    float64
	MaxDelay      time.Duration
	previousDelay time.Duration
}

// NewDecorrelatedJitter creates a new DecorrelatedJitter backoff strategy
// baseDelay is the initial delay, multiplier is the factor for the upper bound
// maxDelay is the maximum delay (0 means no limit)
func NewDecorrelatedJitter(baseDelay time.Duration, multiplier float64, maxDelay time.Duration) *DecorrelatedJitter {
	return &DecorrelatedJitter{
		BaseDelay:     baseDelay,
		Multiplier:    multiplier,
		MaxDelay:      maxDelay,
		previousDelay: 0, // No previous delay initially
	}
}

// Delay returns a decorrelated jitter delay based on the previous delay
// Formula: random_between(base_delay, previous_delay * multiplier)
func (d *DecorrelatedJitter) Delay(attempt int) time.Duration {
	var upperBound time.Duration

	if attempt <= 0 || d.previousDelay == 0 {
		// For first attempt or invalid attempts, use base delay * multiplier as upper bound
		upperBound = time.Duration(float64(d.BaseDelay) * d.Multiplier)
	} else {
		// For subsequent attempts, use previous delay * multiplier as upper bound
		upperBound = time.Duration(float64(d.previousDelay) * d.Multiplier)
	}

	// Apply max delay cap if set
	if d.MaxDelay > 0 && upperBound > d.MaxDelay {
		upperBound = d.MaxDelay
	}

	// Ensure upper bound is at least base delay
	if upperBound < d.BaseDelay {
		upperBound = d.BaseDelay
	}

	// Calculate random delay between base delay and upper bound
	delayRange := upperBound - d.BaseDelay
	randomDelay := d.BaseDelay + time.Duration(rand.Float64()*float64(delayRange))

	// Store this delay as the previous delay for next calculation
	d.previousDelay = randomDelay

	return randomDelay
}

// Fibonacci implements a fibonacci backoff strategy that follows the fibonacci sequence
// for delay calculation, providing a middle ground between linear and exponential growth
type Fibonacci struct {
	BaseDelay time.Duration
	MaxDelay  time.Duration
}

// NewFibonacci creates a new Fibonacci backoff strategy
// baseDelay is the unit delay (multiplied by fibonacci numbers)
// maxDelay is the maximum delay (0 means no limit)
func NewFibonacci(baseDelay time.Duration, maxDelay time.Duration) *Fibonacci {
	return &Fibonacci{
		BaseDelay: baseDelay,
		MaxDelay:  maxDelay,
	}
}

// Delay returns the fibonacci-based delay for the given attempt
// Uses fibonacci sequence: 1, 1, 2, 3, 5, 8, 13, 21, 34, 55, ...
func (f *Fibonacci) Delay(attempt int) time.Duration {
	if attempt <= 0 {
		return f.BaseDelay
	}

	// Calculate fibonacci number for the attempt
	fibNumber := fibonacci(attempt)

	// Calculate delay: baseDelay * fibonacci(attempt)
	delay := time.Duration(fibNumber) * f.BaseDelay

	// Apply max delay cap if set
	if f.MaxDelay > 0 && delay > f.MaxDelay {
		delay = f.MaxDelay
	}

	return delay
}

// fibonacci calculates the nth fibonacci number (1-based)
// Returns: 1, 1, 2, 3, 5, 8, 13, 21, 34, 55, ...
// Optimized iterative implementation with O(n) time complexity
func fibonacci(n int) int {
	if n <= 2 {
		return 1
	}

	a, b := 1, 1
	for i := 3; i <= n; i++ {
		a, b = b, a+b
	}
	return b
}
