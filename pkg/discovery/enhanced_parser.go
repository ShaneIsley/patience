package discovery

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// EnhancedParser extends the base parser with additional HTTP-aware capabilities
type EnhancedParser struct {
	*Parser
	// Additional patterns for enhanced discovery
	enhancedPatterns map[string]*regexp.Regexp
	// Common API patterns for better resource identification
	apiPatterns map[string]*regexp.Regexp
}

// NewEnhancedParser creates a new enhanced parser with additional capabilities
func NewEnhancedParser() *EnhancedParser {
	return &EnhancedParser{
		Parser:           NewParser(),
		enhancedPatterns: compileEnhancedPatterns(),
		apiPatterns:      compileAPIPatterns(),
	}
}

// compileEnhancedPatterns compiles additional header patterns for better discovery
func compileEnhancedPatterns() map[string]*regexp.Regexp {
	patterns := map[string]*regexp.Regexp{
		// Additional standard headers
		"ratelimit-policy":    regexp.MustCompile(`(?i)ratelimit-policy:\s*(\d+);w=(\d+)`),
		"x-ratelimit-window":  regexp.MustCompile(`(?i)x-ratelimit-window:\s*(\d+)`),
		"x-rate-limit-window": regexp.MustCompile(`(?i)x-rate-limit-window:\s*(\d+)`),

		// Cloudflare headers
		"cf-ray":               regexp.MustCompile(`(?i)cf-ray:\s*([a-zA-Z0-9-]+)`),
		"x-ratelimit-limit-cf": regexp.MustCompile(`(?i)x-ratelimit-limit:\s*(\d+)`),

		// AWS API Gateway headers
		"x-amzn-requestid": regexp.MustCompile(`(?i)x-amzn-requestid:\s*([a-zA-Z0-9-]+)`),
		"x-amzn-trace-id":  regexp.MustCompile(`(?i)x-amzn-trace-id:\s*([a-zA-Z0-9-=]+)`),
		"x-amzn-ratelimit": regexp.MustCompile(`(?i)x-amzn-ratelimit-limit:\s*(\d+)`),

		// Google Cloud headers
		"x-goog-quota-limit":     regexp.MustCompile(`(?i)x-goog-quota-limit:\s*(\d+)`),
		"x-goog-quota-remaining": regexp.MustCompile(`(?i)x-goog-quota-remaining:\s*(\d+)`),

		// Microsoft Azure headers
		"x-ms-ratelimit-remaining": regexp.MustCompile(`(?i)x-ms-ratelimit-remaining-[^:]*:\s*(\d+)`),
		"x-ms-request-id":          regexp.MustCompile(`(?i)x-ms-request-id:\s*([a-zA-Z0-9-]+)`),

		// Docker Hub headers
		"docker-ratelimit-source": regexp.MustCompile(`(?i)docker-ratelimit-source:\s*([^\r\n]+)`),
		"ratelimit-limit":         regexp.MustCompile(`(?i)ratelimit-limit:\s*(\d+);w=(\d+)`),
		"ratelimit-remaining":     regexp.MustCompile(`(?i)ratelimit-remaining:\s*(\d+);w=(\d+)`),

		// Kubernetes API headers
		"x-kubernetes-pf-flowschema-uid":    regexp.MustCompile(`(?i)x-kubernetes-pf-flowschema-uid:\s*([a-zA-Z0-9-]+)`),
		"x-kubernetes-pf-prioritylevel-uid": regexp.MustCompile(`(?i)x-kubernetes-pf-prioritylevel-uid:\s*([a-zA-Z0-9-]+)`),

		// Generic rate limit patterns
		"x-rate-limit":   regexp.MustCompile(`(?i)x-rate-limit[^:]*:\s*(\d+)`),
		"rate-limit":     regexp.MustCompile(`(?i)rate-limit[^:]*:\s*(\d+)`),
		"quota-limit":    regexp.MustCompile(`(?i)quota-limit[^:]*:\s*(\d+)`),
		"throttle-limit": regexp.MustCompile(`(?i)throttle-limit[^:]*:\s*(\d+)`),

		// HTTP 429 response patterns
		"http-429":   regexp.MustCompile(`(?i)HTTP/[\d.]+\s+429\s+Too Many Requests`),
		"status-429": regexp.MustCompile(`(?i)status:\s*429`),

		// Common error messages indicating rate limiting
		"rate-limit-exceeded": regexp.MustCompile(`(?i)(rate.limit.exceeded|too.many.requests|quota.exceeded|throttled)`),
	}

	return patterns
}

// compileAPIPatterns compiles patterns for better API resource identification
func compileAPIPatterns() map[string]*regexp.Regexp {
	patterns := map[string]*regexp.Regexp{
		// REST API patterns
		"rest-api": regexp.MustCompile(`(?i)/(api|v\d+)/([^/\s?]+)`),
		"graphql":  regexp.MustCompile(`(?i)/graphql`),

		// Cloud provider patterns
		"aws-api":   regexp.MustCompile(`(?i)\.amazonaws\.com/`),
		"gcp-api":   regexp.MustCompile(`(?i)\.googleapis\.com/`),
		"azure-api": regexp.MustCompile(`(?i)\.azure\.com/`),

		// Container registry patterns
		"docker-hub": regexp.MustCompile(`(?i)registry-1\.docker\.io/`),
		"gcr":        regexp.MustCompile(`(?i)gcr\.io/`),
		"ecr":        regexp.MustCompile(`(?i)\.dkr\.ecr\.[^.]+\.amazonaws\.com/`),

		// Git hosting patterns
		"github-api":    regexp.MustCompile(`(?i)api\.github\.com/`),
		"gitlab-api":    regexp.MustCompile(`(?i)gitlab\.com/api/`),
		"bitbucket-api": regexp.MustCompile(`(?i)api\.bitbucket\.org/`),

		// Kubernetes patterns
		"k8s-api":  regexp.MustCompile(`(?i)/api/v\d+/`),
		"k8s-apis": regexp.MustCompile(`(?i)/apis/[^/]+/v\d+/`),
	}

	return patterns
}

// ParseFromCommandOutputEnhanced provides enhanced parsing with better resource identification
func (ep *EnhancedParser) ParseFromCommandOutputEnhanced(stdout, stderr string, exitCode int, command []string) *DiscoveryResult {
	// Use base parser first
	result := ep.ParseFromCommandOutput(stdout, stderr, exitCode, command)

	// If base parser found something, enhance it
	if result.Found {
		result = ep.enhanceDiscoveryResult(result, stdout, stderr, exitCode, command)
	} else {
		// Try enhanced parsing if base parser didn't find anything
		result = ep.parseWithEnhancedPatterns(stdout, stderr, exitCode, command)
	}

	return result
}

// enhanceDiscoveryResult enhances an existing discovery result with additional information
func (ep *EnhancedParser) enhanceDiscoveryResult(result *DiscoveryResult, stdout, stderr string, exitCode int, command []string) *DiscoveryResult {
	if result.Info == nil {
		return result
	}

	output := stdout + "\n" + stderr

	// Enhance resource identification
	resourceID, host, path := ep.extractEnhancedResourceInfo(command, output)
	if resourceID != "unknown" && resourceID != result.Info.ResourceID {
		result.Info.ResourceID = resourceID
		result.Info.Host = host
		result.Info.Path = path
	}

	// If the base parser didn't extract a rate limit, try enhanced headers
	if result.Info.Limit == 0 {
		headers := ep.extractEnhancedHeaders(output)
		if ep.hasEnhancedRateLimitInfo(headers) {
			ep.populateFromEnhancedHeaders(result.Info, headers, output)
		}
	}

	// Look for additional rate limit indicators
	if ep.hasRateLimitIndicators(output) {
		result.Confidence = min(result.Confidence+0.1, 1.0)
	}

	// Check for 429 responses
	if exitCode == 429 || ep.enhancedPatterns["http-429"].MatchString(output) {
		result.Info.Last429Response = &[]time.Time{time.Now()}[0]
		result.Info.FailedRequests++
		result.Confidence = min(result.Confidence+0.2, 1.0)
	}

	return result
}

// parseWithEnhancedPatterns attempts to parse using enhanced patterns when base parser fails
func (ep *EnhancedParser) parseWithEnhancedPatterns(stdout, stderr string, exitCode int, command []string) *DiscoveryResult {
	output := stdout + "\n" + stderr

	// Extract resource information
	resourceID, host, path := ep.extractEnhancedResourceInfo(command, output)

	// Look for enhanced rate limit patterns
	headers := ep.extractEnhancedHeaders(output)

	if !ep.hasEnhancedRateLimitInfo(headers) {
		return &DiscoveryResult{Found: false}
	}

	// Create rate limit info from enhanced headers
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

	// Extract enhanced rate limit information
	ep.populateFromEnhancedHeaders(info, headers, output)

	// Calculate confidence
	info.Confidence = info.ConfidenceScore()

	return &DiscoveryResult{
		Found:      true,
		Info:       info,
		Source:     SourceHTTPHeader,
		Confidence: info.Confidence,
	}
}

// extractEnhancedResourceInfo provides better resource identification using API patterns
func (ep *EnhancedParser) extractEnhancedResourceInfo(command []string, output string) (resourceID, host, path string) {
	// Start with base extraction
	resourceID, host, path = ep.extractResourceInfo(command)

	// Try to enhance with API pattern matching
	for _, arg := range command {
		if strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://") {
			if u, err := url.Parse(arg); err == nil {
				host = u.Host
				path = u.Path
				if path == "" {
					path = "/"
				}

				// Apply API pattern matching for better resource identification
				resourceID = ep.identifyAPIResource(host, path)
				break
			}
		}
	}

	return resourceID, host, path
}

// identifyAPIResource identifies the type of API resource for better grouping
func (ep *EnhancedParser) identifyAPIResource(host, path string) string {
	fullURL := host + path

	// Check for specific known services first (more specific matches)
	if strings.Contains(host, "docker.io") {
		return "docker:" + ep.normalizeDockerPath(path)
	}
	if strings.Contains(host, "amazonaws.com") {
		return "aws:" + host + ep.normalizeAWSPath(path)
	}
	if strings.Contains(host, "googleapis.com") {
		return "gcp:" + host + ep.normalizeGCPPath(path)
	}
	if strings.Contains(host, "api.github.com") {
		return "github:" + ep.normalizeGitHubPath(path)
	}
	if strings.Contains(host, "kubernetes") || strings.Contains(path, "/api/v") || strings.Contains(path, "/apis/") {
		return "k8s:" + host + ep.normalizeK8sPath(path)
	}

	// Check for generic API patterns
	for pattern, regex := range ep.apiPatterns {
		if regex.MatchString(fullURL) {
			return pattern + ":" + host + path
		}
	}

	// Default to host + path
	return host + path
}

// normalizeAWSPath normalizes AWS API paths for better grouping
func (ep *EnhancedParser) normalizeAWSPath(path string) string {
	// Group by service and version, ignore specific resource IDs
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 2 {
		return "/" + parts[0] + "/" + parts[1] + "/*"
	}
	return path
}

// normalizeGCPPath normalizes Google Cloud API paths
func (ep *EnhancedParser) normalizeGCPPath(path string) string {
	// Group by API version and resource type
	if match := regexp.MustCompile(`(/v\d+/[^/]+)`).FindString(path); match != "" {
		return match + "/*"
	}
	return path
}

// normalizeGitHubPath normalizes GitHub API paths
func (ep *EnhancedParser) normalizeGitHubPath(path string) string {
	// Group by API endpoint type
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 1 {
		return "/" + parts[0] + "/*"
	}
	return path
}

// normalizeDockerPath normalizes Docker Hub paths
func (ep *EnhancedParser) normalizeDockerPath(path string) string {
	// Group by registry operation type
	if strings.Contains(path, "/manifests/") {
		return "/v2/*/manifests/*"
	}
	if strings.Contains(path, "/blobs/") {
		return "/v2/*/blobs/*"
	}
	return path
}

// normalizeK8sPath normalizes Kubernetes API paths
func (ep *EnhancedParser) normalizeK8sPath(path string) string {
	// Group by API version and resource type
	if match := regexp.MustCompile(`(/api/v\d+)`).FindString(path); match != "" {
		return match + "/*"
	}
	if match := regexp.MustCompile(`(/apis/[^/]+/v\d+)`).FindString(path); match != "" {
		return match + "/*"
	}
	return path
}

// extractEnhancedHeaders extracts rate limit information using enhanced patterns
func (ep *EnhancedParser) extractEnhancedHeaders(output string) map[string]interface{} {
	headers := make(map[string]interface{})

	// Check all enhanced patterns
	for name, pattern := range ep.enhancedPatterns {
		if matches := pattern.FindAllStringSubmatch(output, -1); len(matches) > 0 {
			headers[name] = matches
		}
	}

	return headers
}

// hasEnhancedRateLimitInfo checks if enhanced headers contain rate limit information
func (ep *EnhancedParser) hasEnhancedRateLimitInfo(headers map[string]interface{}) bool {
	rateLimitIndicators := []string{
		"ratelimit-policy", "x-ratelimit-window", "x-rate-limit-window",
		"x-ratelimit-limit-cf", "x-amzn-ratelimit", "x-goog-quota-limit",
		"x-ms-ratelimit-remaining", "ratelimit-limit", "x-rate-limit",
		"rate-limit", "quota-limit", "throttle-limit",
	}

	for _, indicator := range rateLimitIndicators {
		if _, exists := headers[indicator]; exists {
			return true
		}
	}

	return false
}

// populateFromEnhancedHeaders populates rate limit info from enhanced headers
func (ep *EnhancedParser) populateFromEnhancedHeaders(info *RateLimitInfo, headers map[string]interface{}, output string) {
	// Set default values
	info.Window = time.Hour
	info.ResetTime = time.Now().Add(info.Window)

	// Extract from ratelimit-policy header (RFC format)
	if matches, exists := headers["ratelimit-policy"]; exists {
		if matchList, ok := matches.([][]string); ok && len(matchList) > 0 && len(matchList[0]) > 2 {
			if limit, err := strconv.Atoi(matchList[0][1]); err == nil {
				info.Limit = limit
			}
			if window, err := strconv.Atoi(matchList[0][2]); err == nil {
				info.Window = time.Duration(window) * time.Second
				info.ResetTime = time.Now().Add(info.Window)
			}
		}
	}

	// Extract from ratelimit-limit header (Docker Hub format)
	if matches, exists := headers["ratelimit-limit"]; exists {
		if matchList, ok := matches.([][]string); ok && len(matchList) > 0 && len(matchList[0]) > 2 {
			if limit, err := strconv.Atoi(matchList[0][1]); err == nil {
				info.Limit = limit
			}
			if window, err := strconv.Atoi(matchList[0][2]); err == nil {
				info.Window = time.Duration(window) * time.Second
				info.ResetTime = time.Now().Add(info.Window)
			}
		}
	}

	// Extract from various limit headers
	limitHeaders := []string{"x-goog-quota-limit", "x-amzn-ratelimit", "x-rate-limit", "rate-limit", "quota-limit"}
	for _, header := range limitHeaders {
		if matches, exists := headers[header]; exists {
			if matchList, ok := matches.([][]string); ok && len(matchList) > 0 && len(matchList[0]) > 1 {
				if limit, err := strconv.Atoi(matchList[0][1]); err == nil {
					info.Limit = limit
					break
				}
			}
		}
	}

	// Extract remaining from various headers
	remainingHeaders := []string{"x-goog-quota-remaining", "x-ms-ratelimit-remaining"}
	for _, header := range remainingHeaders {
		if matches, exists := headers[header]; exists {
			if matchList, ok := matches.([][]string); ok && len(matchList) > 0 && len(matchList[0]) > 1 {
				if remaining, err := strconv.Atoi(matchList[0][1]); err == nil {
					info.Remaining = remaining
					break
				}
			}
		}
	}

	// Check for 429 responses
	if _, exists := headers["http-429"]; exists {
		info.Last429Response = &[]time.Time{time.Now()}[0]
		info.FailedRequests = 1
	}

	// If no specific limit found, try to infer from context
	if info.Limit == 0 {
		info.Limit = ep.inferRateLimitFromContext(output)
	}
}

// inferRateLimitFromContext attempts to infer rate limits from context clues
func (ep *EnhancedParser) inferRateLimitFromContext(output string) int {
	// Common rate limit values based on service patterns
	if strings.Contains(strings.ToLower(output), "github") {
		return 5000 // GitHub API default
	}
	if strings.Contains(strings.ToLower(output), "docker") {
		return 100 // Docker Hub default for anonymous
	}
	if strings.Contains(strings.ToLower(output), "kubernetes") {
		return 400 // Kubernetes API server default
	}
	if strings.Contains(strings.ToLower(output), "aws") {
		return 1000 // AWS API Gateway default
	}

	// Default conservative estimate
	return 100
}

// hasRateLimitIndicators checks for general rate limiting indicators
func (ep *EnhancedParser) hasRateLimitIndicators(output string) bool {
	indicators := []string{
		"rate-limit-exceeded", "http-429", "status-429",
	}

	for _, indicator := range indicators {
		if ep.enhancedPatterns[indicator].MatchString(output) {
			return true
		}
	}

	return false
}

// min returns the minimum of two float64 values
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
