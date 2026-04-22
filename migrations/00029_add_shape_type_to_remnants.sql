-- +goose Up
ALTER TABLE remnants
    ADD COLUMN IF NOT EXISTS shape_type TEXT NOT NULL DEFAULT 'rectangle';

ALTER TABLE remnants
    DROP CONSTRAINT IF EXISTS chk_remnants_shape_type;

ALTER TABLE remnants
    ADD CONSTRAINT chk_remnants_shape_type CHECK (shape_type IN ('rectangle', 'irregular'));

-- +goose Down
ALTER TABLE remnants
    DROP CONSTRAINT IF EXISTS chk_remnants_shape_type;

ALTER TABLE remnants
    DROP COLUMN IF EXISTS shape_type;
