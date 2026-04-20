-- +goose Up
ALTER TABLE scan_events
    ADD COLUMN IF NOT EXISTS device_id TEXT,
    ADD COLUMN IF NOT EXISTS device_name TEXT,
    ADD COLUMN IF NOT EXISTS shift TEXT;

ALTER TABLE scan_events
    DROP CONSTRAINT IF EXISTS chk_scan_events_device_id_len,
    DROP CONSTRAINT IF EXISTS chk_scan_events_device_name_len,
    DROP CONSTRAINT IF EXISTS chk_scan_events_shift_len;

ALTER TABLE scan_events
    ADD CONSTRAINT chk_scan_events_device_id_len CHECK (char_length(device_id) <= 64),
    ADD CONSTRAINT chk_scan_events_device_name_len CHECK (char_length(device_name) <= 120),
    ADD CONSTRAINT chk_scan_events_shift_len CHECK (char_length(shift) <= 40);

-- +goose Down
ALTER TABLE scan_events
    DROP CONSTRAINT IF EXISTS chk_scan_events_shift_len,
    DROP CONSTRAINT IF EXISTS chk_scan_events_device_name_len,
    DROP CONSTRAINT IF EXISTS chk_scan_events_device_id_len;

ALTER TABLE scan_events
    DROP COLUMN IF EXISTS shift,
    DROP COLUMN IF EXISTS device_name,
    DROP COLUMN IF EXISTS device_id;
