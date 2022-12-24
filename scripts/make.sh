#!/bin/bash

# Runs inside the docker container

cd /gomas

# arm64
CC=aarch64-linux-gnu-gcc \
GOOS=linux \
GOARCH=arm64 \
GOARM=7 \
CGO_ENABLED=1 \
go build \
  -o ./bin/arm64/gomas \
  -ldflags "-X main.buildUnixTimestamp=$(date +%s) -X main.commitHash=$(git rev-parse HEAD)" \
  .

# arm32
# CC=arm-linux-gnueabihf-gcc-10 GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1 go build -o ./bin/arm32/gomas .
