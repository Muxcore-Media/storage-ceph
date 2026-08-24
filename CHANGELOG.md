# Changelog

## [Unreleased]

### Added

- **CephFS backend** (`CEPH_STORAGE_BACKEND=cephfs`, `CEPH_CEPHFS_ROOT`) — POSIX store on mounted CephFS, CGO-free
- **RADOS backend** (`CEPH_STORAGE_BACKEND=rados`) — native librados with `go build -tags ceph`
- Admin settings: `storage_backend`, `cephfs_root`; backend factory in `internal/store/open.go`

### Changed

- Document MinIO as the laptop RGW stand-in
- Optional MinIO Docker CRUD smoke (`TestMinIO_PutGetListDelete`); skips without Docker
- `deploy/docker-compose.yml` MinIO fixture for local RGW-compatible testing

## [0.1.0] — 2026-08-10

### Added

- Ceph RGW (S3-compatible) `StorageProvider` + `Streamable`
- gRPC `StorageService` sidecar (`:9680`) + health (`:9681`)
- SettingsProvider for Rook/Ceph + RGW connection knobs
- Unit tests via gofakes3
