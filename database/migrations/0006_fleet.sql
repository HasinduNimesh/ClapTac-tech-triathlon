CREATE TABLE IF NOT EXISTS fleet.vehicles (
    vehicle_id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    temp TEXT NOT NULL,
    weight_cap_kg NUMERIC(12, 3) NOT NULL,
    volume_cap_m3 NUMERIC(12, 3) NOT NULL,
    fuel_type TEXT NOT NULL DEFAULT 'diesel',
    km_per_l NUMERIC(8, 3) NOT NULL,
    weekly_fuel_quota_l NUMERIC(12, 3) NOT NULL,
    home_depot TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fleet.vehicle_availability (
    vehicle_id TEXT NOT NULL REFERENCES fleet.vehicles (vehicle_id),
    date DATE NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('available', 'unavailable', 'in_workshop')),
    reason TEXT,
    PRIMARY KEY (vehicle_id, date)
);
