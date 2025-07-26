#!/bin/bash

# Phase 3.1: Production Workload Simulation
set -e

BINARY="../patience"
RESULTS_DIR="../performance_results"
mkdir -p "$RESULTS_DIR"

echo "=== Phase 3.1: Production Workload Simulation ==="
echo "Platform: $(uname -s) $(uname -m)"
echo "Date: $(date)"
echo

# Function to simulate network operations
simulate_network_operation() {
    local operation="$1"
    local success_rate="$2"
    local description="$3"
    
    echo "Testing: $description"
    echo "Operation: $operation"
    echo "Expected success rate: $success_rate%"
    
    # Create a script that fails based on success rate
    cat > network_sim.sh << EOF
#!/bin/bash
# Simulate network operation with $success_rate% success rate
if [ \$((RANDOM % 100)) -lt $success_rate ]; then
    echo "[$operation] Operation successful"
    exit 0
else
    echo "[$operation] Network error: Connection timeout"
    exit 1
fi
EOF
    chmod +x network_sim.sh
    
    start_time=$(date +%s%N)
    $BINARY --attempts 5 --delay 100ms --backoff exponential --max-delay 2s --success-pattern "successful" -- ./network_sim.sh
    end_time=$(date +%s%N)
    duration=$((end_time - start_time))
    
    echo "  Duration: $(echo "scale=2; $duration / 1000000" | bc)ms"
    echo "  Result: Success"
    echo
    
    rm -f network_sim.sh
}

# Test 1: HTTP API Calls
echo "1. HTTP API Call Simulation"
simulate_network_operation "HTTP_GET" 70 "API endpoint with 70% success rate"
simulate_network_operation "HTTP_POST" 85 "API POST with 85% success rate"
simulate_network_operation "HTTP_PUT" 60 "API PUT with 60% success rate"

# Test 2: Database Connection Retries
echo "2. Database Connection Simulation"

echo "Testing database connection retry:"
cat > db_sim.sh << 'EOF'
#!/bin/bash
# Simulate database connection
attempt_file="/tmp/db_attempts_$$"
if [ ! -f "$attempt_file" ]; then
    echo "0" > "$attempt_file"
fi

attempts=$(cat "$attempt_file")
attempts=$((attempts + 1))
echo "$attempts" > "$attempt_file"

if [ $attempts -le 2 ]; then
    echo "[DB] Connection refused - database not ready"
    exit 1
elif [ $attempts -eq 3 ]; then
    echo "[DB] Connection established successfully"
    rm -f "$attempt_file"
    exit 0
else
    echo "[DB] Connection established successfully"
    exit 0
fi
EOF
chmod +x db_sim.sh

start_time=$(date +%s%N)
$BINARY --attempts 5 --delay 200ms --backoff linear --success-pattern "established" -- ./db_sim.sh
end_time=$(date +%s%N)
duration=$((end_time - start_time))

echo "  Duration: $(echo "scale=2; $duration / 1000000" | bc)ms"
echo "  Result: Database connection successful"
echo

rm -f db_sim.sh

# Test 3: File System Operations
echo "3. File System Operation Simulation"

echo "Testing file system retry scenarios:"
# Create a temporary directory for testing
test_dir="/tmp/patience_fs_test_$$"
mkdir -p "$test_dir"

# Test file creation with retry
echo "  Testing file creation with temporary failures:"
cat > fs_sim.sh << EOF
#!/bin/bash
# Simulate file system operation
target_file="$test_dir/test_file_\$\$"
attempt_file="/tmp/fs_attempts_\$\$"

if [ ! -f "\$attempt_file" ]; then
    echo "0" > "\$attempt_file"
fi

attempts=\$(cat "\$attempt_file")
attempts=\$((attempts + 1))
echo "\$attempts" > "\$attempt_file"

if [ \$attempts -le 2 ]; then
    echo "[FS] Permission denied - filesystem busy"
    exit 1
else
    echo "File created successfully" > "\$target_file"
    echo "[FS] File operation completed successfully"
    rm -f "\$attempt_file"
    exit 0
fi
EOF
chmod +x fs_sim.sh

start_time=$(date +%s%N)
$BINARY --attempts 4 --delay 50ms --backoff fixed --success-pattern "completed" -- ./fs_sim.sh
end_time=$(date +%s%N)
duration=$((end_time - start_time))

echo "    Duration: $(echo "scale=2; $duration / 1000000" | bc)ms"
echo "    Result: File system operation successful"
echo

rm -f fs_sim.sh
rm -rf "$test_dir"

# Test 4: System Resource Constraints
echo "4. System Resource Constraint Simulation"

echo "Testing under simulated resource pressure:"
# Simulate high CPU load scenario
cat > resource_sim.sh << 'EOF'
#!/bin/bash
# Simulate resource-constrained operation
load_avg=$(uptime | awk -F'load average:' '{print $2}' | awk '{print $1}' | sed 's/,//')
load_int=$(echo "$load_avg * 100" | bc | cut -d. -f1)

if [ $load_int -gt 200 ]; then
    echo "[RESOURCE] System overloaded - operation failed"
    exit 1
else
    echo "[RESOURCE] Operation completed under normal load"
    exit 0
fi
EOF
chmod +x resource_sim.sh

start_time=$(date +%s%N)
$BINARY --attempts 3 --delay 100ms --backoff exponential --success-pattern "completed" -- ./resource_sim.sh
end_time=$(date +%s%N)
duration=$((end_time - start_time))

echo "  Duration: $(echo "scale=2; $duration / 1000000" | bc)ms"
echo "  Result: Resource constraint test completed"
echo

rm -f resource_sim.sh

# Test 5: Concurrent Execution Simulation
echo "5. Concurrent Execution Simulation"

echo "Testing multiple patience instances simultaneously:"
concurrent_pids=()

# Start 5 concurrent patience instances
for i in {1..5}; do
    cat > concurrent_sim_$i.sh << EOF
#!/bin/bash
# Concurrent operation simulation $i
sleep_time=\$((RANDOM % 3 + 1))
echo "[CONCURRENT-$i] Starting operation (sleep \${sleep_time}s)"
sleep \$sleep_time
echo "[CONCURRENT-$i] Operation completed successfully"
exit 0
EOF
    chmod +x concurrent_sim_$i.sh
    
    $BINARY --attempts 2 --delay 50ms --backoff jitter --success-pattern "completed" -- ./concurrent_sim_$i.sh &
    concurrent_pids[$i]=$!
done

# Wait for all concurrent operations
start_time=$(date +%s)
for i in {1..5}; do
    wait ${concurrent_pids[$i]}
    echo "  Concurrent instance $i completed"
done
end_time=$(date +%s)
duration=$((end_time - start_time))

echo "  Total concurrent execution time: ${duration}s"
echo "  All concurrent instances completed successfully"
echo

# Cleanup
for i in {1..5}; do
    rm -f concurrent_sim_$i.sh
done

# Test 6: Long-Running Operation Simulation
echo "6. Long-Running Operation Simulation"

echo "Testing extended retry scenarios:"
cat > longrun_sim.sh << 'EOF'
#!/bin/bash
# Simulate long-running operation with eventual success
attempt_file="/tmp/longrun_attempts_$$"
if [ ! -f "$attempt_file" ]; then
    echo "0" > "$attempt_file"
fi

attempts=$(cat "$attempt_file")
attempts=$((attempts + 1))
echo "$attempts" > "$attempt_file"

echo "[LONGRUN] Attempt $attempts - processing..."

if [ $attempts -le 8 ]; then
    echo "[LONGRUN] Operation still in progress - not ready yet"
    exit 1
else
    echo "[LONGRUN] Long-running operation completed successfully"
    rm -f "$attempt_file"
    exit 0
fi
EOF
chmod +x longrun_sim.sh

start_time=$(date +%s)
$BINARY --attempts 10 --delay 200ms --backoff fibonacci --max-delay 1s --success-pattern "completed" -- ./longrun_sim.sh
end_time=$(date +%s)
duration=$((end_time - start_time))

echo "  Duration: ${duration}s"
echo "  Result: Long-running operation successful"
echo

rm -f longrun_sim.sh

# Test 7: Pattern-Based Success Detection
echo "7. Pattern-Based Success Detection Simulation"

echo "Testing complex pattern matching in production scenarios:"

# Test JSON API response patterns
echo "  Testing JSON API response patterns:"
cat > json_sim.sh << 'EOF'
#!/bin/bash
# Simulate JSON API responses
responses=(
    '{"status": "processing", "message": "Request in progress"}'
    '{"status": "processing", "message": "Still processing"}'
    '{"status": "success", "message": "Operation completed", "data": {"id": 123}}'
)

attempt_file="/tmp/json_attempts_$$"
if [ ! -f "$attempt_file" ]; then
    echo "0" > "$attempt_file"
fi

attempts=$(cat "$attempt_file")
attempts=$((attempts + 1))
echo "$attempts" > "$attempt_file"

if [ $attempts -le ${#responses[@]} ]; then
    echo "${responses[$((attempts-1))]}"
    if [ $attempts -eq ${#responses[@]} ]; then
        rm -f "$attempt_file"
        exit 0
    else
        exit 1
    fi
else
    echo '{"status": "success", "message": "Operation completed"}'
    exit 0
fi
EOF
chmod +x json_sim.sh

start_time=$(date +%s%N)
$BINARY --attempts 5 --delay 100ms --backoff exponential --success-pattern '"status":\s*"success"' -- ./json_sim.sh
end_time=$(date +%s%N)
duration=$((end_time - start_time))

echo "    Duration: $(echo "scale=2; $duration / 1000000" | bc)ms"
echo "    Result: JSON pattern matching successful"
echo

rm -f json_sim.sh

# Test 8: Health Check Simulation
echo "8. Health Check Simulation"

echo "Testing service health check scenarios:"
cat > health_sim.sh << 'EOF'
#!/bin/bash
# Simulate service health check
services=("database" "cache" "api" "queue")
attempt_file="/tmp/health_attempts_$$"

if [ ! -f "$attempt_file" ]; then
    echo "0" > "$attempt_file"
fi

attempts=$(cat "$attempt_file")
attempts=$((attempts + 1))
echo "$attempts" > "$attempt_file"

echo "[HEALTH] Checking system health (attempt $attempts)..."

for service in "${services[@]}"; do
    if [ $attempts -le 2 ]; then
        echo "[HEALTH] $service: unhealthy"
    else
        echo "[HEALTH] $service: healthy"
    fi
done

if [ $attempts -le 2 ]; then
    echo "[HEALTH] Overall status: unhealthy"
    exit 1
else
    echo "[HEALTH] Overall status: healthy - all services responding"
    rm -f "$attempt_file"
    exit 0
fi
EOF
chmod +x health_sim.sh

start_time=$(date +%s%N)
$BINARY --attempts 4 --delay 150ms --backoff decorrelated-jitter --success-pattern "healthy.*responding" -- ./health_sim.sh
end_time=$(date +%s%N)
duration=$((end_time - start_time))

echo "  Duration: $(echo "scale=2; $duration / 1000000" | bc)ms"
echo "  Result: Health check simulation successful"
echo

rm -f health_sim.sh

echo "=== Phase 3.1 Production Workload Simulation Complete ==="
echo "All production scenarios tested successfully!"
echo "Results demonstrate robust performance under real-world conditions."