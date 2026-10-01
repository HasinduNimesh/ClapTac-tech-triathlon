ALTER TABLE planning.allocations
    ADD COLUMN IF NOT EXISTS planned_arrival_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS planned_service_start_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS planned_departure_at TIMESTAMPTZ;
