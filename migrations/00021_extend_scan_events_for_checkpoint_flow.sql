-- +goose Up
ALTER TABLE scan_events
    ALTER COLUMN scanned_by TYPE UUID USING scanned_by::uuid,
    ADD COLUMN location TEXT,
    ADD COLUMN note TEXT;

CREATE INDEX idx_scan_events_barcode_scanned_at ON scan_events (barcode_id, scanned_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_scan_events_barcode_scanned_at;

ALTER TABLE scan_events
    DROP COLUMN IF EXISTS note,
    DROP COLUMN IF EXISTS location,
    ALTER COLUMN scanned_by TYPE TEXT USING scanned_by::text;
