-- +goose Up
CREATE TABLE container_overload_log (
    id             UUID PRIMARY KEY,
    container_id   UUID NOT NULL REFERENCES containers(id),
    line_id        UUID NOT NULL REFERENCES container_lines(id),
    projected_cbm  NUMERIC(10,4) NOT NULL,
    max_cbm        NUMERIC(10,4) NOT NULL,
    projected_kg   NUMERIC(12,3) NOT NULL,
    max_kg         NUMERIC(12,3) NOT NULL,
    actor_id       UUID NOT NULL REFERENCES users(id),
    reason         TEXT NOT NULL DEFAULT 'admin override',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_container_overload_log_container ON container_overload_log(container_id);

-- +goose Down
DROP TABLE IF EXISTS container_overload_log;
