-- FR-12: persist the allocator's explanation for each unallocated order so the
-- Dispatcher still sees the real reason (and the other limiting factors) after
-- the generate response is gone, instead of a generic placeholder.
CREATE TABLE IF NOT EXISTS planning.unallocated_reasons (
    plan_id UUID NOT NULL REFERENCES planning.plans (id) ON DELETE CASCADE,
    order_id TEXT NOT NULL,
    reason_code TEXT NOT NULL,
    details JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (plan_id, order_id)
);
