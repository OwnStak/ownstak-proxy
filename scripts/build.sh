#!/bin/bash
source .env

# Assign default platforms if not set
if [ -z "$PLATFORMS" ]; then
    PLATFORMS="linux/amd64"
fi

# Assign default dist directory if not set
if [ -z "$DIST_DIR" ]; then
    DIST_DIR="dist"
fi

# Assign default dist name if not set
if [ -z "$DIST_NAME" ]; then
    DIST_NAME="ownstak-proxy"
fi

# Clean up dist directory
rm -rf $DIST_DIR

# Create dist directory if it doesn't exist
mkdir -p $DIST_DIR

# Build for each platform
echo "Building $APP_NAME $VERSION:"
for PLATFORM in $(echo $PLATFORMS | tr ',' '\n'); do
    echo "Building for $PLATFORM..."
    GOOS=$(echo $PLATFORM | cut -d '/' -f 1)
    GOARCH=$(echo $PLATFORM | cut -d '/' -f 2)
    GOOS=$GOOS GOARCH=$GOARCH go build -buildvcs=false -ldflags "-X '$DIST_NAME/src/constants.AppName=$APP_NAME' -X '$DIST_NAME/src/constants.Version=$VERSION' -X '$DIST_NAME/src/constants.ConsoleURL=$CONSOLE_URL'" -o $DIST_DIR/$DIST_NAME-$GOOS-$GOARCH ./src/
done

echo "✅ Build complete!"
