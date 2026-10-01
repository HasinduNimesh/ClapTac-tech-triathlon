-- FR-25: record who received the delivery alongside the proof itself.
ALTER TABLE delivery.proofs ADD COLUMN IF NOT EXISTS receiver_name TEXT;
