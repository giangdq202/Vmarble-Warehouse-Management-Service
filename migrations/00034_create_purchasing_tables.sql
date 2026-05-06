-- +goose Up
CREATE TABLE material_purchase_orders (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code        TEXT NOT NULL UNIQUE,
    material_id UUID NOT NULL REFERENCES materials(id),
    supplier    TEXT,
    ordered_at  TIMESTAMPTZ,
    received_at TIMESTAMPTZ,
    status      TEXT NOT NULL DEFAULT 'DRAFT',
    note        TEXT,
    created_by  UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE material_purchase_order_items (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    po_id               UUID NOT NULL REFERENCES material_purchase_orders(id) ON DELETE CASCADE,
    quantity            INT NOT NULL CHECK (quantity > 0),
    length_mm           INT NOT NULL CHECK (length_mm > 0),
    width_mm            INT NOT NULL CHECK (width_mm > 0),
    unit_cost_amount    BIGINT NOT NULL CHECK (unit_cost_amount >= 0),
    unit_cost_currency  TEXT NOT NULL DEFAULT 'VND',
    lot_id              UUID REFERENCES inventory_lots(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_mpo_material_id ON material_purchase_orders(material_id);
CREATE INDEX idx_mpo_status      ON material_purchase_orders(status);
CREATE INDEX idx_mpoi_po_id      ON material_purchase_order_items(po_id);

-- +goose Down
DROP TABLE IF EXISTS material_purchase_order_items;
DROP TABLE IF EXISTS material_purchase_orders;
