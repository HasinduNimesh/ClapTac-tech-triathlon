ALTER TABLE shared.outlets
    ADD COLUMN IF NOT EXISTS access_instructions TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS access_instructions_updated_by TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS access_instructions_updated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS access_instructions_confirmed_by TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS access_instructions_confirmed_at TIMESTAMPTZ;

ALTER TABLE delivery.stops
    ADD COLUMN IF NOT EXISTS access_instructions TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS access_instructions_updated_at TIMESTAMPTZ;
