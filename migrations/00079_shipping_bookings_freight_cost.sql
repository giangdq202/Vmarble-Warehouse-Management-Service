-- +goose Up
ALTER TABLE shipping_bookings
    ADD COLUMN freight_cost_cents    BIGINT       NULL,
    ADD COLUMN freight_cost_currency VARCHAR(3)   NULL,
    ADD CONSTRAINT chk_freight_cost_both_or_neither
        CHECK ((freight_cost_cents IS NULL) = (freight_cost_currency IS NULL));

-- +goose Down
ALTER TABLE shipping_bookings
    DROP CONSTRAINT IF EXISTS chk_freight_cost_both_or_neither,
    DROP COLUMN IF EXISTS freight_cost_cents,
    DROP COLUMN IF EXISTS freight_cost_currency;
