-- +goose Up
CREATE TABLE inventory_lots (
    id UUID PRIMARY KEY,
    material_id UUID NOT NULL,
    quantity INT NOT NULL,
    cost_per_sheet_amount BIGINT NOT NULL,
    cost_per_sheet_currency TEXT NOT NULL,
    supplier_ref TEXT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE board_sheets (
    id UUID PRIMARY KEY,
    lot_id UUID NOT NULL REFERENCES inventory_lots(id) ON DELETE CASCADE,
    length_mm INT NOT NULL,
    width_mm INT NOT NULL,
    cost_amount BIGINT NOT NULL,
    cost_currency TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'AVAILABLE',
    issued_to_wo_id UUID
);

CREATE TABLE cutting_records (
    id UUID PRIMARY KEY,
    sheet_id UUID REFERENCES board_sheets(id) ON DELETE SET NULL,
    remnant_source_id UUID,
    work_order_id UUID NOT NULL,
    sku_id UUID NOT NULL,
    used_length_mm INT NOT NULL,
    used_width_mm INT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE remnants (
    id UUID PRIMARY KEY,
    parent_board_id UUID NOT NULL REFERENCES board_sheets(id) ON DELETE CASCADE,
    parent_remnant_id UUID REFERENCES remnants(id) ON DELETE SET NULL,
    length_mm INT NOT NULL,
    width_mm INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'AVAILABLE',
    allocated_to_wo_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS remnants;
DROP TABLE IF EXISTS cutting_records;
DROP TABLE IF EXISTS board_sheets;
DROP TABLE IF EXISTS inventory_lots;
