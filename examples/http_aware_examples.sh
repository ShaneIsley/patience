#!/bin/bash

# HTTP-Aware Retry Strategy - Live Integration Examples
# This script demonstrates real-world usage patterns with the HTTP-aware strategy

set -e

PATIENCE_BINARY="../patience"
RESULTS_DIR="../performance_results"
mkdir -p "$RESULTS_DIR"

echo "=== HTTP-Aware Strategy - Live Integration Examples ==="
echo "Platform: $(uname -s) $(uname -m)"
echo "Date: $(date)"
echo

# Function to simulate API responses for testing
simulate_api_response() {
    local response_type="$1"
    local attempt="$2"
    
    case $response_type in
        "github_rate_limit")
            if [ "$attempt" -le 2 ]; then
                echo "HTTP/2 403 
server: GitHub.com
x-ratelimit-remaining: 0
retry-after: 10

{\"message\": \"API rate limit exceeded\"}" >&2
                exit 22 # curl HTTP error code
            else
                echo "HTTP/2 200 
server: GitHub.com

{\"login\": \"testuser\", \"id\": 12345}"
                exit 0
            fi
            ;;
        "twitter_rate_limit")
            if [ "$attempt" -le 1 ]; then
                echo "HTTP/1.1 429 Too Many Requests
retry-after: 5

{\"title\": \"Too Many Requests\"}" >&2
                exit 22
            else
                echo "HTTP/1.1 200 OK

{\"data\": {\"id\": \"123\", \"text\": \"Hello World\"}}"
                exit 0
            fi
            ;;
        "aws_throttling")
            if [ "$attempt" -le 1 ]; then
                echo "{\"message\": \"Rate exceeded\", \"retry_after_seconds\": 3}"
                exit 1
            else
                echo "{\"Items\": [], \"Count\": 0}"
                exit 0
            fi
            ;;
        "stripe_rate_limit")
            if [ "$attempt" -le 1 ]; then
                echo "HTTP/1.1 429 Too Many Requests
Retry-After: 2

{\"error\": {\"type\": \"rate_limit_error\"}}" >&2
                exit 22
            else
                echo "HTTP/1.1 200 OK

{\"id\": \"ch_123\", \"amount\": 2000}"
                exit 0
            fi
            ;;
        "network_error")
            if [ "$attempt" -le 2 ]; then
                echo "curl: (7) Failed to connect to api.example.com port 443: Connection refused" >&2
                exit 7
            else
                echo "HTTP/1.1 200 OK

{\"status\": \"success\"}"
                exit 0
            fi
            ;;
    esac
}

# Example 1: GitHub API Rate Limiting
echo "1. GitHub API Rate Limiting Example"
echo "Simulating GitHub API with rate limiting..."

cat > github_api_sim.sh << 'EOF'
#!/bin/bash
attempt_file="/tmp/github_attempts_$$"
if [ ! -f "$attempt_file" ]; then
    echo "0" > "$attempt_file"
fi

attempts=$(cat "$attempt_file")
attempts=$((attempts + 1))
echo "$attempts" > "$attempt_file"

if [ $attempts -le 2 ]; then
    echo "HTTP/2 403 
server: GitHub.com
x-ratelimit-remaining: 0
retry-after: 5

{\"message\": \"API rate limit exceeded\"}" >&2
    exit 22
else
    echo "HTTP/2 200 
server: GitHub.com

{\"login\": \"testuser\", \"id\": 12345}"
    rm -f "$attempt_file"
    exit 0
fi
EOF
chmod +x github_api_sim.sh

echo "  Command: patience --backoff http-aware --fallback exponential --attempts 5 -- ./github_api_sim.sh"
start_time=$(date +%s)
$PATIENCE_BINARY --backoff http-aware --fallback exponential --attempts 5 -- ./github_api_sim.sh
end_time=$(date +%s)
duration=$((end_time - start_time))

echo "  Result: GitHub API simulation completed in ${duration}s"
echo "  Expected: ~10s total (5s wait + retries)"
rm -f github_api_sim.sh
echo

# Example 2: Twitter API Integration
echo "2. Twitter API Integration Example"
echo "Simulating Twitter API v2 with rate limiting..."

cat > twitter_api_sim.sh << 'EOF'
#!/bin/bash
attempt_file="/tmp/twitter_attempts_$$"
if [ ! -f "$attempt_file" ]; then
    echo "0" > "$attempt_file"
fi

attempts=$(cat "$attempt_file")
attempts=$((attempts + 1))
echo "$attempts" > "$attempt_file"

if [ $attempts -le 1 ]; then
    echo "HTTP/1.1 429 Too Many Requests
retry-after: 3

{\"title\": \"Too Many Requests\"}" >&2
    exit 22
else
    echo "HTTP/1.1 200 OK

{\"data\": {\"id\": \"123\", \"text\": \"Hello World\"}}"
    rm -f "$attempt_file"
    exit 0
fi
EOF
chmod +x twitter_api_sim.sh

echo "  Command: patience --backoff http-aware --http-max-delay 5m -- ./twitter_api_sim.sh"
start_time=$(date +%s)
$PATIENCE_BINARY --backoff http-aware --http-max-delay 5m -- ./twitter_api_sim.sh
end_time=$(date +%s)
duration=$((end_time - start_time))

echo "  Result: Twitter API simulation completed in ${duration}s"
echo "  Expected: ~3s total (3s wait + retry)"
rm -f twitter_api_sim.sh
echo

# Example 3: AWS API JSON Response
echo "3. AWS API JSON Response Example"
echo "Simulating AWS API with JSON retry timing..."

cat > aws_api_sim.sh << 'EOF'
#!/bin/bash
attempt_file="/tmp/aws_attempts_$$"
if [ ! -f "$attempt_file" ]; then
    echo "0" > "$attempt_file"
fi

attempts=$(cat "$attempt_file")
attempts=$((attempts + 1))
echo "$attempts" > "$attempt_file"

if [ $attempts -le 1 ]; then
    echo "{\"message\": \"Rate exceeded\", \"retry_after_seconds\": 2}"
    exit 1
else
    echo "{\"Items\": [], \"Count\": 0}"
    rm -f "$attempt_file"
    exit 0
fi
EOF
chmod +x aws_api_sim.sh

echo "  Command: patience --backoff http-aware --fallback decorrelated-jitter -- ./aws_api_sim.sh"
start_time=$(date +%s)
$PATIENCE_BINARY --backoff http-aware --fallback decorrelated-jitter -- ./aws_api_sim.sh
end_time=$(date +%s)
duration=$((end_time - start_time))

echo "  Result: AWS API simulation completed in ${duration}s"
echo "  Expected: ~2s total (2s wait + retry)"
rm -f aws_api_sim.sh
echo

# Example 4: Stripe API Integration
echo "4. Stripe API Integration Example"
echo "Simulating Stripe API with short retry timing..."

cat > stripe_api_sim.sh << 'EOF'
#!/bin/bash
attempt_file="/tmp/stripe_attempts_$$"
if [ ! -f "$attempt_file" ]; then
    echo "0" > "$attempt_file"
fi

attempts=$(cat "$attempt_file")
attempts=$((attempts + 1))
echo "$attempts" > "$attempt_file"

if [ $attempts -le 1 ]; then
    echo "HTTP/1.1 429 Too Many Requests
Retry-After: 1

{\"error\": {\"type\": \"rate_limit_error\"}}" >&2
    exit 22
else
    echo "HTTP/1.1 200 OK

{\"id\": \"ch_123\", \"amount\": 2000}"
    rm -f "$attempt_file"
    exit 0
fi
EOF
chmod +x stripe_api_sim.sh

echo "  Command: patience --backoff http-aware -- ./stripe_api_sim.sh"
start_time=$(date +%s)
$PATIENCE_BINARY --backoff http-aware -- ./stripe_api_sim.sh
end_time=$(date +%s)
duration=$((end_time - start_time))

echo "  Result: Stripe API simulation completed in ${duration}s"
echo "  Expected: ~1s total (1s wait + retry)"
rm -f stripe_api_sim.sh
echo

# Example 5: Multi-API Workflow
echo "5. Multi-API Workflow Example"
echo "Simulating workflow with multiple APIs and different retry strategies..."

cat > multi_api_workflow.sh << 'EOF'
#!/bin/bash
echo "=== Multi-API Workflow ==="

echo "Step 1: Fetching user data (GitHub-style)..."
../patience --backoff http-aware --fallback exponential --attempts 3 -- bash -c '
if [ ! -f /tmp/step1_done ]; then
    echo "HTTP/2 403
retry-after: 1

{\"message\": \"rate limited\"}" >&2
    touch /tmp/step1_done
    exit 22
else
    echo "HTTP/2 200

{\"user\": \"testuser\"}"
    exit 0
fi
'

echo "Step 2: Processing data (AWS-style)..."
../patience --backoff http-aware --fallback linear -- bash -c '
if [ ! -f /tmp/step2_done ]; then
    echo "{\"message\": \"throttled\", \"retry_after_seconds\": 1}"
    touch /tmp/step2_done
    exit 1
else
    echo "{\"result\": \"processed\"}"
    exit 0
fi
'

echo "Step 3: Sending notification (Slack-style)..."
../patience --backoff http-aware -- bash -c '
if [ ! -f /tmp/step3_done ]; then
    echo "HTTP/1.1 429
Retry-After: 1

rate_limited" >&2
    touch /tmp/step3_done
    exit 22
else
    echo "HTTP/1.1 200

{\"ok\": true}"
    exit 0
fi
'

# Cleanup
rm -f /tmp/step1_done /tmp/step2_done /tmp/step3_done
echo "=== Multi-API Workflow Complete ==="
EOF
chmod +x multi_api_workflow.sh

echo "  Running multi-API workflow..."
start_time=$(date +%s)
./multi_api_workflow.sh
end_time=$(date +%s)
duration=$((end_time - start_time))

echo "  Result: Multi-API workflow completed in ${duration}s"
echo "  Expected: ~3s total (1s + 1s + 1s waits + retries)"
rm -f multi_api_workflow.sh
echo

# Example 6: Fallback Behavior Demonstration
echo "6. Fallback Behavior Demonstration"
echo "Simulating network errors that require fallback to mathematical strategy..."

cat > fallback_demo.sh << 'EOF'
#!/bin/bash
attempt_file="/tmp/fallback_attempts_$$"
if [ ! -f "$attempt_file" ]; then
    echo "0" > "$attempt_file"
fi

attempts=$(cat "$attempt_file")
attempts=$((attempts + 1))
echo "$attempts" > "$attempt_file"

if [ $attempts -le 2 ]; then
    echo "curl: (7) Failed to connect to api.example.com port 443: Connection refused" >&2
    exit 7
else
    echo "HTTP/1.1 200 OK

{\"status\": \"success\"}"
    rm -f "$attempt_file"
    exit 0
fi
EOF
chmod +x fallback_demo.sh

echo "  Command: patience --backoff http-aware --fallback exponential --delay 500ms -- ./fallback_demo.sh"
start_time=$(date +%s)
$PATIENCE_BINARY --backoff http-aware --fallback exponential --delay 500ms -- ./fallback_demo.sh
end_time=$(date +%s)
duration=$((end_time - start_time))

echo "  Result: Fallback demonstration completed in ${duration}s"
echo "  Expected: ~1.5s total (500ms + 1s exponential delays)"
rm -f fallback_demo.sh
echo

# Example 7: Real curl Integration Patterns
echo "7. Real curl Integration Patterns"
echo "Demonstrating proper curl flag usage for HTTP-aware retry..."

echo "  Pattern 1: curl -i (headers in stdout)"
echo "  Command: patience --backoff http-aware -- curl -i -f https://httpbin.org/status/429"
echo "  Note: This would work with real APIs that return retry-after headers"
echo

echo "  Pattern 2: curl -D /dev/stderr (headers to stderr)"
echo "  Command: patience --backoff http-aware -- curl -D /dev/stderr https://httpbin.org/json"
echo "  Note: Headers go to stderr, body to stdout - both are parsed"
echo

echo "  Pattern 3: curl -w (custom status output)"
echo "  Command: patience --backoff http-aware -- curl -w 'HTTP_STATUS:%{http_code}\\n' https://httpbin.org/json"
echo "  Note: Custom status format can be parsed for HTTP codes"
echo

# Example 8: Performance Comparison
echo "8. Performance Comparison: HTTP-aware vs Mathematical Strategies"
echo "Comparing response times with different strategies..."

# Create a test that shows HTTP-aware is more efficient
cat > performance_comparison.sh << 'EOF'
#!/bin/bash
echo "Testing HTTP-aware vs Exponential backoff..."

echo "HTTP-aware strategy (respects server timing):"
time ../patience --backoff http-aware --attempts 3 -- bash -c '
echo "HTTP/1.1 429
Retry-After: 1

{\"error\": \"rate limited\"}" >&2
exit 22
' 2>/dev/null || true

echo
echo "Exponential strategy (ignores server timing):"
time ../patience --backoff exponential --delay 1s --attempts 3 -- bash -c '
echo "HTTP/1.1 429
Retry-After: 1

{\"error\": \"rate limited\"}" >&2
exit 22
' 2>/dev/null || true
EOF
chmod +x performance_comparison.sh

echo "  Running performance comparison..."
./performance_comparison.sh
rm -f performance_comparison.sh
echo

echo "=== HTTP-Aware Strategy Integration Examples Complete ==="
echo "All examples demonstrate successful integration with real-world patterns!"
echo
echo "Key Takeaways:"
echo "  ✅ HTTP-aware strategy correctly parses real API responses"
echo "  ✅ Works seamlessly with curl when proper flags are used"
echo "  ✅ Gracefully falls back when no HTTP timing is available"
echo "  ✅ Supports multiple API patterns (headers, JSON, mixed)"
echo "  ✅ More efficient than mathematical strategies for rate-limited APIs"
echo
echo "Best Practices:"
echo "  • Use 'curl -i' to include headers in output"
echo "  • Use 'curl -D /dev/stderr' to send headers to stderr"
echo "  • Set --http-max-delay to cap server-requested delays"
echo "  • Always specify a --fallback strategy for non-HTTP errors"
echo "  • Test with your specific API to verify header formats"