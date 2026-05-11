-- +goose Up
-- worker_id is the subject of the labor entry — the person whose time was
-- recorded. actor_id stays as the recorder (foreman / cnc_manager who entered
-- the row via the UI). When worker_id is NULL the recorder is also the worker
-- (legacy rows from before this change).
ALTER TABLE labor_cost_entries
    ADD COLUMN worker_id UUID NULL REFERENCES users(id);

CREATE INDEX idx_labor_cost_entries_worker ON labor_cost_entries (worker_id);

-- +goose Down
DROP INDEX IF EXISTS idx_labor_cost_entries_worker;
ALTER TABLE labor_cost_entries DROP COLUMN IF EXISTS worker_id;
