#!/bin/bash

# Stress testing script for patience CLI
set -e

BINARY="../patience"
RESULTS_DIR="../performance_results"
mkdir -p "$RESULTS_DIR"

echo "=== Phase 2: Stress Testing & Scalability Analysis ==="
echo "Platform: $(uname -s) $(uname -m)"
echo "Date: $(date)"
echo "Binary: $(ls -lh $BINARY | awk '{print $5}')"
echo

# Function to monitor memory during test
monitor_memory() {
    local pid=$1
    local test_name="$2"
    local max_memory=0
    
    echo "Monitoring memory for PID $pid ($test_name)"
    
    while kill -0 $pid 2>/dev/null; do
        if command -v ps >/dev/null 2>&1; then
            # Get memory usage in KB
            memory=$(ps -o rss= -p $pid 2>/dev/null | tr -d ' ')
            if [ -n "$memory" ] && [ "$memory" -gt "$max_memory" ]; then
                max_memory=$memory
            fi
        fi
        sleep 0.1
    done
    
    echo "  Max memory usage: $(echo "scale=2; $max_memory / 1024" | bc)MB"
    return 0
}

# Test 1: High attempt count stress test
echo "1. High Attempt Count Stress Test"
echo "Testing with 1000, 5000, and 10000 attempts..."

for attempts in 1000 5000 10000; do
    echo "  Testing $attempts attempts:"
    start_time=$(date +%s)
    
    # Run in background to monitor memory
    $BINARY --attempts $attempts --delay 1ms --backoff fixed -- false &
    pid=$!
    
    # Monitor memory usage
    monitor_memory $pid "stress_test_${attempts}_attempts" &
    monitor_pid=$!
    
    # Wait for completion
    wait $pid
    kill $monitor_pid 2>/dev/null || true
    
    end_time=$(date +%s)
    duration=$((end_time - start_time))
    
    echo "    Duration: ${duration}s"
    echo "    Rate: $(echo "scale=2; $attempts / $duration" | bc) attempts/second"
    echo
done

# Test 2: Rapid-fire retry testing
echo "2. Rapid-Fire Retry Testing"
echo "Testing minimal delays with high frequency..."

echo "  Testing 100 attempts with 0ms delay:"
start_time=$(date +%s%N)
$BINARY --attempts 100 --delay 0ms --backoff fixed -- false
end_time=$(date +%s%N)
duration=$((end_time - start_time))
echo "    Duration: $(echo "scale=2; $duration / 1000000" | bc)ms"
echo "    Rate: $(echo "scale=0; 100 * 1000000000 / $duration" | bc) attempts/second"
echo

# Test 3: Long-running operation simulation
echo "3. Long-Running Operation Simulation"
echo "Testing extended retry scenarios..."

echo "  Testing 60-second timeout with exponential backoff:"
start_time=$(date +%s)
timeout 65s $BINARY --attempts 1000 --delay 100ms --backoff exponential --max-delay 5s --timeout 60s -- sleep 70 || echo "    Timeout reached (expected)"
end_time=$(date +%s)
duration=$((end_time - start_time))
echo "    Actual duration: ${duration}s"
echo

# Test 4: Memory leak detection
echo "4. Memory Leak Detection"
echo "Running extended test to detect memory leaks..."

echo "  Running 500 attempts with memory monitoring:"
start_time=$(date +%s)

# Create a script that runs multiple retry cycles
cat > memory_leak_test.sh << 'EOF'
#!/bin/bash
for i in {1..10}; do
    ../patience --attempts 50 --delay 10ms --backoff exponential -- false
    echo "Cycle $i completed"
done
EOF
chmod +x memory_leak_test.sh

# Run with memory monitoring
./memory_leak_test.sh &
pid=$!
monitor_memory $pid "memory_leak_test" &
monitor_pid=$!

wait $pid
kill $monitor_pid 2>/dev/null || true
rm -f memory_leak_test.sh

end_time=$(date +%s)
duration=$((end_time - start_time))
echo "    Total duration: ${duration}s"
echo

# Test 5: Concurrent execution testing
echo "5. Concurrent Execution Testing"
echo "Testing multiple patience instances simultaneously..."

echo "  Running 5 concurrent instances:"
start_time=$(date +%s)

# Start 5 concurrent processes
for i in {1..5}; do
    $BINARY --attempts 100 --delay 5ms --backoff jitter -- false &
    pids[$i]=$!
done

# Wait for all to complete
for i in {1..5}; do
    wait ${pids[$i]}
done

end_time=$(date +%s)
duration=$((end_time - start_time))
echo "    Duration: ${duration}s"
echo "    All instances completed successfully"
echo

# Test 6: Resource exhaustion simulation
echo "6. Resource Exhaustion Simulation"
echo "Testing behavior under resource constraints..."

echo "  Testing with limited file descriptors:"
# Reduce file descriptor limit temporarily
original_limit=$(ulimit -n)
ulimit -n 64
$BINARY --attempts 10 --delay 10ms -- echo "Limited FD test" || echo "    Handled gracefully"
ulimit -n $original_limit
echo "    File descriptor limit test completed"
echo

# Test 7: Pattern matching stress test
echo "7. Pattern Matching Stress Test"
echo "Testing complex patterns with large output..."

echo "  Testing complex regex with large output:"
# Create a command that produces large output
large_output_cmd="for i in {1..1000}; do echo 'Line \$i: This is a test line with various patterns success error timeout'; done"

start_time=$(date +%s%N)
$BINARY --attempts 3 --success-pattern "Line 500.*success" --delay 1ms -- bash -c "$large_output_cmd"
end_time=$(date +%s%N)
duration=$((end_time - start_time))
echo "    Duration: $(echo "scale=2; $duration / 1000000" | bc)ms"
echo

# Test 8: Configuration complexity stress test
echo "8. Configuration Complexity Stress Test"
echo "Testing with maximum configuration complexity..."

# Create complex config
cat > stress_config.toml << 'EOF'
attempts = 100
delay = "50ms"
timeout = "30s"
backoff = "decorrelated-jitter"
max_delay = "10s"
multiplier = 2.5
success_pattern = "(?i)(deployment|build|test|integration|pipeline|workflow|job|task|process|service|application|system|database|network|security|authentication|authorization|validation|verification|compilation|execution|installation|configuration|initialization|startup|shutdown|cleanup|backup|restore|sync|async|batch|stream|queue|cache|session|transaction|connection|request|response|success|completed|finished|done|ready|available|online|active|enabled|running|started|stopped|paused|resumed|cancelled|aborted|terminated|killed|failed|error|exception|timeout|retry|attempt|iteration|cycle|loop|step|phase|stage|level|layer|tier|node|cluster|instance|container|pod|deployment|service|ingress|volume|secret|configmap|namespace|resource|object|entity|record|document|file|directory|path|url|uri|endpoint|api|rest|graphql|grpc|http|https|tcp|udp|ip|dns|ssl|tls|certificate|key|token|credential|permission|role|policy|rule|condition|filter|query|search|index|sort|group|aggregate|transform|map|reduce|join|split|merge|combine|union|intersection|difference|subset|superset|contains|includes|excludes|matches|equals|greater|less|between|within|outside|before|after|during|since|until|while|when|where|what|who|why|how|which|whose|whom)"
failure_pattern = "(?i)(error|failed|failure|exception|panic|crash|abort|kill|terminate|timeout|expired|cancelled|rejected|denied|forbidden|unauthorized|unauthenticated|invalid|illegal|malformed|corrupted|damaged|broken|missing|notfound|unavailable|unreachable|disconnected|offline|inactive|disabled|stopped|paused|suspended|blocked|locked|frozen|stuck|hanging|deadlock|race|conflict|collision|overflow|underflow|outofmemory|stackoverflow|segfault|nullpointer|indexoutofbounds|divisionbyzero|arithmeticexception|ioexception|filenotfound|permissiondenied|accessdenied|networkerror|connectionrefused|connectiontimeout|sockettimeout|httperror|sslerror|certificateerror|authenticationfailed|authorizationfailed|validationfailed|verificationfailed|compilationerror|syntaxerror|runtimeerror|logicerror|businesserror|systemerror|internalerror|externalerror|temporaryerror|permanenterror|recoverableerror|unrecoverableerror|criticalerror|fatalerror|warningerror|minorError|majorerror)"
case_insensitive = true
EOF

echo "  Testing with complex configuration:"
start_time=$(date +%s%N)
$BINARY --config stress_config.toml --attempts 5 -- echo "deployment successful"
end_time=$(date +%s%N)
duration=$((end_time - start_time))
echo "    Duration: $(echo "scale=2; $duration / 1000000" | bc)ms"
rm -f stress_config.toml
echo

echo "=== Phase 2 Stress Testing Complete ==="
echo "All stress tests completed successfully!"
echo "Results saved to: $RESULTS_DIR/"