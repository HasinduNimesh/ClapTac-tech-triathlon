CREATE TABLE IF NOT EXISTS delivery.trip_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES delivery.runs(id) ON DELETE CASCADE,
    stop_id UUID REFERENCES delivery.stops(id) ON DELETE CASCADE,
    body TEXT NOT NULL CHECK (length(body) BETWEEN 1 AND 1000),
    sent_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    acknowledged_by TEXT,
    acknowledged_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS delivery_trip_messages_run_idx ON delivery.trip_messages(run_id, created_at);
CREATE TABLE IF NOT EXISTS delivery.trip_message_events (
    id BIGSERIAL PRIMARY KEY,
    message_id UUID NOT NULL REFERENCES delivery.trip_messages(id) ON DELETE CASCADE,
    event_type TEXT NOT NULL CHECK(event_type IN ('SENT','ACKNOWLEDGED')),
    actor_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE OR REPLACE FUNCTION delivery.reject_message_event_mutation() RETURNS trigger AS $$
BEGIN RAISE EXCEPTION 'trip message events are append-only'; END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trip_message_events_append_only ON delivery.trip_message_events;
CREATE TRIGGER trip_message_events_append_only BEFORE UPDATE OR DELETE ON delivery.trip_message_events
FOR EACH ROW EXECUTE FUNCTION delivery.reject_message_event_mutation();
