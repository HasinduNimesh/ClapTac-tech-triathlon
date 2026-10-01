ALTER TABLE fleet.vehicles ADD COLUMN IF NOT EXISTS version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0);
CREATE TABLE IF NOT EXISTS fleet.audit_outbox (
    event_id TEXT PRIMARY KEY,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    delivered_at TIMESTAMPTZ,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_error TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS fleet_audit_outbox_pending_idx ON fleet.audit_outbox (created_at) WHERE delivered_at IS NULL;
