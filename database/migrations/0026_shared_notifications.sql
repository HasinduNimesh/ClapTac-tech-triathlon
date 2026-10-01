-- Outlet SMS is opt-in and disabled until a contact and consent are recorded.
CREATE TABLE IF NOT EXISTS shared.outlet_notification_preferences (
    outlet_id TEXT PRIMARY KEY REFERENCES shared.outlets(id) ON DELETE CASCADE,
    phone_e164 TEXT NOT NULL CHECK (phone_e164 ~ '^[+][1-9][0-9]{7,14}$'),
    consent_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    deferrals_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    major_delays_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    locale TEXT NOT NULL DEFAULT 'en' CHECK (locale IN ('en','si','ta')),
    consented_at TIMESTAMPTZ,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_by TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (NOT consent_enabled OR consented_at IS NOT NULL)
);

-- Store event snapshots and dedupe keys before attempting delivery. A provider
-- timeout remains UNKNOWN and is never retried automatically: Twilio's create
-- endpoint does not document an idempotency key.
CREATE TABLE IF NOT EXISTS shared.notification_outbox (
    id BIGSERIAL PRIMARY KEY,
    event_key TEXT NOT NULL UNIQUE,
    outlet_id TEXT NOT NULL REFERENCES shared.outlets(id),
    event_type TEXT NOT NULL CHECK (event_type IN ('DEFERRAL','MAJOR_DELAY')),
    phone_e164 TEXT NOT NULL,
    body TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','SENDING','QUEUED','SENT','DELIVERED','FAILED','UNKNOWN','SUPPRESSED')),
    provider_message_id TEXT,
    error_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    payload_expires_at TIMESTAMPTZ NOT NULL DEFAULT now() + interval '7 days'
);
CREATE INDEX IF NOT EXISTS notification_outbox_pending_idx ON shared.notification_outbox(created_at) WHERE status='PENDING';
