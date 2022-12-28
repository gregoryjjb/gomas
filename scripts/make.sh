#!/bin/bash

# Runs inside the docker container

GOMASDIR=/gomas

cd $GOMASDIR

VERSION=$1

# arm64
CC=aarch64-linux-gnu-gcc \
GOOS=linux \
GOARCH=arm64 \
GOARM=7 \
CGO_ENABLED=1 \
go build \
  -o $GOMASDIR/bin/arm64/gomas \
  -ldflags "-X main.buildUnixTimestamp=$(date +%s) -X main.commitHash=$(git rev-parse HEAD) -X main.version=$VERSION" \
  .

# arm32
# CC=arm-linux-gnueabihf-gcc-10 GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1 go build -o ./bin/arm32/gomas .

mkdir -p $GOMASDIR/dist
rm -r $GOMASDIR/dist/*

tar -czvf $GOMASDIR/dist/gomas-$VERSION-arm64.tgz -C $GOMASDIR/bin/arm64 .
