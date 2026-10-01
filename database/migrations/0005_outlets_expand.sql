ALTER TABLE shared.outlets
    ADD COLUMN IF NOT EXISTS district TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS depot TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS dock_type TEXT NOT NULL DEFAULT 'normal',
    ADD COLUMN IF NOT EXISTS parking_constraint TEXT NOT NULL DEFAULT 'normal',
    ADD COLUMN IF NOT EXISTS mall_window BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS window_open_time TIME,
    ADD COLUMN IF NOT EXISTS window_close_time TIME;

CREATE TABLE IF NOT EXISTS shared.district_travel (
    from_district TEXT NOT NULL,
    to_district TEXT NOT NULL,
    km NUMERIC(10, 2) NOT NULL,
    minutes INTEGER NOT NULL,
    PRIMARY KEY (from_district, to_district)
);

CREATE TABLE IF NOT EXISTS shared.service_allowance (
    stop_kind TEXT PRIMARY KEY,
    minutes INTEGER NOT NULL
);
