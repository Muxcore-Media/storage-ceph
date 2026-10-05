# storage-ceph

Ceph storage provider for MuxCore — **RGW (S3)**, **CephFS mount**, or **native RADOS** (`-tags ceph`).

## Storage backends

Set `CEPH_STORAGE_BACKEND` (admin **storage_backend** select: `rgw` | `cephfs` | `rados`):

| Backend | Env | Use case |
|---------|-----|----------|
| `rgw` (default) | `CEPH_RGW_*` | Ceph RGW or MinIO (S3 API), CGO-free |
| `cephfs` | `CEPH_CEPHFS_ROOT` | Mounted CephFS volume (Rook `volumeMount`), CGO-free |
| `rados` | `CEPH_MONITORS`, `CEPH_POOL`, `CEPH_USER`, `CEPH_KEYRING` | Native librados — `make build-ceph` |

```bash
# CephFS (Rook pod mount at /mnt/cephfs/muxcore)
export CEPH_STORAGE_BACKEND=cephfs
export CEPH_CEPHFS_ROOT=/mnt/cephfs/muxcore

# Native RADOS (operator cluster)
export CEPH_STORAGE_BACKEND=rados
export CEPH_MONITORS=mon-a,mon-b
export CEPH_POOL=muxcore
export CEPH_USER=client.muxcore
export CEPH_KEYRING=/etc/ceph/keyring
make build-ceph
```

## MuxCore routing

Enabling the sidecar alone does **not** move traffic off in-process `storage.local`. muxcored registers remote providers on discover, but default routing still prefers `storage.local` until a **routing policy** selects this module:

```go
// core/internal/storage/orchestrator.go — example policy
orch.AddPolicy(storage.RoutingPolicy{
    Name:     "ceph",
    Prefix:   "",           // all keys, or e.g. "media/"
    Provider: "storage-ceph",
})
```

MVP stack:

```bash
# _mvp/.env or run-host env
MVP_ENABLE_STORAGE_CEPH=1
CEPH_RGW_ENDPOINT=127.0.0.1:9000
CEPH_BUCKET=muxcore
CEPH_ACCESS_KEY=<your RGW/MinIO access key>
CEPH_SECRET_KEY=<your RGW/MinIO secret key>   # e.g. openssl rand -hex 16
```

Then add a routing policy (admin API / muxcore config) so Put/Get dial `storage-ceph` instead of `storage.local`.

Kubernetes / Rook: see [`deploy/rook-storage-ceph.yaml`](deploy/rook-storage-ceph.yaml).

## RGW / MinIO (default)

On a laptop you do **not** need a Ceph cluster. Point this module at **local MinIO** the same way you would at Ceph RGW (path-style S3).

```bash
# MinIO credentials are required (no default); generate them first:
export MINIO_ROOT_USER="muxcore-$(openssl rand -hex 4)" MINIO_ROOT_PASSWORD="$(openssl rand -hex 16)"
docker compose -f deploy/docker-compose.yml up -d

export CEPH_RGW_ENDPOINT=127.0.0.1:9000
export CEPH_BUCKET=muxcore
export CEPH_ACCESS_KEY="$MINIO_ROOT_USER"
export CEPH_SECRET_KEY="$MINIO_ROOT_PASSWORD"
export CEPH_PATH_STYLE=true
export CEPH_USE_SSL=false
export STORAGE_CEPH_GRPC_ADDR=127.0.0.1:9680
export STORAGE_CEPH_HTTP_ADDR=127.0.0.1:9681
go run ./cmd/module
```

For Rook RGW with a private CA:

```bash
export CEPH_USE_SSL=true
export CEPH_RGW_CA=/etc/ssl/certs/rook-ceph-ca.pem
# optional mTLS:
export CEPH_RGW_CLIENT_CERT=/etc/ssl/certs/rgw-client.crt
export CEPH_RGW_CLIENT_KEY=/etc/ssl/private/rgw-client.key
```

Listens gRPC `127.0.0.1:9680`, HTTP health `127.0.0.1:9681` by default.

### Why keep this module vs `storage-s3`?

Both speak S3 when `storage_backend=rgw`. Prefer [`storage-s3`](https://github.com/Muxcore-Media/storage-s3) for generic MinIO/AWS. Keep **`storage-ceph`** when you want Ceph/Rook-oriented settings (`monitors`, `pool`, `cephfs`, `rados`) and a single module that can switch backends without swapping sidecars.

## Settings

| Group | Keys |
|-------|------|
| Ceph | `storage_backend` (select), `cephfs_root`, `monitors`, `pool`, `user`, `keyring` (path) |
| RGW / MinIO | `rgw_endpoint`, `bucket`, `access_key`, `secret_key`, `prefix`, `use_ssl`, `rgw_ca`, `rgw_client_cert`, `rgw_client_key`, `path_style` |

## Tests

```bash
make test      # CGO-free, gofakes3 + unit tests
make build-ceph # optional RADOS binary (requires librados)
```

`go test` never requires a live Ceph cluster.
