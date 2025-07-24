package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"strconv"
	"time"

	"github.com/shaneisley/patience/pkg/storage"
)

// Server represents the HTTP API server
type Server struct {
	storage    *storage.MetricsStorage
	port       int
	logger     *log.Logger
	httpServer *http.Server
}

// NewServer creates a new HTTP server instance
func NewServer(storage *storage.MetricsStorage, port int, logger *log.Logger) *Server {
	return &Server{
		storage: storage,
		port:    port,
		logger:  logger,
	}
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/metrics/recent", s.handleRecentMetrics)
	mux.HandleFunc("/api/metrics/stats", s.handleAggregatedStats)
	mux.HandleFunc("/api/metrics/export", s.handleExportMetrics)
	mux.HandleFunc("/api/daemon/stats", s.handleDaemonStats)
	mux.HandleFunc("/api/daemon/performance", s.handlePerformanceStats)
	mux.HandleFunc("/api/health", s.handleHealth)

	// Profiling endpoints (pprof is automatically registered)
	// Available at /debug/pprof/ when profiling is enabled

	// Static file serving for dashboard
	mux.HandleFunc("/", s.handleDashboard)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	s.logger.Printf("Starting HTTP server on port %d", s.port)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		return s.httpServer.Shutdown(context.Background())
	case err := <-errChan:
		return err
	}
}

// Stop stops the HTTP server
func (s *Server) Stop() error {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// handleRecentMetrics handles GET /api/metrics/recent
func (s *Server) handleRecentMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse limit parameter
	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Get recent metrics
	metrics := s.storage.GetRecent(limit)

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"metrics": metrics,
		"count":   len(metrics),
	})
}

// handleAggregatedStats handles GET /api/metrics/stats
func (s *Server) handleAggregatedStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse time range parameters
	now := time.Now()
	start := now.Add(-24 * time.Hour) // Default to last 24 hours
	end := now

	if startStr := r.URL.Query().Get("start"); startStr != "" {
		if parsedStart, err := time.Parse(time.RFC3339, startStr); err == nil {
			start = parsedStart
		}
	}

	if endStr := r.URL.Query().Get("end"); endStr != "" {
		if parsedEnd, err := time.Parse(time.RFC3339, endStr); err == nil {
			end = parsedEnd
		}
	}

	// Get aggregated stats
	stats := s.storage.GetAggregatedStats(start, end)

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleExportMetrics handles GET /api/metrics/export
func (s *Server) handleExportMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Export metrics as JSON
	data, err := s.storage.ExportJSON()
	if err != nil {
		http.Error(w, "Failed to export metrics", http.StatusInternalServerError)
		return
	}

	// Set headers for file download
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=retry-metrics-%s.json", time.Now().Format("2006-01-02")))
	w.Write(data)
}

// handleDaemonStats handles GET /api/daemon/stats
func (s *Server) handleDaemonStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get storage stats
	stats := s.storage.GetStats()

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handlePerformanceStats handles GET /api/daemon/performance
func (s *Server) handlePerformanceStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get runtime memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Get goroutine count
	numGoroutines := runtime.NumGoroutine()

	// Get GC stats (simplified - runtime.GCStats is not available in all Go versions)
	var lastPause time.Duration
	if len(memStats.PauseNs) > 0 {
		lastPause = time.Duration(memStats.PauseNs[(memStats.NumGC+255)%256])
	}

	// Compile performance stats
	perfStats := map[string]interface{}{
		"memory": map[string]interface{}{
			"alloc_bytes":         memStats.Alloc,
			"total_alloc_bytes":   memStats.TotalAlloc,
			"sys_bytes":           memStats.Sys,
			"heap_alloc_bytes":    memStats.HeapAlloc,
			"heap_sys_bytes":      memStats.HeapSys,
			"heap_idle_bytes":     memStats.HeapIdle,
			"heap_inuse_bytes":    memStats.HeapInuse,
			"heap_released_bytes": memStats.HeapReleased,
			"heap_objects":        memStats.HeapObjects,
			"stack_inuse_bytes":   memStats.StackInuse,
			"stack_sys_bytes":     memStats.StackSys,
		},
		"gc": map[string]interface{}{
			"num_gc":          memStats.NumGC,
			"num_forced_gc":   memStats.NumForcedGC,
			"gc_cpu_fraction": memStats.GCCPUFraction,
			"last_gc_time":    time.Unix(0, int64(memStats.LastGC)),
			"pause_total_ns":  memStats.PauseTotalNs,
			"last_pause_ns":   lastPause.Nanoseconds(),
		},
		"runtime": map[string]interface{}{
			"goroutines": numGoroutines,
			"gomaxprocs": runtime.GOMAXPROCS(0),
			"num_cpu":    runtime.NumCPU(),
			"go_version": runtime.Version(),
			"compiler":   runtime.Compiler,
			"arch":       runtime.GOARCH,
			"os":         runtime.GOOS,
		},
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(perfStats)
}

// handleHealth handles GET /api/health
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Return health status
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"version":   "1.0.0", // This would come from build info
	})
}

// handleDashboard serves the web dashboard
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Serve a simple HTML dashboard
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Retry Daemon Dashboard</title>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background-color: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; }
        .header { background: #333; color: white; padding: 20px; border-radius: 5px; margin-bottom: 20px; }
        .card { background: white; padding: 20px; border-radius: 5px; margin-bottom: 20px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; }
        .stat { text-align: center; }
        .stat-value { font-size: 2em; font-weight: bold; color: #007bff; }
        .stat-label { color: #666; margin-top: 5px; }
        .loading { text-align: center; color: #666; }
        .error { color: #dc3545; background: #f8d7da; padding: 10px; border-radius: 5px; }
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 10px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background-color: #f8f9fa; }
        .success { color: #28a745; }
        .failure { color: #dc3545; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔄 Retry Daemon Dashboard</h1>
            <p>Real-time monitoring of retry operations</p>
        </div>

        <div class="card">
            <h2>System Statistics</h2>
            <div id="daemon-stats" class="loading">Loading daemon statistics...</div>
        </div>

        <div class="card">
            <h2>Metrics Overview (Last 24 Hours)</h2>
            <div id="metrics-stats" class="loading">Loading metrics statistics...</div>
        </div>

        <div class="card">
            <h2>Recent Operations</h2>
            <div id="recent-metrics" class="loading">Loading recent operations...</div>
        </div>
    </div>

    <script>
        // Fetch and display daemon stats
        fetch('/api/daemon/stats')
            .then(response => response.json())
            .then(data => {
                document.getElementById('daemon-stats').innerHTML = ` + "`" + `
                    <div class="stats">
                        <div class="stat">
                            <div class="stat-value">${data.total_metrics}</div>
                            <div class="stat-label">Total Metrics</div>
                        </div>
                        <div class="stat">
                            <div class="stat-value">${data.max_size}</div>
                            <div class="stat-label">Max Storage</div>
                        </div>
                        <div class="stat">
                            <div class="stat-value">${data.max_age}</div>
                            <div class="stat-label">Max Age</div>
                        </div>
                    </div>
                ` + "`" + `;
            })
            .catch(error => {
                document.getElementById('daemon-stats').innerHTML = ` + "`" + `<div class="error">Error loading daemon stats: ${error.message}</div>` + "`" + `;
            });

        // Fetch and display metrics stats
        fetch('/api/metrics/stats')
            .then(response => response.json())
            .then(data => {
                const successRate = (data.success_rate * 100).toFixed(1);
                document.getElementById('metrics-stats').innerHTML = ` + "`" + `
                    <div class="stats">
                        <div class="stat">
                            <div class="stat-value">${data.total_runs}</div>
                            <div class="stat-label">Total Runs</div>
                        </div>
                        <div class="stat">
                            <div class="stat-value ${data.successful_runs > data.failed_runs ? 'success' : 'failure'}">${successRate}%</div>
                            <div class="stat-label">Success Rate</div>
                        </div>
                        <div class="stat">
                            <div class="stat-value">${data.average_attempts.toFixed(1)}</div>
                            <div class="stat-label">Avg Attempts</div>
                        </div>
                        <div class="stat">
                            <div class="stat-value">${(data.average_duration / 1000000000).toFixed(2)}s</div>
                            <div class="stat-label">Avg Duration</div>
                        </div>
                    </div>
                ` + "`" + `;
            })
            .catch(error => {
                document.getElementById('metrics-stats').innerHTML = ` + "`" + `<div class="error">Error loading metrics stats: ${error.message}</div>` + "`" + `;
            });

        // Fetch and display recent metrics
        fetch('/api/metrics/recent?limit=10')
            .then(response => response.json())
            .then(data => {
                if (data.metrics.length === 0) {
                    document.getElementById('recent-metrics').innerHTML = '<p>No recent operations found.</p>';
                    return;
                }

                let html = '<table><thead><tr><th>Time</th><th>Command</th><th>Status</th><th>Attempts</th><th>Duration</th></tr></thead><tbody>';
                data.metrics.forEach(metric => {
                    const time = new Date(metric.timestamp).toLocaleString();
                    const status = metric.metrics.final_status === 'succeeded' ? 
                        '<span class="success">✓ Success</span>' : 
                        '<span class="failure">✗ Failed</span>';
                    const duration = metric.metrics.total_duration_seconds.toFixed(2) + 's';
                    
                    html += ` + "`" + `<tr>
                        <td>${time}</td>
                        <td><code>${metric.metrics.command}</code></td>
                        <td>${status}</td>
                        <td>${metric.metrics.total_attempts}</td>
                        <td>${duration}</td>
                    </tr>` + "`" + `;
                });
                html += '</tbody></table>';
                
                document.getElementById('recent-metrics').innerHTML = html;
            })
            .catch(error => {
                document.getElementById('recent-metrics').innerHTML = ` + "`" + `<div class="error">Error loading recent metrics: ${error.message}</div>` + "`" + `;
            });

        // Auto-refresh every 30 seconds
        setInterval(() => {
            location.reload();
        }, 30000);
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}
