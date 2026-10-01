CREATE SEQUENCE IF NOT EXISTS planning.plan_ref_seq START 1;

CREATE TABLE IF NOT EXISTS planning.plans (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_ref TEXT NOT NULL UNIQUE,
    delivery_date DATE NOT NULL UNIQUE,
    status TEXT NOT NULL CHECK (status IN ('draft', 'validated', 'confirmed')),
    generated_at TIMESTAMPTZ,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS planning.trips (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id UUID NOT NULL REFERENCES planning.plans (id) ON DELETE CASCADE,
    vehicle_id TEXT NOT NULL,
    trip_number INTEGER NOT NULL CHECK (trip_number IN (1, 2)),
    status TEXT NOT NULL DEFAULT 'draft',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_id, vehicle_id, trip_number)
);

CREATE TABLE IF NOT EXISTS planning.allocations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id UUID NOT NULL REFERENCES planning.plans (id) ON DELETE CASCADE,
    order_id TEXT NOT NULL,
    trip_id UUID NOT NULL REFERENCES planning.trips (id) ON DELETE CASCADE,
    vehicle_id TEXT NOT NULL,
    sequence INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_id, order_id)
);

CREATE TABLE IF NOT EXISTS planning.deferrals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id UUID NOT NULL REFERENCES planning.plans (id) ON DELETE CASCADE,
    order_id TEXT NOT NULL,
    outlet_id TEXT,
    reason_code TEXT NOT NULL,
    reason_detail JSONB,
    comment TEXT,
    deferred_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_id, order_id)
);
