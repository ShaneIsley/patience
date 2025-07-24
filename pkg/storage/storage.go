package storage

import (
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/user/retry/pkg/metrics"
)

// MetricsStorage provides thread-safe storage and aggregation of retry metrics
type MetricsStorage struct {
	mu          sync.RWMutex
	metrics     []StoredMetric
	maxSize     int
	maxAge      time.Duration
	lastCleanup time.Time
}

// StoredMetric represents a stored metrics entry with timestamp
type StoredMetric struct {
	Timestamp time.Time           `json:"timestamp"`
	Metrics   *metrics.RunMetrics `json:"metrics"`
}

// AggregatedStats represents aggregated statistics over a time period
type AggregatedStats struct {
	TimeRange       TimeRange      `json:"time_range"`
	TotalRuns       int            `json:"total_runs"`
	SuccessfulRuns  int            `json:"successful_runs"`
	FailedRuns      int            `json:"failed_runs"`
	SuccessRate     float64        `json:"success_rate"`
	AverageAttempts float64        `json:"average_attempts"`
	AverageDuration time.Duration  `json:"average_duration"`
	TopCommands     []CommandStats `json:"top_commands"`
	HourlyBreakdown []HourlyStats  `json:"hourly_breakdown"`
}

// TimeRange represents a time range for aggregation
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// CommandStats represents statistics for a specific command
type CommandStats struct {
	Command     string        `json:"command"`
	Count       int           `json:"count"`
	SuccessRate float64       `json:"success_rate"`
	AvgDuration time.Duration `json:"avg_duration"`
}

// HourlyStats represents statistics for a specific hour
type HourlyStats struct {
	Hour        time.Time `json:"hour"`
	TotalRuns   int       `json:"total_runs"`
	SuccessRate float64   `json:"success_rate"`
}

// NewMetricsStorage creates a new metrics storage instance
func NewMetricsStorage(maxSize int, maxAge time.Duration) *MetricsStorage {
	return &MetricsStorage{
		metrics:     make([]StoredMetric, 0, maxSize),
		maxSize:     maxSize,
		maxAge:      maxAge,
		lastCleanup: time.Now(),
	}
}

// Store adds a new metrics entry to storage
func (s *MetricsStorage) Store(metric *metrics.RunMetrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Add new metric
	stored := StoredMetric{
		Timestamp: time.Now(),
		Metrics:   metric,
	}

	s.metrics = append(s.metrics, stored)

	// Cleanup if needed
	s.cleanupIfNeeded()

	return nil
}

// GetRecent returns the most recent N metrics
func (s *MetricsStorage) GetRecent(limit int) []StoredMetric {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.metrics) {
		limit = len(s.metrics)
	}

	// Return the last N metrics (most recent)
	start := len(s.metrics) - limit
	result := make([]StoredMetric, limit)
	copy(result, s.metrics[start:])

	return result
}

// GetByTimeRange returns metrics within a specific time range
func (s *MetricsStorage) GetByTimeRange(start, end time.Time) []StoredMetric {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []StoredMetric
	for _, metric := range s.metrics {
		if metric.Timestamp.After(start) && metric.Timestamp.Before(end) {
			result = append(result, metric)
		}
	}

	return result
}

// GetAggregatedStats returns aggregated statistics for a time range
func (s *MetricsStorage) GetAggregatedStats(start, end time.Time) *AggregatedStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metricsInRange := s.getMetricsInRange(start, end)
	if len(metricsInRange) == 0 {
		return &AggregatedStats{
			TimeRange: TimeRange{Start: start, End: end},
		}
	}

	stats := &AggregatedStats{
		TimeRange: TimeRange{Start: start, End: end},
	}

	// Calculate basic stats
	var totalDuration time.Duration
	var totalAttempts int
	commandStats := make(map[string]*CommandStats)
	hourlyStats := make(map[string]*HourlyStats)

	for _, stored := range metricsInRange {
		metric := stored.Metrics
		stats.TotalRuns++

		success := metric.FinalStatus == "succeeded"
		if success {
			stats.SuccessfulRuns++
		} else {
			stats.FailedRuns++
		}

		totalDuration += time.Duration(metric.TotalDurationSeconds * float64(time.Second))
		totalAttempts += len(metric.Attempts)

		// Track command statistics
		cmdKey := metric.Command
		if cmdStats, exists := commandStats[cmdKey]; exists {
			cmdStats.Count++
			if success {
				cmdStats.SuccessRate = (cmdStats.SuccessRate*(float64(cmdStats.Count-1)) + 1.0) / float64(cmdStats.Count)
			} else {
				cmdStats.SuccessRate = cmdStats.SuccessRate * (float64(cmdStats.Count - 1)) / float64(cmdStats.Count)
			}
			duration := time.Duration(metric.TotalDurationSeconds * float64(time.Second))
			cmdStats.AvgDuration = (cmdStats.AvgDuration*time.Duration(cmdStats.Count-1) + duration) / time.Duration(cmdStats.Count)
		} else {
			successRate := 0.0
			if success {
				successRate = 1.0
			}
			commandStats[cmdKey] = &CommandStats{
				Command:     cmdKey,
				Count:       1,
				SuccessRate: successRate,
				AvgDuration: time.Duration(metric.TotalDurationSeconds * float64(time.Second)),
			}
		}

		// Track hourly statistics
		hour := stored.Timestamp.Truncate(time.Hour)
		hourKey := hour.Format(time.RFC3339)
		if hourStats, exists := hourlyStats[hourKey]; exists {
			hourStats.TotalRuns++
			if success {
				hourStats.SuccessRate = (hourStats.SuccessRate*(float64(hourStats.TotalRuns-1)) + 1.0) / float64(hourStats.TotalRuns)
			} else {
				hourStats.SuccessRate = hourStats.SuccessRate * (float64(hourStats.TotalRuns - 1)) / float64(hourStats.TotalRuns)
			}
		} else {
			successRate := 0.0
			if success {
				successRate = 1.0
			}
			hourlyStats[hourKey] = &HourlyStats{
				Hour:        hour,
				TotalRuns:   1,
				SuccessRate: successRate,
			}
		}
	}

	// Calculate derived stats
	if stats.TotalRuns > 0 {
		stats.SuccessRate = float64(stats.SuccessfulRuns) / float64(stats.TotalRuns)
		stats.AverageAttempts = float64(totalAttempts) / float64(stats.TotalRuns)
		stats.AverageDuration = totalDuration / time.Duration(stats.TotalRuns)
	}

	// Convert maps to sorted slices
	stats.TopCommands = s.sortCommandStats(commandStats)
	stats.HourlyBreakdown = s.sortHourlyStats(hourlyStats)

	return stats
}

// GetStats returns current storage statistics
func (s *MetricsStorage) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"total_metrics": len(s.metrics),
		"max_size":      s.maxSize,
		"max_age":       s.maxAge.String(),
		"last_cleanup":  s.lastCleanup,
	}
}

// ExportJSON exports all metrics as JSON
func (s *MetricsStorage) ExportJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return json.MarshalIndent(s.metrics, "", "  ")
}

// Clear removes all stored metrics
func (s *MetricsStorage) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.metrics = s.metrics[:0]
}

// cleanupIfNeeded removes old metrics if storage limits are exceeded
func (s *MetricsStorage) cleanupIfNeeded() {
	now := time.Now()

	// Only cleanup every 5 minutes to avoid excessive work
	if now.Sub(s.lastCleanup) < 5*time.Minute {
		return
	}

	s.lastCleanup = now
	cutoff := now.Add(-s.maxAge)

	// Remove metrics older than maxAge
	var kept []StoredMetric
	for _, metric := range s.metrics {
		if metric.Timestamp.After(cutoff) {
			kept = append(kept, metric)
		}
	}

	s.metrics = kept

	// Remove oldest metrics if we exceed maxSize
	if len(s.metrics) > s.maxSize {
		excess := len(s.metrics) - s.maxSize
		s.metrics = s.metrics[excess:]
	}
}

// getMetricsInRange returns metrics within the specified time range (assumes lock is held)
func (s *MetricsStorage) getMetricsInRange(start, end time.Time) []StoredMetric {
	var result []StoredMetric
	for _, metric := range s.metrics {
		if metric.Timestamp.After(start) && metric.Timestamp.Before(end) {
			result = append(result, metric)
		}
	}
	return result
}

// sortCommandStats converts command stats map to sorted slice
func (s *MetricsStorage) sortCommandStats(commandStats map[string]*CommandStats) []CommandStats {
	var stats []CommandStats
	for _, stat := range commandStats {
		stats = append(stats, *stat)
	}

	// Sort by count (descending)
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Count > stats[j].Count
	})

	// Limit to top 10
	if len(stats) > 10 {
		stats = stats[:10]
	}

	return stats
}

// sortHourlyStats converts hourly stats map to sorted slice
func (s *MetricsStorage) sortHourlyStats(hourlyStats map[string]*HourlyStats) []HourlyStats {
	var stats []HourlyStats
	for _, stat := range hourlyStats {
		stats = append(stats, *stat)
	}

	// Sort by hour (ascending)
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Hour.Before(stats[j].Hour)
	})

	return stats
}
