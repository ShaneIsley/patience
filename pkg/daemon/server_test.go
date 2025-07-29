package daemon

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shaneisley/patience/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_HandleRecentMetrics(t *testing.T) {
	// Given a server with test metrics
	metricsStorage := storage.NewMetricsStorage(100, time.Hour)
	logger := NewLogger("test", LogLevelInfo)
	server := NewServer(metricsStorage, 8080, logger)

	// Add test metrics
	testMetric := createTestRunMetrics("echo test", true, 1.5, 2)
	metricsStorage.Store(testMetric)

	// When requesting recent metrics
	req := httptest.NewRequest("GET", "/api/metrics/recent", nil)
	w := httptest.NewRecorder()
	server.handleRecentMetrics(w, req)

	// Then it should return the metrics
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(1), response["count"])
	metrics, ok := response["metrics"].([]interface{})
	require.True(t, ok)
	require.Len(t, metrics, 1)
}

func TestServer_HandleRecentMetricsWithLimit(t *testing.T) {
	// Given a server with multiple metrics
	metricsStorage := storage.NewMetricsStorage(100, time.Hour)
	logger := NewLogger("test", LogLevelInfo)
	server := NewServer(metricsStorage, 8080, logger)

	// Add multiple test metrics
	for i := 0; i < 5; i++ {
		testMetric := createTestRunMetrics(fmt.Sprintf("echo test%d", i), true, 1.0, 1)
		metricsStorage.Store(testMetric)
	}

	// When requesting recent metrics with limit
	req := httptest.NewRequest("GET", "/api/metrics/recent?limit=3", nil)
	w := httptest.NewRecorder()
	server.handleRecentMetrics(w, req)

	// Then it should return limited results
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, float64(3), response["count"])
	metrics, ok := response["metrics"].([]interface{})
	require.True(t, ok)
	require.Len(t, metrics, 3)
}

func TestServer_HandleAggregatedStats(t *testing.T) {
	// Given a server with test metrics
	metricsStorage := storage.NewMetricsStorage(100, time.Hour)
	logger := NewLogger("test", LogLevelInfo)
	server := NewServer(metricsStorage, 8080, logger)

	// Add test metrics
	testMetric1 := createTestRunMetrics("echo test", true, 1.0, 1)
	testMetric2 := createTestRunMetrics("echo test", false, 2.0, 3)
	metricsStorage.Store(testMetric1)
	metricsStorage.Store(testMetric2)

	// When requesting aggregated stats
	req := httptest.NewRequest("GET", "/api/metrics/stats", nil)
	w := httptest.NewRecorder()
	server.handleAggregatedStats(w, req)

	// Then it should return aggregated statistics
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var stats storage.AggregatedStats
	err := json.Unmarshal(w.Body.Bytes(), &stats)
	require.NoError(t, err)

	assert.Equal(t, 2, stats.TotalRuns)
	assert.Equal(t, 1, stats.SuccessfulRuns)
	assert.Equal(t, 1, stats.FailedRuns)
	assert.Equal(t, 0.5, stats.SuccessRate)
}

func TestServer_HandleAggregatedStatsWithTimeRange(t *testing.T) {
	// Given a server with test metrics
	metricsStorage := storage.NewMetricsStorage(100, time.Hour)
	logger := NewLogger("test", LogLevelInfo)
	server := NewServer(metricsStorage, 8080, logger)

	// Add test metric
	testMetric := createTestRunMetrics("echo test", true, 1.0, 1)
	metricsStorage.Store(testMetric)

	// When requesting stats with time range
	now := time.Now()
	start := now.Add(-time.Hour).Format(time.RFC3339)
	end := now.Add(time.Hour).Format(time.RFC3339)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/metrics/stats?start=%s&end=%s", start, end), nil)
	w := httptest.NewRecorder()
	server.handleAggregatedStats(w, req)

	// Then it should return stats for the time range
	assert.Equal(t, http.StatusOK, w.Code)

	var stats storage.AggregatedStats
	err := json.Unmarshal(w.Body.Bytes(), &stats)
	require.NoError(t, err)

	assert.Equal(t, 1, stats.TotalRuns)
}

func TestServer_HandleExportMetrics(t *testing.T) {
	// Given a server with test metrics
	metricsStorage := storage.NewMetricsStorage(100, time.Hour)
	logger := NewLogger("test", LogLevelInfo)
	server := NewServer(metricsStorage, 8080, logger)

	// Add test metric
	testMetric := createTestRunMetrics("echo test", true, 1.0, 1)
	metricsStorage.Store(testMetric)

	// When requesting metrics export
	req := httptest.NewRequest("GET", "/api/metrics/export", nil)
	w := httptest.NewRecorder()
	server.handleExportMetrics(w, req)

	// Then it should return JSON export
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Header().Get("Content-Disposition"), "attachment")
	assert.Contains(t, w.Header().Get("Content-Disposition"), "retry-metrics-")

	// Verify it's valid JSON
	var exported []storage.StoredMetric
	err := json.Unmarshal(w.Body.Bytes(), &exported)
	require.NoError(t, err)
	require.Len(t, exported, 1)
}

func TestServer_HandleDaemonStats(t *testing.T) {
	// Given a server
	metricsStorage := storage.NewMetricsStorage(100, time.Hour)
	logger := NewLogger("test", LogLevelInfo)
	server := NewServer(metricsStorage, 8080, logger)

	// When requesting daemon stats
	req := httptest.NewRequest("GET", "/api/daemon/stats", nil)
	w := httptest.NewRecorder()
	server.handleDaemonStats(w, req)

	// Then it should return storage statistics
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var stats map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &stats)
	require.NoError(t, err)

	assert.Contains(t, stats, "total_metrics")
	assert.Contains(t, stats, "max_size")
	assert.Contains(t, stats, "max_age")
}

func TestServer_HandlePerformanceStats(t *testing.T) {
	// Given a server
	metricsStorage := storage.NewMetricsStorage(100, time.Hour)
	logger := NewLogger("test", LogLevelInfo)
	server := NewServer(metricsStorage, 8080, logger)

	// When requesting performance stats
	req := httptest.NewRequest("GET", "/api/daemon/performance", nil)
	w := httptest.NewRecorder()
	server.handlePerformanceStats(w, req)

	// Then it should return performance statistics
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var stats map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &stats)
	require.NoError(t, err)

	assert.Contains(t, stats, "memory")
	assert.Contains(t, stats, "gc")
	assert.Contains(t, stats, "runtime")

	// Verify memory stats structure
	memory, ok := stats["memory"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, memory, "alloc_bytes")
	assert.Contains(t, memory, "heap_alloc_bytes")

	// Verify runtime stats structure
	runtime, ok := stats["runtime"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, runtime, "goroutines")
	assert.Contains(t, runtime, "go_version")
}

func TestServer_HandleHealth(t *testing.T) {
	// Given a server
	metricsStorage := storage.NewMetricsStorage(100, time.Hour)
	logger := NewLogger("test", LogLevelInfo)
	server := NewServer(metricsStorage, 8080, logger)

	// When requesting health status
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	server.handleHealth(w, req)

	// Then it should return healthy status
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var health map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &health)
	require.NoError(t, err)

	assert.Equal(t, "healthy", health["status"])
	assert.Contains(t, health, "timestamp")
	assert.Contains(t, health, "version")
}

func TestServer_HandleDashboard(t *testing.T) {
	// Given a server
	metricsStorage := storage.NewMetricsStorage(100, time.Hour)
	logger := NewLogger("test", LogLevelInfo)
	server := NewServer(metricsStorage, 8080, logger)

	// When requesting the dashboard
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	server.handleDashboard(w, req)

	// Then it should return HTML dashboard
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/html", w.Header().Get("Content-Type"))
	assert.Contains(t, w.Body.String(), "Retry Daemon Dashboard")
	assert.Contains(t, w.Body.String(), "<!DOCTYPE html>")
}

func TestServer_MethodNotAllowed(t *testing.T) {
	// Given a server
	metricsStorage := storage.NewMetricsStorage(100, time.Hour)
	logger := NewLogger("test", LogLevelInfo)
	server := NewServer(metricsStorage, 8080, logger)

	// When making a POST request to GET-only endpoint
	req := httptest.NewRequest("POST", "/api/metrics/recent", nil)
	w := httptest.NewRecorder()
	server.handleRecentMetrics(w, req)

	// Then it should return method not allowed
	assert.Equal(t, http.StatusMethodNotAllowed, w.Code)
}

func TestServer_NotFound(t *testing.T) {
	// Given a server
	metricsStorage := storage.NewMetricsStorage(100, time.Hour)
	logger := NewLogger("test", LogLevelInfo)
	server := NewServer(metricsStorage, 8080, logger)

	// When requesting non-existent path
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	server.handleDashboard(w, req)

	// Then it should return not found
	assert.Equal(t, http.StatusNotFound, w.Code)
}
