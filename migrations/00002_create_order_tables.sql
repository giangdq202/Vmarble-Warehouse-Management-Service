-- +goose Up
CREATE TABLE purchase_orders (
    id UUID PRIMARY KEY,
    code TEXT NOT NULL UNIQUE,
    expected_delivery DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE po_line_items (
    id UUID PRIMARY KEY,
    po_id UUID NOT NULL REFERENCES purchase_orders(id) ON DELETE CASCADE,
    sku_id UUID NOT NULL REFERENCES skus(id) ON DELETE CASCADE,
    quantity INT NOT NULL,
    selling_price_amount BIGINT NOT NULL,
    selling_price_currency TEXT NOT NULL DEFAULT 'VND'
);

CREATE INDEX idx_po_line_items_po_id ON po_line_items(po_id);
CREATE INDEX idx_po_line_items_sku_id ON po_line_items(sku_id);

-- +goose Down
DROP TABLE IF EXISTS po_line_items;
DROP TABLE IF EXISTS purchase_orders;
