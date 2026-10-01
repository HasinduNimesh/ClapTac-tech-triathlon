CREATE TABLE IF NOT EXISTS audit.events (
    event_id TEXT PRIMARY KEY,
    correlation_id TEXT,
    actor_id TEXT,
    actor_type TEXT,
    action TEXT NOT NULL,
    resource_type TEXT,
    resource_id TEXT,
    previous_state JSONB,
    new_state JSONB,
    reason TEXT,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT now(),
    source TEXT
);

CREATE INDEX IF NOT EXISTS audit_resource_idx ON audit.events (resource_type, resource_id);
