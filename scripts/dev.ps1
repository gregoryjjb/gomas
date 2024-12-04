env `
  HOST=127.0.0.1 `
  GOMAS_DISABLE_EMBED=1 `
  go run `
    -ldflags "-X main.buildUnixTimestamp=$(Get-Date -UFormat %s -Millisecond 0) -X main.commitHash=$(git rev-parse HEAD) -X main.version=$(git describe --abbrev=0)-dev" `
    .
