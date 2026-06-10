-- +goose Up

-- Audit trail for admin work-order reassignments (BE #21).
CREATE TABLE wo_reassign_log (
    id              UUID PRIMARY KEY,
    work_order_id   UUID NOT NULL REFERENCES work_orders(id),
    from_user_id    UUID,
    to_user_id      UUID NOT NULL,
    reason          TEXT NOT NULL,
    actor_id        UUID NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_wo_reassign_log_work_order_id ON wo_reassign_log (work_order_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_wo_reassign_log_work_order_id;
DROP TABLE IF EXISTS wo_reassign_log;
