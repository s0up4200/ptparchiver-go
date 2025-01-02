# PTP Archiver Go

A Go implementation of PTP's archiver client utility that allows you to allocate "containers" that PTP will provide neglected torrents to archive in. You cannot control what content is put in those containers. This tool only works with qBittorrent.

> [!WARNING]  
> **Important Note**: This implementation follows the original Python script's version (`0.10.0`). The program will stop working if the original Python script is updated, requiring an update of this Go version to maintain compatibility. This is to ensure protocol compliance and correct functionality.

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
    maxStalled: 5 # Max number of stalled torrents before pausing
    category: ptp-archive # qBittorrent category
    tags: # Optional qBittorrent tags
      - ptp
      - archive
    client: seedbox1 # Which qBittorrent client to use

fetchSleep: 5 # Seconds between API requests, do not set lower than 5
```

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
```

## License

MIT
