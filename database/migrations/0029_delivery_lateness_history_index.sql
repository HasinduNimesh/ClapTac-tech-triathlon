CREATE INDEX IF NOT EXISTS delivery_runs_depot_date_idx
    ON delivery.runs (depot, delivery_date);

CREATE INDEX IF NOT EXISTS delivery_stops_arrived_brand_run_idx
    ON delivery.stops (run_id, brand)
    WHERE arrived_at IS NOT NULL;
