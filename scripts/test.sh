#!/bin/bash

# Fail on error
set -euo pipefail

# The default go test command output sucks,
# so we use gotestsum to show it in nicer format
#
# You can run a single package test by replacing ./src/... with the package you want to test:
# go test -v ./src/vips/
# go run -mod=mod gotest.tools/gotestsum@v1.12.2 -- -cover -v ./src/vips/
# go run -mod=mod gotest.tools/gotestsum@v1.12.2 tool slowest -- -v ./src/...
echo "Running tests..."
go run -mod=mod gotest.tools/gotestsum@v1.12.2 -- -cover -v ./src/...