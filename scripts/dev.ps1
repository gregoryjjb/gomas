env `
  HOST=127.0.0.1 `
  GOMAS_NO_EMBED=1 `
  go run `
    -tags nogpio `
    -ldflags "-X main.buildUnixTimestamp=$(Get-Date -UFormat %s -Millisecond 0) -X main.commitHash=$(git rev-parse HEAD)" `
    .
