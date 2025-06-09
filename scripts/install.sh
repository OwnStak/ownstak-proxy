#!/bin/bash

# Fail on error
set -euo pipefail

# Install mocks
if command -v npm &> /dev/null; then
    echo "Installing mocks..."
    npm install --prefix mocks > /dev/null 2>&1
fi

# Installs go packages
if command -v go &> /dev/null; then
    echo "Installing go packages..."
    go install ./src/
else
    echo "❌ Go is not installed"
    exit 1
fi

echo "✅ Installation complete!"