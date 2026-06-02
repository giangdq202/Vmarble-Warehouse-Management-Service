-- +goose Up
ALTER TABLE skus
    ADD COLUMN height_mm      INT,
    ADD COLUMN weight_kg      NUMERIC(10,3),
    ADD COLUMN hs_code        TEXT,
    ADD COLUMN cbm_per_unit   NUMERIC(8,4) GENERATED ALWAYS AS (
        CASE
            WHEN length_mm > 0 AND width_mm > 0 AND height_mm > 0
            THEN (length_mm::numeric * width_mm * height_mm) / 1000000000
            ELSE NULL
        END
    ) STORED;

CREATE INDEX idx_skus_hs_code ON skus(hs_code) WHERE hs_code IS NOT NULL;

CREATE TABLE sku_packing_units (
    sku_id          UUID         NOT NULL REFERENCES skus(id) ON DELETE CASCADE,
    unit            TEXT         NOT NULL CHECK (unit IN ('piece', 'set', 'carton')),
    pieces_per_unit INT          NOT NULL CHECK (pieces_per_unit >= 1),
    is_default      BOOLEAN      NOT NULL DEFAULT false,
    PRIMARY KEY (sku_id, unit)
);

CREATE INDEX idx_sku_packing_units_sku_id ON sku_packing_units(sku_id);

-- +goose Down
DROP INDEX IF EXISTS idx_sku_packing_units_sku_id;
DROP TABLE IF EXISTS sku_packing_units;
DROP INDEX IF EXISTS idx_skus_hs_code;
ALTER TABLE skus
    DROP COLUMN IF EXISTS cbm_per_unit,
    DROP COLUMN IF EXISTS hs_code,
    DROP COLUMN IF EXISTS weight_kg,
    DROP COLUMN IF EXISTS height_mm;
