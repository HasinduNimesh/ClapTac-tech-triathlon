#!/usr/bin/env python3
"""Explicit local demo fixtures. Does not submit orders or publish plans."""
import argparse
import hashlib
import json
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]

def run(args, **kwargs):
    return subprocess.run(args, cwd=ROOT, check=True, text=True, **kwargs)

def sql(statement):
    run(['docker', 'compose', 'exec', '-T', 'postgres', 'psql', '-U', 'waypoint', '-d', 'waypoint', '-v', 'ON_ERROR_STOP=1'], input=statement)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['seed', 'suppression', 'run-due'])
    args = parser.parse_args()
    env = run(['docker', 'compose', 'exec', '-T', 'shared-service', 'printenv', 'ENVIRONMENT'], capture_output=True).stdout.strip()
    if env != 'local':
        raise SystemExit('Demo tools require ENVIRONMENT=local in the running shared-service.')
    if args.action == 'run-due':
        sql("""UPDATE shared.automations SET next_run_at=now()-interval '1 second'
          WHERE status='active' AND owner_id IN (SELECT id FROM shared.users WHERE identity_subject IN ('usr-store-manager','usr-dispatcher'));
        """)
        print('Active automations for the two demo accounts are due. The worker checks every 15 seconds. Refresh My automations / Notifications.')
        return
    # Go marshals struct fields in declaration order: keep this order identical to Prefill.
    pre = {'outletId': 'OUT034', 'orderUnits': 30, 'orderWeightKg': 150, 'orderVolumeM3': 0.42, 'temperatureRequirement': 'ambient'}
    raw = json.dumps(pre, separators=(',', ':'))
    pattern = 'order:' + hashlib.sha256(raw.encode()).hexdigest()
    no_count = 1 if args.action == 'suppression' else 0
    # Only this synthetic order pattern and the demo dispatch pattern are reset.
    sql(f"""
BEGIN;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM shared.users WHERE identity_subject='usr-store-manager') OR NOT EXISTS(SELECT 1 FROM shared.users WHERE identity_subject='usr-dispatcher') THEN RAISE EXCEPTION 'Run the normal identity bootstrap first'; END IF;
END $$;
INSERT INTO audit.events(event_id,actor_id,actor_type,action,resource_type,resource_id,new_state,timestamp,source)
SELECT 'automation-demo-order-'||g||'-'||current_date,u.id,'human','order.created','ORDER','DEMO-A3-ORDER-'||g,'{raw}'::jsonb,now()-g*interval '7 days','automation-demo'
FROM shared.users u CROSS JOIN generate_series(1,3) g WHERE u.identity_subject='usr-store-manager' ON CONFLICT DO NOTHING;
INSERT INTO audit.events(event_id,actor_id,actor_type,action,resource_type,resource_id,new_state,timestamp,source)
SELECT 'automation-demo-priority-'||g||'-'||current_date,u.id,'human','PRIORITY_REVIEW_MARKED','OUTLET','OUT047','{{"outletId":"OUT047"}}'::jsonb,now()-g*interval '2 days','automation-demo'
FROM shared.users u CROSS JOIN generate_series(1,3) g WHERE u.identity_subject='usr-dispatcher' ON CONFLICT DO NOTHING;
INSERT INTO shared.habit_feedback(owner_id,pattern_key,yes_count,no_count,suppressed)
SELECT id,CASE WHEN identity_subject='usr-store-manager' THEN '{pattern}' ELSE 'repeat-deferral-review' END,2,{no_count},false
FROM shared.users WHERE identity_subject IN ('usr-store-manager','usr-dispatcher')
ON CONFLICT(owner_id,pattern_key) DO UPDATE SET yes_count=2,no_count={no_count},suppressed=false;
DELETE FROM shared.habit_suggestions WHERE (owner_id,pattern_key) IN (SELECT id,CASE WHEN identity_subject='usr-store-manager' THEN '{pattern}' ELSE 'repeat-deferral-review' END FROM shared.users WHERE identity_subject IN ('usr-store-manager','usr-dispatcher'));
DELETE FROM shared.priority_reviews WHERE owner_id IN (SELECT id FROM shared.users WHERE identity_subject='usr-dispatcher') AND outlet_id IN ('OUT034','OUT047');
INSERT INTO shared.automation_preferences(owner_id,enabled) SELECT id,true FROM shared.users WHERE identity_subject IN ('usr-store-manager','usr-dispatcher') ON CONFLICT(owner_id) DO UPDATE SET enabled=true;
-- Historical projection fixtures only; no operational orders or plans are created.
DELETE FROM planning.automation_order_history WHERE order_id LIKE 'DEMO-A4-%';
INSERT INTO planning.automation_order_history(plan_id,order_id,outlet_id,delivery_date,state,recorded_at)
SELECT '00000000-0000-0000-0000-000000000001'::uuid,'DEMO-A4-STORE-'||g,'OUT034',current_date-(14+g),'deferred',now()-interval '21 days' FROM generate_series(1,2)g;
INSERT INTO planning.automation_order_history(plan_id,order_id,outlet_id,delivery_date,state,recorded_at)
SELECT '00000000-0000-0000-0000-000000000002'::uuid,'DEMO-A4-DISPATCH-'||g,'OUT047',current_date-(14+g),'deferred',now()-interval '21 days' FROM generate_series(1,2)g;
UPDATE planning.automation_history_start SET since=now()-interval '28 days';
INSERT INTO audit.events(event_id,actor_id,actor_type,action,resource_type,resource_id,new_state,source)
SELECT gen_random_uuid()::text,id,'human','AUTOMATION_DEMO_RESET','AUTOMATION','demo', '{{"seededYesCount":2,"seededNoCount":{no_count}}}'::jsonb,'automation-demo' FROM shared.users WHERE identity_subject IN ('usr-store-manager','usr-dispatcher');
COMMIT;
""")
    print('Synthetic history loaded: three weekly orders, three dispatcher review days, two earlier Yes answers.')
    print('The next Yes demonstrates the third-Yes A3 → A4 handoff.' if no_count == 0 else 'One earlier No is seeded; click No to demonstrate the second-No suppression.')
    print('Reload the browser. Demo fixtures are identified in My automations. Use only a disposable local demo database.')

if __name__ == '__main__':
    main()
