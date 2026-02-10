# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make build          # Cross-compile static linux/amd64 binary to bin/synology-monitor
make test           # go test -v -race ./...
make lint           # go vet ./...
make clean          # Remove bin/
```

Run a single test:
```bash
go test -v -race -run TestParseSATAHealthy ./...
```

## Architecture

All code lives in `package main` (~650 lines production code). The design uses three narrow interfaces for testability:

- **`DriveDiscoverer`** (discovery.go) — globs `/dev/sata[0-9]*` and `/dev/nvme[0-9]*n1`. Takes a `basePath` param (default `/dev`) so tests use `t.TempDir()`.
- **`SmartctlRunner`** (smartctl.go) — executes `smartctl --json -a` and parses JSON output. `ParseMetrics()` extracts fields by SATA attribute ID or NVMe health log struct fields. smartctl non-zero exit is tolerated (exit bit flags); only empty output is a failure.
- **`MetricWriter`** (influx.go) — writes to InfluxDB v2 via blocking API. Measurement `smart`, tags: hostname/device/model/serial/type.

**`Collector`** (collector.go) wires `DriveDiscoverer` + `SmartctlRunner`. Individual drive failures don't stop collection of others.

**`main.go`** — flag parsing, signal handling (`SIGTERM`/`SIGINT`), immediate collection then `time.Ticker` loop. `--run-once` and `--echo` flags for debugging.

## Key Design Decisions

- SATA attributes extracted by ID: 1, 5, 7, 9, 10, 194, 197, 198 (see `sataAttributeIDs` map in smartctl.go)
- Temperature fallback: SATA attr 194 → top-level `temperature.current`
- Missing individual SATA attributes are omitted from fields, not zero-filled
- InfluxDB writes are blocking with no buffering; failures retry on next cycle
- Config: YAML file + `SM_` prefixed env var overrides; `--echo` mode skips InfluxDB validation

## Testing Patterns

- Test fixtures in `testdata/` — realistic smartctl JSON captured from real drives
- Discovery tests create fake device files in `t.TempDir()` with a custom `basePath`
- Mock implementations: `mockDiscoverer`, `mockRunner`, `mockWriter` (in their respective test files and influx.go)
- `testableInfluxWriter` with `pointRecorder` verifies InfluxDB point construction (tags, fields, timestamp) without a live instance
- Main loop tests use short ticker intervals (50ms) and context cancellation
