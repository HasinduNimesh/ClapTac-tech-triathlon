-- Item lines for orders and receipts (owned by order-service).
-- orders.orders keeps its totals (order_units = packs, weight, volume); when an
-- order has lines, the totals are the sum of its lines. Product names and pack
-- sizes are copied onto the line so history stays readable after catalog changes.

CREATE TABLE IF NOT EXISTS orders.order_lines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders.orders (id) ON DELETE CASCADE,
    line_no INTEGER NOT NULL CHECK (line_no BETWEEN 1 AND 200),
    product_id TEXT NOT NULL,
    product_name TEXT NOT NULL,
    pack_name TEXT NOT NULL,
    units_per_pack INTEGER NOT NULL CHECK (units_per_pack > 0),
    pack_qty INTEGER NOT NULL CHECK (pack_qty BETWEEN 1 AND 999),
    weight_kg NUMERIC(10,3) NOT NULL CHECK (weight_kg > 0),
    volume_m3 NUMERIC(10,4) NOT NULL CHECK (volume_m3 > 0),
    source TEXT NOT NULL DEFAULT 'form' CHECK (source IN ('form', 'text_helper', 'habit_helper', 'automation', 'import')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (order_id, line_no)
);
CREATE INDEX IF NOT EXISTS order_lines_product_idx ON orders.order_lines (product_id, created_at DESC);

-- What arrived per line, confirmed by the store. Totals stay on orders.receipts.
CREATE TABLE IF NOT EXISTS orders.receipt_lines (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    receipt_id UUID NOT NULL REFERENCES orders.receipts (id) ON DELETE CASCADE,
    order_line_id UUID NOT NULL REFERENCES orders.order_lines (id),
    expected_packs INTEGER NOT NULL CHECK (expected_packs >= 0),
    received_packs INTEGER NOT NULL CHECK (received_packs >= 0),
    short_packs INTEGER NOT NULL DEFAULT 0 CHECK (short_packs >= 0),
    damaged_packs INTEGER NOT NULL DEFAULT 0 CHECK (damaged_packs >= 0),
    note TEXT NOT NULL DEFAULT '' CHECK (length(note) <= 500),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (receipt_id, order_line_id),
    CHECK (short_packs <= expected_packs),
    CHECK (damaged_packs <= received_packs)
);
