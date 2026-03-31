-- +goose Up
ALTER TABLE board_sheets
    ADD COLUMN supplier_code TEXT,
    ADD COLUMN lot_batch TEXT,
    ADD COLUMN grain_pattern TEXT,
    ADD COLUMN quality_grade TEXT;

-- +goose Down
ALTER TABLE board_sheets
    DROP COLUMN IF EXISTS quality_grade,
    DROP COLUMN IF EXISTS grain_pattern,
    DROP COLUMN IF EXISTS lot_batch,
    DROP COLUMN IF EXISTS supplier_code;
