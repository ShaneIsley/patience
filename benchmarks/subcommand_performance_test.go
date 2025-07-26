package benchmarks

import (
	"fmt"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// Performance targets based on requirements
const (
	MaxStartupTimeMs = 5  // 5ms maximum startup time
	MaxMemoryUsageMB = 10 // 10MB maximum memory usage
	MaxOverheadMs    = 2  // 2ms maximum wrapper overhead
)

// BenchmarkSubcommandStartupTime measures CLI initialization time for new subcommand interface
func BenchmarkSubcommandStartupTime(b *testing.B) {
	binary := "../patience"

	// Test all strategy subcommands
	strategies := []string{"http-aware", "exponential", "linear", "fixed", "jitter", "decorrelated-jitter", "fibonacci"}

	for _, strategy := range strategies {
		b.Run(strategy, func(b *testing.B) {
			var totalTime time.Duration

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				start := time.Now()
				cmd := exec.Command(binary, strategy, "--help")
				err := cmd.Run()
				elapsed := time.Since(start)
				totalTime += elapsed

				if err != nil {
					b.Fatalf("Strategy %s help failed: %v", strategy, err)
				}

				// Fail if any single startup exceeds target
				if elapsed > MaxStartupTimeMs*time.Millisecond {
					b.Fatalf("Startup time %v exceeds target %dms for strategy %s", elapsed, MaxStartupTimeMs, strategy)
				}
			}

			avgTime := totalTime / time.Duration(b.N)
			b.ReportMetric(float64(avgTime.Nanoseconds()), "ns/startup")

			// Report if average exceeds target
			if avgTime > MaxStartupTimeMs*time.Millisecond {
				b.Errorf("Average startup time %v exceeds target %dms for strategy %s", avgTime, MaxStartupTimeMs, strategy)
			}
		})
	}
}

// BenchmarkSubcommandMemoryUsage measures memory consumption for subcommand interface
func BenchmarkSubcommandMemoryUsage(b *testing.B) {
	binary := "../patience"
	strategies := []string{"http-aware", "exponential", "linear", "fixed"}

	for _, strategy := range strategies {
		b.Run(strategy, func(b *testing.B) {
			var maxMemory uint64

			for i := 0; i < b.N; i++ {
				var m1, m2 runtime.MemStats
				runtime.GC()
				runtime.ReadMemStats(&m1)

				cmd := exec.Command(binary, strategy, "--attempts", "1", "--", "echo", "test")
				err := cmd.Run()

				runtime.GC()
				runtime.ReadMemStats(&m2)

				if err != nil {
					b.Fatalf("Strategy %s execution failed: %v", strategy, err)
				}

				memUsed := m2.TotalAlloc - m1.TotalAlloc
				if memUsed > maxMemory {
					maxMemory = memUsed
				}

				b.ReportMetric(float64(memUsed), "bytes/op")
			}

			// Check memory usage target
			maxMemoryMB := float64(maxMemory) / (1024 * 1024)
			if maxMemoryMB > MaxMemoryUsageMB {
				b.Errorf("Memory usage %.2fMB exceeds target %dMB for strategy %s", maxMemoryMB, MaxMemoryUsageMB, strategy)
			}
		})
	}
}

// BenchmarkSubcommandOverhead measures wrapper overhead for new interface
func BenchmarkSubcommandOverhead(b *testing.B) {
	binary := "../patience"

	// Measure direct command execution baseline
	var directTime time.Duration
	b.Run("Direct", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			start := time.Now()
			cmd := exec.Command("echo", "test")
			err := cmd.Run()
			elapsed := time.Since(start)
			directTime += elapsed

			if err != nil {
				b.Fatalf("Direct command failed: %v", err)
			}

			b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/direct")
		}
		directTime = directTime / time.Duration(b.N)
	})

	// Measure patience-wrapped execution with different strategies
	strategies := []string{"fixed", "exponential", "http-aware"}

	for _, strategy := range strategies {
		b.Run(strategy, func(b *testing.B) {
			var totalOverhead time.Duration

			for i := 0; i < b.N; i++ {
				start := time.Now()
				cmd := exec.Command(binary, strategy, "--attempts", "1", "--", "echo", "test")
				err := cmd.Run()
				elapsed := time.Since(start)

				if err != nil {
					b.Fatalf("Strategy %s failed: %v", strategy, err)
				}

				overhead := elapsed - directTime
				totalOverhead += overhead

				b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/wrapped")
				b.ReportMetric(float64(overhead.Nanoseconds()), "ns/overhead")
			}

			avgOverhead := totalOverhead / time.Duration(b.N)
			if avgOverhead > MaxOverheadMs*time.Millisecond {
				b.Errorf("Average overhead %v exceeds target %dms for strategy %s", avgOverhead, MaxOverheadMs, strategy)
			}
		})
	}
}

// BenchmarkHTTPAwarePerformance measures HTTP-aware strategy specific performance
func BenchmarkHTTPAwarePerformance(b *testing.B) {
	binary := "../patience"

	// Test HTTP response parsing performance
	b.Run("HTTPResponseParsing", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			start := time.Now()
			// Use a command that outputs HTTP-like response
			cmd := exec.Command(binary, "http-aware", "--attempts", "1", "--",
				"sh", "-c", "echo 'HTTP/1.1 429 Too Many Requests\nRetry-After: 1\n\n{\"error\":\"rate limited\"}'")
			err := cmd.Run()
			elapsed := time.Since(start)

			if err != nil {
				b.Fatalf("HTTP-aware parsing test failed: %v", err)
			}

			b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/http-parsing")
		}
	})

	// Test fallback strategy performance
	b.Run("FallbackStrategy", func(b *testing.B) {
		fallbacks := []string{"exponential", "linear", "fixed"}

		for _, fallback := range fallbacks {
			b.Run(fallback, func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					start := time.Now()
					cmd := exec.Command(binary, "http-aware", "--fallback", fallback, "--attempts", "1", "--", "echo", "test")
					err := cmd.Run()
					elapsed := time.Since(start)

					if err != nil {
						b.Fatalf("HTTP-aware with fallback %s failed: %v", fallback, err)
					}

					b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/fallback")
				}
			})
		}
	})
}

// BenchmarkStrategySpecificPerformance measures performance of each strategy's unique features
func BenchmarkStrategySpecificPerformance(b *testing.B) {
	binary := "../patience"

	testCases := []struct {
		name     string
		strategy string
		args     []string
	}{
		{"Exponential", "exponential", []string{"--base-delay", "1ms", "--multiplier", "2.0", "--max-delay", "100ms"}},
		{"Linear", "linear", []string{"--increment", "1ms", "--max-delay", "100ms"}},
		{"Fixed", "fixed", []string{"--delay", "1ms"}},
		{"Jitter", "jitter", []string{"--base-delay", "1ms", "--multiplier", "2.0", "--max-delay", "100ms"}},
		{"DecorrelatedJitter", "decorrelated-jitter", []string{"--base-delay", "1ms", "--multiplier", "2.0", "--max-delay", "100ms"}},
		{"Fibonacci", "fibonacci", []string{"--base-delay", "1ms", "--max-delay", "100ms"}},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				start := time.Now()

				args := []string{tc.strategy}
				args = append(args, tc.args...)
				args = append(args, "--attempts", "3", "--", "echo", "test")

				cmd := exec.Command(binary, args...)
				err := cmd.Run()
				elapsed := time.Since(start)

				if err != nil {
					b.Fatalf("Strategy %s failed: %v", tc.strategy, err)
				}

				b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/strategy")
			}
		})
	}
}

// BenchmarkPatternMatchingPerformance measures regex pattern performance with subcommands
func BenchmarkPatternMatchingPerformance(b *testing.B) {
	binary := "../patience"

	patterns := []struct {
		name    string
		pattern string
		output  string
	}{
		{"Simple", "success", "operation successful"},
		{"Complex", `(?i)(deployment|build).*(successful|completed|ready)`, "deployment completed successfully"},
		{"JSON", `"status":\s*"(success|ok|completed)"`, `{"status": "success", "message": "done"}`},
		{"HTTP", `HTTP/[0-9.]+ (200|201|202|204)`, "HTTP/1.1 200 OK"},
	}

	strategies := []string{"fixed", "exponential", "http-aware"}

	for _, strategy := range strategies {
		for _, p := range patterns {
			b.Run(fmt.Sprintf("%s_%s", strategy, p.name), func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					start := time.Now()
					cmd := exec.Command(binary, strategy,
						"--attempts", "1",
						"--success-pattern", p.pattern,
						"--", "echo", p.output)
					err := cmd.Run()
					elapsed := time.Since(start)

					if err != nil {
						b.Fatalf("Pattern %s with strategy %s failed: %v", p.name, strategy, err)
					}

					b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/pattern")
				}
			})
		}
	}
}

// BenchmarkConcurrentExecution measures performance under concurrent load
func BenchmarkConcurrentExecution(b *testing.B) {
	binary := "../patience"

	concurrencyLevels := []int{1, 5, 10, 20}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrent_%d", concurrency), func(b *testing.B) {
			b.SetParallelism(concurrency)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					start := time.Now()
					cmd := exec.Command(binary, "exponential", "--attempts", "1", "--", "echo", "test")
					err := cmd.Run()
					elapsed := time.Since(start)

					if err != nil {
						b.Fatalf("Concurrent execution failed: %v", err)
					}

					b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/concurrent")
				}
			})
		})
	}
}

// TestPerformanceTargets validates that performance targets are met
func TestPerformanceTargets(t *testing.T) {
	binary := "../patience"

	// Test startup time target
	t.Run("StartupTimeTarget", func(t *testing.T) {
		strategies := []string{"http-aware", "exponential", "fixed"}

		for _, strategy := range strategies {
			start := time.Now()
			cmd := exec.Command(binary, strategy, "--help")
			err := cmd.Run()
			elapsed := time.Since(start)

			if err != nil {
				t.Fatalf("Strategy %s help failed: %v", strategy, err)
			}

			if elapsed > MaxStartupTimeMs*time.Millisecond {
				t.Errorf("Startup time %v exceeds target %dms for strategy %s", elapsed, MaxStartupTimeMs, strategy)
			}
		}
	})

	// Test memory usage target
	t.Run("MemoryUsageTarget", func(t *testing.T) {
		var m1, m2 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m1)

		cmd := exec.Command(binary, "exponential", "--attempts", "1", "--", "echo", "test")
		err := cmd.Run()

		runtime.GC()
		runtime.ReadMemStats(&m2)

		if err != nil {
			t.Fatalf("Memory test execution failed: %v", err)
		}

		memUsed := m2.TotalAlloc - m1.TotalAlloc
		memUsedMB := float64(memUsed) / (1024 * 1024)

		if memUsedMB > MaxMemoryUsageMB {
			t.Errorf("Memory usage %.2fMB exceeds target %dMB", memUsedMB, MaxMemoryUsageMB)
		}
	})

	// Test that all strategies work correctly
	t.Run("StrategyFunctionality", func(t *testing.T) {
		strategies := []string{"http-aware", "exponential", "linear", "fixed", "jitter", "decorrelated-jitter", "fibonacci"}

		for _, strategy := range strategies {
			cmd := exec.Command(binary, strategy, "--attempts", "1", "--", "echo", "test")
			err := cmd.Run()

			if err != nil {
				t.Errorf("Strategy %s failed to execute: %v", strategy, err)
			}
		}
	})
}

// BenchmarkRegressionTest compares old vs new interface performance
func BenchmarkRegressionTest(t *testing.B) {
	binary := "../patience"

	// This test will fail initially since we're comparing against the old interface
	// It serves as our RED test to ensure we maintain performance parity

	t.Run("OldVsNewInterface", func(t *testing.B) {
		// Test old interface (this should fail since it's deprecated)
		t.Run("OldInterface", func(t *testing.B) {
			for i := 0; i < t.N; i++ {
				start := time.Now()
				cmd := exec.Command(binary, "--attempts", "1", "--", "echo", "test")
				err := cmd.Run()
				elapsed := time.Since(start)

				// We expect this to fail since old interface is deprecated
				if err == nil {
					t.Errorf("Old interface should be deprecated but still works")
				}

				t.ReportMetric(float64(elapsed.Nanoseconds()), "ns/old")
			}
		})

		// Test new interface
		t.Run("NewInterface", func(t *testing.B) {
			for i := 0; i < t.N; i++ {
				start := time.Now()
				cmd := exec.Command(binary, "fixed", "--attempts", "1", "--", "echo", "test")
				err := cmd.Run()
				elapsed := time.Since(start)

				if err != nil {
					t.Fatalf("New interface failed: %v", err)
				}

				t.ReportMetric(float64(elapsed.Nanoseconds()), "ns/new")
			}
		})
	})
}
