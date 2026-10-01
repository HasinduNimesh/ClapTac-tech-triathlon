-- Forecast evaluation reads only successful outcomes in a bounded time window.
-- Keep this partial index small as the append-only stop history grows.
CREATE INDEX IF NOT EXISTS delivery_stops_service_time_eval_idx
    ON delivery.stops (outcome_at)
    WHERE outcome_at IS NOT NULL
      AND arrived_at IS NOT NULL
      AND outcome_code IN ('DELIVERED', 'PARTIAL');
