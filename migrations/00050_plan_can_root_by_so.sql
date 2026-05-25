-- +goose Up
-- Phase A pivot: a production plan can be rooted by EITHER a purchase order
-- (legacy/internal manufacturing) OR a sales order (Demo-2 customer-driven).
-- Exactly one of (po_id, sales_order_id) must be set on every row.
-- Pre-existing rows are PO-rooted by definition; the existing po_id values
-- already satisfy the new CHECK without backfill.
ALTER TABLE production_plans
    ALTER COLUMN po_id DROP NOT NULL,
    ADD COLUMN sales_order_id UUID REFERENCES sales_orders(id);

-- Exactly one root id must be present. Using XOR-style CHECK so the planner
-- can never create an orphan plan or a dual-rooted plan.
ALTER TABLE production_plans
    ADD CONSTRAINT chk_plan_root CHECK (
        (po_id IS NOT NULL AND sales_order_id IS NULL) OR
        (po_id IS NULL AND sales_order_id IS NOT NULL)
    );

CREATE INDEX idx_pp_sales_order
    ON production_plans (sales_order_id)
    WHERE sales_order_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_pp_sales_order;
ALTER TABLE production_plans DROP CONSTRAINT IF EXISTS chk_plan_root;
ALTER TABLE production_plans DROP COLUMN IF EXISTS sales_order_id;
-- Restoring NOT NULL is only safe if no SO-rooted rows remain. The Down
-- precondition is that all SO-rooted plans have been migrated/removed first.
ALTER TABLE production_plans ALTER COLUMN po_id SET NOT NULL;
