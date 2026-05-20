-- +goose Up
-- Replace (barcode_id, scanned_at DESC) with a composite that includes id as
-- a tie-breaker so keyset pagination over (scanned_at, id) is a pure Index
-- Scan with no Sort node, even when many scans share the same scanned_at
-- (a single barcode can collect dozens of scans within one millisecond
-- during burst-scan flows).
--
-- ASC matches the existing handler order ("show scans chronologically").
DROP INDEX IF EXISTS idx_scan_events_barcode_scanned_at;
CREATE INDEX idx_scan_events_barcode_scanned_at_id
    ON scan_events (barcode_id, scanned_at, id);

-- +goose Down
DROP INDEX IF EXISTS idx_scan_events_barcode_scanned_at_id;
CREATE INDEX idx_scan_events_barcode_scanned_at
    ON scan_events (barcode_id, scanned_at DESC);
