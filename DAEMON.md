# Patience Daemon (patienced)

The patience daemon (`patienced`) is a background service that collects and aggregates metrics from patience CLI instances, providing a centralized monitoring solution for retry operations across your infrastructure.

**Author:** Shane Isley  
**Repository:** [github.com/shaneisley/patience](https://github.com/shaneisley/patience)

## Features

- **Metrics Collection**: Receives metrics from patience CLI instances via Unix domain socket
- **Data Aggregation**: Aggregates metrics with configurable retention policies
- **HTTP API**: RESTful API for accessing metrics and statistics
- **Web Dashboard**: Built-in web interface for monitoring retry operations
- **Performance Monitoring**: Runtime performance metrics and profiling endpoints
- **System Integration**: Service files for systemd and launchd
- **Graceful Shutdown**: Proper cleanup and signal handling

## Installation

### Automatic Installation

Use the provided installation script:

```bash
# Build binaries first
go build ./cmd/retry
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
   sudo mkdir -p /usr/local/etc/retry
   ```

4. **Create data and log directories**:
   ```bash
   sudo mkdir -p /usr/local/var/lib/retry
   sudo mkdir -p /usr/local/var/log/retry
   ```

5. **Create user and group**:
   ```bash
   # Linux
   sudo useradd --system --home-dir /usr/local/var/lib/retry retry
   
   # macOS
   sudo dscl . -create /Users/_retry
   sudo dscl . -create /Users/_retry UserShell /usr/bin/false
   ```

## Configuration

### Configuration File

Create a configuration file at `/usr/local/etc/retry/daemon.json`:

```json
{
  "socket_path": "/tmp/retry-daemon.sock",
  "http_port": 8080,
  "max_metrics": 10000,
  "metrics_max_age": "24h",
  "log_level": "info",
  "pid_file": "/var/run/retry-daemon.pid",
  "enable_http": true,
  "enable_profiling": false
}
```

### Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `socket_path` | string | `/tmp/retry-daemon.sock` | Unix socket path for metrics collection |
| `http_port` | int | `8080` | HTTP server port |
| `max_metrics` | int | `10000` | Maximum number of metrics to store |
| `metrics_max_age` | duration | `24h` | Maximum age of stored metrics |
| `log_level` | string | `info` | Log level (debug, info, warn, error) |
| `pid_file` | string | `/tmp/retry-daemon.pid` | PID file location |
| `enable_http` | bool | `true` | Enable HTTP API server |
| `enable_profiling` | bool | `false` | Enable profiling endpoints |

### Environment Variables

Configuration can also be set via environment variables:

- `RETRY_SOCKET_PATH`
- `RETRY_HTTP_PORT`
- `RETRY_MAX_METRICS`
- `RETRY_METRICS_MAX_AGE`
- `RETRY_LOG_LEVEL`
- `RETRY_PID_FILE`
- `RETRY_ENABLE_HTTP`
- `RETRY_ENABLE_PROFILING`

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
sudo systemctl enable retry-daemon
sudo systemctl start retry-daemon

# Check status
sudo systemctl status retry-daemon

# View logs
sudo journalctl -u retry-daemon -f
```

**launchd (macOS)**:
```bash
# Load and start
sudo launchctl load /Library/LaunchDaemons/com.retry.daemon.plist
sudo launchctl start com.retry.daemon

# Check status
sudo launchctl list | grep retry

# View logs
tail -f /usr/local/var/log/retry/daemon.log
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
        PID file path (default "/tmp/retry-daemon.pid")
  -port int
        HTTP server port (default 8080)
  -socket string
        Unix socket path (default "/tmp/retry-daemon.sock")
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
- **Recent Operations**: Latest retry operations with status
- **Real-time Updates**: Auto-refresh every 30 seconds

## Integration with Retry CLI

The patience CLI automatically sends metrics to the daemon when available. No additional configuration is required - the CLI will attempt to connect to the daemon socket and gracefully continue if the daemon is not running.

### Metrics Sent

For each retry operation, the following metrics are sent:

- Command executed
- Final status (succeeded/failed)
- Total duration
- Number of attempts
- Individual attempt details (duration, exit code, success)
- Timestamp

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
   ls -la /tmp/retry-daemon.sock
   
   # Fix permissions if needed
   sudo chmod 666 /tmp/retry-daemon.sock
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
   journalctl -u retry-daemon -n 50
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

- **Unix socket permissions**: Ensure appropriate access controls
- **HTTP API**: Consider firewall rules for HTTP port
- **User privileges**: Run daemon as dedicated user (not root)
- **Profiling endpoints**: Only enable in development/debugging
- **Log sensitivity**: Logs may contain command arguments

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
cd retry

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