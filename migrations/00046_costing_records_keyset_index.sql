-- +goose Up
CREATE INDEX IF NOT EXISTS idx_costing_records_created_at_id
    ON costing_records (created_at DESC, id DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_costing_records_created_at_id;
