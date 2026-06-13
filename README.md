# Health Monitor

Aggregated module health monitoring and degradation detection for MuxCore.

Collects health reports from all registered modules, tracks state transitions (e.g. running → degraded), and publishes health events.

## Configuration

| Env / Flag | Default | Description |
|---|---|---|
| `HEALTH_MONITOR_GRPC_ADDR` / `--grpc-addr` | `:9202` | gRPC listen address |
| `HEALTH_MONITOR_HTTP_ADDR` / `--http-addr` | `:9203` | HTTP health check address |

## RPCs

- `ReportHealth(modules[])` — modules push their health state; degradation transitions are logged
- `PublishEvent(type, module, message)` — publish a health event for external subscribers
