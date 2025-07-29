package discovery

import (
	"fmt"
	"log"
	"time"
)

// Service provides rate limit discovery functionality
type Service struct {
	db      *Database
	parser  *Parser
	learner *Learner
	enabled bool
}

// NewService creates a new discovery service
func NewService(dbPath string, enabled bool) (*Service, error) {
	if !enabled {
		return &Service{enabled: false}, nil
	}

	// Initialize database
	db, err := NewDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize discovery database: %w", err)
	}

	// Initialize components
	parser := NewParser()
	learner := NewLearner(db)

	service := &Service{
		db:      db,
		parser:  parser,
		learner: learner,
		enabled: true,
	}

	// Start background cleanup routine
	go service.backgroundCleanup()

	return service, nil
}

// ProcessCommandOutput processes command output to discover rate limit information
func (s *Service) ProcessCommandOutput(stdout, stderr string, exitCode int, command []string, responseTime time.Duration) (*DiscoveryResult, error) {
	if !s.enabled {
		return &DiscoveryResult{Found: false}, nil
	}

	// Parse rate limit information from output
	result := s.parser.ParseFromCommandOutput(stdout, stderr, exitCode, command)

	// If we found rate limit info, save it
	if result.Found && result.Info != nil {
		if err := s.db.SaveRateLimitInfo(result.Info); err != nil {
			log.Printf("Warning: failed to save discovered rate limit info: %v", err)
		}
	}

	// Learn from the response regardless of whether we found explicit rate limit info
	resourceID, host, path := s.parser.extractResourceInfo(command)
	commandStr := fmt.Sprintf("%v", command)

	if err := s.learner.LearnFromResponse(resourceID, host, path, commandStr, exitCode, responseTime, time.Now()); err != nil {
		log.Printf("Warning: failed to save learning data: %v", err)
	}

	// Update existing rate limit info based on success/failure
	if exitCode == 429 {
		if err := s.learner.UpdateRateLimitFromFailure(resourceID, host, path, time.Now()); err != nil {
			log.Printf("Warning: failed to update rate limit from failure: %v", err)
		}
	} else if exitCode < 400 {
		if err := s.learner.UpdateRateLimitFromSuccess(resourceID, host, path); err != nil {
			log.Printf("Warning: failed to update rate limit from success: %v", err)
		}
	}

	return result, nil
}

// GetRateLimitInfo retrieves discovered rate limit information for a resource
func (s *Service) GetRateLimitInfo(resourceID, host, path string) (*RateLimitInfo, error) {
	if !s.enabled {
		return nil, nil
	}

	// Try exact match first
	info, err := s.db.GetRateLimitInfo(resourceID, host, path)
	if err == nil {
		return info, nil
	}

	// Try host/path match
	info, err = s.db.GetRateLimitInfoByHost(host, path)
	if err == nil {
		return info, nil
	}

	return nil, nil
}

// GetRateLimitForCommand gets rate limit info for a specific command
func (s *Service) GetRateLimitForCommand(command []string) (*RateLimitInfo, error) {
	if !s.enabled {
		return nil, nil
	}

	resourceID, host, path := s.parser.extractResourceInfo(command)
	return s.GetRateLimitInfo(resourceID, host, path)
}

// ListDiscoveredRateLimits returns all discovered rate limit information
func (s *Service) ListDiscoveredRateLimits() ([]*RateLimitInfo, error) {
	if !s.enabled {
		return nil, nil
	}

	return s.db.ListRateLimits()
}

// GetStats returns discovery service statistics
func (s *Service) GetStats() (map[string]interface{}, error) {
	if !s.enabled {
		return map[string]interface{}{
			"enabled": false,
		}, nil
	}

	stats, err := s.db.GetStats()
	if err != nil {
		return nil, err
	}

	stats["enabled"] = true
	return stats, nil
}

// AnalyzeTrends analyzes trends for a specific resource
func (s *Service) AnalyzeTrends(resourceID string) (*RateLimitInfo, error) {
	if !s.enabled {
		return nil, nil
	}

	return s.learner.AnalyzeRateLimitTrends(resourceID)
}

// backgroundCleanup runs periodic cleanup of old data
func (s *Service) backgroundCleanup() {
	ticker := time.NewTicker(24 * time.Hour) // Run daily
	defer ticker.Stop()

	for range ticker.C {
		if err := s.db.CleanupExpiredData(30 * 24 * time.Hour); err != nil { // Keep 30 days
			log.Printf("Warning: failed to cleanup expired discovery data: %v", err)
		}
	}
}

// Close closes the discovery service and its resources
func (s *Service) Close() error {
	if !s.enabled || s.db == nil {
		return nil
	}

	return s.db.Close()
}

// IsEnabled returns whether the discovery service is enabled
func (s *Service) IsEnabled() bool {
	return s.enabled
}

// ForceLearnRateLimit manually adds rate limit information (for testing or manual configuration)
func (s *Service) ForceLearnRateLimit(info *RateLimitInfo) error {
	if !s.enabled {
		return fmt.Errorf("discovery service is not enabled")
	}

	info.Source = string(SourceManual)
	info.LastSeen = time.Now()
	info.ObservationCount = 1
	info.Confidence = 1.0 // Manual entries have highest confidence

	return s.db.SaveRateLimitInfo(info)
}

// ClearRateLimitInfo removes rate limit information for a resource
func (s *Service) ClearRateLimitInfo(resourceID, host, path string) error {
	if !s.enabled {
		return fmt.Errorf("discovery service is not enabled")
	}

	// This would require a delete method in the database
	// For now, we can set the confidence to 0 to effectively disable it
	info, err := s.db.GetRateLimitInfo(resourceID, host, path)
	if err != nil {
		return err
	}

	info.Confidence = 0.0
	info.LastSeen = time.Now()

	return s.db.SaveRateLimitInfo(info)
}

// GetDiscoveryResult creates a discovery result for existing rate limit info
func (s *Service) GetDiscoveryResult(info *RateLimitInfo) *DiscoveryResult {
	if info == nil {
		return &DiscoveryResult{Found: false}
	}

	return &DiscoveryResult{
		Found:      true,
		Info:       info,
		Source:     DiscoverySource(info.Source),
		Confidence: info.Confidence,
	}
}
