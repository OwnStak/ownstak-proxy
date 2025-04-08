#!/bin/bash
source .env

CURRENT_OS=$(uname -s | tr '[:upper:]' '[:lower:]')
CURRENT_ARCH=$(uname -m)
CURRENT_ARCH=$(echo $CURRENT_ARCH | sed 's/x86_64/amd64/') # Replace x86_64 with amd64
CURRENT_ARCH=$(echo $CURRENT_ARCH | sed 's/aarch64/arm64/') # Replace aarch64 with arm64
PLATFORM="$CURRENT_OS/$CURRENT_ARCH"

# Assign default dist directory if not set
if [ -z "$DIST_DIR" ]; then
    DIST_DIR="dist"
fi

# Assign default dist name if not set
if [ -z "$DIST_NAME" ]; then
    DIST_NAME="ownstack-proxy"
fi

BINARY_PATH="$DIST_DIR/$DIST_NAME-$CURRENT_OS-$CURRENT_ARCH"

if [ ! -f "$BINARY_PATH" ]; then
    echo "Binary not found at '$BINARY_PATH'"
    echo "Please run 'scripts/build.sh' first"
    exit 1
fi

# Start the proxy server
$BINARY_PATH
