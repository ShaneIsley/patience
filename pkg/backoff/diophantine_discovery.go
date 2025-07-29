package backoff

import (
	"time"

	"github.com/shaneisley/patience/pkg/discovery"
)

// DiophantineWithDiscovery extends the Diophantine strategy with rate limit discovery
type DiophantineWithDiscovery struct {
	*DiophantineStrategy
	discoveryService  *discovery.Service
	fallbackRateLimit int
	fallbackWindow    time.Duration
}

// NewDiophantineWithDiscovery creates a new Diophantine strategy with discovery support
func NewDiophantineWithDiscovery(
	fallbackRateLimit int,
	fallbackWindow time.Duration,
	retryOffsets []time.Duration,
	discoveryService *discovery.Service,
) *DiophantineWithDiscovery {
	// Create base Diophantine strategy with fallback values
	base := NewDiophantine(fallbackRateLimit, fallbackWindow, retryOffsets)

	return &DiophantineWithDiscovery{
		DiophantineStrategy: base,
		discoveryService:    discoveryService,
		fallbackRateLimit:   fallbackRateLimit,
		fallbackWindow:      fallbackWindow,
	}
}

// GetRateLimitForCommand gets the rate limit for a specific command, using discovery if available
func (d *DiophantineWithDiscovery) GetRateLimitForCommand(command []string) (int, time.Duration) {
	// If discovery is not available or disabled, use fallback values
	if d.discoveryService == nil || !d.discoveryService.IsEnabled() {
		return d.fallbackRateLimit, d.fallbackWindow
	}

	// Try to get discovered rate limit information
	info, err := d.discoveryService.GetRateLimitForCommand(command)
	if err != nil || info == nil {
		// No discovered information, use fallback
		return d.fallbackRateLimit, d.fallbackWindow
	}

	// Check if the discovered information is reliable enough
	if info.Confidence < 0.5 || info.IsExpired() {
		// Low confidence or expired, use fallback
		return d.fallbackRateLimit, d.fallbackWindow
	}

	// Use discovered rate limit
	return info.Limit, info.Window
}

// UpdateRateLimitFromDiscovery updates the internal rate limit based on discovery
func (d *DiophantineWithDiscovery) UpdateRateLimitFromDiscovery(command []string) {
	rateLimit, window := d.GetRateLimitForCommand(command)

	// Update the base strategy with discovered values
	d.rateLimit = rateLimit
	d.window = window
}

// CanScheduleRequestWithDiscovery checks if a request can be scheduled using discovered rate limits
func (d *DiophantineWithDiscovery) CanScheduleRequestWithDiscovery(existing []time.Time, newRequestTime time.Time, command []string) bool {
	// Update rate limits from discovery before checking
	d.UpdateRateLimitFromDiscovery(command)

	// Use the base Diophantine logic with updated rate limits
	return d.CanScheduleRequest(existing, newRequestTime)
}

// GetDiscoveredRateLimit returns the currently discovered rate limit information
func (d *DiophantineWithDiscovery) GetDiscoveredRateLimit(command []string) *discovery.RateLimitInfo {
	if d.discoveryService == nil || !d.discoveryService.IsEnabled() {
		return nil
	}

	info, err := d.discoveryService.GetRateLimitForCommand(command)
	if err != nil {
		return nil
	}

	return info
}

// ProcessCommandOutput processes command output for discovery learning
func (d *DiophantineWithDiscovery) ProcessCommandOutput(stdout, stderr string, exitCode int, command []string, responseTime time.Duration) *discovery.DiscoveryResult {
	if d.discoveryService == nil || !d.discoveryService.IsEnabled() {
		return &discovery.DiscoveryResult{Found: false}
	}

	result, err := d.discoveryService.ProcessCommandOutput(stdout, stderr, exitCode, command, responseTime)
	if err != nil {
		return &discovery.DiscoveryResult{Found: false, Error: err}
	}

	// If we discovered new rate limit information, update our internal state
	if result.Found && result.Info != nil {
		d.rateLimit = result.Info.Limit
		d.window = result.Info.Window
	}

	return result
}

// GetEffectiveRateLimit returns the currently effective rate limit (discovered or fallback)
func (d *DiophantineWithDiscovery) GetEffectiveRateLimit() int {
	return d.rateLimit
}

// GetEffectiveWindow returns the currently effective window (discovered or fallback)
func (d *DiophantineWithDiscovery) GetEffectiveWindow() time.Duration {
	return d.window
}

// IsUsingDiscoveredLimits returns true if we're currently using discovered rate limits
func (d *DiophantineWithDiscovery) IsUsingDiscoveredLimits(command []string) bool {
	if d.discoveryService == nil || !d.discoveryService.IsEnabled() {
		return false
	}

	info, err := d.discoveryService.GetRateLimitForCommand(command)
	if err != nil || info == nil {
		return false
	}

	return info.Confidence >= 0.5 && !info.IsExpired()
}

// GetDiscoveryStats returns statistics about the discovery service
func (d *DiophantineWithDiscovery) GetDiscoveryStats() (map[string]interface{}, error) {
	if d.discoveryService == nil {
		return map[string]interface{}{"enabled": false}, nil
	}

	return d.discoveryService.GetStats()
}

// ListDiscoveredRateLimits returns all discovered rate limits
func (d *DiophantineWithDiscovery) ListDiscoveredRateLimits() ([]*discovery.RateLimitInfo, error) {
	if d.discoveryService == nil || !d.discoveryService.IsEnabled() {
		return nil, nil
	}

	return d.discoveryService.ListDiscoveredRateLimits()
}

// ForceLearnRateLimit manually adds rate limit information
func (d *DiophantineWithDiscovery) ForceLearnRateLimit(info *discovery.RateLimitInfo) error {
	if d.discoveryService == nil || !d.discoveryService.IsEnabled() {
		return nil
	}

	return d.discoveryService.ForceLearnRateLimit(info)
}

// ClearDiscoveredRateLimit removes discovered rate limit information
func (d *DiophantineWithDiscovery) ClearDiscoveredRateLimit(resourceID, host, path string) error {
	if d.discoveryService == nil || !d.discoveryService.IsEnabled() {
		return nil
	}

	return d.discoveryService.ClearRateLimitInfo(resourceID, host, path)
}
