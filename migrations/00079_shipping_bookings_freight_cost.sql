-- +goose Up
ALTER TABLE shipping_bookings
    ADD COLUMN freight_cost_cents    BIGINT       NULL,
    ADD COLUMN freight_cost_currency VARCHAR(3)   NULL;

-- +goose Down
ALTER TABLE shipping_bookings
    DROP COLUMN IF EXISTS freight_cost_cents,
    DROP COLUMN IF EXISTS freight_cost_currency;
