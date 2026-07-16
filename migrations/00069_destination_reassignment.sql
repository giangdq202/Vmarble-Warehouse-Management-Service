-- +goose Up
ALTER TABLE containers
    ADD COLUMN destination_code TEXT,
    ADD COLUMN destination_name TEXT;

CREATE TABLE container_route_change_log (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    container_id   UUID        NOT NULL REFERENCES containers(id),
    from_dc        TEXT,
    to_dc          TEXT        NOT NULL,
    from_dest      TEXT,
    to_dest        TEXT,
    reason         TEXT,
    actor_id       UUID        NOT NULL REFERENCES users(id),
    changed_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON container_route_change_log (container_id, changed_at DESC);

-- +goose Down
DROP TABLE IF EXISTS container_route_change_log;
ALTER TABLE containers
    DROP COLUMN IF EXISTS destination_code,
    DROP COLUMN IF EXISTS destination_name;
