# Moving delivery-proof storage to Cloudflare R2

Compose's default object store is `adobe/s3mock`, a test double with no persistent volume. Production should use a private, durable bucket. Cloudflare R2 works with the S3 client in `pkg/objectstore` (path-style, SigV4, region `auto`).

## One thing that must be set

The upload path used to ask the store to create the bucket before every write. A key limited to one bucket is refused that request (`AccessDenied`), so every proof upload would fail. Set `MINIO_AUTO_CREATE_BUCKET=false` for R2; the local mock keeps the default `true`.

## Settings (VM `.env`)

```
MINIO_ENDPOINT=https://<account>.r2.cloudflarestorage.com
MINIO_BUCKET=waypoint-proof
MINIO_ACCESS_KEY=<bucket-scoped object read/write key>
MINIO_SECRET_KEY=<its secret>
MINIO_REGION=auto
MINIO_AUTO_CREATE_BUCKET=false
```

`docker-compose.yml` passes these to `delivery-service` (writes) and `loading-service` (reads).

## Cut-over

1. Back up `.env`. Test the key: put, get and delete one object under `connectivity-check/`.
2. Copy every object from the old store to R2 with the same keys; compare the count and total size.
3. Put the settings above in `.env`, then recreate only `delivery-service` and `loading-service`.
4. Upload one proof from the driver app and open it from the dispatcher screen.
5. Roll back by restoring `.env` and recreating the two services. The old store is untouched, so keep it until R2 has served real proofs.
