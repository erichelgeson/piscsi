#!/usr/bin/env bash
# Install PiSCSI Go Web Interface on Raspberry Pi

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
INSTALL_DIR="/opt/piscsi/web"
SERVICE_FILE="piscsi-web.service"
BINARY_NAME="piscsi-web"

# Detect architecture
detect_arch() {
    local arch=$(uname -m)
    case "$arch" in
        aarch64|arm64)
            echo "arm64"
            ;;
        armv7l|armhf)
            echo "armv7"
            ;;
        x86_64|amd64)
            echo "x86_64"
            ;;
        *)
            echo "unknown"
            ;;
    esac
}

# Print colored message
print_msg() {
    local color=$1
    shift
    echo -e "${color}$@${NC}"
}

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    print_msg "$RED" "❌ This script must be run as root (use sudo)"
    exit 1
fi

print_msg "$GREEN" "🚀 PiSCSI Go Web Interface Installation"
print_msg "$GREEN" "========================================"
echo ""

# Detect architecture
ARCH=$(detect_arch)
if [ "$ARCH" = "unknown" ]; then
    print_msg "$RED" "❌ Unsupported architecture: $(uname -m)"
    exit 1
fi

print_msg "$YELLOW" "Detected architecture: $ARCH"

# Determine binary filename
if [ "$ARCH" = "arm64" ]; then
    BINARY_SOURCE="$BINARY_NAME-arm64"
elif [ "$ARCH" = "armv7" ]; then
    BINARY_SOURCE="$BINARY_NAME-armv7"
else
    BINARY_SOURCE="$BINARY_NAME"
fi

# Check if binary exists
if [ ! -f "$BINARY_SOURCE" ]; then
    print_msg "$RED" "❌ Binary not found: $BINARY_SOURCE"
    print_msg "$YELLOW" "Please build the binary first with: make build-linux-$ARCH"
    exit 1
fi

# Stop existing service if running
if systemctl is-active --quiet piscsi-web; then
    print_msg "$YELLOW" "Stopping existing piscsi-web service..."
    systemctl stop piscsi-web
fi

# Create installation directory
print_msg "$YELLOW" "Creating installation directory: $INSTALL_DIR"
mkdir -p "$INSTALL_DIR"

# Copy binary
print_msg "$YELLOW" "Installing binary..."
cp "$BINARY_SOURCE" "$INSTALL_DIR/$BINARY_NAME"
chmod +x "$INSTALL_DIR/$BINARY_NAME"

# Copy web assets
if [ -d "web" ]; then
    print_msg "$YELLOW" "Installing web assets..."
    cp -r web "$INSTALL_DIR/"
fi

# Install systemd service
print_msg "$YELLOW" "Installing systemd service..."
cp "$SERVICE_FILE" /etc/systemd/system/

# Create required directories
print_msg "$YELLOW" "Creating required directories..."
mkdir -p /home/pi/images
mkdir -p /home/pi/shared_files
mkdir -p /home/pi/.config/piscsi

# Set ownership
print_msg "$YELLOW" "Setting ownership..."
chown -R pi:piscsi "$INSTALL_DIR"
chown -R pi:piscsi /home/pi/images
chown -R pi:piscsi /home/pi/shared_files
chown -R pi:piscsi /home/pi/.config/piscsi

# Reload systemd
print_msg "$YELLOW" "Reloading systemd..."
systemctl daemon-reload

# Enable service
print_msg "$YELLOW" "Enabling piscsi-web service..."
systemctl enable piscsi-web

# Start service
print_msg "$YELLOW" "Starting piscsi-web service..."
systemctl start piscsi-web

# Check status
sleep 2
if systemctl is-active --quiet piscsi-web; then
    print_msg "$GREEN" "✅ Installation complete!"
    echo ""
    print_msg "$GREEN" "Service Status:"
    systemctl status piscsi-web --no-pager -l
    echo ""
    print_msg "$GREEN" "Web interface should be available at: http://$(hostname -I | awk '{print $1}'):8080"
    echo ""
    print_msg "$YELLOW" "Useful commands:"
    echo "  sudo systemctl status piscsi-web   # Check service status"
    echo "  sudo systemctl restart piscsi-web  # Restart service"
    echo "  sudo systemctl stop piscsi-web     # Stop service"
    echo "  sudo journalctl -u piscsi-web -f   # View logs"
else
    print_msg "$RED" "❌ Service failed to start"
    print_msg "$YELLOW" "Check logs with: sudo journalctl -u piscsi-web -n 50"
    exit 1
fi
