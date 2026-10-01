CREATE TABLE IF NOT EXISTS shared.planning_policy_versions (
    version INTEGER PRIMARY KEY CHECK (version > 0),
    cutoff_local_time TIME NOT NULL,
    deferral_weight_points INTEGER NOT NULL CHECK (deferral_weight_points BETWEEN 1 AND 60),
    max_deferral_count INTEGER NOT NULL CHECK (max_deferral_count BETWEEN 1 AND 30),
    max_unserved_days INTEGER NOT NULL CHECK (max_unserved_days BETWEEN 30 AND 730),
    max_trips_per_vehicle INTEGER NOT NULL CHECK (max_trips_per_vehicle BETWEEN 1 AND 2),
    created_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS shared.planning_policy_current (
    singleton BOOLEAN PRIMARY KEY DEFAULT true CHECK (singleton),
    version INTEGER NOT NULL REFERENCES shared.planning_policy_versions(version)
);
INSERT INTO shared.planning_policy_versions(version,cutoff_local_time,deferral_weight_points,max_deferral_count,max_unserved_days,max_trips_per_vehicle,created_by)
VALUES (1,'16:00',14,12,365,2,'system') ON CONFLICT(version) DO NOTHING;
INSERT INTO shared.planning_policy_current(singleton,version) VALUES(true,1) ON CONFLICT(singleton) DO NOTHING;
CREATE OR REPLACE FUNCTION shared.reject_policy_version_mutation() RETURNS trigger AS $$
BEGIN RAISE EXCEPTION 'planning policy versions are immutable'; END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS planning_policy_versions_append_only ON shared.planning_policy_versions;
CREATE TRIGGER planning_policy_versions_append_only BEFORE UPDATE OR DELETE ON shared.planning_policy_versions
FOR EACH ROW EXECUTE FUNCTION shared.reject_policy_version_mutation();
