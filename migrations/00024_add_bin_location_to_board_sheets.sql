-- +goose Up
ALTER TABLE board_sheets
    ADD COLUMN IF NOT EXISTS bin_location_id UUID REFERENCES storage_locations(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE board_sheets
    DROP COLUMN IF EXISTS bin_location_id;
