# Health Monitor

[![CI](https://git.zem.systems/muxcore/health-monitor/actions/workflows/ci.yml/badge.svg)](https://git.zem.systems/muxcore/health-monitor/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Aggregated module health monitoring and degradation detection.**

A MuxCore sidecar module that accepts health reports from other modules, actively polls registered modules via Discovery, tracks degraded/stale state, publishes lifecycle events on the mesh bus, and notifies operators through the `notification` capability.

---

## How It Works

```
Discovery ListAll ──→ poll /health ──→ ReportHealth ──→ health-monitor
Other modules ──→ ReportHealth ────────────────┘              │
                                                               ▼
                                                    mesh events + Notify
```

When `MUXCORE_GRPC_ADDR` is set, the module connects to core with backoff, lists registered modules, and polls each module's `http_addr` `/health` endpoint every `poll_interval`. Passive `ReportHealth` RPCs from other modules are still accepted.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `HEALTH_MONITOR_GRPC_ADDR` | `127.0.0.1:9202` | gRPC listen address (loopback by default) |
| `HEALTH_MONITOR_HTTP_ADDR` | `127.0.0.1:9203` | HTTP listen address for `/health` and `/status` |
| `HEALTH_MONITOR_INTERVAL` | `30s` | Poll interval and stale detection window (also `poll_interval` setting) |
| `MUXCORE_GRPC_ADDR` | *(unset)* | Core mesh address; enables active polling and event fan-out |

### HTTP endpoints

| Path | Description |
|------|-------------|
| `GET /health` | Liveness probe (`{"status":"ok"}`) |
| `GET /status` | JSON aggregate of tracked modules, counts, and recent events |

### Settings (admin-ui)

| Key | Description |
|-----|-------------|
| `poll_interval` | Live update of `HEALTH_MONITOR_INTERVAL` (Go duration, e.g. `30s`) |

---

## Quick Start

```bash
make build

export MUXCORE_INSECURE_DISABLE_TLS=true
export MUXCORE_GRPC_ADDR=localhost:9090
./health-monitor
```

---

## Capability

`health.monitor` — Aggregated health monitoring (`HealthMonitor` contract)

## License

GPL-3.0
