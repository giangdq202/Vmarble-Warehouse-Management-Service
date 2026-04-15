-- +goose Up
-- Backfill lot_batch on board_sheets from the parent inventory_lot's supplier_ref.
-- Rows created before the ReceiveStock service fix (which now populates lot_batch
-- automatically) were left with lot_batch = NULL, causing the UI to fall back to
-- displaying a UUID prefix instead of the human-readable supplier reference.
UPDATE board_sheets bs
SET lot_batch = il.supplier_ref
FROM inventory_lots il
WHERE bs.lot_id = il.id
  AND bs.lot_batch IS NULL
  AND il.supplier_ref IS NOT NULL
  AND il.supplier_ref <> '';

-- +goose Down
-- No-op: restoring NULL is not useful and would break already-in-use labels.
