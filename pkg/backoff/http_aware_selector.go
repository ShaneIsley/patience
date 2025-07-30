package backoff

import (
	"crypto/md5"
	"fmt"
	"sync"
	"time"

	"github.com/shaneisley/patience/pkg/patterns"
)

// HTTPAwareBackoffSelector selects optimal backoff strategies based on HTTP responses
type HTTPAwareBackoffSelector struct {
	patternMatcher *patterns.HTTPPatternMatcher
	strategyCache  map[string]*StrategyRecommendation
	effectiveness  map[string]*EffectivenessTracker
	cacheMutex     sync.RWMutex
}

// StrategyRecommendation contains detailed backoff strategy parameters
type StrategyRecommendation struct {
	Type       string
	Parameters map[string]interface{}
	Adaptive   bool
	Priority   int
}

// NewHTTPAwareBackoffSelector creates a new HTTP-aware backoff selector
func NewHTTPAwareBackoffSelector() *HTTPAwareBackoffSelector {
	config := patterns.DefaultHTTPPatternConfig()
	matcher, err := patterns.NewHTTPPatternMatcher(config)
	if err != nil {
		// Fallback to nil matcher - will use generic strategies
		matcher = nil
	}

	return &HTTPAwareBackoffSelector{
		patternMatcher: matcher,
		strategyCache:  make(map[string]*StrategyRecommendation),
		effectiveness:  make(map[string]*EffectivenessTracker),
	}
}

// SelectStrategy chooses the optimal backoff strategy for an HTTP response
func (s *HTTPAwareBackoffSelector) SelectStrategy(response *patterns.HTTPResponse) (string, map[string]interface{}, error) {
	if response == nil {
		return "exponential", map[string]interface{}{}, nil
	}

	// Generate cache key for this response
	cacheKey := s.generateCacheKey(response)

	// Check cache first
	s.cacheMutex.RLock()
	if cached, exists := s.strategyCache[cacheKey]; exists {
		s.cacheMutex.RUnlock()
		return cached.Type, cached.Parameters, nil
	}
	s.cacheMutex.RUnlock()

	// Use the response directly since it's already patterns.HTTPResponse

	// Match HTTP response to patterns
	var strategy string
	var params map[string]interface{}

	if s.patternMatcher != nil {
		result, err := s.patternMatcher.MatchHTTPResponse(response)
		if err == nil {
			// Select strategy based on API type and pattern
			strategy, params = s.selectStrategyFromResult(result)
		} else {
			// Fallback to generic strategy
			strategy, params = s.selectGenericStrategy(response)
		}
	} else {
		// Fallback to generic strategy
		strategy, params = s.selectGenericStrategy(response)
	}

	// Cache the result
	recommendation := &StrategyRecommendation{
		Type:       strategy,
		Parameters: params,
		Adaptive:   s.isAdaptiveStrategy(strategy),
		Priority:   s.getStrategyPriority(strategy),
	}

	s.cacheMutex.Lock()
	s.strategyCache[cacheKey] = recommendation
	s.cacheMutex.Unlock()

	return strategy, params, nil
}

// selectStrategyFromResult selects strategy based on pattern matching result
func (s *HTTPAwareBackoffSelector) selectStrategyFromResult(result *patterns.HTTPMatchResult) (string, map[string]interface{}) {
	switch result.APIType {
	case patterns.APITypeGitHub:
		return s.selectGitHubStrategy(result)
	case patterns.APITypeAWS:
		return s.selectAWSStrategy(result)
	case patterns.APITypeKubernetes:
		return s.selectKubernetesStrategy(result)
	default:
		return s.selectGenericStrategyFromResult(result)
	}
}

// selectGitHubStrategy selects strategy for GitHub APIs
func (s *HTTPAwareBackoffSelector) selectGitHubStrategy(result *patterns.HTTPMatchResult) (string, map[string]interface{}) {
	// GitHub APIs benefit from Diophantine discovery for rate limit patterns
	params := map[string]interface{}{
		"discovery_enabled": true,
		"pattern_learning":  true,
		"max_attempts":      5,
	}

	if retryAfter, exists := result.Context["retry_after"]; exists {
		if retryAfterFloat, ok := retryAfter.(float64); ok {
			params["base_delay"] = time.Duration(retryAfterFloat) * time.Second
		}
	}

	return "diophantine", params
}

// selectAWSStrategy selects strategy for AWS APIs
func (s *HTTPAwareBackoffSelector) selectAWSStrategy(result *patterns.HTTPMatchResult) (string, map[string]interface{}) {
	// AWS APIs work well with polynomial backoff for throttling
	params := map[string]interface{}{
		"degree":      2,
		"coefficient": 1.5,
		"max_delay":   300 * time.Second,
		"jitter":      true,
	}

	return "polynomial", params
}

// selectKubernetesStrategy selects strategy for Kubernetes APIs
func (s *HTTPAwareBackoffSelector) selectKubernetesStrategy(result *patterns.HTTPMatchResult) (string, map[string]interface{}) {
	// Kubernetes APIs use exponential backoff with jitter
	params := map[string]interface{}{
		"multiplier":    2.0,
		"initial_delay": 1 * time.Second,
		"max_delay":     30 * time.Second,
		"jitter":        true,
	}

	return "exponential", params
}

// selectGenericStrategyFromResult selects strategy for generic APIs based on pattern result
func (s *HTTPAwareBackoffSelector) selectGenericStrategyFromResult(result *patterns.HTTPMatchResult) (string, map[string]interface{}) {
	params := map[string]interface{}{
		"learning_enabled":       true,
		"effectiveness_tracking": true,
	}

	return "adaptive", params
}

// selectGenericStrategy selects strategy based on HTTP response characteristics
func (s *HTTPAwareBackoffSelector) selectGenericStrategy(response *patterns.HTTPResponse) (string, map[string]interface{}) {
	params := map[string]interface{}{}

	// Check for rate limiting indicators
	if response.StatusCode == 429 {
		params["initial_delay"] = 60 * time.Second
		params["max_retries"] = 3
		return "fixed", params
	}

	// Check for server errors
	if response.StatusCode >= 500 {
		params["multiplier"] = 2.0
		params["initial_delay"] = 1 * time.Second
		params["max_delay"] = 30 * time.Second
		return "exponential", params
	}

	// Default to adaptive
	params["learning_enabled"] = true
	return "adaptive", params
}

// generateCacheKey generates a cache key for the HTTP response
func (s *HTTPAwareBackoffSelector) generateCacheKey(response *patterns.HTTPResponse) string {
	// Create a hash based on key response characteristics
	key := fmt.Sprintf("%d:%s:%s", response.StatusCode, response.URL, s.getKeyHeaders(response))
	hash := md5.Sum([]byte(key))
	return fmt.Sprintf("%x", hash)
}

// getKeyHeaders extracts key headers for caching
func (s *HTTPAwareBackoffSelector) getKeyHeaders(response *patterns.HTTPResponse) string {
	keyHeaders := []string{
		"X-RateLimit-Limit",
		"X-Amzn-ErrorType",
		"X-GitHub-Media-Type",
		"Content-Type",
	}

	var headerValues []string
	for _, header := range keyHeaders {
		if value, exists := response.Headers[header]; exists {
			headerValues = append(headerValues, fmt.Sprintf("%s:%s", header, value))
		}
	}

	return fmt.Sprintf("[%s]", fmt.Sprintf("%v", headerValues))
}

// isAdaptiveStrategy checks if a strategy is adaptive
func (s *HTTPAwareBackoffSelector) isAdaptiveStrategy(strategy string) bool {
	adaptiveStrategies := map[string]bool{
		"adaptive":    true,
		"diophantine": true,
	}
	return adaptiveStrategies[strategy]
}

// getStrategyPriority returns the priority of a strategy
func (s *HTTPAwareBackoffSelector) getStrategyPriority(strategy string) int {
	priorities := map[string]int{
		"diophantine": 1, // Highest priority
		"polynomial":  2,
		"exponential": 3,
		"adaptive":    4,
		"fixed":       5, // Lowest priority
	}

	if priority, exists := priorities[strategy]; exists {
		return priority
	}
	return 10 // Default low priority
}
