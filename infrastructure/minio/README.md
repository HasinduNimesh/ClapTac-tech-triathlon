# Object storage

Proof-of-delivery files use an S3-compatible API.

Local Compose uses `adobe/s3mock` because `minio/minio` is not pullable from Docker Hub in this environment. The service is still named `minio` and listens internally on `9090` (published as `localhost:9000`).

Kubernetes can use official MinIO when that image is available.

The S3-compatible full-copy restore verifier and production limitations are documented in [the object-storage backup runbook](../../docs/operations/object-storage-backup.md). The local disposable restore test runs against the isolated `waypoint-browser-tests` stack; it is not a scheduled production backup.
