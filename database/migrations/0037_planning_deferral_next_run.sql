-- FR-53: let a deferral record carry the Dispatcher's expected retry date.
ALTER TABLE planning.deferrals ADD COLUMN IF NOT EXISTS next_run_target DATE;
