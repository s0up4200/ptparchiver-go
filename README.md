# PTP Archiver Go

A Go implementation of PTP's archiver client utility that allows you to allocate "containers" that PTP will provide neglected torrents to archive in. You cannot control what content is put in those containers. This tool only works with qBittorrent.

> [!WARNING]  
> **Important Note**: This implementation follows the original Python script's version (`0.10.0`). The program will stop working if the original Python script is updated, requiring an update of this Go version to maintain compatibility. This is to ensure protocol compliance and correct functionality.

## Table of Contents

- [Installation](#installation)
  - [Downloading the binary](#downloading-the-binary)
  - [Building from Source](#building-from-source)
  - [Installing using Go](#installing-using-go)
- [Quick Start](#quick-start)
- [Configuration](#configuration-example)
  - [Container Settings](#container-settings-explained)
- [Space Management](#space-management)
- [Usage](#usage)
  - [Running as a Service](#running-as-a-service)

## Installation

### Downloading the binary

1. Download the binary for your operating system:

   ```bash
   wget $(curl -s https://api.github.com/repos/s0up4200/ptparchiver-go/releases/latest | grep download | grep linux_x86_64 | cut -d\" -f4)
   ```

2. Extract the downloaded archive to `/usr/local/bin`:

   ```bash
   sudo tar -C /usr/local/bin -xzf ptparchiver*.tar.gz
   ```

3. Verify the installation:
   ```bash
   ptparchiver --help
   ```

### Building from Source

```bash
git clone https://github.com/s0up4200/ptparchiver-go.git
cd ptparchiver-go
go build -o ptparchiver ./cmd/archiver
```

### Installing using Go

Requires Go to be installed. Get it from [here](https://go.dev/dl/).

```bash
go install github.com/s0up4200/ptparchiver-go@latest
```

## Quick Start

1. Initialize a config file:

```bash
ptparchiver init
```

2. Edit the generated config file (located in either current directory or `~/.config/ptparchiver-go/config.yaml`)

3. Start archiving:

```bash
# Fetch torrents for all containers
ptparchiver fetch

# Fetch torrents for a specific container
ptparchiver fetch hetzner
```

## Configuration Example

```yaml
# PTP API credentials
apiKey: your-api-key
apiUser: your-api-user
baseUrl: https://passthepopcorn.me

# Define qBittorrent clients
qbittorrent:
  seedbox1:
    url: http://localhost:8080
    username: admin
    password: adminadmin
    basicUser: "" # optional HTTP basic auth
    basicPass: "" # optional HTTP basic auth

# Define archive containers
containers:
  main:
    size: 5T # Total storage allocation
    maxStalled: 5 # Stop fetching new torrents when this many downloads are stalled
    category: ptp-archive # qBittorrent category
    tags: # Optional qBittorrent tags
      - ptp
      - archive
    client: seedbox1 # Which qBittorrent client to use

fetchSleep: 5 # Seconds between API requests, do not set lower than 5
interval: 360 # Minutes between fetch attempts when running as a service (default: 6 hours)
```

### Container Settings Explained

- `size`: Total storage allocation for this container. This is used by PTP to track total allocation, not for local space management.
- `maxStalled`: When this many torrents in the container have stalled downloads (not uploads), the client will stop fetching new torrents until some complete or are removed. A download is considered stalled when it cannot progress due to no available peers.
- `category`: qBittorrent category to assign to downloaded torrents
- `tags`: Optional tags to assign to downloaded torrents
- `client`: Which qBittorrent client configuration to use for this container

### Space Management

The client performs disk space checks before adding each torrent:

- Checks available space in qBittorrent's download directory
- Requires enough free space for the torrent size plus a 10% buffer
- Skips the torrent if insufficient space is available

This ensures safe downloading without running out of disk space, independent of the container's configured size.

## Usage

```bash
# Initialize new config
ptparchiver init

# Fetch torrents for all containers
ptparchiver fetch

# Fetch torrents for specific container
ptparchiver fetch hetzner

# Use custom config location
ptparchiver --config /path/to/config.yaml fetch

# Enable debug logging
ptparchiver --debug fetch

# Show help
ptparchiver help

# Run as a service
ptparchiver run              # Run continuously using interval from config (default: 6 hours)
ptparchiver run --interval 30  # Override config and fetch every 30 minutes
```

### Running as a Service

The `run` command starts ptparchiver in service mode, continuously fetching torrents at a specified interval. The interval can be configured in three ways (in order of precedence):

1. Command line flag: `--interval <minutes>`
2. Config file: `interval: <minutes>`
3. Default value: 360 minutes (6 hours)

When running in Docker, you can configure the interval in your docker-compose.yml:

```yaml
services:
  ptparchiver:
    image: ghcr.io/s0up4200/ptparchiver-go:latest
    container_name: ptparchiver
    volumes:
      - ./config:/config
    restart: unless-stopped
    command: run # Runs as a service using interval from config
```
