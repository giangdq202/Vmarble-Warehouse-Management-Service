-- +goose Up
CREATE TABLE machine_shift_slots (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    machine_id     UUID NOT NULL REFERENCES machines(id) ON DELETE CASCADE,
    shift_date     DATE NOT NULL,
    shift_name     TEXT NOT NULL,
    capacity_hours NUMERIC(6,2) NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (machine_id, shift_date, shift_name)
);

CREATE INDEX idx_slots_machine_date ON machine_shift_slots (machine_id, shift_date);

-- +goose Down
DROP TABLE IF EXISTS machine_shift_slots;
