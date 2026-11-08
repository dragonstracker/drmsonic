#!/bin/sh
set -e

mkdir -p bin

CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o bin/drmsonic-linux-amd64 ./cmd/drmsonic

GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o bin/drmsonic-windows-amd64.exe ./cmd/drmsonic

GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o bin/drmsonic-linux-arm64 ./cmd/drmsonic

ls -la bin/
