#!/bin/bash
source .env

# Runs local mocks if Node.js is installed    
if command -v node &> /dev/null; then
    echo "Starting local mocks..."
    npm start --prefix mocks > /dev/null 2>&1 & # Start mocks in background
    trap "npm stop --prefix mocks > /dev/null 2>&1" EXIT # Stop mocks on exit
else
    echo "WARNING: Node.js is not installed. Local mocks won't run."
fi

# Start the file watcher
go run -mod=mod github.com/air-verse/air@v1.61.7 -c .air.toml