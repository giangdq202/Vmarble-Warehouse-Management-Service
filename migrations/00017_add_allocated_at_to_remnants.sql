-- +goose Up
-- Track when a remnant was allocated so the background task can auto-release
-- remnants that were never consumed after a configurable timeout (default 24h).
ALTER TABLE remnants ADD COLUMN allocated_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE remnants DROP COLUMN allocated_at;
