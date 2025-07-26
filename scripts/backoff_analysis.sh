#!/bin/bash

# Comprehensive backoff strategy analysis
set -e

BINARY="../patience"
RESULTS_DIR="../performance_results"
mkdir -p "$RESULTS_DIR"

echo "=== Phase 2.2: Backoff Strategy Performance Analysis ==="
echo "Platform: $(uname -s) $(uname -m)"
echo "Date: $(date)"
echo

# All available strategies
strategies=("fixed" "exponential" "jitter" "linear" "decorrelated-jitter" "fibonacci")

# Function to test strategy performance
test_strategy() {
    local strategy="$1"
    local attempts="$2"
    local delay="$3"
    local description="$4"
    
    echo "Testing $strategy strategy ($description):"
    echo "  Command: $BINARY --attempts $attempts --delay $delay --backoff $strategy -- false"
    
    # Run multiple iterations for accuracy
    total_time=0
    iterations=5
    
    for i in $(seq 1 $iterations); do
        start_time=$(date +%s%N)
        $BINARY --attempts $attempts --delay $delay --backoff $strategy -- false >/dev/null 2>&1
        end_time=$(date +%s%N)
        duration=$((end_time - start_time))
        total_time=$((total_time + duration))
    done
    
    avg_time=$((total_time / iterations))
    echo "  Average time: $(echo "scale=2; $avg_time / 1000000" | bc)ms"
    echo "  Time per attempt: $(echo "scale=2; $avg_time / $attempts / 1000000" | bc)ms"
    echo
}

# Test 1: Low attempt count comparison (5 attempts)
echo "1. Low Attempt Count Comparison (5 attempts, 10ms delay)"
for strategy in "${strategies[@]}"; do
    test_strategy "$strategy" 5 "10ms" "low attempts"
done

# Test 2: Medium attempt count comparison (25 attempts)
echo "2. Medium Attempt Count Comparison (25 attempts, 50ms delay)"
for strategy in "${strategies[@]}"; do
    test_strategy "$strategy" 25 "50ms" "medium attempts"
done

# Test 3: High attempt count comparison (100 attempts)
echo "3. High Attempt Count Comparison (100 attempts, 10ms delay)"
for strategy in "${strategies[@]}"; do
    test_strategy "$strategy" 100 "10ms" "high attempts"
done

# Test 4: Minimal delay comparison (1ms)
echo "4. Minimal Delay Comparison (20 attempts, 1ms delay)"
for strategy in "${strategies[@]}"; do
    test_strategy "$strategy" 20 "1ms" "minimal delay"
done

# Test 5: Zero delay comparison
echo "5. Zero Delay Comparison (50 attempts, 0ms delay)"
for strategy in "${strategies[@]}"; do
    test_strategy "$strategy" 50 "0ms" "zero delay"
done

# Test 6: Strategy-specific parameter testing
echo "6. Strategy-Specific Parameter Testing"

echo "6a. Exponential with different multipliers:"
for multiplier in 1.5 2.0 2.5 3.0; do
    echo "  Testing exponential with multiplier $multiplier:"
    start_time=$(date +%s%N)
    $BINARY --attempts 10 --delay 10ms --backoff exponential --multiplier $multiplier --max-delay 1s -- false >/dev/null 2>&1
    end_time=$(date +%s%N)
    duration=$((end_time - start_time))
    echo "    Time: $(echo "scale=2; $duration / 1000000" | bc)ms"
done
echo

echo "6b. Jitter with different max delays:"
for max_delay in "100ms" "500ms" "1s" "5s"; do
    echo "  Testing jitter with max-delay $max_delay:"
    start_time=$(date +%s%N)
    $BINARY --attempts 10 --delay 10ms --backoff jitter --max-delay $max_delay -- false >/dev/null 2>&1
    end_time=$(date +%s%N)
    duration=$((end_time - start_time))
    echo "    Time: $(echo "scale=2; $duration / 1000000" | bc)ms"
done
echo

echo "6c. Linear with different max delays:"
for max_delay in "200ms" "1s" "2s" "5s"; do
    echo "  Testing linear with max-delay $max_delay:"
    start_time=$(date +%s%N)
    $BINARY --attempts 15 --delay 50ms --backoff linear --max-delay $max_delay -- false >/dev/null 2>&1
    end_time=$(date +%s%N)
    duration=$((end_time - start_time))
    echo "    Time: $(echo "scale=2; $duration / 1000000" | bc)ms"
done
echo

# Test 7: Memory usage comparison
echo "7. Memory Usage Comparison by Strategy"
for strategy in "${strategies[@]}"; do
    echo "Testing $strategy memory usage:"
    if command -v /usr/bin/time >/dev/null 2>&1; then
        /usr/bin/time -l $BINARY --attempts 50 --delay 5ms --backoff $strategy -- false 2>&1 | grep "maximum resident set size" | awk '{print "  Memory: " $1/1024 "MB"}'
    else
        echo "  Memory monitoring not available"
    fi
done
echo

# Test 8: Calculation overhead measurement
echo "8. Calculation Overhead Measurement"
echo "Testing pure calculation performance (no actual delays):"

for strategy in "${strategies[@]}"; do
    echo "Testing $strategy calculation overhead:"
    start_time=$(date +%s%N)
    # Use 0ms delay to measure pure calculation time
    $BINARY --attempts 100 --delay 0ms --backoff $strategy -- false >/dev/null 2>&1
    end_time=$(date +%s%N)
    duration=$((end_time - start_time))
    echo "  Pure calculation time: $(echo "scale=2; $duration / 1000000" | bc)ms"
    echo "  Per-calculation: $(echo "scale=4; $duration / 100 / 1000000" | bc)ms"
done
echo

# Test 9: Edge case testing
echo "9. Edge Case Performance Testing"

echo "9a. Very high attempt counts:"
for strategy in "fixed" "exponential" "jitter"; do
    echo "  Testing $strategy with 1000 attempts:"
    start_time=$(date +%s)
    timeout 60s $BINARY --attempts 1000 --delay 1ms --backoff $strategy -- false >/dev/null 2>&1 || echo "    Timeout (expected for high counts)"
    end_time=$(date +%s)
    duration=$((end_time - start_time))
    echo "    Duration: ${duration}s"
done
echo

echo "9b. Extreme delay values:"
strategies_with_max=("exponential" "jitter" "linear" "decorrelated-jitter" "fibonacci")
for strategy in "${strategies_with_max[@]}"; do
    echo "  Testing $strategy with extreme max-delay:"
    start_time=$(date +%s%N)
    $BINARY --attempts 5 --delay 1ms --backoff $strategy --max-delay 10s -- false >/dev/null 2>&1
    end_time=$(date +%s%N)
    duration=$((end_time - start_time))
    echo "    $strategy: $(echo "scale=2; $duration / 1000000" | bc)ms"
done
echo

# Test 10: Randomization consistency (for jitter strategies)
echo "10. Randomization Consistency Testing"
jitter_strategies=("jitter" "decorrelated-jitter")

for strategy in "${jitter_strategies[@]}"; do
    echo "Testing $strategy randomization (5 runs):"
    times=()
    for i in {1..5}; do
        start_time=$(date +%s%N)
        $BINARY --attempts 10 --delay 10ms --backoff $strategy --max-delay 100ms -- false >/dev/null 2>&1
        end_time=$(date +%s%N)
        duration=$((end_time - start_time))
        times+=($duration)
        echo "  Run $i: $(echo "scale=2; $duration / 1000000" | bc)ms"
    done
    
    # Calculate variance
    total=0
    for time in "${times[@]}"; do
        total=$((total + time))
    done
    avg=$((total / 5))
    
    variance=0
    for time in "${times[@]}"; do
        diff=$((time - avg))
        variance=$((variance + diff * diff))
    done
    variance=$((variance / 5))
    
    echo "  Average: $(echo "scale=2; $avg / 1000000" | bc)ms"
    echo "  Variance: $(echo "scale=2; $variance / 1000000000000" | bc)ms²"
done
echo

echo "=== Phase 2.2 Backoff Strategy Analysis Complete ==="