#!/bin/bash

# Retry daemon installation script
set -e

# Configuration
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/usr/local/etc/retry"
DATA_DIR="/usr/local/var/lib/retry"
LOG_DIR="/usr/local/var/log/retry"
USER="retry"
GROUP="retry"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root"
        exit 1
    fi
}

# Detect operating system
detect_os() {
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        OS="linux"
        SERVICE_MANAGER="systemd"
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        OS="macos"
        SERVICE_MANAGER="launchd"
    else
        log_error "Unsupported operating system: $OSTYPE"
        exit 1
    fi
    log_info "Detected OS: $OS with service manager: $SERVICE_MANAGER"
}

# Create user and group
create_user() {
    if [[ "$OS" == "linux" ]]; then
        if ! getent group "$GROUP" > /dev/null 2>&1; then
            log_info "Creating group: $GROUP"
            groupadd --system "$GROUP"
        fi
        
        if ! getent passwd "$USER" > /dev/null 2>&1; then
            log_info "Creating user: $USER"
            useradd --system --gid "$GROUP" --home-dir "$DATA_DIR" \
                    --shell /bin/false --comment "Retry daemon user" "$USER"
        fi
    elif [[ "$OS" == "macos" ]]; then
        # On macOS, use _retry as the user name (convention for system users)
        USER="_retry"
        GROUP="_retry"
        
        if ! dscl . -read /Groups/"$GROUP" > /dev/null 2>&1; then
            log_info "Creating group: $GROUP"
            dscl . -create /Groups/"$GROUP"
            dscl . -create /Groups/"$GROUP" PrimaryGroupID 499
        fi
        
        if ! dscl . -read /Users/"$USER" > /dev/null 2>&1; then
            log_info "Creating user: $USER"
            dscl . -create /Users/"$USER"
            dscl . -create /Users/"$USER" UserShell /usr/bin/false
            dscl . -create /Users/"$USER" RealName "Retry daemon user"
            dscl . -create /Users/"$USER" UniqueID 499
            dscl . -create /Users/"$USER" PrimaryGroupID 499
            dscl . -create /Users/"$USER" NFSHomeDirectory "$DATA_DIR"
        fi
    fi
}

# Create directories
create_directories() {
    log_info "Creating directories"
    
    mkdir -p "$CONFIG_DIR"
    mkdir -p "$DATA_DIR"
    mkdir -p "$LOG_DIR"
    
    # Set ownership and permissions
    chown "$USER:$GROUP" "$DATA_DIR"
    chown "$USER:$GROUP" "$LOG_DIR"
    chmod 755 "$CONFIG_DIR"
    chmod 750 "$DATA_DIR"
    chmod 750 "$LOG_DIR"
}

# Install binaries
install_binaries() {
    log_info "Installing binaries"
    
    # Check if binaries exist in current directory
    if [[ ! -f "./retry" ]]; then
        log_error "retry binary not found in current directory"
        exit 1
    fi
    
    if [[ ! -f "./retryd" ]]; then
        log_error "retryd binary not found in current directory"
        exit 1
    fi
    
    # Install binaries
    cp ./retry "$INSTALL_DIR/retry"
    cp ./retryd "$INSTALL_DIR/retryd"
    
    # Set permissions
    chmod 755 "$INSTALL_DIR/retry"
    chmod 755 "$INSTALL_DIR/retryd"
    
    log_info "Binaries installed to $INSTALL_DIR"
}

# Create default configuration
create_config() {
    log_info "Creating default configuration"
    
    cat > "$CONFIG_DIR/daemon.json" << EOF
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
EOF
    
    chmod 644 "$CONFIG_DIR/daemon.json"
    log_info "Configuration created at $CONFIG_DIR/daemon.json"
}

# Install service files
install_service() {
    log_info "Installing service configuration"
    
    if [[ "$SERVICE_MANAGER" == "systemd" ]]; then
        # Copy systemd service file
        if [[ -f "./scripts/systemd/retry-daemon.service" ]]; then
            cp ./scripts/systemd/retry-daemon.service /etc/systemd/system/
            systemctl daemon-reload
            log_info "Systemd service installed"
            log_info "Enable with: systemctl enable retry-daemon"
            log_info "Start with: systemctl start retry-daemon"
        else
            log_warn "Systemd service file not found"
        fi
    elif [[ "$SERVICE_MANAGER" == "launchd" ]]; then
        # Copy launchd plist file
        if [[ -f "./scripts/launchd/com.retry.daemon.plist" ]]; then
            cp ./scripts/launchd/com.retry.daemon.plist /Library/LaunchDaemons/
            chown root:wheel /Library/LaunchDaemons/com.retry.daemon.plist
            chmod 644 /Library/LaunchDaemons/com.retry.daemon.plist
            log_info "Launchd service installed"
            log_info "Load with: sudo launchctl load /Library/LaunchDaemons/com.retry.daemon.plist"
            log_info "Start with: sudo launchctl start com.retry.daemon"
        else
            log_warn "Launchd plist file not found"
        fi
    fi
}

# Main installation function
main() {
    log_info "Starting retry daemon installation"
    
    check_root
    detect_os
    create_user
    create_directories
    install_binaries
    create_config
    install_service
    
    log_info "Installation completed successfully!"
    log_info ""
    log_info "Next steps:"
    log_info "1. Review configuration in $CONFIG_DIR/daemon.json"
    log_info "2. Start the daemon using your service manager"
    log_info "3. Access the dashboard at http://localhost:8080"
    log_info ""
    log_info "For manual testing:"
    log_info "  sudo -u $USER $INSTALL_DIR/retryd -config $CONFIG_DIR/daemon.json"
}

# Run main function
main "$@"