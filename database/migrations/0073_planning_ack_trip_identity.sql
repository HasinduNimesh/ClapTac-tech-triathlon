-- W1/W2: acknowledgements are per trip and recipient, and dispatcher reminders
-- are persisted. Idempotent; legacy rows keep NULL trip_id/vehicle_id and are
-- reported as plan-level (trip not recorded), never as acknowledging every trip.
ALTER TABLE planning.plan_acknowledgements
    ADD COLUMN IF NOT EXISTS trip_id UUID REFERENCES planning.trips(id) ON DELETE CASCADE,
    ADD COLUMN IF NOT EXISTS vehicle_id TEXT;

ALTER TABLE planning.plan_acknowledgements DROP CONSTRAINT IF EXISTS plan_acknowledgements_pkey;

CREATE UNIQUE INDEX IF NOT EXISTS plan_acknowledgements_identity_idx
    ON planning.plan_acknowledgements (plan_id, version, actor_id, actor_role, COALESCE(trip_id, '00000000-0000-0000-0000-000000000000'::uuid));

CREATE TABLE IF NOT EXISTS planning.plan_ack_reminders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id UUID NOT NULL REFERENCES planning.plans(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version > 0),
    trip_id UUID NOT NULL REFERENCES planning.trips(id) ON DELETE CASCADE,
    audience TEXT NOT NULL CHECK (audience IN ('DRIVER', 'LOADER')),
    reminded_by TEXT NOT NULL,
    reminded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (plan_id, version) REFERENCES planning.plan_publications(plan_id, version) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS plan_ack_reminders_lookup_idx
    ON planning.plan_ack_reminders (plan_id, version, trip_id, audience, reminded_at DESC);
