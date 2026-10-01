CREATE TABLE IF NOT EXISTS loading.sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id TEXT NOT NULL UNIQUE,
    plan_id TEXT NOT NULL,
    plan_ref TEXT NOT NULL,
    delivery_date DATE NOT NULL,
    vehicle_id TEXT NOT NULL,
    depot TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'in_progress', 'ready')),
    started_by TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ready_by TEXT,
    ready_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS loading.order_loads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES loading.sessions (id) ON DELETE CASCADE,
    allocation_id TEXT NOT NULL,
    order_id TEXT NOT NULL,
    stop_sequence INTEGER NOT NULL,
    suggested_load_sequence INTEGER NOT NULL,
    expected_units INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'loaded', 'shortfall')),
    updated_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (session_id, order_id)
);

CREATE TABLE IF NOT EXISTS loading.issues (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_load_id UUID NOT NULL REFERENCES loading.order_loads (id) ON DELETE CASCADE,
    issue_type TEXT NOT NULL CHECK (issue_type IN ('MISSING', 'DAMAGED')),
    affected_units INTEGER NOT NULL CHECK (affected_units > 0),
    note TEXT,
    reported_by TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
