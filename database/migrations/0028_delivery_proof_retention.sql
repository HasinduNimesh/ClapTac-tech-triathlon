ALTER TABLE delivery.proofs
    ADD COLUMN IF NOT EXISTS retention_hold BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS retention_hold_reason TEXT;

ALTER TABLE delivery.proofs
    DROP CONSTRAINT IF EXISTS delivery_proofs_retention_hold_reason_check;
ALTER TABLE delivery.proofs
    ADD CONSTRAINT delivery_proofs_retention_hold_reason_check
    CHECK (NOT retention_hold OR NULLIF(btrim(retention_hold_reason), '') IS NOT NULL);

CREATE INDEX IF NOT EXISTS delivery_proofs_retention_due_idx
    ON delivery.proofs (uploaded_at, stop_id)
    WHERE pending = FALSE AND retention_hold = FALSE;

-- Keep a minimal, append-only erasure record after the proof row and blob are
-- removed. It contains no file key, content hash, MIME metadata, or actor PII.
CREATE TABLE IF NOT EXISTS delivery.proof_retention_events (
    proof_id UUID PRIMARY KEY,
    stop_id UUID NOT NULL,
    proof_type TEXT NOT NULL CHECK (proof_type IN ('SIGNATURE', 'PHOTO')),
    trip_completed_at TIMESTAMPTZ NOT NULL,
    erased_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    retention_days INTEGER NOT NULL CHECK (retention_days > 0),
    policy_version TEXT NOT NULL
);

CREATE OR REPLACE FUNCTION delivery.reject_proof_retention_event_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'proof retention events are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS proof_retention_events_append_only ON delivery.proof_retention_events;
CREATE TRIGGER proof_retention_events_append_only
    BEFORE UPDATE OR DELETE ON delivery.proof_retention_events
    FOR EACH ROW EXECUTE FUNCTION delivery.reject_proof_retention_event_mutation();
