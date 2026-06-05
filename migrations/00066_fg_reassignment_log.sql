-- +goose Up
CREATE TABLE fg_reassignment_log (
    id              UUID        PRIMARY KEY,
    fg_id           UUID        NOT NULL REFERENCES fg_pool(id),
    from_sol_id     UUID        REFERENCES sales_order_lines(id),
    to_sol_id       UUID        NOT NULL REFERENCES sales_order_lines(id),
    actor_id        UUID        NOT NULL REFERENCES users(id),
    reason          TEXT        NOT NULL,
    reassigned_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_fg_reassignment_fg ON fg_reassignment_log (fg_id, reassigned_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_fg_reassignment_fg;
DROP TABLE IF EXISTS fg_reassignment_log;
