-- +goose Up
-- WO PartialComplete + Carry-over (#292). Adds three nullable columns + two
-- CHECK constraints + one partial index. PARTIAL_COMPLETE itself is just a
-- new value of work_orders.status (TEXT, no enum/CHECK at DB level — the
-- domain.WorkOrderStatus.CanTransitionTo is the authority).
--
-- BR-P05/P06: actual_qty range enforced by chk_actual_qty.
-- BR-P07/P08: parent_wo_id FK gives carry-over chain traceability; SET NULL
--   on parent delete is intentional — losing the parent should not cascade-
--   delete the carry-over WO (history > FK strictness).
-- BR-P09/P10: read-only columns for the packing FG hook + costing module.

ALTER TABLE work_orders
    ADD COLUMN actual_qty       INT,
    ADD COLUMN parent_wo_id     UUID REFERENCES work_orders(id) ON DELETE SET NULL,
    ADD COLUMN shortfall_reason TEXT;

ALTER TABLE work_orders
    ADD CONSTRAINT chk_actual_qty
        CHECK (actual_qty IS NULL OR (actual_qty >= 0 AND actual_qty <= quantity));

ALTER TABLE work_orders
    ADD CONSTRAINT chk_shortfall_reason
        CHECK (shortfall_reason IS NULL
               OR shortfall_reason IN ('MATERIAL_SHORTAGE','DEFECT','TIME_SHORTAGE','OTHER'));

CREATE INDEX idx_wo_parent ON work_orders(parent_wo_id) WHERE parent_wo_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_wo_parent;
ALTER TABLE work_orders DROP CONSTRAINT IF EXISTS chk_shortfall_reason;
ALTER TABLE work_orders DROP CONSTRAINT IF EXISTS chk_actual_qty;
ALTER TABLE work_orders
    DROP COLUMN IF EXISTS shortfall_reason,
    DROP COLUMN IF EXISTS parent_wo_id,
    DROP COLUMN IF EXISTS actual_qty;
