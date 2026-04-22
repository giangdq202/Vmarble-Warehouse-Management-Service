-- +goose Up
CREATE TABLE machines (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code                     TEXT NOT NULL,
    name                     TEXT NOT NULL,
    capacity_hours_per_shift NUMERIC(6,2) NOT NULL,
    is_active                BOOLEAN NOT NULL DEFAULT TRUE,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX idx_machines_code_active ON machines (code) WHERE is_active = TRUE;

-- +goose Down
DROP TABLE IF EXISTS machines;
