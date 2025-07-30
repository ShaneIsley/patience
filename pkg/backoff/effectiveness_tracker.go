package backoff

import (
	"sync"
	"time"
)

// EffectivenessTracker tracks the success rate of different backoff strategies
type EffectivenessTracker struct {
	mu              sync.RWMutex
	strategyMetrics map[string]*StrategyMetrics
}

// StrategyMetrics contains performance data for a backoff strategy
type StrategyMetrics struct {
	TotalAttempts     int64
	SuccessfulRetries int64
	AverageDelay      time.Duration
	LastUpdated       time.Time
	SuccessRate       float64
}

// NewEffectivenessTracker creates a new effectiveness tracker
func NewEffectivenessTracker() *EffectivenessTracker {
	return &EffectivenessTracker{
		strategyMetrics: make(map[string]*StrategyMetrics),
	}
}

// RecordAttempt records a backoff attempt and its outcome
func (et *EffectivenessTracker) RecordAttempt(strategy string, success bool, delay time.Duration) {
	et.mu.Lock()
	defer et.mu.Unlock()

	if et.strategyMetrics == nil {
		et.strategyMetrics = make(map[string]*StrategyMetrics)
	}

	metrics, exists := et.strategyMetrics[strategy]
	if !exists {
		metrics = &StrategyMetrics{}
		et.strategyMetrics[strategy] = metrics
	}

	metrics.TotalAttempts++
	if success {
		metrics.SuccessfulRetries++
	}

	// Update running average delay
	if metrics.TotalAttempts == 1 {
		metrics.AverageDelay = delay
	} else {
		metrics.AverageDelay = time.Duration(
			(int64(metrics.AverageDelay)*(metrics.TotalAttempts-1) + int64(delay)) / metrics.TotalAttempts,
		)
	}

	metrics.SuccessRate = float64(metrics.SuccessfulRetries) / float64(metrics.TotalAttempts)
	metrics.LastUpdated = time.Now()
}

// GetMetrics returns the metrics for a specific strategy
func (et *EffectivenessTracker) GetMetrics(strategy string) *StrategyMetrics {
	et.mu.RLock()
	defer et.mu.RUnlock()

	if metrics, exists := et.strategyMetrics[strategy]; exists {
		// Return a copy to avoid race conditions
		return &StrategyMetrics{
			TotalAttempts:     metrics.TotalAttempts,
			SuccessfulRetries: metrics.SuccessfulRetries,
			AverageDelay:      metrics.AverageDelay,
			LastUpdated:       metrics.LastUpdated,
			SuccessRate:       metrics.SuccessRate,
		}
	}

	return nil
}

// GetAllMetrics returns metrics for all strategies
func (et *EffectivenessTracker) GetAllMetrics() map[string]*StrategyMetrics {
	et.mu.RLock()
	defer et.mu.RUnlock()

	result := make(map[string]*StrategyMetrics)
	for strategy, metrics := range et.strategyMetrics {
		result[strategy] = &StrategyMetrics{
			TotalAttempts:     metrics.TotalAttempts,
			SuccessfulRetries: metrics.SuccessfulRetries,
			AverageDelay:      metrics.AverageDelay,
			LastUpdated:       metrics.LastUpdated,
			SuccessRate:       metrics.SuccessRate,
		}
	}

	return result
}

// GetBestStrategy returns the strategy with the highest success rate
func (et *EffectivenessTracker) GetBestStrategy() (string, *StrategyMetrics) {
	et.mu.RLock()
	defer et.mu.RUnlock()

	var bestStrategy string
	var bestMetrics *StrategyMetrics
	var bestSuccessRate float64

	for strategy, metrics := range et.strategyMetrics {
		// Only consider strategies with sufficient data
		if metrics.TotalAttempts >= 3 && metrics.SuccessRate > bestSuccessRate {
			bestStrategy = strategy
			bestMetrics = metrics
			bestSuccessRate = metrics.SuccessRate
		}
	}

	if bestMetrics != nil {
		// Return a copy
		return bestStrategy, &StrategyMetrics{
			TotalAttempts:     bestMetrics.TotalAttempts,
			SuccessfulRetries: bestMetrics.SuccessfulRetries,
			AverageDelay:      bestMetrics.AverageDelay,
			LastUpdated:       bestMetrics.LastUpdated,
			SuccessRate:       bestMetrics.SuccessRate,
		}
	}

	return "", nil
}

// Reset clears all metrics
func (et *EffectivenessTracker) Reset() {
	et.mu.Lock()
	defer et.mu.Unlock()

	et.strategyMetrics = make(map[string]*StrategyMetrics)
}

// GetStrategyCount returns the number of strategies being tracked
func (et *EffectivenessTracker) GetStrategyCount() int {
	et.mu.RLock()
	defer et.mu.RUnlock()

	return len(et.strategyMetrics)
}
