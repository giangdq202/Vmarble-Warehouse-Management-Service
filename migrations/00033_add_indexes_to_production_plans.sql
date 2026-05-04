-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_production_plans_status ON production_plans (status);
CREATE INDEX IF NOT EXISTS idx_production_plans_deadline ON production_plans (deadline);
CREATE INDEX IF NOT EXISTS idx_production_plans_created_at ON production_plans (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_production_plans_code_trgm ON production_plans USING gin (code gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_purchase_orders_code_trgm ON purchase_orders USING gin (code gin_trgm_ops);

-- +goose Down
DROP INDEX IF EXISTS idx_purchase_orders_code_trgm;
DROP INDEX IF EXISTS idx_production_plans_code_trgm;
DROP INDEX IF EXISTS idx_production_plans_created_at;
DROP INDEX IF EXISTS idx_production_plans_deadline;
DROP INDEX IF EXISTS idx_production_plans_status;
