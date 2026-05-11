-- +goose Up
CREATE TABLE labor_cost_entries (
    id              UUID        PRIMARY KEY,
    work_order_id   UUID        NOT NULL REFERENCES work_orders(id) ON DELETE CASCADE,
    stage           TEXT        NOT NULL CHECK (stage IN ('CNC', 'GRINDING', 'ASSEMBLY', 'POLISHING')),
    minutes         INTEGER     NOT NULL CHECK (minutes > 0),
    -- rate_per_hour is stored in the smallest currency unit (e.g. VND dong/hour).
    -- labor_cost = SUM(minutes * rate_per_hour / 60) is computed in the costing module.
    rate_per_hour   BIGINT      NOT NULL CHECK (rate_per_hour >= 0),
    actor_id        UUID        NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_labor_cost_entries_wo ON labor_cost_entries (work_order_id);
CREATE INDEX idx_labor_cost_entries_wo_stage ON labor_cost_entries (work_order_id, stage);

-- +goose Down
DROP INDEX IF EXISTS idx_labor_cost_entries_wo_stage;
DROP INDEX IF EXISTS idx_labor_cost_entries_wo;
DROP TABLE IF EXISTS labor_cost_entries;
