# Patience Daemon (patienced)

The patience daemon (`patienced`) is a background service that collects and aggregates metrics from patience CLI instances, providing a centralized monitoring solution for patience operations across your infrastructure.

**Author:** Shane Isley  
**Repository:** [github.com/shaneisley/patience](https://github.com/shaneisley/patience)

## Features

- **Unix Socket Server**: Production-grade Unix domain socket server with JSON protocol
- **Multi-Instance Coordination**: Enables Diophantine strategy coordination across multiple patience instances
- **Metrics Collection**: Receives metrics from patience CLI instances via Unix domain socket
- **Data Aggregation**: Aggregates metrics with configurable retention policies
- **HTTP API**: RESTful API for accessing metrics and statistics
- **Web Dashboard**: Built-in web interface for monitoring patience operations
- **Performance Monitoring**: Runtime performance metrics and profiling endpoints
- **System Integration**: Service files for systemd and launchd
- **Graceful Shutdown**: Proper cleanup and signal handling
- **Protocol Versioning**: Supports versioned JSON protocol for backward compatibility

## Installation

### Automatic Installation

Use the provided installation script:

```bash
# Build binaries first
go build ./cmd/patience
go build ./cmd/patienced

# Run installation script (requires root)
sudo ./scripts/install.sh
```

### Manual Installation

1. **Build the daemon**:
   ```bash
   go build -o patienced ./cmd/patienced
   ```

2. **Install binary**:
   ```bash
   sudo cp patienced /usr/local/bin/
   sudo chmod 755 /usr/local/bin/patienced
   ```

3. **Create configuration directory**:
   ```bash
   sudo mkdir -p /usr/local/etc/patience
   ```

4. **Create data and log directories**:
   ```bash
   sudo mkdir -p /usr/local/var/lib/patience
   sudo mkdir -p /usr/local/var/log/patience
   ```

5. **Create user and group**:
   ```bash
   # Linux
   sudo useradd --system --home-dir /usr/local/var/lib/patience patience
   
   # macOS
   sudo dscl . -create /Users/_patience
   sudo dscl . -create /Users/_patience UserShell /usr/bin/false
   ```

## Configuration

### Configuration File

Create a configuration file at `/usr/local/etc/patience/daemon.json`:

```json
{
  "socket_path": "/var/run/patience/daemon.sock",
  "http_port": 8080,
  "max_metrics": 10000,
  "metrics_max_age": "24h",
  "log_level": "info",
  "pid_file": "/var/run/patience-daemon.pid",
  "enable_http": true,
  "enable_profiling": false
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `socket_path` | string | `/var/run/patience/daemon.sock` | Unix socket path for metrics collection |
| `max_metrics` | int | `10000` | Maximum number of metrics to store |
| `metrics_max_age` | duration | `24h` | Maximum age of stored metrics |
| `log_level` | string | `info` | Log level (debug, info, warn, error) |
| `pid_file` | string | `/var/run/patience/daemon.pid` | PID file location |
| `enable_http` | bool | `true` | Enable HTTP API server |
| `enable_profiling` | bool | `false` | Enable profiling endpoints |

### Environment Variables

Configuration can also be set via environment variables:

- `PATIENCE_SOCKET_PATH`
- `PATIENCE_HTTP_PORT`
- `PATIENCE_MAX_METRICS`
- `PATIENCE_METRICS_MAX_AGE`
- `PATIENCE_LOG_LEVEL`
- `PATIENCE_PID_FILE`
- `PATIENCE_ENABLE_HTTP`
- `PATIENCE_ENABLE_PROFILING`

## Usage

### Starting the Daemon

#### Manual Start
```bash
# Foreground
patienced

# With custom config
patienced -config /path/to/config.json

# Background (basic daemonization)
patienced -daemon
```

#### Using System Service Manager

**systemd (Linux)**:
```bash
# Enable and start
sudo systemctl enable patience-daemon
sudo systemctl start patience-daemon

# Check status
sudo systemctl status patience-daemon

# View logs
sudo journalctl -u patience-daemon -f
```

**launchd (macOS)**:
```bash
# Load and start
sudo launchctl load /Library/LaunchDaemons/com.patience.daemon.plist
sudo launchctl start com.patience.daemon

# Check status
sudo launchctl list | grep patience

# View logs
tail -f /usr/local/var/log/patience/daemon.log
```

### Managing the Daemon

```bash
# Check daemon status
patienced -status

# Stop daemon
patienced -stop

# Show version
patienced -version
```

### Command Line Options

```
Usage: patienced [options]

Options:
  -config string
        Configuration file path
  -daemon
        Run as daemon (background process)
  -enable-http
        Enable HTTP API server (default true)
  -enable-profiling
        Enable profiling endpoints
  -log-level string
        Log level (debug, info, warn, error) (default "info")
  -max-age duration
        Maximum age of metrics (default 24h0m0s)
  -max-metrics int
        Maximum number of metrics to store (default 10000)
  -pid-file string
        PID file path (default "/var/run/patience/daemon.pid")
  -port int
        HTTP server port (default 8080)
  -socket string
        Unix socket path (default "/var/run/patience/daemon.sock")
  -status
        Show daemon status
  -stop
        Stop running daemon
  -version
        Show version information
```

## HTTP API

The daemon provides a RESTful HTTP API for accessing metrics and statistics.

### Endpoints

#### Metrics

- `GET /api/metrics/recent?limit=N` - Get recent metrics
- `GET /api/metrics/stats?start=TIME&end=TIME` - Get aggregated statistics
- `GET /api/metrics/export` - Export all metrics as JSON

#### Daemon

- `GET /api/daemon/stats` - Get daemon statistics
- `GET /api/daemon/performance` - Get performance metrics
- `GET /api/health` - Health check

#### Dashboard

- `GET /` - Web dashboard

#### Profiling (if enabled)

- `GET /debug/pprof/` - Profiling index
- `GET /debug/pprof/goroutine` - Goroutine profiles
- `GET /debug/pprof/heap` - Heap profiles
- `GET /debug/pprof/profile` - CPU profiles

### API Examples

```bash
# Get recent metrics
curl http://localhost:8080/api/metrics/recent?limit=10

# Get statistics for last hour
START=$(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%SZ)
END=$(date -u +%Y-%m-%dT%H:%M:%SZ)
curl "http://localhost:8080/api/metrics/stats?start=$START&end=$END"

# Export all metrics
curl http://localhost:8080/api/metrics/export -o metrics.json

# Check daemon health
curl http://localhost:8080/api/health

# Get performance stats
curl http://localhost:8080/api/daemon/performance
```

## Web Dashboard

Access the web dashboard at `http://localhost:8080` (or your configured port).

The dashboard provides:

- **System Statistics**: Daemon configuration and storage info
- **Metrics Overview**: Success rates, attempt counts, duration statistics
- **Recent Operations**: Latest patience operations with status
- **Real-time Updates**: Auto-refresh every 30 seconds

## Integration with patience CLI

The patience CLI automatically sends metrics to the daemon when available. The daemon also provides coordination services for the Diophantine strategy, enabling multi-instance rate limiting.

### Diophantine Strategy Coordination

When using the `--daemon` flag with the Diophantine strategy, patience instances coordinate through the daemon to prevent rate limit violations:

```bash
# Enable daemon coordination for shared rate limiting
patience diophantine --daemon --resource-id "shared-api" --rate-limit 100 --window 1h -- curl https://httpbin.org/status/429
```

The daemon handles:
- **Schedule Requests**: Determines if a request can be scheduled based on current rate limit usage
- **Request Registration**: Tracks planned requests to prevent over-scheduling
- **Resource Coordination**: Manages rate limits across different resource IDs
- **Graceful Fallback**: Continues operation if daemon is unavailable

### Protocol Communication

The daemon uses a JSON-based line protocol over Unix sockets:

#### Handshake
```json
{"type": "handshake", "version": "1.0", "client": "patience-cli"}
```

#### Schedule Request
```json
{
  "type": "schedule_request",
  "resource_id": "api-endpoint",
  "rate_limit": 100,
  "window": "1h",
  "request_time": "2025-01-01T12:00:00Z"
}
```

#### Register Request
```json
{
  "type": "register_request",
  "requests": [
    {
      "id": "req-123",
      "resource_id": "api-endpoint", 
      "scheduled_at": "2025-01-01T12:01:00Z",
      "expires_at": "2025-01-01T13:00:00Z"
    }
  ]
}
```

### Metrics Sent

For each patience operation, the following metrics are sent:

- Command executed
- Final status (succeeded/failed)
- Total duration
- Number of attempts
- Individual attempt details (duration, exit code, success)
- Timestamp
- Rate limiting information (for Diophantine strategy)

## Performance and Scaling

### Resource Usage

- **Memory**: Configurable with `max_metrics` (approximately 1KB per metric)
- **CPU**: Minimal overhead, async processing
- **Disk**: No persistent storage (in-memory only)
- **Network**: Unix socket communication (very low overhead)

### Scaling Considerations

- **Single daemon per host**: Designed for per-host metrics collection
- **Memory limits**: Configure `max_metrics` based on available memory
- **Age-based cleanup**: Old metrics are automatically removed
- **Concurrent connections**: Handles multiple CLI instances simultaneously

### Performance Monitoring

Monitor daemon performance via:

- `/api/daemon/performance` endpoint
- `/debug/pprof/` endpoints (if profiling enabled)
- System monitoring tools (htop, ps, etc.)

## Troubleshooting

### Common Issues

1. **Permission denied on socket**:
   ```bash
    # Check socket permissions
    ls -la /var/run/patience/daemon.sock
    
    # Fix permissions if needed
    sudo chmod 666 /var/run/patience/daemon.sock
   ```

2. **Port already in use**:
   ```bash
   # Check what's using the port
   sudo lsof -i :8080
   
   # Use different port
   patienced -port 8081
   ```

3. **Daemon won't start**:
   ```bash
   # Check if already running
   patienced -status
   
   # Check logs
   journalctl -u patience-daemon -n 50
   ```

4. **High memory usage**:
   ```bash
   # Reduce max metrics
   patienced -max-metrics 5000
   
   # Reduce max age
   patienced -max-age 12h
   ```

### Logging

Daemon logs include:

- Startup and shutdown events
- Metrics received and stored
- HTTP API requests
- Error conditions
- Performance warnings

Log levels: `debug`, `info`, `warn`, `error`

### Debugging

Enable debug logging:
```bash
patienced -log-level debug
```

Enable profiling:
```bash
patienced -enable-profiling
# Then access http://localhost:8080/debug/pprof/
```

## Security Considerations

- **Unix socket permissions**: Socket created with 0600 permissions for security
- **Protocol validation**: All JSON messages are validated before processing
- **Connection limits**: Configurable maximum concurrent connections
- **HTTP API**: Consider firewall rules for HTTP port
- **User privileges**: Run daemon as dedicated user (not root)
- **Profiling endpoints**: Only enable in development/debugging
- **Log sensitivity**: Logs may contain command arguments
- **Resource isolation**: Rate limiting resources are isolated by resource ID

## Maintenance

### Regular Tasks

1. **Monitor resource usage**: Check memory and CPU usage
2. **Review logs**: Look for errors or warnings
3. **Update configuration**: Adjust limits based on usage patterns
4. **Restart periodically**: Consider periodic restarts for long-running instances

### Backup and Recovery

- **No persistent data**: Daemon stores metrics in memory only
- **Configuration backup**: Backup configuration files
- **Service configuration**: Backup systemd/launchd service files

## Development

### Building from Source

```bash
# Clone repository
git clone <repository-url>
cd patience

# Build daemon
go build -o patienced ./cmd/patienced

# Run tests
go test ./pkg/daemon/...
go test ./pkg/storage/...

# Run with race detection
go run -race ./cmd/patienced
```

### Contributing

See the main project README for contribution guidelines.

## License

See the main project LICENSE file.
