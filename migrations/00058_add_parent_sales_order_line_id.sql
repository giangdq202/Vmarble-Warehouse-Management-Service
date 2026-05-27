-- +goose Up
-- Add parent_sales_order_line_id to sales_order_lines (#303 BR-D17). When a
-- loading_exception is approved with resolution=BACKORDER, the loading
-- exception module creates a carry-over sales_order_line for the missing
-- qty and stamps parent_sales_order_line_id with the original line. The
-- back-reference (loading_exceptions.carry_over_so_line_id) plus this
-- forward-reference together let accountants trace either direction
-- (which exception spawned this line / which line carried over which
-- exception).
ALTER TABLE sales_order_lines
    ADD COLUMN parent_sales_order_line_id UUID
        REFERENCES sales_order_lines(id) ON DELETE SET NULL;

CREATE INDEX idx_so_lines_parent
    ON sales_order_lines (parent_sales_order_line_id)
    WHERE parent_sales_order_line_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_so_lines_parent;
ALTER TABLE sales_order_lines DROP COLUMN IF EXISTS parent_sales_order_line_id;
