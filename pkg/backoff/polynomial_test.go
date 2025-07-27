package backoff

import (
	"testing"
	"time"
)

func TestPolynomialStrategy_Delay_QuadraticGrowth(t *testing.T) {
	// Test quadratic growth (exponent = 2.0)
	strategy, err := NewPolynomial(1*time.Second, 2.0, 60*time.Second)
	if err != nil {
		t.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	testCases := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1 * time.Second},  // 1^2 = 1
		{2, 4 * time.Second},  // 2^2 = 4
		{3, 9 * time.Second},  // 3^2 = 9
		{4, 16 * time.Second}, // 4^2 = 16
		{5, 25 * time.Second}, // 5^2 = 25
	}

	for _, tc := range testCases {
		actual := strategy.Delay(tc.attempt)
		if actual != tc.expected {
			t.Errorf("Delay(%d) = %v, expected %v", tc.attempt, actual, tc.expected)
		}
	}
}

func TestPolynomialStrategy_Delay_SublinearGrowth(t *testing.T) {
	// Test sublinear growth (exponent = 0.5)
	strategy, err := NewPolynomial(1*time.Second, 0.5, 60*time.Second)
	if err != nil {
		t.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	testCases := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1000 * time.Millisecond},  // 1^0.5 = 1.0
		{4, 2000 * time.Millisecond},  // 4^0.5 = 2.0
		{9, 3000 * time.Millisecond},  // 9^0.5 = 3.0
		{16, 4000 * time.Millisecond}, // 16^0.5 = 4.0
	}

	for _, tc := range testCases {
		actual := strategy.Delay(tc.attempt)
		if actual != tc.expected {
			t.Errorf("Delay(%d) = %v, expected %v", tc.attempt, actual, tc.expected)
		}
	}
}

func TestPolynomialStrategy_Delay_MaxDelayRespected(t *testing.T) {
	// Test max delay cap
	maxDelay := 10 * time.Second
	strategy, err := NewPolynomial(1*time.Second, 3.0, maxDelay)
	if err != nil {
		t.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	// Large attempt should be capped at maxDelay
	actual := strategy.Delay(100) // 100^3 would be huge
	if actual != maxDelay {
		t.Errorf("Delay(100) = %v, expected %v (max delay)", actual, maxDelay)
	}
}

func TestPolynomialStrategy_Delay_LinearEquivalence(t *testing.T) {
	// Test that exponent=1.0 behaves like linear
	strategy, err := NewPolynomial(2*time.Second, 1.0, 60*time.Second)
	if err != nil {
		t.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	testCases := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 2 * time.Second}, // 2 * 1^1 = 2
		{2, 4 * time.Second}, // 2 * 2^1 = 4
		{3, 6 * time.Second}, // 2 * 3^1 = 6
		{4, 8 * time.Second}, // 2 * 4^1 = 8
	}

	for _, tc := range testCases {
		actual := strategy.Delay(tc.attempt)
		if actual != tc.expected {
			t.Errorf("Delay(%d) = %v, expected %v", tc.attempt, actual, tc.expected)
		}
	}
}

func TestPolynomialStrategy_Delay_ModerateGrowth(t *testing.T) {
	// Test moderate growth (exponent = 1.5)
	strategy, err := NewPolynomial(1*time.Second, 1.5, 60*time.Second)
	if err != nil {
		t.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	testCases := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1000 * time.Millisecond},  // 1^1.5 = 1.0
		{2, 2828 * time.Millisecond},  // 2^1.5 ≈ 2.828
		{3, 5196 * time.Millisecond},  // 3^1.5 ≈ 5.196
		{4, 8000 * time.Millisecond},  // 4^1.5 = 8.0
		{8, 22627 * time.Millisecond}, // 8^1.5 ≈ 22.627
	}

	for _, tc := range testCases {
		actual := strategy.Delay(tc.attempt)
		// Allow 1ms tolerance for floating point precision
		if abs(actual-tc.expected) > time.Millisecond {
			t.Errorf("Delay(%d) = %v, expected %v (tolerance: 1ms)", tc.attempt, actual, tc.expected)
		}
	}
}

func TestPolynomialStrategy_EdgeCases(t *testing.T) {
	strategy, err := NewPolynomial(1*time.Second, 2.0, 60*time.Second)
	if err != nil {
		t.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	// Test attempt 0 (should handle gracefully)
	delay := strategy.Delay(0)
	if delay < 0 {
		t.Errorf("Delay(0) should not be negative, got %v", delay)
	}

	// Test negative attempt (should handle gracefully)
	delay = strategy.Delay(-1)
	if delay < 0 {
		t.Errorf("Delay(-1) should not be negative, got %v", delay)
	}

	// Test very large attempt (should be capped)
	delay = strategy.Delay(1000000)
	if delay > 60*time.Second {
		t.Errorf("Delay(1000000) should be capped at max delay, got %v", delay)
	}
}

func TestNewPolynomial_ParameterValidation(t *testing.T) {
	// Test invalid parameters
	testCases := []struct {
		name      string
		baseDelay time.Duration
		exponent  float64
		maxDelay  time.Duration
		shouldErr bool
	}{
		{"Valid parameters", 1 * time.Second, 2.0, 60 * time.Second, false},
		{"Zero base delay", 0, 2.0, 60 * time.Second, true},
		{"Negative base delay", -1 * time.Second, 2.0, 60 * time.Second, true},
		{"Negative exponent", 1 * time.Second, -1.0, 60 * time.Second, true},
		{"Zero max delay", 1 * time.Second, 2.0, 0, true},
		{"Negative max delay", 1 * time.Second, 2.0, -1 * time.Second, true},
		{"Base > max", 2 * time.Second, 2.0, 1 * time.Second, true},
		{"Zero exponent (valid)", 1 * time.Second, 0.0, 60 * time.Second, false},
		{"Fractional exponent", 1 * time.Second, 0.5, 60 * time.Second, false},
		{"Large exponent", 1 * time.Second, 10.0, 60 * time.Second, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewPolynomial(tc.baseDelay, tc.exponent, tc.maxDelay)
			hasErr := err != nil
			if hasErr != tc.shouldErr {
				t.Errorf("Expected error=%v, got error=%v (err: %v)", tc.shouldErr, hasErr, err)
			}
		})
	}
}

func TestPolynomialStrategy_GetterMethods(t *testing.T) {
	baseDelay := 2 * time.Second
	exponent := 1.5
	maxDelay := 30 * time.Second

	strategy, err := NewPolynomial(baseDelay, exponent, maxDelay)
	if err != nil {
		t.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	if strategy.GetBaseDelay() != baseDelay {
		t.Errorf("GetBaseDelay() = %v, expected %v", strategy.GetBaseDelay(), baseDelay)
	}

	if strategy.GetExponent() != exponent {
		t.Errorf("GetExponent() = %v, expected %v", strategy.GetExponent(), exponent)
	}

	if strategy.GetMaxDelay() != maxDelay {
		t.Errorf("GetMaxDelay() = %v, expected %v", strategy.GetMaxDelay(), maxDelay)
	}
}

func TestPolynomialStrategy_String(t *testing.T) {
	strategy, err := NewPolynomial(1*time.Second, 2.0, 60*time.Second)
	if err != nil {
		t.Fatalf("Failed to create polynomial strategy: %v", err)
	}

	str := strategy.String()
	expectedSubstrings := []string{"polynomial", "1s", "2.0", "1m0s"}

	for _, substr := range expectedSubstrings {
		if !contains(str, substr) {
			t.Errorf("String() = %q, expected to contain %q", str, substr)
		}
	}
}

// Helper functions
func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
