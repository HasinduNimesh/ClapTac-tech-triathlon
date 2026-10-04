-- W11: offline records made on an older plan version are applied, never
-- overwritten, and listed for the dispatcher to settle.
CREATE TABLE IF NOT EXISTS delivery.sync_conflicts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id TEXT NOT NULL,
    stop_id TEXT,
    operation_id TEXT NOT NULL UNIQUE,
    recorded_plan_version INTEGER NOT NULL CHECK (recorded_plan_version > 0),
    current_plan_version INTEGER NOT NULL CHECK (current_plan_version > recorded_plan_version),
    detail TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    settled_by TEXT,
    settled_at TIMESTAMPTZ,
    CHECK ((settled_by IS NULL) = (settled_at IS NULL))
);
CREATE INDEX IF NOT EXISTS sync_conflicts_open_idx ON delivery.sync_conflicts (created_at) WHERE settled_at IS NULL;
