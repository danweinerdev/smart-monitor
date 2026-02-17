# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make build          # Cross-compile static linux/amd64 binary to bin/smart-monitor
make test           # go test -v -race ./...
make lint           # go vet ./...
make clean          # Remove bin/
```

Run a single test:
```bash
go test -v -race -run TestParseSATAHealthy ./internal/smartctl/...
```

## Architecture

Standard Go layout with four internal packages and a thin `cmd/smart-monitor` entry point.

### Directory structure

```
cmd/smart-monitor/     — package main (entry point, flag parsing, wiring)
internal/
  config/              — YAML + env var config loading, validation, go-monitor config conversion
  collector/           — orchestrates discovery + smartctl, adapts to monitor.CollectFunc
  discovery/           — drive discovery by globbing /dev (DriveDiscoverer interface)
  smartctl/            — smartctl execution + JSON parsing (SmartctlRunner interface)
    testdata/          — realistic smartctl JSON fixtures
```

### Package dependency graph (no cycles)

```
discovery  ←  smartctl  ←  collector  ←  cmd/smart-monitor
                                ↑
config  ←───────────────────────┘
```

- **`discovery`** (internal/discovery) — `DriveDiscoverer` interface, `DevFSDiscoverer` impl. Globs `/dev/sata[0-9]*` and `/dev/nvme[0-9]*n1`. Takes a `basePath` param so tests use `t.TempDir()`.
- **`smartctl`** (internal/smartctl) — `SmartctlRunner` interface, `ExecSmartctl` impl. Executes `smartctl --json -a` and parses JSON output. `ParseMetrics()` extracts fields by SATA attribute ID or NVMe health log struct fields. smartctl non-zero exit is tolerated (exit bit flags); only empty output is a failure.
- **`collector`** (internal/collector) — `Collector` wires `DriveDiscoverer` + `SmartctlRunner`. Individual drive failures don't stop collection of others. `MakeCollectFunc()` adapts to `monitor.CollectFunc`.
- **`config`** (internal/config) — YAML file + `SM_` prefixed env var overrides. `MonitorConfig()` and `InfluxBackendConfig()` convert to go-monitor types.

Uses `github.com/danweinerdev/go-monitor` for infrastructure (main loop, signals, backends, retry, stats, echo mode).

## Key Design Decisions

- SATA attributes extracted by ID: 1, 5, 7, 9, 10, 194, 197, 198 (see `sataAttributeIDs` map in internal/smartctl/smartctl.go)
- Temperature fallback: SATA attr 194 → top-level `temperature.current`
- Missing individual SATA attributes are omitted from fields, not zero-filled
- Config: YAML file + `SM_` prefixed env var overrides; `--echo` mode skips InfluxDB validation

## Testing Patterns

- Test fixtures in `internal/smartctl/testdata/` — realistic smartctl JSON captured from real drives
- Discovery tests create fake device files in `t.TempDir()` with a custom `basePath`
- Mock implementations: `mockDiscoverer`, `mockRunner` (local to each test package)
- Integration tests in `cmd/smart-monitor/` use `monitor.NewEcho(&buf, nil)` + `WithRunOnce(true)`
