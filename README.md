# storage-ceph

Ceph Object Gateway (**RGW**) storage provider for MuxCore — **S3-compatible**, CGO-free.

## Laptop path: MinIO as RGW stand-in

On a laptop you do **not** need a Ceph cluster. Point this module at **local MinIO** the same way you would at Ceph RGW (path-style S3). That is the supported demo / CI path.

```bash
# terminal A — MinIO (RGW stand-in)
docker run --rm -p 9000:9000 -p 9001:9001 \
  -e MINIO_ROOT_USER=minioadmin -e MINIO_ROOT_PASSWORD=minioadmin \
  minio/minio server /data --console-address :9001

# terminal B — module (same knobs as production RGW)
export CEPH_RGW_ENDPOINT=127.0.0.1:9000
export CEPH_BUCKET=muxcore
export CEPH_ACCESS_KEY=minioadmin
export CEPH_SECRET_KEY=minioadmin
export CEPH_PATH_STYLE=true
export CEPH_USE_SSL=false
export MUXCORE_GRPC_ADDR=127.0.0.1:9090
export MUXCORE_INSECURE_DISABLE_TLS=true
go run ./cmd/module
```

Or use the checked-in compose fixture:

```bash
docker compose -f deploy/docker-compose.yml up -d
# CEPH_RGW_ENDPOINT=127.0.0.1:9000 …
```

Listens gRPC `:9680`, HTTP health `:9681`.

### Why keep this module vs `storage-s3`?

Both speak S3. Prefer [`storage-s3`](https://github.com/Muxcore-Media/storage-s3) for generic MinIO/AWS. Keep **`storage-ceph`** when you want Ceph/Rook-oriented settings (`monitors`, `pool`, `user`, `keyring`) in admin settings for a future native path, while still using MinIO or real RGW today. Do **not** archive this repo yet — it is not redundant with `storage-s3` for operators targeting Ceph.

## Deferred: native RADOS / CephFS

| Path | Status |
|------|--------|
| RGW via S3 API (minio-go) | **Supported** — MinIO locally, Ceph RGW in prod |
| Native librados / CephFS | **Deferred** — needs CGO; not in laptop/CI gates |
| Rook operator wiring | **Deferred** — settings knobs reserved only |

`CEPH_MONITORS` / `CEPH_POOL` / `CEPH_USER` / `CEPH_KEYRING` are accepted and exposed in SettingsProvider for forward compatibility; they are **not** used by the S3/RGW client in v0.1.x.

## Settings

| Group | Keys |
|-------|------|
| Ceph (deferred native) | `monitors`, `pool`, `user`, `keyring` |
| RGW / MinIO | `rgw_endpoint`, `bucket`, `access_key`, `secret_key`, `prefix`, `use_ssl`, `path_style` |

## Tests

```bash
make test
```

- **Default:** `gofakes3` in-memory S3 API (no Docker, no Ceph).
- **Optional:** `TestMinIO_PutGetListDelete` against a real MinIO container when Docker is available; skips otherwise.

`go test` never requires a Ceph cluster or cloud credentials.
