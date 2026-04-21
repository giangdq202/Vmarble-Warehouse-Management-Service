-- +goose Up
CREATE TABLE inventory_audit_log (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type   TEXT NOT NULL,
    entity_id     UUID NOT NULL,
    action        TEXT NOT NULL,
    actor_id      UUID NOT NULL,
    from_location UUID,
    to_location   UUID,
    from_status   TEXT,
    to_status     TEXT,
    reason        TEXT,
    session_id    UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_audit_entity ON inventory_audit_log (entity_id, entity_type);
CREATE INDEX idx_audit_actor  ON inventory_audit_log (actor_id);

-- +goose Down
DROP TABLE IF EXISTS inventory_audit_log;
