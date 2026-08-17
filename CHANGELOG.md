# Changelog

## [Unreleased]

### Changed

- Document MinIO as the laptop RGW stand-in; native RADOS/CephFS explicitly deferred
- Optional MinIO Docker CRUD smoke (`TestMinIO_PutGetListDelete`); skips without Docker
- `deploy/docker-compose.yml` MinIO fixture for local RGW-compatible testing

## [0.1.0] — 2026-08-10

### Added

- Ceph RGW (S3-compatible) `StorageProvider` + `Streamable`
- gRPC `StorageService` sidecar (`:9680`) + health (`:9681`)
- SettingsProvider for Rook/Ceph + RGW connection knobs
- Unit tests via gofakes3
