-- +goose Up
ALTER TABLE remnants
    ADD COLUMN supplier_code TEXT,
    ADD COLUMN lot_batch TEXT,
    ADD COLUMN grain_pattern TEXT,
    ADD COLUMN quality_grade TEXT,
    ADD COLUMN bounding_box_length_mm INT,
    ADD COLUMN bounding_box_width_mm INT,
    ADD COLUMN bin_location_id UUID REFERENCES storage_locations(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE remnants
    DROP COLUMN IF EXISTS bin_location_id,
    DROP COLUMN IF EXISTS bounding_box_width_mm,
    DROP COLUMN IF EXISTS bounding_box_length_mm,
    DROP COLUMN IF EXISTS quality_grade,
    DROP COLUMN IF EXISTS grain_pattern,
    DROP COLUMN IF EXISTS lot_batch,
    DROP COLUMN IF EXISTS supplier_code;
