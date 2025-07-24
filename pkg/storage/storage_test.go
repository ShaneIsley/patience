package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/shaneisley/patience/pkg/metrics"
)

func TestMetricsStorage_Store(t *testing.T) {
	// Given a metrics storage instance
	storage := NewMetricsStorage(100, time.Hour)

	// And a test metric
	metric := createTestMetric("echo test", true, 1.5, 2)

	// When storing the metric
	err := storage.Store(metric)

	// Then it should succeed
	require.NoError(t, err)

	// And the metric should be retrievable
	recent := storage.GetRecent(1)
	require.Len(t, recent, 1)
	assert.Equal(t, metric, recent[0].Metrics)
}

func TestMetricsStorage_GetRecent(t *testing.T) {
	// Given a storage with multiple metrics
	storage := NewMetricsStorage(100, time.Hour)

	metrics := []*metrics.RunMetrics{
		createTestMetric("echo test1", true, 1.0, 1),
		createTestMetric("echo test2", false, 2.0, 3),
		createTestMetric("echo test3", true, 1.5, 2),
	}

	for _, metric := range metrics {
		storage.Store(metric)
	}

	// When getting recent metrics
	recent := storage.GetRecent(2)

	// Then it should return the most recent ones
	require.Len(t, recent, 2)
	assert.Equal(t, metrics[1], recent[0].Metrics) // Second most recent
	assert.Equal(t, metrics[2], recent[1].Metrics) // Most recent
}

func TestMetricsStorage_GetByTimeRange(t *testing.T) {
	// Given a storage with metrics at different times
	storage := NewMetricsStorage(100, time.Hour)

	now := time.Now()
	metric1 := createTestMetric("echo test1", true, 1.0, 1)
	metric2 := createTestMetric("echo test2", false, 2.0, 3)

	storage.Store(metric1)
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	storage.Store(metric2)

	// When getting metrics by time range
	start := now.Add(-time.Minute)
	end := now.Add(time.Minute)
	inRange := storage.GetByTimeRange(start, end)

	// Then it should return metrics in the range
	require.Len(t, inRange, 2)
}

func TestMetricsStorage_GetAggregatedStats(t *testing.T) {
	// Given a storage with various metrics
	storage := NewMetricsStorage(100, time.Hour)

	metrics := []*metrics.RunMetrics{
		createTestMetric("echo test", true, 1.0, 1),
		createTestMetric("echo test", false, 2.0, 3),
		createTestMetric("curl example.com", true, 0.5, 1),
		createTestMetric("echo test", true, 1.5, 2),
	}

	for _, metric := range metrics {
		storage.Store(metric)
	}

	// When getting aggregated stats
	start := time.Now().Add(-time.Hour)
	end := time.Now().Add(time.Hour)
	stats := storage.GetAggregatedStats(start, end)

	// Then it should calculate correct statistics
	require.NotNil(t, stats)
	assert.Equal(t, 4, stats.TotalRuns)
	assert.Equal(t, 3, stats.SuccessfulRuns)
	assert.Equal(t, 1, stats.FailedRuns)
	assert.Equal(t, 0.75, stats.SuccessRate)
	assert.Equal(t, 1.75, stats.AverageAttempts) // (1+3+1+2)/4

	// And should have command statistics
	require.Len(t, stats.TopCommands, 2)

	// Sort by count to ensure consistent ordering
	if stats.TopCommands[0].Count < stats.TopCommands[1].Count {
		stats.TopCommands[0], stats.TopCommands[1] = stats.TopCommands[1], stats.TopCommands[0]
	}

	assert.Equal(t, "echo test", stats.TopCommands[0].Command)
	assert.Equal(t, 3, stats.TopCommands[0].Count)
	assert.Equal(t, "curl example.com", stats.TopCommands[1].Command)
	assert.Equal(t, 1, stats.TopCommands[1].Count)
}

func TestMetricsStorage_Cleanup(t *testing.T) {
	// Given a storage with small limits
	storage := NewMetricsStorage(2, 50*time.Millisecond)

	// Manually set lastCleanup to past to force cleanup
	storage.lastCleanup = time.Now().Add(-10 * time.Minute)

	// When adding more metrics than the limit
	for i := 0; i < 5; i++ {
		metric := createTestMetric("echo test", true, 1.0, 1)
		storage.Store(metric)
		time.Sleep(5 * time.Millisecond)
	}

	// Force cleanup by waiting for maxAge and adding another metric
	time.Sleep(100 * time.Millisecond)

	// Reset lastCleanup again to force cleanup
	storage.lastCleanup = time.Now().Add(-10 * time.Minute)

	// Add a new metric to trigger cleanup
	storage.Store(createTestMetric("echo final", true, 1.0, 1))

	// Then old metrics should be removed due to maxSize limit
	recent := storage.GetRecent(10)
	assert.LessOrEqual(t, len(recent), 2) // Should respect maxSize
}
func TestMetricsStorage_ExportJSON(t *testing.T) {
	// Given a storage with metrics
	storage := NewMetricsStorage(100, time.Hour)
	metric := createTestMetric("echo test", true, 1.0, 1)
	storage.Store(metric)

	// When exporting to JSON
	data, err := storage.ExportJSON()

	// Then it should succeed
	require.NoError(t, err)
	assert.Contains(t, string(data), "echo test")
	assert.Contains(t, string(data), "succeeded")
}

func TestMetricsStorage_Clear(t *testing.T) {
	// Given a storage with metrics
	storage := NewMetricsStorage(100, time.Hour)
	storage.Store(createTestMetric("echo test", true, 1.0, 1))

	// When clearing the storage
	storage.Clear()

	// Then it should be empty
	recent := storage.GetRecent(10)
	assert.Len(t, recent, 0)
}

func TestMetricsStorage_GetStats(t *testing.T) {
	// Given a storage with configuration
	storage := NewMetricsStorage(100, time.Hour)

	// When getting storage stats
	stats := storage.GetStats()

	// Then it should return configuration info
	require.NotNil(t, stats)
	assert.Equal(t, 0, stats["total_metrics"])
	assert.Equal(t, 100, stats["max_size"])
	assert.Equal(t, "1h0m0s", stats["max_age"])
}

func TestMetricsStorage_ConcurrentAccess(t *testing.T) {
	// Given a storage instance
	storage := NewMetricsStorage(1000, time.Hour)

	// When accessing concurrently from multiple goroutines
	done := make(chan bool, 10)

	// Start 5 writers
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				metric := createTestMetric("concurrent test", true, 1.0, 1)
				storage.Store(metric)
			}
			done <- true
		}(i)
	}

	// Start 5 readers
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				storage.GetRecent(5)
				start := time.Now().Add(-time.Hour)
				end := time.Now()
				storage.GetAggregatedStats(start, end)
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Then there should be no race conditions (test passes if no panic)
	recent := storage.GetRecent(100)
	assert.Equal(t, 50, len(recent)) // 5 writers * 10 metrics each
}

// createTestMetric creates a test RunMetrics instance
func createTestMetric(command string, success bool, durationSeconds float64, attemptCount int) *metrics.RunMetrics {
	attempts := make([]metrics.AttemptMetric, attemptCount)
	for i := 0; i < attemptCount; i++ {
		attempts[i] = metrics.AttemptMetric{
			Duration: time.Duration(durationSeconds/float64(attemptCount)) * time.Second,
			ExitCode: 0,
			Success:  i == attemptCount-1 && success, // Only last attempt succeeds
		}
	}

	finalStatus := "failed"
	if success {
		finalStatus = "succeeded"
	}

	return &metrics.RunMetrics{
		Command:              command,
		CommandHash:          "test-hash",
		FinalStatus:          finalStatus,
		TotalDurationSeconds: durationSeconds,
		TotalAttempts:        attemptCount,
		SuccessfulAttempts: func() int {
			if success {
				return 1
			} else {
				return 0
			}
		}(),
		FailedAttempts: func() int {
			if success {
				return attemptCount - 1
			} else {
				return attemptCount
			}
		}(),
		Attempts:  attempts,
		Timestamp: time.Now().Unix(),
	}
}
