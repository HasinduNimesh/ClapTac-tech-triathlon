# Delivery-proof retention operations

Waypoint's draft proof-retention period is 180 days after the delivery run is completed. The code is present, but automatic deletion is **off by default**. A Waypoint data owner and qualified privacy/legal reviewer must approve the period, claim-handling process, and production activation before enabling it.

## Behavior

- Only uploaded proofs whose run has a server-recorded completion time at least `DELIVERY_PROOF_RETENTION_DAYS` old are eligible. Pending uploads, active trips, and proofs on retention hold are never selected.
- The worker checks every 24 hours and handles at most 1,000 proofs per sweep. More than that waits for a later run.
- For each proof, it holds the proof row lock, deletes the S3-compatible object, appends a minimal erasure event, and removes the proof row. The erasure event keeps the proof and stop IDs, type, trip-completion and erasure times, retention days, and policy version; it does not keep the blob key, file hash, MIME type, or creator.
- If object storage deletion fails, the proof row remains and the worker retries on a later run. If the database transaction fails after object deletion, the idempotent object deletion is retried while the row still exists. Worker errors and completed sweep counts are logged.
- A hold requires a reason. Holds can be managed only by the restricted database operator today; record the claim/ticket in the approved change-control system as well. Review the hold within 30 days after the claim closes. Do not enable automatic deletion until a staffed hold procedure exists.

## Activation

1. Apply `database/migrations/0028_delivery_proof_retention.sql` to the delivery database.
2. Confirm the approved duration and claim/hold process with the accountable owner.
3. Configure `DELIVERY_PROOF_RETENTION_DAYS` (1–3,650; draft default 180) and set `DELIVERY_PROOF_RETENTION_ENABLED=true` for the delivery service. Keep it false until steps 1 and 2 are complete.
4. Verify the service logs `delivery_proof_retention_enabled` with the approved duration and policy version. Check object-store credentials permit deleting only from the Waypoint proof bucket.

## Legal hold

Before the retention cutoff, an authorized database operator may place a hold:

```sql
UPDATE delivery.proofs
SET retention_hold = TRUE,
    retention_hold_reason = 'claim CASE-1234'
WHERE id = '00000000-0000-0000-0000-000000000000';
```

After the claim is resolved and the accountable reviewer authorizes release, clear the hold through the same controlled change process:

```sql
UPDATE delivery.proofs
SET retention_hold = FALSE,
    retention_hold_reason = NULL
WHERE id = '00000000-0000-0000-0000-000000000000';
```

The next sweep will evaluate the original run-completion time. Do not place names, phone numbers, or claim details in the database reason; use a non-sensitive ticket reference.

## Verification

The delivery PostgreSQL E2E test backdates a completed run, proves an eligible photo/signature blob and proof row are removed, confirms an open-claim hold preserves its blob and row, and checks the erasure table rejects updates/deletes. The retention runner unit tests cover the 180-day cutoff, storage failure retry behavior, and invalid duration bounds.
