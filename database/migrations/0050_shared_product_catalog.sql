-- Product catalog (master data, owned by shared-service).
-- One products table for every brand, with a 1:1 extension table per brand
-- for the attributes only that brand needs. Other schemas refer to a
-- product by its text id (for example FR-MILK-1L) without a foreign key.
--
-- Units: a "pack" is what moves through ordering, loading and delivery
-- (crate, case, bag, carton) and matches orders.order_units. An "each" is one
-- sellable item on the shelf. units_per_pack converts between them.
-- Numbered 0050+ so it applies after the migrations pending in open PRs.

CREATE TABLE IF NOT EXISTS shared.product_categories (
    id TEXT PRIMARY KEY,
    parent_id TEXT REFERENCES shared.product_categories (id),
    brand TEXT NOT NULL CHECK (brand IN ('Fresh', 'Style', 'Tech')),
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
    sort_order INTEGER NOT NULL DEFAULT 0,
    CHECK (parent_id IS NULL OR parent_id <> id)
);

CREATE TABLE IF NOT EXISTS shared.products (
    id TEXT PRIMARY KEY CHECK (id ~ '^(FR|ST|TC)-[A-Z0-9-]{2,40}$'),
    sku TEXT NOT NULL UNIQUE,
    gtin TEXT NOT NULL UNIQUE CHECK (gtin ~ '^[0-9]{13}$'),
    brand TEXT NOT NULL CHECK (brand IN ('Fresh', 'Style', 'Tech')),
    category_id TEXT NOT NULL REFERENCES shared.product_categories (id),
    family TEXT NOT NULL CHECK (family ~ '^[a-z0-9 -]{2,40}$'),
    name TEXT NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    size TEXT NOT NULL DEFAULT '',
    each_name TEXT NOT NULL DEFAULT 'item',
    pack_name TEXT NOT NULL CHECK (pack_name IN ('bag', 'bale', 'box', 'bundle', 'carton', 'case', 'crate', 'pack', 'tray')),
    units_per_pack INTEGER NOT NULL CHECK (units_per_pack BETWEEN 1 AND 1000),
    pack_weight_kg NUMERIC(8,3) NOT NULL CHECK (pack_weight_kg > 0),
    pack_volume_m3 NUMERIC(8,4) NOT NULL CHECK (pack_volume_m3 > 0),
    temperature TEXT NOT NULL CHECK (temperature IN ('ambient', 'chilled')),
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'discontinued')),
    launched_on DATE NOT NULL DEFAULT CURRENT_DATE,
    version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS products_brand_family_idx ON shared.products (brand, family) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS products_category_idx ON shared.products (category_id);

CREATE TABLE IF NOT EXISTS shared.product_fresh (
    product_id TEXT PRIMARY KEY REFERENCES shared.products (id) ON DELETE CASCADE,
    perishable BOOLEAN NOT NULL,
    shelf_life_days INTEGER NOT NULL CHECK (shelf_life_days BETWEEN 1 AND 1095),
    storage_min_c NUMERIC(4,1),
    storage_max_c NUMERIC(4,1),
    CHECK (storage_min_c IS NULL OR storage_max_c IS NULL OR storage_min_c <= storage_max_c)
);

CREATE TABLE IF NOT EXISTS shared.product_style (
    product_id TEXT PRIMARY KEY REFERENCES shared.products (id) ON DELETE CASCADE,
    gender TEXT NOT NULL CHECK (gender IN ('men', 'women', 'kids', 'unisex')),
    size_range TEXT NOT NULL DEFAULT '',
    colours TEXT NOT NULL DEFAULT '',
    material TEXT NOT NULL DEFAULT '',
    season TEXT NOT NULL DEFAULT 'all_year' CHECK (season IN ('all_year', 'avurudu', 'school_term', 'festive', 'monsoon'))
);

CREATE TABLE IF NOT EXISTS shared.product_tech (
    product_id TEXT PRIMARY KEY REFERENCES shared.products (id) ON DELETE CASCADE,
    requires_serial BOOLEAN NOT NULL,
    high_value BOOLEAN NOT NULL,
    seal_required BOOLEAN NOT NULL DEFAULT false,
    warranty_months INTEGER NOT NULL DEFAULT 0 CHECK (warranty_months BETWEEN 0 AND 60)
);

-- Everyday names people use for a product ("yogurt", "parippu").
CREATE TABLE IF NOT EXISTS shared.product_aliases (
    product_id TEXT NOT NULL REFERENCES shared.products (id) ON DELETE CASCADE,
    alias TEXT NOT NULL CHECK (alias = lower(btrim(alias)) AND length(alias) BETWEEN 2 AND 60),
    PRIMARY KEY (product_id, alias)
);
CREATE INDEX IF NOT EXISTS product_aliases_alias_idx ON shared.product_aliases (alias);

-- Effective-dated retail price per sellable item, in Sri Lankan rupees.
CREATE TABLE IF NOT EXISTS shared.product_prices (
    product_id TEXT NOT NULL REFERENCES shared.products (id) ON DELETE CASCADE,
    price_list TEXT NOT NULL DEFAULT 'retail' CHECK (price_list IN ('retail', 'promotion')),
    currency TEXT NOT NULL DEFAULT 'LKR' CHECK (currency ~ '^[A-Z]{3}$'),
    unit_price NUMERIC(12,2) NOT NULL CHECK (unit_price > 0),
    effective_from DATE NOT NULL,
    effective_to DATE,
    PRIMARY KEY (product_id, price_list, effective_from),
    CHECK (effective_to IS NULL OR effective_to > effective_from)
);

-- Store range: which products each outlet stocks and its stock targets (in eaches).
CREATE TABLE IF NOT EXISTS shared.outlet_products (
    outlet_id TEXT NOT NULL REFERENCES shared.outlets (id) ON DELETE CASCADE,
    product_id TEXT NOT NULL REFERENCES shared.products (id),
    listed BOOLEAN NOT NULL DEFAULT true,
    listed_since DATE NOT NULL DEFAULT CURRENT_DATE,
    avg_daily_sales_each NUMERIC(10,2) NOT NULL DEFAULT 0 CHECK (avg_daily_sales_each >= 0),
    reorder_point_each INTEGER NOT NULL DEFAULT 0 CHECK (reorder_point_each >= 0),
    min_stock_each INTEGER NOT NULL DEFAULT 0 CHECK (min_stock_each >= 0),
    max_stock_each INTEGER NOT NULL DEFAULT 0 CHECK (max_stock_each >= min_stock_each),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (outlet_id, product_id)
);
CREATE INDEX IF NOT EXISTS outlet_products_product_idx ON shared.outlet_products (product_id);
