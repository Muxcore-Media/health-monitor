# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Health monitor proto definition and generated Go code
- `ReportHealth` RPC: modules push health state; degraded transitions detected
- `PublishEvent` RPC: publish health events with type/module/message
- HTTP health check endpoint on separate port
- Full test suite: module lifecycle, health reporting, event publishing
- Contract declaration with `MinCoreVersion: 0.4.0`

### Changed

- Makefile/Dockerfile/docker-compose/systemd: your-module → health-monitor
