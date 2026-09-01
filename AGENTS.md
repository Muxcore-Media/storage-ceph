# AGENTS.md — storage-ceph

MuxCore sidecar module (`storage-ceph`). Workspace deploy and SSH: [`../AGENTS.md`](../AGENTS.md). Default ports: [`_mvp/PORTS.md`](../_mvp/PORTS.md).

## Module identity

| Field | Value |
|-------|-------|
| Directory | `storage-ceph` |
| Capabilities | `storage`, `storage.ceph`, `settings` |
| Contracts | `StorageProvider` v0.5.8 |
| Backends | `rgw` (default), `cephfs`, `rados` (`-tags ceph`) |

## Agent rules

- Modules run as gRPC sidecars; capabilities are the security boundary.
- TLS required in production (`MUXCORE_INSECURE_DISABLE_TLS` is dev-only).
- Default listen: `127.0.0.1:9680` (gRPC), `127.0.0.1:9681` (health).
- Match existing Go patterns; run `gofmt` and package tests before finishing.
- Cross-module events: prefer `github.com/Muxcore-Media/contracts-media/events` over deprecated `core/pkg/contracts` aliases.
- Do not edit polluted workspace dumps (see `MASTER-ROADMAP.md` Appendix H).

## Build

```bash
cd storage-ceph
nix-shell -p go --run 'make test'
nix-shell -p go --run 'make build'        # CGO-free RGW/CephFS
nix-shell -p go --run 'make build-ceph'   # RADOS (+ librados)
```

## Deploy

`MVP_ENABLE_STORAGE_CEPH=1` in `_mvp/run-host.sh`. Routing policy required so muxcored selects this module over `storage.local` — see `COMPATIBILITY.md`.
