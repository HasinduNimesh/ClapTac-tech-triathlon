CREATE INDEX IF NOT EXISTS audit_timestamp_event_idx ON audit.events (timestamp DESC, event_id DESC);
CREATE INDEX IF NOT EXISTS audit_action_timestamp_idx ON audit.events (action, timestamp DESC);
CREATE INDEX IF NOT EXISTS audit_actor_timestamp_idx ON audit.events (actor_id, timestamp DESC);

CREATE OR REPLACE FUNCTION audit.reject_event_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit events are append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS audit_events_append_only ON audit.events;
CREATE TRIGGER audit_events_append_only
    BEFORE UPDATE OR DELETE ON audit.events
    FOR EACH ROW EXECUTE FUNCTION audit.reject_event_mutation();
