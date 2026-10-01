ALTER TABLE orders.orders ADD COLUMN IF NOT EXISTS source_system TEXT NOT NULL DEFAULT '';
ALTER TABLE orders.orders ADD COLUMN IF NOT EXISTS external_order_id TEXT;
CREATE UNIQUE INDEX IF NOT EXISTS orders_external_import_id_uq
    ON orders.orders(source_system,external_order_id) WHERE external_order_id IS NOT NULL;
