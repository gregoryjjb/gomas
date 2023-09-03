# Gomas

Play shows synced to music on your Christmas lights.

## Installation

1. Grab the latest release from [the releases page](https://github.com/gregoryjjb/gomas/releases/latest):
    ```sh
    wget https://github.com/gregoryjjb/gomas/releases/download/v0.1.6/gomas-v0.1.6-arm64.tgz
    ```

2. Remove any previous version and extract the new version
    ```sh
    rm -rf /usr/local/gomas && tar -C /usr/local -xzf gomas-v0.1.6-arm64.tgz
     ```

3. To daemonize with systemd:
    ```sh
    # Gomas can create its own service file
    gomas --systemd > /etc/systemd/gomas.service
    # To start on boot:
    systemctl enable gomas
    ```

## Development

To run on non-pi hardware use the `nogpio` tag: `go run -tags nogpio .`, included in the script:

```sh
scripts/dev.ps1
```

In another terminal start the CSS build in watch mode:

```sh
npm run dev
```

### Dev dependencies

- Go (obviously)
- The requirements for [Oto](https://github.com/hajimehoshi/oto) on Linux: `apt install libasound2-dev` 
- Docker (for cross-compiling)
- GitHub CLI (for creating releases)

## Building for ARM

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

To build AND release a new version (requires GitHub CLI installed):

```sh
scripts/release.ps1 v1.2.3
```
