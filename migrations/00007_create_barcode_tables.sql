-- +goose Up
CREATE TABLE barcodes (
    id UUID PRIMARY KEY,
    work_order_id UUID NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    sku_id UUID NOT NULL,
    po_id UUID NOT NULL,
    production_plan_id UUID NOT NULL,
    sku_code TEXT NOT NULL,
    sku_name TEXT NOT NULL,
    dimensions TEXT NOT NULL DEFAULT '',
    produced_date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE scan_events (
    id UUID PRIMARY KEY,
    barcode_id UUID NOT NULL REFERENCES barcodes(id) ON DELETE CASCADE,
    checkpoint TEXT NOT NULL,
    scanned_by TEXT NOT NULL,
    scanned_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS scan_events;
DROP TABLE IF EXISTS barcodes;
