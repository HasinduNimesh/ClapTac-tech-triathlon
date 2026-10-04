-- Make the append-only ledgers really append-only.
--
-- 0052 and 0053 describe inventory.stock_movements and ai.observations as
-- append-only but only blocked UPDATE; DELETE and TRUNCATE were still
-- allowed, and several foreign keys used ON DELETE CASCADE so deleting a
-- parent (a batch, a sale line, a simulation run) silently removed ledger rows.
--
-- This migration
--   1. blocks DELETE and TRUNCATE on both tables;
--   2. turns the cascading foreign keys that reach the ledger (and the
--      simulation-run links on the other inventory tables) into RESTRICT;
--   3. adds the one controlled exception: inventory.delete_simulation_run()
--      removes every row of one run that is flagged as simulated, and nothing
--      else. Rows with simulated = false can never be deleted.
--
-- It is safe to apply once on an environment that already ran 0050-0053 and to
-- re-run (every step checks the current state first).

BEGIN;

-- ---------------------------------------------------------------------------
-- Which runs may be removed, and which observations belong to a run.
-- ---------------------------------------------------------------------------

-- Every existing run came from the customer simulation, hence the default.
-- A run imported from real data would be recorded with simulated = false and
-- delete_simulation_run() refuses it.
ALTER TABLE inventory.simulation_runs ADD COLUMN IF NOT EXISTS simulated BOOLEAN NOT NULL DEFAULT true;

-- ai.observations only had a simulated flag. The run id is a plain id (no
-- foreign key) because ai and inventory are separate schemas.
ALTER TABLE ai.observations ADD COLUMN IF NOT EXISTS simulation_run_id UUID;
CREATE INDEX IF NOT EXISTS observations_simulation_run_idx ON ai.observations (simulation_run_id) WHERE simulation_run_id IS NOT NULL;

-- Tag the observations the seed built from the baseline run. The append-only
-- update trigger is switched off for this one backfill and back on right after,
-- inside this transaction.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM ai.observations o WHERE o.simulated AND o.simulation_run_id IS NULL AND o.source = 'inventory_ledger')
       AND EXISTS (SELECT 1 FROM inventory.simulation_runs WHERE code = 'seed-baseline') THEN
        ALTER TABLE ai.observations DISABLE TRIGGER observations_append_only;
        UPDATE ai.observations o
        SET simulation_run_id = (SELECT id FROM inventory.simulation_runs WHERE code = 'seed-baseline')
        WHERE o.simulated AND o.simulation_run_id IS NULL AND o.source = 'inventory_ledger';
        ALTER TABLE ai.observations ENABLE TRIGGER observations_append_only;
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'observations_run_only_if_simulated' AND conrelid = 'ai.observations'::regclass) THEN
        ALTER TABLE ai.observations ADD CONSTRAINT observations_run_only_if_simulated
            CHECK (simulation_run_id IS NULL OR simulated);
    END IF;
END $$;

-- Indexes for the foreign keys that point at the ledger's parents and for
-- the run-scoped delete: without them every parent delete scans the whole ledger.
CREATE INDEX IF NOT EXISTS stock_movements_batch_idx ON inventory.stock_movements (batch_id) WHERE batch_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS stock_movements_sale_line_idx ON inventory.stock_movements (sale_line_id) WHERE sale_line_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS stock_movements_run_idx ON inventory.stock_movements (simulation_run_id) WHERE simulation_run_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS stock_batches_run_idx ON inventory.stock_batches (simulation_run_id) WHERE simulation_run_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS sales_run_idx ON inventory.sales (simulation_run_id) WHERE simulation_run_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS stock_counts_run_idx ON inventory.stock_counts (simulation_run_id) WHERE simulation_run_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- Block DELETE and TRUNCATE. The only way through is the transaction-local
-- setting waypoint.allow_simulated_delete = 'on' together with
-- waypoint.simulated_delete_run = <run id>, and then only for rows that are
-- simulated and belong to exactly that run. Set by inventory.delete_simulation_run().
-- ---------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION inventory.guard_movement_delete() RETURNS trigger AS $$
DECLARE
    allowed_run text := current_setting('waypoint.simulated_delete_run', true);
BEGIN
    IF current_setting('waypoint.allow_simulated_delete', true) = 'on'
       AND OLD.simulated
       AND OLD.simulation_run_id IS NOT NULL
       AND allowed_run IS NOT NULL AND allowed_run <> ''
       AND OLD.simulation_run_id::text = allowed_run THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'inventory.stock_movements is append-only; record a correcting movement instead (simulated runs are removed with inventory.delete_simulation_run)'
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION ai.guard_observation_delete() RETURNS trigger AS $$
DECLARE
    allowed_run text := current_setting('waypoint.simulated_delete_run', true);
BEGIN
    IF current_setting('waypoint.allow_simulated_delete', true) = 'on'
       AND OLD.simulated
       AND OLD.simulation_run_id IS NOT NULL
       AND allowed_run IS NOT NULL AND allowed_run <> ''
       AND OLD.simulation_run_id::text = allowed_run THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'ai.observations is append-only; record a new observation instead (simulated runs are removed with inventory.delete_simulation_run)'
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

-- TRUNCATE has no per-row hook and no controlled exception: remove a run with
-- inventory.delete_simulation_run instead.
CREATE OR REPLACE FUNCTION inventory.reject_movement_truncate() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'inventory.stock_movements is append-only and cannot be truncated'
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION ai.reject_observation_truncate() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'ai.observations is append-only and cannot be truncated'
        USING ERRCODE = 'restrict_violation';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS stock_movements_no_delete ON inventory.stock_movements;
CREATE TRIGGER stock_movements_no_delete BEFORE DELETE ON inventory.stock_movements
    FOR EACH ROW EXECUTE FUNCTION inventory.guard_movement_delete();
DROP TRIGGER IF EXISTS stock_movements_no_truncate ON inventory.stock_movements;
CREATE TRIGGER stock_movements_no_truncate BEFORE TRUNCATE ON inventory.stock_movements
    FOR EACH STATEMENT EXECUTE FUNCTION inventory.reject_movement_truncate();

DROP TRIGGER IF EXISTS observations_no_delete ON ai.observations;
CREATE TRIGGER observations_no_delete BEFORE DELETE ON ai.observations
    FOR EACH ROW EXECUTE FUNCTION ai.guard_observation_delete();
DROP TRIGGER IF EXISTS observations_no_truncate ON ai.observations;
CREATE TRIGGER observations_no_truncate BEFORE TRUNCATE ON ai.observations
    FOR EACH STATEMENT EXECUTE FUNCTION ai.reject_observation_truncate();

-- ---------------------------------------------------------------------------
-- Foreign keys: nothing may cascade into the ledger. Re-created under the same
-- names with ON DELETE RESTRICT. Covers stock_movements (batch_id,
-- sale_line_id, simulation_run_id) and the simulation_run_id links on
-- stock_batches, sales and stock_counts, so a run can only be removed through
-- inventory.delete_simulation_run. sale_lines -> sales stays CASCADE: deleting
-- a sale whose lines have ledger movements is refused by the movement FK.
-- ---------------------------------------------------------------------------

DO $$
DECLARE
    r record;
BEGIN
    FOR r IN
        SELECT c.conrelid::regclass AS tbl, c.conname, pg_get_constraintdef(c.oid) AS def
        FROM pg_constraint c
        WHERE c.contype = 'f'
          AND c.confdeltype = 'c'
          AND c.conrelid IN ('inventory.stock_movements'::regclass, 'inventory.stock_batches'::regclass,
                             'inventory.sales'::regclass, 'inventory.stock_counts'::regclass)
    LOOP
        EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', r.tbl, r.conname);
        EXECUTE format('ALTER TABLE %s ADD CONSTRAINT %I %s', r.tbl, r.conname,
                       replace(r.def, 'ON DELETE CASCADE', 'ON DELETE RESTRICT'));
    END LOOP;
END $$;

-- ---------------------------------------------------------------------------
-- The controlled path. Runs as the caller (not SECURITY DEFINER), so the caller
-- needs DELETE on the tables anyway. It refuses unknown runs and runs not
-- flagged simulated, deletes only rows with simulated = true tagged to that run,
-- and switches the exception off again before returning.
--
-- Also removed: ai.insights that cite one of the run's observations (their
-- evidence would otherwise point at rows that no longer exist). stock_levels is
-- put back in step with the ledger that remains for the affected items.
-- Returns the number of rows removed per table.
-- ---------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION inventory.delete_simulation_run(p_run_id UUID) RETURNS JSONB AS $$
DECLARE
    v_code TEXT;
    v_simulated BOOLEAN;
    v_pairs TEXT[];
    n_movements BIGINT;
    n_observations BIGINT;
    n_insights BIGINT;
    n_sale_lines BIGINT;
    n_sales BIGINT;
    n_counts BIGINT;
    n_batches BIGINT;
BEGIN
    SELECT code, simulated INTO v_code, v_simulated FROM inventory.simulation_runs WHERE id = p_run_id FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'simulation run % does not exist', p_run_id USING ERRCODE = 'no_data_found';
    END IF;
    IF NOT v_simulated THEN
        RAISE EXCEPTION 'run % (%) is not flagged simulated; real data cannot be deleted', v_code, p_run_id
            USING ERRCODE = 'restrict_violation';
    END IF;

    SELECT array_agg(DISTINCT m.outlet_id || '|' || m.product_id) INTO v_pairs
    FROM inventory.stock_movements m WHERE m.simulated AND m.simulation_run_id = p_run_id;

    PERFORM set_config('waypoint.allow_simulated_delete', 'on', true);
    PERFORM set_config('waypoint.simulated_delete_run', p_run_id::text, true);

    DELETE FROM inventory.stock_movements m WHERE m.simulated AND m.simulation_run_id = p_run_id;
    GET DIAGNOSTICS n_movements = ROW_COUNT;

    DELETE FROM ai.insights i
    WHERE EXISTS (SELECT 1 FROM ai.observations o
                  WHERE o.id = ANY (i.observation_ids) AND o.simulated AND o.simulation_run_id = p_run_id);
    GET DIAGNOSTICS n_insights = ROW_COUNT;

    DELETE FROM ai.observations o WHERE o.simulated AND o.simulation_run_id = p_run_id;
    GET DIAGNOSTICS n_observations = ROW_COUNT;

    DELETE FROM inventory.sale_lines l
    WHERE l.sale_id IN (SELECT s.id FROM inventory.sales s WHERE s.simulated AND s.simulation_run_id = p_run_id);
    GET DIAGNOSTICS n_sale_lines = ROW_COUNT;

    DELETE FROM inventory.sales s WHERE s.simulated AND s.simulation_run_id = p_run_id;
    GET DIAGNOSTICS n_sales = ROW_COUNT;

    DELETE FROM inventory.stock_counts c WHERE c.simulated AND c.simulation_run_id = p_run_id;
    GET DIAGNOSTICS n_counts = ROW_COUNT;

    DELETE FROM inventory.stock_batches b WHERE b.simulated AND b.simulation_run_id = p_run_id;
    GET DIAGNOSTICS n_batches = ROW_COUNT;

    -- Items that have no ledger left lose their level; the rest are recomputed.
    IF v_pairs IS NOT NULL THEN
        DELETE FROM inventory.stock_levels sl
        USING unnest(v_pairs) AS k
        WHERE sl.outlet_id = split_part(k, '|', 1) AND sl.product_id = split_part(k, '|', 2)
          AND NOT EXISTS (SELECT 1 FROM inventory.stock_movements m WHERE m.outlet_id = sl.outlet_id AND m.product_id = sl.product_id);

        UPDATE inventory.stock_levels sl
        SET on_hand_each = GREATEST(t.qty, 0), last_movement_at = t.last_at, updated_at = now()
        FROM (SELECT split_part(k, '|', 1) AS outlet_id, split_part(k, '|', 2) AS product_id FROM unnest(v_pairs) AS k) p
        CROSS JOIN LATERAL (
            SELECT sum(m.quantity_each) AS qty, max(m.occurred_at) AS last_at
            FROM inventory.stock_movements m
            WHERE m.outlet_id = p.outlet_id AND m.product_id = p.product_id
        ) t
        WHERE sl.outlet_id = p.outlet_id AND sl.product_id = p.product_id AND t.qty IS NOT NULL;
    END IF;

    PERFORM set_config('waypoint.allow_simulated_delete', 'off', true);
    PERFORM set_config('waypoint.simulated_delete_run', '', true);

    -- The run row itself goes last; RESTRICT keeps it if anything still points at it.
    DELETE FROM inventory.simulation_runs WHERE id = p_run_id;

    RETURN jsonb_build_object('run', v_code, 'stock_movements', n_movements, 'observations', n_observations,
                              'insights', n_insights, 'sale_lines', n_sale_lines, 'sales', n_sales,
                              'stock_counts', n_counts, 'stock_batches', n_batches);
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION inventory.delete_simulation_run(UUID) IS
    'Only way to delete append-only rows: removes one run flagged simulated and the simulated rows tagged to it; refuses anything else';

COMMIT;
