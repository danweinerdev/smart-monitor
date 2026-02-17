# smart-monitor

A lightweight Go daemon that collects SMART data from SATA and NVMe drives via `smartctl` and ships metrics to InfluxDB v2. Designed for Synology DSM 7.x NAS devices but works on any Linux system with `smartctl` installed.

## Features

- Discovers SATA and NVMe drives automatically via `/dev` filesystem
- Parses `smartctl --json` output for key health indicators
- Ships metrics to InfluxDB v2 with configurable retry
- Graceful shutdown on `SIGTERM`/`SIGINT`
- Config reload on `SIGHUP`
- YAML configuration with environment variable overrides
- Debug modes: `--echo` (stdout) and `--run-once` (single collection)
- Includes a Grafana dashboard (`dashboard.json`)

## Collected Metrics

All metrics are written to the `smart` measurement with tags: `hostname`, `device`, `model`, `serial`, `type`.

### SATA Fields

| Field | SMART Attribute ID |
|---|---|
| `raw_read_error_rate` | 1 |
| `reallocated_sector_count` | 5 |
| `seek_error_rate` | 7 |
| `power_on_hours` | 9 |
| `spin_retry_count` | 10 |
| `temperature` | 194 (fallback: top-level `temperature.current`) |
| `current_pending_sector` | 197 |
| `offline_uncorrectable` | 198 |

Missing attributes are omitted, not zero-filled.

### NVMe Fields

| Field | Source |
|---|---|
| `temperature` | `nvme_smart_health_information_log.temperature` |
| `power_on_hours` | `nvme_smart_health_information_log.power_on_hours` |
| `percentage_used` | `nvme_smart_health_information_log.percentage_used` |
| `available_spare` | `nvme_smart_health_information_log.available_spare` |
| `media_errors` | `nvme_smart_health_information_log.media_errors` |
| `unsafe_shutdowns` | `nvme_smart_health_information_log.unsafe_shutdowns` |
| `data_units_read` | `nvme_smart_health_information_log.data_units_read` |
| `data_units_written` | `nvme_smart_health_information_log.data_units_written` |

## Requirements

- Go 1.25+ (build only)
- `smartctl` (smartmontools) installed on the target system
- InfluxDB v2 instance
- Root access (required by `smartctl`)

## Installation

### Build from source

```bash
make build    # produces bin/smart-monitor (linux/amd64 static binary)
```

### Deploy to Synology

```bash
# Copy binary
scp bin/smart-monitor root@nas:/usr/local/bin/

# Copy and edit config
scp config.yaml.example root@nas:/etc/synology-monitor/config.yaml

# Install systemd service
scp synology-monitor.service root@nas:/etc/systemd/system/
ssh root@nas 'systemctl daemon-reload && systemctl enable --now synology-monitor'
```

Or use the install target (run on the NAS):

```bash
make install
```

## Configuration

Create a config file (default: `/etc/synology-monitor/config.yaml`):

```yaml
influxdb:
  url: "http://192.168.1.100:8086"
  token: "your-token"
  org: "my-org"
  bucket: "synology"
  # retry_attempts: 3     # default: 3
  # retry_delay: 2s       # default: 2s
interval: 30m
# hostname: "ds920plus"  # defaults to os.Hostname()
log_level: info
```

### Environment Variable Overrides

All config values can be overridden with `SM_` prefixed environment variables:

| Variable | Config Key |
|---|---|
| `SM_INFLUXDB_URL` | `influxdb.url` |
| `SM_INFLUXDB_TOKEN` | `influxdb.token` |
| `SM_INFLUXDB_ORG` | `influxdb.org` |
| `SM_INFLUXDB_BUCKET` | `influxdb.bucket` |
| `SM_INTERVAL` | `interval` |
| `SM_HOSTNAME` | `hostname` |

## Usage

```bash
smart-monitor -config /path/to/config.yaml
```

### Flags

| Flag | Description |
|---|---|
| `-config` | Path to config file (default: `/etc/synology-monitor/config.yaml`) |
| `-run-once` | Collect once and exit |
| `-echo` | Print metrics to stdout instead of writing to InfluxDB |

### Debugging

Preview what would be sent to InfluxDB:

```bash
smart-monitor --echo --run-once
```

### Signals

| Signal | Behavior |
|---|---|
| `SIGTERM`, `SIGINT` | Graceful shutdown |
| `SIGHUP` | Reload configuration |

## Project Structure

```
cmd/smart-monitor/         Entry point (flag parsing, wiring)
internal/
  config/                  YAML config loading, validation, env overrides
  collector/               Drive discovery + smartctl orchestration
  discovery/               Drive discovery via /dev filesystem globbing
  smartctl/                smartctl execution and JSON parsing
    testdata/              Realistic smartctl JSON fixtures
config.yaml.example        Example configuration
synology-monitor.service   systemd unit file
dashboard.json             Grafana dashboard
Makefile                   Build, test, lint, install targets
```

## Development

```bash
make test     # go test -v -race ./...
make lint     # go vet ./...
make build    # cross-compile linux/amd64 binary
make clean    # remove bin/
```

Run a single test:

```bash
go test -v -race -run TestParseSATAHealthy ./internal/smartctl/...
```

## Grafana Dashboard

Import `dashboard.json` into Grafana for a pre-built SMART monitoring dashboard. It expects the `smart` measurement in InfluxDB with the tags and fields documented above.

## License

MIT
