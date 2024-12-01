#!/bin/bash

# Runs inside the docker container

set -e

GOMASDIR=/gomas

cd $GOMASDIR

VERSION=$1
ARCH=$2

OUTPUT=$GOMASDIR/bin/$ARCH/gomas

if [ "$ARCH" = "arm64" ]; then
  CC=aarch64-linux-gnu-gcc \
  GOOS=linux \
  GOARCH=arm64 \
  GOARM=7 \
  CGO_ENABLED=1 \
  go build \
    -o $OUTPUT \
    -tags gpio \
    -ldflags "-X main.buildUnixTimestamp=$(date +%s) -X main.commitHash=$(git rev-parse HEAD) -X main.version=$VERSION" \
    .

elif [ "$ARCH" = "arm32" ]; then
  CC=arm-linux-gnueabihf-gcc-10 \
  GOOS=linux \
  GOARCH=arm \
  GOARM=7 \
  CGO_ENABLED=1 \
  go build \
    -o $OUTPUT \
    -tags gpio \
    -ldflags "-X main.buildUnixTimestamp=$(date +%s) -X main.commitHash=$(git rev-parse HEAD) -X main.version=$VERSION" \
    .

else
  echo "Specify arm32 or arm64"
  exit 1
fi

# arm32
# CC=arm-linux-gnueabihf-gcc-10 GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=1 go build -o ./bin/arm32/gomas .

mkdir -p $GOMASDIR/dist

tar -czvf $GOMASDIR/dist/gomas-$VERSION-$ARCH.tgz -C $GOMASDIR/bin/$ARCH .
