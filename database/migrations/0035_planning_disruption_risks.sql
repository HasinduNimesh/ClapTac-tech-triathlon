CREATE TABLE IF NOT EXISTS planning.disruption_risks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_date DATE NOT NULL,
    scope TEXT NOT NULL CHECK (scope IN ('DEPOT','DISTRICT','ROUTE')),
    scope_key TEXT NOT NULL CHECK (length(btrim(scope_key)) BETWEEN 1 AND 120),
    risk_type TEXT NOT NULL CHECK (risk_type IN ('HEAVY_RAIN','FLOODING','LANDSLIDE','ROAD_CLOSURE','ROAD_DAMAGE','OTHER')),
    severity TEXT NOT NULL CHECK (severity IN ('LOW','MEDIUM','HIGH')),
    summary TEXT NOT NULL CHECK (length(btrim(summary)) BETWEEN 1 AND 500),
    source TEXT NOT NULL CHECK (length(btrim(source)) BETWEEN 1 AND 120),
    source_reference TEXT NOT NULL DEFAULT '' CHECK (length(source_reference) <= 300),
    confidence NUMERIC(4,3) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS planning_disruption_risks_date_scope_idx
    ON planning.disruption_risks (delivery_date, scope, scope_key, created_at DESC);

CREATE TABLE IF NOT EXISTS planning.disruption_risk_overrides (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    risk_id UUID NOT NULL REFERENCES planning.disruption_risks(id),
    decision TEXT NOT NULL CHECK (decision IN ('ACKNOWLEDGED','OVERRIDE','DISMISSED')),
    severity_override TEXT CHECK (severity_override IS NULL OR severity_override IN ('LOW','MEDIUM','HIGH')),
    reason TEXT NOT NULL CHECK (length(btrim(reason)) BETWEEN 8 AND 500),
    actor_id TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK ((decision = 'OVERRIDE' AND severity_override IS NOT NULL) OR (decision <> 'OVERRIDE' AND severity_override IS NULL))
);

CREATE INDEX IF NOT EXISTS planning_disruption_risk_overrides_latest_idx
    ON planning.disruption_risk_overrides (risk_id, created_at DESC, id DESC);

CREATE OR REPLACE FUNCTION planning.reject_disruption_risk_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'disruption risk history is append-only';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS disruption_risks_append_only ON planning.disruption_risks;
CREATE TRIGGER disruption_risks_append_only
    BEFORE UPDATE OR DELETE ON planning.disruption_risks
    FOR EACH ROW EXECUTE FUNCTION planning.reject_disruption_risk_mutation();

DROP TRIGGER IF EXISTS disruption_risk_overrides_append_only ON planning.disruption_risk_overrides;
CREATE TRIGGER disruption_risk_overrides_append_only
    BEFORE UPDATE OR DELETE ON planning.disruption_risk_overrides
    FOR EACH ROW EXECUTE FUNCTION planning.reject_disruption_risk_mutation();
