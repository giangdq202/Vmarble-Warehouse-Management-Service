-- +goose Up
-- Sequence drives the numeric suffix in KH-YYYY-NNN codes.
-- Using a dedicated sequence ensures uniqueness across concurrent inserts.
CREATE SEQUENCE IF NOT EXISTS production_plan_code_seq START 1;

ALTER TABLE production_plans
    ADD COLUMN IF NOT EXISTS code TEXT NOT NULL DEFAULT '';

-- Back-fill existing rows so the column is never empty.
UPDATE production_plans
SET code = 'KH-' || TO_CHAR(created_at, 'YYYY') || '-' || LPAD(nextval('production_plan_code_seq')::TEXT, 3, '0')
WHERE code = '';

-- Now add the uniqueness constraint (safe after back-fill).
ALTER TABLE production_plans
    ADD CONSTRAINT production_plans_code_unique UNIQUE (code);

-- +goose Down
ALTER TABLE production_plans DROP CONSTRAINT IF EXISTS production_plans_code_unique;
ALTER TABLE production_plans DROP COLUMN IF EXISTS code;
DROP SEQUENCE IF EXISTS production_plan_code_seq;
