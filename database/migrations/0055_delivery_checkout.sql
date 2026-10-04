CREATE TABLE IF NOT EXISTS delivery.run_checkouts (
    run_id UUID PRIMARY KEY REFERENCES delivery.runs(id) ON DELETE CASCADE,
    plan_version INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('blocked', 'confirmed')),
    confirmed_order_ids TEXT[] NOT NULL DEFAULT '{}',
    missing_order_ids TEXT[] NOT NULL DEFAULT '{}',
    checked_by TEXT NOT NULL,
    checked_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
