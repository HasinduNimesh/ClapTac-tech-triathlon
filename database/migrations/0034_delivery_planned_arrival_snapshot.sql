-- Preserve the published arrival estimate that was visible when a trip was prepared.
-- This immutable-at-run-creation value is the baseline for future ETA calibration.
ALTER TABLE delivery.stops
    ADD COLUMN IF NOT EXISTS planned_arrival_at TIMESTAMPTZ;
