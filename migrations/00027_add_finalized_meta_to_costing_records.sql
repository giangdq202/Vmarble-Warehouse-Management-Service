-- +goose Up
ALTER TABLE costing_records
    ADD COLUMN IF NOT EXISTS finalized_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS finalized_by UUID;

-- +goose Down
ALTER TABLE costing_records
    DROP COLUMN IF EXISTS finalized_by,
    DROP COLUMN IF EXISTS finalized_at;
