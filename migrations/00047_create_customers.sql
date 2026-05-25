-- +goose Up
-- Sequence drives the numeric suffix in KH001 customer codes.
-- Each customer gets a code; the user can supply their own (legacy from
-- Excel/MISA), in which case we just check uniqueness. When blank we draw
-- the next sequence value and format as KH%03d. Gaps are tolerated by design
-- because Postgres sequences are not transactional.
CREATE SEQUENCE IF NOT EXISTS customer_code_seq START 1;

CREATE TABLE customers (
    id              UUID        PRIMARY KEY,
    code            TEXT        UNIQUE NOT NULL,
    name            TEXT        NOT NULL,
    country_code    TEXT,
    address         TEXT,
    contact_person  TEXT,
    contact_phone   TEXT,
    contact_email   TEXT,
    is_active       BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_customers_active ON customers (is_active) WHERE is_active = TRUE;

-- +goose Down
DROP INDEX IF EXISTS idx_customers_active;
DROP TABLE IF EXISTS customers;
DROP SEQUENCE IF EXISTS customer_code_seq;
