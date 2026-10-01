CREATE TABLE IF NOT EXISTS fleet.fuel_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    vehicle_id TEXT NOT NULL REFERENCES fleet.vehicles (vehicle_id),
    entry_date DATE NOT NULL,
    liters NUMERIC(12, 3) NOT NULL CHECK (liters > 0 AND liters <= 10000),
    receipt_ref TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '',
    recorded_by TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS fuel_entries_vehicle_date_idx
    ON fleet.fuel_entries (vehicle_id, entry_date);

CREATE OR REPLACE FUNCTION fleet.reject_fuel_entry_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'fuel ledger entries are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS fuel_entries_append_only ON fleet.fuel_entries;
CREATE TRIGGER fuel_entries_append_only
    BEFORE UPDATE OR DELETE ON fleet.fuel_entries
    FOR EACH ROW EXECUTE FUNCTION fleet.reject_fuel_entry_mutation();
