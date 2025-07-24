# retry Examples

This guide shows the most common, practical uses of `retry` that you'll encounter in everyday development and operations work.

## Network & API Calls

The most common use case – dealing with unreliable network connections and flaky APIs:

```bash
# Retry a failing curl request with exponential backoff
retry --attempts 5 --delay 1s --backoff exponential -- curl -f https://api.example.com/status

# Download a file that might fail (fixed delay)
retry --attempts 3 --delay 2s -- wget https://releases.example.com/package.tar.gz

# API call with exponential backoff and timeout protection
retry --attempts 5 --delay 500ms --backoff exponential --max-delay 10s --timeout 30s -- \
  curl -X POST -d '{"key":"value"}' https://api.example.com/data
```

## Development & Testing

Common scenarios during development and CI/CD:

```bash
# Retry flaky tests
retry --attempts 3 -- npm test

# Wait for a local development server to start
retry --attempts 10 --delay 1s -- curl -f http://localhost:3000

# Retry package installation with exponential backoff
retry --attempts 5 --delay 1s --backoff exponential -- npm install
```

## Database Connections

Databases often need a moment to start up or accept connections:

```bash
# Wait for database to be ready
retry --attempts 10 --delay 2s -- pg_isready -h localhost

# Test database connection
retry --attempts 5 --delay 3s -- \
  psql -h localhost -U user -d mydb -c "SELECT 1;"

# Wait for MySQL to accept connections
retry --attempts 8 --delay 2s -- \
  mysql -h localhost -u root -e "SELECT 1;"
```

## Docker & Containers

Container operations that commonly need retries:

```bash
# Wait for a container to be healthy
retry --attempts 15 --delay 3s -- \
  docker exec mycontainer curl -f http://localhost/health

# Pull an image when registry is flaky (exponential backoff)
retry --attempts 3 --delay 2s --backoff exponential -- docker pull nginx:latest

# Wait for container to start accepting connections
retry --attempts 10 --delay 2s -- \
  docker exec mycontainer nc -z localhost 8080
```

## File Operations

Simple file and directory operations that might need patience:

```bash
# Wait for a file to appear (common in CI/CD)
retry --attempts 20 --delay 1s -- test -f /tmp/build-complete.flag

# Wait for a process to release a file lock
retry --attempts 10 --delay 2s -- rm /var/lock/myapp.lock

# Check if a directory is ready
retry --attempts 5 --delay 1s -- test -d /mnt/shared/ready
```

## SSH & Remote Operations

Remote operations that commonly fail due to network issues:

```bash
# SSH connection (useful after server restarts)
retry --attempts 5 --delay 3s -- ssh user@server 'echo "Connected"'

# Copy files over unreliable connection
retry --attempts 3 --delay 5s -- \
  scp myfile.txt user@server:/home/user/

# Remote command execution
retry --attempts 3 --delay 2s -- \
  ssh user@server 'systemctl status myservice'
```

## Build & Deployment

Common build and deployment scenarios:

```bash
# Retry builds when dependencies are flaky
retry --attempts 3 --delay 10s -- make build

# Deploy when target server might be busy (exponential backoff)
retry --attempts 5 --delay 2s --backoff exponential --max-delay 30s -- ./deploy.sh production

# Health check after deployment
retry --attempts 15 --delay 2s --backoff exponential -- \
  curl -f https://myapp.com/health
```

## Shell Scripts & Automation

Using retry in scripts and automation:

```bash
#!/bin/bash
# Wait for service to be ready before proceeding
if retry --attempts 10 --delay 2s -- curl -f http://localhost:8080/health; then
    echo "Service is ready"
    ./run-integration-tests.sh
else
    echo "Service failed to start"
    exit 1
fi
```

```bash
# In a deployment script
retry --attempts 3 --delay 2s --backoff exponential -- kubectl apply -f deployment.yaml
retry --attempts 15 --delay 5s --backoff exponential -- kubectl rollout status deployment/myapp
```

## Backoff Strategies

### Fixed Delay
Use when you want predictable, consistent timing:

```bash
# Always wait exactly 2 seconds between attempts
retry --attempts 5 --delay 2s -- your-command

# Good for: local operations, predictable timing needs
```

### Exponential Backoff
Use for network operations and external services (recommended):

```bash
# Wait 1s, then 2s, then 4s, then 8s...
retry --attempts 5 --delay 1s --backoff exponential -- api-call

# With custom multiplier (1s, 1.5s, 2.25s, 3.375s...)
retry --attempts 5 --delay 1s --backoff exponential --multiplier 1.5 -- api-call

# With maximum delay cap (1s, 2s, 4s, 5s, 5s...)
retry --attempts 6 --delay 1s --backoff exponential --max-delay 5s -- api-call
```

**Why exponential backoff?**
- Reduces load on failing services
- Industry standard for retry logic
- Gives services time to recover
- Prevents "thundering herd" problems

## Quick Reference

### Most Common Patterns

```bash
# Basic retry (3 attempts, no delay)
retry -- your-command

# Network operations (with exponential backoff)
retry --attempts 5 --delay 1s --backoff exponential -- curl -f https://example.com

# Fixed delay for predictable timing
retry --attempts 3 --delay 2s -- your-command

# With timeout protection
retry --attempts 3 --timeout 30s -- long-running-command

# All options together (exponential backoff with limits)
retry --attempts 5 --delay 500ms --backoff exponential --max-delay 10s --timeout 30s -- your-command
```

### Typical Parameters by Use Case

| Use Case | Attempts | Delay | Backoff | Timeout | Example |
|----------|----------|-------|---------|---------|---------|
| API calls | 3-5 | 1s | exponential | 10-30s | `retry -a 5 -d 1s --backoff exponential -t 15s -- curl -f api.com` |
| File downloads | 3 | 2s | exponential | 60s+ | `retry -a 3 -d 2s --backoff exponential -t 120s -- wget file.zip` |
| Service startup | 10-15 | 1s | exponential | 5-10s | `retry -a 15 -d 1s --backoff exponential -t 5s -- curl localhost:8080` |
| Database connections | 5-8 | 1s | exponential | 5-10s | `retry -a 8 -d 1s --backoff exponential -t 5s -- pg_isready` |
| SSH connections | 3-5 | 2s | exponential | 10-30s | `retry -a 5 -d 2s --backoff exponential -t 15s -- ssh user@host` |
| Quick local checks | 5-10 | 500ms | fixed | 5s | `retry -a 10 -d 500ms -- test -f /tmp/ready` |

## Tips

- **Start simple**: Use `retry -- command` first, then add options as needed
- **Network calls**: Use exponential backoff (`--backoff exponential`) to be respectful to servers
- **Local operations**: Fixed delays or no delays work fine, just `--attempts`
- **Long-running commands**: Always use `--timeout` to prevent hanging
- **Production systems**: Exponential backoff with `--max-delay` prevents excessive waits
- **Quick feedback**: Use fixed delays when you want predictable timing

---

*These are the scenarios where retry shines. Keep it simple and focus on the operations that actually fail in practice.*