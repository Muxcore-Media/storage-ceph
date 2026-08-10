# Compatibility

## Core Version

| Module Version | Core Version | Status |
|----------------|-------------|--------|
| v0.1.0         | 0.5.4+      | Current (remote dial into muxcored orchestrator) |

## Capabilities

- `storage` / `storage.ceph` — Ceph RGW object store
- `settings` — live connection knobs

## Contracts

Implements `contracts.StorageProvider` (+ `Streamable`). Exposes core `muxcore.storage.v1.StorageService` as a **provider** sidecar. From **core v0.5.4**, muxcored dials announce/`HTTPAddr` (gRPC `:9680`) when the module registers with role/capability `storage`. Default routing still prefers in-process `storage.local` unless a routing policy selects this module.

## Notes

v0.1.0 uses RGW only (no CGO). `monitors` / `pool` / `user` / `keyring` are captured for docs and a future native RADOS backend.
