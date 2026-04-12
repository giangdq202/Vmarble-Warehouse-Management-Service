-- +goose Up
ALTER TABLE materials       ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE skus            ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE purchase_orders ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE inventory_lots  ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT true;
ALTER TABLE barcodes        ADD COLUMN is_active BOOLEAN NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE barcodes        DROP COLUMN is_active;
ALTER TABLE inventory_lots  DROP COLUMN is_active;
ALTER TABLE purchase_orders DROP COLUMN is_active;
ALTER TABLE skus            DROP COLUMN is_active;
ALTER TABLE materials       DROP COLUMN is_active;
