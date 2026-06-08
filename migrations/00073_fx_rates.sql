-- +goose Up
CREATE TABLE fx_rates (
    id             UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    currency       TEXT         NOT NULL,
    rate_to_vnd    NUMERIC(16,6) NOT NULL,
    effective_date DATE         NOT NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_fx_rate           UNIQUE (currency, effective_date),
    CONSTRAINT chk_fx_rate_positive CHECK (rate_to_vnd > 0),
    CONSTRAINT chk_fx_currency      CHECK (currency IN ('USD','EUR'))
);

-- costing_records gain optional SO-currency context so accountants can see
-- the equivalent foreign-currency cost for export orders.
ALTER TABLE costing_records
    ADD COLUMN so_currency    TEXT,
    ADD COLUMN fx_rate_to_vnd NUMERIC(16,6);

CREATE INDEX idx_fx_rates_currency_date ON fx_rates (currency, effective_date DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_fx_rates_currency_date;
ALTER TABLE costing_records
    DROP COLUMN IF EXISTS fx_rate_to_vnd,
    DROP COLUMN IF EXISTS so_currency;
DROP TABLE IF EXISTS fx_rates;
