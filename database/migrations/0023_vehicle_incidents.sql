CREATE TABLE IF NOT EXISTS fleet.vehicle_incidents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vehicle_id TEXT NOT NULL REFERENCES fleet.vehicles(vehicle_id),
    incident_date DATE NOT NULL,
    trip_id TEXT,
    incident_type TEXT NOT NULL CHECK (incident_type IN ('breakdown','accident','temperature_failure','other')),
    description TEXT NOT NULL,
    affected_stops JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved')),
    reported_by TEXT NOT NULL,
    reported_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_by TEXT,
    resolved_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS vehicle_incidents_open_idx ON fleet.vehicle_incidents(status,incident_date,reported_at DESC);
