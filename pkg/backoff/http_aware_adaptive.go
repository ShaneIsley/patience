package backoff

import (
	"math/rand"
	"sync"
	"time"

	"github.com/shaneisley/patience/pkg/patterns"
)

// HTTPAwareAdaptiveBackoff implements HTTP-aware backoff strategy selection
type HTTPAwareAdaptiveBackoff struct {
	httpSelector     *HTTPAwareBackoffSelector
	fallbackStrategy Strategy
	lastResponse     *patterns.HTTPResponse
	currentAttempt   int
	rng              *rand.Rand
	mu               sync.Mutex
}

// AdaptiveBackoffConfig represents configuration for adaptive backoff
type AdaptiveBackoffConfig struct {
	InitialDelay    time.Duration
	MaxDelay        time.Duration
	Multiplier      float64
	Jitter          bool
	MaxAttempts     int
	LearningEnabled bool
}

// NewHTTPAwareAdaptiveBackoff creates a new HTTP-aware adaptive backoff
func NewHTTPAwareAdaptiveBackoff(config AdaptiveBackoffConfig) *HTTPAwareAdaptiveBackoff {
	fallback := NewExponential(config.InitialDelay, config.Multiplier, config.MaxDelay)

	return &HTTPAwareAdaptiveBackoff{
		httpSelector:     NewHTTPAwareBackoffSelector(),
		fallbackStrategy: fallback,
		currentAttempt:   0,
		rng:              rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// DefaultAdaptiveBackoffConfig returns the default adaptive backoff configuration
func DefaultAdaptiveBackoffConfig() AdaptiveBackoffConfig {
	return AdaptiveBackoffConfig{
		InitialDelay:    1 * time.Second,
		MaxDelay:        30 * time.Second,
		Multiplier:      2.0,
		Jitter:          true,
		MaxAttempts:     5,
		LearningEnabled: true,
	}
}

// Delay implements the Strategy interface
func (h *HTTPAwareAdaptiveBackoff) Delay(attempt int) time.Duration {
	h.currentAttempt = attempt

	if h.lastResponse != nil {
		return h.NextDelayWithHTTPContext(h.lastResponse)
	}

	return h.fallbackStrategy.Delay(attempt)
}

// NextDelayWithHTTPContext calculates next delay considering HTTP response context
func (h *HTTPAwareAdaptiveBackoff) NextDelayWithHTTPContext(response *patterns.HTTPResponse) time.Duration {
	if response != nil {
		h.lastResponse = response

		// Get HTTP-aware strategy recommendation
		strategy, params, err := h.httpSelector.SelectStrategy(response)
		if err == nil {
			// Apply strategy-specific logic
			return h.applyHTTPAwareStrategy(strategy, params)
		}
	}

	// Fallback to standard adaptive backoff
	return h.fallbackStrategy.Delay(h.currentAttempt)
}

// applyHTTPAwareStrategy applies the selected HTTP-aware strategy
func (h *HTTPAwareAdaptiveBackoff) applyHTTPAwareStrategy(strategy string, params map[string]interface{}) time.Duration {
	switch strategy {
	case "diophantine":
		return h.applyDiophantineStrategy(params)
	case "polynomial":
		return h.applyPolynomialStrategy(params)
	case "exponential":
		return h.applyExponentialStrategy(params)
	case "fixed":
		return h.applyFixedStrategy(params)
	case "adaptive":
		return h.fallbackStrategy.Delay(h.currentAttempt)
	default:
		return h.fallbackStrategy.Delay(h.currentAttempt)
	}
}

// applyDiophantineStrategy applies Diophantine-based backoff
func (h *HTTPAwareAdaptiveBackoff) applyDiophantineStrategy(params map[string]interface{}) time.Duration {
	// For now, use a simple progression that mimics Diophantine behavior
	baseDelay := 60 * time.Second
	if delay, exists := params["base_delay"]; exists {
		if delayDuration, ok := delay.(time.Duration); ok {
			baseDelay = delayDuration
		}
	}

	// Simple progression: base, base*2, base*3, etc.
	attempt := h.GetAttemptCount()
	if attempt == 0 {
		attempt = 1 // Start from attempt 1
	}
	return time.Duration(int64(baseDelay) * int64(attempt))
}

// applyPolynomialStrategy applies polynomial backoff
func (h *HTTPAwareAdaptiveBackoff) applyPolynomialStrategy(params map[string]interface{}) time.Duration {
	degree := 2
	coefficient := 1.5

	if d, exists := params["degree"]; exists {
		if degreeInt, ok := d.(int); ok {
			degree = degreeInt
		}
	}

	if c, exists := params["coefficient"]; exists {
		if coeffFloat, ok := c.(float64); ok {
			coefficient = coeffFloat
		}
	}

	attempt := h.GetAttemptCount()
	if attempt == 0 {
		attempt = 1 // Start from attempt 1
	}
	delay := time.Duration(coefficient*pow(float64(attempt), degree)) * time.Second

	// Apply jitter if enabled
	if jitter, exists := params["jitter"]; exists {
		if jitterBool, ok := jitter.(bool); ok && jitterBool {
			delay = h.applyJitter(delay)
		}
	}

	return delay
}

// applyExponentialStrategy applies exponential backoff
func (h *HTTPAwareAdaptiveBackoff) applyExponentialStrategy(params map[string]interface{}) time.Duration {
	multiplier := 2.0
	initialDelay := 1 * time.Second

	if m, exists := params["multiplier"]; exists {
		if multFloat, ok := m.(float64); ok {
			multiplier = multFloat
		}
	}

	if initial, exists := params["initial_delay"]; exists {
		if initialDuration, ok := initial.(time.Duration); ok {
			initialDelay = initialDuration
		}
	}

	attempt := h.GetAttemptCount()
	delay := time.Duration(float64(initialDelay) * pow(multiplier, attempt-1))

	// Apply jitter if enabled
	if jitter, exists := params["jitter"]; exists {
		if jitterBool, ok := jitter.(bool); ok && jitterBool {
			delay = h.applyJitter(delay)
		}
	}

	return delay
}

// applyFixedStrategy applies fixed delay
func (h *HTTPAwareAdaptiveBackoff) applyFixedStrategy(params map[string]interface{}) time.Duration {
	delay := 60 * time.Second

	if d, exists := params["initial_delay"]; exists {
		if delayDuration, ok := d.(time.Duration); ok {
			delay = delayDuration
		}
	}

	return delay
}

// applyJitter applies jitter to a delay
func (h *HTTPAwareAdaptiveBackoff) applyJitter(delay time.Duration) time.Duration {
	// Simple jitter: ±20%
	jitterAmount := float64(delay) * 0.2
	jitter := (h.random() - 0.5) * 2 * jitterAmount
	return time.Duration(float64(delay) + jitter)
}

// random returns a thread-safe random float64 between 0 and 1
func (h *HTTPAwareAdaptiveBackoff) random() float64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rng.Float64()
}

// pow calculates base^exp for integers
func pow(base float64, exp int) float64 {
	result := 1.0
	for i := 0; i < exp; i++ {
		result *= base
	}
	return result
}

// GetAttemptCount returns the current attempt count
func (h *HTTPAwareAdaptiveBackoff) GetAttemptCount() int {
	return h.currentAttempt
}
