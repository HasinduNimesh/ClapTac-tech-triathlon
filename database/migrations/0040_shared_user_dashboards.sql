-- A2: dashboards a person saves from "Create new dashboard". Owner-scoped:
-- the spec holds only a name, card ids from the fixed store-manager card list
-- and a goods filter (no outlet or user ids); the web app computes every card
-- from the owner's own orders, deliveries and receipts.
CREATE TABLE IF NOT EXISTS shared.user_dashboards (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id TEXT NOT NULL REFERENCES shared.users (id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 60),
    spec JSONB NOT NULL CHECK (jsonb_typeof(spec -> 'cards') = 'array'),
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS user_dashboards_owner_idx ON shared.user_dashboards (owner_user_id, created_at);
