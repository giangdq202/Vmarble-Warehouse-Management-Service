-- +goose Up
-- +goose StatementBegin
CREATE TABLE scrap_sales (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  sale_date       DATE NOT NULL,
  material_id     UUID NOT NULL REFERENCES materials(id),
  quantity_kg     NUMERIC(12,3) NOT NULL CHECK (quantity_kg > 0),
  unit_price      BIGINT NOT NULL CHECK (unit_price >= 0),
  currency        VARCHAR(3) NOT NULL DEFAULT 'VND',
  total_amount    BIGINT GENERATED ALWAYS AS (CAST(quantity_kg * unit_price AS BIGINT)) STORED,
  buyer_name      TEXT,
  invoice_number  TEXT,
  notes           TEXT,
  created_by      UUID NOT NULL REFERENCES users(id),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_scrap_sales_date_material ON scrap_sales(sale_date DESC, material_id);
CREATE INDEX idx_scrap_sales_created_at ON scrap_sales(created_at DESC);

COMMENT ON TABLE scrap_sales IS 'BR-C05/C06: scrap sale revenue ledger, offsets waste cost in WasteReport';
COMMENT ON COLUMN scrap_sales.total_amount IS 'GENERATED column = quantity_kg * unit_price (BIGINT cast for Money compatibility)';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS scrap_sales;
-- +goose StatementEnd
