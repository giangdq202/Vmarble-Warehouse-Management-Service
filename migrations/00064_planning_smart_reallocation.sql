-- +goose Up
ALTER TABLE work_orders
    ADD COLUMN IF NOT EXISTS priority_boost BOOLEAN NOT NULL DEFAULT false;

CREATE TABLE IF NOT EXISTS wo_boost_log (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    wo_id       UUID        NOT NULL REFERENCES work_orders(id),
    reason      TEXT        NOT NULL,
    actor_id    UUID        NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_wo_boost_log_wo    ON wo_boost_log(wo_id);
CREATE INDEX IF NOT EXISTS idx_wo_boost_log_time  ON wo_boost_log(created_at DESC);

CREATE TABLE IF NOT EXISTS wo_preemption_log (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    from_wo_id  UUID        NOT NULL REFERENCES work_orders(id),
    to_wo_id    UUID        NOT NULL REFERENCES work_orders(id),
    material_id UUID        NOT NULL REFERENCES materials(id),
    freed_qty   NUMERIC(12, 3) NOT NULL CHECK (freed_qty > 0),
    reason      TEXT        NOT NULL,
    actor_id    UUID        NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_wo_preemption_from_wo ON wo_preemption_log(from_wo_id);
CREATE INDEX IF NOT EXISTS idx_wo_preemption_to_wo   ON wo_preemption_log(to_wo_id);
CREATE INDEX IF NOT EXISTS idx_wo_preemption_created ON wo_preemption_log(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_work_orders_priority_boost ON work_orders(priority_boost) WHERE priority_boost = true;

-- +goose Down
DROP INDEX IF EXISTS idx_work_orders_priority_boost;
DROP INDEX IF EXISTS idx_wo_preemption_created;
DROP INDEX IF EXISTS idx_wo_preemption_to_wo;
DROP INDEX IF EXISTS idx_wo_preemption_from_wo;
DROP TABLE IF EXISTS wo_preemption_log;
DROP INDEX IF EXISTS idx_wo_boost_log_time;
DROP INDEX IF EXISTS idx_wo_boost_log_wo;
DROP TABLE IF EXISTS wo_boost_log;
ALTER TABLE work_orders DROP COLUMN IF EXISTS priority_boost;
