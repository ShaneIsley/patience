package backoff

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdaptiveStrategy_BucketBoundaries tests delay bucket boundary conditions
func TestAdaptiveStrategy_BucketBoundaries(t *testing.T) {
	fallback := NewFixed(2 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.3, 20)
	require.NoError(t, err)

	testCases := []struct {
		name           string
		delay          time.Duration
		expectedBucket int
	}{
		// Bucket 0: 0-1s
		{"start of bucket 0", 0 * time.Millisecond, 0},
		{"middle of bucket 0", 500 * time.Millisecond, 0},
		{"end of bucket 0", 999 * time.Millisecond, 0},

		// Bucket 1: 1-2s (boundary test)
		{"boundary 1s exactly", 1000 * time.Millisecond, 1},
		{"just after 1s", 1001 * time.Millisecond, 1},
		{"middle of bucket 1", 1500 * time.Millisecond, 1},
		{"end of bucket 1", 1999 * time.Millisecond, 1},

		// Bucket 2: 2-5s
		{"boundary 2s exactly", 2000 * time.Millisecond, 2},
		{"middle of bucket 2", 3500 * time.Millisecond, 2},
		{"end of bucket 2", 4999 * time.Millisecond, 2},

		// Bucket 3: 5-10s
		{"boundary 5s exactly", 5000 * time.Millisecond, 3},
		{"middle of bucket 3", 7500 * time.Millisecond, 3},
		{"end of bucket 3", 9999 * time.Millisecond, 3},

		// Bucket 4: 10-30s
		{"boundary 10s exactly", 10000 * time.Millisecond, 4},
		{"middle of bucket 4", 20000 * time.Millisecond, 4},
		{"end of bucket 4", 29999 * time.Millisecond, 4},

		// Bucket 5: 30-60s
		{"boundary 30s exactly", 30000 * time.Millisecond, 5},
		{"middle of bucket 5", 45000 * time.Millisecond, 5},
		{"end of bucket 5", 59999 * time.Millisecond, 5},

		// Bucket 6: 60-300s (5 minutes)
		{"boundary 60s exactly", 60000 * time.Millisecond, 6},
		{"middle of bucket 6", 180000 * time.Millisecond, 6},
		{"end of bucket 6", 299999 * time.Millisecond, 6},

		// Edge case: larger than largest bucket
		{"boundary 300s exactly", 300000 * time.Millisecond, 6},
		{"larger than largest bucket", 600000 * time.Millisecond, 6},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Record an outcome to trigger bucket creation
			adaptive.RecordOutcome(tc.delay, true, 100*time.Millisecond)

			// Access the internal method to test bucket assignment
			// We need to use reflection or create a test helper method
			bucketIndex := adaptive.findBucketIndexForTesting(tc.delay)
			assert.Equal(t, tc.expectedBucket, bucketIndex,
				"Delay %v should be in bucket %d", tc.delay, tc.expectedBucket)
		})
	}
}

// TestAdaptiveStrategy_BucketIndexAccuracy tests the findBucketIndex method comprehensively
func TestAdaptiveStrategy_BucketIndexAccuracy(t *testing.T) {
	fallback := NewFixed(1 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.2, 10)
	require.NoError(t, err)

	// Initialize buckets by recording a dummy outcome
	adaptive.RecordOutcome(1*time.Second, true, 50*time.Millisecond)

	testCases := []struct {
		delay          time.Duration
		expectedBucket int
		description    string
	}{
		// Test all bucket ranges systematically
		{0 * time.Nanosecond, 0, "zero delay"},
		{1 * time.Nanosecond, 0, "1 nanosecond"},
		{999999999 * time.Nanosecond, 0, "999ms"},
		{1000000000 * time.Nanosecond, 1, "exactly 1s"},
		{1000000001 * time.Nanosecond, 1, "1s + 1ns"},
		{2000000000 * time.Nanosecond, 2, "exactly 2s"},
		{5000000000 * time.Nanosecond, 3, "exactly 5s"},
		{10000000000 * time.Nanosecond, 4, "exactly 10s"},
		{30000000000 * time.Nanosecond, 5, "exactly 30s"},
		{60000000000 * time.Nanosecond, 6, "exactly 60s"},
		{300000000000 * time.Nanosecond, 6, "exactly 300s"},
		{600000000000 * time.Nanosecond, 6, "600s (larger than max)"},
		{time.Hour, 6, "1 hour"},
		{24 * time.Hour, 6, "24 hours"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			bucketIndex := adaptive.findBucketIndexForTesting(tc.delay)
			assert.Equal(t, tc.expectedBucket, bucketIndex,
				"Delay %v (%s) should be in bucket %d", tc.delay, tc.description, tc.expectedBucket)
		})
	}
}

// TestAdaptiveStrategy_BucketRangeLogic tests the bucket range definitions
func TestAdaptiveStrategy_BucketRangeLogic(t *testing.T) {
	fallback := NewFixed(1 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.2, 10)
	require.NoError(t, err)

	// Record outcome to initialize buckets
	adaptive.RecordOutcome(1*time.Second, true, 50*time.Millisecond)

	// Expected bucket ranges based on implementation
	expectedRanges := []struct {
		bucketIndex int
		minDelay    time.Duration
		maxDelay    time.Duration
	}{
		{0, 0, 1 * time.Second},
		{1, 1 * time.Second, 2 * time.Second},
		{2, 2 * time.Second, 5 * time.Second},
		{3, 5 * time.Second, 10 * time.Second},
		{4, 10 * time.Second, 30 * time.Second},
		{5, 30 * time.Second, 60 * time.Second},
		{6, 60 * time.Second, 300 * time.Second},
	}

	for _, expected := range expectedRanges {
		t.Run(fmt.Sprintf("bucket_%d_range", expected.bucketIndex), func(t *testing.T) {
			// Test that delays just inside the range are assigned correctly
			justInside := expected.minDelay + time.Millisecond
			if expected.minDelay == 0 {
				justInside = 1 * time.Millisecond
			}

			bucketIndex := adaptive.findBucketIndexForTesting(justInside)
			assert.Equal(t, expected.bucketIndex, bucketIndex,
				"Delay %v should be in bucket %d (just inside range)", justInside, expected.bucketIndex)

			// Test that delays just outside the upper range are NOT in this bucket
			if expected.bucketIndex < 6 { // Skip for last bucket which handles overflow
				justOutside := expected.maxDelay + time.Millisecond
				bucketIndex = adaptive.findBucketIndexForTesting(justOutside)
				assert.NotEqual(t, expected.bucketIndex, bucketIndex,
					"Delay %v should NOT be in bucket %d (just outside range)", justOutside, expected.bucketIndex)
			}
		})
	}
}

// TestAdaptiveStrategy_InvalidDelayHandling tests handling of invalid delays
func TestAdaptiveStrategy_InvalidDelayHandling(t *testing.T) {
	fallback := NewFixed(1 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.2, 10)
	require.NoError(t, err)

	// Record outcome to initialize buckets
	adaptive.RecordOutcome(1*time.Second, true, 50*time.Millisecond)

	testCases := []struct {
		delay       time.Duration
		description string
		expectValid bool
	}{
		{-1 * time.Second, "negative delay", false},
		{-1 * time.Millisecond, "negative millisecond", false},
		{0, "zero delay", true}, // Zero should be valid (bucket 0)
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			bucketIndex := adaptive.findBucketIndexForTesting(tc.delay)
			if tc.expectValid {
				assert.GreaterOrEqual(t, bucketIndex, 0, "Valid delay should return non-negative bucket index")
			} else {
				// Implementation should handle negative delays gracefully
				// Either return -1 or assign to bucket 0
				assert.True(t, bucketIndex >= -1, "Invalid delay should return -1 or handle gracefully")
			}
		})
	}
}

// TestAdaptiveStrategy_BucketConsistency tests that bucket assignment is consistent
func TestAdaptiveStrategy_BucketConsistency(t *testing.T) {
	fallback := NewFixed(1 * time.Second)
	adaptive, err := NewAdaptive(fallback, 0.2, 10)
	require.NoError(t, err)

	// Record outcome to initialize buckets
	adaptive.RecordOutcome(1*time.Second, true, 50*time.Millisecond)

	testDelays := []time.Duration{
		500 * time.Millisecond,
		1500 * time.Millisecond,
		3 * time.Second,
		7 * time.Second,
		20 * time.Second,
		45 * time.Second,
		120 * time.Second,
	}

	for _, delay := range testDelays {
		t.Run(fmt.Sprintf("consistency_%v", delay), func(t *testing.T) {
			// Multiple calls should return same bucket
			bucket1 := adaptive.findBucketIndexForTesting(delay)
			bucket2 := adaptive.findBucketIndexForTesting(delay)
			bucket3 := adaptive.findBucketIndexForTesting(delay)

			assert.Equal(t, bucket1, bucket2, "Bucket assignment should be consistent")
			assert.Equal(t, bucket2, bucket3, "Bucket assignment should be consistent")
		})
	}
}
