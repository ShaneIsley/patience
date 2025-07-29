package discovery

import (
	"testing"
	"time"
)

func TestResourceGrouper_GroupResource(t *testing.T) {
	grouper := NewResourceGrouper()

	tests := []struct {
		name          string
		resourceID    string
		host          string
		path          string
		expectedGroup string
		expectedShare bool
		expectedLimit int
	}{
		{
			name:          "GitHub API Issues",
			resourceID:    "github.com/repos/owner/repo/issues",
			host:          "api.github.com",
			path:          "/repos/owner/repo/issues",
			expectedGroup: "GitHub API Core",
			expectedShare: true,
			expectedLimit: 5000,
		},
		{
			name:          "GitHub Search API",
			resourceID:    "github.com/search/repositories",
			host:          "api.github.com",
			path:          "/search/repositories",
			expectedGroup: "GitHub API Search",
			expectedShare: true,
			expectedLimit: 30,
		},
		{
			name:          "Docker Hub Manifests",
			resourceID:    "registry-1.docker.io/v2/library/nginx/manifests/latest",
			host:          "registry-1.docker.io",
			path:          "/v2/library/nginx/manifests/latest",
			expectedGroup: "Docker Hub Registry",
			expectedShare: true,
			expectedLimit: 100,
		},
		{
			name:          "AWS API",
			resourceID:    "ec2.amazonaws.com/v1/instances",
			host:          "ec2.amazonaws.com",
			path:          "/v1/instances",
			expectedGroup: "AWS API Gateway",
			expectedShare: false,
			expectedLimit: 1000,
		},
		{
			name:          "Google Cloud API",
			resourceID:    "compute.googleapis.com/compute/v1/projects",
			host:          "compute.googleapis.com",
			path:          "/compute/v1/projects",
			expectedGroup: "Google Cloud APIs",
			expectedShare: false,
			expectedLimit: 100,
		},
		{
			name:          "Kubernetes Core API",
			resourceID:    "kubernetes.default.svc/api/v1/pods",
			host:          "kubernetes.default.svc",
			path:          "/api/v1/pods",
			expectedGroup: "Kubernetes API Core",
			expectedShare: true,
			expectedLimit: 400,
		},
		{
			name:          "Generic REST API",
			resourceID:    "api.example.com/api/v1/users",
			host:          "api.example.com",
			path:          "/api/v1/users",
			expectedGroup: "REST API Generic",
			expectedShare: false,
			expectedLimit: 100,
		},
		{
			name:          "GraphQL Endpoint",
			resourceID:    "api.example.com/graphql",
			host:          "api.example.com",
			path:          "/graphql",
			expectedGroup: "GraphQL Endpoints",
			expectedShare: true,
			expectedLimit: 60,
		},
		{
			name:          "Unknown Resource",
			resourceID:    "unknown.example.com/some/path",
			host:          "unknown.example.com",
			path:          "/some/path",
			expectedGroup: "Default HTTP",
			expectedShare: false,
			expectedLimit: 60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			group := grouper.GroupResource(tt.resourceID, tt.host, tt.path)

			if group.Name != tt.expectedGroup {
				t.Errorf("GroupResource() group name = %v, want %v", group.Name, tt.expectedGroup)
			}

			if group.Config.ShareRateLimit != tt.expectedShare {
				t.Errorf("GroupResource() share rate limit = %v, want %v", group.Config.ShareRateLimit, tt.expectedShare)
			}

			if group.Config.DefaultLimit != tt.expectedLimit {
				t.Errorf("GroupResource() default limit = %v, want %v", group.Config.DefaultLimit, tt.expectedLimit)
			}

			// Verify group ID is generated correctly
			if group.ID == "" {
				t.Error("GroupResource() group ID should not be empty")
			}

			// Verify metadata is populated
			if group.Metadata["host"] != tt.host {
				t.Errorf("GroupResource() metadata host = %v, want %v", group.Metadata["host"], tt.host)
			}

			if group.Metadata["path"] != tt.path {
				t.Errorf("GroupResource() metadata path = %v, want %v", group.Metadata["path"], tt.path)
			}
		})
	}
}

func TestResourceGrouper_GetDefaultRateLimit(t *testing.T) {
	grouper := NewResourceGrouper()

	tests := []struct {
		name           string
		resourceID     string
		host           string
		path           string
		expectedLimit  int
		expectedWindow time.Duration
	}{
		{
			name:           "GitHub API",
			resourceID:     "github.com/repos/owner/repo/issues",
			host:           "api.github.com",
			path:           "/repos/owner/repo/issues",
			expectedLimit:  5000,
			expectedWindow: time.Hour,
		},
		{
			name:           "Docker Hub",
			resourceID:     "registry-1.docker.io/v2/library/nginx/manifests/latest",
			host:           "registry-1.docker.io",
			path:           "/v2/library/nginx/manifests/latest",
			expectedLimit:  100,
			expectedWindow: 6 * time.Hour,
		},
		{
			name:           "Default",
			resourceID:     "unknown.example.com/path",
			host:           "unknown.example.com",
			path:           "/path",
			expectedLimit:  60,
			expectedWindow: time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit, window := grouper.GetDefaultRateLimit(tt.resourceID, tt.host, tt.path)

			if limit != tt.expectedLimit {
				t.Errorf("GetDefaultRateLimit() limit = %v, want %v", limit, tt.expectedLimit)
			}

			if window != tt.expectedWindow {
				t.Errorf("GetDefaultRateLimit() window = %v, want %v", window, tt.expectedWindow)
			}
		})
	}
}

func TestResourceGrouper_ShouldShareRateLimit(t *testing.T) {
	grouper := NewResourceGrouper()

	tests := []struct {
		name          string
		resourceID    string
		host          string
		path          string
		expectedShare bool
	}{
		{
			name:          "GitHub API (shared)",
			resourceID:    "github.com/repos/owner/repo/issues",
			host:          "api.github.com",
			path:          "/repos/owner/repo/issues",
			expectedShare: true,
		},
		{
			name:          "AWS API (individual)",
			resourceID:    "ec2.amazonaws.com/v1/instances",
			host:          "ec2.amazonaws.com",
			path:          "/v1/instances",
			expectedShare: false,
		},
		{
			name:          "Docker Hub (shared)",
			resourceID:    "registry-1.docker.io/v2/library/nginx/manifests/latest",
			host:          "registry-1.docker.io",
			path:          "/v2/library/nginx/manifests/latest",
			expectedShare: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			share := grouper.ShouldShareRateLimit(tt.resourceID, tt.host, tt.path)

			if share != tt.expectedShare {
				t.Errorf("ShouldShareRateLimit() = %v, want %v", share, tt.expectedShare)
			}
		})
	}
}

func TestResourceGrouper_NormalizeResourceID(t *testing.T) {
	grouper := NewResourceGrouper()

	tests := []struct {
		name       string
		resourceID string
		host       string
		path       string
		expectSame bool // Whether normalized ID should be same as original
	}{
		{
			name:       "GitHub API (shared - should normalize)",
			resourceID: "github.com/repos/owner/repo/issues",
			host:       "api.github.com",
			path:       "/repos/owner/repo/issues",
			expectSame: false, // Should be normalized to group ID
		},
		{
			name:       "AWS API (individual - should keep same)",
			resourceID: "ec2.amazonaws.com/v1/instances",
			host:       "ec2.amazonaws.com",
			path:       "/v1/instances",
			expectSame: true, // Should keep original ID
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := grouper.NormalizeResourceID(tt.resourceID, tt.host, tt.path)

			if tt.expectSame {
				if normalized != tt.resourceID {
					t.Errorf("NormalizeResourceID() = %v, want %v (same as original)", normalized, tt.resourceID)
				}
			} else {
				if normalized == tt.resourceID {
					t.Errorf("NormalizeResourceID() = %v, expected to be different from original %v", normalized, tt.resourceID)
				}
			}

			// Normalized ID should never be empty
			if normalized == "" {
				t.Error("NormalizeResourceID() should not return empty string")
			}
		})
	}
}

func TestResourceGrouper_AddUpdateRemoveResourceGroup(t *testing.T) {
	grouper := NewResourceGrouper()

	// Test adding a new resource group
	newConfig := &ResourceGroupConfig{
		Name:           "Test API",
		Pattern:        `test\.example\.com/api/`,
		DefaultLimit:   200,
		DefaultWindow:  30 * time.Second,
		ShareRateLimit: true,
		Priority:       5,
		Description:    "Test API for unit testing",
	}

	err := grouper.AddResourceGroup(newConfig)
	if err != nil {
		t.Errorf("AddResourceGroup() error = %v", err)
	}

	// Verify it was added
	groups := grouper.GetResourceGroups()
	if _, exists := groups["Test API"]; !exists {
		t.Error("AddResourceGroup() did not add the group")
	}

	// Test updating the resource group
	updatedConfig := &ResourceGroupConfig{
		Name:           "Test API",
		Pattern:        `test\.example\.com/api/v\d+/`,
		DefaultLimit:   300,
		DefaultWindow:  45 * time.Second,
		ShareRateLimit: false,
		Priority:       6,
		Description:    "Updated test API",
	}

	err = grouper.UpdateResourceGroup("Test API", updatedConfig)
	if err != nil {
		t.Errorf("UpdateResourceGroup() error = %v", err)
	}

	// Verify it was updated
	groups = grouper.GetResourceGroups()
	if config, exists := groups["Test API"]; exists {
		if config.DefaultLimit != 300 {
			t.Errorf("UpdateResourceGroup() limit = %v, want 300", config.DefaultLimit)
		}
		if config.ShareRateLimit != false {
			t.Errorf("UpdateResourceGroup() share rate limit = %v, want false", config.ShareRateLimit)
		}
	} else {
		t.Error("UpdateResourceGroup() group not found after update")
	}

	// Test removing the resource group
	grouper.RemoveResourceGroup("Test API")

	// Verify it was removed
	groups = grouper.GetResourceGroups()
	if _, exists := groups["Test API"]; exists {
		t.Error("RemoveResourceGroup() did not remove the group")
	}
}

func TestResourceGrouper_AnalyzeResourcePattern(t *testing.T) {
	grouper := NewResourceGrouper()

	tests := []struct {
		name      string
		resources []string
		wantHost  string
		wantNil   bool
	}{
		{
			name: "GitHub resources",
			resources: []string{
				"https://api.github.com/repos/owner1/repo1/issues",
				"https://api.github.com/repos/owner1/repo1/pulls",
				"https://api.github.com/repos/owner2/repo2/issues",
			},
			wantHost: "api.github.com",
			wantNil:  false,
		},
		{
			name: "Mixed resources",
			resources: []string{
				"https://api.example.com/v1/users/123",
				"https://api.example.com/v1/users/456",
				"https://api.example.com/v1/posts/789",
			},
			wantHost: "api.example.com",
			wantNil:  false,
		},
		{
			name:      "Empty resources",
			resources: []string{},
			wantNil:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestion := grouper.AnalyzeResourcePattern(tt.resources)

			if tt.wantNil {
				if suggestion != nil {
					t.Errorf("AnalyzeResourcePattern() = %v, want nil", suggestion)
				}
				return
			}

			if suggestion == nil {
				t.Error("AnalyzeResourcePattern() = nil, want non-nil")
				return
			}

			if suggestion.CommonHost != tt.wantHost {
				t.Errorf("AnalyzeResourcePattern() common host = %v, want %v", suggestion.CommonHost, tt.wantHost)
			}

			if suggestion.ResourceCount != len(tt.resources) {
				t.Errorf("AnalyzeResourcePattern() resource count = %v, want %v", suggestion.ResourceCount, len(tt.resources))
			}

			if suggestion.Confidence < 0.0 || suggestion.Confidence > 1.0 {
				t.Errorf("AnalyzeResourcePattern() confidence = %v, want between 0.0 and 1.0", suggestion.Confidence)
			}
		})
	}
}

func TestResourceGrouper_GetResourceGroupStats(t *testing.T) {
	grouper := NewResourceGrouper()

	stats := grouper.GetResourceGroupStats()

	// Verify basic structure
	if totalGroups, ok := stats["total_groups"].(int); !ok || totalGroups == 0 {
		t.Errorf("GetResourceGroupStats() total_groups = %v, want > 0", stats["total_groups"])
	}

	if compiledPatterns, ok := stats["compiled_patterns"].(int); !ok || compiledPatterns == 0 {
		t.Errorf("GetResourceGroupStats() compiled_patterns = %v, want > 0", stats["compiled_patterns"])
	}

	if groups, ok := stats["groups"].(map[string]interface{}); !ok || len(groups) == 0 {
		t.Errorf("GetResourceGroupStats() groups = %v, want non-empty map", stats["groups"])
	}

	// Verify that default groups are present
	groups := stats["groups"].(map[string]interface{})
	expectedGroups := []string{"GitHub API Core", "Docker Hub Registry", "Default HTTP"}

	for _, expectedGroup := range expectedGroups {
		if _, exists := groups[expectedGroup]; !exists {
			t.Errorf("GetResourceGroupStats() missing expected group: %s", expectedGroup)
		}
	}
}

func TestResourceGrouper_InvalidPattern(t *testing.T) {
	grouper := NewResourceGrouper()

	// Test adding a resource group with invalid regex pattern
	invalidConfig := &ResourceGroupConfig{
		Name:           "Invalid Pattern",
		Pattern:        `[invalid regex pattern`,
		DefaultLimit:   100,
		DefaultWindow:  time.Minute,
		ShareRateLimit: true,
		Priority:       1,
		Description:    "Invalid pattern for testing",
	}

	err := grouper.AddResourceGroup(invalidConfig)
	if err == nil {
		t.Error("AddResourceGroup() expected error for invalid pattern, got nil")
	}
}
