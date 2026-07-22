# Health Monitor

[![CI](https://github.com/Muxcore-Media/health-monitor/actions/workflows/ci.yml/badge.svg)](https://github.com/Muxcore-Media/health-monitor/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Aggregated module health monitoring and degradation detection.**

A MuxCore sidecar module that accepts health reports from other modules, tracks degraded state, and publishes health events. Provides the `health.monitor` capability.

---

## How It Works

```
Modules ──→ ReportHealth ──→ health-monitor
                                  │
                                  ▼
                           Aggregate status /
                           PublishEvent
```

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `HEALTH_MONITOR_GRPC_ADDR` | `:9202` | gRPC listen address |
| HTTP listen | `:9203` | Health/HTTP listen address |

---

## Quick Start

```bash
make build

export MUXCORE_INSECURE_DISABLE_TLS=true
./health-monitor --muxcore-mesh-addr localhost:9090
```

---

## Capability

`health.monitor` — Aggregated health monitoring

## License

GPL-3.0
