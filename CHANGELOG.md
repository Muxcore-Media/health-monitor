# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.6] — 2026-08-31

### Added

- Active monitoring: Discovery `ListAll` + poll each module `/health` on `poll_interval`
- `module.recovered` events on degraded/stale → running transitions
- Operator `Notify` via `notification` capability on degraded/stale
- Mesh dial retry with backoff; `Health()` fails when mesh configured but not connected
- `--health-check` CLI flag; Docker exposes `9202`/`9203`
- Forgejo CI `golangci-lint` job

### Changed

- Default listen addresses bind to `127.0.0.1` (`9202`/`9203`)
- Mesh fan-out payloads use `contracts.ModuleDegradedPayload` / lifecycle event constants
- Declare `HealthMonitor` contract in `muxcore.json` / `Info()`
- First-seen degraded modules emit `module.degraded` (not only running → degraded)

### Fixed

- Dockerfile `HEALTHCHECK` and `EXPOSE`; docker-compose publishes HTTP port
- Docs: env vars, `/status`, `poll_interval` setting, Forgejo CI badge, compatibility matrix

## [0.1.5] — 2026-08-10

### Added

- Aggregation loop: expire module reports as `stale` after `2 × HEALTH_MONITOR_INTERVAL` (default 30s)
- HTTP `GET /status` JSON summary (modules, degraded/stale counts, recent events)
- In-memory recent event ring for `PublishEvent` + degradation/stale transitions
- Env: `HEALTH_MONITOR_HTTP_ADDR`, `HEALTH_MONITOR_INTERVAL`

### Changed

- Advertise `settings` capability for admin-ui Settings discovery
- `Info().HTTPAddr` reports the HTTP listen address

## [0.1.4] — 2026-08-10

### Added

- SettingsProvider mesh (`RegisterSettings`) for live `poll_interval` updates.

## [0.1.3] — 2026-08-10

### Fixed

- Sync Info()/muxcore.json version to **0.1.3**.

## [0.1.0] - 2026-07-22

### Added

- Initial project scaffold from muxcore-module-starter
