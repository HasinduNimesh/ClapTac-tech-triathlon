CREATE TABLE IF NOT EXISTS shared.loader_profiles (
    user_id TEXT PRIMARY KEY REFERENCES shared.users (id),
    depot TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
