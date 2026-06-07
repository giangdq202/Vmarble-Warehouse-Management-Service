-- +goose Up
-- Index to accelerate the daily aging query (scans only AVAILABLE rows by age).
CREATE INDEX IF NOT EXISTS idx_remnants_available_created_at
    ON remnants (created_at ASC)
    WHERE status = 'AVAILABLE';

-- +goose Down
DROP INDEX IF EXISTS idx_remnants_available_created_at;
