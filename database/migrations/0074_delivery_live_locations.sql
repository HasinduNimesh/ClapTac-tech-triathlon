CREATE TABLE IF NOT EXISTS delivery.run_locations (
    run_id UUID PRIMARY KEY REFERENCES delivery.runs(id) ON DELETE CASCADE,
    vehicle_id TEXT NOT NULL,
    latitude DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    recorded_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
