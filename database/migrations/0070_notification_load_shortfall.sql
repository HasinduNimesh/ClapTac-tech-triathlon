-- W4: a loader shortfall decision is sent to the affected outlet.
ALTER TABLE shared.notification_outbox DROP CONSTRAINT IF EXISTS notification_outbox_event_type_check;
ALTER TABLE shared.notification_outbox ADD CONSTRAINT notification_outbox_event_type_check
    CHECK (event_type IN ('DEFERRAL','MAJOR_DELAY','LOAD_SHORTFALL'));
