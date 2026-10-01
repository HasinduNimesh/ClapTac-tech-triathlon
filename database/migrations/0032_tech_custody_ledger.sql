CREATE TABLE IF NOT EXISTS orders.tech_custody_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders.orders(id),
    stage TEXT NOT NULL CHECK (stage IN ('LOADED','DISPATCHED','DELIVERED','RECEIVED')),
    seal_id TEXT NOT NULL,
    serial_numbers TEXT[] NOT NULL CHECK (cardinality(serial_numbers) > 0),
    condition TEXT NOT NULL,
    evidence_ref TEXT NOT NULL DEFAULT '',
    receiver_name TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL UNIQUE,
    recorded_by TEXT NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS tech_custody_order_stage_idx
    ON orders.tech_custody_events(order_id, recorded_at, id);

CREATE OR REPLACE FUNCTION orders.reject_tech_custody_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'Tech custody events are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tech_custody_events_immutable ON orders.tech_custody_events;
CREATE TRIGGER tech_custody_events_immutable
    BEFORE UPDATE OR DELETE ON orders.tech_custody_events
    FOR EACH ROW EXECUTE FUNCTION orders.reject_tech_custody_mutation();
