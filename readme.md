# Gomas

Play shows synced to music on your Christmas lights.

## Development

To run on non-pi hardware use the `nogpio` tag: `go run -tags nogpio .`, included in the script:

```powershell
scripts/dev.ps1
```

### Dependencies

- Go (obviously)
- The requirements for [Oto](https://github.com/hajimehoshi/oto) on Linux: `apt install libasound2-dev` 
- Docker (for cross-compiling)
- GitHub CLI (for creating releases)

## Building

Cross-compiling requires that we have an ARM-compatible GCC and the ARM version of libasound available, since this is finicky there's Dockerfiles included that spin up all our required dependencies.

Build Docker images (set `DOCKER_BUILDKIT=0` for better debugging if it's failing at some intermediate step):

```sh
scripts/prepare-containers.ps1
```

To build and tar binaries for arm32 and arm64:

```sh
# Run with no version to print previous version
scripts/build.ps1 v1.2.3
```

To build AND release a new version (requires GitHub CLI):

```sh
scripts/release.ps1 v1.2.3
```
