#!/bin/bash
source .env

# Start the file watcher
go run -mod=mod github.com/air-verse/air@v1.61.7 -c .air.toml