-- +goose Up
-- BR-D07: every container line transfer is recorded with a mandatory reason.
-- BR-D16/D17: is_cross_plan distinguishes worker-ok (false) from planner-required (true).

CREATE TABLE container_transfer_audit (
    id                  UUID        PRIMARY KEY,
    source_container_id UUID        NOT NULL REFERENCES containers(id),
    target_container_id UUID        NOT NULL REFERENCES containers(id),
    line_id             UUID        NOT NULL,   -- original line id on the source container
    sku_id              UUID        NOT NULL REFERENCES skus(id),
    qty_transferred     INT         NOT NULL CHECK (qty_transferred > 0),
    reason              TEXT        NOT NULL,
    is_cross_plan       BOOLEAN     NOT NULL DEFAULT false,
    actor_id            UUID        NOT NULL,
    actor_role          TEXT        NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_cta_source ON container_transfer_audit(source_container_id);
CREATE INDEX idx_cta_target ON container_transfer_audit(target_container_id);

-- +goose Down
DROP TABLE IF EXISTS container_transfer_audit;
