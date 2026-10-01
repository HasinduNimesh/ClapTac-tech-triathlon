CREATE TABLE IF NOT EXISTS shared.users (
    id TEXT PRIMARY KEY,
    identity_subject TEXT NOT NULL UNIQUE,
    role TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS shared.outlets (
    id TEXT PRIMARY KEY,
    brand TEXT NOT NULL,
    name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS shared.store_manager_profiles (
    user_id TEXT PRIMARY KEY REFERENCES shared.users (id),
    outlet_id TEXT NOT NULL REFERENCES shared.outlets (id)
);

CREATE TABLE IF NOT EXISTS shared.operating_calendar (
    date DATE PRIMARY KEY,
    is_operating BOOLEAN NOT NULL
);
