# Object-store backup and restore verification

`scripts/create-object-storage-backup.sh` assigns each full object-store snapshot a new target bucket name built from `WAYPOINT_BACKUP_BUCKET_PREFIX`, the UTC timestamp, and process ID, then invokes `scripts/verify-object-storage-restore.py`. The verifier copies every object in one S3-compatible source bucket to that separate, empty target bucket. It pages through the source inventory, preserves content type and user metadata, reads each copied object back, checks SHA-256 equality, and confirms the source inventory did not change during the copy. It never writes to or deletes from the source bucket. The source should be quiescent while the copy runs.

The restore bucket must already exist or allow bucket creation, be empty, and be different from the source location. The verifier refuses a non-empty target rather than overwriting data. If a copy fails partway through, the target can contain a partial restore; inspect it and clear/recreate that disposable target through the storage provider's normal recovery process before retrying. The source requires list/read access. The target requires list/read/write access.

## Persistent PostgreSQL archive

`scripts/create-postgres-backup.sh` writes a timestamped custom-format dump into `WAYPOINT_BACKUP_DIR`, validates its catalog with `pg_restore --list`, writes a SHA-256 sidecar, and uses mode `0600` for the archive. It refuses name collisions and never deletes older artifacts. Set `SOURCE_DATABASE_URL` and `WAYPOINT_BACKUP_DIR` from a protected secret/environment file, then run `make backup-postgres`. Store that directory on durable storage separate from the database host. The command creates an archive; use `scripts/verify-postgres-backup.sh` with a separate empty database to rehearse restoration. Never set the restore URL to the live source database.

This repository does not prescribe a production cadence, retention period, encryption/key ownership, or RPO/RTO. An infrastructure owner must select those values and configure a scheduler and independent storage before production use. The local command and test do not make the development host a durable backup target.

For local verification, keep the isolated `waypoint-browser-tests` stack running and run:

```sh
make verify-object-storage-restore
```

That command starts a disposable S3 mock on port 9001, creates a timestamped target bucket under the prefix `waypoint-proof-restore`, copies and verifies the current local `waypoint-proof` bucket, then stops the mock. It does not alter the source bucket. The test requires the local proof bucket to contain at least one object so the restore check cannot pass vacuously.

For an explicitly configured S3-compatible environment, set `SOURCE_OBJECT_ENDPOINT`, `SOURCE_OBJECT_BUCKET`, `SOURCE_OBJECT_ACCESS_KEY`, `SOURCE_OBJECT_SECRET_KEY`, `RESTORE_OBJECT_ENDPOINT`, `RESTORE_OBJECT_ACCESS_KEY`, and `RESTORE_OBJECT_SECRET_KEY`, optionally set `WAYPOINT_BACKUP_BUCKET_PREFIX`, then run:

```sh
make backup-object-storage
```

Use HTTPS endpoints, narrowly scoped credentials, and a target location with independent durability and access controls. The wrapper creates a new bucket for each run and never removes prior snapshots; if a run fails partway, that named target can contain a partial copy and must be inspected before retrying. The wrapper does not schedule runs, expire old snapshots, preserve bucket policies/tags/lifecycle rules, establish cross-region replication, or enforce recovery-time/recovery-point objectives. Production storage, backup cadence, retention, encryption, and restore objectives still need an accountable infrastructure owner.
