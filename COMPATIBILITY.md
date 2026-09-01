# Compatibility

## Core Version

| Module Version | Core Version | Status |
|----------------|-------------|--------|
| v0.2.0         | 0.5.8+      | Current |
| v0.1.0         | 0.5.4+      | Superseded |

## Capabilities

- `storage` / `storage.ceph` — Ceph-backed object store (RGW, CephFS, or RADOS)
- `settings` — live connection knobs

## Contracts

Implements `contracts.StorageProvider` (+ `Streamable`). Exposes core `muxcore.storage.v1.StorageService` as a **provider** sidecar.

From **core v0.5.4**, muxcored dials announce/`HTTPAddr` (gRPC, default `127.0.0.1:9680`) when the module registers with capability `storage`. **Default routing still prefers in-process `storage.local`** until a routing policy in the storage orchestrator selects this module (`Provider: "storage-ceph"`).

## Backends (v0.2.0)

| Backend | Build | Notes |
|---------|-------|-------|
| `rgw` | default (CGO-free) | S3-compatible RGW or MinIO |
| `cephfs` | default (CGO-free) | POSIX on mounted CephFS |
| `rados` | `-tags ceph` + librados | Native RADOS pool |

## MVP

Set `MVP_ENABLE_STORAGE_CEPH=1` in `_mvp/run-host.sh` / `.env` to start the sidecar binary. Also configure a storage routing policy so muxcored sends Put/Get to `storage-ceph` instead of `storage.local`.
