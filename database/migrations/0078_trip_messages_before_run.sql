-- A dispatcher can decide a loading shortfall before the driver has started the trip, and the driver must see that
-- decision. A trip has no delivery run until it is prepared, so messages are now kept by trip id and a run is optional.
-- Additive only: existing rows keep working and are backfilled.
ALTER TABLE delivery.trip_messages ADD COLUMN IF NOT EXISTS trip_id TEXT;
ALTER TABLE delivery.trip_messages ALTER COLUMN run_id DROP NOT NULL;

UPDATE delivery.trip_messages m SET trip_id = r.trip_id
FROM delivery.runs r WHERE r.id = m.run_id AND m.trip_id IS NULL;

-- A row written with only a run id (returned-goods notices) still gets its trip id.
CREATE OR REPLACE FUNCTION delivery.trip_messages_fill_trip() RETURNS trigger AS $$
BEGIN
  IF NEW.trip_id IS NULL AND NEW.run_id IS NOT NULL THEN
    SELECT trip_id INTO NEW.trip_id FROM delivery.runs WHERE id = NEW.run_id;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS trip_messages_fill_trip ON delivery.trip_messages;
CREATE TRIGGER trip_messages_fill_trip BEFORE INSERT ON delivery.trip_messages
FOR EACH ROW EXECUTE FUNCTION delivery.trip_messages_fill_trip();

CREATE INDEX IF NOT EXISTS delivery_trip_messages_trip_idx ON delivery.trip_messages(trip_id, created_at);
