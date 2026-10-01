INSERT INTO shared.outlets (id, brand, name) VALUES
    ('OUT034', 'Fresh', 'Waypoint Fresh OUT034'),
    ('OUT021', 'Style', 'Waypoint Style OUT021'),
    ('OUT088', 'Tech', 'Waypoint Tech OUT088')
ON CONFLICT (id) DO UPDATE SET brand = EXCLUDED.brand, name = EXCLUDED.name;
