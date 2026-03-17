-- +goose Up
CREATE TABLE materials (
    id UUID PRIMARY KEY,
    type TEXT NOT NULL,
    name TEXT NOT NULL,
    unit TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE skus (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    length_mm INT NOT NULL,
    width_mm INT NOT NULL,
    requires_metal BOOL NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE bom_components (
    id UUID PRIMARY KEY,
    sku_id UUID NOT NULL REFERENCES skus(id) ON DELETE CASCADE,
    material_id UUID NOT NULL REFERENCES materials(id) ON DELETE CASCADE,
    material_type TEXT NOT NULL,
    quantity_per_unit NUMERIC NOT NULL,
    unit TEXT NOT NULL
);

CREATE INDEX idx_bom_components_sku_id ON bom_components(sku_id);

-- +goose Down
DROP TABLE IF EXISTS bom_components;
DROP TABLE IF EXISTS skus;
DROP TABLE IF EXISTS materials;
