package discovery

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Parser extracts rate limit information from HTTP responses
type Parser struct {
	// Compiled regex patterns for performance
	headerPatterns map[string]*regexp.Regexp
}

// NewParser creates a new rate limit parser
func NewParser() *Parser {
	return &Parser{
		headerPatterns: compileHeaderPatterns(),
	}
}

// compileHeaderPatterns compiles all the regex patterns for header parsing
func compileHeaderPatterns() map[string]*regexp.Regexp {
	patterns := map[string]*regexp.Regexp{
		// Standard rate limit headers
		"x-ratelimit-limit":     regexp.MustCompile(`(?i)x-ratelimit-limit:\s*(\d+)`),
		"x-ratelimit-remaining": regexp.MustCompile(`(?i)x-ratelimit-remaining:\s*(\d+)`),
		"x-ratelimit-reset":     regexp.MustCompile(`(?i)x-ratelimit-reset:\s*(\d+)`),
		"retry-after":           regexp.MustCompile(`(?i)retry-after:\s*(\d+)`),

		// GitHub-style headers
		"x-ratelimit-limit-gh":     regexp.MustCompile(`(?i)x-ratelimit-limit:\s*(\d+)`),
		"x-ratelimit-remaining-gh": regexp.MustCompile(`(?i)x-ratelimit-remaining:\s*(\d+)`),
		"x-ratelimit-reset-gh":     regexp.MustCompile(`(?i)x-ratelimit-reset:\s*(\d+)`),

		// Twitter-style headers (lowercase)
		"x-rate-limit-limit":     regexp.MustCompile(`(?i)x-rate-limit-limit:\s*(\d+)`),
		"x-rate-limit-remaining": regexp.MustCompile(`(?i)x-rate-limit-remaining:\s*(\d+)`),
		"x-rate-limit-reset":     regexp.MustCompile(`(?i)x-rate-limit-reset:\s*(\d+)`),

		// Alternative header formats
		"ratelimit-limit":     regexp.MustCompile(`(?i)ratelimit-limit:\s*(\d+)`),
		"ratelimit-remaining": regexp.MustCompile(`(?i)ratelimit-remaining:\s*(\d+)`),
		"ratelimit-reset":     regexp.MustCompile(`(?i)ratelimit-reset:\s*(\d+)`),

		// AWS-style headers
		"x-amzn-requestid": regexp.MustCompile(`(?i)x-amzn-requestid:\s*([a-zA-Z0-9-]+)`),
	}

	return patterns
}

// ParseFromCommandOutput extracts rate limit information from command output
func (p *Parser) ParseFromCommandOutput(stdout, stderr string, exitCode int, command []string) *DiscoveryResult {
	// Combine stdout and stderr for analysis
	output := stdout + "\n" + stderr

	// Limit processing size to prevent memory issues
	const maxProcessingSize = 50 * 1024 // 50KB
	if len(output) > maxProcessingSize {
		output = output[:maxProcessingSize]
	}

	// Extract resource information from command
	resourceID, host, path := p.extractResourceInfo(command)

	// Try parsing HTTP headers first (most reliable)
	if result := p.parseHTTPHeaders(output, resourceID, host, path); result.Found {
		return result
	}

	// Try parsing JSON response body
	if result := p.parseJSONResponse(output, resourceID, host, path); result.Found {
		return result
	}

	// No rate limit information found
	return &DiscoveryResult{
		Found:      false,
		Source:     SourceHTTPHeader,
		Confidence: 0.0,
	}
}

// extractResourceInfo extracts resource identification from command
func (p *Parser) extractResourceInfo(command []string) (resourceID, host, path string) {
	if len(command) == 0 {
		return "unknown", "unknown", "/"
	}

	// Default resource ID
	resourceID = strings.Join(command, " ")

	// Try to extract host and path from common commands
	switch command[0] {
	case "curl":
		if urlStr := p.extractURLFromCurl(command); urlStr != "" {
			if u, err := url.Parse(urlStr); err == nil {
				host = u.Host
				path = u.Path
				if path == "" {
					path = "/"
				}
				resourceID = host + path
			}
		}
	case "wget":
		if urlStr := p.extractURLFromWget(command); urlStr != "" {
			if u, err := url.Parse(urlStr); err == nil {
				host = u.Host
				path = u.Path
				if path == "" {
					path = "/"
				}
				resourceID = host + path
			}
		}
	case "http", "https":
		// HTTPie commands
		if len(command) > 1 {
			if u, err := url.Parse(command[1]); err == nil {
				host = u.Host
				path = u.Path
				if path == "" {
					path = "/"
				}
				resourceID = host + path
			}
		}
	}

	if host == "" {
		host = "unknown"
	}
	if path == "" {
		path = "/"
	}

	return resourceID, host, path
}

// extractURLFromCurl extracts URL from curl command arguments
func (p *Parser) extractURLFromCurl(command []string) string {
	for i, arg := range command {
		if i == 0 {
			continue // Skip "curl"
		}
		if !strings.HasPrefix(arg, "-") {
			// First non-flag argument is likely the URL
			return arg
		}
		if arg == "-u" || arg == "--url" {
			// Next argument is the URL
			if i+1 < len(command) {
				return command[i+1]
			}
		}
	}
	return ""
}

// extractURLFromWget extracts URL from wget command arguments
func (p *Parser) extractURLFromWget(command []string) string {
	for i, arg := range command {
		if i == 0 {
			continue // Skip "wget"
		}
		if !strings.HasPrefix(arg, "-") {
			// First non-flag argument is likely the URL
			return arg
		}
	}
	return ""
}

// parseHTTPHeaders extracts rate limit information from HTTP headers
func (p *Parser) parseHTTPHeaders(output, resourceID, host, path string) *DiscoveryResult {
	headers := p.extractRateLimitHeaders(output)

	// Check if we found any rate limit headers
	if !p.hasRateLimitInfo(headers) {
		return &DiscoveryResult{Found: false}
	}

	// Create rate limit info from headers
	info := &RateLimitInfo{
		ResourceID:         resourceID,
		Host:               host,
		Path:               path,
		Source:             string(SourceHTTPHeader),
		LastSeen:           time.Now(),
		ObservationCount:   1,
		SuccessfulRequests: 0,
		FailedRequests:     0,
	}

	// Extract limit
	if headers.RateLimit != nil {
		info.Limit = *headers.RateLimit
	} else if headers.GitHubLimit != nil {
		info.Limit = *headers.GitHubLimit
	} else if headers.TwitterLimit != nil {
		info.Limit = *headers.TwitterLimit
	}

	// Extract remaining
	if headers.RateLimitRemaining != nil {
		info.Remaining = *headers.RateLimitRemaining
	} else if headers.GitHubRemaining != nil {
		info.Remaining = *headers.GitHubRemaining
	} else if headers.TwitterRemaining != nil {
		info.Remaining = *headers.TwitterRemaining
	}

	// Extract reset time
	if headers.RateLimitReset != nil {
		info.ResetTime = *headers.RateLimitReset
		// Calculate window duration
		info.Window = time.Until(*headers.RateLimitReset)
		if info.Window < 0 {
			info.Window = time.Hour // Default to 1 hour if reset time is in the past
		}
	} else if headers.GitHubReset != nil {
		info.ResetTime = *headers.GitHubReset
		info.Window = time.Until(*headers.GitHubReset)
		if info.Window < 0 {
			info.Window = time.Hour
		}
	} else if headers.TwitterReset != nil {
		info.ResetTime = *headers.TwitterReset
		info.Window = time.Until(*headers.TwitterReset)
		if info.Window < 0 {
			info.Window = time.Hour
		}
	} else if headers.RetryAfter != nil {
		// Use Retry-After as window duration
		info.Window = time.Duration(*headers.RetryAfter) * time.Second
		info.ResetTime = time.Now().Add(info.Window)
	}

	// Set default window if not determined
	if info.Window == 0 {
		info.Window = time.Hour // Default to 1 hour
		info.ResetTime = time.Now().Add(info.Window)
	}

	// Calculate confidence
	info.Confidence = info.ConfidenceScore()

	return &DiscoveryResult{
		Found:      true,
		Info:       info,
		Source:     SourceHTTPHeader,
		Confidence: info.Confidence,
	}
}

// extractRateLimitHeaders extracts rate limit headers from HTTP response text
func (p *Parser) extractRateLimitHeaders(output string) *RateLimitHeaders {
	headers := &RateLimitHeaders{}

	// Extract standard headers
	if match := p.headerPatterns["x-ratelimit-limit"].FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.Atoi(strings.TrimSpace(match[1])); err == nil {
			headers.RateLimit = &val
		}
	}

	if match := p.headerPatterns["x-ratelimit-remaining"].FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.Atoi(strings.TrimSpace(match[1])); err == nil {
			headers.RateLimitRemaining = &val
		}
	}

	if match := p.headerPatterns["x-ratelimit-reset"].FindStringSubmatch(output); len(match) > 1 {
		if timestamp, err := strconv.ParseInt(strings.TrimSpace(match[1]), 10, 64); err == nil {
			resetTime := time.Unix(timestamp, 0)
			headers.RateLimitReset = &resetTime
		}
	}

	if match := p.headerPatterns["retry-after"].FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.Atoi(strings.TrimSpace(match[1])); err == nil {
			headers.RetryAfter = &val
		}
	}

	// Extract GitHub-style headers (same patterns, different struct fields)
	headers.GitHubLimit = headers.RateLimit
	headers.GitHubRemaining = headers.RateLimitRemaining
	headers.GitHubReset = headers.RateLimitReset

	// Extract Twitter-style headers
	if match := p.headerPatterns["x-rate-limit-limit"].FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.Atoi(strings.TrimSpace(match[1])); err == nil {
			headers.TwitterLimit = &val
		}
	}

	if match := p.headerPatterns["x-rate-limit-remaining"].FindStringSubmatch(output); len(match) > 1 {
		if val, err := strconv.Atoi(strings.TrimSpace(match[1])); err == nil {
			headers.TwitterRemaining = &val
		}
	}

	if match := p.headerPatterns["x-rate-limit-reset"].FindStringSubmatch(output); len(match) > 1 {
		if timestamp, err := strconv.ParseInt(strings.TrimSpace(match[1]), 10, 64); err == nil {
			resetTime := time.Unix(timestamp, 0)
			headers.TwitterReset = &resetTime
		}
	}

	return headers
}

// hasRateLimitInfo checks if the headers contain any rate limit information
func (p *Parser) hasRateLimitInfo(headers *RateLimitHeaders) bool {
	return headers.RateLimit != nil ||
		headers.RateLimitRemaining != nil ||
		headers.RateLimitReset != nil ||
		headers.RetryAfter != nil ||
		headers.GitHubLimit != nil ||
		headers.GitHubRemaining != nil ||
		headers.GitHubReset != nil ||
		headers.TwitterLimit != nil ||
		headers.TwitterRemaining != nil ||
		headers.TwitterReset != nil
}

// parseJSONResponse extracts rate limit information from JSON response bodies
func (p *Parser) parseJSONResponse(output, resourceID, host, path string) *DiscoveryResult {
	// Look for JSON content
	if !strings.Contains(output, "{") {
		return &DiscoveryResult{Found: false}
	}

	// Find JSON in the output
	start := strings.Index(output, "{")
	if start == -1 {
		return &DiscoveryResult{Found: false}
	}

	end := strings.LastIndex(output, "}")
	if end == -1 || end <= start {
		return &DiscoveryResult{Found: false}
	}

	jsonStr := output[start : end+1]

	// Parse JSON
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return &DiscoveryResult{Found: false}
	}

	// Extract rate limit information
	rateLimitJSON := p.extractRateLimitFromJSON(data)
	if !p.hasJSONRateLimitInfo(rateLimitJSON) {
		return &DiscoveryResult{Found: false}
	}

	// Create rate limit info from JSON
	info := &RateLimitInfo{
		ResourceID:         resourceID,
		Host:               host,
		Path:               path,
		Source:             string(SourceJSONBody),
		LastSeen:           time.Now(),
		ObservationCount:   1,
		SuccessfulRequests: 0,
		FailedRequests:     0,
	}

	// Extract values from JSON
	if rateLimitJSON.Limit != nil {
		info.Limit = *rateLimitJSON.Limit
	} else if rateLimitJSON.RateLimit != nil && rateLimitJSON.RateLimit.Limit != nil {
		info.Limit = *rateLimitJSON.RateLimit.Limit
	}

	if rateLimitJSON.Remaining != nil {
		info.Remaining = *rateLimitJSON.Remaining
	} else if rateLimitJSON.RateLimit != nil && rateLimitJSON.RateLimit.Remaining != nil {
		info.Remaining = *rateLimitJSON.RateLimit.Remaining
	}

	// Handle retry after
	var retryAfterSeconds int
	if rateLimitJSON.RetryAfter != nil {
		retryAfterSeconds = *rateLimitJSON.RetryAfter
	} else if rateLimitJSON.RetryAfterSec != nil {
		retryAfterSeconds = *rateLimitJSON.RetryAfterSec
	} else if rateLimitJSON.Error != nil && rateLimitJSON.Error.RetryAfter != nil {
		retryAfterSeconds = *rateLimitJSON.Error.RetryAfter
	}

	if retryAfterSeconds > 0 {
		info.Window = time.Duration(retryAfterSeconds) * time.Second
		info.ResetTime = time.Now().Add(info.Window)
	} else if rateLimitJSON.RateLimit != nil && rateLimitJSON.RateLimit.Reset != nil {
		resetTime := time.Unix(int64(*rateLimitJSON.RateLimit.Reset), 0)
		info.ResetTime = resetTime
		info.Window = time.Until(resetTime)
		if info.Window < 0 {
			info.Window = time.Hour
		}
	}

	// Set default window if not determined
	if info.Window == 0 {
		info.Window = time.Hour
		info.ResetTime = time.Now().Add(info.Window)
	}

	// Calculate confidence
	info.Confidence = info.ConfidenceScore()

	return &DiscoveryResult{
		Found:      true,
		Info:       info,
		Source:     SourceJSONBody,
		Confidence: info.Confidence,
	}
}

// extractRateLimitFromJSON extracts rate limit information from parsed JSON
func (p *Parser) extractRateLimitFromJSON(data map[string]interface{}) *RateLimitJSON {
	result := &RateLimitJSON{}

	// Direct fields
	if val, ok := data["limit"]; ok {
		if intVal, ok := val.(float64); ok {
			limit := int(intVal)
			result.Limit = &limit
		}
	}

	if val, ok := data["remaining"]; ok {
		if intVal, ok := val.(float64); ok {
			remaining := int(intVal)
			result.Remaining = &remaining
		}
	}

	if val, ok := data["retry_after"]; ok {
		if intVal, ok := val.(float64); ok {
			retryAfter := int(intVal)
			result.RetryAfter = &retryAfter
		}
	}

	if val, ok := data["retry_after_seconds"]; ok {
		if intVal, ok := val.(float64); ok {
			retryAfterSec := int(intVal)
			result.RetryAfterSec = &retryAfterSec
		}
	}

	// Nested rate_limit object
	if rateLimitObj, ok := data["rate_limit"]; ok {
		if rateLimitMap, ok := rateLimitObj.(map[string]interface{}); ok {
			result.RateLimit = &struct {
				Limit     *int `json:"limit,omitempty"`
				Remaining *int `json:"remaining,omitempty"`
				Reset     *int `json:"reset,omitempty"`
			}{}

			if val, ok := rateLimitMap["limit"]; ok {
				if intVal, ok := val.(float64); ok {
					limit := int(intVal)
					result.RateLimit.Limit = &limit
				}
			}

			if val, ok := rateLimitMap["remaining"]; ok {
				if intVal, ok := val.(float64); ok {
					remaining := int(intVal)
					result.RateLimit.Remaining = &remaining
				}
			}

			if val, ok := rateLimitMap["reset"]; ok {
				if intVal, ok := val.(float64); ok {
					reset := int(intVal)
					result.RateLimit.Reset = &reset
				}
			}
		}
	}

	// Error object
	if errorObj, ok := data["error"]; ok {
		if errorMap, ok := errorObj.(map[string]interface{}); ok {
			result.Error = &struct {
				Code       *string `json:"code,omitempty"`
				Message    *string `json:"message,omitempty"`
				RetryAfter *int    `json:"retry_after,omitempty"`
			}{}

			if val, ok := errorMap["code"]; ok {
				if strVal, ok := val.(string); ok {
					result.Error.Code = &strVal
				}
			}

			if val, ok := errorMap["message"]; ok {
				if strVal, ok := val.(string); ok {
					result.Error.Message = &strVal
				}
			}

			if val, ok := errorMap["retry_after"]; ok {
				if intVal, ok := val.(float64); ok {
					retryAfter := int(intVal)
					result.Error.RetryAfter = &retryAfter
				}
			}
		}
	}

	return result
}

// hasJSONRateLimitInfo checks if the JSON contains any rate limit information
func (p *Parser) hasJSONRateLimitInfo(rateLimitJSON *RateLimitJSON) bool {
	return rateLimitJSON.Limit != nil ||
		rateLimitJSON.Remaining != nil ||
		rateLimitJSON.RetryAfter != nil ||
		rateLimitJSON.RetryAfterSec != nil ||
		(rateLimitJSON.RateLimit != nil && (rateLimitJSON.RateLimit.Limit != nil || rateLimitJSON.RateLimit.Remaining != nil || rateLimitJSON.RateLimit.Reset != nil)) ||
		(rateLimitJSON.Error != nil && rateLimitJSON.Error.RetryAfter != nil)
}
