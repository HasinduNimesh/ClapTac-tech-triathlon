-- Distance a vehicle has driven on a run, built from the driver phone's position reports. Only the latest point and
-- the running total are kept (never a trail of every point), so the dispatcher can compare the distance with the
-- plan and estimate fuel use without storing where the driver has been.
ALTER TABLE delivery.run_locations
    ADD COLUMN IF NOT EXISTS distance_m DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (distance_m >= 0),
    ADD COLUMN IF NOT EXISTS fixes INTEGER NOT NULL DEFAULT 0 CHECK (fixes >= 0);
