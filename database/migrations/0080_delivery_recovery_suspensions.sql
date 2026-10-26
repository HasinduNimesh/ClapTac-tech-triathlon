-- A broken vehicle's old stop must never accept a late/offline completion after
-- cargo has been handed over. This record survives planning retries/failures.
CREATE TABLE delivery.recovery_suspensions (
    trip_id TEXT NOT NULL,
    order_id TEXT NOT NULL,
    case_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(trip_id, order_id)
);
CREATE FUNCTION delivery.guard_recovered_stop() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE trip TEXT;
BEGIN
    IF NEW.status IN ('arrived', 'completed') AND NEW.status IS DISTINCT FROM OLD.status THEN
        SELECT trip_id INTO trip FROM delivery.runs WHERE id = NEW.run_id;
        PERFORM pg_advisory_xact_lock(hashtextextended(trip || ':' || NEW.order_id, 79));
        IF EXISTS (SELECT 1 FROM delivery.recovery_suspensions WHERE trip_id = trip AND order_id = NEW.order_id) THEN
            RAISE EXCEPTION 'conflict: this stop is suspended for breakdown recovery; contact dispatch';
        END IF;
    END IF;
    RETURN NEW;
END $$;
CREATE TRIGGER guard_recovered_stop BEFORE UPDATE OF status ON delivery.stops
    FOR EACH ROW EXECUTE FUNCTION delivery.guard_recovered_stop();
