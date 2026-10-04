-- Dispatcher decision on a loader shortfall (Figma "Dispatcher / Load exception decision").
-- A trip cannot be marked ready while a shortfall has no decision, or is on hold.
ALTER TABLE loading.issues
    ADD COLUMN IF NOT EXISTS decision TEXT CHECK (decision IN ('PARTIAL_LOAD', 'HOLD', 'MOVE_TO_NEXT_RUN')),
    ADD COLUMN IF NOT EXISTS decision_note TEXT,
    ADD COLUMN IF NOT EXISTS decided_by TEXT,
    ADD COLUMN IF NOT EXISTS decided_at TIMESTAMPTZ;
