-- +goose Up
-- Composite index supports keyset pagination on cutting_records ordered by
-- (created_at DESC, id DESC). Without this, the list-history endpoint
-- falls back to a Sort node once the table grows past memory, and even
-- with sufficient memory it has to scan + discard offset rows.
--
-- The id tie-breaker matters: a CNC kiosk can fire many cuts within the
-- same millisecond during burst-cut flows; a row at the page boundary
-- would otherwise be returned twice or skipped depending on the Postgres
-- row return order.
CREATE INDEX IF NOT EXISTS idx_cutting_records_created_at_id
    ON cutting_records (created_at DESC, id DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_cutting_records_created_at_id;
