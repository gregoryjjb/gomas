#!/bin/bash

cd /gomas
# arm64
CC=aarch64-linux-gnu-gcc GOOS=linux GOARCH=arm64 GOARM=7 CGO_ENABLED=1 go build -o ./bin/arm64/gomas .

# arm32
# CC=arm-linux-gnueabihf-gcc-10 GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1 go build -o ./bin/arm32/gomas .
