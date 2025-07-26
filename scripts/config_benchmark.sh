#!/bin/bash

# Configuration performance benchmarking script
set -e

BINARY="../patience"
RESULTS_DIR="../performance_results"
mkdir -p "$RESULTS_DIR"

echo "=== Configuration Performance Benchmarking ==="
echo "Platform: $(uname -s) $(uname -m)"
echo "Date: $(date)"
echo

# Function to measure config performance
measure_config() {
    local setup_cmd="$1"
    local test_cmd="$2"
    local description="$3"
    local iterations=50
    
    echo "Testing: $description"
    echo "Setup: $setup_cmd"
    echo "Command: $test_cmd"
    
    # Setup
    eval "$setup_cmd"
    
    total_time=0
    for i in $(seq 1 $iterations); do
        start_time=$(date +%s%N)
        eval "$test_cmd" >/dev/null 2>&1
        end_time=$(date +%s%N)
        duration=$((end_time - start_time))
        total_time=$((total_time + duration))
    done
    
    avg_time=$((total_time / iterations))
    echo "  Average: $(echo "scale=2; $avg_time / 1000000" | bc)ms"
    echo
    
    # Cleanup
    rm -f *.toml *.yaml
    for var in PATIENCE_ATTEMPTS PATIENCE_DELAY PATIENCE_TIMEOUT PATIENCE_BACKOFF PATIENCE_MAX_DELAY PATIENCE_MULTIPLIER PATIENCE_SUCCESS_PATTERN PATIENCE_FAILURE_PATTERN PATIENCE_CASE_INSENSITIVE; do
        unset $var
    done
}

# Test 1: Simple TOML config
measure_config \
    "echo 'attempts = 3' > simple.toml" \
    "$BINARY --config simple.toml --attempts 1 -- echo test" \
    "Simple TOML configuration"

# Test 2: Complex TOML config
measure_config \
    "cat > complex.toml << 'EOF'
attempts = 10
delay = \"500ms\"
timeout = \"30s\"
backoff = \"exponential\"
max_delay = \"10s\"
multiplier = 2.5
success_pattern = \"(?i)(success|completed|ready)\"
failure_pattern = \"(?i)(error|failed|timeout)\"
case_insensitive = true
EOF" \
    "$BINARY --config complex.toml --attempts 1 -- echo test" \
    "Complex TOML configuration"

# Test 3: Environment variables (minimal)
measure_config \
    "export PATIENCE_ATTEMPTS=3" \
    "$BINARY --attempts 1 -- echo test" \
    "Minimal environment variables"

# Test 4: Environment variables (comprehensive)
measure_config \
    "export PATIENCE_ATTEMPTS=5
export PATIENCE_DELAY=100ms
export PATIENCE_TIMEOUT=30s
export PATIENCE_BACKOFF=exponential
export PATIENCE_MAX_DELAY=5s
export PATIENCE_MULTIPLIER=2.0
export PATIENCE_SUCCESS_PATTERN=success
export PATIENCE_FAILURE_PATTERN=error
export PATIENCE_CASE_INSENSITIVE=true" \
    "$BINARY --attempts 1 -- echo test" \
    "Comprehensive environment variables"

# Test 5: Precedence resolution (config + env + flags)
measure_config \
    "echo 'attempts = 10' > precedence.toml
export PATIENCE_DELAY=200ms" \
    "$BINARY --config precedence.toml --attempts 1 --backoff exponential -- echo test" \
    "Precedence resolution (config + env + flags)"

# Test 6: Auto-discovery
measure_config \
    "echo 'attempts = 5' > .patience.toml" \
    "$BINARY --attempts 1 -- echo test" \
    "Auto-discovery of .patience.toml"

# Test 7: Invalid config handling
measure_config \
    "echo 'invalid_field = true' > invalid.toml" \
    "$BINARY --config invalid.toml --attempts 1 -- echo test || true" \
    "Invalid configuration handling"

# Test 8: Large config file
measure_config \
    "cat > large.toml << 'EOF'
# Large configuration file with comments
attempts = 100
delay = \"1s\"
timeout = \"60s\"
backoff = \"decorrelated-jitter\"
max_delay = \"30s\"
multiplier = 3.0

# Pattern matching configuration
success_pattern = \"(?i)(deployment|build|test|integration).*(successful|completed|passed|ready|ok|done)\"
failure_pattern = \"(?i)(error|failed|timeout|exception|panic|crash|abort|kill|terminate)\"
case_insensitive = true

# Additional comments to increase file size
# This is a comprehensive configuration for production use
# It includes all available options with sensible defaults
# The patterns are designed to catch common success/failure indicators
# in modern CI/CD and deployment scenarios
EOF" \
    "$BINARY --config large.toml --attempts 1 -- echo test" \
    "Large configuration file with comments"

echo "=== Configuration Benchmarking Complete ==="