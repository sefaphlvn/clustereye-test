#!/bin/bash

# Configuration
GITHUB_REPO="sefaphlvn/clustereye-test"
BINARY_NAME="clustereye-api-linux-amd64"
SERVICE_NAME="clustereye-api"
INSTALL_DIR="/usr/local/bin"
GITHUB_TOKEN="${GITHUB_TOKEN:-}"  # Token will be provided via environment variable

# Check if token is provided
if [ -z "$GITHUB_TOKEN" ]; then
    echo "Error: GITHUB_TOKEN environment variable is not set"
    exit 1
fi

# Get the latest release version
echo "Checking for updates..."
LATEST_VERSION=$(curl -s -H "Authorization: token ${GITHUB_TOKEN}" \
    "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | \
    grep '"tag_name":' | \
    sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST_VERSION" ]; then
    echo "Error: Could not fetch latest version"
    exit 1
fi

echo "Latest version: ${LATEST_VERSION}"

# Get current version
CURRENT_VERSION=$($INSTALL_DIR/$BINARY_NAME --version 2>/dev/null || echo "none")
echo "Current version: ${CURRENT_VERSION}"

if [ "$LATEST_VERSION" = "$CURRENT_VERSION" ]; then
    echo "Already running the latest version"
    exit 0
fi

# Create temporary directory
TMP_DIR=$(mktemp -d)
cd $TMP_DIR

# Download the new binary
echo "Downloading new version..."
curl -s -L -H "Authorization: token ${GITHUB_TOKEN}" \
    "https://github.com/${GITHUB_REPO}/releases/download/${LATEST_VERSION}/${BINARY_NAME}" \
    -o "${BINARY_NAME}"

if [ ! -f "${BINARY_NAME}" ]; then
    echo "Error: Failed to download binary"
    rm -rf $TMP_DIR
    exit 1
fi

# Make binary executable
chmod +x "${BINARY_NAME}"

# Stop the service
echo "Stopping service..."
sudo systemctl stop $SERVICE_NAME

# Backup current binary
if [ -f "$INSTALL_DIR/$BINARY_NAME" ]; then
    echo "Backing up current binary..."
    sudo mv "$INSTALL_DIR/$BINARY_NAME" "$INSTALL_DIR/$BINARY_NAME.backup"
fi

# Install new binary
echo "Installing new binary..."
sudo mv "${BINARY_NAME}" "$INSTALL_DIR/$BINARY_NAME"

# Start the service
echo "Starting service..."
sudo systemctl start $SERVICE_NAME

# Check service status
echo "Checking service status..."
sudo systemctl status $SERVICE_NAME

# Cleanup
rm -rf $TMP_DIR

echo "Update completed successfully!" 