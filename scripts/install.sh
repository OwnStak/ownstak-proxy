#!/bin/bash

# Fail on error
set -euo pipefail

# Installs go packages
go install ./src/

echo "✅ Installation complete!"