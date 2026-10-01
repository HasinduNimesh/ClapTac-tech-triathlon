CREATE TABLE IF NOT EXISTS planning.plan_publications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id UUID NOT NULL REFERENCES planning.plans(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version > 0),
    content_hash TEXT NOT NULL,
    published_by TEXT NOT NULL,
    published_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (plan_id, version)
);

ALTER TABLE planning.plans
    ADD COLUMN IF NOT EXISTS current_version INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;

CREATE TABLE IF NOT EXISTS planning.plan_acknowledgements (
    plan_id UUID NOT NULL REFERENCES planning.plans(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version > 0),
    actor_id TEXT NOT NULL,
    actor_role TEXT NOT NULL,
    acknowledged_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (plan_id, version, actor_id, actor_role),
    FOREIGN KEY (plan_id, version) REFERENCES planning.plan_publications(plan_id, version) ON DELETE CASCADE
);
