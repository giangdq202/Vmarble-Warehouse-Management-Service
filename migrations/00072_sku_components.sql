-- +goose Up
CREATE TABLE sku_components (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    sku_id         UUID         NOT NULL REFERENCES skus(id) ON DELETE CASCADE,
    component_type TEXT         NOT NULL,
    cbm_per_unit   NUMERIC(8,4) NOT NULL DEFAULT 0,
    sort_order     INT          NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_sku_component UNIQUE (sku_id, component_type)
);

ALTER TABLE fg_pool
    ADD COLUMN component_type TEXT,
    ADD COLUMN unit_index     INT;

CREATE INDEX idx_sku_components_sku ON sku_components (sku_id);
CREATE INDEX idx_fg_pool_component  ON fg_pool (sku_id, unit_index) WHERE component_type IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_fg_pool_component;
DROP INDEX IF EXISTS idx_sku_components_sku;
ALTER TABLE fg_pool
    DROP COLUMN IF EXISTS unit_index,
    DROP COLUMN IF EXISTS component_type;
DROP TABLE IF EXISTS sku_components;
