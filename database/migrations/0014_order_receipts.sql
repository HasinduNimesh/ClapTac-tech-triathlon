CREATE TABLE IF NOT EXISTS orders.receipts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL UNIQUE REFERENCES orders.orders(id),
    delivery_run_id UUID NOT NULL,
    delivery_stop_id UUID NOT NULL,
    delivery_outcome TEXT NOT NULL CHECK (delivery_outcome IN ('DELIVERED', 'PARTIAL')),
    expected_units INTEGER NOT NULL CHECK (expected_units > 0),
    received_units INTEGER NOT NULL CHECK (received_units > 0 AND received_units <= expected_units),
    status TEXT NOT NULL CHECK (status IN ('confirmed', 'confirmed_with_issue')),
    confirmed_by TEXT NOT NULL,
    confirmed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    version INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS orders.receipt_issues (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    receipt_id UUID NOT NULL REFERENCES orders.receipts(id),
    issue_type TEXT NOT NULL CHECK (issue_type IN ('MISSING', 'DAMAGED', 'QUANTITY_MISMATCH', 'OTHER')),
    affected_units INTEGER NOT NULL CHECK (affected_units >= 0),
    note TEXT NOT NULL DEFAULT '',
    idempotency_key TEXT NOT NULL UNIQUE,
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS receipts_status_idx ON orders.receipts (status, confirmed_at DESC);
CREATE INDEX IF NOT EXISTS receipt_issues_created_idx ON orders.receipt_issues (created_at DESC);
