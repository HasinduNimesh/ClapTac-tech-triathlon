-- Preserve the order quantity on the immutable delivery snapshot. Historical
-- stops without a matching loading snapshot remain unknown rather than 0.
ALTER TABLE delivery.stops
    ADD COLUMN expected_units INTEGER CHECK (expected_units > 0),
    ADD COLUMN unit_label TEXT NOT NULL DEFAULT 'units';

UPDATE delivery.stops AS st
SET expected_units = ol.expected_units
FROM delivery.runs AS r
JOIN loading.sessions AS ls ON ls.trip_id = r.trip_id
JOIN loading.order_loads AS ol ON ol.session_id = ls.id
WHERE st.run_id = r.id
  AND st.allocation_id = ol.allocation_id
  AND st.order_id = ol.order_id
  AND ol.expected_units > 0;
