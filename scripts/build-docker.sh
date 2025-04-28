#!/bin/bash
source .env

# Fail on error
set -euo pipefail

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
    DIST_NAME="ownstak-proxy"
fi

# Build binaries
./scripts/build.sh

# Build the Docker image
sudo docker build --build-arg DIST_NAME=$DIST_NAME --build-arg PLATFORM=$PLATFORM --build-arg OS=$CURRENT_OS --build-arg ARCH=$CURRENT_ARCH -t $REGISTRY_URL/$DIST_NAME:$VERSION-next -f Dockerfile .

echo "✅ Docker image build complete!"
echo "Image: $REGISTRY_URL/$DIST_NAME:$VERSION-next"