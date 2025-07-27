package backoff

import (
	"fmt"
	"math"
	"time"
)

// PolynomialStrategy implements polynomial backoff: delay = base_delay * (attempt ^ exponent)
type PolynomialStrategy struct {
	baseDelay time.Duration
	exponent  float64
	maxDelay  time.Duration
}

// NewPolynomial creates a new polynomial backoff strategy
func NewPolynomial(baseDelay time.Duration, exponent float64, maxDelay time.Duration) (*PolynomialStrategy, error) {
	if baseDelay <= 0 {
		return nil, fmt.Errorf("base delay must be positive, got %v", baseDelay)
	}
	if exponent < 0 {
		return nil, fmt.Errorf("exponent must be non-negative, got %f", exponent)
	}
	if maxDelay <= 0 {
		return nil, fmt.Errorf("max delay must be positive, got %v", maxDelay)
	}
	if baseDelay > maxDelay {
		return nil, fmt.Errorf("base delay (%v) cannot be greater than max delay (%v)", baseDelay, maxDelay)
	}

	return &PolynomialStrategy{
		baseDelay: baseDelay,
		exponent:  exponent,
		maxDelay:  maxDelay,
	}, nil
}

// Delay calculates the delay for the given attempt using polynomial growth
func (p *PolynomialStrategy) Delay(attempt int) time.Duration {
	if attempt <= 0 {
		return p.baseDelay
	}

	// Calculate: base_delay * (attempt ^ exponent)
	multiplier := math.Pow(float64(attempt), p.exponent)
	delay := float64(p.baseDelay) * multiplier

	// Apply max delay cap
	if delay > float64(p.maxDelay) {
		return p.maxDelay
	}

	return time.Duration(delay)
}

// GetBaseDelay returns the base delay
func (p *PolynomialStrategy) GetBaseDelay() time.Duration {
	return p.baseDelay
}

// GetExponent returns the exponent
func (p *PolynomialStrategy) GetExponent() float64 {
	return p.exponent
}

// GetMaxDelay returns the maximum delay
func (p *PolynomialStrategy) GetMaxDelay() time.Duration {
	return p.maxDelay
}

// String returns a string representation of the strategy
func (p *PolynomialStrategy) String() string {
	return fmt.Sprintf("polynomial(base=%v, exponent=%.1f, max=%v)",
		p.baseDelay, p.exponent, p.maxDelay)
}
