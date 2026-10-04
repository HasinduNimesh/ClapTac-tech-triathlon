-- W4: a loader shortfall decision is sent to the affected outlet.
ALTER TABLE shared.notification_outbox DROP CONSTRAINT IF EXISTS notification_outbox_event_type_check;
ALTER TABLE shared.notification_outbox ADD CONSTRAINT notification_outbox_event_type_check
    -- Lists every event type any workflow sends, so the order in which the
    -- migrations that touch this constraint run cannot drop one of them.
    CHECK (event_type IN ('DEFERRAL','MAJOR_DELAY','ARRIVAL_CHANGE','DELIVERY_REJECTED','LOAD_SHORTFALL'));
