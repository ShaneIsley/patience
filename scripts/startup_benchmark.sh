#!/bin/bash

# Startup time benchmarking script for patience CLI
set -e

BINARY="../patience"
RESULTS_DIR="../performance_results"
mkdir -p "$RESULTS_DIR"

echo "=== Startup Time Benchmarking for Patience CLI ==="
echo "Platform: $(uname -s) $(uname -m)"
echo "Date: $(date)"
echo "Binary size: $(ls -lh $BINARY | awk '{print $5}')"
echo

# Function to measure startup time
measure_startup() {
    local cmd="$1"
    local description="$2"
    local iterations=100
    
    echo "Testing: $description"
    echo "Command: $cmd"
    echo "Iterations: $iterations"
    
    total_time=0
    min_time=999999
    max_time=0
    
    for i in $(seq 1 $iterations); do
        start_time=$(date +%s%N)
        eval "$cmd" >/dev/null 2>&1
        end_time=$(date +%s%N)
        
        duration=$((end_time - start_time))
        total_time=$((total_time + duration))
        
        if [ $duration -lt $min_time ]; then
            min_time=$duration
        fi
        
        if [ $duration -gt $max_time ]; then
            max_time=$duration
        fi
    done
    
    avg_time=$((total_time / iterations))
    
    echo "  Average: $(echo "scale=2; $avg_time / 1000000" | bc)ms"
    echo "  Minimum: $(echo "scale=2; $min_time / 1000000" | bc)ms"
    echo "  Maximum: $(echo "scale=2; $max_time / 1000000" | bc)ms"
    echo
}

# Test 1: Help command (fastest startup)
measure_startup "$BINARY --help" "Help command startup"

# Test 2: Version command
measure_startup "$BINARY --version" "Version command startup"

# Test 3: Basic command execution
measure_startup "$BINARY --attempts 1 -- echo test" "Basic command execution"

# Test 4: Config file loading
echo "attempts = 3" > test_config.toml
measure_startup "$BINARY --config test_config.toml --attempts 1 -- echo test" "Config file loading"
rm -f test_config.toml

# Test 5: Environment variable resolution
export PATIENCE_ATTEMPTS=1
measure_startup "$BINARY -- echo test" "Environment variable resolution"
unset PATIENCE_ATTEMPTS

# Test 6: Complex configuration
measure_startup "$BINARY --attempts 3 --delay 1ms --backoff exponential --max-delay 1s --success-pattern 'test' -- echo test" "Complex configuration"

echo "=== Startup Benchmarking Complete ==="