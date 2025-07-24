# retry Examples

This guide shows the most common, practical uses of `retry` that you'll encounter in everyday development and operations work.

## Network & API Calls

The most common use case – dealing with unreliable network connections and flaky APIs:

```bash
# Retry a failing curl request
retry --attempts 5 --delay 1s -- curl -f https://api.example.com/status

# Download a file that might fail
retry --attempts 3 --delay 2s -- wget https://releases.example.com/package.tar.gz

# API call with timeout protection
retry --attempts 3 --delay 2s --timeout 10s -- \
  curl -X POST -d '{"key":"value"}' https://api.example.com/data
```

## Development & Testing

Common scenarios during development and CI/CD:

```bash
# Retry flaky tests
retry --attempts 3 -- npm test

# Wait for a local development server to start
retry --attempts 10 --delay 1s -- curl -f http://localhost:3000

# Retry package installation when registry is slow
retry --attempts 5 --delay 3s -- npm install
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

# Pull an image when registry is flaky
retry --attempts 3 --delay 5s -- docker pull nginx:latest

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

# Deploy when target server might be busy
retry --attempts 5 --delay 5s -- ./deploy.sh production

# Health check after deployment
retry --attempts 20 --delay 5s -- \
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
retry --attempts 3 --delay 5s -- kubectl apply -f deployment.yaml
retry --attempts 15 --delay 10s -- kubectl rollout status deployment/myapp
```

## Quick Reference

### Most Common Patterns

```bash
# Basic retry (3 attempts, no delay)
retry -- your-command

# Network operations (with delay)
retry --attempts 5 --delay 2s -- curl -f https://example.com

# With timeout protection
retry --attempts 3 --timeout 30s -- long-running-command

# All options together
retry --attempts 5 --delay 2s --timeout 10s -- your-command
```

### Typical Parameters by Use Case

| Use Case | Attempts | Delay | Timeout | Example |
|----------|----------|-------|---------|---------|
| API calls | 3-5 | 1-3s | 10-30s | `retry -a 5 -d 2s -t 15s -- curl -f api.com` |
| File downloads | 3 | 2-5s | 60s+ | `retry -a 3 -d 5s -t 120s -- wget file.zip` |
| Service startup | 10-20 | 1-3s | 5-10s | `retry -a 15 -d 2s -t 5s -- curl localhost:8080` |
| Database connections | 5-10 | 2-3s | 5-10s | `retry -a 8 -d 2s -t 5s -- pg_isready` |
| SSH connections | 3-5 | 3-5s | 10-30s | `retry -a 5 -d 3s -t 15s -- ssh user@host` |

## Tips

- **Start simple**: Use `retry -- command` first, then add options as needed
- **Network calls**: Almost always want `--delay` to avoid overwhelming servers
- **Local operations**: Usually don't need delays, just `--attempts`
- **Long-running commands**: Always use `--timeout` to prevent hanging

---

*These are the scenarios where retry shines. Keep it simple and focus on the operations that actually fail in practice.*