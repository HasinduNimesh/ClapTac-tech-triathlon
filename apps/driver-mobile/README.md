# Waypoint Driver (Flutter)

Offline-first route execution. Interfaces:

- `LocalDatabase`
- `SyncQueue` / `SyncEvent`
- `ConflictResolution`

Each sync event has a client-generated `idempotencyKey`. Retrying must not create duplicate outcomes or proof records.

Generate platform projects if they are missing:

```bash
flutter create . --project-name waypoint_driver
flutter analyze
```

The agent plane is not on this workflow.
