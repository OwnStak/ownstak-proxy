#!/bin/bash
# Install libvips binaries into the lib directory

# Fail on error
set -euo pipefail

# Version of sharp-libvips to install
SHARP_LIBVIPS_VERSION="1.1.0"

# Define supported platforms
PLATFORMS=(
    "linux-x64"
    "linux-arm64"
    "darwin-x64"
    "darwin-arm64"
    "win32-x64"
)

for platform in "${PLATFORMS[@]}"; do
    IFS='-' read -r os arch <<< "$platform"
done

# Create lib directory if it doesn't exist
mkdir -p lib

# Download packages for all platforms
echo "Downloading libvips binaries for all platforms..."
for platform in "${PLATFORMS[@]}"; do
    echo "Downloading for $platform..."
    # Use prebuilt binaries of libvips for sharp.
    # They already contains other dependencies like libjpeg, libpng, libtiff, etc.
    wget https://registry.npmjs.org/@img/sharp-libvips-$platform/-/sharp-libvips-$platform-$SHARP_LIBVIPS_VERSION.tgz -O /tmp/sharp-libvips-$platform-$SHARP_LIBVIPS_VERSION.tgz
    tar xf /tmp/sharp-libvips-$platform-$SHARP_LIBVIPS_VERSION.tgz
    IFS='-' read -r os arch <<< "$platform"
    
    # Convert platform names to match Go's naming
    osName=$os
    osName=$(echo $osName | sed 's/win32/windows/') # Replace win32 with windows that GO uses
    archName=$arch
    archName=$(echo $archName | sed 's/x64/amd64/') # Replace x64 with amd64 that GO uses
    archName=$(echo $archName | sed 's/arm64/arm64/') # Replace arm64 with arm64 that GO uses

    case "$os" in
        linux)
            extension="so"
            ;;
        darwin)
            extension="dylib"
            ;;
        win32)
            extension="dll"
            ;;
    esac
    
    # Find and move the library file to the lib directory
    find package -name "libvips-*" -exec cp {} "lib/libvips-$osName-$archName.$extension" \;
    
    # Clean up extracted and downloaded files
    rm -rf package
    rm -rf /tmp/sharp-libvips-$platform-$SHARP_LIBVIPS_VERSION.tgz
done
echo "✅ Libvips installation completed!"
