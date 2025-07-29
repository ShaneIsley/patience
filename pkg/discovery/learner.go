package discovery

import (
	"fmt"
	"math"
	"sort"
	"time"
)

// Learner implements algorithms to learn rate limits from 429 responses and patterns
type Learner struct {
	db *Database
}

// NewLearner creates a new rate limit learner
func NewLearner(db *Database) *Learner {
	return &Learner{
		db: db,
	}
}

// LearnFromResponse analyzes a command response and learns rate limit patterns
func (l *Learner) LearnFromResponse(resourceID, host, path, command string, responseCode int, responseTime time.Duration, requestTime time.Time) error {
	// Save learning data
	learningData := &LearningData{
		ResourceID:   resourceID,
		RequestTime:  requestTime,
		ResponseCode: responseCode,
		Success:      responseCode < 400,
		ResponseTime: responseTime,
		Command:      command,
		Host:         host,
		Path:         path,
	}

	if err := l.db.SaveLearningData(learningData); err != nil {
		return fmt.Errorf("failed to save learning data: %w", err)
	}

	// If this is a 429 response, try to learn rate limits
	if responseCode == 429 {
		return l.learnFromRateLimitError(resourceID, host, path, requestTime)
	}

	return nil
}

// learnFromRateLimitError analyzes 429 responses to infer rate limits
func (l *Learner) learnFromRateLimitError(resourceID, host, path string, errorTime time.Time) error {
	// Get recent learning data to analyze patterns
	since := errorTime.Add(-24 * time.Hour) // Look at last 24 hours
	learningData, err := l.db.GetLearningData(resourceID, since)
	if err != nil {
		return fmt.Errorf("failed to get learning data: %w", err)
	}

	if len(learningData) < 2 {
		// Not enough data to learn from
		return nil
	}

	// Analyze request patterns to infer rate limits
	rateLimitInfo := l.analyzeRequestPatterns(learningData, resourceID, host, path, errorTime)
	if rateLimitInfo == nil {
		return nil // Could not infer rate limits
	}

	// Save the learned rate limit information
	return l.db.SaveRateLimitInfo(rateLimitInfo)
}

// analyzeRequestPatterns analyzes request patterns to infer rate limits
func (l *Learner) analyzeRequestPatterns(data []*LearningData, resourceID, host, path string, errorTime time.Time) *RateLimitInfo {
	// Sort data by request time
	sort.Slice(data, func(i, j int) bool {
		return data[i].RequestTime.Before(data[j].RequestTime)
	})

	// Find the most recent successful requests before the 429 error
	var recentSuccessful []*LearningData
	var recent429s []*LearningData

	for _, d := range data {
		if d.RequestTime.After(errorTime) {
			continue // Skip requests after the error
		}

		if d.ResponseCode == 429 {
			recent429s = append(recent429s, d)
		} else if d.Success {
			recentSuccessful = append(recentSuccessful, d)
		}
	}

	// Try different window sizes to find patterns
	windowSizes := []time.Duration{
		time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		time.Hour,
		24 * time.Hour,
	}

	for _, window := range windowSizes {
		if info := l.analyzeWindowPattern(recentSuccessful, recent429s, window, resourceID, host, path, errorTime); info != nil {
			return info
		}
	}

	return nil
}

// analyzeWindowPattern analyzes request patterns within a specific time window
func (l *Learner) analyzeWindowPattern(successful, failed []*LearningData, window time.Duration, resourceID, host, path string, errorTime time.Time) *RateLimitInfo {
	// Count requests in the window before the 429 error
	windowStart := errorTime.Add(-window)

	var requestsInWindow int
	var lastSuccessfulTime time.Time

	// Count successful requests in the window
	for _, d := range successful {
		if d.RequestTime.After(windowStart) && d.RequestTime.Before(errorTime) {
			requestsInWindow++
			if d.RequestTime.After(lastSuccessfulTime) {
				lastSuccessfulTime = d.RequestTime
			}
		}
	}

	// Check if there were any 429s in this window (indicating we hit the limit)
	var had429InWindow bool
	for _, d := range failed {
		if d.RequestTime.After(windowStart) && d.RequestTime.Before(errorTime) {
			had429InWindow = true
			break
		}
	}

	// If we had successful requests followed by a 429, this might be the rate limit
	if requestsInWindow > 0 && had429InWindow {
		// Estimate the rate limit as slightly higher than what we observed
		estimatedLimit := int(math.Ceil(float64(requestsInWindow) * 1.2)) // Add 20% buffer

		// Create rate limit info
		info := &RateLimitInfo{
			ResourceID:         resourceID,
			Host:               host,
			Path:               path,
			Limit:              estimatedLimit,
			Window:             window,
			Remaining:          0, // We hit the limit
			ResetTime:          errorTime.Add(window),
			Source:             string(SourceLearned),
			LastSeen:           time.Now(),
			ObservationCount:   1,
			SuccessfulRequests: requestsInWindow,
			FailedRequests:     1,
		}

		// Set last 429 response time
		info.Last429Response = &errorTime

		// Calculate confidence based on data quality
		info.Confidence = l.calculateLearningConfidence(requestsInWindow, len(failed), window)

		// Only return if confidence is reasonable
		if info.Confidence >= 0.3 {
			return info
		}
	}

	return nil
}

// calculateLearningConfidence calculates confidence for learned rate limits
func (l *Learner) calculateLearningConfidence(successfulRequests, failedRequests int, window time.Duration) float64 {
	// Base confidence starts low for learned data
	baseConfidence := 0.4

	// Increase confidence with more data points
	totalRequests := successfulRequests + failedRequests
	dataFactor := math.Min(1.0, float64(totalRequests)/10.0) // Max confidence at 10+ requests

	// Prefer shorter windows (more precise)
	windowFactor := 1.0
	if window > time.Hour {
		windowFactor = 0.8
	}
	if window > 6*time.Hour {
		windowFactor = 0.6
	}

	// Penalize if we have very few successful requests
	if successfulRequests < 2 {
		return 0.0 // Not enough data
	}

	confidence := baseConfidence * dataFactor * windowFactor

	// Ensure confidence is between 0.0 and 1.0
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.0 {
		confidence = 0.0
	}

	return confidence
}

// UpdateRateLimitFromSuccess updates rate limit info when requests succeed
func (l *Learner) UpdateRateLimitFromSuccess(resourceID, host, path string) error {
	// Get existing rate limit info
	info, err := l.db.GetRateLimitInfo(resourceID, host, path)
	if err != nil {
		// Try to get by host/path if exact resource ID not found
		info, err = l.db.GetRateLimitInfoByHost(host, path)
		if err != nil {
			return nil // No existing info to update
		}
	}

	// Update successful request count
	info.SuccessfulRequests++
	info.LastSeen = time.Now()
	info.ObservationCount++

	// Recalculate confidence
	info.Confidence = info.ConfidenceScore()

	// Save updated info
	return l.db.SaveRateLimitInfo(info)
}

// UpdateRateLimitFromFailure updates rate limit info when requests fail with 429
func (l *Learner) UpdateRateLimitFromFailure(resourceID, host, path string, failureTime time.Time) error {
	// Get existing rate limit info
	info, err := l.db.GetRateLimitInfo(resourceID, host, path)
	if err != nil {
		// Try to get by host/path if exact resource ID not found
		info, err = l.db.GetRateLimitInfoByHost(host, path)
		if err != nil {
			return nil // No existing info to update
		}
	}

	// Update failed request count
	info.FailedRequests++
	info.LastSeen = time.Now()
	info.ObservationCount++
	info.Last429Response = &failureTime

	// If we're hitting 429s, we might need to adjust our rate limit estimate
	if info.Source == string(SourceLearned) {
		// For learned rate limits, reduce the estimate if we're getting 429s
		totalRequests := info.SuccessfulRequests + info.FailedRequests
		failureRate := float64(info.FailedRequests) / float64(totalRequests)

		if failureRate > 0.2 { // If more than 20% of requests are failing
			// Reduce the rate limit estimate
			info.Limit = int(float64(info.Limit) * 0.9)
			if info.Limit < 1 {
				info.Limit = 1
			}
		}
	}

	// Recalculate confidence
	info.Confidence = info.ConfidenceScore()

	// Save updated info
	return l.db.SaveRateLimitInfo(info)
}

// AnalyzeRateLimitTrends analyzes trends in rate limit data to improve accuracy
func (l *Learner) AnalyzeRateLimitTrends(resourceID string) (*RateLimitInfo, error) {
	// Get recent learning data
	since := time.Now().Add(-7 * 24 * time.Hour) // Last week
	learningData, err := l.db.GetLearningData(resourceID, since)
	if err != nil {
		return nil, fmt.Errorf("failed to get learning data: %w", err)
	}

	if len(learningData) < 10 {
		return nil, nil // Not enough data for trend analysis
	}

	// Group data by time windows and analyze success/failure patterns
	return l.analyzeTrendPatterns(learningData, resourceID)
}

// analyzeTrendPatterns analyzes patterns in the learning data over time
func (l *Learner) analyzeTrendPatterns(data []*LearningData, resourceID string) (*RateLimitInfo, error) {
	// Sort data by time
	sort.Slice(data, func(i, j int) bool {
		return data[i].RequestTime.Before(data[j].RequestTime)
	})

	// Extract host and path from the first data point
	if len(data) == 0 {
		return nil, nil
	}
	host := data[0].Host
	path := data[0].Path

	// Analyze different time windows to find consistent patterns
	windowSizes := []time.Duration{
		time.Minute,
		5 * time.Minute,
		15 * time.Minute,
		time.Hour,
	}

	bestInfo := &RateLimitInfo{}
	bestConfidence := 0.0

	for _, window := range windowSizes {
		if info := l.analyzeWindowTrends(data, window, resourceID, host, path); info != nil {
			if info.Confidence > bestConfidence {
				bestInfo = info
				bestConfidence = info.Confidence
			}
		}
	}

	if bestConfidence > 0.5 {
		return bestInfo, nil
	}

	return nil, nil
}

// analyzeWindowTrends analyzes trends within a specific time window
func (l *Learner) analyzeWindowTrends(data []*LearningData, window time.Duration, resourceID, host, path string) *RateLimitInfo {
	// Group requests by time windows
	windowGroups := make(map[int64][]*LearningData)

	for _, d := range data {
		windowStart := d.RequestTime.Truncate(window).Unix()
		windowGroups[windowStart] = append(windowGroups[windowStart], d)
	}

	// Analyze each window to find patterns
	var successfulWindows []int
	var failedWindows []int

	for _, group := range windowGroups {
		successCount := 0
		failureCount := 0

		for _, d := range group {
			if d.Success {
				successCount++
			} else if d.ResponseCode == 429 {
				failureCount++
			}
		}

		if failureCount > 0 {
			// This window had rate limit failures
			failedWindows = append(failedWindows, successCount)
		} else if successCount > 0 {
			// This window was successful
			successfulWindows = append(successfulWindows, successCount)
		}
	}

	// If we have both successful and failed windows, we can estimate the rate limit
	if len(failedWindows) > 0 && len(successfulWindows) > 0 {
		// The rate limit is likely around the maximum successful requests in a window
		maxSuccessful := 0
		for _, count := range successfulWindows {
			if count > maxSuccessful {
				maxSuccessful = count
			}
		}

		// The minimum requests that caused failures
		minFailed := math.MaxInt32
		for _, count := range failedWindows {
			if count < minFailed {
				minFailed = count
			}
		}

		// Estimate rate limit as between these values
		estimatedLimit := (maxSuccessful + minFailed) / 2
		if estimatedLimit < 1 {
			estimatedLimit = maxSuccessful + 1
		}

		info := &RateLimitInfo{
			ResourceID:         resourceID,
			Host:               host,
			Path:               path,
			Limit:              estimatedLimit,
			Window:             window,
			Remaining:          0,
			ResetTime:          time.Now().Add(window),
			Source:             string(SourceLearned),
			LastSeen:           time.Now(),
			ObservationCount:   len(successfulWindows) + len(failedWindows),
			SuccessfulRequests: len(successfulWindows),
			FailedRequests:     len(failedWindows),
		}

		// Calculate confidence based on data consistency
		totalWindows := len(successfulWindows) + len(failedWindows)
		confidence := math.Min(0.8, float64(totalWindows)/20.0) // Max 0.8 confidence for learned data

		info.Confidence = confidence

		return info
	}

	return nil
}
