-- +goose Up
ALTER TABLE costing_records
    ADD COLUMN costing_type TEXT NOT NULL DEFAULT 'ACTUAL';

-- +goose Down
ALTER TABLE costing_records
    DROP COLUMN costing_type;
