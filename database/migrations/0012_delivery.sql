CREATE TABLE IF NOT EXISTS delivery.runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    trip_id TEXT NOT NULL UNIQUE,
    plan_id TEXT NOT NULL,
    plan_ref TEXT NOT NULL,
    delivery_date DATE NOT NULL,
    vehicle_id TEXT NOT NULL,
    depot TEXT NOT NULL,
    trip_number INTEGER NOT NULL DEFAULT 0,
    vehicle_type TEXT NOT NULL DEFAULT '',
    vehicle_temperature_capability TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('prepared', 'in_progress', 'completed')),
    started_by TEXT,
    started_at TIMESTAMPTZ,
    completed_by TEXT,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    version INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS delivery.stops (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES delivery.runs (id) ON DELETE CASCADE,
    allocation_id TEXT NOT NULL,
    order_id TEXT NOT NULL,
    order_ref TEXT NOT NULL DEFAULT '',
    outlet_id TEXT NOT NULL DEFAULT '',
    brand TEXT NOT NULL DEFAULT '',
    outlet_name TEXT NOT NULL DEFAULT '',
    district TEXT NOT NULL DEFAULT '',
    dock_type TEXT NOT NULL DEFAULT '',
    parking_constraint TEXT NOT NULL DEFAULT '',
    stop_sequence INTEGER NOT NULL,
    temperature_requirement TEXT NOT NULL DEFAULT '',
    planned_window_open TEXT NOT NULL DEFAULT '',
    planned_window_close TEXT NOT NULL DEFAULT '',
    loading_status TEXT NOT NULL DEFAULT '',
    loading_shortfall_summary JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL CHECK (status IN ('pending', 'arrived', 'completed')),
    arrived_at TIMESTAMPTZ,
    arrived_received_at TIMESTAMPTZ,
    outcome_code TEXT,
    outcome_reason TEXT,
    outcome_note TEXT,
    outcome_at TIMESTAMPTZ,
    outcome_received_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    version INTEGER NOT NULL DEFAULT 1,
    UNIQUE (run_id, order_id)
);

CREATE TABLE IF NOT EXISTS delivery.proofs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stop_id UUID NOT NULL REFERENCES delivery.stops (id) ON DELETE CASCADE,
    proof_type TEXT NOT NULL CHECK (proof_type IN ('SIGNATURE', 'PHOTO')),
    object_key TEXT NOT NULL,
    mime_type TEXT NOT NULL,
    sha256 TEXT NOT NULL DEFAULT '',
    captured_at TIMESTAMPTZ,
    uploaded_at TIMESTAMPTZ,
    created_by TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    pending BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS delivery.sync_operations (
    operation_id TEXT PRIMARY KEY,
    run_id TEXT NOT NULL,
    stop_id TEXT,
    operation_type TEXT NOT NULL,
    occurred_at TIMESTAMPTZ,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    result_status TEXT NOT NULL CHECK (result_status IN ('APPLIED', 'CONFLICT', 'REJECTED')),
    result_payload JSONB NOT NULL DEFAULT '{}'::jsonb
);
