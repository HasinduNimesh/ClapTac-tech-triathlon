-- A store is always told in-app what will arrive, whatever its SMS consent or alert choices are. A message
-- that may not be sent by SMS is stored as IN_APP_ONLY: it keeps its text for the store's Notifications
-- page, carries no phone number (phone_e164 is '') and is never selected by the SMS sender, which only
-- claims PENDING rows. Lists every status any workflow uses, so the order in which migrations that touch
-- this constraint run cannot drop one of them. Safe to run more than once.
ALTER TABLE shared.notification_outbox DROP CONSTRAINT IF EXISTS notification_outbox_status_check;
ALTER TABLE shared.notification_outbox ADD CONSTRAINT notification_outbox_status_check
    CHECK (status IN ('PENDING','SENDING','QUEUED','SENT','DELIVERED','FAILED','UNKNOWN','SUPPRESSED','IN_APP_ONLY'));
