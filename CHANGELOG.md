# Changelog


## [0.1.3] — 2026-08-10

### Fixed
- Sync Info()/muxcore.json version to **0.1.3**.

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
