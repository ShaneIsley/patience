# patience Examples

This guide shows the most common, practical uses of `patience` with the new strategy-based interface. Each strategy is designed for specific use cases and scenarios.

## Diophantine Strategy Examples

The Diophantine strategy provides mathematical proactive rate limiting that prevents rate limit violations before they occur, with optional multi-instance coordination.

### Basic Proactive Rate Limiting

```bash
# Prevent rate limit violations for GitHub API (5000 requests/hour)
patience diophantine --rate-limit 5000 --window 1h -- curl -H "Authorization: token $GITHUB_TOKEN" https://api.github.com/user

# Twitter API v2 rate limiting (300 requests/15min)
patience diophantine --rate-limit 300 --window 15m -- curl -H "Authorization: Bearer $TWITTER_TOKEN" https://api.twitter.com/2/tweets

# AWS API with conservative rate limiting
patience diophantine --rate-limit 100 --window 1h --retry-offsets "2s,10s,30s" -- aws s3 ls s3://my-bucket

# Using abbreviation for brevity
patience dio -l 1000 -w 1h -- api-call.sh
```

### Multi-Instance Coordination

```bash
# Coordinate across multiple CI/CD runners
patience diophantine --daemon --resource-id "github-api" --rate-limit 5000 --window 1h -- \
  curl -H "Authorization: token $GITHUB_TOKEN" https://api.github.com/repos/owner/repo

# Shared database API across microservices
patience diophantine --daemon --resource-id "db-api" --rate-limit 500 --window 1h -- \
  curl -X POST -d '{"query":"SELECT * FROM users"}' https://db-api.internal.com/query

# Production deployment with custom daemon socket
patience diophantine --daemon --daemon-address "/var/run/patience/daemon.sock" \
  --resource-id "production-api" --rate-limit 1000 --window 1h -- production-deploy.sh

# Kubernetes pod coordination
patience diophantine --daemon --resource-id "k8s-api-$(kubectl config current-context)" \
  --rate-limit 200 --window 1h -- kubectl apply -f deployment.yaml
```

### Enterprise Use Cases

```bash
# Salesforce API with strict rate limits (15,000 requests/24h)
patience diophantine --daemon --resource-id "salesforce-api" \
  --rate-limit 15000 --window 24h --retry-offsets "5s,30s,2m" -- \
  curl -H "Authorization: Bearer $SF_TOKEN" https://your-instance.salesforce.com/services/data/v52.0/sobjects

# Stripe API coordination across payment services
patience diophantine --daemon --resource-id "stripe-api" \
  --rate-limit 100 --window 1s --retry-offsets "1s,3s,10s" -- \
  curl -H "Authorization: Bearer $API_TOKEN" https://httpbin.org/bearer

# Docker Hub API for CI/CD systems
patience diophantine --daemon --resource-id "dockerhub-api" \
  --rate-limit 200 --window 6h -- \
  curl -H "Authorization: JWT $DOCKERHUB_TOKEN" https://hub.docker.com/v2/repositories/
```

## HTTP-Aware Strategy Examples

The HTTP-aware strategy is patience's flagship feature - it intelligently parses server responses for optimal retry timing.

### Basic HTTP-Aware Usage

```bash
# GitHub API with rate limiting (respects Retry-After headers)
patience http-aware -- curl -i https://api.github.com/user

# Twitter API with fallback strategy
patience http-aware --fallback exponential -- curl -i https://api.twitter.com/2/tweets

# AWS API with decorrelated jitter fallback
patience http-aware --fallback decorrelated-jitter -- aws s3 ls

# Using abbreviation for brevity
patience ha -f exp -- curl -i https://httpbin.org/bearer
```

### Real-World HTTP Examples

```bash
# Slack webhook with HTTP intelligence
patience http-aware --attempts 5 --timeout 30s -- \
  curl -X POST -H "Content-Type: application/json" \
  -d '{"text":"Deployment complete"}' \
  https://hooks.slack.com/services/YOUR/WEBHOOK/URL

# Discord webhook with retry logic
patience http-aware --max-delay 5m -- \
  curl -X POST -H "Content-Type: application/json" \
  -d '{"content":"Build failed"}' \
  https://discord.com/api/webhooks/YOUR/WEBHOOK

# Reddit API with intelligent retries
patience http-aware --fallback fibonacci -- \
  curl -H "User-Agent: MyBot/1.0" https://www.reddit.com/r/programming.json
```

## Network & API Calls

### Exponential Backoff (Recommended for APIs)

```bash
# Retry a failing curl request with exponential backoff
patience exponential --attempts 5 --base-delay 1s -- curl -f https://httpbin.org/status/503

# API call with custom multiplier and timeout protection
patience exponential --base-delay 500ms --multiplier 1.5 --max-delay 10s --timeout 30s -- \
  curl -X POST -d '{"key":"value"}' https://httpbin.org/post

# Using abbreviation
patience exp -b 1s -x 2.0 -m 30s -- curl https://httpbin.org/delay/1
```

### Linear Backoff (Good for Rate-Limited APIs)

```bash
# Rate-limited API with predictable timing
patience linear --increment 2s --max-delay 30s -- curl https://api.rate-limited.com

# Database API with gradual backoff
patience linear --increment 5s --attempts 6 -- \
  curl -H "Authorization: Bearer $TOKEN" https://httpbin.org/bearer
```

### Fixed Delay (Simple and Predictable)

```bash
# Download a file that might fail (fixed delay)
patience fixed --delay 3s --attempts 3 -- wget https://releases.example.com/package.tar.gz

# Simple retry with consistent timing
patience fixed --delay 5s -- curl https://httpbin.org/delay/1
```

## Development & Testing

### Exponential Backoff for Development

```bash
# Retry flaky tests with exponential backoff
patience exponential --attempts 3 --base-delay 2s -- npm test

# Wait for a local development server to start
patience exponential --attempts 10 --base-delay 1s --max-delay 10s -- curl -f http://localhost:3000

# Retry package installation with exponential backoff
patience exponential --attempts 5 --base-delay 1s -- npm install
```

### Linear Backoff for Predictable Development Tasks

```bash
# Wait for Docker container to be ready (predictable timing)
patience linear --increment 3s --attempts 10 -- \
  docker exec mycontainer curl -f http://localhost/health

# Gradle build with predictable retry timing
patience linear --increment 10s --max-delay 60s -- ./gradlew build

# Maven build with linear backoff
patience linear --increment 5s --attempts 5 -- mvn clean install
```

### Fixed Delay for Simple Development Tasks

```bash
# Simple test retry with fixed timing
patience fixed --delay 5s --attempts 3 -- pytest tests/integration/

# Wait for file to appear in CI/CD
patience fixed --delay 2s --attempts 15 -- test -f /tmp/build-complete.flag

# Simple service health check
patience fixed --delay 3s --attempts 10 -- curl -f http://localhost:8080/health
```

## Database Connections

### Exponential Backoff for Database Connections

```bash
# Wait for PostgreSQL to be ready with exponential backoff
patience exponential --attempts 10 --base-delay 1s --max-delay 30s -- pg_isready -h localhost

# Test database connection with exponential growth
patience exponential --attempts 5 --base-delay 2s -- \
  psql -h localhost -U user -d mydb -c "SELECT 1;"

# Wait for MySQL to accept connections
patience exponential --attempts 8 --base-delay 1s --max-delay 20s -- \
  mysql -h localhost -u root -e "SELECT 1;"
```

### Linear Backoff for Database Startup

```bash
# Wait for database startup with predictable timing
patience linear --increment 3s --max-delay 45s --attempts 15 -- \
  pg_isready -h db.example.com

# MongoDB connection with linear backoff
patience linear --increment 2s --attempts 10 -- \
  mongo --eval "db.adminCommand('ismaster')"

# Redis connection check
patience linear --increment 1s --max-delay 30s -- \
  redis-cli -h localhost ping
```

### Fibonacci Backoff for Database Recovery

```bash
# Database recovery with moderate growth
patience fibonacci --base-delay 1s --max-delay 60s -- \
  psql -h recovering-db.example.com -c "SELECT 1;"

# Elasticsearch cluster health check
patience fibonacci --base-delay 2s --attempts 8 -- \
  curl -f http://localhost:9200/_cluster/health

# Cassandra connection with Fibonacci timing
patience fibonacci --base-delay 3s --max-delay 120s -- \
  cqlsh -e "DESCRIBE KEYSPACES;"
```

## Docker & Containers

### Exponential Backoff for Container Operations

```bash
# Wait for a container to be healthy with exponential backoff
patience exponential --attempts 15 --base-delay 2s --max-delay 30s -- \
  docker exec mycontainer curl -f http://localhost/health

# Pull an image when registry is flaky
patience exponential --attempts 3 --base-delay 2s -- docker pull nginx:latest

# Wait for Kubernetes pod to be ready
patience exponential --attempts 20 --base-delay 1s --max-delay 60s -- \
  kubectl get pod mypod -o jsonpath='{.status.phase}' | grep Running
```

### Linear Backoff for Container Startup

```bash
# Wait for container to start accepting connections (predictable timing)
patience linear --increment 3s --attempts 10 -- \
  docker exec mycontainer nc -z localhost 8080

# Docker Compose service startup
patience linear --increment 5s --max-delay 60s -- \
  docker-compose exec web curl -f http://localhost/health

# Container log monitoring
patience linear --increment 2s --attempts 15 -- \
  docker logs mycontainer 2>&1 | grep "Server started"
```

### Jitter for Distributed Container Systems

```bash
# Multiple containers starting simultaneously (prevents thundering herd)
patience jitter --base-delay 2s --multiplier 2.0 --max-delay 30s -- \
  docker exec web-1 curl -f http://localhost/health

# Kubernetes deployment rollout with jitter
patience jitter --base-delay 3s --attempts 20 -- \
  kubectl rollout status deployment/myapp

# Docker Swarm service convergence
patience jitter --base-delay 5s --max-delay 120s -- \
  docker service ps myservice | grep Running
```

## File Operations

Simple file and directory operations that might need patience:

```bash
# Wait for a file to appear (common in CI/CD)
patience fixed --attempts 20 --delay 1s -- test -f /tmp/build-complete.flag

# Wait for a process to release a file lock
patience fixed --attempts 10 --delay 2s -- rm /var/lock/myapp.lock

# Check if a directory is ready
patience fixed --attempts 5 --delay 1s -- test -d /mnt/shared/ready
```

## SSH & Remote Operations

### Exponential Backoff for SSH Operations

```bash
# SSH connection after server restart with exponential backoff
patience exponential --attempts 5 --base-delay 3s --max-delay 60s -- \
  ssh user@server 'echo "Connected"'

# Remote command execution with exponential growth
patience exponential --attempts 3 --base-delay 2s -- \
  ssh user@server 'systemctl status myservice'

# Remote file operations
patience exponential --attempts 4 --base-delay 5s -- \
  ssh user@server 'test -f /var/log/myapp.log'
```

### Linear Backoff for File Transfers

```bash
# Copy files over unreliable connection with predictable timing
patience linear --increment 5s --attempts 3 -- \
  scp myfile.txt user@server:/home/user/

# Rsync with linear backoff
patience linear --increment 10s --max-delay 60s -- \
  rsync -av /local/path/ user@server:/remote/path/

# SFTP file transfer
patience linear --increment 3s --attempts 5 -- \
  sftp user@server <<< "put localfile.txt remotefile.txt"
```

### Decorrelated Jitter for AWS/Cloud Operations

```bash
# AWS EC2 instance operations (AWS-recommended strategy)
patience decorrelated-jitter --base-delay 1s --multiplier 3.0 --max-delay 30s -- \
  ssh -i mykey.pem ec2-user@instance.amazonaws.com 'uptime'

# Google Cloud operations
patience decorrelated-jitter --base-delay 2s --attempts 8 -- \
  gcloud compute ssh myinstance --command="systemctl status myapp"

# Azure operations
patience decorrelated-jitter --base-delay 1s --max-delay 45s -- \
  az vm run-command invoke --resource-group mygroup --name myvm --command-id RunShellScript --scripts "uptime"
```

## Build & Deployment

### Exponential Backoff for Build Operations

```bash
# Retry builds when dependencies are flaky
patience exponential --attempts 3 --base-delay 10s --max-delay 120s -- make build

# Docker build with exponential backoff
patience exponential --attempts 4 --base-delay 5s -- docker build -t myapp .

# Webpack build with exponential growth
patience exponential --attempts 3 --base-delay 15s -- npm run build
```

### Linear Backoff for Deployment Operations

```bash
# Deploy when target server might be busy (predictable timing)
patience linear --increment 10s --max-delay 60s --attempts 5 -- ./deploy.sh production

# Kubernetes deployment with linear backoff
patience linear --increment 15s --attempts 8 -- kubectl apply -f deployment.yaml

# Terraform apply with linear growth
patience linear --increment 20s --max-delay 180s -- terraform apply -auto-approve
```

### HTTP-Aware for Health Checks

```bash
# Health check after deployment (respects server timing)
patience http-aware --attempts 15 --fallback exponential -- \
  curl -f https://myapp.com/health

# API readiness check with HTTP intelligence
patience http-aware --max-delay 5m -- \
  curl -f https://api.myapp.com/ready

# Load balancer health check
patience http-aware --attempts 20 --timeout 10s -- \
  curl -f https://lb.myapp.com/health
```

### Fibonacci for Gradual Deployment Recovery

```bash
# Gradual deployment recovery with Fibonacci timing
patience fibonacci --base-delay 5s --max-delay 300s -- \
  kubectl rollout status deployment/myapp

# Service mesh readiness
patience fibonacci --base-delay 3s --attempts 10 -- \
  istioctl proxy-status | grep SYNCED

# Database migration with moderate growth
patience fibonacci --base-delay 10s --max-delay 600s -- \
  ./migrate.sh production
```

## Shell Scripts & Automation

### Script Integration Examples

```bash
#!/bin/bash
# Wait for service to be ready before proceeding
if patience exponential --attempts 10 --base-delay 2s --max-delay 30s -- curl -f http://localhost:8080/health; then
    echo "Service is ready"
    ./run-integration-tests.sh
else
    echo "Service failed to start"
    exit 1
fi
```

```bash
#!/bin/bash
# Deployment script with multiple strategies
echo "Deploying application..."

# Apply deployment with linear backoff
patience linear --attempts 3 --increment 5s -- kubectl apply -f deployment.yaml

# Wait for rollout with exponential backoff
patience exponential --attempts 15 --base-delay 3s --max-delay 60s -- \
  kubectl rollout status deployment/myapp

# Health check with HTTP-aware strategy
patience http-aware --attempts 20 --timeout 10s -- \
  curl -f https://myapp.com/health

echo "Deployment complete!"
```

### CI/CD Pipeline Examples

```bash
# Jenkins/GitHub Actions pipeline step
- name: Wait for database
  run: patience exponential --attempts 8 --base-delay 2s -- pg_isready -h postgres

- name: Run integration tests
  run: patience linear --increment 10s --attempts 5 -- npm run test:integration

- name: Deploy to staging
  run: patience http-aware --fallback exponential -- ./deploy.sh staging

- name: Smoke test
  run: patience fixed --delay 5s --attempts 3 -- curl -f https://staging.myapp.com/health
```

### Pattern Matching in Scripts

```bash
#!/bin/bash
# Deployment with success pattern matching
patience exponential --attempts 5 --success-pattern "deployment successful" -- ./deploy.sh

# Log monitoring with failure pattern
patience linear --increment 5s --failure-pattern "(?i)error|failed" -- \
  tail -f /var/log/myapp.log | head -20

# Case-insensitive pattern matching
patience fixed --delay 3s --success-pattern "ready" --case-insensitive -- \
  ./check-service-status.sh
```

## Backoff Strategies

### Fixed Delay
Use when you want predictable, consistent timing:

```bash
# Always wait exactly 2 seconds between attempts
patience fixed --attempts 5 --delay 2s -- your-command

# Good for: local operations, predictable timing needs
```

### Exponential Backoff
Use for network operations and external services (recommended):

```bash
# Wait 1s, then 2s, then 4s, then 8s...
patience exponential --attempts 5 --base-delay 1s -- api-call

# With custom multiplier (1s, 1.5s, 2.25s, 3.375s...)
patience exponential --attempts 5 --base-delay 1s --multiplier 1.5 -- api-call

# With maximum delay cap (1s, 2s, 4s, 5s, 5s...)
patience exponential --attempts 6 --base-delay 1s --max-delay 5s -- api-call
```

**Why exponential backoff?**
- Reduces load on failing services
- Industry standard for retry logic
- Gives services time to recover
- Prevents "thundering herd" problems

### Jitter Backoff
Use to prevent thundering herd problems when multiple instances retry simultaneously:

```bash
# Random delays between 0 and exponential backoff time
patience jitter --attempts 5 --base-delay 1s -- distributed-api-call

# With max delay cap to prevent excessive waits
patience jitter --attempts 5 --base-delay 1s --max-delay 10s -- high-scale-service
```

**Why jitter?**
- Prevents multiple clients from retrying at the same time
- Essential for distributed systems and microservices
- Reduces server load spikes during outages
- AWS and Google Cloud recommend this approach

### Linear Backoff
Use when you want predictable, incremental delays:

```bash
# Wait 1s, then 2s, then 3s, then 4s...
patience linear --attempts 5 --increment 1s -- gradual-retry

# With max delay cap (1s, 2s, 3s, 5s, 5s...)
patience linear --attempts 6 --increment 1s --max-delay 5s -- capped-linear
```

**Why linear backoff?**
- Predictable timing for debugging
- Good for operations that need steady progression
- Less aggressive than exponential growth
- Useful for rate-limited APIs

### Decorrelated Jitter
Use for AWS services and high-scale distributed systems (AWS recommended):

```bash
# Smart jitter based on previous delay
patience decorrelated-jitter --attempts 5 --base-delay 1s --multiplier 3.0 -- aws-api-call

# With max delay for production systems
patience decorrelated-jitter --attempts 8 --base-delay 500ms --multiplier 3.0 --max-delay 30s -- production-service
```

**Why decorrelated jitter?**
- AWS-recommended strategy for their services
- Better distribution than simple jitter
- Uses previous delay to calculate next delay
- Optimal for high-scale distributed systems

### Fibonacci Backoff
Use when you want moderate growth between linear and exponential:

```bash
# Wait 1s, 1s, 2s, 3s, 5s, 8s...
patience fibonacci --attempts 6 --base-delay 1s -- moderate-growth

# Good for services that need time to recover
patience fibonacci --attempts 8 --base-delay 500ms --max-delay 15s -- recovery-service
```

**Why fibonacci backoff?**
- Moderate growth rate (between linear and exponential)
- Natural progression that's not too aggressive
- Good for services that need gradual recovery time
- Mathematical elegance with practical benefits

### Polynomial Backoff
Use when you want customizable growth patterns:

```bash
# Quadratic growth (1s, 4s, 9s, 16s...)
patience polynomial --attempts 5 --base-delay 1s --exponent 2.0 -- database-connection

# Moderate growth (1s, 2.8s, 5.2s, 8s...)
patience polynomial --attempts 5 --base-delay 1s --exponent 1.5 -- api-call

# Gentle sublinear growth
patience polynomial --attempts 5 --base-delay 1s --exponent 0.8 -- frequent-operation
```

**Why polynomial backoff?**
- Highly customizable growth patterns
- Fine-tuned control over delay progression
- Can be sublinear, linear, or superlinear
- Mathematical precision for specific use cases

### Adaptive Strategy
Use when you want machine learning-inspired optimization:

```bash
# Basic adaptive with exponential fallback
patience adaptive --attempts 10 --learning-rate 0.1 --memory-window 50 -- flaky-service

# Fast learning for rapidly changing conditions
patience adaptive --attempts 8 --learning-rate 0.5 --fallback fixed -- dynamic-api

# Conservative learning with large memory
patience adaptive --attempts 15 --learning-rate 0.05 --memory-window 200 -- database-operation
```

**Why adaptive strategy?**
- Learns from success/failure patterns
- Optimizes timing based on actual performance
- Adapts to changing service conditions
- Machine learning-inspired approach

### Diophantine Strategy
Use when you want proactive rate limit compliance and optimal throughput:

```bash
# Proactive scheduling for rate-limited APIs (10 requests per hour)
patience diophantine --rate-limit 10 --window 1h --retry-offsets 0,10m,30m -- api-task

# High-frequency operations with tight rate limits (5 requests per minute)
patience diophantine --rate-limit 5 --window 1m --retry-offsets 0,10s,30s -- frequent-api-call

# Batch processing with predictable retry patterns
patience diophantine --rate-limit 100 --window 15m --retry-offsets 0,2m,5m,10m -- batch-operation
```

**Why diophantine strategy?**
- Prevents rate limit violations before they occur
- Maximizes throughput within rate limit constraints
- Uses mathematical modeling (Diophantine inequalities) for precision
- Ideal for controlled environments where you schedule tasks
- Proactive rather than reactive approach to rate limiting

## Quick Reference

### Most Common Patterns

```bash
# HTTP-aware retry (recommended for APIs)
patience http-aware -- curl -f https://httpbin.org/status/503

# Exponential backoff for network operations
patience exponential --base-delay 1s --attempts 5 -- curl -f https://example.com

# Linear backoff for predictable timing
patience linear --increment 2s --attempts 3 -- your-command

# Fixed delay for simple retries
patience fixed --delay 3s --attempts 3 -- your-command

# With timeout protection
patience exponential --timeout 30s --attempts 3 -- long-running-command

# All options together (exponential backoff with limits)
patience exponential --attempts 5 --base-delay 500ms --max-delay 10s --timeout 30s -- your-command
```

### Strategy Selection Guide

| Use Case | Recommended Strategy | Why | Example |
|----------|---------------------|-----|---------|
| HTTP APIs | `http-aware` | Respects server timing | `patience ha -- curl -f api.com` |
| Network calls | `exponential` | Industry standard | `patience exp -b 1s -- curl api.com` |
| Distributed systems | `jitter` | Prevents thundering herd | `patience jit -b 1s -- distributed-call` |
| AWS/Cloud services | `decorrelated-jitter` | AWS recommended | `patience dj -b 1s -x 3.0 -- aws s3 ls` |
| Rate-limited APIs | `linear` | Predictable timing | `patience lin -i 2s -- rate-limited-api` |
| Database connections | `exponential` | Good for startup delays | `patience exp -b 1s -- pg_isready` |
| Simple retries | `fixed` | Consistent timing | `patience fix -d 3s -- simple-command` |
| Gradual recovery | `fibonacci` | Moderate growth | `patience fib -b 2s -- recovering-service` |
| Custom growth patterns | `polynomial` | Fine-tuned control | `patience poly -e 1.5 -- custom-service` |
| Learning systems | `adaptive` | Optimizes over time | `patience adapt -r 0.1 -- changing-service` |

### Typical Parameters by Use Case

| Use Case | Strategy | Attempts | Base Delay | Max Delay | Timeout | Example |
|----------|----------|----------|------------|-----------|---------|---------|
| HTTP APIs | `http-aware` | 3-5 | - | 5-30m | 10-30s | `patience ha -a 5 -m 5m -t 15s -- curl -f api.com` |
| Network calls | `exponential` | 3-5 | 1s | 30-60s | 10-30s | `patience exp -a 5 -b 1s -m 30s -t 15s -- curl api.com` |
| Distributed APIs | `jitter` | 3-5 | 1s | 30-60s | 10-30s | `patience jit -a 5 -b 1s -m 30s -t 15s -- api-call` |
| AWS services | `decorrelated-jitter` | 5-8 | 500ms | 30-60s | 15-30s | `patience dj -a 8 -b 500ms -x 3.0 -m 30s -- aws s3 ls` |
| File downloads | `exponential` | 3 | 2s | 120s | 60s+ | `patience exp -a 3 -b 2s -m 120s -t 300s -- wget file.zip` |
| Service startup | `fibonacci` | 10-15 | 1s | 60s | 5-10s | `patience fib -a 15 -b 1s -m 60s -t 5s -- curl localhost:8080` |
| Database connections | `exponential` | 5-8 | 1s | 30s | 5-10s | `patience exp -a 8 -b 1s -m 30s -t 5s -- pg_isready` |
| SSH connections | `exponential` | 3-5 | 2s | 60s | 10-30s | `patience exp -a 5 -b 2s -m 60s -t 15s -- ssh user@host` |
| Rate-limited APIs | `linear` | 5-8 | - | 60s | 10-20s | `patience lin -a 8 -i 5s -m 60s -t 15s -- api-call` |
| Quick local checks | `fixed` | 5-10 | 500ms | - | 5s | `patience fix -a 10 -d 500ms -t 5s -- test -f /tmp/ready` |

## Advanced Examples

### Combining Strategies with Pattern Matching

```bash
# HTTP-aware with success pattern
patience http-aware --success-pattern "\"status\":\"ok\"" -- \
  curl -s https://httpbin.org/json

# Exponential backoff with failure pattern
patience exponential --failure-pattern "(?i)error|timeout" --base-delay 2s -- \
  ./flaky-script.sh

# Linear backoff with case-insensitive pattern
patience linear --increment 3s --success-pattern "ready" --case-insensitive -- \
  ./check-service.sh
```

### Multi-Service Orchestration

```bash
#!/bin/bash
# Complex deployment orchestration

echo "Starting database..."
patience exponential --attempts 10 --base-delay 2s -- pg_isready -h db

echo "Starting cache..."
patience linear --increment 1s --attempts 8 -- redis-cli ping

echo "Starting application..."
patience fibonacci --base-delay 3s --attempts 12 -- \
  curl -f http://localhost:8080/health

echo "Running smoke tests..."
patience http-aware --attempts 5 --timeout 30s -- \
  ./smoke-tests.sh

echo "Deployment complete!"
```

### Cloud-Native Examples

```bash
# Kubernetes with different strategies
patience exponential --attempts 15 -- kubectl apply -f deployment.yaml
patience linear --increment 10s -- kubectl rollout status deployment/myapp
patience http-aware --attempts 20 -- curl -f https://myapp.k8s.local/health

# AWS with decorrelated jitter
patience decorrelated-jitter --base-delay 1s --multiplier 3.0 -- aws s3 sync ./dist s3://mybucket
patience decorrelated-jitter --attempts 8 -- aws ecs wait services-stable --cluster mycluster

# Docker Compose orchestration
patience fixed --delay 5s -- docker-compose up -d
patience exponential --base-delay 3s -- docker-compose exec web curl -f http://localhost/health
```

## Tips

- **Start with HTTP-aware**: For any HTTP/API calls, try `patience http-aware` first
- **Use exponential for network**: Standard choice for network operations and external services
- **Linear for rate limits**: When you know the service has predictable rate limiting
- **Jitter for distributed**: Prevents thundering herd when multiple instances retry
- **Decorrelated jitter for AWS**: AWS-recommended strategy for their services
- **Fixed for simplicity**: When you want predictable, consistent timing
- **Fibonacci for recovery**: Good middle ground between linear and exponential growth
- **Polynomial for precision**: When you need exact control over growth patterns
- **Adaptive for learning**: When services have changing patterns or you want optimization
- **Always use timeouts**: Prevent commands from hanging with `--timeout`
- **Pattern matching**: Use `--success-pattern` and `--failure-pattern` for smart detection
- **Configuration files**: Use `.patience.toml` for project defaults
- **Environment variables**: Use `PATIENCE_*` for CI/CD environments
- **Abbreviations save time**: `ha`, `exp`, `lin`, `fix`, `jit`, `dj`, `fib`, `poly`, `adapt`

---

*Choose the right strategy for your use case. When in doubt, start with `http-aware` for APIs or `exponential` for everything else.*