package monitoring

import (
	"fmt"
	"runtime"
	"time"
)

// ResourceMonitor tracks system resource usage and enforces limits
type ResourceMonitor struct {
	maxMemoryMB    float64
	maxGoroutines  int
	maxFileHandles int
	enabled        bool
}

// ResourceSnapshot represents resource usage at a point in time
type ResourceSnapshot struct {
	Timestamp    time.Time
	AllocMB      float64
	SysMB        float64
	NumGoroutine int
	HeapObjects  uint64
}

// NewResourceMonitor creates a new resource monitor with specified limits
func NewResourceMonitor(maxMemoryMB float64, maxGoroutines int) *ResourceMonitor {
	return &ResourceMonitor{
		maxMemoryMB:   maxMemoryMB,
		maxGoroutines: maxGoroutines,
		enabled:       true,
	}
}

// Enable enables or disables resource monitoring
func (rm *ResourceMonitor) Enable(enabled bool) {
	rm.enabled = enabled
}

// GetSnapshot returns current resource usage
func (rm *ResourceMonitor) GetSnapshot() ResourceSnapshot {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return ResourceSnapshot{
		Timestamp:    time.Now(),
		AllocMB:      float64(m.Alloc) / 1024 / 1024,
		SysMB:        float64(m.Sys) / 1024 / 1024,
		NumGoroutine: runtime.NumGoroutine(),
		HeapObjects:  m.HeapObjects,
	}
}

// CheckLimits validates current resource usage against configured limits
func (rm *ResourceMonitor) CheckLimits() error {
	if !rm.enabled {
		return nil
	}

	snapshot := rm.GetSnapshot()

	// Check memory usage
	if snapshot.AllocMB > rm.maxMemoryMB {
		return fmt.Errorf("memory limit exceeded: %.2f MB > %.2f MB",
			snapshot.AllocMB, rm.maxMemoryMB)
	}

	// Check goroutine count
	if snapshot.NumGoroutine > rm.maxGoroutines {
		return fmt.Errorf("goroutine limit exceeded: %d > %d",
			snapshot.NumGoroutine, rm.maxGoroutines)
	}

	return nil
}

// ForceGC triggers garbage collection if memory usage is high
func (rm *ResourceMonitor) ForceGC(threshold float64) {
	if !rm.enabled {
		return
	}

	// Memory monitoring - removed manual GC call as per best practices
	// Let Go's garbage collector handle memory management automatically
}

// GetMemoryGrowth calculates memory growth since baseline
func (rm *ResourceMonitor) GetMemoryGrowth(baseline ResourceSnapshot) float64 {
	current := rm.GetSnapshot()
	return current.AllocMB - baseline.AllocMB
}

// GetGoroutineGrowth calculates goroutine growth since baseline
func (rm *ResourceMonitor) GetGoroutineGrowth(baseline ResourceSnapshot) int {
	current := rm.GetSnapshot()
	return current.NumGoroutine - baseline.NumGoroutine
}
