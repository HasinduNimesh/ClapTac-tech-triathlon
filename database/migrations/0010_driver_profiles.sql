CREATE TABLE IF NOT EXISTS shared.driver_profiles (
    user_id TEXT PRIMARY KEY REFERENCES shared.users (id),
    vehicle_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
