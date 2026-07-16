-- +goose Up
ALTER TABLE purchase_orders
    ADD COLUMN source_rejection_id UUID NULL;

CREATE INDEX idx_po_source_rejection ON purchase_orders (source_rejection_id)
    WHERE source_rejection_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_po_source_rejection;
ALTER TABLE purchase_orders
    DROP COLUMN IF EXISTS source_rejection_id;
