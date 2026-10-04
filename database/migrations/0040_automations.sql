BEGIN;
CREATE TABLE shared.automation_preferences (
 owner_id text PRIMARY KEY REFERENCES shared.users(id), enabled boolean NOT NULL DEFAULT true
);
CREATE TABLE shared.habit_feedback (
 owner_id text NOT NULL REFERENCES shared.users(id), pattern_key text NOT NULL,
 yes_count integer NOT NULL DEFAULT 0, no_count integer NOT NULL DEFAULT 0,
 suppressed boolean NOT NULL DEFAULT false, last_response timestamptz,
 PRIMARY KEY(owner_id,pattern_key)
);
CREATE TABLE shared.habit_suggestions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id text NOT NULL REFERENCES shared.users(id),
 pattern_key text NOT NULL, occurrence_key text NOT NULL, payload jsonb NOT NULL,
 response text CHECK(response IN ('yes','no','never')), created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id,pattern_key,occurrence_key)
);
CREATE TABLE shared.automation_previews (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id text NOT NULL REFERENCES shared.users(id), definition jsonb NOT NULL, result jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE shared.automations (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id text NOT NULL REFERENCES shared.users(id),
 definition jsonb NOT NULL, status text NOT NULL CHECK(status IN ('active','paused','deleted')),
 next_run_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 activation_key uuid NOT NULL, UNIQUE(owner_id,activation_key)
);
CREATE TABLE shared.automation_runs (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), automation_id uuid NOT NULL REFERENCES shared.automations(id),
 owner_id text NOT NULL REFERENCES shared.users(id), scheduled_at timestamptz NOT NULL,
 status text NOT NULL, result jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(automation_id,scheduled_at)
);
CREATE TABLE shared.automation_inbox (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id text NOT NULL REFERENCES shared.users(id),
 title text NOT NULL, payload jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE shared.priority_reviews (
 owner_id text NOT NULL REFERENCES shared.users(id), outlet_id text NOT NULL REFERENCES shared.outlets(id),
 expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(owner_id,outlet_id)
);
CREATE INDEX automations_due ON shared.automations(next_run_at) WHERE status='active';
CREATE INDEX automation_inbox_owner ON shared.automation_inbox(owner_id,created_at DESC);

-- Planning owns its historical projection. It is exposed only via a scoped API.
CREATE TABLE planning.automation_history_start (singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton), since timestamptz NOT NULL DEFAULT now());
INSERT INTO planning.automation_history_start(singleton) VALUES(true);
CREATE TABLE planning.automation_order_history (
 id bigserial PRIMARY KEY, plan_id uuid NOT NULL, order_id text NOT NULL, outlet_id text NOT NULL DEFAULT '',
 delivery_date date NOT NULL, state text NOT NULL, recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX automation_order_history_lookup ON planning.automation_order_history(order_id,delivery_date DESC,recorded_at DESC,id DESC);
CREATE FUNCTION planning.capture_automation_order() RETURNS trigger AS $$
DECLARE row_order text; row_plan uuid; row_outlet text := ''; row_state text; row_date date;
BEGIN
 IF TG_OP='DELETE' THEN row_order:=OLD.order_id; row_plan:=OLD.plan_id; row_state:='cleared';
 ELSE row_order:=NEW.order_id; row_plan:=NEW.plan_id;
   IF TG_TABLE_NAME='deferrals' THEN row_state:='deferred'; ELSE row_state:='allocated'; END IF;
 END IF;
 IF TG_TABLE_NAME='deferrals' THEN
   IF TG_OP='DELETE' THEN row_outlet:=COALESCE(OLD.outlet_id,''); ELSE row_outlet:=COALESCE(NEW.outlet_id,''); END IF;
 END IF;
 SELECT delivery_date INTO row_date FROM planning.plans WHERE id=row_plan;
 IF row_date IS NOT NULL THEN
 INSERT INTO planning.automation_order_history(plan_id,order_id,outlet_id,delivery_date,state)
 VALUES(row_plan,row_order,row_outlet,row_date,row_state);
 END IF;
 RETURN NULL;
END; $$ LANGUAGE plpgsql;
CREATE TRIGGER automation_deferral_history AFTER INSERT OR UPDATE OR DELETE ON planning.deferrals FOR EACH ROW EXECUTE FUNCTION planning.capture_automation_order();
CREATE TRIGGER automation_allocation_history AFTER INSERT OR UPDATE OR DELETE ON planning.allocations FOR EACH ROW EXECUTE FUNCTION planning.capture_automation_order();
INSERT INTO planning.automation_order_history(plan_id,order_id,outlet_id,delivery_date,state)
 SELECT d.plan_id,d.order_id,COALESCE(d.outlet_id,''),p.delivery_date,'deferred' FROM planning.deferrals d JOIN planning.plans p ON p.id=d.plan_id;
INSERT INTO planning.automation_order_history(plan_id,order_id,delivery_date,state)
 SELECT a.plan_id,a.order_id,p.delivery_date,'allocated' FROM planning.allocations a JOIN planning.plans p ON p.id=a.plan_id;
COMMIT;
