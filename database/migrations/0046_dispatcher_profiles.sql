-- Which depot a dispatcher works from. A dispatcher with no row, or a row with no depot, covers every
-- depot (head office). The depot is a default view, not a permission: dispatchers can still switch to
-- another depot or to all depots, and no service restricts a dispatcher by it.
CREATE TABLE IF NOT EXISTS shared.dispatcher_profiles (
    user_id TEXT PRIMARY KEY REFERENCES shared.users (id),
    depot TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
