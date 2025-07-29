package discovery

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ResourceGrouper provides intelligent resource grouping for rate limit management
type ResourceGrouper struct {
	// Compiled patterns for performance
	patterns map[string]*regexp.Regexp
	// Resource group configurations
	groupConfigs map[string]*ResourceGroupConfig
}

// ResourceGroupConfig defines how resources should be grouped and managed
type ResourceGroupConfig struct {
	Name           string        `json:"name"`
	Pattern        string        `json:"pattern"`
	DefaultLimit   int           `json:"default_limit"`
	DefaultWindow  time.Duration `json:"default_window"`
	ShareRateLimit bool          `json:"share_rate_limit"` // Whether resources in this group share rate limits
	Priority       int           `json:"priority"`         // Higher priority groups are checked first
	Description    string        `json:"description"`
}

// ResourceGroup represents a group of related resources that share rate limits
type ResourceGroup struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Pattern     string                 `json:"pattern"`
	Resources   []string               `json:"resources"`
	RateLimit   *RateLimitInfo         `json:"rate_limit,omitempty"`
	Config      *ResourceGroupConfig   `json:"config"`
	Metadata    map[string]interface{} `json:"metadata"`
	LastUpdated time.Time              `json:"last_updated"`
}

// NewResourceGrouper creates a new resource grouper with predefined patterns
func NewResourceGrouper() *ResourceGrouper {
	grouper := &ResourceGrouper{
		patterns:     make(map[string]*regexp.Regexp),
		groupConfigs: make(map[string]*ResourceGroupConfig),
	}

	grouper.initializeDefaultConfigs()
	grouper.compilePatterns()

	return grouper
}

// initializeDefaultConfigs sets up default resource group configurations
func (rg *ResourceGrouper) initializeDefaultConfigs() {
	configs := []*ResourceGroupConfig{
		{
			Name:           "GitHub API Core",
			Pattern:        `github\.com/repos/[^/]+/[^/]+/(issues|pulls|commits)`,
			DefaultLimit:   5000,
			DefaultWindow:  time.Hour,
			ShareRateLimit: true,
			Priority:       10,
			Description:    "GitHub repository core operations (issues, PRs, commits)",
		},
		{
			Name:           "GitHub API Search",
			Pattern:        `github\.com/search/`,
			DefaultLimit:   30,
			DefaultWindow:  time.Minute,
			ShareRateLimit: true,
			Priority:       9,
			Description:    "GitHub search API (more restrictive limits)",
		},
		{
			Name:           "Docker Hub Registry",
			Pattern:        `registry-1\.docker\.io/v2/[^/]+/[^/]+/(manifests|blobs)`,
			DefaultLimit:   100,
			DefaultWindow:  6 * time.Hour,
			ShareRateLimit: true,
			Priority:       8,
			Description:    "Docker Hub registry operations",
		},
		{
			Name:           "AWS API Gateway",
			Pattern:        `[^.]+\.amazonaws\.com/`,
			DefaultLimit:   1000,
			DefaultWindow:  time.Second,
			ShareRateLimit: false, // AWS APIs have per-service limits
			Priority:       7,
			Description:    "AWS API Gateway endpoints",
		},
		{
			Name:           "Google Cloud APIs",
			Pattern:        `[^.]+\.googleapis\.com/`,
			DefaultLimit:   100,
			DefaultWindow:  time.Second,
			ShareRateLimit: false, // GCP APIs have per-service limits
			Priority:       6,
			Description:    "Google Cloud Platform APIs",
		},
		{
			Name:           "Kubernetes API Core",
			Pattern:        `kubernetes[^/]*/api/v1/(pods|services|configmaps|secrets)`,
			DefaultLimit:   400,
			DefaultWindow:  time.Second,
			ShareRateLimit: true,
			Priority:       5,
			Description:    "Kubernetes core API resources",
		},
		{
			Name:           "Kubernetes API Extensions",
			Pattern:        `kubernetes[^/]*/apis/[^/]+/v[^/]+/`,
			DefaultLimit:   200,
			DefaultWindow:  time.Second,
			ShareRateLimit: true,
			Priority:       4,
			Description:    "Kubernetes extension APIs",
		},
		{
			Name:           "REST API Generic",
			Pattern:        `/api/v\d+/`,
			DefaultLimit:   100,
			DefaultWindow:  time.Minute,
			ShareRateLimit: false,
			Priority:       3,
			Description:    "Generic REST API endpoints",
		},
		{
			Name:           "GraphQL Endpoints",
			Pattern:        `/graphql`,
			DefaultLimit:   60,
			DefaultWindow:  time.Minute,
			ShareRateLimit: true,
			Priority:       2,
			Description:    "GraphQL API endpoints",
		},
		{
			Name:           "Default HTTP",
			Pattern:        `.*`,
			DefaultLimit:   60,
			DefaultWindow:  time.Minute,
			ShareRateLimit: false,
			Priority:       1,
			Description:    "Default rate limits for unmatched resources",
		},
	}

	for _, config := range configs {
		rg.groupConfigs[config.Name] = config
	}
}

// compilePatterns compiles all regex patterns for performance
func (rg *ResourceGrouper) compilePatterns() {
	for name, config := range rg.groupConfigs {
		if pattern, err := regexp.Compile(config.Pattern); err == nil {
			rg.patterns[name] = pattern
		}
	}
}

// GroupResource determines which resource group a resource belongs to
func (rg *ResourceGrouper) GroupResource(resourceID, host, path string) *ResourceGroup {
	fullResource := host + path

	// Check patterns in priority order
	var matchedConfig *ResourceGroupConfig
	var matchedName string

	for name, config := range rg.groupConfigs {
		if pattern, exists := rg.patterns[name]; exists {
			if pattern.MatchString(fullResource) {
				if matchedConfig == nil || config.Priority > matchedConfig.Priority {
					matchedConfig = config
					matchedName = name
				}
			}
		}
	}

	if matchedConfig == nil {
		// Fallback to default
		matchedConfig = rg.groupConfigs["Default HTTP"]
		matchedName = "Default HTTP"
	}

	// Generate group ID
	groupID := rg.generateGroupID(matchedName, fullResource, matchedConfig.ShareRateLimit)

	return &ResourceGroup{
		ID:        groupID,
		Name:      matchedName,
		Pattern:   matchedConfig.Pattern,
		Resources: []string{resourceID},
		Config:    matchedConfig,
		Metadata: map[string]interface{}{
			"host":          host,
			"path":          path,
			"full_resource": fullResource,
			"share_limits":  matchedConfig.ShareRateLimit,
		},
		LastUpdated: time.Now(),
	}
}

// generateGroupID creates a unique ID for a resource group
func (rg *ResourceGrouper) generateGroupID(groupName, resource string, shareRateLimit bool) string {
	if shareRateLimit {
		// All resources in this group share the same ID
		hash := sha256.Sum256([]byte(groupName))
		return fmt.Sprintf("group_%s_%x", strings.ToLower(strings.ReplaceAll(groupName, " ", "_")), hash[:4])
	} else {
		// Each resource gets its own group ID
		hash := sha256.Sum256([]byte(groupName + ":" + resource))
		return fmt.Sprintf("resource_%x", hash[:8])
	}
}

// GetResourceGroups returns all configured resource groups
func (rg *ResourceGrouper) GetResourceGroups() map[string]*ResourceGroupConfig {
	return rg.groupConfigs
}

// AddResourceGroup adds a new resource group configuration
func (rg *ResourceGrouper) AddResourceGroup(config *ResourceGroupConfig) error {
	// Validate pattern
	if _, err := regexp.Compile(config.Pattern); err != nil {
		return fmt.Errorf("invalid pattern %s: %w", config.Pattern, err)
	}

	rg.groupConfigs[config.Name] = config
	rg.patterns[config.Name] = regexp.MustCompile(config.Pattern)

	return nil
}

// RemoveResourceGroup removes a resource group configuration
func (rg *ResourceGrouper) RemoveResourceGroup(name string) {
	delete(rg.groupConfigs, name)
	delete(rg.patterns, name)
}

// UpdateResourceGroup updates an existing resource group configuration
func (rg *ResourceGrouper) UpdateResourceGroup(name string, config *ResourceGroupConfig) error {
	if _, exists := rg.groupConfigs[name]; !exists {
		return fmt.Errorf("resource group %s does not exist", name)
	}

	// Validate pattern
	if _, err := regexp.Compile(config.Pattern); err != nil {
		return fmt.Errorf("invalid pattern %s: %w", config.Pattern, err)
	}

	rg.groupConfigs[name] = config
	rg.patterns[name] = regexp.MustCompile(config.Pattern)

	return nil
}

// GetDefaultRateLimit returns the default rate limit for a resource
func (rg *ResourceGrouper) GetDefaultRateLimit(resourceID, host, path string) (int, time.Duration) {
	group := rg.GroupResource(resourceID, host, path)
	return group.Config.DefaultLimit, group.Config.DefaultWindow
}

// ShouldShareRateLimit determines if resources should share rate limits
func (rg *ResourceGrouper) ShouldShareRateLimit(resourceID, host, path string) bool {
	group := rg.GroupResource(resourceID, host, path)
	return group.Config.ShareRateLimit
}

// NormalizeResourceID normalizes a resource ID for consistent grouping
func (rg *ResourceGrouper) NormalizeResourceID(resourceID, host, path string) string {
	group := rg.GroupResource(resourceID, host, path)

	if group.Config.ShareRateLimit {
		// Use the group ID for shared rate limits
		return group.ID
	}

	// Use the original resource ID for individual rate limits
	return resourceID
}

// GetResourceGroupStats returns statistics about resource grouping
func (rg *ResourceGrouper) GetResourceGroupStats() map[string]interface{} {
	stats := map[string]interface{}{
		"total_groups":      len(rg.groupConfigs),
		"compiled_patterns": len(rg.patterns),
		"groups":            make(map[string]interface{}),
	}

	groups := make(map[string]interface{})
	for name, config := range rg.groupConfigs {
		groups[name] = map[string]interface{}{
			"pattern":          config.Pattern,
			"default_limit":    config.DefaultLimit,
			"default_window":   config.DefaultWindow.String(),
			"share_rate_limit": config.ShareRateLimit,
			"priority":         config.Priority,
			"description":      config.Description,
		}
	}
	stats["groups"] = groups

	return stats
}

// AnalyzeResourcePattern analyzes a resource pattern and suggests grouping
func (rg *ResourceGrouper) AnalyzeResourcePattern(resources []string) *ResourceGroupSuggestion {
	if len(resources) == 0 {
		return nil
	}

	// Analyze common patterns in the resources
	hostCounts := make(map[string]int)
	pathPatterns := make(map[string]int)

	for _, resource := range resources {
		if u, err := url.Parse(resource); err == nil {
			hostCounts[u.Host]++

			// Extract path pattern
			pathPattern := rg.extractPathPattern(u.Path)
			pathPatterns[pathPattern]++
		}
	}

	// Find the most common host and path pattern
	var commonHost string
	var commonPathPattern string
	maxHostCount := 0
	maxPathCount := 0

	for host, count := range hostCounts {
		if count > maxHostCount {
			maxHostCount = count
			commonHost = host
		}
	}

	for pattern, count := range pathPatterns {
		if count > maxPathCount {
			maxPathCount = count
			commonPathPattern = pattern
		}
	}

	// Generate suggestion
	suggestion := &ResourceGroupSuggestion{
		SuggestedName:    fmt.Sprintf("Custom %s", commonHost),
		SuggestedPattern: fmt.Sprintf(`%s%s`, regexp.QuoteMeta(commonHost), commonPathPattern),
		ResourceCount:    len(resources),
		CommonHost:       commonHost,
		CommonPath:       commonPathPattern,
		Confidence:       float64(maxHostCount+maxPathCount) / float64(len(resources)*2),
		Resources:        resources,
	}

	return suggestion
}

// extractPathPattern extracts a generalized pattern from a path
func (rg *ResourceGrouper) extractPathPattern(path string) string {
	// Replace numeric IDs with wildcards
	pattern := regexp.MustCompile(`/\d+`).ReplaceAllString(path, `/\d+`)
	// Replace UUIDs with wildcards
	pattern = regexp.MustCompile(`/[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`).ReplaceAllString(pattern, `/[a-f0-9-]+`)
	// Replace other IDs with wildcards
	pattern = regexp.MustCompile(`/[a-zA-Z0-9_-]{8,}`).ReplaceAllString(pattern, `/[a-zA-Z0-9_-]+`)

	return pattern
}

// ResourceGroupSuggestion represents a suggested resource grouping
type ResourceGroupSuggestion struct {
	SuggestedName    string   `json:"suggested_name"`
	SuggestedPattern string   `json:"suggested_pattern"`
	ResourceCount    int      `json:"resource_count"`
	CommonHost       string   `json:"common_host"`
	CommonPath       string   `json:"common_path"`
	Confidence       float64  `json:"confidence"`
	Resources        []string `json:"resources"`
}
