-- +goose Up
ALTER TABLE work_orders
    ADD COLUMN IF NOT EXISTS estimated_hours  NUMERIC(6,2),
    ADD COLUMN IF NOT EXISTS machine_slot_id  UUID REFERENCES machine_shift_slots(id) ON DELETE SET NULL;

CREATE INDEX idx_wo_slot ON work_orders (machine_slot_id) WHERE machine_slot_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_wo_slot;
ALTER TABLE work_orders
    DROP COLUMN IF EXISTS machine_slot_id,
    DROP COLUMN IF EXISTS estimated_hours;
