#!/bin/bash

# llm-proxy Start Script
# This script starts llm-proxy directly (no systemd), with proper error handling and logging

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SERVICE_NAME="llm-proxy"
# Resolve non-root user's HOME when running via sudo
if [[ -n "$SUDO_USER" && "$SUDO_USER" != "root" ]]; then
    USER_HOME="$(eval echo ~${SUDO_USER})"
    TARGET_USER="$SUDO_USER"
else
    USER_HOME="$HOME"
    TARGET_USER="$USER"
fi
TARGET_GROUP="$(id -gn "$TARGET_USER" 2>/dev/null || echo "$TARGET_USER")"
BASE_DIR="${USER_HOME}/.config/llm-proxy"
CONFIG_FILE="${BASE_DIR}/config.yml"
BINARY_PATH="/usr/local/bin/llm-proxy"
DEFAULT_PORT=8090
PID_FILE="${BASE_DIR}/llm-proxy.pid"
LOG_FILE="${BASE_DIR}/llm-proxy.log"

print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Note: systemd-related logic removed for simplicity; this script always starts directly

start_directly() {
    print_info "Starting llm-proxy directly..."
    
    # Check if already running
    if [[ -f "$PID_FILE" ]]; then
        local pid=$(cat "$PID_FILE")
        if kill -0 "$pid" 2>/dev/null; then
            print_warning "llm-proxy is already running (PID: $pid)"
            return 0
        else
            print_info "Removing stale PID file"
            rm -f "$PID_FILE"
        fi
    fi
    
    # Check if binary exists
    if [[ ! -x "$BINARY_PATH" ]]; then
        print_error "llm-proxy binary not found at $BINARY_PATH"
        print_info "Please run the install script first: ./deploy/install.sh"
        return 1
    fi
    
    # Check if config exists
    if [[ ! -f "$CONFIG_FILE" ]]; then
        print_warning "Configuration file not found at $CONFIG_FILE"
        print_info "Starting with default configuration..."
        CONFIG_ARGS=""
    else
        # Config exists, binary will auto-detect it from $HOME/.config/llm-proxy/
        CONFIG_ARGS=""
    fi
    
    # Ensure base directory exists and is owned by target user
    mkdir -p "$BASE_DIR"
    chown "$TARGET_USER:$TARGET_GROUP" "$BASE_DIR" 2>/dev/null || true
    
    # Start llm-proxy in background
    print_info "Starting llm-proxy process..."
    
    if [[ $EUID -eq 0 ]]; then
        # Running with sudo/root; start as invoking user so files live under their HOME
        print_info "Running as user: $TARGET_USER"
        sudo -u "$TARGET_USER" bash -c "mkdir -p '$BASE_DIR'; \"$BINARY_PATH\" $CONFIG_ARGS >> '$LOG_FILE' 2>&1 & echo \$! > '$PID_FILE'"
        local pid
        pid=$(cat "$PID_FILE" 2>/dev/null || true)
    else
        "$BINARY_PATH" $CONFIG_ARGS > "$LOG_FILE" 2>&1 &
        local pid=$!
        echo "$pid" > "$PID_FILE"
    fi
    
    # Wait a moment and check if process is still running
    sleep 2
    
    if kill -0 "$pid" 2>/dev/null; then
        print_success "llm-proxy started successfully (PID: $pid)"
        print_info "Process information:"
        echo "  • PID: $pid"
        echo "  • Log file: $LOG_FILE"
        echo "  • Config: ${CONFIG_FILE:-"default"}"
        local port
        port=$(get_configured_port)
        echo "  • Web interface: http://localhost:${port}"
        echo
        print_info "To stop llm-proxy: ./stop.sh"
        print_info "To view logs: tail -f $LOG_FILE"
    else
        print_error "llm-proxy failed to start"
        if [[ -f "$LOG_FILE" ]]; then
            print_info "Last few log lines:"
            tail -n 10 "$LOG_FILE"
        fi
        rm -f "$PID_FILE"
        return 1
    fi
}

get_configured_port() {
    local port="$DEFAULT_PORT"
    
    # Try to get port from config using llm-proxy binary
    if [[ -x "$BINARY_PATH" ]]; then
        local config_port
        config_port=$("$BINARY_PATH" config get server.port 2>/dev/null) || true
        if [[ -n "$config_port" && "$config_port" =~ ^[0-9]+$ ]]; then
            port="$config_port"
        fi
    fi
    
    echo "$port"
}

check_port() {
    local port=${1:-$DEFAULT_PORT}
    
    if command -v netstat >/dev/null 2>&1; then
        if netstat -tuln | grep -q ":$port "; then
            print_warning "Port $port is already in use"
            print_info "Processes using port $port:"
            netstat -tulnp | grep ":$port " || true
            return 1
        fi
    elif command -v ss >/dev/null 2>&1; then
        if ss -tuln | grep -q ":$port "; then
            print_warning "Port $port is already in use"
            print_info "Processes using port $port:"
            ss -tulnp | grep ":$port " || true
            return 1
        fi
    fi
    
    return 0
}

main() {
    print_info "Starting llm-proxy..."
    
    # Get configured port
    local port
    port=$(get_configured_port)
    
    # Check if port is available
    if ! check_port "$port"; then
        print_error "Cannot start llm-proxy: port $port is already in use"
        return 1
    fi
    
    # Always start directly
    start_directly
}

# Handle script arguments
case "${1:-}" in
    --help|-h)
        echo "Usage: $0"
        echo
        echo "This script starts llm-proxy directly (no systemd)."
        echo "Logs: $LOG_FILE"
        echo "PID file: $PID_FILE"
        exit 0
        ;;
    "")
        main
        ;;
    *)
        print_error "Unknown option: $1"
        print_info "Use --help for usage information"
        exit 1
        ;;
esac
