-- +goose Up
ALTER TABLE costing_records
    ADD COLUMN labor_cost_amount BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN labor_cost_currency TEXT NOT NULL DEFAULT 'VND';

-- +goose Down
ALTER TABLE costing_records
    DROP COLUMN IF EXISTS labor_cost_currency,
    DROP COLUMN IF EXISTS labor_cost_amount;