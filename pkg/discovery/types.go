package discovery

import (
	"time"
)

// RateLimitInfo represents discovered rate limit information for a resource
type RateLimitInfo struct {
	// Resource identification
	ResourceID string `json:"resource_id" db:"resource_id"`
	Host       string `json:"host" db:"host"`
	Path       string `json:"path" db:"path"`

	// Rate limit details
	Limit     int           `json:"limit" db:"limit"`         // Requests per window
	Window    time.Duration `json:"window" db:"window"`       // Time window duration
	Remaining int           `json:"remaining" db:"remaining"` // Remaining requests in current window
	ResetTime time.Time     `json:"reset_time" db:"reset_time"`

	// Discovery metadata
	Source           string    `json:"source" db:"source"`                       // How this was discovered (header, json, learned)
	Confidence       float64   `json:"confidence" db:"confidence"`               // Confidence score (0.0-1.0)
	LastSeen         time.Time `json:"last_seen" db:"last_seen"`                 // When this info was last observed
	ObservationCount int       `json:"observation_count" db:"observation_count"` // Number of times observed

	// Learning data
	SuccessfulRequests int        `json:"successful_requests" db:"successful_requests"`
	FailedRequests     int        `json:"failed_requests" db:"failed_requests"`
	Last429Response    *time.Time `json:"last_429_response,omitempty" db:"last_429_response"`
}

// DiscoverySource represents how rate limit information was discovered
type DiscoverySource string

const (
	SourceHTTPHeader DiscoverySource = "http_header"
	SourceJSONBody   DiscoverySource = "json_body"
	SourceLearned    DiscoverySource = "learned"
	SourceManual     DiscoverySource = "manual"
)

// RateLimitHeaders represents common rate limit headers found in HTTP responses
type RateLimitHeaders struct {
	// Standard headers
	RateLimit          *int       `json:"rate_limit,omitempty"`           // X-RateLimit-Limit
	RateLimitRemaining *int       `json:"rate_limit_remaining,omitempty"` // X-RateLimit-Remaining
	RateLimitReset     *time.Time `json:"rate_limit_reset,omitempty"`     // X-RateLimit-Reset
	RetryAfter         *int       `json:"retry_after,omitempty"`          // Retry-After (seconds)

	// GitHub-style headers
	GitHubLimit     *int       `json:"github_limit,omitempty"`     // X-RateLimit-Limit
	GitHubRemaining *int       `json:"github_remaining,omitempty"` // X-RateLimit-Remaining
	GitHubReset     *time.Time `json:"github_reset,omitempty"`     // X-RateLimit-Reset

	// Twitter-style headers
	TwitterLimit     *int       `json:"twitter_limit,omitempty"`     // x-rate-limit-limit
	TwitterRemaining *int       `json:"twitter_remaining,omitempty"` // x-rate-limit-remaining
	TwitterReset     *time.Time `json:"twitter_reset,omitempty"`     // x-rate-limit-reset

	// AWS-style headers
	AWSThrottleLimit     *int       `json:"aws_throttle_limit,omitempty"`     // X-Amzn-RequestId
	AWSThrottleRemaining *int       `json:"aws_throttle_remaining,omitempty"` // X-Amzn-Trace-Id
	AWSThrottleReset     *time.Time `json:"aws_throttle_reset,omitempty"`     // Custom calculation
}

// RateLimitJSON represents rate limit information found in JSON response bodies
type RateLimitJSON struct {
	// Common JSON field patterns
	Limit         *int `json:"limit,omitempty"`
	Remaining     *int `json:"remaining,omitempty"`
	RetryAfter    *int `json:"retry_after,omitempty"`
	RetryAfterSec *int `json:"retry_after_seconds,omitempty"`

	// Nested rate limit objects
	RateLimit *struct {
		Limit     *int `json:"limit,omitempty"`
		Remaining *int `json:"remaining,omitempty"`
		Reset     *int `json:"reset,omitempty"`
	} `json:"rate_limit,omitempty"`

	// Error-specific fields
	Error *struct {
		Code       *string `json:"code,omitempty"`
		Message    *string `json:"message,omitempty"`
		RetryAfter *int    `json:"retry_after,omitempty"`
	} `json:"error,omitempty"`
}

// DiscoveryResult represents the result of attempting to discover rate limit information
type DiscoveryResult struct {
	Found      bool            `json:"found"`
	Info       *RateLimitInfo  `json:"info,omitempty"`
	Source     DiscoverySource `json:"source"`
	Confidence float64         `json:"confidence"`
	Error      error           `json:"error,omitempty"`
}

// LearningData represents data collected for learning rate limits
type LearningData struct {
	ResourceID   string        `json:"resource_id" db:"resource_id"`
	RequestTime  time.Time     `json:"request_time" db:"request_time"`
	ResponseCode int           `json:"response_code" db:"response_code"`
	Success      bool          `json:"success" db:"success"`
	ResponseTime time.Duration `json:"response_time" db:"response_time"`

	// Context about the request
	Command string `json:"command" db:"command"`
	Host    string `json:"host" db:"host"`
	Path    string `json:"path" db:"path"`
}

// ConfidenceScore calculates a confidence score for discovered rate limit information
func (r *RateLimitInfo) ConfidenceScore() float64 {
	if r.ObservationCount == 0 {
		return 0.0
	}

	// Base confidence from source
	var baseConfidence float64
	switch DiscoverySource(r.Source) {
	case SourceHTTPHeader:
		baseConfidence = 0.9 // HTTP headers are very reliable
	case SourceJSONBody:
		baseConfidence = 0.8 // JSON responses are quite reliable
	case SourceLearned:
		baseConfidence = 0.6 // Learned data is less reliable
	case SourceManual:
		baseConfidence = 1.0 // Manual configuration is most reliable
	default:
		baseConfidence = 0.5
	}

	// Adjust based on observation count (more observations = higher confidence)
	observationFactor := float64(r.ObservationCount) / (float64(r.ObservationCount) + 10.0)

	// Adjust based on success rate
	totalRequests := r.SuccessfulRequests + r.FailedRequests
	var successFactor float64 = 1.0
	if totalRequests > 0 {
		successRate := float64(r.SuccessfulRequests) / float64(totalRequests)
		successFactor = 0.5 + (successRate * 0.5) // Range: 0.5-1.0
	}

	// Adjust based on recency (more recent = higher confidence)
	timeSinceLastSeen := time.Since(r.LastSeen)
	var recencyFactor float64 = 1.0
	if timeSinceLastSeen > 24*time.Hour {
		// Decay confidence over time
		daysSince := timeSinceLastSeen.Hours() / 24.0
		recencyFactor = 1.0 / (1.0 + daysSince/7.0) // Decay over weeks
	}

	confidence := baseConfidence * observationFactor * successFactor * recencyFactor

	// Ensure confidence is between 0.0 and 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.0 {
		confidence = 0.0
	}

	return confidence
}

// IsExpired checks if the rate limit information is too old to be reliable
func (r *RateLimitInfo) IsExpired() bool {
	// Consider information expired after 7 days without observation
	return time.Since(r.LastSeen) > 7*24*time.Hour
}

// ShouldUpdate determines if this rate limit info should be updated with new data
func (r *RateLimitInfo) ShouldUpdate(newInfo *RateLimitInfo) bool {
	// Always update if new info has higher confidence
	if newInfo.ConfidenceScore() > r.ConfidenceScore() {
		return true
	}

	// Update if current info is expired
	if r.IsExpired() {
		return true
	}

	// Update if new info is from a more reliable source
	currentSourcePriority := getSourcePriority(DiscoverySource(r.Source))
	newSourcePriority := getSourcePriority(DiscoverySource(newInfo.Source))

	return newSourcePriority > currentSourcePriority
}

// getSourcePriority returns a priority score for different discovery sources
func getSourcePriority(source DiscoverySource) int {
	switch source {
	case SourceManual:
		return 4
	case SourceHTTPHeader:
		return 3
	case SourceJSONBody:
		return 2
	case SourceLearned:
		return 1
	default:
		return 0
	}
}
