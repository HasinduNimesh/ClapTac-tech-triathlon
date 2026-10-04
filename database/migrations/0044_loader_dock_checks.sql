-- Loader dock checks from the Loader Workspace designs: a "wrong item" report
-- type, a photo on a report, when the dispatcher first saw a report, the
-- chilled-zone reading and door seal at departure, which plan version changed
-- an order line, and the loader's "tell dispatcher" alert for goods staged at
-- the wrong vehicle.

ALTER TABLE loading.issues DROP CONSTRAINT IF EXISTS issues_issue_type_check;
ALTER TABLE loading.issues ADD CONSTRAINT issues_issue_type_check
    CHECK (issue_type IN ('MISSING', 'DAMAGED', 'WRONG_ITEM'));

ALTER TABLE loading.issues
    ADD COLUMN IF NOT EXISTS photo_object_key TEXT,
    ADD COLUMN IF NOT EXISTS photo_mime TEXT CHECK (photo_mime IS NULL OR photo_mime IN ('image/jpeg', 'image/png')),
    ADD COLUMN IF NOT EXISTS seen_by TEXT,
    ADD COLUMN IF NOT EXISTS seen_at TIMESTAMPTZ;

ALTER TABLE loading.sessions
    ADD COLUMN IF NOT EXISTS ready_temperature_c NUMERIC(4,1) CHECK (ready_temperature_c IS NULL OR ready_temperature_c BETWEEN -30 AND 30),
    ADD COLUMN IF NOT EXISTS ready_seal TEXT CHECK (ready_seal IS NULL OR length(ready_seal) BETWEEN 1 AND 40);

ALTER TABLE loading.order_loads
    ADD COLUMN IF NOT EXISTS changed_in_version INTEGER,
    ADD COLUMN IF NOT EXISTS change_note TEXT;

CREATE TABLE IF NOT EXISTS loading.dock_alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_trip_id TEXT NOT NULL,
    depot TEXT NOT NULL,
    delivery_date DATE NOT NULL,
    alert_type TEXT NOT NULL CHECK (alert_type IN ('WRONG_VEHICLE')),
    order_ref TEXT NOT NULL CHECK (length(order_ref) BETWEEN 1 AND 64),
    belongs_vehicle_id TEXT NOT NULL DEFAULT '',
    note TEXT NOT NULL DEFAULT '' CHECK (length(note) <= 500),
    reported_by TEXT NOT NULL,
    idempotency_key TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_by TEXT,
    resolved_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS dock_alerts_open_idx ON loading.dock_alerts (delivery_date, depot) WHERE resolved_at IS NULL;
