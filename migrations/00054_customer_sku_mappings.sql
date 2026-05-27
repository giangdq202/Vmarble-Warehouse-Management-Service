-- +goose Up
-- customer_sku_mappings (#304). Bridges the SKU code shipped in a customer's
-- packing-list Excel with the internal catalog SKU id, so the Excel parser
-- (#301) can fail-fast on unmapped codes (BR-D09 / BR-CSM01).
--
-- BR-CSM02: composite PK enforces "one (customer, code) -> one sku".
-- BR-CSM03: ON DELETE CASCADE on customer_id — removing a customer drops
--           their mappings; sales onboards a fresh set on re-add.
-- BR-CSM04: ON DELETE RESTRICT on sku_id — refuses SKU deletion while any
--           mapping still references it; sales must unmap first. Pushed to
--           the DB instead of a service-layer guard so a misbehaving import
--           script cannot leave dangling mappings.

CREATE TABLE customer_sku_mappings (
    customer_id       UUID        NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
    customer_sku_code TEXT        NOT NULL,
    sku_id            UUID        NOT NULL REFERENCES skus(id)      ON DELETE RESTRICT,
    notes             TEXT,
    created_by        UUID        REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (customer_id, customer_sku_code)
);

CREATE INDEX idx_csm_sku ON customer_sku_mappings(sku_id);

-- +goose Down
DROP INDEX IF EXISTS idx_csm_sku;
DROP TABLE IF EXISTS customer_sku_mappings;
