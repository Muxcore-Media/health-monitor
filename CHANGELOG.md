# Changelog

## [0.1.7] - 2026-10-05


### Security
- NFR-SEC-011 / T-M3-07: HTTP listen default is now `127.0.0.1:9203` (was `:9203`). A non-loopback `HEALTH_MONITOR_HTTP_ADDR` requires `HEALTH_MONITOR_HTTP_TOKEN` and the module refuses to start without it. When a token is set, `/status` requires `Authorization: Bearer <token>` (constant-time compare); `GET /health` stays open. No default token.

## [0.1.5] — 2026-08-10

### Changed

- Advertise `settings` capability for admin-ui Settings discovery

## [0.1.4] — 2026-08-10

### Added
- SettingsProvider mesh (`RegisterSettings`) for live `poll_interval` updates.

## [0.1.3] — 2026-08-10

### Fixed
- Sync Info()/muxcore.json version to **0.1.3**.

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.8] - 2026-10-05


### Changed

- Reported version comes from muxcore.json (ADR-0021); built on core v0.6.12 / sdk/go/module v0.6.3 (mesh enrollment, ADR-0017).

## [0.1.6] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

### Changed

- Default gRPC listen address is `127.0.0.1:9202` (override with `HEALTH_MONITOR_GRPC_ADDR`)

### Added

- Aggregation loop: expire module reports as `stale` after `2 × HEALTH_MONITOR_INTERVAL` (default 30s)
- HTTP `GET /status` JSON summary (modules, degraded/stale counts, recent events)
- In-memory recent event ring for `PublishEvent` + degradation/stale transitions
- Env: `HEALTH_MONITOR_HTTP_ADDR`, `HEALTH_MONITOR_INTERVAL`

### Changed

- Module version `0.1.1`; `Info().HTTPAddr` now reports the HTTP listen address

## [0.1.0] - 2026-07-22

### Added

- Initial project scaffold from muxcore-module-starter
