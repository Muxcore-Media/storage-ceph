# Changelog

## [0.2.0] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

## [0.2.0] — 2026-08-31

### Added

- **CephFS backend** path confinement on `List`; stream `Close` fixes
- **RADOS backend** chunked Put/Get/Stream via `radosIO` interface; `make build-ceph`
- Admin `storage_backend` select (`rgw|cephfs|rados`); RGW TLS CA + optional mTLS client cert settings
- Keyring path validation for `backend=rados`; generic `/health` 503 body
- Streaming gRPC `Put` (no full-object buffer); `Backend.Close()` on stop/settings swap with rollback
- `deploy/rook-storage-ceph.yaml`; CI sibling checkout + golangci-lint + race tests
- Module/server tests (capabilities, secret masking, dial smoke, large Put streaming)

### Changed

- Version 0.2.0; default bind `127.0.0.1:9680` / `127.0.0.1:9681`
- Docs: `MVP_ENABLE_STORAGE_CEPH`, routing policy vs `storage.local`, all three backends

## [0.1.0] — 2026-08-10

### Added

- Ceph RGW (S3-compatible) `StorageProvider` + `Streamable`
- gRPC `StorageService` sidecar + health HTTP
- SettingsProvider for RGW connection knobs
- Unit tests via gofakes3
