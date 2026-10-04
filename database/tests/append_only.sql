-- Proves that inventory.stock_movements and ai.observations are append-only
-- (migration 0054): UPDATE, DELETE and TRUNCATE are refused, deleting a parent
-- row cannot cascade into them, and inventory.delete_simulation_run() is the
-- only way to remove rows - simulated rows of one simulated run, nothing else.
--
-- Everything runs in one transaction that is rolled back, with test rows named
-- 'zt-*' / ZT-OUT, so it is safe on a database that already holds seed data.
--
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f database/tests/append_only.sql
--
-- Any failed check raises an exception and stops the script with a non-zero exit.

\set ON_ERROR_STOP on
\set QUIET on
\pset tuples_only on
\pset format unaligned
SET client_min_messages = notice;
BEGIN;

CREATE PROCEDURE pg_temp.expect_fail(label text, stmt text, want_state text DEFAULT '23001') AS $$
DECLARE
    failed boolean := false;
BEGIN
    BEGIN
        EXECUTE stmt;
    EXCEPTION WHEN OTHERS THEN
        IF SQLSTATE <> want_state THEN
            RAISE EXCEPTION 'FAIL %: expected SQLSTATE % but got % (%)', label, want_state, SQLSTATE, SQLERRM;
        END IF;
        failed := true;
        RAISE NOTICE 'PASS  % -> %', label, SQLERRM;
    END;
    IF NOT failed THEN
        RAISE EXCEPTION 'FAIL %: statement succeeded but must be refused', label;
    END IF;
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION pg_temp.expect_true(label text, ok boolean) RETURNS text AS $$
BEGIN
    IF ok IS DISTINCT FROM true THEN
        RAISE EXCEPTION 'FAIL %', label;
    END IF;
    RETURN 'PASS  ' || label;
END;
$$ LANGUAGE plpgsql;

-- ---------------------------------------------------------------------------
-- Fixtures: two simulated runs (A, B), one run recorded as real data (R) and
-- real (simulated = false) rows with no run at all.
-- ---------------------------------------------------------------------------
INSERT INTO inventory.simulation_runs (id, code, scenario, seed, period_start, period_end, created_by, simulated) VALUES
  ('00000000-0000-4000-8000-0000000000a1', 'zt-sim-a', 'test', 1, '2026-01-01', '2026-01-07', 'test', true),
  ('00000000-0000-4000-8000-0000000000b1', 'zt-sim-b', 'test', 1, '2026-01-01', '2026-01-07', 'test', true),
  ('00000000-0000-4000-8000-0000000000c1', 'zt-real-r', 'real till import', 1, '2026-01-01', '2026-01-07', 'test', false);

INSERT INTO inventory.stock_batches (id, outlet_id, product_id, source_type, received_each, remaining_each, received_at, simulated, simulation_run_id) VALUES
  ('10000000-0000-4000-8000-000000000001', 'ZT-OUT', 'FR-ZT-REAL', 'receipt', 10, 10, '2026-01-02', false, NULL),
  ('10000000-0000-4000-8000-0000000000a1', 'ZT-OUT', 'FR-ZT-SIM',  'receipt', 10, 6,  '2026-01-02', true,  '00000000-0000-4000-8000-0000000000a1'),
  ('10000000-0000-4000-8000-0000000000a2', 'ZT-OUT', 'FR-ZT-MIX',  'receipt', 10, 10, '2026-01-02', true,  '00000000-0000-4000-8000-0000000000a1'),
  ('10000000-0000-4000-8000-0000000000b1', 'ZT-OUT', 'FR-ZT-SIMB', 'receipt', 10, 10, '2026-01-02', true,  '00000000-0000-4000-8000-0000000000b1'),
  ('10000000-0000-4000-8000-0000000000c1', 'ZT-OUT', 'FR-ZT-RR',   'receipt', 10, 10, '2026-01-02', false, '00000000-0000-4000-8000-0000000000c1');

INSERT INTO inventory.sales (id, outlet_id, business_date, sold_at, channel, receipt_no, simulated, simulation_run_id) VALUES
  ('20000000-0000-4000-8000-0000000000a1', 'ZT-OUT', '2026-01-03', '2026-01-03 15:00+00', 'simulated', 'ZT-A-1', true, '00000000-0000-4000-8000-0000000000a1');
INSERT INTO inventory.sale_lines (id, sale_id, product_id, quantity_each, unit_price, line_amount) VALUES
  ('30000000-0000-4000-8000-0000000000a1', '20000000-0000-4000-8000-0000000000a1', 'FR-ZT-SIM', 4, 100, 400);

INSERT INTO inventory.stock_counts (outlet_id, product_id, counted_each, system_each, counted_at, counted_by, simulated, simulation_run_id) VALUES
  ('ZT-OUT', 'FR-ZT-SIM', 6, 6, '2026-01-04', 'test', true, '00000000-0000-4000-8000-0000000000a1');

-- Ledger. Ids are fixed so the checks below can refer to them.
INSERT INTO inventory.stock_movements (id, outlet_id, product_id, batch_id, movement_type, quantity_each, source_type, sale_line_id, occurred_at, recorded_by, simulated, simulation_run_id) VALUES
  (9000001, 'ZT-OUT', 'FR-ZT-REAL', '10000000-0000-4000-8000-000000000001', 'RECEIVED', 10, 'receipt', NULL, '2026-01-02', 'test', false, NULL),
  (9000002, 'ZT-OUT', 'FR-ZT-SIM',  '10000000-0000-4000-8000-0000000000a1', 'RECEIVED', 10, 'receipt', NULL, '2026-01-02', 'test', true,  '00000000-0000-4000-8000-0000000000a1'),
  (9000003, 'ZT-OUT', 'FR-ZT-SIM',  '10000000-0000-4000-8000-0000000000a1', 'SOLD',     -4, 'sale', '30000000-0000-4000-8000-0000000000a1', '2026-01-03', 'test', true, '00000000-0000-4000-8000-0000000000a1'),
  -- FR-ZT-MIX has a real movement and a simulated one: the simulated one goes, the item stays.
  (9000004, 'ZT-OUT', 'FR-ZT-MIX',  '10000000-0000-4000-8000-0000000000a2', 'RECEIVED', 10, 'receipt', NULL, '2026-01-02', 'test', true,  '00000000-0000-4000-8000-0000000000a1'),
  (9000005, 'ZT-OUT', 'FR-ZT-MIX',  NULL, 'RECEIVED', 5, 'receipt', NULL, '2026-01-05', 'test', false, NULL),
  (9000006, 'ZT-OUT', 'FR-ZT-SIMB', '10000000-0000-4000-8000-0000000000b1', 'RECEIVED', 10, 'receipt', NULL, '2026-01-02', 'test', true,  '00000000-0000-4000-8000-0000000000b1'),
  (9000007, 'ZT-OUT', 'FR-ZT-RR',   '10000000-0000-4000-8000-0000000000c1', 'RECEIVED', 10, 'receipt', NULL, '2026-01-02', 'test', false, '00000000-0000-4000-8000-0000000000c1');

INSERT INTO inventory.stock_levels (outlet_id, product_id, on_hand_each) VALUES
  ('ZT-OUT', 'FR-ZT-REAL', 10), ('ZT-OUT', 'FR-ZT-SIM', 6), ('ZT-OUT', 'FR-ZT-MIX', 99), ('ZT-OUT', 'FR-ZT-SIMB', 10), ('ZT-OUT', 'FR-ZT-RR', 10);

INSERT INTO ai.agents (code, name, description, audience) VALUES ('A5', 'Customer simulation', 'test', 'test')
ON CONFLICT (code) DO NOTHING;

INSERT INTO ai.observations (id, subject_type, subject_id, metric, value, unit, period_start, period_end, source, simulated, simulation_run_id) VALUES
  (9100001, 'outlet_product', 'ZT-OUT:FR-ZT-REAL', 'units_sold', 1, 'each', '2026-01-01', '2026-01-07', 'zt', false, NULL),
  (9100002, 'outlet_product', 'ZT-OUT:FR-ZT-SIM',  'units_sold', 4, 'each', '2026-01-01', '2026-01-07', 'zt', true, '00000000-0000-4000-8000-0000000000a1'),
  (9100003, 'outlet_product', 'ZT-OUT:FR-ZT-SIMB', 'units_sold', 2, 'each', '2026-01-01', '2026-01-07', 'zt', true, '00000000-0000-4000-8000-0000000000b1');

INSERT INTO ai.insights (id, agent_code, kind, subject_type, subject_id, outlet_id, product_id, summary, score, evidence, observation_ids, valid_from, valid_until) VALUES
  ('40000000-0000-4000-8000-0000000000a1', 'A5', 'TREND_UP', 'outlet_product', 'ZT-OUT:FR-ZT-SIM',  'ZT-OUT', 'FR-ZT-SIM',  'test a', 0.5, '{}', '{9100002}', '2026-01-01', '2026-01-07'),
  ('40000000-0000-4000-8000-0000000000b1', 'A5', 'TREND_UP', 'outlet_product', 'ZT-OUT:FR-ZT-SIMB', 'ZT-OUT', 'FR-ZT-SIMB', 'test b', 0.5, '{}', '{9100003}', '2026-01-01', '2026-01-07');

CREATE TEMP TABLE zt_before AS SELECT
  (SELECT count(*) FROM inventory.stock_movements WHERE id BETWEEN 9000001 AND 9000007) AS movements,
  (SELECT count(*) FROM ai.observations WHERE id BETWEEN 9100001 AND 9100003) AS observations;

-- ---------------------------------------------------------------------------
-- 1. UPDATE is refused (this was already true)
-- ---------------------------------------------------------------------------
CALL pg_temp.expect_fail('UPDATE real stock_movement', $$UPDATE inventory.stock_movements SET reason = 'x' WHERE id = 9000001$$, 'P0001');
CALL pg_temp.expect_fail('UPDATE simulated stock_movement', $$UPDATE inventory.stock_movements SET reason = 'x' WHERE id = 9000002$$, 'P0001');
CALL pg_temp.expect_fail('UPDATE real observation', $$UPDATE ai.observations SET value = 99 WHERE id = 9100001$$, 'P0001');
CALL pg_temp.expect_fail('UPDATE simulated observation', $$UPDATE ai.observations SET value = 99 WHERE id = 9100002$$, 'P0001');

-- ---------------------------------------------------------------------------
-- 2. Ordinary DELETE is refused, real and simulated alike
-- ---------------------------------------------------------------------------
CALL pg_temp.expect_fail('DELETE real stock_movement', $$DELETE FROM inventory.stock_movements WHERE id = 9000001$$);
CALL pg_temp.expect_fail('DELETE simulated stock_movement', $$DELETE FROM inventory.stock_movements WHERE id = 9000002$$);
CALL pg_temp.expect_fail('DELETE every stock_movement of a run', $$DELETE FROM inventory.stock_movements WHERE simulation_run_id = '00000000-0000-4000-8000-0000000000a1'$$);
CALL pg_temp.expect_fail('DELETE real observation', $$DELETE FROM ai.observations WHERE id = 9100001$$);
CALL pg_temp.expect_fail('DELETE simulated observation', $$DELETE FROM ai.observations WHERE id = 9100002$$);

-- ---------------------------------------------------------------------------
-- 3. TRUNCATE is refused, including TRUNCATE ... CASCADE from a parent table
-- ---------------------------------------------------------------------------
CALL pg_temp.expect_fail('TRUNCATE stock_movements', $$TRUNCATE inventory.stock_movements$$);
CALL pg_temp.expect_fail('TRUNCATE ai.observations', $$TRUNCATE ai.observations$$);
CALL pg_temp.expect_fail('TRUNCATE stock_batches CASCADE (reaches the ledger)', $$TRUNCATE inventory.stock_batches CASCADE$$);
CALL pg_temp.expect_fail('TRUNCATE sale_lines CASCADE (reaches the ledger)', $$TRUNCATE inventory.sale_lines CASCADE$$);

-- ---------------------------------------------------------------------------
-- 4. Deleting a parent row that used to cascade into the ledger now fails
-- ---------------------------------------------------------------------------
CALL pg_temp.expect_fail('DELETE stock_batch that has movements', $$DELETE FROM inventory.stock_batches WHERE id = '10000000-0000-4000-8000-000000000001'$$, '23503');
CALL pg_temp.expect_fail('DELETE sale_line that has a SOLD movement', $$DELETE FROM inventory.sale_lines WHERE id = '30000000-0000-4000-8000-0000000000a1'$$, '23503');
CALL pg_temp.expect_fail('DELETE sale (cascades to its lines, then the ledger)', $$DELETE FROM inventory.sales WHERE id = '20000000-0000-4000-8000-0000000000a1'$$, '23503');
CALL pg_temp.expect_fail('DELETE simulation_run directly', $$DELETE FROM inventory.simulation_runs WHERE id = '00000000-0000-4000-8000-0000000000a1'$$, '23503');
CALL pg_temp.expect_fail('DELETE real-data run directly', $$DELETE FROM inventory.simulation_runs WHERE id = '00000000-0000-4000-8000-0000000000c1'$$, '23503');

SELECT pg_temp.expect_true('no ON DELETE CASCADE foreign key is left on stock_movements, stock_batches, sales or stock_counts',
  NOT EXISTS (SELECT 1 FROM pg_constraint c
              WHERE c.contype = 'f' AND c.confdeltype = 'c'
                AND c.conrelid IN ('inventory.stock_movements'::regclass, 'inventory.stock_batches'::regclass,
                                   'inventory.sales'::regclass, 'inventory.stock_counts'::regclass)));

-- ---------------------------------------------------------------------------
-- 5. The exception cannot be forced from outside its narrow scope
-- ---------------------------------------------------------------------------
SET LOCAL waypoint.allow_simulated_delete = 'on';
SET LOCAL waypoint.simulated_delete_run = '00000000-0000-4000-8000-0000000000a1';
CALL pg_temp.expect_fail('with the setting on, a real row is still refused', $$DELETE FROM inventory.stock_movements WHERE id = 9000001$$);
CALL pg_temp.expect_fail('with the setting on, a row of another run is still refused', $$DELETE FROM inventory.stock_movements WHERE id = 9000006$$);
CALL pg_temp.expect_fail('with the setting on, a real row of a real run is still refused', $$DELETE FROM inventory.stock_movements WHERE id = 9000007$$);
CALL pg_temp.expect_fail('with the setting on, a real observation is still refused', $$DELETE FROM ai.observations WHERE id = 9100001$$);
CALL pg_temp.expect_fail('with the setting on, an observation of another run is still refused', $$DELETE FROM ai.observations WHERE id = 9100003$$);
SET LOCAL waypoint.allow_simulated_delete = 'off';
SET LOCAL waypoint.simulated_delete_run = '';

-- ---------------------------------------------------------------------------
-- 6. delete_simulation_run refuses what it must
-- ---------------------------------------------------------------------------
CALL pg_temp.expect_fail('delete_simulation_run on a run recorded as real data',
  $$SELECT inventory.delete_simulation_run('00000000-0000-4000-8000-0000000000c1')$$);
CALL pg_temp.expect_fail('delete_simulation_run on an unknown run',
  $$SELECT inventory.delete_simulation_run('00000000-0000-4000-8000-00000000ffff')$$, 'P0002');
SELECT pg_temp.expect_true('refused calls removed nothing',
  (SELECT movements FROM zt_before) = (SELECT count(*) FROM inventory.stock_movements WHERE id BETWEEN 9000001 AND 9000007)
  AND (SELECT observations FROM zt_before) = (SELECT count(*) FROM ai.observations WHERE id BETWEEN 9100001 AND 9100003));

-- ---------------------------------------------------------------------------
-- 7. delete_simulation_run removes the simulated rows of that run only
-- ---------------------------------------------------------------------------
CREATE TEMP TABLE zt_result AS SELECT inventory.delete_simulation_run('00000000-0000-4000-8000-0000000000a1') AS r;
SELECT r FROM zt_result \gset
\echo delete_simulation_run returned :r

SELECT pg_temp.expect_true('run A: stock_movements gone',
  NOT EXISTS (SELECT 1 FROM inventory.stock_movements WHERE simulation_run_id = '00000000-0000-4000-8000-0000000000a1'));
SELECT pg_temp.expect_true('run A: observations, batches, sales, lines, counts and the run row gone',
  NOT EXISTS (SELECT 1 FROM ai.observations WHERE simulation_run_id = '00000000-0000-4000-8000-0000000000a1')
  AND NOT EXISTS (SELECT 1 FROM inventory.stock_batches WHERE simulation_run_id = '00000000-0000-4000-8000-0000000000a1')
  AND NOT EXISTS (SELECT 1 FROM inventory.sales WHERE simulation_run_id = '00000000-0000-4000-8000-0000000000a1')
  AND NOT EXISTS (SELECT 1 FROM inventory.sale_lines WHERE id = '30000000-0000-4000-8000-0000000000a1')
  AND NOT EXISTS (SELECT 1 FROM inventory.stock_counts WHERE simulation_run_id = '00000000-0000-4000-8000-0000000000a1')
  AND NOT EXISTS (SELECT 1 FROM inventory.simulation_runs WHERE id = '00000000-0000-4000-8000-0000000000a1'));
SELECT pg_temp.expect_true('run A: the insight that cited its observation is gone, run B insight kept',
  NOT EXISTS (SELECT 1 FROM ai.insights WHERE id = '40000000-0000-4000-8000-0000000000a1')
  AND EXISTS (SELECT 1 FROM ai.insights WHERE id = '40000000-0000-4000-8000-0000000000b1'));
SELECT pg_temp.expect_true('real rows untouched (movement, observation, batch, real run and its movement)',
  EXISTS (SELECT 1 FROM inventory.stock_movements WHERE id = 9000001)
  AND EXISTS (SELECT 1 FROM inventory.stock_movements WHERE id = 9000005)
  AND EXISTS (SELECT 1 FROM inventory.stock_movements WHERE id = 9000007)
  AND EXISTS (SELECT 1 FROM ai.observations WHERE id = 9100001)
  AND EXISTS (SELECT 1 FROM inventory.stock_batches WHERE id = '10000000-0000-4000-8000-000000000001')
  AND EXISTS (SELECT 1 FROM inventory.simulation_runs WHERE id = '00000000-0000-4000-8000-0000000000c1'));
SELECT pg_temp.expect_true('run B untouched (movement, observation, batch, run row)',
  EXISTS (SELECT 1 FROM inventory.stock_movements WHERE id = 9000006)
  AND EXISTS (SELECT 1 FROM ai.observations WHERE id = 9100003)
  AND EXISTS (SELECT 1 FROM inventory.stock_batches WHERE id = '10000000-0000-4000-8000-0000000000b1')
  AND EXISTS (SELECT 1 FROM inventory.simulation_runs WHERE id = '00000000-0000-4000-8000-0000000000b1'));
SELECT pg_temp.expect_true('stock_levels: item with no ledger left is removed, mixed item recomputed from the remaining ledger, others unchanged',
  NOT EXISTS (SELECT 1 FROM inventory.stock_levels WHERE outlet_id = 'ZT-OUT' AND product_id = 'FR-ZT-SIM')
  AND (SELECT on_hand_each FROM inventory.stock_levels WHERE outlet_id = 'ZT-OUT' AND product_id = 'FR-ZT-MIX') = 5
  AND (SELECT on_hand_each FROM inventory.stock_levels WHERE outlet_id = 'ZT-OUT' AND product_id = 'FR-ZT-SIMB') = 10
  AND (SELECT on_hand_each FROM inventory.stock_levels WHERE outlet_id = 'ZT-OUT' AND product_id = 'FR-ZT-REAL') = 10);

-- The exception switches itself off again: a plain delete is refused right after.
SELECT pg_temp.expect_true('the delete setting is off again after the call',
  coalesce(current_setting('waypoint.allow_simulated_delete', true), 'off') <> 'on');
CALL pg_temp.expect_fail('after the call, a run B movement still cannot be deleted', $$DELETE FROM inventory.stock_movements WHERE id = 9000006$$);
CALL pg_temp.expect_fail('after the call, a run B observation still cannot be deleted', $$DELETE FROM ai.observations WHERE id = 9100003$$);

-- Run B can be removed in its own right, and only that.
SELECT inventory.delete_simulation_run('00000000-0000-4000-8000-0000000000b1');
SELECT pg_temp.expect_true('run B removed on request; real rows still there',
  NOT EXISTS (SELECT 1 FROM inventory.stock_movements WHERE simulation_run_id = '00000000-0000-4000-8000-0000000000b1')
  AND EXISTS (SELECT 1 FROM inventory.stock_movements WHERE id = 9000001)
  AND EXISTS (SELECT 1 FROM ai.observations WHERE id = 9100001));

ROLLBACK;
\echo append_only.sql: all checks passed (transaction rolled back)
