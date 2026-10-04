-- Agent knowledge store (new schema, owned by insights-service).
-- Agents never connect to PostgreSQL: they read and write these tables through
-- insights-service with the signed-in person's token.
--
--   observations  raw facts, append-only (what happened, in numbers)
--   insights      patterns found in observations, with the evidence kept
--   suggestions   what an agent showed a person, and suggestion_responses
--                 for the answer
--
-- The habit helper (A3) and workflow builder (A4) keep their patterns,
-- automations and runs in shared.* (open PR #34); they are not duplicated here.

CREATE SCHEMA IF NOT EXISTS ai;
COMMENT ON SCHEMA ai IS 'Owned by insights-service (agent observations, insights, suggestions)';

CREATE TABLE IF NOT EXISTS ai.agents (
    code TEXT PRIMARY KEY CHECK (code ~ '^A[0-9]{1,2}$'),
    name TEXT NOT NULL,
    description TEXT NOT NULL,
    audience TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    version TEXT NOT NULL DEFAULT '1'
);

CREATE TABLE IF NOT EXISTS ai.observations (
    id BIGSERIAL PRIMARY KEY,
    subject_type TEXT NOT NULL CHECK (subject_type IN ('outlet', 'product', 'outlet_product', 'user', 'vehicle')),
    subject_id TEXT NOT NULL,
    metric TEXT NOT NULL CHECK (metric ~ '^[a-z][a-z0-9_]{2,60}$'),
    value NUMERIC(16,4) NOT NULL,
    unit TEXT NOT NULL,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL,
    source TEXT NOT NULL,
    raw JSONB NOT NULL DEFAULT '{}'::jsonb,
    simulated BOOLEAN NOT NULL DEFAULT false,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (period_end >= period_start),
    UNIQUE (subject_type, subject_id, metric, period_start, period_end, source)
);
CREATE INDEX IF NOT EXISTS observations_subject_idx ON ai.observations (subject_type, subject_id, metric, period_start DESC);

CREATE OR REPLACE FUNCTION ai.reject_observation_change() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'ai.observations is append-only; record a new observation instead';
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS observations_append_only ON ai.observations;
CREATE TRIGGER observations_append_only BEFORE UPDATE ON ai.observations
    FOR EACH ROW EXECUTE FUNCTION ai.reject_observation_change();

CREATE TABLE IF NOT EXISTS ai.insights (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_code TEXT NOT NULL REFERENCES ai.agents (code),
    kind TEXT NOT NULL CHECK (kind IN ('WASTE_RISK', 'STOCKOUT_RISK', 'TREND_UP', 'TREND_DOWN', 'REPEAT_ORDER', 'SHORTFALL_PATTERN')),
    subject_type TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    outlet_id TEXT,
    product_id TEXT,
    summary TEXT NOT NULL CHECK (length(summary) BETWEEN 1 AND 300),
    score NUMERIC(5,4) NOT NULL CHECK (score BETWEEN 0 AND 1),
    evidence JSONB NOT NULL,
    observation_ids BIGINT[] NOT NULL DEFAULT '{}',
    valid_from DATE NOT NULL,
    valid_until DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (valid_until >= valid_from),
    UNIQUE (kind, subject_type, subject_id, valid_from)
);
CREATE INDEX IF NOT EXISTS insights_active_idx ON ai.insights (outlet_id, product_id, valid_until DESC);

CREATE TABLE IF NOT EXISTS ai.suggestions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_code TEXT NOT NULL REFERENCES ai.agents (code),
    user_id TEXT NOT NULL,
    outlet_id TEXT,
    insight_id UUID REFERENCES ai.insights (id) ON DELETE SET NULL,
    context TEXT NOT NULL CHECK (context IN ('order_draft', 'dashboard', 'popup', 'notification')),
    message TEXT NOT NULL CHECK (length(message) BETWEEN 1 AND 500),
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    shown_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS suggestions_user_idx ON ai.suggestions (user_id, shown_at DESC);

CREATE TABLE IF NOT EXISTS ai.suggestion_responses (
    suggestion_id UUID PRIMARY KEY REFERENCES ai.suggestions (id) ON DELETE CASCADE,
    answer TEXT NOT NULL CHECK (answer IN ('yes', 'no', 'dont_ask_again')),
    answered_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Per-person switch for each agent (agent rule 5: easy to say no).
CREATE TABLE IF NOT EXISTS ai.agent_preferences (
    user_id TEXT NOT NULL,
    agent_code TEXT NOT NULL REFERENCES ai.agents (code),
    enabled BOOLEAN NOT NULL DEFAULT true,
    muted_kinds TEXT[] NOT NULL DEFAULT '{}',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, agent_code)
);

-- One row per agent call, for audit and evaluation. Input text is stored only as a hash.
CREATE TABLE IF NOT EXISTS ai.agent_runs (
    id BIGSERIAL PRIMARY KEY,
    agent_code TEXT NOT NULL REFERENCES ai.agents (code),
    user_id TEXT NOT NULL,
    input_hash TEXT NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
    tools_used TEXT[] NOT NULL DEFAULT '{}',
    outcome TEXT NOT NULL CHECK (outcome IN ('ok', 'asked', 'denied', 'invalid', 'unavailable', 'error')),
    latency_ms INTEGER NOT NULL CHECK (latency_ms >= 0),
    correlation_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_runs_agent_time_idx ON ai.agent_runs (agent_code, created_at DESC);
