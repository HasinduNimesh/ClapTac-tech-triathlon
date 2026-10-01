CREATE SEQUENCE IF NOT EXISTS orders.order_ref_seq START 1;

CREATE TABLE IF NOT EXISTS orders.orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_ref TEXT NOT NULL UNIQUE,
    outlet_id TEXT NOT NULL,
    brand TEXT NOT NULL,
    requested_delivery_date DATE NOT NULL,
    order_units INTEGER NOT NULL CHECK (order_units > 0),
    order_weight_kg NUMERIC(12, 3) NOT NULL CHECK (order_weight_kg > 0),
    order_volume_m3 NUMERIC(12, 3) NOT NULL CHECK (order_volume_m3 > 0),
    temperature_requirement TEXT NOT NULL CHECK (temperature_requirement IN ('ambient', 'chilled')),
    status TEXT NOT NULL CHECK (status = 'confirmed'),
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS orders_outlet_idx ON orders.orders (outlet_id);
CREATE INDEX IF NOT EXISTS orders_date_idx ON orders.orders (requested_delivery_date);
