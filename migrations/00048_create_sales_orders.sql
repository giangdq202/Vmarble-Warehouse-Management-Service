-- +goose Up
-- Daily counter for SO codes. Each (year, month, day) keeps its own monotonic
-- sequence so the human-readable code "SO20260525-001" is dense per-day rather
-- than borrowing a global sequence. The counter is mutated via INSERT … ON
-- CONFLICT DO UPDATE … RETURNING so concurrent inserts serialize on the row.
CREATE TABLE sales_order_code_counters (
    date_key  DATE PRIMARY KEY,
    last_seq  INT  NOT NULL DEFAULT 0
);

CREATE TABLE sales_orders (
    id                  UUID        PRIMARY KEY,
    code                TEXT        UNIQUE NOT NULL,
    customer_id         UUID        NOT NULL REFERENCES customers(id),
    -- Incoterm + ports + non-VND currency are required only at confirm time
    -- when country_code != 'VN' (BR-S05); the columns are nullable so DRAFT
    -- orders can be saved incrementally.
    incoterm            TEXT,
    port_of_loading     TEXT,
    port_of_discharge   TEXT,
    currency            TEXT        NOT NULL DEFAULT 'VND',
    status              TEXT        NOT NULL DEFAULT 'DRAFT',
    expected_ship_date  DATE,
    note                TEXT,
    created_by          UUID        NOT NULL REFERENCES users(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_so_status CHECK (status IN
        ('DRAFT','CONFIRMED','IN_PRODUCTION','PARTIALLY_SHIPPED','SHIPPED','CANCELLED')),
    CONSTRAINT chk_so_currency CHECK (currency IN ('VND','USD','EUR'))
);

CREATE INDEX idx_so_customer       ON sales_orders (customer_id);
CREATE INDEX idx_so_status_ship    ON sales_orders (status, expected_ship_date);

CREATE TABLE sales_order_lines (
    id                   UUID        PRIMARY KEY,
    sales_order_id       UUID        NOT NULL REFERENCES sales_orders(id) ON DELETE CASCADE,
    sku_id               UUID        NOT NULL REFERENCES skus(id),
    qty_ordered          INT         NOT NULL CHECK (qty_ordered > 0),
    qty_planned          INT         NOT NULL DEFAULT 0,
    qty_shipped          INT         NOT NULL DEFAULT 0,
    -- Stored in the smallest unit of unit_price_currency (e.g. cents/đồng).
    -- Line currency may differ from sales_orders.currency by design — sample
    -- accessories priced in USD inside a VND export order, etc. Reports
    -- convert per-line via fx_rates (#295).
    unit_price_amount    BIGINT      NOT NULL CHECK (unit_price_amount >= 0),
    unit_price_currency  TEXT        NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (sales_order_id, sku_id),
    CONSTRAINT chk_qty_planned_le_ordered CHECK (qty_planned <= qty_ordered),
    CONSTRAINT chk_qty_shipped_le_planned CHECK (qty_shipped <= qty_planned),
    CONSTRAINT chk_so_line_currency       CHECK (unit_price_currency IN ('VND','USD','EUR'))
);

CREATE INDEX idx_so_lines_sku ON sales_order_lines (sku_id);
CREATE INDEX idx_so_lines_so  ON sales_order_lines (sales_order_id);

-- +goose Down
DROP INDEX IF EXISTS idx_so_lines_so;
DROP INDEX IF EXISTS idx_so_lines_sku;
DROP TABLE IF EXISTS sales_order_lines;
DROP INDEX IF EXISTS idx_so_status_ship;
DROP INDEX IF EXISTS idx_so_customer;
DROP TABLE IF EXISTS sales_orders;
DROP TABLE IF EXISTS sales_order_code_counters;
