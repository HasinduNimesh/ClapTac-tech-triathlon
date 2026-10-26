-- Persist analyses, store preferences and the dispatcher publication receipt.
CREATE TABLE planning.recovery_cases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plan_id UUID NOT NULL REFERENCES planning.plans(id),
    snapshot TEXT NOT NULL,
    allocation_snapshot TEXT NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX recovery_cases_plan ON planning.recovery_cases(plan_id, created_at DESC);
CREATE TABLE planning.recovery_preferences (
    case_id UUID NOT NULL REFERENCES planning.recovery_cases(id),
    outlet_id TEXT NOT NULL,
    candidate_id TEXT NOT NULL,
    note TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY(case_id, outlet_id)
);
