# Gomas

Play shows synced to music on your Christmas lights.

## Installation

1. Grab the latest release from [the releases page](https://github.com/gregoryjjb/gomas/releases/latest):
    ```sh
    wget https://github.com/gregoryjjb/gomas/releases/download/v0.1.13/gomas-v0.1.13-arm64.tgz
    ```

2. Remove any previous version and extract the new version
    ```sh
    rm -rf /usr/local/bin/gomas && tar -C /usr/local/bin -xzf gomas-v0.1.13-arm64.tgz

    # Or, more likely, with sudo:
    sudo rm -rf /usr/local/bin/gomas && sudo tar -C /usr/local/bin -xzf gomas-v0.1.13-arm64.tgz
     ```

3. To daemonize with systemd:
    ```sh
    # Gomas can create its own service file
    gomas --systemd --user="$(whoami)" > /etc/systemd/system/gomas.service
    # To start on boot:
    systemctl enable gomas
    ```
    Or if upgrading an existing installation:
    ```sh
    systemctl restart gomas
    ```

## Configuration

Config is provided by placing a `gomas.toml` file in either the working directory, the home directory, or somewhere else and passed to Gomas by the `GOMAS_CONFIG` environment variable or the `--config` flag.

```toml
# Required! Path to data directory
data_dir = "/path/to/data"

# BCM pin numbering of channels; necessary to play any lights
pinout = [1, 2, 3, 4, 5, 6, 7, 8]

# Skip this many channels when mapping channels to pins; useful when this instance is responsible for playing only a subset of channels
channel_offset = 8

# How long to rest in between each song
rest_period = "5s"

# FPS that the show runs at
frames_per_second = 120

# Offset (delay) playback of lights keyframes by this duration. Useful if 
# there is an uncontrollable but consistent amount of speaker lag.
bias = "100ms"

# Duration of audio to buffer. Playback is delayed by this amount.
speaker_buffer = "100ms"
```

## Development

By default, GPIO is not included. Build/run with `-tags gpio` to include it.

To start the dev server:

```sh
scripts/dev.ps1
```

In another terminal start the CSS build in watch mode:

```sh
npm run dev
```

With hot reloading (this is missing the ldflags):

```sh
modd
```

To start a slave instance with hot reloading:

```sh
$env:PORT=1226; $env:GOMAS_CONFIG="gomas-slave.toml"; modd
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

## Time sync

1. Install chrony `apt install chrony`
2. Update the config `sudo nano /etc/chrony/chrony.conf` to have your time server
3. 
