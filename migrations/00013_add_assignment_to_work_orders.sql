-- +goose Up
ALTER TABLE work_orders
    ADD COLUMN assigned_to UUID        REFERENCES users(id),
    ADD COLUMN assigned_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE work_orders
    DROP COLUMN assigned_to,
    DROP COLUMN assigned_at;
