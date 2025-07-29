package discovery

import (
	"testing"
	"time"
)

func TestEnhancedParser_ParseFromCommandOutputEnhanced(t *testing.T) {
	parser := NewEnhancedParser()

	tests := []struct {
		name      string
		stdout    string
		stderr    string
		exitCode  int
		command   []string
		wantFound bool
		wantLimit int
		wantHost  string
	}{
		{
			name: "GitHub API rate limit headers",
			stdout: `HTTP/1.1 200 OK
X-RateLimit-Limit: 5000
X-RateLimit-Remaining: 4999
X-RateLimit-Reset: 1640995200
Content-Type: application/json`,
			command:   []string{"curl", "https://api.github.com/user"},
			wantFound: true,
			wantLimit: 5000,
			wantHost:  "api.github.com",
		},
		{
			name: "Docker Hub rate limit with enhanced patterns",
			stdout: `HTTP/1.1 200 OK
RateLimit-Limit: 100;w=21600
RateLimit-Remaining: 99;w=21600
Docker-RateLimit-Source: registry-1.docker.io`,
			command:   []string{"curl", "https://registry-1.docker.io/v2/library/nginx/manifests/latest"},
			wantFound: true,
			wantLimit: 100,
			wantHost:  "registry-1.docker.io",
		},
		{
			name: "AWS API Gateway headers",
			stdout: `HTTP/1.1 200 OK
X-Amzn-RequestId: 12345-67890
X-Amzn-RateLimit-Limit: 1000
Content-Type: application/json`,
			command:   []string{"curl", "https://api.amazonaws.com/v1/resource"},
			wantFound: true,
			wantLimit: 1000,
			wantHost:  "api.amazonaws.com",
		},
		{
			name: "Google Cloud API headers",
			stdout: `HTTP/1.1 200 OK
X-Goog-Quota-Limit: 10000
X-Goog-Quota-Remaining: 9999
Content-Type: application/json`,
			command:   []string{"curl", "https://compute.googleapis.com/compute/v1/projects/test/zones"},
			wantFound: true,
			wantLimit: 10000,
			wantHost:  "compute.googleapis.com",
		},
		{
			name: "HTTP 429 Too Many Requests",
			stdout: `HTTP/1.1 429 Too Many Requests
Retry-After: 60
Content-Type: application/json

{"error": "Rate limit exceeded"}`,
			exitCode:  429,
			command:   []string{"curl", "https://api.example.com/data"},
			wantFound: true,
			wantHost:  "api.example.com",
		},
		{
			name: "Kubernetes API headers",
			stdout: `HTTP/1.1 200 OK
X-Kubernetes-PF-FlowSchema-UID: abc-123
Content-Type: application/json`,
			command:   []string{"kubectl", "get", "pods"},
			wantFound: false, // No explicit rate limit, but should be detected by context
		},
		{
			name: "No rate limit information",
			stdout: `HTTP/1.1 200 OK
Content-Type: text/html

<html><body>Hello World</body></html>`,
			command:   []string{"curl", "https://example.com"},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.ParseFromCommandOutputEnhanced(tt.stdout, tt.stderr, tt.exitCode, tt.command)

			if result.Found != tt.wantFound {
				t.Errorf("ParseFromCommandOutputEnhanced() found = %v, want %v", result.Found, tt.wantFound)
				return
			}

			if !result.Found {
				return // No need to check other fields if not found
			}

			if result.Info == nil {
				t.Error("ParseFromCommandOutputEnhanced() info is nil when found = true")
				return
			}

			if tt.wantLimit > 0 && result.Info.Limit != tt.wantLimit {
				t.Errorf("ParseFromCommandOutputEnhanced() limit = %v, want %v", result.Info.Limit, tt.wantLimit)
			}

			if tt.wantHost != "" && result.Info.Host != tt.wantHost {
				t.Errorf("ParseFromCommandOutputEnhanced() host = %v, want %v", result.Info.Host, tt.wantHost)
			}

			// Verify confidence is reasonable
			if result.Confidence < 0.0 || result.Confidence > 1.0 {
				t.Errorf("ParseFromCommandOutputEnhanced() confidence = %v, want between 0.0 and 1.0", result.Confidence)
			}
		})
	}
}

func TestEnhancedParser_IdentifyAPIResource(t *testing.T) {
	parser := NewEnhancedParser()

	tests := []struct {
		name     string
		host     string
		path     string
		expected string
	}{
		{
			name:     "GitHub API",
			host:     "api.github.com",
			path:     "/repos/owner/repo/issues",
			expected: "github:/repos/*",
		},
		{
			name:     "AWS API",
			host:     "ec2.amazonaws.com",
			path:     "/v1/instances/i-1234567890abcdef0",
			expected: "aws:ec2.amazonaws.com/v1/instances/*",
		},
		{
			name:     "Docker Hub",
			host:     "registry-1.docker.io",
			path:     "/v2/library/nginx/manifests/latest",
			expected: "docker:/v2/*/manifests/*",
		},
		{
			name:     "Kubernetes API",
			host:     "kubernetes.default.svc",
			path:     "/api/v1/namespaces/default/pods",
			expected: "k8s:kubernetes.default.svc/api/v1/*",
		},
		{
			name:     "Google Cloud API",
			host:     "compute.googleapis.com",
			path:     "/compute/v1/projects/test/zones",
			expected: "gcp:compute.googleapis.com/compute/v1/*",
		},
		{
			name:     "Generic API",
			host:     "api.example.com",
			path:     "/v1/users/123",
			expected: "api.example.com/v1/users/123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.identifyAPIResource(tt.host, tt.path)
			if result != tt.expected {
				t.Errorf("identifyAPIResource() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEnhancedParser_InferRateLimitFromContext(t *testing.T) {
	parser := NewEnhancedParser()

	tests := []struct {
		name     string
		output   string
		expected int
	}{
		{
			name:     "GitHub context",
			output:   "api.github.com returned rate limit information",
			expected: 5000,
		},
		{
			name:     "Docker context",
			output:   "registry-1.docker.io rate limit exceeded",
			expected: 100,
		},
		{
			name:     "Kubernetes context",
			output:   "kubernetes api server responded",
			expected: 400,
		},
		{
			name:     "AWS context",
			output:   "amazonaws.com throttling detected",
			expected: 1000,
		},
		{
			name:     "Unknown context",
			output:   "some random api response",
			expected: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.inferRateLimitFromContext(tt.output)
			if result != tt.expected {
				t.Errorf("inferRateLimitFromContext() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEnhancedParser_HasRateLimitIndicators(t *testing.T) {
	parser := NewEnhancedParser()

	tests := []struct {
		name     string
		output   string
		expected bool
	}{
		{
			name:     "HTTP 429 response",
			output:   "HTTP/1.1 429 Too Many Requests",
			expected: true,
		},
		{
			name:     "Rate limit exceeded message",
			output:   "Error: rate limit exceeded, please try again later",
			expected: true,
		},
		{
			name:     "Too many requests message",
			output:   "too many requests from your IP address",
			expected: true,
		},
		{
			name:     "Quota exceeded message",
			output:   "quota exceeded for this resource",
			expected: true,
		},
		{
			name:     "Throttled message",
			output:   "request was throttled by the server",
			expected: true,
		},
		{
			name:     "No rate limit indicators",
			output:   "successful response with data",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parser.hasRateLimitIndicators(tt.output)
			if result != tt.expected {
				t.Errorf("hasRateLimitIndicators() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEnhancedParser_NormalizePaths(t *testing.T) {
	parser := NewEnhancedParser()

	tests := []struct {
		name     string
		method   func(string) string
		input    string
		expected string
	}{
		{
			name:     "AWS path normalization",
			method:   parser.normalizeAWSPath,
			input:    "/v1/instances/i-1234567890abcdef0/status",
			expected: "/v1/instances/*",
		},
		{
			name:     "GCP path normalization",
			method:   parser.normalizeGCPPath,
			input:    "/compute/v1/projects/test/zones/us-central1-a/instances",
			expected: "/v1/projects/*",
		},
		{
			name:     "GitHub path normalization",
			method:   parser.normalizeGitHubPath,
			input:    "/repos/owner/repo/issues/123",
			expected: "/repos/*",
		},
		{
			name:     "Docker path normalization - manifests",
			method:   parser.normalizeDockerPath,
			input:    "/v2/library/nginx/manifests/latest",
			expected: "/v2/*/manifests/*",
		},
		{
			name:     "Docker path normalization - blobs",
			method:   parser.normalizeDockerPath,
			input:    "/v2/library/nginx/blobs/sha256:abc123",
			expected: "/v2/*/blobs/*",
		},
		{
			name:     "Kubernetes API path normalization",
			method:   parser.normalizeK8sPath,
			input:    "/api/v1/namespaces/default/pods/nginx-123",
			expected: "/api/v1/*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.method(tt.input)
			if result != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestEnhancedParser_ExtractEnhancedHeaders(t *testing.T) {
	parser := NewEnhancedParser()

	output := `HTTP/1.1 200 OK
RateLimit-Policy: 100;w=3600
X-RateLimit-Window: 3600
X-Goog-Quota-Limit: 10000
X-MS-RateLimit-Remaining-tenant-reads: 11999
Docker-RateLimit-Source: registry-1.docker.io
Content-Type: application/json`

	headers := parser.extractEnhancedHeaders(output)

	expectedHeaders := []string{
		"ratelimit-policy",
		"x-ratelimit-window",
		"x-goog-quota-limit",
		"x-ms-ratelimit-remaining",
		"docker-ratelimit-source",
	}

	for _, expected := range expectedHeaders {
		if _, exists := headers[expected]; !exists {
			t.Errorf("Expected header %s not found in extracted headers", expected)
		}
	}

	// Test that we have rate limit info
	if !parser.hasEnhancedRateLimitInfo(headers) {
		t.Error("hasEnhancedRateLimitInfo() should return true for this output")
	}
}

func TestEnhancedParser_Integration(t *testing.T) {
	parser := NewEnhancedParser()

	// Test a complex real-world scenario
	stdout := `HTTP/1.1 429 Too Many Requests
RateLimit-Policy: 100;w=3600
RateLimit-Remaining: 0;w=3600
Retry-After: 3600
Docker-RateLimit-Source: registry-1.docker.io
Content-Type: application/json

{
  "errors": [
    {
      "code": "TOOMANYREQUESTS",
      "message": "You have reached your pull rate limit. You may increase the limit by authenticating and upgrading: https://www.docker.com/increase-rate-limits"
    }
  ]
}`

	command := []string{"curl", "-H", "Accept: application/vnd.docker.distribution.manifest.v2+json",
		"https://registry-1.docker.io/v2/library/nginx/manifests/latest"}

	result := parser.ParseFromCommandOutputEnhanced(stdout, "", 429, command)

	// Verify the result
	if !result.Found {
		t.Error("Expected to find rate limit information")
		return
	}

	if result.Info == nil {
		t.Error("Expected rate limit info to be populated")
		return
	}

	// Check specific values - the limit should be extracted from the RateLimit-Policy header
	if result.Info.Limit == 0 {
		t.Errorf("Expected limit to be extracted from headers, got %d", result.Info.Limit)
	}

	if result.Info.Host != "registry-1.docker.io" {
		t.Errorf("Expected host registry-1.docker.io, got %s", result.Info.Host)
	}

	if result.Info.ResourceID != "docker:/v2/*/manifests/*" {
		t.Errorf("Expected resourceID docker:/v2/*/manifests/*, got %s", result.Info.ResourceID)
	}

	if result.Info.Window != time.Hour {
		t.Errorf("Expected window 1 hour, got %v", result.Info.Window)
	}

	if result.Info.Last429Response == nil {
		t.Error("Expected Last429Response to be set for 429 status")
	}

	if result.Confidence < 0.3 {
		t.Errorf("Expected reasonable confidence (>0.3), got %f", result.Confidence)
	}

	t.Logf("Integration test passed: %+v", result.Info)
}
