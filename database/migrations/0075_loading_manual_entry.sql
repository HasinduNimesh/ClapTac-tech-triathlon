-- LD-6: how a load line was confirmed. A loader who cannot scan a label may type
-- the order ID instead, but must say why. The method and the reason are kept on the
-- load line so the dispatcher can see which lines were not scanned. Lines loaded
-- before this migration (or by a client that sends nothing) keep NULL = unknown.

ALTER TABLE loading.order_loads
    ADD COLUMN IF NOT EXISTS entry_method TEXT,
    ADD COLUMN IF NOT EXISTS manual_reason TEXT,
    ADD COLUMN IF NOT EXISTS manual_note TEXT;

ALTER TABLE loading.order_loads DROP CONSTRAINT IF EXISTS order_loads_entry_method_check;
ALTER TABLE loading.order_loads ADD CONSTRAINT order_loads_entry_method_check
    CHECK (entry_method IS NULL OR entry_method IN ('SCAN', 'MANUAL'));

ALTER TABLE loading.order_loads DROP CONSTRAINT IF EXISTS order_loads_manual_reason_check;
ALTER TABLE loading.order_loads ADD CONSTRAINT order_loads_manual_reason_check
    CHECK (manual_reason IS NULL OR manual_reason IN ('DAMAGED_LABEL', 'UNREADABLE', 'NO_CAMERA', 'OTHER'));

ALTER TABLE loading.order_loads DROP CONSTRAINT IF EXISTS order_loads_manual_note_check;
ALTER TABLE loading.order_loads ADD CONSTRAINT order_loads_manual_note_check
    CHECK (manual_note IS NULL OR length(manual_note) <= 200);

-- A manual entry always carries its reason.
ALTER TABLE loading.order_loads DROP CONSTRAINT IF EXISTS order_loads_manual_requires_reason;
ALTER TABLE loading.order_loads ADD CONSTRAINT order_loads_manual_requires_reason
    CHECK (entry_method IS DISTINCT FROM 'MANUAL' OR manual_reason IS NOT NULL);
