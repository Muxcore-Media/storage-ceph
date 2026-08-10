# storage-ceph

Ceph / Rook storage provider for MuxCore.

**v0.1.0** speaks to **Ceph Object Gateway (RGW)** over the S3 API (CGO-free). Settings also capture Rook/cluster knobs (`monitors`, `pool`, `user`, `keyring`) for a future native RADOS/CephFS path.

## Run

```bash
export CEPH_RGW_ENDPOINT=127.0.0.1:7480
export CEPH_BUCKET=muxcore
export CEPH_ACCESS_KEY=...
export CEPH_SECRET_KEY=...
export CEPH_MONITORS=10.0.0.1:6789,10.0.0.2:6789
export CEPH_POOL=muxcore
export MUXCORE_GRPC_ADDR=127.0.0.1:9090
export MUXCORE_INSECURE_DISABLE_TLS=true
go run ./cmd/module
```

Listens gRPC `:9680`, HTTP health `:9681`.

## Settings

| Group | Keys |
|-------|------|
| Ceph | `monitors`, `pool`, `user`, `keyring` |
| RGW | `rgw_endpoint`, `bucket`, `access_key`, `secret_key`, `prefix`, `use_ssl`, `path_style` |

## Status

Scaffold — native librados (requires CGO) and Rook operator integration are follow-ups. Pin alongside `storage-s3`; core routes via `CapabilityStorage` when selected.
