CREATE TABLE IF NOT EXISTS delivery.arrival_predictions (
    stop_id UUID PRIMARY KEY REFERENCES delivery.stops(id) ON DELETE CASCADE,
    plan_version INTEGER NOT NULL,
    source_event_at TIMESTAMPTZ,
    current_eta TIMESTAMPTZ NOT NULL,
    communicated_eta TIMESTAMPTZ NOT NULL,
    previous_notified_eta TIMESTAMPTZ,
    notified_at TIMESTAMPTZ,
    range_lower TIMESTAMPTZ,
    range_upper TIMESTAMPTZ,
    risk TEXT NOT NULL,
    notice_sequence INTEGER NOT NULL DEFAULT 0,
    pending_event_key TEXT NOT NULL DEFAULT '',
    pending_old_eta TIMESTAMPTZ,
    pending_new_eta TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
