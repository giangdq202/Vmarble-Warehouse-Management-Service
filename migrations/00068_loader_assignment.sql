-- +goose Up
ALTER TABLE containers ADD COLUMN loader_id UUID REFERENCES users(id);

CREATE TABLE container_loader_log (
    id             UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    container_id   UUID        NOT NULL REFERENCES containers(id),
    from_loader_id UUID        REFERENCES users(id),
    to_loader_id   UUID        REFERENCES users(id),
    reason         TEXT,
    assigned_by    UUID        NOT NULL REFERENCES users(id),
    assigned_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ON container_loader_log (container_id, assigned_at DESC);

-- +goose Down
DROP TABLE IF EXISTS container_loader_log;
ALTER TABLE containers DROP COLUMN IF EXISTS loader_id;
