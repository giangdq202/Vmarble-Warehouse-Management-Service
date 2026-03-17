-- +goose Up
CREATE TABLE costing_records (
    id UUID PRIMARY KEY,
    work_order_id UUID UNIQUE NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    sku_id UUID NOT NULL,
    material_cost_amount BIGINT NOT NULL,
    material_cost_currency TEXT NOT NULL,
    auxiliary_cost_amount BIGINT NOT NULL,
    auxiliary_cost_currency TEXT NOT NULL,
    total_cost_amount BIGINT NOT NULL,
    total_cost_currency TEXT NOT NULL,
    finalized BOOL NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS costing_records;
