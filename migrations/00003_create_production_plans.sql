-- +goose Up
CREATE TABLE production_plans (
    id UUID PRIMARY KEY,
    po_id UUID NOT NULL REFERENCES purchase_orders(id),
    status TEXT NOT NULL DEFAULT 'DRAFT',
    deadline DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE plan_items (
    id UUID PRIMARY KEY,
    plan_id UUID NOT NULL REFERENCES production_plans(id) ON DELETE CASCADE,
    sku_id UUID NOT NULL REFERENCES skus(id),
    quantity INT NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS plan_items;
DROP TABLE IF EXISTS production_plans;
