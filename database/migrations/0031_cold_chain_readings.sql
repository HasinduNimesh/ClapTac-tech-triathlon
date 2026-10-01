ALTER TABLE shared.outlets
    ADD COLUMN IF NOT EXISTS chilled_temperature_min_c NUMERIC(5,2),
    ADD COLUMN IF NOT EXISTS chilled_temperature_max_c NUMERIC(5,2);

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'outlets_chilled_temperature_range_check') THEN
        ALTER TABLE shared.outlets ADD CONSTRAINT outlets_chilled_temperature_range_check
        CHECK ((chilled_temperature_min_c IS NULL AND chilled_temperature_max_c IS NULL) OR
               (chilled_temperature_min_c BETWEEN -40 AND 40 AND chilled_temperature_max_c BETWEEN -40 AND 40 AND chilled_temperature_min_c < chilled_temperature_max_c));
    END IF;
END $$;

ALTER TABLE delivery.stops
    ADD COLUMN IF NOT EXISTS chilled_temperature_min_c NUMERIC(5,2),
    ADD COLUMN IF NOT EXISTS chilled_temperature_max_c NUMERIC(5,2);

CREATE TABLE IF NOT EXISTS delivery.temperature_readings (
    operation_id TEXT PRIMARY KEY,
    run_id UUID NOT NULL REFERENCES delivery.runs(id) ON DELETE CASCADE,
    stop_id UUID NOT NULL REFERENCES delivery.stops(id) ON DELETE CASCADE,
    value_c NUMERIC(5,2) NOT NULL CHECK (value_c BETWEEN -40 AND 100),
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_id TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('manual','iot')),
    source_ref TEXT NOT NULL DEFAULT '',
    min_c_snapshot NUMERIC(5,2),
    max_c_snapshot NUMERIC(5,2),
    evaluation TEXT NOT NULL CHECK (evaluation IN ('IN_RANGE','OUT_OF_RANGE','LIMITS_UNCONFIGURED')),
    note TEXT NOT NULL DEFAULT '',
    CHECK ((min_c_snapshot IS NULL AND max_c_snapshot IS NULL) OR min_c_snapshot < max_c_snapshot)
);

CREATE INDEX IF NOT EXISTS temperature_readings_stop_time_idx
    ON delivery.temperature_readings(stop_id, occurred_at DESC);
