package benchmarks

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// BenchmarkStartupTime measures CLI initialization time
func BenchmarkStartupTime(b *testing.B) {
	binary := "../patience"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		cmd := exec.Command(binary, "--help")
		err := cmd.Run()
		elapsed := time.Since(start)

		if err != nil {
			b.Fatalf("Command failed: %v", err)
		}

		// Record individual measurement
		b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/startup")
	}
}

// BenchmarkConfigLoading measures configuration loading performance
func BenchmarkConfigLoading(b *testing.B) {
	binary := "../patience"

	// Create test config file
	configContent := `
attempts = 5
delay = "1s"
timeout = "30s"
backoff = "exponential"
max_delay = "10s"
multiplier = 2.0
success_pattern = "success"
failure_pattern = "error"
case_insensitive = true
`

	configFile := "test_config.toml"
	err := os.WriteFile(configFile, []byte(configContent), 0644)
	if err != nil {
		b.Fatalf("Failed to create config file: %v", err)
	}
	defer os.Remove(configFile)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		cmd := exec.Command(binary, "--config", configFile, "--help")
		err := cmd.Run()
		elapsed := time.Since(start)

		if err != nil {
			b.Fatalf("Command failed: %v", err)
		}

		b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/config-load")
	}
}

// BenchmarkCommandOverhead measures patience wrapper overhead
func BenchmarkCommandOverhead(b *testing.B) {
	binary := "../patience"

	// Measure direct command execution
	b.Run("Direct", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			start := time.Now()
			cmd := exec.Command("echo", "test")
			err := cmd.Run()
			elapsed := time.Since(start)

			if err != nil {
				b.Fatalf("Direct command failed: %v", err)
			}

			b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/direct")
		}
	})

	// Measure patience-wrapped execution
	b.Run("Wrapped", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			start := time.Now()
			cmd := exec.Command(binary, "--attempts", "1", "--", "echo", "test")
			err := cmd.Run()
			elapsed := time.Since(start)

			if err != nil {
				b.Fatalf("Wrapped command failed: %v", err)
			}

			b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/wrapped")
		}
	})
}

// BenchmarkBackoffStrategies measures performance of different backoff strategies
func BenchmarkBackoffStrategies(b *testing.B) {
	binary := "../patience"
	strategies := []string{"fixed", "exponential", "jitter", "linear", "decorrelated-jitter", "fibonacci"}

	for _, strategy := range strategies {
		b.Run(strategy, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				start := time.Now()
				cmd := exec.Command(binary,
					"--attempts", "3",
					"--delay", "10ms",
					"--backoff", strategy,
					"--", "echo", "test")
				err := cmd.Run()
				elapsed := time.Since(start)

				if err != nil {
					b.Fatalf("Strategy %s failed: %v", strategy, err)
				}

				b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/strategy")
			}
		})
	}
}

// BenchmarkMemoryUsage measures memory consumption
func BenchmarkMemoryUsage(b *testing.B) {
	binary := "../patience"

	b.Run("BasicUsage", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var m1, m2 runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&m1)

			cmd := exec.Command(binary, "--attempts", "1", "--", "echo", "test")
			err := cmd.Run()

			runtime.GC()
			runtime.ReadMemStats(&m2)

			if err != nil {
				b.Fatalf("Command failed: %v", err)
			}

			memUsed := m2.TotalAlloc - m1.TotalAlloc
			b.ReportMetric(float64(memUsed), "bytes/op")
		}
	})
}

// BenchmarkHighAttemptCount tests performance with many retry attempts
func BenchmarkHighAttemptCount(b *testing.B) {
	binary := "../patience"
	attemptCounts := []int{10, 50, 100, 500}

	for _, attempts := range attemptCounts {
		b.Run(fmt.Sprintf("Attempts_%d", attempts), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				start := time.Now()
				cmd := exec.Command(binary,
					"--attempts", fmt.Sprintf("%d", attempts),
					"--delay", "1ms",
					"--backoff", "fixed",
					"--", "false") // Command that always fails
				_ = cmd.Run() // Expect failure
				elapsed := time.Since(start)

				b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/high-attempts")
			}
		})
	}
}

// BenchmarkPatternMatching measures regex pattern performance
func BenchmarkPatternMatching(b *testing.B) {
	binary := "../patience"

	patterns := []struct {
		name    string
		pattern string
	}{
		{"Simple", "success"},
		{"Complex", `(?i)(deployment|build).*(successful|completed|ready)`},
		{"JSON", `"status":\s*"(success|ok|completed)"`},
		{"HTTP", `HTTP/[0-9.]+ (200|201|202|204)`},
	}

	for _, p := range patterns {
		b.Run(p.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				start := time.Now()
				cmd := exec.Command(binary,
					"--attempts", "1",
					"--success-pattern", p.pattern,
					"--", "echo", "deployment successful")
				err := cmd.Run()
				elapsed := time.Since(start)

				if err != nil {
					b.Fatalf("Pattern %s failed: %v", p.name, err)
				}

				b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/pattern")
			}
		})
	}
}

// BenchmarkEnvironmentVariables measures env var resolution performance
func BenchmarkEnvironmentVariables(b *testing.B) {
	binary := "../patience"

	// Set environment variables
	envVars := map[string]string{
		"PATIENCE_ATTEMPTS":         "3",
		"PATIENCE_DELAY":            "100ms",
		"PATIENCE_TIMEOUT":          "5s",
		"PATIENCE_BACKOFF":          "exponential",
		"PATIENCE_MAX_DELAY":        "2s",
		"PATIENCE_MULTIPLIER":       "2.0",
		"PATIENCE_SUCCESS_PATTERN":  "success",
		"PATIENCE_FAILURE_PATTERN":  "error",
		"PATIENCE_CASE_INSENSITIVE": "true",
	}

	// Set environment variables
	for key, value := range envVars {
		os.Setenv(key, value)
	}
	defer func() {
		for key := range envVars {
			os.Unsetenv(key)
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		start := time.Now()
		cmd := exec.Command(binary, "--", "echo", "test")
		err := cmd.Run()
		elapsed := time.Since(start)

		if err != nil {
			b.Fatalf("Environment variable test failed: %v", err)
		}

		b.ReportMetric(float64(elapsed.Nanoseconds()), "ns/env-vars")
	}
}
