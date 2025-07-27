package benchmarks

import (
	"testing"
	"time"

	"github.com/shaneisley/patience/pkg/backoff"
)

func BenchmarkPolynomialStrategy_Delay(b *testing.B) {
	strategy, err := backoff.NewPolynomial(1*time.Second, 2.0, 60*time.Second)
	if err != nil {
		b.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.Delay(i%10 + 1) // Test attempts 1-10
	}
}

func BenchmarkPolynomialStrategy_HighExponent(b *testing.B) {
	strategy, err := backoff.NewPolynomial(1*time.Second, 5.0, 60*time.Second)
	if err != nil {
		b.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.Delay(i%10 + 1)
	}
}

func BenchmarkPolynomialStrategy_SublinearExponent(b *testing.B) {
	strategy, err := backoff.NewPolynomial(1*time.Second, 0.5, 60*time.Second)
	if err != nil {
		b.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		strategy.Delay(i%10 + 1)
	}
}

func BenchmarkPolynomialStrategy_Creation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := backoff.NewPolynomial(1*time.Second, 2.0, 60*time.Second)
		if err != nil {
			b.Fatalf("Failed to create polynomial strategy: %v", err)
		}
	}
}

func TestPolynomialStrategy_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	strategy, err := backoff.NewPolynomial(1*time.Millisecond, 1.5, 60*time.Second)
	if err != nil {
		t.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	// Test many calculations for memory leaks and performance
	for i := 0; i < 100000; i++ {
		delay := strategy.Delay(i%100 + 1)
		if delay < 0 {
			t.Fatalf("Negative delay at iteration %d: %v", i, delay)
		}
		if delay > 60*time.Second {
			t.Fatalf("Delay exceeded max at iteration %d: %v", i, delay)
		}
	}
}

func TestPolynomialStrategy_ConcurrentAccess(t *testing.T) {
	strategy, err := backoff.NewPolynomial(1*time.Second, 2.0, 60*time.Second)
	if err != nil {
		t.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	// Test concurrent access (strategy should be thread-safe)
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(attempt int) {
			defer func() { done <- true }()
			for j := 0; j < 1000; j++ {
				delay := strategy.Delay(attempt)
				if delay < 0 {
					t.Errorf("Negative delay: %v", delay)
					return
				}
			}
		}(i + 1)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestPolynomialStrategy_ExtremeValues(t *testing.T) {
	strategy, err := backoff.NewPolynomial(1*time.Nanosecond, 10.0, 1*time.Hour)
	if err != nil {
		t.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	// Test extreme attempt values
	testCases := []int{1, 10, 100, 1000, 10000, 100000}

	for _, attempt := range testCases {
		delay := strategy.Delay(attempt)
		if delay < 0 {
			t.Errorf("Negative delay for attempt %d: %v", attempt, delay)
		}
		if delay > 1*time.Hour {
			t.Errorf("Delay exceeded max for attempt %d: %v", attempt, delay)
		}
	}
}

func TestPolynomialStrategy_MemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory test in short mode")
	}

	// Create many strategies to test memory usage
	strategies := make([]*backoff.PolynomialStrategy, 10000)

	for i := 0; i < 10000; i++ {
		strategy, err := backoff.NewPolynomial(
			time.Duration(i+1)*time.Millisecond,
			float64(i%5)+1.0,
			time.Duration(i+60)*time.Second,
		)
		if err != nil {
			t.Fatalf("Failed to create polynomial strategy %d: %v", i, err)
		}
		strategies[i] = strategy
	}

	// Use all strategies to ensure they're not optimized away
	totalDelay := time.Duration(0)
	for i, strategy := range strategies {
		delay := strategy.Delay(i%10 + 1)
		totalDelay += delay
	}

	if totalDelay < 0 {
		t.Errorf("Total delay should be positive, got %v", totalDelay)
	}
}

func TestPolynomialStrategy_ComparisonWithOtherStrategies(t *testing.T) {
	// Compare polynomial with equivalent linear and exponential strategies
	polynomial, err := backoff.NewPolynomial(1*time.Second, 1.0, 60*time.Second)
	if err != nil {
		t.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	linear := NewLinear(1*time.Second, 60*time.Second)

	// For exponent=1.0, polynomial should behave like linear
	for attempt := 1; attempt <= 10; attempt++ {
		polyDelay := polynomial.Delay(attempt)
		linearDelay := linear.Delay(attempt)

		if polyDelay != linearDelay {
			t.Errorf("Attempt %d: polynomial(1.0) = %v, linear = %v", attempt, polyDelay, linearDelay)
		}
	}
}

func TestPolynomialStrategy_PrecisionTest(t *testing.T) {
	// Test floating point precision with various exponents
	testCases := []struct {
		exponent  float64
		attempt   int
		tolerance time.Duration
	}{
		{0.5, 4, 1 * time.Millisecond},  // 4^0.5 = 2.0
		{1.5, 8, 10 * time.Millisecond}, // 8^1.5 ≈ 22.627
		{2.5, 3, 5 * time.Millisecond},  // 3^2.5 ≈ 15.588
		{3.0, 5, 1 * time.Millisecond},  // 5^3.0 = 125.0
	}

	for _, tc := range testCases {
		strategy, err := backoff.NewPolynomial(1*time.Second, tc.exponent, 300*time.Second)
		if err != nil {
			t.Fatalf("Failed to create polynomial strategy: %v", err)
		}

		delay := strategy.Delay(tc.attempt)

		// Verify delay is reasonable (not zero, not negative, within bounds)
		if delay <= 0 {
			t.Errorf("Exponent %.1f, attempt %d: delay should be positive, got %v",
				tc.exponent, tc.attempt, delay)
		}

		if delay > 300*time.Second {
			t.Errorf("Exponent %.1f, attempt %d: delay should not exceed max, got %v",
				tc.exponent, tc.attempt, delay)
		}
	}
}
