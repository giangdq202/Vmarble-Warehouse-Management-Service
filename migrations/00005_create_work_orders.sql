-- +goose Up
CREATE TABLE work_orders (
    id UUID PRIMARY KEY,
    plan_id UUID NOT NULL REFERENCES production_plans(id) ON DELETE CASCADE,
    sku_id UUID NOT NULL REFERENCES skus(id),
    quantity INT NOT NULL,
    status TEXT NOT NULL DEFAULT 'PLANNED',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE consumption_records (
    id UUID PRIMARY KEY,
    work_order_id UUID NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    material_id UUID NOT NULL REFERENCES materials(id),
    material_type TEXT NOT NULL,
    quantity NUMERIC NOT NULL,
    unit TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS consumption_records;
DROP TABLE IF EXISTS work_orders;
