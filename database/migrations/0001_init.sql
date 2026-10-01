-- Single PostgreSQL instance. Independently deployable services own a schema.
-- Cross-schema SQL is forbidden; use the owning service API.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS orders;
CREATE SCHEMA IF NOT EXISTS planning;
CREATE SCHEMA IF NOT EXISTS fleet;
CREATE SCHEMA IF NOT EXISTS loading;
CREATE SCHEMA IF NOT EXISTS delivery;
CREATE SCHEMA IF NOT EXISTS shared;
CREATE SCHEMA IF NOT EXISTS audit;

COMMENT ON SCHEMA orders IS 'Owned by order-service';
COMMENT ON SCHEMA planning IS 'Owned by planning-service';
COMMENT ON SCHEMA fleet IS 'Owned by fleet-service';
COMMENT ON SCHEMA loading IS 'Owned by loading-service';
COMMENT ON SCHEMA delivery IS 'Owned by delivery-service';
COMMENT ON SCHEMA shared IS 'Owned by shared-service (profiles, notifications)';
COMMENT ON SCHEMA audit IS 'Owned by shared-service (audit ingest)';
