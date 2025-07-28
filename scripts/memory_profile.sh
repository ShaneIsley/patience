#!/bin/bash

# Memory profiling script for patience CLI
set -e

BINARY="../patience"
RESULTS_DIR="../tmp/performance_results"
mkdir -p "$RESULTS_DIR"

echo "=== Memory Profiling for Patience CLI ==="
echo "Platform: $(uname -s) $(uname -m)"
echo "Date: $(date)"
echo

# Test 1: Basic memory usage
echo "1. Basic Memory Usage Test"
echo "Command: $BINARY --attempts 1 -- echo test"
if command -v /usr/bin/time >/dev/null 2>&1; then
    /usr/bin/time -l $BINARY --attempts 1 -- echo "test" 2>&1 | grep -E "(maximum resident set size|real|user|sys)"
elif command -v time >/dev/null 2>&1; then
    time $BINARY --attempts 1 -- echo "test"
else
    echo "time command not available, running without memory measurement"
    $BINARY --attempts 1 -- echo "test"
fi
echo

# Test 2: High retry count memory usage
echo "2. High Retry Count Memory Test (100 attempts)"
echo "Command: $BINARY --attempts 100 --delay 1ms -- false"
if command -v /usr/bin/time >/dev/null 2>&1; then
    /usr/bin/time -l $BINARY --attempts 100 --delay 1ms -- false 2>&1 | grep -E "(maximum resident set size|real|user|sys)"
elif command -v time >/dev/null 2>&1; then
    time $BINARY --attempts 100 --delay 1ms -- false
else
    echo "time command not available, running without memory measurement"
    $BINARY --attempts 100 --delay 1ms -- false
fi
echo

# Test 3: Complex configuration memory usage
echo "3. Complex Configuration Memory Test"
echo "Command: $BINARY --attempts 10 --delay 100ms --backoff exponential --max-delay 5s --success-pattern 'success' --failure-pattern 'error' -- echo 'success'"
if command -v /usr/bin/time >/dev/null 2>&1; then
    /usr/bin/time -l $BINARY --attempts 10 --delay 100ms --backoff exponential --max-delay 5s --success-pattern "success" --failure-pattern "error" -- echo "success" 2>&1 | grep -E "(maximum resident set size|real|user|sys)"
elif command -v time >/dev/null 2>&1; then
    time $BINARY --attempts 10 --delay 100ms --backoff exponential --max-delay 5s --success-pattern "success" --failure-pattern "error" -- echo "success"
else
    echo "time command not available, running without memory measurement"
    $BINARY --attempts 10 --delay 100ms --backoff exponential --max-delay 5s --success-pattern "success" --failure-pattern "error" -- echo "success"
fi
echo

# Test 4: All backoff strategies memory comparison
echo "4. Backoff Strategies Memory Comparison"
strategies=("fixed" "exponential" "jitter" "linear" "decorrelated-jitter" "fibonacci")

for strategy in "${strategies[@]}"; do
    echo "Testing $strategy strategy:"
    if command -v /usr/bin/time >/dev/null 2>&1; then
        /usr/bin/time -l $BINARY --attempts 5 --delay 10ms --backoff "$strategy" -- echo "test" 2>&1 | grep -E "(maximum resident set size|real|user|sys)" | head -1
    elif command -v time >/dev/null 2>&1; then
        time $BINARY --attempts 5 --delay 10ms --backoff "$strategy" -- echo "test"
    else
        echo "time command not available, running without memory measurement"
        $BINARY --attempts 5 --delay 10ms --backoff "$strategy" -- echo "test" >/dev/null
    fi
done
echo

echo "=== Memory Profiling Complete ==="