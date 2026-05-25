-- +goose Up
-- Lineage from work order back to its sales order line. Nullable because
-- legacy WOs (created before #289) and PO-only flows have no SO link.
-- The cross-SO reassignment (#315) and partial-complete carry-over (#292)
-- both rely on this column to trace lineage SO → WO → FG.
ALTER TABLE work_orders
    ADD COLUMN sales_order_line_id UUID REFERENCES sales_order_lines(id);

CREATE INDEX idx_wo_so_line
    ON work_orders (sales_order_line_id)
    WHERE sales_order_line_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_wo_so_line;
ALTER TABLE work_orders DROP COLUMN IF EXISTS sales_order_line_id;
