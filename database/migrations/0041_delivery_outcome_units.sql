-- Keep quantities from driver outcomes alongside the stop snapshot. Existing
-- outcomes remain unknown until a client supplies a structured quantity.
ALTER TABLE delivery.stops
    ADD COLUMN delivered_units INTEGER CHECK (delivered_units >= 0),
    ADD COLUMN shortfall_units INTEGER GENERATED ALWAYS AS (
        CASE WHEN expected_units IS NOT NULL AND delivered_units IS NOT NULL
            THEN expected_units - delivered_units
        END
    ) STORED,
    ADD CONSTRAINT delivery_stop_delivered_units_within_expected
        CHECK (delivered_units IS NULL OR
            (expected_units IS NOT NULL AND delivered_units <= expected_units));
