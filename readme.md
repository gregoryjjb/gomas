



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

Cross-compiling requires that we have an ARM-compatible GCC and the ARM version of libasound available, since this is finicky there's a Dockerfile included that spins up all our required dependencies.

Build Docker image (set `DOCKER_BUILDKIT=0` for better debugging if it's failing at some intermediate step):

```sh
docker build --tag xgomas:latest ./
```

To just build the tgz'd ARM binary:

```sh
# Run with no version to print previous version
scripts/build.ps1 v1.2.3
```

To build AND release a new version:

```sh
scripts/release.ps1 v1.2.3
```
