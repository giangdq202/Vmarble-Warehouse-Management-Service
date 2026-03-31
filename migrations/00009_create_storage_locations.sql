-- +goose Up
-- Barcode format convention for shelf labels: {zone}-{rack}-{shelf} (e.g. A-01-03).
CREATE TABLE storage_locations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    zone TEXT NOT NULL,
    rack TEXT NOT NULL,
    shelf TEXT NOT NULL,
    label TEXT NOT NULL,
    barcode TEXT NOT NULL UNIQUE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (zone, rack, shelf)
);

CREATE INDEX idx_storage_locations_active
    ON storage_locations (is_active)
    WHERE is_active = TRUE;

-- +goose Down
DROP TABLE IF EXISTS storage_locations;
