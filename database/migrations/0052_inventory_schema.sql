-- Store stock, sales and waste (new schema, owned by inventory-service).
-- stock_movements is the append-only ledger; stock_levels and the remaining
-- quantities on stock_batches are kept in step with it. Quantities here are
-- eaches (single sellable items). Rows produced by the customer simulation
-- agent carry simulated = true and a simulation_run_id so they can be
-- excluded from real reporting or removed as a whole run.

CREATE SCHEMA IF NOT EXISTS inventory;
COMMENT ON SCHEMA inventory IS 'Owned by inventory-service (store stock, sales, waste)';

CREATE TABLE IF NOT EXISTS inventory.simulation_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code TEXT NOT NULL UNIQUE,
    scenario TEXT NOT NULL,
    seed INTEGER NOT NULL,
    period_start DATE NOT NULL,
    period_end DATE NOT NULL CHECK (period_end >= period_start),
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS inventory.stock_levels (
    outlet_id TEXT NOT NULL,
    product_id TEXT NOT NULL,
    on_hand_each INTEGER NOT NULL CHECK (on_hand_each >= 0),
    last_movement_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (outlet_id, product_id)
);

-- A received lot. Fresh lots carry an expiry date so waste can be traced.
CREATE TABLE IF NOT EXISTS inventory.stock_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    outlet_id TEXT NOT NULL,
    product_id TEXT NOT NULL,
    source_type TEXT NOT NULL CHECK (source_type IN ('receipt', 'opening_balance', 'transfer', 'adjustment')),
    source_ref TEXT NOT NULL DEFAULT '',
    receipt_line_id UUID,
    received_each INTEGER NOT NULL CHECK (received_each > 0),
    remaining_each INTEGER NOT NULL CHECK (remaining_each >= 0),
    received_at TIMESTAMPTZ NOT NULL,
    expires_on DATE,
    simulated BOOLEAN NOT NULL DEFAULT false,
    simulation_run_id UUID REFERENCES inventory.simulation_runs (id) ON DELETE CASCADE,
    CHECK (remaining_each <= received_each)
);
CREATE INDEX IF NOT EXISTS stock_batches_open_idx ON inventory.stock_batches (outlet_id, product_id, expires_on) WHERE remaining_each > 0;

CREATE TABLE IF NOT EXISTS inventory.sales (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    outlet_id TEXT NOT NULL,
    business_date DATE NOT NULL,
    sold_at TIMESTAMPTZ NOT NULL,
    channel TEXT NOT NULL CHECK (channel IN ('till', 'simulated')),
    receipt_no TEXT NOT NULL,
    total_amount NUMERIC(14,2) NOT NULL DEFAULT 0 CHECK (total_amount >= 0),
    currency TEXT NOT NULL DEFAULT 'LKR',
    simulated BOOLEAN NOT NULL DEFAULT false,
    simulation_run_id UUID REFERENCES inventory.simulation_runs (id) ON DELETE CASCADE,
    UNIQUE (outlet_id, receipt_no),
    CHECK (simulated = (channel = 'simulated'))
);
CREATE INDEX IF NOT EXISTS sales_outlet_date_idx ON inventory.sales (outlet_id, business_date);

CREATE TABLE IF NOT EXISTS inventory.sale_lines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sale_id UUID NOT NULL REFERENCES inventory.sales (id) ON DELETE CASCADE,
    product_id TEXT NOT NULL,
    quantity_each INTEGER NOT NULL CHECK (quantity_each > 0),
    unit_price NUMERIC(12,2) NOT NULL CHECK (unit_price >= 0),
    line_amount NUMERIC(14,2) NOT NULL CHECK (line_amount >= 0)
);
CREATE INDEX IF NOT EXISTS sale_lines_sale_idx ON inventory.sale_lines (sale_id);
CREATE INDEX IF NOT EXISTS sale_lines_product_idx ON inventory.sale_lines (product_id);

-- Append-only ledger. quantity_each is signed: positive adds stock, negative removes it.
CREATE TABLE IF NOT EXISTS inventory.stock_movements (
    id BIGSERIAL PRIMARY KEY,
    outlet_id TEXT NOT NULL,
    product_id TEXT NOT NULL,
    batch_id UUID REFERENCES inventory.stock_batches (id) ON DELETE CASCADE,
    movement_type TEXT NOT NULL CHECK (movement_type IN ('RECEIVED', 'SOLD', 'WASTED', 'EXPIRED', 'DAMAGED', 'RETURNED', 'ADJUSTED')),
    quantity_each INTEGER NOT NULL CHECK (quantity_each <> 0),
    reason TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT '' CHECK (source_type IN ('', 'receipt', 'sale', 'stock_count', 'opening_balance', 'write_off', 'delivery_return')),
    source_ref TEXT NOT NULL DEFAULT '',
    sale_line_id UUID REFERENCES inventory.sale_lines (id) ON DELETE CASCADE,
    occurred_at TIMESTAMPTZ NOT NULL,
    recorded_by TEXT NOT NULL,
    simulated BOOLEAN NOT NULL DEFAULT false,
    simulation_run_id UUID REFERENCES inventory.simulation_runs (id) ON DELETE CASCADE,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (movement_type = 'RECEIVED' AND quantity_each > 0) OR
        (movement_type IN ('SOLD', 'WASTED', 'EXPIRED', 'DAMAGED') AND quantity_each < 0) OR
        movement_type IN ('RETURNED', 'ADJUSTED')
    ),
    CHECK (movement_type <> 'SOLD' OR sale_line_id IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS stock_movements_item_time_idx ON inventory.stock_movements (outlet_id, product_id, occurred_at);
CREATE INDEX IF NOT EXISTS stock_movements_type_time_idx ON inventory.stock_movements (movement_type, occurred_at);

CREATE OR REPLACE FUNCTION inventory.reject_movement_change() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'inventory.stock_movements is append-only; record a correcting movement instead';
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS stock_movements_append_only ON inventory.stock_movements;
CREATE TRIGGER stock_movements_append_only BEFORE UPDATE ON inventory.stock_movements
    FOR EACH ROW EXECUTE FUNCTION inventory.reject_movement_change();

-- Physical counts. A difference from the system quantity is posted as an ADJUSTED movement.
CREATE TABLE IF NOT EXISTS inventory.stock_counts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    outlet_id TEXT NOT NULL,
    product_id TEXT NOT NULL,
    counted_each INTEGER NOT NULL CHECK (counted_each >= 0),
    system_each INTEGER NOT NULL CHECK (system_each >= 0),
    variance_each INTEGER GENERATED ALWAYS AS (counted_each - system_each) STORED,
    counted_at TIMESTAMPTZ NOT NULL,
    counted_by TEXT NOT NULL,
    simulated BOOLEAN NOT NULL DEFAULT false,
    simulation_run_id UUID REFERENCES inventory.simulation_runs (id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS stock_counts_item_idx ON inventory.stock_counts (outlet_id, product_id, counted_at DESC);
