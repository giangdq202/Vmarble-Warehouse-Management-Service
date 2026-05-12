-- +goose Up
CREATE TABLE bom_variants (
    id            UUID        PRIMARY KEY,
    sku_id        UUID        NOT NULL REFERENCES skus(id) ON DELETE CASCADE,
    variant_code  TEXT        NOT NULL,
    name          TEXT        NOT NULL,
    is_default    BOOL        NOT NULL DEFAULT false,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (sku_id, variant_code)
);

CREATE INDEX idx_bom_variants_sku ON bom_variants(sku_id);

-- +goose Down
DROP INDEX IF EXISTS idx_bom_variants_sku;
DROP TABLE IF EXISTS bom_variants;
