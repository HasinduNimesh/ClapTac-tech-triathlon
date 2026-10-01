-- FR-22: Driver-side categorised incident report (vehicle, road, outlet,
-- goods, safety, other). Append-only; stop_id is optional since not every
-- incident (e.g. a road closure between stops) is tied to a specific stop.
CREATE TABLE IF NOT EXISTS delivery.driver_incidents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES delivery.runs (id) ON DELETE CASCADE,
    stop_id UUID REFERENCES delivery.stops (id) ON DELETE CASCADE,
    operation_id TEXT NOT NULL UNIQUE,
    category TEXT NOT NULL CHECK (category IN ('VEHICLE', 'ROAD', 'OUTLET', 'GOODS', 'SAFETY', 'OTHER')),
    description TEXT NOT NULL CHECK (length(btrim(description)) BETWEEN 1 AND 1000),
    reported_by TEXT NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS delivery_driver_incidents_run_idx ON delivery.driver_incidents (run_id, created_at DESC);
