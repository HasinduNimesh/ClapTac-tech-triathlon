-- Approximate outlet locations for the operations and store tracking maps.
-- The official dataset has no coordinates, so each outlet is placed near its
-- district centre with a small deterministic offset (about ±2 km) unless an
-- exact latitude/longitude is set on the outlet. Responses flag approximate positions.
ALTER TABLE shared.outlets
    ADD COLUMN IF NOT EXISTS latitude DOUBLE PRECISION CHECK (latitude BETWEEN -90 AND 90),
    ADD COLUMN IF NOT EXISTS longitude DOUBLE PRECISION CHECK (longitude BETWEEN -180 AND 180);

CREATE TABLE IF NOT EXISTS shared.district_locations (
    district TEXT PRIMARY KEY,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL
);

INSERT INTO shared.district_locations (district, latitude, longitude) VALUES
    ('Colombo', 6.9271, 79.8612),
    ('Gampaha', 7.0840, 79.9939),
    ('Kalutara', 6.5854, 79.9607),
    ('Galle', 6.0535, 80.2210),
    ('Matara', 5.9549, 80.5550),
    ('Kurunegala', 7.4863, 80.3647),
    ('Puttalam', 8.0362, 79.8283),
    ('Kandy', 7.2906, 80.6337),
    ('Matale', 7.4675, 80.6234),
    ('Nuwara Eliya', 6.9497, 80.7891),
    ('Badulla', 6.9934, 81.0550),
    ('Kegalle', 7.2513, 80.3464)
ON CONFLICT (district) DO UPDATE SET latitude = EXCLUDED.latitude, longitude = EXCLUDED.longitude;
