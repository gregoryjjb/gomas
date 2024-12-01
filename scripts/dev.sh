
HOST=127.0.0.1 \
GOMAS_NO_EMBED=1 \
go run \
  -ldflags "-X main.buildUnixTimestamp=$(date +%s) -X main.commitHash=$(git rev-parse HEAD) -X main.version=$(git describe --abbrev=0)-dev" \
  .
