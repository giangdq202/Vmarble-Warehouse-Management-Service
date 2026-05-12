-- +goose Up
CREATE TABLE bom_variant_components (
    id                UUID        PRIMARY KEY,
    variant_id        UUID        NOT NULL REFERENCES bom_variants(id) ON DELETE CASCADE,
    material_id       UUID        NOT NULL REFERENCES materials(id),
    material_type     TEXT        NOT NULL,
    quantity_per_unit NUMERIC     NOT NULL,
    unit              TEXT        NOT NULL
);

CREATE INDEX idx_bom_variant_components_variant ON bom_variant_components(variant_id);

-- Backfill: create one DEFAULT variant per SKU that already has BOM components.
INSERT INTO bom_variants (id, sku_id, variant_code, name, is_default, created_at)
SELECT gen_random_uuid(), sku_id, 'DEFAULT', 'Default', true, now()
FROM (SELECT DISTINCT sku_id FROM bom_components) sub;

-- Copy existing components into the new DEFAULT variants.
INSERT INTO bom_variant_components (id, variant_id, material_id, material_type, quantity_per_unit, unit)
SELECT gen_random_uuid(), bv.id, bc.material_id, bc.material_type, bc.quantity_per_unit, bc.unit
FROM bom_components bc
JOIN bom_variants bv ON bv.sku_id = bc.sku_id AND bv.variant_code = 'DEFAULT';

-- +goose Down
DELETE FROM bom_variants WHERE variant_code = 'DEFAULT';
DROP INDEX IF EXISTS idx_bom_variant_components_variant;
DROP TABLE IF EXISTS bom_variant_components;
