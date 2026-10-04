-- Small-change amendment to LO-4/LD-3: the store can write down the temperature of chilled
-- goods when it receives them, as the loader already does at loading. Optional; NULL means no
-- reading was taken. Kept on the receipt so the order timeline shows both readings.
ALTER TABLE orders.receipts
    ADD COLUMN IF NOT EXISTS received_temperature_c NUMERIC(5,2);

ALTER TABLE orders.receipts DROP CONSTRAINT IF EXISTS receipts_received_temperature_check;
ALTER TABLE orders.receipts ADD CONSTRAINT receipts_received_temperature_check
    CHECK (received_temperature_c IS NULL OR received_temperature_c BETWEEN -40 AND 60);
